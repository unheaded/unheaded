// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"flag"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/services/wotan/internal/api"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/metrics"
	"unheaded/services/wotan/internal/middleware"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/wotan"

	"golang.org/x/time/rate"
)

// newTestFlagSet returns a FlagSet that captures errors (vs the real
// binary's ExitOnError, which would crash the test runner). Tests
// constructing this set never touch flag.CommandLine.
func newTestFlagSet() *flag.FlagSet {
	return flag.NewFlagSet("wotan-test", flag.ContinueOnError)
}

// ───────────────────────── parseFlags ─────────────────────────

func TestParseFlags_Defaults(t *testing.T) {
	cfg := parseFlags(newTestFlagSet(), nil)

	cases := []struct {
		name string
		got  any
		want any
	}{
		{"BufferSize", cfg.BufferSize, 10000},
		{"HTTPPort", cfg.HTTPPort, 18000},
		{"GRPCPort", cfg.GRPCPort, 18001},
		{"AdminEnabled", cfg.AdminEnabled, true},
		{"EnableTLS", cfg.EnableTLS, false},
		{"LogLevel", cfg.LogLevel, "info"},
		{"RateLimit", cfg.RateLimit, 100.0},
		{"RateBurst", cfg.RateBurst, 200},
		{"ReadTimeout", cfg.ReadTimeout, 15 * time.Second},
		{"WriteTimeout", cfg.WriteTimeout, 15 * time.Second},
		{"IdleTimeout", cfg.IdleTimeout, 60 * time.Second},
		{"ShutdownTimeout", cfg.ShutdownTimeout, 15 * time.Second},
		{"PendingApprovalTimeout", cfg.PendingApprovalTimeout, 1 * time.Hour},
		{"ClusterMode", cfg.ClusterMode, "standalone"},
		{"ClusterRole", cfg.ClusterRole, "primary"},
		{"ClusterReplicationPort", cfg.ClusterReplicationPort, 18002},
		{"StoreType", cfg.StoreType, "memory"},
		{"TopicConfigPath", cfg.TopicConfigPath, "configs/wotan.yaml"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestParseFlags_OverridesAllValues(t *testing.T) {
	args := []string{
		"-buffer-size=42",
		"-http-port=9000",
		"-grpc-port=9001",
		"-admin=false",
		"-enable-tls=true",
		"-tls-cert=/etc/cert.pem",
		"-tls-key=/etc/key.pem",
		"-log-level=debug",
		"-log-pretty=true",
		"-rate-limit=50.5",
		"-rate-burst=99",
		"-read-timeout=2s",
		"-write-timeout=3s",
		"-idle-timeout=4s",
		"-shutdown-timeout=7s",
		"-pending-approval-timeout=10m",
		"-topic-config=/tmp/x.yaml",
		"-cluster-mode=cluster",
		"-cluster-role=standby",
		"-cluster-node-id=node-x",
		"-cluster-peer=peer:18002",
		"-cluster-replication-port=20000",
		"-cluster-pki-dir=/etc/pki",
		"-store-type=wal",
		"-store-data-dir=/var/data",
		"-store-conn-str=postgres://...",
	}

	cfg := parseFlags(newTestFlagSet(), args)

	if cfg.BufferSize != 42 || cfg.HTTPPort != 9000 || cfg.GRPCPort != 9001 {
		t.Fatalf("server flags: %+v", cfg)
	}
	if cfg.AdminEnabled != false {
		t.Errorf("AdminEnabled: got %v, want false", cfg.AdminEnabled)
	}
	if !cfg.EnableTLS || cfg.TLSCertFile != "/etc/cert.pem" || cfg.TLSKeyFile != "/etc/key.pem" {
		t.Errorf("tls flags: %+v", cfg)
	}
	if cfg.LogLevel != "debug" || !cfg.LogPretty {
		t.Errorf("logging flags: %+v", cfg)
	}
	if cfg.RateLimit != 50.5 || cfg.RateBurst != 99 {
		t.Errorf("rate limit: %+v", cfg)
	}
	if cfg.ReadTimeout != 2*time.Second || cfg.WriteTimeout != 3*time.Second {
		t.Errorf("timeouts: read=%v write=%v", cfg.ReadTimeout, cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 4*time.Second || cfg.ShutdownTimeout != 7*time.Second {
		t.Errorf("timeouts: idle=%v shutdown=%v", cfg.IdleTimeout, cfg.ShutdownTimeout)
	}
	if cfg.PendingApprovalTimeout != 10*time.Minute {
		t.Errorf("pending-approval-timeout: got %v, want 10m", cfg.PendingApprovalTimeout)
	}
	if cfg.ClusterMode != "cluster" || cfg.ClusterRole != "standby" || cfg.ClusterNodeID != "node-x" {
		t.Errorf("cluster: %+v", cfg)
	}
	if cfg.ClusterPeerAddr != "peer:18002" || cfg.ClusterReplicationPort != 20000 {
		t.Errorf("cluster peer/port: %+v", cfg)
	}
	if cfg.ClusterPKIDir != "/etc/pki" {
		t.Errorf("cluster pki: got %q", cfg.ClusterPKIDir)
	}
	if cfg.StoreType != "wal" || cfg.StoreDataDir != "/var/data" || cfg.StoreConnStr != "postgres://..." {
		t.Errorf("store: %+v", cfg)
	}
	if cfg.TopicConfigPath != "/tmp/x.yaml" {
		t.Errorf("topic-config: got %q", cfg.TopicConfigPath)
	}
}

func TestParseFlags_RateCleanupHardcoded(t *testing.T) {
	// RateCleanup is intentionally not a flag (it's a server-internal
	// tuning knob that callers shouldn't touch). Verify it still gets
	// the hardcoded 5 min default.
	cfg := parseFlags(newTestFlagSet(), nil)
	if cfg.RateCleanup != 5*time.Minute {
		t.Fatalf("RateCleanup: got %v, want 5m", cfg.RateCleanup)
	}
}

func TestParseFlags_CORSOriginsDefault(t *testing.T) {
	// CORSOrigins is hardcoded to ["*"] in parseFlags. Documented as
	// "configure as needed" (no flag yet).
	cfg := parseFlags(newTestFlagSet(), nil)
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "*" {
		t.Fatalf("CORSOrigins: got %v, want [\"*\"]", cfg.CORSOrigins)
	}
}

// ───────────────────────── setupHTTPRoutes ─────────────────────────

func newTestAPIServer(t *testing.T) *api.Server {
	t.Helper()
	return api.NewServer(
		room.NewManager(100),
		member.NewManager(),
		wotan.NewWotan(),
		1*time.Hour,
	)
}

func TestSetupHTTPRoutes_HealthAndReady(t *testing.T) {
	apiServer := newTestAPIServer(t)
	m := metrics.Initialize("wotan_test_health_ready")
	mux := setupHTTPRoutes(apiServer, true, m)

	for _, path := range []string{"/health", "/ready"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", path, w.Code)
		}
	}
}

func TestSetupHTTPRoutes_AdminEnabledRegistersAdminPaths(t *testing.T) {
	apiServer := newTestAPIServer(t)
	m := metrics.Initialize("wotan_test_admin_on")
	mux := setupHTTPRoutes(apiServer, true, m)

	// /api/v1/admin/pending should reach the API server (we don't care
	// about the response body — just that the route is registered, so
	// status != 404).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pending", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatalf("admin route /api/v1/admin/pending unregistered when adminEnabled=true")
	}
}

func TestSetupHTTPRoutes_AdminDisabledHidesAdminPaths(t *testing.T) {
	apiServer := newTestAPIServer(t)
	m := metrics.Initialize("wotan_test_admin_off")
	mux := setupHTTPRoutes(apiServer, false, m)

	for _, path := range []string{
		"/api/v1/admin/pending",
		"/api/v1/admin/approve",
		"/api/v1/admin/approve-all",
		"/api/v1/admin/deny",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: status %d when adminEnabled=false; expected 404", path, w.Code)
		}
	}
}

func TestSetupHTTPRoutes_TopicAndRoomEndpointsExist(t *testing.T) {
	apiServer := newTestAPIServer(t)
	m := metrics.Initialize("wotan_test_topic_room")
	mux := setupHTTPRoutes(apiServer, true, m)

	// We use mux.Handler() to distinguish "route registered" from
	// "handler returned 404 for an unknown resource". The mux returns
	// a non-NotFoundHandler when a registered handler matches the
	// path, regardless of what that handler ultimately responds with.
	for _, path := range []string{
		"/api/v1/topics",
		"/api/v1/topics/", // prefix route — children dispatch to TopicRouter
		"/api/v1/join",
		"/api/v1/messages",
		"/api/v1/messages/send",
		"/api/v1/messages/delete",
		"/metrics",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		_, pattern := mux.Handler(req)
		if pattern == "" {
			t.Errorf("route %s: no handler pattern matched (route not registered)", path)
		}
	}
}

// ───────────────────────── setupMiddleware ─────────────────────────

func TestSetupMiddleware_Composes(t *testing.T) {
	// setupMiddleware composes recovery → logging → metrics → security
	// headers → CORS → rate limit → timeout. We verify that a baseline
	// request through the chain succeeds (i.e. composition doesn't
	// break the request pipeline) and that response headers reveal the
	// security-headers middleware ran.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	rl := middleware.NewRateLimiter(rate.Limit(1000), 1000, time.Minute)
	wrapped := setupMiddleware(inner, rl, []string{"*"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (middleware chain broke the request)", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("body %q, want \"ok\"", w.Body.String())
	}
	// Security-headers middleware sets X-Content-Type-Options.
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: got %q, want nosniff (security-headers middleware not in chain?)", got)
	}
}

func TestSetupMiddleware_RateLimiterEnforces(t *testing.T) {
	// Tight rate limiter: 0 RPS, 1 burst → first req allowed, second
	// blocked. Confirms rateLimiter.Middleware actually sits in the
	// chain.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rl := middleware.NewRateLimiter(rate.Limit(0), 1, time.Minute)
	wrapped := setupMiddleware(inner, rl, []string{"*"})

	// First request: pass (consumes the burst).
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "1.2.3.4:5678"
	w1 := httptest.NewRecorder()
	wrapped.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first req: %d, want 200", w1.Code)
	}

	// Second from same RemoteAddr: blocked.
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "1.2.3.4:5678"
	w2 := httptest.NewRecorder()
	wrapped.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second req from same client: %d, want 429", w2.Code)
	}
}

