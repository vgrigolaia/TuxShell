// Package hostio imports hosts from files (CSV, Excel, other tools' exports)
// and exports them.
//
// Files are read and checked here. The frontend only sees a preview: hosts as
// they would be saved, never a password. Plain passwords found in a file are
// counted and dropped; only 1Password references (op://…) are kept.
package hostio

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/vgrigolaia/tuxshell/internal/cliphist"
	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// Source ids the frontend can ask for.
const (
	SourceCSV       = "csv"
	SourceJSON      = "json"
	SourceXLSX      = "xlsx"
	SourceMobaXterm = "mobaxterm"
	SourceRemmina   = "remmina"
	SourcePuTTY     = "putty"
	SourcePuTTYReg  = "puttyreg"
	SourceAnsible   = "ansible"
)

// Duplicate modes for hosts that are already saved.
const (
	DuplicateSkip   = "skip"
	DuplicateUpdate = "update"
	DuplicateCopy   = "copy"
)

// Filter is a file type offered in the file picker; Pattern is ";"-separated.
type Filter struct {
	Name    string
	Pattern string
}

// PickFunc shows a file picker and returns the chosen path, or "" if cancelled.
type PickFunc func(title string, filters []Filter) (string, error)

type source struct {
	title   string
	filters []Filter
	read    func(data []byte) (*table, error)
	// scan reads a folder the tool keeps, returning the table and the folder.
	scan func() (*table, string, error)
	// fixed sources know their columns, so the preview doesn't offer to change them.
	fixed bool
}

var sources = map[string]source{
	SourceCSV: {
		title:   "Import hosts from a CSV or TSV file",
		filters: []Filter{{"CSV or TSV", "*.csv;*.CSV;*.tsv;*.TSV;*.txt;*.TXT"}},
		read:    readDelimited,
	},
	SourceXLSX: {
		title:   "Import hosts from an Excel file",
		filters: []Filter{{"Excel workbook (.xlsx)", "*.xlsx;*.XLSX"}},
		read:    readXLSX,
	},
	SourceJSON: {
		title:   "Import a TuxShell hosts file",
		filters: []Filter{{"TuxShell hosts (JSON)", "*.json;*.JSON"}},
		read:    readTuxShellJSON,
		fixed:   true,
	},
	SourceMobaXterm: {
		title:   "Import MobaXterm sessions",
		filters: []Filter{{"MobaXterm sessions (.mxtsessions)", "*.mxtsessions;*.MXTSESSIONS;*.ini;*.INI"}},
		read:    readMobaXterm,
		fixed:   true,
	},
	SourcePuTTYReg: {
		title:   "Import PuTTY sessions from a Windows .reg export",
		filters: []Filter{{"Registry export (.reg)", "*.reg;*.REG"}},
		read:    readPuTTYReg,
		fixed:   true,
	},
	SourceAnsible: {
		title:   "Import an Ansible inventory",
		filters: []Filter{{"Ansible INI inventory", "*.ini;*.INI;hosts;inventory;*.cfg;*"}},
		read:    readAnsibleINI,
		fixed:   true,
	},
	// These read the tool's own folder; there's no file to pick.
	SourceRemmina: {scan: scanRemmina, fixed: true},
	SourcePuTTY:   {scan: scanPuTTY, fixed: true},
}

// Column is one spreadsheet column and what it fills.
type Column struct {
	Header string `json:"header"`
	Field  string `json:"field"`
	// Sample is the first values, for telling columns apart. Values that look
	// like passwords are replaced with "••••".
	Sample []string `json:"sample"`
}

// Row is one host as it would be imported.
type Row struct {
	// Line is the row's line in the file, counting from 1.
	Line int        `json:"line"`
	Host hosts.Host `json:"host"`
	// Error says why the row can't be imported; Field names the bad field.
	Error string `json:"error"`
	Field string `json:"field"`
	// ExistingID is the saved host with the same address, user and port.
	ExistingID int64  `json:"existingId"`
	Existing   string `json:"existing"`
	// SameAs is the earlier line with the same address, user and port.
	SameAs int `json:"sameAs"`
}

// Preview is what the import window shows before anything is saved.
type Preview struct {
	ID     string `json:"id"`
	File   string `json:"file"`
	Source string `json:"source"`
	// Header is true when the first row holds column names.
	Header bool `json:"header"`
	// Tabular is true for spreadsheets, whose columns the user can remap.
	Tabular bool     `json:"tabular"`
	Columns []Column `json:"columns"`
	Rows    []Row    `json:"rows"`
	// Passwords counts plain passwords found in the file.
	Passwords int `json:"passwords"`
	// SavePasswords is true when they will be saved in the keyring (saving
	// passwords is on); otherwise they're left out.
	SavePasswords bool `json:"savePasswords"`
}

