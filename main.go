package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

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
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	// Frameless: the frontend draws its own title bar and window controls.
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "TuxShell",
		Width:            1280,
		Height:           800,
		MinWidth:         760,
		MinHeight:        480,
		Frameless:        true,
		BackgroundColour: application.NewRGB(0x08, 0x09, 0x0C),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
