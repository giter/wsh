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

	// cap is the command output currently being attributed to a reasoning card
	// (nil when nothing is being tracked). capSeq makes a stale timer harmless:
	// each arming gets a fresh sequence number and timers validate it.
	cap    *execCapture
	capSeq int
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
	s.appendCapture(text, push)
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

// Command output capture -----------------------------------------------------
//
// A command the user ran from a reasoning card is tracked so its output can be
// handed back to the model as the next turn of the same conversation. The tap
// cannot see the shell's own view of the prompt, so the end of a command's output
// is inferred: the stream goes quiet *and* its tail looks like a shell prompt.
// Both conditions are required, and the result is only used as advisory context,
// so a mis-detection costs a truncated analysis, never a lost command.
const (
	// captureQuiet is how long the output must be silent before the segment is
	// considered finished.
	captureQuiet = 400 * time.Millisecond
	// captureQuietGrace is the extra silence allowed when the quiet period
	// elapsed but no prompt is visible yet (a command that prints without a
	// trailing newline).
	captureQuietGrace = 1500 * time.Millisecond
	// captureTimeout is the hard limit for one command, so a streaming command
	// (`tail -f`, `journalctl -f`) still hands over what it printed so far.
	captureTimeout = 30 * time.Second
	// captureLimit bounds one segment, keeping a runaway log out of the model.
	captureLimit = 32 << 10
)

// captureStreaming is how long a command may keep producing output before it is
// called out as a long-running stream. A `ping` without `-c` would otherwise sit
// silently until the 30s timeout; this tells the UI earlier, so it can offer to
// stop the command instead of leaving a bare spinner. It is a var (not a const)
// purely so tests can shorten it.
var captureStreaming = 3 * time.Second

// execCapture accumulates the output of one tracked command.
type execCapture struct {
	seq     int
	trackID string
	command string
	start   time.Time
	// lastData is when output last arrived; completion waits for silence after it.
	lastData time.Time
	text     []byte
	// grace records that the prompt-less grace period was already granted.
	grace    bool
	trunc    bool
	finished bool
	// streaming records that the long-running notice was already sent, so it is
	// pushed once per command and not on every chunk.
	streaming bool
	timer     *time.Timer
	hard      *time.Timer
	stream    *time.Timer
}

// armCapture starts recording output for the card identified by trackID. A capture
// still in flight is flushed first, so two commands never share one segment.
func (s *sniffer) armCapture(trackID, command string, push func(interface{})) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.finishCaptureLocked(push, false)

	now := time.Now()
	s.capSeq++
	c := &execCapture{
		seq:      s.capSeq,
		trackID:  trackID,
		command:  command,
		start:    now,
		lastData: now,
	}
	s.cap = c
	c.timer = time.AfterFunc(captureQuiet, func() { s.captureIdle(c.seq, push) })
	c.hard = time.AfterFunc(captureTimeout, func() { s.captureOverran(c.seq, push) })
	c.stream = time.AfterFunc(captureStreaming, func() { s.captureStillRunning(c.seq, push) })
}

// finishCapture flushes the active capture, if any.
func (s *sniffer) finishCapture(push func(interface{})) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishCaptureLocked(push, false)
}

// stop drops the capture timers when the session ends. Nothing is pushed: the
// browser socket is already gone.
func (s *sniffer) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishCaptureLocked(nil, false)
}

// appendCapture feeds one chunk of visible text to the active capture. Callers
// must hold s.mu.
func (s *sniffer) appendCapture(text string, push func(interface{})) {
	c := s.cap
	if c == nil || c.finished {
		return
	}
	if s.altScreen {
		// A full-screen application owns the screen now: there is no
		// line-oriented output left to attribute to the command.
		s.finishCaptureLocked(push, false)
		return
	}
	now := time.Now()
	c.lastData = now
	if text == "" {
		return
	}
	c.text = append(c.text, text...)
	if len(c.text) >= captureLimit {
		c.text = c.text[:captureLimit]
		c.trunc = true
		s.finishCaptureLocked(push, false)
	}
}

