package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewValidatesConfig(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("empty config should report ErrNotConfigured, got %v", err)
	}
	if _, err := New(Config{Kind: KindOpenAI}); err == nil {
		t.Fatal("a provider without a model should be rejected")
	}
	if _, err := New(Config{Kind: "bedrock", Model: "x"}); err == nil {
		t.Fatal("an unknown provider should be rejected")
	}

	p, err := New(Config{Kind: KindOpenAI, Model: "gpt-4o-mini"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.(*openAIProvider).baseURL; got != "https://api.openai.com/v1" {
		t.Fatalf("default base URL = %q", got)
	}

	p, err = New(Config{Kind: KindOllama, Model: "qwen2.5:7b"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.(*ollamaProvider).baseURL; got != "http://127.0.0.1:11434" {
		t.Fatalf("default ollama URL = %q", got)
	}
}

func TestParseOpenAILine(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		delta string
		done  bool
	}{
		{"delta", `data: {"choices":[{"delta":{"content":"你好"}}]}`, "你好", false},
		{"text fallback", `data: {"choices":[{"text":"hi"}]}`, "hi", false},
		{"done marker", `data: [DONE]`, complete, true},
		{"comment", `: keep-alive`, "", false},
		{"blank", ``, "", false},
		{"malformed", `data: {oops`, "", false},
		{"error field", `data: {"error":{"message":"rate limited"}}`, "[AI 错误] rate limited", true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			delta, done := parseOpenAILine([]byte(tt.line))
			if delta != tt.delta || done != tt.done {
				t.Fatalf("got (%q, %v), want (%q, %v)", delta, done, tt.delta, tt.done)
			}
		})
	}
}

func TestParseOllamaLine(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		delta string
		done  bool
	}{
		{"delta", `{"message":{"content":"部分"},"done":false}`, "部分", false},
		{"final with text", `{"message":{"content":"末"},"done":true}`, "末", true},
		{"final empty", `{"done":true}`, complete, true},
		{"blank", ``, "", false},
		{"garbage", `not json`, "", false},
		{"error", `{"error":"model not found"}`, "[AI 错误] model not found", true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			delta, done := parseOllamaLine([]byte(tt.line))
			if delta != tt.delta || done != tt.done {
				t.Fatalf("got (%q, %v), want (%q, %v)", delta, done, tt.delta, tt.done)
			}
		})
	}
}

// TestOpenAIStream drives the provider against a fake streaming endpoint and
// checks both the request it sends and the deltas it yields.
func TestOpenAIStream(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = jsonDecode(r.Body, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p, err := New(Config{Kind: KindOpenAI, BaseURL: srv.URL, Model: "test-model", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := p.Stream(ctx, []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for delta := range ch {
		sb.WriteString(delta)
	}
	if sb.String() != "Hello" {
		t.Fatalf("streamed %q, want %q", sb.String(), "Hello")
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotBody["model"] != "test-model" || gotBody["stream"] != true {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
}

// TestOllamaStream covers the fully offline provider.
func TestOllamaStream(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"message":{"content":"离"},"done":false}`+"\n")
		fmt.Fprint(w, `{"message":{"content":"线"},"done":false}`+"\n")
		fmt.Fprint(w, `{"done":true}`+"\n")
	}))
	defer srv.Close()

	p, err := New(Config{Kind: KindOllama, BaseURL: srv.URL, Model: "qwen2.5:7b"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := p.Stream(ctx, []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for delta := range ch {
		sb.WriteString(delta)
	}
	if sb.String() != "离线" {
		t.Fatalf("streamed %q", sb.String())
	}
	if gotPath != "/api/chat" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestStreamReportsAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()

	p, err := New(Config{Kind: KindOpenAI, BaseURL: srv.URL, Model: "m", APIKey: "bad"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected an error for a 401 response")
	}
	if !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("error should hint at the API key: %v", err)
	}
}

func TestStreamRespectsContextCancellation(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-release // hold the stream open
	}))
	defer srv.Close()
	defer close(release)

	p, _ := New(Config{Kind: KindOpenAI, BaseURL: srv.URL, Model: "m"})
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.Stream(ctx, []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if delta, ok := <-ch; !ok || delta != "a" {
		t.Fatalf("expected the first delta, got %q ok=%v", delta, ok)
	}
	cancel()
	// The channel must close promptly instead of blocking forever.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected the stream to be closed after cancellation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not stop after the context was cancelled")
	}
}

func jsonDecode(r io.Reader, v any) error {
	buf, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf, v)
}
