// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

// Blue-team / QA integration tests for the daemon's outer middleware
// chain (auth + rate-limit). The standard newTestDaemon helper omits
// these layers because they're not relevant to most integration tests
// — but their behavior IS production-critical, so we exercise them
// here with the full chain wired identically to main().

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"unheaded/pkg/agent"
	"unheaded/pkg/auth"
)

// newAuthDaemon constructs a daemon with the auth middleware enabled
// using the supplied API keys. /health, /ready, /metrics, and
// /api/v1/openapi.json are skipped per main()'s middleware setup.
func newAuthDaemon(t *testing.T, apiKeys []string, retr agent.Retriever, llm agent.LLM) (*testDaemonHandle, string) {
	t.Helper()
	root := t.TempDir()
	store := newMetricsActionStore(nopActionStore{})
	pool := newChampionPool(store)
	srv := &server{
		pool:        pool,
		defaultRoot: root,
		allowed:     map[string]struct{}{root: {}},
		retriever:   retr,
		llm:         llm,
		vorURL:      "http://127.0.0.1:1",
		llamaURL:    "http://127.0.0.1:1",
		ready:       newReadyTracker(&http.Client{}, "http://127.0.0.1:1", "http://127.0.0.1:1"),
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/health", instrument("/health", http.HandlerFunc(srv.handleHealth)))
	mux.Handle("/ready", instrument("/ready", http.HandlerFunc(srv.handleReady)))
	mux.Handle("/api/v1/agent/ask", instrument("/api/v1/agent/ask", http.HandlerFunc(srv.handleAsk)))
	mux.Handle("/api/v1/agent/confirm", instrument("/api/v1/agent/confirm", http.HandlerFunc(srv.handleConfirm)))
	mux.Handle("/api/v1/openapi.json", instrument("/api/v1/openapi.json", http.HandlerFunc(srv.handleOpenAPI)))

	authMW := auth.SetupMiddleware(auth.ServiceAuthConfig{
		Enabled:     true,
		APIKeys:     apiKeys,
		ServiceName: "zhen-agentd-test",
	})
	if authMW == nil {
		t.Fatalf("auth middleware did not initialize (Enabled=true, keys=%d)", len(apiKeys))
	}
	extended := auth.SkipAuthPaths(authMW, "/api/v1/openapi.json")
	handler := extended(mux)

	return startDaemon(t, handler), root
}

// newRateLimitDaemon builds a daemon with the rate-limit middleware
// configured at rps=2, burst=2 — small enough that 5 rapid requests
// from the same client will see at least one 429.
func newRateLimitDaemon(t *testing.T, retr agent.Retriever, llm agent.LLM) (*testDaemonHandle, string) {
	t.Helper()
	root := t.TempDir()
	store := newMetricsActionStore(nopActionStore{})
	pool := newChampionPool(store)
	srv := &server{
		pool:        pool,
		defaultRoot: root,
		allowed:     map[string]struct{}{root: {}},
		retriever:   retr,
		llm:         llm,
		vorURL:      "http://127.0.0.1:1",
		llamaURL:    "http://127.0.0.1:1",
		ready:       newReadyTracker(&http.Client{}, "http://127.0.0.1:1", "http://127.0.0.1:1"),
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/health", instrument("/health", http.HandlerFunc(srv.handleHealth)))
	mux.Handle("/api/v1/agent/ask", instrument("/api/v1/agent/ask", http.HandlerFunc(srv.handleAsk)))

	rl := newRateLimiter(2, 2) // 2 rps, burst 2
	if rl == nil {
		t.Fatalf("rate limiter did not initialize")
	}
	handler := rl.Middleware(mux)

	return startDaemon(t, handler), root
}

// startDaemon wraps a handler in httptest.NewServer with t.Cleanup
// registered. Returns just the URL — every middleware test goes
// through the standard *http.Client, so the underlying *httptest.Server
// doesn't need to be exposed.
func startDaemon(t *testing.T, h http.Handler) *testDaemonHandle {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return &testDaemonHandle{URL: ts.URL}
}

type testDaemonHandle struct {
	URL string
}

// --- AUTH (blue-team) ---

func TestBlueTeam_Auth_RejectsMissingKey(t *testing.T) {
	d, _ := newAuthDaemon(t, []string{"sekret-1"}, canonicalStub(), &scriptedLLM{})

	resp, err := http.Post(d.URL+"/api/v1/agent/ask", "application/json", strings.NewReader(`{"goal":"x"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestBlueTeam_Auth_RejectsWrongKey(t *testing.T) {
	d, _ := newAuthDaemon(t, []string{"correct-key"}, canonicalStub(), &scriptedLLM{})

	req, _ := http.NewRequest("POST", d.URL+"/api/v1/agent/ask", strings.NewReader(`{"goal":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "wrong-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestBlueTeam_Auth_AcceptsValidKey(t *testing.T) {
	llm := &scriptedLLM{out: []string{`{"thought":"k","answer":"authd"}`}}
	d, _ := newAuthDaemon(t, []string{"correct-key"}, canonicalStub(), llm)

	req, _ := http.NewRequest("POST", d.URL+"/api/v1/agent/ask", strings.NewReader(`{"goal":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "correct-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var ar askResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ar.Answer != "authd" {
		t.Errorf("answer: got %q, want authd", ar.Answer)
	}
}

func TestBlueTeam_Auth_HealthReadyMetricsBypassAuth(t *testing.T) {
	d, _ := newAuthDaemon(t, []string{"k"}, canonicalStub(), &scriptedLLM{})
	for _, path := range []string{"/health", "/metrics"} {
		resp, err := http.Get(d.URL + path)
		if err != nil {
			t.Errorf("GET %s: %v", path, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s: status got %d, want 200 (must bypass auth)", path, resp.StatusCode)
		}
	}
	// /ready returns 503 here (backends unreachable), but it shouldn't
	// be 401.
	resp, err := http.Get(d.URL + "/ready")
	if err != nil {
		t.Fatalf("GET /ready: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("/ready: status 401, must bypass auth")
	}
}

func TestBlueTeam_Auth_OpenAPIBypassesAuth(t *testing.T) {
	d, _ := newAuthDaemon(t, []string{"k"}, canonicalStub(), &scriptedLLM{})
	resp, err := http.Get(d.URL + "/api/v1/openapi.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200 (openapi must bypass auth so clients can discover how to authenticate)", resp.StatusCode)
	}
}

// --- RATE LIMIT (blue-team) ---

func TestBlueTeam_RateLimit_BurstExceeds429(t *testing.T) {
	llm := &constantLLM{out: `{"thought":"k","answer":"ok"}`}
	d, _ := newRateLimitDaemon(t, canonicalStub(), llm)

	// rps=2, burst=2. Fire 5 rapid sequential requests; we expect at
	// least one 429 (after the burst is consumed and refill hasn't
	// caught up). Sequential — go's rate.Limiter Allow() is
	// monotonic and the requests share an IP from the test client.
	var got200, got429 int
	for i := 0; i < 5; i++ {
		resp, err := http.Post(d.URL+"/api/v1/agent/ask", "application/json", strings.NewReader(`{"goal":"x"}`))
		if err != nil {
			t.Fatalf("post #%d: %v", i, err)
		}
		resp.Body.Close()
		switch resp.StatusCode {
		case 200:
			got200++
		case http.StatusTooManyRequests:
			got429++
			if h := resp.Header.Get("Retry-After"); h == "" {
				t.Errorf("429 without Retry-After header")
			}
		default:
			t.Errorf("unexpected status: %d", resp.StatusCode)
		}
	}
	if got429 == 0 {
		t.Errorf("expected at least one 429; got 200×%d 429×%d", got200, got429)
	}
	if got200 == 0 {
		t.Errorf("expected at least one 200 (burst tokens); got 200×%d 429×%d", got200, got429)
	}
}

func TestBlueTeam_RateLimit_HealthBypassesLimiter(t *testing.T) {
	d, _ := newRateLimitDaemon(t, canonicalStub(), &scriptedLLM{})
	// 10 health checks must all succeed — never rate-limited.
	for i := 0; i < 10; i++ {
		resp, err := http.Get(d.URL + "/health")
		if err != nil {
			t.Fatalf("GET /health #%d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("/health #%d: status got %d, want 200", i, resp.StatusCode)
		}
	}
}
