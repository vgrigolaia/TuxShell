package terminal

import (
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// Foreground describes an ssh the user started by hand in a local shell tab.
// Its connection is private to that ssh, so the file panel can't browse it;
// the app offers to open the same server as its own connection instead.
type Foreground struct {
	// SSH is false when the tab's foreground program isn't ssh (or can't be told).
	SSH  bool   `json:"ssh"`
	User string `json:"user"`
	Host string `json:"host"`
	Port int    `json:"port"`
	// Target is "[user@]host[:port]", ready for a quick connect tab.
	Target string `json:"target"`
	// Extra is set when the command had options that aren't carried over (-J, -i, -o…).
	Extra bool `json:"extra"`
}

// Foreground reports whether a local shell session is running ssh right now.
func (s *Service) Foreground(id string) (Foreground, error) {
	sess, err := s.get(id)
	if err != nil {
		return Foreground{}, err
	}
	argv := foregroundArgs(sess.ptmx)
	if argv == nil {
		return Foreground{}, nil
	}
	return parseSSH(argv), nil
}

// sshArgOpts are ssh's options that take a value (ssh(1)).
const sshArgOpts = "BbcDEeFIiJLlmOoPpQRSWw"

// parseSSH reads user, host and port from an ssh command line.
func parseSSH(argv []string) Foreground {
	if len(argv) == 0 || strings.TrimPrefix(filepath.Base(argv[0]), "-") != "ssh" {
		return Foreground{}
	}
	var f Foreground
	dest := ""
	args := argv[1:]
loop:
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			if i+1 < len(args) {
				dest = args[i+1]
			}
			break loop
		case len(a) > 1 && a[0] == '-':
			for j := 1; j < len(a); j++ {
				c := a[j]
				if !strings.ContainsRune(sshArgOpts, rune(c)) {
					continue // a flag such as -v, -t or -4
				}
				val := a[j+1:]
				if val == "" && i+1 < len(args) {
					i++
					val = args[i]
				}
				switch c {
				case 'l':
					f.User = val
				case 'p':
					f.Port, _ = strconv.Atoi(val)
				default:
					f.Extra = true
				}
				break
			}
		default:
			dest = a
			break loop
		}
	}
	if u, err := url.Parse(dest); err == nil && u.Scheme == "ssh" {
		if u.User != nil {
			f.User = u.User.Username()
		}
		dest = u.Hostname()
		if p, _ := strconv.Atoi(u.Port()); p > 0 {
			f.Port = p
		}
	} else if i := strings.LastIndex(dest, "@"); i >= 0 {
		f.User, dest = dest[:i], dest[i+1:]
	}
	if dest == "" || strings.HasPrefix(dest, "-") {
		return Foreground{}
	}
	f.SSH, f.Host = true, dest
	f.Target = dest
	if f.Port > 0 && f.Port != 22 {
		f.Target = net.JoinHostPort(dest, strconv.Itoa(f.Port))
	}
	if f.User != "" {
		f.Target = f.User + "@" + f.Target
	}
	return f
}
