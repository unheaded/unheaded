// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package logagg

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/transport"
)

// Publisher is a zerolog.Hook that forwards log entries to Wotan.
// Each log entry is published to topic: logs.<service>.<level>
type Publisher struct {
	serviceName string
	conn        transport.Connection
	enabled     bool
}

// NewPublisher creates a new log publisher hook.
// If conn is nil, the publisher is disabled (logs are not forwarded).
func NewPublisher(serviceName string, conn transport.Connection) *Publisher {
	return &Publisher{
		serviceName: serviceName,
		conn:        conn,
		enabled:     conn != nil,
	}
}

// Run implements zerolog.Hook. It publishes the log entry to Wotan
// on a best-effort basis — errors are silently dropped to avoid
// cascading failures in the logging pipeline.
func (p *Publisher) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if !p.enabled {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Service:   p.serviceName,
		Level:     levelString(level),
		Message:   msg,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	topic := fmt.Sprintf("logs.%s.%s", p.serviceName, strings.ToLower(entry.Level))

	// Best-effort publish with short timeout — never block the logger
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Fire and forget — logging should never block application logic
	go func() {
		defer func() { recover() }() // guard against panics
		p.conn.Publish(ctx, topic, data)
	}()
}

// Enabled returns whether the publisher is actively forwarding logs.
func (p *Publisher) Enabled() bool {
	return p.enabled
}

// SetEnabled enables or disables log forwarding.
func (p *Publisher) SetEnabled(enabled bool) {
	p.enabled = enabled && p.conn != nil
}

// BufferHook is a zerolog.Hook that pushes log entries directly into a RingBuffer.
// Use this for local log aggregation (e.g., dashboard live tail).
type BufferHook struct {
	serviceName string
	buf         *RingBuffer
}

// NewBufferHook creates a hook that pushes to a local ring buffer.
func NewBufferHook(serviceName string, buf *RingBuffer) *BufferHook {
	return &BufferHook{serviceName: serviceName, buf: buf}
}

// Run implements zerolog.Hook.
func (h *BufferHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	h.buf.Push(LogEntry{
		Timestamp: time.Now(),
		Service:   h.serviceName,
		Level:     levelString(level),
		Message:   msg,
	})
}

// levelString converts a zerolog.Level to a string.
func levelString(level zerolog.Level) string {
	switch level {
	case zerolog.DebugLevel:
		return "debug"
	case zerolog.InfoLevel:
		return "info"
	case zerolog.WarnLevel:
		return "warn"
	case zerolog.ErrorLevel:
		return "error"
	case zerolog.FatalLevel:
		return "fatal"
	case zerolog.PanicLevel:
		return "panic"
	case zerolog.TraceLevel:
		return "trace"
	default:
		return "unknown"
	}
}
