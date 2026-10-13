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
)

// Store persists hosts in a SQLite database.
type Store struct {
	db *sql.DB
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
}

// DefaultPath returns $XDG_DATA_HOME/tuxshell/hosts.db.
func DefaultPath() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "tuxshell", "hosts.db"), nil
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
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

const columns = `id, name, grp, alias, hostname, user, port, proxy_jump, identity_file, password_ref, tags`

type scanner interface{ Scan(...any) error }

func scanHost(row scanner) (Host, error) {
	var h Host
	var tags string
	err := row.Scan(&h.ID, &h.Name, &h.Group, &h.Alias, &h.Hostname, &h.User, &h.Port,
		&h.ProxyJump, &h.IdentityFile, &h.PasswordRef, &tags)
	if err != nil {
		return h, err
	}
	if err := json.Unmarshal([]byte(tags), &h.Tags); err != nil || h.Tags == nil {
		h.Tags = []string{}
	}
	return h, nil
}

// List returns all hosts ordered by group, then name.
func (s *Store) List() ([]Host, error) {
	rows, err := s.db.Query(`SELECT ` + columns + ` FROM hosts ORDER BY grp = '', grp COLLATE NOCASE, name COLLATE NOCASE`)
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
	var err error
	if h.ID == 0 {
		var res sql.Result
		res, err = s.db.Exec(`INSERT INTO hosts (name, grp, alias, hostname, user, port, proxy_jump, identity_file, password_ref, tags)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			h.Name, h.Group, h.Alias, h.Hostname, h.User, h.Port, h.ProxyJump, h.IdentityFile, h.PasswordRef, string(tags))
		if err == nil {
			h.ID, err = res.LastInsertId()
		}
	} else {
		var res sql.Result
		res, err = s.db.Exec(`UPDATE hosts SET name = ?, grp = ?, alias = ?, hostname = ?, user = ?, port = ?,
			proxy_jump = ?, identity_file = ?, password_ref = ?, tags = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			h.Name, h.Group, h.Alias, h.Hostname, h.User, h.Port, h.ProxyJump, h.IdentityFile, h.PasswordRef, string(tags), h.ID)
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
	return nil
}
