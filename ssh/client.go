package sshclient

import (
	"crypto/x509"
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

// PassphraseError reports that a managed private key could not be used because
// its passphrase was missing or wrong. The UI resolves it by prompting for the
// passphrase and retrying; nothing needs to be re-persisted.
type PassphraseError struct {
	// KeyID identifies the managed key that needs a passphrase (empty when it
	// could not be determined).
	KeyID string
}

func (e *PassphraseError) Error() string {
	return "该私钥已加密，需要口令"
}

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
	signer, serr := signerFor(c, resolve)
	if serr != nil {
		// The key is passphrase-protected and we have no usable passphrase.
		// Only surface it when password auth can't cover this dial; otherwise
		// the key is skipped and the password below is tried.
		if password == "" {
			return nil, serr
		}
	} else if signer != nil {
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

// KeyMaterial is decrypted private key material ready for authentication. ID
// is the managed key it came from (empty for ad-hoc callers without one).
type KeyMaterial struct {
	ID         string
	PEM        string
	Passphrase string
}

// DialAdhoc establishes a client for a temporary (unsaved) host, as used by the
// quick-connect bar. As with saved connections, keys are tried before the
// password so a key-only server still works. keyPassphrase is the passphrase
// typed at a connect prompt; it is applied to encrypted managed keys that have
// no stored passphrase (an ad-hoc dial cannot target one specific key).
func DialAdhoc(host string, port int, user, password, keyPassphrase string, keys []KeyMaterial) (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	var needPass string
	for _, k := range keys {
		if k.PEM == "" {
			continue
		}
		pass := k.Passphrase
		if pass == "" {
			pass = keyPassphrase
		}
		signer, err := parseSigner([]byte(k.PEM), pass)
		if err != nil {
			// Missing or wrong passphrase: report it so the UI can prompt
			// again (a wrong typed passphrase must not end in a dead end).
			if PassphraseProblem(err) && needPass == "" {
				needPass = k.ID
			}
			continue
		}
		cfg.Auth = append(cfg.Auth, ssh.PublicKeys(signer))
	}
	if password != "" {
		cfg.Auth = append(cfg.Auth, ssh.Password(password))
	}
	if len(cfg.Auth) == 0 {
		// Nothing usable: if an encrypted key blocked us, ask for its
		// passphrase instead of reporting a dead end.
		if password == "" && needPass != "" {
			return nil, &PassphraseError{KeyID: needPass}
		}
		return nil, fmt.Errorf("no authentication method configured for %s", host)
	}

	if port == 0 {
		port = 22
	}
	return ssh.Dial("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)), cfg)
}

// signerFor returns the first usable signer for the profile: a managed key
// referenced by ID, then a local identity file. A PassphraseError is returned
// when the managed key is passphrase-protected and no usable passphrase was
// available; it carries the key ID so the UI can prompt for that key.
func signerFor(c *storage.Connection, resolve KeyResolver) (ssh.Signer, error) {
	if c.KeyID != "" && resolve != nil {
		if pem, passphrase, err := resolve(c.KeyID); err == nil && pem != "" {
			signer, perr := parseSigner([]byte(pem), passphrase)
			if perr == nil {
				return signer, nil
			}
			if PassphraseProblem(perr) {
				return nil, &PassphraseError{KeyID: c.KeyID}
			}
		}
	}
	if c.PrivateKeyPath != "" {
		if signer, err := loadSigner(c.PrivateKeyPath); err == nil {
			return signer, nil
		}
	}
	return nil, nil
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

// isWrongPassphrase reports whether err means a supplied passphrase did not
// decrypt the key (as opposed to no passphrase being given at all).
func isWrongPassphrase(err error) bool {
	return errors.Is(err, x509.IncorrectPasswordError) ||
		strings.Contains(err.Error(), x509.IncorrectPasswordError.Error())
}

// PassphraseProblem reports whether err means the passphrase for a private key
// was missing or wrong, i.e. the UI should prompt for one and retry.
func PassphraseProblem(err error) bool {
	return PassphraseRequired(err) || isWrongPassphrase(err)
}

func loadSigner(path string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSigner(key, "")
}
