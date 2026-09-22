package server

import (
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/ssh"

	sshclient "sshclient/ssh"
	"sshclient/storage"
)

// connView is the connection shape sent to the browser. It never carries the
// stored ciphertext; hasPassword lets the UI show whether a password exists.
type connView struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	User           string   `json:"user"`
	FolderID       string   `json:"folderId"`
	Order          int      `json:"order"`
	SavePassword   bool     `json:"savePassword"`
	HasPassword    bool     `json:"hasPassword"`
	PrivateKeyPath string   `json:"privateKeyPath"`
	KeyID          string   `json:"keyId"`
	Color          string   `json:"color"`
	JumpHostIDs    []string `json:"jumpHostIds"`
}

func toConnView(c *storage.Connection) connView {
	return connView{
		ID:             c.ID,
		Name:           c.Name,
		Host:           c.Host,
		Port:           c.Port,
		User:           c.User,
		FolderID:       c.FolderID,
		Order:          c.Order,
		SavePassword:   c.SavePassword,
		HasPassword:    c.EncryptedPassword != "",
		PrivateKeyPath: c.PrivateKeyPath,
		KeyID:          c.KeyID,
		Color:          c.Color,
		JumpHostIDs:    c.JumpHostIDs,
	}
}

type saveConnParams struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	User           string   `json:"user"`
	FolderID       string   `json:"folderId"`
	Password       string   `json:"password"`
	SavePassword   bool     `json:"savePassword"`
	PrivateKeyPath string   `json:"privateKeyPath"`
	KeyID          string   `json:"keyId"`
	JumpHostIDs    []string `json:"jumpHostIds"`
}

