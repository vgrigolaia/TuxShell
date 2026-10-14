package hostio

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// Export formats.
const (
	FormatJSON      = "json"
	FormatCSV       = "csv"
	FormatSSHConfig = "sshconfig"
)

// SaveFunc shows a save dialog and returns the chosen path, or "" if cancelled.
type SaveFunc func(title, filename string, filters []Filter) (string, error)

// exportFile is the TuxShell JSON format. Version changes only when old files
// can no longer be read as they are.
type exportFile struct {
	TuxShell int          `json:"tuxshell"`
	Exported time.Time    `json:"exported"`
	Hosts    []exportHost `json:"hosts"`
}

type exportHost struct {
	Name         string   `json:"name"`
	Group        string   `json:"group,omitempty"`
	Alias        string   `json:"alias,omitempty"`
	Hostname     string   `json:"hostname,omitempty"`
	User         string   `json:"user,omitempty"`
	Port         int      `json:"port,omitempty"`
	ProxyJump    string   `json:"proxyJump,omitempty"`
	IdentityFile string   `json:"identityFile,omitempty"`
	PasswordRef  string   `json:"passwordRef,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	OS           string   `json:"os,omitempty"`
	// Protocol and RDP are set for remote desktops only.
	Protocol string     `json:"protocol,omitempty"`
	RDP      *hosts.RDP `json:"rdp,omitempty"`
}

// csvHeader is also read back by the CSV import (see headerNames).
var csvHeader = []string{"Name", "Group", "Hostname", "User", "Port", "Tags", "ProxyJump", "IdentityFile", "PasswordRef", "Alias", "OS", "Protocol", "Domain", "RDPOptions"}

var formats = map[string]struct {
	ext    string
	filter Filter
}{
	FormatJSON:      {".json", Filter{"TuxShell hosts (JSON)", "*.json"}},
	FormatCSV:       {".csv", Filter{"CSV", "*.csv"}},
	FormatSSHConfig: {".conf", Filter{"SSH config", "*.conf;config"}},
}

// ErrNothingToExport is returned when none of the given hosts exist.
var ErrNothingToExport = errors.New("choose at least one host to export")

// Export writes the hosts with the given ids to a file the user picks. It
// returns the path, or "" when the dialog was cancelled.
func (s *Service) Export(format string, ids []int64) (string, error) {
	f, ok := formats[format]
	if !ok {
		return "", fmt.Errorf("unknown export format %q", format)
	}
	all, err := s.store.List()
	if err != nil {
		return "", err
	}
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	list := []hosts.Host{}
	for _, h := range all {
		if want[h.ID] {
			list = append(list, h)
		}
	}
	if len(list) == 0 {
		return "", ErrNothingToExport
	}
	data, err := encode(format, list, time.Now())
	if err != nil {
		return "", err
	}
	name := "tuxshell-hosts-" + time.Now().Format("2006-01-02") + f.ext
	path, err := s.save("Export hosts", name, []Filter{f.filter})
	if err != nil || path == "" {
		return "", err
	}
	// The Linux save dialog doesn't fill in a name, so add the extension if it was left off.
	if filepath.Ext(path) == "" {
		path += f.ext
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	// WriteFile keeps the mode of a file it overwrites.
	return path, os.Chmod(path, 0o600)
}

func encode(format string, list []hosts.Host, now time.Time) ([]byte, error) {
	switch format {
	case FormatJSON:
		out := exportFile{TuxShell: 1, Exported: now.UTC().Truncate(time.Second), Hosts: []exportHost{}}
		for _, h := range list {
			port := h.Port
			if port == 22 {
				port = 0
			}
			e := exportHost{h.Name, h.Group, h.Alias, h.Hostname, h.User, port, h.ProxyJump, h.IdentityFile, h.PasswordRef, h.Tags, h.OS, "", nil}
			if h.Protocol == hosts.ProtocolRDP {
				r := h.RDP
				e.Protocol, e.RDP = hosts.ProtocolRDP, &r
			}
			out.Hosts = append(out.Hosts, e)
		}
		b, err := json.MarshalIndent(out, "", "  ")
		return append(b, '\n'), err
	case FormatCSV:
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write(csvHeader)
		for _, h := range list {
			protocol, domain, options := "SSH", "", ""
			if h.Protocol == hosts.ProtocolRDP {
				b, _ := json.Marshal(h.RDP)
				protocol, domain, options = "RDP", h.RDP.Domain, string(b)
			}
			_ = w.Write([]string{h.Name, h.Group, h.Hostname, h.User, strconv.Itoa(h.Port), strings.Join(h.Tags, " "), h.ProxyJump, h.IdentityFile, h.PasswordRef, h.Alias, h.OS, protocol, domain, options})
		}
		w.Flush()
		return buf.Bytes(), w.Error()
	case FormatSSHConfig:
		return sshConfig(list, now), nil
	}
	return nil, fmt.Errorf("unknown export format %q", format)
}

var aliasUnsafe = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// sshConfig writes one Host block per host. Names become aliases with spaces
// and other characters ssh_config can't take replaced by "-".
func sshConfig(list []hosts.Host, now time.Time) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Exported from TuxShell on %s.\n# Include it from ~/.ssh/config, or copy the blocks you need.\n", now.Format("2006-01-02"))
	used := map[string]bool{}
	for _, h := range list {
		if h.Protocol == hosts.ProtocolRDP {
			continue // ssh can't open remote desktops
		}
		alias := h.Alias
		if alias == "" {
			alias = strings.Trim(aliasUnsafe.ReplaceAllString(h.Name, "-"), "-.")
		}
		if alias == "" {
			alias = h.Hostname
		}
		base := alias
		for i := 2; used[alias]; i++ {
			alias = fmt.Sprintf("%s-%d", base, i)
		}
		used[alias] = true
		fmt.Fprintf(&b, "\nHost %s\n", alias)
		if h.Group != "" {
			fmt.Fprintf(&b, "    # Group: %s\n", strings.ReplaceAll(h.Group, "\n", " "))
		}
		if h.Hostname != "" {
			fmt.Fprintf(&b, "    HostName %s\n", h.Hostname)
		}
		if h.User != "" {
			fmt.Fprintf(&b, "    User %s\n", h.User)
		}
		if h.Port != 0 && h.Port != 22 {
			fmt.Fprintf(&b, "    Port %d\n", h.Port)
		}
		if h.ProxyJump != "" {
			fmt.Fprintf(&b, "    ProxyJump %s\n", h.ProxyJump)
		}
		if h.IdentityFile != "" {
			fmt.Fprintf(&b, "    IdentityFile %s\n", quoteIfSpaced(h.IdentityFile))
		}
	}
	return []byte(b.String())
}

func quoteIfSpaced(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}

// readTuxShellJSON turns an export back into a table with our CSV header, so
// it goes through the same checks as a spreadsheet.
func readTuxShellJSON(data []byte) (*table, error) {
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil || f.TuxShell == 0 {
		return nil, errors.New("this isn't a TuxShell hosts file")
	}
	if f.TuxShell > 1 {
		return nil, errors.New("this file is from a newer TuxShell; update TuxShell to import it")
	}
	if len(f.Hosts) > maxRows {
		return nil, ErrTooLarge
	}
	t := &table{rows: [][]string{csvHeader}}
	for _, h := range f.Hosts {
		port := ""
		if h.Port != 0 {
			port = strconv.Itoa(h.Port)
		}
		protocol, domain, options := h.Protocol, "", ""
		if h.RDP != nil {
			b, _ := json.Marshal(h.RDP)
			domain, options = h.RDP.Domain, string(b)
		}
		t.rows = append(t.rows, []string{h.Name, h.Group, h.Hostname, h.User, port, strings.Join(h.Tags, " "), h.ProxyJump, h.IdentityFile, h.PasswordRef, h.Alias, h.OS, protocol, domain, options})
	}
	return t, nil
}

// templateCSV is a starting point for people who keep hosts in a spreadsheet.
// Its headers are all recognised by the import, and lines starting with # are
// skipped, so the notes can stay in the file.
const templateCSV = `Name,Host,User,Password,Port,Group,OS,Tags,Bastion,Key file,Type,Domain
"# Only Host is required; delete these example rows. Lines starting with # are skipped.",,,,,,,,,,,
"# Host: a name or IP address (user@host:port works too). Port: 22 if empty (3389 for RDP). Tags: separated by spaces.",,,,,,,,,,,
"# OS: e.g. Ubuntu, Debian, RHEL, AlmaLinux, Rocky, Fedora, CentOS, openSUSE, Arch, Alpine, FreeBSD, Windows.",,,,,,,,,,,
"# Bastion: the Name of another row (or user@host:port) to connect through. Key file: a path like ~/.ssh/id_ed25519.",,,,,,,,,,,
"# Password: saved encrypted in your system keyring if you allow it (Preferences > Security). 1Password references (op://...) always work.",,,,,,,,,,,
"# Type: SSH (or empty) for a terminal, RDP for a remote desktop. Domain: the Windows domain of an RDP host.",,,,,,,,,,,
bastion-01,203.0.113.10,admin,,22,Jump,Ubuntu,gateway,,~/.ssh/id_ed25519,,
db-01,10.0.0.7,postgres,op://Servers/db-01/password,2222,Production,Rocky Linux,db primary,bastion-01,,,
web-01,10.0.0.5,deploy,,,Production,Debian,web,bastion-01,,,
win-01,10.0.5.4,Administrator,,,Windows,Windows,ad,bastion-01,,RDP,CORP
`

// SaveTemplate writes the CSV template to a file the user picks. It returns
// the path, or "" when the dialog was cancelled.
func (s *Service) SaveTemplate() (string, error) {
	path, err := s.save("Save the host list template", "tuxshell-hosts-template.csv", []Filter{{"CSV", "*.csv"}})
	if err != nil || path == "" {
		return "", err
	}
	if filepath.Ext(path) == "" {
		path += ".csv"
	}
	return path, os.WriteFile(path, []byte(templateCSV), 0o644)
}
