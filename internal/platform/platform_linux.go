package platform

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const opener = "xdg-open"

// DataDir is $XDG_DATA_HOME/tuxshell (~/.local/share/tuxshell).
func DataDir() (string, error) { return xdgDir("XDG_DATA_HOME", ".local", "share") }

// ConfigDir is $XDG_CONFIG_HOME/tuxshell (~/.config/tuxshell).
func ConfigDir() (string, error) { return xdgDir("XDG_CONFIG_HOME", ".config") }

func xdgDir(env string, fallback ...string) (string, error) {
	dir := os.Getenv(env)
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(append([]string{home}, fallback...)...)
	}
	return filepath.Join(dir, "tuxshell"), nil
}

// RuntimeDir is $XDG_RUNTIME_DIR (private, cleared at logout), or "".
func RuntimeDir() string { return os.Getenv("XDG_RUNTIME_DIR") }

// SocketTempDir is where private socket folders go when RuntimeDir is
// missing or too long.
func SocketTempDir() string { return os.TempDir() }

// DownloadsDir is the desktop's Downloads folder (XDG_DOWNLOAD_DIR).
func DownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	if f, err := os.Open(filepath.Join(cfg, "user-dirs.dirs")); err == nil {
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			v, ok := strings.CutPrefix(strings.TrimSpace(s.Text()), "XDG_DOWNLOAD_DIR=")
			if !ok {
				continue
			}
			v = strings.ReplaceAll(strings.Trim(v, `"`), "$HOME", home)
			if filepath.IsAbs(v) && filepath.Clean(v) != home {
				dir = filepath.Clean(v)
			}
		}
	}
	return dir, nil
}

// DisplayServer names the desktop session type, for the status bar.
func DisplayServer() string {
	switch os.Getenv("XDG_SESSION_TYPE") {
	case "wayland":
		return "Wayland"
	case "x11":
		return "X11"
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "Wayland"
	}
	if os.Getenv("DISPLAY") != "" {
		return "X11"
	}
	return ""
}
