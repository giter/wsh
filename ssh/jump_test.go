package ssh

import (
	"strings"
	"testing"

	"github.com/giter/wsh/storage"
)

// lookup builds a resolver over a fixed set of connections.
func lookup(conns ...*storage.Connection) ConnectionLookup {
	byID := make(map[string]*storage.Connection, len(conns))
	for _, c := range conns {
		byID[c.ID] = c
	}
	return func(id string) *storage.Connection { return byID[id] }
}

func TestJumpChainOrdersBastions(t *testing.T) {
	a := &storage.Connection{ID: "a", Name: "bastion-a"}
	b := &storage.Connection{ID: "b", Name: "bastion-b"}
	target := &storage.Connection{ID: "t", Name: "target", JumpHostIDs: []string{"a", "b"}}

	chain, err := JumpChain(target, lookup(a, b, target))
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 2 || chain[0].ID != "a" || chain[1].ID != "b" {
		t.Fatalf("chain order wrong: %+v", chain)
	}
}

func TestJumpChainDirectConnection(t *testing.T) {
	target := &storage.Connection{ID: "t", Name: "target"}
	chain, err := JumpChain(target, lookup(target))
	if err != nil {
		t.Fatal(err)
	}
	if chain != nil {
		t.Fatalf("a connection without jump hosts must dial directly, got %+v", chain)
	}
}

func TestJumpChainRejectsBadConfig(t *testing.T) {
	a := &storage.Connection{ID: "a", Name: "bastion-a"}

	cases := []struct {
		name   string
		target *storage.Connection
		substr string
	}{
		{
			name:   "self reference",
			target: &storage.Connection{ID: "t", JumpHostIDs: []string{"t"}},
			substr: "环",
		},
		{
			name:   "repeat",
			target: &storage.Connection{ID: "t", JumpHostIDs: []string{"a", "a"}},
			substr: "环",
		},
		{
			name:   "unknown id",
			target: &storage.Connection{ID: "t", JumpHostIDs: []string{"missing"}},
			substr: "不存在",
		},
		{
			name:   "too many hops",
			target: &storage.Connection{ID: "t", JumpHostIDs: []string{"1", "2", "3", "4", "5", "6"}},
			substr: "层数过多",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := JumpChain(tt.target, lookup(a))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.substr) {
				t.Fatalf("error %q should mention %q", err.Error(), tt.substr)
			}
		})
	}
}

// TestClientConfigPrefersKeyOverPassword keeps the documented behaviour: a
// configured key is tried before the password.
func TestClientConfigPrefersKeyOverPassword(t *testing.T) {
	conn := &storage.Connection{ID: "c", Name: "n", User: "root", Host: "h"}
	cfg, err := clientConfig(conn, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Auth) != 1 {
		t.Fatalf("expected password-only auth, got %d methods", len(cfg.Auth))
	}
	// No credentials at all is an error, not a silent dial.
	if _, err := clientConfig(&storage.Connection{ID: "x", Name: "n"}, "", nil); err == nil {
		t.Fatal("expected an error when no authentication method is available")
	}
}

func TestHostPortDefault(t *testing.T) {
	if got := hostPort(&storage.Connection{Host: "example.com"}); got != "example.com:22" {
		t.Fatalf("hostPort = %q", got)
	}
	if got := hostPort(&storage.Connection{Host: "example.com", Port: 2222}); got != "example.com:2222" {
		t.Fatalf("hostPort = %q", got)
	}
}
