package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestParseKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "test@wsh")
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(block)

	info, err := ParseKey(pemBytes, "")
	if err != nil {
		t.Fatalf("ParseKey: %v", err)
	}
	if info.KeyType != "ssh-ed25519" {
		t.Errorf("KeyType = %q, want ssh-ed25519", info.KeyType)
	}
	if !strings.HasPrefix(info.PublicKey, "ssh-ed25519 ") {
		t.Errorf("PublicKey = %q", info.PublicKey)
	}
	if strings.HasSuffix(info.PublicKey, "\n") {
		t.Errorf("PublicKey should not have a trailing newline: %q", info.PublicKey)
	}
	if !strings.HasPrefix(info.Fingerprint, "SHA256:") {
		t.Errorf("Fingerprint = %q", info.Fingerprint)
	}
}

func TestParseKeyEncrypted(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "enc@wsh", []byte("s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(block)

	if _, err := ParseKey(pemBytes, ""); err == nil {
		t.Fatal("expected an error for an encrypted key without a passphrase")
	} else if !PassphraseRequired(err) {
		t.Fatalf("PassphraseRequired = false for %v", err)
	}

	info, err := ParseKey(pemBytes, "s3cret")
	if err != nil {
		t.Fatalf("ParseKey with passphrase: %v", err)
	}
	if info.KeyType != "ssh-ed25519" {
		t.Errorf("KeyType = %q, want ssh-ed25519", info.KeyType)
	}

	if _, err := ParseKey(pemBytes, "wrong"); err == nil {
		t.Error("expected an error for a wrong passphrase")
	}
}
