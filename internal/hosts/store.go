package hosts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/vgrigolaia/tuxshell/internal/platform"
)

// Store persists hosts in a SQLite database.
type Store struct {
	db *sql.DB
	// OnDelete, when set, is called with the IDs of hosts that were deleted
	// (to remove their saved passwords).
	OnDelete func(ids []int64)
}

// migrations run in order; PRAGMA user_version records how many have been applied.
var migrations = []string{
	`CREATE TABLE hosts (
		id            INTEGER PRIMARY KEY,
		name          TEXT NOT NULL UNIQUE COLLATE NOCASE,
		grp           TEXT NOT NULL DEFAULT '',
		alias         TEXT NOT NULL DEFAULT '',
		hostname      TEXT NOT NULL DEFAULT '',
		user          TEXT NOT NULL DEFAULT '',
		port          INTEGER NOT NULL DEFAULT 22,
		proxy_jump    TEXT NOT NULL DEFAULT '',
		identity_file TEXT NOT NULL DEFAULT '',
		password_ref  TEXT NOT NULL DEFAULT '',
		tags          TEXT NOT NULL DEFAULT '[]',
		created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE snippets (
		id         INTEGER PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
		command    TEXT NOT NULL,
		grp        TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE tunnels (
		id          INTEGER PRIMARY KEY,
		name        TEXT NOT NULL DEFAULT '',
		host_id     INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
		kind        TEXT NOT NULL,
		bind_port   INTEGER NOT NULL,
		target_host TEXT NOT NULL DEFAULT '',
		target_port INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`ALTER TABLE hosts ADD COLUMN os TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE hosts ADD COLUMN password_saved INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE hosts ADD COLUMN profile TEXT NOT NULL DEFAULT ''`,
	`CREATE TABLE history (
		id      INTEGER PRIMARY KEY,
		host_id INTEGER NOT NULL DEFAULT 0,
		name    TEXT NOT NULL DEFAULT '',
		target  TEXT NOT NULL DEFAULT '',
		started INTEGER NOT NULL,
		ended   INTEGER NOT NULL DEFAULT 0,
		result  TEXT NOT NULL DEFAULT 'open'
	)`,
	`CREATE INDEX history_started ON history (started)`,
	`ALTER TABLE hosts ADD COLUMN protocol TEXT NOT NULL DEFAULT 'ssh'`,
	`ALTER TABLE hosts ADD COLUMN rdp TEXT NOT NULL DEFAULT '{}'`,
	`ALTER TABLE hosts ADD COLUMN rdp_cert TEXT NOT NULL DEFAULT ''`,
	// Order inside a group, set by dragging; ties (and never-moved hosts) sort by name.
	`ALTER TABLE hosts ADD COLUMN position INTEGER NOT NULL DEFAULT 0`,
}

// DefaultPath returns hosts.db in the data folder ($XDG_DATA_HOME/tuxshell on Linux).
func DefaultPath() (string, error) {
	dir, err := platform.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hosts.db"), nil
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	// The host list is private. The directory may already exist (WebKit
	// creates it too), so tighten permissions rather than relying on MkdirAll.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	// SQLite gives its -wal and -shm files the main database file's mode.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(p, 0o600); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// One connection keeps writes serialized and avoids SQLITE_BUSY between our own goroutines.
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

const columns = `id, name, grp, alias, hostname, user, port, proxy_jump, identity_file, password_ref, tags, os, password_saved, profile, protocol, rdp, rdp_cert`

type scanner interface{ Scan(...any) error }

func scanHost(row scanner) (Host, error) {
	var h Host
	var tags, rdp string
	err := row.Scan(&h.ID, &h.Name, &h.Group, &h.Alias, &h.Hostname, &h.User, &h.Port,
		&h.ProxyJump, &h.IdentityFile, &h.PasswordRef, &tags, &h.OS, &h.PasswordSaved, &h.Profile, &h.Protocol, &rdp, &h.RDPCert)
	if err != nil {
		return h, err
	}
	h.RDP = DefaultRDP()
	_ = json.Unmarshal([]byte(rdp), &h.RDP)
	if h.RDP.Folders == nil {
		h.RDP.Folders = []Folder{}
	}
	if err := json.Unmarshal([]byte(tags), &h.Tags); err != nil || h.Tags == nil {
		h.Tags = []string{}
	}
	return h, nil
}

// List returns all hosts ordered by group, then the order set by dragging, then name.
func (s *Store) List() ([]Host, error) {
	rows, err := s.db.Query(`SELECT ` + columns + ` FROM hosts ORDER BY grp = '', grp COLLATE NOCASE, position, name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Host{}
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, rows.Err()
}

