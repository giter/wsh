package server

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// sink collects push messages in place, so tests can inspect what the sniffer
// emitted. The mutex matters for captures: their completion timer pushes from its
// own goroutine.
type sink struct {
	mu   sync.Mutex
	msgs []interface{}
}

func (s *sink) push(v interface{}) {
	s.mu.Lock()
	s.msgs = append(s.msgs, v)
	s.mu.Unlock()
}

// snapshot returns a copy of the collected messages.
func (s *sink) snapshot() []interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]interface{}(nil), s.msgs...)
}

// await blocks until at least n messages have been collected, so a test can wait
// for a capture timer instead of sleeping for a fixed period.
func (s *sink) await(t *testing.T, n int, within time.Duration) []interface{} {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		got := s.snapshot()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d pushes, got %d: %+v", n, len(got), got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSnifferAltScreen(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	// Entering vim (smcup) must be reported once.
	s.feed([]byte("$ vim /etc/hosts\r\n\x1b[?1049h\x1b[H"), sk.push)
	if len(sk.msgs) != 1 {
		t.Fatalf("expected one push on alt-screen enter, got %d: %+v", len(sk.msgs), sk.msgs)
	}
	m, ok := sk.msgs[0].(terminalModeMsg)
	if !ok || !m.AltScreen || m.SessionID != "s1" {
		t.Fatalf("unexpected message: %+v", sk.msgs[0])
	}

	// A redraw inside the alternate screen is not a transition.
	s.feed([]byte("\x1b[?1049h"), sk.push)
	if len(sk.msgs) != 1 {
		t.Fatalf("re-entering should not push again: %+v", sk.msgs)
	}

	// Leaving it (rmcup) is reported.
	s.feed([]byte("\x1b[?1049l$ "), sk.push)
	if len(sk.msgs) != 2 {
		t.Fatalf("expected an alt-screen leave push, got %+v", sk.msgs)
	}
	if m, ok := sk.msgs[1].(terminalModeMsg); !ok || m.AltScreen {
		t.Fatalf("unexpected leave message: %+v", sk.msgs[1])
	}
}

// TestSnifferAltScreenSplitAcrossChunks covers the sequence arriving in two
// reads, which is common with a slow PTY.
func TestSnifferAltScreenSplitAcrossChunks(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	s.feed([]byte("\x1b[?10"), sk.push)
	if len(sk.msgs) != 0 {
		t.Fatalf("partial sequence must not push: %+v", sk.msgs)
	}
	s.feed([]byte("49h"), sk.push)
	if len(sk.msgs) != 1 {
		t.Fatalf("split sequence should be detected: %+v", sk.msgs)
	}
}

func TestSnifferErrors(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		kind     string
		severity string
	}{
		{"segfault", "$ ./app\nSegmentation fault (core dumped)\n", "segfault", "high"},
		{"oom", "kernel: Out of memory: Killed process 1234 (nginx)\n", "oom", "high"},
		{"fatal", "2026-09-21T18:00:12 FATAL config load failed\n", "fatal", "high"},
		{"permission", "cat: /etc/shadow: Permission denied\n", "permission", "medium"},
		{"not found", "$ nginxx\nbash: nginxx: command not found\n", "not-found", "medium"},
		{"port in use", "bind: Address already in use\n", "port-in-use", "medium"},
		{"panic", "panic: runtime error: index out of range\n", "panic", "high"},
		{"disk full", "write error: No space left on device\n", "disk-full", "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sk := &sink{}
			s := newSniffer("s1")
			s.feed([]byte(tt.out), sk.push)
			if len(sk.msgs) != 1 {
				t.Fatalf("expected one error push, got %+v", sk.msgs)
			}
			m, ok := sk.msgs[0].(terminalErrorMsg)
			if !ok {
				t.Fatalf("unexpected message type: %+v", sk.msgs[0])
			}
			if m.Kind != tt.kind || m.Severity != tt.severity {
				t.Fatalf("got kind=%q severity=%q, want kind=%q severity=%q", m.Kind, m.Severity, tt.kind, tt.severity)
			}
			if m.SessionID != "s1" {
				t.Fatalf("session id missing: %+v", m)
			}
		})
	}
}

