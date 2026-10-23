// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeAPI is an httptest stand-in for the Messages API. Each POST to
// /v1/messages consumes the next canned response and replays it as the
// SSE stream the real API sends. No test talks to the real API.
type fakeAPI struct {
	t   *testing.T
	srv *httptest.Server

	mu        sync.Mutex
	responses []cannedMsg
	requests  []capturedRequest
}

type capturedRequest struct {
	Body   map[string]any
	Betas  string
	APIKey string
}

// cannedMsg is a response to stream back.
type cannedMsg struct {
	Model       string
	StopReason  string
	StopDetails map[string]any // refusal only
	Blocks      []cannedBlock
	Usage       map[string]any // message_delta usage; defaults filled in
}

type cannedBlock struct {
	Text      string // text block when ToolName == ""
	ToolID    string
	ToolName  string
	ToolInput string // raw JSON
}

func newFakeAPI(t *testing.T, responses ...cannedMsg) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, responses: responses}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/v1/messages") {
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, capturedRequest{Body: body, Betas: r.Header.Get("anthropic-beta"), APIKey: r.Header.Get("x-api-key")})
	if len(f.responses) == 0 {
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"fake API has no canned response left"}}`)
		return
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	n := len(f.requests)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	writeSSE(w, resp, fmt.Sprintf("msg_fake_%d", n))
}

func (f *fakeAPI) reqs() []capturedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]capturedRequest(nil), f.requests...)
}

func event(w io.Writer, name string, data any) {
	b, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
}

func writeSSE(w io.Writer, m cannedMsg, id string) {
	model := m.Model
	if model == "" {
		model = DefaultModel
	}
	usage := map[string]any{"input_tokens": 1000, "output_tokens": 200, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
	for k, v := range m.Usage {
		usage[k] = v
	}
	event(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": id, "type": "message", "role": "assistant", "model": model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": usage["input_tokens"], "output_tokens": 1},
		},
	})
	for i, b := range m.Blocks {
		if b.ToolName == "" {
			event(w, "content_block_start", map[string]any{"type": "content_block_start", "index": i, "content_block": map[string]any{"type": "text", "text": ""}})
			event(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": i, "delta": map[string]any{"type": "text_delta", "text": b.Text}})
		} else {
			event(w, "content_block_start", map[string]any{"type": "content_block_start", "index": i, "content_block": map[string]any{"type": "tool_use", "id": b.ToolID, "name": b.ToolName, "input": map[string]any{}}})
			event(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": i, "delta": map[string]any{"type": "input_json_delta", "partial_json": b.ToolInput}})
		}
		event(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": i})
	}
	delta := map[string]any{"stop_reason": m.StopReason, "stop_sequence": nil}
	if m.StopDetails != nil {
		delta["stop_details"] = m.StopDetails
	}
	event(w, "message_delta", map[string]any{"type": "message_delta", "delta": delta, "usage": usage})
	event(w, "message_stop", map[string]any{"type": "message_stop"})
}

// messages returns the request's messages array.
func (c capturedRequest) messages(t *testing.T) []map[string]any {
	t.Helper()
	raw, ok := c.Body["messages"].([]any)
	if !ok {
		t.Fatalf("request has no messages: %v", c.Body)
	}
	out := make([]map[string]any, len(raw))
	for i, m := range raw {
		out[i] = m.(map[string]any)
	}
	return out
}

// toolResults returns the tool_result blocks of the request's last message.
func (c capturedRequest) toolResults(t *testing.T) []map[string]any {
	t.Helper()
	msgs := c.messages(t)
	last := msgs[len(msgs)-1]
	if last["role"] != "user" {
		t.Fatalf("last message role = %v", last["role"])
	}
	content, _ := last["content"].([]any)
	var out []map[string]any
	for _, b := range content {
		bm := b.(map[string]any)
		if bm["type"] == "tool_result" {
			out = append(out, bm)
		}
	}
	return out
}

// resultText flattens a tool_result's content (string or text blocks).
func resultText(r map[string]any) string {
	switch c := r["content"].(type) {
	case string:
		return c
	case []any:
		var sb strings.Builder
		for _, b := range c {
			if bm, ok := b.(map[string]any); ok {
				s, _ := bm["text"].(string)
				sb.WriteString(s)
			}
		}
		return sb.String()
	}
	return ""
}
