package server

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	sshclient "github.com/lijiajie/wsh/ssh"
	"github.com/lijiajie/wsh/storage"
)

// storeWithConnection writes a one-connection config file and loads it, so the
// login paths can be exercised against a real store.
func storeWithConnection(t *testing.T, c *storage.Connection) *storage.Store {
	t.Helper()
	seed, err := json.Marshal(map[string]interface{}{"connections": []*storage.Connection{c}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func serverWithStore(t *testing.T, store *storage.Store) *Server {
	t.Helper()
	pool := sshclient.NewPool(nil)
	return NewServer(store, pool, sshclient.NewTunnelManager(pool), nil)
}

// closedPort returns a loopback port that is guaranteed to refuse connections,
// so a dial fails fast and deterministically.
func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// TestOpenTerminalPromptsForMissingUser covers connections that were imported
// without a username (Xshell sessions often leave it blank and ask at login):
// opening a session must ask for one instead of dialing into an auth failure.
func TestOpenTerminalPromptsForMissingUser(t *testing.T) {
	store := storeWithConnection(t, &storage.Connection{
		ID: "c1", Name: "无用户名", Host: "127.0.0.1", Port: 22,
	})
	s := serverWithStore(t, store)

	res, err := s.handleOpenTerminal(nil, json.RawMessage(`{"connId":"c1"}`))
	if err != nil {
		t.Fatalf("handleOpenTerminal: %v", err)
	}
	prompt, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("expected a prompt map, got %#v", res)
	}
	if prompt["needUser"] != true {
		t.Fatalf("expected needUser, got %v", prompt)
	}
	if msg, _ := prompt["message"].(string); msg == "" {
		t.Errorf("prompt should carry a message, got %v", prompt)
	}
}

// TestOpenTerminalUserOverrideIsNotPersisted checks that a username typed at the
// login prompt gets past the user prompt without rewriting the stored record, so
// a one-off login cannot silently change the user's configuration.
func TestOpenTerminalUserOverrideIsNotPersisted(t *testing.T) {
	store := storeWithConnection(t, &storage.Connection{
		ID: "c1", Name: "无用户名", Host: "127.0.0.1", Port: closedPort(t),
	})
	s := serverWithStore(t, store)

	// No secret is stored, so the flow stops at the password prompt; what matters
	// here is that the user prompt was skipped and the stored record was left
	// alone, so a one-off login cannot silently change the configuration.
	res, err := s.handleOpenTerminal(nil, json.RawMessage(`{"connId":"c1","user":"root"}`))
	if err != nil {
		t.Fatalf("handleOpenTerminal: %v", err)
	}
	if prompt, ok := res.(map[string]interface{}); ok && prompt["needUser"] == true {
		t.Fatalf("a typed username should not prompt again, got %v", prompt)
	}
	if got := store.Connections()[0].User; got != "" {
		t.Fatalf("stored username should be untouched, got %q", got)
	}
}

// TestOpenTerminalUsesStoredUser checks that a connection with a username goes
// straight to the credential stage instead of asking for a username again.
func TestOpenTerminalUsesStoredUser(t *testing.T) {
	store := storeWithConnection(t, &storage.Connection{
		ID: "c1", Name: "有用户名", Host: "127.0.0.1", Port: closedPort(t), User: "root",
	})
	s := serverWithStore(t, store)

	res, err := s.handleOpenTerminal(nil, json.RawMessage(`{"connId":"c1"}`))
	if err != nil {
		t.Fatalf("handleOpenTerminal: %v", err)
	}
	prompt, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("expected a credential prompt, got %#v", res)
	}
	if prompt["needUser"] == true {
		t.Fatalf("a stored username should not be asked for again, got %v", prompt)
	}
	// With no stored secret the backend asks for a password, which is the
	// pre-existing behaviour for this path.
	if prompt["needPassword"] != true {
		t.Fatalf("expected needPassword, got %v", prompt)
	}
}
