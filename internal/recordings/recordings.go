// Package recordings saves terminal output as asciinema v2 files (.cast),
// which `asciinema play` and TuxShell's own player can replay. Only what the
// terminal showed is saved, never the keys that were typed.
package recordings

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/vgrigolaia/tuxshell/internal/platform"
)

// MaxSize stops a recording that grows past it.
const MaxSize = 200 << 20

// MaxPlay is the largest recording the built-in player loads.
const MaxPlay = 50 << 20

// ErrTooLarge is returned when a recording reached MaxSize and stopped.
var ErrTooLarge = fmt.Errorf("the recording reached %d MB and stopped", MaxSize>>20)

// DefaultDir is recordings in the data folder ($XDG_DATA_HOME/tuxshell on Linux).
func DefaultDir() (string, error) {
	dir, err := platform.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "recordings"), nil
}

type header struct {
	Version   int    `json:"version"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Title     string `json:"title,omitempty"`
}

// Writer appends one session's output to a .cast file.
type Writer struct {
	mu      sync.Mutex
	f       *os.File
	w       *bufio.Writer
	start   time.Time
	size    int64
	pending []byte // an incomplete UTF-8 sequence from the last write
	err     error
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

// Create starts a recording in dir and returns it with its file name.
func Create(dir, title string, cols, rows int) (*Writer, string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, "", err
	}
	now := time.Now()
	label := strings.Trim(unsafeName.ReplaceAllString(title, "-"), "-.")
	if len(label) > 60 {
		label = label[:60]
	}
	if label == "" {
		label = "session"
	}
	base := now.Format("2006-01-02_15-04-05") + "_" + label
	var f *os.File
	var name string
	for i := 0; i < 100; i++ {
		name = base + ".cast"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.cast", base, i)
		}
		var err error
		f, err = os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
		f = nil
	}
	if f == nil {
		return nil, "", errors.New("too many recordings with this name")
	}
	w := &Writer{f: f, w: bufio.NewWriter(f), start: now}
	h, _ := json.Marshal(header{Version: 2, Width: max(cols, 1), Height: max(rows, 1), Timestamp: now.Unix(), Title: title})
	w.line(h)
	if w.err != nil {
		f.Close()
		return nil, "", w.err
	}
	return w, name, nil
}

func (w *Writer) line(b []byte) {
	if w.err != nil {
		return
	}
	n, err := w.w.Write(append(b, '\n'))
	w.size += int64(n)
	if err == nil && w.size > MaxSize {
		err = ErrTooLarge
	}
	w.err = err
}

func (w *Writer) event(kind string, data string) {
	t := time.Since(w.start).Seconds()
	b, _ := json.Marshal([]any{json.Number(fmt.Sprintf("%.6f", t)), kind, data})
	w.line(b)
}

// Output records bytes the terminal showed. A UTF-8 character split across
// two reads is kept back and written whole with the next one.
func (w *Writer) Output(p []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	data := append(w.pending, p...)
	cut := completeUTF8(data)
	w.pending = append([]byte(nil), data[cut:]...)
	if cut > 0 {
		w.event("o", string(data[:cut]))
	}
	return w.err
}

// Resize records a new terminal size.
func (w *Writer) Resize(cols, rows int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.event("r", fmt.Sprintf("%dx%d", cols, rows))
	return w.err
}

// Close finishes the file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		w.event("o", string(w.pending))
		w.pending = nil
	}
	err := w.w.Flush()
	if cerr := w.f.Close(); err == nil {
		err = cerr
	}
	return err
}

// completeUTF8 returns how much of b ends on a character boundary: a trailing
// sequence that's cut short (at most 3 bytes) is left out.
func completeUTF8(b []byte) int {
	for i := 1; i <= 3 && i <= len(b); i++ {
		c := b[len(b)-i]
		if c < 0x80 {
			return len(b) // ASCII: nothing pending
		}
		if c >= 0xC0 { // a lead byte: is its sequence complete?
			if utf8.FullRune(b[len(b)-i:]) {
				return len(b)
			}
			return len(b) - i
		}
	}
	return len(b)
}

// Recording is a saved .cast file.
type Recording struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Started int64  `json:"started"` // Unix milliseconds
	// Duration is the time of the last event, in seconds.
	Duration float64 `json:"duration"`
	Size     int64   `json:"size"`
}

// Service lists, plays and deletes recordings. Bound to the frontend.
type Service struct {
	dir string
}

// NewService manages the recordings in dir.
func NewService(dir string) *Service { return &Service{dir: dir} }

var nameRe = regexp.MustCompile(`^[^/\x00]+\.cast$`)

func (s *Service) path(name string) (string, error) {
	if !nameRe.MatchString(name) || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", errors.New("not a recording")
	}
	return filepath.Join(s.dir, name), nil
}

// List returns the recordings, newest first.
func (s *Service) List() ([]Recording, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Recording{}, nil
	}
	if err != nil {
		return nil, err
	}
	list := []Recording{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cast") {
			continue
		}
		r, err := describe(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		r.Name = e.Name()
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Started > list[j].Started })
	return list, nil
}

// describe reads a recording's header and the time of its last event.
func describe(path string) (Recording, error) {
	f, err := os.Open(path)
	if err != nil {
		return Recording{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Recording{}, err
	}
	first, err := bufio.NewReaderSize(f, 4096).ReadBytes('\n')
	if err != nil && len(first) == 0 {
		return Recording{}, err
	}
	var h header
	if json.Unmarshal(first, &h) != nil || h.Version != 2 {
		return Recording{}, errors.New("not an asciinema v2 file")
	}
	r := Recording{Title: h.Title, Started: h.Timestamp * 1000, Size: info.Size()}
	// The last complete line holds the last event's time.
	tail := int64(64 << 10)
	if tail > info.Size() {
		tail = info.Size()
	}
	buf := make([]byte, tail)
	if _, err := f.ReadAt(buf, info.Size()-tail); err != nil && err != io.EOF {
		return r, nil
	}
	lines := bytes.Split(bytes.TrimRight(buf, "\n"), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var ev []json.RawMessage
		if json.Unmarshal(lines[i], &ev) == nil && len(ev) == 3 {
			var t float64
			if json.Unmarshal(ev[0], &t) == nil {
				r.Duration = t
				break
			}
		}
	}
	return r, nil
}

// Load returns a recording's contents for the player.
func (s *Service) Load(name string) (string, error) {
	p, err := s.path(name)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if info.Size() > MaxPlay {
		return "", fmt.Errorf("this recording is over %d MB; play it with: asciinema play %s", MaxPlay>>20, p)
	}
	b, err := os.ReadFile(p)
	return string(b), err
}

// Delete removes a recording.
func (s *Service) Delete(name string) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// OpenFolder shows the recordings folder in the file manager.
func (s *Service) OpenFolder() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	return platform.Open(s.dir)
}

// Dir is where recordings are saved, for display.
func (s *Service) Dir() string { return s.dir }

// Folder is where recordings are saved, with ~ for the home folder, for Preferences.
func (s *Service) Folder() string { return platform.Tilde(s.dir) }
