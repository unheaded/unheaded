// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"unheaded/pkg/metrics/prom"
)

// L6: the gateway binds loopback unless told otherwise, and the compose file
// that does tell it publishes on the host's loopback only.
func TestListenBind(t *testing.T) {
	t.Setenv("SOPHIA_GATEWAY_BIND", "")
	sg := NewSophiaGateway()
	if got := sg.listenAddr("20105"); got != "127.0.0.1:20105" {
		t.Fatalf("default = %q", got)
	}
	sg.gatewayBind = "0.0.0.0"
	if got := sg.listenAddr("9090"); got != "0.0.0.0:9090" {
		t.Fatalf("explicit = %q", got)
	}

	// The same instance: its metrics register once.
	big := `{"model":"` + strings.Repeat("a", maxChatBodyBytes+1) + `"}`
	w := httptest.NewRecorder()
	sg.handleChat(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(big)))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("chat body over the limit: %d, want 413", w.Code)
	}

	raw, err := os.ReadFile("../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(raw)
	for _, want := range []string{`SOPHIA_GATEWAY_BIND: "0.0.0.0"`, `"127.0.0.1:20106:9090"`, `"127.0.0.1:20107:20105"`} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %s", want)
		}
	}
}

// L10: an inference service's answer is read up to a limit, like every other
// internal read: past it the caller gets an error, not a gateway out of memory.
func TestChat_InferenceAnswerIsBounded(t *testing.T) {
	saved := maxInferenceBody
	t.Cleanup(func() { maxInferenceBody = saved })
	maxInferenceBody = 1 << 10

	var answer string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(answer))
	}))
	defer upstream.Close()
	sg := &SophiaGateway{
		vllmAddr:         upstream.URL,
		requestsTotal:    prom.NewCounter(prom.CounterOpts{Name: "r"}),
		inferenceErrors:  prom.NewCounter(prom.CounterOpts{Name: "e"}),
		tokensGenerated:  prom.NewCounter(prom.CounterOpts{Name: "g"}),
		tokensPrompted:   prom.NewCounter(prom.CounterOpts{Name: "p"}),
		inferenceLatency: prom.NewHistogram(prom.HistogramOpts{Name: "l"}),
	}
	chat := func() int {
		body := strings.NewReader(`{"model":"deepseek","messages":[]}`)
		rec := httptest.NewRecorder()
		sg.handleChat(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body))
		return rec.Code
	}
	answer = `{"id":"` + strings.Repeat("a", 2<<10) + `"}`
	if code := chat(); code != http.StatusBadGateway {
		t.Errorf("answer over the limit: status %d, want 502", code)
	}
	answer = `{"id":"short"}`
	if code := chat(); code != http.StatusOK {
		t.Errorf("answer under the limit: status %d, want 200", code)
	}
}
