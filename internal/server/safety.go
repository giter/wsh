package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"sshclient/internal/safety"
)

// This file wires the local AST safety engine into the terminal path.
//
// Only the Smart Input submission goes through the engine (`terminal.exec`).
// Raw keystrokes keep using `terminal.input`, which stays a transparent pipe, so
// a TUI application never sees its keys intercepted.

func (s *Server) handleSafetyCheck(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	return safety.Analyze(p.Command), nil
}

// handleSafetyConfirm issues a one-shot authorisation token for a command the
// user has explicitly approved in the dry-run panel. The token is bound to the
// session and to the exact command text, expires quickly and can be used once,
// so a confirmation cannot be replayed for a different command later.
func (s *Server) handleSafetyConfirm(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Command   string `json:"command"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Command == "" {
		return nil, fmt.Errorf("命令不能为空")
	}
	res := safety.Analyze(p.Command)
	if res.Blocked() {
		return nil, fmt.Errorf("%s", res.Reason)
	}
	if !res.NeedsConfirm() {
		// Nothing to approve: the caller can execute it directly.
		return map[string]interface{}{"token": "", "result": res}, nil
	}
	return map[string]interface{}{
		"token":  s.confirms.Issue(p.SessionID, p.Command),
		"result": res,
	}, nil
}

type terminalExecParams struct {
	SessionID string `json:"sessionId"`
	Command   string `json:"command"`
	// ConfirmToken is the one-shot token from safety.confirm, required only for
	// commands the engine classifies as needing confirmation.
	ConfirmToken string `json:"confirmToken"`
	// TrackID, when set, asks the backend to capture this command's output and
	// push it back tagged with the same value. The UI passes the id of the
	// reasoning card that proposed the command, which is how the model's next turn
	// gets the output of its own suggestion.
	TrackID string `json:"trackId"`
}

// execResult tells the browser what happened to a submitted command.
type execResult struct {
	// Written is true when the command was actually sent to the remote shell.
	Written bool `json:"written"`
	// Blocked is true when the engine refused the command outright (红区).
	Blocked bool `json:"blocked"`
	// Confirm is true when the command needs explicit approval (黄区) and no
	// valid token was supplied.
	Confirm bool          `json:"confirm"`
	Result  safety.Result `json:"result"`
}

func (s *Server) handleTerminalExec(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p terminalExecParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Command == "" {
		return nil, fmt.Errorf("命令不能为空")
	}
	ws, ok := s.session(p.SessionID)
	if !ok {
		return nil, fmt.Errorf("会话不存在")
	}

	decision := decideExec(p.SessionID, p.Command, p.ConfirmToken, s.confirms)
	out := execResult{
		Blocked: decision.Blocked,
		Confirm: decision.NeedsConfirm,
		Result:  decision.Result,
	}
	if decision.Blocked || decision.NeedsConfirm {
		return out, nil
	}
	// Submit the line exactly as typed; the shell echoes it like any other input.
	//
	// Tracking is armed before the write so no early output is missed. A plain
	// submission with no track id ends whatever segment was still open: the
	// output of two commands must never land in one card.
	if ws.sniff != nil {
		if p.TrackID != "" {
			ws.sniff.armCapture(p.TrackID, p.Command, ws.pushMsg)
		} else {
			ws.sniff.finishCapture(ws.pushMsg)
		}
	}
	ws.HandleInput([]byte(p.Command + "\n"))
	out.Written = true
	return out, nil
}

// execDecision is the pure part of terminal.exec: it holds no SSH state, which
// makes the gate easy to test.
type execDecision struct {
	Blocked      bool
	NeedsConfirm bool
	Result       safety.Result
}

func decideExec(sessionID, cmd, token string, confirms *confirmStore) execDecision {
	res := safety.Analyze(cmd)
	switch {
	case res.Blocked():
		return execDecision{Blocked: true, Result: res}
	case res.NeedsConfirm():
		if confirms.Consume(sessionID, cmd, token) {
			return execDecision{Result: res}
		}
		return execDecision{NeedsConfirm: true, Result: res}
	default:
		return execDecision{Result: res}
	}
}

// confirmTokenTTL is how long an approval stays valid. Long enough to read the
// dry-run panel, short enough that a stale approval is worthless.
const confirmTokenTTL = 2 * time.Minute

// confirmStore holds one-shot approval tokens for yellow-zone commands.
type confirmStore struct {
	mu     sync.Mutex
	tokens map[string]confirmEntry
}

type confirmEntry struct {
	sessionID string
	hash      string
	expires   time.Time
}

func newConfirmStore() *confirmStore {
	return &confirmStore{tokens: make(map[string]confirmEntry)}
}

// Issue returns a fresh token authorising cmd in the given session.
func (cs *confirmStore) Issue(sessionID, cmd string) string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is not recoverable in practice; fall back to a
		// time-derived token so the flow still works (it is only a CSRF-style
		// guard on a loopback socket).
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", sessionID, cmd, time.Now().UnixNano())))
		copy(buf, sum[:16])
	}
	token := hex.EncodeToString(buf)

	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.gcLocked()
	cs.tokens[token] = confirmEntry{
		sessionID: sessionID,
		hash:      hashCommand(cmd),
		expires:   time.Now().Add(confirmTokenTTL),
	}
	return token
}

// Consume validates and burns a token. It returns false when the token is
// unknown, expired, already used, or was issued for another command/session.
func (cs *confirmStore) Consume(sessionID, cmd, token string) bool {
	if token == "" {
		return false
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	entry, ok := cs.tokens[token]
	if !ok {
		return false
	}
	delete(cs.tokens, token) // one-shot, even on failure
	if time.Now().After(entry.expires) {
		return false
	}
	return entry.sessionID == sessionID && entry.hash == hashCommand(cmd)
}

// gcLocked drops expired tokens. Callers must hold cs.mu.
func (cs *confirmStore) gcLocked() {
	now := time.Now()
	for token, entry := range cs.tokens {
		if now.After(entry.expires) {
			delete(cs.tokens, token)
		}
	}
}

func hashCommand(cmd string) string {
	sum := sha256.Sum256([]byte(cmd))
	return hex.EncodeToString(sum[:])
}
