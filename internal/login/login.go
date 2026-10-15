// Package login answers ssh's password prompts for terminal sessions, with
// a 1Password reference or a password saved in the system keyring.
//
// The rules:
//   - Only sessions for saved hosts take part: hosts with a 1Password
//     reference always, others while "Allow saving passwords" is on.
//     Otherwise ssh asks in the terminal as before.
//   - The first answer comes from 1Password, else from the keyring.
//   - Only a password prompt from the session's own ssh process is answered.
//     Jump hosts run their own ssh, so their prompts go to the terminal and
//     never receive the target's password. So do host key questions and key
//     passphrases.
//   - A saved password is tried once. If ssh asks again it was wrong, and the
//     user is asked instead (and can replace it).
package login

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/askpass"
	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/secrets"
)

// EventPassword asks the frontend for a password (a PasswordRequest).
const EventPassword = "login:password"

// EventDone tells the frontend a request ended without its answer (the
// session closed or it timed out); it carries the request ID.
const EventDone = "login:done"

// PasswordRequest is shown as a password box over the session's pane.
type PasswordRequest struct {
	RequestID string `json:"requestId"`
	SessionID string `json:"sessionId"`
	HostName  string `json:"hostName"`
	Prompt    string `json:"prompt"`
	// SavedFailed is true when the saved password was just tried and refused.
	SavedFailed bool `json:"savedFailed"`
	// VaultFailed is true when the password from 1Password was just refused.
	VaultFailed bool `json:"vaultFailed"`
	// VaultError says why 1Password couldn't give the password ("" if it did
	// or the host has no reference).
	VaultError string `json:"vaultError"`
	// CanSave is true when the password may be remembered.
	CanSave bool `json:"canSave"`
}

// Status is what Preferences shows.
type Status struct {
	// Available is false when there's no system keyring.
	Available bool `json:"available"`
	// Saved counts hosts with a saved password.
	Saved int `json:"saved"`
}

// askTimeout is how long a password box waits before ssh gives up.
const askTimeout = 10 * time.Minute

// ErrDisabled is returned when saving passwords is off.
var ErrDisabled = errors.New("saving passwords is turned off (Preferences → Security)")

type answer struct {
	password string
	remember bool
	cancel   bool
}

type session struct {
	id     string
	hostID int64
	pid    atomic.Int64
	done   chan struct{}

	mu       sync.Mutex
	attempts int
	// last says where the previous answer came from: "vault", "saved", "typed+saved" or "typed".
	last string
}

// Manager runs the askpass server and answers prompts. It is not bound to the
// frontend: only Go code may start it or prepare sessions (Start chooses the
// program ssh runs). The frontend gets Service.
type Manager struct {
	hosts   *hosts.Store
	secrets *secrets.Store
	enabled func() bool
	emit    func(name string, data any)
	exe     string
	server  *askpass.Server
	vault   Vault

	mu       sync.Mutex
	sessions map[string]*session // by token
	pending  map[string]chan answer
}

// Vault reads 1Password references (see internal/onepassword).
type Vault interface {
	Read(ctx context.Context, ref string) (string, error)
}

// UseVault answers prompts for hosts with a 1Password reference from v.
// A function, so it isn't bound.
func UseVault(m *Manager, v Vault) { m.vault = v }

// New returns a manager. Start must be called before sessions use it.
func New(store *hosts.Store, sec *secrets.Store, enabled func() bool, emit func(string, any)) *Manager {
	return &Manager{
		hosts: store, secrets: sec, enabled: enabled, emit: emit,
		sessions: map[string]*session{}, pending: map[string]chan answer{},
	}
}

// Start listens for ssh's askpass helper in dir (private to this user), which
// runs exe. Without it, prompts simply stay in the terminal.
func (s *Manager) Start(dir, exe string) error {
	srv, err := askpass.Listen(dir, s.handle)
	if err != nil {
		return err
	}
	s.server, s.exe = srv, exe
	return nil
}

// Close stops the askpass server.
func (s *Manager) Close() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// Service is what the frontend may call: answering password boxes, the host
// form's password field and Preferences.
type Service struct {
	m *Manager
}

