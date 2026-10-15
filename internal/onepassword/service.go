package onepassword

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Status is what Preferences and Home show about 1Password.
type Status struct {
	// Installed is true when op was found; Version is its version.
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	// Error says why op couldn't be run ("" when it could).
	Error string `json:"error"`
	// Agent is true when the 1Password SSH agent is running.
	Agent     bool   `json:"agent"`
	AgentPath string `json:"agentPath"`
	// ConfigUsesAgent is true when ~/.ssh/config mentions the agent socket.
	ConfigUsesAgent bool `json:"configUsesAgent"`
}

// Service is bound to the frontend. It never returns a secret's value.
type Service struct {
	Client Client
}

// Status checks for op and the SSH agent.
func (s *Service) Status(ctx context.Context) Status {
	st := Status{AgentPath: AgentSocket()}
	st.Agent = st.AgentPath != "" && AgentRunning(st.AgentPath)
	st.ConfigUsesAgent = configUsesAgent()
	v, err := s.Client.Version(ctx)
	if err != nil {
		st.Installed = !errors.Is(err, ErrNotInstalled)
		st.Error = err.Error()
		return st
	}
	st.Installed, st.Version = true, v
	return st
}

// Check reads ref and returns "" if it worked, else why not. 1Password may
// ask the user to approve the request. The value is discarded.
func (s *Service) Check(ctx context.Context, ref string) string {
	if _, err := s.Client.Read(ctx, strings.TrimSpace(ref)); err != nil {
		return err.Error()
	}
	return ""
}

// configUsesAgent reports whether ~/.ssh/config points IdentityAgent at the
// 1Password socket (Include files aren't followed).
func configUsesAgent() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return false
	}
	for line := range strings.Lines(string(data)) {
		f := strings.Fields(line)
		lower := strings.ToLower(line)
		if len(f) >= 2 && strings.EqualFold(f[0], "IdentityAgent") && strings.Contains(lower, "1password") && strings.Contains(lower, "agent.sock") {
			return true
		}
	}
	return false
}
