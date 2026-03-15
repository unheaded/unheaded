// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophia

import (
	"testing"
	"time"
)

func TestCreateMapsAndVerifyEmpty(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	if mgr.SigCount() != 0 {
		t.Errorf("expected sig count 0, got %d", mgr.SigCount())
	}
	if mgr.KeyCount() != 0 {
		t.Errorf("expected key count 0, got %d", mgr.KeyCount())
	}

	// Lookup on empty map should return error
	_, err := mgr.LookupSignature(0)
	if err == nil {
		t.Error("expected error looking up signature in empty map")
	}
	_, err = mgr.LookupKey(0)
	if err == nil {
		t.Error("expected error looking up key in empty map")
	}
	_, err = mgr.LookupPolicy(0, 0)
	if err == nil {
		t.Error("expected error looking up policy in empty map")
	}
}

func TestLoadSignatureAndLookupRoundTrip(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	now := time.Now().Unix()
	entry := &PQCSigEntry{
		AlgoID:    0x01,
		ParamSet:  0x02,
		Tier:      0x01,
		Status:    0x01,
		FlowID:    [16]byte{0x01, 0x02, 0x03, 0x04},
		Epoch:     42,
		CreatedAt: now,
		ExpiresAt: now + 3600,
		SigLen:    256,
		Signature: make([]byte, 256),
	}
	// Fill signature with test data
	for i := range entry.Signature {
		entry.Signature[i] = byte(i % 256)
	}

	ref, err := mgr.LoadSignature(entry)
	if err != nil {
		t.Fatalf("LoadSignature() error: %v", err)
	}

	got, err := mgr.LookupSignature(ref)
	if err != nil {
		t.Fatalf("LookupSignature(%d) error: %v", ref, err)
	}

	if got.AlgoID != entry.AlgoID {
		t.Errorf("AlgoID: got %d, want %d", got.AlgoID, entry.AlgoID)
	}
	if got.ParamSet != entry.ParamSet {
		t.Errorf("ParamSet: got %d, want %d", got.ParamSet, entry.ParamSet)
	}
	if got.Tier != entry.Tier {
		t.Errorf("Tier: got %d, want %d", got.Tier, entry.Tier)
	}
	if got.Status != entry.Status {
		t.Errorf("Status: got %d, want %d", got.Status, entry.Status)
	}
	if got.FlowID != entry.FlowID {
		t.Errorf("FlowID: got %v, want %v", got.FlowID, entry.FlowID)
	}
	if got.Epoch != entry.Epoch {
		t.Errorf("Epoch: got %d, want %d", got.Epoch, entry.Epoch)
	}
	if got.CreatedAt != entry.CreatedAt {
		t.Errorf("CreatedAt: got %d, want %d", got.CreatedAt, entry.CreatedAt)
	}
	if got.ExpiresAt != entry.ExpiresAt {
		t.Errorf("ExpiresAt: got %d, want %d", got.ExpiresAt, entry.ExpiresAt)
	}
	if got.SigLen != entry.SigLen {
		t.Errorf("SigLen: got %d, want %d", got.SigLen, entry.SigLen)
	}
	if len(got.Signature) != len(entry.Signature) {
		t.Errorf("Signature length: got %d, want %d", len(got.Signature), len(entry.Signature))
	}
}

