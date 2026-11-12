package sshcmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/platform"
)

// Unix socket paths are limited to 108 bytes (sun_path); ssh's %C hash is 40
// hex characters, so the directory must leave room for "/cm-" plus that.
const maxSocketPath = 104

// ControlPersist keeps a shared connection open this long (seconds) after its
// last session ends. TuxShell closes it itself when a host's last tab closes
// (Resolver.CloseShared); this only covers a crash.
const controlPersist = "60"

// ControlDir returns a private directory for ssh ControlMaster sockets:
// $XDG_RUNTIME_DIR/tuxshell, or /tmp/tuxshell-<uid> when that path is too
// long for a socket. Fallback directories are verified to be ours.
func ControlDir() (string, error) {
	if rt := platform.RuntimeDir(); rt != "" {
		dir := filepath.Join(rt, "tuxshell")
		if len(dir)+instanceLen+len("/cm-")+40 < maxSocketPath {
			if err := ensurePrivateDir(dir); err == nil {
				return dir, nil
			}
		}
	}
	dir := filepath.Join(platform.SocketTempDir(), "tuxshell-"+strconv.Itoa(os.Getuid()))
	if len(dir)+instanceLen+len("/cm-")+40 >= maxSocketPath {
		return "", errors.New("no short enough directory for ssh control sockets")
	}
	return dir, ensurePrivateDir(dir)
}

// instanceLen is the length of "/" plus an instance folder's name.
const instanceLen = 1 + 8

var instanceName = regexp.MustCompile(`^[0-9a-f]{8}$`)

// InstanceDir makes this run's own folder inside base (from ControlDir), so
// its shared connections are never used by another TuxShell running at the
// same time and can be closed with ssh -O exit. A lock file held until the
// process ends marks it as in use; folders whose lock is free were left by
// runs that ended and are removed first. unlock is for tests.
func InstanceDir(base string) (dir string, unlock func(), err error) {
	removeStaleInstances(base)
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	dir = filepath.Join(base, hex.EncodeToString(b))
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", nil, err
	}
	lock, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return "", nil, err
	}
	return dir, func() { lock.Close() }, nil
}

// removeStaleInstances deletes instance folders in base nobody holds a lock on.
func removeStaleInstances(base string) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !instanceName.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(base, e.Name())
		lock, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_RDWR, 0)
		if err != nil {
			// No lock yet: maybe a run that is starting right now.
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Minute {
				_ = os.RemoveAll(dir)
			}
			continue
		}
		if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			_ = os.RemoveAll(dir)
		}
		lock.Close()
	}
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
	env  []string
}

// Command returns an ssh command that runs script on the remote host.
// script is passed to the remote login shell as one argument.
func (r Remote) Command(ctx context.Context, script string) *exec.Cmd {
	args := append(append([]string{}, r.args...), script)
	cmd := exec.CommandContext(ctx, r.ssh, args...)
	cmd.Env = r.env
	return cmd
}