// Result summarises an import.
type Result struct {
	Imported int `json:"imported"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"`
	// Passwords counts passwords saved in the keyring.
	Passwords int      `json:"passwords"`
	Errors    []string `json:"errors"`
}

// ErrStale means the preview was replaced or the window reopened.
var ErrStale = errors.New("this preview is out of date; open the file again")

// Service is bound to the frontend.
type Service struct {
	store *hosts.Store
	pick  PickFunc
	save  SaveFunc
	// Saving imported plain passwords, set with UsePasswords.
	pwEnabled func() bool
	pwSave    func(hostID int64, password string) error

	mu      sync.Mutex
	seq     int
	pending *pending
}

type pending struct {
	id     string
	file   string
	source string
	table  *table
	fields []string
	header bool
}

// NewService imports into and exports from store, asking for files with
// pick and save.
func NewService(store *hosts.Store, pick PickFunc, save SaveFunc) *Service {
	return &Service{store: store, pick: pick, save: save}
}

// UsePasswords lets s save plain passwords from imported files through save
// while enabled() is true. A function rather than a method so it isn't bound
// to the frontend.
func UsePasswords(s *Service, enabled func() bool, save func(hostID int64, password string) error) {
	s.pwEnabled, s.pwSave = enabled, save
}

func (s *Service) passwordsOn() bool { return s.pwEnabled != nil && s.pwSave != nil && s.pwEnabled() }

// Open asks for a file of the given source and previews it. It returns nil
// when the picker was cancelled.
func (s *Service) Open(sourceID string) (*Preview, error) {
	src, ok := sources[sourceID]
	if !ok {
		return nil, fmt.Errorf("unknown import source %q", sourceID)
	}
	if src.scan != nil {
		t, dir, err := src.scan()
		if err != nil {
			return nil, err
		}
		return s.adopt(sourceID, dir, t)
	}
	path, err := s.pick(src.title, src.filters)
	if err != nil || path == "" {
		return nil, err
	}
	return s.load(sourceID, path)
}

func (s *Service) load(sourceID, path string) (*Preview, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	t, err := sources[sourceID].read(data)
	if err != nil {
		return nil, err
	}
	return s.adopt(sourceID, path, t)
}

func (s *Service) adopt(sourceID, path string, t *table) (*Preview, error) {
	if len(t.rows) == 0 {
		return nil, errors.New("this file has no rows")
	}
	fields, header := t.guess()
	s.mu.Lock()
	s.seq++
	p := &pending{id: strconv.Itoa(s.seq), file: filepath.Base(path), source: sourceID, table: t, fields: fields, header: header}
	s.pending = p
	s.mu.Unlock()
	return s.preview(p)
}

// Remap changes what each column fills and whether the first row is a header.
func (s *Service) Remap(id string, fields []string, header bool) (*Preview, error) {
	s.mu.Lock()
	p := s.pending
	if p == nil || p.id != id {
		s.mu.Unlock()
		return nil, ErrStale
	}
	if sources[p.source].fixed {
		s.mu.Unlock()
		return s.preview(p)
	}
	valid := map[string]bool{}
	for _, f := range Fields {
		valid[f] = true
	}
	next := make([]string, len(p.fields))
	used := map[string]bool{}
	for i := range next {
		if i < len(fields) && valid[fields[i]] && (fields[i] == FieldIgnore || !used[fields[i]]) {
			next[i] = fields[i]
			used[fields[i]] = true
		}
	}
	p.fields, p.header = next, header
	s.mu.Unlock()
	return s.preview(p)
}

func (s *Service) preview(p *pending) (*Preview, error) {
	existing, err := s.store.List()
	if err != nil {
		return nil, err
	}
	pv := &Preview{ID: p.id, File: p.file, Source: p.source, Header: p.header, Tabular: !sources[p.source].fixed}
	pv.Columns = columns(p)
	var plain map[int]string
	pv.Rows, plain = buildRows(p, existing)
	pv.Passwords = len(plain)
	pv.SavePasswords = s.passwordsOn()
	return pv, nil
}

