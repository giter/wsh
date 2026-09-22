package server

import (
	"testing"

	"github.com/giter/wsh/storage"
)

// TestCleanJumpHosts covers the validation applied to a submitted bastion chain.
func TestCleanJumpHosts(t *testing.T) {
	srv := newTestServer(nil)
	a := &storage.Connection{ID: "a", Name: "bastion-a", Host: "a", Port: 22, User: "u"}
	b := &storage.Connection{ID: "b", Name: "bastion-b", Host: "b", Port: 22, User: "u"}
	if err := srv.store.AddConnection(a); err != nil {
		t.Fatal(err)
	}
	if err := srv.store.AddConnection(b); err != nil {
		t.Fatal(err)
	}

	got := srv.cleanJumpHosts("self", []string{"a", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("cleanJumpHosts = %v", got)
	}

	// Unknown IDs, empties, repeats and self-references are dropped.
	got = srv.cleanJumpHosts("a", []string{"a", "", "missing", "b", "b"})
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("cleanJumpHosts filtered incorrectly: %v", got)
	}

	if got := srv.cleanJumpHosts("x", nil); got != nil {
		t.Fatalf("empty input should stay empty, got %v", got)
	}
}
