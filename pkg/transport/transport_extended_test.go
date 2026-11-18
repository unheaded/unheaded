// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// ---------------------------------------------------------------------------
// httpConnection tests
// ---------------------------------------------------------------------------

func TestHTTPConnection_Type(t *testing.T) {
	hc := &httpConnection{}
	if hc.Type() != HTTP {
		t.Errorf("httpConnection.Type() = %q, want %q", hc.Type(), HTTP)
	}
}

func TestHTTPConnection_Healthy(t *testing.T) {
	hc := &httpConnection{}
	// Default atomic.Bool is false
	if hc.Healthy() {
		t.Error("new httpConnection should not be healthy by default")
	}
	hc.healthy.Store(true)
	if !hc.Healthy() {
		t.Error("httpConnection.Healthy() should return true after storing true")
	}
}

func TestHTTPConnection_Publish_NilClient(t *testing.T) {
	hc := &httpConnection{} // client is nil
	err := hc.Publish(context.Background(), "test.topic", []byte("hello"))
	if err != ErrNotConnected {
		t.Errorf("Publish with nil client: got %v, want ErrNotConnected", err)
	}
}

func TestHTTPConnection_Subscribe_NilClient(t *testing.T) {
	hc := &httpConnection{} // client is nil
	_, err := hc.Subscribe(context.Background(), "test.topic")
	if err != ErrNotConnected {
		t.Errorf("Subscribe with nil client: got %v, want ErrNotConnected", err)
	}
}

func TestHTTPConnection_Close_NilClient(t *testing.T) {
	hc := &httpConnection{} // client is nil
	err := hc.Close()
	if err != nil {
		t.Errorf("Close with nil client: got %v, want nil", err)
	}
}

func TestHTTPConnection_Close_SetsUnhealthy(t *testing.T) {
	// Create a real HTTP connection via the constructor so client is non-nil.
	// We need a running HTTP server for NewClient to accept the addr.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Strip the http:// prefix — wotanClient.NewClient prepends it.
	addr := srv.Listener.Addr().String()
	hc, err := wrapForTest(addr)
	if err != nil {
		t.Fatalf("wrapForTest: %v", err)
	}

	if !hc.Healthy() {
		t.Error("new httpConnection should be healthy after construction")
	}

	err = hc.Close()
	if err != nil {
		t.Errorf("Close: %v", err)
	}

	if hc.Healthy() {
		t.Error("httpConnection should be unhealthy after Close")
	}

	// Close again should be safe (nil client path)
	err = hc.Close()
	if err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// HealthServer: concurrent access (race detector coverage)
// ---------------------------------------------------------------------------

func TestHealthServer_ConcurrentAccess(t *testing.T) {
	hs := NewHealthServer("race-test")
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			hs.SetGRPCStatus(i%2 == 0)
			hs.SetHTTPStatus(i%3 == 0)
			hs.SetStatus(StatusDegraded)
			_ = hs.Status()
		}
	}()

	for i := 0; i < 100; i++ {
		hs.SetGRPCStatus(i%3 == 0)
		hs.SetHTTPStatus(i%2 == 0)
		_ = hs.Status()
	}

	<-done
}

// ---------------------------------------------------------------------------
// HealthServer: ReadyEndpoint with Degraded status (should be ready)
// ---------------------------------------------------------------------------

func TestHealthServer_ReadyEndpoint_Degraded(t *testing.T) {
	hs := NewHealthServer("myservice")
	hs.SetStatus(StatusDegraded)

	mux := http.NewServeMux()
	hs.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("degraded ready status code = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != `{"ready":true}` {
		t.Errorf("body = %q, want %q", body, `{"ready":true}`)
	}
}

// ---------------------------------------------------------------------------
// HealthServer: /health response fields for degraded
// ---------------------------------------------------------------------------

func TestHealthServer_HTTPHealthEndpoint_DegradedFields(t *testing.T) {
	hs := NewHealthServer("svc-degraded")
	hs.SetHTTPStatus(false) // HTTP down, gRPC up → DEGRADED

	mux := http.NewServeMux()
	hs.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp healthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "DEGRADED" {
		t.Errorf("status = %q, want DEGRADED", resp.Status)
	}
	if resp.Service != "svc-degraded" {
		t.Errorf("service = %q, want svc-degraded", resp.Service)
	}
	if resp.GRPC != nil {
		t.Errorf("GRPC = %v, want null: nothing set it (ADR-147, Developer D18)", *resp.GRPC)
	}
	if resp.HTTP {
		t.Error("HTTP should be false")
	}
}

