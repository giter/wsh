package server

// Push message types sent from the native desktop shell to the web frontend.
//
// The app is split across several native windows (session manager, terminal
// sessions, file transfer, tunnels, options, key manager). They all talk to the
// same backend, so a push that concerns one of them is broadcast to every
// window and each window reacts only to the types it owns.
const (
	// UIOpenSession asks the session window to open a terminal tab. It carries
	// either ConnID (a saved connection) or Host/Port/User (a one-off target).
	UIOpenSession = "ui.open-session"
	// UINewConnection opens the connection editor. It is pushed by the native
	// menu bar; the app's own menu calls the same action locally.
	UINewConnection = "ui.new-connection"
	// UIConnectionsChanged tells the other windows that connections or folders
	// changed, so their connection pickers and session trees refresh.
	UIConnectionsChanged = "ui.connections-changed"
	// UISettingsChanged tells other windows to re-apply global options.
	UISettingsChanged = "ui.settings-changed"
	// UIKeysChanged tells other windows that the key manager changed.
	UIKeysChanged = "ui.keys-changed"
)

// UIMsg is a generic push message sent from the native desktop shell (menu
// bar) to the web frontend over the existing WebSocket channel.
//
//   - Type == UIOpenSession: open a terminal tab for ConnID (saved) or
//     Host/Port/User (ad-hoc quick connect).
//   - Type == UINewConnection: opens the connection editor modal.
//   - Type == UIConnectionsChanged: reload the connection / folder lists.
//   - Type == UISettingsChanged: reload global options (font, theme, ...).
//   - Type == UIKeysChanged: refresh the managed-key list.
type UIMsg struct {
	Type   string `json:"type"`
	ConnID string `json:"connId,omitempty"`
	Host   string `json:"host,omitempty"`
	Port   int    `json:"port,omitempty"`
	User   string `json:"user,omitempty"`
}
