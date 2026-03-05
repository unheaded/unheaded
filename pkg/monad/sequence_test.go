// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package monad

import (
	"fmt"
	"sync"
	"testing"
)

func TestSeqTracker_FirstPacketAlwaysPasses(t *testing.T) {
	tracker := NewSeqTracker()
	if !tracker.Check(1) {
		t.Fatal("first packet should always pass")
	}

	// Also verify with a high sequence number as first packet.
	tracker2 := NewSeqTracker()
	if !tracker2.Check(50000) {
		t.Fatal("first packet with high seqNum should pass")
	}

	// Zero sequence number as first packet.
	tracker3 := NewSeqTracker()
	if !tracker3.Check(0) {
		t.Fatal("first packet with seqNum=0 should pass")
	}
}

func TestSeqTracker_DuplicateDetected(t *testing.T) {
	tracker := NewSeqTracker()

	if !tracker.Check(42) {
		t.Fatal("first packet should pass")
	}

	if tracker.Check(42) {
		t.Fatal("duplicate seqNum should be detected as replay")
	}

	// Third attempt should also fail.
	if tracker.Check(42) {
		t.Fatal("triple seqNum should be detected as replay")
	}
}

func TestSeqTracker_WindowAdvanceClearsOldEntries(t *testing.T) {
	tracker := NewSeqTracker()

	// Accept packet 1.
	if !tracker.Check(1) {
		t.Fatal("seqNum 1 should pass")
	}

	// Advance the window far enough that seqNum 1 falls out.
	// SeqWindowSize = 16384, so advancing by 16384 from 1 means
	// seqNum 16385 will be the new max, and seqNum 1 will be too old.
	newSeq := uint16(1 + SeqWindowSize)
	if !tracker.Check(newSeq) {
		t.Fatalf("seqNum %d should pass (advances window)", newSeq)
	}

	if tracker.MaxSeq() != newSeq {
		t.Fatalf("MaxSeq() = %d, want %d", tracker.MaxSeq(), newSeq)
	}

	// Now seqNum 1 is outside the window and should be rejected.
	if tracker.Check(1) {
		t.Fatal("seqNum 1 should be rejected (too old, outside window)")
	}
}

func TestSeqTracker_OutOfWindowRejected(t *testing.T) {
	tracker := NewSeqTracker()

	// Set a high maxSeq.
	highSeq := uint16(20000)
	if !tracker.Check(highSeq) {
		t.Fatal("first packet should pass")
	}

	// A packet that is SeqWindowSize behind should be rejected.
	oldSeq := highSeq - SeqWindowSize
	if tracker.Check(oldSeq) {
		t.Fatal("packet outside window should be rejected")
	}

	// A packet that is SeqWindowSize+1 behind should also be rejected.
	veryOld := highSeq - SeqWindowSize - 1
	if tracker.Check(veryOld) {
		t.Fatal("packet well outside window should be rejected")
	}
}

func TestSeqTracker_SequentialPacketsPass(t *testing.T) {
	tracker := NewSeqTracker()

	// Send 1000 sequential packets; all should pass.
	for i := uint16(1); i <= 1000; i++ {
		if !tracker.Check(i) {
			t.Fatalf("sequential seqNum %d should pass", i)
		}
	}

	if tracker.MaxSeq() != 1000 {
		t.Fatalf("MaxSeq() = %d, want 1000", tracker.MaxSeq())
	}
}

func TestSeqTracker_OutOfOrderWithinWindowPasses(t *testing.T) {
	tracker := NewSeqTracker()

	// Send packet 100 first.
	if !tracker.Check(100) {
		t.Fatal("seqNum 100 should pass")
	}

	// Send packet 50 (within window, not seen before).
	if !tracker.Check(50) {
		t.Fatal("seqNum 50 should pass (within window, not seen)")
	}

	// Send packet 75.
	if !tracker.Check(75) {
		t.Fatal("seqNum 75 should pass (within window, not seen)")
	}

	// Now replay 50; should fail.
	if tracker.Check(50) {
		t.Fatal("replay of seqNum 50 should be detected")
	}

	// MaxSeq should still be 100.
	if tracker.MaxSeq() != 100 {
		t.Fatalf("MaxSeq() = %d, want 100", tracker.MaxSeq())
	}
}

func TestSeqTracker_MaxSeqReturnsHighest(t *testing.T) {
	tracker := NewSeqTracker()

	if tracker.MaxSeq() != 0 {
		t.Fatalf("MaxSeq() on new tracker = %d, want 0", tracker.MaxSeq())
	}

	tracker.Check(5)
	if tracker.MaxSeq() != 5 {
		t.Fatalf("MaxSeq() = %d, want 5", tracker.MaxSeq())
	}

	tracker.Check(3) // behind, does not change max
	if tracker.MaxSeq() != 5 {
		t.Fatalf("MaxSeq() = %d, want 5 after accepting behind packet", tracker.MaxSeq())
	}

	tracker.Check(10) // ahead, updates max
	if tracker.MaxSeq() != 10 {
		t.Fatalf("MaxSeq() = %d, want 10", tracker.MaxSeq())
	}
}