func columns(p *pending) []Column {
	cols := make([]Column, len(p.fields))
	start := 0
	if p.header {
		start = 1
	}
	for i := range cols {
		c := Column{Field: p.fields[i], Sample: []string{}}
		if p.header && i < len(p.table.rows[0]) {
			c.Header = p.table.rows[0][i]
		}
		hidden := c.Field == FieldPassword || headerNames[normalizeHeader(c.Header)] == FieldPassword
		for _, r := range p.table.rows[start:] {
			if len(c.Sample) == 3 {
				break
			}
			if i >= len(r) || r[i] == "" {
				continue
			}
			v := r[i]
			if hidden || cliphist.Sensitive(v) {
				v = "••••"
			} else if len(v) > 40 {
				v = v[:40] + "…"
			}
			c.Sample = append(c.Sample, v)
		}
		cols[i] = c
	}
	return cols
}

// key identifies a connection: the same address, user and port.
func key(h hosts.Host) string {
	addr := h.Hostname
	if addr == "" {
		addr = h.Alias
	}
	port := h.Port
	if port == 0 {
		port = 22
	}
	return strings.ToLower(addr) + "\x00" + h.User + "\x00" + strconv.Itoa(port)
}

// buildRows previews the pending table. plain holds plain passwords by line;
// they're kept here in Go for Import and never put in a Row.
func buildRows(p *pending, existing []hosts.Host) (rows []Row, plain map[int]string) {
	saved := map[string]hosts.Host{}
	taken := map[string]bool{}
	for _, h := range existing {
		if _, ok := saved[key(h)]; !ok {
			saved[key(h)] = h
		}
		taken[strings.ToLower(h.Name)] = true
	}
	seen := map[string]int{}
	rows = []Row{}
	plain = map[int]string{}
	start := 0
	if p.header {
		start = 1
	}
	type parsed struct {
		line int
		host hosts.Host
		err  error
	}
	var list []parsed
	for i, cells := range p.table.rows[start:] {
		// Lines starting with # are notes, like the ones in the template.
		if len(cells) > 0 && strings.HasPrefix(cells[0], "#") {
			continue
		}
		h, pw, err := rowHost(cells, p.fields)
		if pw != "" {
			plain[start+i+1] = pw
		}
		if h.Name == "" {
			h.Name = h.Hostname
		}
		list = append(list, parsed{start + i + 1, h, err})
	}
	// A bastion may be given by name: a host in the file or one already saved.
	byName := map[string]hosts.Host{}
	for _, h := range existing {
		byName[strings.ToLower(h.Name)] = h
	}
	for _, x := range list {
		if x.err == nil && x.host.Name != "" {
			byName[strings.ToLower(x.host.Name)] = x.host
		}
	}
	for _, x := range list {
		r := Row{Line: x.line}
		h, err := x.host, x.err
		h.ProxyJump = resolveJump(h.Name, h.ProxyJump, byName, 0)
		h.Normalize()
		if err == nil {
			err = h.Validate()
		}
		var ve *hosts.ValidationError
		if errors.As(err, &ve) {
			r.Error, r.Field = ve.Message, ve.Field
		} else if err != nil {
			r.Error = err.Error()
		}
		if r.Error == "" {
			k := key(h)
			if old, ok := saved[k]; ok {
				r.ExistingID, r.Existing = old.ID, old.Name
			} else if line, ok := seen[k]; ok {
				r.SameAs = line
			} else {
				seen[k] = r.Line
				h.Name = uniqueName(h.Name, taken)
			}
		}
		r.Host = h
		rows = append(rows, r)
	}
	return rows, plain
}

// resolveJump replaces hops that name a known host with that host's address,
// keeping the host's own jump hosts in front, so plain ssh can follow them.
func resolveJump(self, jump string, byName map[string]hosts.Host, depth int) string {
	if jump == "" || depth > 4 {
		return jump
	}
	var hops []string
	for _, hop := range strings.Split(jump, ",") {
		hop = strings.TrimSpace(hop)
		b, ok := byName[strings.ToLower(hop)]
		if !ok || strings.EqualFold(hop, self) || (b.Hostname == "" && b.Alias == "") {
			hops = append(hops, hop)
			continue
		}
		if inner := resolveJump(b.Name, b.ProxyJump, byName, depth+1); inner != "" {
			hops = append(hops, inner)
		}
		hops = append(hops, address(b))
	}
	return strings.Join(hops, ",")
}

