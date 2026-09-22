// Package sanitize implements the local data-sanitization gateway.
//
// Terminal excerpts and user prompts are masked here before they are sent to an
// LLM, so secrets and addresses never leave the machine in the clear. Everything
// is one pass of pre-compiled regexps over an in-memory string: no network, no
// allocation of a second copy of the config, nothing persisted.
//
// See PLAN.md (milestone M2).
package sanitize

import (
	"regexp"
	"strings"
)

// Placeholders substituted for redacted content.
const (
	IPMask    = "[IP_MASKED]"
	Sensitive = "[SENSITIVE_DATA]"
)

// Mask redacts IP addresses and credentials from s.
//
// The rules are deliberately conservative: a timestamp, a version number or a
// C++ symbol such as `std::array` must survive untouched, because over-masking
// makes the diagnostics the LLM receives useless.
func Mask(s string) string {
	if s == "" {
		return s
	}
	// Order matters. Structured secrets first (a private key block may contain
	// anything, including look-alike IPs), addresses last.
	for _, r := range rules {
		s = applyRule(s, r)
	}
	return s
}

// MaskBytes is Mask for byte slices.
func MaskBytes(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	return []byte(Mask(string(b)))
}

// rule is one masking step: a compiled pattern, a replacement (with $1-style
// group references) and an optional validator.
type rule struct {
	re   *regexp.Regexp
	repl string
	// valid optionally rejects a candidate match. before/after hold the single
	// byte preceding/following the match ("" at the string edges).
	valid func(m, before, after string) bool
}

func applyRule(s string, r rule) string {
	locs := r.re.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	replaced := false
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if start < last {
			continue
		}
		m := s[start:end]
		before, after := "", ""
		if start > 0 {
			before = s[start-1 : start]
		}
		if end < len(s) {
			after = s[end : end+1]
		}
		if r.valid != nil && !r.valid(m, before, after) {
			continue
		}
		out := m
		if r.repl != m {
			out = r.re.ReplaceAllString(m, r.repl)
		}
		if out == m {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(out)
		last = end
		replaced = true
	}
	if !replaced {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// isTokenByte reports whether c can be part of a hostname, path or identifier,
// i.e. a character that means an address match is really a substring of
// something else (`1.2.3.4.5`, `nginx/1.18.0`, `/var/log/1.2.3.4.log`).
func isTokenByte(c byte) bool {
	switch {
	case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return true
	case c == '.', c == '_', c == '-':
		return true
	}
	return false
}

// noTokenBoundary accepts a match only when it is not glued into a longer token.
func noTokenBoundary(_ string, before, after string) bool {
	return (before == "" || !isTokenByte(before[0])) && (after == "" || !isTokenByte(after[0]))
}

var (
	// Private key material: matched as a whole so its body never reaches the
	// LLM. Non-greedy so it stops at the first END marker.
	rePrivateKey = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`)

	// user:password@host inside a connection string / URL.
	reConnString = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://[^/\s:@]+):([^@\s/]+)@`)

	// HTTP/SSH style authorization headers.
	reAuthHeader = regexp.MustCompile(`(?i)((?:authorization|proxy-authorization)\s*:\s*(?:bearer|basic|token)\s+)(\S+)`)

	// key=value / key: value, with optional quotes. A quoted value is consumed
	// as a whole so nothing of it leaks past the first space.
	reKeyValue = regexp.MustCompile(`(?i)([A-Za-z0-9_.\-]*(?:password|passwd|passphrase|pwd|secret|token|api[_-]?key|apikey|access[_-]?key|private[_-]?key|client[_-]?secret|credential)s?)\s*([:=])\s*("[^"]*"|'[^']*'|[^\s"';&|,]+)`)

	// long flags whose value follows a space: --password hunter2
	reFlagValue = regexp.MustCompile(`(?i)(--?(?:password|passwd|passphrase|token|api-?key|secret|client-secret)(?:=|\s+))("[^"]*"|'[^']*'|\S+)`)

	// JSON Web Tokens.
	reJWT = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{4,}`)

	// AWS access key IDs.
	reAWSKey = regexp.MustCompile(`(?:AKIA|ASIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASCA)[0-9A-Z]{16}`)

	// IPv4: four decimal octets. Value and boundary are validated below.
	reIPv4 = regexp.MustCompile(`\d{1,3}(?:\.\d{1,3}){3}`)

	// IPv6: either the full eight-group form, `::`-compressed forms, or a
	// leading `::`. Timestamps (18:00:12) have no `::` and fewer than eight
	// groups, so they never match.
	reIPv6 = regexp.MustCompile(
		`(?:[0-9A-Fa-f]{1,4}:){7}[0-9A-Fa-f]{1,4}` +
			`|::[0-9A-Fa-f]{1,4}(?::[0-9A-Fa-f]{1,4}){0,6}` +
			`|(?:[0-9A-Fa-f]{1,4}:){1,7}(?::[0-9A-Fa-f]{1,4}){1,7}`)
)

// rules are applied in order; see Mask.
var rules = []rule{
	{re: rePrivateKey, repl: Sensitive},
	{re: reConnString, repl: `$1:` + Sensitive + `@`},
	{re: reAuthHeader, repl: `$1` + Sensitive},
	{re: reKeyValue, repl: `$1$2` + Sensitive},
	{re: reFlagValue, repl: `$1` + Sensitive},
	{re: reJWT, repl: Sensitive},
	{re: reAWSKey, repl: Sensitive, valid: noTokenBoundary},
	{re: reIPv4, repl: IPMask, valid: validIPv4},
	{re: reIPv6, repl: IPMask, valid: noTokenBoundary},
}

// validIPv4 accepts four octets in range that are not part of a longer token.
func validIPv4(m, before, after string) bool {
	if !noTokenBoundary(m, before, after) {
		return false
	}
	octets := strings.Split(m, ".")
	if len(octets) != 4 {
		return false
	}
	for _, o := range octets {
		if len(o) > 3 {
			return false
		}
		n := 0
		for i := 0; i < len(o); i++ {
			n = n*10 + int(o[i]-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}
