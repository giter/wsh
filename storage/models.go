package storage

// Connection describes a saved SSH server profile.
type Connection struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	// FolderID groups this connection under a folder; empty means ungrouped.
	FolderID string `json:"folder_id,omitempty"`
	// Order is the position of this connection inside its folder (or the
	// ungrouped group). It is written by drag-and-drop reordering and sorted on
	// read, so the tree keeps the user's arrangement across restarts.
	Order int `json:"order,omitempty"`
	// EncryptedPassword is the AES-GCM ciphertext (base64) of the password,
	// or empty when only key-based auth is used.
	EncryptedPassword string `json:"encrypted_password,omitempty"`
	// SavePassword records whether the password should be persisted.
	SavePassword bool `json:"save_password"`
	// PrivateKeyPath is an optional path to an identity file for key auth.
	PrivateKeyPath string `json:"private_key_path,omitempty"`
	// KeyID references a key managed by the key manager (storage.SSHKey).
	// When set it is preferred for authentication at login time.
	KeyID string `json:"key_id,omitempty"`
	// Color is the accent color used to identify this connection in the UI.
	Color string `json:"color"`
}

// SSHKey is a user-submitted private key kept encrypted at rest (same machine
// key scheme as passwords). Connections reference it by ID for key auth.
// The private material is never sent to the browser; only the derived public
// key and its fingerprint are.
type SSHKey struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Comment is an optional free-form note shown in the key list.
	Comment string `json:"comment,omitempty"`
	// EncryptedPrivateKey is the AES-GCM ciphertext (base64) of the PEM key.
	EncryptedPrivateKey string `json:"encrypted_private_key"`
	// EncryptedPassphrase is the ciphertext of the key passphrase. It is only
	// populated when the user opted in; otherwise the passphrase is asked for
	// at connect time and never touches disk.
	EncryptedPassphrase string `json:"encrypted_passphrase,omitempty"`
	// KeyEncrypted records that the private key material itself is protected
	// by a passphrase. It is derived at save time so the UI can show whether
	// connecting will require one (the passphrase itself is never stored here
	// unless the user opted in above).
	KeyEncrypted bool `json:"key_encrypted"`
	// PublicKey is the derived authorized_keys line (safe to display).
	PublicKey string `json:"public_key,omitempty"`
	// Fingerprint is the SHA256 fingerprint of the public key.
	Fingerprint string `json:"fingerprint,omitempty"`
	// KeyType is the SSH algorithm, e.g. "ssh-ed25519" or "ssh-rsa".
	KeyType string `json:"key_type,omitempty"`
}

// Folder is a named group used to organize connections in the UI.
type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Order is the position of this folder in the session tree. Folders are
	// sorted by it so drag-and-drop reordering survives a restart; entries that
	// share an order (or predate the field) keep their stored order.
	Order int `json:"order,omitempty"`
}

// Settings holds global application options persisted in settings.json.
type Settings struct {
	// FontSize is the global UI font size in px; 0 means the default (13).
	FontSize int `json:"font_size"`
	// Theme is "dark" (default) or "light".
	Theme string `json:"theme"`
	// DefaultPort is the pre-filled SSH port for new connections (0 => 22).
	DefaultPort int `json:"default_port"`
	// DefaultUser is the pre-filled user for new connections.
	DefaultUser string `json:"default_user"`
	// TunnelLocalPort is the pre-filled local port for new tunnels (0 => 8080).
	TunnelLocalPort int `json:"tunnel_local_port"`
	// TunnelRemotePort is the pre-filled remote port for new tunnels (0 => 80).
	TunnelRemotePort int `json:"tunnel_remote_port"`
}

// Tunnel describes a TCP port forward over SSH.
// Direction is "local" (listen locally, forward to a remote address) or
// "remote" (listen on the SSH server, forward to a local address).
type Tunnel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ConnectionID  string `json:"connection_id"`
	Direction     string `json:"direction"`
	LocalAddress  string `json:"local_address"`
	LocalPort     int    `json:"local_port"`
	RemoteAddress string `json:"remote_address"`
	RemotePort    int    `json:"remote_port"`
	// Remark is an optional free-form note shown in the tunnel list.
	Remark  string `json:"remark,omitempty"`
	Enabled bool   `json:"enabled"`
}