// TestSnifferExcerptIsClean checks the excerpt is the visible line, without ANSI
// escapes, because it is shown to the user and (masked) to the LLM.
func TestSnifferExcerptIsClean(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")
	s.feed([]byte("\x1b[31mFATAL\x1b[0m: config line 42 missing ';'\r\n"), sk.push)
	if len(sk.msgs) != 1 {
		t.Fatalf("expected one push, got %+v", sk.msgs)
	}
	m := sk.msgs[0].(terminalErrorMsg)
	if strings.Contains(m.Excerpt, "\x1b") {
		t.Fatalf("excerpt still contains escape sequences: %q", m.Excerpt)
	}
	if !strings.Contains(m.Excerpt, "config line 42 missing") {
		t.Fatalf("unexpected excerpt: %q", m.Excerpt)
	}
}

// TestSnifferSuppressesRepeats keeps a log that repeats the same failure from
// flooding the reasoning pane.
func TestSnifferSuppressesRepeats(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	for i := 0; i < 20; i++ {
		s.feed([]byte("ERROR connection refused by upstream\n"), sk.push)
	}
	if len(sk.msgs) != 1 {
		t.Fatalf("repeated identical errors should be suppressed, got %d pushes", len(sk.msgs))
	}
}

// TestSnifferQuietOnNormalOutput guards against the sniffer becoming noisy on
// ordinary command output.
func TestSnifferQuietOnNormalOutput(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	s.feed([]byte("total 24\ndrwxr-xr-x 2 root root 4096 Sep 21 18:00 .\n"), sk.push)
	s.feed([]byte("active (running) since Sun 2026-09-21 18:00:12 UTC\n"), sk.push)
	if len(sk.msgs) != 0 {
		t.Fatalf("ordinary output should not produce pushes: %+v", sk.msgs)
	}
}

