// Package settings keeps user preferences in a JSON file under
// $XDG_CONFIG_HOME/tuxshell, readable and editable by hand.
package settings

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Settings are the app-wide preferences.
type Settings struct {
	// Theme is "dark", "light" or "system".
	Theme string `json:"theme"`
	// HostsSidebar shows the hosts sidebar.
	HostsSidebar bool `json:"hostsSidebar"`
	// HostStatus pings saved hosts in the background to show which are up.
	HostStatus bool `json:"hostStatus"`
	// UpdateCheck asks GitHub for a newer release at startup. Off by default:
	// the app makes no requests of its own unless the user turns this on.
	UpdateCheck bool     `json:"updateCheck"`
	Terminal    Terminal `json:"terminal"`
}

// Terminal holds the look and behaviour of terminal panes.
type Terminal struct {
	// FontFamily is a font family name; empty means the bundled JetBrains Mono.
	FontFamily     string  `json:"fontFamily"`
	FontSize       int     `json:"fontSize"`
	LineHeight     float64 `json:"lineHeight"`
	LetterSpacing  int     `json:"letterSpacing"`
	FontWeight     int     `json:"fontWeight"`
	FontWeightBold int     `json:"fontWeightBold"`
	// CursorStyle is "block", "bar" or "underline".
	CursorStyle string `json:"cursorStyle"`
	CursorBlink bool   `json:"cursorBlink"`
	// ColorScheme is a scheme id; "auto" follows the app theme.
	ColorScheme     string `json:"colorScheme"`
	Scrollback      int    `json:"scrollback"`
	CopyOnSelect    bool   `json:"copyOnSelect"`
	RightClickPaste bool   `json:"rightClickPaste"`
	// CtrlWheelZoom zooms terminal text with Ctrl+mouse wheel.
	CtrlWheelZoom bool `json:"ctrlWheelZoom"`
	// Bell is "off" or "visual".
	Bell string `json:"bell"`
	// ConfirmClose asks before closing tabs or the window with live sessions.
	ConfirmClose bool `json:"confirmClose"`
	// Shell runs in local tabs; empty means $SHELL. Must be listed in /etc/shells.
	Shell string `json:"shell"`
}

// Limits for numeric settings.
const (
	MinFontSize, MaxFontSize           = 6, 72
	MinLineHeight, MaxLineHeight       = 1.0, 2.0
	MinLetterSpacing, MaxLetterSpacing = -2, 10
	MaxScrollback                      = 200000
)

// Defaults returns the out-of-the-box settings.
func Defaults() Settings {
	return Settings{
		Theme:        "dark",
		HostsSidebar: true,
		HostStatus:   true,
		Terminal: Terminal{
			FontSize:       13,
			LineHeight:     1.25,
			FontWeight:     400,
			FontWeightBold: 700,
			CursorStyle:    "block",
			CursorBlink:    true,
			ColorScheme:    "auto",
			Scrollback:     10000,
			Bell:           "off",
			ConfirmClose:   true,
			CtrlWheelZoom:  true,
		},
	}
}

// ValidationError names the setting that was rejected.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// A font family ends up inside a CSS font-family list, so keep it to plain names.
var fontFamilyRe = regexp.MustCompile(`^[\p{L}\p{N} ._+&()-]{1,64}$`)

var schemeRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// normalize clamps numbers into range and replaces unknown choices with
// defaults, so a hand-edited file never breaks the app.
func (s *Settings) normalize() {
	d := Defaults()
	oneOf(&s.Theme, d.Theme, "dark", "light", "system")
	t, dt := &s.Terminal, d.Terminal
	t.FontFamily = strings.TrimSpace(t.FontFamily)
	if t.FontFamily != "" && !fontFamilyRe.MatchString(t.FontFamily) {
		t.FontFamily = ""
	}
	if t.FontSize == 0 {
		t.FontSize = dt.FontSize
	}
	t.FontSize = clamp(t.FontSize, MinFontSize, MaxFontSize)
	if t.LineHeight == 0 || math.IsNaN(t.LineHeight) {
		t.LineHeight = dt.LineHeight
	}
	t.LineHeight = math.Round(math.Min(math.Max(t.LineHeight, MinLineHeight), MaxLineHeight)*100) / 100
	t.LetterSpacing = clamp(t.LetterSpacing, MinLetterSpacing, MaxLetterSpacing)
	t.FontWeight = weight(t.FontWeight, dt.FontWeight)
	t.FontWeightBold = weight(t.FontWeightBold, dt.FontWeightBold)
	oneOf(&t.CursorStyle, dt.CursorStyle, "block", "bar", "underline")
	if !schemeRe.MatchString(t.ColorScheme) {
		t.ColorScheme = dt.ColorScheme
	}
	t.Scrollback = clamp(t.Scrollback, 0, MaxScrollback)
	oneOf(&t.Bell, dt.Bell, "off", "visual")
}

func oneOf(v *string, def string, allowed ...string) {
	for _, a := range allowed {
		if *v == a {
			return
		}
	}
	*v = def
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

// weight rounds to a CSS weight step (100–900).
func weight(v, def int) int {
	if v == 0 {
		return def
	}
	return clamp((v+50)/100*100, 100, 900)
}

// DefaultPath is $XDG_CONFIG_HOME/tuxshell/settings.json.
func DefaultPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "tuxshell", "settings.json"), nil
}

// load reads path; a missing file gives the defaults and exists=false.
// Fields missing from the file keep their defaults.
func load(path string) (s Settings, exists bool, err error) {
	s = Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Defaults(), true, fmt.Errorf("%s: %w", path, err)
	}
	s.normalize()
	return s, true, nil
}

// save writes s atomically: a half-written file never replaces a good one.
func save(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// shells lists the login shells in /etc/shells.
func shells(etcShells string) ([]string, error) {
	f, err := os.Open(etcShells)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || !filepath.IsAbs(line) || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	return out, sc.Err()
}
