// Package files lists, downloads and uploads files on a pane's host: over the
// pane's shared ssh connection for remote hosts, or directly for local shells.
package files

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// MaxEntries caps a listing; huge directories are truncated.
const MaxEntries = 5000

// Entry is one item in a directory.
type Entry struct {
	Name string `json:"name"`
	// Type is "dir", "file", "link" or "other".
	Type string `json:"type"`
	// LinkDir is true for symlinks that point at a directory.
	LinkDir bool   `json:"linkDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"` // Unix milliseconds
	Mode    string `json:"mode"`    // octal permission bits, e.g. "755"
}

// Listing is a directory's resolved path and contents.
type Listing struct {
	Path      string  `json:"path"`
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}

// backend is a filesystem reachable from a pane.
type backend interface {
	list(ctx context.Context, dir string) (Listing, error)
	download(ctx context.Context, file string, w io.Writer) error
	// upload creates dir/name from r, refusing to overwrite.
	upload(ctx context.Context, dir, name string, r io.Reader) error
	rename(ctx context.Context, dir, from, to string) error
	mkdir(ctx context.Context, dir, name string) error
	remove(ctx context.Context, dir string, names []string) error
	chmod(ctx context.Context, dir, name string, mode uint32, recursive bool) error
	stat(ctx context.Context, file string) (Stat, error)
	// replace overwrites a regular file with r, keeping its permissions.
	replace(ctx context.Context, file string, r io.Reader) error
}

// ErrNotConnected means no shared ssh connection exists and the host
// can't be reached non-interactively.
var ErrNotConnected = errors.New("not connected: open a terminal to this host first (password logins need an open session)")

// quote makes s a single shell word.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func checkPath(p string) error {
	if strings.ContainsRune(p, 0) {
		return errors.New("path contains a NUL byte")
	}
	return nil
}

// runner starts a remote script; satisfied by sshcmd.Remote.
type runner interface {
	Command(ctx context.Context, script string) *exec.Cmd
}

type remote struct{ r runner }

func (b remote) run(ctx context.Context, script string, stdin io.Reader, stdout io.Writer) error {
	cmd := b.r.Command(ctx, script)
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(stderr.String())
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 255 {
		// 255 is ssh's own failure (connection or auth), not the script's.
		if msg == "" || strings.Contains(msg, "Permission denied") || strings.Contains(msg, "Host key verification failed") {
			return ErrNotConnected
		}
		return fmt.Errorf("%w (%s)", ErrNotConnected, lastLine(msg))
	}
	if msg != "" {
		return errors.New(lastLine(msg))
	}
	return err
}

// shellNoise matches the "bash: line 1: cd: " / "sh: 1: cd: " prefixes shells put on errors.
var shellNoise = regexp.MustCompile(`^(?:\S+: )?(?:(?:line )?\d+: )?(?:cd: )?`)

// lastLine returns the last stderr line without the shell's prefix.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return shellNoise.ReplaceAllString(strings.TrimSpace(lines[len(lines)-1]), "")
}

// listScript prints the resolved directory, a NUL, then one NUL-terminated
// record per entry. It needs GNU find (all supported server distros have it).
func listScript(dir string) string {
	cd := "cd"
	switch {
	case dir == "" || dir == "~":
	case strings.HasPrefix(dir, "~/"):
		// A path from the terminal's prompt, relative to the home folder.
		cd = `cd -- "$HOME"/` + quote(dir[2:])
	default:
		cd = "cd -- " + quote(dir)
	}
	return cd + ` || exit 2
pwd -P
printf '\0'
find . -mindepth 1 -maxdepth 1 -printf '%y\t%Y\t%s\t%T@\t%m\t%P\0' 2>/dev/null | head -z -n ` + strconv.Itoa(MaxEntries+1)
}

func (b remote) list(ctx context.Context, dir string) (Listing, error) {
	if err := checkPath(dir); err != nil {
		return Listing{}, err
	}
	var out bytes.Buffer
	if err := b.run(ctx, listScript(dir), nil, &out); err != nil {
		return Listing{}, err
	}
	return parseListing(out.Bytes())
}

