package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowState(t *testing.T) {
	path := WindowStatePath(filepath.Join(t.TempDir(), "settings.json"))
	if got := LoadWindowState(path); got != (WindowState{}) {
		t.Fatalf("missing file: %+v", got)
	}
	want := WindowState{Width: 1400, Height: 900, Maximized: true}
	if err := SaveWindowState(path, want); err != nil {
		t.Fatal(err)
	}
	if got := LoadWindowState(path); got != want {
		t.Fatalf("round trip: %+v", got)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	// Out of range sizes fall back to the default; the maximized flag stays.
	os.WriteFile(path, []byte(`{"width":100,"height":50000,"maximized":true}`), 0o600)
	if got := LoadWindowState(path); got != (WindowState{Maximized: true}) {
		t.Fatalf("bad size: %+v", got)
	}
	os.WriteFile(path, []byte(`{nope`), 0o600)
	if got := LoadWindowState(path); got != (WindowState{}) {
		t.Fatalf("broken file: %+v", got)
	}
}
