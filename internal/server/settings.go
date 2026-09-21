package server

import (
	"encoding/json"

	"sshclient/storage"
)

func (s *Server) handleGetSettings(c *wsClient, params json.RawMessage) (interface{}, error) {
	return toSettingsView(s.store.Settings()), nil
}

// settingsView mirrors storage.Settings in camelCase for the browser.
type settingsView struct {
	FontSize         int    `json:"fontSize"`
	Theme            string `json:"theme"`
	DefaultPort      int    `json:"defaultPort"`
	DefaultUser      string `json:"defaultUser"`
	TunnelLocalPort  int    `json:"tunnelLocalPort"`
	TunnelRemotePort int    `json:"tunnelRemotePort"`
}

func toSettingsView(st storage.Settings) settingsView {
	return settingsView{
		FontSize:         st.FontSize,
		Theme:            st.Theme,
		DefaultPort:      st.DefaultPort,
		DefaultUser:      st.DefaultUser,
		TunnelLocalPort:  st.TunnelLocalPort,
		TunnelRemotePort: st.TunnelRemotePort,
	}
}

// saveSettingsParams mirrors storage.Settings in camelCase for the browser.
type saveSettingsParams struct {
	FontSize         int    `json:"fontSize"`
	Theme            string `json:"theme"`
	DefaultPort      int    `json:"defaultPort"`
	DefaultUser      string `json:"defaultUser"`
	TunnelLocalPort  int    `json:"tunnelLocalPort"`
	TunnelRemotePort int    `json:"tunnelRemotePort"`
}

func (s *Server) handleSaveSettings(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p saveSettingsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Theme != "dark" && p.Theme != "light" {
		p.Theme = "dark"
	}
	if p.FontSize < 8 || p.FontSize > 32 {
		p.FontSize = 0 // keep stored/default
	}
	st := storage.Settings{
		FontSize:         p.FontSize,
		Theme:            p.Theme,
		DefaultPort:      p.DefaultPort,
		DefaultUser:      p.DefaultUser,
		TunnelLocalPort:  p.TunnelLocalPort,
		TunnelRemotePort: p.TunnelRemotePort,
	}
	if err := s.store.UpdateSettings(st); err != nil {
		return nil, err
	}
	// Tell every open window (main window, settings window, ...) to re-apply
	// the new options so font/theme changes show up immediately.
	s.NotifyAll(UIMsg{Type: UISettingsChanged})
	return toSettingsView(st), nil
}
