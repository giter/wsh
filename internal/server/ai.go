package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sshclient/internal/ai"
	"sshclient/internal/safety"
	"sshclient/internal/sanitize"
	"sshclient/storage"
)

// The AI layer is strictly advisory. It never decides whether a command runs:
// anything the model proposes goes back through safety.Analyze and then through
// terminal.exec, so an unavailable or hallucinating model cannot loosen the
// guard rails (see PLAN.md, M6).

// aiRequestTimeout bounds one generation.
const aiRequestTimeout = 2 * time.Minute

// maxReplyLen bounds the accumulated reply, so a runaway stream cannot exhaust
// memory.
const maxReplyLen = 128 << 10

// aiConfig builds the provider configuration from the stored settings.
func (s *Server) aiConfig() ai.Config {
	st := s.store.Settings()
	cfg := ai.Config{
		Kind:    st.AIProvider,
		BaseURL: st.AIBaseURL,
		Model:   st.AIModel,
	}
	if st.AIKeyEncrypted != "" {
		if key, err := storage.DecryptSecret(st.AIKeyEncrypted); err == nil {
			cfg.APIKey = key
		}
	}
	return cfg
}

func (s *Server) aiProvider() (ai.Provider, error) {
	return ai.New(s.aiConfig())
}

func (s *Server) handleAIStatus(c *wsClient, params json.RawMessage) (interface{}, error) {
	cfg := s.aiConfig()
	out := map[string]interface{}{
		"configured":  false,
		"provider":    cfg.Kind,
		"model":       cfg.Model,
		"autoAnalyze": s.store.Settings().AIAutoAnalyze,
		"noContext":   s.store.Settings().AINoContext,
	}
	if p, err := ai.New(cfg); err == nil {
		out["configured"] = true
		out["name"] = p.Name()
	}
	return out, nil
}

type aiAskParams struct {
	SessionID string `json:"sessionId"`
	Prompt    string `json:"prompt"`
	// Kind is "command" (natural language to shell) or "diagnose" (root cause).
	Kind string `json:"kind"`
	// Excerpt is explicit context, e.g. the error line the sniffer just found.
	Excerpt string `json:"excerpt"`
}

func (s *Server) handleAIAsk(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p aiAskParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	p.Prompt = strings.TrimSpace(p.Prompt)
	if p.Prompt == "" && p.Excerpt == "" {
		return nil, fmt.Errorf("请输入要询问的内容")
	}
	provider, err := s.aiProvider()
	if err != nil {
		return nil, err
	}
	kind := p.Kind
	if kind != "diagnose" {
		kind = "command"
	}

	system := ai.SystemPromptCommand()
	if kind == "diagnose" {
		system = ai.SystemPromptDiagnose()
	}
	user := s.buildUserPrompt(kind, p)

	requestID := storage.NewID()
	// Stream in the background: the RPC returns immediately so the UI can render
	// deltas as they arrive. The sink is passed in rather than captured, which
	// keeps the streaming logic testable without a WebSocket.
	send := func(v interface{}) { c.send(v) }
	go s.streamAI(send, provider, requestID, system, user, kind)
	return map[string]interface{}{"requestId": requestID, "kind": kind}, nil
}

// buildUserPrompt assembles the user turn, attaching masked terminal context
// unless the user turned context sharing off.
func (s *Server) buildUserPrompt(kind string, p aiAskParams) string {
	var b strings.Builder
	switch kind {
	case "diagnose":
		b.WriteString("终端最近输出（已脱敏）：\n")
	default:
		b.WriteString("用户请求：")
		b.WriteString(sanitize.Mask(p.Prompt))
		b.WriteString("\n")
	}

	st := s.store.Settings()
	if !st.AINoContext {
		ctx := p.Excerpt
		if ctx == "" && p.SessionID != "" {
			if ws, ok := s.session(p.SessionID); ok && ws.sniff != nil {
				ctx = ws.sniff.recentText()
			}
		}
		if ctx != "" {
			b.WriteString(sanitize.Mask(ctx))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// streamAI forwards deltas to the browser and finishes with a structured result.
func (s *Server) streamAI(send func(interface{}), provider ai.Provider, requestID, system, user, kind string) {
	ctx, cancel := context.WithTimeout(context.Background(), aiRequestTimeout)
	defer cancel()

	ch, err := provider.Stream(ctx, []ai.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
	if err != nil {
		send(aiDoneMsg{Type: "ai.done", RequestID: requestID, Error: err.Error()})
		return
	}

	var sb strings.Builder
	for delta := range ch {
		if sb.Len() < maxReplyLen {
			sb.WriteString(delta)
		}
		send(aiDeltaMsg{Type: "ai.delta", RequestID: requestID, Text: delta})
	}
	if ctx.Err() == context.DeadlineExceeded {
		send(aiDoneMsg{Type: "ai.done", RequestID: requestID, Error: "AI 请求超时"})
		return
	}

	text := sb.String()
	done := aiDoneMsg{
		Type:      "ai.done",
		RequestID: requestID,
		OK:        true,
		Text:      text,
		Steps:     ai.ParseSteps(text),
	}
	if kind == "command" {
		cmd := ai.GeneratedCommand(text)
		done.Command = cmd
		if cmd != "" {
			// The proposed command is classified locally; the model's own
			// opinion about risk is ignored.
			done.Risk = safety.Analyze(cmd)
		}
	}
	send(done)
}

// aiDeltaMsg is one streamed fragment of the reply.
type aiDeltaMsg struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	Text      string `json:"text"`
}

// aiDoneMsg closes a request with the parsed result.
type aiDoneMsg struct {
	Type      string    `json:"type"`
	RequestID string    `json:"requestId"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
	Text      string    `json:"text,omitempty"`
	Steps     []ai.Step `json:"steps,omitempty"`
	// Command is the single command the model proposed, already classified.
	Command string        `json:"command,omitempty"`
	Risk    safety.Result `json:"risk,omitempty"`
}