func TestLoadKeyAndLookupRoundTrip(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	now := time.Now().Unix()
	entry := &PQCKeyEntry{
		AlgoID:     0x03,
		ParamSet:   0x01,
		TrustTier:  0x02,
		Status:     KeyStatusActive,
		ServiceID:  1001,
		KeyLen:     1952,
		CreatedAt:  now,
		ExpiresAt:  now + 86400,
		RotationID: 7,
		PublicKey:   make([]byte, 1952),
	}

	ref, err := mgr.LoadKey(entry)
	if err != nil {
		t.Fatalf("LoadKey() error: %v", err)
	}

	got, err := mgr.LookupKey(ref)
	if err != nil {
		t.Fatalf("LookupKey(%d) error: %v", ref, err)
	}

	if got.AlgoID != entry.AlgoID {
		t.Errorf("AlgoID: got %d, want %d", got.AlgoID, entry.AlgoID)
	}
	if got.Status != KeyStatusActive {
		t.Errorf("Status: got %d, want %d", got.Status, KeyStatusActive)
	}
	if got.ServiceID != entry.ServiceID {
		t.Errorf("ServiceID: got %d, want %d", got.ServiceID, entry.ServiceID)
	}
	if got.KeyLen != entry.KeyLen {
		t.Errorf("KeyLen: got %d, want %d", got.KeyLen, entry.KeyLen)
	}
	if got.RotationID != entry.RotationID {
		t.Errorf("RotationID: got %d, want %d", got.RotationID, entry.RotationID)
	}
	if len(got.PublicKey) != len(entry.PublicKey) {
		t.Errorf("PublicKey length: got %d, want %d", len(got.PublicKey), len(entry.PublicKey))
	}
}

func TestLoadPolicyAndLookup(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	policy := &PQCPolicy{
		SrcServiceID: 100,
		DstServiceID: 200,
		Mode:         PolicyModePessimistic,
		RequiredTier: 0x02,
		AllowedAlgos: 0x07, // algos 1, 2, 3
		MaxSigAge:    300,
		GracePeriod:  60,
		Flags:        0x00,
	}

	if err := mgr.LoadPolicy(policy); err != nil {
		t.Fatalf("LoadPolicy() error: %v", err)
	}

	got, err := mgr.LookupPolicy(100, 200)
	if err != nil {
		t.Fatalf("LookupPolicy(100, 200) error: %v", err)
	}

	if got.Mode != PolicyModePessimistic {
		t.Errorf("Mode: got %d, want %d", got.Mode, PolicyModePessimistic)
	}
	if got.RequiredTier != 0x02 {
		t.Errorf("RequiredTier: got %d, want %d", got.RequiredTier, 0x02)
	}
	if got.AllowedAlgos != 0x07 {
		t.Errorf("AllowedAlgos: got %d, want %d", got.AllowedAlgos, 0x07)
	}
	if got.MaxSigAge != 300 {
		t.Errorf("MaxSigAge: got %d, want %d", got.MaxSigAge, 300)
	}
	if got.GracePeriod != 60 {
		t.Errorf("GracePeriod: got %d, want %d", got.GracePeriod, 60)
	}

	// Looking up non-existent policy should fail
	_, err = mgr.LookupPolicy(999, 888)
	if err == nil {
		t.Error("expected error looking up non-existent policy")
	}
}

func TestDeleteSignature(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	entry := &PQCSigEntry{
		AlgoID:    0x01,
		Signature: []byte{0xAA, 0xBB, 0xCC},
	}

	ref, err := mgr.LoadSignature(entry)
	if err != nil {
		t.Fatalf("LoadSignature() error: %v", err)
	}

	if mgr.SigCount() != 1 {
		t.Errorf("expected sig count 1, got %d", mgr.SigCount())
	}

	if err := mgr.DeleteSignature(ref); err != nil {
		t.Fatalf("DeleteSignature(%d) error: %v", ref, err)
	}

	if mgr.SigCount() != 0 {
		t.Errorf("expected sig count 0 after delete, got %d", mgr.SigCount())
	}

	// Looking up deleted sig should fail
	_, err = mgr.LookupSignature(ref)
	if err == nil {
		t.Error("expected error looking up deleted signature")
	}

	// Deleting again should fail
	err = mgr.DeleteSignature(ref)
	if err == nil {
		t.Error("expected error deleting non-existent signature")
	}
}

