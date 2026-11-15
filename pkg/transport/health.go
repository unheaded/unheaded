// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package transport

import (
	"net/http"
	"sync"
	"unheaded/pkg/httputil"
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
	// grpcOK is nil until SetGRPCStatus is called: a server nobody told
	// about gRPC says nothing ("grpc": null) instead of claiming it
	// (ADR-147, Developer D18; it said true by default).
	grpcOK *bool
	httpOK bool
}

// NewHealthServer creates a new health server for the named service.
func NewHealthServer(serviceName string) *HealthServer {
	return &HealthServer{
		ServiceName: serviceName,
		status:      StatusHealthy,
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
	h.grpcOK = &ok
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
// An unknown gRPC state is not a failure.
func (h *HealthServer) updateStatus() {
	grpcUp := h.grpcOK == nil || *h.grpcOK
	switch {
	case grpcUp && h.httpOK:
		h.status = StatusHealthy
	case !grpcUp && !h.httpOK:
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
	GRPC    *bool  `json:"grpc"` // null until the service sets it
	HTTP    bool   `json:"http"`
}

// RegisterHTTP registers the HTTP health check endpoint on the given mux.
func (h *HealthServer) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/health", httputil.ProbeMethods(h.handleHealth))
	mux.HandleFunc("/ready", httputil.ProbeMethods(h.handleReady))
}

// handleHealth implements the HTTP /health endpoint.
func (h *HealthServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	status := h.status
	var grpc *bool
	if h.grpcOK != nil {
		v := *h.grpcOK
		grpc = &v
	}
	httpOK := h.httpOK
	h.mu.RUnlock()

	resp := healthResponse{
		Status:  string(status),
		Service: h.ServiceName,
		GRPC:    grpc,
		HTTP:    httpOK,
	}

	code := http.StatusOK // DEGRADED too: 200, and the status field says so
	if status == StatusDown {
		code = http.StatusServiceUnavailable
	}
	httputil.WriteJSON(w, code, resp)
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
