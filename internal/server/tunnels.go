package server

import (
	"encoding/json"
	"fmt"

	"sshclient/storage"
)

// tunnelView is the tunnel shape sent to the browser.
type tunnelView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Direction      string `json:"direction"`
	LocalAddress   string `json:"localAddress"`
	LocalPort      int    `json:"localPort"`
	RemoteAddress  string `json:"remoteAddress"`
	RemotePort     int    `json:"remotePort"`
	Remark         string `json:"remark"`
	Running        bool   `json:"running"`
}

func (s *Server) tunnelView(t *storage.Tunnel) tunnelView {
	name := t.ConnectionID
	for _, cc := range s.store.Connections() {
		if cc.ID == t.ConnectionID {
			name = cc.Name
			break
		}
	}
	return tunnelView{
		ID:             t.ID,
		Name:           t.Name,
		ConnectionID:   t.ConnectionID,
		ConnectionName: name,
		Direction:      t.Direction,
		LocalAddress:   t.LocalAddress,
		LocalPort:      t.LocalPort,
		RemoteAddress:  t.RemoteAddress,
		RemotePort:     t.RemotePort,
		Remark:         t.Remark,
		Running:        s.tunnels.IsRunning(t.ID),
	}
}

func (s *Server) handleListTunnels(c *wsClient, params json.RawMessage) (interface{}, error) {
	ts := s.store.Tunnels()
	out := make([]tunnelView, 0, len(ts))
	for _, t := range ts {
		out = append(out, s.tunnelView(t))
	}
	return out, nil
}

type saveTunnelParams struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ConnectionID  string `json:"connectionId"`
	Direction     string `json:"direction"`
	LocalAddress  string `json:"localAddress"`
	LocalPort     int    `json:"localPort"`
	RemoteAddress string `json:"remoteAddress"`
	RemotePort    int    `json:"remotePort"`
	Remark        string `json:"remark"`
}

func (s *Server) handleSaveTunnel(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p saveTunnelParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, fmt.Errorf("请填写隧道名称")
	}
	if p.LocalPort < 1 || p.LocalPort > 65535 || p.RemotePort < 1 || p.RemotePort > 65535 {
		return nil, fmt.Errorf("端口必须是 1-65535 之间的数字")
	}
	if p.ConnectionID == "" {
		return nil, fmt.Errorf("请选择连接")
	}
	if p.Direction == "" {
		p.Direction = "local"
	}
	if p.Direction != "local" && p.Direction != "remote" {
		return nil, fmt.Errorf("方向只能是本地转发或远程转发")
	}
	if p.LocalAddress == "" {
		p.LocalAddress = "127.0.0.1"
	}
	if p.RemoteAddress == "" {
		p.RemoteAddress = "127.0.0.1"
	}

	if p.ID == "" {
		t := &storage.Tunnel{
			ID:            storage.NewID(),
			Name:          p.Name,
			ConnectionID:  p.ConnectionID,
			Direction:     p.Direction,
			LocalAddress:  p.LocalAddress,
			LocalPort:     p.LocalPort,
			RemoteAddress: p.RemoteAddress,
			RemotePort:    p.RemotePort,
			Remark:        p.Remark,
		}
		if err := s.store.AddTunnel(t); err != nil {
			return nil, err
		}
		return s.tunnelView(t), nil
	}

	for _, t := range s.store.Tunnels() {
		if t.ID == p.ID {
			t.Name = p.Name
			t.ConnectionID = p.ConnectionID
			t.Direction = p.Direction
			t.LocalAddress = p.LocalAddress
			t.LocalPort = p.LocalPort
			t.RemoteAddress = p.RemoteAddress
			t.RemotePort = p.RemotePort
			t.Remark = p.Remark
			if err := s.store.UpdateTunnel(t); err != nil {
				return nil, err
			}
			return s.tunnelView(t), nil
		}
	}
	return nil, fmt.Errorf("隧道不存在")
}

func (s *Server) handleDeleteTunnel(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	s.tunnels.Stop(p.ID)
	if err := s.store.DeleteTunnel(p.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Server) handleStartTunnel(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		ID string `json:"id"`
		// Credentials typed at a connect prompt, used when the connection has no
		// usable saved password.
		Password      string `json:"password,omitempty"`
		KeyPassphrase string `json:"keyPassphrase,omitempty"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	for _, t := range s.store.Tunnels() {
		if t.ID == p.ID {
			conn, err := s.findConnection(t.ConnectionID)
			if err != nil {
				return nil, err
			}
			if p.KeyPassphrase != "" && conn.KeyID != "" {
				s.pool.ProvidePassphrase(conn.KeyID, p.KeyPassphrase)
			}
			var passPtr *string
			if p.Password != "" {
				passPtr = &p.Password
			}
			if _, err := s.tunnels.StartWithPassword(conn, t, passPtr); err != nil {
				if prompt, ok := s.credentialPrompt(err); ok {
					return prompt, nil
				}
				return nil, err
			}
			return s.tunnelView(t), nil
		}
	}
	return nil, fmt.Errorf("隧道不存在")
}

func (s *Server) handleStopTunnel(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	s.tunnels.Stop(p.ID)
	return nil, nil
}
