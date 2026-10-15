package rdp

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Files copied to and from a remote desktop in a tab. The page has no file
// access of its own: Go serves only the files the user picked or dropped
// (one-time links), and writes what Windows copied under Downloads. Both work
// only while that tab's connection is open.

// Offered is a local file (or folder) the tab can paste on the remote desktop.
type Offered struct {
	Name string `json:"name"`
	// Path is the folder inside what was dropped, with `\` (the RDP convention); "" at the top.
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"` // ms since the epoch
	Dir      bool   `json:"dir"`
	// URL reads the file once (empty for folders).
	URL string `json:"url"`
}

// FilesEvent tells a tab that files were dropped on it.
type FilesEvent struct {
	Key   string    `json:"key"`
	Files []Offered `json:"files"`
	Error string    `json:"error"`
}

// EventFiles is emitted when files are dropped on a remote desktop tab.
const EventFiles = "rdp:files"

// DropPrefix starts the element id of a tab's drop area; the files key follows.
const DropPrefix = "rdp-drop-"

// Limits for one offer: enough for everyday files, small enough that the
// page (which holds them in memory) stays responsive.
const (
	maxOfferBytes   = 2 << 30
	maxOfferEntries = 10000
	maxSaveBytes    = 2 << 30
)

// tabFiles belongs to one open tab.
type tabFiles struct {
	offered map[string]string // one-time token → local path
}

func newKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// open and close bracket one tab's connection.
func (r *Relay) openTab(key string) {
	r.mu.Lock()
	r.tabs[key] = &tabFiles{offered: map[string]string{}}
	r.mu.Unlock()
}

func (r *Relay) closeTab(key string) {
	r.mu.Lock()
	delete(r.tabs, key)
	r.mu.Unlock()
}

func (r *Relay) live(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return key != "" && r.tabs[key] != nil
}

