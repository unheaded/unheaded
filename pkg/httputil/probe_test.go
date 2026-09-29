// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeMethods(t *testing.T) {
	calls := 0
	h := ProbeMethods(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	})
	for _, tc := range []struct {
		method string
		code   int
	}{
		{http.MethodGet, 200}, {http.MethodHead, 200},
		{http.MethodPost, 405}, {http.MethodDelete, 405}, {"BREW", 405},
	} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(tc.method, "/health", nil))
		if w.Code != tc.code {
			t.Errorf("%s: %d, want %d", tc.method, w.Code, tc.code)
		}
		if tc.code == 405 && w.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("%s: Allow = %q", tc.method, w.Header().Get("Allow"))
		}
	}
	if calls != 2 {
		t.Errorf("handler ran %d times, want 2 (GET, HEAD)", calls)
	}
}
