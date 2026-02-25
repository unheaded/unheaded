// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package logger

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/audit"
)

// --- mock storage ---

type mockStorage struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
	byID   map[string]*audit.AuditEvent
	closed bool
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		events: make([]*audit.AuditEvent, 0),
		byID:   make(map[string]*audit.AuditEvent),
	}
}

func (m *mockStorage) Store(ctx context.Context, event *audit.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	m.byID[event.ID] = event
	return nil
}

func (m *mockStorage) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
	for _, e := range events {
		if err := m.Store(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockStorage) Get(ctx context.Context, id string) (*audit.AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ev, ok := m.byID[id]; ok {
		return ev, nil
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (m *mockStorage) Query(ctx context.Context, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*audit.AuditEvent, len(m.events))
	copy(result, m.events)
	return result, nil
}

func (m *mockStorage) GetLastHash(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return "", fmt.Errorf("no events")
	}
	return m.events[len(m.events)-1].Hash, nil
}

func (m *mockStorage) Count(ctx context.Context, query *audit.AuditQuery) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.events)), nil
}

func (m *mockStorage) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockStorage) eventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// --- helpers ---

func makeValidEvent(id string) *audit.AuditEvent {
	return &audit.AuditEvent{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Type:      audit.EventTypeSystem,
		Severity:  audit.SeverityInfo,
		Action:    "test.action",
		Outcome:   audit.OutcomeSuccess,
		Actor:     &audit.Actor{ID: "actor-1", Type: "user", Name: "Test"},
		Hash:      "hash-" + id,
	}
}

func newTestLogger(t *testing.T, storage *mockStorage) *Logger {
	t.Helper()
	cfg := &audit.Config{
		BufferSize:            10,
		FlushInterval:         50 * time.Millisecond,
		AsyncWorkers:          2,
		EnableTamperDetection: false,
	}
	l, err := New(cfg, storage, nil)
	if err != nil {
		t.Fatalf("New logger failed: %v", err)
	}
	return l
}

// ===================== Buffer Tests =====================

func TestNewBuffer(t *testing.T) {
	b := NewBuffer(50, time.Second, nil)
	if b.Capacity() != 50 {
		t.Errorf("expected capacity 50, got %d", b.Capacity())
	}
	if b.Size() != 0 {
		t.Errorf("expected size 0, got %d", b.Size())
	}
}

func TestNewBuffer_Defaults(t *testing.T) {
	b := NewBuffer(0, 0, nil)
	if b.Capacity() != 100 {
		t.Errorf("expected default capacity 100, got %d", b.Capacity())
	}
	if b.flushInterval != 5*time.Second {
		t.Errorf("expected default flush interval 5s, got %v", b.flushInterval)
	}
}

func TestBuffer_Add(t *testing.T) {
	var flushed []*audit.AuditEvent
	b := NewBuffer(100, time.Minute, func(events []*audit.AuditEvent) error {
		flushed = append(flushed, events...)
		return nil
	})

	ev := makeValidEvent("b1")
	if err := b.Add(ev); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if b.Size() != 1 {
		t.Errorf("expected size 1, got %d", b.Size())
	}
}

func TestBuffer_AutoFlush_OnCapacity(t *testing.T) {
	var mu sync.Mutex
	var flushed []*audit.AuditEvent
	b := NewBuffer(3, time.Minute, func(events []*audit.AuditEvent) error {
		mu.Lock()
		flushed = append(flushed, events...)
		mu.Unlock()
		return nil
	})

	for i := 0; i < 3; i++ {
		_ = b.Add(makeValidEvent(fmt.Sprintf("af%d", i)))
	}

	mu.Lock()
	count := len(flushed)
	mu.Unlock()

	if count != 3 {
		t.Errorf("expected 3 flushed events, got %d", count)
	}
	if b.Size() != 0 {
		t.Errorf("expected size 0 after auto-flush, got %d", b.Size())
	}
}

func TestBuffer_ManualFlush(t *testing.T) {
	var flushed []*audit.AuditEvent
	b := NewBuffer(100, time.Minute, func(events []*audit.AuditEvent) error {
		flushed = append(flushed, events...)
		return nil
	})

	_ = b.Add(makeValidEvent("mf1"))
	_ = b.Add(makeValidEvent("mf2"))

	if err := b.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if len(flushed) != 2 {
		t.Errorf("expected 2 flushed events, got %d", len(flushed))
	}
	if b.Size() != 0 {
		t.Errorf("expected size 0, got %d", b.Size())
	}
}

func TestBuffer_FlushEmpty(t *testing.T) {
	b := NewBuffer(10, time.Minute, nil)
	if err := b.Flush(); err != nil {
		t.Fatalf("Flush of empty buffer failed: %v", err)
	}
}

func TestBuffer_NilFlushFn(t *testing.T) {
	b := NewBuffer(2, time.Minute, nil)
	_ = b.Add(makeValidEvent("nf1"))
	_ = b.Add(makeValidEvent("nf2"))
	// Should not panic even with nil flushFn
	if b.Size() != 0 {
		// After capacity reached, flushLocked runs but flushFn is nil, so it's fine
		// size should be 0 because events were flushed
		t.Logf("size after capacity flush: %d", b.Size())
	}
}

