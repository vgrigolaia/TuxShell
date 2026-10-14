package hosts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSaveListUpdateDelete(t *testing.T) {
	s := openTemp(t)

	h, err := s.Save(Host{Name: " web-1 ", Group: "prod", Hostname: "10.0.0.5", User: "deploy", Tags: []string{"Web", "web", " "}})
	if err != nil {
		t.Fatal(err)
	}
	if h.ID == 0 || h.Name != "web-1" || h.Port != 22 {
		t.Fatalf("saved host = %+v", h)
	}
	if len(h.Tags) != 1 || h.Tags[0] != "web" {
		t.Fatalf("tags = %v, want [web]", h.Tags)
	}

	if _, err := s.Save(Host{Name: "db-1", Hostname: "db.internal", Port: 2222}); err != nil {
		t.Fatal(err)
	}

	h.Port = 2200
	h.ProxyJump = "jump@bastion:22"
	if _, err := s.Save(h); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != 2200 || got.ProxyJump != "jump@bastion:22" {
		t.Fatalf("updated host = %+v", got)
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	// Grouped hosts sort before ungrouped ones.
	if len(list) != 2 || list[0].Name != "web-1" || list[1].Name != "db-1" {
		t.Fatalf("list = %+v", list)
	}

	if err := s.Delete(h.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
	if err := s.Delete(h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestDuplicateNameIsValidationError(t *testing.T) {
	s := openTemp(t)
	if _, err := s.Save(Host{Name: "web", Hostname: "a"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Save(Host{Name: "WEB", Hostname: "b"})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "name" {
		t.Fatalf("err = %v, want name validation error", err)
	}
}

func TestReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Host{Name: "keep", Hostname: "k"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("after reopen: %v %v", list, err)
	}
}

func TestValidateRejectsOptionInjection(t *testing.T) {
	cases := map[string]Host{
		"hostname":     {Name: "x", Hostname: "-oProxyCommand=evil"},
		"user":         {Name: "x", Hostname: "h", User: "-v"},
		"proxyJump":    {Name: "x", Hostname: "h", ProxyJump: "-oProxyCommand=evil"},
		"identityFile": {Name: "x", Hostname: "h", IdentityFile: "-F/tmp/cfg"},
		"alias":        {Name: "x", Alias: "-G"},
	}
	for field, h := range cases {
		h.Normalize()
		var ve *ValidationError
		if err := h.Validate(); !errors.As(err, &ve) || ve.Field != field {
			t.Errorf("%s: err = %v, want %s validation error", field, err, field)
		}
	}
}

func TestValidateFields(t *testing.T) {
	bad := map[string]Host{
		"name":        {Hostname: "h"},
		"hostname":    {Name: "x", Hostname: "has space"},
		"port":        {Name: "x", Hostname: "h", Port: 70000},
		"passwordRef": {Name: "x", Hostname: "h", PasswordRef: "hunter2"},
		"tags":        {Name: "x", Hostname: "h", Tags: []string{"a b"}},
	}
	for field, h := range bad {
		h.Normalize()
		var ve *ValidationError
		if err := h.Validate(); !errors.As(err, &ve) || ve.Field != field {
			t.Errorf("%s: err = %v", field, err)
		}
	}

	good := Host{Name: "v6", Hostname: "[fe80::1%eth0]", User: "me@corp", ProxyJump: "a@b:22,c", PasswordRef: "op://Infra/web/password"}
	good.Normalize()
	if err := good.Validate(); err != nil {
		t.Fatalf("valid host rejected: %v", err)
	}
}

func TestOpenRestrictsPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tuxshell")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hosts.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Save(Host{Name: "x", Hostname: "x"}); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{dir: 0o700, path: 0o600, path + "-wal": 0o600} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", filepath.Base(p), got, want)
		}
	}
}

func TestDeleteMany(t *testing.T) {
	s := openTemp(t)
	var ids []int64
	for _, name := range []string{"a", "b", "c"} {
		h, err := s.Save(Host{Name: name, Hostname: name})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, h.ID)
	}
	n, err := s.DeleteMany([]int64{ids[0], ids[2], 999, ids[0]})
	if err != nil || n != 2 {
		t.Fatalf("DeleteMany = %d, %v; want 2", n, err)
	}
	list, _ := s.List()
	if len(list) != 1 || list[0].Name != "b" {
		t.Fatalf("remaining = %+v", list)
	}
	if n, err := s.DeleteMany(nil); err != nil || n != 0 {
		t.Fatalf("empty DeleteMany = %d, %v", n, err)
	}
}

