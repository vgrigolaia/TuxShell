// Package onepassword reads secrets from 1Password with its command-line
// tool, op. Hosts store only op://Vault/Item/field references; the value is
// read in Go when a connection needs it and is never logged, stored or sent
// to the frontend.
//
// op signs in through the 1Password desktop app ("Integrate with 1Password
// CLI"), which asks the user to approve each new use.
package onepassword

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/vgrigolaia/tuxshell/internal/platform"
)

// readTimeout leaves time to approve the request in the 1Password app.
const readTimeout = 2 * time.Minute

// ErrNotInstalled is returned when the op tool can't be found.
var ErrNotInstalled = errors.New("the 1Password CLI (op) isn't installed")

// AgentSocket is where the 1Password desktop app's SSH agent listens.
func AgentSocket() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, agentPath)
}

// AgentRunning reports whether sock is a socket (the 1Password app is
// running with its SSH agent turned on).
func AgentRunning(sock string) bool {
	st, err := os.Stat(sock)
	return err == nil && st.Mode()&os.ModeSocket != 0
}

// ValidRef reports whether ref looks like op://Vault/Item[/section]/field,
// optionally with a ?query. Names may contain spaces, not control characters.
func ValidRef(ref string) bool {
	rest, ok := strings.CutPrefix(ref, "op://")
	if !ok || strings.IndexFunc(ref, unicode.IsControl) >= 0 {
		return false
	}
	path, _, _ := strings.Cut(rest, "?")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || len(parts) > 4 {
		return false
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return false
		}
	}
	return true
}

// Client runs op. The zero value finds op on PATH or in the usual places.
type Client struct {
	// Path is op's location; "" means look it up.
	Path string
}

// candidates are checked when op isn't on PATH, as happens for apps started
// from a desktop launcher or an AppImage.
var candidates = []string{"/usr/bin/op", "/usr/local/bin/op", "/opt/homebrew/bin/op", "/opt/1Password/op"}

// path finds op, on the host when TuxShell runs in a Flatpak.
func (c Client) path() (string, error) {
	if c.Path != "" {
		return c.Path, nil
	}
	list := candidates
	if home, err := os.UserHomeDir(); err == nil {
		list = append(list, filepath.Join(home, ".local", "bin", "op"), filepath.Join(home, "bin", "op"))
	}
	p, err := platform.LookHostPath("op", list...)
	if err != nil {
		return "", ErrNotInstalled
	}
	return p, nil
}

// Read returns the secret ref points to.
func (c Client) Read(ctx context.Context, ref string) (string, error) {
	if !ValidRef(ref) {
		return "", errors.New("not a 1Password reference like op://Vault/Item/field")
	}
	bin, err := c.path()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := platform.HostCommand(ctx, nil, "", bin, "read", "--no-newline", "--", ref)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", errors.New("1Password didn't answer in time")
		}
		return "", fmt.Errorf("1Password: %s", reason(stderr.String()))
	}
	if stdout.Len() == 0 {
		return "", errors.New("1Password: the field is empty")
	}
	return stdout.String(), nil
}

// Version returns op's version, or ErrNotInstalled.
func (c Client) Version(ctx context.Context) (string, error) {
	bin, err := c.path()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := platform.HostCommand(ctx, nil, "", bin, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("running op: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// reason turns op's error output ("[ERROR] 2026/10/15 15:04:05 could not
// read secret ...") into a short sentence.
func reason(stderr string) string {
	for line := range strings.Lines(stderr) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "[ERROR] "); ok {
			// Drop the date and time.
			if f := strings.SplitN(rest, " ", 3); len(f) == 3 && strings.Count(f[0], "/") == 2 {
				rest = f[2]
			}
			line = rest
		}
		if len(line) > 300 {
			line = line[:300] + "…"
		}
		return line
	}
	return "op failed without saying why"
}
