// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package server

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func (s *Server) streamSubCount() int {
	s.streamSubsMu.RLock()
	defer s.streamSubsMu.RUnlock()
	return len(s.streamSubs)
}

func waitForStreamSubs(t *testing.T, s *Server, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for s.streamSubCount() != want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := s.streamSubCount(); got != want {
		t.Fatalf("stream subscriptions = %d, want %d", got, want)
	}
}

// M20: a request that never became a connection holds no subscription.
func TestStream_FailedUpgradeLeavesNoSubscription(t *testing.T) {
	srv := newStartedTestServer(t)
	for name, mod := range map[string]func(*http.Request){
		"not a websocket request": func(*http.Request) {},
		"foreign origin": func(r *http.Request) {
			r.Header.Set("Origin", "https://elsewhere.example")
			r.Header.Set("Upgrade", "websocket")
			r.Header.Set("Connection", "Upgrade")
			r.Header.Set("Sec-WebSocket-Key", base64.StdEncoding.EncodeToString([]byte("the sample nonce"))) // RFC 6455 section 1.3
		},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stream?types=metrics", nil)
		mod(req)
		rec := httptest.NewRecorder()
		srv.handleStreamWebSocket(rec, req)
		if rec.Code < 400 {
			t.Errorf("%s: status %d, want a refusal", name, rec.Code)
		}
		if got := srv.streamSubCount(); got != 0 {
			t.Errorf("%s: %d subscriptions left behind", name, got)
		}
	}
}

// M20: a subscription lives exactly as long as its connection.
func TestStream_SubscriptionEndsWithTheConnection(t *testing.T) {
	srv := newStartedTestServer(t)
	hs := httptest.NewServer(http.HandlerFunc(srv.handleStreamWebSocket))
	defer hs.Close()

	url := "ws" + strings.TrimPrefix(hs.URL, "http") + "?types=metrics"
	conn, resp, err := websocket.DefaultDialer.Dial(url, sameOriginHeader(hs.URL))
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	waitForStreamSubs(t, srv, 1)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	waitForStreamSubs(t, srv, 0)
}

// M20: the filter is bounded, by entries and by size.
func TestStream_OversizedFilterRefused(t *testing.T) {
	srv := newStartedTestServer(t)
	for name, query := range map[string]string{
		"types entries":    "types=" + strings.Repeat(",", maxStreamFilterEntries),
		"services entries": "services=" + strings.Repeat("a,", maxStreamFilterEntries+1),
		"types size":       "types=" + strings.Repeat("a", maxStreamFilterBytes+1),
		"services size":    "services=" + strings.Repeat("a", maxStreamFilterBytes+1),
	} {
		rec := httptest.NewRecorder()
		srv.handleStreamWebSocket(rec, httptest.NewRequest(http.MethodGet, "/api/v1/stream?"+query, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	if got := srv.streamSubCount(); got != 0 {
		t.Errorf("%d subscriptions left behind", got)
	}
}

// L9: markup that reaches a page cannot post a form off this origin.
func TestDashboardCSP_LimitsForms(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'self'") {
		t.Fatalf("CSP %q has no form-action", csp)
	}
}
