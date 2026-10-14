// Package askpass lets TuxShell answer ssh's prompts without the answer ever
// appearing in ssh's arguments, environment or the terminal.
//
// ssh is started with SSH_ASKPASS pointing at the TuxShell binary and
// SSH_ASKPASS_REQUIRE=force. For every prompt ssh runs that binary with the
// prompt as its argument; the binary sees EnvSocket and runs Helper instead of
// the app. The helper sends the prompt and the session's token over a private
// Unix socket. The app decides: answer it, cancel it, or let the helper ask on
// the terminal as ssh would have (host keys, key passphrases, jump hosts).
package askpass

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Environment variables the helper reads.
const (
	EnvSocket = "TUXSHELL_ASKPASS_SOCKET"
	EnvToken  = "TUXSHELL_ASKPASS_TOKEN"
)

// Actions in a Reply.
const (
	ActionAnswer = "answer"
	ActionCancel = "cancel"
	// ActionTTY asks on the terminal, as ssh does without askpass.
	ActionTTY = "tty"
)

// Request is one prompt from ssh.
type Request struct {
	Token  string `json:"token"`
	Prompt string `json:"prompt"`
	// ParentPID is the process that ran the helper (the ssh asking). The
	// server fills it in from the kernel; what the helper sends is ignored.
	ParentPID int `json:"-"`
}

// Reply tells the helper what to do.
type Reply struct {
	Action string `json:"action"`
	Answer string `json:"answer,omitempty"`
}

// Handler decides how to answer a request. It may block (e.g. while the user
// types a password); the helper waits.
type Handler func(Request) Reply

// Env returns the environment that makes ssh ask exe for its prompts.
func Env(exe, socket, token string) []string {
	return []string{
		"SSH_ASKPASS=" + exe,
		"SSH_ASKPASS_REQUIRE=force",
		EnvSocket + "=" + socket,
		EnvToken + "=" + token,
	}
}

// Server listens for helpers on a socket only this user can reach.
type Server struct {
	ln     *net.UnixListener
	path   string
	handle Handler
	wg     sync.WaitGroup
}

// Listen starts a server in dir, which must be private to this user (the ssh
// control socket directory is).
func Listen(dir string, h Handler) (*Server, error) {
	path := filepath.Join(dir, "askpass-"+strconv.Itoa(os.Getpid())+".sock")
	_ = os.Remove(path)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	s := &Server{ln: ln, path: path, handle: h}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

// Path is the socket for EnvSocket.
func (s *Server) Path() string { return s.path }

// Close stops accepting helpers and removes the socket.
func (s *Server) Close() error {
	err := s.ln.Close()
	s.wg.Wait()
	_ = os.Remove(s.path)
	return err
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.AcceptUnix()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn *net.UnixConn) {
	defer conn.Close()
	pid, ok := peer(conn)
	if !ok {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReaderSize(conn, 64*1024).ReadBytes('\n')
	if err != nil {
		return
	}
	var req Request
	if json.Unmarshal(line, &req) != nil {
		return
	}
	req.ParentPID = parentPID(pid)
	reply := s.handle(req)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	b, _ := json.Marshal(reply)
	_, _ = conn.Write(append(b, '\n'))
}

// peer returns the connecting process's PID, and false unless it runs as us.
func peer(conn *net.UnixConn) (int, bool) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, false
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil || credErr != nil {
		return 0, false
	}
	return int(cred.Pid), int(cred.Uid) == os.Getuid()
}

// parentPID reads a process's parent from /proc, or 0 if it can't.
func parentPID(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	// The command name in parentheses may contain spaces; fields follow the last ")".
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(f[1])
	return n
}

// ask sends one prompt to the server.
func ask(socket, token, prompt string) (Reply, error) {
	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return Reply{}, err
	}
	defer conn.Close()
	b, _ := json.Marshal(Request{Token: token, Prompt: prompt})
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return Reply{}, err
	}
	// The user may take a while to type a password.
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Minute))
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return Reply{}, err
	}
	var r Reply
	if err := json.Unmarshal(line, &r); err != nil {
		return Reply{}, err
	}
	return r, nil
}

// IsHelper reports whether this process was started by ssh as its askpass.
func IsHelper() bool { return os.Getenv(EnvSocket) != "" }

// Helper runs as ssh's askpass and returns the exit code. args are the
// command-line arguments after the program name (ssh passes the prompt).
func Helper(args []string) int {
	prompt := strings.Join(args, " ")
	switch os.Getenv("SSH_ASKPASS_PROMPT") {
	case "none":
		// A notice such as "touch your security key"; ssh kills us when done.
		_ = toTTY(prompt + "\n")
		select {}
	case "confirm":
		answer, err := readTTY(prompt+" (yes/no) ", false)
		if err != nil || !strings.HasPrefix(strings.ToLower(answer), "y") {
			return 1
		}
		return 0
	}
	reply, err := ask(os.Getenv(EnvSocket), os.Getenv(EnvToken), prompt)
	if err != nil {
		reply = Reply{Action: ActionTTY}
	}
	switch reply.Action {
	case ActionAnswer:
		fmt.Println(reply.Answer)
		return 0
	case ActionCancel:
		return 1
	}
	// Host key questions are answered visibly; passwords and passphrases aren't.
	answer, err := readTTY(prompt, strings.Contains(prompt, "(yes/no"))
	if err != nil {
		return 1
	}
	fmt.Println(answer)
	return 0
}

func toTTY(s string) error {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer tty.Close()
	_, err = tty.WriteString(s)
	return err
}

// readTTY asks on the controlling terminal (the TuxShell pane), hiding the
// typed text unless echo is set.
func readTTY(prompt string, echo bool) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	defer tty.Close()
	if _, err := tty.WriteString(prompt); err != nil {
		return "", err
	}
	if !echo {
		if restore, err := noEcho(int(tty.Fd())); err == nil {
			defer func() {
				restore()
				_, _ = tty.WriteString("\r\n")
			}()
		}
	}
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// noEcho turns off terminal echo and returns a function that restores it.
func noEcho(fd int) (func(), error) {
	var old syscall.Termios
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCGETS, uintptrOf(&old)); e != 0 {
		return nil, e
	}
	t := old
	t.Lflag &^= syscall.ECHO
	t.Lflag |= syscall.ICANON
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCSETS, uintptrOf(&t)); e != 0 {
		return nil, e
	}
	return func() {
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCSETS, uintptrOf(&old))
	}, nil
}
