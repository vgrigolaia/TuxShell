package hosts

import (
	"errors"
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
