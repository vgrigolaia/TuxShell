package cliphist

import (
	"bytes"
	"os"
	"path/filepath"
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
		if ok, err := s.Add(text); err != nil || !ok {
			t.Fatalf("Add(%q) = %v, %v", text, ok, err)
		}
	}
	if ok, _ := s.Add("PASSWORD=hunter2"); ok {
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
	if _, err := s.Add(marker); err != nil {
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
	if _, err := s.Add("secret-ish note"); err != nil {
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

	if _, err := s.Add("old"); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(Retention + time.Hour)
	if _, err := s.Add("new"); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List()
	if len(list) != 1 || list[0].Text != "new" {
		t.Fatalf("after retention = %+v", list)
	}

	for i := range MaxEntries + 5 {
		clock = clock.Add(time.Millisecond)
		if _, err := s.Add("entry " + string(rune('a'+i%26)) + time.Duration(i).String()); err != nil {
			t.Fatal(err)
		}
	}
	if list, _ = s.List(); len(list) != MaxEntries {
		t.Fatalf("len = %d, want %d", len(list), MaxEntries)
	}
}
