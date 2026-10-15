package login

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/vgrigolaia/tuxshell/internal/askpass"
	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/secrets"
)

type memKeyring map[string]string

func (m memKeyring) Set(s, u, p string) error { m[s+"/"+u] = p; return nil }
func (m memKeyring) Get(s, u string) (string, error) {
	if p, ok := m[s+"/"+u]; ok {
		return p, nil
	}
	return "", keyring.ErrNotFound
}
func (m memKeyring) Delete(s, u string) error { delete(m, s+"/"+u); return nil }

type fixture struct {
	svc   *Manager
	store *hosts.Store
	kr    memKeyring
	host  hosts.Host
	on    bool
	asked chan PasswordRequest
}

func setup(t *testing.T) *fixture {
	t.Helper()
	store, err := hosts.Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	h, err := store.Save(hosts.Host{Name: "db", Hostname: "10.0.0.7", User: "root", Tags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{store: store, kr: memKeyring{}, host: h, on: true, asked: make(chan PasswordRequest, 4)}
	f.svc = New(store, secrets.NewWith(f.kr), func() bool { return f.on }, func(name string, data any) {
		if r, ok := data.(PasswordRequest); ok && name == EventPassword {
			f.asked <- r
		}
	})
	if err := f.svc.Start(t.TempDir(), "/usr/bin/tuxshell"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.svc.Close() })
	return f
}

// session starts a fake session whose ssh has PID 4242 and returns its token.
func (f *fixture) session(t *testing.T) (string, func()) {
	t.Helper()
	env, started, end := f.svc.Begin("s1", f.host.ID)
	if env == nil {
		t.Fatal("session not prepared")
	}
	started(4242)
	for _, e := range env {
		if v, ok := cutPrefix(e, askpass.EnvToken+"="); ok {
			return v, end
		}
	}
	t.Fatal("no token in env")
	return "", nil
}

func cutPrefix(s, p string) (string, bool) {
	if len(s) >= len(p) && s[:len(p)] == p {
		return s[len(p):], true
	}
	return "", false
}

func (f *fixture) prompt(token, prompt string, parent int) askpass.Reply {
	return f.svc.handle(askpass.Request{Token: token, Prompt: prompt, ParentPID: parent})
}

// next waits for a password box and answers it.
func (f *fixture) next(t *testing.T, pw string, remember bool) PasswordRequest {
	t.Helper()
	select {
	case r := <-f.asked:
		go f.svc.sendAnswer(r.RequestID, pw, remember)
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("no password box")
	}
	return PasswordRequest{}
}

func TestSavedPasswordUsedOnce(t *testing.T) {
	f := setup(t)
	if err := f.svc.setPassword(f.host.ID, "right"); err != nil {
		t.Fatal(err)
	}
	tok, end := f.session(t)
	defer end()
	if r := f.prompt(tok, "root@10.0.0.7's password: ", 4242); r.Action != askpass.ActionAnswer || r.Answer != "right" {
		t.Fatalf("first prompt = %+v", r)
	}
	// Asked again: the saved one was wrong, so the user is asked.
	done := make(chan askpass.Reply)
	go func() { done <- f.prompt(tok, "root@10.0.0.7's password: ", 4242) }()
	req := f.next(t, "typed", false)
	if !req.SavedFailed || req.HostName != "db" {
		t.Errorf("request = %+v", req)
	}
	if r := <-done; r.Answer != "typed" {
		t.Errorf("second prompt = %+v", r)
	}
	if f.kr["TuxShell/host-password:"+itoa(f.host.ID)] != "right" {
		t.Error("not remembering shouldn't change the saved password")
	}
}

func TestRememberAndForgetWrong(t *testing.T) {
	f := setup(t)
	tok, end := f.session(t)
	defer end()
	done := make(chan askpass.Reply)
	go func() { done <- f.prompt(tok, "Password:", 4242) }()
	f.next(t, "first-try", true)
	<-done
	h, _ := f.store.Get(f.host.ID)
	if !h.PasswordSaved || f.kr["TuxShell/host-password:"+itoa(f.host.ID)] != "first-try" {
		t.Fatalf("not remembered: %+v %v", h, f.kr)
	}
	// Asked again: what was just remembered is wrong and is removed.
	go func() { done <- f.prompt(tok, "Password:", 4242) }()
	f.next(t, "second", false)
	<-done
	h, _ = f.store.Get(f.host.ID)
	if h.PasswordSaved || len(f.kr) != 0 {
		t.Errorf("wrong password kept: %+v %v", h, f.kr)
	}
}

func TestOtherPromptsGoToTerminal(t *testing.T) {
	f := setup(t)
	_ = f.svc.setPassword(f.host.ID, "right")
	tok, end := f.session(t)
	defer end()
	for _, c := range []struct {
		prompt string
		parent int
	}{
		{"root@bastion's password: ", 9999}, // a jump host's own ssh
		{"Enter passphrase for key '/home/me/.ssh/id_ed25519': ", 4242},
		{"Are you sure you want to continue connecting (yes/no/[fingerprint])? ", 4242},
		{"New password: ", 4242},
	} {
		if r := f.prompt(tok, c.prompt, c.parent); r.Action != askpass.ActionTTY {
			t.Errorf("%q from %d = %+v, want tty", c.prompt, c.parent, r)
		}
	}
	if r := f.prompt("unknown-token", "Password:", 4242); r.Action != askpass.ActionTTY {
		t.Errorf("unknown token = %+v", r)
	}
}

func TestDisabled(t *testing.T) {
	f := setup(t)
	f.on = false
	if env, _, _ := f.svc.Begin("s", f.host.ID); env != nil {
		t.Error("session prepared while saving passwords is off")
	}
	if err := f.svc.setPassword(f.host.ID, "x"); err != ErrDisabled {
		t.Errorf("SetPassword err = %v", err)
	}
}

func TestCancelAndSessionEnd(t *testing.T) {
	f := setup(t)
	tok, end := f.session(t)
	done := make(chan askpass.Reply)
	go func() { done <- f.prompt(tok, "Password:", 4242) }()
	r := <-f.asked
	_ = f.svc.cancelRequest(r.RequestID)
	if got := <-done; got.Action != askpass.ActionCancel {
		t.Errorf("cancel = %+v", got)
	}
	go func() { done <- f.prompt(tok, "Password:", 4242) }()
	<-f.asked
	end() // the pane was closed while the box was open
	if got := <-done; got.Action != askpass.ActionCancel {
		t.Errorf("session end = %+v", got)
	}
}

func TestForgetAll(t *testing.T) {
	f := setup(t)
	_ = f.svc.setPassword(f.host.ID, "x")
	if st, _ := f.svc.status(); !st.Available || st.Saved != 1 {
		t.Errorf("status = %+v", st)
	}
	if n, err := f.svc.forgetAll(); n != 1 || err != nil {
		t.Errorf("ForgetAll = %d, %v", n, err)
	}
	if st, _ := f.svc.status(); st.Saved != 0 || len(f.kr) != 0 {
		t.Errorf("after ForgetAll: %+v %v", st, f.kr)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// fakeVault answers references from a map and counts reads.
type fakeVault struct {
	values map[string]string
	reads  int
}

func (v *fakeVault) Read(_ context.Context, ref string) (string, error) {
	v.reads++
	if pw, ok := v.values[ref]; ok {
		return pw, nil
	}
	return "", errors.New("1Password: no such item")
}

// withRef gives the fixture's host a 1Password reference and a vault.
func (f *fixture) withRef(t *testing.T, values map[string]string) *fakeVault {
	t.Helper()
	f.host.PasswordRef = "op://Infra/db/password"
	h, err := f.store.Save(f.host)
	if err != nil {
		t.Fatal(err)
	}
	f.host = h
	v := &fakeVault{values: values}
	UseVault(f.svc, v)
	return v
}

func TestVaultPasswordFirstEvenWithSavingOff(t *testing.T) {
	f := setup(t)
	if err := f.svc.setPassword(f.host.ID, "from-keyring"); err != nil {
		t.Fatal(err)
	}
	v := f.withRef(t, map[string]string{"op://Infra/db/password": "from-vault"})
	f.on = false
	tok, end := f.session(t)
	defer end()
	if r := f.prompt(tok, "root@10.0.0.7's password: ", 4242); r.Action != askpass.ActionAnswer || r.Answer != "from-vault" {
		t.Fatalf("first prompt = %+v", r)
	}
	// Refused: the user is asked, told why, and can't save while saving is off.
	done := make(chan askpass.Reply)
	go func() { done <- f.prompt(tok, "root@10.0.0.7's password: ", 4242) }()
	req := f.next(t, "typed", false)
	if !req.VaultFailed || req.SavedFailed || req.CanSave {
		t.Errorf("request = %+v", req)
	}
	if r := <-done; r.Answer != "typed" {
		t.Errorf("second prompt = %+v", r)
	}
	if v.reads != 1 {
		t.Errorf("1Password read %d times, want once", v.reads)
	}
}

func TestVaultErrorFallsBackToSavedPassword(t *testing.T) {
	f := setup(t)
	if err := f.svc.setPassword(f.host.ID, "from-keyring"); err != nil {
		t.Fatal(err)
	}
	f.withRef(t, nil)
	tok, end := f.session(t)
	defer end()
	if r := f.prompt(tok, "Password:", 4242); r.Answer != "from-keyring" {
		t.Fatalf("first prompt = %+v", r)
	}
}

func TestVaultErrorShownInPasswordBox(t *testing.T) {
	f := setup(t)
	f.withRef(t, nil)
	f.on = false
	tok, end := f.session(t)
	defer end()
	done := make(chan askpass.Reply)
	go func() { done <- f.prompt(tok, "Password:", 4242) }()
	req := f.next(t, "typed", false)
	if req.VaultError != "1Password: no such item" || req.VaultFailed {
		t.Errorf("request = %+v", req)
	}
	<-done
}

func TestNoRefNoSavingStaysInTerminal(t *testing.T) {
	f := setup(t)
	UseVault(f.svc, &fakeVault{})
	f.on = false
	if env, _, _ := f.svc.Begin("s1", f.host.ID); env != nil {
		t.Fatal("a host without a reference took part while saving is off")
	}
}
