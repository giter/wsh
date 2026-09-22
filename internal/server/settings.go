package server

import (
	"encoding/json"

	"sshclient/storage"
)

func (s *Server) handleGetSettings(c *wsClient, params json.RawMessage) (interface{}, error) {
	return toSettingsView(s.store.Settings()), nil
}

// settingsView mirrors storage.Settings in camelCase for the browser. The AI key
// is never sent back: hasAIKey tells the UI whether one is stored.
type settingsView struct {
	FontSize         int    `json:"fontSize"`
	Theme            string `json:"theme"`
	DefaultPort      int    `json:"defaultPort"`
	DefaultUser      string `json:"defaultUser"`
	TunnelLocalPort  int    `json:"tunnelLocalPort"`
	TunnelRemotePort int    `json:"tunnelRemotePort"`

	AIProvider    string `json:"aiProvider"`
	AIBaseURL     string `json:"aiBaseUrl"`
	AIModel       string `json:"aiModel"`
	HasAIKey      bool   `json:"hasAiKey"`
	AIAutoAnalyze bool   `json:"aiAutoAnalyze"`
	AINoContext   bool   `json:"aiNoContext"`
}

func toSettingsView(st storage.Settings) settingsView {
	return settingsView{
		FontSize:         st.FontSize,
		Theme:            st.Theme,
		DefaultPort:      st.DefaultPort,
		DefaultUser:      st.DefaultUser,
		TunnelLocalPort:  st.TunnelLocalPort,
		TunnelRemotePort: st.TunnelRemotePort,

		AIProvider:    st.AIProvider,
		AIBaseURL:     st.AIBaseURL,
		AIModel:       st.AIModel,
		HasAIKey:      st.AIKeyEncrypted != "",
		AIAutoAnalyze: st.AIAutoAnalyze,
		AINoContext:   st.AINoContext,
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

	AIProvider string `json:"aiProvider"`
	AIBaseURL  string `json:"aiBaseUrl"`
	AIModel    string `json:"aiModel"`
	// AIKey is a plaintext key; empty keeps whatever is already stored.
	AIKey string `json:"aiKey"`
	// ClearAIKey removes the stored key instead of keeping it.
	ClearAIKey    bool `json:"clearAiKey"`
	AIAutoAnalyze bool `json:"aiAutoAnalyze"`
	AINoContext   bool `json:"aiNoContext"`
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
	switch p.AIProvider {
	case "", "openai", "ollama":
	default:
		p.AIProvider = ""
	}

	current := s.store.Settings()
	encKey := current.AIKeyEncrypted
	switch {
	case p.ClearAIKey:
		encKey = ""
	case p.AIKey != "":
		enc, err := storage.EncryptPassword(p.AIKey)
		if err != nil {
			return nil, err
		}
		encKey = enc
	}

	st := storage.Settings{
		FontSize:         p.FontSize,
		Theme:            p.Theme,
		DefaultPort:      p.DefaultPort,
		DefaultUser:      p.DefaultUser,
		TunnelLocalPort:  p.TunnelLocalPort,
		TunnelRemotePort: p.TunnelRemotePort,

		AIProvider:     p.AIProvider,
		AIBaseURL:      p.AIBaseURL,
		AIModel:        p.AIModel,
		AIKeyEncrypted: encKey,
		AIAutoAnalyze:  p.AIAutoAnalyze,
		AINoContext:    p.AINoContext,
	}
	if err := s.store.UpdateSettings(st); err != nil {
		return nil, err
	}
	// Tell every open window (main window, settings window, ...) to re-apply
	// the new options so font/theme changes show up immediately.
	s.NotifyAll(UIMsg{Type: UISettingsChanged})
	return toSettingsView(st), nil
}
