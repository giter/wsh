package sshclient

import (
	"fmt"

	"golang.org/x/crypto/ssh"

	"github.com/fyne-io/terminal"
)

// TerminalSession wires a fyne terminal widget to a live SSH shell. It owns
// the ssh.Session and keeps the remote PTY in step with the widget's size.
type TerminalSession struct {
	client   *ssh.Client
	session  *ssh.Session
	term     *terminal.Terminal
	shutdown chan struct{}
	done     chan struct{}

	OnExit func() // called when the remote shell ends
}

// NewTerminalSession prepares a session bound to the given terminal widget.
// Call Start to actually connect.
func NewTerminalSession(c *ssh.Client, term *terminal.Terminal) *TerminalSession {
	return &TerminalSession{
		client:   c,
		term:     term,
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start dials the remote shell and hands control of the terminal widget to it.
func (ts *TerminalSession) Start() error {
	session, err := ts.client.NewSession()
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	ts.session = session

	// Allocate the PTY at a sensible default; the resize listener will correct
	// the size the moment the widget reports its real dimensions.
	rows, cols := 40, 120
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = session.Close()
		return fmt.Errorf("request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	shell := "$SHELL || bash"
	if err := session.Start(shell); err != nil {
		_ = session.Close()
		return fmt.Errorf("start shell: %w", err)
	}

	// Keep remote size aligned with the widget.
	go ts.watchResize(session)

	// Hand the widget to the connection. RunWithConnection blocks until the
	// remote connection is closed, so it runs on its own goroutine. The
	// terminal reads remote output from `stdout` and writes keypresses to
	// `stdin`; it reports exit() once the connection drops.
	go func() {
		_ = ts.term.RunWithConnection(stdin, stdout)
		ts.exit()
	}()

	return nil
}

func (ts *TerminalSession) exit() {
	select {
	case <-ts.done:
		return
	default:
	}
	close(ts.done)
	if ts.OnExit != nil {
		ts.OnExit()
	}
}

// Close tears down the session and the underlying connection.
func (ts *TerminalSession) Close() {
	close(ts.shutdown)
	if ts.session != nil {
		_ = ts.session.Close()
	}
	ts.exit()
	_ = ts.client.Close()
}

// watchResize forwards terminal size changes to the remote PTY.
func (ts *TerminalSession) watchResize(session *ssh.Session) {
	ch := make(chan terminal.Config, 8)
	ts.term.AddListener(ch)

	rows, cols := uint(0), uint(0)
	for {
		select {
		case <-ts.shutdown:
			ts.term.RemoveListener(ch)
			return
		case config := <-ch:
			if rows == config.Rows && cols == config.Columns {
				continue
			}
			rows, cols = config.Rows, config.Columns
			if rows > 0 && cols > 0 {
				_ = session.WindowChange(int(rows), int(cols))
			}
		}
	}
}
