package sshclient

import (
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

// LocalTunnel forwards a local TCP port to a remote address through an SSH
// connection. A single tunnel runs until Stop is called.
type LocalTunnel struct {
	LocalAddress string
	LocalPort    int
	RemoteHost   string
	RemotePort   int

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
			lt.handle(c)
		}(conn)
	}
}

func (lt *LocalTunnel) handle(local net.Conn) {
	defer local.Close()

	remoteAddr := net.JoinHostPort(lt.RemoteHost, fmt.Sprintf("%d", lt.RemotePort))
	remote, err := lt.client.Dial("tcp", remoteAddr)
	if err != nil {
		return
	}
	defer remote.Close()

	var once sync.Once
	closeBoth := func() {
		_ = local.Close()
		_ = remote.Close()
	}
	go func() {
		_, _ = io.Copy(remote, local)
		once.Do(closeBoth)
	}()
	go func() {
		_, _ = io.Copy(local, remote)
		once.Do(closeBoth)
	}()
}

// Addr returns the bound local address (useful when the OS chose the port).
func (lt *LocalTunnel) Addr() net.Addr {
	if lt.ln == nil {
		return nil
	}
	return lt.ln.Addr()
}

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
