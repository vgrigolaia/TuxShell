package settings

import (
	"context"
	"log"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/vgrigolaia/tuxshell/internal/platform"
)

// Bundled fonts are always available, so they're not in the system list.
var bundled = []string{"JetBrains Mono", "JetBrains Mono Variable", "Geist", "Geist Variable", "Space Grotesk", "Space Grotesk Variable"}

// Service exposes the settings to the frontend and to other Go services.
type Service struct {
	path       string
	etcShells  string
	mu         sync.Mutex
	current    Settings
	fileExists bool
}

// NewService loads settings from path. A broken file is logged and the
// defaults are used until the user saves again.
func NewService(path string) *Service {
	s, exists, err := load(path)
	if err != nil {
		log.Printf("settings: %v (using defaults)", err)
	}
	svc := &Service{path: path, etcShells: "/etc/shells", current: s, fileExists: exists}
	// A shell that was removed from /etc/shells falls back to $SHELL.
	if svc.current.Terminal.Shell != "" && !svc.shellAllowed(svc.current.Terminal.Shell) {
		svc.current.Terminal.Shell = ""
	}
	return svc
}

// Loaded is the stored settings plus whether a settings file exists yet
// (the frontend migrates its older saved choices when it doesn't).
type Loaded struct {
	Settings Settings `json:"settings"`
	Saved    bool     `json:"saved"`
}

// Get returns the current settings.
func (s *Service) Get() Loaded {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Loaded{Settings: s.current, Saved: s.fileExists}
}

// Save validates and stores settings, returning them as stored
// (out-of-range numbers are clamped).
func (s *Service) Save(in Settings) (Settings, error) {
	in.normalize()
	in.Terminal.Shell = strings.TrimSpace(in.Terminal.Shell)
	if in.Terminal.Shell != "" && !s.shellAllowed(in.Terminal.Shell) {
		return Settings{}, &ValidationError{Field: "shell", Message: "not a login shell listed in /etc/shells"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := save(s.path, in); err != nil {
		return Settings{}, err
	}
	s.current, s.fileExists = in, true
	return in, nil
}

// Reset restores the defaults.
func (s *Service) Reset() (Settings, error) {
	return s.Save(Defaults())
}

// Shell returns the configured local shell, or "" for $SHELL.
func (s *Service) Shell() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current.Terminal.Shell
}

// File is the settings file, with ~ for the home folder, for Preferences.
func (s *Service) File() string { return platform.Tilde(s.path) }

// Shells lists the login shells from /etc/shells.
func (s *Service) Shells() []string {
	list, err := shells(s.etcShells)
	if err != nil {
		return []string{}
	}
	return list
}

func (s *Service) shellAllowed(sh string) bool {
	return slices.Contains(s.Shells(), sh)
}

// Fonts lists installed monospace font families (via fontconfig), sorted,
// without the bundled ones.
func (s *Service) Fonts(ctx context.Context) []string {
	out, err := exec.CommandContext(ctx, "fc-list", "--format", "%{family[0]}\n", ":spacing=mono").Output()
	if err != nil {
		return []string{}
	}
	return parseFonts(string(out))
}

func parseFonts(out string) []string {
	seen := map[string]bool{}
	list := []string{}
	for _, line := range strings.Split(out, "\n") {
		// fontconfig escapes "-", ",", ":" etc. with a backslash.
		name := strings.TrimSpace(strings.ReplaceAll(line, `\`, ""))
		if name == "" || seen[name] || slices.Contains(bundled, name) || !fontFamilyRe.MatchString(name) {
			continue
		}
		seen[name] = true
		list = append(list, name)
	}
	slices.SortFunc(list, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return list
}
