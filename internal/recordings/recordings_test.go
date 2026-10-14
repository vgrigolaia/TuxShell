package recordings

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteListLoadDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rec")
	w, name, err := Create(dir, "pg-primary / prod", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, "_pg-primary-prod.cast") {
		t.Errorf("name = %q", name)
	}
	// "é" (C3 A9) and "€" (E2 82 AC) split across writes.
	for _, chunk := range [][]byte{[]byte("caf\xC3"), []byte("\xA9 \xE2\x82"), []byte("\xAC\r\n")} {
		if err := w.Output(chunk); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Resize(80, 24)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v", info.Mode().Perm())
	}
	if info, _ := os.Stat(filepath.Join(dir, name)); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v", info.Mode().Perm())
	}

	f, _ := os.Open(filepath.Join(dir, name))
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Scan()
	var h header
	if json.Unmarshal(sc.Bytes(), &h) != nil || h.Version != 2 || h.Width != 120 || h.Height != 40 || h.Title != "pg-primary / prod" {
		t.Fatalf("header = %s", sc.Text())
	}
	var out strings.Builder
	var kinds []string
	for sc.Scan() {
		var ev []any
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil || len(ev) != 3 {
			t.Fatalf("event %q: %v", sc.Text(), err)
		}
		kinds = append(kinds, ev[1].(string))
		if ev[1] == "o" {
			out.WriteString(ev[2].(string))
		}
	}
	if out.String() != "café €\r\n" {
		t.Errorf("output = %q", out.String())
	}
	if strings.Join(kinds, ",") != "o,o,o,r" {
		t.Errorf("events = %v", kinds)
	}

	s := NewService(dir)
	list, err := s.List()
	if err != nil || len(list) != 1 || list[0].Name != name || list[0].Title != "pg-primary / prod" || list[0].Started == 0 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if data, err := s.Load(name); err != nil || !strings.Contains(data, `"version":2`) {
		t.Errorf("load = %.40q, %v", data, err)
	}
	for _, bad := range []string{"../x.cast", ".hidden.cast", "x.txt", "a/b.cast"} {
		if _, err := s.Load(bad); err == nil {
			t.Errorf("Load(%q) allowed", bad)
		}
		if err := s.Delete(bad); err == nil {
			t.Errorf("Delete(%q) allowed", bad)
		}
	}
	if err := s.Delete(name); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Errorf("after delete: %+v", list)
	}
	if list, err := NewService(filepath.Join(dir, "missing")).List(); err != nil || len(list) != 0 {
		t.Errorf("missing dir: %v, %v", list, err)
	}
}

func TestCompleteUTF8(t *testing.T) {
	for in, want := range map[string]int{"abc": 3, "a\xC3": 1, "a\xC3\xA9": 3, "\xE2\x82": 0, "\xF0\x9F\x98": 0, "\xF0\x9F\x98\x80": 4, "": 0} {
		if got := completeUTF8([]byte(in)); got != want {
			t.Errorf("completeUTF8(%q) = %d, want %d", in, got, want)
		}
	}
}
