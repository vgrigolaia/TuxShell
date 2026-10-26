// Package secrets keeps saved host passwords in the system keyring (Secret
// Service: GNOME Keyring, KWallet). Nothing here ever touches the disk itself,
// and passwords are never logged or sent to the frontend.
package secrets

import (
	"errors"
	"strconv"

	"github.com/zalando/go-keyring"
)

// keyringService is shared with the clipboard history key.
const keyringService = "TuxShell"

// ErrUnavailable means there's no usable system keyring.
var ErrUnavailable = errors.New("no system keyring found (GNOME Keyring or KWallet is needed to save passwords)")

// Keyring is the subset of go-keyring used, so tests can swap it.
type Keyring interface {
	Set(service, user, secret string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type systemKeyring struct{}

// SystemKeyring is the desktop's keyring, for other packages' own entries.
func SystemKeyring() Keyring { return systemKeyring{} }

func (systemKeyring) Set(s, u, p string) error        { return keyring.Set(s, u, p) }
func (systemKeyring) Get(s, u string) (string, error) { return keyring.Get(s, u) }
func (systemKeyring) Delete(s, u string) error        { return keyring.Delete(s, u) }

// Store saves one password per host.
type Store struct {
	kr Keyring
}

// New uses the system keyring.
func New() *Store { return &Store{kr: systemKeyring{}} }

// NewWith uses kr; for tests.
func NewWith(kr Keyring) *Store { return &Store{kr: kr} }

func entry(hostID int64) string { return "host-password:" + strconv.FormatInt(hostID, 10) }

// Available reports whether the keyring answers at all.
func (s *Store) Available() bool {
	_, err := s.kr.Get(keyringService, "probe")
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}

// Set saves the password for a host.
func (s *Store) Set(hostID int64, password string) error {
	if password == "" {
		return errors.New("the password is empty")
	}
	if err := s.kr.Set(keyringService, entry(hostID), password); err != nil {
		if errors.Is(err, keyring.ErrSetDataTooBig) {
			return errors.New("the password is too long")
		}
		return ErrUnavailable
	}
	return nil
}

// Get returns the saved password, or "" when there is none.
func (s *Store) Get(hostID int64) (string, error) {
	p, err := s.kr.Get(keyringService, entry(hostID))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", ErrUnavailable
	}
	return p, nil
}

// Delete removes a host's password; a missing one is not an error.
func (s *Store) Delete(hostID int64) error {
	err := s.kr.Delete(keyringService, entry(hostID))
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return ErrUnavailable
	}
	return nil
}
