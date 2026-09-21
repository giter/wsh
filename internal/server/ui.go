package server

// Push message types sent from the native menu bar to the web frontend.
const (
	// UINavigate opens a page; UIMsg.Page selects it.
	UINavigate = "ui.navigate"
	// UINewConnection opens the connection editor modal.
	UINewConnection = "ui.new-connection"
	// UISettingsChanged tells other windows to re-apply global options.
	UISettingsChanged = "ui.settings-changed"
	// UIKeysChanged tells other windows that the key manager changed.
	UIKeysChanged = "ui.keys-changed"
)

// UIMsg is a generic push message sent from the native desktop shell (menu
// bar) to the web frontend over the existing WebSocket channel.
//
//   - Type == UINavigate: Page names a page to open ("home", "connections",
//     "sftp", "tunnels").
//   - Type == UINewConnection: opens the connection editor modal.
//   - Type == UISettingsChanged: reload global options (font, theme, ...).
//   - Type == UIKeysChanged: refresh the managed-key list.
type UIMsg struct {
	Type string `json:"type"`
	Page string `json:"page,omitempty"`
}
