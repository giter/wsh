package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// configDir returns the directory where app state is kept, creating it if needed.
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "wsh")
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
	folders     []*Folder
	keys        []*SSHKey
	settings    Settings
	path        string
	dirty       bool
}

// NewStore loads existing state from disk. Missing file means a fresh store.
func NewStore() (*Store, error) {
	path, err := configFile()
	if err != nil {
		return nil, err
	}
	return LoadStore(path)
}

// LoadStore reads state from an explicit config path. Missing file means a fresh
// store. It is separated from NewStore so tests (and future import/export) can
// point the store at any file.
func LoadStore(path string) (*Store, error) {
	s := &Store{path: path, settings: defaultSettings()}

	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		type disk struct {
			Connections []*Connection `json:"connections"`
			Tunnels     []*Tunnel     `json:"tunnels"`
			Folders     []*Folder     `json:"folders"`
			Keys        []*SSHKey     `json:"keys"`
		}
		var d disk
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		s.connections = d.Connections
		s.tunnels = d.Tunnels
		s.folders = d.Folders
		s.keys = d.Keys
	}

	// Settings live in their own file so global options can be edited
	// without touching connection state. They are loaded even when config.json
	// does not exist yet: a user who only ever changed the options (or the AI
	// provider) has no connections, and silently falling back to defaults would
	// discard those choices.
	if err := s.loadSettings(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

// NewMemoryStore returns a store that never touches the disk. It is used by
// tests and by callers that only need in-process state.
func NewMemoryStore() *Store {
	return &Store{settings: defaultSettings()}
}

// defaultSettings returns the settings used when no settings.json exists.
func defaultSettings() Settings {
	return Settings{Theme: "dark"}
}

// settingsFile is the path to the global options JSON file.
func settingsFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func (s *Store) loadSettings() error {
	path, err := settingsFile()
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var st Settings
	if err := json.Unmarshal(raw, &st); err != nil {
		return err
	}
	s.settings = st
	return nil
}

// Settings returns the current global options.
func (s *Store) Settings() Settings { return s.settings }

// AllowedCommands returns the command lines the user approved permanently.
func (s *Store) AllowedCommands() []string {
	return append([]string(nil), s.settings.AllowedCommands...)
}

// CommandAllowed reports whether cmd was approved permanently. It matches the
// exact line, so an approval never widens into "anything like it".
func (s *Store) CommandAllowed(cmd string) bool {
	for _, c := range s.settings.AllowedCommands {
		if c == cmd {
			return true
		}
	}
	return false
}

// AllowCommand records a permanent approval for cmd. It is idempotent.
func (s *Store) AllowCommand(cmd string) error {
	if cmd == "" || s.CommandAllowed(cmd) {
		return nil
	}
	st := s.settings
	st.AllowedCommands = append(st.AllowedCommands, cmd)
	return s.UpdateSettings(st)
}

// ForgetCommand drops one permanent approval.
func (s *Store) ForgetCommand(cmd string) error {
	st := s.settings
	kept := make([]string, 0, len(st.AllowedCommands))
	for _, c := range st.AllowedCommands {
		if c != cmd {
			kept = append(kept, c)
		}
	}
	if len(kept) == len(st.AllowedCommands) {
		return nil
	}
	st.AllowedCommands = kept
	return s.UpdateSettings(st)
}

// ForgetAllCommands clears every permanent approval.
func (s *Store) ForgetAllCommands() error {
	st := s.settings
	if len(st.AllowedCommands) == 0 {
		return nil
	}
	st.AllowedCommands = nil
	return s.UpdateSettings(st)
}

// UpdateSettings persists the given global options to settings.json.
func (s *Store) UpdateSettings(st Settings) error {
	s.settings = st
	if s.path == "" {
		// In-memory store: nothing is persisted.
		return nil
	}
	path, err := settingsFile()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Save writes state to disk when something changed. A store without a path is
// in-memory (see NewMemoryStore) and only clears the dirty flag.
func (s *Store) Save() error {
	if !s.dirty {
		return nil
	}
	if s.path == "" {
		s.dirty = false
		return nil
	}
	type disk struct {
		Connections []*Connection `json:"connections"`
		Tunnels     []*Tunnel     `json:"tunnels"`
		Folders     []*Folder     `json:"folders"`
		Keys        []*SSHKey     `json:"keys"`
	}
	d := disk{Connections: s.connections, Tunnels: s.tunnels, Folders: s.folders, Keys: s.keys}
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

// Connections returns a copy of the connection slice in display order (by
// Order, with ties keeping their stored order).
func (s *Store) Connections() []*Connection {
	out := make([]*Connection, len(s.connections))
	copy(out, s.connections)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// Connection returns the saved connection with the given ID, or nil when it does
// not exist. It satisfies the resolver used for jump hosts.
func (s *Store) Connection(id string) *Connection {
	for _, c := range s.connections {
		if c.ID == id {
			return c
		}
	}
	return nil
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

// DeleteConnection removes a connection and any tunnels that depend on it. It
// also drops the connection from every jump-host chain, so no saved profile is
// left pointing at a missing bastion.
func (s *Store) DeleteConnection(id string) error {
	kept := s.connections[:0]
	for _, c := range s.connections {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	s.connections = kept

	for _, c := range s.connections {
		if len(c.JumpHostIDs) == 0 {
			continue
		}
		chain := c.JumpHostIDs[:0]
		for _, hop := range c.JumpHostIDs {
			if hop != id {
				chain = append(chain, hop)
			}
		}
		c.JumpHostIDs = chain
	}

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

// Folders returns a copy of the folder slice in display order (by Order, with
// ties keeping their stored order).
func (s *Store) Folders() []*Folder {
	out := make([]*Folder, len(s.folders))
	copy(out, s.folders)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// AddFolder inserts a new folder and persists.
func (s *Store) AddFolder(f *Folder) error {
	s.folders = append(s.folders, f)
	s.mark()
	return s.Save()
}

// UpdateFolder replaces a folder by ID.
func (s *Store) UpdateFolder(f *Folder) error {
	for i, existing := range s.folders {
		if existing.ID == f.ID {
			s.folders[i] = f
			s.mark()
			return s.Save()
		}
	}
	return errors.New("folder not found")
}

// DeleteFolder removes a folder by ID and ungroups any connections inside it.
func (s *Store) DeleteFolder(id string) error {
	kept := s.folders[:0]
	for _, f := range s.folders {
		if f.ID != id {
			kept = append(kept, f)
		}
	}
	s.folders = kept
	for _, c := range s.connections {
		if c.FolderID == id {
			c.FolderID = ""
		}
	}
	s.mark()
	return s.Save()
}

// MoveConnection moves a connection into a folder (empty folderID = ungrouped)
// and places it at position index within that folder, renumbering the affected
// groups. It is what drag-and-drop in the session tree persists.
func (s *Store) MoveConnection(id, folderID string, index int) error {
	var moved *Connection
	for _, c := range s.connections {
		if c.ID == id {
			moved = c
			break
		}
	}
	if moved == nil {
		return errors.New("connection not found")
	}

	// Collect the destination group without the moved connection, so a move
	// inside the same folder is a plain reorder.
	group := make([]*Connection, 0, len(s.connections))
	for _, c := range s.connections {
		if c.ID != id && c.FolderID == folderID {
			group = append(group, c)
		}
	}
	if index < 0 || index > len(group) {
		index = len(group)
	}

	moved.FolderID = folderID
	group = append(group, nil)
	copy(group[index+1:], group[index:])
	group[index] = moved
	for i, c := range group {
		c.Order = i
	}

	s.mark()
	return s.Save()
}

// ReorderConnections renumbers one group in the given order. IDs that are not in
// the group are ignored, and any group member missing from ids keeps its stored
// order at the end.
func (s *Store) ReorderConnections(folderID string, ids []string) error {
	rank := make(map[string]int, len(ids))
	for i, id := range ids {
		rank[id] = i
	}
	var group []*Connection
	for _, c := range s.connections {
		if c.FolderID == folderID {
			group = append(group, c)
		}
	}
	sort.SliceStable(group, func(i, j int) bool {
		ri, iok := rank[group[i].ID]
		rj, jok := rank[group[j].ID]
		if iok != jok {
			return iok
		}
		if iok && jok {
			return ri < rj
		}
		return group[i].Order < group[j].Order
	})
	for i, c := range group {
		c.Order = i
	}
	s.mark()
	return s.Save()
}

// ReorderFolders renumbers the folders in the given order, ignoring unknown IDs.
func (s *Store) ReorderFolders(ids []string) error {
	rank := make(map[string]int, len(ids))
	for i, id := range ids {
		rank[id] = i
	}
	sort.SliceStable(s.folders, func(i, j int) bool {
		ri, iok := rank[s.folders[i].ID]
		rj, jok := rank[s.folders[j].ID]
		if iok != jok {
			return iok
		}
		if iok && jok {
			return ri < rj
		}
		return s.folders[i].Order < s.folders[j].Order
	})
	for i, f := range s.folders {
		f.Order = i
	}
	s.mark()
	return s.Save()
}

// Keys returns a copy of the managed key slice.
func (s *Store) Keys() []*SSHKey {
	out := make([]*SSHKey, len(s.keys))
	copy(out, s.keys)
	return out
}

// AddKey inserts a new managed key and persists.
func (s *Store) AddKey(k *SSHKey) error {
	s.keys = append(s.keys, k)
	s.mark()
	return s.Save()
}

// UpdateKey replaces a managed key by ID.
func (s *Store) UpdateKey(k *SSHKey) error {
	for i, existing := range s.keys {
		if existing.ID == k.ID {
			s.keys[i] = k
			s.mark()
			return s.Save()
		}
	}
	return errors.New("key not found")
}

// DeleteKey removes a managed key and clears the reference from any connection
// that used it, so no connection is left pointing at a missing key.
func (s *Store) DeleteKey(id string) error {
	kept := s.keys[:0]
	for _, k := range s.keys {
		if k.ID != id {
			kept = append(kept, k)
		}
	}
	s.keys = kept
	for _, c := range s.connections {
		if c.KeyID == id {
			c.KeyID = ""
		}
	}
	s.mark()
	return s.Save()
}

// KeyMaterial decrypts a managed key for use as an SSH identity. It returns the
// PEM-encoded private key and its passphrase (empty when the key is not
// encrypted). It satisfies sshclient.KeyResolver.
func (s *Store) KeyMaterial(id string) (string, string, error) {
	for _, k := range s.keys {
		if k.ID != id {
			continue
		}
		pem, err := DecryptSecret(k.EncryptedPrivateKey)
		if err != nil {
			return "", "", err
		}
		var passphrase string
		if k.EncryptedPassphrase != "" {
			passphrase, err = DecryptSecret(k.EncryptedPassphrase)
			if err != nil {
				return "", "", err
			}
		}
		return pem, passphrase, nil
	}
	return "", "", errors.New("key not found")
}
