// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collector

import (
	"context"
	"testing"
)

func TestNoOpGRPCClient_AllMethodsNoError(t *testing.T) {
	t.Parallel()
	c := NewNoOpGRPCClient()
	ctx := context.Background()

	if err := c.Send(ctx, &TelemetryEvent{
		SourcePod: "src-pod",
		DestPod:   "dst-pod",
		Verdict:   "allow",
		LatencyNs: 1_500_000,
		BytesSent: 1024,
		BytesRecv: 2048,
		Timestamp: 1_700_000_000_000_000_000,
	}); err != nil {
		t.Errorf("Send: %v", err)
	}

	if err := c.Stream(ctx); err != nil {
		t.Errorf("Stream: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestNoOpGRPCClient_SendAcceptsNilEvent(t *testing.T) {
	t.Parallel()
	// No-op should not panic on nil — defensive contract for test scaffolds.
	c := NewNoOpGRPCClient()
	if err := c.Send(context.Background(), nil); err != nil {
		t.Errorf("Send(nil): %v", err)
	}
}

func TestNewNoOpGRPCClient_SatisfiesInterface(t *testing.T) {
	t.Parallel()
	var _ GRPCClient = NewNoOpGRPCClient() //nolint:staticcheck // QF1011 false-positive: removing the type defeats the interface assertion
}
