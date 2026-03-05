// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package sophia

import (
	"fmt"
	"sync"
)

// KEMOperation represents an encapsulate or decapsulate operation.
type KEMOperation struct {
	AlgoID       uint8
	ParamSet     uint8
	SharedSecret []byte
	Ciphertext   []byte
}

// KEMWrapper provides Sophia-level KEM operations.
// It wraps the lower-level pkg/crypto/pqc KEM implementations with
// Sophia-specific key storage and Wotan event publishing.
type KEMWrapper struct {
	mu sync.RWMutex

	// Pluggable encapsulate/decapsulate functions
	encapFn  func(algoID, paramSet uint8, pubKey []byte) (ciphertext, sharedSecret []byte, err error)
	decapFn  func(algoID, paramSet uint8, ciphertext, secretKey []byte) (sharedSecret []byte, err error)

	// Operation log
	operations []KEMOperation
	maxOps     int
}

// NewKEMWrapper creates a new KEM wrapper.
func NewKEMWrapper() *KEMWrapper {
	return &KEMWrapper{
		operations: make([]KEMOperation, 0),
		maxOps:     10000,
	}
}

// SetEncapFunc sets the encapsulation function.
func (w *KEMWrapper) SetEncapFunc(fn func(algoID, paramSet uint8, pubKey []byte) (ciphertext, sharedSecret []byte, err error)) {
	w.encapFn = fn
}

// SetDecapFunc sets the decapsulation function.
func (w *KEMWrapper) SetDecapFunc(fn func(algoID, paramSet uint8, ciphertext, secretKey []byte) (sharedSecret []byte, err error)) {
	w.decapFn = fn
}

// Encapsulate performs KEM encapsulation.
func (w *KEMWrapper) Encapsulate(algoID, paramSet uint8, pubKey []byte) (ciphertext, sharedSecret []byte, err error) {
	if err := ValidateKEMParam(algoID, paramSet); err != nil {
		return nil, nil, err
	}

	if w.encapFn == nil {
		return nil, nil, fmt.Errorf("sophia: KEM encapsulate function not set")
	}

	ct, ss, err := w.encapFn(algoID, paramSet, pubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("sophia: KEM encapsulate: %w", err)
	}

	w.mu.Lock()
	if len(w.operations) < w.maxOps {
		w.operations = append(w.operations, KEMOperation{
			AlgoID:       algoID,
			ParamSet:     paramSet,
			SharedSecret: ss,
			Ciphertext:   ct,
		})
	}
	w.mu.Unlock()

	return ct, ss, nil
}

// Decapsulate performs KEM decapsulation.
func (w *KEMWrapper) Decapsulate(algoID, paramSet uint8, ciphertext, secretKey []byte) (sharedSecret []byte, err error) {
	if err := ValidateKEMParam(algoID, paramSet); err != nil {
		return nil, err
	}

	if w.decapFn == nil {
		return nil, fmt.Errorf("sophia: KEM decapsulate function not set")
	}

	ss, err := w.decapFn(algoID, paramSet, ciphertext, secretKey)
	if err != nil {
		return nil, fmt.Errorf("sophia: KEM decapsulate: %w", err)
	}

	return ss, nil
}

// OperationCount returns the number of logged KEM operations.
func (w *KEMWrapper) OperationCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.operations)
}
