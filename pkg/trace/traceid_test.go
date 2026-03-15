// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package trace

import (
	"sort"
	"testing"
	"time"
)

func TestNewTraceID_Uniqueness(t *testing.T) {
	// D4-001 remediation: 128-bit IDs must have zero collisions at 100K samples.
	// With 122 bits of entropy, P(collision) at 100K = ~1.5e-28.
	const count = 100_000
	seen := make(map[TraceID]struct{}, count)

	for i := 0; i < count; i++ {
		id, err := NewTraceID()
		if err != nil {
			t.Fatalf("NewTraceID() failed at iteration %d: %v", i, err)
		}
		if id.IsZero() {
			t.Fatalf("NewTraceID() returned zero ID at iteration %d", i)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("collision at iteration %d: duplicate trace ID %s", i, id)
		}
		seen[id] = struct{}{}
	}

	if len(seen) != count {
		t.Errorf("expected %d unique IDs, got %d", count, len(seen))
	}
}

func TestNewTraceID_TimeOrdering(t *testing.T) {
	// UUIDv7 is time-ordered: later IDs must sort after earlier ones.
	// This is critical for efficient log correlation.
	const count = 1000
	ids := make([]TraceID, 0, count)

	for i := 0; i < count; i++ {
		id, err := NewTraceID()
		if err != nil {
			t.Fatalf("NewTraceID() failed at iteration %d: %v", i, err)
		}
		ids = append(ids, id)
		// Small delay to ensure different timestamps (UUIDv7 has ms resolution)
		if i%100 == 0 {
			time.Sleep(time.Millisecond)
		}
	}

	// Verify IDs are already sorted (string comparison works for UUIDv7 hex)
	sorted := make([]string, len(ids))
	for i, id := range ids {
		sorted[i] = id.String()
	}

	isSorted := sort.SliceIsSorted(sorted, func(i, j int) bool {
		return sorted[i] <= sorted[j]
	})

	if !isSorted {
		t.Error("trace IDs are not time-ordered; UUIDv7 k-sortability violated")
		// Find the first out-of-order pair for debugging
		for i := 1; i < len(sorted); i++ {
			if sorted[i] < sorted[i-1] {
				t.Errorf("  out of order at index %d: %s > %s", i, sorted[i-1], sorted[i])
				break
			}
		}
	}
}

func TestNewTraceID_CollisionResistance(t *testing.T) {
	// D4 comparison: 20-bit flow labels have ~40% collision at 1000 flows.
	// 128-bit trace IDs should have ZERO collisions at 1000 flows.
	const count = 1000
	seen := make(map[TraceID]struct{}, count)

	for i := 0; i < count; i++ {
		id, err := NewTraceID()
		if err != nil {
			t.Fatalf("NewTraceID() failed: %v", err)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("128-bit collision at only %d IDs (20-bit would expect ~40%% collision here)", i)
		}
		seen[id] = struct{}{}
	}
}

func TestFlowLabelHint_Is20Bit(t *testing.T) {
	for i := 0; i < 1000; i++ {
		id, err := NewTraceID()
		if err != nil {
			t.Fatalf("NewTraceID() failed: %v", err)
		}
		label := id.FlowLabelHint()
		if label > 0x000FFFFF {
			t.Errorf("flow label %#x exceeds 20-bit maximum", label)
		}
	}
}

func TestFlowLabelHint_DemonstrateLossyProjection(t *testing.T) {
	// D4-001: Flow label is a LOSSY projection. Multiple trace IDs can map
	// to the same flow label. This test demonstrates why the full 128-bit
	// ID must be used for trace correlation, not the flow label.
	const count = 100_000
	labels := make(map[uint32]int, count)

	for i := 0; i < count; i++ {
		id, err := NewTraceID()
		if err != nil {
			t.Fatalf("NewTraceID() failed: %v", err)
		}
		labels[id.FlowLabelHint()]++
	}

	// With 100K IDs projected to 20-bit space (1M possible values),
	// birthday paradox predicts many collisions.
	collisions := 0
	for _, count := range labels {
		if count > 1 {
			collisions += count - 1
		}
	}

	t.Logf("flow label collisions: %d out of %d IDs (%.2f%%)",
		collisions, count, float64(collisions)/float64(count)*100)

	// We EXPECT collisions -- this proves why flow labels alone are insufficient
	if collisions == 0 {
		t.Error("expected some flow label collisions (20-bit space); got zero -- check implementation")
	}
}

func TestTraceIDFromFlowLabel(t *testing.T) {
	// Test that flow label keys are properly masked to 20 bits
	tests := []struct {
		input    uint32
		expected uint32
	}{
		{0x00000000, 0x00000000},
		{0x000FFFFF, 0x000FFFFF},
		{0xFFFFFFFF, 0x000FFFFF}, // Upper bits stripped
		{0xFFF12345, 0x00012345}, // Only lower 20 bits kept
		{0x00012345, 0x00012345},
	}

	for _, tt := range tests {
		key := TraceIDFromFlowLabel(tt.input)
		if uint32(key) != tt.expected {
			t.Errorf("TraceIDFromFlowLabel(%#x) = %#x, want %#x", tt.input, uint32(key), tt.expected)
		}
	}
}

func TestFlowLabelKey_String(t *testing.T) {
	key := TraceIDFromFlowLabel(0x1234)
	if s := key.String(); s != "01234" {
		t.Errorf("FlowLabelKey.String() = %q, want %q", s, "01234")
	}
}

