package backgrounds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChooseCopiesAndPrunes(t *testing.T) {
	src := t.TempDir()
	png := filepath.Join(src, "a.png")
	os.WriteFile(png, []byte("\x89PNG\r\n\x1a\nfirst"), 0o644)
	jpg := filepath.Join(src, "photo.jpeg")
	os.WriteFile(jpg, []byte("\xFF\xD8\xFF\xE0second"), 0o644)
	txt := filepath.Join(src, "fake.png")
	os.WriteFile(txt, []byte("not an image"), 0o644)

	dir := filepath.Join(t.TempDir(), "bg")
	var next string
	var used []string
	s := NewService(dir, func() (string, error) { return next, nil }, func() []string { return used })

	next = png
	a, err := s.Choose()
	if err != nil || !nameRe.MatchString(a) || !strings.HasSuffix(a, ".png") {
		t.Fatalf("Choose = %q, %v", a, err)
	}
	if st, _ := os.Stat(filepath.Join(dir, a)); st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", st.Mode().Perm())
	}
	url, err := s.Data(a)
	if err != nil || !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("Data = %.40q, %v", url, err)
	}

	// The first image isn't used by any profile, so adding a second removes it.
	next = jpg
	b, err := s.Choose()
	if err != nil || !strings.HasSuffix(b, ".jpg") {
		t.Fatalf("Choose = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, a)); !os.IsNotExist(err) {
		t.Error("unused image kept")
	}

	next = txt
	if _, err := s.Choose(); err != ErrNotImage {
		t.Errorf("text file: %v", err)
	}
	next = ""
	if n, err := s.Choose(); n != "" || err != nil {
		t.Errorf("cancel = %q, %v", n, err)
	}
	for _, bad := range []string{"../hosts.db", "x.png", b + "/.."} {
		if _, err := s.Data(bad); err == nil {
			t.Errorf("Data(%q) allowed", bad)
		}
	}
}
