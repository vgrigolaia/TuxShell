package onepassword

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeOp writes a stand-in for op that logs its arguments to args.txt and
// then runs body.
func fakeOp(t *testing.T, body string) (Client, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + log + "'\n" + body + "\n"
	path := filepath.Join(dir, "op")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return Client{Path: path}, log
}

func TestValidRef(t *testing.T) {
	for ref, want := range map[string]bool{
		"op://Infra/db-01/password":                        true,
		"op://My Vault/Web Server/admin/pass":              true,
		"op://Infra/db-01/one-time password?attribute=otp": true,
		"op://Infra/db-01":                                 false,
		"op://Infra//password":                             false,
		"op://a/b/c/d/e":                                   false,
		"-op://Infra/db-01/password":                       false,
		"op://Infra/db\n01/password":                       false,
		"https://Infra/db-01/password":                     false,
		"":                                                 false,
	} {
		if got := ValidRef(ref); got != want {
			t.Errorf("ValidRef(%q) = %v, want %v", ref, got, want)
		}
	}
}

func TestReadPassesTheReferenceAfterDoubleDash(t *testing.T) {
	c, log := fakeOp(t, "printf 'hunter2'") // gitleaks:allow
	got, err := c.Read(context.Background(), "op://Infra/db-01/password")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hunter2" { // gitleaks:allow
		t.Fatalf("Read = %q", got)
	}
	args, _ := os.ReadFile(log)
	if want := "read\n--no-newline\n--\nop://Infra/db-01/password\n"; string(args) != want {
		t.Fatalf("op args = %q, want %q", args, want)
	}
}

func TestReadReportsOpErrors(t *testing.T) {
	c, _ := fakeOp(t, `echo '[ERROR] 2026/10/15 15:04:05 could not read secret "op://Infra/x/password": "x" isn'"'"'t an item' >&2; exit 1`)
	_, err := c.Read(context.Background(), "op://Infra/x/password")
	if err == nil || err.Error() != `1Password: could not read secret "op://Infra/x/password": "x" isn't an item` {
		t.Fatalf("err = %v", err)
	}
}

func TestReadRejectsBadReferencesAndEmptyValues(t *testing.T) {
	c, log := fakeOp(t, "exit 0")
	if _, err := c.Read(context.Background(), "--help"); err == nil {
		t.Fatal("a non-reference was passed to op")
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("op ran for a bad reference")
	}
	if _, err := c.Read(context.Background(), "op://Infra/db-01/password"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty value: err = %v", err)
	}
}

func TestNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	old := candidates
	candidates = nil
	t.Cleanup(func() { candidates = old })
	if _, err := (Client{}).Read(context.Background(), "op://Infra/db-01/password"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func TestVersion(t *testing.T) {
	c, _ := fakeOp(t, "echo 2.31.1")
	if v, err := c.Version(context.Background()); err != nil || v != "2.31.1" {
		t.Fatalf("Version = %q, %v", v, err)
	}
}

func TestServiceCheckNeverReturnsTheValue(t *testing.T) {
	c, _ := fakeOp(t, "printf 'hunter2'") // gitleaks:allow
	s := Service{Client: c}
	if msg := s.Check(context.Background(), " op://Infra/db-01/password "); msg != "" {
		t.Fatalf("Check = %q, want empty", msg)
	}
	if msg := s.Check(context.Background(), "op://Infra"); msg == "" || strings.Contains(msg, "hunter2") {
		t.Fatalf("Check of a bad reference = %q", msg)
	}
}

func TestServiceStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	_ = os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Host *\n  IdentityAgent ~/.1password/agent.sock\n"), 0o600)
	c, _ := fakeOp(t, "echo 2.31.1")
	st := (&Service{Client: c}).Status(context.Background())
	if !st.Installed || st.Version != "2.31.1" || st.Agent || !st.ConfigUsesAgent {
		t.Fatalf("Status = %+v", st)
	}
}
