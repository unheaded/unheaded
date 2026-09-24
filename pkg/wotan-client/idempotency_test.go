// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/metrics/prom"
)

func TestIdempotencyCache_MissOnEmpty(t *testing.T) {
	ic := NewIdempotencyCache(time.Hour)
	defer ic.Stop()

	_, ok := ic.Check("nonexistent")
	if ok {
		t.Error("expected miss on empty cache")
	}
}

func TestIdempotencyCache_RecordAndHit(t *testing.T) {
	ic := NewIdempotencyCache(time.Hour)
	defer ic.Stop()

	ic.Record("msg-001", ProcessResult{Err: nil, Processed: time.Now()})

	result, ok := ic.Check("msg-001")
	if !ok {
		t.Fatal("expected cache hit for msg-001")
	}
	if result.Err != nil {
		t.Errorf("expected nil error, got %v", result.Err)
	}
}

func TestIdempotencyCache_RecordErrorResult(t *testing.T) {
	ic := NewIdempotencyCache(time.Hour)
	defer ic.Stop()

	expectedErr := fmt.Errorf("processing failed")
	ic.Record("msg-err", ProcessResult{Err: expectedErr, Processed: time.Now()})

	result, ok := ic.Check("msg-err")
	if !ok {
		t.Fatal("expected cache hit for msg-err")
	}
	if result.Err == nil || result.Err.Error() != "processing failed" {
		t.Errorf("expected processing failed error, got %v", result.Err)
	}
}

func TestIdempotencyCache_DuplicateDetection(t *testing.T) {
	ic := NewIdempotencyCache(time.Hour)
	defer ic.Stop()

	// First processing.
	ic.Record("msg-dup", ProcessResult{Err: nil, Processed: time.Now()})

	// Simulate duplicate delivery.
	result, ok := ic.Check("msg-dup")
	if !ok {
		t.Fatal("duplicate should be detected")
	}
	if result.Err != nil {
		t.Error("cached result should be success")
	}
}

func TestIdempotencyCache_TTLExpiry(t *testing.T) {
	// Use a very short TTL.
	ic := NewIdempotencyCache(50 * time.Millisecond)
	defer ic.Stop()

	ic.Record("msg-expire", ProcessResult{Err: nil, Processed: time.Now()})

	// Immediately should hit.
	_, ok := ic.Check("msg-expire")
	if !ok {
		t.Fatal("expected immediate cache hit")
	}

	// Wait for expiry.
	time.Sleep(100 * time.Millisecond)

	_, ok = ic.Check("msg-expire")
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestIdempotencyCache_Len(t *testing.T) {
	ic := NewIdempotencyCache(time.Hour)
	defer ic.Stop()

	if ic.Len() != 0 {
		t.Errorf("expected 0, got %d", ic.Len())
	}

	ic.Record("a", ProcessResult{Processed: time.Now()})
	ic.Record("b", ProcessResult{Processed: time.Now()})
	ic.Record("c", ProcessResult{Processed: time.Now()})

	if ic.Len() != 3 {
		t.Errorf("expected 3, got %d", ic.Len())
	}

	// Overwrite existing entry — len should stay the same.
	ic.Record("a", ProcessResult{Err: fmt.Errorf("retry"), Processed: time.Now()})
	if ic.Len() != 3 {
		t.Errorf("expected 3 after overwrite, got %d", ic.Len())
	}
}

func TestIdempotencyCache_Cleanup(t *testing.T) {
	ic := NewIdempotencyCache(50 * time.Millisecond)
	defer ic.Stop()

	ic.Record("x", ProcessResult{Processed: time.Now()})
	ic.Record("y", ProcessResult{Processed: time.Now()})

	// Wait for entries to expire.
	time.Sleep(100 * time.Millisecond)

	// Manually trigger cleanup.
	ic.cleanup()

	if ic.Len() != 0 {
		t.Errorf("expected 0 after cleanup, got %d", ic.Len())
	}
}

func TestIdempotencyCache_StopIdempotent(t *testing.T) {
	ic := NewIdempotencyCache(time.Hour)

	// Double stop should not panic.
	ic.Stop()
	ic.Stop()
}

func TestIdempotencyCache_DefaultTTL(t *testing.T) {
	ic := NewIdempotencyCache(0) // 0 should use default
	defer ic.Stop()

	if ic.ttl != DefaultIdempotencyTTL {
		t.Errorf("expected default TTL %v, got %v", DefaultIdempotencyTTL, ic.ttl)
	}
}

func TestIdempotencyCache_AutoProcessedTime(t *testing.T) {
	ic := NewIdempotencyCache(time.Hour)
	defer ic.Stop()

	// Record without setting Processed — should auto-fill.
	ic.Record("auto-time", ProcessResult{Err: nil})

	result, ok := ic.Check("auto-time")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if result.Processed.IsZero() {
		t.Error("expected Processed to be auto-set, got zero")
	}
}

// The idempotency metrics register with the first cache, not at package
// load, so a binary without a cache publishes no idempotency zeros.
func TestIdempotencyMetrics_RegisteredOnlyByACache(t *testing.T) {
	scrape := func() string {
		rec := httptest.NewRecorder()
		prom.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
		return rec.Body.String()
	}
	// Other tests in this package may already have built a cache, so this
	// can only assert the "after" half unconditionally.
	ic := NewIdempotencyCache(time.Minute)
	defer ic.Stop()
	if !strings.Contains(scrape(), "wotan_idempotency_entries") {
		t.Error("idempotency metrics not registered after NewIdempotencyCache")
	}
	ic2 := NewIdempotencyCache(time.Minute) // second cache must not panic on re-register
	ic2.Stop()
}
