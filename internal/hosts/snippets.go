package hosts

import (
	"database/sql"
	"strings"
)

// Snippet is a saved command that can be pasted into a terminal.
type Snippet struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Group   string `json:"group"`
}

// MaxSnippetBytes caps a snippet's command.
const MaxSnippetBytes = 16 * 1024

func (sn *Snippet) normalize() {
	sn.Name = strings.TrimSpace(sn.Name)
	sn.Group = strings.TrimSpace(sn.Group)
	sn.Command = strings.TrimRight(sn.Command, " \t\r\n")
}

func (sn *Snippet) validate() error {
	switch {
	case sn.Name == "":
		return &ValidationError{Field: "name", Message: "Name is required"}
	case len(sn.Name) > 100:
		return &ValidationError{Field: "name", Message: "Name is too long"}
	case strings.TrimSpace(sn.Command) == "":
		return &ValidationError{Field: "command", Message: "Command is required"}
	case len(sn.Command) > MaxSnippetBytes:
		return &ValidationError{Field: "command", Message: "Command is too long"}
	case strings.ContainsRune(sn.Command, 0):
		return &ValidationError{Field: "command", Message: "Command contains a NUL byte"}
	}
	return nil
}

// ListSnippets returns all snippets by group, then name.
func (s *Store) ListSnippets() ([]Snippet, error) {
	rows, err := s.db.Query(`SELECT id, name, command, grp FROM snippets ORDER BY grp = '', grp COLLATE NOCASE, name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Snippet{}
	for rows.Next() {
		var sn Snippet
		if err := rows.Scan(&sn.ID, &sn.Name, &sn.Command, &sn.Group); err != nil {
			return nil, err
		}
		list = append(list, sn)
	}
	return list, rows.Err()
}

// SaveSnippet inserts (ID 0) or updates a snippet.
func (s *Store) SaveSnippet(sn Snippet) (Snippet, error) {
	sn.normalize()
	if err := sn.validate(); err != nil {
		return sn, err
	}
	var err error
	if sn.ID == 0 {
		var res sql.Result
		res, err = s.db.Exec(`INSERT INTO snippets (name, command, grp) VALUES (?, ?, ?)`, sn.Name, sn.Command, sn.Group)
		if err == nil {
			sn.ID, err = res.LastInsertId()
		}
	} else {
		var res sql.Result
		res, err = s.db.Exec(`UPDATE snippets SET name = ?, command = ?, grp = ? WHERE id = ?`, sn.Name, sn.Command, sn.Group, sn.ID)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				return sn, ErrNotFound
			}
		}
	}
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: snippets.name") {
		return sn, &ValidationError{Field: "name", Message: "A snippet with this name already exists"}
	}
	return sn, err
}

// DeleteSnippet removes a snippet.
func (s *Store) DeleteSnippet(id int64) error {
	res, err := s.db.Exec(`DELETE FROM snippets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SnippetService exposes snippets to the frontend.
type SnippetService struct{ store *Store }

// NewSnippetService wraps store for binding.
func NewSnippetService(store *Store) *SnippetService { return &SnippetService{store: store} }

// List returns all snippets.
func (s *SnippetService) List() ([]Snippet, error) { return s.store.ListSnippets() }

// Save creates or updates a snippet; problems come back as a ValidationError.
func (s *SnippetService) Save(sn Snippet) (Snippet, error) { return s.store.SaveSnippet(sn) }

// Delete removes a snippet.
func (s *SnippetService) Delete(id int64) error { return s.store.DeleteSnippet(id) }
