package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Window holds the global window options.
type Window struct {
	// Start is how the window opens: "last" (as it was when TuxShell last
	// closed), "maximized" or "fullscreen".
	Start string `json:"start"`
	// AlwaysOnTop keeps the window above others. X11 and macOS only: Wayland
	// has no way for an app to ask for it.
	AlwaysOnTop bool `json:"alwaysOnTop"`
}

// WindowState is how the window was when TuxShell last closed. It lives in
// its own file (WindowStatePath), written only by Go, so saving settings from
// the frontend never overwrites it with an older size.
type WindowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized"`
}

// Window size limits: the window's own minimum, and a sane maximum.
const (
	MinWindowWidth  = 760
	MinWindowHeight = 480
	maxWindowSide   = 16384
)

// WindowStatePath is window.json next to settings.json.
func WindowStatePath(settingsPath string) string {
	return filepath.Join(filepath.Dir(settingsPath), "window.json")
}

// LoadWindowState reads the last window state; a missing or broken file, or
// a size out of range, gives a zero size (the app's default).
func LoadWindowState(path string) WindowState {
	var s WindowState
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &s) != nil {
		return WindowState{}
	}
	if s.Width < MinWindowWidth || s.Height < MinWindowHeight || s.Width > maxWindowSide || s.Height > maxWindowSide {
		s.Width, s.Height = 0, 0
	}
	return s
}

// SaveWindowState writes s atomically (0600 in the private config folder).
func SaveWindowState(path string, s WindowState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".window-*.json")
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
