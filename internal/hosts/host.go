// Package hosts stores saved SSH hosts in SQLite.
package hosts

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Host is a saved SSH connection. Secret fields hold only op:// references;
// values are resolved at connect time and never stored.
type Host struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
	// Alias is the ~/.ssh/config Host entry this was imported from. When set,
	// ssh connects to the alias so the rest of that config still applies.
	Alias        string   `json:"alias"`
	Hostname     string   `json:"hostname"`
	User         string   `json:"user"`
	Port         int      `json:"port"`
	ProxyJump    string   `json:"proxyJump"`
	IdentityFile string   `json:"identityFile"`
	PasswordRef  string   `json:"passwordRef"`
	Tags         []string `json:"tags"`
	// OS is one of OSes, or "" when not set. It only picks the logo shown.
	OS string `json:"os"`
	// PasswordSaved is true when a password for this host is in the system
	// keyring. Only Store.SetPasswordSaved changes it; Save leaves it alone.
	PasswordSaved bool `json:"passwordSaved"`
	// Profile is the terminal profile's ID, or "" for the default profile. A
	// profile that no longer exists also means the default.
	Profile string `json:"profile"`
}

// ValidationError reports a problem with one field, so the form can show it inline.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

var (
	// Hostnames, IPv4/IPv6 literals and ssh_config aliases. No spaces, and no
	// leading dash so a value can never be read as an ssh option.
	hostRe = regexp.MustCompile(`^[A-Za-z0-9_.:\[\]%][A-Za-z0-9_.:\[\]%-]*$`)
	userRe = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.@-]*$`)
	// ProxyJump: comma-separated [user@]host[:port] hops.
	jumpRe = regexp.MustCompile(`^[A-Za-z0-9_.@:\[\]%][A-Za-z0-9_.@:\[\]%,-]*$`)
	tagRe  = regexp.MustCompile(`^[\p{L}\p{N}_.-]+$`)
	// Matches settings' profile IDs.
	profileRe = regexp.MustCompile(`^[a-z0-9]{1,32}$`)
)

// Normalize trims fields and applies defaults.
func (h *Host) Normalize() {
	h.Name = strings.TrimSpace(h.Name)
	h.Group = strings.TrimSpace(h.Group)
	h.Alias = strings.TrimSpace(h.Alias)
	h.Hostname = strings.TrimSpace(h.Hostname)
	h.User = strings.TrimSpace(h.User)
	h.ProxyJump = strings.TrimSpace(h.ProxyJump)
	h.IdentityFile = strings.TrimSpace(h.IdentityFile)
	h.PasswordRef = strings.TrimSpace(h.PasswordRef)
	h.OS = strings.ToLower(strings.TrimSpace(h.OS))
	if h.Profile = strings.TrimSpace(h.Profile); h.Profile == "default" {
		h.Profile = ""
	}
	if h.Port == 0 {
		h.Port = 22
	}
	tags := h.Tags[:0]
	seen := map[string]bool{}
	for _, t := range h.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] {
			seen[t] = true
			tags = append(tags, t)
		}
	}
	h.Tags = tags
}

// Validate checks a normalized host.
func (h *Host) Validate() error {
	invalid := func(field, format string, args ...any) error {
		return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
	}
	switch {
	case h.Name == "":
		return invalid("name", "Name is required")
	case len(h.Name) > 100:
		return invalid("name", "Name is too long")
	case h.Hostname == "" && h.Alias == "":
		return invalid("hostname", "Host is required")
	case h.Hostname != "" && !hostRe.MatchString(h.Hostname):
		return invalid("hostname", "Use a hostname or IP address without spaces")
	case h.Alias != "" && !hostRe.MatchString(h.Alias):
		return invalid("alias", "Not a valid ssh_config alias")
	case h.User != "" && !userRe.MatchString(h.User):
		return invalid("user", "User may contain letters, digits, . _ - and @")
	case h.Port < 1 || h.Port > 65535:
		return invalid("port", "Port must be between 1 and 65535")
	case h.ProxyJump != "" && !jumpRe.MatchString(h.ProxyJump):
		return invalid("proxyJump", "Use [user@]host[:port], comma-separated for several hops")
	case strings.HasPrefix(h.IdentityFile, "-") || strings.ContainsAny(h.IdentityFile, "\n\r"):
		return invalid("identityFile", "Not a valid file path")
	case h.PasswordRef != "" && !strings.HasPrefix(h.PasswordRef, "op://"):
		return invalid("passwordRef", "Use a 1Password reference like op://Vault/Item/field")
	case h.OS != "" && !knownOS(h.OS):
		return invalid("os", "Unknown operating system")
	case h.Profile != "" && !profileRe.MatchString(h.Profile):
		return invalid("profile", "Unknown profile")
	}
	for _, t := range h.Tags {
		if !tagRe.MatchString(t) {
			return invalid("tags", "Tags may contain letters, digits, . _ and -")
		}
	}
	return nil
}

// ErrNotFound is returned when a host ID doesn't exist.
var ErrNotFound = errors.New("host not found")
