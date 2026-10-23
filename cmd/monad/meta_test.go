// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
