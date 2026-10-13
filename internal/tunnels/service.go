// Package tunnels runs saved port forwards (ssh -L and -D) and reports their state.
package tunnels

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/sshcmd"
)

// EventChanged is emitted (without data) whenever a tunnel starts or stops.
const EventChanged = "tunnels:changed"

// Store keeps tunnel definitions; satisfied by *hosts.Store.
type Store interface {
	ListTunnels() ([]hosts.Tunnel, error)
	GetTunnel(id int64) (hosts.Tunnel, error)
	SaveTunnel(t hosts.Tunnel) (hosts.Tunnel, error)
	DeleteTunnel(id int64) error
}

// Commands builds the ssh processes; satisfied by sshcmd.Resolver.
type Commands interface {
	TunnelCommand(id int64, f sshcmd.Forward) (*exec.Cmd, error)
	ControlCommand(id int64, op string, f sshcmd.Forward) (*exec.Cmd, error)
}

// State is a tunnel with its live status.
type State struct {
	hosts.Tunnel
	// Status is "stopped", "starting", "running" or "failed".
	Status string `json:"status"`
	// Shared is true when the forward rides on an open terminal's connection
	// (password-only hosts); it ends when that connection closes.
	Shared  bool   `json:"shared"`
	Message string `json:"message"`
	Since   int64  `json:"since"` // Unix milliseconds when it started
}

type live struct {
	cmd     *exec.Cmd // nil for shared forwards
	forward sshcmd.Forward
	hostID  int64
	since   time.Time
	shared  bool
	stopped bool
}

// Service is bound to the frontend.
type Service struct {
	store Store
	cmds  Commands
	emit  func()

	mu      sync.Mutex
	running map[int64]*live
	failed  map[int64]string
}

// NewService runs tunnels from store with cmds and calls emit on every change.
func NewService(store Store, cmds Commands, emit func()) *Service {
	return &Service{store: store, cmds: cmds, emit: emit, running: map[int64]*live{}, failed: map[int64]string{}}
}

// ServiceStartup watches shared forwards; Wails cancels ctx at shutdown.
func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	go s.watchShared(ctx)
	return nil
}

// List returns every saved tunnel with its status.
func (s *Service) List() ([]State, error) {
	list, err := s.store.ListTunnels()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]State, 0, len(list))
	for _, t := range list {
		st := State{Tunnel: t, Status: "stopped"}
		if l, ok := s.running[t.ID]; ok {
			st.Status, st.Shared, st.Since = "running", l.shared, l.since.UnixMilli()
		} else if msg, ok := s.failed[t.ID]; ok {
			st.Status, st.Message = "failed", msg
		}
		out = append(out, st)
	}
	return out, nil
}

// Save creates or updates a tunnel. A running tunnel is restarted with the new settings.
func (s *Service) Save(t hosts.Tunnel) (hosts.Tunnel, error) {
	wasRunning := t.ID != 0 && s.isRunning(t.ID)
	if wasRunning {
		s.Stop(t.ID)
	}
	saved, err := s.store.SaveTunnel(t)
	if err != nil {
		return saved, err
	}
	if wasRunning {
		_ = s.Start(saved.ID)
	}
	s.emit()
	return saved, nil
}

// Delete stops and removes a tunnel.
func (s *Service) Delete(id int64) error {
	s.Stop(id)
	if err := s.store.DeleteTunnel(id); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.failed, id)
	s.mu.Unlock()
	s.emit()
	return nil
}

func forwardOf(t hosts.Tunnel) sshcmd.Forward {
	if t.Kind == "dynamic" {
		return sshcmd.DynamicForward(t.BindPort)
	}
	return sshcmd.LocalForward(t.BindPort, t.TargetHost, t.TargetPort)
}

// startWait is how long a new tunnel process must survive to count as up:
// ExitOnForwardFailure and auth errors make ssh exit well within it.
var startWait = 4 * time.Second

