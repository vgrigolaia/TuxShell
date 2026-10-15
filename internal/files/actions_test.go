package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Both backends must behave the same; the remote one runs its real scripts in
// a local sh.
func backends() map[string]backend {
	if !gnuHost() {
		return map[string]backend{"local": local{}}
	}
	return map[string]backend{"remote": remote{shRunner{}}, "local": local{}}
}

// gnuHost reports whether this machine can stand in for a remote server: the
// remote scripts are written for Linux servers with GNU tools, which macOS
// (BSD head, stat and find) is not.
func gnuHost() bool { return runtime.GOOS == "linux" }

func TestRenameMkdirRemove(t *testing.T) {
	ctx := context.Background()
	for kind, b := range backends() {
		t.Run(kind, func(t *testing.T) {
			dir := populate(t)
			for _, n := range tricky {
				to := "renamed " + strings.ReplaceAll(n, "\n", "_")
				if err := b.rename(ctx, dir, n, to); err != nil {
					t.Fatalf("rename %q: %v", n, err)
				}
				if got, err := os.ReadFile(filepath.Join(dir, to)); err != nil || string(got) != "data:"+n {
					t.Errorf("after rename %q: %q, %v", n, got, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "PWNED")); err == nil {
				t.Fatal("a file name ran as a command")
			}
			// Never replaces: neither a file nor a dangling link.
			_ = os.Symlink("nowhere", filepath.Join(dir, "dangling"))
			for _, to := range []string{"renamed plain.txt", "dangling"} {
				if err := b.rename(ctx, dir, "renamed with space.log", to); !errors.Is(err, ErrExists) {
					t.Errorf("rename onto %q: %v", to, err)
				}
			}

			if err := b.mkdir(ctx, dir, "new folder"); err != nil {
				t.Fatal(err)
			}
			if err := b.mkdir(ctx, dir, "new folder"); !errors.Is(err, ErrExists) {
				t.Errorf("mkdir twice: %v", err)
			}
			_ = os.WriteFile(filepath.Join(dir, "new folder", "inner"), []byte("x"), 0o600)

			if err := b.remove(ctx, dir, []string{"new folder", "renamed it's quoted", "-not-there"}); err != nil {
				t.Fatal(err)
			}
			for _, n := range []string{"new folder", "renamed it's quoted"} {
				if _, err := os.Lstat(filepath.Join(dir, n)); !os.IsNotExist(err) {
					t.Errorf("%q not removed", n)
				}
			}
			// The link is removed, not the folder it points at.
			if err := b.remove(ctx, dir, []string{"link to dir"}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "sub dir")); err != nil {
				t.Error("removing a link removed its target")
			}
		})
	}
}

func TestChmodStatReplace(t *testing.T) {
	ctx := context.Background()
	for kind, b := range backends() {
		t.Run(kind, func(t *testing.T) {
			dir := populate(t)
			_ = os.WriteFile(filepath.Join(dir, "sub dir", "a"), []byte("x"), 0o644)
			if err := b.chmod(ctx, dir, "with space.log", 0o600, false); err != nil {
				t.Fatal(err)
			}
			if st, _ := os.Stat(filepath.Join(dir, "with space.log")); st.Mode().Perm() != 0o600 {
				t.Errorf("mode = %v", st.Mode().Perm())
			}
			if err := b.chmod(ctx, dir, "sub dir", 0o750, true); err != nil {
				t.Fatal(err)
			}
			if st, _ := os.Stat(filepath.Join(dir, "sub dir", "a")); st.Mode().Perm() != 0o750 {
				t.Errorf("recursive mode = %v", st.Mode().Perm())
			}

			file := filepath.Join(dir, "it's quoted")
			st, err := b.stat(ctx, file)
			if err != nil || st.Size != int64(len("data:it's quoted")) || st.Mode != "640" || st.ModTime == 0 {
				t.Fatalf("stat = %+v, %v", st, err)
			}
			if _, err := b.stat(ctx, filepath.Join(dir, "sub dir")); err == nil {
				t.Error("stat of a folder should fail")
			}

			if err := b.replace(ctx, file, strings.NewReader("new content")); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(file)
			info, _ := os.Stat(file)
			if string(got) != "new content" || info.Mode().Perm() != 0o640 {
				t.Errorf("after replace: %q %v", got, info.Mode().Perm())
			}
			// Through a link the target is written and the link stays.
			_ = os.Symlink("it's quoted", filepath.Join(dir, "via link"))
			if err := b.replace(ctx, filepath.Join(dir, "via link"), strings.NewReader("through link")); err != nil {
				t.Fatal(err)
			}
			if fi, _ := os.Lstat(filepath.Join(dir, "via link")); fi.Mode()&os.ModeSymlink == 0 {
				t.Error("the link was replaced by a file")
			}
			if got, _ := os.ReadFile(file); string(got) != "through link" {
				t.Errorf("target = %q", got)
			}
			// A read-only folder: written in place.
			ro := filepath.Join(dir, "ro")
			_ = os.Mkdir(ro, 0o755)
			_ = os.WriteFile(filepath.Join(ro, "f"), []byte("old"), 0o644)
			_ = os.Chmod(ro, 0o555)
			t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
			if err := b.replace(ctx, filepath.Join(ro, "f"), strings.NewReader("in place")); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(filepath.Join(ro, "f")); string(got) != "in place" {
				t.Errorf("read-only folder: %q", got)
			}
			if entries, _ := os.ReadDir(dir); len(entries) == 0 {
				t.Fatal("empty dir")
			} else {
				for _, e := range entries {
					if strings.HasPrefix(e.Name(), ".tuxshell-edit") {
						t.Errorf("temporary file left: %s", e.Name())
					}
				}
			}
		})
	}
}

