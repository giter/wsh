package server

import (
	"encoding/json"
	"fmt"

	sshclient "sshclient/ssh"
	"sshclient/storage"
)

// connView is the connection shape sent to the browser. It never carries the
// stored ciphertext; hasPassword lets the UI show whether a password exists.
type connView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	User           string `json:"user"`
	FolderID       string `json:"folderId"`
	SavePassword   bool   `json:"savePassword"`
	HasPassword    bool   `json:"hasPassword"`
	PrivateKeyPath string `json:"privateKeyPath"`
	Color          string `json:"color"`
}

func toConnView(c *storage.Connection) connView {
	return connView{
		ID:             c.ID,
		Name:           c.Name,
		Host:           c.Host,
		Port:           c.Port,
		User:           c.User,
		FolderID:       c.FolderID,
		SavePassword:   c.SavePassword,
		HasPassword:    c.EncryptedPassword != "",
		PrivateKeyPath: c.PrivateKeyPath,
		Color:          c.Color,
	}
}

type saveConnParams struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	User           string `json:"user"`
	FolderID       string `json:"folderId"`
	Password       string `json:"password"`
	SavePassword   bool   `json:"savePassword"`
	PrivateKeyPath string `json:"privateKeyPath"`
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
			Color:             "#34D399",
		}
		if err := s.store.AddConnection(conn); err != nil {
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
	if err := s.store.UpdateConnection(existing); err != nil {
		return nil, err
	}
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
	var p struct {
		ID       string `json:"id"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		KeyPath  string `json:"keyPath"`
	}
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
	}
	// If editing an existing connection and no password given, fall back to
	// the stored one so the test works without re-typing.
	if p.ID != "" && p.Password == "" {
		if existing, err := s.findConnection(p.ID); err == nil {
			conn.EncryptedPassword = existing.EncryptedPassword
		}
	}
	client, err := sshclient.Dial(conn, p.Password)
	if err != nil {
		return nil, fmt.Errorf("连接失败：%w", err)
	}
	_ = client.Close()
	return "连接成功", nil
}

func (s *Server) findConnection(id string) (*storage.Connection, error) {
	for _, cc := range s.store.Connections() {
		if cc.ID == id {
			return cc, nil
		}
	}
	return nil, fmt.Errorf("连接不存在")
}
