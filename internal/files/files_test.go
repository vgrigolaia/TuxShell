package files

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vgrigolaia/tuxshell/internal/sshcmd"
)

// shRunner runs scripts with a local sh, the way ssh hands them to the
// remote login shell, so the real scripts and quoting are exercised.
type shRunner struct{}

func (shRunner) Command(ctx context.Context, script string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", script)
}

// Names that break naive quoting or parsing.
var tricky = []string{
	"plain.txt",
	"with space.log",
	"it's quoted",
	`$(touch PWNED)`,
	"semi;colon&amp",
	"tab\there",
	"new\nline",
	"-starts-with-dash",
}

func populate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range tricky {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("data:"+n), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub dir", filepath.Join(dir, "link to dir")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestQuoteSurvivesShell(t *testing.T) {
	for _, n := range tricky {
		out, err := exec.Command("sh", "-c", "printf %s "+quote(n)).Output()
		if err != nil || string(out) != n {
			t.Errorf("quote(%q) round trip = %q, %v", n, out, err)
		}
	}
}

func checkListing(t *testing.T, l Listing, dir string) {
	t.Helper()
	if want, _ := filepath.EvalSymlinks(dir); l.Path != want {
		t.Errorf("path = %q, want %q", l.Path, want)
	}
	byName := map[string]Entry{}
	for _, e := range l.Entries {
		byName[e.Name] = e
	}
	for _, n := range tricky {
		e, ok := byName[n]
		if !ok || e.Type != "file" || e.Size != int64(len("data:"+n)) || e.Mode != "640" {
			t.Errorf("entry %q = %+v (found %v)", n, e, ok)
		}
	}
	if e := byName["sub dir"]; e.Type != "dir" {
		t.Errorf("sub dir = %+v", e)
	}
	if e := byName["link to dir"]; e.Type != "link" || !e.LinkDir {
		t.Errorf("link to dir = %+v", e)
	}
	// Folders sort first.
	if l.Entries[0].Name != "link to dir" && l.Entries[0].Name != "sub dir" {
		t.Errorf("first entry = %q, want a folder", l.Entries[0].Name)
	}
}

func TestRemoteBackend(t *testing.T) {
	if !gnuHost() {
		t.Skip("the remote scripts target Linux servers with GNU tools")
	}
	dir := populate(t)
	b := remote{shRunner{}}
	ctx := context.Background()

	l, err := b.list(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	checkListing(t, l, dir)
	if _, err := os.Stat(filepath.Join(dir, "PWNED")); err == nil {
		t.Fatal("a file name was executed as a command")
	}

	var buf bytes.Buffer
	if err := b.download(ctx, filepath.Join(dir, "it's quoted"), &buf); err != nil || buf.String() != "data:it's quoted" {
		t.Fatalf("download = %q, %v", buf.String(), err)
	}
	if err := b.download(ctx, filepath.Join(dir, "sub dir"), &buf); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("download dir err = %v", err)
	}

	if err := b.upload(ctx, dir, "up 'loaded'.txt", strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "up 'loaded'.txt")); string(got) != "hello" {
		t.Fatalf("uploaded = %q", got)
	}
	// Never overwrite.
	if err := b.upload(ctx, dir, "plain.txt", strings.NewReader("clobber")); err == nil {
		t.Fatal("upload overwrote an existing file")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "plain.txt")); string(got) != "data:plain.txt" {
		t.Fatalf("existing file changed to %q", got)
	}

	if _, err := b.list(ctx, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("listing a missing folder succeeded")
	}
}

func TestLocalBackend(t *testing.T) {
	dir := populate(t)
	b := local{}
	ctx := context.Background()
	l, err := b.list(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	checkListing(t, l, dir)

	if err := b.upload(ctx, dir, "plain.txt", strings.NewReader("x")); err == nil {
		t.Fatal("local upload overwrote a file")
	}
	var buf bytes.Buffer
	if err := b.download(ctx, filepath.Join(dir, "new\nline"), &buf); err != nil || buf.String() != "data:new\nline" {
		t.Fatalf("local download = %q, %v", buf.String(), err)
	}
}

func TestSafeNameAndUnique(t *testing.T) {
	for in, want := range map[string]string{"../../etc/passwd": "passwd", "..": "download", "bad\x07bell": "bad_bell", "a\\b": "b", "": "download"} {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
	dir := t.TempDir()
	var names []string
	for range 3 {
		f, err := createUnique(dir, "report.tar.gz")
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, filepath.Base(f.Name()))
		f.Close()
	}
	if strings.Join(names, "|") != "report.tar.gz|report.tar (1).gz|report.tar (2).gz" {
		t.Fatalf("names = %q", names)
	}
}

type fakeRemotes struct{}

func (fakeRemotes) HostRemote(int64, string) (sshcmd.Remote, error) { return sshcmd.Remote{}, nil }
func (fakeRemotes) TargetRemote(string) (sshcmd.Remote, error)      { return sshcmd.Remote{}, nil }

func TestDropsAreSingleUseAndGoOnly(t *testing.T) {
	svc, record := NewService(fakeRemotes{}, nil)
	src := filepath.Join(t.TempDir(), "drop me.txt")
	os.WriteFile(src, []byte("dropped"), 0o644)
	dest := t.TempDir()

	ev := record([]string{src})
	if len(ev.Names) != 1 || ev.Names[0] != "drop me.txt" {
		t.Fatalf("event = %+v", ev)
	}
	res, err := svc.UploadDropped(context.Background(), ev.ID, Location{Kind: "local"}, dest)
	if err != nil || len(res.Uploaded) != 1 {
		t.Fatalf("upload dropped = %+v, %v", res, err)
	}
	if _, err := svc.UploadDropped(context.Background(), ev.ID, Location{Kind: "local"}, dest); err == nil {
		t.Fatal("a drop could be uploaded twice")
	}
}

func TestNotConnectedMessage(t *testing.T) {
	b := remote{exitRunner{code: 255, stderr: "user@10.0.0.1: Permission denied (publickey,password)."}}
	if _, err := b.list(context.Background(), ""); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("err = %v, want ErrNotConnected", err)
	}
}

type exitRunner struct {
	code   int
	stderr string
}

func (r exitRunner) Command(ctx context.Context, _ string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", `printf '%s\n' "$1" >&2; exit `+strconv.Itoa(r.code), "sh", r.stderr)
}

func TestErrorMessagesAreClean(t *testing.T) {
	for in, want := range map[string]string{
		"bash: line 1: cd: /srv/x: No such file or directory": "/srv/x: No such file or directory",
		"sh: 1: cd: can't cd to /root":                        "can't cd to /root",
		"cat: /etc/shadow: Permission denied":                 "/etc/shadow: Permission denied",
		"not a regular file":                                  "not a regular file",
	} {
		if got := lastLine(in); got != want {
			t.Errorf("lastLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDownloadsDirNeverHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No user-dirs config: xdg-user-dir (if installed) answers $HOME.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	dir, err := downloadsDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(home, "Downloads") {
		t.Fatalf("downloads dir = %q, want %q", dir, filepath.Join(home, "Downloads"))
	}
}
