// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package transport

import (
	"encoding/json"
	"net/http"
	"sync"
)

// HealthStatus represents the health state of a service.
type HealthStatus string

const (
	StatusHealthy  HealthStatus = "HEALTHY"
	StatusDegraded HealthStatus = "DEGRADED"
	StatusDown     HealthStatus = "DOWN"
)

// HealthServer provides dual-protocol health check registration.
// Every Unheaded service registers BOTH HTTP /health and gRPC health.
type HealthServer struct {
	ServiceName string
	mu          sync.RWMutex
	status      HealthStatus
	grpcOK      bool
	httpOK      bool
}

// NewHealthServer creates a new health server for the named service.
func NewHealthServer(serviceName string) *HealthServer {
	return &HealthServer{
		ServiceName: serviceName,
		status:      StatusHealthy,
		grpcOK:      true,
		httpOK:      true,
	}
}

// SetStatus sets the overall service health status.
func (h *HealthServer) SetStatus(status HealthStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = status
}

// SetGRPCStatus sets the gRPC transport health.
func (h *HealthServer) SetGRPCStatus(ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.grpcOK = ok
	h.updateStatus()
}

// SetHTTPStatus sets the HTTP transport health.
func (h *HealthServer) SetHTTPStatus(ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.httpOK = ok
	h.updateStatus()
}

// updateStatus derives status from transport states. Must be called with lock held.
func (h *HealthServer) updateStatus() {
	switch {
	case h.grpcOK && h.httpOK:
		h.status = StatusHealthy
	case !h.grpcOK && !h.httpOK:
		h.status = StatusDown
	default:
		h.status = StatusDegraded
	}
}

// Status returns the current health status.
func (h *HealthServer) Status() HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

// healthResponse is the JSON response for the HTTP health endpoint.
type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	GRPC    bool   `json:"grpc"`
	HTTP    bool   `json:"http"`
}

// RegisterHTTP registers the HTTP health check endpoint on the given mux.
func (h *HealthServer) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/ready", h.handleReady)
}

// handleHealth implements the HTTP /health endpoint.
func (h *HealthServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	status := h.status
	grpc := h.grpcOK
	httpOK := h.httpOK
	h.mu.RUnlock()

	resp := healthResponse{
		Status:  string(status),
		Service: h.ServiceName,
		GRPC:    grpc,
		HTTP:    httpOK,
	}

	w.Header().Set("Content-Type", "application/json")
	switch status {
	case StatusHealthy:
		w.WriteHeader(http.StatusOK)
	case StatusDegraded:
		w.WriteHeader(http.StatusOK) // 200 but status field says DEGRADED
	case StatusDown:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleReady implements the HTTP /ready endpoint.
func (h *HealthServer) handleReady(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	status := h.status
	h.mu.RUnlock()

	if status == StatusDown {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"ready":false}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ready":true}`))
}
