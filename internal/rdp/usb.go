package rdp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// usbRoot is where Linux lists USB devices (also readable in the Flatpak).
const usbRoot = "/sys/bus/usb/devices"

// USBDevices lists the USB devices plugged into this computer that can be
// passed to a remote desktop, by vendor:product id. Hubs are left out. On
// systems without /sys (macOS) the list is empty.
func (s *Service) USBDevices() []hosts.USBDevice {
	return listUSB(usbRoot)
}

func listUSB(root string) []hosts.USBDevice {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []hosts.USBDevice{}
	}
	read := func(dir, name string) string {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		return strings.TrimSpace(string(b))
	}
	seen := map[string]bool{}
	out := []hosts.USBDevice{}
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		vendor, product := strings.ToLower(read(dir, "idVendor")), strings.ToLower(read(dir, "idProduct"))
		// Interfaces ("1-1:1.0") have no ids; hubs (class 09) aren't passed through.
		if len(vendor) != 4 || len(product) != 4 || read(dir, "bDeviceClass") == "09" {
			continue
		}
		id := vendor + ":" + product
		if seen[id] {
			continue
		}
		seen[id] = true
		name := strings.TrimSpace(read(dir, "manufacturer") + " " + read(dir, "product"))
		if name == "" {
			name = "USB device"
		}
		// FreeRDP separates its options with commas, so names never carry one.
		name = strings.Join(strings.Fields(strings.ReplaceAll(name, ",", " ")), " ")
		out = append(out, hosts.USBDevice{ID: id, Name: clip(name, 100)})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n])
}
