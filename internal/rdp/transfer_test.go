package rdp

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveUnder(t *testing.T) {
	dir := t.TempDir()
	got, err := saveUnder(dir, `docs\img`, "a.txt", strings.NewReader("one"))
	if err != nil || got != filepath.Join(dir, "docs", "img", "a.txt") {
		t.Fatalf("save = %q, %v", got, err)
	}
	// Never replaces: the second gets a number.
	got, err = saveUnder(dir, `docs\img`, "a.txt", strings.NewReader("two"))
	if err != nil || filepath.Base(got) != "a (1).txt" {
		t.Fatalf("second save = %q, %v", got, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "docs", "img", "a.txt")); string(b) != "one" {
		t.Errorf("first file changed: %q", b)
	}
	for _, bad := range [][2]string{{`..`, "x"}, {`a\..\..`, "x"}, {"", "../x"}, {"", `a\b`}, {"", ""}, {"", ".."}} {
		if _, err := saveUnder(dir, bad[0], bad[1], strings.NewReader("x")); err == nil {
			t.Errorf("saved %q / %q", bad[0], bad[1])
		}
	}
	// A link planted inside Downloads doesn't lead out of it.
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := saveUnder(dir, "link", "x.txt", strings.NewReader("x")); err == nil {
		t.Error("wrote through a link")
	}
	if _, err := os.Stat(filepath.Join(outside, "x.txt")); err == nil {
		t.Error("file landed outside Downloads")
	}
}

func TestOfferAndSave(t *testing.T) {
	r, err := StartRelay()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "folder", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(src, "folder", "sub", "b.txt"), []byte("bee"), 0o600)
	_ = os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o600)

	// Nothing works before the tab's connection is open.
	if _, err := r.offer("k1", []string{filepath.Join(src, "a.txt")}); err == nil {
		t.Fatal("offered without a live tab")
	}
	r.openTab("k1")
	files, err := r.offer("k1", []string{filepath.Join(src, "a.txt"), filepath.Join(src, "folder")})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Path+"|"+f.Name)
	}
	if strings.Join(names, ",") != `|a.txt,|folder,folder|sub,folder\sub|b.txt` {
		t.Fatalf("offered = %v", names)
	}
	get := func(url string) (int, string) {
		resp, err := http.Get(url) //nolint:gosec // test server
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := get(files[0].URL); code != 200 || body != "hello" {
		t.Fatalf("read = %d %q", code, body)
	}
	if code, _ := get(files[0].URL); code != http.StatusNotFound {
		t.Error("link worked twice")
	}

	// Saving needs the live key.
	dl := t.TempDir()
	t.Setenv("HOME", dl)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dl, "cfg"))
	post := func(key string) int {
		resp, err := http.Post(r.base()+"/save?k="+key+"&path=d&name=c.txt", "application/octet-stream", strings.NewReader("sea"))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if code := post("nope"); code != http.StatusForbidden {
		t.Errorf("unknown key = %d", code)
	}
	if code := post("k1"); code != 200 {
		t.Fatalf("save = %d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(dl, "Downloads", "d", "c.txt")); string(b) != "sea" {
		t.Errorf("saved %q", b)
	}
	// After the connection ends, the key and the links are gone.
	r.closeTab("k1")
	if code := post("k1"); code != http.StatusForbidden {
		t.Errorf("closed tab key = %d", code)
	}
	if code, _ := get(files[3].URL); code != http.StatusNotFound {
		t.Error("link outlived its tab")
	}
}
