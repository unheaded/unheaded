// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Standard health check response helpers for Unheaded Kingdom services.
// Every service's /health endpoint MUST return this shape for platform consistency:
//
//	{
//	  "service": "wotan",
//	  "status": "healthy",
//	  "version": "0.1.0",
//	  "timestamp": "2026-02-10T01:25:57Z"
//	}
//
// Status constants (StatusHealthy, StatusUnhealthy, StatusDegraded) are defined
// in aggregator.go as type HealthStatus. Use string() to convert when needed.
package health

import (
	"encoding/json"
	"net/http"
	"time"
)

// Response is the standard health check response used by all Unheaded services.
// Every service's /health endpoint MUST return this shape for platform consistency.
type Response struct {
	Service   string       `json:"service"`
	Status    HealthStatus `json:"status"`
	Version   string       `json:"version"`
	Timestamp time.Time    `json:"timestamp"`
}

// WriteHealthy writes a standard healthy response.
func WriteHealthy(w http.ResponseWriter, service, version string) {
	writeHealthResponse(w, http.StatusOK, Response{
		Service:   service,
		Status:    StatusHealthy,
		Version:   version,
		Timestamp: time.Now().UTC(),
	})
}

// WriteUnhealthy writes a standard unhealthy response (503).
func WriteUnhealthy(w http.ResponseWriter, service, version string) {
	writeHealthResponse(w, http.StatusServiceUnavailable, Response{
		Service:   service,
		Status:    StatusUnhealthy,
		Version:   version,
		Timestamp: time.Now().UTC(),
	})
}

// WriteDegraded writes a standard degraded response (200 with degraded status).
func WriteDegraded(w http.ResponseWriter, service, version string) {
	writeHealthResponse(w, http.StatusOK, Response{
		Service:   service,
		Status:    StatusDegraded,
		Version:   version,
		Timestamp: time.Now().UTC(),
	})
}

func writeHealthResponse(w http.ResponseWriter, status int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
