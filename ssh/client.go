package sshclient

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"sshclient/storage"
)

// KeyResolver returns the decrypted PEM material for a managed key ID. It is
// provided by the storage layer so this package stays independent of the key
// registry (see storage.Store.KeyMaterial).
type KeyResolver func(keyID string) (pem string, passphrase string, err error)

// Dial establishes an SSH client from a saved connection profile.
// Pass a plaintext password if one should be used; it may be empty when key
// auth (or a stored encrypted password) is preferred. Key-based auth is always
// attempted before password auth (登录时优先使用密钥).
func Dial(c *storage.Connection, password string, resolve KeyResolver) (*ssh.Client, error) {
	if password == "" && c.EncryptedPassword != "" {
		dec, err := storage.DecryptPassword(c.EncryptedPassword)
		if err == nil {
			password = dec
		}
	}

	cfg := &ssh.ClientConfig{
		User:            c.User,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // host keys are accepted on first use
		Timeout:         15 * time.Second,
	}

	// Key auth first: both a managed key and a local identity file take
	// precedence over the password, so a configured key is preferred at login.
	if signer := signerFor(c, resolve); signer != nil {
		cfg.Auth = append(cfg.Auth, ssh.PublicKeys(signer))
	}

	if password != "" {
		cfg.Auth = append(cfg.Auth, ssh.Password(password))
	}

	if len(cfg.Auth) == 0 {
		return nil, fmt.Errorf("no authentication method configured for %s", c.Name)
	}

	port := c.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(c.Host, fmt.Sprintf("%d", port))
	return ssh.Dial("tcp", addr, cfg)
}

// signerFor returns the first usable signer for the profile: a managed key
// referenced by ID, then a local identity file. It returns nil when neither is
// configured or could be loaded.
func signerFor(c *storage.Connection, resolve KeyResolver) ssh.Signer {
	if c.KeyID != "" && resolve != nil {
		if pem, passphrase, err := resolve(c.KeyID); err == nil && pem != "" {
			if signer, err := parseSigner([]byte(pem), passphrase); err == nil {
				return signer
			}
		}
	}
	if c.PrivateKeyPath != "" {
		if signer, err := loadSigner(c.PrivateKeyPath); err == nil {
			return signer
		}
	}
	return nil
}

// parseSigner parses a PEM private key, decrypting it with passphrase when one
// is provided or when the key is encrypted.
func parseSigner(pem []byte, passphrase string) (ssh.Signer, error) {
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(pem, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(pem)
}

// KeyInfo describes the public half of a parsed private key.
type KeyInfo struct {
	// PublicKey is the authorized_keys line (no trailing newline).
	PublicKey string
	// Fingerprint is the SHA256 fingerprint, e.g. "SHA256:abcd...".
	Fingerprint string
	// KeyType is the SSH algorithm, e.g. "ssh-ed25519" or "ssh-rsa".
	KeyType string
}

// ParseKey validates a private key (optionally encrypted with passphrase) and
// returns its public metadata. It is used by the key manager before persisting
// a submitted key.
func ParseKey(pem []byte, passphrase string) (KeyInfo, error) {
	signer, err := parseSigner(pem, passphrase)
	if err != nil {
		return KeyInfo{}, err
	}
	pub := signer.PublicKey()
	return KeyInfo{
		PublicKey:   strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))),
		Fingerprint: ssh.FingerprintSHA256(pub),
		KeyType:     pub.Type(),
	}, nil
}

// PassphraseRequired reports whether err means the key is encrypted and needs a
// passphrase to be parsed.
func PassphraseRequired(err error) bool {
	var missing *ssh.PassphraseMissingError
	return errors.As(err, &missing)
}

func loadSigner(path string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSigner(key, "")
}
