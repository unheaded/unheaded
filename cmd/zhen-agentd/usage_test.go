// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// An answer that went through the daemon showed "0 tok" on the page: the
// usage llama-server reports reached the audit row and nowhere else (D-02).
// One ask is several model calls; the answer carries their sum.

// usageLlama answers each call with the next body; the last one repeats.
func usageLlama(t *testing.T, bodies ...string) *httptest.Server {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		i := int(n.Add(1)) - 1
		if i >= len(bodies) {
			i = len(bodies) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, bodies[i])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func completion(t *testing.T, content, usage string) string {
	t.Helper()
	c, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if usage != "" {
		usage = `,"usage":` + usage
	}
	return fmt.Sprintf(`{"model":"qwen.gguf","choices":[{"finish_reason":"stop","message":{"content":%s}}]%s}`, c, usage)
}

func usageServer(t *testing.T, usage1, usage2 string) *server {
	t.Helper()
	root := t.TempDir()
	turn1 := fmt.Sprintf(`{"thought":"write","tool_call":{"name":"write_file","args":{"path":%q,"content":"x"}}}`, filepath.Join(root, "a.txt"))
	turn2 := `{"thought":"done","answer":"waiting"}`
	llama := usageLlama(t, completion(t, turn1, usage1), completion(t, turn2, usage2))
	return &server{
		pool:        newChampionPool(newMetricsActionStore(nopActionStore{}), nil),
		defaultRoot: root,
		allowed:     map[string]struct{}{root: {}},
		retriever:   externalStub(),
		llm:         &llamaLLM{client: llama.Client(), baseURL: llama.URL, model: "qwen"},
	}
}

func askJSON(t *testing.T, srv *server) map[string]any {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"goal": "write a.txt"})
	rec := httptest.NewRecorder()
	srv.handleAsk(rec, httptest.NewRequest(http.MethodPost, "/api/v1/agent/ask", bytes.NewReader(b)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out
}

func TestAskUsage_SumsTheTurns(t *testing.T) {
	srv := usageServer(t, `{"prompt_tokens":10,"completion_tokens":3}`, `{"prompt_tokens":12,"completion_tokens":5}`)
	out := askJSON(t, srv)
	if out["turns_used"] != float64(2) {
		t.Fatalf("turns_used = %v, want 2 (the fixture did not run two turns): %v", out["turns_used"], out)
	}
	if out["tokens_input"] != float64(22) || out["tokens_output"] != float64(8) {
		t.Errorf("tokens_input, tokens_output = %v, %v; want 22, 8", out["tokens_input"], out["tokens_output"])
	}
}

func TestAskUsage_NoUsageIsZeroNotAnError(t *testing.T) {
	srv := usageServer(t, "", "")
	out := askJSON(t, srv)
	in, okIn := out["tokens_input"]
	outTok, okOut := out["tokens_output"]
	if !okIn || !okOut || in != float64(0) || outTok != float64(0) {
		t.Errorf("tokens_input, tokens_output = %v, %v; want both present and 0", in, outTok)
	}
}

func TestAskUsage_OneAskDoesNotCountAnothers(t *testing.T) {
	srv := usageServer(t, `{"prompt_tokens":10,"completion_tokens":3}`, `{"prompt_tokens":12,"completion_tokens":5}`)
	_ = askJSON(t, srv)
	// The fake now repeats its last body: each further turn is 12 in, 5 out.
	out := askJSON(t, srv)
	turns, _ := out["turns_used"].(float64)
	if out["tokens_input"] != 12*turns || out["tokens_output"] != 5*turns {
		t.Errorf("second ask: tokens %v/%v over %v turns; the first ask's usage leaked in", out["tokens_input"], out["tokens_output"], turns)
	}
}

func TestAskStreamUsage_DoneEventCarriesTheSum(t *testing.T) {
	srv := usageServer(t, `{"prompt_tokens":10,"completion_tokens":3}`, `{"prompt_tokens":12,"completion_tokens":5}`)
	b, _ := json.Marshal(map[string]any{"goal": "write a.txt"})
	rec := httptest.NewRecorder()
	srv.handleAskStream(rec, httptest.NewRequest(http.MethodPost, "/api/v1/agent/ask/stream", bytes.NewReader(b)))
	body := rec.Body.String()
	i := strings.Index(body, "event: done\ndata: ")
	if i < 0 {
		t.Fatalf("no done event: %s", body)
	}
	line := strings.SplitN(body[i+len("event: done\ndata: "):], "\n", 2)[0]
	var done map[string]any
	if err := json.Unmarshal([]byte(line), &done); err != nil {
		t.Fatalf("decode %s: %v", line, err)
	}
	if done["tokens_input"] != float64(22) || done["tokens_output"] != float64(8) {
		t.Errorf("done event tokens = %v, %v; want 22, 8", done["tokens_input"], done["tokens_output"])
	}
}
