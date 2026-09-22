package server

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
	"time"
)

// sniffer taps the bytes the terminal is about to render and derives two things
// the UI can act on, without ever consuming the stream itself:
//
//   - whether a full-screen (alternate screen) application is running, so the
//     Smart Input can switch to raw passthrough and stop stealing keys;
//   - error features in the output, which open a diagnostic card on the right.
//
// It is deliberately a passive tap: the same bytes are still delivered to
// xterm.js unchanged, and ZMODEM frames never reach it (they are diverted before
// pushOutput is called).
type sniffer struct {
	sessionID string

	mu        sync.Mutex
	carry     []byte // tail of the previous chunk, for sequences split by reads
	altScreen bool
	lastSig   string
	lastPush  time.Time
	// recent holds the tail of the visible output, used as (masked) context for
	// the reasoning panel. Capped so a runaway log cannot grow it without bound.
	recent []byte
}

func newSniffer(sessionID string) *sniffer {
	return &sniffer{sessionID: sessionID}
}

// carryLen is how much of the previous chunk is re-scanned. Escape sequences and
// the longest error keyword are far shorter than this.
const carryLen = 64

// errorSuppressWindow is how long an identical error is kept quiet, so a log that
// repeats the same line does not flood the reasoning pane.
const errorSuppressWindow = 10 * time.Second

// errorFloor is the minimum gap between two different error pushes.
const errorFloor = 300 * time.Millisecond

// recentCap bounds the context buffer kept per session.
const recentCap = 4 << 10

// feed inspects one chunk of terminal output. push delivers messages to the
// browser; it is called at most a couple of times per chunk.
func (s *sniffer) feed(chunk []byte, push func(interface{})) {
	if len(chunk) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	buf := chunk
	if len(s.carry) > 0 {
		buf = append(append([]byte{}, s.carry...), chunk...)
	}
	if len(buf) > carryLen {
		s.carry = append([]byte{}, buf[len(buf)-carryLen:]...)
	} else {
		s.carry = append([]byte{}, buf...)
	}

	s.scanAltScreen(buf, push)

	// Error detection works on the visible text only.
	text := stripANSI(string(buf))
	s.remember(text)
	s.scanErrors(text, push)
}

// remember appends to the tail buffer, keeping only its last recentCap bytes.
// Callers must hold s.mu.
func (s *sniffer) remember(text string) {
	s.recent = append(s.recent, text...)
	if len(s.recent) > recentCap {
		s.recent = append([]byte{}, s.recent[len(s.recent)-recentCap:]...)
	}
}

// recentText returns the tail of the visible output for the given session.
func (s *sniffer) recentText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.recent)
}

// altEnter/altLeave are the escape sequences that switch to and from the
// alternate screen buffer (smcup/rmcup as emitted by vim, htop, less, ...).
var (
	altEnter = []string{"\x1b[?1049h", "\x1b[?1047h", "\x1b[?47h"}
	altLeave = []string{"\x1b[?1049l", "\x1b[?1047l", "\x1b[?47l"}
)

func (s *sniffer) scanAltScreen(buf []byte, push func(interface{})) {
	for {
		idx, target := -1, false
		for _, p := range altEnter {
			if i := bytes.Index(buf, []byte(p)); i >= 0 && (idx < 0 || i < idx) {
				idx, target = i, true
			}
		}
		for _, p := range altLeave {
			if i := bytes.Index(buf, []byte(p)); i >= 0 && (idx < 0 || i < idx) {
				idx, target = i, false
			}
		}
		if idx < 0 {
			return
		}
		buf = buf[idx+1:]
		if target != s.altScreen {
			s.altScreen = target
			push(terminalModeMsg{
				Type:      "terminal.mode",
				SessionID: s.sessionID,
				AltScreen: target,
			})
		}
	}
}

// errorPattern matches one kind of failure in the terminal output.
type errorPattern struct {
	kind     string
	severity string
	// needles are matched literally against the ANSI-stripped text.
	needles []string
}

