package hosts

import (
	"database/sql"
	"strings"
)

// Tunnel is a saved port forward through a host.
type Tunnel struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	HostID int64  `json:"hostId"`
	// Kind is "local" (-L: a local port reaches TargetHost:TargetPort from
	// the host) or "dynamic" (-D: a SOCKS proxy on the local port).
	Kind       string `json:"kind"`
	BindPort   int    `json:"bindPort"`
	TargetHost string `json:"targetHost"`
	TargetPort int    `json:"targetPort"`
}

func (t *Tunnel) normalize() {
	t.Name = strings.TrimSpace(t.Name)
	t.TargetHost = strings.TrimSpace(t.TargetHost)
	if t.Kind == "dynamic" {
		t.TargetHost, t.TargetPort = "", 0
	}
}

func (t *Tunnel) validate() error {
	invalid := func(field, msg string) error { return &ValidationError{Field: field, Message: msg} }
	switch {
	case len(t.Name) > 100:
		return invalid("name", "Name is too long")
	case t.HostID == 0:
		return invalid("hostId", "Choose the host to forward through")
	case t.Kind != "local" && t.Kind != "dynamic":
		return invalid("kind", "Choose local or SOCKS")
	case t.BindPort < 1 || t.BindPort > 65535:
		return invalid("bindPort", "Port must be 1–65535")
	}
	if t.Kind == "local" {
		switch {
		case t.TargetHost == "":
			return invalid("targetHost", "Target host is required")
		case len(t.TargetHost) > 253 || !hostRe.MatchString(t.TargetHost):
			return invalid("targetHost", "Use a host name or address, without spaces or a leading dash")
		case t.TargetPort < 1 || t.TargetPort > 65535:
			return invalid("targetPort", "Port must be 1–65535")
		}
	}
	return nil
}

// ListTunnels returns all saved tunnels.
func (s *Store) ListTunnels() ([]Tunnel, error) {
	rows, err := s.db.Query(`SELECT id, name, host_id, kind, bind_port, target_host, target_port FROM tunnels ORDER BY bind_port`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Tunnel{}
	for rows.Next() {
		var t Tunnel
		if err := rows.Scan(&t.ID, &t.Name, &t.HostID, &t.Kind, &t.BindPort, &t.TargetHost, &t.TargetPort); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// GetTunnel returns one tunnel.
func (s *Store) GetTunnel(id int64) (Tunnel, error) {
	var t Tunnel
	err := s.db.QueryRow(`SELECT id, name, host_id, kind, bind_port, target_host, target_port FROM tunnels WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &t.HostID, &t.Kind, &t.BindPort, &t.TargetHost, &t.TargetPort)
	if err == sql.ErrNoRows {
		return t, ErrNotFound
	}
	return t, err
}

// SaveTunnel validates and inserts (ID 0) or updates a tunnel.
func (s *Store) SaveTunnel(t Tunnel) (Tunnel, error) {
	t.normalize()
	if err := t.validate(); err != nil {
		return t, err
	}
	if _, err := s.Get(t.HostID); err != nil {
		return t, &ValidationError{Field: "hostId", Message: "That host no longer exists"}
	}
	if t.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO tunnels (name, host_id, kind, bind_port, target_host, target_port) VALUES (?, ?, ?, ?, ?, ?)`,
			t.Name, t.HostID, t.Kind, t.BindPort, t.TargetHost, t.TargetPort)
		if err != nil {
			return t, err
		}
		t.ID, err = res.LastInsertId()
		return t, err
	}
	res, err := s.db.Exec(`UPDATE tunnels SET name = ?, host_id = ?, kind = ?, bind_port = ?, target_host = ?, target_port = ? WHERE id = ?`,
		t.Name, t.HostID, t.Kind, t.BindPort, t.TargetHost, t.TargetPort, t.ID)
	if err == nil {
		if n, _ := res.RowsAffected(); n == 0 {
			return t, ErrNotFound
		}
	}
	return t, err
}

// DeleteTunnel removes a tunnel.
func (s *Store) DeleteTunnel(id int64) error {
	res, err := s.db.Exec(`DELETE FROM tunnels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
