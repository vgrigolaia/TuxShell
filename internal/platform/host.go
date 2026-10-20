package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Flatpak is true when TuxShell runs in a Flatpak sandbox. ssh is bundled
// there; the user's shell, op and FreeRDP are on the host and run through
// HostCommand.
var Flatpak = inFlatpak()

func inFlatpak() bool {
	_, err := os.Stat("/.flatpak-info")
	return err == nil
}

// HostCommand is exec.CommandContext for a program installed on the host. In
// a Flatpak it runs through "flatpak-spawn --host", which passes on stdin,
// stdout, stderr (a terminal too), the exit code and signals; env
// ("KEY=VALUE") and dir apply to the host program. Outside a Flatpak env is
// added to this process's environment.
func HostCommand(ctx context.Context, env []string, dir, name string, args ...string) *exec.Cmd {
	if !Flatpak {
		cmd := exec.CommandContext(ctx, name, args...)
		if env != nil {
			cmd.Env = append(os.Environ(), env...)
		}
		cmd.Dir = dir
		return cmd
	}
	// --watch-bus ends the host program when TuxShell exits.
	spawn := []string{"--host", "--watch-bus"}
	for _, e := range env {
		spawn = append(spawn, "--env="+e)
	}
	if dir != "" {
		spawn = append(spawn, "--directory="+dir)
	}
	cmd := exec.CommandContext(ctx, "flatpak-spawn", append(append(spawn, "--", name), args...)...)
	// flatpak-spawn passes SIGTERM on to the host program; SIGKILL would only
	// end flatpak-spawn and leave the program running.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// ErrNotFound means a program isn't installed on the host.
var ErrNotFound = errors.New("not installed")

// LookHostPath finds name on the host's PATH, then tries fallbacks (absolute
// paths) in order, and returns the first executable one.
func LookHostPath(name string, fallbacks ...string) (string, error) {
	if !Flatpak {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
		for _, p := range fallbacks {
			if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
				return p, nil
			}
		}
		return "", ErrNotFound
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const script = `command -v -- "$1" && exit 0; shift; for p in "$@"; do [ -f "$p" ] && [ -x "$p" ] && { echo "$p"; exit 0; }; done; exit 1`
	out, err := HostCommand(ctx, nil, "", "sh", append([]string{"-c", script, "sh", name}, fallbacks...)...).Output()
	if err != nil {
		return "", ErrNotFound
	}
	p, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if !strings.HasPrefix(p, "/") {
		// A shell builtin or alias, not a program.
		return "", ErrNotFound
	}
	return p, nil
}
