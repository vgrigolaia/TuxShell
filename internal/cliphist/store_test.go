package cliphist

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func openStore(t *testing.T, path string, key []byte) *Store {
	t.Helper()
	s, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAddListDedupeDelete(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "clip.db"), testKey(1))
	clock := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return clock }

	for _, text := range []string{"first", "second", "first"} {
		clock = clock.Add(time.Second)
		if ok, err := s.Add(text, DefaultSettings); err != nil || !ok {
			t.Fatalf("Add(%q) = %v, %v", text, ok, err)
		}
	}
	if ok, _ := s.Add("PASSWORD=hunter2", DefaultSettings); ok {
		t.Fatal("sensitive text recorded")
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	// "first" was re-copied, so it moves to the top instead of duplicating.
	if len(list) != 2 || list[0].Text != "first" || list[1].Text != "second" {
		t.Fatalf("list = %+v", list)
	}

	if err := s.Delete(list[1].ID); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.List(); len(list) != 1 {
		t.Fatalf("after delete = %+v", list)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.List(); len(list) != 0 {
		t.Fatalf("after clear = %+v", list)
	}
}

func TestPlaintextNeverOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.db")
	s := openStore(t, path, testKey(2))
	marker := "kubectl get secret very-unique-marker-text"
	if _, err := s.Add(marker, DefaultSettings); err != nil {
		t.Fatal(err)
	}
	s.Close()

	files, _ := filepath.Glob(path + "*")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if bytes.Contains(b, []byte("very-unique-marker")) {
			t.Fatalf("plaintext found in %s", filepath.Base(f))
		}
		if info, _ := os.Stat(f); info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o", filepath.Base(f), info.Mode().Perm())
		}
	}
}

func TestWrongKeyHidesEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.db")
	s := openStore(t, path, testKey(3))
	if _, err := s.Add("secret-ish note", DefaultSettings); err != nil {
		t.Fatal(err)
	}
	s.Close()

	other := openStore(t, path, testKey(4))
	list, err := other.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("wrong key list = %+v, %v", list, err)
	}
}

func TestRetentionAndCap(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "clip.db"), testKey(5))
	clock := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return clock }
	set := Settings{RetentionDays: 7, MaxEntries: 3}

	if _, err := s.Add("old", set); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveClip("pinned and old", ""); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(8 * 24 * time.Hour)
	for _, text := range []string{"a1", "a2", "a3", "a4"} {
		clock = clock.Add(time.Second)
		if _, err := s.Add(text, set); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.List()
	var got []string
	for _, c := range list {
		got = append(got, c.Text)
	}
	// Pinned survives both retention and the cap; history keeps the newest 3.
	want := []string{"pinned and old", "a4", "a3", "a2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("list = %q, want %q", got, want)
	}

	// 0 means forever / no limit.
	unlimited := Settings{}
	for i := range 10 {
		clock = clock.Add(time.Second)
		s.Add(fmt.Sprintf("bulk %d", i), unlimited)
	}
	clock = clock.Add(400 * 24 * time.Hour)
	s.Prune(unlimited)
	if list, _ = s.List(); len(list) != 14 {
		t.Fatalf("unlimited kept %d, want 14", len(list))
	}
}

func TestSavedClips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.db")
	s := openStore(t, path, testKey(6))

	c, err := s.SaveClip("sudo systemctl restart patroni", "restart patroni")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Pinned || c.Label != "restart patroni" {
		t.Fatalf("saved = %+v", c)
	}
	if _, err := s.SaveClip("export TOKEN=abc123", ""); !errors.Is(err, ErrSensitive) {
		t.Fatalf("sensitive save err = %v", err)
	}

	// Pinning an existing history entry by saving the same text.
	s.Add("df -h", DefaultSettings)
	again, err := s.SaveClip("df -h", "disk")
	if err != nil || !again.Pinned {
		t.Fatalf("pin existing = %+v, %v", again, err)
	}

	edited, err := s.UpdateClip(c.ID, "sudo systemctl restart patroni --no-block", "restart (async)")
	if err != nil || edited.Text != "sudo systemctl restart patroni --no-block" || edited.Label != "restart (async)" {
		t.Fatalf("edited = %+v, %v", edited, err)
	}
	// Editing to text another entry already has merges into this one.
	if _, err := s.UpdateClip(c.ID, "df -h", "merged"); err != nil {
		t.Fatal(err)
	}

	s.Add("unpinned", DefaultSettings)
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List()
	if len(list) != 1 || list[0].Label != "merged" {
		t.Fatalf("after clear = %+v", list)
	}

	if err := s.SetPinned(list[0].ID, false); err != nil {
		t.Fatal(err)
	}
	s.Clear()
	if list, _ = s.List(); len(list) != 0 {
		t.Fatalf("unpinned survived clear: %+v", list)
	}

	// Labels are encrypted too.
	s.SaveClip("ls", "very-unique-label-marker")
	s.Close()
	files, _ := filepath.Glob(path + "*")
	for _, f := range files {
		if b, _ := os.ReadFile(f); bytes.Contains(b, []byte("very-unique-label")) {
			t.Fatalf("label plaintext in %s", filepath.Base(f))
		}
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "clip.db"), testKey(7))
	if got, _ := s.Settings(); got != DefaultSettings {
		t.Fatalf("defaults = %+v", got)
	}
	want := Settings{RetentionDays: 0, MaxEntries: 5000, CaptureSystem: false}
	if err := s.SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Settings(); got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
	for _, bad := range []Settings{{RetentionDays: -1}, {RetentionDays: 99999}, {MaxEntries: -5}, {MaxEntries: 1 << 30}} {
		if err := s.SaveSettings(bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestMigratesVersionZeroDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.db")
	// Build a v0 database the way the first release did.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0]); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s := openStore(t, path, testKey(8))
	if _, err := s.Add("survives upgrade", DefaultSettings); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = openStore(t, path, testKey(8))
	if list, _ := s.List(); len(list) != 1 || list[0].Pinned {
		t.Fatalf("after upgrade = %+v", list)
	}
}
