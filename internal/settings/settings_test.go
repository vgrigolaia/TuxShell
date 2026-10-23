package settings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	etc := filepath.Join(dir, "shells")
	os.WriteFile(etc, []byte("# comment\n/bin/sh\n/bin/bash\n\nrelative/zsh\n/bin/bash\n"), 0o644)
	s := NewService(filepath.Join(dir, "cfg", "tuxshell", "settings.json"))
	s.etcShells = etc
	return s
}

func TestDefaultsWhenMissing(t *testing.T) {
	s := newTestService(t)
	got := s.Get()
	if got.Saved || !reflect.DeepEqual(got.Settings, Defaults()) {
		t.Fatalf("Get() = %+v, want defaults and Saved=false", got)
	}
}

func TestSaveRoundTripAndClamp(t *testing.T) {
	s := newTestService(t)
	in := Defaults()
	in.Theme = "light"
	in.Terminal.FontFamily = "Source Code Pro"
	in.Terminal.FontSize = 200
	in.Terminal.LineHeight = 0.5
	in.Terminal.FontWeight = 449
	in.Terminal.CursorStyle = "beam"
	in.Terminal.Scrollback = -5
	in.Terminal.Shell = "/bin/bash"
	out, err := s.Save(in)
	if err != nil {
		t.Fatal(err)
	}
	tt := out.Terminal
	if tt.FontSize != MaxFontSize || tt.LineHeight != MinLineHeight || tt.FontWeight != 400 || tt.CursorStyle != "block" || tt.Scrollback != 0 {
		t.Fatalf("not normalized: %+v", tt)
	}

	// A fresh service reads back what was stored.
	again := NewService(s.path)
	again.etcShells = s.etcShells
	got := again.Get()
	if !got.Saved || !reflect.DeepEqual(got.Settings, out) {
		t.Fatalf("reloaded %+v, want %+v", got.Settings, out)
	}
	if info, err := os.Stat(filepath.Dir(s.path)); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("config dir mode = %v, %v", info.Mode(), err)
	}
}

func TestShellMustBeListed(t *testing.T) {
	s := newTestService(t)
	in := Defaults()
	for _, sh := range []string{"/usr/bin/python3", "relative/zsh", "/bin/sh -c 'x'"} {
		in.Terminal.Shell = sh
		var ve *ValidationError
		if _, err := s.Save(in); !errors.As(err, &ve) || ve.Field != "shell" {
			t.Errorf("Save(shell=%q) err = %v, want shell ValidationError", sh, err)
		}
	}
	if got := s.Shells(); !reflect.DeepEqual(got, []string{"/bin/sh", "/bin/bash"}) {
		t.Fatalf("Shells() = %q", got)
	}
}

func TestHandEditedFile(t *testing.T) {
	s := newTestService(t)
	os.MkdirAll(filepath.Dir(s.path), 0o700)
	// Unknown keys are ignored, missing keys keep defaults, bad values are replaced.
	os.WriteFile(s.path, []byte(`{"theme":"neon","tabsPosition":"bottom","groupOrder":["Lab","","Lab","Prod"],"emptyGroups":["New","New",""],"sidebarWidth":9000,"terminal":{"fontSize":16,"fontFamily":"Evil\"; }","bell":"visual","extra":1}}`), 0o600)
	got, exists, err := load(s.path)
	if err != nil || !exists {
		t.Fatal(err)
	}
	want := Defaults()
	want.Terminal.FontSize = 16
	want.Terminal.Bell = "visual"
	want.GroupOrder = []string{"Lab", "Prod"}
	want.EmptyGroups = []string{"New"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("load = %+v, want %+v", got, want)
	}

	os.WriteFile(s.path, []byte(`{not json`), 0o600)
	if got, _, err := load(s.path); err == nil || !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("broken file: %+v, %v", got, err)
	}
}

func TestParseFonts(t *testing.T) {
	out := "Source Code Pro\nAdwaita Mono\nSource Code Pro\nJetBrains Mono\nSimSun\\-ExtB\nbad\"name\n\n"
	want := []string{"Adwaita Mono", "SimSun-ExtB", "Source Code Pro"}
	if got := parseFonts(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFonts = %q, want %q", got, want)
	}
}