func TestServiceChecks(t *testing.T) {
	s, _ := NewService(nil, nil)
	ctx := context.Background()
	loc := Location{Kind: "local"}
	dir := t.TempDir()
	for _, name := range []string{"", ".", "..", "a/b", "../x", "nul\x00"} {
		if err := s.Rename(ctx, loc, dir, "x", name); err == nil {
			t.Errorf("Rename to %q allowed", name)
		}
		if err := s.Delete(ctx, loc, dir, []string{name}); err == nil {
			t.Errorf("Delete %q allowed", name)
		}
	}
	for _, d := range []string{"", "relative", "~"} {
		if err := s.MakeDir(ctx, loc, d, "x"); err == nil {
			t.Errorf("MakeDir in %q allowed", d)
		}
	}
	for _, m := range []string{"", "9", "1777", "rwx", "0o7", "-755"} {
		if err := s.Chmod(ctx, loc, dir, "x", m, false); err == nil {
			t.Errorf("mode %q allowed", m)
		}
	}
}

func TestListHomeRelative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.MkdirAll(filepath.Join(home, "my dir", "it's"), 0o755)
	for kind, b := range backends() {
		l, err := b.list(context.Background(), "~/my dir/it's")
		want, _ := filepath.EvalSymlinks(filepath.Join(home, "my dir", "it's"))
		if err != nil || l.Path != want {
			t.Errorf("%s: list ~/… = %q, %v; want %q", kind, l.Path, err, want)
		}
	}
}

func TestEditRoundTrip(t *testing.T) {
	s, _ := NewService(nil, nil)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	events := make(chan EditEvent, 4)
	var opened string
	s.edit.open = func(p string) error { opened = p; return nil }
	s.edit.poll = 20 * time.Millisecond
	UseEvents(s, func(name string, data any) {
		if e, ok := data.(EditEvent); ok && name == EventEdited {
			events <- e
		}
	})
	ctx := context.Background()
	loc := Location{Kind: "local"}
	dir := t.TempDir()
	orig := filepath.Join(dir, "app.conf")
	_ = os.WriteFile(orig, []byte("port=1\n"), 0o640)

	e, err := s.Edit(ctx, loc, orig)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(filepath.Dir(opened)); info.Mode().Perm() != 0o700 {
		t.Errorf("edit folder mode = %v", info.Mode().Perm())
	}
	if info, _ := os.Stat(opened); info.Mode().Perm() != 0o600 {
		t.Errorf("copy mode = %v", info.Mode().Perm())
	}

	// The editor saves the copy: reported once.
	time.Sleep(30 * time.Millisecond)
	_ = os.WriteFile(opened, []byte("port=2\n"), 0o600)
	select {
	case ev := <-events:
		if ev.ID != e.ID {
			t.Errorf("event = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("save not reported")
	}
	if l := s.Edits(); len(l) != 1 || !l[0].Changed {
		t.Errorf("edits = %+v", l)
	}
	if err := s.SaveEdit(ctx, e.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(orig); string(got) != "port=2\n" {
		t.Errorf("original = %q", got)
	}
	if info, _ := os.Stat(orig); info.Mode().Perm() != 0o640 {
		t.Errorf("mode not kept: %v", info.Mode().Perm())
	}

	// Someone else changes the original: saving needs force.
	_ = os.WriteFile(orig, []byte("port=3 from someone else\n"), 0o640)
	_ = os.WriteFile(opened, []byte("port=4\n"), 0o600)
	if err := s.SaveEdit(ctx, e.ID, false); !errors.Is(err, ErrChangedOnServer) {
		t.Fatalf("conflict: %v", err)
	}
	if err := s.SaveEdit(ctx, e.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(orig); string(got) != "port=4\n" {
		t.Errorf("forced save = %q", got)
	}

	s.CloseEdit(e.ID)
	if _, err := os.Stat(filepath.Dir(opened)); !os.IsNotExist(err) {
		t.Error("copy kept after closing")
	}
	if err := s.SaveEdit(ctx, e.ID, false); err == nil {
		t.Error("save after close")
	}

	big := filepath.Join(dir, "big")
	_ = os.WriteFile(big, make([]byte, MaxEditSize+1), 0o600)
	if _, err := s.Edit(ctx, loc, big); err == nil {
		t.Error("big file opened")
	}
}
