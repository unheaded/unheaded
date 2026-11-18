// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func accessLogged(t *testing.T, path string, status int) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	l := zerolog.New(&buf).Level(zerolog.DebugLevel)
	h := AccessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}), l, func(context.Context) string { return "req-1" })
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("%s: no JSON line: %q", path, buf.String())
	}
	return line
}

// A probe answered 200 is a debug line: probes were most of west's log
// lines (ADR-147, Developer D15: 247 lines a minute). A real request stays
// at info.
func TestProbePathsAtDebug(t *testing.T) {
	for _, p := range []string{"/health", "/ready", "/live", "/metrics"} {
		if got := accessLogged(t, p, http.StatusOK)["level"]; got != "debug" {
			t.Errorf("%s 200 logged at %v, want debug", p, got)
		}
	}
	if got := accessLogged(t, "/api/v1/tasks", http.StatusOK)["level"]; got != "info" {
		t.Errorf("/api/v1/tasks logged at %v, want info", got)
	}
}

// A probe that fails is worth reading.
func TestFailingProbeStillWarns(t *testing.T) {
	if got := accessLogged(t, "/health", http.StatusServiceUnavailable)["level"]; got != "warn" {
		t.Errorf("/health 503 logged at %v, want warn", got)
	}
}

// The fields kanban's request log wrote: method, path, status, duration,
// remote address and request id, under the message "request".
func TestAccessLogFields(t *testing.T) {
	line := accessLogged(t, "/api/v1/tasks", http.StatusCreated)
	for k, want := range map[string]any{
		"method": "GET", "path": "/api/v1/tasks", "status": 201.0, "request_id": "req-1", "message": "request",
	} {
		if line[k] != want {
			t.Errorf("%s = %v, want %v", k, line[k], want)
		}
	}
	for _, k := range []string{"duration", "remote_addr"} {
		if _, ok := line[k]; !ok {
			t.Errorf("no %s in %v", k, line)
		}
	}
}
