package cliphist

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/zalando/go-keyring"
)

// Keyring entry holding the history key.
const (
	keyringService = "TuxShell"
	keyringUser    = "clipboard-history-key"
)

// EventChanged tells the frontend to reload the history.
const EventChanged = "clipboard:changed"

// Status describes whether history is available.
type Status struct {
	Enabled bool   `json:"enabled"`
	Paused  bool   `json:"paused"`
	Error   string `json:"error"`
}

// Service exposes clipboard history to the frontend.
type Service struct {
	path   string
	read   func() (string, bool)
	notify func()

	mu     sync.Mutex
	store  *Store
	err    error
	paused bool
}

// NewService keeps history at path. read returns the system clipboard text;
// notify is called whenever the history changes.
func NewService(path string, read func() (string, bool), notify func()) *Service {
	return &Service{path: path, read: read, notify: notify}
}

// ServiceStartup opens the store with the keyring key. Failures disable
// history rather than falling back to unencrypted storage.
func (s *Service) ServiceStartup(context.Context, application.ServiceOptions) error {
	key, err := loadKey()
	if err == nil {
		var st *Store
		st, err = Open(s.path, key)
		if err == nil {
			err = st.Prune()
			s.mu.Lock()
			s.store = st
			s.mu.Unlock()
		}
	}
	if err != nil {
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
	}
	return nil
}

// ServiceShutdown closes the store.
func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}

func loadKey() ([]byte, error) {
	enc, err := keyring.Get(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := keyring.Set(keyringService, keyringUser, base64.StdEncoding.EncodeToString(key)); err != nil {
			return nil, fmt.Errorf("save key to keyring: %w", err)
		}
		return key, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read key from keyring: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || len(key) != 32 {
		return nil, errors.New("keyring holds an invalid clipboard key")
	}
	return key, nil
}

// Status reports whether history is on, paused, or unavailable.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Enabled: s.store != nil, Paused: s.paused}
	if s.err != nil {
		st.Error = s.err.Error()
	}
	return st
}

// SetPaused stops or resumes recording.
func (s *Service) SetPaused(paused bool) {
	s.mu.Lock()
	s.paused = paused
	s.mu.Unlock()
}

// List returns the history, newest first.
func (s *Service) List() ([]Clip, error) {
	st, err := s.active()
	if err != nil {
		return []Clip{}, nil
	}
	return st.List()
}

// Add records text copied inside TuxShell. It returns whether it was kept.
func (s *Service) Add(text string) (bool, error) {
	s.mu.Lock()
	st, paused := s.store, s.paused
	s.mu.Unlock()
	if st == nil || paused {
		return false, nil
	}
	ok, err := st.Add(text)
	if ok {
		s.notify()
	}
	return ok, err
}

// CaptureSystem records the current system clipboard. On Wayland apps can
// only read the clipboard while focused, so this runs when the window gains focus.
func (s *Service) CaptureSystem() {
	if text, ok := s.read(); ok {
		_, _ = s.Add(text)
	}
}

// Delete removes one entry.
func (s *Service) Delete(id int64) error {
	st, err := s.active()
	if err != nil {
		return err
	}
	if err := st.Delete(id); err != nil {
		return err
	}
	s.notify()
	return nil
}

// Clear removes every entry.
func (s *Service) Clear() error {
	st, err := s.active()
	if err != nil {
		return err
	}
	if err := st.Clear(); err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *Service) active() (*Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		return nil, errors.New("clipboard history is unavailable")
	}
	return s.store, nil
}
