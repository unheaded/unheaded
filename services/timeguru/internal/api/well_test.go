// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/services/timeguru/internal/storage"
)

// Without the Well timeguru serves the file and refuses writes with 503
// {"error":"the Well is unavailable"}; /health says "well":"waiting".
func TestWellDownAnswers503(t *testing.T) {
	st := storage.New()
	if err := st.Load(context.Background(), createMockTimeline(), time.Now()); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(st)
	h.SetWellHealth(func() any { return st.WellState() })

	w := httptest.NewRecorder()
	h.HandleGetTimeline(w, httptest.NewRequest(http.MethodGet, "/timeline", nil))
	if w.Code != http.StatusOK {
		t.Errorf("GET /timeline without the Well: %d, want 200 (the file)", w.Code)
	}

	id := createMockTimeline().Milestones[0].ID
	req := httptest.NewRequest(http.MethodPost, "/milestones/"+id+"/update", strings.NewReader(`{"progress":10,"status":"in_progress"}`))
	w = httptest.NewRecorder()
	h.HandleUpdateMilestone(w, req, id)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != http.StatusServiceUnavailable || body["error"] != "the Well is unavailable" {
		t.Errorf("update without the Well: %d %s, want 503 the Well is unavailable", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.HandleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	body = nil
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != http.StatusOK || body["well"] != "waiting" {
		t.Errorf("/health = %d %s, want 200 with well waiting", w.Code, w.Body.String())
	}
}
