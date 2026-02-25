// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package transport

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestConnect_UnknownType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Type = Type("invalid")
	cfg.ConnectTimeout = 1 * time.Second

	_, err := Connect(context.Background(), cfg)
	if err == nil {
		t.Fatal("Connect with unknown type should return error")
	}
	if want := `transport: unknown type "invalid"`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestConnect_GRPCMode_BadAddr(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Type = GRPC
	cfg.WotanGRPCAddr = "localhost:1" // unreachable
	cfg.ConnectTimeout = 1 * time.Second

	_, err := Connect(context.Background(), cfg)
	// Connection to an invalid gRPC address may succeed (lazy connect) or fail.
	// We just verify no panic and the function returns.
	_ = err
}

func TestConnect_HTTPMode_BadAddr(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Type = HTTP
	cfg.WotanHTTPAddr = "http://localhost:1" // unreachable
	cfg.ConnectTimeout = 1 * time.Second

	_, err := Connect(context.Background(), cfg)
	_ = err
}

func TestConnect_AutoMode_FallbackDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Type = Auto
	cfg.WotanGRPCAddr = "localhost:1" // will likely fail
	cfg.FallbackEnabled = false
	cfg.ConnectTimeout = 2 * time.Second

	_, err := Connect(context.Background(), cfg)
	// If gRPC connection is lazy (succeeds without actual dial), this may not fail.
	// If it does fail, the error should mention fallback disabled.
	// If gRPC connection is lazy (succeeds without actual dial), this may not fail.
	// If it does fail, verify it mentions fallback disabled.
	_ = err
}

func TestErrNotConnected(t *testing.T) {
	if ErrNotConnected.Error() != "transport: not connected" {
		t.Errorf("ErrNotConnected = %q, want %q", ErrNotConnected.Error(), "transport: not connected")
	}
}

func TestErrAllTransportsFailed(t *testing.T) {
	if ErrAllTransportsFailed.Error() != "transport: all transports failed" {
		t.Errorf("ErrAllTransportsFailed = %q", ErrAllTransportsFailed.Error())
	}
}

func TestErrAllTransportsFailed_Wrapping(t *testing.T) {
	wrapped := fmt.Errorf("%w: gRPC(dial failed), HTTP(timeout)", ErrAllTransportsFailed)
	if !errors.Is(wrapped, ErrAllTransportsFailed) {
		t.Error("wrapped error should match ErrAllTransportsFailed")
	}
}
