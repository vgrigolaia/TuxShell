package sshcmd

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

func TestControlDirPrivate(t *testing.T) {
	rt := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", rt)
	dir, err := ControlDir()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir %s mode = %v, %v", dir, info.Mode().Perm(), err)
	}
	if len(dir)+len("/cm-")+40 >= maxSocketPath {
		t.Fatalf("socket path would be too long: %s", dir)
	}
}

func TestControlDirRejectsSymlink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "tuxshell")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDir(link); err == nil {
		t.Fatal("a symlinked control dir was accepted")
	}
}

func TestSessionAndBatchArgs(t *testing.T) {
	bin := t.TempDir()
	fake := filepath.Join(bin, "ssh")
	os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", bin)

	store, err := hosts.Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h, _ := store.Save(hosts.Host{Name: "web", Hostname: "web.internal", Port: 22})
	r := Resolver{Store: store, ControlDir: "/run/user/1000/tuxshell"}

	cmd, err := r.HostCommand(h.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{fake, "-o", "ControlMaster=auto", "-o", "ControlPath=/run/user/1000/tuxshell/cm-%C", "-o", "ControlPersist=120", "-p", "22", "--", "web.internal"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("session args = %q", cmd.Args)
	}

	rem, err := r.HostRemote(h.ID, "admin")
	if err != nil {
		t.Fatal(err)
	}
	got := rem.Command(context.Background(), "echo hi").Args
	joined := strings.Join(got, " ")
	for _, part := range []string{"-T", "BatchMode=yes", "ControlMaster=no", "ControlPath=/run/user/1000/tuxshell/cm-%C", "-l admin", "-- web.internal echo hi"} {
		if !strings.Contains(joined, part) {
			t.Errorf("batch args %q missing %q", joined, part)
		}
	}
	// The script is a single trailing argument, never split.
	if got[len(got)-1] != "echo hi" {
		t.Errorf("script arg = %q", got[len(got)-1])
	}
	if _, err := r.HostRemote(h.ID, "-oProxyCommand=x"); err == nil {
		t.Fatal("option-like user accepted for file access")
	}
}
