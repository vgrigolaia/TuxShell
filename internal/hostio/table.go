package hostio

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/keepassxc"
)

// Field names a host field a spreadsheet column can fill. "" ignores the column.
const (
	FieldIgnore       = ""
	FieldName         = "name"
	FieldGroup        = "group"
	FieldHostname     = "hostname"
	FieldUser         = "user"
	FieldPort         = "port"
	FieldProxyJump    = "proxyJump"
	FieldIdentityFile = "identityFile"
	FieldTags         = "tags"
	// FieldAlias is a ~/.ssh/config Host alias that ssh connects through.
	FieldAlias = "alias"
	// FieldOS is free text ("Ubuntu 24.04"); unrecognised values become a tag.
	FieldOS = "os"
	// FieldPassword keeps 1Password references (op://…) on the host. Plain
	// passwords go to the keyring when saving passwords is on; otherwise
	// they're dropped. They never reach the frontend.
	FieldPassword = "password"
	// FieldProtocol is "SSH" or "RDP" (remote desktop); empty means SSH.
	FieldProtocol = "protocol"
	// FieldDomain is a remote desktop's Windows domain.
	FieldDomain = "domain"
	// FieldRDP carries a remote desktop's options as JSON (our own exports
	// and other apps' files; not offered for spreadsheets).
	FieldRDP = "rdpOptions"
)

// Fields lists the choices offered for each column, in display order.
var Fields = []string{FieldIgnore, FieldName, FieldHostname, FieldUser, FieldPort, FieldGroup, FieldOS, FieldTags, FieldProxyJump, FieldIdentityFile, FieldAlias, FieldPassword, FieldProtocol, FieldDomain}

// headerNames maps normalized header text to a field. Normalized means lower
// case with everything but letters and digits removed ("User Name" → "username").
var headerNames = map[string]string{
	"name": FieldName, "label": FieldName, "title": FieldName, "session": FieldName,
	"sessionname": FieldName, "displayname": FieldName, "servername": FieldName, "connection": FieldName,

	"host": FieldHostname, "hostname": FieldHostname, "ip": FieldHostname, "ipaddress": FieldHostname,
	"address": FieldHostname, "server": FieldHostname, "serveraddress": FieldHostname, "fqdn": FieldHostname,
	"hostaddress": FieldHostname, "hostip": FieldHostname, "remotehost": FieldHostname,

	"user": FieldUser, "username": FieldUser, "login": FieldUser, "loginname": FieldUser,
	"account": FieldUser, "userid": FieldUser,

	"port": FieldPort, "sshport": FieldPort,

	"group": FieldGroup, "folder": FieldGroup, "category": FieldGroup, "environment": FieldGroup,
	"env": FieldGroup, "project": FieldGroup,

	"tags": FieldTags, "tag": FieldTags, "labels": FieldTags,

	"jump": FieldProxyJump, "jumphost": FieldProxyJump, "proxyjump": FieldProxyJump,
	"bastion": FieldProxyJump, "via": FieldProxyJump, "gateway": FieldProxyJump,

	"key": FieldIdentityFile, "keyfile": FieldIdentityFile, "identityfile": FieldIdentityFile,
	"privatekey": FieldIdentityFile, "sshkey": FieldIdentityFile, "identity": FieldIdentityFile,

	"alias": FieldAlias, "sshalias": FieldAlias,

	"os": FieldOS, "ostype": FieldOS, "operatingsystem": FieldOS, "system": FieldOS, "platform": FieldOS,
	"distro": FieldOS, "distribution": FieldOS,

	"keyfilepath": FieldIdentityFile, "sshkeyfile": FieldIdentityFile,
	"jumpserver": FieldProxyJump, "bastionhost": FieldProxyJump,
	"1password": FieldPassword, "1passwordreference": FieldPassword,

	"password": FieldPassword, "pass": FieldPassword, "passwd": FieldPassword, "pwd": FieldPassword,
	"secret": FieldPassword, "passwordref": FieldPassword,

	"protocol": FieldProtocol, "type": FieldProtocol, "connectiontype": FieldProtocol, "kind": FieldProtocol,
	"domain": FieldDomain, "windowsdomain": FieldDomain, "addomain": FieldDomain,
	"rdpoptions": FieldRDP,
}

// table is a spreadsheet read into rows of cells.
type table struct {
	rows [][]string
}

// maxRows keeps a mistaken pick (a huge log, say) from freezing the preview.
const maxRows = 5000

// maxFileBytes is the most a host file may weigh.
const maxFileBytes = 20 << 20

// ErrTooLarge is returned for files over maxFileBytes or maxRows.
var ErrTooLarge = errors.New("this file is too large to be a host list")

func readFile(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > maxFileBytes {
		return nil, ErrTooLarge
	}
	return os.ReadFile(path)
}

