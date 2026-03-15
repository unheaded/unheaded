// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophia

import (
	"testing"
)

func TestKEMRegistryLookup(t *testing.T) {
	tests := []struct {
		name      string
		wantAlgo  uint8
		wantAvail bool
	}{
		{"ML-KEM-512", 0x04, true},
		{"ML-KEM-768", 0x04, true},
		{"ML-KEM-1024", 0x04, true},
		{"HQC-128", 0x05, false},
		{"HQC-192", 0x05, false},
	}

	for _, tt := range tests {
		p, ok := LookupKEMParam(tt.name)
		if !ok {
			t.Errorf("LookupKEMParam(%q) not found", tt.name)
			continue
		}
		if p.AlgoID != tt.wantAlgo {
			t.Errorf("%s: AlgoID = 0x%02x, want 0x%02x", tt.name, p.AlgoID, tt.wantAlgo)
		}
		if p.Available != tt.wantAvail {
			t.Errorf("%s: Available = %v, want %v", tt.name, p.Available, tt.wantAvail)
		}
	}
}

func TestKEMRegistryByID(t *testing.T) {
	p, ok := LookupKEMByID(0x04, 0x02)
	if !ok {
		t.Fatal("ML-KEM-768 not found by ID")
	}
	if p.Name != "ML-KEM-768" {
		t.Errorf("Name = %q, want ML-KEM-768", p.Name)
	}
}

func TestKEMRegistryUnknown(t *testing.T) {
	_, ok := LookupKEMByID(0xFF, 0xFF)
	if ok {
		t.Fatal("should not find unknown KEM")
	}
}

func TestAvailableKEMParams(t *testing.T) {
	avail := AvailableKEMParams()
	for _, p := range avail {
		if !p.Available {
			t.Errorf("%s should be available", p.Name)
		}
	}
	if len(avail) != 3 {
		t.Errorf("expected 3 available KEM params, got %d", len(avail))
	}
}

func TestValidateKEMParam(t *testing.T) {
	if err := ValidateKEMParam(0x04, 0x01); err != nil {
		t.Errorf("ML-KEM-512 should be valid: %v", err)
	}
	if err := ValidateKEMParam(0xFF, 0xFF); err == nil {
		t.Error("unknown param should be invalid")
	}
}

