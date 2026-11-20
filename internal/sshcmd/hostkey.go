package sshcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// HostKeyNames returns the names ssh files host keys under in known_hosts
// ("host", "[host]:port" or a HostKeyAlias) for a saved host (hostID) or an
// ad-hoc target and for each of its jump hosts, plus the user's known_hosts
// files that exist. ssh -G reads the config and connects to nothing.
func (r Resolver) HostKeyNames(ctx context.Context, hostID int64, target string) (names, files []string, err error) {
	var h hosts.Host
	if hostID != 0 {
		h, err = r.host(hostID, nil)
	} else {
		h, err = ParseTarget(target)
	}
	if err != nil {
		return nil, nil, err
	}
	ssh, err := sshPath()
	if err != nil {
		return nil, nil, err
	}
	all := []hosts.Host{h}
	if h.ProxyJump != "" {
		for _, hop := range strings.Split(h.ProxyJump, ",") {
			if j, err := ParseTarget(strings.TrimSpace(hop)); err == nil {
				all = append(all, j)
			}
		}
	}
	seen := map[string]bool{}
	for _, x := range all {
		x.ProxyJump = ""
		out, err := exec.CommandContext(ctx, ssh, append([]string{"-G"}, Args(x)...)...).Output()
		if err != nil {
			return nil, nil, fmt.Errorf("ssh -G: %w", err)
		}
		name, known := keyName(string(out))
		if name != "" && !strings.HasPrefix(name, "-") {
			names = append(names, name)
		}
		for _, f := range known {
			if _, err := os.Stat(f); err == nil && !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return names, files, nil
}

// keyName reads ssh -G output: the known_hosts name of the connection and the
// user's known_hosts files (the global ones belong to root and stay as they are).
func keyName(out string) (name string, files []string) {
	var host, port, alias string
	for line := range strings.Lines(out) {
		key, val, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch strings.ToLower(key) {
		case "hostname":
			host = val
		case "port":
			port = val
		case "hostkeyalias":
			alias = val
		case "userknownhostsfile":
			for _, f := range strings.Fields(val) {
				files = append(files, expandHome(f))
			}
		}
	}
	switch {
	case alias != "" && alias != "none":
		return alias, files
	case port == "" || port == "22":
		return host, files
	default:
		return "[" + host + "]:" + port, files
	}
}
