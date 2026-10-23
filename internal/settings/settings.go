// Package settings keeps user preferences in a JSON file under
// $XDG_CONFIG_HOME/tuxshell, readable and editable by hand.
package settings

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/platform"
)

// Settings are the app-wide preferences.
type Settings struct {
	// Theme is "dark", "light" or "system".
	Theme string `json:"theme"`
	// HostsSidebar shows the hosts sidebar.
	HostsSidebar bool `json:"hostsSidebar"`
	// SidebarWidth is the hosts sidebar's width in pixels, set by dragging its
	// edge; 0 means the default.
	SidebarWidth int `json:"sidebarWidth"`
	// TabsPosition puts the open tabs in the left panel ("left") or the title bar ("top").
	TabsPosition string `json:"tabsPosition"`
	// GroupOrder is the order of host groups chosen by dragging; groups not
	// listed follow in their usual order.
	GroupOrder []string `json:"groupOrder"`
	// EmptyGroups are host groups made with "New group" that have no hosts
	// yet; a group with hosts exists through its hosts.
	EmptyGroups []string `json:"emptyGroups"`
	// HiddenSSHConfigHosts are ~/.ssh/config aliases the user chose not to be
	// reminded about on Home; a new alias brings the reminder back.
	HiddenSSHConfigHosts []string `json:"hiddenSshConfigHosts"`
	// HostStatus pings saved hosts in the background to show which are up.
	HostStatus bool `json:"hostStatus"`
	// UpdateCheck asks GitHub for a newer release at startup (on by default;
	// it only links to the release, never downloads anything).
	UpdateCheck bool `json:"updateCheck"`
	// SavePasswords allows saving host passwords in the system keyring (on by
	// default): typed into a host, ticked "Remember", or found in an imported
	// file. Keys, the ssh agent or 1Password stay the safer choice.
	SavePasswords bool `json:"savePasswords"`
	// OnePasswordAgent makes TuxShell's ssh use the 1Password SSH agent
	// (~/.1password/agent.sock) for keys. An IdentityAgent in ~/.ssh/config
	// still wins for its hosts.
	OnePasswordAgent bool     `json:"onePasswordAgent"`
	Terminal         Terminal `json:"terminal"`
	// Profiles are the user's named terminal profiles.
	Profiles []NamedProfile `json:"profiles"`
	// History keeps a list of connections: host, time and duration only.
	History History `json:"history"`
	// RDPClient picks the FreeRDP program: "auto", "x11" or "sdl".
	RDPClient string `json:"rdpClient"`
	// Keybindings maps an app action ("toggleHosts") to a key combo
	// ("Ctrl+Shift+L"). Actions not listed keep their default; "" turns one off.
	// The actions and their defaults live in the frontend.
	Keybindings map[string]string `json:"keybindings"`
}

// History says whether and how long connections are remembered.
type History struct {
	Enabled bool `json:"enabled"`
	// Days is how long entries are kept.
	Days int `json:"days"`
	// Max is how many entries are kept.
	Max int `json:"max"`
}

// Terminal holds the default profile plus the settings that apply to every
// terminal. The default profile's fields sit at the top level (embedded), so
// settings files from before profiles still load.
type Terminal struct {
	Profile
	CopyOnSelect    bool `json:"copyOnSelect"`
	RightClickPaste bool `json:"rightClickPaste"`
	// CtrlWheelZoom zooms terminal text with Ctrl+mouse wheel.
	CtrlWheelZoom bool `json:"ctrlWheelZoom"`
	// ConfirmClose asks before closing tabs or the window with live sessions.
	ConfirmClose bool `json:"confirmClose"`
	// CloseOnExit closes a pane when its session ends with exit or logout;
	// errors (ssh's own exit code 255) keep it open with Reconnect.
	CloseOnExit bool `json:"closeOnExit"`
	// Shell runs in local tabs; empty means $SHELL. Must be listed in /etc/shells.
	Shell string `json:"shell"`
}

