// Package platform holds what differs between Linux and macOS: where files
// live, how to show a file or folder in the desktop, and the session type.
package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Open shows path in the desktop's default app for it (a folder in the file
// manager). It doesn't wait for that app to close.
func Open(path string) error {
	cmd := exec.Command(opener, path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("couldn't open %s: %w", path, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// AddToolDirs appends the platform's extra tool folders (Homebrew on macOS)
// to PATH, so ssh, op and FreeRDP are found when the app was started from
// the Dock with a minimal PATH.
func AddToolDirs() {
	path := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range toolDirs {
		if !slices.Contains(path, dir) {
			path = append(path, dir)
		}
	}
	_ = os.Setenv("PATH", strings.Join(path, string(os.PathListSeparator)))
}
