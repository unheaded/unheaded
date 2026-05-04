// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotan

import (
	"encoding/binary"
	"sync"
	"testing"
)

// referenceState returns a fully populated PQCState for testing.
func referenceState() PQCState {
	return PQCState{
		SigCount:     42,
		KeyCount:     7,
		VerifyPass:   1000,
		VerifyFail:   3,
		LastRotation: 1709568000, // 2024-03-04T16:00:00Z
		PolicyMode:   PolicyModePessimistic,
		ActiveTier:   ActiveTierEnhanced,
		KEMTunnels:   16,
		LastVerifyNs: 850_000,
		SeqHighWater: 99999,
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	original := referenceState()
	buf := original.Marshal()

	restored, err := UnmarshalPQCState(buf[:])
	if err != nil {
		t.Fatalf("UnmarshalPQCState: %v", err)
	}

	if restored != original {
		t.Fatalf("round-trip mismatch:\n  got  %+v\n  want %+v", restored, original)
	}
}

func TestMarshalSize(t *testing.T) {
	var s PQCState
	buf := s.Marshal()

	if len(buf) != PQCStateSize {
		t.Fatalf("marshal size = %d, want %d", len(buf), PQCStateSize)
	}

	if PQCStateSize != 40 {
		t.Fatalf("PQCStateSize = %d, want 40", PQCStateSize)
	}
}

func TestUnmarshalBufferTooSmall(t *testing.T) {
	_, err := UnmarshalPQCState(make([]byte, 39))
	if err == nil {
		t.Fatal("expected error for undersized buffer, got nil")
	}
}

func TestFieldOffsets(t *testing.T) {
	s := referenceState()
	buf := s.Marshal()

	tests := []struct {
		name   string
		offset int
		size   int
		want   uint64
	}{
		{"SigCount", 0x00, 4, uint64(s.SigCount)},
		{"KeyCount", 0x04, 4, uint64(s.KeyCount)},
		{"VerifyPass", 0x08, 4, uint64(s.VerifyPass)},
		{"VerifyFail", 0x0C, 4, uint64(s.VerifyFail)},
		{"LastRotation", 0x10, 8, uint64(s.LastRotation)},
		{"PolicyMode", 0x18, 1, uint64(s.PolicyMode)},
		{"ActiveTier", 0x19, 1, uint64(s.ActiveTier)},
		{"KEMTunnels", 0x1A, 2, uint64(s.KEMTunnels)},
		{"LastVerifyNs", 0x1C, 4, uint64(s.LastVerifyNs)},
		{"SeqHighWater", 0x20, 4, uint64(s.SeqHighWater)},
		{"ReservedZero", 0x24, 4, 0}, // reserved range is forced to zero on the wire
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got uint64
			switch tc.size {
			case 1:
				got = uint64(buf[tc.offset])
			case 2:
				got = uint64(binary.BigEndian.Uint16(buf[tc.offset:]))
			case 4:
				got = uint64(binary.BigEndian.Uint32(buf[tc.offset:]))
			case 8:
				got = binary.BigEndian.Uint64(buf[tc.offset:])
			}
			if got != tc.want {
				t.Fatalf("offset 0x%02X: got %d, want %d", tc.offset, got, tc.want)
			}
		})
	}
}

func TestAddressRange(t *testing.T) {
	if PQCStateBaseAddr != 0xFF00 {
		t.Fatalf("PQCStateBaseAddr = 0x%04X, want 0xFF00", PQCStateBaseAddr)
	}
	if PQCStateEndAddr != 0xFF27 {
		t.Fatalf("PQCStateEndAddr = 0x%04X, want 0xFF27", PQCStateEndAddr)
	}
	span := PQCStateEndAddr - PQCStateBaseAddr + 1
	if span != PQCStateSize {
		t.Fatalf("address span = %d, want %d", span, PQCStateSize)
	}
}

func TestDefaultStateZeroed(t *testing.T) {
	mgr := NewPQCStateManager()
	s := mgr.Read()

	if s != (PQCState{}) {
		t.Fatalf("default state not zeroed: %+v", s)
	}
}

