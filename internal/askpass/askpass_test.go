package askpass

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

// The test binary doubles as the helper, like the TuxShell binary does.
func TestMain(m *testing.M) {
	if IsHelper() && os.Getenv("ASKPASS_TEST_HELPER") == "1" {
		os.Exit(Helper(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeSSH writes a script that asks its askpass once, the way ssh does (as
// a child process), and prints what it got. The prompt comes from
// FAKE_PROMPT, so quotes in it can't break the script.
func fakeSSH(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ssh")
	script := "#!/bin/sh\nanswer=$(\"$SSH_ASKPASS\" \"$FAKE_PROMPT\") || { echo CANCELLED; exit 1; }\necho \"GOT:$answer\"\n"
	if err := os.WriteFile(p, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func run(t *testing.T, s *Server, token, prompt string) (string, int) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fakeSSH(t))
	cmd.Env = append(os.Environ(), append(Env(exe, s.Path(), token), "ASKPASS_TEST_HELPER=1", "FAKE_PROMPT="+prompt)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // never read the developer's terminal
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)), cmd.Process.Pid
}

func TestAnswerAndCancel(t *testing.T) {
	var mu sync.Mutex
	var got []Request
	s, err := Listen(t.TempDir(), func(r Request) Reply {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
		if r.Token != "tok" {
			return Reply{Action: ActionCancel}
		}
		return Reply{Action: ActionAnswer, Answer: "p@ss word"}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if st, _ := os.Stat(s.Path()); st.Mode().Perm() != 0o600 {
		t.Errorf("socket mode = %v", st.Mode().Perm())
	}

	out, pid := run(t, s, "tok", "root@db's password: ")
	if out != "GOT:p@ss word" {
		t.Errorf("ssh got %q", out)
	}
	if len(got) != 1 || got[0].Prompt != "root@db's password: " || got[0].ParentPID != pid {
		t.Errorf("request = %+v, want parent %d", got, pid)
	}
	if out, _ := run(t, s, "wrong", "x: "); out != "CANCELLED" {
		t.Errorf("cancel: ssh got %q", out)
	}
}

// Without a server the helper falls back to the terminal. In its own session
// there is no terminal, so it fails, which ssh treats as a cancelled prompt.
func TestNoServer(t *testing.T) {
	exe, _ := os.Executable()
	cmd := exec.Command(fakeSSH(t))
	cmd.Env = append(os.Environ(), append(Env(exe, filepath.Join(t.TempDir(), "none.sock"), "t"), "ASKPASS_TEST_HELPER=1", "FAKE_PROMPT=pw: ")...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if out, _ := cmd.Output(); strings.TrimSpace(string(out)) != "CANCELLED" {
		t.Errorf("ssh got %q", out)
	}
}

func TestParentPID(t *testing.T) {
	if got := parentPID(os.Getpid()); got != os.Getppid() {
		t.Errorf("parentPID = %d, want %d", got, os.Getppid())
	}
	if parentPID(0) != 0 {
		t.Error("parentPID(0) should be 0")
	}
}
