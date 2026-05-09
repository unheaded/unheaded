// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNoOpTelemetryHandler_Returns501(t *testing.T) {
	t.Parallel()
	h := NewNoOpTelemetryHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws/telemetry", nil)
	h.Handle(w, r)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not available") {
		t.Errorf("expected message about availability, got %q", w.Body.String())
	}
}

func TestNewNoOpTelemetryHandler_ReturnsValidImplementation(t *testing.T) {
	t.Parallel()
	h := NewNoOpTelemetryHandler()
	if h == nil {
		t.Fatal("nil handler returned")
	}
	// Verify interface satisfaction at compile time + runtime no-panic.
	var _ TelemetryHandler = h
}
