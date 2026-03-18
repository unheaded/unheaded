// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dashboard

import (
	"net/http"
)

// TelemetryHandler describes the interface for streaming eBPF telemetry
// to WebSocket clients in real time.
//
// Contract: Handle(w, r) accepts a WebSocket upgrade request and streams
// TelemetryEvent messages to connected clients. Canvas mapping:
//   - source_pod -> source node
//   - dest_pod -> dest node
//   - verdict -> arrow color (green=allow, red=deny, yellow=drop)
//   - latency_ns -> arrow thickness
//
// The default implementation is NoOpTelemetryHandler (safe no-op for dev/testing).
//
// To integrate real eBPF telemetry:
//  1. Implement this interface with real Busboy/gRPC integration
//  2. Create ring buffer reader from kernel eBPF programs
//  3. Deserialize TelemetryEvent protobuf messages
//  4. Apply rate limiting (default: 100 events/sec)
//  5. Broadcast to all connected WebSocket clients
//  6. Swap in: router.HandleFunc("/ws/telemetry", realHandler.Handle)
type TelemetryHandler interface {
	// Handle upgrades an HTTP connection to WebSocket and streams telemetry events.
	Handle(w http.ResponseWriter, r *http.Request)
}

// NoOpTelemetryHandler is a no-op implementation that returns 501 Not Implemented.
// Use in development/testing environments without eBPF support.
type NoOpTelemetryHandler struct{}

// Handle returns 501 Not Implemented (no-op).
func (n *NoOpTelemetryHandler) Handle(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "eBPF telemetry not available", http.StatusNotImplemented)
}

// NewNoOpTelemetryHandler creates a no-op telemetry handler.
func NewNoOpTelemetryHandler() TelemetryHandler {
	return &NoOpTelemetryHandler{}
}
