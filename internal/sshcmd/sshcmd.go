// Package sshcmd builds system ssh invocations for saved hosts and ad-hoc targets.
package sshcmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// Args returns ssh's arguments (without the program name) for h.
// h must already be validated, which guarantees no value starts with "-".
func Args(h hosts.Host) []string {
	args := []string{"-p", strconv.Itoa(h.Port)}
	if h.User != "" {
		args = append(args, "-l", h.User)
	}
	if h.ProxyJump != "" {
		args = append(args, "-J", h.ProxyJump)
	}
	if h.IdentityFile != "" {
		args = append(args, "-i", expandHome(h.IdentityFile))
	}

	dest := h.Hostname
	if h.Alias != "" {
		// Connect via the alias so the rest of its ssh_config block applies;
		// an edited hostname still wins.
		dest = h.Alias
		if h.Hostname != "" && h.Hostname != h.Alias {
			args = append(args, "-o", "HostName="+h.Hostname)
		}
	}
	return append(args, "--", dest)
}

// ParseTarget turns "[user@]host[:port]" into a validated ad-hoc host.
func ParseTarget(target string) (hosts.Host, error) {
	target = strings.TrimSpace(target)
	h := hosts.Host{Name: target}

	rest := target
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		h.User, rest = rest[:i], rest[i+1:]
	}
	switch {
	case strings.HasPrefix(rest, "["):
		// [ipv6]:port
		end := strings.Index(rest, "]")
		if end < 0 {
			return h, &hosts.ValidationError{Field: "hostname", Message: "Missing ] in IPv6 address"}
		}
		h.Hostname = rest[1:end]
		if port, ok := strings.CutPrefix(rest[end+1:], ":"); ok {
			h.Port, _ = strconv.Atoi(port)
			if h.Port == 0 {
				return h, &hosts.ValidationError{Field: "port", Message: "Port must be a number"}
			}
		}
	case strings.Count(rest, ":") == 1:
		host, port, _ := strings.Cut(rest, ":")
		h.Hostname = host
		h.Port, _ = strconv.Atoi(port)
		if h.Port == 0 {
			return h, &hosts.ValidationError{Field: "port", Message: "Port must be a number"}
		}
	default:
		h.Hostname = rest
	}

	h.Normalize()
	if err := h.Validate(); err != nil {
		return h, err
	}
	return h, nil
}

// Resolver builds ssh commands from the host store.
type Resolver struct {
	Store *hosts.Store
	// ControlDir holds ControlMaster sockets; empty disables connection sharing.
	ControlDir string
}

// host loads and re-validates a saved host (the database may have been edited
// by hand), optionally replacing its login user.
func (r Resolver) host(id int64, user *string) (hosts.Host, error) {
	h, err := r.Store.Get(id)
	if err != nil {
		return h, err
	}
	if user != nil {
		h.User = *user
	}
	h.Normalize()
	if err := h.Validate(); err != nil {
		return h, fmt.Errorf("host %q: %w", h.Name, err)
	}
	return h, nil
}

// HostCommand returns the interactive command for a saved host.
func (r Resolver) HostCommand(id int64) (*exec.Cmd, error) {
	h, err := r.host(id, nil)
	if err != nil {
		return nil, err
	}
	return r.session(h)
}

// HostCommandAs is HostCommand with the login user replaced, for a
// username typed at connect time.
func (r Resolver) HostCommandAs(id int64, user string) (*exec.Cmd, error) {
	h, err := r.host(id, &user)
	if err != nil {
		var ve *hosts.ValidationError
		if errors.As(err, &ve) {
			return nil, ve // keep the field so the login prompt can show it
		}
		return nil, err
	}
	return r.session(h)
}

// TargetCommand returns the interactive command for an ad-hoc "[user@]host[:port]".
func (r Resolver) TargetCommand(target string) (*exec.Cmd, error) {
	h, err := ParseTarget(target)
	if err != nil {
		return nil, err
	}
	return r.session(h)
}

// HostRemote returns a batch runner for a saved host, logging in as user
// when it's non-empty (matching a "Connect as" session).
func (r Resolver) HostRemote(id int64, user string) (Remote, error) {
	var override *string
	if user != "" {
		override = &user
	}
	h, err := r.host(id, override)
	if err != nil {
		return Remote{}, err
	}
	return r.remote(h)
}

// DraftRemote returns a batch runner for host values that may not be saved
// yet (the host editor's Test button). They're validated like saved hosts.
func (r Resolver) DraftRemote(h hosts.Host) (Remote, error) {
	h.Normalize()
	if err := h.Validate(); err != nil {
		return Remote{}, err
	}
	return r.remote(h)
}

// TargetRemote returns a batch runner for an ad-hoc target.
func (r Resolver) TargetRemote(target string) (Remote, error) {
	h, err := ParseTarget(target)
	if err != nil {
		return Remote{}, err
	}
	return r.remote(h)
}

func (r Resolver) session(h hosts.Host) (*exec.Cmd, error) {
	ssh, err := sshPath()
	if err != nil {
		return nil, err
	}
	return exec.Command(ssh, append(sessionOptions(r.ControlDir), Args(h)...)...), nil
}

func (r Resolver) remote(h hosts.Host) (Remote, error) {
	ssh, err := sshPath()
	if err != nil {
		return Remote{}, err
	}
	return Remote{ssh: ssh, args: append(batchOptions(r.ControlDir), Args(h)...)}, nil
}

func sshPath() (string, error) {
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		return "", fmt.Errorf("ssh not found; install openssh-clients: %w", err)
	}
	return ssh, nil
}

func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return path
}
