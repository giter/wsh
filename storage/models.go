package storage

// Connection describes a saved SSH server profile.
type Connection struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	// EncryptedPassword is the AES-GCM ciphertext (base64) of the password,
	// or empty when only key-based auth is used.
	EncryptedPassword string `json:"encrypted_password,omitempty"`
	// SavePassword records whether the password should be persisted.
	SavePassword bool `json:"save_password"`
	// PrivateKeyPath is an optional path to an identity file for key auth.
	PrivateKeyPath string `json:"private_key_path,omitempty"`
	// Color is the accent color used to identify this connection in the UI.
	Color string `json:"color"`
}

// Tunnel describes a local -> remote TCP forward.
type Tunnel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ConnectionID  string `json:"connection_id"`
	LocalAddress  string `json:"local_address"`
	LocalPort     int    `json:"local_port"`
	RemoteAddress string `json:"remote_address"`
	RemotePort    int    `json:"remote_port"`
	Enabled       bool   `json:"enabled"`
}
