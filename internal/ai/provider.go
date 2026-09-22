package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpClient is shared by both providers. No overall timeout: a generation can
// take a while, and the caller bounds it through the request context.
var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     60 * time.Second,
	},
}

// maxLineLen bounds one streamed protocol line (a delta is small, but a model
// can emit a long single chunk).
const maxLineLen = 1 << 20

// errBodyLimit caps how much of an error response is echoed back.
const errBodyLimit = 4 << 10

// checkResponse turns a non-2xx response into an actionable error and closes the
// body in that case.
func checkResponse(resp *http.Response) error {
	if resp.StatusCode/100 == 2 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit))
	_ = resp.Body.Close()
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("AI 服务拒绝认证（%d）：请检查 API Key", resp.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("AI 接口不存在（404）：请检查 Base URL 与模型名，响应：%s", msg)
	}
	return fmt.Errorf("AI 服务返回 %d：%s", resp.StatusCode, msg)
}

// postJSON sends a JSON body and returns the streaming response.
func postJSON(ctx context.Context, url string, headers map[string]string, payload any) (*http.Response, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// scanLines reads a streaming response line by line, handing each line to parse
// until it reports completion or the stream ends.
//
// handle returns (delta, done). Empty deltas are skipped so protocol noise does
// not reach the UI.
func scanLines(ctx context.Context, body io.Reader, handle func(line []byte) (string, bool)) <-chan string {
	out := make(chan string, 32)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(body)
		sc.Buffer(make([]byte, 0, 64<<10), maxLineLen)
		for sc.Scan() {
			delta, done := handle(sc.Bytes())
			if delta != "" {
				select {
				case out <- delta:
				case <-ctx.Done():
					return
				}
			}
			if done {
				return
			}
		}
	}()
	return out
}

// complete is the sentinel a provider returns once the reply is finished.
const complete = "\x00done"

// ---- OpenAI-compatible ----

type openAIProvider struct {
	baseURL string
	model   string
	apiKey  string
}

func (p *openAIProvider) Name() string { return "openai(" + p.model + ")" }

func (p *openAIProvider) Stream(ctx context.Context, msgs []Message) (<-chan string, error) {
	headers := map[string]string{}
	if p.apiKey != "" {
		headers["Authorization"] = "Bearer " + p.apiKey
	}
	resp, err := postJSON(ctx, p.baseURL+"/chat/completions", headers, map[string]any{
		"model":    p.model,
		"messages": wireMessages(msgs),
		"stream":   true,
	})
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = resp.Body.Close()
	}()
	ch := scanLines(ctx, resp.Body, parseOpenAILine)
	return stripSentinel(ctx, ch), nil
}

// parseOpenAILine decodes one SSE line of an OpenAI-compatible stream.
func parseOpenAILine(line []byte) (string, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
		return "", false
	}
	payload := bytes.TrimSpace(line[len("data:"):])
	if bytes.Equal(payload, []byte("[DONE]")) {
		return complete, true
	}
	var msg struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			// Some gateways put the whole text here instead of in the delta.
			Text string `json:"text"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return "", false
	}
	if msg.Error != nil && msg.Error.Message != "" {
		return "[AI 错误] " + msg.Error.Message, true
	}
	if len(msg.Choices) == 0 {
		return "", false
	}
	if c := msg.Choices[0].Delta.Content; c != "" {
		return c, false
	}
	return msg.Choices[0].Text, false
}

// ---- Ollama ----

type ollamaProvider struct {
	baseURL string
	model   string
}

func (p *ollamaProvider) Name() string { return "ollama(" + p.model + ")" }

func (p *ollamaProvider) Stream(ctx context.Context, msgs []Message) (<-chan string, error) {
	resp, err := postJSON(ctx, p.baseURL+"/api/chat", nil, map[string]any{
		"model":    p.model,
		"messages": wireMessages(msgs),
		"stream":   true,
	})
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = resp.Body.Close()
	}()
	return stripSentinel(ctx, scanLines(ctx, resp.Body, parseOllamaLine)), nil
}

// parseOllamaLine decodes one NDJSON line of an Ollama stream.
func parseOllamaLine(line []byte) (string, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return "", false
	}
	var msg struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Done  bool   `json:"done"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return "", false
	}
	if msg.Error != "" {
		return "[AI 错误] " + msg.Error, true
	}
	if msg.Done {
		if msg.Message.Content != "" {
			return msg.Message.Content, true
		}
		return complete, true
	}
	return msg.Message.Content, false
}

// stripSentinel drops the internal completion marker so callers only ever see
// model text.
func stripSentinel(ctx context.Context, in <-chan string) <-chan string {
	out := make(chan string, 32)
	go func() {
		defer close(out)
		for delta := range in {
			if delta == complete {
				return
			}
			select {
			case out <- delta:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// wireMessages renders the conversation for the wire format both providers
// accept (Ollama uses the same role/content shape).
func wireMessages(msgs []Message) []map[string]string {
	out := make([]map[string]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, map[string]string{"role": m.Role, "content": m.Content})
	}
	return out
}
