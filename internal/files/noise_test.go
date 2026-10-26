package files

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// noisyRunner is a server whose login files print text before every command.
type noisyRunner struct{}

func (noisyRunner) Command(ctx context.Context, script string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", "echo ok; printf 'Welcome\\n\\n'; "+script)
}

func TestLoginNoiseIsDropped(t *testing.T) {
	if !gnuHost() {
		t.Skip("the remote scripts target Linux servers with GNU tools")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	b := remote{noisyRunner{}}
	l, err := b.list(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	if l.Path != want || len(l.Entries) != 1 {
		t.Fatalf("listing = %q %v, want %q with one entry", l.Path, l.Entries, want)
	}
	var out bytes.Buffer
	if err := b.download(context.Background(), filepath.Join(dir, "a.txt"), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello" {
		t.Fatalf("download = %q, want %q", out.String(), "hello")
	}
}

func TestAfterMarker(t *testing.T) {
	const m = "tuxshell-abc"
	for name, writes := range map[string][]string{
		"no noise":     {"tuxshell-abc\nbody"},
		"noise":        {"ok\nbanner\ntuxshell-abc\nbody"},
		"split marker": {"ok\ntuxs", "hell-ab", "c\nbo", "dy"},
	} {
		var out bytes.Buffer
		a := newAfterMarker(&out, m)
		for _, w := range writes {
			if n, err := a.Write([]byte(w)); err != nil || n != len(w) {
				t.Fatalf("%s: Write = %d, %v", name, n, err)
			}
		}
		if out.String() != "body" {
			t.Errorf("%s: got %q, want %q", name, out.String(), "body")
		}
	}
	// Endless noise without a marker is never passed on, and isn't kept whole.
	var out bytes.Buffer
	a := newAfterMarker(&out, m)
	for range 100 {
		_, _ = a.Write([]byte(strings.Repeat("x", 4096)))
	}
	if out.Len() != 0 || len(a.held) > maxNoise+4096 {
		t.Fatalf("passed %d bytes, holding %d", out.Len(), len(a.held))
	}
}
