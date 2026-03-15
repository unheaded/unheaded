// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logger

import (
	"bytes"
	"context"
	"testing"
)

func TestInitialize_DefaultOutput(t *testing.T) {
	// Should not panic with default config
	Initialize(Config{
		Level: "info",
	})
}

func TestInitialize_LevelsAccepted(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			buf := &bytes.Buffer{}
			Initialize(Config{
				Level:  level,
				Output: buf,
			})
		})
	}
}

func TestInitialize_PrettyPrint(t *testing.T) {
	buf := &bytes.Buffer{}
	Initialize(Config{
		Level:  "info",
		Pretty: true,
		Output: buf,
	})
}

func TestInitialize_WithCaller(t *testing.T) {
	buf := &bytes.Buffer{}
	Initialize(Config{
		Level:      "info",
		Output:     buf,
		WithCaller: true,
	})
}

func TestFromContext_Default(t *testing.T) {
	ctx := context.Background()
	logger := FromContext(ctx)
	if logger == nil {
		t.Fatal("FromContext returned nil for empty context")
	}
}

func TestWithContext_RoundTrip(t *testing.T) {
	buf := &bytes.Buffer{}
	Initialize(Config{
		Level:  "info",
		Output: buf,
	})

	ctx := context.Background()
	logger := FromContext(ctx)

	// Add logger to context
	ctx2 := WithContext(ctx, logger)

	// Retrieve it
	logger2 := FromContext(ctx2)
	if logger2 == nil {
		t.Fatal("expected non-nil logger from context")
	}
}

func TestWithRequestID(t *testing.T) {
	buf := &bytes.Buffer{}
	Initialize(Config{
		Level:  "info",
		Output: buf,
	})

	ctx := context.Background()
	ctx2, logger := WithRequestID(ctx, "req-123")
	if logger == nil {
		t.Fatal("WithRequestID returned nil logger")
	}
	if ctx2 == nil {
		t.Fatal("WithRequestID returned nil context")
	}

	// Verify logger was embedded in the context
	retrieved := FromContext(ctx2)
	if retrieved == nil {
		t.Fatal("expected to retrieve logger from context")
	}
}

func TestWithMember(t *testing.T) {
	buf := &bytes.Buffer{}
	Initialize(Config{
		Level:  "info",
		Output: buf,
	})

	ctx := context.Background()
	logger := FromContext(ctx)

	memberLogger := WithMember(logger, "member-42")
	if memberLogger == nil {
		t.Fatal("WithMember returned nil")
	}
}

func TestWithRoom(t *testing.T) {
	buf := &bytes.Buffer{}
	Initialize(Config{
		Level:  "info",
		Output: buf,
	})

	ctx := context.Background()
	logger := FromContext(ctx)

	roomLogger := WithRoom(logger, "room-alpha")
	if roomLogger == nil {
		t.Fatal("WithRoom returned nil")
	}
}

func TestContextFunctions(t *testing.T) {
	buf := &bytes.Buffer{}
	Initialize(Config{
		Level:  "debug",
		Output: buf,
	})

	ctx := context.Background()

	// These should not panic
	Info(ctx, "test info")
	Debug(ctx, "test debug")
	Warn(ctx, "test warn")
	Error(ctx, nil, "test error")

	// Verify something was written
	if buf.Len() == 0 {
		t.Error("expected log output to be written")
	}
}

func TestHTTPRequest(t *testing.T) {
	buf := &bytes.Buffer{}
	Initialize(Config{
		Level:  "info",
		Output: buf,
	})

	ctx := context.Background()
	logger := FromContext(ctx)

	HTTPRequest(logger, "GET", "/api/v1/health", "127.0.0.1:1234", 200, 50000000) // 50ms

	output := buf.String()
	if output == "" {
		t.Error("expected HTTP request log output")
	}
}
