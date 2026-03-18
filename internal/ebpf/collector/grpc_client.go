// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collector

import (
	"context"
)

// TelemetryEvent represents a single eBPF telemetry observation.
// Flow: BPF ring buffer -> userspace collector -> gRPC -> Dashboard.
type TelemetryEvent struct {
	SourcePod string // Source pod/container name
	DestPod   string // Destination pod/container name
	Verdict   string // allow, deny, drop
	LatencyNs int64  // Latency in nanoseconds
	BytesSent uint64 // Bytes transmitted
	BytesRecv uint64 // Bytes received
	Timestamp int64  // Unix nanoseconds
}

// GRPCClient registers an eBPF collector with the service mesh
// and streams telemetry events to subscribed consumers.
//
// Contract:
//
//	Flow: BPF ring buffer -> userspace collector -> protobuf -> gRPC -> Busboy -> WebSocket -> Dashboard
//	Rate limiting: Configurable, default 100 events/sec
//	Sacred Principle: ZERO payload capture — metadata only (no user data)
//
// The default implementation is NoOpGRPCClient (safe no-op for dev/testing).
//
// To integrate real gRPC telemetry:
//  1. Import Busboy gRPC protobuf definitions
//  2. Create gRPC client stub
//  3. Connect to Busboy service
//  4. Read eBPF ring buffer events (from kernel)
//  5. Serialize events to protobuf TelemetryEvent
//  6. Stream to Busboy via gRPC
type GRPCClient interface {
	// Send transmits a telemetry event to the collector.
	Send(ctx context.Context, event *TelemetryEvent) error

	// Stream opens a bidirectional gRPC stream.
	Stream(ctx context.Context) error

	// Close closes the gRPC connection.
	Close() error
}

// NoOpGRPCClient is a no-op gRPC client for development/testing.
type NoOpGRPCClient struct{}

// Send is a no-op that returns nil.
func (n *NoOpGRPCClient) Send(_ context.Context, _ *TelemetryEvent) error {
	return nil
}

// Stream is a no-op that returns nil.
func (n *NoOpGRPCClient) Stream(_ context.Context) error {
	return nil
}

// Close is a no-op that returns nil.
func (n *NoOpGRPCClient) Close() error {
	return nil
}

// NewNoOpGRPCClient creates a no-op gRPC client.
func NewNoOpGRPCClient() GRPCClient {
	return &NoOpGRPCClient{}
}
