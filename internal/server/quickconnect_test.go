package server

import (
	"net"
	"strconv"
	"strings"
	"testing"

	sshclient "sshclient/ssh"
)

// TestDialAdhoc covers the dialing used by the quick-connect bar: an unsaved
// host is dialed directly (no pool entry), and when no credential is available
// the error is the one that makes the UI fall back to a password prompt.
func TestDialAdhoc(t *testing.T) {
	addr, _ := startFakeSSH(t)
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	// The fake server accepts any credential.
	client, err := sshclient.DialAdhoc(host, port, "u", "p", nil)
	if err != nil {
		t.Fatalf("DialAdhoc with password: %v", err)
	}
	_ = client.Close()

	// No password and no keys: refuse locally, with the message the frontend
	// keys off to ask for a password.
	if _, err := sshclient.DialAdhoc(host, port, "u", "", nil); err == nil {
		t.Fatal("expected an error when no authentication method is available")
	} else if !strings.Contains(err.Error(), "no authentication method") {
		t.Fatalf("unexpected error: %v", err)
	}
}
