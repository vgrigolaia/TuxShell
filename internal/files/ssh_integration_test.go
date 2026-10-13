package files

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// argsRunner runs scripts over a real ssh with fixed arguments.
type argsRunner struct{ args []string }

func (r argsRunner) Command(ctx context.Context, script string) *exec.Cmd {
	return exec.CommandContext(ctx, "ssh", append(append([]string{}, r.args...), script)...)
}

// TestOverRealSSH exercises the scripts through a real sshd. Opt-in:
// TUXSHELL_IT_SSH holds the ssh arguments (options and destination) that
// reach a test server, e.g. through a ControlMaster socket.
func TestOverRealSSH(t *testing.T) {
	spec := os.Getenv("TUXSHELL_IT_SSH")
	if spec == "" {
		t.Skip("set TUXSHELL_IT_SSH to run against a real ssh server")
	}
	dir := populate(t)
	b := remote{argsRunner{strings.Fields(spec)}}
	ctx := context.Background()

	l, err := b.list(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	checkListing(t, l, dir)

	var buf bytes.Buffer
	if err := b.download(ctx, filepath.Join(dir, "with space.log"), &buf); err != nil || buf.String() != "data:with space.log" {
		t.Fatalf("download = %q, %v", buf.String(), err)
	}
	big := bytes.Repeat([]byte("0123456789abcdef"), 1<<16) // 1 MiB, binary-safe check
	big[100] = 0
	if err := b.upload(ctx, dir, "big.bin", bytes.NewReader(big)); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "big.bin")); !bytes.Equal(got, big) {
		t.Fatalf("uploaded %d bytes, want %d", len(got), len(big))
	}
	if home, err := b.list(ctx, ""); err != nil || home.Path == "" {
		t.Fatalf("home listing = %+v, %v", home, err)
	}
}
