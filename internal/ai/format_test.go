package ai

import "testing"

func TestParseSteps(t *testing.T) {
	reply := `[意图分析] 用户需要查找占用 8080 端口的进程
[策略选择] 优先使用 lsof，不存在则降级至 ss/netstat
[安全判定] 只读查询，风险等级：绿区
[生成指令] lsof -i :8080
这是不该被解析的一段说明
[未知] 也不该被解析`

	steps := ParseSteps(reply)
	if len(steps) != 4 {
		t.Fatalf("expected 4 steps, got %d: %+v", len(steps), steps)
	}
	if steps[0].Label != "意图分析" || steps[3].Detail != "lsof -i :8080" {
		t.Fatalf("unexpected parse: %+v", steps)
	}
}

func TestGeneratedCommand(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "explicit step",
			in:   "[意图分析] 查端口\n[生成指令] lsof -i :8080",
			want: "lsof -i :8080",
		},
		{
			name: "backticked",
			in:   "[生成指令] `ss -lntp | grep 8080`",
			want: "ss -lntp | grep 8080",
		},
		{
			name: "dollar prefix",
			in:   "[生成指令] $ systemctl status nginx",
			want: "systemctl status nginx",
		},
		{
			name: "fenced fallback",
			in:   "可以用这条命令：\n```bash\ndf -h\n```",
			want: "df -h",
		},
		{
			name: "empty result",
			in:   "[修复] 需要人工确认\n[命令] 无",
			want: "",
		},
		{
			name: "placeholder rejection",
			in:   "[生成指令] <你的命令>",
			want: "",
		},
		{
			name: "prose rejection",
			in:   "[生成指令] 请先确认服务是否在运行。",
			want: "",
		},
		{
			name: "no command at all",
			in:   "我不确定你的环境，请提供更多信息。",
			want: "",
		},
		{
			name: "redirect is not a placeholder",
			in:   "[生成指令] mysql app < /tmp/dump.sql > /tmp/out.log",
			want: "mysql app < /tmp/dump.sql > /tmp/out.log",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := GeneratedCommand(tt.in); got != tt.want {
				t.Fatalf("GeneratedCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPromptsMentionFormat keeps the prompt and the parser in sync: if the
// prompt stops asking for "[生成指令]", GeneratedCommand silently returns "".
func TestPromptsMentionFormat(t *testing.T) {
	if !contains(SystemPromptCommand(), "[生成指令]") {
		t.Fatal("the command prompt must ask for the [生成指令] step the parser expects")
	}
	if !contains(SystemPromptDiagnose(), "[根因]") {
		t.Fatal("the diagnose prompt must ask for the [根因] step the parser expects")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
