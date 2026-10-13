package status

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/sshcmd"
)

// Drafts builds batch ssh runners for unsaved host values; satisfied by sshcmd.Resolver.
type Drafts interface {
	DraftRemote(h hosts.Host) (sshcmd.Remote, error)
}

// TestResult is the outcome of a test login.
type TestResult struct {
	OK      bool    `json:"ok"`
	Message string  `json:"message"`
	MS      float64 `json:"ms"`
}

// Test logs in to h without a terminal and runs `true`. It never prompts:
// hosts that need a typed password report that instead.
func (s *Service) Test(ctx context.Context, h hosts.Host) (TestResult, error) {
	if s.drafts == nil {
		return TestResult{}, errors.New("testing is unavailable")
	}
	r, err := s.drafts.DraftRemote(h)
	if err != nil {
		return TestResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := r.Command(ctx, "true")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	start := time.Now()
	err = cmd.Run()
	ms := float64(time.Since(start).Microseconds()) / 1000
	if err == nil {
		return TestResult{OK: true, Message: "Logged in", MS: ms}, nil
	}
	if ctx.Err() != nil {
		return TestResult{Message: "Timed out after 15 s"}, nil
	}
	return TestResult{Message: explain(stderr.String(), err)}, nil
}

// explain turns ssh's stderr into one readable line.
func explain(stderr string, err error) string {
	switch {
	case strings.Contains(stderr, "Permission denied"):
		return "Reached the host, but no key or agent was accepted (a password login needs a terminal)"
	case strings.Contains(stderr, "Could not resolve hostname"):
		return "Could not resolve the host name"
	case strings.Contains(stderr, "Connection refused"):
		return "Connection refused (is sshd running on that port?)"
	case strings.Contains(stderr, "timed out"):
		return "Connection timed out"
	case strings.Contains(stderr, "Host key verification failed"):
		return "Host key not trusted yet: connect once in a terminal to accept it"
	}
	if lines := strings.Split(strings.TrimSpace(stderr), "\n"); lines[len(lines)-1] != "" {
		return lines[len(lines)-1]
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "ssh exited with code " + exit.String()
	}
	return err.Error()
}
