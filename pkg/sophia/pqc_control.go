// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophia

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// PQCMapManager manages PQC BPF maps in Sophia.
// It maintains userspace mirrors of the BPF maps and handles
// reference allocation, capacity enforcement, and concurrent access.
type PQCMapManager struct {
	mu sync.RWMutex

	// In-memory maps (userspace mirrors of BPF maps)
	signatures    map[uint32]*PQCSigEntry
	keys          map[uint16]*PQCKeyEntry
	policies      map[uint32]*PQCPolicy // key = (srcSvcID << 16) | dstSvcID
	sovereignSigs map[uint32]*SovereignSigEntry
	kemKeys       map[uint16]*KEMKeyEntry

	// Counters (accessed atomically for stats reads without holding mu)
	sigCount int64
	keyCount int64

	// Next reference allocators (monotonically increasing, protected by mu)
	nextSigRef uint32
	nextKeyRef uint16

	// Pin paths for BPF maps
	pinBasePath string
}

// NewPQCMapManager creates a new map manager.
// pinBasePath is the filesystem path where BPF maps are pinned (e.g., "/sys/fs/bpf/sophia").
func NewPQCMapManager(pinBasePath string) *PQCMapManager {
	return &PQCMapManager{
		signatures:    make(map[uint32]*PQCSigEntry),
		keys:          make(map[uint16]*PQCKeyEntry),
		policies:      make(map[uint32]*PQCPolicy),
		sovereignSigs: make(map[uint32]*SovereignSigEntry),
		kemKeys:       make(map[uint16]*KEMKeyEntry),
		pinBasePath:   pinBasePath,
	}
}

// CreatePQCMaps initializes all PQC BPF maps.
// In a real deployment this would create kernel BPF maps via syscall.
// Here it ensures all userspace mirror maps are ready.
func (m *PQCMapManager) CreatePQCMaps() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Reset all maps to empty state
	m.signatures = make(map[uint32]*PQCSigEntry)
	m.keys = make(map[uint16]*PQCKeyEntry)
	m.policies = make(map[uint32]*PQCPolicy)
	m.sovereignSigs = make(map[uint32]*SovereignSigEntry)
	m.kemKeys = make(map[uint16]*KEMKeyEntry)

	// Reset counters and refs
	atomic.StoreInt64(&m.sigCount, 0)
	atomic.StoreInt64(&m.keyCount, 0)
	m.nextSigRef = 0
	m.nextKeyRef = 0

	return nil
}

// --- Signature operations ---

