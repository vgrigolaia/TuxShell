package main

import (
	"embed"
	"log"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/vgrigolaia/tuxshell/internal/cliphist"
	"github.com/vgrigolaia/tuxshell/internal/files"
	"github.com/vgrigolaia/tuxshell/internal/hostio"
	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/settings"
	"github.com/vgrigolaia/tuxshell/internal/sshcmd"
	"github.com/vgrigolaia/tuxshell/internal/sshconfig"
	"github.com/vgrigolaia/tuxshell/internal/status"
	"github.com/vgrigolaia/tuxshell/internal/terminal"
	"github.com/vgrigolaia/tuxshell/internal/tunnels"
	"github.com/vgrigolaia/tuxshell/internal/updates"
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
	application.RegisterEvent[files.DropEvent](files.EventDropped)
	application.RegisterEvent[int](eventConfirmClose)
	application.RegisterEvent[[]status.Result](status.EventChanged)
	application.RegisterEvent[application.Void](tunnels.EventChanged)
}

// eventConfirmClose asks the frontend to confirm closing the window while
// sessions are live; it carries the number of live sessions.
const eventConfirmClose = "app:confirm-close"

func main() {
	dbPath, err := hosts.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	store, err := hosts.Open(dbPath)
	if err != nil {
		log.Fatalf("open host store: %v", err)
	}

	settingsPath, err := settings.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	prefs := settings.NewService(settingsPath)

	sshConfigPath, err := sshconfig.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}

	// Shared ssh connections let the file browser reuse a terminal's login.
	// Without a usable socket directory, sessions just don't share.
	controlDir, err := sshcmd.ControlDir()
	if err != nil {
		log.Printf("ssh connection sharing disabled: %v", err)
		controlDir = ""
	}
	resolver := sshcmd.Resolver{Store: store, ControlDir: controlDir}

	var app *application.App
	fileBrowser, recordDrop := files.NewService(resolver, func() ([]string, error) {
		return app.Dialog.OpenFile().
			CanChooseFiles(true).
			SetTitle("Upload files").
			SetButtonText("Upload").
			PromptForMultipleSelection()
	})
	hostFiles := hostio.NewService(store, func(title string, filters []hostio.Filter) (string, error) {
		d := app.Dialog.OpenFile().CanChooseFiles(true).SetTitle(title).SetButtonText("Open")
		for _, f := range filters {
			d.AddFilter(f.Name, f.Pattern)
		}
		d.AddFilter("All files", "*")
		return d.PromptForSingleSelection()
	})
	clips := cliphist.NewService(
		filepath.Join(filepath.Dir(dbPath), "clipboard.db"),
		func() (string, bool) { return app.Clipboard.Text() },
		func() { app.Event.Emit(cliphist.EventChanged) },
	)
	hostStatus := status.NewService(store, resolver,
		func() bool { return prefs.Get().Settings.HostStatus },
		func(r []status.Result) { app.Event.Emit(status.EventChanged, r) },
	)
	forwards := tunnels.NewService(store, resolver, func() { app.Event.Emit(tunnels.EventChanged) })
	terminals := terminal.NewService(func(name string, data any) {
		app.Event.Emit(name, data)
	}, resolver, prefs.Shell)

	app = application.New(application.Options{
		Name:        "TuxShell",
		Description: "SSH connection manager for Linux",
		// GNOME matches the window to build/linux/tuxshell.desktop (StartupWMClass)
		// by this id, which is how the dock finds our icon. The program name stays
		// "tuxshell" so WebKit keeps its data in the same folders.
		Linux: application.LinuxOptions{
			ApplicationID: "io.github.vgrigolaia.TuxShell",
			ProgramName:   "tuxshell",
		},
		Services: []application.Service{
			application.NewService(terminals),
			application.NewService(hosts.NewService(store)),
			application.NewService(sshconfig.NewService(store, sshConfigPath)),
			application.NewService(hostFiles),
			application.NewService(clips),
			application.NewService(fileBrowser),
			application.NewService(prefs),
			application.NewService(hostStatus),
			application.NewService(hosts.NewSnippetService(store)),
			application.NewService(forwards),
			application.NewService(updates.NewService()),
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
		EnableFileDrop:   true,
		BackgroundColour: application.NewRGB(0x08, 0x09, 0x0C),
		URL:              "/",
	})

	// Closing with live sessions (title bar button or the window manager) asks
	// first. The frontend hangs the sessions up and closes again once confirmed.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if n := terminals.LiveCount(); n > 0 && prefs.Get().Settings.Terminal.ConfirmClose {
			e.Cancel()
			app.Event.Emit(eventConfirmClose, n)
		}
	})

	// Wayland only lets the focused app read the clipboard, so capture what
	// was copied elsewhere each time the window regains focus. Off the event
	// goroutine, since the clipboard read hops to the main thread.
	window.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) {
		go clips.CaptureSystem()
	})

	// Files dropped on the file panel are uploaded to the folder it shows.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		target := e.Context().DropTargetDetails()
		if target == nil || target.ElementID != "file-panel" {
			return
		}
		if paths := e.Context().DroppedFiles(); len(paths) > 0 {
			app.Event.Emit(files.EventDropped, recordDrop(paths))
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