func TestCASSucceedsOnMatch(t *testing.T) {
	mgr := NewPQCStateManager()

	desired := referenceState()
	if !mgr.CAS(PQCState{}, desired) {
		t.Fatal("CAS should succeed when current matches expected")
	}

	got := mgr.Read()
	if got != desired {
		t.Fatalf("state after CAS:\n  got  %+v\n  want %+v", got, desired)
	}
}

func TestCASFailsOnMismatch(t *testing.T) {
	mgr := NewPQCStateManager()
	mgr.Write(referenceState())

	wrongExpected := PQCState{SigCount: 999}
	desired := PQCState{SigCount: 1}

	if mgr.CAS(wrongExpected, desired) {
		t.Fatal("CAS should fail when current does not match expected")
	}

	// State must be unchanged.
	got := mgr.Read()
	if got != referenceState() {
		t.Fatalf("state changed after failed CAS:\n  got  %+v\n  want %+v", got, referenceState())
	}
}

func TestWriteRead(t *testing.T) {
	mgr := NewPQCStateManager()
	s := referenceState()

	mgr.Write(s)
	got := mgr.Read()

	if got != s {
		t.Fatalf("Read after Write:\n  got  %+v\n  want %+v", got, s)
	}
}

func TestAtomicIncrVerifyPass(t *testing.T) {
	mgr := NewPQCStateManager()
	const n = 10_000

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			mgr.IncrVerifyPass()
		}()
	}
	wg.Wait()

	got := mgr.Read().VerifyPass
	if got != n {
		t.Fatalf("VerifyPass = %d, want %d", got, n)
	}
}

func TestAtomicIncrVerifyFail(t *testing.T) {
	mgr := NewPQCStateManager()
	const n = 10_000

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			mgr.IncrVerifyFail()
		}()
	}
	wg.Wait()

	got := mgr.Read().VerifyFail
	if got != n {
		t.Fatalf("VerifyFail = %d, want %d", got, n)
	}
}

func TestAtomicCountersConcurrentMixed(t *testing.T) {
	mgr := NewPQCStateManager()
	const n = 5_000

	var wg sync.WaitGroup
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			mgr.IncrVerifyPass()
		}()
		go func() {
			defer wg.Done()
			mgr.IncrVerifyFail()
		}()
	}
	wg.Wait()

	s := mgr.Read()
	if s.VerifyPass != n {
		t.Fatalf("VerifyPass = %d, want %d", s.VerifyPass, n)
	}
	if s.VerifyFail != n {
		t.Fatalf("VerifyFail = %d, want %d", s.VerifyFail, n)
	}
}

func TestReadSnapshotConsistency(t *testing.T) {
	mgr := NewPQCStateManager()
	mgr.Write(referenceState())

	// Take multiple snapshots concurrently while writing.
	var wg sync.WaitGroup
	errs := make(chan string, 100)

	// Writers that update various fields.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1_000; i++ {
			mgr.UpdateSigCount(uint32(i))
			mgr.IncrVerifyPass()
		}
	}()

	// Readers that take snapshots.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1_000; i++ {
			s := mgr.Read()
			// Snapshot must serialize cleanly (no partial writes).
			buf := s.Marshal()
			restored, err := UnmarshalPQCState(buf[:])
			if err != nil {
				errs <- err.Error()
				return
			}
			if restored != s {
				errs <- "snapshot round-trip mismatch"
				return
			}
		}
	}()

	wg.Wait()
	close(errs)

	for e := range errs {
		t.Fatalf("consistency error: %s", e)
	}
}

func TestUpdateSigCount(t *testing.T) {
	mgr := NewPQCStateManager()
	mgr.UpdateSigCount(42)

	if got := mgr.Read().SigCount; got != 42 {
		t.Fatalf("SigCount = %d, want 42", got)
	}
}

func TestUpdateKeyCount(t *testing.T) {
	mgr := NewPQCStateManager()
	mgr.UpdateKeyCount(7)

	if got := mgr.Read().KeyCount; got != 7 {
		t.Fatalf("KeyCount = %d, want 7", got)
	}
}

func TestSetLastRotation(t *testing.T) {
	mgr := NewPQCStateManager()
	mgr.SetLastRotation(1709568000)

	if got := mgr.Read().LastRotation; got != 1709568000 {
		t.Fatalf("LastRotation = %d, want 1709568000", got)
	}
}

