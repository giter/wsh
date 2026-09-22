// Package ai is a small, dependency-free client for the two streaming APIs that
// matter here: OpenAI-compatible `/chat/completions` (OpenAI, Azure, vLLM,
// LocalAI, one-api, ...) and Ollama's native `/api/chat` for a fully offline
// setup.
//
// Nothing in this package is on the safety path: an unreachable or absent
// provider only disables the reasoning panel, never the terminal. See PLAN.md
// (milestone M6).
package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Provider kinds.
const (
	KindOpenAI = "openai"
	KindOllama = "ollama"
)

// Config describes one provider endpoint.
type Config struct {
	// Kind is KindOpenAI or KindOllama. Empty means KindOpenAI.
	Kind string `json:"kind"`
	// BaseURL is the endpoint root, e.g. https://api.openai.com/v1 or
	// http://127.0.0.1:11434.
	BaseURL string `json:"baseUrl"`
	// Model is the model name, e.g. "gpt-4o-mini" or "qwen2.5:7b".
	Model string `json:"model"`
	// APIKey is the bearer token; empty for local providers.
	APIKey string `json:"apiKey"`
}

// Message is one turn of the conversation.
type Message struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

// Provider streams an assistant reply one text delta at a time.
type Provider interface {
	// Stream sends the conversation and returns a channel of content deltas.
	// The channel is closed when the reply is complete; the context cancels it.
	Stream(ctx context.Context, msgs []Message) (<-chan string, error)
	// Name identifies the provider for diagnostics and the UI.
	Name() string
}

// ErrNotConfigured is returned when no usable provider is configured, so callers
// can disable AI features silently instead of surfacing a dead end.
var ErrNotConfigured = errors.New("未配置 AI 提供方")

// New validates the configuration and returns the matching provider.
func New(cfg Config) (Provider, error) {
	kind := strings.ToLower(strings.TrimSpace(cfg.Kind))
	if kind == "" {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("未配置 AI 模型")
	}
	switch kind {
	case KindOpenAI:
		base := strings.TrimSpace(cfg.BaseURL)
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		return &openAIProvider{baseURL: strings.TrimRight(base, "/"), model: cfg.Model, apiKey: cfg.APIKey}, nil
	case KindOllama:
		base := strings.TrimSpace(cfg.BaseURL)
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		return &ollamaProvider{baseURL: strings.TrimRight(base, "/"), model: cfg.Model}, nil
	default:
		return nil, fmt.Errorf("不支持的 AI 提供方：%s", cfg.Kind)
	}
}
