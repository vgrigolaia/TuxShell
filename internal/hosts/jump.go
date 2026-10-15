package hosts

import (
	"strconv"
	"strings"
)

// ResolveJump replaces hops that name a known SSH host (byName is keyed by
// lower-case name) with that host's address, keeping the host's own jump
// hosts in front, so plain ssh can follow them.
func ResolveJump(self, jump string, byName map[string]Host) string {
	return resolveJump(self, jump, byName, 0)
}

func resolveJump(self, jump string, byName map[string]Host, depth int) string {
	if jump == "" || depth > 4 {
		return jump
	}
	var hops []string
	for _, hop := range strings.Split(jump, ",") {
		hop = strings.TrimSpace(hop)
		b, ok := byName[strings.ToLower(hop)]
		if !ok || strings.EqualFold(hop, self) || b.Protocol == ProtocolRDP || (b.Hostname == "" && b.Alias == "") {
			hops = append(hops, hop)
			continue
		}
		if inner := resolveJump(b.Name, b.ProxyJump, byName, depth+1); inner != "" {
			hops = append(hops, inner)
		}
		hops = append(hops, Address(b))
	}
	return strings.Join(hops, ",")
}

// Address is [user@]host[:port] for ssh -J, with IPv6 in brackets.
func Address(h Host) string {
	host := h.Hostname
	if host == "" {
		host = h.Alias
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if h.User != "" {
		host = h.User + "@" + host
	}
	if h.Port != 0 && h.Port != 22 {
		host += ":" + strconv.Itoa(h.Port)
	}
	return host
}