// Service returns the frontend's view of m.
func (s *Manager) Service() *Service { return &Service{m: s} }

// ServiceShutdown stops the askpass server when the app quits.
func (s *Service) ServiceShutdown() error { return s.m.Close() }

// Begin prepares a terminal session for a saved host. It returns the extra
// environment for its ssh (nil when it doesn't take part), a function to call
// with ssh's PID once it runs, and one to call when the session ends.
func (s *Manager) Begin(sessionID string, hostID int64) (env []string, started func(pid int), end func()) {
	noop := func() {}
	if s.server == nil || !s.takesPart(hostID) {
		return nil, func(int) {}, noop
	}
	token := randomID()
	sess := &session{id: sessionID, hostID: hostID, done: make(chan struct{})}
	s.mu.Lock()
	s.sessions[token] = sess
	s.mu.Unlock()
	var once sync.Once
	return askpass.Env(s.exe, s.server.Path(), token),
		func(pid int) { sess.pid.Store(int64(pid)) },
		func() {
			once.Do(func() {
				s.mu.Lock()
				delete(s.sessions, token)
				s.mu.Unlock()
				close(sess.done)
			})
		}
}

// takesPart reports whether a host's prompts are answered here.
func (s *Manager) takesPart(hostID int64) bool {
	if s.enabled() {
		return true
	}
	h, err := s.hosts.Get(hostID)
	return err == nil && h.PasswordRef != "" && s.vault != nil
}

// isPassword reports whether ssh is asking for a login password, as opposed
// to a key passphrase, a host key question or a new password.
func isPassword(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	return strings.HasSuffix(p, ":") && strings.Contains(p, "password") &&
		!strings.Contains(p, "passphrase") && !strings.Contains(p, "new password") &&
		!strings.Contains(p, "retype") && !strings.Contains(p, "again")
}

func (s *Manager) handle(req askpass.Request) askpass.Reply {
	tty := askpass.Reply{Action: askpass.ActionTTY}
	s.mu.Lock()
	sess := s.sessions[req.Token]
	s.mu.Unlock()
	if sess == nil || !isPassword(req.Prompt) {
		return tty
	}
	// Only the session's own ssh: a jump host's ssh is a child of it.
	if pid := sess.pid.Load(); pid == 0 || int64(req.ParentPID) != pid {
		return tty
	}
	h, err := s.hosts.Get(sess.hostID)
	if err != nil {
		return tty
	}
	fromVault := h.PasswordRef != "" && s.vault != nil
	if !s.enabled() && !fromVault {
		return tty
	}

	sess.mu.Lock()
	sess.attempts++
	first, last := sess.attempts == 1, sess.last
	sess.mu.Unlock()

	vaultErr := ""
	if first && fromVault {
		pw, err := s.vault.Read(context.Background(), h.PasswordRef)
		if err == nil {
			sess.setLast("vault")
			return askpass.Reply{Action: askpass.ActionAnswer, Answer: pw}
		}
		vaultErr = err.Error()
	}
	if first && s.enabled() && h.PasswordSaved {
		if pw, err := s.secrets.Get(h.ID); err == nil && pw != "" {
			sess.setLast("saved")
			return askpass.Reply{Action: askpass.ActionAnswer, Answer: pw}
		}
	}
	// A password remembered on the previous try was wrong: don't keep it.
	if last == "typed+saved" {
		s.forget(h.ID)
	}

	a, ok := s.ask(sess, PasswordRequest{
		SessionID: sess.id, HostName: h.Name, Prompt: strings.TrimSpace(req.Prompt),
		SavedFailed: last == "saved", VaultFailed: last == "vault", VaultError: vaultErr,
		CanSave: s.enabled() && s.secrets.Available(),
	})
	if !ok || a.cancel {
		return askpass.Reply{Action: askpass.ActionCancel}
	}
	if a.remember && s.enabled() {
		if err := s.save(h.ID, a.password); err != nil {
			log.Printf("saving the password for host %d: %v", h.ID, err)
			sess.setLast("typed")
		} else {
			sess.setLast("typed+saved")
		}
	} else {
		sess.setLast("typed")
	}
	return askpass.Reply{Action: askpass.ActionAnswer, Answer: a.password}
}

