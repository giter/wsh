package server

import (
	"errors"
	"testing"

	sshclient "sshclient/ssh"
)

// TestCredentialPrompt covers the mapping from a dial failure to the credential
// prompt the UI shows. This is what turns "文件传输打开时报错" for a connection
// with no saved password into a password prompt; the terminal, file transfer and
// tunnel paths all share it.
func TestCredentialPrompt(t *testing.T) {
	s := newTestServer(nil)

	tests := []struct {
		name string
		err  error
		kind string // "", "needPassword" or "needPassphrase"
	}{
		{"nil error", nil, ""},
		{"unrelated dial error", errors.New("dial tcp 10.0.0.1:22: connect: connection refused"), ""},
		{"no authentication method", errors.New("no authentication method configured for web-01"), "needPassword"},
		{
			"rejected password",
			errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password]"),
			"needPassword",
		},
		{"encrypted managed key", &sshclient.PassphraseError{KeyID: "k1"}, "needPassphrase"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt, ok := s.credentialPrompt(tc.err)
			if tc.kind == "" {
				if ok {
					t.Fatalf("expected no prompt, got %v", prompt)
				}
				return
			}
			if !ok {
				t.Fatalf("expected a %s prompt, got none", tc.kind)
			}
			if prompt[tc.kind] != true {
				t.Fatalf("expected a %s prompt, got %v", tc.kind, prompt)
			}
			if msg, _ := prompt["message"].(string); msg == "" {
				t.Errorf("prompt should carry a message, got %v", prompt)
			}
		})
	}
}

// TestCredentialPromptPassphraseKeyID checks that the passphrase prompt carries
// the key ID, which is what lets the UI name the key it belongs to.
func TestCredentialPromptPassphraseKeyID(t *testing.T) {
	s := newTestServer(nil)

	// The key is not in the store, so the message falls back to the generic
	// wording, but the ID must still be reported.
	prompt, ok := s.credentialPrompt(&sshclient.PassphraseError{KeyID: "k1"})
	if !ok {
		t.Fatal("expected a passphrase prompt")
	}
	if prompt["keyId"] != "k1" {
		t.Errorf("prompt should carry the key ID, got %v", prompt["keyId"])
	}
}
