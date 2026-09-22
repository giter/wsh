package ssh

import (
	"sync"

	"github.com/lijiajie/wsh/storage"
)

// TunnelManager owns the lifecycle of active tunnels keyed by tunnel ID.
type TunnelManager struct {
	pool *Pool

	mu      sync.Mutex
	running map[string]Tunnel
}

// NewTunnelManager creates a manager backed by a connection pool.
func NewTunnelManager(pool *Pool) *TunnelManager {
	return &TunnelManager{
		pool:    pool,
		running: make(map[string]Tunnel),
	}
}

// Start launches a tunnel for the profile, reusing the pooled connection.
// The tunnel direction (local or remote) is taken from t.Direction; an empty
// direction falls back to a local forward for backwards compatibility.
func (tm *TunnelManager) Start(c *storage.Connection, t *storage.Tunnel) (Tunnel, error) {
	return tm.StartWithPassword(c, t, nil)
}

// StartWithPassword is Start with an optional password typed at a connect prompt
// (used when the connection stores no password). A nil password falls back to
// the saved/remembered credentials.
func (tm *TunnelManager) StartWithPassword(c *storage.Connection, t *storage.Tunnel, password *string) (Tunnel, error) {
	client, err := tm.pool.Get(c, password)
	if err != nil {
		return nil, err
	}

	var tun Tunnel
	switch t.Direction {
	case DirectionRemote:
		tun = &RemoteTunnel{
			LocalAddress: t.LocalAddress,
			LocalPort:    t.LocalPort,
			RemoteHost:   t.RemoteAddress,
			RemotePort:   t.RemotePort,
			connectionID: c.ID,
			client:       client,
		}
	default:
		tun = &LocalTunnel{
			LocalAddress: t.LocalAddress,
			LocalPort:    t.LocalPort,
			RemoteHost:   t.RemoteAddress,
			RemotePort:   t.RemotePort,
			connectionID: c.ID,
			client:       client,
		}
	}
	if err := tun.Start(); err != nil {
		return nil, err
	}

	tm.mu.Lock()
	tm.running[t.ID] = tun
	tm.mu.Unlock()
	return tun, nil
}

// Stop ends a running tunnel.
func (tm *TunnelManager) Stop(id string) {
	tm.mu.Lock()
	tun, ok := tm.running[id]
	if ok {
		delete(tm.running, id)
	}
	tm.mu.Unlock()
	if ok {
		tun.Stop()
	}
}

// IsRunning reports whether a tunnel is currently active.
func (tm *TunnelManager) IsRunning(id string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	_, ok := tm.running[id]
	return ok
}

// StopByConnection stops every tunnel that rides on the given connection.
func (tm *TunnelManager) StopByConnection(connID string) {
	tm.mu.Lock()
	var stopped []Tunnel
	for id, tun := range tm.running {
		if tun.ConnectionID() == connID {
			delete(tm.running, id)
			stopped = append(stopped, tun)
		}
	}
	tm.mu.Unlock()
	for _, tun := range stopped {
		tun.Stop()
	}
}

// StopAll closes every active tunnel.
func (tm *TunnelManager) StopAll() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for id, tun := range tm.running {
		tun.Stop()
		delete(tm.running, id)
	}
}
