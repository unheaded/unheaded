// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package localllm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T, status int, body string, seen *map[string]any) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if seen != nil {
			_ = json.Unmarshal(raw, seen)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestChat_ParsesUsageAndSendsSeed(t *testing.T) {
	var seen map[string]any
	s := server(t, 200, `{"model":"qwen.gguf","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`, &seen)
	seed := 42
	c := &Client{BaseURL: s.URL, Model: "qwen-7b", MaxTokens: 64}
	a, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "q"}}, Seed: &seed})
	if err != nil {
		t.Fatal(err)
	}
	if a.Text != "hi" || a.Model != "qwen.gguf" || a.PromptTokens != 12 || a.CompletionTokens != 3 || a.Truncated() {
		t.Errorf("answer = %+v", a)
	}
	if seen["seed"] != float64(42) || seen["max_tokens"] != float64(64) || seen["model"] != "qwen-7b" || seen["temperature"] != float64(0) {
		t.Errorf("request = %v", seen)
	}
}

func TestChat_NoSeedNotSent(t *testing.T) {
	var seen map[string]any
	s := server(t, 200, `{"choices":[{"message":{"content":"x"}}]}`, &seen)
	a, err := (&Client{BaseURL: s.URL}).Chat(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := seen["seed"]; ok {
		t.Error("seed sent without one")
	}
	if a.Model != DefaultModel || seen["max_tokens"] != float64(DefaultMaxTokens) {
		t.Errorf("defaults: %+v %v", a, seen)
	}
}

func TestChat_Errors(t *testing.T) {
	for name, s := range map[string]*httptest.Server{
		"status":     server(t, 503, "loading model", nil),
		"not json":   server(t, 200, "<html>", nil),
		"no choices": server(t, 200, `{"choices":[]}`, nil),
	} {
		if _, err := (&Client{BaseURL: s.URL}).Chat(context.Background(), Request{}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := (&Client{BaseURL: "ftp://x"}).Chat(context.Background(), Request{}); err == nil {
		t.Error("non-http URL accepted")
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	if _, err := (&Client{BaseURL: dead.URL}).Chat(context.Background(), Request{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("down server: %v", err)
	}
}

func TestChat_TruncatedAndEmptyAreReturnedNotErrors(t *testing.T) {
	s := server(t, 200, `{"choices":[{"finish_reason":"length","message":{"content":""}}]}`, nil)
	a, err := (&Client{BaseURL: s.URL}).Chat(context.Background(), Request{})
	if err != nil || !a.Truncated() || a.Text != "" {
		t.Errorf("a = %+v, err = %v", a, err)
	}
}

func TestChat_Timeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(s.Close)
	t0 := time.Now()
	if _, err := (&Client{BaseURL: s.URL, Timeout: 100 * time.Millisecond}).Chat(context.Background(), Request{}); err == nil {
		t.Fatal("no timeout")
	}
	if time.Since(t0) > time.Second {
		t.Error("timeout not applied")
	}
}

func TestChat_OversizedBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"`))
		_, _ = w.Write([]byte(strings.Repeat("a", MaxBodyBytes+10)))
		_, _ = w.Write([]byte(`"}}]}`))
	}))
	t.Cleanup(s.Close)
	if _, err := (&Client{BaseURL: s.URL}).Chat(context.Background(), Request{}); err == nil {
		t.Error("oversized body accepted")
	}
}