// captureIdle runs when the output has been silent for a while. It only finishes
// the segment when the tail looks like a shell prompt, giving a command that is
// slow to print its prompt one extra grace period.
func (s *sniffer) captureIdle(seq int, push func(interface{})) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := s.cap
	if c == nil || c.seq != seq || c.finished {
		return
	}
	need := captureQuiet
	if c.grace {
		need = captureQuietGrace
	}
	if quiet := time.Since(c.lastData); quiet < need {
		// Output arrived after the timer was armed; wait out the remainder.
		c.timer = time.AfterFunc(need-quiet, func() { s.captureIdle(seq, push) })
		return
	}
	if !c.grace && !looksLikePrompt(string(c.text)) {
		c.grace = true
		c.timer = time.AfterFunc(captureQuietGrace, func() { s.captureIdle(seq, push) })
		return
	}
	s.finishCaptureLocked(push, false)
}

// captureStillRunning fires when a tracked command has been producing output for
// longer than captureStreaming without going quiet. It reports the stream so the
// card can offer "stop listening (Ctrl+C)" instead of showing a spinner that
// looks stuck; the capture itself stays open and keeps accumulating.
func (s *sniffer) captureStillRunning(seq int, push func(interface{})) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cap
	if c == nil || c.seq != seq || c.finished || c.streaming {
		return
	}
	c.streaming = true
	push(terminalStreamingMsg{
		Type:      "terminal.streaming",
		SessionID: s.sessionID,
		TrackID:   c.trackID,
		Command:   c.command,
		ElapsedMs: time.Since(c.start).Milliseconds(),
	})
}

// captureOverran ends a segment that never went quiet.
func (s *sniffer) captureOverran(seq int, push func(interface{})) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cap
	if c == nil || c.seq != seq || c.finished {
		return
	}
	s.finishCaptureLocked(push, true)
}

// finishCaptureLocked emits the active capture and clears it. Callers must hold
// s.mu. A nil push (session teardown) drops the segment silently.
func (s *sniffer) finishCaptureLocked(push func(interface{}), timedOut bool) {
	c := s.cap
	if c == nil {
		return
	}
	s.cap = nil
	if c.timer != nil {
		c.timer.Stop()
	}
	if c.hard != nil {
		c.hard.Stop()
	}
	if c.stream != nil {
		c.stream.Stop()
	}
	if c.finished || push == nil {
		return
	}
	c.finished = true
	push(terminalOutputMsg{
		Type:       "terminal.output",
		SessionID:  s.sessionID,
		TrackID:    c.trackID,
		Command:    c.command,
		Text:       string(c.text),
		DurationMs: time.Since(c.start).Milliseconds(),
		Truncated:  c.trunc,
		TimedOut:   timedOut,
	})
}

// promptEnds are the characters a shell prompt ends with (bash/zsh `$` and `#`,
// csh `%`, and the `❯`/`➜` used by prompt themes).
const promptEnds = "$#%>❯➜λ"

// maxPromptLen bounds the line that may be mistaken for a prompt: real prompts are
// short, while output that happens to end in "$" is usually part of a longer line.
const maxPromptLen = 96

// looksLikePrompt reports whether the captured output ends with a shell prompt.
func looksLikePrompt(text string) bool {
	line := strings.TrimRight(lastLine(text), " \t")
	if line == "" {
		return false
	}
	r := []rune(line)
	if len(r) > maxPromptLen {
		return false
	}
	return strings.ContainsRune(promptEnds, r[len(r)-1])
}

// lastLine returns the last non-empty line of s.
func lastLine(s string) string {
	s = strings.TrimRight(s, "\n")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
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

// terminalOutputMsg carries the captured output of one tracked command back to
// the card that proposed it (TrackID is that card's id). Truncated and TimedOut
// tell the UI the segment is incomplete, so it can label the analysis honestly.
type terminalOutputMsg struct {
	Type       string `json:"type"`
	SessionID  string `json:"sessionId"`
	TrackID    string `json:"trackId"`
	Command    string `json:"command"`
	Text       string `json:"text"`
	DurationMs int64  `json:"durationMs"`
	Truncated  bool   `json:"truncated"`
	TimedOut   bool   `json:"timedOut"`
}

// terminalStreamingMsg announces that a tracked command is still producing output
// well after it started (a stream: `tail -f`, a bare `ping`, `top`). It is
// advisory: the capture keeps running, and the final segment still arrives as a
// terminalOutputMsg.
type terminalStreamingMsg struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	TrackID   string `json:"trackId"`
	Command   string `json:"command"`
	ElapsedMs int64  `json:"elapsedMs"`
}