func TestFlowSeqTracker_PerFlowIsolation(t *testing.T) {
	ft := NewFlowSeqTracker(100)

	// Flow "A" accepts seqNum 1.
	if !ft.Check("A", 1) {
		t.Fatal("flow A seqNum 1 should pass")
	}

	// Flow "B" also accepts seqNum 1 (independent).
	if !ft.Check("B", 1) {
		t.Fatal("flow B seqNum 1 should pass (independent flow)")
	}

	// Flow "A" rejects duplicate seqNum 1.
	if ft.Check("A", 1) {
		t.Fatal("flow A duplicate seqNum 1 should be detected")
	}

	// Flow "B" rejects duplicate seqNum 1.
	if ft.Check("B", 1) {
		t.Fatal("flow B duplicate seqNum 1 should be detected")
	}

	// Each flow can advance independently.
	if !ft.Check("A", 100) {
		t.Fatal("flow A seqNum 100 should pass")
	}

	if !ft.Check("B", 200) {
		t.Fatal("flow B seqNum 200 should pass")
	}

	if ft.FlowCount() != 2 {
		t.Fatalf("FlowCount() = %d, want 2", ft.FlowCount())
	}
}

func TestFlowSeqTracker_MaxFlowsEnforced(t *testing.T) {
	ft := NewFlowSeqTracker(2)

	if !ft.Check("flow-1", 1) {
		t.Fatal("flow-1 should be accepted")
	}
	if !ft.Check("flow-2", 1) {
		t.Fatal("flow-2 should be accepted")
	}

	// Third flow should be rejected (at capacity).
	if ft.Check("flow-3", 1) {
		t.Fatal("flow-3 should be rejected (max flows reached)")
	}

	// Existing flows should still work.
	if !ft.Check("flow-1", 2) {
		t.Fatal("flow-1 seqNum 2 should pass (existing flow)")
	}
}

func TestFlowSeqTracker_FlowCount(t *testing.T) {
	ft := NewFlowSeqTracker(100)

	if ft.FlowCount() != 0 {
		t.Fatalf("FlowCount() on empty tracker = %d, want 0", ft.FlowCount())
	}

	ft.Check("a", 1)
	ft.Check("b", 1)
	ft.Check("c", 1)

	if ft.FlowCount() != 3 {
		t.Fatalf("FlowCount() = %d, want 3", ft.FlowCount())
	}

	// Checking an existing flow does not increase count.
	ft.Check("a", 2)
	if ft.FlowCount() != 3 {
		t.Fatalf("FlowCount() = %d, want 3 after re-check", ft.FlowCount())
	}
}

func TestSeqTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewSeqTracker()

	const goroutines = 100
	const packetsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Each goroutine sends a unique range of sequence numbers.
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			base := uint16(id * packetsPerGoroutine)
			for i := uint16(1); i <= packetsPerGoroutine; i++ {
				tracker.Check(base + i)
			}
		}(g)
	}

	wg.Wait()

	// After all goroutines complete, MaxSeq should be set to some valid value.
	// We cannot predict the exact max due to goroutine scheduling, but
	// the tracker should not have panicked or corrupted state.
	maxSeq := tracker.MaxSeq()
	if maxSeq == 0 {
		t.Fatal("MaxSeq() should be non-zero after concurrent access")
	}
}

func TestFlowSeqTracker_ConcurrentAccess(t *testing.T) {
	ft := NewFlowSeqTracker(1000)

	const goroutines = 50
	const packetsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			flowID := fmt.Sprintf("flow-%d", id)
			for i := uint16(1); i <= packetsPerGoroutine; i++ {
				ft.Check(flowID, i)
			}
		}(g)
	}

	wg.Wait()

	count := ft.FlowCount()
	if count != goroutines {
		t.Fatalf("FlowCount() = %d, want %d", count, goroutines)
	}
}

func TestSeqTracker_Uint16Wraparound(t *testing.T) {
	tracker := NewSeqTracker()

	// Start near the uint16 max.
	if !tracker.Check(65530) {
		t.Fatal("seqNum 65530 should pass")
	}

	// Advance through the wraparound point.
	for seq := uint16(65531); seq != 10; seq++ {
		if !tracker.Check(seq) {
			t.Fatalf("seqNum %d should pass during wraparound", seq)
		}
	}

	if !tracker.Check(10) {
		t.Fatal("seqNum 10 should pass after wraparound")
	}

	// Verify 65530 is now behind but still within window.
	// The distance from 10 back to 65530 wraps: 10 - 65530 = 65516 mod 65536
	// This is within SeqWindowSize (16384)? No, 65516 > 16384, so it's out of window.
	if tracker.Check(65530) {
		t.Fatal("seqNum 65530 should be rejected (outside window after wraparound)")
	}
}

func TestFlowSeqTracker_String(t *testing.T) {
	ft := NewFlowSeqTracker(100)
	ft.Check("a", 1)
	ft.Check("b", 1)

	s := ft.String()
	expected := "FlowSeqTracker{flows=2, max=100}"
	if s != expected {
		t.Fatalf("String() = %q, want %q", s, expected)
	}
}
