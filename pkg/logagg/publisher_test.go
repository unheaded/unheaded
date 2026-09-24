// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
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

// A transport error is not a drop: the entry left the queue but never landed.
// Counted separately so "we dropped nothing" cannot hide a dead link.
func TestPublisher_PublishErrorsCountedSeparately(t *testing.T) {
	conn := &failingConnection{}
	p := NewPublisher("failing-svc", conn)
	defer p.Close()

	p.Run(nil, zerolog.ErrorLevel, "boom")
	waitFor(t, func() bool { return p.Failed() == 1 })
	if p.Published() != 0 {
		t.Errorf("Published() = %d, want 0 — a failed publish is not a publish", p.Published())
	}
	if p.Dropped() != 0 {
		t.Errorf("Dropped() = %d, want 0 — a transport error is not a queue drop", p.Dropped())
	}
}

// failingConnection always fails to publish.
type failingConnection struct{ mockConnection }

func (f *failingConnection) Publish(context.Context, string, []byte) error {
	return errors.New("transport down")
}

// The counters are only useful if a service can actually serve them. Register
// them in a pkg/metrics registry — the kind nine of the ten services build —
// and assert the rendered exposition carries the live values.
func TestPublisher_CollectorsRenderIntoARegistry(t *testing.T) {
	conn := &blockingConnection{release: make(chan struct{})}
	p := NewPublisherWithQueue("metrics-svc", conn, 2)
	defer p.Close()

	reg := metrics.NewRegistry()
	for _, c := range p.Collectors() {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	for i := 0; i < 10; i++ {
		p.Run(nil, zerolog.InfoLevel, "x")
	}
	waitFor(t, func() bool { return p.Dropped() > 0 })

	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatalf("gather registry: %v", err)
	}
	out := buf.String()

	want := fmt.Sprintf("unheaded_logagg_entries_dropped_total{service=%q} %d", "metrics-svc", p.Dropped())
	if !strings.Contains(out, want) {
		t.Errorf("exposition missing %q\ngot:\n%s", want, out)
	}
	for _, name := range []string{
		"unheaded_logagg_entries_published_total",
		"unheaded_logagg_publish_errors_total",
	} {
		if !strings.Contains(out, name) {
			t.Errorf("exposition missing %s", name)
		}
	}

	// Registry.Gather writes # HELP and # TYPE itself. A collector that also
	// writes them produces duplicates, which the text format forbids.
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "# HELP ") {
			continue
		}
		if n := strings.Count(out, line); n != 1 {
			t.Errorf("%q appears %d times, want 1 (duplicate HELP is invalid exposition)", line, n)
		}
	}
	close(conn.release)
}

// Both views over the publisher's atomics must report the same numbers: a
// drop counted in one and not the other is the drift this test exists to
// catch. (A third, client_golang view went with ADR-094 step 4.)
func TestPublisher_BothViewsAgree(t *testing.T) {
	conn := &blockingConnection{release: make(chan struct{})}
	p := NewPublisherWithQueue("two-views", conn, 2)
	defer p.Close()

	for i := 0; i < 9; i++ {
		p.Run(nil, zerolog.InfoLevel, "x")
	}
	waitFor(t, func() bool { return p.Dropped() > 0 })
	dropped := p.Dropped()

	// 1. pkg/metrics registry
	reg := metrics.NewRegistry()
	for _, c := range p.Collectors() {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	var regBuf bytes.Buffer
	if err := reg.Gather(&regBuf); err != nil {
		t.Fatalf("gather: %v", err)
	}

	// 2. hand-rolled text
	var textBuf bytes.Buffer
	if err := p.WriteMetrics(&textBuf); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}

	want := fmt.Sprintf(`unheaded_logagg_entries_dropped_total{service="two-views"} %d`, dropped)
	for name, out := range map[string]string{"registry": regBuf.String(), "text": textBuf.String()} {
		if !strings.Contains(out, want) {
			t.Errorf("%s view missing %q\ngot:\n%s", name, want, out)
		}
	}

	close(conn.release)
}