// Profile is the look and behaviour of a terminal pane. The default profile
// is Terminal.Profile; hosts can pick a NamedProfile instead.
type Profile struct {
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
	// ColorScheme is a scheme id; "auto" follows the app theme, "custom" uses Palette.
	ColorScheme string  `json:"colorScheme"`
	Palette     Palette `json:"palette"`
	// Background is an image in the backgrounds folder (see Backgrounds), or "".
	Background string `json:"background"`
	// BackgroundShade is how much of the colour scheme's background covers
	// the image, 0–100.
	BackgroundShade int `json:"backgroundShade"`
	// HeaderColor tints the pane header: "" (none) or "#rrggbb".
	HeaderColor string `json:"headerColor"`
	Scrollback  int    `json:"scrollback"`
	// Scrollbar is whether new panes show a scrollbar.
	Scrollbar bool `json:"scrollbar"`
	// ScrollOnOutput jumps to the bottom when new output arrives.
	ScrollOnOutput bool `json:"scrollOnOutput"`
	// ScrollOnKeypress jumps to the bottom when typing.
	ScrollOnKeypress bool `json:"scrollOnKeypress"`
	// ScrollSpeed multiplies mouse wheel scrolling, 1–10.
	ScrollSpeed int `json:"scrollSpeed"`
	// Bell is "off" or "visual" (flash the pane).
	Bell string `json:"bell"`
	// BellIcon marks the pane and tab with a bell until it's looked at.
	BellIcon bool `json:"bellIcon"`
	// BellSound plays a short beep.
	BellSound bool `json:"bellSound"`
	// Command runs on SSH hosts after login instead of only a shell.
	Command string `json:"command"`
	// KeepShell starts the login shell after Command ends.
	KeepShell bool `json:"keepShell"`
	// Backspace sends "del" (^?) or "ctrl-h" (^H).
	Backspace string `json:"backspace"`
	// Delete sends "escape" (ESC [3~), "del" or "ctrl-h".
	Delete string `json:"delete"`
	// Record starts recording each session's output when it connects.
	Record bool `json:"record"`
}

// Palette is a custom colour scheme: 16 ANSI colours (normal then bright)
// plus the special colours. All are "#rrggbb".
type Palette struct {
	Background string   `json:"background"`
	Foreground string   `json:"foreground"`
	Cursor     string   `json:"cursor"`
	Selection  string   `json:"selection"`
	Colors     []string `json:"colors"`
}

// NamedProfile is a profile the user created, chosen per host.
type NamedProfile struct {
	// ID is what hosts refer to; it never changes when the profile is renamed.
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Profile Profile `json:"profile"`
}

// UnmarshalJSON fills settings missing from the file with the defaults.
func (p *NamedProfile) UnmarshalJSON(b []byte) error {
	type plain NamedProfile
	v := plain{Profile: DefaultProfile()}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*p = NamedProfile(v)
	return nil
}

// RemoteCommand is what ssh runs on the server for this profile, or "" for
// just the login shell.
func (p Profile) RemoteCommand() string {
	if p.Command == "" {
		return ""
	}
	if p.KeepShell {
		return p.Command + "; exec $SHELL -l"
	}
	return p.Command
}

// DefaultProfileID stands for Terminal.Profile.
const DefaultProfileID = "default"

// MaxProfiles limits how many named profiles there can be.
const MaxProfiles = 50

// Limits for numeric settings.
const (
	MinFontSize, MaxFontSize           = 6, 72
	MinLineHeight, MaxLineHeight       = 1.0, 2.0
	MinLetterSpacing, MaxLetterSpacing = -2, 10
	MaxScrollback                      = 200000
	MaxCommand                         = 1000
)

// Defaults returns the out-of-the-box settings.
func Defaults() Settings {
	return Settings{
		Theme:         "dark",
		HostsSidebar:  true,
		HostStatus:    true,
		TabsPosition:  "left",
		UpdateCheck:   true,
		SavePasswords: true,
		Terminal: Terminal{
			Profile:      DefaultProfile(),
			ConfirmClose: true,
			// On for new installs since 2026-10-23; saved settings keep their choice.
			CopyOnSelect:  true,
			CloseOnExit:   true,
			CtrlWheelZoom: true,
		},
		Profiles:             []NamedProfile{},
		GroupOrder:           []string{},
		EmptyGroups:          []string{},
		HiddenSSHConfigHosts: []string{},
		History:              History{Enabled: true, Days: 90, Max: 1000},
		RDPClient:            "auto",
		Keybindings:          map[string]string{},
	}
}

