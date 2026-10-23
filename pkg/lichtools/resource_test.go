// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"errors"
	"sync"
	"testing"
)

func TestSemaphore_NPlusOneRefused(t *testing.T) {
	s := NewSemaphore(2)
	r1, err := s.TryAcquire()
	if err != nil {
		t.Fatalf("1st acquire: %v", err)
	}
	r2, err := s.TryAcquire()
	if err != nil {
		t.Fatalf("2nd acquire: %v", err)
	}
	if _, err := s.TryAcquire(); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("3rd acquire should be ErrAtCapacity, got %v", err)
	}
	if s.Live() != 2 {
		t.Fatalf("live = %d, want 2", s.Live())
	}
	r1()
	// A slot freed; now one more acquire succeeds.
	r3, err := s.TryAcquire()
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	r2()
	r3()
	if s.Live() != 0 {
		t.Fatalf("live = %d, want 0 after all releases", s.Live())
	}
}

func TestSemaphore_ReleaseIdempotent(t *testing.T) {
	s := NewSemaphore(1)
	r, err := s.TryAcquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	r()
	r() // second release must not free a slot it does not hold
	if s.Live() != 0 {
		t.Fatalf("live = %d, want 0", s.Live())
	}
	if _, err := s.TryAcquire(); err != nil {
		t.Fatalf("slot should be free: %v", err)
	}
}

func TestSemaphore_ConcurrentNeverOvercommits(t *testing.T) {
	const cap = 3
	s := NewSemaphore(cap)
	var wg sync.WaitGroup
	var maxLive int
	var mu sync.Mutex
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := s.TryAcquire()
			if err != nil {
				return
			}
			mu.Lock()
			if s.Live() > maxLive {
				maxLive = s.Live()
			}
			mu.Unlock()
			rel()
		}()
	}
	wg.Wait()
	if maxLive > cap {
		t.Fatalf("over-committed: max live %d > cap %d", maxLive, cap)
	}
}
