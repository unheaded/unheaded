// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fakeLlama(t *testing.T, status int, body string, delay time.Duration) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var reqs []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		reqs = append(reqs, m)
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

func chatBody(content, finish string) string {
	b, _ := json.Marshal(map[string]any{
		"model":   "qwen2.5-coder-7b-instruct",
		"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]any{"role": "assistant", "content": content}}},
	})
	return string(b)
}

func closedURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	u := "http://" + ln.Addr().String()
	_ = ln.Close()
	return u
}

func TestLocalComplete(t *testing.T) {
	srv, reqs := fakeLlama(t, 200, chatBody("forty-two", "stop"), 0)
	l := &LocalLLM{BaseURL: srv.URL + "/", MaxTokens: 77, Temperature: 0.1}
	ans, err := l.Complete(context.Background(), "sys", "what?")
	if err != nil || ans.Text != "forty-two" || ans.Model != "qwen2.5-coder-7b-instruct" || ans.FinishReason != "stop" {
		t.Fatalf("%+v %v", ans, err)
	}
	r := (*reqs)[0]
	msgs := r["messages"].([]any)
	if len(msgs) != 2 || r["max_tokens"] != float64(77) || r["stream"] != false || r["model"] != DefaultLocalModel {
		t.Errorf("request = %v", r)
	}
}

func TestLocalCompleteFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"http 503", 503, `{"error":"loading model"}`, "HTTP 503"},
		{"not json", 200, "nope", "not an OpenAI chat completion"},
		{"no choices", 200, `{"choices":[]}`, "not an OpenAI chat completion"},
		{"empty", 200, chatBody("  ", "stop"), "empty answer"},
		{"too big", 200, strings.Repeat("x", maxLocalBodyBytes+5), "exceeds"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := fakeLlama(t, c.status, c.body, 0)
			_, err := (&LocalLLM{BaseURL: srv.URL}).Complete(context.Background(), "", "q")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want %q", err, c.want)
			}
		})
	}
	srv, _ := fakeLlama(t, 200, chatBody("half an ans", "length"), 0)
	ans, err := (&LocalLLM{BaseURL: srv.URL}).Complete(context.Background(), "", "q")
	if !errors.Is(err, ErrLocalTruncated) || ans.Text != "half an ans" {
		t.Errorf("length: %+v %v", ans, err)
	}
	if _, err := (&LocalLLM{BaseURL: closedURL(t)}).Complete(context.Background(), "", "q"); err == nil || !strings.Contains(err.Error(), "local LLM unavailable") {
		t.Errorf("down: %v", err)
	}
	slow, _ := fakeLlama(t, 200, chatBody("late", "stop"), 5*time.Second)
	t0 := time.Now()
	if _, err := (&LocalLLM{BaseURL: slow.URL, Timeout: 200 * time.Millisecond}).Complete(context.Background(), "", "q"); err == nil || time.Since(t0) > 3*time.Second {
		t.Errorf("timeout: %v after %s", err, time.Since(t0))
	}
	for _, bad := range []string{"file:///etc/passwd", "127.0.0.1:8081", "http://"} {
		if _, err := (&LocalLLM{BaseURL: bad}).Complete(context.Background(), "", "q"); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestRunCheck(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if r := RunCheck(ctx, []string{"grep", "-q", "forty"}, dir, "forty-two", 5*time.Second); !r.Accepted || r.Exit != 0 {
		t.Errorf("match: %+v", r)
	}
	if r := RunCheck(ctx, []string{"grep", "-q", "seven"}, dir, "forty-two", 5*time.Second); r.Accepted || r.Exit != 1 {
		t.Errorf("no match: %+v", r)
	}
	if r := RunCheck(ctx, []string{"/no/such/binary"}, dir, "x", 5*time.Second); r.Accepted || r.Exit != -1 || r.Err == "" {
		t.Errorf("missing: %+v", r)
	}
	if r := RunCheck(ctx, []string{"sleep", "10"}, dir, "x", 200*time.Millisecond); r.Accepted || !strings.Contains(r.Err, "timed out") {
		t.Errorf("timeout: %+v", r)
	}
	if r := RunCheck(ctx, nil, dir, "x", time.Second); r.Accepted || r.Err == "" {
		t.Errorf("empty: %+v", r)
	}
	// No shell: metacharacters are plain arguments.
	if r := RunCheck(ctx, []string{"test", "a;b", "=", "a;b"}, dir, "", 5*time.Second); !r.Accepted {
		t.Errorf("argv literal: %+v", r)
	}
}
