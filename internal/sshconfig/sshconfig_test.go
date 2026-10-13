package sshconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func aliasNames(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Alias)
	}
	return out
}

func TestAliases(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	write(t, cfg, `
# comment
Include conf.d/*.conf
Host web-1 web-2  # trailing comment
  HostName 10.0.0.5
Host *.internal !bastion
Host=db
Host "quoted name" web-1
Match host foo
  User bar
Include missing.conf
`)
	write(t, filepath.Join(dir, "conf.d", "a.conf"), "Host pg-primary pg-?\n")
	// Relative includes resolve against ~/.ssh, not the including file.
	write(t, filepath.Join(dir, "conf.d", "b.conf"), "Include loop.conf\nHost nested\n")
	write(t, filepath.Join(dir, "loop.conf"), "Host from-loop\n")

	got, err := Aliases(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pg-primary", "from-loop", "nested", "web-1", "web-2", "db", "quoted name"}
	if names := aliasNames(got); !slices.Equal(names, want) {
		t.Fatalf("aliases = %q, want %q", names, want)
	}
}

func TestAliasesMissingConfig(t *testing.T) {
	got, err := Aliases(filepath.Join(t.TempDir(), "nope"))
	if err != nil || got != nil {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestIncludeDepthLimit(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "config"), "Include config\n")
	if _, err := Aliases(filepath.Join(dir, "config")); err == nil {
		t.Fatal("expected error for self-include")
	}
}

func TestParseG(t *testing.T) {
	s := parseG("user deploy\nhostname 10.0.0.5\nport 2222\nproxyjump none\nidentityfile ~/.ssh/id_rsa\n")
	if s != (Settings{Hostname: "10.0.0.5", User: "deploy", Port: 2222}) {
		t.Fatalf("settings = %+v", s)
	}
	if s := parseG("proxyjump ops@bastion\n"); s.ProxyJump != "ops@bastion" {
		t.Fatalf("proxyjump = %q", s.ProxyJump)
	}
}

func TestImport(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	write(t, cfg, "Host web db broken -bad\n")

	store, err := hosts.Open(filepath.Join(dir, "hosts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := NewService(store, cfg)
	svc.resolve = func(_ context.Context, alias string) (Settings, error) {
		if alias == "broken" {
			return Settings{}, errors.New("boom")
		}
		return Settings{Hostname: alias + ".example", User: "me", Port: 22}, nil
	}
	ctx := context.Background()

	res, err := svc.Import(ctx, []string{"web", "broken", "-bad", "not-there"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 || len(res.Skipped) != 3 {
		t.Fatalf("result = %+v", res)
	}

	list, _ := store.List()
	if len(list) != 1 || list[0].Alias != "web" || list[0].Hostname != "web.example" || list[0].Group != ImportGroup {
		t.Fatalf("hosts = %+v", list)
	}

	// Re-importing is a no-op; the scan marks it imported.
	res, err = svc.Import(ctx, []string{"web"})
	if err != nil || res.Imported != 0 {
		t.Fatalf("reimport = %+v, %v", res, err)
	}
	cands, err := svc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if c.Imported != (c.Alias == "web") {
			t.Errorf("%s imported = %v", c.Alias, c.Imported)
		}
	}
}
