// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package transport

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrNotConnected is returned when the transport is not connected.
	ErrNotConnected = errors.New("transport: not connected")
	// ErrAllTransportsFailed is returned when all transport attempts fail.
	ErrAllTransportsFailed = errors.New("transport: all transports failed")
)

// Connect establishes a connection using the configured transport cascade.
// For Auto mode: tries gRPC first, falls back to HTTP if enabled.
func Connect(ctx context.Context, cfg Config) (Connection, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	switch cfg.Type {
	case GRPC:
		return connectGRPC(ctx, cfg)
	case HTTP:
		return connectHTTP(ctx, cfg)
	case Auto:
		return connectAuto(ctx, cfg)
	default:
		return nil, fmt.Errorf("transport: unknown type %q", cfg.Type)
	}
}

// connectGRPC attempts a gRPC-only connection.
func connectGRPC(ctx context.Context, cfg Config) (Connection, error) {
	conn, err := newGRPCConnection(ctx, cfg.WotanGRPCAddr)
	if err != nil {
		return nil, fmt.Errorf("transport: gRPC connection to %s failed: %w", cfg.WotanGRPCAddr, err)
	}
	return conn, nil
}

// connectHTTP attempts an HTTP-only connection.
func connectHTTP(ctx context.Context, cfg Config) (Connection, error) {
	conn, err := newHTTPConnection(ctx, cfg.WotanHTTPAddr)
	if err != nil {
		return nil, fmt.Errorf("transport: HTTP connection to %s failed: %w", cfg.WotanHTTPAddr, err)
	}
	return conn, nil
}

// connectAuto tries gRPC first, then falls back to HTTP.
func connectAuto(ctx context.Context, cfg Config) (Connection, error) {
	// Try gRPC first
	grpcConn, grpcErr := newGRPCConnection(ctx, cfg.WotanGRPCAddr)
	if grpcErr == nil {
		return grpcConn, nil
	}

	// Fall back to HTTP if enabled
	if !cfg.FallbackEnabled {
		return nil, fmt.Errorf("transport: gRPC failed (%v), fallback disabled", grpcErr)
	}

	httpConn, httpErr := newHTTPConnection(ctx, cfg.WotanHTTPAddr)
	if httpErr == nil {
		return httpConn, nil
	}

	return nil, fmt.Errorf("%w: gRPC(%v), HTTP(%v)", ErrAllTransportsFailed, grpcErr, httpErr)
}
