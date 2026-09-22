package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

	sshclient "github.com/giter/wsh/ssh"
	"github.com/giter/wsh/storage"
)

// WebSession wires a browser terminal tab to a live SSH shell. Unlike the
// Fyne variant it is transport-agnostic: output is pushed through a callback
// and input arrives via HandleInput, so it can be driven by WebSocket.
type WebSession struct {
	id     string
	client *ssh.Client
	sess   *ssh.Session
	stdin  io.WriteCloser

	// ownedClient marks an ad-hoc (quick connect) client that this session owns
	// and must close, rather than a pooled client shared with other sessions.
	ownedClient bool
	clientOnce  sync.Once

	mu     sync.Mutex
	closed bool
	done   chan struct{}

	// pushOutput delivers terminal output to the browser. It must be safe to
	// call from the session reader goroutine.
	pushOutput func(data []byte)
	// pushMsg delivers arbitrary push messages (e.g. zmodem events).
	pushMsg func(v interface{})
	onExit  func()
	// onClose runs at most once when the session ends, whichever path got there
	// first (explicit close or remote EOF). It releases the host probe.
	onClose   func()
	closeOnce sync.Once

	// connID is the saved connection this session belongs to (empty for ad-hoc
	// quick-connect sessions).
	connID string

	// sniff watches the rendered output for alternate-screen switches and error
	// features, so the UI can react without parsing the terminal itself.
	sniff *sniffer

	// zmMu guards the active zmodem transfer (if any) and the upload-prompt
	// dedup flag.
	zmMu             sync.Mutex
	zm               *zmSession
	zmUploadPrompted bool

	// Chunked upload accumulation (zmodem.sendBegin / sendChunk / sendEnd).
	// Guarded by zmMu.
	uploadName string
	uploadSize int
	uploadData []byte
}

