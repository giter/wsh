// Package safety implements a deterministic, fully local safety engine for
// shell commands.
//
// A command is parsed into an AST (mvdan.cc/sh/v3/syntax) and classified into
// three risk levels. The verdict never depends on a remote LLM and — unlike a
// regex blocklist — it understands nesting, so `sudo bash -c 'rm -rf /'`,
// `echo 1 > /dev/sda` and command substitutions are all resolved properly.
//
// See PLAN.md (milestone M1) for the rule table and the rationale.
package safety

import (
	"path"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Level is the risk classification of a command.
type Level int

const (
	// LevelSafe (绿区) is a read-only or low-risk command: run it right away.
	LevelSafe Level = iota
	// LevelCaution (黄区) may be disruptive: ask for confirmation first (the
	// dry-run panel) before it is sent to the remote shell.
	LevelCaution
	// LevelBlocked (红区) is destructive beyond recovery: never send it.
	LevelBlocked
)

// String returns the stable identifier used in JSON and by the frontend.
func (l Level) String() string {
	switch l {
	case LevelBlocked:
		return "blocked"
	case LevelCaution:
		return "caution"
	default:
		return "safe"
	}
}

// MarshalJSON renders the level as a string so the browser can compare it
// directly (level === "blocked").
func (l Level) MarshalJSON() ([]byte, error) {
	return []byte(`"` + l.String() + `"`), nil
}

// Finding is a single rule hit.
type Finding struct {
	Level   Level  `json:"level"`
	Rule    string `json:"rule"`
	Reason  string `json:"reason"`
	Command string `json:"command,omitempty"`
}

// Result is the verdict for one command line.
type Result struct {
	Level Level `json:"level"`
	// Reason explains the most severe finding, for direct display.
	Reason   string    `json:"reason,omitempty"`
	Findings []Finding `json:"findings"`
	// ParseError is set when the input is not (yet) valid shell syntax, e.g.
	// while the user is still typing. Such input is handed to the shell as-is.
	ParseError string `json:"parseError,omitempty"`
}

// Blocked reports whether the command must not be sent at all.
func (r Result) Blocked() bool { return r.Level == LevelBlocked }

// NeedsConfirm reports whether the user must confirm before it is sent.
func (r Result) NeedsConfirm() bool { return r.Level == LevelCaution }

// Analyze classifies a command line. It is safe for concurrent use.
func Analyze(cmd string) Result {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return Result{Level: LevelSafe}
	}
	a := &analyzer{}
	if len(cmd) > maxCommandLen {
		a.parseErr = "命令过长，已跳过安全分析"
	} else {
		a.analyzeSource(cmd, 0)
	}
	return a.result()
}

// Classify is Analyze reduced to the risk level.
func Classify(cmd string) Level { return Analyze(cmd).Level }

// maxCommandLen bounds the parser input; anything longer is not a command a
// human typed into the Smart Input box and is passed through unchecked.
const maxCommandLen = 64 << 10

// maxDepth bounds the recursion used for nested scripts (bash -c, eval, ...).
const maxDepth = 4

// analyzer accumulates findings while walking the AST.
type analyzer struct {
	findings []Finding
	seen     map[string]bool
	parseErr string
}

func (a *analyzer) result() Result {
	if len(a.findings) == 0 {
		return Result{Level: LevelSafe, ParseError: a.parseErr}
	}
	level := LevelSafe
	for _, f := range a.findings {
		if f.Level > level {
			level = f.Level
		}
	}
	// Reason explains the most severe finding, preferring the first one found.
	reason := ""
	for _, f := range a.findings {
		if f.Level == level {
			reason = f.Reason
			break
		}
	}
	return Result{Level: level, Reason: reason, Findings: a.findings, ParseError: a.parseErr}
}

func (a *analyzer) add(f Finding) {
	key := f.Rule + "\x00" + f.Command
	if a.seen == nil {
		a.seen = make(map[string]bool)
	}
	if a.seen[key] {
		return
	}
	a.seen[key] = true
	a.findings = append(a.findings, f)
}

