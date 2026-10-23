// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import "errors"

// ErrAtCapacity is returned when the host-resource cap on concurrent sandboxes
// is reached. A driver that asks for more live runs than the cap cannot
// fork-bomb the host — excess calls are refused, not queued unboundedly.
var ErrAtCapacity = errors.New("lichtools: at sandbox capacity")

// Semaphore bounds the number of simultaneously live sandboxes. It never
// over-commits: the (N+1)th TryAcquire refuses with ErrAtCapacity.
type Semaphore struct {
	tokens chan struct{}
}

// NewSemaphore builds a semaphore of n slots. n<=0 is treated as 1 (never
// unbounded).
func NewSemaphore(n int) *Semaphore {
	if n <= 0 {
		n = 1
	}
	return &Semaphore{tokens: make(chan struct{}, n)}
}

// TryAcquire takes a slot without blocking, returning a release func, or
// ErrAtCapacity if none is free. The release func is idempotent.
func (s *Semaphore) TryAcquire() (func(), error) {
	select {
	case s.tokens <- struct{}{}:
		released := false
		return func() {
			if released {
				return
			}
			released = true
			<-s.tokens
		}, nil
	default:
		return nil, ErrAtCapacity
	}
}

// Live returns the number of slots currently held.
func (s *Semaphore) Live() int { return len(s.tokens) }

// Cap returns the maximum number of live sandboxes.
func (s *Semaphore) Cap() int { return cap(s.tokens) }
