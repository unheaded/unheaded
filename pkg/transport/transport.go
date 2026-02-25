// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package transport provides a unified transport abstraction for Unheaded services.
// It implements the gRPC-first transport cascade:
//   1. Primary: Wotan gRPC streaming (port 18001)
//   2. Fallback: HTTP (port 18000)
//
// Every service uses transport.Connect() to establish a connection and
// transport.NewHealthServer() for dual-protocol health checks.
package transport

import (
	"context"
	"time"
)

// Type represents a transport protocol.
type Type string

const (
	GRPC Type = "grpc"
	HTTP Type = "http"
	Auto Type = "auto" // Try gRPC first, cascade to HTTP
)

// Config holds transport configuration for a service.
type Config struct {
	// Type is the preferred transport type (default: Auto)
	Type Type
	// WotanGRPCAddr is the Wotan gRPC endpoint (default: localhost:18001)
	WotanGRPCAddr string
	// WotanHTTPAddr is the Wotan HTTP endpoint (default: http://localhost:18000)
	WotanHTTPAddr string
	// ConnectTimeout for initial connection (default: 5s)
	ConnectTimeout time.Duration
	// FallbackEnabled allows falling back from gRPC to HTTP (default: true)
	FallbackEnabled bool
}

// DefaultConfig returns sensible defaults for the transport configuration.
func DefaultConfig() Config {
	return Config{
		Type:            Auto,
		WotanGRPCAddr:   "localhost:18001",
		WotanHTTPAddr:   "http://localhost:18000",
		ConnectTimeout:  5 * time.Second,
		FallbackEnabled: true,
	}
}

// Connection represents an active transport connection to Wotan.
type Connection interface {
	// Type returns the active transport type.
	Type() Type
	// Publish sends a message to a Wotan topic.
	Publish(ctx context.Context, topic string, data []byte) error
	// Subscribe listens to a Wotan topic and returns a channel of messages.
	Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
	// Close tears down the connection.
	Close() error
	// Healthy returns true if the transport is operational.
	Healthy() bool
}
