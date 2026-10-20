package rdp

import (
	"context"
	"errors"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/platform"
)

// Client is an installed FreeRDP program.
type Client struct {
	Path string `json:"path"`
	// Kind is "x11" (xfreerdp) or "sdl" (sdl-freerdp, native Wayland).
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

// ErrNoClient means FreeRDP 3 isn't installed.
var ErrNoClient = errors.New("FreeRDP 3 isn't installed")

// Names differ by distribution: Fedora and RHEL ship xfreerdp/sdl-freerdp,
// Debian and Ubuntu xfreerdp3/sdl-freerdp3.
var candidates = map[string][]string{
	"x11": {"xfreerdp3", "xfreerdp"},
	"sdl": {"sdl-freerdp3", "sdl-freerdp"},
}

var versionRe = regexp.MustCompile(`FreeRDP version (\d+)\.(\d+)\.(\d+)`)

// FindClient returns the FreeRDP client to use (the host's, in a Flatpak). prefer is "auto", "x11" or
// "sdl"; auto uses the X11 client (the most complete, and it runs on Wayland
// through XWayland) and falls back to the SDL one.
func FindClient(prefer string) (Client, error) {
	order := []string{"x11", "sdl"}
	// macOS has no X server by default; Homebrew's sdl-freerdp runs natively.
	if prefer == "sdl" || (prefer == "auto" && runtime.GOOS == "darwin") {
		order = []string{"sdl", "x11"}
	}
	for _, kind := range order {
		for _, name := range candidates[kind] {
			path, err := platform.LookHostPath(name)
			if err != nil {
				continue
			}
			if v, ok := version(path); ok {
				return Client{Path: path, Kind: kind, Version: v}, nil
			}
		}
	}
	return Client{}, ErrNoClient
}

// version runs "<client> /version" and accepts FreeRDP 3 or newer.
func version(path string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, _ := platform.HostCommand(ctx, []string{"WLOG_LEVEL=OFF"}, "", path, "/version").CombinedOutput()
	m := versionRe.FindSubmatch(out)
	if m == nil {
		return "", false
	}
	major, _ := strconv.Atoi(string(m[1]))
	return string(m[1]) + "." + string(m[2]) + "." + string(m[3]), major >= 3
}

// InstallHint says how to install FreeRDP on common distributions.
const InstallHint = "Install FreeRDP 3: Fedora/RHEL: sudo dnf install freerdp · Debian/Ubuntu: sudo apt install freerdp3-x11"