// address is [user@]host[:port] for ssh -J, with IPv6 in brackets.
func address(h hosts.Host) string {
	host := h.Hostname
	if host == "" {
		host = h.Alias
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if h.User != "" {
		host = h.User + "@" + host
	}
	if h.Port != 0 && h.Port != 22 {
		host += ":" + strconv.Itoa(h.Port)
	}
	return host
}

// uniqueName returns name, or "name (2)" and so on when it's taken, and marks
// the result as taken.
func uniqueName(name string, taken map[string]bool) string {
	n := name
	for i := 2; taken[strings.ToLower(n)]; i++ {
		n = fmt.Sprintf("%s (%d)", name, i)
	}
	taken[strings.ToLower(n)] = true
	return n
}

// Import saves the chosen lines of the preview. Hosts without a group go into
// group. duplicate says what to do with hosts that are already saved.
func (s *Service) Import(id string, lines []int, duplicate, group string) (Result, error) {
	s.mu.Lock()
	p := s.pending
	s.mu.Unlock()
	if p == nil || p.id != id {
		return Result{}, ErrStale
	}
	existing, err := s.store.List()
	if err != nil {
		return Result{}, err
	}
	taken := map[string]bool{}
	for _, h := range existing {
		taken[strings.ToLower(h.Name)] = true
	}
	rows, plain := buildRows(p, existing)
	savePw := s.passwordsOn()
	want := map[int]bool{}
	for _, l := range lines {
		want[l] = true
	}
	group = strings.TrimSpace(group)

	res := Result{Errors: []string{}}
	fail := func(r Row, msg string) {
		res.Skipped++
		res.Errors = append(res.Errors, fmt.Sprintf("Line %d: %s", r.Line, msg))
	}
	// keepPassword saves the row's plain password for the stored host.
	keepPassword := func(r Row, hostID int64) {
		pw := plain[r.Line]
		if !savePw || pw == "" {
			return
		}
		if err := s.pwSave(hostID, pw); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("Line %d: the password wasn't saved: %v", r.Line, err))
			return
		}
		res.Passwords++
	}
	save := func(r Row, h hosts.Host) error {
		saved, err := s.store.Save(h)
		var ve *hosts.ValidationError
		if errors.As(err, &ve) {
			fail(r, ve.Message)
			return nil
		}
		if err == nil {
			res.Imported++
			keepPassword(r, saved.ID)
		}
		return err
	}
	// New hosts already have free names from the preview. Copies of saved
	// hosts are named after them, once the new names are taken.
	for _, r := range rows {
		if want[r.Line] && r.Error == "" && r.SameAs == 0 && r.ExistingID == 0 {
			taken[strings.ToLower(r.Host.Name)] = true
		}
	}
	var copies []Row
	for _, r := range rows {
		if !want[r.Line] {
			continue
		}
		h := r.Host
		if h.Group == "" {
			h.Group = group
		}
		r.Host = h
		switch {
		case r.Error != "":
			fail(r, r.Error)
		case r.SameAs != 0:
			res.Skipped++
		case r.ExistingID != 0 && duplicate == DuplicateUpdate:
			old, err := s.store.Get(r.ExistingID)
			if err == nil {
				_, err = s.store.Save(merge(old, h))
			}
			if err != nil {
				fail(r, err.Error())
			} else {
				res.Updated++
				keepPassword(r, r.ExistingID)
			}
		case r.ExistingID != 0 && duplicate == DuplicateCopy:
			copies = append(copies, r)
		case r.ExistingID != 0:
			res.Skipped++
		default:
			if err := save(r, h); err != nil {
				return res, err
			}
		}
	}
	for _, r := range copies {
		r.Host.Name = uniqueName(r.Host.Name, taken)
		if err := save(r, r.Host); err != nil {
			return res, err
		}
	}
	s.mu.Lock()
	if s.pending == p {
		s.pending = nil
	}
	s.mu.Unlock()
	return res, nil
}

// merge fills old with the fields the file gave. The saved host keeps its
// name; empty cells never clear a field.
func merge(old, h hosts.Host) hosts.Host {
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&old.Group, h.Group)
	set(&old.ProxyJump, h.ProxyJump)
	set(&old.IdentityFile, h.IdentityFile)
	set(&old.PasswordRef, h.PasswordRef)
	set(&old.OS, h.OS)
	seen := map[string]bool{}
	for _, t := range old.Tags {
		seen[t] = true
	}
	for _, t := range h.Tags {
		if !seen[t] {
			old.Tags = append(old.Tags, t)
		}
	}
	return old
}

// Cancel forgets the open preview.
func (s *Service) Cancel() {
	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()
}
