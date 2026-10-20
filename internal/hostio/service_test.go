package hostio

import (
	"encoding/json"
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
	return NewService(store, nil, nil), store
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

// The template imports cleanly: notes are skipped, every column is matched,
// the OS is read and bastion names become addresses.
func TestTemplate(t *testing.T) {
	s, store := newService(t)
	if _, err := store.Save(hosts.Host{Name: "old-gw", Hostname: "198.51.100.1", User: "ops", Port: 2200}); err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(templateCSV, "web-01,10.0.0.5,deploy,,,Production,Debian,web,bastion-01,", "web-01,10.0.0.5,deploy,,,Production,Debian,web,\"bastion-01,old-gw\",", 1) +
		"inner,10.9.9.9,,,,,Solaris 11,,db-01,\n"
	p, err := s.load(SourceCSV, writeFile(t, "t.csv", body))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Columns {
		if c.Field == FieldIgnore {
			t.Errorf("column %q not matched", c.Header)
		}
	}
	if len(p.Rows) != 5 {
		t.Fatalf("rows = %d, want 5 (notes skipped)", len(p.Rows))
	}
	got := map[string]hosts.Host{}
	for _, r := range p.Rows {
		if r.Error != "" {
			t.Errorf("line %d: %s", r.Line, r.Error)
		}
		got[r.Host.Name] = r.Host
	}
	if w := got["win-01"]; w.Protocol != hosts.ProtocolRDP || w.RDP.Domain != "CORP" || w.OS != "windows" {
		t.Errorf("win-01 = %+v", w)
	}
	if got["bastion-01"].OS != "ubuntu" || got["db-01"].OS != "rocky" || got["web-01"].OS != "debian" {
		t.Errorf("OS not read: %+v", got)
	}
	if j := got["db-01"].ProxyJump; j != "admin@203.0.113.10" {
		t.Errorf("db-01 jump = %q", j)
	}
	if j := got["web-01"].ProxyJump; j != "admin@203.0.113.10,ops@198.51.100.1:2200" {
		t.Errorf("web-01 jump = %q", j)
	}
	// A bastion that has its own bastion keeps both hops, outermost first.
	if j := got["inner"].ProxyJump; j != "admin@203.0.113.10,postgres@10.0.0.7:2222" {
		t.Errorf("inner jump = %q", j)
	}
	if h := got["inner"]; h.OS != "" || strings.Join(h.Tags, ",") != "solaris" {
		t.Errorf("unknown OS should become a tag: %+v", h)
	}
	if got["db-01"].PasswordRef != "op://Servers/db-01/password" || p.Passwords != 0 {
		t.Errorf("op:// reference lost: %+v (passwords %d)", got["db-01"], p.Passwords)
	}
}

// With saving passwords on, plain passwords from the file go to the keyring
// (through UsePasswords) and are never part of the preview.
func TestImportSavesPasswords(t *testing.T) {
	s, store := newService(t)
	saved := map[int64]string{}
	on := false
	UsePasswords(s, func() bool { return on }, func(id int64, pw string) error { saved[id] = pw; return nil })
	csv := "Name,Host,User,Password\nweb,10.0.0.5,root,Sup3r-secret!\nref,10.0.0.6,root,op://Ops/ref/password\n"

	p, err := s.load(SourceCSV, writeFile(t, "pw.csv", csv))
	if err != nil {
		t.Fatal(err)
	}
	if p.Passwords != 1 || p.SavePasswords {
		t.Errorf("off: passwords %d, save %v", p.Passwords, p.SavePasswords)
	}
	if b, _ := json.Marshal(p); strings.Contains(string(b), "Sup3r") {
		t.Fatal("the preview contains a password")
	}
	res, _ := s.Import(p.ID, lines(p), DuplicateSkip, "")
	if res.Passwords != 0 || len(saved) != 0 {
		t.Errorf("saved while off: %+v %v", res, saved)
	}

	on = true
	for _, h := range mustList(t, store) {
		_ = store.Delete(h.ID)
	}
	p, _ = s.load(SourceCSV, writeFile(t, "pw.csv", csv))
	if !p.SavePasswords {
		t.Error("preview doesn't say passwords will be saved")
	}
	res, err = s.Import(p.ID, lines(p), DuplicateSkip, "")
	if err != nil || res.Imported != 2 || res.Passwords != 1 {
		t.Fatalf("res = %+v, %v", res, err)
	}
	for id, pw := range saved {
		h, _ := store.Get(id)
		if h.Name != "web" || pw != "Sup3r-secret!" {
			t.Errorf("saved %q for %+v", pw, h)
		}
	}
	for _, h := range mustList(t, store) {
		if h.Name == "ref" && h.PasswordRef != "op://Ops/ref/password" {
			t.Errorf("op:// reference lost: %+v", h)
		}
	}
}

func mustList(t *testing.T, s *hosts.Store) []hosts.Host {
	t.Helper()
	l, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// "Only add their passwords" saves passwords for hosts that are already saved
// and leaves the hosts themselves alone.
func TestImportOnlyPasswordsForExisting(t *testing.T) {
	s, store := newService(t)
	saved := map[int64]string{}
	UsePasswords(s, func() bool { return true }, func(id int64, pw string) error { saved[id] = pw; return nil })
	p, _ := s.load(SourceCSV, writeFile(t, "a.csv", "Name,Host,User\nweb,10.0.0.5,root\nnopw,10.0.0.7,root\n"))
	if _, err := s.Import(p.ID, lines(p), DuplicateSkip, ""); err != nil {
		t.Fatal(err)
	}
	var web hosts.Host
	for _, h := range mustList(t, store) {
		if h.Name == "web" {
			web = h
			web.Name = "renamed in TuxShell"
			if _, err := store.Save(web); err != nil {
				t.Fatal(err)
			}
		}
	}

	csv := "Name,Host,User,Password\nweb,10.0.0.5,root,Sup3r-secret!\nnopw,10.0.0.7,root,\n"
	p, _ = s.load(SourceCSV, writeFile(t, "b.csv", csv))
	res, err := s.Import(p.ID, lines(p), DuplicatePasswords, "")
	if err != nil || res.Imported != 0 || res.Updated != 0 || res.Skipped != 1 || res.Passwords != 1 {
		t.Fatalf("res = %+v, %v", res, err)
	}
	if saved[web.ID] != "Sup3r-secret!" || len(saved) != 1 { // gitleaks:allow
		t.Errorf("saved = %d passwords", len(saved))
	}
	if h, _ := store.Get(web.ID); h.Name != "renamed in TuxShell" || len(mustList(t, store)) != 2 {
		t.Errorf("host changed: %+v", h)
	}
}
