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
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Clip is a decrypted history entry. Pinned clips are kept until removed by
// hand: retention, the entry cap and Clear skip them.
type Clip struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	Label     string    `json:"label"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"createdAt"`
}

// Settings control how much unpinned history is kept.
type Settings struct {
	// RetentionDays is how long entries are kept; 0 keeps them forever.
	RetentionDays int `json:"retentionDays"`
	// MaxEntries caps unpinned entries; 0 means no limit.
	MaxEntries int `json:"maxEntries"`
	// CaptureSystem records what was copied in other apps when TuxShell regains focus.
	CaptureSystem bool `json:"captureSystem"`
}

// DefaultSettings are used until the user changes them.
var DefaultSettings = Settings{RetentionDays: 30, MaxEntries: 500, CaptureSystem: true}

// Limits on what Settings may hold.
const (
	MaxRetentionDays = 3650
	MaxEntriesLimit  = 100_000
	MaxLabelLen      = 100
)

// ErrSensitive is returned when a manually saved clip looks like a secret.
var ErrSensitive = errors.New("this looks like a password or token; keep secrets in 1Password instead")

// Store persists encrypted clips in SQLite.
type Store struct {
	db   *sql.DB
	aead cipher.AEAD
	mac  []byte
	now  func() time.Time
}

// migrations run in order; PRAGMA user_version records how many have been applied.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS clips (
		id         INTEGER PRIMARY KEY,
		-- HMAC of the plaintext, for de-duplication without storing it.
		digest     TEXT NOT NULL UNIQUE,
		nonce      BLOB NOT NULL,
		ciphertext BLOB NOT NULL,
		created_at INTEGER NOT NULL
	)`,
	`ALTER TABLE clips ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE clips ADD COLUMN label_nonce BLOB;
	ALTER TABLE clips ADD COLUMN label_ciphertext BLOB;
	CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
}

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
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	// Derive separate encryption and MAC keys from the master key.
	block, err := aes.NewCipher(derive(key, "tuxshell clipboard encryption"))
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
			return fmt.Errorf("clipboard migration %d: %w", i+1, err)
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

