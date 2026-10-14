package hostio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

var sampleHosts = []hosts.Host{
	{ID: 1, Name: "db primary", Group: "Prod", Hostname: "10.0.0.7", User: "admin", Port: 2222, Tags: []string{"pg", "leader"}, PasswordRef: "op://Ops/db/password"},
	{ID: 2, Name: "work", Alias: "work", Port: 22, Tags: []string{}},
	{ID: 3, Name: "inner", Hostname: "10.0.1.5", ProxyJump: "bastion", IdentityFile: "~/.ssh/my key", Port: 22, Tags: []string{}},
}

// Exports in JSON and CSV import back to the same hosts.
func TestExportRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for _, format := range []string{FormatJSON, FormatCSV} {
		data, err := encode(format, sampleHosts, now)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := newService(t)
		src := map[string]string{FormatJSON: SourceJSON, FormatCSV: SourceCSV}[format]
		p, err := s.load(src, writeFile(t, "hosts."+format, string(data)))
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if len(p.Rows) != len(sampleHosts) {
			t.Fatalf("%s: rows = %d", format, len(p.Rows))
		}
		for i, r := range p.Rows {
			want := sampleHosts[i]
			want.ID = 0
			got := r.Host
			if r.Error != "" || got.Name != want.Name || got.Group != want.Group || got.Alias != want.Alias ||
				got.Hostname != want.Hostname || got.User != want.User || got.Port != want.Port ||
				got.ProxyJump != want.ProxyJump || got.IdentityFile != want.IdentityFile ||
				got.PasswordRef != want.PasswordRef || strings.Join(got.Tags, ",") != strings.Join(want.Tags, ",") {
				t.Errorf("%s row %d:\n got %+v\nwant %+v (%s)", format, i, got, want, r.Error)
			}
		}
		if p.Passwords != 0 {
			t.Errorf("%s: an op:// reference was counted as a password", format)
		}
	}
}

func TestSSHConfigExport(t *testing.T) {
	out := string(sshConfig(sampleHosts, time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)))
	for _, want := range []string{
		"\nHost db-primary\n    # Group: Prod\n    HostName 10.0.0.7\n    User admin\n    Port 2222\n",
		"\nHost work\n",
		"\nHost inner\n    HostName 10.0.1.5\n    ProxyJump bastion\n    IdentityFile \"~/.ssh/my key\"\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "op://") {
		t.Error("ssh config export contains a 1Password reference")
	}
}

func TestExportWritesPrivateFile(t *testing.T) {
	s, store := newService(t)
	h := sampleHosts[0]
	h.ID = 0
	h, err := store.Save(h)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out.json")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.save = func(string, string, []Filter) (string, error) { return dest, nil }
	if _, err := s.Export(FormatJSON, nil); err != ErrNothingToExport {
		t.Errorf("empty export err = %v", err)
	}
	path, err := s.Export(FormatJSON, []int64{h.ID})
	if err != nil || path != dest {
		t.Fatalf("path = %q, err = %v", path, err)
	}
	st, _ := os.Stat(dest)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", st.Mode().Perm())
	}
	data, _ := os.ReadFile(dest)
	if !strings.Contains(string(data), `"db primary"`) {
		t.Errorf("unexpected export: %s", data)
	}
}

func TestJSONRejectsOtherFiles(t *testing.T) {
	for _, body := range []string{`{"hosts":[]}`, `[1,2]`, `{"tuxshell":2,"hosts":[]}`} {
		if _, err := readTuxShellJSON([]byte(body)); err == nil {
			t.Errorf("%s: accepted", body)
		}
	}
}
