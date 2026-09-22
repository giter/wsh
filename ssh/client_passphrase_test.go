package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/giter/wsh/storage"
)

// startNoAuthServer listens on a loopback port and accepts any client; it
// covers the dial paths used by quick connect without a real host.
func startNoAuthServer(t *testing.T) (host string, port int) {
	t.Helper()
	config := &ssh.ServerConfig{NoClientAuth: true}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	config.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				sconn, chans, reqs, err := ssh.NewServerConn(c, config)
				if err != nil {
					_ = c.Close()
					return
				}
				go ssh.DiscardRequests(reqs)
				go func() {
					for nc := range chans {
						_ = nc.Reject(ssh.UnknownChannelType, "not needed")
					}
				}()
				defer sconn.Close()
			}(c)
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

// encryptedKeyPEM returns a passphrase-protected OpenSSH private key and its
// passphrase.
func encryptedKeyPEM(t *testing.T) (pemStr, passphrase string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const pass = "test-passphrase"
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "test", []byte(pass))
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block)), pass
}

func TestDialAdhocPassphrasePrompt(t *testing.T) {
	host, port := startNoAuthServer(t)
	pemData, pass := encryptedKeyPEM(t)
	keys := []KeyMaterial{{ID: "k1", PEM: pemData}}

	// An encrypted key with no passphrase and no password must surface a
	// PassphraseError carrying the key ID, so the UI can prompt.
	_, err := DialAdhoc(host, port, "u", "", "", keys)
	var pe *PassphraseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected PassphraseError, got: %v", err)
	}
	if pe.KeyID != "k1" {
		t.Fatalf("expected key ID k1, got %q", pe.KeyID)
	}

	// With a password available the dial falls back to password auth instead
	// of demanding the passphrase.
	client, err := DialAdhoc(host, port, "u", "pw", "", keys)
	if err != nil {
		t.Fatalf("DialAdhoc with password fallback: %v", err)
	}
	_ = client.Close()

	// The typed passphrase unlocks the key.
	client, err = DialAdhoc(host, port, "u", "", pass, keys)
	if err != nil {
		t.Fatalf("DialAdhoc with typed passphrase: %v", err)
	}
	_ = client.Close()

	// A wrong passphrase does not authenticate; without a password the dial
	// must report the passphrase problem again so the user can retry.
	_, err = DialAdhoc(host, port, "u", "", "wrong", keys)
	if !errors.As(err, &pe) {
		t.Fatalf("expected PassphraseError for wrong passphrase, got: %v", err)
	}
}

func TestParseSignerPassphraseDetection(t *testing.T) {
	pemData, pass := encryptedKeyPEM(t)

	if _, err := parseSigner([]byte(pemData), pass); err != nil {
		t.Fatalf("correct passphrase should parse: %v", err)
	}
	_, err := parseSigner([]byte(pemData), "nope")
	if !PassphraseProblem(err) {
		t.Fatalf("wrong passphrase should be a passphrase problem: %v", err)
	}
	if PassphraseRequired(err) {
		t.Fatalf("wrong passphrase should not look like a missing one: %v", err)
	}
	_, err = parseSigner([]byte(pemData), "")
	if !PassphraseRequired(err) {
		t.Fatalf("missing passphrase should be reported: %v", err)
	}
}

func TestDialManagedKeyPassphrase(t *testing.T) {
	host, port := startNoAuthServer(t)
	pemData, pass := encryptedKeyPEM(t)

	c := &storage.Connection{Name: "t", Host: host, Port: port, User: "u", KeyID: "k1"}
	// The fake resolver stands in for the storage layer.
	resolver := func(stored string) KeyResolver {
		return func(keyID string) (string, string, error) {
			if keyID != "k1" {
				return "", "", errors.New("key not found")
			}
			return pemData, stored, nil
		}
	}

	// No passphrase anywhere: dial reports that the key needs one.
	_, err := Dial(c, "", resolver(""))
	var pe *PassphraseError
	if !errors.As(err, &pe) || pe.KeyID != "k1" {
		t.Fatalf("expected PassphraseError for k1, got: %v", err)
	}

	// With the passphrase resolvable, the dial succeeds.
	client, err := Dial(c, "", resolver(pass))
	if err != nil {
		t.Fatalf("Dial with passphrase: %v", err)
	}
	_ = client.Close()
}

func TestPoolResolveKeyPrefersPrompted(t *testing.T) {
	pemData, pass := encryptedKeyPEM(t)

	var storedAsked string
	keys := func(keyID string) (string, string, error) {
		storedAsked = keyID
		return pemData, "stored-pass", nil
	}
	p := NewPool(keys)

	// Without a prompted passphrase the stored one is used.
	pem, pass2, err := p.resolveKey("k1")
	if err != nil || pem != pemData || pass2 != "stored-pass" {
		t.Fatalf("stored passphrase not returned: %q %q %v", pem, pass2, err)
	}

	// A prompted passphrase wins over the stored one.
	p.ProvidePassphrase("k1", pass)
	_, pass2, err = p.resolveKey("k1")
	if err != nil || pass2 != pass {
		t.Fatalf("prompted passphrase should win, got %q %v", pass2, err)
	}
	if storedAsked != "k1" {
		t.Fatalf("resolver should still be asked for the PEM, got %q", storedAsked)
	}
}