// cleanJumpHosts validates a requested bastion chain: every entry must exist,
// must not be the connection itself and must not repeat. Unknown entries are
// dropped rather than failing the save, so a deleted bastion cannot block an
// unrelated edit.
func (s *Server) cleanJumpHosts(selfID string, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := map[string]bool{}
	if selfID != "" {
		seen[selfID] = true
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		if s.store.Connection(id) == nil {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func (s *Server) handleListConnections(c *wsClient, params json.RawMessage) (interface{}, error) {
	conns := s.store.Connections()
	out := make([]connView, 0, len(conns))
	for _, cc := range conns {
		out = append(out, toConnView(cc))
	}
	return out, nil
}

func (s *Server) handleSaveConnection(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p saveConnParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Name == "" || p.Host == "" || p.User == "" {
		return nil, fmt.Errorf("名称、主机和用户不能为空")
	}
	if p.Port < 1 || p.Port > 65535 {
		return nil, fmt.Errorf("端口必须是 1-65535 之间的数字")
	}

	var enc string
	if p.SavePassword && p.Password != "" {
		var err error
		enc, err = storage.EncryptPassword(p.Password)
		if err != nil {
			return nil, fmt.Errorf("保存密码失败：%w", err)
		}
	}

	if p.ID == "" {
		conn := &storage.Connection{
			ID:                storage.NewID(),
			Name:              p.Name,
			Host:              p.Host,
			Port:              p.Port,
			User:              p.User,
			FolderID:          p.FolderID,
			EncryptedPassword: enc,
			SavePassword:      p.SavePassword,
			PrivateKeyPath:    p.PrivateKeyPath,
			KeyID:             p.KeyID,
			Color:             "#34D399",
			JumpHostIDs:       s.cleanJumpHosts("", p.JumpHostIDs),
		}
		if err := s.store.AddConnection(conn); err != nil {
			return nil, err
		}
		// Append to the end of its folder so a new connection does not jump to
		// the top of a group the user has arranged by hand.
		if err := s.store.MoveConnection(conn.ID, p.FolderID, -1); err != nil {
			return nil, err
		}
		return toConnView(conn), nil
	}

	existing, err := s.findConnection(p.ID)
	if err != nil {
		return nil, err
	}
	// Blank password field means "keep the stored password".
	if enc == "" {
		enc = existing.EncryptedPassword
		p.SavePassword = existing.SavePassword || p.SavePassword
	}
	existing.Name = p.Name
	existing.Host = p.Host
	existing.Port = p.Port
	existing.User = p.User
	existing.FolderID = p.FolderID
	existing.EncryptedPassword = enc
	existing.SavePassword = p.SavePassword
	existing.PrivateKeyPath = p.PrivateKeyPath
	existing.KeyID = p.KeyID
	existing.JumpHostIDs = s.cleanJumpHosts(existing.ID, p.JumpHostIDs)
	if err := s.store.UpdateConnection(existing); err != nil {
		return nil, err
	}
	// The dial parameters may have changed, so drop any live client.
	s.pool.Drop(existing.ID)
	return toConnView(existing), nil
}

func (s *Server) handleDeleteConnection(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	// Drop pooled connection and any tunnels that depend on it.
	s.pool.Drop(p.ID)
	s.tunnels.StopByConnection(p.ID)
	if err := s.store.DeleteConnection(p.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Server) handleTestConnection(c *wsClient, params json.RawMessage) (interface{}, error) {
	type testConnParams struct {
		ID            string   `json:"id"`
		Host          string   `json:"host"`
		Port          int      `json:"port"`
		User          string   `json:"user"`
		Password      string   `json:"password"`
		KeyPath       string   `json:"keyPath"`
		KeyID         string   `json:"keyId"`
		KeyPassphrase string   `json:"keyPassphrase"`
		JumpHostIDs   []string `json:"jumpHostIds"`
	}
	var p testConnParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Port == 0 {
		p.Port = 22
	}
	conn := &storage.Connection{
		Host:           p.Host,
		Port:           p.Port,
		User:           p.User,
		PrivateKeyPath: p.KeyPath,
		KeyID:          p.KeyID,
		JumpHostIDs:    s.cleanJumpHosts(p.ID, p.JumpHostIDs),
	}
	// If editing an existing connection and no password given, fall back to
	// the stored one so the test works without re-typing.
	if p.ID != "" && p.Password == "" {
		if existing, err := s.findConnection(p.ID); err == nil {
			conn.EncryptedPassword = existing.EncryptedPassword
		}
	}
	// The resolver applies a passphrase typed at the prompt to managed keys
	// that have none stored, mirroring the connect flow.
	resolve := func(keyID string) (string, string, error) {
		pem, pass, err := s.store.KeyMaterial(keyID)
		if err == nil && pass == "" && p.KeyPassphrase != "" {
			pass = p.KeyPassphrase
		}
		return pem, pass, err
	}
	client, err := s.dialForTest(conn, p.Password, resolve)
	if err != nil {
		var pe *sshclient.PassphraseError
		if errors.As(err, &pe) {
			name := ""
			if k, kerr := s.findKey(pe.KeyID); kerr == nil {
				name = k.Name
			}
			msg := "该私钥已加密，请输入口令"
			if name != "" {
				msg = fmt.Sprintf("私钥「%s」已加密，请输入口令", name)
			}
			return map[string]interface{}{"needPassphrase": true, "keyId": pe.KeyID, "message": msg}, nil
		}
		return nil, fmt.Errorf("连接失败：%w", err)
	}
	_ = client.Close()
	return "连接成功", nil
}

// moveConnectionParams is the payload of a drag-and-drop move in the session
// tree: the connection, its destination folder ("" = ungrouped) and the index it
// should occupy inside that folder.
type moveConnectionParams struct {
	ID       string `json:"id"`
	FolderID string `json:"folderId"`
	Index    int    `json:"index"`
}

func (s *Server) handleMoveConnection(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p moveConnectionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.ID == "" {
		return nil, fmt.Errorf("缺少连接 ID")
	}
	// An unknown folder would leave the connection invisible in the tree.
	if p.FolderID != "" {
		found := false
		for _, f := range s.store.Folders() {
			if f.ID == p.FolderID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("文件夹不存在")
		}
	}
	if err := s.store.MoveConnection(p.ID, p.FolderID, p.Index); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Server) handleReorderConnections(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		FolderID string   `json:"folderId"`
		IDs      []string `json:"ids"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if err := s.store.ReorderConnections(p.FolderID, p.IDs); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Server) handleReorderFolders(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if err := s.store.ReorderFolders(p.IDs); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Server) findConnection(id string) (*storage.Connection, error) {
	for _, cc := range s.store.Connections() {
		if cc.ID == id {
			return cc, nil
		}
	}
	return nil, fmt.Errorf("连接不存在")
}

// dialForTest opens a connection the way the terminal would, including any
// jump-host chain, so "测试连接" validates the whole path.
func (s *Server) dialForTest(conn *storage.Connection, password string, resolve sshclient.KeyResolver) (*ssh.Client, error) {
	chain, err := sshclient.JumpChain(conn, s.store.Connection)
	if err != nil {
		return nil, err
	}
	if len(chain) > 0 {
		return sshclient.DialChain(chain, conn, password, resolve)
	}
	return sshclient.Dial(conn, password, resolve)
}
