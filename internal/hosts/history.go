package hosts

import (
	"strconv"
	"strings"
	"time"
)

// Visit is one connection in the history. Only which host, as whom, when and
// for how long are kept: never commands or anything shown in the terminal.
type Visit struct {
	ID int64 `json:"id"`
	// HostID is the saved host, or 0 for an ad-hoc target. The host may have
	// been deleted since; Name keeps what it was called.
	HostID int64  `json:"hostId"`
	Name   string `json:"name"`
	// Target is what ssh connected to: user@host[:port].
	Target  string `json:"target"`
	Started int64  `json:"started"` // Unix milliseconds
	// Ended is 0 while the session is open.
	Ended int64 `json:"ended"`
	// Result is "open", "ok" (ended normally) or "failed" (ssh ended with an
	// error: it couldn't connect or log in, or the connection dropped).
	Result string `json:"result"`
}

// HistoryLimits says how much history to keep.
type HistoryLimits struct {
	Days int
	Max  int
}

// StartVisit records a connection that just started and returns its ID.
func (s *Store) StartVisit(v Visit) (int64, error) {
	if v.Started == 0 {
		v.Started = time.Now().UnixMilli()
	}
	res, err := s.db.Exec(`INSERT INTO history (host_id, name, target, started, ended, result) VALUES (?, ?, ?, ?, 0, 'open')`,
		v.HostID, v.Name, v.Target, v.Started)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EndVisit records how a connection ended.
func (s *Store) EndVisit(id int64, failed bool) error {
	result := "ok"
	if failed {
		result = "failed"
	}
	_, err := s.db.Exec(`UPDATE history SET ended = ?, result = ? WHERE id = ?`, time.Now().UnixMilli(), result, id)
	return err
}

// CloseOpenVisits marks visits left open by a crash or a killed app as ended
// at their start, so they don't show as still running.
func (s *Store) CloseOpenVisits() error {
	_, err := s.db.Exec(`UPDATE history SET ended = started, result = 'ok' WHERE result = 'open'`)
	return err
}

// ListVisits returns the newest visits first; query filters by name or target.
func (s *Store) ListVisits(query string, limit int) ([]Visit, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	q := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(query)) + "%"
	rows, err := s.db.Query(`SELECT id, host_id, name, target, started, ended, result FROM history
		WHERE name LIKE ? ESCAPE '\' OR target LIKE ? ESCAPE '\' ORDER BY started DESC, id DESC LIMIT ?`, q, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Visit{}
	for rows.Next() {
		var v Visit
		if err := rows.Scan(&v.ID, &v.HostID, &v.Name, &v.Target, &v.Started, &v.Ended, &v.Result); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

// DeleteVisit removes one entry.
func (s *Store) DeleteVisit(id int64) error {
	_, err := s.db.Exec(`DELETE FROM history WHERE id = ?`, id)
	return err
}

// ClearHistory removes every entry and returns how many there were.
func (s *Store) ClearHistory() (int, error) {
	res, err := s.db.Exec(`DELETE FROM history`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PruneHistory removes entries older than the limits allow (open ones stay).
func (s *Store) PruneHistory(l HistoryLimits) error {
	if l.Days > 0 {
		cutoff := time.Now().AddDate(0, 0, -l.Days).UnixMilli()
		if _, err := s.db.Exec(`DELETE FROM history WHERE result != 'open' AND started < ?`, cutoff); err != nil {
			return err
		}
	}
	if l.Max > 0 {
		if _, err := s.db.Exec(`DELETE FROM history WHERE result != 'open' AND id NOT IN
			(SELECT id FROM history ORDER BY started DESC, id DESC LIMIT ?)`, l.Max); err != nil {
			return err
		}
	}
	return nil
}

// Recorder adds terminal sessions to the history (terminal.SetHistory).
type Recorder struct {
	Store   *Store
	Enabled func() bool
	Limits  func() HistoryLimits
	// Changed is called after each change, to tell the frontend.
	Changed func()
}

// Begin records a session to a saved host (hostID, logging in as user when
// set) or an ad-hoc target, and returns the function to call with ssh's exit
// code. It returns nil when history is off.
func (r Recorder) Begin(hostID int64, user, target string) func(code int) {
	if r.Enabled != nil && !r.Enabled() {
		return nil
	}
	v := Visit{HostID: hostID, Name: target, Target: target}
	if hostID != 0 {
		h, err := r.Store.Get(hostID)
		if err != nil {
			return nil
		}
		if user == "" {
			user = h.User
		}
		addr := h.Hostname
		if addr == "" {
			addr = h.Alias
		}
		v.Name, v.Target = h.Name, addr
		if user != "" {
			v.Target = user + "@" + addr
		}
		if h.Port != 0 && h.Port != 22 {
			v.Target += ":" + strconv.Itoa(h.Port)
		}
	}
	id, err := r.Store.StartVisit(v)
	if err != nil {
		return nil
	}
	if r.Limits != nil {
		_ = r.Store.PruneHistory(r.Limits())
	}
	r.changed()
	return func(code int) {
		// ssh exits 255 when it can't connect or log in, or the connection drops.
		_ = r.Store.EndVisit(id, code == 255)
		r.changed()
	}
}

func (r Recorder) changed() {
	if r.Changed != nil {
		r.Changed()
	}
}

// EventHistoryChanged tells the frontend the history changed.
const EventHistoryChanged = "history:changed"

// HistoryService exposes the history to the frontend.
type HistoryService struct {
	store *Store
}

// NewHistoryService returns the bound history service.
func NewHistoryService(store *Store) *HistoryService { return &HistoryService{store: store} }

// List returns the newest visits first, filtered by name or target.
func (s *HistoryService) List(query string) ([]Visit, error) { return s.store.ListVisits(query, 0) }

// Delete removes one entry.
func (s *HistoryService) Delete(id int64) error { return s.store.DeleteVisit(id) }

// Clear removes the whole history and returns how many entries there were.
func (s *HistoryService) Clear() (int, error) { return s.store.ClearHistory() }
