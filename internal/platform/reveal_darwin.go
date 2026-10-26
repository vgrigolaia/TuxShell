package platform

import (
	"fmt"
	"os/exec"
)

// Reveal shows file selected in a Finder window.
func Reveal(file string) error {
	cmd := exec.Command("open", "-R", "--", file)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("couldn't show %s: %w", file, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