func TestDeleteKey(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	entry := &PQCKeyEntry{
		AlgoID:    0x02,
		Status:    KeyStatusActive,
		PublicKey: []byte{0x01, 0x02, 0x03},
	}

	ref, err := mgr.LoadKey(entry)
	if err != nil {
		t.Fatalf("LoadKey() error: %v", err)
	}

	if mgr.KeyCount() != 1 {
		t.Errorf("expected key count 1, got %d", mgr.KeyCount())
	}

	if err := mgr.DeleteKey(ref); err != nil {
		t.Fatalf("DeleteKey(%d) error: %v", ref, err)
	}

	if mgr.KeyCount() != 0 {
		t.Errorf("expected key count 0 after delete, got %d", mgr.KeyCount())
	}

	// Looking up deleted key should fail
	_, err = mgr.LookupKey(ref)
	if err == nil {
		t.Error("expected error looking up deleted key")
	}

	// Deleting again should fail
	err = mgr.DeleteKey(ref)
	if err == nil {
		t.Error("expected error deleting non-existent key")
	}
}

func TestMaxCapacityEnforcementSignatures(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	// We cannot load 65536 entries in a unit test efficiently,
	// so we test the logic by directly manipulating the internal state.
	// Load a few entries, then fill the map artificially.
	entry := &PQCSigEntry{AlgoID: 0x01, Signature: []byte{0x00}}

	// Load one entry successfully
	_, err := mgr.LoadSignature(entry)
	if err != nil {
		t.Fatalf("LoadSignature() error: %v", err)
	}

	// Artificially fill the map to capacity
	mgr.mu.Lock()
	for i := uint32(1); i < MaxSignatures; i++ {
		mgr.signatures[i+1000] = entry // use offset to avoid collision
	}
	mgr.mu.Unlock()

	// Next load should fail
	_, err = mgr.LoadSignature(entry)
	if err == nil {
		t.Error("expected error when signature map is full")
	}
}

func TestMaxCapacityEnforcementKeys(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	entry := &PQCKeyEntry{AlgoID: 0x01, PublicKey: []byte{0x00}}

	// Load one entry successfully
	_, err := mgr.LoadKey(entry)
	if err != nil {
		t.Fatalf("LoadKey() error: %v", err)
	}

	// Artificially fill the map to capacity
	mgr.mu.Lock()
	for i := uint16(1); i < MaxKeys; i++ {
		mgr.keys[i+5000] = entry
	}
	mgr.mu.Unlock()

	// Next load should fail
	_, err = mgr.LoadKey(entry)
	if err == nil {
		t.Error("expected error when key map is full")
	}
}

func TestPolicyKeyEncoding(t *testing.T) {
	tests := []struct {
		src  uint16
		dst  uint16
		want uint32
	}{
		{0, 0, 0x00000000},
		{1, 0, 0x00010000},
		{0, 1, 0x00000001},
		{1, 1, 0x00010001},
		{0xFFFF, 0xFFFF, 0xFFFFFFFF},
		{100, 200, (100 << 16) | 200},
		{0x1234, 0x5678, 0x12345678},
	}

	for _, tt := range tests {
		got := PolicyKey(tt.src, tt.dst)
		if got != tt.want {
			t.Errorf("PolicyKey(%d, %d) = 0x%08X, want 0x%08X", tt.src, tt.dst, got, tt.want)
		}
	}
}

