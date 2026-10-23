package hostio

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// --- Remote Desktop files (.rdp) -------------------------------------------

// addRDPFile turns one Windows Remote Desktop file into a remote desktop host
// named after the file. Lines are "key:type:value" (often UTF-16). Saved
// passwords ("password 51:b:") are encrypted with the Windows user's own key,
// so they can't be read here and are left out.
func addRDPFile(t *table, filename string, data []byte) bool {
	v := map[string]string{}
	for _, l := range textLines(data) {
		key, rest, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		_, val, ok := strings.Cut(rest, ":")
		if ok {
			v[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(val)
		}
	}
	addr := v["full address"]
	if addr == "" {
		addr = v["alternate full address"]
	}
	host, port := splitHostPort(addr, 0)
	if host == "" {
		return false
	}
	if port == 0 {
		port, _ = strconv.Atoi(v["server port"])
	}

	user, domain := v["username"], v["domain"]
	if d, u, ok := strings.Cut(user, `\`); ok { // DOMAIN\user
		user = u
		if domain == "" {
			domain = d
		}
	}

	r := hosts.DefaultRDP()
	if v["screen mode id"] == "2" {
		r.Window = "fullscreen"
	}
	w, _ := strconv.Atoi(v["desktopwidth"])
	h, _ := strconv.Atoi(v["desktopheight"])
	if v["dynamic resolution"] == "0" && w >= 200 && h >= 200 && w <= 8192 && h <= 8192 {
		r.Size, r.Width, r.Height = "fixed", w, h
	}
	r.Multimon = v["use multimon"] == "1"
	r.Clipboard = v["redirectclipboard"] != "0"
	r.Printers = v["redirectprinters"] == "1"
	r.SmartCards = v["redirectsmartcards"] == "1"
	// Windows drive letters mean nothing here; sharing them all becomes the home folder.
	r.HomeFolder = strings.Contains(v["drivestoredirect"], "*")
	switch v["audiomode"] {
	case "1":
		r.Sound = "remote"
	case "2":
		r.Sound = "off"
	}
	r.Microphone = v["audiocapturemode"] == "1"
	switch v["connection type"] {
	case "1":
		r.Quality = "low"
	case "6":
		r.Quality = "best"
	}
	// gatewayusagemethod 0 means "don't use a gateway".
	if g := v["gatewayhostname"]; g != "" && v["gatewayusagemethod"] != "0" {
		gh, gp := splitHostPort(g, 443)
		r.Gateway = hosts.Gateway{Host: gh, Port: gp}
	}
	if v["remoteapplicationmode"] == "1" && v["remoteapplicationprogram"] != "" {
		r.RemoteApp = hosts.RemoteApp{Program: v["remoteapplicationprogram"], Name: v["remoteapplicationname"], Args: v["remoteapplicationcmdline"]}
	}

	options, _ := json.Marshal(r)
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	t.add(func(row []string) {
		row[colName], row[colHostname], row[colUser], row[colDomain] = name, host, user, domain
		if port > 0 {
			row[colPort] = strconv.Itoa(port)
		}
		row[colProtocol], row[colOS], row[colRDP] = hosts.ProtocolRDP, "windows", string(options)
	})
	return true
}

// splitHostPort reads "host", "host:port" or "[v6]:port"; def when there's no port.
func splitHostPort(s string, def int) (string, int) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		if end := strings.Index(s, "]"); end > 0 {
			port := def
			if p, err := strconv.Atoi(strings.TrimPrefix(s[end+1:], ":")); err == nil {
				port = p
			}
			return s[1:end], port
		}
	}
	if strings.Count(s, ":") == 1 {
		h, p, _ := strings.Cut(s, ":")
		if n, err := strconv.Atoi(p); err == nil {
			return h, n
		}
	}
	return s, def
}
