// Package hostkeys removes a server's old key from known_hosts after ssh
// refused it as changed, when the user asks to. It only touches keys of the
// host being connected to or its jump hosts.
package hostkeys

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// Names is satisfied by sshcmd.Resolver.
type Names interface {
	HostKeyNames(ctx context.Context, hostID int64, target string) (names, files []string, err error)
}

// Service is bound to the frontend.
type Service struct {
	names  Names
	keygen func(ctx context.Context, name, file string) (found bool, err error)
}

// NewService returns a Service that resolves names with n.
func NewService(n Names) *Service { return &Service{names: n, keygen: removeKey} }

// Forget removes keyHost's key ("host" or "[host]:port", as ssh's warning
// names it) from the user's known_hosts files. keyHost must be the saved
// host's (hostID) or ad-hoc target's own name or one of its jump hosts'.
// The next connection then asks to trust the new key, in the terminal.
func (s *Service) Forget(hostID int64, target, keyHost string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	names, files, err := s.names.HostKeyNames(ctx, hostID, target)
	if err != nil {
		return err
	}
	ok := false
	for _, n := range names {
		if strings.EqualFold(n, keyHost) {
			keyHost, ok = n, true
			break
		}
	}
	if !ok || strings.HasPrefix(keyHost, "-") {
		return errors.New("that key doesn't belong to this host or its jump hosts")
	}
	removed := false
	for _, f := range files {
		found, err := s.keygen(ctx, keyHost, f)
		if err != nil {
			return err
		}
		removed = removed || found
	}
	if !removed {
		return errors.New("no saved key for " + keyHost + " was found in your known_hosts files")
	}
	return nil
}

// removeKey runs ssh-keygen -R (it keeps a .old copy of the file).
func removeKey(ctx context.Context, name, file string) (bool, error) {
	out, err := exec.CommandContext(ctx, "ssh-keygen", "-R", name, "-f", file).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "not found") {
		return false, errors.New(strings.TrimSpace(string(out)))
	}
	return strings.Contains(string(out), "found: line"), nil
}
