package ai

import (
	"regexp"
	"strings"
)

// Step is one line of the model's chain of thought, e.g.
//
//	[意图分析] 用户需要查找占用 8080 端口的进程
type Step struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

var reStep = regexp.MustCompile(`^\[([^\[\]]{1,20})\]\s*(.*)$`)

// ParseSteps extracts the bracketed reasoning steps from a model reply. Lines
// that do not follow the convention are ignored, so a model that answers in
// prose simply yields fewer steps.
func ParseSteps(text string) []Step {
	var out []Step
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := reStep.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		label := strings.TrimSpace(m[1])
		if label == "" || !isKnownLabel(label) {
			continue
		}
		out = append(out, Step{Label: label, Detail: strings.TrimSpace(m[2])})
	}
	return out
}

// knownLabels keeps prose such as "[注意]" from being rendered as a reasoning
// step; the prompt asks for these exact labels.
var knownLabels = map[string]bool{
	"意图分析": true, "策略选择": true, "安全判定": true, "生成指令": true,
	"根因": true, "影响": true, "修复": true, "命令": true, "说明": true, "风险": true,
}

func isKnownLabel(s string) bool { return knownLabels[s] }

var (
	reFence    = regexp.MustCompile("(?s)```[a-zA-Z]*\\s*\\n(.*?)```")
	reBacktick = regexp.MustCompile("^`([^`]*)`$")
	// rePlaceholder catches stand-ins such as <command> / <你的命令> while
	// leaving real redirections (`mysql < dump.sql`) alone.
	rePlaceholder = regexp.MustCompile(`<[A-Za-z_\p{Han}][A-Za-z0-9_ \p{Han}]{0,30}>`)
)

// GeneratedCommand pulls the single shell command out of a model reply. It only
// ever returns a command that the caller must still run through the safety
// engine: the model has no say in whether something is executed.
func GeneratedCommand(text string) string {
	// Preferred: the explicit "[生成指令]" step.
	for _, s := range ParseSteps(text) {
		if s.Label == "生成指令" || s.Label == "命令" {
			if cmd := cleanCommand(s.Detail); cmd != "" {
				return cmd
			}
		}
	}
	// Fallback: a fenced code block.
	if m := reFence.FindStringSubmatch(text); m != nil {
		for _, line := range strings.Split(m[1], "\n") {
			if cmd := cleanCommand(line); cmd != "" {
				return cmd
			}
		}
	}
	return ""
}

// cleanCommand normalises one candidate line into a bare command, or "" when the
// line clearly is not one (prose, a placeholder, an empty result).
func cleanCommand(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$ ")
	s = strings.TrimPrefix(s, "# ")
	s = strings.TrimSpace(s)
	if m := reBacktick.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}
	s = strings.Trim(s, "`")
	s = strings.TrimSpace(s)
	if s == "" || rePlaceholder.MatchString(s) {
		return ""
	}
	// A sentence that ends in Chinese punctuation is prose, not a command.
	if strings.HasSuffix(s, "。") || strings.HasSuffix(s, "，") || strings.HasSuffix(s, "：") {
		return ""
	}
	switch strings.ToLower(s) {
	case "无", "none", "n/a", "null":
		return ""
	}
	return s
}

// SystemPromptCommand asks for the reasoning-step format the reasoning pane
// renders, ending in exactly one command.
func SystemPromptCommand() string {
	return strings.Join([]string{
		"你是一名资深 Linux/Unix 运维专家，运行在一个 SSH 终端工具里。",
		"用户会用自然语言描述意图，你需要输出一条可直接在远端 shell 执行的命令。",
		"严格按以下格式输出，每行一个步骤，不要输出多余内容：",
		"[意图分析] <一句话说明用户想要什么>",
		"[策略选择] <说明为什么选这条命令，必要时给出降级方案>",
		"[安全判定] <只读查询/系统变更>，风险等级：绿区|黄区|红区",
		"[生成指令] <一条完整的 shell 命令>",
		"要求：",
		"1. [生成指令] 只能是一行命令，不要加 markdown 代码围栏。",
		"2. 优先使用只读命令；需要变更系统时选择影响最小的写法。",
		"3. 不要输出解释性段落，不要询问确认。",
	}, "\n")
}

// SystemPromptDiagnose asks for a root-cause analysis of a terminal error.
func SystemPromptDiagnose() string {
	return strings.Join([]string{
		"你是一名资深 Linux/Unix 运维专家，正在分析 SSH 终端里的报错输出。",
		"严格按以下格式输出，每行一个步骤，不要输出多余内容：",
		"[根因] <最可能的故障原因，尽量具体到配置文件或服务>",
		"[影响] <该故障的影响范围>",
		"[修复] <建议的修复命令或操作，一句话>",
		"[命令] <一条可直接执行的修复命令，不确定时留空>",
		"注意：输出中不要复述敏感信息（IP、密码、密钥）。",
	}, "\n")
}