func TestHKDFDeriveKey(t *testing.T) {
	secret := []byte("test-shared-secret-32-bytes-long")
	salt := []byte("test-salt")
	info := []byte("test-info")

	key, err := HKDFDeriveKey(secret, salt, info, 32)
	if err != nil {
		t.Fatalf("HKDFDeriveKey failed: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32", len(key))
	}

	// Same inputs = same output
	key2, _ := HKDFDeriveKey(secret, salt, info, 32)
	for i := range key {
		if key[i] != key2[i] {
			t.Fatal("deterministic: same inputs should produce same output")
		}
	}

	// Different info = different output
	key3, _ := HKDFDeriveKey(secret, salt, []byte("other-info"), 32)
	same := true
	for i := range key {
		if key[i] != key3[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different info should produce different output")
	}
}

func TestHKDFInvalidLength(t *testing.T) {
	if _, err := HKDFDeriveKey([]byte("key"), nil, nil, 0); err == nil {
		t.Error("length 0 should fail")
	}
	if _, err := HKDFDeriveKey([]byte("key"), nil, nil, -1); err == nil {
		t.Error("negative length should fail")
	}
}

func TestDeriveTunnelKey(t *testing.T) {
	secret := []byte("shared-secret-for-tunnel-derivation")
	key, err := DeriveTunnelKey(secret, "tun-abc123", 100, 200)
	if err != nil {
		t.Fatalf("DeriveTunnelKey failed: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("tunnel key length = %d, want 32", len(key))
	}
}

func TestDeriveNonce(t *testing.T) {
	secret := []byte("shared-secret-for-nonce-derivation")
	nonce, err := DeriveNonce(secret, 1)
	if err != nil {
		t.Fatalf("DeriveNonce failed: %v", err)
	}
	if len(nonce) != 12 {
		t.Errorf("nonce length = %d, want 12", len(nonce))
	}

	// Different counter = different nonce
	nonce2, _ := DeriveNonce(secret, 2)
	same := true
	for i := range nonce {
		if nonce[i] != nonce2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different counter should produce different nonce")
	}
}

func TestPolicyMap(t *testing.T) {
	pm := NewPolicyMap()

	yaml := []byte(`
version: "1.0"
policies:
  - service_id: 100
    service_name: "monad"
    mode: "pessimistic"
    required_tier: 2
    allowed_algos: [1, 2]
    max_sig_age_sec: 3600
    grace_sec: 60
  - service_id: 200
    service_name: "sophia"
    mode: "optimistic"
    required_tier: 1
`)

	if err := pm.LoadYAML(yaml); err != nil {
		t.Fatalf("LoadYAML failed: %v", err)
	}

	if pm.Count() != 2 {
		t.Errorf("count = %d, want 2", pm.Count())
	}

	p, ok := pm.Get(100)
	if !ok {
		t.Fatal("policy for service 100 not found")
	}
	if p.Mode != "pessimistic" {
		t.Errorf("mode = %q, want pessimistic", p.Mode)
	}
	if p.RequiredTier != 2 {
		t.Errorf("required_tier = %d, want 2", p.RequiredTier)
	}
}

func TestPolicyMapSetDelete(t *testing.T) {
	pm := NewPolicyMap()
	pm.Set(&AppPolicy{ServiceID: 300, ServiceName: "test"})

	if pm.Count() != 1 {
		t.Errorf("count after set = %d, want 1", pm.Count())
	}

	pm.Delete(300)
	if pm.Count() != 0 {
		t.Errorf("count after delete = %d, want 0", pm.Count())
	}
}

func TestSovereignMap(t *testing.T) {
	sm := NewSovereignMap()

	entry := &SovereignEntry{
		SigRef: 1,
		Slots: [3]SovereignSlot{
			{SigRef: 10, AlgoID: 0x01, Status: 1}, // SLH-DSA valid
			{SigRef: 20, AlgoID: 0x02, Status: 1}, // ML-DSA valid
			{SigRef: 30, AlgoID: 0x03, Status: 0}, // FN-DSA pending
		},
	}

	if err := sm.Store(entry); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	found, ok := sm.Lookup(1)
	if !ok {
		t.Fatal("entry not found")
	}
	if found.Slots[0].AlgoID != 0x01 {
		t.Errorf("slot 0 AlgoID = 0x%02x, want 0x01", found.Slots[0].AlgoID)
	}
}

func TestSovereignConsensus(t *testing.T) {
	sm := NewSovereignMap()

	// 2-of-3 from distinct families should pass
	entry := &SovereignEntry{
		SigRef: 1,
		Slots: [3]SovereignSlot{
			{SigRef: 10, AlgoID: 0x01, Status: 1}, // hash-based: valid
			{SigRef: 20, AlgoID: 0x02, Status: 1}, // lattice: valid
			{SigRef: 30, AlgoID: 0x03, Status: 2}, // lattice: invalid
		},
	}
	sm.Store(entry)

	if err := sm.UpdateConsensus(1); err != nil {
		t.Fatalf("UpdateConsensus failed: %v", err)
	}

	updated, _ := sm.Lookup(1)
	if updated.Consensus != 1 {
		t.Errorf("consensus = %d, want 1 (passed)", updated.Consensus)
	}
}

func TestSovereignConsensusSameFamily(t *testing.T) {
	sm := NewSovereignMap()

	// 2 valid but same family (lattice) = partial
	entry := &SovereignEntry{
		SigRef: 2,
		Slots: [3]SovereignSlot{
			{SigRef: 10, AlgoID: 0x02, Status: 1}, // ML-DSA: valid
			{SigRef: 20, AlgoID: 0x03, Status: 1}, // FN-DSA: valid (same family!)
			{SigRef: 30, AlgoID: 0x01, Status: 2}, // SLH-DSA: invalid
		},
	}
	sm.Store(entry)
	sm.UpdateConsensus(2)

	updated, _ := sm.Lookup(2)
	if updated.Consensus != 3 {
		t.Errorf("consensus = %d, want 3 (partial — same family)", updated.Consensus)
	}
}

func TestKEMWrapper(t *testing.T) {
	w := NewKEMWrapper()

	// No function set — should error
	_, _, err := w.Encapsulate(0x04, 0x01, []byte("pubkey"))
	if err == nil {
		t.Fatal("should error without encap function")
	}

	// Set mock function
	w.SetEncapFunc(func(algoID, paramSet uint8, pubKey []byte) ([]byte, []byte, error) {
		return []byte("ciphertext"), []byte("shared-secret"), nil
	})

	ct, ss, err := w.Encapsulate(0x04, 0x01, []byte("pubkey"))
	if err != nil {
		t.Fatalf("Encapsulate failed: %v", err)
	}
	if string(ct) != "ciphertext" {
		t.Errorf("ciphertext = %q", ct)
	}
	if string(ss) != "shared-secret" {
		t.Errorf("shared secret = %q", ss)
	}
	if w.OperationCount() != 1 {
		t.Errorf("operation count = %d, want 1", w.OperationCount())
	}
}

func TestKEMWrapperInvalidParam(t *testing.T) {
	w := NewKEMWrapper()
	w.SetEncapFunc(func(algoID, paramSet uint8, pubKey []byte) ([]byte, []byte, error) {
		return nil, nil, nil
	})

	_, _, err := w.Encapsulate(0xFF, 0xFF, []byte("key"))
	if err == nil {
		t.Fatal("should fail on invalid KEM param")
	}
}
