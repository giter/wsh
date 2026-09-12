package server

// Push message types sent from the native menu bar to the web frontend.
const (
	// UINavigate opens a page; UIMsg.Page selects it.
	UINavigate = "ui.navigate"
	// UINewConnection opens the connection editor modal.
	UINewConnection = "ui.new-connection"
	// UIOpenSettings opens the global options modal.
	UIOpenSettings = "ui.open-settings"
)

// UIMsg is a generic push message sent from the native desktop shell (menu
// bar) to the web frontend over the existing WebSocket channel.
//
//   - Type == UINavigate: Page names a page to open ("home", "connections",
//     "sftp", "tunnels").
//   - Type == UINewConnection: opens the connection editor modal.
//   - Type == UIOpenSettings: opens the settings modal.
type UIMsg struct {
	Type string `json:"type"`
	Page string `json:"page,omitempty"`
}
