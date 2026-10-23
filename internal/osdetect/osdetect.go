// Package osdetect finds a saved host's operating system when the user asks
// (the host editor's Detect button); nothing here runs on its own.
//
// With a terminal open to the host it reads /etc/os-release over that shared
// connection. Otherwise it reads the greeting every ssh server sends before
// login ("SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13"), which names the system
// only on some of them (Ubuntu, Debian, FreeBSD, Windows).
package osdetect

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/sshcmd"
)

// ReleaseCommand is the only command Detect runs on a host.
const ReleaseCommand = "cat /etc/os-release"

// Resolver is what Detect needs from sshcmd.Resolver.
type Resolver interface {
	ControlCommand(id int64, op string, f sshcmd.Forward) (*exec.Cmd, error)
	HostRemote(id int64, user string) (sshcmd.Remote, error)
	Endpoint(ctx context.Context, id int64) (sshcmd.Endpoint, error)
}

// Service is bound to the frontend.
type Service struct {
	resolver Resolver
}

// NewService returns a Service that resolves hosts with r.
func NewService(r Resolver) *Service {
	return &Service{resolver: r}
}

// Result is what Detect found.
type Result struct {
	// OS is one of hosts.OSes, or "" when it couldn't be told.
	OS string `json:"os"`
	// Source is "os-release" or "greeting".
	Source string `json:"source"`
	// Detail is the system's own name (PRETTY_NAME) or the greeting line.
	Detail string `json:"detail"`
	// Message explains an empty OS.
	Message string `json:"message"`
}

// Detect finds the OS of saved host id. It never logs in: /etc/os-release is
// read only through a connection that's already open.
func (s *Service) Detect(ctx context.Context, id int64) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if s.connected(id) {
		if r, err := s.release(ctx, id); err == nil {
			return r, nil
		}
		// Not a Unix shell (Windows) or no os-release: the greeting may still tell.
	}
	return s.greeting(ctx, id)
}

func (s *Service) connected(id int64) bool {
	cmd, err := s.resolver.ControlCommand(id, "check", sshcmd.Forward{})
	if err != nil {
		return false
	}
	return cmd.Run() == nil
}

func (s *Service) release(ctx context.Context, id int64) (Result, error) {
	remote, err := s.resolver.HostRemote(id, "")
	if err != nil {
		return Result{}, err
	}
	cmd := remote.Command(ctx, ReleaseCommand)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}
	text, _ := io.ReadAll(io.LimitReader(out, 64<<10))
	if err := cmd.Wait(); err != nil {
		return Result{}, err
	}
	os, name := ParseRelease(string(text))
	if os == "" && name == "" {
		return Result{}, errors.New("empty /etc/os-release")
	}
	r := Result{OS: os, Source: "os-release", Detail: name}
	if os == "" {
		r.Message = "Not one of the systems TuxShell has a logo for."
	}
	return r, nil
}

func (s *Service) greeting(ctx context.Context, id int64) (Result, error) {
	ep, err := s.resolver.Endpoint(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if ep.Proxied {
		return Result{Source: "greeting", Message: "This host is reached through a jump host. Open a session to it, then Detect reads /etc/os-release."}, nil
	}
	line, err := readGreeting(ctx, net.JoinHostPort(ep.Hostname, strconv.Itoa(ep.Port)))
	if err != nil {
		return Result{Source: "greeting", Message: "Couldn't read the SSH greeting: " + err.Error()}, nil
	}
	r := Result{OS: ParseGreeting(line), Source: "greeting", Detail: line}
	if r.OS == "" {
		r.Message = "The SSH greeting doesn't name the system. Open a session to this host and click Detect again to read /etc/os-release."
	}
	return r, nil
}

// readGreeting connects to addr and returns the server's "SSH-" line. It
// sends nothing and closes right after: no login is attempted.
func readGreeting(ctx context.Context, addr string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	deadline, _ := ctx.Deadline()
	_ = conn.SetReadDeadline(deadline)
	// Servers may send other lines first (RFC 4253 4.2); a line is at most 255 bytes.
	br := bufio.NewReaderSize(io.LimitReader(conn, 8<<10), 512)
	for range 20 {
		line, err := br.ReadString('\n')
		line = clean(line)
		if strings.HasPrefix(line, "SSH-") {
			return line, nil
		}
		if err != nil {
			return "", errors.New("no SSH greeting (is this an ssh server?)")
		}
	}
	return "", errors.New("no SSH greeting (is this an ssh server?)")
}

// clean keeps printable ASCII, so a server's text can't carry control codes to the UI.
func clean(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimRight(s, "\r\n") {
		if r >= 0x20 && r < 0x7f && b.Len() < 255 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ParseRelease reads /etc/os-release text: the OS id (hosts.OSes, "linux" for
// other Linux systems, "" when it can't tell) and the system's own name.
func ParseRelease(text string) (os, name string) {
	vals := map[string]string{}
	for line := range strings.Lines(text) {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		vals[key] = clean(strings.Trim(val, `"'`))
	}
	name = vals["PRETTY_NAME"]
	if name == "" {
		name = vals["NAME"]
	}
	if vals["ID"] == "" && name == "" {
		return "", ""
	}
	if vals["ID"] == "ol" {
		return "linux", name // Oracle Linux says it's like "fedora", which isn't its logo
	}
	// The system's own id first, then the ones it says it's like ("ubuntu debian").
	for _, id := range append([]string{vals["ID"]}, strings.Fields(vals["ID_LIKE"])...) {
		if os := idOS(id); os != "" {
			return os, name
		}
	}
	if os := hosts.ParseOS(name); os != "" {
		return os, name
	}
	return "linux", name
}

func idOS(id string) string {
	switch id = strings.ToLower(id); id {
	case "":
		return ""
	case "raspbian":
		return "debian"
	case "archarm":
		return "arch"
	}
	if os := hosts.ParseOS(id); os != "linux" {
		return os
	}
	return ""
}

// ParseGreeting reads the OS from an ssh server's greeting, or "" when it
// doesn't say ("SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13" → ubuntu).
func ParseGreeting(line string) string {
	_, software, ok := strings.Cut(line, "-2.0-")
	if !ok {
		_, software, ok = strings.Cut(line, "-1.99-")
	}
	if !ok {
		return ""
	}
	s := strings.ToLower(software)
	switch {
	case strings.Contains(s, "raspbian"):
		return "debian"
	case strings.Contains(s, "for_windows"):
		return "windows"
	}
	for _, os := range []string{"ubuntu", "debian", "freebsd"} {
		if strings.Contains(s, os) {
			return os
		}
	}
	return ""
}
