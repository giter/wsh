package ssh

import (
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

// Tunnel direction constants persisted on storage.Tunnel.Direction.
const (
	DirectionLocal  = "local"
	DirectionRemote = "remote"
)

// Tunnel is the common lifecycle interface for port forwards.
type Tunnel interface {
	Start() error
	Stop()
	Addr() net.Addr
	// ConnectionID reports the owning connection profile ID.
	ConnectionID() string
}

// LocalTunnel forwards a local TCP port to a remote address through an SSH
// connection (ssh -L). A single tunnel runs until Stop is called.
type LocalTunnel struct {
	LocalAddress string
	LocalPort    int
	RemoteHost   string
	RemotePort   int

	connectionID string

	client *ssh.Client
	ln     net.Listener
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
}

// Start begins accepting local connections and forwarding them over SSH.
func (lt *LocalTunnel) Start() error {
	addr := net.JoinHostPort(lt.LocalAddress, fmt.Sprintf("%d", lt.LocalPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	lt.ln = ln

	lt.wg.Add(1)
	go lt.acceptLoop()
	return nil
}

func (lt *LocalTunnel) acceptLoop() {
	defer lt.wg.Done()
	for {
		conn, err := lt.ln.Accept()
		if err != nil {
			lt.mu.Lock()
			closed := lt.closed
			lt.mu.Unlock()
			if closed {
				return
			}
			continue
		}
		lt.wg.Add(1)
		go func(c net.Conn) {
			defer lt.wg.Done()
			remoteAddr := net.JoinHostPort(lt.RemoteHost, fmt.Sprintf("%d", lt.RemotePort))
			remote, err := lt.client.Dial("tcp", remoteAddr)
			if err != nil {
				_ = c.Close()
				return
			}
			pipe(c, remote)
		}(conn)
	}
}

// Addr returns the bound local address (useful when the OS chose the port).
func (lt *LocalTunnel) Addr() net.Addr {
	if lt.ln == nil {
		return nil
	}
	return lt.ln.Addr()
}

// ConnectionID returns the owning connection profile ID.
func (lt *LocalTunnel) ConnectionID() string { return lt.connectionID }

// Stop closes the listener and waits for in-flight connections to finish.
func (lt *LocalTunnel) Stop() {
	lt.mu.Lock()
	if lt.closed {
		lt.mu.Unlock()
		return
	}
	lt.closed = true
	lt.mu.Unlock()

	if lt.ln != nil {
		_ = lt.ln.Close()
	}
	lt.wg.Wait()
}

// RemoteTunnel listens on the SSH server side and forwards each accepted
// connection to a local address through the SSH connection (ssh -R).
type RemoteTunnel struct {
	LocalAddress string
	LocalPort    int
	RemoteHost   string
	RemotePort   int

	connectionID string

	client *ssh.Client
	ln     net.Listener
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
}

// Start requests a remote listen on the SSH server and begins forwarding.
func (rt *RemoteTunnel) Start() error {
	addr := net.JoinHostPort(rt.RemoteHost, fmt.Sprintf("%d", rt.RemotePort))
	ln, err := rt.client.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("remote listen %s: %w", addr, err)
	}
	rt.ln = ln

	rt.wg.Add(1)
	go rt.acceptLoop()
	return nil
}

func (rt *RemoteTunnel) acceptLoop() {
	defer rt.wg.Done()
	for {
		conn, err := rt.ln.Accept()
		if err != nil {
			rt.mu.Lock()
			closed := rt.closed
			rt.mu.Unlock()
			if closed {
				return
			}
			continue
		}
		rt.wg.Add(1)
		go func(c net.Conn) {
			defer rt.wg.Done()
			localAddr := net.JoinHostPort(rt.LocalAddress, fmt.Sprintf("%d", rt.LocalPort))
			local, err := net.Dial("tcp", localAddr)
			if err != nil {
				_ = c.Close()
				return
			}
			pipe(c, local)
		}(conn)
	}
}

// Addr returns the bound address of the remote listener.
func (rt *RemoteTunnel) Addr() net.Addr {
	if rt.ln == nil {
		return nil
	}
	return rt.ln.Addr()
}

// ConnectionID returns the owning connection profile ID.
func (rt *RemoteTunnel) ConnectionID() string { return rt.connectionID }

// Stop closes the listener and waits for in-flight connections to finish.
func (rt *RemoteTunnel) Stop() {
	rt.mu.Lock()
	if rt.closed {
		rt.mu.Unlock()
		return
	}
	rt.closed = true
	rt.mu.Unlock()

	if rt.ln != nil {
		_ = rt.ln.Close()
	}
	rt.wg.Wait()
}

// pipe bidirectionally copies between two connections until either side
// closes, then tears down both.
func pipe(a, b net.Conn) {
	defer a.Close()
	defer b.Close()
	var once sync.Once
	closeBoth := func() {
		_ = a.Close()
		_ = b.Close()
	}
	go func() {
		_, _ = io.Copy(b, a)
		once.Do(closeBoth)
	}()
	_, _ = io.Copy(a, b)
	once.Do(closeBoth)
}
