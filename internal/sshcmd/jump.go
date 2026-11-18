package sshcmd

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// jumpPersist is how long (seconds) a shared jump host connection stays after
// the last connection through it ends. Every server session through it keeps
// it in use, and those are closed with their last tab, so this is when the
// jump connection goes too (user rule: nothing may stay connected).
const jumpPersist = "5"

// Parts of a jump hop that may go into a ProxyCommand, which ssh runs through
// a shell: no quotes, spaces or other shell characters, never a leading "-".
var (
	hopUserRe = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.@-]*$`)
	hopHostRe = regexp.MustCompile(`^[A-Za-z0-9_.:%][A-Za-z0-9_.:%-]*$`)
	hopPortRe = regexp.MustCompile(`^[0-9]{1,5}$`)
	hopListRe = regexp.MustCompile(`^[A-Za-z0-9_.@:\[\]%][A-Za-z0-9_.@:\[\]%-]*$`)
	safePath  = regexp.MustCompile(`^[A-Za-z0-9_./+-]+$`)
)

// args is Args with the jump host reached through a shared connection: ssh -J
// opens a new connection to the jump host (key exchange, login, an agent
// signature) for every server behind it, while this opens one per jump host
// and runs the others through it (ProxyCommand "ssh -W" with ControlMaster).
// batch keeps the jump login from prompting. Without a control folder, or for
// a hop it can't pass safely, it falls back to plain -J.
func (r Resolver) args(h hosts.Host, batch bool) []string {
	if h.ProxyJump == "" || r.ControlDir == "" {
		return Args(h)
	}
	proxy, ok := r.proxyCommand(h.ProxyJump, batch)
	if !ok {
		return Args(h)
	}
	h.ProxyJump = ""
	return append([]string{"-o", "ProxyCommand=" + proxy}, Args(h)...)
}

// proxyCommand builds the ProxyCommand for a -J list: ssh to its last hop
// (through the earlier ones with -J) over a shared connection, forwarding
// stdin/stdout to the server (%h:%p, filled in by the outer ssh).
func (r Resolver) proxyCommand(jump string, batch bool) (string, bool) {
	hops := strings.Split(jump, ",")
	for i := range hops {
		hops[i] = strings.TrimSpace(hops[i])
		if !hopListRe.MatchString(hops[i]) {
			return "", false
		}
	}
	user, host, port, ok := splitHop(hops[len(hops)-1])
	ssh, err := sshPath()
	if !ok || err != nil || !safePath.MatchString(ssh) || !safePath.MatchString(r.ControlDir) {
		return "", false
	}
	args := []string{
		"-o", "ControlMaster=auto",
		// Own sockets (jm-), never a terminal's (cm-): closing a jump host's
		// own tab must not cut the sessions that run through it.
		"-o", "ControlPath=" + filepath.Join(r.ControlDir, "jm-%C"),
		"-o", "ControlPersist=" + jumpPersist,
	}
	if batch {
		args = append(args, "-o", "BatchMode=yes", "-o", "ConnectTimeout=10")
	}
	if inner := hops[:len(hops)-1]; len(inner) > 0 {
		args = append(args, "-J", strings.Join(inner, ","))
	}
	if port != "" {
		args = append(args, "-p", port)
	}
	if user != "" {
		args = append(args, "-l", user)
	}
	args = append(args, "-W", "[%h]:%p", "--", host)

	// ssh expands %-tokens in a ProxyCommand before the shell sees it: %h and
	// %p are the server's, and everything else is escaped (%%C reaches the
	// inner ssh as %C, its own connection's hash).
	quoted := []string{quote(ssh)}
	for i, a := range args {
		if i > 0 && args[i-1] == "-W" {
			quoted = append(quoted, quote(a))
			continue
		}
		quoted = append(quoted, quote(strings.ReplaceAll(a, "%", "%%")))
	}
	return strings.Join(quoted, " "), true
}

// splitHop splits "[user@]host[:port]" (IPv6 as "[addr]:port"). port is ""
// when the hop doesn't name one, so ~/.ssh/config can still pick it.
func splitHop(hop string) (user, host, port string, ok bool) {
	rest := hop
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		user, rest = rest[:i], rest[i+1:]
		if !hopUserRe.MatchString(user) {
			return "", "", "", false
		}
	}
	switch {
	case strings.HasPrefix(rest, "["):
		end := strings.Index(rest, "]")
		if end < 0 {
			return "", "", "", false
		}
		host = rest[1:end]
		after := rest[end+1:]
		if after != "" {
			p, found := strings.CutPrefix(after, ":")
			if !found {
				return "", "", "", false
			}
			port = p
		}
	case strings.Count(rest, ":") == 1:
		host, port, _ = strings.Cut(rest, ":")
	default:
		host = rest
	}
	if !hopHostRe.MatchString(host) || (port != "" && !hopPortRe.MatchString(port)) {
		return "", "", "", false
	}
	return user, host, port, true
}

// quote wraps s in single quotes for the shell ssh runs a ProxyCommand with.
// Callers pass only values without quotes or backslashes (checked above).
func quote(s string) string { return "'" + s + "'" }
