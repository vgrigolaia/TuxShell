package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/vgrigolaia/tuxshell/internal/askpass"
	"github.com/vgrigolaia/tuxshell/internal/backgrounds"
	"github.com/vgrigolaia/tuxshell/internal/cliphist"
	"github.com/vgrigolaia/tuxshell/internal/files"
	"github.com/vgrigolaia/tuxshell/internal/hostio"
	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/login"
	"github.com/vgrigolaia/tuxshell/internal/onepassword"
	"github.com/vgrigolaia/tuxshell/internal/rdp"
	"github.com/vgrigolaia/tuxshell/internal/recordings"
	"github.com/vgrigolaia/tuxshell/internal/secrets"
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
	application.RegisterEvent[terminal.RecordingEvent](terminal.EventRecording)
	application.RegisterEvent[application.Void](cliphist.EventChanged)
	application.RegisterEvent[files.DropEvent](files.EventDropped)
	application.RegisterEvent[files.EditEvent](files.EventEdited)
	application.RegisterEvent[int](eventConfirmClose)
	application.RegisterEvent[[]status.Result](status.EventChanged)
	application.RegisterEvent[application.Void](tunnels.EventChanged)
	application.RegisterEvent[application.Void](hosts.EventHistoryChanged)
	application.RegisterEvent[application.Void](rdp.EventChanged)
	application.RegisterEvent[rdp.FilesEvent](rdp.EventFiles)
	application.RegisterEvent[login.PasswordRequest](login.EventPassword)
	application.RegisterEvent[string](login.EventDone)
}

// eventConfirmClose asks the frontend to confirm closing the window while
// sessions are live; it carries the number of live sessions.
const eventConfirmClose = "app:confirm-close"

