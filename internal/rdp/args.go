// Package rdp opens remote desktops with the system FreeRDP 3 client, in its
// own window. Every option, the password included, is written to FreeRDP's
// stdin (/args-from:stdin), so nothing secret is ever on a command line.
package rdp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// Connection is what one FreeRDP run needs besides the host's options.
type Connection struct {
	// Server and Port are where FreeRDP connects: the host itself, or the
	// local end of an SSH tunnel.
	Server string
	Port   int
	// Password logs in (and into the gateway when it has no user of its own).
	Password string
	// Fingerprint is the trusted certificate's SHA-256 (hex), or "" to let
	// FreeRDP trust it on first use (used with a gateway).
	Fingerprint string
	// Title names the window.
	Title string
}

// safe rejects values that would break FreeRDP's one-argument-per-line input.
func safe(s string) bool { return !strings.ContainsAny(s, "\n\r\x00") }

// Args returns FreeRDP's arguments for h, one per line as /args-from expects.
func Args(h hosts.Host, c Connection) ([]string, error) {
	if h.Protocol != hosts.ProtocolRDP {
		return nil, errors.New("not a remote desktop host")
	}
	r := h.RDP
	for _, v := range []string{c.Server, c.Password, c.Title, h.User, r.Domain} {
		if !safe(v) {
			return nil, errors.New("a value contains a line break")
		}
	}
	server := c.Server
	if strings.Contains(server, ":") && !strings.HasPrefix(server, "[") {
		server = "[" + server + "]" // IPv6 literal
	}
	args := []string{"/v:" + server + ":" + strconv.Itoa(c.Port)}
	if h.User != "" {
		args = append(args, "/u:"+h.User)
	}
	if r.Domain != "" {
		args = append(args, "/d:"+r.Domain)
	}
	if c.Password != "" {
		args = append(args, "/p:"+c.Password)
	}
	if c.Fingerprint != "" {
		args = append(args, "/cert:fingerprint:sha256:"+c.Fingerprint)
	} else {
		args = append(args, "/cert:tofu")
	}
	if c.Title != "" {
		args = append(args, "/t:"+c.Title)
	}

	// Display.
	switch {
	case r.Window == "fullscreen":
		args = append(args, "/f", "/floatbar:sticky:off,default:visible,show:fullscreen")
		if r.Multimon {
			args = append(args, "/multimon")
		}
		if r.Size == "fit" {
			args = append(args, "/dynamic-resolution")
		}
	case r.Size == "fixed":
		args = append(args, fmt.Sprintf("/size:%dx%d", r.Width, r.Height))
	default:
		args = append(args, "/size:80%", "/dynamic-resolution")
	}

	// Sharing.
	if r.Clipboard {
		args = append(args, "/clipboard")
	} else {
		args = append(args, "-clipboard")
	}
	for _, f := range r.Folders {
		args = append(args, "/drive:"+f.Name+","+f.Path)
	}
	if r.HomeFolder {
		args = append(args, "+home-drive")
	}

	// Sound.
	switch r.Sound {
	case "local":
		args = append(args, "/sound", "/audio-mode:redirect")
	case "remote":
		args = append(args, "/audio-mode:server")
	default:
		args = append(args, "/audio-mode:none")
	}
	if r.Microphone {
		args = append(args, "/microphone")
	}

	// Quality.
	switch r.Quality {
	case "best":
		args = append(args, "/network:lan", "/bpp:32", "/gfx", "+fonts", "+aero", "+wallpaper", "+themes", "+window-drag", "+menu-anims")
	case "low":
		args = append(args, "/network:modem", "/bpp:16", "-wallpaper", "-themes", "/compression-level:2")
	default:
		args = append(args, "/network:auto", "/bpp:32", "/gfx")
	}

	if g := r.Gateway; g.Host != "" {
		user, domain := g.User, g.Domain
		if user == "" {
			user, domain = h.User, r.Domain
		}
		// Gateway credentials are comma-separated fields, so a comma can't be passed.
		if strings.Contains(c.Password, ",") {
			return nil, errors.New("this password contains a comma, which FreeRDP can't pass to a gateway")
		}
		gw := fmt.Sprintf("/gateway:g:%s:%d", g.Host, g.Port)
		if user != "" {
			gw += ",u:" + user
		}
		if domain != "" {
			gw += ",d:" + domain
		}
		if c.Password != "" {
			gw += ",p:" + c.Password
		}
		args = append(args, gw)
	}

	return append(args, "+auto-reconnect", "/auto-reconnect-max-retries:5"), nil
}
