package updates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serve(t *testing.T, status int, body string) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Service{url: srv.URL, client: srv.Client()}
}

func TestCheck(t *testing.T) {
	page := ReleasesURL + "/tag/v1.10.0"
	s := serve(t, http.StatusOK, `{"tag_name":"v1.10.0","html_url":"`+page+`"}`)
	for _, tc := range []struct {
		current string
		newer   bool
	}{
		{"1.1.1", true},
		{"1.9.9", true},
		{"1.10.0", false},
		{"2.0.0", false},
	} {
		got, err := s.Check(context.Background(), tc.current)
		if err != nil {
			t.Fatalf("%s: %v", tc.current, err)
		}
		if got.Newer != tc.newer || got.Latest != "1.10.0" || got.URL != page {
			t.Errorf("%s: got %+v, want newer=%v", tc.current, got, tc.newer)
		}
	}
}

func TestCheckForeignURL(t *testing.T) {
	s := serve(t, http.StatusOK, `{"tag_name":"v9.0.0","html_url":"https://evil.example/x"}`)
	got, err := s.Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != ReleasesURL {
		t.Errorf("URL = %q, want %q", got.URL, ReleasesURL)
	}
}

func TestCheckUnavailable(t *testing.T) {
	for name, s := range map[string]*Service{
		"not found": serve(t, http.StatusNotFound, `{}`),
		"bad tag":   serve(t, http.StatusOK, `{"tag_name":"nightly"}`),
		"bad json":  serve(t, http.StatusOK, `<html>`),
	} {
		if _, err := s.Check(context.Background(), "1.0.0"); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s: err = %v, want ErrUnavailable", name, err)
		}
	}
}

func TestParse(t *testing.T) {
	for v, ok := range map[string]bool{"1.2.3": true, "v1.2.3": true, "1.2": false, "1.2.x": false, "-1.0.0": false, "": false} {
		if _, got := parse(v); got != ok {
			t.Errorf("parse(%q) ok = %v, want %v", v, got, ok)
		}
	}
}
