package sshclient

import (
	"fmt"
	"sync"

	"sshclient/storage"
)

// TunnelManager owns the lifecycle of active local tunnels keyed by tunnel ID.
type TunnelManager struct {
	pool *Pool

	mu      sync.Mutex
	running map[string]*LocalTunnel
}

// NewTunnelManager creates a manager backed by a connection pool.
func NewTunnelManager(pool *Pool) *TunnelManager {
	return &TunnelManager{
		pool:    pool,
		running: make(map[string]*LocalTunnel),
	}
}

// Start launches a tunnel for the profile, reusing the pooled connection.
func (tm *TunnelManager) Start(c *storage.Connection, t *storage.Tunnel) (*LocalTunnel, error) {
	client, err := tm.pool.Get(c, nil)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	lt := &LocalTunnel{
		LocalAddress: t.LocalAddress,
		LocalPort:    t.LocalPort,
		RemoteHost:   t.RemoteAddress,
		RemotePort:   t.RemotePort,
		client:       client,
	}
	if err := lt.Start(); err != nil {
		return nil, err
	}

	tm.mu.Lock()
	tm.running[t.ID] = lt
	tm.mu.Unlock()
	return lt, nil
}

// Stop ends a running tunnel.
func (tm *TunnelManager) Stop(id string) {
	tm.mu.Lock()
	lt, ok := tm.running[id]
	if ok {
		delete(tm.running, id)
	}
	tm.mu.Unlock()
	if ok {
		lt.Stop()
	}
}

// IsRunning reports whether a tunnel is currently active.
func (tm *TunnelManager) IsRunning(id string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	_, ok := tm.running[id]
	return ok
}

// StopAll closes every active tunnel.
func (tm *TunnelManager) StopAll() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for id, lt := range tm.running {
		lt.Stop()
		delete(tm.running, id)
	}
}