func TestFlowLabelKey_Bytes(t *testing.T) {
	key := TraceIDFromFlowLabel(0xABCDE)
	b := key.Bytes()
	if len(b) != 4 {
		t.Fatalf("FlowLabelKey.Bytes() length = %d, want 4", len(b))
	}
	// Big-endian: 0x000ABCDE
	if b[0] != 0x00 || b[1] != 0x0A || b[2] != 0xBC || b[3] != 0xDE {
		t.Errorf("FlowLabelKey.Bytes() = %#v, want [0x00, 0x0A, 0xBC, 0xDE]", b)
	}
}

func TestTraceIDFromBytes(t *testing.T) {
	original, err := NewTraceID()
	if err != nil {
		t.Fatalf("NewTraceID() failed: %v", err)
	}

	restored, err := TraceIDFromBytes(original.Bytes())
	if err != nil {
		t.Fatalf("TraceIDFromBytes() failed: %v", err)
	}

	if original != restored {
		t.Errorf("round-trip failed: %s != %s", original, restored)
	}
}

func TestTraceIDFromBytes_InvalidLength(t *testing.T) {
	for _, n := range []int{0, 1, 8, 15, 17, 32} {
		_, err := TraceIDFromBytes(make([]byte, n))
		if err == nil {
			t.Errorf("TraceIDFromBytes(make([]byte, %d)) should fail", n)
		}
	}
}

func TestTraceIDFromHex(t *testing.T) {
	original, err := NewTraceID()
	if err != nil {
		t.Fatalf("NewTraceID() failed: %v", err)
	}

	restored, err := TraceIDFromHex(original.String())
	if err != nil {
		t.Fatalf("TraceIDFromHex() failed: %v", err)
	}

	if original != restored {
		t.Errorf("round-trip failed: %s != %s", original, restored)
	}
}

func TestTraceIDFromHex_WithDashes(t *testing.T) {
	// Accept UUID-style format with dashes
	hexStr := "01234567-89ab-cdef-0123-456789abcdef"
	id, err := TraceIDFromHex(hexStr)
	if err != nil {
		t.Fatalf("TraceIDFromHex(%q) failed: %v", hexStr, err)
	}
	if id.IsZero() {
		t.Error("TraceIDFromHex returned zero ID")
	}
}

func TestTraceIDFromHex_Invalid(t *testing.T) {
	tests := []string{
		"",
		"too-short",
		"0123456789abcdef0123456789abcdeg", // invalid hex char
		"0123456789abcdef",                  // 16 chars (only 8 bytes)
	}
	for _, s := range tests {
		_, err := TraceIDFromHex(s)
		if err == nil {
			t.Errorf("TraceIDFromHex(%q) should fail", s)
		}
	}
}

func TestTraceID_IsZero(t *testing.T) {
	var zero TraceID
	if !zero.IsZero() {
		t.Error("zero TraceID.IsZero() should return true")
	}

	id, err := NewTraceID()
	if err != nil {
		t.Fatalf("NewTraceID() failed: %v", err)
	}
	if id.IsZero() {
		t.Error("generated TraceID.IsZero() should return false")
	}
}

func TestTraceID_String_Length(t *testing.T) {
	id, err := NewTraceID()
	if err != nil {
		t.Fatalf("NewTraceID() failed: %v", err)
	}
	s := id.String()
	if len(s) != 32 {
		t.Errorf("TraceID.String() length = %d, want 32", len(s))
	}
}

func TestTraceID_TimestampMS(t *testing.T) {
	before := time.Now().UnixMilli()
	id, err := NewTraceID()
	if err != nil {
		t.Fatalf("NewTraceID() failed: %v", err)
	}
	after := time.Now().UnixMilli()

	ts := id.TimestampMS()
	// Allow 10ms tolerance for clock skew between uuid library and time.Now()
	tolerance := int64(10)
	if ts < before-tolerance || ts > after+tolerance {
		t.Errorf("TimestampMS() = %d, want between %d and %d (tolerance %dms)",
			ts, before-tolerance, after+tolerance, tolerance)
	}
}

func TestTraceID_W3CTraceParent(t *testing.T) {
	id, err := NewTraceID()
	if err != nil {
		t.Fatalf("NewTraceID() failed: %v", err)
	}
	spanID := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	tp := id.W3CTraceParent(spanID, true)
	// Format: "00-{32 hex}-{16 hex}-01"
	if len(tp) != 55 {
		t.Errorf("W3CTraceParent length = %d, want 55", len(tp))
	}
	if tp[:3] != "00-" {
		t.Errorf("W3CTraceParent does not start with '00-': %s", tp)
	}
	if tp[len(tp)-2:] != "01" {
		t.Errorf("W3CTraceParent sampled flag should be '01': %s", tp)
	}

	// Unsampled
	tp2 := id.W3CTraceParent(spanID, false)
	if tp2[len(tp2)-2:] != "00" {
		t.Errorf("W3CTraceParent unsampled flag should be '00': %s", tp2)
	}
}

func TestMustNewTraceID(t *testing.T) {
	// MustNewTraceID should not panic under normal conditions
	id := MustNewTraceID()
	if id.IsZero() {
		t.Error("MustNewTraceID() returned zero ID")
	}
}
