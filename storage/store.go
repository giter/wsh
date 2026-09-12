package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// configDir returns the directory where app state is kept, creating it if needed.
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "fyneshell")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// configFile is the path to the JSON state file.
func configFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Store holds all persisted app state and provides load/save accessors.
type Store struct {
	connections []*Connection
	tunnels     []*Tunnel
	path        string
	dirty       bool
}

// NewStore loads existing state from disk. Missing file means a fresh store.
func NewStore() (*Store, error) {
	path, err := configFile()
	if err != nil {
		return nil, err
	}
	s := &Store{path: path}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	type disk struct {
		Connections []*Connection `json:"connections"`
		Tunnels     []*Tunnel     `json:"tunnels"`
	}
	var d disk
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	s.connections = d.Connections
	s.tunnels = d.Tunnels
	return s, nil
}

// Save writes state to disk when something changed.
func (s *Store) Save() error {
	if !s.dirty {
		return nil
	}
	type disk struct {
		Connections []*Connection `json:"connections"`
		Tunnels     []*Tunnel     `json:"tunnels"`
	}
	d := disk{Connections: s.connections, Tunnels: s.tunnels}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

func (s *Store) mark() { s.dirty = true }

// Connections returns a copy of the connection slice.
func (s *Store) Connections() []*Connection {
	out := make([]*Connection, len(s.connections))
	copy(out, s.connections)
	return out
}

// AddConnection inserts a new connection and persists.
func (s *Store) AddConnection(c *Connection) error {
	s.connections = append(s.connections, c)
	s.mark()
	return s.Save()
}

// UpdateConnection replaces a connection by ID.
func (s *Store) UpdateConnection(c *Connection) error {
	for i, existing := range s.connections {
		if existing.ID == c.ID {
			s.connections[i] = c
			s.mark()
			return s.Save()
		}
	}
	return errors.New("connection not found")
}

// DeleteConnection removes a connection and any tunnels that depend on it.
func (s *Store) DeleteConnection(id string) error {
	kept := s.connections[:0]
	for _, c := range s.connections {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	s.connections = kept

	tunnels := s.tunnels[:0]
	for _, t := range s.tunnels {
		if t.ConnectionID != id {
			tunnels = append(tunnels, t)
		}
	}
	s.tunnels = tunnels
	s.mark()
	return s.Save()
}

// Tunnels returns a copy of the tunnel slice.
func (s *Store) Tunnels() []*Tunnel {
	out := make([]*Tunnel, len(s.tunnels))
	copy(out, s.tunnels)
	return out
}

// AddTunnel inserts a new tunnel and persists.
func (s *Store) AddTunnel(t *Tunnel) error {
	s.tunnels = append(s.tunnels, t)
	s.mark()
	return s.Save()
}

// UpdateTunnel replaces a tunnel by ID.
func (s *Store) UpdateTunnel(t *Tunnel) error {
	for i, existing := range s.tunnels {
		if existing.ID == t.ID {
			s.tunnels[i] = t
			s.mark()
			return s.Save()
		}
	}
	return errors.New("tunnel not found")
}

// DeleteTunnel removes a tunnel by ID.
func (s *Store) DeleteTunnel(id string) error {
	kept := s.tunnels[:0]
	for _, t := range s.tunnels {
		if t.ID != id {
			kept = append(kept, t)
		}
	}
	s.tunnels = kept
	s.mark()
	return s.Save()
}
