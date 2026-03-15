// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewServiceMetrics(t *testing.T) {
	// Use a unique name to avoid Prometheus duplicate registration
	m := NewServiceMetrics("test_svc_" + t.Name())
	if m == nil {
		t.Fatal("nil")
	}
	if m.RequestsTotal == nil {
		t.Error("RequestsTotal nil")
	}
	if m.RequestDuration == nil {
		t.Error("RequestDuration nil")
	}
}

func TestStatusRecorder_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: w, status: 200}

	rec.WriteHeader(http.StatusNotFound)
	if rec.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.status)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("underlying writer code = %d, want 404", w.Code)
	}
}

func TestInstrumentHandler(t *testing.T) {
	m := NewServiceMetrics("test_instrument_" + t.Name())
	var handlerCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	instrumented := InstrumentHandler(m, handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	instrumented(w, req)

	if !handlerCalled {
		t.Error("handler not called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestInstrumentHandler_CustomStatus(t *testing.T) {
	m := NewServiceMetrics("test_custom_status_" + t.Name())
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	instrumented := InstrumentHandler(m, handler)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/items", nil)
	instrumented(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestRequestError_Error(t *testing.T) {
	err := &RequestError{
		Status:  400,
		Code:    "BAD",
		Message: "bad request",
	}
	if err.Error() != "bad request" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestHandleRequestError_RequestError(t *testing.T) {
	w := httptest.NewRecorder()
	err := &RequestError{
		Status:  http.StatusBadRequest,
		Code:    "INVALID",
		Message: "invalid input",
	}
	HandleRequestError(w, err)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != "INVALID" {
		t.Errorf("error code = %v", resp.Error)
	}
}

func TestHandleRequestError_GenericError(t *testing.T) {
	w := httptest.NewRecorder()
	HandleRequestError(w, fmt.Errorf("something broke"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestIsRequestError_NotRequestError(t *testing.T) {
	_, ok := IsRequestError(fmt.Errorf("plain error"))
	if ok {
		t.Error("should return false for plain error")
	}
}

func TestDecodeJSONBody_ValidJSON(t *testing.T) {
	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	var dst struct {
		Name string `json:"name"`
	}
	if err := DecodeJSONBody(req, 0, &dst); err != nil {
		t.Fatalf("DecodeJSONBody: %v", err)
	}
	if dst.Name != "test" {
		t.Errorf("Name = %q", dst.Name)
	}
}
