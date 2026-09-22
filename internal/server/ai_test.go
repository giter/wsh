package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sshclient/internal/ai"
	sshclient "sshclient/ssh"
	"sshclient/storage"
)

// fakeProvider replays canned deltas, so the streaming pipeline can be tested
// without a network or a model.
type fakeProvider struct {
	chunks []string
	err    error
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Stream(ctx context.Context, msgs []ai.Message) (<-chan string, error) {
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan string, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func TestHandleAIStatusDefaultsToUnconfigured(t *testing.T) {
	srv := newTestServer(nil)
	out, err := srv.handleAIStatus(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]interface{})
	if m["configured"] != false {
		t.Fatalf("a fresh install must report AI as unconfigured: %+v", m)
	}
}

// TestStreamAIClassifiesModelOutput is the important one: whatever the model
// claims about risk, the verdict comes from the local engine.
func TestStreamAIClassifiesModelOutput(t *testing.T) {
	srv := newTestServer(nil)
	reply := "[意图分析] 用户想清空磁盘\n" +
		"[策略选择] 直接删除根目录\n" +
		"[安全判定] 只读查询，风险等级：绿区\n" + // the model lies about the risk
		"[生成指令] rm -rf /\n"
	p := &fakeProvider{chunks: []string{reply[:20], reply[20:]}}

	var msgs []interface{}
	srv.streamAI(context.Background(), func(v interface{}) { msgs = append(msgs, v) }, p, "req1", "sys", "user", "command")

	if len(msgs) != 3 {
		t.Fatalf("expected 2 deltas + 1 done, got %d: %+v", len(msgs), msgs)
	}
	if d, ok := msgs[0].(aiDeltaMsg); !ok || d.RequestID != "req1" {
		t.Fatalf("unexpected first message: %+v", msgs[0])
	}
	done, ok := msgs[len(msgs)-1].(aiDoneMsg)
	if !ok {
		t.Fatalf("last message should be ai.done, got %+v", msgs[len(msgs)-1])
	}
	if done.Command != "rm -rf /" {
		t.Fatalf("generated command = %q", done.Command)
	}
	if !done.Risk.Blocked() {
		t.Fatalf("the engine must classify the proposed command as blocked locally, got %+v", done.Risk)
	}
	if len(done.Steps) != 4 {
		t.Fatalf("expected the reasoning steps to be parsed, got %+v", done.Steps)
	}
}

func TestStreamAIReportsProviderErrors(t *testing.T) {
	srv := newTestServer(nil)
	p := &fakeProvider{err: errors.New("dial tcp: connection refused")}

	var msgs []interface{}
	srv.streamAI(context.Background(), func(v interface{}) { msgs = append(msgs, v) }, p, "req2", "sys", "user", "command")
	if len(msgs) != 1 {
		t.Fatalf("expected a single done message, got %+v", msgs)
	}
	done := msgs[0].(aiDoneMsg)
	if done.OK || done.Error == "" {
		t.Fatalf("expected an error result, got %+v", done)
	}
}

// TestStreamAIDiagnoseHasNoCommand checks the diagnosis path does not invent a
// command field.
func TestStreamAIDiagnoseHasNoCommand(t *testing.T) {
	srv := newTestServer(nil)
	p := &fakeProvider{chunks: []string{"[根因] nginx 配置缺少分号\n[影响] 服务无法启动\n[修复] 补全分号后 reload"}}

	var msgs []interface{}
	srv.streamAI(context.Background(), func(v interface{}) { msgs = append(msgs, v) }, p, "req3", "sys", "user", "diagnose")
	done := msgs[len(msgs)-1].(aiDoneMsg)
	if done.Command != "" {
		t.Fatalf("diagnose must not yield a command, got %q", done.Command)
	}
	if len(done.Steps) != 3 {
		t.Fatalf("steps = %+v", done.Steps)
	}
}

// TestBuildUserPromptMasksContext guarantees terminal text is sanitized before it
// can leave the machine, and that the user's own prompt is masked too.
func TestBuildUserPromptMasksContext(t *testing.T) {
	srv := newTestServer(nil)

	got := srv.buildUserPrompt("command", aiAskParams{
		Prompt: "找出占用 8080 端口的进程",
		// Simulate the sniffer-supplied context via Excerpt (the same code path).
		Excerpt: "ssh root@192.168.1.9 with password=hunter2 failed",
	})
	if strings.Contains(got, "192.168.1.9") || strings.Contains(got, "hunter2") {
		t.Fatalf("prompt leaked secrets: %q", got)
	}
	if !strings.Contains(got, "[IP_MASKED]") || !strings.Contains(got, "[SENSITIVE_DATA]") {
		t.Fatalf("expected masked placeholders in %q", got)
	}
	if !strings.Contains(got, "找出占用 8080 端口的进程") {
		t.Fatalf("the user's own request should be preserved: %q", got)
	}
}

// TestBuildUserPromptDiagnoseShape documents the diagnose layout.
func TestBuildUserPromptDiagnoseShape(t *testing.T) {
	srv := newTestServer(nil)
	got := srv.buildUserPrompt("diagnose", aiAskParams{Excerpt: "nginx: [emerg] unknown directive"})
	if !strings.HasPrefix(got, "终端最近输出") {
		t.Fatalf("unexpected diagnose prompt: %q", got)
	}
}

// TestBuildUserPromptResultShape documents the result-analysis layout: the command
// that ran, then its captured output.
func TestBuildUserPromptResultShape(t *testing.T) {
	srv := newTestServer(nil)
	got := srv.buildUserPrompt("result", aiAskParams{
		Prompt:  "ps aux | grep wglink",
		Excerpt: "root 292 wglink --serve",
	})
	if !strings.Contains(got, "已执行命令：ps aux | grep wglink") {
		t.Fatalf("the command should be part of the prompt: %q", got)
	}
	if !strings.Contains(got, "root 292 wglink --serve") {
		t.Fatalf("the captured output should follow the header: %q", got)
	}
}

// TestBuildUserPromptResultWithContextOff covers the privacy setting: the output is
// withheld, and the prompt says so instead of leaving a dangling header.
func TestBuildUserPromptResultWithContextOff(t *testing.T) {
	store := storage.NewMemoryStore()
	if err := store.UpdateSettings(storage.Settings{Theme: "dark", AINoContext: true}); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(store, sshclient.NewPool(nil), sshclient.NewTunnelManager(sshclient.NewPool(nil)), nil)

	got := srv.buildUserPrompt("result", aiAskParams{
		Prompt:  "ps aux | grep wglink",
		Excerpt: "root 292 wglink --serve",
	})
	if strings.Contains(got, "wglink --serve") {
		t.Fatalf("context sharing is off, the output must not be sent: %q", got)
	}
	if !strings.Contains(got, "已关闭上下文共享") {
		t.Fatalf("the prompt should explain the missing output: %q", got)
	}
}

// TestBuildUserPromptResultWarnsAboutCredentialPrompt: a segment that ended at a
// password prompt is not a result. Unless the prompt says so, the model reads
// `[sudo] password for lee:` as the command's output and reports a confident
// success for a command that never ran.
func TestBuildUserPromptResultWarnsAboutCredentialPrompt(t *testing.T) {
	srv := newTestServer(nil)

	got := srv.buildUserPrompt("result", aiAskParams{
		Prompt:  "sudo systemctl restart wglink",
		Excerpt: "sudo systemctl restart wglink\n[sudo] password for lee: ",
	})
	if !strings.Contains(got, "等待输入") {
		t.Fatalf("the prompt must say the command is waiting for input: %q", got)
	}

	// A command that really finished gets no such warning.
	normal := srv.buildUserPrompt("result", aiAskParams{
		Prompt:  "uptime",
		Excerpt: " 10:53 up 3 days,  1 user,  load average: 0.12",
	})
	if strings.Contains(normal, "等待输入") {
		t.Fatalf("a completed command must not carry the warning: %q", normal)
	}
}

func TestHandleAIAskRequiresProviderAndPrompt(t *testing.T) {
	srv := newTestServer(nil)
	if _, err := srv.handleAIAsk(nil, []byte(`{"prompt":"ls"}`)); err == nil {
		t.Fatal("without a configured provider the call must fail loudly")
	}
}

// TestStreamAICancelIsReported covers the Esc key: the parent context is killed,
// the stream stops, and the card is told the reply was abandoned rather than
// receiving a half answer as if it were complete.
func TestStreamAICancelIsReported(t *testing.T) {
	srv := newTestServer(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already abandoned before the first delta

	p := &fakeProvider{chunks: []string{"[结论] 半句话"}}
	var msgs []interface{}
	srv.streamAI(ctx, func(v interface{}) { msgs = append(msgs, v) }, p, "req4", "sys", "user", "result")

	if len(msgs) == 0 {
		t.Fatal("a cancelled request must still close the card")
	}
	done := msgs[len(msgs)-1].(aiDoneMsg)
	if !done.Cancelled || done.OK {
		t.Fatalf("expected a cancelled result, got %+v", done)
	}
}

// TestHandleAICancelReachesTheRegistry checks the RPC side: the stored cancel func
// is called once and consumed, so Esc cannot be replayed against a finished turn.
func TestHandleAICancelReachesTheRegistry(t *testing.T) {
	srv := newTestServer(nil)
	ctx, cancel := context.WithCancel(context.Background())
	srv.aiCancels["req-x"] = cancel

	if _, err := srv.handleAICancel(nil, []byte(`{"requestId":"req-x"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("the cancel func must reach the in-flight request")
	}
	srv.aiMu.Lock()
	_, still := srv.aiCancels["req-x"]
	srv.aiMu.Unlock()
	if still {
		t.Fatal("the request should be dropped from the registry")
	}
}
