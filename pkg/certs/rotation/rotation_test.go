// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rotation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.CheckInterval != 5*time.Minute {
		t.Errorf("CheckInterval = %v, want %v", cfg.CheckInterval, 5*time.Minute)
	}
	if cfg.RenewBefore != 15*time.Minute {
		t.Errorf("RenewBefore = %v, want %v", cfg.RenewBefore, 15*time.Minute)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.RetryDelay != 30*time.Second {
		t.Errorf("RetryDelay = %v, want %v", cfg.RetryDelay, 30*time.Second)
	}
}

func TestRotator_RegisterUnregister(t *testing.T) {
	r := New(nil, func(serial string) {})
	exp := time.Now().Add(24 * time.Hour)

	r.Register("cert-001", exp)
	r.Register("cert-002", exp)

	tracked := r.GetTracked()
	if len(tracked) != 2 {
		t.Errorf("tracked count = %d, want 2", len(tracked))
	}

	r.Unregister("cert-001")
	tracked = r.GetTracked()
	if len(tracked) != 1 {
		t.Errorf("tracked count after unregister = %d, want 1", len(tracked))
	}
}

func TestRotator_NeedsRenewal(t *testing.T) {
	cfg := &Config{
		CheckInterval: time.Hour,
		RenewBefore:   30 * time.Minute,
		MaxRetries:    3,
		RetryDelay:    time.Second,
	}
	r := New(cfg, func(serial string) {})

	// Cert expiring in 1 hour (within 30min renewal window? No, 1h > 30min)
	r.Register("far-out", time.Now().Add(1*time.Hour))
	if r.NeedsRenewal("far-out") {
		t.Error("cert expiring in 1h should not need renewal with 30min window")
	}

	// Cert expiring in 10 minutes (within 30min window)
	r.Register("close", time.Now().Add(10*time.Minute))
	if !r.NeedsRenewal("close") {
		t.Error("cert expiring in 10min should need renewal with 30min window")
	}

	// Nonexistent cert
	if r.NeedsRenewal("nonexistent") {
		t.Error("nonexistent cert should not need renewal")
	}
}

func TestRotator_TimeToRenewal(t *testing.T) {
	cfg := &Config{
		CheckInterval: time.Hour,
		RenewBefore:   30 * time.Minute,
		MaxRetries:    3,
		RetryDelay:    time.Second,
	}
	r := New(cfg, func(serial string) {})

	// Cert expiring in 1 hour, renewal window is 30min before → renewal in ~30min
	r.Register("test", time.Now().Add(1*time.Hour))
	ttl := r.TimeToRenewal("test")
	if ttl < 25*time.Minute || ttl > 35*time.Minute {
		t.Errorf("TimeToRenewal = %v, expected ~30min", ttl)
	}

	// Nonexistent
	if ttl := r.TimeToRenewal("nope"); ttl != 0 {
		t.Errorf("TimeToRenewal(nonexistent) = %v, want 0", ttl)
	}
}

func TestRotator_CheckAndRenew(t *testing.T) {
	var renewed atomic.Int32
	cfg := &Config{
		CheckInterval: 10 * time.Millisecond,
		RenewBefore:   1 * time.Hour,
		MaxRetries:    2,
		RetryDelay:    time.Millisecond,
	}
	r := New(cfg, func(serial string) {
		renewed.Add(1)
	})

	// Register cert that expires soon (within renewal window)
	r.Register("expiring", time.Now().Add(30*time.Minute))

	// Manually trigger check
	r.checkAndRenew()

	// Wait for goroutine callback
	time.Sleep(50 * time.Millisecond)

	if renewed.Load() < 1 {
		t.Error("expected renewal callback to be called")
	}
}

func TestRotator_MaxRetriesRemoves(t *testing.T) {
	cfg := &Config{
		CheckInterval: 10 * time.Millisecond,
		RenewBefore:   1 * time.Hour,
		MaxRetries:    0, // no retries allowed
		RetryDelay:    time.Millisecond,
	}
	r := New(cfg, func(serial string) {})

	r.Register("will-expire", time.Now().Add(30*time.Minute))

	// First check triggers renewal (retries = 0, increments to 1)
	r.checkAndRenew()
	// Second check: retries (1) >= MaxRetries (0), so it gets removed
	r.checkAndRenew()

	tracked := r.GetTracked()
	if len(tracked) != 0 {
		t.Errorf("expected cert to be removed after max retries, got %d tracked", len(tracked))
	}
}

func TestRotator_Start(t *testing.T) {
	var renewed atomic.Int32
	cfg := &Config{
		CheckInterval: 10 * time.Millisecond,
		RenewBefore:   1 * time.Hour,
		MaxRetries:    5,
		RetryDelay:    time.Millisecond,
	}
	r := New(cfg, func(serial string) {
		renewed.Add(1)
	})

	r.Register("test", time.Now().Add(30*time.Minute))

	stopCh := make(chan struct{})
	go r.Start(stopCh)

	// Wait for at least one tick
	time.Sleep(50 * time.Millisecond)
	close(stopCh)

	if renewed.Load() < 1 {
		t.Error("expected at least one renewal from Start loop")
	}

	// Double Start should be no-op
	stopCh2 := make(chan struct{})
	close(stopCh2)
	r.Start(stopCh2) // should return immediately
}

