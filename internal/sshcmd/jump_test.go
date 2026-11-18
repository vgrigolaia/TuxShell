package sshcmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

func fakeSSH(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	fake := filepath.Join(bin, "ssh")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return fake
}

func TestProxyCommand(t *testing.T) {
	fake := fakeSSH(t)
	r := Resolver{ControlDir: "/run/user/1000/tuxshell/0a1b2c3d"}
	base := "'" + fake + "' '-o' 'ControlMaster=auto' '-o' 'ControlPath=/run/user/1000/tuxshell/0a1b2c3d/jm-%%C' '-o' 'ControlPersist=5'"
	cases := []struct {
		jump  string
		batch bool
		want  string
	}{
		{"bastion", false, base + " '-W' '[%h]:%p' '--' 'bastion'"},
		{"v.grigolaia@79.137.113.19", false, base + " '-l' 'v.grigolaia' '-W' '[%h]:%p' '--' '79.137.113.19'"},
		{"ops@edge:2200,admin@bastion:2222", true, base + " '-o' 'BatchMode=yes' '-o' 'ConnectTimeout=10' '-J' 'ops@edge:2200' '-p' '2222' '-l' 'admin' '-W' '[%h]:%p' '--' 'bastion'"},
		{"root@[2001:db8::1]:2200", false, base + " '-p' '2200' '-l' 'root' '-W' '[%h]:%p' '--' '2001:db8::1'"},
		// A % in a hop (IPv6 zone) must not be expanded by the outer ssh.
		{"[fe80::1%eth0]", false, base + " '-W' '[%h]:%p' '--' 'fe80::1%%eth0'"},
	}
	for _, c := range cases {
		got, ok := r.proxyCommand(c.jump, c.batch)
		if !ok || got != c.want {
			t.Errorf("proxyCommand(%q) = %v %s\nwant %s", c.jump, ok, got, c.want)
		}
	}
	for _, bad := range []string{"-oProxyCommand=x", "a,-oFoo", "a b", "a'b", "user@", "[::1", "h:99999x", "a;reboot", "$(id)"} {
		if got, ok := r.proxyCommand(bad, false); ok {
			t.Errorf("proxyCommand(%q) accepted: %s", bad, got)
		}
	}
}

func TestArgsSharesJumpOnlyWithControlDir(t *testing.T) {
	fakeSSH(t)
	h := hosts.Host{Hostname: "172.16.0.5", User: "ansible", Port: 22, ProxyJump: "jump.user@203.0.113.7"}
	if got := (Resolver{}).args(h, false); !slices.Contains(got, "-J") {
		t.Errorf("without a control folder args = %q, want plain -J", got)
	}
	got := Resolver{ControlDir: "/run/user/1000/tuxshell/0a1b2c3d"}.args(h, false)
	if slices.Contains(got, "-J") || len(got) < 2 || got[0] != "-o" || !strings.HasPrefix(got[1], "ProxyCommand=") {
		t.Fatalf("args = %q, want a ProxyCommand and no -J", got)
	}
	if got[len(got)-2] != "--" || got[len(got)-1] != "172.16.0.5" {
		t.Errorf("destination not last: %q", got)
	}
	// A hop it can't pass through a shell falls back to -J, as before.
	h.ProxyJump = "jump@host,x y"
	if got := (Resolver{ControlDir: "/run/x"}).args(h, false); !slices.Contains(got, "-J") {
		t.Errorf("unsafe hop args = %q, want plain -J", got)
	}
}