func (a *analyzer) block(rule, reason, cmd string) {
	a.add(Finding{Level: LevelBlocked, Rule: rule, Reason: reason, Command: cmd})
}

func (a *analyzer) caution(rule, reason, cmd string) {
	a.add(Finding{Level: LevelCaution, Rule: rule, Reason: reason, Command: cmd})
}

// analyzeSource parses one script and walks its AST. The walk descends into
// command substitutions, subshells, pipelines and blocks by itself, so every
// nested command is visited; only text-based nesting (`bash -c '…'`, eval,
// find -exec) needs the explicit re-parse below.
func (a *analyzer) analyzeSource(src string, depth int) {
	if depth > maxDepth {
		return
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(src), "")
	if err != nil {
		if a.parseErr == "" {
			a.parseErr = err.Error()
		}
		return
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			a.checkCall(n, depth)
		case *syntax.Redirect:
			a.checkRedirect(n)
		case *syntax.FuncDecl:
			a.checkFuncDecl(n)
		}
		return true
	})
}

// arg is one word of a call, resolved statically when possible.
type arg struct {
	text string
	// dynamic is true when the word contains expansions (variables, command
	// substitution, ...) whose value is only known at run time.
	dynamic bool
}

func callArgs(call *syntax.CallExpr) []arg {
	out := make([]arg, 0, len(call.Args))
	for _, w := range call.Args {
		text, dynamic := staticWord(w)
		out = append(out, arg{text: text, dynamic: dynamic})
	}
	return out
}

// staticWord resolves a word to its literal text. dynamic reports that the word
// contains a part (variable, substitution, arithmetic, ...) whose value cannot
// be resolved without running the shell.
func staticWord(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", true
	}
	var b strings.Builder
	dynamic := false
	var add func(parts []syntax.WordPart)
	add = func(parts []syntax.WordPart) {
		for _, part := range parts {
			switch p := part.(type) {
			case *syntax.Lit:
				b.WriteString(p.Value)
			case *syntax.SglQuoted:
				b.WriteString(p.Value)
			case *syntax.DblQuoted:
				add(p.Parts)
			default:
				dynamic = true
			}
		}
	}
	add(w.Parts)
	return b.String(), dynamic
}

func (a *analyzer) checkCall(call *syntax.CallExpr, depth int) {
	args := callArgs(call)
	if len(args) == 0 || args[0].dynamic {
		return
	}
	name := path.Base(args[0].text)
	if name == "" || name == "." || name == "/" {
		return
	}
	// Privilege escalation is confirmation-worthy on its own: it changes what the
	// command is allowed to touch, and the tool almost always stops to ask for a
	// password — a prompt an unattended (auto-run) command can never answer. Left
	// unflagged, `sudo systemctl restart x` would be auto-submitted and the
	// terminal would sit at the password prompt while the captured "output" (just
	// the prompt) looked like a silent success.
	switch name {
	case "sudo", "doas", "su", "runuser", "pkexec":
		a.caution("priv.escalate", "风险操作：需要提权（"+name+"），可能要求输入密码，需二次确认", name)
	}
	// Peel wrappers (sudo, env, timeout, ssh, xargs, ...) to reach the command
	// that actually runs.
	inner, rest := resolve(name, args[1:])
	if inner == "" {
		return
	}
	// A shell given a script string (`bash -c '…'`) or eval hides a command in
	// its argument: re-parse that text and analyze it too.
	if a.checkInlineScript(inner, rest, depth) {
		return
	}
	if inner == "find" {
		a.checkFindExec(rest, depth)
	}
	a.applyRules(inner, rest)
}

