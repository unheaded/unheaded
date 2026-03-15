// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package sequence

import (
	"sync"
	"testing"
)

func TestNamespaceCounterIncrement(t *testing.T) {
	nc := NewNamespaceCounter("test")

	for i := uint32(1); i <= 10; i++ {
		result := nc.Increment()
		if result != i {
			t.Errorf("Expected %d, got %d", i, result)
		}
	}
}

func TestNamespaceCounterCurrent(t *testing.T) {
	nc := NewNamespaceCounter("test")

	nc.Increment()
	nc.Increment()

	current := nc.Current()
	if current != 2 {
		t.Errorf("Expected 2, got %d", current)
	}

	// Current should not increment
	current = nc.Current()
	if current != 2 {
		t.Errorf("Expected 2, got %d", current)
	}
}

func TestDetectGapNoGap(t *testing.T) {
	nc := NewNamespaceCounter("test")

	lost, reordered := nc.DetectGap(5, 5)
	if lost != 0 || reordered {
		t.Errorf("Expected no gap and no reorder, got lost=%d, reordered=%v", lost, reordered)
	}
}

func TestDetectGapWithLoss(t *testing.T) {
	nc := NewNamespaceCounter("test")

	// Expected 5, received 8 means packets 6, 7 were lost (2 packets)
	lost, reordered := nc.DetectGap(5, 8)
	if lost != 2 || reordered {
		t.Errorf("Expected lost=2, reordered=false, got lost=%d, reordered=%v", lost, reordered)
	}
}

func TestDetectGapReordered(t *testing.T) {
	nc := NewNamespaceCounter("test")

	// Received out of order (packet arrives before expected)
	lost, reordered := nc.DetectGap(10, 5)
	if lost != 0 || !reordered {
		t.Errorf("Expected reordered=true, got lost=%d, reordered=%v", lost, reordered)
	}
}

func TestSequenceTrackerIncrement(t *testing.T) {
	st := NewSequenceTracker()

	result1 := st.Increment("ns1")
	result2 := st.Increment("ns1")
	result3 := st.Increment("ns2")

	if result1 != 1 {
		t.Errorf("Expected 1, got %d", result1)
	}
	if result2 != 2 {
		t.Errorf("Expected 2, got %d", result2)
	}
	if result3 != 1 {
		t.Errorf("Expected 1, got %d", result3)
	}
}

func TestSequenceTrackerValidateMonotonicity(t *testing.T) {
	st := NewSequenceTracker()

	// First packet with sequence 1
	lost, reordered, err := st.ValidateMonotonicity("ns1", 1)
	if err != nil || lost != 0 || reordered {
		t.Errorf("First packet: err=%v, lost=%d, reordered=%v", err, lost, reordered)
	}

	// Second packet with sequence 2 (expected)
	lost, reordered, err = st.ValidateMonotonicity("ns1", 2)
	if err != nil || lost != 0 || reordered {
		t.Errorf("Second packet: err=%v, lost=%d, reordered=%v", err, lost, reordered)
	}

	// Packet with gap (sequence 5, expected 3): packet 4 is lost (gap-1 = 5-3-1 = 1)
	lost, reordered, err = st.ValidateMonotonicity("ns1", 5)
	if err != nil || lost != 1 || reordered {
		t.Errorf("Gap packet: err=%v, lost=%d, reordered=%v (expected lost=1)", err, lost, reordered)
	}

	// Out of order packet (sequence 4, expected 6)
	lost, reordered, err = st.ValidateMonotonicity("ns1", 4)
	if err != nil || !reordered {
		t.Errorf("Out of order packet: err=%v, reordered=%v (expected true)", err, reordered)
	}
}

func TestConcurrentIncrement(t *testing.T) {
	st := NewSequenceTracker()
	numGoroutines := 100
	increments := 100
	var wg sync.WaitGroup

	results := make(chan uint32, numGoroutines*increments)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < increments; i++ {
				result := st.Increment("concurrent")
				results <- result
			}
		}()
	}

	wg.Wait()
	close(results)

	// Collect all results
	seen := make(map[uint32]bool)
	for result := range results {
		if seen[result] {
			t.Errorf("Duplicate sequence number: %d", result)
		}
		seen[result] = true
	}

	// Verify we got all unique sequences from 1 to numGoroutines*increments
	expected := uint32(numGoroutines * increments)
	if uint32(len(seen)) != expected {
		t.Errorf("Expected %d unique sequences, got %d", expected, len(seen))
	}
}

func TestMultipleNamespaces(t *testing.T) {
	st := NewSequenceTracker()

	// Increment different namespaces
	for i := 0; i < 5; i++ {
		st.Increment("ns1")
		st.Increment("ns2")
		st.Increment("ns3")
	}

	if st.Current("ns1") != 5 {
		t.Errorf("Expected ns1=5, got %d", st.Current("ns1"))
	}
	if st.Current("ns2") != 5 {
		t.Errorf("Expected ns2=5, got %d", st.Current("ns2"))
	}
	if st.Current("ns3") != 5 {
		t.Errorf("Expected ns3=5, got %d", st.Current("ns3"))
	}
}
