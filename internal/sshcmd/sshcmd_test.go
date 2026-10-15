package sshcmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

func TestArgs(t *testing.T) {
	home, _ := os.UserHomeDir()
	cases := []struct {
		name string
		host hosts.Host
		want []string
	}{
		{
			name: "plain",
			host: hosts.Host{Hostname: "10.0.0.5", Port: 22},
			want: []string{"-p", "22", "--", "10.0.0.5"},
		},
		{
			name: "all fields",
			host: hosts.Host{Hostname: "db", Port: 2222, User: "pg", ProxyJump: "ops@bastion", IdentityFile: "~/.ssh/id_ed25519"},
			want: []string{"-p", "2222", "-l", "pg", "-J", "ops@bastion", "-i", filepath.Join(home, ".ssh/id_ed25519"), "--", "db"},
		},
		{
			name: "alias",
			host: hosts.Host{Alias: "prod-db", Hostname: "prod-db", Port: 22},
			want: []string{"-p", "22", "--", "prod-db"},
		},
		{
			name: "alias with edited hostname",
			host: hosts.Host{Alias: "prod-db", Hostname: "10.1.1.1", Port: 22},
			want: []string{"-p", "22", "-o", "HostName=10.1.1.1", "--", "prod-db"},
		},
	}
	for _, c := range cases {
		if got := Args(c.host); !slices.Equal(got, c.want) {
			t.Errorf("%s: Args = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParseTarget(t *testing.T) {
	cases := map[string]hosts.Host{
		"example.com":          {Hostname: "example.com", Port: 22},
		"root@10.0.0.1":        {User: "root", Hostname: "10.0.0.1", Port: 22},
		"me@corp@jump:2200":    {User: "me@corp", Hostname: "jump", Port: 2200},
		"[fe80::1]:2222":       {Hostname: "fe80::1", Port: 2222},
		"admin@[2001:db8::1]":  {User: "admin", Hostname: "2001:db8::1", Port: 22},
		"  spaced.example.org": {Hostname: "spaced.example.org", Port: 22},
	}
	for in, want := range cases {
		got, err := ParseTarget(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got.User != want.User || got.Hostname != want.Hostname || got.Port != want.Port {
			t.Errorf("%q: got %s@%s:%d, want %s@%s:%d", in, got.User, got.Hostname, got.Port, want.User, want.Hostname, want.Port)
		}
	}
}

func TestParseTargetRejects(t *testing.T) {
	for _, in := range []string{"", "-oProxyCommand=x", "-v@host", "host:abc", "two words", "[::1"} {
		var ve *hosts.ValidationError
		if _, err := ParseTarget(in); !errors.As(err, &ve) {
			t.Errorf("%q: err = %v, want validation error", in, err)
		}
	}
}

func TestResolverUsesStoreAndPath(t *testing.T) {
	bin := t.TempDir()
	fake := filepath.Join(bin, "ssh")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	store, err := hosts.Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h, err := store.Save(hosts.Host{Name: "web", Hostname: "web.internal", User: "deploy", Port: 2222})
	if err != nil {
		t.Fatal(err)
	}

	r := Resolver{Store: store}
	cmd, err := r.HostCommand(h.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{fake, "-p", "2222", "-l", "deploy", "--", "web.internal"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}

	// A profile's command goes after the destination, with a terminal.
	r.RemoteCommand = func(got hosts.Host) string {
		if got.ID == h.ID {
			return "tmux attach; exec $SHELL -l"
		}
		return ""
	}
	cmd, _ = r.HostCommand(h.ID)
	want = []string{fake, "-t", "-p", "2222", "-l", "deploy", "--", "web.internal", "tmux attach; exec $SHELL -l"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}
	r.RemoteCommand = nil

	if _, err := r.HostCommand(h.ID + 100); !errors.Is(err, hosts.ErrNotFound) {
		t.Fatalf("missing host: err = %v", err)
	}
	if _, err := r.TargetCommand("-oProxyCommand=evil"); err == nil {
		t.Fatal("option-like target accepted")
	}
}

func TestHostCommandAs(t *testing.T) {
	bin := t.TempDir()
	fake := filepath.Join(bin, "ssh")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	store, err := hosts.Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h, err := store.Save(hosts.Host{Name: "node", Alias: "node", Hostname: "10.0.0.9"})
	if err != nil {
		t.Fatal(err)
	}

	r := Resolver{Store: store}
	cmd, err := r.HostCommandAs(h.ID, "admin")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{fake, "-p", "22", "-l", "admin", "-o", "HostName=10.0.0.9", "--", "node"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}
	if _, err := r.HostCommandAs(h.ID, "-oProxyCommand=evil"); err == nil {
		t.Fatal("option-like user accepted")
	}
	// The override is per-connection; the stored host is unchanged.
	if got, _ := store.Get(h.ID); got.User != "" {
		t.Fatalf("stored user = %q, want empty", got.User)
	}
}

func TestJumpTunnelCommand(t *testing.T) {
	bin := t.TempDir()
	fake := filepath.Join(bin, "ssh")
	_ = os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", bin)
	cmd, err := Resolver{}.JumpTunnelCommand("ops@edge:2200,admin@bastion", LocalForward(40000, "10.0.5.4", 3389))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cmd.Args, " ")
	for _, want := range []string{"-L 127.0.0.1:40000:10.0.5.4:3389", "-l admin", "-J ops@edge:2200", "-- bastion", "BatchMode=yes"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	for _, bad := range []string{"", "-oProxyCommand=x", "a,,"} {
		if _, err := (Resolver{}).JumpTunnelCommand(bad, LocalForward(1, "x", 1)); err == nil {
			t.Errorf("jump %q accepted", bad)
		}
	}
}

func TestAgentIsPassedAsAuthSock(t *testing.T) {
	f := LocalForward(40000, "10.0.5.4", 3389)
	cmd, err := Resolver{Agent: func() string { return "/home/me/.1password/agent.sock" }}.JumpTunnelCommand("admin@bastion", f)
	if err != nil {
		t.Fatal(err)
	}
	if got := cmd.Env[len(cmd.Env)-1]; got != "SSH_AUTH_SOCK=/home/me/.1password/agent.sock" {
		t.Fatalf("last env entry = %q", got)
	}
	if slices.ContainsFunc(cmd.Args, func(a string) bool { return strings.Contains(a, "IdentityAgent") }) {
		t.Fatalf("args override ~/.ssh/config: %v", cmd.Args)
	}
	for _, agent := range []func() string{nil, func() string { return "" }} {
		cmd, _ := Resolver{Agent: agent}.JumpTunnelCommand("admin@bastion", f)
		if cmd.Env != nil {
			t.Fatalf("env set without an agent: %v", cmd.Env)
		}
	}
}
