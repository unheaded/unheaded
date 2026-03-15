// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package sophia

import (
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentMapAccess(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	const goroutines = 50
	const opsPerGoroutine = 20

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*opsPerGoroutine)

	// Concurrent signature loads
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gIdx int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				entry := &PQCSigEntry{
					AlgoID:    uint8((gIdx*opsPerGoroutine + i) % 5),
					ParamSet:  uint8(i % 3),
					Signature: []byte{byte(gIdx), byte(i)},
				}
				ref, err := mgr.LoadSignature(entry)
				if err != nil {
					errCh <- fmt.Errorf("goroutine %d, op %d: LoadSignature: %w", gIdx, i, err)
					return
				}

				// Immediately look it up
				_, err = mgr.LookupSignature(ref)
				if err != nil {
					errCh <- fmt.Errorf("goroutine %d, op %d: LookupSignature(%d): %w", gIdx, i, ref, err)
					return
				}
			}
		}(g)
	}

	// Concurrent key loads
	for g := 0; g < goroutines/5; g++ {
		wg.Add(1)
		go func(gIdx int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				entry := &PQCKeyEntry{
					AlgoID:    uint8(i % 5),
					Status:    KeyStatusActive,
					PublicKey: []byte{byte(gIdx), byte(i)},
				}
				ref, err := mgr.LoadKey(entry)
				if err != nil {
					errCh <- fmt.Errorf("goroutine %d, op %d: LoadKey: %w", gIdx, i, err)
					return
				}

				_, err = mgr.LookupKey(ref)
				if err != nil {
					errCh <- fmt.Errorf("goroutine %d, op %d: LookupKey(%d): %w", gIdx, i, ref, err)
					return
				}
			}
		}(g)
	}

	// Concurrent policy loads
	for g := 0; g < goroutines/10; g++ {
		wg.Add(1)
		go func(gIdx int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				policy := &PQCPolicy{
					SrcServiceID: uint16(gIdx),
					DstServiceID: uint16(i),
					Mode:         PolicyModePessimistic,
				}
				if err := mgr.LoadPolicy(policy); err != nil {
					errCh <- fmt.Errorf("goroutine %d, op %d: LoadPolicy: %w", gIdx, i, err)
					return
				}

				_, err := mgr.LookupPolicy(uint16(gIdx), uint16(i))
				if err != nil {
					errCh <- fmt.Errorf("goroutine %d, op %d: LookupPolicy: %w", gIdx, i, err)
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	// Verify final counts are consistent
	expectedSigs := int64(goroutines * opsPerGoroutine)
	if mgr.SigCount() != expectedSigs {
		t.Errorf("SigCount: got %d, want %d", mgr.SigCount(), expectedSigs)
	}

	expectedKeys := int64((goroutines / 5) * opsPerGoroutine)
	if mgr.KeyCount() != expectedKeys {
		t.Errorf("KeyCount: got %d, want %d", mgr.KeyCount(), expectedKeys)
	}
}

func TestSigRefAllocationMonotonicallyIncreasing(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	var prevRef uint32
	for i := 0; i < 100; i++ {
		entry := &PQCSigEntry{
			AlgoID:    0x01,
			Signature: []byte{byte(i)},
		}
		ref, err := mgr.LoadSignature(entry)
		if err != nil {
			t.Fatalf("LoadSignature(%d) error: %v", i, err)
		}

		if i > 0 && ref <= prevRef {
			t.Errorf("SigRef not monotonically increasing: ref[%d]=%d <= ref[%d]=%d",
				i, ref, i-1, prevRef)
		}
		prevRef = ref
	}
}

func TestKeyRefAllocation(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	var prevRef uint16
	for i := 0; i < 50; i++ {
		entry := &PQCKeyEntry{
			AlgoID:    uint8(i%5 + 1),
			Status:    KeyStatusActive,
			PublicKey: []byte{byte(i)},
		}
		ref, err := mgr.LoadKey(entry)
		if err != nil {
			t.Fatalf("LoadKey(%d) error: %v", i, err)
		}

		if i > 0 && ref <= prevRef {
			t.Errorf("KeyRef not monotonically increasing: ref[%d]=%d <= ref[%d]=%d",
				i, ref, i-1, prevRef)
		}
		prevRef = ref
	}
}

func TestSigRefStartsAtZero(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	ref, err := mgr.LoadSignature(&PQCSigEntry{AlgoID: 0x01, Signature: []byte{0x00}})
	if err != nil {
		t.Fatalf("LoadSignature() error: %v", err)
	}
	if ref != 0 {
		t.Errorf("first SigRef: got %d, want 0", ref)
	}
}

func TestKeyRefStartsAtZero(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	ref, err := mgr.LoadKey(&PQCKeyEntry{AlgoID: 0x01, PublicKey: []byte{0x00}})
	if err != nil {
		t.Fatalf("LoadKey() error: %v", err)
	}
	if ref != 0 {
		t.Errorf("first KeyRef: got %d, want 0", ref)
	}
}

func TestErrorOnOverflow(t *testing.T) {
	t.Run("signature overflow", func(t *testing.T) {
		mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
		if err := mgr.CreatePQCMaps(); err != nil {
			t.Fatalf("CreatePQCMaps() error: %v", err)
		}

		// Fill the map to capacity artificially
		mgr.mu.Lock()
		for i := uint32(0); i < MaxSignatures; i++ {
			mgr.signatures[i] = &PQCSigEntry{AlgoID: 0x01}
		}
		mgr.mu.Unlock()

		// Attempt to load one more
		_, err := mgr.LoadSignature(&PQCSigEntry{AlgoID: 0x01, Signature: []byte{0xFF}})
		if err == nil {
			t.Error("expected error on signature map overflow")
		}
		// Verify error message follows project convention
		want := fmt.Sprintf("sophia: %s: map full (%d/%d)", PQCSigMapName, MaxSignatures, MaxSignatures)
		if err.Error() != want {
			t.Errorf("error message: got %q, want %q", err.Error(), want)
		}
	})

	t.Run("key overflow", func(t *testing.T) {
		mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
		if err := mgr.CreatePQCMaps(); err != nil {
			t.Fatalf("CreatePQCMaps() error: %v", err)
		}

		// Fill the map to capacity
		mgr.mu.Lock()
		for i := uint16(0); i < MaxKeys; i++ {
			mgr.keys[i] = &PQCKeyEntry{AlgoID: 0x01}
		}
		mgr.mu.Unlock()

		_, err := mgr.LoadKey(&PQCKeyEntry{AlgoID: 0x01, PublicKey: []byte{0xFF}})
		if err == nil {
			t.Error("expected error on key map overflow")
		}
		want := fmt.Sprintf("sophia: %s: map full (%d/%d)", PQCKeyMapName, MaxKeys, MaxKeys)
		if err.Error() != want {
			t.Errorf("error message: got %q, want %q", err.Error(), want)
		}
	})

	t.Run("policy overflow", func(t *testing.T) {
		mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
		if err := mgr.CreatePQCMaps(); err != nil {
			t.Fatalf("CreatePQCMaps() error: %v", err)
		}

		// Fill the policy map to capacity
		mgr.mu.Lock()
		for i := uint32(0); i < MaxPolicies; i++ {
			mgr.policies[i] = &PQCPolicy{Mode: PolicyModePessimistic}
		}
		mgr.mu.Unlock()

		err := mgr.LoadPolicy(&PQCPolicy{
			SrcServiceID: 0xFFFF,
			DstServiceID: 0xFFFF,
			Mode:         PolicyModeOptimistic,
		})
		if err == nil {
			t.Error("expected error on policy map overflow")
		}
	})

	t.Run("sovereign sig overflow", func(t *testing.T) {
		mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
		if err := mgr.CreatePQCMaps(); err != nil {
			t.Fatalf("CreatePQCMaps() error: %v", err)
		}

		// Fill the sovereign sig map to capacity
		mgr.mu.Lock()
		for i := uint32(0); i < MaxSovereignSigs; i++ {
			mgr.sovereignSigs[i] = &SovereignSigEntry{}
		}
		mgr.mu.Unlock()

		err := mgr.LoadSovereignSig(MaxSovereignSigs+1, &SovereignSigEntry{AlgoID1: 0x01})
		if err == nil {
			t.Error("expected error on sovereign sig map overflow")
		}
	})

	t.Run("KEM key overflow", func(t *testing.T) {
		mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
		if err := mgr.CreatePQCMaps(); err != nil {
			t.Fatalf("CreatePQCMaps() error: %v", err)
		}

		// Fill the KEM key map to capacity
		mgr.mu.Lock()
		for i := uint16(0); i < MaxKEMKeys; i++ {
			mgr.kemKeys[i] = &KEMKeyEntry{}
		}
		mgr.mu.Unlock()

		err := mgr.LoadKEMKey(MaxKEMKeys+1, &KEMKeyEntry{AlgoID: 0x01})
		if err == nil {
			t.Error("expected error on KEM key map overflow")
		}
	})
}

func TestConcurrentReadsDuringWrites(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	// Pre-load some data
	for i := 0; i < 10; i++ {
		mgr.LoadSignature(&PQCSigEntry{AlgoID: uint8(i), Signature: []byte{byte(i)}})
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	// Readers
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				// Read stats (should never panic)
				_ = mgr.SigCount()
				_ = mgr.KeyCount()

				// Try lookups (some may not exist, that is fine)
				mgr.LookupSignature(uint32(i % 20))
			}
		}()
	}

	// Writers
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func(gIdx int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_, err := mgr.LoadSignature(&PQCSigEntry{
					AlgoID:    uint8(gIdx),
					Signature: []byte{byte(i)},
				})
				if err != nil {
					errCh <- err
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

func TestCreatePQCMapsResetsState(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	// Load some data
	mgr.LoadSignature(&PQCSigEntry{AlgoID: 0x01, Signature: []byte{0x00}})
	mgr.LoadKey(&PQCKeyEntry{AlgoID: 0x01, PublicKey: []byte{0x00}})
	mgr.LoadPolicy(&PQCPolicy{SrcServiceID: 1, DstServiceID: 2, Mode: PolicyModePessimistic})

	if mgr.SigCount() != 1 || mgr.KeyCount() != 1 {
		t.Fatal("pre-condition failed: expected 1 sig and 1 key")
	}

	// Re-create maps should reset everything
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() reset error: %v", err)
	}

	if mgr.SigCount() != 0 {
		t.Errorf("after reset SigCount: got %d, want 0", mgr.SigCount())
	}
	if mgr.KeyCount() != 0 {
		t.Errorf("after reset KeyCount: got %d, want 0", mgr.KeyCount())
	}

	// Previous entries should not be findable
	_, err := mgr.LookupSignature(0)
	if err == nil {
		t.Error("expected error looking up sig after reset")
	}
	_, err = mgr.LookupKey(0)
	if err == nil {
		t.Error("expected error looking up key after reset")
	}
	_, err = mgr.LookupPolicy(1, 2)
	if err == nil {
		t.Error("expected error looking up policy after reset")
	}
}

func TestNewPQCMapManagerInitialization(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/test")

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.signatures == nil {
		t.Error("expected signatures map to be initialized")
	}
	if mgr.keys == nil {
		t.Error("expected keys map to be initialized")
	}
	if mgr.policies == nil {
		t.Error("expected policies map to be initialized")
	}
	if mgr.sovereignSigs == nil {
		t.Error("expected sovereignSigs map to be initialized")
	}
	if mgr.kemKeys == nil {
		t.Error("expected kemKeys map to be initialized")
	}
	if mgr.pinBasePath != "/sys/fs/bpf/test" {
		t.Errorf("pinBasePath: got %q, want %q", mgr.pinBasePath, "/sys/fs/bpf/test")
	}
}