// readDelimited reads CSV or TSV, guessing the delimiter from the first line.
func readDelimited(data []byte) (*table, error) {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF")) // Excel writes a BOM
	first, _, _ := bytes.Cut(data, []byte("\n"))
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sniffDelimiter(string(first))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	t := &table{}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("couldn't read this file as CSV: %w", err)
		}
		if len(t.rows) == maxRows+1 {
			return nil, ErrTooLarge
		}
		t.rows = append(t.rows, rec)
	}
	t.trim()
	return t, nil
}

// sniffDelimiter picks whichever of tab, semicolon and comma the header uses
// most. LibreOffice and Excel in many locales save CSV with semicolons.
func sniffDelimiter(line string) rune {
	best, n := ',', strings.Count(line, ",")
	for _, d := range []rune{';', '\t'} {
		if c := strings.Count(line, string(d)); c > n {
			best, n = d, c
		}
	}
	return best
}

// trim drops blank rows and trailing blank columns.
func (t *table) trim() {
	rows := t.rows[:0]
	for _, r := range t.rows {
		for i := range r {
			r[i] = strings.TrimSpace(r[i])
		}
		for len(r) > 0 && r[len(r)-1] == "" {
			r = r[:len(r)-1]
		}
		if len(r) > 0 {
			rows = append(rows, r)
		}
	}
	t.rows = rows
}

func (t *table) width() int {
	w := 0
	for _, r := range t.rows {
		w = max(w, len(r))
	}
	return w
}

func normalizeHeader(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// guess maps the first row's headers to fields. It reports whether the first
// row looks like a header at all (it names at least a host column).
func (t *table) guess() (fields []string, header bool) {
	fields = make([]string, t.width())
	if len(t.rows) == 0 {
		return fields, false
	}
	used := map[string]bool{}
	for i, h := range t.rows[0] {
		f := headerNames[normalizeHeader(h)]
		if f != "" && !used[f] {
			fields[i], used[f] = f, true
		}
	}
	return fields, used[FieldHostname]
}

// rowHost builds a host from one row. plain is a plain (not op://) password
// found in it, or "".
func rowHost(cells []string, fields []string) (h hosts.Host, plain string, err error) {
	h.Tags = []string{}
	h.RDP = hosts.DefaultRDP()
	// The options first, so a Domain column can still override theirs.
	for i, f := range fields {
		if f == FieldRDP && i < len(cells) && cells[i] != "" {
			if json.Unmarshal([]byte(cells[i]), &h.RDP) != nil {
				return h, plain, &hosts.ValidationError{Field: "rdp", Message: "The remote desktop options can't be read"}
			}
		}
	}
	for i, f := range fields {
		if i >= len(cells) || cells[i] == "" {
			continue
		}
		v := cells[i]
		switch f {
		case FieldProtocol:
			switch p := strings.ToLower(strings.TrimSpace(v)); {
			case p == "ssh":
				h.Protocol = hosts.ProtocolSSH
			case strings.Contains(p, "rdp") || strings.Contains(p, "remote desktop") || p == "mstsc":
				h.Protocol = hosts.ProtocolRDP
			default:
				return h, plain, &hosts.ValidationError{Field: "protocol", Message: fmt.Sprintf("Type %q: use SSH or RDP", v)}
			}
		case FieldDomain:
			h.RDP.Domain = v
		case FieldName:
			h.Name = v
		case FieldGroup:
			h.Group = v
		case FieldHostname:
			h.Hostname = v
		case FieldUser:
			h.User = v
		case FieldPort:
			p, perr := strconv.Atoi(v)
			if perr != nil {
				return h, plain, &hosts.ValidationError{Field: "port", Message: fmt.Sprintf("Port %q is not a number", v)}
			}
			h.Port = p
		case FieldProxyJump:
			h.ProxyJump = v
		case FieldIdentityFile:
			h.IdentityFile = v
		case FieldTags:
			h.Tags = append(h.Tags, splitTags(v)...)
		case FieldAlias:
			h.Alias = v
		case FieldOS:
			if h.OS = hosts.ParseOS(v); h.OS == "" {
				h.Tags = append(h.Tags, strings.Fields(strings.ToLower(v))[0])
			}
		case FieldPassword:
			if strings.HasPrefix(v, "op://") || keepassxc.IsRef(v) {
				h.PasswordRef = v
			} else {
				plain = v
			}
		}
	}
	splitAddress(&h)
	return h, plain, nil
}

func splitTags(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '|' })
}

// splitAddress turns "user@host:port" in the host cell into its parts when
// those parts weren't given in their own columns.
func splitAddress(h *hosts.Host) {
	if u, rest, ok := strings.Cut(h.Hostname, "@"); ok && h.User == "" {
		h.User, h.Hostname = u, rest
	}
	// A single colon is host:port; more than one is an IPv6 address.
	if host, port, ok := strings.Cut(h.Hostname, ":"); ok && !strings.Contains(port, ":") && h.Port == 0 {
		if p, err := strconv.Atoi(port); err == nil {
			h.Hostname, h.Port = host, p
		}
	}
}