func TestSnippetsAndTunnels(t *testing.T) {
	s := openTemp(t)
	sn, err := s.SaveSnippet(Snippet{Name: " lag ", Command: "select * from pg_stat_replication;\n"})
	if err != nil || sn.Name != "lag" || sn.Command != "select * from pg_stat_replication;" {
		t.Fatalf("SaveSnippet = %+v, %v", sn, err)
	}
	if _, err := s.SaveSnippet(Snippet{Name: "LAG", Command: "x"}); err == nil {
		t.Fatal("duplicate snippet name accepted")
	}
	h, _ := s.Save(Host{Name: "pg", Hostname: "10.0.0.11"})
	for _, bad := range []Tunnel{
		{HostID: h.ID, Kind: "local", BindPort: 5432, TargetHost: "-oProxyCommand=x", TargetPort: 5432},
		{HostID: h.ID, Kind: "local", BindPort: 70000, TargetHost: "db", TargetPort: 5432},
		{HostID: h.ID, Kind: "remote", BindPort: 5432},
		{HostID: 999, Kind: "dynamic", BindPort: 1080},
	} {
		if _, err := s.SaveTunnel(bad); err == nil {
			t.Errorf("SaveTunnel(%+v) accepted", bad)
		}
	}
	tn, err := s.SaveTunnel(Tunnel{HostID: h.ID, Kind: "dynamic", BindPort: 1080, TargetHost: "ignored", TargetPort: 9})
	if err != nil || tn.TargetHost != "" || tn.TargetPort != 0 {
		t.Fatalf("SaveTunnel dynamic = %+v, %v", tn, err)
	}
	// Deleting the host removes its tunnels.
	if err := s.Delete(h.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListTunnels(); len(list) != 0 {
		t.Fatalf("tunnels after host delete = %+v", list)
	}
}

func TestOS(t *testing.T) {
	for in, want := range map[string]string{
		"Ubuntu 24.04": "ubuntu", "RHEL 9": "rhel", "Red Hat Enterprise Linux": "rhel", "AlmaLinux 10": "almalinux",
		"Rocky Linux": "rocky", "CentOS Stream 9": "centos", "SLES 15": "opensuse", "Windows Server 2022": "windows",
		"Linux": "linux", "debian": "debian", "": "", "Solaris": "",
	} {
		if got := ParseOS(in); got != want {
			t.Errorf("ParseOS(%q) = %q, want %q", in, got, want)
		}
	}
	s := openTemp(t)
	h, err := s.Save(Host{Name: "u", Hostname: "u.example", OS: "Ubuntu", Tags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(h.ID)
	if got.OS != "ubuntu" {
		t.Errorf("OS = %q", got.OS)
	}
	if _, err := s.Save(Host{Name: "x", Hostname: "x.example", OS: "solaris"}); err == nil {
		t.Error("unknown OS accepted")
	}
}

func TestPasswordSaved(t *testing.T) {
	s := openTemp(t)
	var deleted []int64
	s.OnDelete = func(ids []int64) { deleted = append(deleted, ids...) }
	h, err := s.Save(Host{Name: "p", Hostname: "p.example", Tags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPasswordSaved(h.ID, true); err != nil {
		t.Fatal(err)
	}
	// A save from the form can't clear (or set) the flag.
	h.PasswordSaved = false
	h.User = "root"
	if _, err := s.Save(h); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(h.ID)
	if !got.PasswordSaved {
		t.Error("Save cleared PasswordSaved")
	}
	n, _ := s.Save(Host{Name: "q", Hostname: "q.example", PasswordSaved: true, Tags: []string{}})
	if got, _ := s.Get(n.ID); got.PasswordSaved {
		t.Error("Save set PasswordSaved")
	}
	if err := s.Delete(h.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteMany([]int64{n.ID}); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 2 || deleted[0] != h.ID || deleted[1] != n.ID {
		t.Errorf("OnDelete got %v", deleted)
	}
}