func TestSetPolicyMode(t *testing.T) {
	mgr := NewPQCStateManager()
	mgr.SetPolicyMode(0x02)

	if got := mgr.Read().PolicyMode; got != 0x02 {
		t.Fatalf("PolicyMode = 0x%02X, want 0x02", got)
	}
}

func TestSetActiveTier(t *testing.T) {
	mgr := NewPQCStateManager()
	mgr.SetActiveTier(0x03)

	if got := mgr.Read().ActiveTier; got != 0x03 {
		t.Fatalf("ActiveTier = 0x%02X, want 0x03", got)
	}
}

func TestWriteSyncsAtomicCounters(t *testing.T) {
	mgr := NewPQCStateManager()

	// Increment counters first.
	mgr.IncrVerifyPass()
	mgr.IncrVerifyPass()
	mgr.IncrVerifyFail()

	// Write overwrites everything including counters.
	s := referenceState()
	mgr.Write(s)

	got := mgr.Read()
	if got.VerifyPass != s.VerifyPass {
		t.Fatalf("VerifyPass after Write = %d, want %d", got.VerifyPass, s.VerifyPass)
	}
	if got.VerifyFail != s.VerifyFail {
		t.Fatalf("VerifyFail after Write = %d, want %d", got.VerifyFail, s.VerifyFail)
	}
}

func TestMarshalZeroState(t *testing.T) {
	var s PQCState
	buf := s.Marshal()

	for i, b := range buf {
		if b != 0 {
			t.Fatalf("zero state byte[%d] = 0x%02X, want 0x00", i, b)
		}
	}
}

// TestUnmarshalBufferOversized — the old implementation silently
// accepted oversized buffers (`len(b) < PQCStateSize` rather than `!=`),
// reading only the first 40 bytes. The fix requires exact length.
func TestUnmarshalBufferOversized(t *testing.T) {
	buf := make([]byte, PQCStateSize+8) // +8 trailing bytes
	if _, err := UnmarshalPQCState(buf); err == nil {
		t.Fatal("expected error for oversized buffer, got nil")
	}
}

// TestUnmarshalRejectsNonZeroReserved — regression for the covert-channel
// finding (#3 from the 2026-05-03 review). The reserved 4-byte range at
// offset 0x24 round-tripped silently; the fix forces zero on Marshal and
// rejects non-zero on Unmarshal so endpoints can't smuggle data through it.
func TestUnmarshalRejectsNonZeroReserved(t *testing.T) {
	ref := referenceState()
	good := ref.Marshal()

	// Build a buffer with valid contents but non-zero reserved bytes.
	bad := good[:]
	bad[0x24] = 0xCA
	bad[0x25] = 0xFE
	bad[0x26] = 0xBA
	bad[0x27] = 0xBE

	if _, err := UnmarshalPQCState(bad); err == nil {
		t.Fatal("expected error for non-zero reserved bytes, got nil (covert channel open)")
	}
}

// TestMarshalForcesReservedZero — companion to the above. Even if a
// hypothetical caller managed to set reserved bytes via reflection or a
// later struct change, Marshal must zero them on the way out.
func TestMarshalForcesReservedZero(t *testing.T) {
	ref := referenceState()
	buf := ref.Marshal()
	for i := 0x24; i < 0x28; i++ {
		if buf[i] != 0 {
			t.Fatalf("Marshal byte[0x%02X] = 0x%02X, want 0x00 (reserved must be zero)", i, buf[i])
		}
	}
}