func TestBuffer_StartStop(t *testing.T) {
	var mu sync.Mutex
	flushCount := 0
	b := NewBuffer(100, 50*time.Millisecond, func(events []*audit.AuditEvent) error {
		mu.Lock()
		flushCount++
		mu.Unlock()
		return nil
	})

	b.Start()
	_ = b.Add(makeValidEvent("ss1"))
	time.Sleep(120 * time.Millisecond) // Wait for timer flush

	if err := b.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestBuffer_Stats(t *testing.T) {
	b := NewBuffer(50, 3*time.Second, nil)
	_ = b.Add(makeValidEvent("st1"))

	stats := b.Stats()
	if stats.Size != 1 {
		t.Errorf("expected size 1, got %d", stats.Size)
	}
	if stats.Capacity != 50 {
		t.Errorf("expected capacity 50, got %d", stats.Capacity)
	}
	if stats.FlushInterval != 3*time.Second {
		t.Errorf("expected flush interval 3s, got %v", stats.FlushInterval)
	}
}

// ===================== AsyncProcessor Tests =====================

func TestNewAsyncProcessor(t *testing.T) {
	p := NewAsyncProcessor(4, func(e *audit.AuditEvent) error { return nil })
	if p.workers != 4 {
		t.Errorf("expected 4 workers, got %d", p.workers)
	}
}

func TestNewAsyncProcessor_DefaultWorkers(t *testing.T) {
	p := NewAsyncProcessor(0, func(e *audit.AuditEvent) error { return nil })
	if p.workers != 1 {
		t.Errorf("expected default 1 worker, got %d", p.workers)
	}
}

func TestAsyncProcessor_SubmitAndProcess(t *testing.T) {
	var processed int64
	p := NewAsyncProcessor(2, func(e *audit.AuditEvent) error {
		atomic.AddInt64(&processed, 1)
		return nil
	})

	p.Start()

	for i := 0; i < 10; i++ {
		ok := p.Submit(makeValidEvent(fmt.Sprintf("ap%d", i)))
		if !ok {
			t.Errorf("Submit returned false for event %d", i)
		}
	}

	p.Stop()

	count := atomic.LoadInt64(&processed)
	if count != 10 {
		t.Errorf("expected 10 processed events, got %d", count)
	}
	if p.ProcessedCount() != 10 {
		t.Errorf("expected ProcessedCount 10, got %d", p.ProcessedCount())
	}
}

func TestAsyncProcessor_ErrorCounting(t *testing.T) {
	callCount := int64(0)
	p := NewAsyncProcessor(1, func(e *audit.AuditEvent) error {
		n := atomic.AddInt64(&callCount, 1)
		if n%2 == 0 {
			return fmt.Errorf("error")
		}
		return nil
	})

	p.Start()

	for i := 0; i < 6; i++ {
		p.Submit(makeValidEvent(fmt.Sprintf("ec%d", i)))
	}

	p.Stop()

	if p.ErrorCount() != 3 {
		t.Errorf("expected 3 errors, got %d", p.ErrorCount())
	}
}

func TestAsyncProcessor_SubmitAfterStop(t *testing.T) {
	p := NewAsyncProcessor(1, func(e *audit.AuditEvent) error { return nil })
	p.Start()
	p.Stop()

	ok := p.Submit(makeValidEvent("post-stop"))
	if ok {
		t.Error("expected Submit to return false after Stop")
	}
}

func TestAsyncProcessor_DoubleStop(t *testing.T) {
	p := NewAsyncProcessor(1, func(e *audit.AuditEvent) error { return nil })
	p.Start()
	p.Stop()
	p.Stop() // Should not panic
}

func TestAsyncProcessor_QueueSize(t *testing.T) {
	p := NewAsyncProcessor(2, func(e *audit.AuditEvent) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	// Before Start, queue size should be 0
	if p.QueueSize() != 0 {
		t.Errorf("expected queue size 0, got %d", p.QueueSize())
	}
}

func TestAsyncProcessor_Stats(t *testing.T) {
	p := NewAsyncProcessor(3, func(e *audit.AuditEvent) error { return nil })
	p.Start()
	p.Submit(makeValidEvent("stats1"))
	p.Stop()

	stats := p.Stats()
	if stats.Workers != 3 {
		t.Errorf("expected 3 workers, got %d", stats.Workers)
	}
	if stats.ProcessedCount != 1 {
		t.Errorf("expected 1 processed, got %d", stats.ProcessedCount)
	}
}

// ===================== Logger Tests =====================

func TestNew_NilStorage(t *testing.T) {
	_, err := New(nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil storage")
	}
}

func TestNew_DefaultConfig(t *testing.T) {
	storage := newMockStorage()
	l, err := New(nil, storage, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer l.Close()

	if l.config.BufferSize != 1000 {
		t.Errorf("expected default buffer size 1000, got %d", l.config.BufferSize)
	}
}

func TestLogger_Log(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	defer l.Close()

	ev := makeValidEvent("log1")
	if err := l.Log(context.Background(), ev); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Wait for async processing
	time.Sleep(200 * time.Millisecond)

	if storage.eventCount() < 1 {
		t.Error("expected at least 1 event stored")
	}
}

func TestLogger_Log_GeneratesID(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	defer l.Close()

	ev := &audit.AuditEvent{
		Timestamp: time.Now().UTC(),
		Type:      audit.EventTypeSystem,
		Action:    "test",
		Actor:     &audit.Actor{ID: "a1"},
	}

	if err := l.Log(context.Background(), ev); err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if ev.ID == "" {
		t.Error("expected ID to be generated")
	}
}

func TestLogger_Log_SetsTimestamp(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	defer l.Close()

	ev := &audit.AuditEvent{
		ID:     "ts1",
		Type:   audit.EventTypeSystem,
		Action: "test",
		Actor:  &audit.Actor{ID: "a1"},
	}

	before := time.Now().UTC()
	if err := l.Log(context.Background(), ev); err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	after := time.Now().UTC()

	if ev.Timestamp.Before(before) || ev.Timestamp.After(after) {
		t.Errorf("timestamp %v not in range [%v, %v]", ev.Timestamp, before, after)
	}
}

func TestLogger_Log_ValidationError(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	defer l.Close()

	// Missing required fields
	ev := &audit.AuditEvent{
		ID:        "invalid",
		Timestamp: time.Now().UTC(),
	}
	err := l.Log(context.Background(), ev)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLogger_Log_Closed(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	l.Close()

	err := l.Log(context.Background(), makeValidEvent("closed"))
	if err == nil {
		t.Fatal("expected error when logging to closed logger")
	}
}

func TestLogger_LogBatch(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	defer l.Close()

	events := []*audit.AuditEvent{
		makeValidEvent("batch1"),
		makeValidEvent("batch2"),
		makeValidEvent("batch3"),
	}

	if err := l.LogBatch(context.Background(), events); err != nil {
		t.Fatalf("LogBatch failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if storage.eventCount() < 3 {
		t.Errorf("expected at least 3 events, got %d", storage.eventCount())
	}
}

func TestLogger_Query(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	defer l.Close()

	ev := makeValidEvent("q1")
	_ = l.Log(context.Background(), ev)
	time.Sleep(200 * time.Millisecond)

	events, err := l.Query(context.Background(), &audit.AuditQuery{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) < 1 {
		t.Error("expected at least 1 event from query")
	}
}

func TestLogger_Query_Closed(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	l.Close()

	_, err := l.Query(context.Background(), &audit.AuditQuery{})
	if err == nil {
		t.Fatal("expected error querying closed logger")
	}
}

func TestLogger_Export_Closed(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	l.Close()

	_, err := l.Export(context.Background(), "json", nil)
	if err == nil {
		t.Fatal("expected error exporting from closed logger")
	}
}

func TestLogger_VerifyIntegrity_NotEnabled(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	defer l.Close()

	_, err := l.VerifyIntegrity(context.Background(), "a", "b")
	if err == nil {
		t.Fatal("expected error when tamper detection is not enabled")
	}
}

func TestLogger_VerifyIntegrity_Closed(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	l.Close()

	_, err := l.VerifyIntegrity(context.Background(), "a", "b")
	if err == nil {
		t.Fatal("expected error for closed logger")
	}
}

func TestLogger_Close_Idempotent(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)

	if err := l.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second Close should not error: %v", err)
	}
}

func TestLogger_Stats(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	defer l.Close()

	stats := l.Stats()
	if stats.BufferCapacity != 10 {
		t.Errorf("expected buffer capacity 10, got %d", stats.BufferCapacity)
	}
	if stats.Workers != 2 {
		t.Errorf("expected 2 workers, got %d", stats.Workers)
	}
}

func TestLogger_TamperDetection(t *testing.T) {
	storage := newMockStorage()
	cfg := &audit.Config{
		BufferSize:            10,
		FlushInterval:         50 * time.Millisecond,
		AsyncWorkers:          1,
		EnableTamperDetection: true,
	}
	l, err := New(cfg, storage, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer l.Close()

	ev := makeValidEvent("td1")
	if err := l.Log(context.Background(), ev); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	if ev.Hash == "" {
		t.Error("expected hash to be set with tamper detection enabled")
	}
	if ev.Hash == "hash-td1" {
		t.Error("hash should have been recomputed, not kept as original")
	}
}

func TestLogger_ConcurrentLog(t *testing.T) {
	storage := newMockStorage()
	l := newTestLogger(t, storage)
	defer l.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ev := makeValidEvent(fmt.Sprintf("conc%d", n))
			_ = l.Log(context.Background(), ev)
		}(i)
	}
	wg.Wait()

	time.Sleep(300 * time.Millisecond)

	if storage.eventCount() < 20 {
		t.Errorf("expected at least 20 events, got %d", storage.eventCount())
	}
}