type openTerminalParams struct {
	ConnectionID  string `json:"connId"`
	Cols          int    `json:"cols"`
	Rows          int    `json:"rows"`
	Password      string `json:"password"`      // 用户本次输入的密码（可选）
	KeyPassphrase string `json:"keyPassphrase"` // 用户本次输入的私钥口令（可选）

	// User is the username to log in with. For an ad-hoc target it is required;
	// for a saved connection it is an optional override carrying the username
	// typed at the login prompt, used for this run only.
	User string `json:"user"`

	// Ad-hoc connection (quick connect): used when ConnectionID is empty, so a
	// one-off session can be opened without saving a connection first.
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (s *Server) handleOpenTerminal(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p openTerminalParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	var (
		conn   *storage.Connection
		client *ssh.Client
		err    error
		adhoc  bool
	)

	switch {
	case p.ConnectionID != "":
		conn, err = s.findConnection(p.ConnectionID)
		if err != nil {
			return nil, err
		}
		// A username typed at the login prompt wins for this run. The override is
		// applied to a copy, so a one-off login never rewrites stored state.
		if p.User != "" && p.User != conn.User {
			override := *conn
			override.User = p.User
			conn = &override
		}
		// A connection saved without a username (Xshell sessions often leave it
		// blank and ask at login) cannot authenticate, so ask for one instead of
		// dialing into an opaque handshake failure.
		if conn.User == "" {
			return map[string]interface{}{
				"needUser": true,
				"message":  fmt.Sprintf("连接「%s」未设置用户名，请输入登录用户名", conn.Name),
			}, nil
		}
		var passPtr *string
		if p.Password != "" {
			passPtr = &p.Password
		}
		// A passphrase typed at the prompt wins over any stored copy for this
		// run (it also replaces a stale saved one that keeps failing).
		if p.KeyPassphrase != "" && conn.KeyID != "" {
			s.pool.ProvidePassphrase(conn.KeyID, p.KeyPassphrase)
		}
		client, err = s.pool.Get(conn, passPtr)

	case p.Host != "":
		// Quick connect: dial a temporary host and own the client, so it is
		// closed with the session instead of being kept in the pool.
		if p.User == "" {
			return nil, fmt.Errorf("请填写用户名")
		}
		if p.Port == 0 {
			p.Port = 22
		}
		adhoc = true
		conn = &storage.Connection{
			ID:   "adhoc-" + storage.NewID(),
			Name: fmt.Sprintf("%s@%s", p.User, p.Host),
			Host: p.Host,
			Port: p.Port,
			User: p.User,
		}
		client, err = sshclient.DialAdhoc(p.Host, p.Port, p.User, p.Password, p.KeyPassphrase, s.allKeyMaterials(p.KeyPassphrase))

	default:
		return nil, fmt.Errorf("缺少连接信息")
	}

	if err != nil {
		if prompt, ok := s.credentialPrompt(err); ok {
			return prompt, nil
		}
		return nil, err
	}

	if p.Cols <= 0 {
		p.Cols = 80
	}
	if p.Rows <= 0 {
		p.Rows = 24
	}

	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", p.Rows, p.Cols, modes); err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("request pty: %w", err)
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	sid := storage.NewID()
	ws := &WebSession{
		id:          sid,
		sniff:       newSniffer(sid),
		client:      client,
		sess:        sess,
		stdin:       stdin,
		done:        make(chan struct{}),
		ownedClient: adhoc,
		connID:      conn.ID,
	}

	// Bind the session to this browser connection for output and lifecycle.
	ws.pushOutput = func(data []byte) {
		c.send(terminalDataMsg{Type: "terminal.data", SessionID: ws.id, Data: string(data)})
		// Tap the rendered stream for alternate-screen and error detection. The
		// bytes themselves are already on their way to xterm.js untouched.
		ws.sniff.feed(data, c.send)
	}
	ws.pushMsg = func(v interface{}) {
		c.send(v)
	}
	ws.onExit = func() {
		c.send(terminalDataMsg{Type: "terminal.exit", SessionID: ws.id})
		s.unregister(ws.id)
	}
	// The host probe runs only while at least one session is open.
	ws.onClose = func() {
		if !adhoc {
			s.probes.release(conn.ID)
		}
	}

	if err := sess.Start("$SHELL || bash"); err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	s.register(ws)
	if !adhoc {
		s.probes.acquire(conn.ID)
	}
	go ws.readLoop(stdout)
	return map[string]interface{}{"sessionId": ws.id}, nil
}

// readLoop copies remote output to the browser until the session ends.
func (ws *WebSession) readLoop(stdout io.Reader) {
	buf := make([]byte, 32*1024)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			// Copy so the push callback is safe while we reuse the buffer.
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			ws.routeOutput(chunk)
		}
		if err != nil {
			break
		}
	}
	ws.close()
	ws.onExit()
}

// routeOutput sends stdout bytes to the browser, switching to ZMODEM mode as
// soon as a transfer frame header appears in the stream. The remote rz banner
// triggers an upload prompt (rz waits for our ZRQINIT instead of sending a
// frame itself), while a ZMODEM frame header (remote sz) starts a download.
func (ws *WebSession) routeOutput(chunk []byte) {
	// Upload: remote rz printed its banner and is waiting for our ZRQINIT.
	// Check before frame detection: lrzsz's rz prints the banner and its
	// ZRINIT frame in one burst, which the frame branch below would swallow
	// entirely. The prompt is idempotent (an extra push only rebuilds the
	// banner UI in the browser).
	ws.zmMu.Lock()
	prompted := ws.zmUploadPrompted
	ws.zmMu.Unlock()
	if !prompted && bytes.Contains(chunk, []byte("rz waiting to receive")) {
		ws.zmMu.Lock()
		ws.zmUploadPrompted = true
		ws.zmMu.Unlock()
		ws.pushMsg(zmodemMsg{Type: "zmodem.send-file", SessionID: ws.id})
	}

	ws.zmMu.Lock()
	zm := ws.zm
	ws.zmMu.Unlock()
	if zm != nil && zm.active() {
		zm.feed(chunk)
		return
	}
	if idx := findZmodemStart(chunk); idx >= 0 {
		z := startZmSession(ws)
		ws.zmMu.Lock()
		ws.zm = z
		ws.zmMu.Unlock()
		// Direction is decided by onHeader once the first frame is parsed:
		// ZRQINIT (remote sz) -> download-start, ZRINIT (remote rz) -> the
		// upload banner. We must NOT dismiss the banner here, because rz also
		// sends a ZRINIT frame right after its banner.
		if idx > 0 {
			ws.pushOutput(chunk[:idx])
		}
		z.feed(chunk[idx:])
		return
	}
	ws.pushOutput(chunk)
}

