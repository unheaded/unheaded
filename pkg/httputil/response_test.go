// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	WriteSuccess(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}
	if resp.Error != nil {
		t.Error("expected error to be nil")
	}
	if resp.Meta == nil {
		t.Error("expected meta to be non-nil")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "INVALID_INPUT", "title is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Success {
		t.Error("expected success to be false")
	}
	if resp.Error == nil {
		t.Fatal("expected error to be non-nil")
	}
	if resp.Error.Code != "INVALID_INPUT" {
		t.Errorf("expected error code INVALID_INPUT, got %s", resp.Error.Code)
	}
	if resp.Error.Message != "title is required" {
		t.Errorf("expected error message 'title is required', got %s", resp.Error.Message)
	}
}

func TestWriteErrorWithDetails(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErrorWithDetails(w, http.StatusInternalServerError, "DB_ERROR", "database failed", "connection refused")

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error.Details != "connection refused" {
		t.Errorf("expected details 'connection refused', got %s", resp.Error.Details)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]string{"status": "healthy"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var data map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if data["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %s", data["status"])
	}
}

// A value json cannot encode is the server's failure, and the client must see
// it as one. json.Encoder writing straight to the ResponseWriter left an empty
// 200, or the handler's own status with an empty body.
func TestWriteJSON_UnencodableValueIs500(t *testing.T) {
	for name, v := range map[string]any{
		"channel":  make(chan int),
		"nan":      map[string]float64{"cpu": math.NaN()},
		"infinity": []float64{1, math.Inf(1)},
		"late":     []any{"fine", "fine", make(chan int)},
	} {
		for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusNotFound} {
			rec := httptest.NewRecorder()
			WriteJSON(rec, status, v)
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("%s at %d: code = %d, want 500", name, status, rec.Code)
			}
			assertEncodeFailure(t, rec)
		}
	}
}

func TestWriteSuccess_UnencodableValueIs500(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteSuccess(rec, http.StatusOK, map[string]float64{"cpu": math.NaN()})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
	assertEncodeFailure(t, rec)
}

func assertEncodeFailure(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var r Response
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body, err)
	}
	if r.Success || r.Error == nil || r.Error.Code != "internal_error" {
		t.Fatalf("body = %s, want the error envelope", rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
}

// What a handler that moved here sends is byte for byte what json.Encoder
// sent: same escaping, same trailing newline, same status.
func TestWriteJSON_MatchesTheEncoder(t *testing.T) {
	v := map[string]any{"html": "<a&b>", "n": 1.5, "list": []string{"x"}}
	var want bytes.Buffer
	if err := json.NewEncoder(&want).Encode(v); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusAccepted, v)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("code = %d", rec.Code)
	}
	if rec.Body.String() != want.String() {
		t.Fatalf("body = %q, want %q", rec.Body, want.String())
	}
}

// A handler that named its own JSON type keeps it.
func TestWriteJSON_KeepsAContentTypeAlreadySet(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/problem+json")
	WriteJSON(rec, http.StatusBadRequest, map[string]string{"title": "no"})
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q", got)
	}
}