func TestProfiles(t *testing.T) {
	s := newTestService(t)
	os.MkdirAll(filepath.Dir(s.path), 0o700)
	// Settings from before profiles: the default profile is read from the flat
	// terminal fields and new options get their defaults.
	os.WriteFile(s.path, []byte(`{"terminal":{"fontSize":16,"bell":"visual"},"profiles":[
		{"id":"prod","name":"  Production  ","profile":{"fontSize":20,"headerColor":"#FF0000","colorScheme":"custom","palette":{"colors":["#000000"]},"command":"tmux attach"}},
		{"id":"prod","name":"duplicate"},
		{"id":"Bad ID","name":"x"},
		{"id":"evil","profile":{"command":"a\nb","background":"../../hosts.db","headerColor":"red"}}
	]}`), 0o600)
	again := NewService(s.path)
	got := again.Get().Settings
	if got.Terminal.FontSize != 16 || got.Terminal.Bell != "visual" || !got.Terminal.Scrollbar || got.Terminal.Backspace != "del" {
		t.Errorf("default profile = %+v", got.Terminal.Profile)
	}
	if len(got.Profiles) != 2 {
		t.Fatalf("profiles = %+v", got.Profiles)
	}
	p := got.Profiles[0]
	if p.Name != "Production" || p.Profile.FontSize != 20 || p.Profile.HeaderColor != "#ff0000" || !p.Profile.KeepShell || !p.Profile.Scrollbar {
		t.Errorf("prod = %+v", p)
	}
	if c := p.Profile.Palette.Colors; len(c) != 16 || c[0] != "#000000" || c[1] != DefaultPalette().Colors[1] {
		t.Errorf("palette = %v", c)
	}
	if p.Profile.RemoteCommand() != "tmux attach; exec $SHELL -l" {
		t.Errorf("remote command = %q", p.Profile.RemoteCommand())
	}
	e := got.Profiles[1].Profile
	if e.Command != "" || e.Background != "" || e.HeaderColor != "" || got.Profiles[1].Name != "Profile" {
		t.Errorf("evil = %+v", got.Profiles[1])
	}
	if got.ProfileFor("prod").FontSize != 20 || got.ProfileFor("").FontSize != 16 || got.ProfileFor("gone").FontSize != 16 {
		t.Error("ProfileFor")
	}
	if (Profile{Command: "htop"}).RemoteCommand() != "htop" || (Profile{KeepShell: true}).RemoteCommand() != "" {
		t.Error("RemoteCommand")
	}
}

func TestHiddenSSHConfigHostsCleaned(t *testing.T) {
	s := Defaults()
	s.HiddenSSHConfigHosts = []string{"web", "", "web", strings.Repeat("x", 201), "db"}
	s.normalize()
	if got := strings.Join(s.HiddenSSHConfigHosts, ","); got != "web,db" {
		t.Fatalf("hidden = %q", got)
	}
}

func TestKeybindingsCleaned(t *testing.T) {
	s := Defaults()
	s.Keybindings = map[string]string{
		"toggleHosts": "Ctrl+Shift+L",
		"zoomIn":      "Ctrl+=",
		"focusLeft":   "Cmd+Shift+Left",
		"rename":      "Alt+F2",
		"backslash":   `Ctrl+Alt+\`,
		"broadcast":   "", // turned off
		"split":       "Shift+Ctrl+A",
		"bad id":      "Ctrl+K",
		"Upper":       "Ctrl+K",
		"twoKeys":     "Ctrl+K+L",
		"lower":       "Ctrl+k",
		"noKey":       "Ctrl+Shift+",
		"control":     "Ctrl+\x00",
	}
	s.normalize()
	want := map[string]string{
		"toggleHosts": "Ctrl+Shift+L",
		"zoomIn":      "Ctrl+=",
		"focusLeft":   "Cmd+Shift+Left",
		"rename":      "Alt+F2",
		"backslash":   `Ctrl+Alt+\`,
		"broadcast":   "",
	}
	if !reflect.DeepEqual(s.Keybindings, want) {
		t.Fatalf("keybindings = %q", s.Keybindings)
	}
	s.Keybindings = nil
	s.normalize()
	if s.Keybindings == nil {
		t.Fatal("nil keybindings should become an empty map")
	}
}