func parseListing(out []byte) (Listing, error) {
	head, rest, ok := bytes.Cut(out, []byte{0})
	if !ok {
		return Listing{}, errors.New("unexpected listing output (is GNU find installed on the host?)")
	}
	l := Listing{Path: strings.TrimRight(string(head), "\n"), Entries: []Entry{}}
	sc := bufio.NewScanner(bytes.NewReader(rest))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	sc.Split(splitNUL)
	for sc.Scan() {
		f := strings.SplitN(sc.Text(), "\t", 6)
		if len(f) != 6 || f[5] == "" {
			continue
		}
		if len(l.Entries) == MaxEntries {
			l.Truncated = true
			break
		}
		size, _ := strconv.ParseInt(f[2], 10, 64)
		secs, _ := strconv.ParseFloat(f[3], 64)
		e := Entry{Name: f[5], Size: size, ModTime: int64(secs * 1000), Mode: f[4]}
		switch f[0] {
		case "d":
			e.Type = "dir"
		case "f":
			e.Type = "file"
		case "l":
			e.Type = "link"
			e.LinkDir = f[1] == "d"
		default:
			e.Type = "other"
		}
		l.Entries = append(l.Entries, e)
	}
	sortEntries(l.Entries)
	return l, sc.Err()
}

func splitNUL(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// sortEntries puts folders (and links to folders) first, then names A–Z.
func sortEntries(es []Entry) {
	sort.SliceStable(es, func(i, j int) bool {
		di, dj := es[i].Type == "dir" || es[i].LinkDir, es[j].Type == "dir" || es[j].LinkDir
		if di != dj {
			return di
		}
		return strings.ToLower(es[i].Name) < strings.ToLower(es[j].Name)
	})
}

func (b remote) download(ctx context.Context, file string, w io.Writer) error {
	if err := checkPath(file); err != nil {
		return err
	}
	q := quote(file)
	return b.run(ctx, `[ -f `+q+` ] || { echo "not a regular file" >&2; exit 3; }
exec cat -- `+q, nil, w)
}

func (b remote) upload(ctx context.Context, dir, name string, r io.Reader) error {
	if err := checkPath(dir); err != nil {
		return err
	}
	// noclobber (set -C) makes ">" fail instead of overwriting.
	return b.run(ctx, `cd -- `+quote(dir)+` || exit 2
set -C
exec cat > `+quote(name), r, nil)
}

type local struct{}

func (local) list(_ context.Context, dir string) (Listing, error) {
	if dir == "" || dir == "~" || strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Listing{}, err
		}
		dir = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(dir, "~"), "/"))
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Listing{}, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return Listing{}, err
	}
	l := Listing{Path: abs, Entries: []Entry{}}
	for _, de := range des {
		if len(l.Entries) == MaxEntries {
			l.Truncated = true
			break
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		e := Entry{Name: de.Name(), Size: info.Size(), ModTime: info.ModTime().UnixMilli(), Mode: strconv.FormatUint(uint64(info.Mode().Perm()), 8)}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			e.Type = "link"
			if t, err := os.Stat(filepath.Join(abs, de.Name())); err == nil && t.IsDir() {
				e.LinkDir = true
			}
		case info.IsDir():
			e.Type = "dir"
		case info.Mode().IsRegular():
			e.Type = "file"
		default:
			e.Type = "other"
		}
		l.Entries = append(l.Entries, e)
	}
	sortEntries(l.Entries)
	return l, nil
}

func (local) download(_ context.Context, file string, w io.Writer) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	_, err = io.Copy(w, f)
	return err
}

func (local) upload(_ context.Context, dir, name string, r io.Reader) error {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	return f.Close()
}

// safeName turns a remote file name into a safe local base name.
func safeName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	return name
}

// createUnique creates dir/name, or "name (1).ext" etc. when it exists.
func createUnique(dir, name string) (*os.File, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < 1000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		f, err := os.OpenFile(filepath.Join(dir, candidate), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
	}
	return nil, errors.New("too many files with this name in Downloads")
}

// downloadsDir is the XDG download directory, falling back to ~/Downloads.
func downloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if out, err := exec.Command("xdg-user-dir", "DOWNLOAD").Output(); err == nil {
		// xdg-user-dir answers $HOME when no Downloads folder is configured;
		// files shouldn't land loose in the home folder.
		if dir := strings.TrimSpace(string(out)); dir != "" && filepath.IsAbs(dir) && filepath.Clean(dir) != filepath.Clean(home) {
			return dir, os.MkdirAll(dir, 0o755)
		}
	}
	dir := filepath.Join(home, "Downloads")
	return dir, os.MkdirAll(dir, 0o755)
}

// timeouts for listing (quick) and transfers (may be large).
const (
	listTimeout     = 20 * time.Second
	transferTimeout = 2 * time.Hour
)
