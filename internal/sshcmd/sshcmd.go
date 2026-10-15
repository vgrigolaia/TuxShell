// Package sshcmd builds system ssh invocations for saved hosts and ad-hoc targets.
package sshcmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// Args returns ssh's arguments (without the program name) for h.
// h must already be validated, which guarantees no value starts with "-".
func Args(h hosts.Host) []string {
	args := []string{"-p", strconv.Itoa(h.Port)}
	if h.User != "" {
		args = append(args, "-l", h.User)
	}
	if h.ProxyJump != "" {
		args = append(args, "-J", h.ProxyJump)
	}
	if h.IdentityFile != "" {
		args = append(args, "-i", expandHome(h.IdentityFile))
	}

	dest := h.Hostname
	if h.Alias != "" {
		// Connect via the alias so the rest of its ssh_config block applies;
		// an edited hostname still wins.
		dest = h.Alias
		if h.Hostname != "" && h.Hostname != h.Alias {
			args = append(args, "-o", "HostName="+h.Hostname)
		}
	}
	return append(args, "--", dest)
}

// ParseTarget turns "[user@]host[:port]" into a validated ad-hoc host.
func ParseTarget(target string) (hosts.Host, error) {
	target = strings.TrimSpace(target)
	h := hosts.Host{Name: target}

	rest := target
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		h.User, rest = rest[:i], rest[i+1:]
	}
	switch {
	case strings.HasPrefix(rest, "["):
		// [ipv6]:port
		end := strings.Index(rest, "]")
		if end < 0 {
			return h, &hosts.ValidationError{Field: "hostname", Message: "Missing ] in IPv6 address"}
		}
		h.Hostname = rest[1:end]
		if port, ok := strings.CutPrefix(rest[end+1:], ":"); ok {
			h.Port, _ = strconv.Atoi(port)
			if h.Port == 0 {
				return h, &hosts.ValidationError{Field: "port", Message: "Port must be a number"}
			}
		}
	case strings.Count(rest, ":") == 1:
		host, port, _ := strings.Cut(rest, ":")
		h.Hostname = host
		h.Port, _ = strconv.Atoi(port)
		if h.Port == 0 {
			return h, &hosts.ValidationError{Field: "port", Message: "Port must be a number"}
		}
	default:
		h.Hostname = rest
	}

	h.Normalize()
	if err := h.Validate(); err != nil {
		return h, err
	}
	return h, nil
}

// Resolver builds ssh commands from the host store.
type Resolver struct {
	Store *hosts.Store
	// ControlDir holds ControlMaster sockets; empty disables connection sharing.
	ControlDir string
	// RemoteCommand, when set, returns a command for interactive sessions to
	// run on the server (from the host's terminal profile), or "".
	RemoteCommand func(h hosts.Host) string
	// Agent, when set, returns an SSH agent socket for ssh to use ("" for
	// the usual one). It is passed as SSH_AUTH_SOCK, so jump hosts use it too
	// and an IdentityAgent in ~/.ssh/config still wins.
	Agent func() string
}

// env returns the environment for ssh (nil: inherit TuxShell's).
func (r Resolver) env() []string {
	if r.Agent == nil {
		return nil
	}
	sock := r.Agent()
	if sock == "" {
		return nil
	}
	return append(os.Environ(), "SSH_AUTH_SOCK="+sock)
}

// command returns ssh with args and the resolver's environment.
func (r Resolver) command(ssh string, args ...string) *exec.Cmd {
	cmd := exec.Command(ssh, args...)
	cmd.Env = r.env()
	return cmd
}

// host loads and re-validates a saved host (the database may have been edited
// by hand), optionally replacing its login user.
func (r Resolver) host(id int64, user *string) (hosts.Host, error) {
	h, err := r.Store.Get(id)
	if err != nil {
		return h, err
	}
	if h.Protocol == hosts.ProtocolRDP {
		return h, fmt.Errorf("%q is a remote desktop host, not SSH", h.Name)
	}
	if user != nil {
		h.User = *user
	}
	h.Normalize()
	if err := h.Validate(); err != nil {
		return h, fmt.Errorf("host %q: %w", h.Name, err)
	}
	return h, nil
}

// HostCommand returns the interactive command for a saved host.
func (r Resolver) HostCommand(id int64) (*exec.Cmd, error) {
	h, err := r.host(id, nil)
	if err != nil {
		return nil, err
	}
	return r.session(h)
}

// HostCommandAs is HostCommand with the login user replaced, for a
// username typed at connect time.
func (r Resolver) HostCommandAs(id int64, user string) (*exec.Cmd, error) {
	h, err := r.host(id, &user)
	if err != nil {
		var ve *hosts.ValidationError
		if errors.As(err, &ve) {
			return nil, ve // keep the field so the login prompt can show it
		}
		return nil, err
	}
	return r.session(h)
}

// TargetCommand returns the interactive command for an ad-hoc "[user@]host[:port]".
func (r Resolver) TargetCommand(target string) (*exec.Cmd, error) {
	h, err := ParseTarget(target)
	if err != nil {
		return nil, err
	}
	return r.session(h)
}