func TestRotator_NilConfig(t *testing.T) {
	r := New(nil, func(serial string) {})
	if r.config.MaxRetries != 3 {
		t.Errorf("expected default MaxRetries=3, got %d", r.config.MaxRetries)
	}
}

// --- Renewer Tests ---

func TestRenewer_SuccessFirstAttempt(t *testing.T) {
	cfg := &Config{MaxRetries: 3, RetryDelay: time.Millisecond}
	r := NewRenewer(cfg, func(ctx context.Context, serial string) (string, error) {
		return "new-" + serial, nil
	})

	result := r.Renew(context.Background(), "cert-001")
	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if result.NewSerial != "new-cert-001" {
		t.Errorf("NewSerial = %q, want %q", result.NewSerial, "new-cert-001")
	}
	if result.Retries != 0 {
		t.Errorf("Retries = %d, want 0", result.Retries)
	}
}

func TestRenewer_SuccessAfterRetries(t *testing.T) {
	var attempts atomic.Int32
	cfg := &Config{MaxRetries: 3, RetryDelay: time.Millisecond}
	r := NewRenewer(cfg, func(ctx context.Context, serial string) (string, error) {
		if attempts.Add(1) < 3 {
			return "", errors.New("transient error")
		}
		return "new-" + serial, nil
	})

	result := r.Renew(context.Background(), "cert-002")
	if !result.Success {
		t.Errorf("expected success after retries, got error: %v", result.Error)
	}
	if result.Retries != 2 {
		t.Errorf("Retries = %d, want 2", result.Retries)
	}
}

func TestRenewer_MaxRetriesExhausted(t *testing.T) {
	cfg := &Config{MaxRetries: 2, RetryDelay: time.Millisecond}
	r := NewRenewer(cfg, func(ctx context.Context, serial string) (string, error) {
		return "", errors.New("permanent error")
	})

	result := r.Renew(context.Background(), "cert-003")
	if result.Success {
		t.Error("expected failure after max retries")
	}
	if result.Error == nil {
		t.Error("expected error")
	}
}

func TestRenewer_ContextCancellation(t *testing.T) {
	cfg := &Config{MaxRetries: 10, RetryDelay: 100 * time.Millisecond}
	r := NewRenewer(cfg, func(ctx context.Context, serial string) (string, error) {
		return "", errors.New("fail")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := r.Renew(ctx, "cert-004")
	if result.Success {
		t.Error("expected failure due to context cancellation")
	}
}

func TestRenewer_AlreadyInProgress(t *testing.T) {
	cfg := &Config{MaxRetries: 0, RetryDelay: time.Millisecond}
	started := make(chan struct{})
	done := make(chan struct{})
	r := NewRenewer(cfg, func(ctx context.Context, serial string) (string, error) {
		close(started)
		<-done
		return "new", nil
	})

	// Start first renewal
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.Renew(context.Background(), "cert-005")
	}()

	<-started

	// Try second renewal
	if !r.IsInProgress("cert-005") {
		t.Error("expected IsInProgress to be true")
	}

	result := r.Renew(context.Background(), "cert-005")
	if result.Success {
		t.Error("expected failure for duplicate renewal")
	}

	state := r.GetState("cert-005")
	if state == nil {
		t.Error("expected non-nil state for in-progress renewal")
	}

	close(done)
	wg.Wait()

	// After completion
	if r.IsInProgress("cert-005") {
		t.Error("expected IsInProgress to be false after completion")
	}
	if s := r.GetState("cert-005"); s != nil {
		t.Error("expected nil state after completion")
	}
}

func TestRenewer_NilConfig(t *testing.T) {
	r := NewRenewer(nil, func(ctx context.Context, serial string) (string, error) {
		return "new", nil
	})
	if r.config.MaxRetries != 3 {
		t.Errorf("expected default MaxRetries=3, got %d", r.config.MaxRetries)
	}
}

// --- BatchRenewer Tests ---

func TestBatchRenewer_RenewAll(t *testing.T) {
	cfg := &Config{MaxRetries: 1, RetryDelay: time.Millisecond}
	r := NewRenewer(cfg, func(ctx context.Context, serial string) (string, error) {
		return "new-" + serial, nil
	})

	br := NewBatchRenewer(r, 3)
	results := br.RenewAll(context.Background(), []string{"a", "b", "c", "d"})

	if len(results) != 4 {
		t.Fatalf("results count = %d, want 4", len(results))
	}
	for i, res := range results {
		if !res.Success {
			t.Errorf("result[%d] failed: %v", i, res.Error)
		}
	}
}

func TestBatchRenewer_DefaultConcurrency(t *testing.T) {
	r := NewRenewer(&Config{MaxRetries: 0, RetryDelay: time.Millisecond}, func(ctx context.Context, serial string) (string, error) {
		return "new", nil
	})

	br := NewBatchRenewer(r, 0) // should default to 5
	if br.concurrency != 5 {
		t.Errorf("concurrency = %d, want 5", br.concurrency)
	}
}