// LoadSignature stores a PQC signature and returns its SigRef.
// Returns an error if the signature map is at capacity.
func (m *PQCMapManager) LoadSignature(entry *PQCSigEntry) (uint32, error) {
	if entry == nil {
		return 0, fmt.Errorf("sophia: %s: nil entry", PQCSigMapName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.signatures) >= MaxSignatures {
		return 0, fmt.Errorf("sophia: %s: map full (%d/%d)", PQCSigMapName, len(m.signatures), MaxSignatures)
	}

	ref := m.nextSigRef
	m.nextSigRef++

	m.signatures[ref] = entry
	atomic.AddInt64(&m.sigCount, 1)

	return ref, nil
}

// LookupSignature retrieves a signature by SigRef.
func (m *PQCMapManager) LookupSignature(sigRef uint32) (*PQCSigEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.signatures[sigRef]
	if !ok {
		return nil, fmt.Errorf("sophia: %s: sigref %d not found", PQCSigMapName, sigRef)
	}

	return entry, nil
}

// DeleteSignature removes a signature by SigRef.
func (m *PQCMapManager) DeleteSignature(sigRef uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.signatures[sigRef]; !ok {
		return fmt.Errorf("sophia: %s: sigref %d not found", PQCSigMapName, sigRef)
	}

	delete(m.signatures, sigRef)
	atomic.AddInt64(&m.sigCount, -1)

	return nil
}

// --- Key operations ---

// LoadKey stores a public key and returns its KeyRef.
// Returns an error if the key map is at capacity.
func (m *PQCMapManager) LoadKey(entry *PQCKeyEntry) (uint16, error) {
	if entry == nil {
		return 0, fmt.Errorf("sophia: %s: nil entry", PQCKeyMapName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.keys) >= MaxKeys {
		return 0, fmt.Errorf("sophia: %s: map full (%d/%d)", PQCKeyMapName, len(m.keys), MaxKeys)
	}

	ref := m.nextKeyRef
	m.nextKeyRef++

	m.keys[ref] = entry
	atomic.AddInt64(&m.keyCount, 1)

	return ref, nil
}

// LookupKey retrieves a key by KeyRef.
func (m *PQCMapManager) LookupKey(keyRef uint16) (*PQCKeyEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.keys[keyRef]
	if !ok {
		return nil, fmt.Errorf("sophia: %s: keyref %d not found", PQCKeyMapName, keyRef)
	}

	return entry, nil
}

// DeleteKey removes a key by KeyRef.
func (m *PQCMapManager) DeleteKey(keyRef uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.keys[keyRef]; !ok {
		return fmt.Errorf("sophia: %s: keyref %d not found", PQCKeyMapName, keyRef)
	}

	delete(m.keys, keyRef)
	atomic.AddInt64(&m.keyCount, -1)

	return nil
}

// --- Policy operations ---

// LoadPolicy stores a verification policy for a service pair.
// The policy is keyed by (SrcServiceID << 16) | DstServiceID.
func (m *PQCMapManager) LoadPolicy(policy *PQCPolicy) error {
	if policy == nil {
		return fmt.Errorf("sophia: %s: nil policy", PQCPolicyMapName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.policies) >= MaxPolicies {
		// Check if this is an update to an existing policy
		key := PolicyKey(policy.SrcServiceID, policy.DstServiceID)
		if _, exists := m.policies[key]; !exists {
			return fmt.Errorf("sophia: %s: map full (%d/%d)", PQCPolicyMapName, len(m.policies), MaxPolicies)
		}
	}

	key := PolicyKey(policy.SrcServiceID, policy.DstServiceID)
	m.policies[key] = policy

	return nil
}

// LookupPolicy retrieves a policy for a service pair.
func (m *PQCMapManager) LookupPolicy(srcSvcID, dstSvcID uint16) (*PQCPolicy, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := PolicyKey(srcSvcID, dstSvcID)
	policy, ok := m.policies[key]
	if !ok {
		return nil, fmt.Errorf("sophia: %s: policy for %d->%d not found", PQCPolicyMapName, srcSvcID, dstSvcID)
	}

	return policy, nil
}

// --- Sovereign signature operations ---

// LoadSovereignSig stores a sovereign multi-sig entry keyed by SigRef.
func (m *PQCMapManager) LoadSovereignSig(sigRef uint32, entry *SovereignSigEntry) error {
	if entry == nil {
		return fmt.Errorf("sophia: %s: nil entry", SovereignSigMapName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.sovereignSigs) >= MaxSovereignSigs {
		if _, exists := m.sovereignSigs[sigRef]; !exists {
			return fmt.Errorf("sophia: %s: map full (%d/%d)", SovereignSigMapName, len(m.sovereignSigs), MaxSovereignSigs)
		}
	}

	m.sovereignSigs[sigRef] = entry

	return nil
}

// LookupSovereignSig retrieves a sovereign sig entry by SigRef.
func (m *PQCMapManager) LookupSovereignSig(sigRef uint32) (*SovereignSigEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.sovereignSigs[sigRef]
	if !ok {
		return nil, fmt.Errorf("sophia: %s: sigref %d not found", SovereignSigMapName, sigRef)
	}

	return entry, nil
}

// --- KEM key operations ---

// LoadKEMKey stores a KEM tunnel key entry keyed by a uint16 reference.
func (m *PQCMapManager) LoadKEMKey(ref uint16, entry *KEMKeyEntry) error {
	if entry == nil {
		return fmt.Errorf("sophia: %s: nil entry", KEMKeyMapName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.kemKeys) >= MaxKEMKeys {
		if _, exists := m.kemKeys[ref]; !exists {
			return fmt.Errorf("sophia: %s: map full (%d/%d)", KEMKeyMapName, len(m.kemKeys), MaxKEMKeys)
		}
	}

	m.kemKeys[ref] = entry

	return nil
}

// LookupKEMKey retrieves a KEM key entry by reference.
func (m *PQCMapManager) LookupKEMKey(ref uint16) (*KEMKeyEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.kemKeys[ref]
	if !ok {
		return nil, fmt.Errorf("sophia: %s: ref %d not found", KEMKeyMapName, ref)
	}

	return entry, nil
}

// --- Stats ---

// SigCount returns the number of loaded signatures.
func (m *PQCMapManager) SigCount() int64 {
	return atomic.LoadInt64(&m.sigCount)
}

// KeyCount returns the number of loaded keys.
func (m *PQCMapManager) KeyCount() int64 {
	return atomic.LoadInt64(&m.keyCount)
}

// PinMaps pins all BPF maps to the filesystem.
// In a real deployment this would call bpf(BPF_OBJ_PIN) for each map.
// Here it validates the pin base path and returns the intended pin paths.
func (m *PQCMapManager) PinMaps() error {
	if m.pinBasePath == "" {
		return fmt.Errorf("sophia: pin_maps: pin base path not set")
	}

	mapNames := []string{
		PQCSigMapName,
		PQCKeyMapName,
		PQCPolicyMapName,
		SovereignSigMapName,
		KEMKeyMapName,
	}

	for _, name := range mapNames {
		pinPath := filepath.Join(m.pinBasePath, name)
		// In production: bpf(BPF_OBJ_PIN, fd, pinPath)
		_ = pinPath
	}

	return nil
}
