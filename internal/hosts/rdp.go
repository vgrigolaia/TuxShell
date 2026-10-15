package hosts

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Protocols a host can use.
const (
	ProtocolSSH = "ssh"
	ProtocolRDP = "rdp"
)

// RDP holds a remote desktop host's options. Everything here ends up as
// FreeRDP arguments built in Go (internal/rdp), never on a command line.
type RDP struct {
	Domain string `json:"domain"`
	// Window is "window" or "fullscreen".
	Window string `json:"window"`
	// Size is "fit" (follow the window, resizing the remote desktop) or
	// "fixed" (Width x Height).
	Size   string `json:"size"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Multimon spans all monitors (in fullscreen).
	Multimon bool `json:"multimon"`
	// Clipboard shares copy and paste (text and files) both ways.
	Clipboard bool `json:"clipboard"`
	// Folders are local folders the remote computer can open.
	Folders []Folder `json:"folders"`
	// HomeFolder shares the local home folder.
	HomeFolder bool `json:"homeFolder"`
	// Sound is "local" (play here), "remote" (on the server) or "off".
	Sound      string `json:"sound"`
	Microphone bool   `json:"microphone"`
	// Quality is "best", "balanced" or "low" (slow links).
	Quality string `json:"quality"`
	// Gateway is an RD Gateway to connect through (empty Host for none).
	Gateway Gateway `json:"gateway"`
}

// Folder is a shared folder: the name the remote computer sees and the
// local path.
type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Gateway is an RD Gateway. Without a user it logs in with the host's login.
type Gateway struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	User   string `json:"user"`
	Domain string `json:"domain"`
}

// DefaultRDP is a new remote desktop host's options.
func DefaultRDP() RDP {
	return RDP{Window: "window", Size: "fit", Width: 1600, Height: 900, Clipboard: true, Sound: "local", Quality: "balanced", Folders: []Folder{}}
}

// UnmarshalJSON fills options missing from stored JSON with the defaults.
func (r *RDP) UnmarshalJSON(b []byte) error {
	type plain RDP
	v := plain(DefaultRDP())
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = RDP(v)
	return nil
}

var (
	// Share names: FreeRDP separates its arguments with commas.
	folderNameRe = regexp.MustCompile(`^[\p{L}\p{N} _.-]{1,32}$`)
	domainRe     = regexp.MustCompile(`^[\p{L}\p{N}_.-]{0,64}$`)
)

func oneOf(v *string, def string, allowed ...string) {
	for _, a := range allowed {
		if *v == a {
			return
		}
	}
	*v = def
}

func (r *RDP) normalize() {
	d := DefaultRDP()
	r.Domain = strings.TrimSpace(r.Domain)
	oneOf(&r.Window, d.Window, "window", "fullscreen")
	oneOf(&r.Size, d.Size, "fit", "fixed")
	if r.Width == 0 {
		r.Width = d.Width
	}
	if r.Height == 0 {
		r.Height = d.Height
	}
	oneOf(&r.Sound, d.Sound, "local", "remote", "off")
	oneOf(&r.Quality, d.Quality, "best", "balanced", "low")
	folders := []Folder{}
	for _, f := range r.Folders {
		f.Name, f.Path = strings.TrimSpace(f.Name), strings.TrimSpace(f.Path)
		if f.Name != "" || f.Path != "" {
			folders = append(folders, f)
		}
	}
	r.Folders = folders
	r.Gateway.Host = strings.TrimSpace(r.Gateway.Host)
	r.Gateway.User = strings.TrimSpace(r.Gateway.User)
	r.Gateway.Domain = strings.TrimSpace(r.Gateway.Domain)
	if r.Gateway.Host != "" && r.Gateway.Port == 0 {
		r.Gateway.Port = 443
	}
}

// noBreak rejects characters that would split a FreeRDP argument.
func noBreak(s string) bool { return !strings.ContainsAny(s, ",\n\r\x00") }

func (r *RDP) validate() error {
	invalid := func(field, format string, args ...any) error {
		return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
	}
	switch {
	case !domainRe.MatchString(r.Domain):
		return invalid("rdp.domain", "Use a Windows domain name like CORP or corp.example.com")
	case r.Size == "fixed" && (r.Width < 200 || r.Width > 8192 || r.Height < 200 || r.Height > 8192):
		return invalid("rdp.size", "Width and height must be between 200 and 8192")
	case len(r.Folders) > 20:
		return invalid("rdp.folders", "At most 20 shared folders")
	}
	seen := map[string]bool{}
	for _, f := range r.Folders {
		switch {
		case !folderNameRe.MatchString(f.Name):
			return invalid("rdp.folders", "Folder names may contain letters, digits, spaces, . _ and - (up to 32)")
		case seen[strings.ToLower(f.Name)]:
			return invalid("rdp.folders", "Two shared folders are called %q", f.Name)
		case !filepath.IsAbs(f.Path) || !noBreak(f.Path):
			return invalid("rdp.folders", "%q: choose a folder (its path can't contain commas)", f.Name)
		}
		seen[strings.ToLower(f.Name)] = true
	}
	if g := r.Gateway; g.Host != "" {
		switch {
		case !hostRe.MatchString(g.Host):
			return invalid("rdp.gateway", "Gateway: use a hostname or IP address")
		case g.Port < 1 || g.Port > 65535:
			return invalid("rdp.gateway", "Gateway port must be between 1 and 65535")
		case g.User != "" && (!userRe.MatchString(g.User) || !noBreak(g.User)):
			return invalid("rdp.gateway", "Gateway user may contain letters, digits, . _ - and @")
		case !domainRe.MatchString(g.Domain):
			return invalid("rdp.gateway", "Gateway domain: use a name like CORP")
		}
	}
	return nil
}
