package sshcmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

// Unix socket paths are limited to 108 bytes (sun_path); ssh's %C hash is 40
// hex characters, so the directory must leave room for "/cm-" plus that.
const maxSocketPath = 104

// ControlPersist keeps a shared connection open this long after its last
// session ends, so the file browser keeps working briefly after a disconnect.
const controlPersist = "120"

// ControlDir returns a private directory for ssh ControlMaster sockets:
// $XDG_RUNTIME_DIR/tuxshell, or /tmp/tuxshell-<uid> when that path is too
// long for a socket. Fallback directories are verified to be ours.
func ControlDir() (string, error) {
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		dir := filepath.Join(rt, "tuxshell")
		if len(dir)+len("/cm-")+40 < maxSocketPath {
			if err := ensurePrivateDir(dir); err == nil {
				return dir, nil
			}
		}
	}
	dir := filepath.Join(os.TempDir(), "tuxshell-"+strconv.Itoa(os.Getuid()))
	if len(dir)+len("/cm-")+40 >= maxSocketPath {
		return "", errors.New("no short enough directory for ssh control sockets")
	}
	return dir, ensurePrivateDir(dir)
}

// ensurePrivateDir creates dir 0700 or checks an existing one is a real
// directory (not a symlink) owned by us with no group/other access.
func ensurePrivateDir(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s is not a directory owned by this user", dir)
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// sessionOptions make an interactive ssh become (or reuse) the shared master.
func sessionOptions(controlDir string) []string {
	if controlDir == "" {
		return nil
	}
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + filepath.Join(controlDir, "cm-%C"),
		"-o", "ControlPersist=" + controlPersist,
	}
}

// batchOptions run a non-interactive command through an existing master and
// never prompt: without a master (and without key/agent auth) it fails fast.
func batchOptions(controlDir string) []string {
	opts := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10"}
	if controlDir == "" {
		return opts
	}
	return append(opts,
		"-o", "ControlMaster=no",
		"-o", "ControlPath="+filepath.Join(controlDir, "cm-%C"),
	)
}

// tunnelOptions run a forward-only connection of its own (-N): it never
// prompts, fails if the port can't be forwarded, and notices dead links.
func tunnelOptions() []string {
	return []string{
		"-N", "-T",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ControlMaster=no",
		"-o", "ControlPath=none",
	}
}

// Remote runs shell scripts on a host over ssh in batch mode.
type Remote struct {
	ssh  string
	args []string
}

// Command returns an ssh command that runs script on the remote host.
// script is passed to the remote login shell as one argument.
func (r Remote) Command(ctx context.Context, script string) *exec.Cmd {
	args := append(append([]string{}, r.args...), script)
	return exec.CommandContext(ctx, r.ssh, args...)
}