// zmodemDone clears the active transfer once its session goroutine exits.
func (ws *WebSession) zmodemDone(z *zmSession) {
	ws.zmMu.Lock()
	if ws.zm == z {
		ws.zm = nil
	}
	ws.zmUploadPrompted = false
	ws.zmMu.Unlock()
}

// HandleInput forwards keyboard input from the browser to the remote shell.
func (ws *WebSession) HandleInput(data []byte) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.closed {
		return
	}
	_, _ = ws.stdin.Write(data)
}

// Resize forwards a terminal size change to the remote PTY.
func (ws *WebSession) Resize(cols, rows int) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.closed {
		return
	}
	_ = ws.sess.WindowChange(rows, cols)
}

// Close terminates the session.
func (ws *WebSession) Close() {
	ws.mu.Lock()
	if ws.closed {
		ws.mu.Unlock()
		return
	}
	ws.closed = true
	ws.mu.Unlock()

	select {
	case <-ws.done:
	default:
		close(ws.done)
	}
	if ws.sess != nil {
		_ = ws.sess.Close()
	}
	ws.closeOwnedClient()
	ws.runCloseHooks()
}

func (ws *WebSession) close() {
	ws.mu.Lock()
	if ws.closed {
		ws.mu.Unlock()
		return
	}
	ws.closed = true
	ws.mu.Unlock()
	ws.closeOwnedClient()
	ws.runCloseHooks()
}

// runCloseHooks fires the session's cleanup callback exactly once.
func (ws *WebSession) runCloseHooks() {
	ws.closeOnce.Do(func() {
		if ws.sniff != nil {
			ws.sniff.stop()
		}
		if ws.onClose != nil {
			ws.onClose()
		}
	})
}

// closeOwnedClient closes the SSH client of an ad-hoc session. Pooled clients
// are owned by the pool and must not be closed here.
func (ws *WebSession) closeOwnedClient() {
	if !ws.ownedClient || ws.client == nil {
		return
	}
	ws.clientOnce.Do(func() {
		_ = ws.client.Close()
	})
}

