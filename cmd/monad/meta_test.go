// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A response's meta is about that request: its own id, and how long it
// took. duration_ms was 0 on every response, and request_id was whatever the
// shared counter held when the body was encoded, so a request that another
// one overtook reported the other's id.
func TestResponseMeta_IsThisRequests(t *testing.T) {
	hs := newTestServer(t)
	h := hs.loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hs.getNextRequestID() // a second request arrives meanwhile
		time.Sleep(5 * time.Millisecond)
		if r.URL.Path == "/fail" {
			hs.writeError(w, http.StatusBadRequest, "X", "x", nil)
			return
		}
		hs.writeSuccess(w, http.StatusOK, nil)
	}))
	for i, path := range []string{"/ok", "/fail"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		var resp struct {
			Meta struct {
				RequestID  string  `json:"request_id"`
				DurationMS float64 `json:"duration_ms"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		// Requests 1 and 3 are ours; 2 and 4 the ones that arrived meanwhile.
		if want := []string{"req_1", "req_3"}[i]; resp.Meta.RequestID != want {
			t.Errorf("%s: request_id = %q, want %q", path, resp.Meta.RequestID, want)
		}
		if resp.Meta.DurationMS < 5 {
			t.Errorf("%s: duration_ms = %v, want >= 5 (the handler slept 5ms)", path, resp.Meta.DurationMS)
		}
	}
}

// A name with no handler ran nothing: 404, and not an executed operation.
// It used to answer 400 OPERATION_FAILED and add to
// monad_operations_executed_total.
func TestExecuteOperation_UnknownNameIsNotExecuted(t *testing.T) {
	hs := newTestServer(t)
	hs.service.RegisterQuery("known", func(context.Context, interface{}) (interface{}, error) {
		return nil, errors.New("ran and failed")
	})
	for _, tc := range []struct {
		body     string
		code     int
		executed int64
	}{
		{`{"name":"nope","type":"query"}`, http.StatusNotFound, 0},
		{`{"name":"nope","type":"mutation"}`, http.StatusNotFound, 0},
		{`{"name":"nope","type":"effect"}`, http.StatusNotFound, 0},
		{`{"name":"known","type":"query"}`, http.StatusBadRequest, 1},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/operations", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		hs.operationsHandler(w, r)
		if w.Code != tc.code {
			t.Errorf("%s: status %d, want %d", tc.body, w.Code, tc.code)
		}
		hs.metrics.mu.RLock()
		got := hs.metrics.OperationsExecuted
		hs.metrics.mu.RUnlock()
		if got != tc.executed {
			t.Errorf("%s: operations executed = %d, want %d", tc.body, got, tc.executed)
		}
	}
}
