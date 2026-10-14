package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

type memKeyring map[string]string

func (m memKeyring) Set(s, u, p string) error { m[s+"/"+u] = p; return nil }
func (m memKeyring) Get(s, u string) (string, error) {
	p, ok := m[s+"/"+u]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return p, nil
}
func (m memKeyring) Delete(s, u string) error {
	if _, ok := m[s+"/"+u]; !ok {
		return keyring.ErrNotFound
	}
	delete(m, s+"/"+u)
	return nil
}

type brokenKeyring struct{}

func (brokenKeyring) Set(string, string, string) error { return errors.New("dbus: no secret service") }
func (brokenKeyring) Get(string, string) (string, error) {
	return "", errors.New("dbus: no secret service")
}
func (brokenKeyring) Delete(string, string) error { return errors.New("dbus: no secret service") }

func TestStore(t *testing.T) {
	kr := memKeyring{}
	s := NewWith(kr)
	if !s.Available() {
		t.Fatal("memory keyring not available")
	}
	if p, err := s.Get(7); p != "" || err != nil {
		t.Fatalf("missing password = %q, %v", p, err)
	}
	if err := s.Set(7, ""); err == nil {
		t.Error("empty password accepted")
	}
	if err := s.Set(7, "s3cret"); err != nil {
		t.Fatal(err)
	}
	if kr["TuxShell/host-password:7"] != "s3cret" {
		t.Errorf("keyring = %v", kr)
	}
	if p, _ := s.Get(7); p != "s3cret" {
		t.Errorf("Get = %q", p)
	}
	if err := s.Delete(7); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(7); err != nil {
		t.Errorf("deleting a missing password: %v", err)
	}
}

func TestUnavailable(t *testing.T) {
	s := NewWith(brokenKeyring{})
	if s.Available() {
		t.Error("broken keyring reported available")
	}
	if err := s.Set(1, "x"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Set err = %v", err)
	}
	if _, err := s.Get(1); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Get err = %v", err)
	}
}
