// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package shield

import (
	"testing"
	"time"
)

func TestPacketBitmapWindow_Fresh(t *testing.T) {
	w := NewPacketBitmapWindow()

	if !w.Check("flow1", 1) {
		t.Fatal("first packet should be fresh")
	}
	if !w.Check("flow1", 2) {
		t.Fatal("second packet should be fresh")
	}
	if !w.Check("flow1", 5) {
		t.Fatal("skip to 5 should be fresh")
	}
}

func TestPacketBitmapWindow_Replay(t *testing.T) {
	w := NewPacketBitmapWindow()

	w.Check("flow1", 1)
	if w.Check("flow1", 1) {
		t.Fatal("replay of seq 1 should be detected")
	}

	w.Check("flow1", 10)
	if w.Check("flow1", 10) {
		t.Fatal("replay of seq 10 should be detected")
	}
}

func TestPacketBitmapWindow_OldOutsideWindow(t *testing.T) {
	w := NewPacketBitmapWindow()

	// Advance to seq 100.
	w.Check("flow1", 100)

	// Seq 1 is outside 64-bit window (100-1 = 99 > 64).
	if w.Check("flow1", 1) {
		t.Fatal("very old seq should be rejected")
	}

	// Seq 40 is within window (100-40 = 60 < 64).
	if !w.Check("flow1", 40) {
		t.Fatal("seq 40 should be within window")
	}
}

func TestPacketBitmapWindow_SeparateFlows(t *testing.T) {
	w := NewPacketBitmapWindow()

	w.Check("flow1", 1)
	if !w.Check("flow2", 1) {
		t.Fatal("same seq on different flow should be fresh")
	}

	if w.FlowCount() != 2 {
		t.Fatalf("FlowCount = %d, want 2", w.FlowCount())
	}
}

func TestPacketBitmapWindow_LargeSkip(t *testing.T) {
	w := NewPacketBitmapWindow()

	w.Check("flow1", 1)
	// Jump forward by more than 64, clearing the bitmap.
	if !w.Check("flow1", 100) {
		t.Fatal("large skip should be accepted")
	}

	// Old seq 1 should now be outside window.
	if w.Check("flow1", 1) {
		t.Fatal("old seq after large skip should be rejected")
	}
}

func TestEntropyMonitor_Check(t *testing.T) {
	em := NewEntropyMonitor(256)

	bits := em.AvailableBits()
	if bits == -1 {
		t.Skip("entropy file not available (non-Linux)")
	}

	// On a healthy Linux system, entropy should be >= 256.
	err := em.Check()
	if err != nil {
		t.Logf("entropy check: %v (bits=%d)", err, bits)
	}
}

func TestEntropyMonitor_NonLinux(t *testing.T) {
	em := NewEntropyMonitor(256)
	em.entropyFile = "/nonexistent/path"

	bits := em.AvailableBits()
	if bits != -1 {
		t.Fatalf("non-existent file should return -1, got %d", bits)
	}
}

func TestTOCTOUCache_PinAndCheck(t *testing.T) {
	cache := NewTOCTOUCache(10 * time.Second)

	if cache.IsVerified("flow1", 100) {
		t.Fatal("should not be verified before pinning")
	}

	cache.Pin("flow1", 100)

	if !cache.IsVerified("flow1", 100) {
		t.Fatal("should be verified after pinning")
	}

	if cache.IsVerified("flow1", 200) {
		t.Fatal("different sigRef should not be verified")
	}

	if cache.Size() != 1 {
		t.Fatalf("Size = %d, want 1", cache.Size())
	}
}

func TestTOCTOUCache_Expiry(t *testing.T) {
	cache := NewTOCTOUCache(1 * time.Millisecond)

	cache.Pin("flow1", 100)
	time.Sleep(5 * time.Millisecond)

	if cache.IsVerified("flow1", 100) {
		t.Fatal("should not be verified after TTL expires")
	}
}

func TestTOCTOUCache_Evict(t *testing.T) {
	cache := NewTOCTOUCache(1 * time.Millisecond)

	cache.Pin("flow1", 1)
	cache.Pin("flow2", 2)
	cache.Pin("flow3", 3)

	time.Sleep(5 * time.Millisecond)

	evicted := cache.Evict()
	if evicted != 3 {
		t.Fatalf("Evict = %d, want 3", evicted)
	}
	if cache.Size() != 0 {
		t.Fatalf("Size after evict = %d, want 0", cache.Size())
	}
}

func TestSecurityHardening_Validate(t *testing.T) {
	sh := NewSecurityHardening()

	pkt := &PQCPacketInfo{
		Flags:  0x0A,
		SigRef: 1,
		KeyRef: 1,
		SeqNum: 1,
	}

	// First validation should pass.
	if err := sh.Validate(pkt, "flow1"); err != nil {
		t.Fatalf("first validation should pass: %v", err)
	}

	// Same seq should fail (replay).
	if err := sh.Validate(pkt, "flow1"); err == nil {
		t.Fatal("replay should fail")
	}

	// Different seq should pass.
	pkt2 := &PQCPacketInfo{
		Flags:  0x0A,
		SigRef: 1,
		KeyRef: 1,
		SeqNum: 2,
	}
	if err := sh.Validate(pkt2, "flow1"); err != nil {
		t.Fatalf("different seq should pass: %v", err)
	}
}

func TestSecurityHardening_PinVerification(t *testing.T) {
	sh := NewSecurityHardening()

	sh.PinVerification("flow1", 100)

	// After pinning, Validate should short-circuit.
	pkt := &PQCPacketInfo{
		Flags:  0x0A,
		SigRef: 100,
		KeyRef: 1,
		SeqNum: 1,
	}
	if err := sh.Validate(pkt, "flow1"); err != nil {
		t.Fatalf("pinned verification should pass: %v", err)
	}
}

func TestSecurityHardening_CheckEntropy(t *testing.T) {
	sh := NewSecurityHardening()

	err := sh.CheckEntropy()
	bits := sh.EntropyBits()

	if bits == -1 {
		t.Skip("entropy file not available")
	}

	if err != nil && bits >= 256 {
		t.Fatalf("entropy check failed but bits=%d >= 256", bits)
	}
}

func TestSecurityHardening_ValidateWithAlgo(t *testing.T) {
	sh := NewSecurityHardening()

	pkt := &PQCPacketInfo{
		Flags:  0x0A,
		SigRef: 1,
		KeyRef: 1,
		SeqNum: 1,
	}

	if err := sh.ValidateWithAlgo(pkt, "flow1", 0x02); err != nil {
		t.Fatalf("first validation with algo should pass: %v", err)
	}

	// Same flow, different algo should fail (algo confusion).
	pkt2 := &PQCPacketInfo{
		Flags:  0x0A,
		SigRef: 2,
		KeyRef: 1,
		SeqNum: 2,
	}
	if err := sh.ValidateWithAlgo(pkt2, "flow1", 0x01); err == nil {
		t.Fatal("algo confusion should be detected")
	}
}
