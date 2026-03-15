// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

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
		PolicyMode:   0x01,      // PESSIMISTIC
		ActiveTier:   0x02,
		KEMTunnels:   16,
		LastVerifyNs: 850_000,
		SeqHighWater: 99999,
		Reserved:     0,
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
		{"Reserved", 0x24, 4, uint64(s.Reserved)},
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