// HostRemote returns a batch runner for a saved host, logging in as user
// when it's non-empty (matching a "Connect as" session).
func (r Resolver) HostRemote(id int64, user string) (Remote, error) {
	var override *string
	if user != "" {
		override = &user
	}
	h, err := r.host(id, override)
	if err != nil {
		return Remote{}, err
	}
	return r.remote(h)
}

// DraftRemote returns a batch runner for host values that may not be saved
// yet (the host editor's Test button). They're validated like saved hosts.
func (r Resolver) DraftRemote(h hosts.Host) (Remote, error) {
	h.Normalize()
	if err := h.Validate(); err != nil {
		return Remote{}, err
	}
	return r.remote(h)
}

// Forward is a -L or -D argument pair, e.g. {"-L", "127.0.0.1:5432:db:5432"}.
// Build it with LocalForward or DynamicForward so values are always checked.
type Forward [2]string

// LocalForward forwards 127.0.0.1:bind to target:port as seen from the host.
// Only loopback is bound, so other machines can't use the tunnel.
func LocalForward(bind int, target string, port int) Forward {
	if strings.Contains(target, ":") && !strings.HasPrefix(target, "[") {
		target = "[" + target + "]" // IPv6 literal
	}
	return Forward{"-L", fmt.Sprintf("127.0.0.1:%d:%s:%d", bind, target, port)}
}

// DynamicForward opens a SOCKS proxy on 127.0.0.1:bind.
func DynamicForward(bind int) Forward {
	return Forward{"-D", fmt.Sprintf("127.0.0.1:%d", bind)}
}

// TunnelCommand returns a forward-only ssh process with its own connection
// for a saved host. It needs key or agent auth (it never prompts).
func (r Resolver) TunnelCommand(id int64, f Forward) (*exec.Cmd, error) {
	h, err := r.host(id, nil)
	if err != nil {
		return nil, err
	}
	ssh, err := sshPath()
	if err != nil {
		return nil, err
	}
	args := append(tunnelOptions(), f[0], f[1])
	return r.command(ssh, append(args, Args(h)...)...), nil
}

// JumpTunnelCommand returns a forward-only ssh through jump, a ProxyJump
// list ("[user@]host[:port],..."): it connects to the last hop, through the
// others. Like TunnelCommand it needs key or agent auth.
func (r Resolver) JumpTunnelCommand(jump string, f Forward) (*exec.Cmd, error) {
	hops := strings.Split(strings.TrimSpace(jump), ",")
	if len(hops) == 0 || strings.TrimSpace(hops[len(hops)-1]) == "" {
		return nil, errors.New("no jump host")
	}
	h, err := ParseTarget(hops[len(hops)-1])
	if err != nil {
		return nil, fmt.Errorf("jump host: %w", err)
	}
	h.ProxyJump = strings.Join(hops[:len(hops)-1], ",")
	h.Normalize()
	if err := h.Validate(); err != nil {
		return nil, fmt.Errorf("jump host: %w", err)
	}
	ssh, err := sshPath()
	if err != nil {
		return nil, err
	}
	args := append(tunnelOptions(), f[0], f[1])
	return r.command(ssh, append(args, Args(h)...)...), nil
}

// ControlCommand asks the host's shared connection (an open terminal) to
// "check", "forward" or "cancel" f, so password-only hosts can tunnel too.
func (r Resolver) ControlCommand(id int64, op string, f Forward) (*exec.Cmd, error) {
	if r.ControlDir == "" {
		return nil, errors.New("ssh connection sharing is off")
	}
	switch op {
	case "check", "forward", "cancel":
	default:
		return nil, fmt.Errorf("unknown control operation %q", op)
	}
	h, err := r.host(id, nil)
	if err != nil {
		return nil, err
	}
	ssh, err := sshPath()
	if err != nil {
		return nil, err
	}
	args := []string{"-o", "ControlPath=" + filepath.Join(r.ControlDir, "cm-%C"), "-O", op}
	if op != "check" {
		args = append(args, f[0], f[1])
	}
	return r.command(ssh, append(args, Args(h)...)...), nil
}

// TargetRemote returns a batch runner for an ad-hoc target.
func (r Resolver) TargetRemote(target string) (Remote, error) {
	h, err := ParseTarget(target)
	if err != nil {
		return Remote{}, err
	}
	return r.remote(h)
}

func (r Resolver) session(h hosts.Host) (*exec.Cmd, error) {
	ssh, err := sshPath()
	if err != nil {
		return nil, err
	}
	args := sessionOptions(r.ControlDir)
	command := ""
	if r.RemoteCommand != nil {
		command = r.RemoteCommand(h)
	}
	if command == "" {
		return r.command(ssh, append(args, Args(h)...)...), nil
	}
	// A remote command gets no terminal unless asked for. It goes after the
	// destination as one argument; the server's shell runs it.
	args = append(append(args, "-t"), Args(h)...)
	return r.command(ssh, append(args, command)...), nil
}

func (r Resolver) remote(h hosts.Host) (Remote, error) {
	ssh, err := sshPath()
	if err != nil {
		return Remote{}, err
	}
	return Remote{ssh: ssh, args: append(batchOptions(r.ControlDir), Args(h)...), env: r.env()}, nil
}

func sshPath() (string, error) {
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		return "", fmt.Errorf("ssh not found; install openssh-clients: %w", err)
	}
	return ssh, nil
}

func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return path
}
