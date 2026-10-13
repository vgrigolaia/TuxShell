package main

import (
	"embed"
	"log"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/vgrigolaia/tuxshell/internal/cliphist"
	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/sshcmd"
	"github.com/vgrigolaia/tuxshell/internal/sshconfig"
	"github.com/vgrigolaia/tuxshell/internal/terminal"
)

// The built frontend is embedded into the binary.
//
//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Registered events get typed bindings in the frontend.
	application.RegisterEvent[terminal.DataEvent](terminal.EventData)
	application.RegisterEvent[terminal.ExitEvent](terminal.EventExit)
	application.RegisterEvent[application.Void](cliphist.EventChanged)
}

func main() {
	dbPath, err := hosts.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	store, err := hosts.Open(dbPath)
	if err != nil {
		log.Fatalf("open host store: %v", err)
	}

	sshConfigPath, err := sshconfig.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}

	var app *application.App
	clips := cliphist.NewService(
		filepath.Join(filepath.Dir(dbPath), "clipboard.db"),
		func() (string, bool) { return app.Clipboard.Text() },
		func() { app.Event.Emit(cliphist.EventChanged) },
	)
	terminals := terminal.NewService(func(name string, data any) {
		app.Event.Emit(name, data)
	}, sshcmd.Resolver{Store: store})

	app = application.New(application.Options{
		Name:        "TuxShell",
		Description: "SSH connection manager for Linux",
		Services: []application.Service{
			application.NewService(terminals),
			application.NewService(hosts.NewService(store)),
			application.NewService(sshconfig.NewService(store, sshConfigPath)),
			application.NewService(clips),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	// Frameless: the frontend draws its own title bar and window controls.
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "TuxShell",
		Width:            1280,
		Height:           800,
		MinWidth:         760,
		MinHeight:        480,
		Frameless:        true,
		BackgroundColour: application.NewRGB(0x08, 0x09, 0x0C),
		URL:              "/",
	})

	// Wayland only lets the focused app read the clipboard, so capture what
	// was copied elsewhere each time the window regains focus. Off the event
	// goroutine, since the clipboard read hops to the main thread.
	window.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) {
		go clips.CaptureSystem()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
