// Package terminal runs processes in pseudo-terminals and streams their I/O
// to the frontend through Wails events.
package terminal

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/creack/pty"
)

// Event names emitted to the frontend.
const (
	EventData = "terminal:data"
	EventExit = "terminal:exit"
)

// DataEvent carries a chunk of PTY output. Data is base64 so that UTF-8
// sequences split across reads survive the JSON hop intact.
type DataEvent struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

// ExitEvent is emitted once when a session's process ends.
type ExitEvent struct {
	ID   string `json:"id"`
	Code int    `json:"code"`
}

// Emitter sends an event to the frontend.
type Emitter func(name string, data any)

// Resolver builds the ssh command for a saved host or an ad-hoc target.
// Commands are only ever built in Go; the frontend can't choose what runs.
type Resolver interface {
	HostCommand(id int64) (*exec.Cmd, error)
	HostCommandAs(id int64, user string) (*exec.Cmd, error)
	TargetCommand(target string) (*exec.Cmd, error)
}

type session struct {
	cmd  *exec.Cmd
	ptmx *os.File
	// hungUp is set once the session was asked to close.
	hungUp bool
}

// Service manages PTY sessions. Its exported methods are bound to the frontend.
type Service struct {
	emit     Emitter
	resolver Resolver
	// shell returns the configured local shell ("" means $SHELL).
	shell    func() string
	nextID   atomic.Uint64
	mu       sync.Mutex
	sessions map[string]*session
}

// NewService returns a Service that reports output and exits through emit,
// builds ssh commands with resolver and runs shell() in local tabs (nil or
// "" means $SHELL).
func NewService(emit Emitter, resolver Resolver, shell func() string) *Service {
	return &Service{emit: emit, resolver: resolver, shell: shell, sessions: map[string]*session{}}
}

// StartShell starts the user's login shell and returns the session ID.
func (s *Service) StartShell(cols, rows int) (string, error) {
	shell := ""
	if s.shell != nil {
		shell = s.shell()
	}
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-l")
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	return s.start(cmd, cols, rows)
}

// StartHost connects to a saved host and returns the session ID.
func (s *Service) StartHost(hostID int64, cols, rows int) (string, error) {
	cmd, err := s.resolver.HostCommand(hostID)
	if err != nil {
		return "", err
	}
	return s.start(cmd, cols, rows)
}

// StartHostAs connects to a saved host as user, overriding its saved user.
func (s *Service) StartHostAs(hostID int64, user string, cols, rows int) (string, error) {
	cmd, err := s.resolver.HostCommandAs(hostID, user)
	if err != nil {
		return "", err
	}
	return s.start(cmd, cols, rows)
}

// StartTarget connects to an ad-hoc "[user@]host[:port]" and returns the session ID.
func (s *Service) StartTarget(target string, cols, rows int) (string, error) {
	cmd, err := s.resolver.TargetCommand(target)
	if err != nil {
		return "", err
	}
	return s.start(cmd, cols, rows)
}

// start runs cmd in a new PTY of the given size and begins streaming its output.
func (s *Service) start(cmd *exec.Cmd, cols, rows int) (string, error) {
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	ptmx, err := pty.StartWithSize(cmd, winsize(cols, rows))
	if err != nil {
		return "", fmt.Errorf("start %s: %w", cmd.Path, err)
	}

	id := strconv.FormatUint(s.nextID.Add(1), 10)
	s.mu.Lock()
	s.sessions[id] = &session{cmd: cmd, ptmx: ptmx}
	s.mu.Unlock()

	go s.pump(id, cmd, ptmx)
	return id, nil
}

// pump forwards PTY output until the process exits, then reports the exit code.
func (s *Service) pump(id string, cmd *exec.Cmd, ptmx *os.File) {
	buf := make([]byte, 32*1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			s.emit(EventData, DataEvent{ID: id, Data: base64.StdEncoding.EncodeToString(buf[:n])})
		}
		if err != nil {
			// Linux returns EIO once the child side closes; either way we're done.
			break
		}
	}

	code := 0
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	_ = ptmx.Close()
	s.emit(EventExit, ExitEvent{ID: id, Code: code})
}

// Write sends keyboard input to a session.
func (s *Service) Write(id, data string) error {
	sess, err := s.get(id)
	if err != nil {
		return err
	}
	_, err = io.WriteString(sess.ptmx, data)
	return err
}

// Resize updates a session's terminal size.
func (s *Service) Resize(id string, cols, rows int) error {
	sess, err := s.get(id)
	if err != nil {
		return err
	}
	return pty.Setsize(sess.ptmx, winsize(cols, rows))
}

// Close hangs up a session. The exit event follows once the process is gone.
func (s *Service) Close(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("no terminal session %q", id)
	}
	return s.hangUp(sess)
}

// LiveCount is how many sessions are running and haven't been asked to close.
func (s *Service) LiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, sess := range s.sessions {
		if !sess.hungUp {
			n++
		}
	}
	return n
}

// CloseAll hangs up every session, e.g. before the window closes.
func (s *Service) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		_ = s.hangUp(sess)
	}
}

// hangUp signals a session; callers hold s.mu.
func (s *Service) hangUp(sess *session) error {
	sess.hungUp = true
	return sess.cmd.Process.Signal(syscall.SIGHUP)
}

// ServiceShutdown hangs up every session when the app quits.
func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		_ = s.hangUp(sess)
	}
	return nil
}

func (s *Service) get(id string) (*session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("no terminal session %q", id)
	}
	return sess, nil
}

func winsize(cols, rows int) *pty.Winsize {
	return &pty.Winsize{Cols: uint16(max(cols, 1)), Rows: uint16(max(rows, 1))}
}