// DefaultProfile returns the out-of-the-box profile.
func DefaultProfile() Profile {
	return Profile{
		FontSize:         13,
		LineHeight:       1.25,
		FontWeight:       400,
		FontWeightBold:   700,
		CursorStyle:      "block",
		CursorBlink:      true,
		ColorScheme:      "auto",
		Palette:          DefaultPalette(),
		BackgroundShade:  70,
		Scrollback:       10000,
		Scrollbar:        true,
		ScrollOnKeypress: true,
		ScrollSpeed:      1,
		Bell:             "off",
		KeepShell:        true,
		Backspace:        "del",
		Delete:           "escape",
	}
}

// DefaultPalette is the starting point for a custom scheme (Tango).
func DefaultPalette() Palette {
	return Palette{
		Background: "#111318", Foreground: "#d3d7cf", Cursor: "#8b7cff", Selection: "#3b3f4c",
		Colors: []string{
			"#2e3436", "#cc0000", "#4e9a06", "#c4a000", "#3465a4", "#75507b", "#06989a", "#d3d7cf",
			"#555753", "#ef2929", "#8ae234", "#fce94f", "#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec",
		},
	}
}

// ProfileFor returns the profile with id, or the default profile.
func (s Settings) ProfileFor(id string) Profile {
	for _, p := range s.Profiles {
		if p.ID == id {
			return p.Profile
		}
	}
	return s.Terminal.Profile
}

// ValidationError names the setting that was rejected.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// A font family ends up inside a CSS font-family list, so keep it to plain names.
var fontFamilyRe = regexp.MustCompile(`^[\p{L}\p{N} ._+&()-]{1,64}$`)

var (
	schemeRe     = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	profileIDRe  = regexp.MustCompile(`^[a-z0-9]{1,32}$`)
	colorRe      = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	backgroundRe = regexp.MustCompile(`^[0-9a-f]{16}\.(png|jpg|webp)$`)
)

// normalize clamps numbers into range and replaces unknown choices with
// defaults, so a hand-edited file never breaks the app.
func (s *Settings) normalize() {
	d := Defaults()
	oneOf(&s.Theme, d.Theme, "dark", "light", "system")
	s.Terminal.Profile.normalize()
	oneOf(&s.RDPClient, d.RDPClient, "auto", "x11", "sdl")
	oneOf(&s.TabsPosition, d.TabsPosition, "left", "top")
	if s.SidebarWidth != 0 && (s.SidebarWidth < MinSidebarWidth || s.SidebarWidth > MaxSidebarWidth) {
		s.SidebarWidth = 0
	}
	s.GroupOrder = cleanGroupOrder(s.GroupOrder)
	s.EmptyGroups = cleanGroupOrder(s.EmptyGroups)
	s.HiddenSSHConfigHosts = cleanGroupOrder(s.HiddenSSHConfigHosts)
	s.Keybindings = cleanKeybindings(s.Keybindings)
	if s.History.Days == 0 {
		s.History.Days = d.History.Days
	}
	if s.History.Max == 0 {
		s.History.Max = d.History.Max
	}
	s.History.Days = clamp(s.History.Days, 1, 3650)
	s.History.Max = clamp(s.History.Max, 50, 100000)
	seen := map[string]bool{DefaultProfileID: true}
	list := []NamedProfile{}
	for _, p := range s.Profiles {
		p.Name = strings.TrimSpace(p.Name)
		if !profileIDRe.MatchString(p.ID) || seen[p.ID] || len(list) == MaxProfiles {
			continue
		}
		seen[p.ID] = true
		if p.Name == "" {
			p.Name = "Profile"
		}
		if len([]rune(p.Name)) > 40 {
			p.Name = string([]rune(p.Name)[:40])
		}
		p.Profile.normalize()
		list = append(list, p)
	}
	s.Profiles = list
}

