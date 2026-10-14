package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// MaxEditSize is the largest file that can be opened for editing.
const MaxEditSize = 10 << 20

// EventEdited tells the frontend an edited copy was saved (an EditEvent).
const EventEdited = "files:edited"

// ErrChangedOnServer is returned by SaveEdit when the original changed since
// it was opened; saving again with force overwrites it.
var ErrChangedOnServer = errors.New("the file changed on the server since you opened it")

// Edit is a file opened in a desktop app. The local copy lives in a private
// folder until the edit is closed or the app quits.
type Edit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Path is the original file, on the host or this computer.
	Path string `json:"path"`
	// Changed is true when the copy was saved and not uploaded yet.
	Changed bool `json:"changed"`
}

// EditEvent reports an edit whose copy changed.
type EditEvent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type editSession struct {
	Edit
	loc   Location
	local string
	dir   string
	// orig is the original's size and time when opened or last uploaded.
	orig Stat
	// seen is the copy's modification time last reported.
	seen time.Time
	stop chan struct{}
}

type editor struct {
	mu    sync.Mutex
	edits map[string]*editSession
	seq   int
	// open shows a file in the desktop's default app.
	open func(path string) error
	emit func(name string, data any)
	poll time.Duration
}

func newEditor() *editor {
	return &editor{edits: map[string]*editSession{}, open: xdgOpen, poll: time.Second}
}

func xdgOpen(path string) error {
	cmd := exec.Command("xdg-open", path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("couldn't open an editor: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// UseEvents makes s report saved edits through emit. It's a function rather
// than a method so it isn't bound to the frontend.
func UseEvents(s *Service, emit func(name string, data any)) { s.edit.emit = emit }

// editDir is a new private folder for one edit.
func editDir() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	base = filepath.Join(base, "tuxshell")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(base, "edit-")
	if err != nil {
		return "", err
	}
	return dir, os.Chmod(dir, 0o700)
}

// Edit copies a file to this computer and opens it in the default app for
// its type. Saves are reported with EventEdited; SaveEdit uploads them.
func (s *Service) Edit(ctx context.Context, loc Location, file string) (Edit, error) {
	b, err := s.backend(loc)
	if err != nil {
		return Edit{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	st, err := b.stat(ctx, file)
	if err != nil {
		return Edit{}, err
	}
	if st.Size > MaxEditSize {
		return Edit{}, fmt.Errorf("files over %d MB can't be edited here; download it instead", MaxEditSize>>20)
	}
	dir, err := editDir()
	if err != nil {
		return Edit{}, err
	}
	name := safeName(filepath.Base(file))
	local := filepath.Join(dir, name)
	f, err := os.OpenFile(local, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		err = b.download(ctx, file, f)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return Edit{}, err
	}
	info, err := os.Stat(local)
	if err != nil {
		_ = os.RemoveAll(dir)
		return Edit{}, err
	}

	e := s.edit
	e.mu.Lock()
	e.seq++
	sess := &editSession{
		Edit: Edit{ID: strconv.Itoa(e.seq), Name: name, Path: file},
		loc:  loc, local: local, dir: dir, orig: st, seen: info.ModTime(), stop: make(chan struct{}),
	}
	e.edits[sess.ID] = sess
	e.mu.Unlock()
	go e.watch(sess)

	if err := e.open(local); err != nil {
		e.close(sess.ID)
		return Edit{}, err
	}
	return sess.Edit, nil
}

// watch reports each save of the local copy once.
func (e *editor) watch(sess *editSession) {
	t := time.NewTicker(e.poll)
	defer t.Stop()
	for {
		select {
		case <-sess.stop:
			return
		case <-t.C:
		}
		info, err := os.Stat(sess.local)
		if err != nil {
			continue
		}
		e.mu.Lock()
		changed := !info.ModTime().Equal(sess.seen)
		if changed {
			sess.seen, sess.Changed = info.ModTime(), true
		}
		emit := e.emit
		e.mu.Unlock()
		if changed && emit != nil {
			emit(EventEdited, EditEvent{ID: sess.ID, Name: sess.Name})
		}
	}
}

func (e *editor) get(id string) (*editSession, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	sess, ok := e.edits[id]
	if !ok {
		return nil, errors.New("this edit has been closed")
	}
	return sess, nil
}

// SaveEdit uploads the edited copy over the original. Unless force is set,
// it refuses with ErrChangedOnServer when the original changed meanwhile.
func (s *Service) SaveEdit(ctx context.Context, id string, force bool) error {
	sess, err := s.edit.get(id)
	if err != nil {
		return err
	}
	b, err := s.backend(sess.loc)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	now, err := b.stat(ctx, sess.Path)
	if err != nil {
		return err
	}
	s.edit.mu.Lock()
	orig := sess.orig
	s.edit.mu.Unlock()
	if !force && (now.Size != orig.Size || now.ModTime != orig.ModTime) {
		return ErrChangedOnServer
	}
	f, err := os.Open(sess.local)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := b.replace(ctx, sess.Path, f); err != nil {
		return err
	}
	after, err := b.stat(ctx, sess.Path)
	if err != nil {
		return err
	}
	s.edit.mu.Lock()
	sess.orig, sess.Changed = after, false
	s.edit.mu.Unlock()
	return nil
}

// Edits lists the open edits.
func (s *Service) Edits() []Edit {
	s.edit.mu.Lock()
	defer s.edit.mu.Unlock()
	list := []Edit{}
	for _, sess := range s.edit.edits {
		list = append(list, sess.Edit)
	}
	return list
}

// CloseEdit stops watching an edit and deletes its local copy. Unsaved
// changes in the copy are lost.
func (s *Service) CloseEdit(id string) { s.edit.close(id) }

func (e *editor) close(id string) {
	e.mu.Lock()
	sess, ok := e.edits[id]
	delete(e.edits, id)
	e.mu.Unlock()
	if ok {
		close(sess.stop)
		_ = os.RemoveAll(sess.dir)
	}
}

// ServiceShutdown deletes every edit's local copy when the app quits.
func (s *Service) ServiceShutdown() error {
	s.edit.mu.Lock()
	ids := make([]string, 0, len(s.edit.edits))
	for id := range s.edit.edits {
		ids = append(ids, id)
	}
	s.edit.mu.Unlock()
	for _, id := range ids {
		s.edit.close(id)
	}
	return nil
}
