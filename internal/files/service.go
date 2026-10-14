package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/vgrigolaia/tuxshell/internal/sshcmd"
)

// Location says whose files to browse: a pane's saved host (with its login
// user), an ad-hoc target, or the local machine.
type Location struct {
	// Kind is "local", "host" or "target".
	Kind   string `json:"kind"`
	HostID int64  `json:"hostId"`
	User   string `json:"user"`
	Target string `json:"target"`
}

// Remotes builds batch ssh runners; satisfied by sshcmd.Resolver.
type Remotes interface {
	HostRemote(id int64, user string) (sshcmd.Remote, error)
	TargetRemote(target string) (sshcmd.Remote, error)
}

// DropEvent tells the frontend that files were dropped on the panel.
type DropEvent struct {
	ID    string   `json:"id"`
	Names []string `json:"names"`
}

// EventDropped is emitted when files are dropped onto the file panel.
const EventDropped = "files:dropped"

// UploadResult reports what was uploaded and what was skipped.
type UploadResult struct {
	Uploaded []string `json:"uploaded"`
	Failed   []string `json:"failed"`
}

// Service exposes file browsing to the frontend.
type Service struct {
	remotes Remotes
	// pick opens a native multi-file picker; nil disables "Upload…".
	pick func() ([]string, error)

	mu      sync.Mutex
	drops   map[string][]string
	dropSeq atomic.Uint64

	edit *editor
}

// NewService browses through remotes; pick opens the upload file picker.
// It also returns the function the window's drop handler calls. That stays
// in Go: it isn't a bound method, so the page can never register paths.
func NewService(remotes Remotes, pick func() ([]string, error)) (*Service, func(paths []string) DropEvent) {
	s := &Service{remotes: remotes, pick: pick, drops: map[string][]string{}, edit: newEditor()}
	return s, s.dropped
}

func (s *Service) backend(loc Location) (backend, error) {
	switch loc.Kind {
	case "local":
		return local{}, nil
	case "host":
		r, err := s.remotes.HostRemote(loc.HostID, loc.User)
		if err != nil {
			return nil, err
		}
		return remote{r}, nil
	case "target":
		r, err := s.remotes.TargetRemote(loc.Target)
		if err != nil {
			return nil, err
		}
		return remote{r}, nil
	}
	return nil, fmt.Errorf("unknown location %q", loc.Kind)
}

// List returns a directory's contents; an empty dir means the home folder.
func (s *Service) List(ctx context.Context, loc Location, dir string) (Listing, error) {
	b, err := s.backend(loc)
	if err != nil {
		return Listing{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	return b.list(ctx, dir)
}

// Rename renames dir/from to dir/to; it never replaces an existing item.
func (s *Service) Rename(ctx context.Context, loc Location, dir, from, to string) error {
	if err := checkName(from); err != nil {
		return err
	}
	if err := checkName(to); err != nil {
		return err
	}
	return s.do(ctx, loc, dir, func(ctx context.Context, b backend) error { return b.rename(ctx, dir, from, to) })
}

// MakeDir creates the folder dir/name.
func (s *Service) MakeDir(ctx context.Context, loc Location, dir, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	return s.do(ctx, loc, dir, func(ctx context.Context, b backend) error { return b.mkdir(ctx, dir, name) })
}

// Delete removes items in dir, folders with everything in them.
func (s *Service) Delete(ctx context.Context, loc Location, dir string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	for _, n := range names {
		if err := checkName(n); err != nil {
			return err
		}
	}
	return s.do(ctx, loc, dir, func(ctx context.Context, b backend) error { return b.remove(ctx, dir, names) })
}

// Chmod sets dir/name's permissions (octal, e.g. "755"), and with recursive
// everything inside a folder too.
func (s *Service) Chmod(ctx context.Context, loc Location, dir, name, mode string, recursive bool) error {
	if err := checkName(name); err != nil {
		return err
	}
	m, err := parseMode(mode)
	if err != nil {
		return err
	}
	return s.do(ctx, loc, dir, func(ctx context.Context, b backend) error { return b.chmod(ctx, dir, name, m, recursive) })
}

// do runs one change with the transfer timeout (a recursive delete may be slow).
func (s *Service) do(ctx context.Context, loc Location, dir string, fn func(context.Context, backend) error) error {
	// Changes happen in a folder the panel listed, never relative to
	// wherever the app or the ssh login happens to start.
	if !strings.HasPrefix(dir, "/") || strings.ContainsRune(dir, 0) {
		return errors.New("not a folder path")
	}
	b, err := s.backend(loc)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	return fn(ctx, b)
}

// Download saves a file into the Downloads folder and returns its local path.
func (s *Service) Download(ctx context.Context, loc Location, file string) (string, error) {
	b, err := s.backend(loc)
	if err != nil {
		return "", err
	}
	dir, err := downloadsDir()
	if err != nil {
		return "", err
	}
	f, err := createUnique(dir, safeName(filepath.Base(file)))
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	if err := b.download(ctx, file, f); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), f.Close()
}

// UploadPick asks for local files with a native picker and uploads them to dir.
func (s *Service) UploadPick(ctx context.Context, loc Location, dir string) (UploadResult, error) {
	if s.pick == nil {
		return UploadResult{}, errors.New("file picker unavailable")
	}
	paths, err := s.pick()
	if err != nil || len(paths) == 0 {
		return UploadResult{Uploaded: []string{}, Failed: []string{}}, err
	}
	return s.upload(ctx, loc, dir, paths)
}

// UploadDropped uploads a batch of files dropped onto the panel.
func (s *Service) UploadDropped(ctx context.Context, id string, loc Location, dir string) (UploadResult, error) {
	s.mu.Lock()
	paths, ok := s.drops[id]
	delete(s.drops, id)
	s.mu.Unlock()
	if !ok {
		return UploadResult{}, errors.New("those dropped files are no longer available; drop them again")
	}
	return s.upload(ctx, loc, dir, paths)
}

// dropped records files dropped onto the window and returns the event that
// lets the frontend upload them by ID, so local paths never round-trip
// through the page.
func (s *Service) dropped(paths []string) DropEvent {
	id := strconv.FormatUint(s.dropSeq.Add(1), 10)
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	s.mu.Lock()
	s.drops = map[string][]string{id: paths} // only the latest drop is kept
	s.mu.Unlock()
	return DropEvent{ID: id, Names: names}
}

func (s *Service) upload(ctx context.Context, loc Location, dir string, paths []string) (UploadResult, error) {
	b, err := s.backend(loc)
	if err != nil {
		return UploadResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	res := UploadResult{Uploaded: []string{}, Failed: []string{}}
	for _, p := range paths {
		name := filepath.Base(p)
		if err := uploadOne(ctx, b, dir, name, p); err != nil {
			if errors.Is(err, ErrNotConnected) {
				return res, err
			}
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		res.Uploaded = append(res.Uploaded, name)
	}
	return res, nil
}

func uploadOne(ctx context.Context, b backend, dir, name, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("only regular files can be uploaded")
	}
	return b.upload(ctx, dir, name, f)
}
