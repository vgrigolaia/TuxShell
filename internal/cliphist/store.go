// Package cliphist keeps an encrypted clipboard history. Entries are sealed
// with AES-256-GCM under a key held in the OS keyring; nothing readable is
// written to disk.
package cliphist

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	// Retention is how long entries are kept.
	Retention = 30 * 24 * time.Hour
	// MaxEntries caps the history size; the oldest entries go first.
	MaxEntries = 500
)

// Clip is a decrypted history entry.
type Clip struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// Store persists encrypted clips in SQLite.
type Store struct {
	db   *sql.DB
	aead cipher.AEAD
	mac  []byte
	now  func() time.Time
}

const schema = `CREATE TABLE IF NOT EXISTS clips (
	id         INTEGER PRIMARY KEY,
	-- HMAC of the plaintext, for de-duplication without storing it.
	digest     TEXT NOT NULL UNIQUE,
	nonce      BLOB NOT NULL,
	ciphertext BLOB NOT NULL,
	created_at INTEGER NOT NULL
)`

// Open opens the history at path, encrypting with a 32-byte key.
func Open(path string, key []byte) (*Store, error) {
	if len(key) != 32 {
		return nil, errors.New("clipboard key must be 32 bytes")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
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

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=secure_delete(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	// Derive separate encryption and MAC keys from the master key.
	encKey := derive(key, "tuxshell clipboard encryption")
	block, err := aes.NewCipher(encKey)
	if err != nil {
		db.Close()
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, aead: aead, mac: derive(key, "tuxshell clipboard digest"), now: time.Now}, nil
}

func derive(key []byte, label string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(label))
	return h.Sum(nil)
}

func (s *Store) digest(text string) string {
	h := hmac.New(sha256.New, s.mac)
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Add records text, moving an identical earlier entry to the top.
// It returns false when the text is filtered out.
func (s *Store) Add(text string) (bool, error) {
	if !Recordable(text) {
		return false, nil
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return false, err
	}
	digest := s.digest(text)
	// The digest is bound as associated data, so a row's ciphertext can't be
	// swapped onto another row unnoticed.
	sealed := s.aead.Seal(nil, nonce, []byte(text), []byte(digest))
	_, err := s.db.Exec(`INSERT INTO clips (digest, nonce, ciphertext, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(digest) DO UPDATE SET nonce = excluded.nonce, ciphertext = excluded.ciphertext, created_at = excluded.created_at`,
		digest, nonce, sealed, s.now().UnixMilli())
	if err != nil {
		return false, err
	}
	return true, s.Prune()
}

// Prune drops entries past retention and beyond MaxEntries.
func (s *Store) Prune() error {
	cutoff := s.now().Add(-Retention).UnixMilli()
	if _, err := s.db.Exec(`DELETE FROM clips WHERE created_at < ?`, cutoff); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM clips WHERE id NOT IN (SELECT id FROM clips ORDER BY created_at DESC, id DESC LIMIT ?)`, MaxEntries)
	return err
}

// List returns entries newest first. Entries that fail to decrypt (e.g. after
// the keyring key was replaced) are skipped.
func (s *Store) List() ([]Clip, error) {
	rows, err := s.db.Query(`SELECT id, digest, nonce, ciphertext, created_at FROM clips ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Clip{}
	for rows.Next() {
		var (
			id, created          int64
			digest               string
			nonce, sealed, plain []byte
		)
		if err := rows.Scan(&id, &digest, &nonce, &sealed, &created); err != nil {
			return nil, err
		}
		plain, err = s.aead.Open(nil, nonce, sealed, []byte(digest))
		if err != nil {
			continue
		}
		out = append(out, Clip{ID: id, Text: string(plain), CreatedAt: time.UnixMilli(created)})
	}
	return out, rows.Err()
}

// Delete removes one entry.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM clips WHERE id = ?`, id)
	return err
}

// Clear removes every entry.
func (s *Store) Clear() error {
	if _, err := s.db.Exec(`DELETE FROM clips`); err != nil {
		return err
	}
	// Reclaim pages so deleted ciphertext doesn't linger in the file.
	_, err := s.db.Exec(`VACUUM`)
	if err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}