// offer registers local paths (folders are walked) for the tab.
func (r *Relay) offer(key string, paths []string) ([]Offered, error) {
	if !r.live(key) {
		return nil, errors.New("this remote desktop isn't connected")
	}
	var out []Offered
	var total int64
	add := func(abs, rel string, d fs.DirEntry) error {
		info, err := d.Info()
		if err != nil {
			return err
		}
		if len(out) >= maxOfferEntries {
			return fmt.Errorf("more than %d files; use the remote desktop's own window for this many", maxOfferEntries)
		}
		o := Offered{Name: d.Name(), Modified: info.ModTime().UnixMilli(), Dir: d.IsDir()}
		if dir := filepath.Dir(rel); dir != "." {
			o.Path = strings.ReplaceAll(dir, string(filepath.Separator), `\`)
		}
		if !d.IsDir() {
			if !info.Mode().IsRegular() {
				return nil // sockets, devices and links are skipped
			}
			total += info.Size()
			if total > maxOfferBytes {
				return errors.New("more than 2 GB; use the remote desktop's own window for files this big")
			}
			o.Size = info.Size()
			tok := newKey()
			r.mu.Lock()
			if t := r.tabs[key]; t != nil {
				t.offered[tok] = abs
			}
			r.mu.Unlock()
			o.URL = r.base() + "/file?t=" + tok
		}
		out = append(out, o)
		return nil
	}
	for _, p := range paths {
		p = filepath.Clean(p)
		info, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if err := add(p, filepath.Base(p), fs.FileInfoToDirEntry(info)); err != nil {
				return nil, err
			}
			continue
		}
		parent := filepath.Dir(p)
		err = filepath.WalkDir(p, func(abs string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			rel, err := filepath.Rel(parent, abs)
			if err != nil {
				return err
			}
			return add(abs, rel, d)
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *Relay) base() string { return "http://" + r.ln.Addr().String() }

// cors lets TuxShell's page (wails:// or the dev server) call the relay.
// Every route still needs a token only Go hands out.
func cors(w http.ResponseWriter, req *http.Request) bool {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST")
	h.Set("Access-Control-Allow-Headers", "Content-Type")
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// serveFile hands one offered file to the page, once.
func (r *Relay) serveFile(w http.ResponseWriter, req *http.Request) {
	if cors(w, req) {
		return
	}
	tok := req.URL.Query().Get("t")
	var path string
	r.mu.Lock()
	for _, t := range r.tabs {
		if p, ok := t.offered[tok]; ok && tok != "" {
			path = p
			delete(t.offered, tok)
			break
		}
	}
	r.mu.Unlock()
	if path == "" {
		http.Error(w, "unknown file", http.StatusNotFound)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "can't read the file", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "can't read the file", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, req, "", info.ModTime(), f)
}

// saveFile writes a file Windows copied under Downloads (keeping its
// folders), never replacing anything there.
func (r *Relay) saveFile(w http.ResponseWriter, req *http.Request) {
	if cors(w, req) {
		return
	}
	q := req.URL.Query()
	if req.Method != http.MethodPost || !r.live(q.Get("k")) {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	dir, err := downloadsDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	saved, err := saveUnder(dir, q.Get("path"), q.Get("name"), http.MaxBytesReader(w, req.Body, maxSaveBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"saved": saved})
}

// cleanParts splits a remote path (`\` or `/`) into safe names.
func cleanParts(p string) ([]string, error) {
	var out []string
	for _, part := range strings.FieldsFunc(p, func(c rune) bool { return c == '\\' || c == '/' }) {
		if part == "." || part == ".." || strings.ContainsRune(part, 0) {
			return nil, errors.New("bad file name")
		}
		out = append(out, part)
	}
	return out, nil
}

// saveUnder writes body to dir/path/name. os.Root keeps every step inside
// dir, even through links someone left there.
func saveUnder(dir, path, name string, body io.Reader) (string, error) {
	folders, err := cleanParts(path)
	if err != nil {
		return "", err
	}
	base, err := cleanParts(name)
	if err != nil || len(base) != 1 {
		return "", errors.New("bad file name")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	sub := "."
	for _, f := range folders {
		sub = filepath.Join(sub, f)
		if err := root.Mkdir(sub, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	ext := filepath.Ext(base[0])
	stem := strings.TrimSuffix(base[0], ext)
	for i := 0; i < 1000; i++ {
		n := base[0]
		if i > 0 {
			n = stem + " (" + strconv.Itoa(i) + ")" + ext
		}
		rel := filepath.Join(sub, n)
		f, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = io.Copy(f, body)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = root.Remove(rel)
			return "", err
		}
		return filepath.Join(dir, rel), nil
	}
	return "", errors.New("too many files with this name")
}

// downloadsDir is the desktop's Downloads folder (XDG_DOWNLOAD_DIR), made if missing.
func downloadsDir() (string, error) {
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Dropped offers files dropped on a tab. A function, so it isn't bound: only
// the window's drop handler names local paths.
func Dropped(s *Service, key string, paths []string) ([]Offered, error) {
	if s.relay == nil {
		return nil, errors.New("remote desktops in tabs aren't available")
	}
	return s.relay.offer(key, paths)
}

// UseFilePicker sets the dialog for "Send files…". A function, so it isn't bound.
func UseFilePicker(s *Service, pick func() ([]string, error)) { s.pickFiles = pick }

// PickFiles asks for files to paste on the remote desktop in tab key.
func (s *Service) PickFiles(key string) ([]Offered, error) {
	if s.relay == nil || s.pickFiles == nil {
		return nil, errors.New("sending files isn't available")
	}
	if !s.relay.live(key) {
		return nil, errors.New("this remote desktop isn't connected")
	}
	paths, err := s.pickFiles()
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	return s.relay.offer(key, paths)
}

// OpenDownloads shows the Downloads folder in the file manager.
func (s *Service) OpenDownloads() error {
	dir, err := downloadsDir()
	if err != nil {
		return err
	}
	cmd := exec.Command("xdg-open", dir)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