// TestCASMonotonicWithConcurrentIncr — regression for the CAS-vs-Incr
// race (finding #2). Old code held mu for state assignment but used a
// separate atomic.Uint64 for verifyPass; an IncrVerifyPass between
// CAS's atomic Load and CAS's atomic Store would be silently lost.
//
// The fix routes Incr through the same mutex; any IncrVerifyPass
// concurrent with a CAS now serializes cleanly. We verify the
// post-condition: the final counter equals (number of CAS-survivors
// applying VerifyPass=initial) + (number of Incrs that landed after
// the last successful CAS). For the simpler invariant we assert here:
// after N concurrent Incrs and exactly one CAS that sets VerifyPass=0,
// the final value must be at least the count of Incrs that ran AFTER
// the CAS. We can't tag those externally, so the loosest invariant
// that catches the bug is: total observed value across the run is
// non-decreasing once all goroutines have completed.
func TestCASMonotonicWithConcurrentIncr(t *testing.T) {
	const incrCount = 5_000

	mgr := NewPQCStateManager()
	mgr.Write(PQCState{VerifyPass: 100})

	var wg sync.WaitGroup
	wg.Add(incrCount + 1)

	for i := 0; i < incrCount; i++ {
		go func() {
			defer wg.Done()
			mgr.IncrVerifyPass()
		}()
	}

	// One CAS that swaps based on a current it expects to observe.
	go func() {
		defer wg.Done()
		// We don't care if this CAS succeeds — what matters is that
		// the increments serialize correctly around it. Try repeatedly
		// so we exercise the race window.
		for i := 0; i < 100; i++ {
			cur := mgr.Read()
			next := cur
			next.SigCount = 999
			mgr.CAS(cur, next)
		}
	}()

	wg.Wait()

	// All 5000 increments must be visible. With the buggy split-state
	// implementation, some IncrVerifyPass calls would be lost when CAS
	// stomped the atomic counter mid-flight.
	got := mgr.Read().VerifyPass
	want := uint32(100 + incrCount)
	if got != want {
		t.Fatalf("VerifyPass after %d concurrent Incrs + concurrent CAS: got %d, want %d (lost increments → CAS race regression)",
			incrCount, got, want)
	}
}

// TestVerifyCounterUint32WrapsCleanly — finding #1 (atomic.Uint64 with
// uint32 wire field caused silent truncation). Wire format dictates 4
// bytes; we now treat the counter as uint32 throughout, so wrap at
// 2^32 is well-defined modular arithmetic — not a truncation surprise.
func TestVerifyCounterUint32WrapsCleanly(t *testing.T) {
	mgr := NewPQCStateManager()
	mgr.Write(PQCState{VerifyPass: 0xFFFFFFFE}) // 2^32 - 2

	mgr.IncrVerifyPass() // → 0xFFFFFFFF
	if got := mgr.Read().VerifyPass; got != 0xFFFFFFFF {
		t.Fatalf("VerifyPass after Incr at MaxUint32-1: got 0x%08x, want 0xFFFFFFFF", got)
	}

	mgr.IncrVerifyPass() // → 0 (wrap)
	if got := mgr.Read().VerifyPass; got != 0 {
		t.Fatalf("VerifyPass wrap-around: got 0x%08x, want 0x00000000 (uint32 wrap)", got)
	}

	mgr.IncrVerifyPass() // → 1
	if got := mgr.Read().VerifyPass; got != 1 {
		t.Fatalf("VerifyPass after wrap: got 0x%08x, want 0x00000001", got)
	}
}

// TestApplyAtomicMultiField — finding #7 (collapse 6 SetX/UpdateX
// methods to one Apply). Verifies that Apply runs the callback under
// the write lock so concurrent observers can't see a half-applied
// multi-field edit.
func TestApplyAtomicMultiField(t *testing.T) {
	mgr := NewPQCStateManager()
	mgr.Apply(func(s *PQCState) {
		s.SigCount = 100
		s.KeyCount = 7
		s.LastRotation = 1709568000
		s.PolicyMode = PolicyModeOptimistic
		s.ActiveTier = ActiveTierStrict
	})

	got := mgr.Read()
	want := PQCState{
		SigCount:     100,
		KeyCount:     7,
		LastRotation: 1709568000,
		PolicyMode:   PolicyModeOptimistic,
		ActiveTier:   ActiveTierStrict,
	}
	if got != want {
		t.Fatalf("Apply multi-field:\n  got  %+v\n  want %+v", got, want)
	}
}

// TestApplyConcurrentReadersSeeOnlyFullEdits — Apply's contract is that
// readers either see the pre-edit state or the post-edit state, never a
// half-edit. Run a writer doing 1000 multi-field Apply calls in parallel
// with a reader; every read snapshot must be one of the two states the
// writer cycles through.
func TestApplyConcurrentReadersSeeOnlyFullEdits(t *testing.T) {
	mgr := NewPQCStateManager()

	stateA := PQCState{SigCount: 1, KeyCount: 100}
	stateB := PQCState{SigCount: 2, KeyCount: 200}
	mgr.Write(stateA)

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				mgr.Apply(func(s *PQCState) {
					if s.SigCount == 1 {
						*s = stateB
					} else {
						*s = stateA
					}
				})
			}
		}
	}()

	for i := 0; i < 1_000; i++ {
		s := mgr.Read()
		if s != stateA && s != stateB {
			close(stop)
			t.Fatalf("snapshot %d torn read: got %+v (want %+v or %+v)", i, s, stateA, stateB)
		}
	}
	close(stop)
}