func (t *Profile) normalize() {
	dt := DefaultProfile()
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
	t.Palette.normalize()
	if t.Background != "" && !backgroundRe.MatchString(t.Background) {
		t.Background = ""
	}
	t.BackgroundShade = clamp(t.BackgroundShade, 0, 100)
	t.HeaderColor = strings.ToLower(t.HeaderColor)
	if t.HeaderColor != "" && !colorRe.MatchString(t.HeaderColor) {
		t.HeaderColor = ""
	}
	t.Scrollback = clamp(t.Scrollback, 0, MaxScrollback)
	if t.ScrollSpeed == 0 {
		t.ScrollSpeed = dt.ScrollSpeed
	}
	t.ScrollSpeed = clamp(t.ScrollSpeed, 1, 10)
	oneOf(&t.Bell, dt.Bell, "off", "visual")
	t.Command = strings.TrimSpace(strings.ReplaceAll(t.Command, "\x00", ""))
	if len(t.Command) > MaxCommand || strings.ContainsAny(t.Command, "\n\r") {
		t.Command = ""
	}
	oneOf(&t.Backspace, dt.Backspace, "del", "ctrl-h")
	oneOf(&t.Delete, dt.Delete, "escape", "del", "ctrl-h")
}

func (p *Palette) normalize() {
	d := DefaultPalette()
	fix := func(v *string, def string) {
		*v = strings.ToLower(strings.TrimSpace(*v))
		if !colorRe.MatchString(*v) {
			*v = def
		}
	}
	fix(&p.Background, d.Background)
	fix(&p.Foreground, d.Foreground)
	fix(&p.Cursor, d.Cursor)
	fix(&p.Selection, d.Selection)
	colors := make([]string, 16)
	for i := range colors {
		if i < len(p.Colors) {
			colors[i] = p.Colors[i]
		}
		fix(&colors[i], d.Colors[i])
	}
	p.Colors = colors
}

func oneOf(v *string, def string, allowed ...string) {
	for _, a := range allowed {
		if *v == a {
			return
		}
	}
	*v = def
}

// The hosts sidebar can be dragged between these widths (pixels).
const (
	MinSidebarWidth = 200
	MaxSidebarWidth = 520
)

// Limits for GroupOrder, EmptyGroups and HiddenSSHConfigHosts, so a hand-edited file stays small.
const (
	maxGroups      = 1000
	maxGroupLength = 200
)

// cleanGroupOrder drops blank, overlong and repeated names (group names or aliases).
func cleanGroupOrder(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, g := range in {
		if g == "" || len(g) > maxGroupLength || seen[g] || len(out) == maxGroups {
			continue
		}
		seen[g] = true
		out = append(out, g)
	}
	return out
}

// MaxKeybindings caps the keybindings map; there are far fewer actions.
const MaxKeybindings = 100

var (
	actionIDRe = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,39}$`)
	// A combo is its modifiers in this order, then one key: "Ctrl+Shift+L".
	comboRe = regexp.MustCompile(`^(Ctrl\+)?(Alt\+)?(Cmd\+)?(Shift\+)?([A-Z0-9]|F[1-9]|F1[0-9]|F2[0-4]|Up|Down|Left|Right|Insert|Delete|Home|End|PageUp|PageDown|Tab|Enter|Space|Backspace|[,./;'\[\]\\\x60=-])$`)
)

// cleanKeybindings drops entries with a malformed action or combo, so those
// actions fall back to their default.
func cleanKeybindings(in map[string]string) map[string]string {
	out := map[string]string{}
	for id, combo := range in {
		if len(out) == MaxKeybindings {
			break
		}
		if actionIDRe.MatchString(id) && (combo == "" || comboRe.MatchString(combo)) {
			out[id] = combo
		}
	}
	return out
}

func readShells(path string) ([]byte, error) {
	if platform.Flatpak && path == "/etc/shells" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return platform.HostCommand(ctx, nil, "", "cat", "/etc/shells").Output()
	}
	return os.ReadFile(path)
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

// weight rounds to a CSS weight step (100–900).
func weight(v, def int) int {
	if v == 0 {
		return def
	}
	return clamp((v+50)/100*100, 100, 900)
}

// DefaultPath is settings.json in the config folder ($XDG_CONFIG_HOME/tuxshell on Linux).
func DefaultPath() (string, error) {
	dir, err := platform.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
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

// shells lists the login shells in /etc/shells (the host's in a Flatpak,
// where local tabs run on the host).
func shells(etcShells string) ([]string, error) {
	data, err := readShells(etcShells)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
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
