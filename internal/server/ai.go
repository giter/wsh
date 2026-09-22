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
	// Kind is "command" (natural language to shell), "diagnose" (root cause of
	// a terminal error) or "result" (interpret the output of a command the user
	// just ran from a card).
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
	switch kind {
	case "diagnose", "result":
	default:
		kind = "command"
	}

	system := ai.SystemPromptCommand()
	switch kind {
	case "diagnose":
		system = ai.SystemPromptDiagnose()
	case "result":
		system = ai.SystemPromptResult()
	}
	user := s.buildUserPrompt(kind, p)

	requestID := storage.NewID()
	// Stream in the background: the RPC returns immediately so the UI can render
	// deltas as they arrive. The sink is passed in rather than captured, which
	// keeps the streaming logic testable without a WebSocket.
	//
	// The request is registered as cancellable before the goroutine starts, so an
	// ai.cancel arriving immediately after the reply can still stop it.
	ctx, cancel := context.WithCancel(context.Background())
	s.aiMu.Lock()
	s.aiCancels[requestID] = cancel
	s.aiMu.Unlock()

	send := func(v interface{}) { c.send(v) }
	go s.streamAI(ctx, send, provider, requestID, system, user, kind)
	return map[string]interface{}{"requestId": requestID, "kind": kind}, nil
}

// handleAICancel abandons an in-flight generation. It is wired to the Esc key in
// the Smart Input: a model that is slow to answer must not hold the UI.
func (s *Server) handleAICancel(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	s.aiMu.Lock()
	cancel := s.aiCancels[p.RequestID]
	delete(s.aiCancels, p.RequestID)
	s.aiMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return map[string]interface{}{"ok": true}, nil
}

// buildUserPrompt assembles the user turn, attaching masked terminal context
// unless the user turned context sharing off.
func (s *Server) buildUserPrompt(kind string, p aiAskParams) string {
	var b strings.Builder
	st := s.store.Settings()

	// Resolve the context first: a result analysis must not leave a "命令输出："
	// header dangling when the user has turned context sharing off.
	ctx := ""
	if !st.AINoContext {
		ctx = p.Excerpt
		if ctx == "" && p.SessionID != "" {
			if ws, ok := s.session(p.SessionID); ok && ws.sniff != nil {
				ctx = ws.sniff.recentText()
			}
		}
		ctx = strings.TrimSpace(ctx)
	}

	switch kind {
	case "diagnose":
		b.WriteString("终端最近输出（已脱敏）：\n")
	case "result":
		// The prompt carries the command that ran and the excerpt carries its
		// captured output, so the model reviews its own suggestion.
		b.WriteString("已执行命令：")
		b.WriteString(sanitize.Mask(p.Prompt))
		b.WriteString("\n命令输出（已脱敏）：\n")
	default:
		b.WriteString("用户请求：")
		b.WriteString(sanitize.Mask(p.Prompt))
		b.WriteString("\n")
	}

	switch {
	case ctx != "":
		b.WriteString(sanitize.Mask(ctx))
		b.WriteString("\n")
	case kind == "result":
		b.WriteString("（用户已关闭上下文共享，未附带输出）\n")
	}
	return b.String()
}

// streamAI forwards deltas to the browser and finishes with a structured result.
// parent is cancelled when the user abandons the request (ai.cancel).
func (s *Server) streamAI(parent context.Context, send func(interface{}), provider ai.Provider, requestID, system, user, kind string) {
	defer func() {
		s.aiMu.Lock()
		delete(s.aiCancels, requestID)
		s.aiMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(parent, aiRequestTimeout)
	defer cancel()

	ch, err := provider.Stream(ctx, []ai.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
	if err != nil {
		if parent.Err() != nil {
			send(aiDoneMsg{Type: "ai.done", RequestID: requestID, Cancelled: true})
			return
		}
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
	if parent.Err() != nil {
		// The user pressed Esc: say so, and do not pretend a partial reply is an
		// answer.
		send(aiDoneMsg{Type: "ai.done", RequestID: requestID, Cancelled: true})
		return
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
	switch kind {
	case "command":
		cmd := ai.GeneratedCommand(text)
		done.Command = cmd
		if cmd != "" {
			// The proposed command is classified locally; the model's own
			// opinion about risk is ignored.
			done.Risk = safety.Analyze(cmd)
		}
	case "result":
		// Follow-up commands become buttons on the same card, which is what makes
		// the investigation a loop instead of a dead end.
		done.Suggestions = ai.ParseSuggestions(text)
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
	// Cancelled marks a request the user abandoned (Esc). The card says so
	// instead of keeping a spinner that will never resolve.
	Cancelled bool `json:"cancelled,omitempty"`
	// Command is the single command the model proposed, already classified.
	Command string        `json:"command,omitempty"`
	Risk    safety.Result `json:"risk,omitempty"`
	// Suggestions are the follow-up commands proposed by a result analysis.
	Suggestions []ai.Suggestion `json:"suggestions,omitempty"`
}