func TestSovereignSigEntryConsensus(t *testing.T) {
	tests := []struct {
		name    string
		entry   SovereignSigEntry
		want    uint8
	}{
		{
			name: "all three passed",
			entry: SovereignSigEntry{
				Status1: 0x01, AlgoID1: 0x01,
				Status2: 0x01, AlgoID2: 0x02,
				Status3: 0x01, AlgoID3: 0x03,
			},
			want: ConsensusPassed,
		},
		{
			name: "two of three passed (1+2)",
			entry: SovereignSigEntry{
				Status1: 0x01, AlgoID1: 0x01,
				Status2: 0x01, AlgoID2: 0x02,
				Status3: 0x02, AlgoID3: 0x03,
			},
			want: ConsensusPassed,
		},
		{
			name: "two of three passed (1+3)",
			entry: SovereignSigEntry{
				Status1: 0x01, AlgoID1: 0x01,
				Status2: 0x02, AlgoID2: 0x02,
				Status3: 0x01, AlgoID3: 0x03,
			},
			want: ConsensusPassed,
		},
		{
			name: "two of three passed (2+3)",
			entry: SovereignSigEntry{
				Status1: 0x02, AlgoID1: 0x01,
				Status2: 0x01, AlgoID2: 0x02,
				Status3: 0x01, AlgoID3: 0x03,
			},
			want: ConsensusPassed,
		},
		{
			name: "one of three passed",
			entry: SovereignSigEntry{
				Status1: 0x01, AlgoID1: 0x01,
				Status2: 0x02, AlgoID2: 0x02,
				Status3: 0x02, AlgoID3: 0x03,
			},
			want: ConsensusPartial,
		},
		{
			name: "none passed but all evaluated",
			entry: SovereignSigEntry{
				Status1: 0x02, AlgoID1: 0x01,
				Status2: 0x02, AlgoID2: 0x02,
				Status3: 0x02, AlgoID3: 0x03,
			},
			want: ConsensusFailed,
		},
		{
			name: "all unknown (zero)",
			entry: SovereignSigEntry{
				Status1: 0x00, AlgoID1: 0x01,
				Status2: 0x00, AlgoID2: 0x02,
				Status3: 0x00, AlgoID3: 0x03,
			},
			want: ConsensusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateConsensus(&tt.entry)
			if got != tt.want {
				t.Errorf("EvaluateConsensus() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStatsCounters(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	// Initial counts are zero
	if mgr.SigCount() != 0 {
		t.Errorf("initial SigCount: got %d, want 0", mgr.SigCount())
	}
	if mgr.KeyCount() != 0 {
		t.Errorf("initial KeyCount: got %d, want 0", mgr.KeyCount())
	}

	// Load signatures
	for i := 0; i < 5; i++ {
		_, err := mgr.LoadSignature(&PQCSigEntry{AlgoID: uint8(i + 1), Signature: []byte{byte(i)}})
		if err != nil {
			t.Fatalf("LoadSignature() error: %v", err)
		}
	}
	if mgr.SigCount() != 5 {
		t.Errorf("after 5 loads, SigCount: got %d, want 5", mgr.SigCount())
	}

	// Load keys
	for i := 0; i < 3; i++ {
		_, err := mgr.LoadKey(&PQCKeyEntry{AlgoID: uint8(i + 1), PublicKey: []byte{byte(i)}})
		if err != nil {
			t.Fatalf("LoadKey() error: %v", err)
		}
	}
	if mgr.KeyCount() != 3 {
		t.Errorf("after 3 loads, KeyCount: got %d, want 3", mgr.KeyCount())
	}

	// Delete one signature
	if err := mgr.DeleteSignature(0); err != nil {
		t.Fatalf("DeleteSignature(0) error: %v", err)
	}
	if mgr.SigCount() != 4 {
		t.Errorf("after delete, SigCount: got %d, want 4", mgr.SigCount())
	}

	// Delete one key
	if err := mgr.DeleteKey(0); err != nil {
		t.Fatalf("DeleteKey(0) error: %v", err)
	}
	if mgr.KeyCount() != 2 {
		t.Errorf("after delete, KeyCount: got %d, want 2", mgr.KeyCount())
	}
}

func TestNilEntryErrors(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	if _, err := mgr.LoadSignature(nil); err == nil {
		t.Error("expected error for nil signature entry")
	}

	if _, err := mgr.LoadKey(nil); err == nil {
		t.Error("expected error for nil key entry")
	}

	if err := mgr.LoadPolicy(nil); err == nil {
		t.Error("expected error for nil policy")
	}

	if err := mgr.LoadSovereignSig(0, nil); err == nil {
		t.Error("expected error for nil sovereign sig entry")
	}

	if err := mgr.LoadKEMKey(0, nil); err == nil {
		t.Error("expected error for nil KEM key entry")
	}
}

func TestSovereignSigLoadAndLookup(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	entry := &SovereignSigEntry{
		SigRef1: 100, AlgoID1: 0x01, Status1: 0x01,
		SigRef2: 200, AlgoID2: 0x02, Status2: 0x01,
		SigRef3: 300, AlgoID3: 0x03, Status3: 0x00,
		Consensus: ConsensusUnknown,
	}

	if err := mgr.LoadSovereignSig(42, entry); err != nil {
		t.Fatalf("LoadSovereignSig() error: %v", err)
	}

	got, err := mgr.LookupSovereignSig(42)
	if err != nil {
		t.Fatalf("LookupSovereignSig(42) error: %v", err)
	}

	if got.SigRef1 != 100 || got.SigRef2 != 200 || got.SigRef3 != 300 {
		t.Errorf("SigRefs: got %d/%d/%d, want 100/200/300", got.SigRef1, got.SigRef2, got.SigRef3)
	}

	// Lookup non-existent
	_, err = mgr.LookupSovereignSig(999)
	if err == nil {
		t.Error("expected error looking up non-existent sovereign sig")
	}
}

func TestKEMKeyLoadAndLookup(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	now := time.Now().Unix()
	entry := &KEMKeyEntry{
		AlgoID:       0x01,
		ParamSet:     0x03,
		Status:       KeyStatusActive,
		PeerService:  500,
		TunnelID:     7,
		SharedSecret: [32]byte{0xDE, 0xAD, 0xBE, 0xEF},
		CreatedAt:    now,
		ExpiresAt:    now + 3600,
	}

	if err := mgr.LoadKEMKey(10, entry); err != nil {
		t.Fatalf("LoadKEMKey() error: %v", err)
	}

	got, err := mgr.LookupKEMKey(10)
	if err != nil {
		t.Fatalf("LookupKEMKey(10) error: %v", err)
	}

	if got.PeerService != 500 {
		t.Errorf("PeerService: got %d, want 500", got.PeerService)
	}
	if got.TunnelID != 7 {
		t.Errorf("TunnelID: got %d, want 7", got.TunnelID)
	}
	if got.SharedSecret[0] != 0xDE || got.SharedSecret[3] != 0xEF {
		t.Error("SharedSecret mismatch")
	}

	// Lookup non-existent
	_, err = mgr.LookupKEMKey(999)
	if err == nil {
		t.Error("expected error looking up non-existent KEM key")
	}
}

func TestPolicyUpdate(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	// Load initial policy
	policy := &PQCPolicy{
		SrcServiceID: 10,
		DstServiceID: 20,
		Mode:         PolicyModePessimistic,
		MaxSigAge:    300,
	}
	if err := mgr.LoadPolicy(policy); err != nil {
		t.Fatalf("LoadPolicy() error: %v", err)
	}

	// Update same policy
	updated := &PQCPolicy{
		SrcServiceID: 10,
		DstServiceID: 20,
		Mode:         PolicyModeOptimistic,
		MaxSigAge:    600,
	}
	if err := mgr.LoadPolicy(updated); err != nil {
		t.Fatalf("LoadPolicy() update error: %v", err)
	}

	got, err := mgr.LookupPolicy(10, 20)
	if err != nil {
		t.Fatalf("LookupPolicy() error: %v", err)
	}
	if got.Mode != PolicyModeOptimistic {
		t.Errorf("Mode: got %d, want %d", got.Mode, PolicyModeOptimistic)
	}
	if got.MaxSigAge != 600 {
		t.Errorf("MaxSigAge: got %d, want 600", got.MaxSigAge)
	}
}

func TestPinMaps(t *testing.T) {
	t.Run("with valid path", func(t *testing.T) {
		mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
		if err := mgr.PinMaps(); err != nil {
			t.Errorf("PinMaps() error: %v", err)
		}
	})

	t.Run("with empty path", func(t *testing.T) {
		mgr := NewPQCMapManager("")
		if err := mgr.PinMaps(); err == nil {
			t.Error("expected error for empty pin path")
		}
	})
}

func TestKeyStatusConstants(t *testing.T) {
	if KeyStatusActive != 0x01 {
		t.Errorf("KeyStatusActive: got %d, want 0x01", KeyStatusActive)
	}
	if KeyStatusRotating != 0x02 {
		t.Errorf("KeyStatusRotating: got %d, want 0x02", KeyStatusRotating)
	}
	if KeyStatusRevoked != 0x03 {
		t.Errorf("KeyStatusRevoked: got %d, want 0x03", KeyStatusRevoked)
	}
	if KeyStatusExpired != 0x04 {
		t.Errorf("KeyStatusExpired: got %d, want 0x04", KeyStatusExpired)
	}
}

func TestConsensusConstants(t *testing.T) {
	if ConsensusUnknown != 0x00 {
		t.Errorf("ConsensusUnknown: got %d, want 0x00", ConsensusUnknown)
	}
	if ConsensusPassed != 0x01 {
		t.Errorf("ConsensusPassed: got %d, want 0x01", ConsensusPassed)
	}
	if ConsensusFailed != 0x02 {
		t.Errorf("ConsensusFailed: got %d, want 0x02", ConsensusFailed)
	}
	if ConsensusPartial != 0x03 {
		t.Errorf("ConsensusPartial: got %d, want 0x03", ConsensusPartial)
	}
}

func TestMapNameConstants(t *testing.T) {
	if PQCSigMapName != "pqc_sig_map" {
		t.Errorf("PQCSigMapName: got %q, want %q", PQCSigMapName, "pqc_sig_map")
	}
	if PQCKeyMapName != "pqc_key_map" {
		t.Errorf("PQCKeyMapName: got %q, want %q", PQCKeyMapName, "pqc_key_map")
	}
	if PQCPolicyMapName != "pqc_policy_map" {
		t.Errorf("PQCPolicyMapName: got %q, want %q", PQCPolicyMapName, "pqc_policy_map")
	}
	if SovereignSigMapName != "sovereign_sig_map" {
		t.Errorf("SovereignSigMapName: got %q, want %q", SovereignSigMapName, "sovereign_sig_map")
	}
	if KEMKeyMapName != "kem_key_map" {
		t.Errorf("KEMKeyMapName: got %q, want %q", KEMKeyMapName, "kem_key_map")
	}
}

func TestMapSizeLimits(t *testing.T) {
	if MaxSignatures != 65536 {
		t.Errorf("MaxSignatures: got %d, want 65536", MaxSignatures)
	}
	if MaxKeys != 4096 {
		t.Errorf("MaxKeys: got %d, want 4096", MaxKeys)
	}
	if MaxPolicies != 1024 {
		t.Errorf("MaxPolicies: got %d, want 1024", MaxPolicies)
	}
	if MaxSovereignSigs != 8192 {
		t.Errorf("MaxSovereignSigs: got %d, want 8192", MaxSovereignSigs)
	}
	if MaxKEMKeys != 2048 {
		t.Errorf("MaxKEMKeys: got %d, want 2048", MaxKEMKeys)
	}
}

func TestMultipleSignatures(t *testing.T) {
	mgr := NewPQCMapManager("/sys/fs/bpf/sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps() error: %v", err)
	}

	refs := make([]uint32, 10)
	for i := 0; i < 10; i++ {
		entry := &PQCSigEntry{
			AlgoID:    uint8(i%5 + 1),
			ParamSet:  uint8(i % 3),
			SigLen:    uint32(100 + i),
			Signature: make([]byte, 100+i),
		}
		ref, err := mgr.LoadSignature(entry)
		if err != nil {
			t.Fatalf("LoadSignature(%d) error: %v", i, err)
		}
		refs[i] = ref
	}

	if mgr.SigCount() != 10 {
		t.Errorf("SigCount: got %d, want 10", mgr.SigCount())
	}

	// Verify each entry is individually accessible
	for i, ref := range refs {
		got, err := mgr.LookupSignature(ref)
		if err != nil {
			t.Fatalf("LookupSignature(%d) error: %v", ref, err)
		}
		wantAlgo := uint8(i%5 + 1)
		if got.AlgoID != wantAlgo {
			t.Errorf("sig %d AlgoID: got %d, want %d", ref, got.AlgoID, wantAlgo)
		}
	}
}