// Start opens a tunnel. It first tries a connection of its own (key or
// agent auth); if that's refused and a terminal to the host is open, it
// adds the forward to that terminal's shared connection instead.
func (s *Service) Start(id int64) error {
	t, err := s.store.GetTunnel(id)
	if err != nil {
		return err
	}
	if s.isRunning(id) {
		return nil
	}
	f := forwardOf(t)
	cmd, err := s.cmds.TunnelCommand(t.HostID, f)
	if err != nil {
		return s.fail(id, err.Error())
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return s.fail(id, err.Error())
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		msg := explain(stderr.String(), err)
		if strings.Contains(stderr.String(), "Permission denied") && s.shared(t.HostID, f) {
			s.mu.Lock()
			s.running[id] = &live{forward: f, hostID: t.HostID, since: time.Now(), shared: true}
			delete(s.failed, id)
			s.mu.Unlock()
			s.emit()
			return nil
		}
		return s.fail(id, msg)
	case <-time.After(startWait):
	}

	l := &live{cmd: cmd, forward: f, hostID: t.HostID, since: time.Now()}
	s.mu.Lock()
	s.running[id] = l
	delete(s.failed, id)
	s.mu.Unlock()
	s.emit()

	go func() {
		err := <-done
		s.mu.Lock()
		if s.running[id] == l {
			delete(s.running, id)
			if !l.stopped {
				s.failed[id] = "Disconnected: " + explain(stderr.String(), err)
			}
		}
		s.mu.Unlock()
		s.emit()
	}()
	return nil
}

// shared adds f to an open terminal's connection to the host, if any.
func (s *Service) shared(hostID int64, f sshcmd.Forward) bool {
	check, err := s.cmds.ControlCommand(hostID, "check", f)
	if err != nil || check.Run() != nil {
		return false
	}
	fwd, err := s.cmds.ControlCommand(hostID, "forward", f)
	return err == nil && fwd.Run() == nil
}

// Stop closes a tunnel.
func (s *Service) Stop(id int64) {
	s.mu.Lock()
	l, ok := s.running[id]
	if ok {
		l.stopped = true
		delete(s.running, id)
	}
	delete(s.failed, id)
	s.mu.Unlock()
	if !ok {
		return
	}
	if l.cmd != nil {
		_ = l.cmd.Process.Signal(syscall.SIGTERM)
	} else if c, err := s.cmds.ControlCommand(l.hostID, "cancel", l.forward); err == nil {
		_ = c.Run()
	}
	s.emit()
}

// StopAll closes every tunnel, e.g. when the app quits.
func (s *Service) StopAll() {
	s.mu.Lock()
	ids := make([]int64, 0, len(s.running))
	for id := range s.running {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.Stop(id)
	}
}

// ServiceShutdown stops tunnels when the app quits.
func (s *Service) ServiceShutdown() error {
	s.StopAll()
	return nil
}

func (s *Service) isRunning(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.running[id]
	return ok
}

func (s *Service) fail(id int64, msg string) error {
	s.mu.Lock()
	s.failed[id] = msg
	s.mu.Unlock()
	s.emit()
	return errors.New(msg)
}

// watchShared notices when the terminal connection behind a shared forward closes.
func (s *Service) watchShared(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.mu.Lock()
		var shared []int64
		for id, l := range s.running {
			if l.shared {
				shared = append(shared, id)
			}
		}
		s.mu.Unlock()
		for _, id := range shared {
			s.mu.Lock()
			l := s.running[id]
			s.mu.Unlock()
			if l == nil {
				continue
			}
			c, err := s.cmds.ControlCommand(l.hostID, "check", l.forward)
			if err == nil && c.Run() == nil {
				continue
			}
			s.mu.Lock()
			if s.running[id] == l {
				delete(s.running, id)
				s.failed[id] = "The terminal connection it used was closed"
			}
			s.mu.Unlock()
			s.emit()
		}
	}
}

// explain turns ssh's stderr into one readable line.
func explain(stderr string, err error) string {
	switch {
	case strings.Contains(stderr, "Address already in use"), strings.Contains(stderr, "cannot listen to port"):
		return "That local port is already in use"
	case strings.Contains(stderr, "Permission denied"):
		return "No key or agent was accepted. For password-only hosts, open a terminal to the host first"
	case strings.Contains(stderr, "Could not resolve hostname"):
		return "Could not resolve the host name"
	case strings.Contains(stderr, "Connection refused"):
		return "Connection refused"
	case strings.Contains(stderr, "timed out"):
		return "Connection timed out"
	case strings.Contains(stderr, "Host key verification failed"):
		return "Host key not trusted yet: connect once in a terminal to accept it"
	}
	if lines := strings.Split(strings.TrimSpace(stderr), "\n"); lines[len(lines)-1] != "" {
		return lines[len(lines)-1]
	}
	if err != nil {
		return err.Error()
	}
	return "ssh exited"
}