// checkInlineScript re-parses a command hidden inside a string argument and
// reports whether it consumed the call (so generic rules are skipped).
func (a *analyzer) checkInlineScript(name string, args []arg, depth int) bool {
	switch name {
	case "bash", "sh", "dash", "zsh", "ksh", "ksh93", "ash", "mksh", "posh", "yash",
		"su", "runuser":
		for i, ar := range args {
			if ar.dynamic || !strings.HasPrefix(ar.text, "-") || strings.HasPrefix(ar.text, "--") {
				continue
			}
			// -c, and clusters such as -lc / -ec.
			if !strings.Contains(ar.text[1:], "c") {
				continue
			}
			if i+1 < len(args) && !args[i+1].dynamic {
				a.analyzeSource(args[i+1].text, depth+1)
			}
			return true
		}
	case "eval":
		var parts []string
		for _, ar := range args {
			if ar.dynamic {
				a.caution("eval.dynamic", "风险操作：eval 的内容无法静态确定，需二次确认", name)
				return true
			}
			parts = append(parts, ar.text)
		}
		if len(parts) > 0 {
			a.analyzeSource(strings.Join(parts, " "), depth+1)
		}
		return true
	}
	return false
}

// checkFindExec analyzes the command run by `find … -exec … ;`, which is
// invisible to a walk of the call itself because the call's name is "find".
func (a *analyzer) checkFindExec(args []arg, depth int) {
	for i, ar := range args {
		switch ar.text {
		case "-delete":
			a.caution("find.delete", "风险操作：find -delete 会批量删除匹配的文件", "find")
		case "-exec", "-execdir", "-ok", "-okdir":
			var inner []arg
			for j := i + 1; j < len(args); j++ {
				if args[j].dynamic || args[j].text == ";" || args[j].text == "+" {
					break
				}
				ar := args[j]
				// `{}` stands for each matched path, which is only known once find
				// runs; treat it as dynamic so the target is never assumed harmless.
				if strings.Contains(ar.text, "{}") {
					ar.dynamic = true
				}
				inner = append(inner, ar)
			}
			// A bare `{}` placeholder is not a command name.
			if len(inner) == 0 || inner[0].dynamic {
				continue
			}
			name := path.Base(inner[0].text)
			rest := inner[1:]
			if a.checkInlineScript(name, rest, depth) {
				continue
			}
			a.applyRules(name, rest)
		}
	}
}

// checkRedirect classifies redirections such as `> /dev/sda` or `> /etc/hosts`,
// which write to a file with no command involved at all.
func (a *analyzer) checkRedirect(r *syntax.Redirect) {
	switch r.Op {
	case syntax.RdrOut, syntax.AppOut:
	default:
		return
	}
	target, dynamic := staticWord(r.Word)
	if dynamic {
		return
	}
	switch {
	case isBlockDevice(target):
		a.block("redirect.device", "安全阻断：重定向会覆盖块设备（> /dev/…）", ">")
	case isSystemPath(target):
		a.caution("redirect.system", "风险操作：会覆盖系统目录下的文件（"+target+"）", ">")
	case hasGlob(target):
		a.caution("redirect.glob", "风险操作：通配符批量覆盖文件（"+target+"）", ">")
	}
}

// checkFuncDecl detects fork bombs: a function that calls itself in the
// background (the classic `:(){ :|:& };:`).
func (a *analyzer) checkFuncDecl(fd *syntax.FuncDecl) {
	if fd.Name == nil || fd.Body == nil {
		return
	}
	name := fd.Name.Value
	recursive := false
	syntax.Walk(fd.Body, func(node syntax.Node) bool {
		st, ok := node.(*syntax.Stmt)
		if !ok || !st.Background || st.Cmd == nil {
			return true
		}
		if callsCommand(st.Cmd, name) {
			recursive = true
			return false
		}
		return true
	})
	if recursive || (name == ":" && callsCommand(fd.Body, name)) {
		a.block("forkbomb", "安全阻断：检测到自递归后台调用（fork 炸弹）", name)
	}
}

// callsCommand reports whether the node invokes the given command name.
func callsCommand(node syntax.Node, name string) bool {
	found := false
	syntax.Walk(node, func(n syntax.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		if text, dynamic := staticWord(call.Args[0]); !dynamic && path.Base(text) == name {
			found = true
			return false
		}
		return true
	})
	return found
}
