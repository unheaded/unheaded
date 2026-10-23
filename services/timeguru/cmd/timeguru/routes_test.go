// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/services/timeguru/internal/api"
	"unheaded/services/timeguru/internal/storage"
)

// /api/v1/milestones/{id}/update answered 400 "milestone ID required" for
// every request: only "/milestones/" was trimmed from the path.
func TestMilestoneRoutes_APIPrefix(t *testing.T) {
	st, err := storage.NewStore(filepath.Join(t.TempDir(), "timeguru.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := handleMilestoneRoutes(api.NewHandler(st))
	for _, path := range []string{"/milestones/m1/update", "/api/v1/milestones/m1/update"} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"status":"done"}`)))
		if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "milestone ID required") {
			t.Errorf("%s: %d %q, want the update handler to see milestone m1", path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/api/v1/milestones/", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("no id: %d, want 400", w.Code)
	}
}
