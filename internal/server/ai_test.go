package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sshclient/internal/ai"
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
	srv.streamAI(func(v interface{}) { msgs = append(msgs, v) }, p, "req1", "sys", "user", "command")

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
	srv.streamAI(func(v interface{}) { msgs = append(msgs, v) }, p, "req2", "sys", "user", "command")
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
	srv.streamAI(func(v interface{}) { msgs = append(msgs, v) }, p, "req3", "sys", "user", "diagnose")
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

func TestHandleAIAskRequiresProviderAndPrompt(t *testing.T) {
	srv := newTestServer(nil)
	if _, err := srv.handleAIAsk(nil, []byte(`{"prompt":"ls"}`)); err == nil {
		t.Fatal("without a configured provider the call must fail loudly")
	}
}
