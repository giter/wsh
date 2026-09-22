package ssh

import (
	"fmt"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/giter/wsh/storage"
)

// Pool caches live SSH clients keyed by connection ID so the terminal,
// transfer, and tunnel views share one connection per server.
type Pool struct {
	mu      sync.Mutex
	clients map[string]*ssh.Client
	dials   map[string]*sync.Once
	pass    map[string]string // plaintext passwords provided for this run
	// passphrases holds key passphrases typed at a connect prompt. They live
	// in memory for this run only and win over any stored (opt-in) copy so a
	// freshly entered passphrase can replace a stale saved one.
	passphrases map[string]string
	keys        KeyResolver // resolves managed keys for key auth
	// jumps resolves a connection ID to a saved profile, used to build a
	// bastion chain. Nil disables jump hosts.
	jumps ConnectionLookup
}

// NewPool returns an empty pool. keys resolves managed key IDs to decrypted
// PEM material; it may be nil when no key manager is available.
func NewPool(keys KeyResolver) *Pool {
	return &Pool{
		clients:     make(map[string]*ssh.Client),
		dials:       make(map[string]*sync.Once),
		pass:        make(map[string]string),
		passphrases: make(map[string]string),
		keys:        keys,
	}
}

// SetJumpResolver installs the lookup used to resolve jump-host IDs. Passing
// nil (the default) makes every connection dial directly.
func (p *Pool) SetJumpResolver(lookup ConnectionLookup) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.jumps = lookup
}

// ProvidePassword records a freshly entered password for a connection ID so
// future Dial calls in this run can use it without prompting again.
func (p *Pool) ProvidePassword(id, password string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pass[id] = password
}

// ProvidePassphrase records a passphrase typed at a connect prompt for a
// managed key ID. It is kept in memory for this run only.
func (p *Pool) ProvidePassphrase(keyID, passphrase string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.passphrases[keyID] = passphrase
}

// resolveKey resolves managed key material, preferring a passphrase typed this
// run (see ProvidePassphrase) over the stored opt-in copy.
func (p *Pool) resolveKey(keyID string) (string, string, error) {
	if p.keys == nil {
		return "", "", fmt.Errorf("no key resolver")
	}
	p.mu.Lock()
	pass := p.passphrases[keyID]
	p.mu.Unlock()
	if pass != "" {
		pem, _, err := p.keys(keyID)
		return pem, pass, err
	}
	return p.keys(keyID)
}

// Get returns a live client for the profile, dialing on first use. A non-nil
// password wins over stored/saved ones; pass nil to use saved credentials.
//
// A password that was explicitly provided and worked is remembered for the rest
// of the run, so the other features on the same connection (file transfer,
// tunnels) do not have to ask for it again.
func (p *Pool) Get(c *storage.Connection, password *string) (*ssh.Client, error) {
	p.mu.Lock()
	if client, ok := p.clients[c.ID]; ok {
		p.mu.Unlock()
		return client, nil
	}
	once, ok := p.dials[c.ID]
	if !ok {
		once = &sync.Once{}
		p.dials[c.ID] = once
	}
	p.mu.Unlock()

	var (
		client *ssh.Client
		err    error
	)
	once.Do(func() {
		pw := ""
		if password != nil {
			pw = *password
		} else if saved, ok := p.pass[c.ID]; ok {
			pw = saved
		}
		p.mu.Lock()
		jumps := p.jumps
		p.mu.Unlock()

		// A jump-host chain is validated before dialing so a cycle or a
		// missing bastion surfaces as a clear error rather than a timeout.
		var chain []*storage.Connection
		if len(c.JumpHostIDs) > 0 {
			var cerr error
			chain, cerr = JumpChain(c, jumps)
			if cerr != nil {
				err = cerr
				p.mu.Lock()
				delete(p.dials, c.ID)
				p.mu.Unlock()
				return
			}
		}

		if len(chain) > 0 {
			client, err = DialChain(chain, c, pw, p.resolveKey)
		} else {
			client, err = Dial(c, pw, p.resolveKey)
		}
		if err != nil {
			// Reset the gate so a later attempt (e.g. with the right password)
			// can try again instead of being stuck on the failure.
			p.mu.Lock()
			delete(p.dials, c.ID)
			p.mu.Unlock()
			return
		}
		p.mu.Lock()
		p.clients[c.ID] = client
		if password != nil && pw != "" {
			p.pass[c.ID] = pw
		}
		p.mu.Unlock()
	})
	return client, err
}

// Active reports whether a live client exists for the connection ID.
func (p *Pool) Active(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.clients[id]
	return ok
}

// Drop closes and forgets a connection, letting it be re-dialed later.
func (p *Pool) Drop(id string) {
	p.mu.Lock()
	client, ok := p.clients[id]
	if ok {
		delete(p.clients, id)
	}
	delete(p.dials, id)
	p.mu.Unlock()
	if ok && client != nil {
		_ = client.Close()
	}
}

// CloseAll closes every live connection.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, client := range p.clients {
		_ = client.Close()
		delete(p.clients, id)
		delete(p.dials, id)
	}
}