func TestStripANSI(t *testing.T) {
	cases := map[string]string{
		"\x1b[31mred\x1b[0m":         "red",
		"\x1b]0;title\x07text":       "text",
		"\x1b[?25lhidden\x1b[?25h":   "hidden",
		"line1\rline2":               "line1\nline2",
		"plain text":                 "plain text",
		"\x1b[2K\x1b[1Gprogress 50%": "progress 50%",
	}
	for in, want := range cases {
		if got := stripANSI(in); got != want {
			t.Errorf("stripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSnifferCarryDoesNotDuplicate matches the behaviour of a stream that is
// split mid-keyword: the error is still found, but only once.
func TestSnifferCarryDoesNotDuplicate(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	s.feed([]byte("tar: /data: Cannot open: Permission den"), sk.push)
	if len(sk.msgs) != 0 {
		t.Fatalf("incomplete keyword should not match: %+v", sk.msgs)
	}
	s.feed([]byte("ied\n"), sk.push)
	if len(sk.msgs) != 1 {
		t.Fatalf("split keyword should match exactly once, got %+v", sk.msgs)
	}
	// The suppression window keeps a re-scan from duplicating it.
	s.feed([]byte("tar: /data: Cannot open: Permission denied\n"), sk.push)
	if len(sk.msgs) != 1 {
		t.Fatalf("duplicate should be suppressed, got %+v", sk.msgs)
	}
}

func TestLooksLikePrompt(t *testing.T) {
	tests := map[string]bool{
		"lee@debian:~$ ":                        true,
		"root@host:/var/log# ":                  true,
		"$ ":                                    true,
		"❯ ":                                    true,
		"(venv) lee@debian:~/app$ ":             true,
		"total 24\ndrwxr-xr-x 2 root root 4096": false,
		"lee@debian:~$ ls -la\n":                false,
		"":                                      false,
		// A line longer than a real prompt may be output that merely ends in "$".
		strings.Repeat("a", maxPromptLen) + " $": false,
	}
	for in, want := range tests {
		if got := looksLikePrompt(in); got != want {
			t.Errorf("looksLikePrompt(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestSnifferCaptureEmitsOnPrompt covers the happy path of the command → output
// → analysis loop: the segment is only handed over once the shell has printed its
// next prompt and the stream has gone quiet.
func TestSnifferCaptureEmitsOnPrompt(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	s.armCapture("r7", "ps aux | grep wglink", sk.push)
	if got := sk.snapshot(); len(got) != 0 {
		t.Fatalf("arming must not push anything: %+v", got)
	}

	s.feed([]byte("ps aux | grep wglink\r\nroot 292 wglink --serve\r\nlee@debian:~$ "), sk.push)

	got := sk.await(t, 1, 3*time.Second)
	m, ok := got[0].(terminalOutputMsg)
	if !ok {
		t.Fatalf("unexpected message: %+v", got[0])
	}
	if m.TrackID != "r7" || m.SessionID != "s1" {
		t.Fatalf("tracking metadata lost: %+v", m)
	}
	if !strings.Contains(m.Text, "wglink --serve") {
		t.Fatalf("captured text is missing the output: %q", m.Text)
	}
	if m.TimedOut || m.Truncated {
		t.Fatalf("a complete segment must not be flagged: %+v", m)
	}
}

// TestSnifferCaptureFlushedByNextCommand keeps two commands' output from landing
// in the same card.
// TestSnifferCaptureReportsStreaming covers long-running commands: once output has
// kept arriving past captureStreaming without going quiet, the sniffer announces a
// stream (so the card can offer Ctrl+C) exactly once, and the command's output
// still lands in its final segment.
func TestSnifferCaptureReportsStreaming(t *testing.T) {
	old := captureStreaming
	captureStreaming = 20 * time.Millisecond
	defer func() { captureStreaming = old }()

	sk := &sink{}
	s := newSniffer("s1")

	s.armCapture("r5", "ping 223.5.5.5", sk.push)
	// Keep the capture busy so it never goes quiet: a bare ping streams forever.
	for i := 0; i < 6; i++ {
		s.feed([]byte("64 bytes from 223.5.5.5: icmp_seq=1 ttl=117 time=12.7 ms\n"), sk.push)
		time.Sleep(10 * time.Millisecond)
	}

	got := sk.await(t, 1, time.Second)
	m, ok := got[0].(terminalStreamingMsg)
	if !ok {
		t.Fatalf("expected a streaming notice, got %+v", got[0])
	}
	if m.TrackID != "r5" || m.SessionID != "s1" || m.Command != "ping 223.5.5.5" {
		t.Fatalf("streaming notice lost its metadata: %+v", m)
	}

	// Stopping the command (Ctrl+C) silences it, which closes the segment normally.
	s.finishCapture(sk.push)
	all := sk.snapshot()
	if len(all) != 2 {
		t.Fatalf("expected the stream notice plus one segment, got %+v", all)
	}
	out, ok := all[1].(terminalOutputMsg)
	if !ok || out.TrackID != "r5" || out.TimedOut {
		t.Fatalf("unexpected final segment: %+v", all[1])
	}
	if !strings.Contains(out.Text, "icmp_seq=1") {
		t.Fatalf("the streamed output should still be captured: %q", out.Text)
	}
}

// TestLooksLikeCredentialPrompt pins the shapes that count as a command asking
// for a secret, and — just as important — the ordinary output that does not.
func TestLooksLikeCredentialPrompt(t *testing.T) {
	yes := []string{
		"[sudo] password for lee: ",
		"Password:",
		"password:",
		"lee@debian:~$ ssh deploy@10.0.0.5\ndeploy@10.0.0.5's password:",
		"Enter passphrase for key '/home/lee/.ssh/id_rsa':",
		"New password:",
		"请输入密码：",
	}
	for _, s := range yes {
		if !looksLikeCredentialPrompt(s) {
			t.Fatalf("looksLikeCredentialPrompt(%q) = false, want true", s)
		}
	}

	no := []string{
		"",
		"lee@debian:~$ ",
		"Warning: your password will expire in 3 days",
		"Authentication failed: invalid password",
		"total 4\ndrwxr-xr-x 2 lee lee 4096 password/",
		"--spin: 3 threads started",
		// A real output line is not a short prompt, even if it ends in a colon.
		strings.Repeat("x", 200) + " password:",
	}
	for _, s := range no {
		if looksLikeCredentialPrompt(s) {
			t.Fatalf("looksLikeCredentialPrompt(%q) = true, want false", s)
		}
	}
}

// TestSnifferHoldsCaptureAtCredentialPrompt covers `sudo`: the segment must stay
// open at the password prompt instead of being delivered — reporting that prompt
// as the command's output is exactly what made a `sudo` that never ran look like
// a success. Once the user answers, the real output is what gets captured.
func TestSnifferHoldsCaptureAtCredentialPrompt(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	s.armCapture("r9", "sudo systemctl restart wglink", sk.push)
	s.feed([]byte("sudo systemctl restart wglink\r\n[sudo] password for lee: "), sk.push)

	// The prompt must be announced as waiting, not delivered as a result.
	got := sk.await(t, 1, 3*time.Second)
	m, ok := got[0].(terminalWaitingMsg)
	if !ok || !m.Waiting || m.TrackID != "r9" || m.SessionID != "s1" {
		t.Fatalf("expected a waiting notice, got %+v", got[0])
	}
	if len(got) != 1 {
		t.Fatalf("the segment must not be delivered while it waits: %+v", got)
	}

	// The user types the password: the command's real output arrives and the
	// segment ends at the shell prompt.
	s.feed([]byte("\r\nRestarting wglink...\r\nlee@debian:~$ "), sk.push)

	all := sk.await(t, 3, 3*time.Second)
	var out terminalOutputMsg
	var found, resumed bool
	for _, v := range all {
		switch msg := v.(type) {
		case terminalOutputMsg:
			out, found = msg, true
		case terminalWaitingMsg:
			if !msg.Waiting {
				resumed = true
			}
		}
	}
	if !found {
		t.Fatalf("expected the segment after the password, got %+v", all)
	}
	if !resumed {
		t.Fatalf("expected a resume notice so the card leaves the waiting state: %+v", all)
	}
	if out.WaitingInput {
		t.Fatalf("a command that ran to completion must not be flagged as waiting: %+v", out)
	}
	if !strings.Contains(out.Text, "Restarting wglink") {
		t.Fatalf("the output after the password should be captured: %q", out.Text)
	}
}

func TestSnifferCaptureFlushedByNextCommand(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	s.armCapture("r1", "first", sk.push)
	s.feed([]byte("first output\n"), sk.push)
	s.armCapture("r2", "second", sk.push)

	got := sk.snapshot()
	if len(got) != 1 {
		t.Fatalf("arming a second capture should flush the first: %+v", got)
	}
	m := got[0].(terminalOutputMsg)
	if m.TrackID != "r1" || !strings.Contains(m.Text, "first output") {
		t.Fatalf("unexpected flushed segment: %+v", m)
	}
}

// TestSnifferCaptureCancelledByAltScreen stops tracking when a full-screen
// application takes over: there is no line-oriented output to attribute.
func TestSnifferCaptureCancelledByAltScreen(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	s.armCapture("r3", "htop", sk.push)
	s.feed([]byte("\x1b[?1049h"), sk.push)

	got := sk.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected a mode push and a capture push, got %+v", got)
	}
	if _, ok := got[0].(terminalModeMsg); !ok {
		t.Fatalf("first push should be the alt-screen transition: %+v", got[0])
	}
	m, ok := got[1].(terminalOutputMsg)
	if !ok || m.TrackID != "r3" {
		t.Fatalf("capture should be flushed: %+v", got[1])
	}
}

// TestSnifferCaptureInactiveByDefault guards the common case: output that was not
// requested by a card must never produce a capture push.
func TestSnifferCaptureInactiveByDefault(t *testing.T) {
	sk := &sink{}
	s := newSniffer("s1")

	s.feed([]byte("total 24\ndrwxr-xr-x 2 root root 4096\n"), sk.push)
	s.finishCapture(sk.push)

	if got := sk.snapshot(); len(got) != 0 {
		t.Fatalf("an untracked session should stay quiet: %+v", got)
	}
}