// seal encrypts plain, binding aad so ciphertext can't be moved between rows
// or fields unnoticed.
func (s *Store) seal(plain, aad string) (nonce, sealed []byte, err error) {
	nonce = make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, s.aead.Seal(nil, nonce, []byte(plain), []byte(aad)), nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Add records copied text, moving an identical earlier entry to the top
// (keeping its pin and label). It returns false when the text is filtered out.
func (s *Store) Add(text string, settings Settings) (bool, error) {
	if !Recordable(text) {
		return false, nil
	}
	digest := s.digest(text)
	nonce, sealed, err := s.seal(text, digest)
	if err != nil {
		return false, err
	}
	_, err = s.db.Exec(`INSERT INTO clips (digest, nonce, ciphertext, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(digest) DO UPDATE SET nonce = excluded.nonce, ciphertext = excluded.ciphertext, created_at = excluded.created_at`,
		digest, nonce, sealed, s.now().UnixMilli())
	if err != nil {
		return false, err
	}
	return true, s.Prune(settings)
}

// SaveClip stores text as a pinned clip with an optional label. Saving text
// that is already in the history pins that entry.
func (s *Store) SaveClip(text, label string) (Clip, error) {
	text, label, err := checkSaved(text, label)
	if err != nil {
		return Clip{}, err
	}
	digest := s.digest(text)
	nonce, sealed, err := s.seal(text, digest)
	if err != nil {
		return Clip{}, err
	}
	labelNonce, labelSealed, err := s.sealLabel(label, digest)
	if err != nil {
		return Clip{}, err
	}
	var id int64
	err = s.db.QueryRow(`INSERT INTO clips (digest, nonce, ciphertext, created_at, pinned, label_nonce, label_ciphertext)
		VALUES (?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(digest) DO UPDATE SET pinned = 1, label_nonce = excluded.label_nonce, label_ciphertext = excluded.label_ciphertext
		RETURNING id`,
		digest, nonce, sealed, s.now().UnixMilli(), labelNonce, labelSealed).Scan(&id)
	if err != nil {
		return Clip{}, err
	}
	return s.get(id)
}

// UpdateClip changes a clip's text and label. If the new text matches another
// entry, that entry is replaced.
func (s *Store) UpdateClip(id int64, text, label string) (Clip, error) {
	text, label, err := checkSaved(text, label)
	if err != nil {
		return Clip{}, err
	}
	digest := s.digest(text)
	nonce, sealed, err := s.seal(text, digest)
	if err != nil {
		return Clip{}, err
	}
	labelNonce, labelSealed, err := s.sealLabel(label, digest)
	if err != nil {
		return Clip{}, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Clip{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM clips WHERE digest = ? AND id != ?`, digest, id); err != nil {
		return Clip{}, err
	}
	res, err := tx.Exec(`UPDATE clips SET digest = ?, nonce = ?, ciphertext = ?, label_nonce = ?, label_ciphertext = ? WHERE id = ?`,
		digest, nonce, sealed, labelNonce, labelSealed, id)
	if err != nil {
		return Clip{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Clip{}, errors.New("clip not found")
	}
	if err := tx.Commit(); err != nil {
		return Clip{}, err
	}
	return s.get(id)
}

// SetPinned pins or unpins a clip.
func (s *Store) SetPinned(id int64, pinned bool) error {
	_, err := s.db.Exec(`UPDATE clips SET pinned = ? WHERE id = ?`, pinned, id)
	return err
}

func checkSaved(text, label string) (string, string, error) {
	label = strings.TrimSpace(label)
	switch {
	case strings.TrimSpace(text) == "":
		return "", "", errors.New("clip text is empty")
	case len(text) > MaxClipBytes:
		return "", "", fmt.Errorf("clip is larger than %d KB", MaxClipBytes/1024)
	case len(label) > MaxLabelLen:
		return "", "", errors.New("name is too long")
	case Sensitive(text):
		return "", "", ErrSensitive
	}
	return text, label, nil
}

// Labels are bound to the row's digest with a distinct prefix, so a label
// ciphertext can't be passed off as clip text.
func (s *Store) sealLabel(label, digest string) ([]byte, []byte, error) {
	if label == "" {
		return nil, nil, nil
	}
	return s.seal(label, "label:"+digest)
}

// Prune drops unpinned entries past retention and beyond the entry cap.
func (s *Store) Prune(settings Settings) error {
	if settings.RetentionDays > 0 {
		cutoff := s.now().Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour).UnixMilli()
		if _, err := s.db.Exec(`DELETE FROM clips WHERE pinned = 0 AND created_at < ?`, cutoff); err != nil {
			return err
		}
	}
	if settings.MaxEntries > 0 {
		_, err := s.db.Exec(`DELETE FROM clips WHERE pinned = 0 AND id NOT IN
			(SELECT id FROM clips WHERE pinned = 0 ORDER BY created_at DESC, id DESC LIMIT ?)`, settings.MaxEntries)
		return err
	}
	return nil
}

const clipColumns = `id, digest, nonce, ciphertext, created_at, pinned, label_nonce, label_ciphertext`

type scanner interface{ Scan(...any) error }

// scan decrypts one row; ok is false when it doesn't decrypt (e.g. after the
// keyring key was replaced).
func (s *Store) scan(row scanner) (c Clip, ok bool, err error) {
	var (
		created                        int64
		digest                         string
		nonce, sealed, lNonce, lSealed []byte
	)
	if err := row.Scan(&c.ID, &digest, &nonce, &sealed, &created, &c.Pinned, &lNonce, &lSealed); err != nil {
		return c, false, err
	}
	plain, err := s.aead.Open(nil, nonce, sealed, []byte(digest))
	if err != nil {
		return c, false, nil
	}
	c.Text = string(plain)
	if len(lSealed) > 0 {
		if label, err := s.aead.Open(nil, lNonce, lSealed, []byte("label:"+digest)); err == nil {
			c.Label = string(label)
		}
	}
	c.CreatedAt = time.UnixMilli(created)
	return c, true, nil
}

// List returns pinned clips first, then history, each newest first.
func (s *Store) List() ([]Clip, error) {
	rows, err := s.db.Query(`SELECT ` + clipColumns + ` FROM clips ORDER BY pinned DESC, created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Clip{}
	for rows.Next() {
		c, ok, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

func (s *Store) get(id int64) (Clip, error) {
	c, ok, err := s.scan(s.db.QueryRow(`SELECT `+clipColumns+` FROM clips WHERE id = ?`, id))
	if err == nil && !ok {
		err = errors.New("clip could not be decrypted")
	}
	return c, err
}

// Delete removes one entry, pinned or not.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM clips WHERE id = ?`, id)
	return err
}

// Clear removes all unpinned entries.
func (s *Store) Clear() error {
	if _, err := s.db.Exec(`DELETE FROM clips WHERE pinned = 0`); err != nil {
		return err
	}
	// Reclaim pages so deleted ciphertext doesn't linger in the file.
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}

// Settings returns the saved settings, falling back to defaults.
func (s *Store) Settings() (Settings, error) {
	st := DefaultSettings
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		switch k {
		case "retention_days":
			st.RetentionDays, _ = strconv.Atoi(v)
		case "max_entries":
			st.MaxEntries, _ = strconv.Atoi(v)
		case "capture_system":
			st.CaptureSystem = v == "1"
		}
	}
	return st, rows.Err()
}

// SaveSettings validates and stores settings, then prunes to match.
func (s *Store) SaveSettings(st Settings) error {
	if st.RetentionDays < 0 || st.RetentionDays > MaxRetentionDays {
		return fmt.Errorf("retention must be 0 (forever) to %d days", MaxRetentionDays)
	}
	if st.MaxEntries < 0 || st.MaxEntries > MaxEntriesLimit {
		return fmt.Errorf("entry limit must be 0 (none) to %d", MaxEntriesLimit)
	}
	capture := "0"
	if st.CaptureSystem {
		capture = "1"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range map[string]string{
		"retention_days": strconv.Itoa(st.RetentionDays),
		"max_entries":    strconv.Itoa(st.MaxEntries),
		"capture_system": capture,
	} {
		if _, err := tx.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.Prune(st)
}