// errorPatterns are ordered so that the most specific/severe match wins.
var errorPatterns = []errorPattern{
	{kind: "panic", severity: "high", needles: []string{"panic:", "goroutine 1 [running]"}},
	{kind: "segfault", severity: "high", needles: []string{"Segmentation fault"}},
	{kind: "oom", severity: "high", needles: []string{"Out of memory", "out of memory", "OOMKilled", "Cannot allocate memory", "Killed process"}},
	{kind: "disk-full", severity: "high", needles: []string{"No space left on device"}},
	{kind: "fatal", severity: "high", needles: []string{"FATAL", "fatal:"}},
	{kind: "core-dump", severity: "high", needles: []string{"core dumped"}},
	{kind: "port-in-use", severity: "medium", needles: []string{"Address already in use", "address already in use"}},
	{kind: "permission", severity: "medium", needles: []string{"Permission denied", "Access denied", "Operation not permitted"}},
	{kind: "not-found", severity: "medium", needles: []string{"command not found", "No such file or directory"}},
	{kind: "connection", severity: "medium", needles: []string{"Connection refused", "Connection timed out", "Connection reset by peer", "Could not resolve hostname", "Name or service not known"}},
	{kind: "denied", severity: "medium", needles: []string{"authentication failure", "Authentication failed", "Invalid password", "Permission to "}},
	{kind: "timeout", severity: "low", needles: []string{"timed out", "Timeout"}},
	{kind: "error", severity: "low", needles: []string{"ERROR", "Error:"}},
}

// maxExcerpt bounds the text handed to the UI (and later to the LLM).
const maxExcerpt = 400

func (s *sniffer) scanErrors(text string, push func(interface{})) {
	if text == "" {
		return
	}
	for _, p := range errorPatterns {
		line, needle, ok := firstMatchingLine(text, p.needles)
		if !ok {
			continue
		}
		sig := p.kind + "\x00" + needle
		if !s.allow(sig) {
			// A more specific pattern may still be worth reporting; keep looking
			// only when the signature differs.
			continue
		}
		push(terminalErrorMsg{
			Type:      "terminal.error",
			SessionID: s.sessionID,
			Kind:      p.kind,
			Severity:  p.severity,
			Excerpt:   truncate(line, maxExcerpt),
		})
		return
	}
}

// allow applies the suppression window. Callers must hold s.mu.
func (s *sniffer) allow(sig string) bool {
	now := time.Now()
	if sig == s.lastSig && now.Sub(s.lastPush) < errorSuppressWindow {
		return false
	}
	if now.Sub(s.lastPush) < errorFloor {
		return false
	}
	s.lastSig = sig
	s.lastPush = now
	return true
}

// firstMatchingLine returns the first line containing one of the needles, along
// with the needle that matched.
func firstMatchingLine(text string, needles []string) (line, needle string, ok bool) {
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		for _, n := range needles {
			if strings.Contains(l, n) {
				return strings.TrimSpace(l), n, true
			}
		}
	}
	return "", "", false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

var (
	reCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	reOSC = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	reEsc = regexp.MustCompile(`\x1b[@-Z\\-_]`)
)

// stripANSI removes escape sequences so matching happens against the text a
// human sees. Carriage returns become newlines (progress bars redraw with \r),
// and a trailing partial line is kept as-is.
func stripANSI(s string) string {
	s = reOSC.ReplaceAllString(s, "")
	s = reCSI.ReplaceAllString(s, "")
	s = reEsc.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	// Drop remaining control characters except tab/newline.
	if strings.IndexFunc(s, func(r rune) bool {
		return r < 0x20 && r != '\n' && r != '\t'
	}) >= 0 {
		var b strings.Builder
		b.Grow(len(s))
		for _, r := range s {
			if r == '\n' || r == '\t' || r >= 0x20 {
				b.WriteRune(r)
			}
		}
		s = b.String()
	}
	return s
}

// terminalModeMsg reports an alternate-screen transition.
type terminalModeMsg struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	AltScreen bool   `json:"altScreen"`
}

// terminalErrorMsg reports an error feature found in the output.
type terminalErrorMsg struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	Kind      string `json:"kind"`
	Severity  string `json:"severity"`
	Excerpt   string `json:"excerpt"`
}