type terminalInputParams struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type terminalResizeParams struct {
	SessionID string `json:"sessionId"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

func (s *Server) handleTerminalInput(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p terminalInputParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	ws, ok := s.session(p.SessionID)
	if !ok {
		return nil, fmt.Errorf("会话不存在")
	}
	ws.HandleInput([]byte(p.Data))
	return nil, nil
}

func (s *Server) handleTerminalResize(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p terminalResizeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	ws, ok := s.session(p.SessionID)
	if !ok {
		return nil, fmt.Errorf("会话不存在")
	}
	ws.Resize(p.Cols, p.Rows)
	return nil, nil
}

func (s *Server) handleTerminalClose(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	ws, ok := s.session(p.SessionID)
	if !ok {
		return nil, nil
	}
	ws.Close()
	s.unregister(p.SessionID)
	return nil, nil
}

func (s *Server) session(id string) (*WebSession, bool) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	ws, ok := s.sessions[id]
	return ws, ok
}

// terminalDataMsg is an outbound push for terminal output / lifecycle events.
type terminalDataMsg struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	Data      string `json:"data,omitempty"`
}

// credentialPrompt reports whether a dial error is one the UI can resolve by
// asking the user for a credential. It returns the prompt payload to send back
// ({needPassword} / {needPassphrase}) and true when a prompt applies.
//
//   - An encrypted managed key without a usable passphrase is resolved by
//     prompting for that key's passphrase and retrying.
//   - An authentication failure means the UI should prompt for a password and
//     retry, instead of showing a dead-end error.
func (s *Server) credentialPrompt(err error) (map[string]interface{}, bool) {
	if err == nil {
		return nil, false
	}
	var pe *sshclient.PassphraseError
	if errors.As(err, &pe) {
		name := ""
		if k, kerr := s.findKey(pe.KeyID); kerr == nil {
			name = k.Name
		}
		msg := "该私钥已加密，请输入口令"
		if name != "" {
			msg = fmt.Sprintf("私钥「%s」已加密，请输入口令", name)
		}
		return map[string]interface{}{"needPassphrase": true, "keyId": pe.KeyID, "message": msg}, true
	}
	if needsPassword(err) {
		return map[string]interface{}{"needPassword": true, "message": err.Error()}, true
	}
	return nil, false
}

// needsPassword reports whether a dial error stems from missing or wrong
// credentials, which the UI should resolve by prompting for a password.
func needsPassword(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, kw := range []string{
		"unable to authenticate",
		"no authentication method",
		"handshake failed",
	} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// sendZmFile delivers a fully assembled upload file to the transfer session.
// If no transfer is running yet (rz only printed its banner), a fresh sender
// session is started and prods rz with ZRQINIT. Shared by the chunked upload
// handlers (sendEnd) and the legacy single-shot path.
func (s *Server) sendZmFile(ws *WebSession, f zmFile) error {
	ws.zmMu.Lock()
	zm := ws.zm
	if zm != nil && zm.active() {
		ws.zmMu.Unlock()
		if zm.sendable() {
			select {
			case zm.fileCh <- f:
				return nil
			case <-zm.done:
				return fmt.Errorf("传输已结束")
			}
		}
		// The session is alive but its direction was never resolved (the
		// remote rz ZRINIT frame may have been split across output chunks).
		// Re-arm it as the sender and prod rz with ZRQINIT.
		zm.stateMu.Lock()
		mode := zm.mode
		zm.stateMu.Unlock()
		if mode != zmModeUnknown {
			return fmt.Errorf("当前会话未在等待文件")
		}
		zm.presetSend(f)
		return nil
	}
	// No active transfer: rz is waiting on its banner; start a sender session
	// that answers with ZRQINIT and streams the file once rz replies ZRINIT.
	zm = startZmSession(ws)
	zm.presetSend(f)
	ws.zm = zm
	ws.zmUploadPrompted = false
	ws.zmMu.Unlock()
	return nil
}

// handleZmodemSendBegin starts a chunked upload: records file metadata and
// resets the accumulation buffer.
func (s *Server) handleZmodemSendBegin(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Name      string `json:"name"`
		Size      int    `json:"size"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, fmt.Errorf("文件名不能为空")
	}
	ws, ok := s.session(p.SessionID)
	if !ok {
		return nil, fmt.Errorf("会话不存在")
	}
	ws.zmMu.Lock()
	ws.uploadName = p.Name
	ws.uploadSize = p.Size
	ws.uploadData = nil
	ws.zmMu.Unlock()
	return nil, nil
}

// handleZmodemSendChunk appends one base64-encoded chunk to the upload buffer.
// Chunks arrive in order because the browser awaits each RPC before sending
// the next one.
func (s *Server) handleZmodemSendChunk(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Index     int    `json:"index"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	ws, ok := s.session(p.SessionID)
	if !ok {
		return nil, fmt.Errorf("会话不存在")
	}
	data, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return nil, fmt.Errorf("文件数据解码失败")
	}
	ws.zmMu.Lock()
	ws.uploadData = append(ws.uploadData, data...)
	ws.zmMu.Unlock()
	return nil, nil
}

// handleZmodemSendEnd finalizes the upload: assembles zmFile and hands it to
// the transfer session.
func (s *Server) handleZmodemSendEnd(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	ws, ok := s.session(p.SessionID)
	if !ok {
		return nil, fmt.Errorf("会话不存在")
	}
	ws.zmMu.Lock()
	f := zmFile{name: ws.uploadName, size: ws.uploadSize, data: ws.uploadData}
	ws.uploadData = nil
	ws.zmMu.Unlock()
	if f.name == "" {
		return nil, fmt.Errorf("请先调用 sendBegin")
	}
	if err := s.sendZmFile(ws, f); err != nil {
		return nil, err
	}
	return nil, nil
}

// handleZmodemCancel aborts an active transfer.
func (s *Server) handleZmodemCancel(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	ws, ok := s.session(p.SessionID)
	if !ok {
		return nil, nil
	}
	ws.zmMu.Lock()
	zm := ws.zm
	ws.zmMu.Unlock()
	if zm != nil {
		zm.cancel()
	}
	return nil, nil
}
