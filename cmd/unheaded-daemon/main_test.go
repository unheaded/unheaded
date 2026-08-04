// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"unheaded/pkg/logger"
	"unheaded/pkg/transport"
)

// newTestDaemon builds a Daemon with the same collaborators main() gives it,
// minus anything that talks to the network.
func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()
	return NewDaemon(
		&Config{NodeID: "test", NodeName: "test", HTTPAddr: ":0", GRPCAddr: ":0"},
		logger.New(io.Discard),
		transport.DefaultConfig(),
		transport.NewHealthServer("unheaded-daemon-test"),
	)
}

// registerHandlers registered /health and /ready twice — once via
// healthSrv.RegisterHTTP and again directly. http.ServeMux panics on a
// conflicting pattern (Go 1.22+), so the daemon aborted during Start() before
// it could listen. Nothing caught it: this package had no test file at all, so
// `go test ./...` ran nothing here.
func TestRegisterHandlers_NoDuplicatePatterns(t *testing.T) {
	d := newTestDaemon(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registerHandlers panicked, daemon cannot start: %v", r)
		}
	}()

	d.registerHandlers(http.NewServeMux())
}

// The endpoints CLAUDE.md requires of every service must actually answer, and
// /health must report the transport's real dual-protocol state rather than a
// hardcoded "healthy".
func TestRegisterHandlers_HealthAndReadyRespond(t *testing.T) {
	d := newTestDaemon(t)
	mux := http.NewServeMux()
	d.registerHandlers(mux)

	// The two endpoints have different payloads: /health carries the transport's
	// dual-protocol view, /ready is a bare readiness flag.
	for path, wantKey := range map[string]string{"/health": "service", "/ready": "ready"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 200 or 503", path, rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: body is not JSON: %v", path, err)
			continue
		}
		if _, ok := body[wantKey]; !ok {
			t.Errorf("%s: response has no %q field: %v", path, wantKey, body)
		}
	}
}

// The daemon reports gRPC state through healthSrv; /health must reflect a change
// to it. A hardcoded handler would pass the test above but fail this one.
func TestHealth_ReflectsTransportState(t *testing.T) {
	d := newTestDaemon(t)
	mux := http.NewServeMux()
	d.registerHandlers(mux)

	read := func() map[string]any {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("/health body is not JSON: %v", err)
		}
		return body
	}

	d.healthSrv.SetGRPCStatus(true)
	if got := read()["grpc"]; got != true {
		t.Errorf("after SetGRPCStatus(true), /health grpc = %v, want true", got)
	}

	d.healthSrv.SetGRPCStatus(false)
	if got := read()["grpc"]; got != false {
		t.Errorf("after SetGRPCStatus(false), /health grpc = %v, want false", got)
	}
}
