package server

import (
	"encoding/json"
	"testing"
	"time"
)

// TestDecideExec covers the gate itself: red zone is refused, yellow zone needs
// a one-shot token, green zone passes straight through.
func TestDecideExec(t *testing.T) {
	store := newConfirmStore()
	const sess = "s1"

	// 红区：直接拒绝，不需要令牌。
	if d := decideExec(sess, "rm -rf /", "", store); !d.Blocked || d.NeedsConfirm {
		t.Fatalf("red zone should be blocked: %+v", d)
	}
	// 黄区：无令牌要求确认。
	if d := decideExec(sess, "rm -rf /tmp/build", "", store); !d.NeedsConfirm || d.Blocked {
		t.Fatalf("yellow zone without a token should ask for confirmation: %+v", d)
	}
	// 黄区：带有效令牌放行。
	token := store.Issue(sess, "rm -rf /tmp/build")
	if d := decideExec(sess, "rm -rf /tmp/build", token, store); d.Blocked || d.NeedsConfirm {
		t.Fatalf("yellow zone with a valid token should pass: %+v", d)
	}
	// 令牌是一次性的。
	if d := decideExec(sess, "rm -rf /tmp/build", token, store); !d.NeedsConfirm {
		t.Fatalf("token must not be reusable: %+v", d)
	}
	// 绿区：无需令牌直接放行。
	if d := decideExec(sess, "ls -la", "", store); d.Blocked || d.NeedsConfirm {
		t.Fatalf("green zone should pass: %+v", d)
	}
}

// TestConfirmStoreBinding makes sure an approval cannot be moved to another
// command or another session.
func TestConfirmStoreBinding(t *testing.T) {
	store := newConfirmStore()

	token := store.Issue("s1", "rm -rf /tmp/a")
	if store.Consume("s1", "rm -rf /tmp/b", token) {
		t.Fatal("token must not authorise a different command")
	}

	token = store.Issue("s1", "rm -rf /tmp/a")
	if store.Consume("s2", "rm -rf /tmp/a", token) {
		t.Fatal("token must not authorise a different session")
	}

	token = store.Issue("s1", "rm -rf /tmp/a")
	if !store.Consume("s1", "rm -rf /tmp/a", token) {
		t.Fatal("token should authorise its own command and session")
	}

	if store.Consume("s1", "rm -rf /tmp/a", "") {
		t.Fatal("an empty token must never be accepted")
	}
	if store.Consume("s1", "rm -rf /tmp/a", "nope") {
		t.Fatal("an unknown token must never be accepted")
	}
}

func TestConfirmStoreExpiry(t *testing.T) {
	store := newConfirmStore()
	token := store.Issue("s1", "reboot")
	// Age the entry past its TTL instead of sleeping for it.
	store.mu.Lock()
	e := store.tokens[token]
	e.expires = time.Now().Add(-time.Second)
	store.tokens[token] = e
	store.mu.Unlock()

	if store.Consume("s1", "reboot", token) {
		t.Fatal("an expired token must not be accepted")
	}
}

// TestHandleTerminalExecEndToEnd drives the real RPC handler against the fake
// SSH server: a blocked command must never reach the remote, a safe one must.
func TestHandleTerminalExecEndToEnd(t *testing.T) {
	srv := newTestServer(nil)
	ws, fr, _ := newTestWs(t, "sess-1")
	srv.register(ws)

	call := func(t *testing.T, cmd, token string) execResult {
		t.Helper()
		raw, err := json.Marshal(terminalExecParams{SessionID: "sess-1", Command: cmd, ConfirmToken: token})
		if err != nil {
			t.Fatal(err)
		}
		// The handler never touches the client (the reply is sent by dispatch),
		// so nil is fine here.
		out, err := srv.handleTerminalExec(nil, raw)
		if err != nil {
			t.Fatalf("handleTerminalExec(%q): %v", cmd, err)
		}
		return out.(execResult)
	}

	expectNothingWritten := func(t *testing.T) {
		t.Helper()
		select {
		case got := <-fr.rcvCh:
			t.Fatalf("nothing should have been written to the remote, got %q", got)
		case <-time.After(150 * time.Millisecond):
		}
	}

	// 红区：拒绝，且没有任何字节下发。
	if res := call(t, "rm -rf /", ""); !res.Blocked || res.Written {
		t.Fatalf("red zone: %+v", res)
	}
	expectNothingWritten(t)

	// 黄区：要求确认，且没有任何字节下发。
	if res := call(t, "rm -rf /tmp/build", ""); !res.Confirm || res.Written {
		t.Fatalf("yellow zone without token: %+v", res)
	}
	expectNothingWritten(t)

	// 黄区 + 令牌：放行，命令（含换行）抵达远端。
	token := srv.confirms.Issue("sess-1", "rm -rf /tmp/build")
	if res := call(t, "rm -rf /tmp/build", token); !res.Written || res.Confirm || res.Blocked {
		t.Fatalf("yellow zone with token: %+v", res)
	}
	select {
	case got := <-fr.rcvCh:
		if string(got) != "rm -rf /tmp/build\n" {
			t.Fatalf("remote received %q, want the command plus a newline", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the command to reach the remote")
	}

	// 绿区：直接放行。
	if res := call(t, "ls -la", ""); !res.Written {
		t.Fatalf("green zone: %+v", res)
	}
	select {
	case got := <-fr.rcvCh:
		if string(got) != "ls -la\n" {
			t.Fatalf("remote received %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the command to reach the remote")
	}
}

// TestHandleSafetyConfirm covers the token issuance endpoint.
func TestHandleSafetyConfirm(t *testing.T) {
	srv := newTestServer(nil)

	issue := func(t *testing.T, cmd string) map[string]interface{} {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"sessionId": "s1", "command": cmd})
		out, err := srv.handleSafetyConfirm(nil, raw)
		if err != nil {
			t.Fatalf("handleSafetyConfirm(%q): %v", cmd, err)
		}
		return out.(map[string]interface{})
	}

	// 红区不给令牌。
	raw, _ := json.Marshal(map[string]string{"sessionId": "s1", "command": "rm -rf /"})
	if _, err := srv.handleSafetyConfirm(nil, raw); err == nil {
		t.Fatal("a red-zone command must not be confirmable")
	}

	// 绿区不需要令牌。
	if got := issue(t, "ls"); got["token"] != "" {
		t.Fatalf("a green-zone command needs no token, got %v", got["token"])
	}

	// 黄区返回可用令牌。
	got := issue(t, "systemctl stop nginx")
	token, _ := got["token"].(string)
	if token == "" {
		t.Fatal("expected a token for a yellow-zone command")
	}
	if !srv.confirms.Consume("s1", "systemctl stop nginx", token) {
		t.Fatal("the issued token should be valid")
	}
}

// TestHandleSafetyCheck exposes the classification used for live highlighting.
func TestHandleSafetyCheck(t *testing.T) {
	srv := newTestServer(nil)
	raw, _ := json.Marshal(map[string]string{"command": "rm -rf /"})
	out, err := srv.handleSafetyCheck(nil, raw)
	if err != nil {
		t.Fatal(err)
	}
	res := out.(interface{ Blocked() bool })
	if !res.Blocked() {
		t.Fatal("expected the check to report the command as blocked")
	}
}