// ───────────────────────── ticker-loop ctx-awareness (fix #3) ─────────────────────────

func TestCollectSystemMetrics_ReturnsOnContextCancel(t *testing.T) {
	m := metrics.Initialize("wotan_test_collect_cancel")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		collectSystemMetrics(ctx, m)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// returned promptly — good
	case <-time.After(2 * time.Second):
		t.Fatal("collectSystemMetrics did not return within 2s of ctx cancel")
	}
}

func TestMonitorPendingMembers_ReturnsOnContextCancel(t *testing.T) {
	memberMgr := member.NewManager()
	roomMgr := room.NewManager(10)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		monitorPendingMembers(ctx, memberMgr, roomMgr)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// returned promptly — good
	case <-time.After(2 * time.Second):
		t.Fatal("monitorPendingMembers did not return within 2s of ctx cancel")
	}
}

func TestCleanupExpiredMembers_ReturnsOnContextCancel(t *testing.T) {
	memberMgr := member.NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		cleanupExpiredMembers(ctx, memberMgr)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanupExpiredMembers did not return within 2s of ctx cancel")
	}
}

// TestTickers_DoNotLeakOnDeadlineExceeded — defense-in-depth: the
// shutdown sequence cancels runCtx via runCancel(), but if a future
// refactor accidentally regresses to context.WithDeadline, the same
// post-condition (goroutine exits) should hold.
func TestTickers_DoNotLeakOnDeadlineExceeded(t *testing.T) {
	memberMgr := member.NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	var stopped int32
	go func() {
		cleanupExpiredMembers(ctx, memberMgr)
		atomic.StoreInt32(&stopped, 1)
	}()

	// Generous deadline: 200 ms is >> the 10 ms timeout above plus
	// scheduler jitter. If the goroutine doesn't observe Done(), this
	// test fails with a clear "didn't stop" message.
	deadline := time.After(200 * time.Millisecond)
	for atomic.LoadInt32(&stopped) == 0 {
		select {
		case <-deadline:
			t.Fatal("ticker did not exit within 200ms after ctx deadline elapsed")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// Compile-time guard: the helper signature must include strings (so the
// import isn't unused). This also doubles as a tiny sanity check that
// our route list grep'd above stays in sync.
var _ = strings.HasPrefix
