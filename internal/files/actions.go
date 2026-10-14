package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Stat is what editing needs to know about a file.
type Stat struct {
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"` // Unix seconds
	Mode    string `json:"mode"`
}

// ErrExists is returned when a rename or new folder would replace something.
var ErrExists = errors.New("something with that name is already there")

// checkName accepts a single file name: no slashes, not "." or "..".
func checkName(name string) error {
	switch {
	case name == "" || strings.TrimSpace(name) == "":
		return errors.New("the name is empty")
	case name == "." || name == "..":
		return errors.New(`"." and ".." can't be used as names`)
	case strings.ContainsAny(name, "/\x00"):
		return errors.New("names can't contain /")
	case len(name) > 255:
		return errors.New("the name is too long")
	}
	return nil
}

// parseMode accepts octal permissions from 0 to 777.
func parseMode(mode string) (uint32, error) {
	if len(mode) < 1 || len(mode) > 4 {
		return 0, errors.New("permissions must be an octal number like 755")
	}
	n, err := strconv.ParseUint(mode, 8, 32)
	if err != nil || n > 0o777 {
		return 0, errors.New("permissions must be an octal number like 755")
	}
	return uint32(n), nil
}

// The shell's "cd -- dir || exit 2" with checks on dir.
func cdScript(dir string) (string, error) {
	if err := checkPath(dir); err != nil {
		return "", err
	}
	if dir == "" {
		return "", errors.New("no folder")
	}
	return "cd -- " + quote(dir) + " || exit 2\n", nil
}

func (b remote) rename(ctx context.Context, dir, from, to string) error {
	cd, err := cdScript(dir)
	if err != nil {
		return err
	}
	f, t := quote(from), quote(to)
	return mapExists(b.run(ctx, cd+`if [ -e `+t+` ] || [ -L `+t+` ]; then echo exists >&2; exit 17; fi
exec mv -- `+f+` `+t, nil, nil))
}

func (b remote) mkdir(ctx context.Context, dir, name string) error {
	cd, err := cdScript(dir)
	if err != nil {
		return err
	}
	n := quote(name)
	return mapExists(b.run(ctx, cd+`if [ -e `+n+` ] || [ -L `+n+` ]; then echo exists >&2; exit 17; fi
exec mkdir -- `+n, nil, nil))
}

func (b remote) remove(ctx context.Context, dir string, names []string) error {
	cd, err := cdScript(dir)
	if err != nil {
		return err
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = quote(n)
	}
	return b.run(ctx, cd+"exec rm -rf -- "+strings.Join(q, " "), nil, nil)
}

func (b remote) chmod(ctx context.Context, dir, name string, mode uint32, recursive bool) error {
	cd, err := cdScript(dir)
	if err != nil {
		return err
	}
	flag := ""
	if recursive {
		flag = "-R "
	}
	return b.run(ctx, cd+fmt.Sprintf("exec chmod %s%o -- %s", flag, mode, quote(name)), nil, nil)
}

func (b remote) stat(ctx context.Context, file string) (Stat, error) {
	if err := checkPath(file); err != nil {
		return Stat{}, err
	}
	q := quote(file)
	var out strings.Builder
	if err := b.run(ctx, `[ -f `+q+` ] || { echo "not a regular file" >&2; exit 3; }
exec stat -L -c '%s %Y %a' -- `+q, nil, &out); err != nil {
		return Stat{}, err
	}
	return parseStat(out.String())
}

func parseStat(s string) (Stat, error) {
	f := strings.Fields(s)
	if len(f) != 3 {
		return Stat{}, errors.New("unexpected stat output (is GNU stat installed on the host?)")
	}
	size, err1 := strconv.ParseInt(f[0], 10, 64)
	mtime, err2 := strconv.ParseInt(f[1], 10, 64)
	if err1 != nil || err2 != nil {
		return Stat{}, errors.New("unexpected stat output")
	}
	return Stat{Size: size, ModTime: mtime, Mode: f[2]}, nil
}

// replaceScript writes stdin over file. Through a symlink it writes the
// target. The new content goes to a temporary file next to it first and is
// then moved over, keeping the permissions, so a dropped connection never
// leaves half a file. If the folder isn't writable it writes in place.
const replaceScript = `f=$(readlink -f -- "$1") || exit 2
[ -f "$f" ] || { echo "not a regular file" >&2; exit 3; }
cd -- "$(dirname -- "$f")" || exit 2
b=$(basename -- "$f")
t=".tuxshell-edit-$$"
if ( set -C; : > "$t" ) 2>/dev/null; then
  cat >> "$t" || { rm -f -- "$t"; exit 1; }
  chmod --reference="$b" -- "$t" 2>/dev/null
  exec mv -f -- "$t" "$b"
fi
exec cat > "$b"`

func (b remote) replace(ctx context.Context, file string, r io.Reader) error {
	if err := checkPath(file); err != nil {
		return err
	}
	// The path goes in as $1 of an inner sh, never into the script text.
	return b.run(ctx, "sh -c "+quote(replaceScript)+" sh "+quote(file), r, nil)
}

// mapExists turns the scripts' "exists" exit into ErrExists.
func mapExists(err error) error {
	if err != nil && err.Error() == "exists" {
		return ErrExists
	}
	return err
}

func (local) rename(_ context.Context, dir, from, to string) error {
	dst := filepath.Join(dir, to)
	if _, err := os.Lstat(dst); err == nil {
		return ErrExists
	}
	return os.Rename(filepath.Join(dir, from), dst)
}

func (local) mkdir(_ context.Context, dir, name string) error {
	err := os.Mkdir(filepath.Join(dir, name), 0o755)
	if errors.Is(err, os.ErrExist) {
		return ErrExists
	}
	return err
}

func (local) remove(_ context.Context, dir string, names []string) error {
	for _, n := range names {
		if err := os.RemoveAll(filepath.Join(dir, n)); err != nil {
			return err
		}
	}
	return nil
}

func (local) chmod(_ context.Context, dir, name string, mode uint32, recursive bool) error {
	root := filepath.Join(dir, name)
	if !recursive {
		return os.Chmod(root, os.FileMode(mode))
	}
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil // like chmod -R, links themselves are left alone
		}
		return os.Chmod(p, os.FileMode(mode))
	})
}

func (local) stat(_ context.Context, file string) (Stat, error) {
	info, err := os.Stat(file)
	if err != nil {
		return Stat{}, err
	}
	if !info.Mode().IsRegular() {
		return Stat{}, errors.New("not a regular file")
	}
	return Stat{Size: info.Size(), ModTime: info.ModTime().Unix(), Mode: strconv.FormatUint(uint64(info.Mode().Perm()), 8)}, nil
}

func (local) replace(_ context.Context, file string, r io.Reader) error {
	target, err := filepath.EvalSymlinks(file)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".tuxshell-edit-*")
	if err != nil {
		// Folder not writable: write in place.
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, r); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}
