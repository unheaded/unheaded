// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_RequiresName(t *testing.T) {
	_, err := New(Config{Port: "9999"})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestNew_RequiresPort(t *testing.T) {
	_, err := New(Config{Name: "test"})
	if err == nil {
		t.Fatal("expected error for missing port")
	}
}

func TestNew_Defaults(t *testing.T) {
	svc, err := New(Config{Name: "test", Port: "9999"})
	if err != nil {
		t.Fatal(err)
	}
	if svc.config.ShutdownTimeout != 30*time.Second {
		t.Errorf("expected 30s shutdown timeout, got %v", svc.config.ShutdownTimeout)
	}
	if svc.config.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", svc.config.Version)
	}
}

func TestHealthEndpoint(t *testing.T) {
	svc, err := New(Config{Name: "test-svc", Port: "9999", Version: "2.0.0"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	svc.mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["service"] != "test-svc" {
		t.Errorf("expected service=test-svc, got %s", resp["service"])
	}
	if resp["version"] != "2.0.0" {
		t.Errorf("expected version=2.0.0, got %s", resp["version"])
	}
}

func TestReadyEndpoint_NotReady(t *testing.T) {
	svc, err := New(Config{Name: "test", Port: "9999"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	svc.mux.ServeHTTP(w, req)

	if w.Code != 503 {
		t.Errorf("expected 503 when not ready, got %d", w.Code)
	}
}

func TestReadyEndpoint_Ready(t *testing.T) {
	svc, err := New(Config{Name: "test", Port: "9999"})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetReady(true)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	svc.mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 when ready, got %d", w.Code)
	}
}

func TestReadyEndpoint_CustomReadyFunc(t *testing.T) {
	svc, err := New(Config{
		Name:      "test",
		Port:      "9999",
		ReadyFunc: func() bool { return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetReady(true) // base ready, but ReadyFunc returns false

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	svc.mux.ServeHTTP(w, req)

	if w.Code != 503 {
		t.Errorf("expected 503 when ReadyFunc returns false, got %d", w.Code)
	}
}

func TestHandle(t *testing.T) {
	svc, err := New(Config{Name: "test", Port: "9999"})
	if err != nil {
		t.Fatal(err)
	}

	called := false
	svc.Handle("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()
	svc.mux.ServeHTTP(w, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestRunShutdown(t *testing.T) {
	svc, err := New(Config{
		Name:            "test",
		Port:            "0", // OS-assigned port
		ShutdownTimeout: 1 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("unexpected error on shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timed out")
	}
}
