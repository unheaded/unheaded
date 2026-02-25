// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHealthServer(t *testing.T) {
	hs := NewHealthServer("test-service")
	if hs.ServiceName != "test-service" {
		t.Errorf("ServiceName = %q, want %q", hs.ServiceName, "test-service")
	}
	if hs.Status() != StatusHealthy {
		t.Errorf("initial status = %q, want %q", hs.Status(), StatusHealthy)
	}
}

func TestHealthServer_SetStatus(t *testing.T) {
	hs := NewHealthServer("test")
	hs.SetStatus(StatusDegraded)
	if hs.Status() != StatusDegraded {
		t.Errorf("status = %q, want %q", hs.Status(), StatusDegraded)
	}
	hs.SetStatus(StatusDown)
	if hs.Status() != StatusDown {
		t.Errorf("status = %q, want %q", hs.Status(), StatusDown)
	}
}

func TestHealthServer_DerivedStatus(t *testing.T) {
	tests := []struct {
		name   string
		grpc   bool
		http   bool
		expect HealthStatus
	}{
		{"both healthy", true, true, StatusHealthy},
		{"grpc down", false, true, StatusDegraded},
		{"http down", true, false, StatusDegraded},
		{"both down", false, false, StatusDown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := NewHealthServer("test")
			hs.SetGRPCStatus(tt.grpc)
			hs.SetHTTPStatus(tt.http)
			if hs.Status() != tt.expect {
				t.Errorf("status = %q, want %q", hs.Status(), tt.expect)
			}
		})
	}
}

func TestHealthServer_HTTPHealthEndpoint_Healthy(t *testing.T) {
	hs := NewHealthServer("myservice")
	mux := http.NewServeMux()
	hs.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp healthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "HEALTHY" {
		t.Errorf("resp.Status = %q, want %q", resp.Status, "HEALTHY")
	}
	if resp.Service != "myservice" {
		t.Errorf("resp.Service = %q, want %q", resp.Service, "myservice")
	}
	if !resp.GRPC || !resp.HTTP {
		t.Errorf("resp.GRPC=%v, resp.HTTP=%v, want both true", resp.GRPC, resp.HTTP)
	}
}

func TestHealthServer_HTTPHealthEndpoint_Down(t *testing.T) {
	hs := NewHealthServer("myservice")
	hs.SetGRPCStatus(false)
	hs.SetHTTPStatus(false)

	mux := http.NewServeMux()
	hs.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp healthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "DOWN" {
		t.Errorf("resp.Status = %q, want %q", resp.Status, "DOWN")
	}
}

func TestHealthServer_HTTPHealthEndpoint_Degraded(t *testing.T) {
	hs := NewHealthServer("myservice")
	hs.SetGRPCStatus(false) // gRPC down, HTTP still up

	mux := http.NewServeMux()
	hs.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("degraded status code = %d, want %d", w.Code, http.StatusOK)
	}
	var resp healthResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "DEGRADED" {
		t.Errorf("resp.Status = %q, want %q", resp.Status, "DEGRADED")
	}
}

func TestHealthServer_ReadyEndpoint_Healthy(t *testing.T) {
	hs := NewHealthServer("myservice")
	mux := http.NewServeMux()
	hs.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != `{"ready":true}` {
		t.Errorf("body = %q, want %q", body, `{"ready":true}`)
	}
}

func TestHealthServer_ReadyEndpoint_Down(t *testing.T) {
	hs := NewHealthServer("myservice")
	hs.SetStatus(StatusDown)

	mux := http.NewServeMux()
	hs.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if body := w.Body.String(); body != `{"ready":false}` {
		t.Errorf("body = %q, want %q", body, `{"ready":false}`)
	}
}

func TestHealthStatusConstants(t *testing.T) {
	if string(StatusHealthy) != "HEALTHY" {
		t.Error("StatusHealthy mismatch")
	}
	if string(StatusDegraded) != "DEGRADED" {
		t.Error("StatusDegraded mismatch")
	}
	if string(StatusDown) != "DOWN" {
		t.Error("StatusDown mismatch")
	}
}
