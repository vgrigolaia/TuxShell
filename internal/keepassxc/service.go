package keepassxc

import (
	"context"
	"strings"
)

// Service is bound to the frontend. It never returns a password.
type Service struct {
	Client *Client
}

// Status reports whether KeePassXC is reachable and knows TuxShell.
func (s *Service) Status(ctx context.Context) Status { return s.Client.Status(ctx) }

// Connect asks KeePassXC to allow TuxShell (KeePassXC shows a window for it)
// and returns the name the user gave the connection.
func (s *Service) Connect(ctx context.Context) (string, error) { return s.Client.Associate(ctx) }

// Disconnect forgets KeePassXC's permission on TuxShell's side.
func (s *Service) Disconnect() error { return s.Client.Forget() }

// Check reads ref and returns "" if it worked, else why not. KeePassXC may
// ask the user to unlock or allow access. The password is discarded.
func (s *Service) Check(ctx context.Context, ref string) string {
	if _, err := s.Client.Read(ctx, strings.TrimSpace(ref)); err != nil {
		return err.Error()
	}
	return ""
}
