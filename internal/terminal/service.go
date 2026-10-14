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

	"github.com/vgrigolaia/tuxshell/internal/recordings"
)

// Event names emitted to the frontend.
const (
	EventData = "terminal:data"
	EventExit = "terminal:exit"
	// EventRecording reports a recording that started or stopped (a RecordingEvent).
	EventRecording = "terminal:recording"
)

// RecordingEvent says whether a session is being recorded. Error is set when
// a recording stopped by itself (e.g. it grew too large).
type RecordingEvent struct {
	ID        string `json:"id"`
	Recording bool   `json:"recording"`
	Name      string `json:"name"`
	Error     string `json:"error"`
}

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

// Auth lets ssh's password prompts be answered for saved hosts (see
// internal/login). Begin returns extra environment for ssh (nil to leave
// prompts in the terminal), a function for ssh's PID and one for the end.
type Auth interface {
	Begin(sessionID string, hostID int64) (env []string, started func(pid int), end func())
}

// History records sessions (see hosts.Recorder). Begin returns the function
// to call with the exit code, or nil when nothing is recorded.
type History interface {
	Begin(hostID int64, user, target string) func(code int)
}

// SetHistory records s's ssh sessions in h. A function, so it isn't bound.
func SetHistory(s *Service, h History) { s.history = h }

// visit says what a session connects to, for the history.
type visit struct {
	hostID int64
	user   string
	target string
}

type session struct {
	cmd  *exec.Cmd
	ptmx *os.File
	// hungUp is set once the session was asked to close.
	hungUp bool
	end    func()
	// visited records the session's end in the history (nil if not recorded).
	visited    func(code int)
	cols, rows int
	// rec is the running recording, or nil.
	rec     *recordings.Writer
	recName string
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
	auth     Auth
	history  History
	recDir   string
}

// SetRecordings saves s's recordings in dir. A function, so it isn't bound.
func SetRecordings(s *Service, dir string) { s.recDir = dir }

// StartRecording records a session's output (never its input) to a new file
// and returns the file's name. title names the file and the recording.
func (s *Service) StartRecording(id, title string) (string, error) {
	if s.recDir == "" {
		return "", errors.New("recordings aren't available")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return "", fmt.Errorf("no terminal session %q", id)
	}
	if sess.rec != nil {
		return sess.recName, nil
	}
	w, name, err := recordings.Create(s.recDir, title, sess.cols, sess.rows)
	if err != nil {
		return "", err
	}
	sess.rec, sess.recName = w, name
	s.emit(EventRecording, RecordingEvent{ID: id, Recording: true, Name: name})
	return name, nil
}

// StopRecording ends a session's recording.
func (s *Service) StopRecording(id string) error {
	s.mu.Lock()
	sess, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	return s.stopRecording(id, sess, nil)
}

func (s *Service) stopRecording(id string, sess *session, cause error) error {
	s.mu.Lock()
	w, name := sess.rec, sess.recName
	sess.rec = nil
	s.mu.Unlock()
	if w == nil {
		return nil
	}
	err := w.Close()
	ev := RecordingEvent{ID: id, Name: name}
	if cause != nil {
		ev.Error = cause.Error()
	}
	s.emit(EventRecording, ev)
	return err
}

// SetAuth answers s's saved hosts' password prompts through a. It's a
// function rather than a method so it isn't bound to the frontend.
func SetAuth(s *Service, a Auth) { s.auth = a }

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
	return s.start(cmd, cols, rows, visit{})
}

// StartHost connects to a saved host and returns the session ID.
func (s *Service) StartHost(hostID int64, cols, rows int) (string, error) {
	cmd, err := s.resolver.HostCommand(hostID)
	if err != nil {
		return "", err
	}
	return s.start(cmd, cols, rows, visit{hostID: hostID})
}

// StartHostAs connects to a saved host as user, overriding its saved user.
func (s *Service) StartHostAs(hostID int64, user string, cols, rows int) (string, error) {
	cmd, err := s.resolver.HostCommandAs(hostID, user)
	if err != nil {
		return "", err
	}
	return s.start(cmd, cols, rows, visit{hostID: hostID, user: user})
}

// StartTarget connects to an ad-hoc "[user@]host[:port]" and returns the session ID.
func (s *Service) StartTarget(target string, cols, rows int) (string, error) {
	cmd, err := s.resolver.TargetCommand(target)
	if err != nil {
		return "", err
	}
	return s.start(cmd, cols, rows, visit{target: target})
}

// start runs cmd in a new PTY of the given size and begins streaming its
// output. v says what it connects to (empty for a local shell).
func (s *Service) start(cmd *exec.Cmd, cols, rows int, v visit) (string, error) {
	hostID := v.hostID
	id := strconv.FormatUint(s.nextID.Add(1), 10)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	started, end := func(int) {}, func() {}
	if s.auth != nil && hostID != 0 {
		var env []string
		env, started, end = s.auth.Begin(id, hostID)
		cmd.Env = append(cmd.Env, env...)
	}
	ptmx, err := pty.StartWithSize(cmd, winsize(cols, rows))
	if err != nil {
		end()
		return "", fmt.Errorf("start %s: %w", cmd.Path, err)
	}
	started(cmd.Process.Pid)
	var visited func(code int)
	if s.history != nil && (v.hostID != 0 || v.target != "") {
		visited = s.history.Begin(v.hostID, v.user, v.target)
	}

	s.mu.Lock()
	s.sessions[id] = &session{cmd: cmd, ptmx: ptmx, end: end, visited: visited, cols: cols, rows: rows}
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
			s.record(id, buf[:n])
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
	sess := s.sessions[id]
	if sess != nil && sess.end != nil {
		sess.end()
	}
	delete(s.sessions, id)
	s.mu.Unlock()
	if sess != nil && sess.visited != nil {
		sess.visited(code)
	}
	if sess != nil {
		_ = s.stopRecording(id, sess, nil)
	}
	_ = ptmx.Close()
	s.emit(EventExit, ExitEvent{ID: id, Code: code})
}

// record adds output to the session's recording, if it has one.
func (s *Service) record(id string, data []byte) {
	s.mu.Lock()
	sess := s.sessions[id]
	var w *recordings.Writer
	if sess != nil {
		w = sess.rec
	}
	s.mu.Unlock()
	if w == nil {
		return
	}
	if err := w.Output(data); err != nil {
		_ = s.stopRecording(id, sess, err)
	}
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
	s.mu.Lock()
	sess.cols, sess.rows = cols, rows
	w := sess.rec
	s.mu.Unlock()
	if w != nil {
		_ = w.Resize(cols, rows)
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

// DisplayServer names the desktop session type ("Wayland" or "X11"), for the status bar.
func (s *Service) DisplayServer() string {
	switch os.Getenv("XDG_SESSION_TYPE") {
	case "wayland":
		return "Wayland"
	case "x11":
		return "X11"
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "Wayland"
	}
	if os.Getenv("DISPLAY") != "" {
		return "X11"
	}
	return ""
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