// ---------------------------------------------------------------------------
// ConfigFromEnv: partial env override
// ---------------------------------------------------------------------------

func TestConfigFromEnv_PartialOverride(t *testing.T) {
	cfg := DefaultConfig()
	t.Setenv("TRANSPORT_TYPE", "grpc")
	// Leave other vars unset

	ConfigFromEnv(&cfg)

	if cfg.Type != GRPC {
		t.Errorf("Type = %q, want %q", cfg.Type, GRPC)
	}
	// Other fields should keep defaults
	if cfg.WotanGRPCAddr != "localhost:18001" {
		t.Errorf("WotanGRPCAddr = %q, want default", cfg.WotanGRPCAddr)
	}
	if cfg.WotanHTTPAddr != "http://localhost:18000" {
		t.Errorf("WotanHTTPAddr = %q, want default", cfg.WotanHTTPAddr)
	}
	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("ConnectTimeout = %v, want default 5s", cfg.ConnectTimeout)
	}
}

// ---------------------------------------------------------------------------
// Connection interface compliance
// ---------------------------------------------------------------------------

func TestHTTPConnectionImplementsConnection(t *testing.T) {
	var _ Connection = (*httpConnection)(nil)
}

func TestHTTPConnection_Publish_CancelledContext_NilClient(t *testing.T) {
	hc := &httpConnection{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := hc.Publish(ctx, "t", []byte("d"))
	if err != ErrNotConnected {
		t.Errorf("got %v, want ErrNotConnected", err)
	}
}

func TestHTTPConnection_Subscribe_CancelledContext_NilClient(t *testing.T) {
	hc := &httpConnection{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := hc.Subscribe(ctx, "t")
	if err != ErrNotConnected {
		t.Errorf("got %v, want ErrNotConnected", err)
	}
}

// ---------------------------------------------------------------------------
// newHTTPConnection error path: empty addr triggers wotan-client error
// ---------------------------------------------------------------------------

// wrapForTest is WrapHTTPClient over a client for addr, as logagg.Connect
// builds it (the HTTP and gRPC connection constructors went with ADR-147
// Phase 7: a library takes the process's client, it does not build one).
func wrapForTest(addr string) (*httpConnection, error) {
	c, err := wotanClient.NewClient(addr)
	if err != nil {
		return nil, err
	}
	return WrapHTTPClient(c, addr).(*httpConnection), nil
}

// ---------------------------------------------------------------------------
// httpConnection: Publish error path (non-nil client, but publish fails)
// ---------------------------------------------------------------------------

func TestHTTPConnection_Publish_Error(t *testing.T) {
	// Create a mock Wotan HTTP server that returns errors for publish.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 500 for all requests to simulate publish failure.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"code":"INTERNAL","message":"test error"}}`))
	}))
	defer srv.Close()

	addr := srv.Listener.Addr().String()
	hc, err := wrapForTest(addr)
	if err != nil {
		t.Fatalf("wrapForTest: %v", err)
	}
	defer hc.Close()

	// Publish will fail because the wotan client requires subscription first.
	err = hc.Publish(context.Background(), "test.topic", []byte("hello"))
	if err == nil {
		t.Fatal("Publish should fail when not subscribed")
	}
	// After a publish error, healthy should be false.
	if hc.Healthy() {
		t.Error("httpConnection should be unhealthy after publish error")
	}
}

// ---------------------------------------------------------------------------
// httpConnection: Subscribe error path (non-nil client, but subscribe fails)
// ---------------------------------------------------------------------------

func TestHTTPConnection_Subscribe_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"code":"INTERNAL","message":"test error"}}`))
	}))
	defer srv.Close()

	addr := srv.Listener.Addr().String()
	hc, err := wrapForTest(addr)
	if err != nil {
		t.Fatalf("wrapForTest: %v", err)
	}
	defer hc.Close()

	_, err = hc.Subscribe(context.Background(), "test.topic")
	if err == nil {
		t.Fatal("Subscribe should fail when not subscribed on wotan client")
	}
	if hc.Healthy() {
		t.Error("httpConnection should be unhealthy after subscribe error")
	}
}
