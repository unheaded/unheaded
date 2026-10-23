// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"unheaded/services/timeguru/internal/storage"
)

// With the real store and no timeline.md (east, 2026-10-23), /timeline
// answered 500: api.ErrNotFound was its own errors.New and never matched
// the store's. And readiness has to say there is nothing to serve.
func TestNoTimeline_404AndNotReady(t *testing.T) {
	st, err := storage.NewStore(filepath.Join(t.TempDir(), "timeguru.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := NewHandler(st)

	w := httptest.NewRecorder()
	h.HandleGetTimeline(w, httptest.NewRequest(http.MethodGet, "/timeline", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /timeline with no timeline: %d, want 404", w.Code)
	}
	w = httptest.NewRecorder()
	h.HandleGetTimelineWithFormat(w, httptest.NewRequest(http.MethodGet, "/timeline", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /timeline (format) with no timeline: %d, want 404", w.Code)
	}
	w = httptest.NewRecorder()
	h.HandleReady(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("/ready with no timeline: %d, want 503", w.Code)
	}
}

// Sync and import write; a GET used to run them.
func TestSyncImport_RequirePost(t *testing.T) {
	st, err := storage.NewStore(filepath.Join(t.TempDir(), "timeguru.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := NewHandler(st)
	for name, fn := range map[string]http.HandlerFunc{"sync": h.HandleSync, "import": h.HandleImport} {
		w := httptest.NewRecorder()
		fn(w, httptest.NewRequest(http.MethodGet, "/api/v1/timeline/"+name, nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: %d, want 405", name, w.Code)
		}
	}
}
