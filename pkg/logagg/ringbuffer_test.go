// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package logagg

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewRingBuffer(t *testing.T) {
	rb := NewRingBuffer(100)
	if rb.Cap() != 100 {
		t.Errorf("Cap() = %d, want 100", rb.Cap())
	}
	if rb.Len() != 0 {
		t.Errorf("Len() = %d, want 0", rb.Len())
	}
}

func TestNewRingBuffer_InvalidCapacity(t *testing.T) {
	rb := NewRingBuffer(0)
	if rb.Cap() != DefaultCapacity {
		t.Errorf("Cap() = %d, want %d (default)", rb.Cap(), DefaultCapacity)
	}

	rb = NewRingBuffer(-5)
	if rb.Cap() != DefaultCapacity {
		t.Errorf("Cap() = %d, want %d (default)", rb.Cap(), DefaultCapacity)
	}
}

func TestRingBuffer_PushAndQuery(t *testing.T) {
	rb := NewRingBuffer(100)
	now := time.Now()

	rb.Push(LogEntry{Timestamp: now, Service: "svc1", Level: "info", Message: "hello"})
	rb.Push(LogEntry{Timestamp: now.Add(time.Second), Service: "svc2", Level: "error", Message: "world"})

	results := rb.Query(LogQuery{})
	if len(results) != 2 {
		t.Fatalf("Query() returned %d results, want 2", len(results))
	}
	if results[0].Message != "hello" {
		t.Errorf("first entry = %q, want %q", results[0].Message, "hello")
	}
	if results[1].Message != "world" {
		t.Errorf("second entry = %q, want %q", results[1].Message, "world")
	}
}

func TestRingBuffer_WrapsAtCapacity(t *testing.T) {
	rb := NewRingBuffer(5)
	now := time.Now()

	for i := 0; i < 10; i++ {
		rb.Push(LogEntry{Timestamp: now.Add(time.Duration(i) * time.Second), Service: "svc", Level: "info", Message: fmt.Sprintf("msg-%d", i)})
	}

	if rb.Len() != 5 {
		t.Errorf("Len() = %d, want 5", rb.Len())
	}

	results := rb.Query(LogQuery{Limit: 10})
	if len(results) != 5 {
		t.Fatalf("Query() returned %d results, want 5", len(results))
	}
	// Should have entries 5-9 (oldest 0-4 evicted)
	if results[0].Message != "msg-5" {
		t.Errorf("oldest entry = %q, want %q", results[0].Message, "msg-5")
	}
	if results[4].Message != "msg-9" {
		t.Errorf("newest entry = %q, want %q", results[4].Message, "msg-9")
	}
}

func TestRingBuffer_QueryByService(t *testing.T) {
	rb := NewRingBuffer(100)
	now := time.Now()

	rb.Push(LogEntry{Timestamp: now, Service: "timeguru", Level: "info", Message: "a"})
	rb.Push(LogEntry{Timestamp: now, Service: "captain", Level: "info", Message: "b"})
	rb.Push(LogEntry{Timestamp: now, Service: "timeguru", Level: "error", Message: "c"})

	results := rb.Query(LogQuery{Service: "timeguru"})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Service != "timeguru" {
			t.Errorf("entry.Service = %q, want %q", r.Service, "timeguru")
		}
	}
}

func TestRingBuffer_QueryByLevel(t *testing.T) {
	rb := NewRingBuffer(100)
	now := time.Now()

	rb.Push(LogEntry{Timestamp: now, Service: "svc", Level: "info", Message: "a"})
	rb.Push(LogEntry{Timestamp: now, Service: "svc", Level: "error", Message: "b"})
	rb.Push(LogEntry{Timestamp: now, Service: "svc", Level: "INFO", Message: "c"})

	results := rb.Query(LogQuery{Level: "info"})
	if len(results) != 2 { // case-insensitive
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestRingBuffer_QueryByTimeRange(t *testing.T) {
	rb := NewRingBuffer(100)
	base := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)

	rb.Push(LogEntry{Timestamp: base, Service: "svc", Level: "info", Message: "old"})
	rb.Push(LogEntry{Timestamp: base.Add(time.Hour), Service: "svc", Level: "info", Message: "mid"})
	rb.Push(LogEntry{Timestamp: base.Add(2 * time.Hour), Service: "svc", Level: "info", Message: "new"})

	results := rb.Query(LogQuery{
		From: base.Add(30 * time.Minute),
		To:   base.Add(90 * time.Minute),
	})
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Message != "mid" {
		t.Errorf("entry = %q, want %q", results[0].Message, "mid")
	}
}

func TestRingBuffer_QuerySearch(t *testing.T) {
	rb := NewRingBuffer(100)
	now := time.Now()

	rb.Push(LogEntry{Timestamp: now, Service: "svc", Level: "info", Message: "connection established"})
	rb.Push(LogEntry{Timestamp: now, Service: "svc", Level: "error", Message: "timeout error"})
	rb.Push(LogEntry{Timestamp: now, Service: "svc", Level: "info", Message: "other",
		Fields: map[string]interface{}{"detail": "connection reset"}})

	results := rb.Query(LogQuery{Search: "connection"})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestRingBuffer_QueryPagination(t *testing.T) {
	rb := NewRingBuffer(100)
	now := time.Now()

	for i := 0; i < 20; i++ {
		rb.Push(LogEntry{Timestamp: now.Add(time.Duration(i) * time.Second), Service: "svc", Level: "info", Message: fmt.Sprintf("msg-%d", i)})
	}

	page1 := rb.Query(LogQuery{Limit: 5, Offset: 0})
	page2 := rb.Query(LogQuery{Limit: 5, Offset: 5})

	if len(page1) != 5 {
		t.Fatalf("page1 len = %d, want 5", len(page1))
	}
	if len(page2) != 5 {
		t.Fatalf("page2 len = %d, want 5", len(page2))
	}
	if page1[0].Message != "msg-0" {
		t.Errorf("page1[0] = %q, want %q", page1[0].Message, "msg-0")
	}
	if page2[0].Message != "msg-5" {
		t.Errorf("page2[0] = %q, want %q", page2[0].Message, "msg-5")
	}
}

func TestRingBuffer_EmptyQueryReturnsEmptySlice(t *testing.T) {
	rb := NewRingBuffer(100)
	results := rb.Query(LogQuery{})
	if results == nil {
		t.Error("empty query should return empty slice, not nil")
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestRingBuffer_ConcurrentSafety(t *testing.T) {
	rb := NewRingBuffer(1000)
	var wg sync.WaitGroup

	// Writers
	for w := 0; w < 10; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				rb.Push(LogEntry{
					Timestamp: time.Now(),
					Service:   fmt.Sprintf("svc-%d", id),
					Level:     "info",
					Message:   fmt.Sprintf("msg-%d-%d", id, i),
				})
			}
		}(w)
	}

	// Readers
	for r := 0; r < 5; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				rb.Query(LogQuery{Service: "svc-0"})
				rb.Len()
			}
		}()
	}

	wg.Wait()

	if rb.Len() > rb.Cap() {
		t.Errorf("Len() = %d > Cap() = %d", rb.Len(), rb.Cap())
	}
}
