// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package store provides Sophia-backed storage for PQC keys and signatures.
package store

import (
	"fmt"
	"sync"
)

// SophiaStore manages PQC key and signature storage backed by Sophia BPF maps.
type SophiaStore struct {
	mu         sync.RWMutex
	signatures map[uint32][]byte // sigRef → raw signature
	keys       map[uint16][]byte // keyRef → raw public key
}

// NewSophiaStore creates a new Sophia-backed store.
func NewSophiaStore() *SophiaStore {
	return &SophiaStore{
		signatures: make(map[uint32][]byte),
		keys:       make(map[uint16][]byte),
	}
}

// StoreSignature stores a signature and returns its SigRef.
func (s *SophiaStore) StoreSignature(sigRef uint32, sig []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.signatures[sigRef]; exists {
		return fmt.Errorf("sophia_store: sigRef %d already exists", sigRef)
	}

	// Copy to prevent mutation
	stored := make([]byte, len(sig))
	copy(stored, sig)
	s.signatures[sigRef] = stored
	return nil
}

// LookupSignature retrieves a signature by SigRef.
func (s *SophiaStore) LookupSignature(sigRef uint32) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sig, ok := s.signatures[sigRef]
	if !ok {
		return nil, fmt.Errorf("sophia_store: sigRef %d not found", sigRef)
	}
	return sig, nil
}

// DeleteSignature removes a signature.
func (s *SophiaStore) DeleteSignature(sigRef uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.signatures[sigRef]; !ok {
		return fmt.Errorf("sophia_store: sigRef %d not found", sigRef)
	}
	delete(s.signatures, sigRef)
	return nil
}

// StoreKey stores a public key.
func (s *SophiaStore) StoreKey(keyRef uint16, pubKey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.keys[keyRef]; exists {
		return fmt.Errorf("sophia_store: keyRef %d already exists", keyRef)
	}

	stored := make([]byte, len(pubKey))
	copy(stored, pubKey)
	s.keys[keyRef] = stored
	return nil
}

// LookupKey retrieves a public key by KeyRef.
func (s *SophiaStore) LookupKey(keyRef uint16) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, ok := s.keys[keyRef]
	if !ok {
		return nil, fmt.Errorf("sophia_store: keyRef %d not found", keyRef)
	}
	return key, nil
}

// DeleteKey removes a key.
func (s *SophiaStore) DeleteKey(keyRef uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.keys[keyRef]; !ok {
		return fmt.Errorf("sophia_store: keyRef %d not found", keyRef)
	}
	delete(s.keys, keyRef)
	return nil
}

// SigCount returns the number of stored signatures.
func (s *SophiaStore) SigCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.signatures)
}

// KeyCount returns the number of stored keys.
func (s *SophiaStore) KeyCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.keys)
}
