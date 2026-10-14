package hostio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

func newService(t *testing.T) (*Service, *hosts.Store) {
	t.Helper()
	store, err := hosts.Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return NewService(store, nil), store
}

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func lines(p *Preview) []int {
	out := []int{}
	for _, r := range p.Rows {
		out = append(out, r.Line)
	}
	return out
}

func TestCSVPreviewAndImport(t *testing.T) {
	s, store := newService(t)
	if _, err := store.Save(hosts.Host{Name: "web", Hostname: "10.0.0.5", User: "root"}); err != nil {
		t.Fatal(err)
	}
	csv := "\xEF\xBB\xBFServer Name;IP Address;User Name;Password;Environment;Notes\n" +
		"db;10.0.0.7;admin;hunter2-very-secret;prod;primary\n" +
		"web;10.0.0.5;root;op://Ops/web/password;prod;\n" +
		";deploy@10.0.0.9:2222;;;;\n" +
		"bad;has space;;;;\n" +
		"db again;10.0.0.7;admin;;;\n"
	p, err := s.load(SourceCSV, writeFile(t, "hosts.csv", csv))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Header {
		t.Fatal("header not detected")
	}
	wantFields := []string{FieldName, FieldHostname, FieldUser, FieldPassword, FieldGroup, FieldIgnore}
	for i, c := range p.Columns {
		if c.Field != wantFields[i] {
			t.Errorf("column %d (%s) = %q, want %q", i, c.Header, c.Field, wantFields[i])
		}
	}
	for _, v := range p.Columns[3].Sample {
		if v != "••••" {
			t.Errorf("password sample leaked: %q", v)
		}
	}
	if p.Passwords != 1 {
		t.Errorf("Passwords = %d, want 1", p.Passwords)
	}
	if len(p.Rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(p.Rows))
	}
	db, web, addr, bad, again := p.Rows[0], p.Rows[1], p.Rows[2], p.Rows[3], p.Rows[4]
	if db.Error != "" || db.Host.Group != "prod" || db.Host.PasswordRef != "" {
		t.Errorf("db row = %+v", db)
	}
	if web.ExistingID == 0 || web.Existing != "web" {
		t.Errorf("web not matched to the saved host: %+v", web)
	}
	if h := addr.Host; h.User != "deploy" || h.Hostname != "10.0.0.9" || h.Port != 2222 || h.Name != "10.0.0.9" {
		t.Errorf("address not split: %+v", h)
	}
	if bad.Error == "" || bad.Field != "hostname" {
		t.Errorf("bad row = %+v", bad)
	}
	if again.SameAs != db.Line {
		t.Errorf("SameAs = %d, want %d", again.SameAs, db.Line)
	}

	res, err := s.Import(p.ID, lines(p), DuplicateUpdate, "Imported")
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 2 || res.Updated != 1 || res.Skipped != 2 || len(res.Errors) != 1 {
		t.Errorf("result = %+v", res)
	}
	list, _ := store.List()
	got := map[string]hosts.Host{}
	for _, h := range list {
		got[h.Name] = h
	}
	if got["web"].PasswordRef != "op://Ops/web/password" || got["web"].Group != "prod" {
		t.Errorf("web not updated: %+v", got["web"])
	}
	if got["10.0.0.9"].Group != "Imported" {
		t.Errorf("default group not applied: %+v", got["10.0.0.9"])
	}
	for _, h := range list {
		if strings.Contains(h.PasswordRef, "hunter2") {
			t.Fatal("a plain password was stored")
		}
	}
	if _, err := s.Import(p.ID, lines(p), DuplicateSkip, ""); err != ErrStale {
		t.Errorf("second import err = %v, want ErrStale", err)
	}
}

func TestCopyDuplicatesGetFreeNames(t *testing.T) {
	s, store := newService(t)
	if _, err := store.Save(hosts.Host{Name: "web", Hostname: "web.example"}); err != nil {
		t.Fatal(err)
	}
	p, err := s.load(SourceCSV, writeFile(t, "h.tsv", "name\thost\nweb\tweb.example\nweb\tother.example\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Rows[1].Host.Name != "web (2)" {
		t.Errorf("new host with a taken name = %q, want web (2)", p.Rows[1].Host.Name)
	}
	res, err := s.Import(p.ID, lines(p), DuplicateCopy, "")
	if err != nil || res.Imported != 2 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	list, _ := store.List()
	if len(list) != 3 {
		t.Errorf("hosts = %d, want 3", len(list))
	}
}

func TestNoHeaderAndRemap(t *testing.T) {
	s, _ := newService(t)
	p, err := s.load(SourceCSV, writeFile(t, "h.csv", "alpha,10.1.1.1,22\nbeta,10.1.1.2,2200\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Header {
		t.Fatal("data row taken for a header")
	}
	p, err = s.Remap(p.ID, []string{FieldName, FieldHostname, FieldPort}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 2 || p.Rows[1].Host.Port != 2200 || p.Rows[0].Host.Name != "alpha" {
		t.Errorf("rows = %+v", p.Rows)
	}
	// A field can only be used once; the second use is ignored.
	p, _ = s.Remap(p.ID, []string{FieldHostname, FieldHostname, FieldPort}, false)
	if p.Columns[1].Field != FieldIgnore {
		t.Errorf("duplicate field kept: %+v", p.Columns)
	}
	if _, err := s.Remap("old", nil, false); err != ErrStale {
		t.Errorf("err = %v, want ErrStale", err)
	}
}

func TestSniffDelimiter(t *testing.T) {
	for line, want := range map[string]rune{"a,b,c": ',', "a;b;c": ';', "a\tb\tc": '\t', "a": ','} {
		if got := sniffDelimiter(line); got != want {
			t.Errorf("%q: got %q, want %q", line, got, want)
		}
	}
}
