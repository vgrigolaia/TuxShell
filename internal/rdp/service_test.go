package rdp

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

type memPasswords map[int64]string

func (m memPasswords) Saved(id int64) (string, error) { return m[id], nil }
func (m memPasswords) Save(id int64, p string) error  { m[id] = p; return nil }

// fakeClient puts an "xfreerdp" on PATH that records its arguments and stdin.
func fakeClient(t *testing.T, exit int) (argvFile, stdinFile string) {
	t.Helper()
	bin := t.TempDir()
	argvFile, stdinFile = filepath.Join(bin, "argv"), filepath.Join(bin, "stdin")
	script := "#!/bin/sh\nif [ \"$1\" = /version ]; then echo 'This is FreeRDP version 3.32.1 (n/a)'; exit 0; fi\n" +
		"echo \"$@\" > " + argvFile + "\ncat > " + stdinFile + "\necho '[ERROR] ERRCONNECT_LOGON_FAILURE [0x00020014]'\nexit " + strconv.Itoa(exit) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "xfreerdp"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	return argvFile, stdinFile
}

func TestConnectFlow(t *testing.T) {
	argvFile, stdinFile := fakeClient(t, 131)
	store, err := hosts.Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cert := selfSigned(t)
	addr := fakeRDP(t, cert, protocolSSL)
	host, port, _ := net.SplitHostPort(addr)
	p, _ := strconv.Atoi(port)
	h, err := store.Save(hosts.Host{Name: "win", Hostname: host, Port: p, User: "admin", Protocol: hosts.ProtocolRDP, RDP: hosts.DefaultRDP(), Tags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	changes := make(chan struct{}, 10)
	pw := memPasswords{}
	s := NewService(store, nil, pw, func() string { return "auto" }, func(string, any) { changes <- struct{}{} })
	ctx := context.Background()

	// First connection: the certificate must be trusted first.
	_, err = s.Connect(ctx, h.ID, ConnectOptions{Password: "S3cret pass"})
	var ce *CertError
	if !errors.As(err, &ce) || ce.Kind != "new" || ce.Cert.Subject != "WIN-TEST" {
		t.Fatalf("first connect: %v", err)
	}
	sess, err := s.Connect(ctx, h.ID, ConnectOptions{Password: "S3cret pass", Remember: true, Trust: ce.Cert.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(h.ID); got.RDPCert != ce.Cert.Fingerprint {
		t.Error("certificate not pinned")
	}
	if pw[h.ID] != "S3cret pass" {
		t.Error("password not remembered")
	}

	// Wait for the fake client to end.
	deadline := time.After(5 * time.Second)
	for {
		if l := s.Sessions(); len(l) == 1 && l[0].State == "ended" {
			if l[0].Error != "Wrong user name or password." || l[0].ID != sess.ID {
				t.Errorf("session = %+v", l[0])
			}
			break
		}
		select {
		case <-changes:
		case <-deadline:
			t.Fatalf("never ended: %+v", s.Sessions())
		}
	}
	argv, _ := os.ReadFile(argvFile)
	stdin, _ := os.ReadFile(stdinFile)
	if strings.TrimSpace(string(argv)) != "/args-from:stdin" {
		t.Errorf("argv = %q", argv)
	}
	if !strings.Contains(string(stdin), "/p:S3cret pass\n") || !strings.Contains(string(stdin), "/cert:fingerprint:sha256:"+ce.Cert.Fingerprint) {
		t.Errorf("stdin = %q", stdin)
	}
	if len(s.Log(sess.ID)) == 0 {
		t.Error("no log kept")
	}

	// The saved password is used when none is typed.
	if _, err := s.Connect(ctx, h.ID, ConnectOptions{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if stdin, _ := os.ReadFile(stdinFile); !strings.Contains(string(stdin), "/p:S3cret pass\n") {
		t.Errorf("saved password not used: %q", stdin)
	}

	// A different certificate is refused as "changed".
	_ = store.SetRDPCert(h.ID, strings.Repeat("0", 64))
	if _, err := s.Connect(ctx, h.ID, ConnectOptions{}); !errors.As(err, &ce) || ce.Kind != "changed" || ce.Previous != strings.Repeat("0", 64) {
		t.Fatalf("changed cert: %v", err)
	}

	// Nothing listening: a plain reason.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	h2, _ := store.Save(hosts.Host{Name: "gone", Hostname: "127.0.0.1", Port: closed, Protocol: hosts.ProtocolRDP, RDP: hosts.DefaultRDP(), Tags: []string{}})
	if _, err := s.Connect(ctx, h2.ID, ConnectOptions{}); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("closed port: %v", err)
	}
}

func TestExplain(t *testing.T) {
	if explain([]string{"x ERRINFO_LOGOFF_BY_USER y"}, errors.New("exit status 2")) != "" {
		t.Error("logoff should be a normal end")
	}
	if explain([]string{"ERRCONNECT_ACCOUNT_LOCKED_OUT"}, nil) != "The account is locked." {
		t.Error("locked")
	}
}
