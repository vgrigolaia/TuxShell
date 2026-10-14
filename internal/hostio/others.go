package hostio

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Other tools' formats are turned into a table with csvHeader, so their hosts
// get the same checks and preview as a spreadsheet. Passwords go into the
// PasswordRef column, where anything but an op:// reference is dropped and counted.

const (
	colName = iota
	colGroup
	colHostname
	colUser
	colPort
	colTags
	colProxyJump
	colIdentityFile
	colPassword
	colAlias
)

func newFixedTable() *table { return &table{rows: [][]string{csvHeader}} }

func (t *table) add(set func(row []string)) {
	row := make([]string, len(csvHeader))
	set(row)
	t.rows = append(t.rows, row)
}

// textLines splits text into trimmed lines, decoding UTF-16 (regedit's .reg
// export) and dropping a UTF-8 BOM.
func textLines(data []byte) []string {
	if len(data) >= 2 && (data[0] == 0xFF && data[1] == 0xFE || data[0] == 0xFE && data[1] == 0xFF) {
		data = decodeUTF16(data)
	}
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		out = append(out, strings.TrimSpace(sc.Text()))
	}
	return out
}

func decodeUTF16(b []byte) []byte {
	big := b[0] == 0xFE
	b = b[2:]
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if big {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return []byte(string(utf16.Decode(u)))
}

// --- MobaXterm -------------------------------------------------------------

// readMobaXterm reads a MobaXterm session export (.mxtsessions). Sessions sit
// in [Bookmarks…] sections whose SubRep names the folder; each SSH session is
// "name=#icon#0%host%port%user%…" (type 0 is SSH; other types are skipped).
func readMobaXterm(data []byte) (*table, error) {
	t := newFixedTable()
	inBookmarks, folder := false, ""
	for _, l := range textLines(data) {
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
			sec := strings.ToLower(l[1 : len(l)-1])
			inBookmarks = sec == "bookmarks" || strings.HasPrefix(sec, "bookmarks_")
			folder = ""
			continue
		}
		key, val, ok := strings.Cut(l, "=")
		if !inBookmarks || !ok {
			continue
		}
		switch key {
		case "SubRep":
			folder = strings.ReplaceAll(val, `\`, " / ")
			continue
		case "ImgNum":
			continue
		}
		if !strings.HasPrefix(val, "#") {
			continue
		}
		parts := strings.Split(val, "%")
		head := strings.Split(strings.TrimPrefix(parts[0], "#"), "#")
		if len(head) < 2 || head[1] != "0" || len(parts) < 4 {
			continue
		}
		t.add(func(r []string) {
			r[colName], r[colGroup] = strings.TrimSpace(key), folder
			r[colHostname], r[colPort], r[colUser] = parts[1], parts[2], parts[3]
		})
	}
	if len(t.rows) == 1 {
		return nil, errors.New("no SSH sessions found in this MobaXterm file")
	}
	return t, nil
}

// --- Remmina ---------------------------------------------------------------

// remminaDirs are where Remmina keeps one .remmina file per connection: the
// distribution package and the Flatpak.
func remminaDirs() []string {
	home, _ := os.UserHomeDir()
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	return []string{filepath.Join(data, "remmina"), filepath.Join(home, ".var", "app", "org.remmina.Remmina", "data", "remmina")}
}

func scanRemmina() (*table, string, error) {
	t := newFixedTable()
	found := ""
	for _, dir := range remminaDirs() {
		files, _ := filepath.Glob(filepath.Join(dir, "*.remmina"))
		sort.Strings(files)
		for _, f := range files {
			data, err := readFile(f)
			if err != nil {
				continue
			}
			if addRemmina(t, data) && found == "" {
				found = dir
			}
		}
	}
	if found == "" {
		return nil, "", errors.New("no Remmina SSH connections found in ~/.local/share/remmina")
	}
	return t, found, nil
}

// addRemmina adds one connection file if it's an SSH connection.
func addRemmina(t *table, data []byte) bool {
	v := map[string]string{}
	in := false
	for _, l := range textLines(data) {
		if strings.HasPrefix(l, "[") {
			in = strings.EqualFold(l, "[remmina]")
			continue
		}
		if k, val, ok := strings.Cut(l, "="); in && ok {
			v[strings.ToLower(k)] = val
		}
	}
	if !strings.EqualFold(v["protocol"], "SSH") {
		return false
	}
	server := v["server"]
	if server == "" {
		server = v["ssh_server"]
	}
	user := v["username"]
	if user == "" {
		user = v["ssh_username"]
	}
	t.add(func(r []string) {
		r[colName], r[colGroup], r[colHostname], r[colUser] = v["name"], v["group"], server, user
		r[colIdentityFile] = v["ssh_privatekey"]
	})
	return true
}

// --- PuTTY -----------------------------------------------------------------

// puttySession turns one saved session's settings into a row. PuTTY keys
// (.ppk) can't be used by OpenSSH, so they're not imported.
func puttySession(t *table, name string, v map[string]string) {
	if name == "Default Settings" || (v["protocol"] != "" && !strings.EqualFold(v["protocol"], "ssh")) || v["hostname"] == "" {
		return
	}
	t.add(func(r []string) {
		r[colName], r[colHostname], r[colUser], r[colPort] = name, v["hostname"], v["username"], v["portnumber"]
	})
}

// puttyDir is where PuTTY on Linux saves sessions, one file per session.
func puttyDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".putty", "sessions")
}

func scanPuTTY() (*table, string, error) {
	dir := puttyDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", errors.New("no PuTTY sessions found in ~/.putty/sessions (for sessions from Windows, use the .reg export)")
	}
	t := newFixedTable()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := readFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		v := map[string]string{}
		for _, l := range textLines(data) {
			if k, val, ok := strings.Cut(l, "="); ok {
				v[strings.ToLower(k)] = val
			}
		}
		name, err := url.PathUnescape(e.Name())
		if err != nil {
			name = e.Name()
		}
		puttySession(t, name, v)
	}
	if len(t.rows) == 1 {
		return nil, "", errors.New("no SSH sessions found in ~/.putty/sessions")
	}
	return t, dir, nil
}

// readPuTTYReg reads sessions from a Windows registry export of
// HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions.
func readPuTTYReg(data []byte) (*table, error) {
	t := newFixedTable()
	const marker = `\software\simontatham\putty\sessions\`
	name, v := "", map[string]string{}
	flush := func() {
		if name != "" {
			puttySession(t, name, v)
		}
		name, v = "", map[string]string{}
	}
	for _, l := range textLines(data) {
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
			flush()
			if i := strings.Index(strings.ToLower(l), marker); i >= 0 {
				if n, err := url.PathUnescape(l[i+len(marker) : len(l)-1]); err == nil {
					name = n
				}
			}
			continue
		}
		k, val, ok := strings.Cut(l, "=")
		if name == "" || !ok {
			continue
		}
		k = strings.ToLower(strings.Trim(k, `"`))
		switch {
		case strings.HasPrefix(val, `"`):
			val = strings.ReplaceAll(strings.Trim(val, `"`), `\\`, `\`)
		case strings.HasPrefix(val, "dword:"):
			if n, err := strconv.ParseUint(strings.TrimPrefix(val, "dword:"), 16, 32); err == nil {
				val = strconv.FormatUint(n, 10)
			}
		}
		v[k] = val
	}
	flush()
	if len(t.rows) == 1 {
		return nil, errors.New("no PuTTY SSH sessions found in this .reg file")
	}
	return t, nil
}

// --- Ansible ---------------------------------------------------------------

type ansibleHost struct {
	groups []string
	vars   map[string]string
}

// readAnsibleINI reads an INI inventory: hosts under [group], variables
// inline or in [group:vars]. The first group becomes the host's group and the
// others its tags. Simple numeric ranges (web[01:03]) are expanded.
func readAnsibleINI(data []byte) (*table, error) {
	hosts := map[string]*ansibleHost{}
	var order []string
	groupVars := map[string]map[string]string{}
	section, kind := "ungrouped", ""
	for _, l := range textLines(data) {
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, ";") {
			continue
		}
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
			section, kind, _ = strings.Cut(l[1:len(l)-1], ":")
			continue
		}
		switch kind {
		case "children":
			continue
		case "vars":
			if k, v, ok := strings.Cut(l, "="); ok {
				if groupVars[section] == nil {
					groupVars[section] = map[string]string{}
				}
				groupVars[section][strings.TrimSpace(k)] = unquote(strings.TrimSpace(v))
			}
			continue
		}
		fields := strings.Fields(l)
		names, err := expandRange(fields[0])
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			h := hosts[n]
			if h == nil {
				h = &ansibleHost{vars: map[string]string{}}
				hosts[n] = h
				order = append(order, n)
			}
			h.groups = append(h.groups, section)
			for _, kv := range fields[1:] {
				if k, v, ok := strings.Cut(kv, "="); ok {
					h.vars[k] = unquote(v)
				}
			}
		}
		if len(order) > maxRows {
			return nil, ErrTooLarge
		}
	}
	if len(order) == 0 {
		return nil, errors.New("no hosts found in this inventory")
	}
	t := newFixedTable()
	for _, n := range order {
		h := hosts[n]
		get := func(keys ...string) string {
			for _, k := range keys {
				if v := h.vars[k]; v != "" {
					return v
				}
			}
			for _, g := range h.groups {
				for _, k := range keys {
					if v := groupVars[g][k]; v != "" {
						return v
					}
				}
			}
			for _, k := range keys {
				if v := groupVars["all"][k]; v != "" {
					return v
				}
			}
			return ""
		}
		group := h.groups[0]
		if group == "ungrouped" {
			group = ""
		}
		t.add(func(r []string) {
			r[colName], r[colGroup] = n, group
			r[colHostname] = get("ansible_host", "ansible_ssh_host")
			if r[colHostname] == "" {
				r[colHostname] = n
			}
			r[colUser] = get("ansible_user", "ansible_ssh_user")
			r[colPort] = get("ansible_port", "ansible_ssh_port")
			r[colIdentityFile] = get("ansible_ssh_private_key_file", "ansible_private_key_file")
			r[colPassword] = get("ansible_password", "ansible_ssh_pass", "ansible_ssh_password")
			if len(h.groups) > 1 {
				r[colTags] = strings.Join(h.groups[1:], " ")
			}
		})
	}
	return t, nil
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// expandRange expands one numeric range like "web[01:03].lan". Other
// patterns are left as they are; they fail host validation in the preview.
func expandRange(name string) ([]string, error) {
	open := strings.IndexByte(name, '[')
	end := strings.IndexByte(name, ']')
	if open < 0 || end < open {
		return []string{name}, nil
	}
	lo, hi, ok := strings.Cut(name[open+1:end], ":")
	a, err1 := strconv.Atoi(lo)
	b, err2 := strconv.Atoi(hi)
	if !ok || err1 != nil || err2 != nil || b < a {
		return []string{name}, nil
	}
	if b-a >= maxRows {
		return nil, ErrTooLarge
	}
	out := make([]string, 0, b-a+1)
	for i := a; i <= b; i++ {
		out = append(out, name[:open]+fmt.Sprintf("%0*d", len(lo), i)+name[end+1:])
	}
	return out, nil
}
