// SPDX-License-Identifier: GPL-3.0-or-later
package websocket

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestWarnDroppedRateLimits pins the contract that made the 2026-09-21 OOM
// impossible to log our way out of: every drop is counted, the first drop
// warns, drops inside the interval are silent, and the next drop after the
// interval warns with the cumulative count.
func TestWarnDroppedRateLimits(t *testing.T) {
	var count, last atomic.Int64

	if n := warnDropped(&count, &last); n != 1 {
		t.Fatalf("first drop: want warn with total 1, got %d", n)
	}
	for i := 0; i < 1000; i++ {
		if n := warnDropped(&count, &last); n != -1 {
			t.Fatalf("drop %d inside interval: want suppressed (-1), got %d", i+2, n)
		}
	}
	if got := count.Load(); got != 1001 {
		t.Fatalf("suppressed drops must still be counted: want 1001, got %d", got)
	}

	// Age the last warning past the interval; the next drop must warn again
	// and carry the full count, not a count that reset at the last warning.
	last.Store(time.Now().Add(-dropLogInterval - time.Second).UnixNano())
	if n := warnDropped(&count, &last); n != 1002 {
		t.Fatalf("first drop after interval: want warn with total 1002, got %d", n)
	}
	if n := warnDropped(&count, &last); n != -1 {
		t.Fatalf("drop right after a warn: want suppressed, got %d", n)
	}
}

// TestWarnDroppedConcurrentSingleWinner: when many goroutines drop at once
// after the interval elapses, exactly one of them gets to log.
func TestWarnDroppedConcurrentSingleWinner(t *testing.T) {
	var count, last atomic.Int64
	last.Store(time.Now().Add(-dropLogInterval - time.Second).UnixNano())

	const workers = 64
	var winners atomic.Int64
	done := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			if warnDropped(&count, &last) >= 0 {
				winners.Add(1)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < workers; i++ {
		<-done
	}
	if w := winners.Load(); w != 1 {
		t.Fatalf("want exactly 1 goroutine to log, got %d", w)
	}
	if c := count.Load(); c != workers {
		t.Fatalf("want all %d drops counted, got %d", workers, c)
	}
}
