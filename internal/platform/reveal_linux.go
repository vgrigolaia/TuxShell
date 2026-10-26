package platform

import (
	"net/url"
	"path/filepath"

	"github.com/godbus/dbus/v5"
)

// Reveal shows file selected in its folder: through the desktop's file
// manager (org.freedesktop.FileManager1), or by opening the folder when no
// file manager answers (and in the Flatpak, which can't reach it).
func Reveal(file string) error {
	if !Flatpak && showItems(file) == nil {
		return nil
	}
	return Open(filepath.Dir(file))
}

func showItems(file string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	uri := (&url.URL{Scheme: "file", Path: file}).String()
	obj := conn.Object("org.freedesktop.FileManager1", "/org/freedesktop/FileManager1")
	return obj.Call("org.freedesktop.FileManager1.ShowItems", 0, []string{uri}, "").Err
}