func main() {
	// ssh runs this binary as its askpass helper; answer and exit before
	// anything of the app starts.
	if askpass.IsHelper() {
		os.Exit(askpass.Helper(os.Args[1:]))
	}

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
	resolver := sshcmd.Resolver{Store: store, ControlDir: controlDir, RemoteCommand: func(h hosts.Host) string {
		return prefs.Get().Settings.ProfileFor(h.Profile).RemoteCommand()
	}}

	var app *application.App
	fileBrowser, recordDrop := files.NewService(resolver, func() ([]string, error) {
		return app.Dialog.OpenFile().
			CanChooseFiles(true).
			SetTitle("Upload files").
			SetButtonText("Upload").
			PromptForMultipleSelection()
	})
	files.UseEvents(fileBrowser, func(name string, data any) { app.Event.Emit(name, data) })
	hostFiles := hostio.NewService(store, func(title string, filters []hostio.Filter) (string, error) {
		d := app.Dialog.OpenFile().CanChooseFiles(true).SetTitle(title).SetButtonText("Open")
		for _, f := range filters {
			d.AddFilter(f.Name, f.Pattern)
		}
		d.AddFilter("All files", "*")
		return d.PromptForSingleSelection()
	}, func(title, filename string, filters []hostio.Filter) (string, error) {
		opts := &application.SaveFileDialogOptions{Title: title, Filename: filename, ButtonText: "Export", CanCreateDirectories: true}
		for _, f := range filters {
			opts.Filters = append(opts.Filters, application.FileFilter{DisplayName: f.Name, Pattern: f.Pattern})
		}
		d := app.Dialog.SaveFile()
		d.SetOptions(opts)
		return d.PromptForSingleSelection()
	})
	backgroundDir, err := backgrounds.DefaultDir()
	if err != nil {
		log.Fatal(err)
	}
	images := backgrounds.NewService(backgroundDir, func() (string, error) {
		return app.Dialog.OpenFile().CanChooseFiles(true).SetTitle("Background image").SetButtonText("Choose").
			AddFilter("Images", "*.png;*.jpg;*.jpeg;*.webp").PromptForSingleSelection()
	}, func() []string {
		cur := prefs.Get().Settings
		used := []string{cur.Terminal.Background}
		for _, p := range cur.Profiles {
			used = append(used, p.Profile.Background)
		}
		return used
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
	// Saved passwords answer ssh's prompts through a private socket next to
	// the control sockets; without that directory they stay in the terminal.
	keyring := secrets.New()
	logins := login.New(store, keyring,
		func() bool { return prefs.Get().Settings.SavePasswords },
		func(name string, data any) { app.Event.Emit(name, data) },
	)
	login.UseVault(logins, onepassword.Client{})
	store.OnDelete = logins.HostsDeleted
	hostio.UsePasswords(hostFiles, func() bool { return prefs.Get().Settings.SavePasswords }, logins.SavePassword)
	if exe, err := os.Executable(); err != nil || controlDir == "" {
		log.Printf("saved passwords unavailable: no executable path or private socket directory")
	} else if err := logins.Start(controlDir, exe); err != nil {
		log.Printf("saved passwords unavailable: %v", err)
	}

	// Remote desktops open in FreeRDP's own window; saved passwords come from
	// the keyring like ssh's.
	remoteDesktops := rdp.NewService(store, resolver, rdpPasswords{store: store, keyring: keyring, logins: logins,
		enabled: func() bool { return prefs.Get().Settings.SavePasswords }},
		func() string { return prefs.Get().Settings.RDPClient },
		func(name string, data any) { app.Event.Emit(name, data) })

	// Remote desktops in tabs reach their server through a loopback relay.
	if relay, err := rdp.StartRelay(); err != nil {
		log.Printf("remote desktops in tabs unavailable: %v", err)
	} else {
		rdp.UseRelay(remoteDesktops, relay)
	}
	rdp.UseFolderPicker(remoteDesktops, func() (string, error) {
		return app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).
			SetTitle("Folder to share with the remote desktop").SetButtonText("Share").PromptForSingleSelection()
	})
	rdp.UseFilePicker(remoteDesktops, func() ([]string, error) {
		return app.Dialog.OpenFile().CanChooseFiles(true).
			SetTitle("Send files to the remote desktop").SetButtonText("Send").PromptForMultipleSelection()
	})

	terminals := terminal.NewService(func(name string, data any) {
		app.Event.Emit(name, data)
	}, resolver, prefs.Shell)
	terminal.SetAuth(terminals, logins)
	recordingDir, err := recordings.DefaultDir()
	if err != nil {
		log.Fatal(err)
	}
	terminal.SetRecordings(terminals, recordingDir)
	// Connection history: which host, when and for how long, never what was typed or shown.
	if err := store.CloseOpenVisits(); err != nil {
		log.Printf("history: %v", err)
	}
	terminal.SetHistory(terminals, hosts.Recorder{
		Store:   store,
		Enabled: func() bool { return prefs.Get().Settings.History.Enabled },
		Limits: func() hosts.HistoryLimits {
			h := prefs.Get().Settings.History
			return hosts.HistoryLimits{Days: h.Days, Max: h.Max}
		},
		Changed: func() { app.Event.Emit(hosts.EventHistoryChanged) },
	})

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
			application.NewService(logins.Service()),
			application.NewService(hosts.NewService(store)),
			application.NewService(sshconfig.NewService(store, sshConfigPath)),
			application.NewService(hostFiles),
			application.NewService(clips),
			application.NewService(fileBrowser),
			application.NewService(prefs),
			application.NewService(hostStatus),
			application.NewService(hosts.NewSnippetService(store)),
			application.NewService(hosts.NewHistoryService(store)),
			application.NewService(recordings.NewService(recordingDir)),
			application.NewService(forwards),
			application.NewService(updates.NewService()),
			application.NewService(images),
			application.NewService(remoteDesktops),
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
		if target == nil {
			return
		}
		// Files dropped on a remote desktop tab are offered for pasting there.
		if key, ok := strings.CutPrefix(target.ElementID, rdp.DropPrefix); ok {
			paths := e.Context().DroppedFiles()
			go func() {
				offered, err := rdp.Dropped(remoteDesktops, key, paths)
				ev := rdp.FilesEvent{Key: key, Files: offered}
				if err != nil {
					ev.Error = err.Error()
				}
				app.Event.Emit(rdp.EventFiles, ev)
			}()
			return
		}
		if target.ElementID != "file-panel" {
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

// rdpPasswords gives remote desktops their password: from 1Password when the
// host has a reference, else the saved one while "Allow saving passwords" is on.
type rdpPasswords struct {
	store   *hosts.Store
	keyring *secrets.Store
	logins  *login.Manager
	enabled func() bool
}

func (p rdpPasswords) Saved(id int64) (string, error) {
	h, err := p.store.Get(id)
	if err != nil {
		return "", err
	}
	if h.PasswordRef != "" {
		return onepassword.Client{}.Read(context.Background(), h.PasswordRef)
	}
	if !p.enabled() || !h.PasswordSaved {
		return "", nil
	}
	if pw, err := p.keyring.Get(id); err == nil {
		return pw, nil
	}
	return "", nil
}

func (p rdpPasswords) Save(id int64, password string) error {
	if !p.enabled() {
		return login.ErrDisabled
	}
	return p.logins.SavePassword(id, password)
}