// TestMarshalTo — finding #8 (zero-alloc serialization). Verifies that
// MarshalTo writes the exact bytes Marshal() would, into a caller-
// supplied buffer, without allocating.
func TestMarshalTo(t *testing.T) {
	ref := referenceState()

	// Fixed-array path
	wantArr := ref.Marshal()

	// MarshalTo into a slice of exactly 40 bytes
	dst := make([]byte, PQCStateSize)
	if err := ref.MarshalTo(dst); err != nil {
		t.Fatalf("MarshalTo: %v", err)
	}
	for i := range wantArr {
		if dst[i] != wantArr[i] {
			t.Fatalf("byte %d: MarshalTo=%02x, Marshal=%02x", i, dst[i], wantArr[i])
		}
	}

	// Round-trip through MarshalTo
	restored, err := UnmarshalPQCState(dst)
	if err != nil {
		t.Fatalf("Unmarshal MarshalTo output: %v", err)
	}
	if restored != ref {
		t.Fatalf("round-trip mismatch:\n  got  %+v\n  want %+v", restored, ref)
	}
}

func TestMarshalToRejectsWrongSize(t *testing.T) {
	ref := referenceState()
	for _, n := range []int{0, 1, 39, 41, 100} {
		dst := make([]byte, n)
		if err := ref.MarshalTo(dst); err == nil {
			t.Fatalf("MarshalTo with len=%d: expected error, got nil", n)
		}
	}
}

func TestMarshalToZerosReserved(t *testing.T) {
	ref := referenceState()
	// Pre-fill dst with non-zero junk; MarshalTo must overwrite reserved bytes.
	dst := make([]byte, PQCStateSize)
	for i := range dst {
		dst[i] = 0xAA
	}
	if err := ref.MarshalTo(dst); err != nil {
		t.Fatalf("MarshalTo: %v", err)
	}
	for i := 0x24; i < 0x28; i++ {
		if dst[i] != 0 {
			t.Fatalf("reserved byte[0x%02X] = 0x%02X, want 0x00", i, dst[i])
		}
	}
}

// TestMarshalToZeroAlloc — confirms MarshalTo doesn't allocate. The
// motivating use case for this method is writing into a pre-existing
// protocol-RAM region without taking a heap allocation per call.
func TestMarshalToZeroAlloc(t *testing.T) {
	ref := referenceState()
	dst := make([]byte, PQCStateSize)
	allocs := testing.AllocsPerRun(100, func() {
		_ = ref.MarshalTo(dst)
	})
	if allocs != 0 {
		t.Fatalf("MarshalTo allocated %v times per call, want 0", allocs)
	}
}

// TestPolicyAndTierConstants — finding #6 (no typed constants for the
// documented enum values). Tests that the named constants resolve to
// the wire-documented byte values, so any future caller relying on the
// constants is talking to the same bytes the spec describes.
func TestPolicyAndTierConstants(t *testing.T) {
	if PolicyModePessimistic != 0x01 {
		t.Fatalf("PolicyModePessimistic = 0x%02X, want 0x01", PolicyModePessimistic)
	}
	if PolicyModeOptimistic != 0x02 {
		t.Fatalf("PolicyModeOptimistic = 0x%02X, want 0x02", PolicyModeOptimistic)
	}
	tiers := []struct {
		name string
		got  uint8
		want uint8
	}{
		{"Baseline", ActiveTierBaseline, 0x00},
		{"Standard", ActiveTierStandard, 0x01},
		{"Enhanced", ActiveTierEnhanced, 0x02},
		{"Strict", ActiveTierStrict, 0x03},
	}
	for _, tc := range tiers {
		if tc.got != tc.want {
			t.Fatalf("ActiveTier%s = 0x%02X, want 0x%02X", tc.name, tc.got, tc.want)
		}
	}
}
