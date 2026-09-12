package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
)

// keyFile is the file holding the machine-local key used to encrypt stored
// passwords. Keeping the key on the machine (not embedded in the config) means
// the stored ciphertext is useless if the config alone is copied away.
const keyFile = "secret.key"

// machineKey loads or creates the persistent machine key.
func machineKey() ([]byte, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, keyFile)

	raw, err := os.ReadFile(path)
	if err == nil {
		if len(raw) == 32 {
			return raw, nil
		}
		// Derive a 32-byte key from whatever length we have.
		sum := sha256.Sum256(raw)
		return sum[:], nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Create a fresh random key.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// EncryptPassword protects a plaintext password for storage.
func EncryptPassword(plain string) (string, error) {
	key, err := machineKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptPassword recovers a plaintext password from its stored form.
func DecryptPassword(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	key, err := machineKey()
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
