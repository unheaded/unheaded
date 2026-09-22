// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/transport"
)

// Publisher is a zerolog.Hook that forwards log entries to Wotan.
// Each log entry is published to topic: logs.<service>.<level>
//
// Entries go through a bounded queue drained by a single worker goroutine.
// When the queue is full the entry is dropped and counted — the logger is
// never blocked and the process never grows a goroutine per log line.
type Publisher struct {
	serviceName string
	conn        transport.Connection
	enabled     atomic.Bool
	queue       chan LogEntry
	dropped     atomic.Uint64
	published   atomic.Uint64
	failed      atomic.Uint64
	done        chan struct{}
	closeOnce   sync.Once
}

// DefaultQueueSize is the number of log entries the publisher buffers
// before it starts dropping.
const DefaultQueueSize = 1024

// publishTimeout bounds a single publish so a stalled Wotan cannot wedge
// the worker (and therefore fill the queue) indefinitely.
const publishTimeout = 500 * time.Millisecond

// NewPublisher creates a new log publisher hook.
// If conn is nil, the publisher is disabled (logs are not forwarded).
func NewPublisher(serviceName string, conn transport.Connection) *Publisher {
	return NewPublisherWithQueue(serviceName, conn, DefaultQueueSize)
}

// NewPublisherWithQueue is NewPublisher with an explicit queue depth.
// A depth < 1 is treated as 1.
func NewPublisherWithQueue(serviceName string, conn transport.Connection, depth int) *Publisher {
	if depth < 1 {
		depth = 1
	}
	p := &Publisher{
		serviceName: serviceName,
		conn:        conn,
		queue:       make(chan LogEntry, depth),
		done:        make(chan struct{}),
	}
	p.enabled.Store(conn != nil)
	if conn != nil {
		go p.worker()
	}
	return p
}

// Run implements zerolog.Hook. It enqueues the entry for the worker on a
// best-effort basis — a full queue drops the entry and bumps Dropped().
func (p *Publisher) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	p.enqueue(levelString(level), msg)
}

// enqueue is the logger-agnostic entry point shared by the zerolog and
// pkg/logger adapters.
func (p *Publisher) enqueue(level, msg string) {
	if !p.enabled.Load() {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Service:   p.serviceName,
		Level:     level,
		Message:   msg,
	}

	select {
	case p.queue <- entry:
	default:
		p.dropped.Add(1)
	}
}

// worker drains the queue until Close. Publish errors are swallowed —
// logging must never cascade into the logging pipeline.
func (p *Publisher) worker() {
	for {
		select {
		case <-p.done:
			return
		case entry := <-p.queue:
			p.publish(entry)
		}
	}
}

func (p *Publisher) publish(entry LogEntry) {
	defer func() { _ = recover() }() // guard against panics in the transport

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	topic := fmt.Sprintf("logs.%s.%s", p.serviceName, strings.ToLower(entry.Level))

	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	if err := p.conn.Publish(ctx, topic, data); err != nil {
		p.failed.Add(1)
		return
	}
	p.published.Add(1)
}

// Close stops the worker. Entries still queued are discarded. Safe to call
// more than once; Run after Close drops everything.
func (p *Publisher) Close() {
	p.closeOnce.Do(func() {
		p.enabled.Store(false)
		close(p.done)
	})
}

// Dropped returns how many entries were discarded because the queue was full.
func (p *Publisher) Dropped() uint64 {
	return p.dropped.Load()
}

// Published returns how many entries reached the transport successfully.
func (p *Publisher) Published() uint64 {
	return p.published.Load()
}

// Failed returns how many publishes the transport rejected. These are not
// drops — the entry left the queue — but the log never arrived, so a link
// that is down cannot hide behind a zero drop count.
func (p *Publisher) Failed() uint64 {
	return p.failed.Load()
}

// Enabled returns whether the publisher is actively forwarding logs.
func (p *Publisher) Enabled() bool {
	return p.enabled.Load()
}

// SetEnabled enables or disables log forwarding.
func (p *Publisher) SetEnabled(enabled bool) {
	select {
	case <-p.done:
		enabled = false // no worker left to drain the queue
	default:
	}
	p.enabled.Store(enabled && p.conn != nil)
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
