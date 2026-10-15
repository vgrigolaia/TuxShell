package platform

import (
	"os"
	"path/filepath"
)

const opener = "open"

// toolDirs are Homebrew (Apple silicon, then Intel) and MacPorts.
var toolDirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"}

// DataDir is ~/Library/Application Support/TuxShell.
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "TuxShell"), nil
}

// ConfigDir is the same folder as DataDir, as usual on macOS.
func ConfigDir() (string, error) { return DataDir() }

// RuntimeDir is "": macOS has no per-login private runtime folder.
func RuntimeDir() string { return "" }

// SocketTempDir is /tmp: $TMPDIR is too long for socket paths on macOS.
func SocketTempDir() string { return "/tmp" }

// DownloadsDir is ~/Downloads.
func DownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Downloads"), nil
}

// DisplayServer names the desktop, for the status bar.
func DisplayServer() string { return "macOS" }
