package hosts

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestProfileField(t *testing.T) {
	s := openTemp(t)
	h, err := s.Save(Host{Name: "p", Hostname: "p.example", Profile: "prod", Tags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(h.ID); got.Profile != "prod" {
		t.Errorf("profile = %q", got.Profile)
	}
	h.Profile = "default"
	if h, _ = s.Save(h); h.Profile != "" {
		t.Errorf("default profile stored as %q", h.Profile)
	}
	h.Profile = "-oEvil"
	var ve *ValidationError
	if _, err := s.Save(h); !errors.As(err, &ve) || ve.Field != "profile" {
		t.Errorf("bad profile: %v", err)
	}
}

func TestMoveHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	add := func(name, group string) int64 {
		h, err := s.Save(Host{Name: name, Group: group, Hostname: "10.0.0.1"})
		if err != nil {
			t.Fatal(err)
		}
		return h.ID
	}
	a, b, c := add("a", "g1"), add("b", "g1"), add("c", "g1")
	x := add("x", "g2")
	order := func(group string) []string {
		list, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, h := range list {
			if h.Group == group {
				names = append(names, h.Name)
			}
		}
		return names
	}
	same := func(got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	}
	same(order("g1"), "a", "b", "c") // never moved: by name

	// Within the group: c before a (the top), then a to the end.
	if err := s.MoveHost(c, "g1", a); err != nil {
		t.Fatal(err)
	}
	same(order("g1"), "c", "a", "b")
	if err := s.MoveHost(a, "g1", 0); err != nil { // 0 means the end
		t.Fatal(err)
	}
	same(order("g1"), "c", "b", "a")

	// To another group, before x; the old group keeps its order.
	if err := s.MoveHost(b, "g2", x); err != nil {
		t.Fatal(err)
	}
	same(order("g1"), "c", "a")
	same(order("g2"), "b", "x")
	got, _ := s.Get(b)
	if got.Group != "g2" {
		t.Fatalf("group = %q", got.Group)
	}

	// "Before" a host from another group counts as the end.
	if err := s.MoveHost(a, "g2", c); err != nil {
		t.Fatal(err)
	}
	same(order("g2"), "b", "x", "a")

	// To no group, and into a brand new group.
	if err := s.MoveHost(x, "", 0); err != nil {
		t.Fatal(err)
	}
	same(order(""), "x")
	if err := s.MoveHost(c, "  new  ", 0); err != nil {
		t.Fatal(err)
	}
	same(order("new"), "c")

	// Bad input changes nothing.
	var ve *ValidationError
	if err := s.MoveHost(a, strings.Repeat("g", 101), 0); !errors.As(err, &ve) {
		t.Fatalf("long group: %v", err)
	}
	if err := s.MoveHost(a, "x\ny", 0); !errors.As(err, &ve) {
		t.Fatalf("line break: %v", err)
	}
	if err := s.MoveHost(9999, "g1", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing host: %v", err)
	}
	same(order("g2"), "b", "a")

	// The moves are stored, not just in memory.
	_ = s.Close()
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	list, _ := again.List()
	if len(list) != 4 {
		t.Fatalf("hosts after reopen = %d", len(list))
	}
	for _, h := range list {
		if h.Name == "c" && h.Group != "new" {
			t.Fatalf("c is in %q after reopen", h.Group)
		}
	}
}

func TestRenameGroup(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, h := range []Host{{Name: "b", Group: "old"}, {Name: "a", Group: "old"}, {Name: "z", Group: "web"}, {Name: "y", Group: "web"}, {Name: "solo"}} {
		h.Hostname = "10.0.0.1"
		if _, err := s.Save(h); err != nil {
			t.Fatal(err)
		}
	}
	order := func() string {
		list, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, h := range list {
			out = append(out, h.Group+":"+h.Name)
		}
		return strings.Join(out, " ")
	}

	// A new name keeps the hosts' order.
	if n, err := s.RenameGroup("old", " lab "); err != nil || n != 2 {
		t.Fatalf("rename: %d, %v", n, err)
	}
	if got, want := order(), "lab:a lab:b web:y web:z :solo"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Into an existing group: after its hosts.
	if n, err := s.RenameGroup("lab", "web"); err != nil || n != 2 {
		t.Fatalf("merge: %d, %v", n, err)
	}
	if got, want := order(), "web:y web:z web:a web:b :solo"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// "" deletes the group; its hosts stay, ungrouped.
	if n, err := s.RenameGroup("web", ""); err != nil || n != 4 {
		t.Fatalf("delete: %d, %v", n, err)
	}
	if got, want := order(), ":solo :y :z :a :b"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Nothing to do, and bad names.
	if n, err := s.RenameGroup("", "x"); err != nil || n != 0 {
		t.Fatalf("empty from: %d, %v", n, err)
	}
	var ve *ValidationError
	if _, err := s.RenameGroup("x", "a\nb"); !errors.As(err, &ve) {
		t.Fatalf("line break: %v", err)
	}
}
