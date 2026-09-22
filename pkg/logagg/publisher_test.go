// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"context"
	"encoding/json"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/logger"
	"unheaded/pkg/transport"
)

// mockConnection implements transport.Connection for testing.
type mockConnection struct {
	mu        sync.Mutex
	published []publishedMsg
	healthy   bool
}

type publishedMsg struct {
	topic string
	data  []byte
}

func (m *mockConnection) Type() transport.Type { return "mock" }

func (m *mockConnection) Publish(_ context.Context, topic string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, publishedMsg{topic: topic, data: data})
	return nil
}

func (m *mockConnection) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	return nil, nil
}

func (m *mockConnection) Close() error { return nil }

func (m *mockConnection) Healthy() bool { return m.healthy }

func TestPublisher_NilConnection(t *testing.T) {
	p := NewPublisher("test", nil)
	if p.Enabled() {
		t.Error("publisher with nil conn should be disabled")
	}
	// Should not panic
	p.Run(nil, zerolog.InfoLevel, "hello")
}

func TestPublisher_Enabled(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := NewPublisher("test", conn)
	if !p.Enabled() {
		t.Error("publisher with conn should be enabled")
	}
}

func TestPublisher_SetEnabled(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := NewPublisher("test", conn)

	p.SetEnabled(false)
	if p.Enabled() {
		t.Error("SetEnabled(false) should disable")
	}

	p.SetEnabled(true)
	if !p.Enabled() {
		t.Error("SetEnabled(true) should enable")
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level zerolog.Level
		want  string
	}{
		{zerolog.DebugLevel, "debug"},
		{zerolog.InfoLevel, "info"},
		{zerolog.WarnLevel, "warn"},
		{zerolog.ErrorLevel, "error"},
		{zerolog.FatalLevel, "fatal"},
		{zerolog.PanicLevel, "panic"},
		{zerolog.TraceLevel, "trace"},
	}
	for _, tt := range tests {
		if got := levelString(tt.level); got != tt.want {
			t.Errorf("levelString(%v) = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func (m *mockConnection) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.published)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

// The old publisher cancelled its context before the fire-and-forget
// goroutine ran, so nothing ever reached the transport. Prove it does now.
func TestPublisher_Delivers(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := NewPublisher("svc", conn)
	defer p.Close()

	p.Run(nil, zerolog.WarnLevel, "hello")
	waitFor(t, func() bool { return conn.count() == 1 })

	conn.mu.Lock()
	got := conn.published[0]
	conn.mu.Unlock()
	if got.topic != "logs.svc.warn" {
		t.Errorf("topic = %q, want logs.svc.warn", got.topic)
	}
	var entry LogEntry
	if err := json.Unmarshal(got.data, &entry); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if entry.Message != "hello" || entry.Service != "svc" || entry.Level != "warn" {
		t.Errorf("entry = %+v", entry)
	}
	if p.Dropped() != 0 {
		t.Errorf("dropped = %d, want 0", p.Dropped())
	}
}

// blockingConnection holds every Publish until released.
type blockingConnection struct {
	mockConnection
	release chan struct{}
}

func (b *blockingConnection) Publish(ctx context.Context, topic string, data []byte) error {
	<-b.release // deliberately ignores ctx: models a transport that hangs
	return b.mockConnection.Publish(ctx, topic, data)
}

// A stalled transport must never block the logger or grow goroutines:
// entries beyond the queue depth are dropped and counted.
func TestPublisher_DropsWhenQueueFull(t *testing.T) {
	conn := &blockingConnection{release: make(chan struct{})}
	p := NewPublisherWithQueue("svc", conn, 4)
	defer p.Close()

	before := runtime.NumGoroutine()
	// 4 queued (+1 in flight if the worker already woke) — the rest dropped.
	for i := 0; i < 25; i++ {
		p.Run(nil, zerolog.InfoLevel, "x")
	}
	dropped := p.Dropped()
	if dropped < 20 || dropped > 21 {
		t.Fatalf("dropped = %d, want 20 or 21", dropped)
	}
	if grew := runtime.NumGoroutine() - before; grew > 1 {
		t.Errorf("goroutines grew by %d; the publisher must not spawn per entry", grew)
	}

	close(conn.release)
	waitFor(t, func() bool { return conn.count() == int(25-dropped) })
}

func TestPublisher_CloseStopsForwarding(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := NewPublisher("svc", conn)
	p.Close()
	p.Close() // idempotent

	if p.Enabled() {
		t.Error("closed publisher should report disabled")
	}
	p.SetEnabled(true)
	if p.Enabled() {
		t.Error("SetEnabled(true) after Close must not re-enable")
	}
	p.Run(nil, zerolog.InfoLevel, "late")
	time.Sleep(20 * time.Millisecond)
	if conn.count() != 0 {
		t.Errorf("published %d after Close, want 0", conn.count())
	}
}

func TestPublisher_LoggerHook(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := NewPublisher("svc", conn)
	defer p.Close()

	l := logger.New(io.Discard).AddHook(p.LoggerHook())
	l.Error().Msg("boom")
	waitFor(t, func() bool { return conn.count() == 1 })

	conn.mu.Lock()
	got := conn.published[0]
	conn.mu.Unlock()
	if got.topic != "logs.svc.error" {
		t.Errorf("topic = %q, want logs.svc.error", got.topic)
	}
	var entry LogEntry
	if err := json.Unmarshal(got.data, &entry); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if entry.Message != "boom" || entry.Level != "error" {
		t.Errorf("entry = %+v", entry)
	}
}
