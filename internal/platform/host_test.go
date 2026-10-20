package platform

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestHostCommandOutsideFlatpak(t *testing.T) {
	old := Flatpak
	Flatpak = false
	t.Cleanup(func() { Flatpak = old })
	cmd := HostCommand(context.Background(), []string{"A=1"}, "/tmp", "sh", "-c", "true")
	if filepath.Base(cmd.Path) != "sh" || !slices.Equal(cmd.Args, []string{"sh", "-c", "true"}) || cmd.Dir != "/tmp" {
		t.Fatalf("cmd = %q %q in %q", cmd.Path, cmd.Args, cmd.Dir)
	}
	if !slices.Contains(cmd.Env, "A=1") || len(cmd.Env) < 2 {
		t.Fatalf("env should be this process's plus A=1, got %d entries", len(cmd.Env))
	}
	if HostCommand(context.Background(), nil, "", "true").Env != nil {
		t.Fatal("no extra env should keep the inherited environment")
	}
}

func TestHostCommandInFlatpak(t *testing.T) {
	old := Flatpak
	Flatpak = true
	t.Cleanup(func() { Flatpak = old })
	cmd := HostCommand(context.Background(), []string{"TERM=xterm-256color"}, "/home/me", "/bin/bash", "-l")
	want := []string{"flatpak-spawn", "--host", "--watch-bus", "--env=TERM=xterm-256color", "--directory=/home/me", "--", "/bin/bash", "-l"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}
	if cmd.Cancel == nil || cmd.WaitDelay == 0 {
		t.Fatal("cancelling should send SIGTERM and wait, not SIGKILL")
	}
	// A name that looks like an option still comes after "--".
	cmd = HostCommand(context.Background(), nil, "", "op", "--version")
	if !slices.Equal(cmd.Args, []string{"flatpak-spawn", "--host", "--watch-bus", "--", "op", "--version"}) {
		t.Fatalf("args = %q", cmd.Args)
	}
}

func TestLookHostPathOutsideFlatpak(t *testing.T) {
	old := Flatpak
	Flatpak = false
	t.Cleanup(func() { Flatpak = old })
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755)
	plain := filepath.Join(dir, "plain")
	os.WriteFile(plain, []byte("x"), 0o644)
	t.Setenv("PATH", "")
	if p, err := LookHostPath("tool", plain, tool); err != nil || p != tool {
		t.Fatalf("LookHostPath = %q, %v; want the executable fallback", p, err)
	}
	if _, err := LookHostPath("tool", plain); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	t.Setenv("PATH", dir)
	if p, err := LookHostPath("tool"); err != nil || p != tool {
		t.Fatalf("LookHostPath on PATH = %q, %v", p, err)
	}
}

func TestTilde(t *testing.T) {
	t.Setenv("HOME", "/home/me")
	for in, want := range map[string]string{
		"/home/me":                     "~",
		"/home/me/.config/tuxshell":    "~/.config/tuxshell",
		"/home/meadow/x":               "/home/meadow/x",
		"/var/lib/x":                   "/var/lib/x",
		"/home/me/.var/app/io.x/data/": "~/.var/app/io.x/data/",
	} {
		if got := Tilde(in); got != want {
			t.Errorf("Tilde(%q) = %q, want %q", in, got, want)
		}
	}
}
