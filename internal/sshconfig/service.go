package sshconfig

import (
	"context"
	"errors"
	"fmt"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// ImportGroup is the group imported hosts land in.
const ImportGroup = "SSH config"

// Candidate is an alias found in ~/.ssh/config, offered for import.
type Candidate struct {
	Entry
	Settings
	// Imported is true when a saved host already uses this alias.
	Imported bool `json:"imported"`
	// Error explains why the alias couldn't be resolved or imported.
	Error string `json:"error"`
}

// ImportResult summarises an import.
type ImportResult struct {
	Imported int      `json:"imported"`
	Skipped  []string `json:"skipped"`
}

// Service exposes ~/.ssh/config import to the frontend.
type Service struct {
	store   *hosts.Store
	path    string
	resolve func(ctx context.Context, alias string) (Settings, error)
}

// NewService imports from the config at path into store.
func NewService(store *hosts.Store, path string) *Service {
	return &Service{store: store, path: path, resolve: Resolve}
}

// Scan lists the config's aliases with their effective settings.
func (s *Service) Scan(ctx context.Context) ([]Candidate, error) {
	entries, err := Aliases(s.path)
	if err != nil {
		return nil, err
	}
	existing, err := s.store.List()
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, h := range existing {
		if h.Alias != "" {
			used[h.Alias] = true
		}
	}

	out := make([]Candidate, 0, len(entries))
	for _, e := range entries {
		c := Candidate{Entry: e, Imported: used[e.Alias]}
		if set, err := s.resolve(ctx, e.Alias); err != nil {
			c.Error = err.Error()
		} else {
			c.Settings = set
		}
		out = append(out, c)
	}
	return out, nil
}

// Import saves the chosen aliases as hosts. Only aliases that are actually
// in the config are accepted; already-imported ones are skipped.
func (s *Service) Import(ctx context.Context, aliases []string) (ImportResult, error) {
	candidates, err := s.Scan(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	byAlias := map[string]Candidate{}
	for _, c := range candidates {
		byAlias[c.Alias] = c
	}

	res := ImportResult{Skipped: []string{}}
	for _, alias := range aliases {
		c, ok := byAlias[alias]
		switch {
		case !ok:
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: not in ssh config", alias))
			continue
		case c.Imported:
			continue
		case c.Error != "":
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %s", alias, c.Error))
			continue
		}
		_, err := s.store.Save(hosts.Host{
			Name:      c.Alias,
			Group:     ImportGroup,
			Alias:     c.Alias,
			Hostname:  c.Hostname,
			User:      c.User,
			Port:      c.Port,
			ProxyJump: c.ProxyJump,
		})
		var ve *hosts.ValidationError
		switch {
		case errors.As(err, &ve):
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %s", alias, ve.Message))
		case err != nil:
			return res, err
		default:
			res.Imported++
		}
	}
	return res, nil
}
