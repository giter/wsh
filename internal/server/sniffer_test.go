package server

import (
	"strings"
	"testing"
)

// sink collects push messages in place, so tests can inspect what the sniffer
// emitted.
type sink struct{ msgs []interface{} }

func (s *sink) push(v interface{}) { s.msgs = append(s.msgs, v) }

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
