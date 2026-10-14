// Package backgrounds keeps the images chosen as terminal backgrounds. A
// chosen image is copied into a private folder, so moving or deleting the
// original doesn't break the profile, and named by its content.
package backgrounds

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

// MaxSize is the largest image accepted.
const MaxSize = 15 << 20

// PickFunc shows a file chooser and returns the chosen path ("" if cancelled).
type PickFunc func() (string, error)

// Service is bound to the frontend.
type Service struct {
	dir   string
	pick  PickFunc
	inUse func() []string
}

// ErrNotImage is returned for files that aren't PNG, JPEG or WebP.
var ErrNotImage = errors.New("choose a PNG, JPEG or WebP image")

var nameRe = regexp.MustCompile(`^[0-9a-f]{16}\.(png|jpg|webp)$`)

// DefaultDir is $XDG_DATA_HOME/tuxshell/backgrounds.
func DefaultDir() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "tuxshell", "backgrounds"), nil
}

// NewService stores images in dir. inUse lists the images profiles still
// use; the others are removed when a new one is added.
func NewService(dir string, pick PickFunc, inUse func() []string) *Service {
	return &Service{dir: dir, pick: pick, inUse: inUse}
}

// Choose asks for an image and returns its name for the profile, or "" if
// the user cancelled.
func (s *Service) Choose() (string, error) {
	path, err := s.pick()
	if err != nil || path == "" {
		return "", err
	}
	return s.add(path)
}

func (s *Service) add(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrNotImage
	}
	if info.Size() > MaxSize {
		return "", fmt.Errorf("the image is larger than %d MB", MaxSize>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	ext := kind(data)
	if ext == "" {
		return "", ErrNotImage
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:8]) + "." + ext
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(s.dir, name), data, 0o600); err != nil {
		return "", err
	}
	s.prune(name)
	return name, nil
}

// prune removes images no profile uses, except keep.
func (s *Service) prune(keep string) {
	used := []string{keep}
	if s.inUse != nil {
		used = append(used, s.inUse()...)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if nameRe.MatchString(e.Name()) && !slices.Contains(used, e.Name()) {
			_ = os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
}

// Data returns an image as a data: URL for the terminal to show.
func (s *Service) Data(name string) (string, error) {
	if !nameRe.MatchString(name) {
		return "", ErrNotImage
	}
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return "", err
	}
	mime := map[string]string{"png": "image/png", "jpg": "image/jpeg", "webp": "image/webp"}[kind(data)]
	if mime == "" {
		return "", ErrNotImage
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// kind recognises an image by its first bytes, not its file name.
func kind(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "jpg"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "webp"
	}
	return ""
}
