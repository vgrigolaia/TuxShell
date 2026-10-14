package login

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/vgrigolaia/tuxshell/internal/askpass"
)

// The test binary doubles as the askpass helper, like the TuxShell binary.
func TestMain(m *testing.M) {
	if askpass.IsHelper() {
		os.Exit(askpass.Helper(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// TestRealSSHPrompts runs the real ssh against a throwaway sshd: the host key
// question and a key passphrase go through the helper to the terminal, the
// passphrase is never echoed, and the login works. Opt-in with
// TUXSHELL_IT_SSHD=1 (needs ssh, ssh-keygen and /usr/sbin/sshd).
func TestRealSSHPrompts(t *testing.T) {
	if os.Getenv("TUXSHELL_IT_SSHD") == "" {
		t.Skip("set TUXSHELL_IT_SSHD=1 to run against a real sshd")
	}
	dir := t.TempDir()
	const passphrase = "hunter2-pass"
	run := func(name string, args ...string) {
		if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
	}
	run("ssh-keygen", "-q", "-t", "ed25519", "-N", passphrase, "-f", dir+"/client")
	run("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", dir+"/host")
	run("cp", dir+"/client.pub", dir+"/authorized_keys")
	port := freePort(t)
	conf := "Port " + port + "\nListenAddress 127.0.0.1\nHostKey " + dir + "/host\nAuthorizedKeysFile " + dir +
		"/authorized_keys\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nStrictModes no\nPidFile " + dir + "/pid\n"
	if err := os.WriteFile(dir+"/sshd_config", []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	sshd := exec.Command("/usr/sbin/sshd", "-D", "-f", dir+"/sshd_config", "-E", dir+"/sshd.log")
	if err := sshd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sshd.Process.Kill(); _ = sshd.Wait() })
	waitPort(t, port)

	f := setup(t)
	f.svc.exe, _ = os.Executable()
	env, started, end := f.svc.Begin("s1", f.host.ID)
	defer end()
	cmd := exec.Command("ssh", "-F", "/dev/null", "-p", port, "-i", dir+"/client", "-o", "IdentitiesOnly=yes",
		"-o", "UserKnownHostsFile="+filepath.Join(dir, "known_hosts"), "-o", "StrictHostKeyChecking=ask",
		"-tt", "127.0.0.1", "echo LOGGED_IN")
	cmd.Env = append(os.Environ(), env...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	started(cmd.Process.Pid)

	chunks := make(chan string)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunks <- string(buf[:n])
			}
			if err != nil {
				close(chunks)
				return
			}
		}
	}()
	var out strings.Builder
	timeout := time.After(20 * time.Second)
	for done := false; !done; {
		select {
		case s, ok := <-chunks:
			if !ok {
				done = true
				break
			}
			out.WriteString(s)
			switch {
			case strings.Contains(s, "(yes/no"):
				_, _ = ptmx.WriteString("yes\n")
			case strings.Contains(s, "Enter passphrase"):
				_, _ = ptmx.WriteString(passphrase + "\n")
			}
		case <-timeout:
			t.Fatalf("timed out; output:\n%s", out.String())
		}
	}
	got := out.String()
	if !strings.Contains(got, "LOGGED_IN") || !strings.Contains(got, "Enter passphrase") || !strings.Contains(got, "(yes/no") {
		t.Fatalf("unexpected output:\n%s", got)
	}
	if strings.Contains(got, passphrase) {
		t.Fatal("the passphrase was echoed")
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

func waitPort(t *testing.T, port string) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if c, err := net.Dial("tcp", "127.0.0.1:"+port); err == nil {
			c.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("sshd didn't start")
}