// Get returns one host.
func (s *Store) Get(id int64) (Host, error) {
	h, err := scanHost(s.db.QueryRow(`SELECT `+columns+` FROM hosts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return h, ErrNotFound
	}
	return h, err
}

// Save validates h and inserts it (ID 0) or updates it, returning the stored host.
func (s *Store) Save(h Host) (Host, error) {
	h.Normalize()
	if err := h.Validate(); err != nil {
		return h, err
	}
	tags, _ := json.Marshal(h.Tags)
	rdp, _ := json.Marshal(h.RDP)
	var err error
	if h.ID == 0 {
		var res sql.Result
		res, err = s.db.Exec(`INSERT INTO hosts (name, grp, alias, hostname, user, port, proxy_jump, identity_file, password_ref, tags, os, profile, protocol, rdp)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			h.Name, h.Group, h.Alias, h.Hostname, h.User, h.Port, h.ProxyJump, h.IdentityFile, h.PasswordRef, string(tags), h.OS, h.Profile, h.Protocol, string(rdp))
		if err == nil {
			h.ID, err = res.LastInsertId()
		}
	} else {
		var res sql.Result
		res, err = s.db.Exec(`UPDATE hosts SET name = ?, grp = ?, alias = ?, hostname = ?, user = ?, port = ?,
			proxy_jump = ?, identity_file = ?, password_ref = ?, tags = ?, os = ?, profile = ?, protocol = ?, rdp = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			h.Name, h.Group, h.Alias, h.Hostname, h.User, h.Port, h.ProxyJump, h.IdentityFile, h.PasswordRef, string(tags), h.OS, h.Profile, h.Protocol, string(rdp), h.ID)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				return h, ErrNotFound
			}
		}
	}
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: hosts.name") {
		return h, &ValidationError{Field: "name", Message: "A host with this name already exists"}
	}
	return h, err
}

// Delete removes a host.
func (s *Store) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM hosts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if s.OnDelete != nil {
		s.OnDelete([]int64{id})
	}
	return nil
}

// MoveHost puts host id into group just before host `before` (0, or a host
// that isn't in the group, means at the end), in one transaction: the group may
// change, and the group's hosts are renumbered so the order sticks. "" means
// no group.
func (s *Store) MoveHost(id int64, group string, before int64) error {
	group = strings.TrimSpace(group)
	if len(group) > 100 || strings.ContainsAny(group, "\n\r") {
		return &ValidationError{Field: "group", Message: "Group name is too long or has a line break"}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM hosts WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	// The group's hosts as listed now (case-insensitive, like the list), without id.
	rows, err := tx.Query(`SELECT id FROM hosts WHERE grp = ? COLLATE NOCASE AND id <> ? ORDER BY position, name COLLATE NOCASE`, group, id)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var other int64
		if err := rows.Scan(&other); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, other)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_ = rows.Close()
	index := len(ids)
	for i, other := range ids {
		if other == before {
			index = i
			break
		}
	}
	ids = append(ids[:index], append([]int64{id}, ids[index:]...)...)
	if _, err := tx.Exec(`UPDATE hosts SET grp = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND grp <> ?`, group, id, group); err != nil {
		return err
	}
	for i, hid := range ids {
		if _, err := tx.Exec(`UPDATE hosts SET position = ? WHERE id = ?`, i+1, hid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RenameGroup moves every host of group `from` into group `to` ("" means no
// group, which is how a group is deleted) and returns how many moved. When `to`
// already has hosts, the moved ones go after them in their own order.
func (s *Store) RenameGroup(from, to string) (int, error) {
	to = strings.TrimSpace(to)
	if len(to) > 100 || strings.ContainsAny(to, "\n\r") {
		return 0, &ValidationError{Field: "group", Message: "Group name is too long or has a line break"}
	}
	if from == "" || from == to {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	ids := func(query string, args ...any) ([]int64, error) {
		rows, err := tx.Query(query, args...)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		var out []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, rows.Err()
	}
	// Both as listed now; the target is matched like the list matches groups (case-insensitive).
	kept, err := ids(`SELECT id FROM hosts WHERE grp = ? COLLATE NOCASE AND grp <> ? ORDER BY position, name COLLATE NOCASE`, to, from)
	if err != nil {
		return 0, err
	}
	moved, err := ids(`SELECT id FROM hosts WHERE grp = ? ORDER BY position, name COLLATE NOCASE`, from)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE hosts SET grp = ?, updated_at = CURRENT_TIMESTAMP WHERE grp = ?`, to, from); err != nil {
		return 0, err
	}
	for i, id := range append(kept, moved...) {
		if _, err := tx.Exec(`UPDATE hosts SET position = ? WHERE id = ?`, i+1, id); err != nil {
			return 0, err
		}
	}
	return len(moved), tx.Commit()
}

// SetRDPCert records the certificate fingerprint trusted for a remote desktop.
func (s *Store) SetRDPCert(id int64, fingerprint string) error {
	res, err := s.db.Exec(`UPDATE hosts SET rdp_cert = ? WHERE id = ?`, fingerprint, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPasswordSaved records whether the host has a password in the keyring.
func (s *Store) SetPasswordSaved(id int64, saved bool) error {
	res, err := s.db.Exec(`UPDATE hosts SET password_saved = ? WHERE id = ?`, saved, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMany removes several hosts in one transaction and returns how many
// existed. Unknown IDs are ignored.
func (s *Store) DeleteMany(ids []int64) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`DELETE FROM hosts WHERE id = ?`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	deleted := 0
	for _, id := range ids {
		res, err := stmt.Exec(id)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		deleted += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if s.OnDelete != nil && deleted > 0 {
		s.OnDelete(ids)
	}
	return deleted, nil
}
