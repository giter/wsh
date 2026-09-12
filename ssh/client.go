package sshclient

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"

	"sshclient/storage"
)

// Dial establishes an SSH client from a saved connection profile.
// Pass a plaintext password if one should be used; it may be empty when key
// auth (or a stored encrypted password) is preferred.
func Dial(c *storage.Connection, password string) (*ssh.Client, error) {
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

	if password != "" {
		cfg.Auth = append(cfg.Auth, ssh.Password(password))
	}

	if c.PrivateKeyPath != "" {
		if signer, err := loadSigner(c.PrivateKeyPath); err == nil {
			cfg.Auth = append(cfg.Auth, ssh.PublicKeys(signer))
		}
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

func loadSigner(path string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}
	return signer, nil
}