func (sess *session) setLast(v string) {
	sess.mu.Lock()
	sess.last = v
	sess.mu.Unlock()
}

// ask shows a password box and waits for Answer or Cancel.
func (s *Manager) ask(sess *session, req PasswordRequest) (answer, bool) {
	req.RequestID = randomID()
	ch := make(chan answer, 1)
	s.mu.Lock()
	s.pending[req.RequestID] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, req.RequestID)
		s.mu.Unlock()
	}()
	s.emit(EventPassword, req)
	select {
	case a := <-ch:
		return a, true
	case <-sess.done:
	case <-time.After(askTimeout):
	}
	s.emit(EventDone, req.RequestID)
	return answer{}, false
}

func (s *Manager) reply(requestID string, a answer) error {
	s.mu.Lock()
	ch, ok := s.pending[requestID]
	s.mu.Unlock()
	if !ok {
		return errors.New("this password request has ended")
	}
	select {
	case ch <- a:
	default:
	}
	return nil
}

func (s *Manager) sendAnswer(requestID, password string, remember bool) error {
	return s.reply(requestID, answer{password: password, remember: remember})
}

func (s *Manager) cancelRequest(requestID string) error {
	return s.reply(requestID, answer{cancel: true})
}

func (s *Manager) save(hostID int64, password string) error {
	if err := s.secrets.Set(hostID, password); err != nil {
		return err
	}
	return s.hosts.SetPasswordSaved(hostID, true)
}

func (s *Manager) forget(hostID int64) {
	if err := s.secrets.Delete(hostID); err != nil {
		log.Printf("removing the password for host %d: %v", hostID, err)
	}
	_ = s.hosts.SetPasswordSaved(hostID, false)
}

func (s *Manager) setPassword(hostID int64, password string) error {
	if !s.enabled() {
		return ErrDisabled
	}
	if _, err := s.hosts.Get(hostID); err != nil {
		return err
	}
	return s.save(hostID, password)
}

func (s *Manager) clearPassword(hostID int64) error {
	if err := s.secrets.Delete(hostID); err != nil {
		return err
	}
	return s.hosts.SetPasswordSaved(hostID, false)
}

func (s *Manager) status() (Status, error) {
	list, err := s.hosts.List()
	if err != nil {
		return Status{}, err
	}
	st := Status{Available: s.secrets.Available()}
	for _, h := range list {
		if h.PasswordSaved {
			st.Saved++
		}
	}
	return st, nil
}

func (s *Manager) forgetAll() (int, error) {
	list, err := s.hosts.List()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, h := range list {
		if !h.PasswordSaved {
			continue
		}
		if err := s.clearPassword(h.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// SavePassword saves a password for a host, e.g. from an imported file. The
// caller checks that saving passwords is on.
func (s *Manager) SavePassword(hostID int64, password string) error { return s.save(hostID, password) }

// HostsDeleted removes the saved passwords of deleted hosts (hosts.Store.OnDelete).
func (s *Manager) HostsDeleted(ids []int64) {
	for _, id := range ids {
		if err := s.secrets.Delete(id); err != nil {
			log.Printf("removing the password for deleted host %d: %v", id, err)
		}
	}
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Answer sends the password typed into a password box. remember saves it for
// the host (when saving passwords is on).
func (s *Service) Answer(requestID, password string, remember bool) error {
	return s.m.sendAnswer(requestID, password, remember)
}

// Cancel closes a password box; ssh's login attempt fails.
func (s *Service) Cancel(requestID string) error { return s.m.cancelRequest(requestID) }

// SetPassword saves a password for a host from the host form.
func (s *Service) SetPassword(hostID int64, password string) error {
	return s.m.setPassword(hostID, password)
}

// ClearPassword removes a host's saved password.
func (s *Service) ClearPassword(hostID int64) error { return s.m.clearPassword(hostID) }

// Status reports whether the keyring works and how many passwords are saved.
func (s *Service) Status() (Status, error) { return s.m.status() }

// ForgetAll removes every saved password and returns how many there were.
func (s *Service) ForgetAll() (int, error) { return s.m.forgetAll() }
