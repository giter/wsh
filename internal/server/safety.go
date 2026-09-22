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

// handleSafetyConfirm issues the authorisation for a command the user explicitly
// approved in the dry-run panel. It also records how far that approval reaches:
//
//   - scope "": one-shot. The token is bound to the session and to the exact
//     command text, expires in two minutes and can be used once, so a confirmation
//     cannot be replayed later.
//   - scope "session": remember it for this session only (in memory).
//   - scope "always": remember it for good, in settings.json.
//
// Recording the wider scopes here (rather than on a bare "allow" call) keeps the
// approval and the execution inseparable: one user gesture, one decision.
func (s *Server) handleSafetyConfirm(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Command   string `json:"command"`
		Scope     string `json:"scope"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Command == "" {
		return nil, fmt.Errorf("命令不能为空")
	}
	res := safety.Analyze(p.Command)
	if res.Blocked() {
		// Red zone is never approvable, whatever scope the caller asks for: the
		// allowlist must not become a way to run a disk-wiping command.
		return nil, fmt.Errorf("%s", res.Reason)
	}

	switch p.Scope {
	case "session":
		s.approvals.Allow(p.SessionID, p.Command)
		return map[string]interface{}{"token": "", "result": res, "scope": "session"}, nil
	case "always":
		if err := s.store.AllowCommand(p.Command); err != nil {
			return nil, err
		}
		return map[string]interface{}{"token": "", "result": res, "scope": "always"}, nil
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
	// valid token or standing approval covered it.
	Confirm bool          `json:"confirm"`
	Result  safety.Result `json:"result"`
	// AllowedBy names the standing approval that let a yellow-zone command through
	// ("session" or "always"), so the UI can say why it did not ask.
	AllowedBy string `json:"allowedBy,omitempty"`
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
	// A command the user already approved (this session, or for good) skips the
	// panel. Red still wins: decideExec refuses it before we get here.
	if decision.NeedsConfirm {
		switch {
		case s.approvals.Allowed(p.SessionID, p.Command):
			out.Confirm, out.AllowedBy = false, "session"
		case s.store.CommandAllowed(p.Command):
			out.Confirm, out.AllowedBy = false, "always"
		}
	}
	if out.Blocked || out.Confirm {
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

// approvalStore remembers the commands the user allowed "for this session".
//
// It is keyed by session id, so opening a second terminal to a different host does
// not inherit an approval given elsewhere, and it only lives in memory: restarting
// the app forgets every session-scoped approval, which is the point of "本会话".
// The permanent list lives in settings.json instead (see storage.Store).
type approvalStore struct {
	mu    sync.Mutex
	byKey map[string]map[string]struct{}
}

func newApprovalStore() *approvalStore {
	return &approvalStore{byKey: make(map[string]map[string]struct{})}
}

// Allow records a session-scoped approval for the exact command text.
func (as *approvalStore) Allow(sessionID, cmd string) {
	if sessionID == "" || cmd == "" {
		return
	}
	as.mu.Lock()
	defer as.mu.Unlock()
	set := as.byKey[sessionID]
	if set == nil {
		set = make(map[string]struct{})
		as.byKey[sessionID] = set
	}
	set[cmd] = struct{}{}
}

// Allowed reports whether the session already approved this exact command.
func (as *approvalStore) Allowed(sessionID, cmd string) bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	_, ok := as.byKey[sessionID][cmd]
	return ok
}

// Forget drops everything a session approved, so closing a terminal does not
// leave approvals behind for a future session that happens to reuse the id.
func (as *approvalStore) Forget(sessionID string) {
	as.mu.Lock()
	delete(as.byKey, sessionID)
	as.mu.Unlock()
}

// handleSafetyAllowed lists the permanent approvals, so the options window can
// show — and revoke — what the user will never be asked about again.
func (s *Server) handleSafetyAllowed(c *wsClient, params json.RawMessage) (interface{}, error) {
	return map[string]interface{}{"commands": s.store.AllowedCommands()}, nil
}

// handleSafetyRevoke removes a permanent approval, or all of them.
func (s *Server) handleSafetyRevoke(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		Command string `json:"command"`
		All     bool   `json:"all"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	var err error
	if p.All {
		err = s.store.ForgetAllCommands()
	} else {
		err = s.store.ForgetCommand(p.Command)
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"commands": s.store.AllowedCommands()}, nil
}
