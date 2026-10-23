package hostio

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// utf16File writes text the way Windows saves .rdp files: UTF-16LE with a BOM.
func utf16File(t *testing.T, dir, name, text string) string {
	t.Helper()
	b := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(text)) {
		b = append(b, byte(u), byte(u>>8))
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRDPFiles(t *testing.T) {
	s, store := newService(t)
	dir := t.TempDir()
	app := utf16File(t, dir, "Accounting app.rdp", "screen mode id:i:2\r\nuse multimon:i:1\r\ndesktopwidth:i:1920\r\ndesktopheight:i:1080\r\n"+
		"full address:s:ts.corp.example:3390\r\nusername:s:CORP\\alice\r\nredirectprinters:i:1\r\nredirectsmartcards:i:1\r\n"+
		"redirectclipboard:i:0\r\ndrivestoredirect:s:*\r\naudiomode:i:2\r\naudiocapturemode:i:1\r\nconnection type:i:6\r\n"+
		"gatewayhostname:s:gw.corp.example\r\ngatewayusagemethod:i:1\r\nremoteapplicationmode:i:1\r\n"+
		"remoteapplicationprogram:s:||accounting\r\nremoteapplicationname:s:Accounting\r\npassword 51:b:01000000D08C9DDF\r\n")
	desk := filepath.Join(dir, "build box.RDP")
	os.WriteFile(desk, []byte("full address:s:10.0.5.4\nusername:s:builder\ndynamic resolution:i:0\ndesktopwidth:i:1600\ndesktopheight:i:900\ngatewayhostname:s:unused\ngatewayusagemethod:i:0\n"), 0o600)
	empty := filepath.Join(dir, "empty.rdp")
	os.WriteFile(empty, []byte("screen mode id:i:1\n"), 0o600)

	s.pick = func(_ string, _ []Filter, many bool) ([]string, error) {
		if !many {
			t.Error("expected a multi-file picker")
		}
		return []string{app, desk, empty}, nil
	}
	p, err := s.Open(SourceRDPFiles)
	if err != nil {
		t.Fatal(err)
	}
	if p.File != "3 files" || len(p.Rows) != 2 {
		t.Fatalf("preview: file %q, %d rows", p.File, len(p.Rows))
	}
	if _, err := s.Import(p.ID, lines(p), DuplicateSkip, ""); err != nil {
		t.Fatal(err)
	}
	list, _ := store.List()
	byName := map[string]hosts.Host{}
	for _, h := range list {
		byName[h.Name] = h
	}

	a := byName["Accounting app"]
	r := a.RDP
	if a.Protocol != hosts.ProtocolRDP || a.Hostname != "ts.corp.example" || a.Port != 3390 || a.User != "alice" || r.Domain != "CORP" || a.OS != "windows" ||
		a.PasswordSaved || a.PasswordRef != "" {
		t.Errorf("app host = %+v", a)
	}
	if r.Window != "fullscreen" || !r.Multimon || r.Size != "fit" || r.Clipboard || !r.Printers || !r.SmartCards || !r.HomeFolder ||
		r.Sound != "off" || !r.Microphone || r.Quality != "best" || r.Gateway.Host != "gw.corp.example" || r.Gateway.Port != 443 ||
		r.RemoteApp != (hosts.RemoteApp{Program: "||accounting", Name: "Accounting"}) {
		t.Errorf("app options = %+v", r)
	}

	b := byName["build box"]
	if b.Port != 3389 || b.User != "builder" || b.RDP.Size != "fixed" || b.RDP.Width != 1600 || b.RDP.Gateway.Host != "" || !b.RDP.Clipboard {
		t.Errorf("desktop host = %+v", b)
	}
}
