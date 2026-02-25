// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package registry provides a generic, thread-safe registry for extensible
// protocol code points.
//
// This implements the "Specification Required" registration pattern from
// RFC 8126 §4.6 and the IANA conformance group model from RFC 9669 §7.
//
// All Unheaded protocol packages with extensible code points (errors, flowtype,
// settings, tlv) SHOULD use this shared registry rather than implementing their
// own. This ensures consistent behavior for:
//   - Thread-safe concurrent access (sync.RWMutex)
//   - Range validation (reserved ranges, allocation policies)
//   - Duplicate detection (no silent overwrites)
//   - Iteration (for BPF map population)
//
// The registry is parameterized by key type K and value type V, allowing
// type-safe registries for different code point types.
package registry

import (
	"errors"
	"fmt"
	"sync"
)

// ErrAlreadyRegistered is returned when attempting to register a key that
// already exists in the registry.
var ErrAlreadyRegistered = errors.New("registry: key already registered")

// ErrKeyInReservedRange is returned when attempting to register a key in
// a reserved range. Reserved ranges are defined per IANA registry policy.
var ErrKeyInReservedRange = errors.New("registry: key falls in reserved range")

// ErrRegistryFrozen is returned when attempting to modify a frozen registry.
// Registries are frozen after initial population to prevent runtime mutation
// of core protocol constants.
var ErrRegistryFrozen = errors.New("registry: registry is frozen")

// RangePolicy defines whether a key value is allowed for registration.
// This models IANA allocation policies per RFC 8126.
type RangePolicy[K comparable] func(key K) error

// Registry is a generic, thread-safe registry for protocol code points.
//
// It implements the pattern described in RFC 8126 for IANA registry management:
//   - Initial values populated at package init time
//   - Extension values registered at runtime (if not frozen)
//   - Thread-safe access for concurrent BPF map population
//   - Range validation via configurable policies
//
// Type parameters:
//   - K: The key type (e.g., uint16 for error codes, uint8 for TLV types)
//   - V: The value type (e.g., ErrorInfo, FlowTypeInfo)
type Registry[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]V
	policy  RangePolicy[K]
	frozen  bool
	name    string
}

// New creates a new Registry with the given name and optional range policy.
//
// The name is used in error messages and corresponds to the IANA registry
// name (e.g., "Unheaded Error Codes", "Unheaded TLV Types").
//
// If policy is nil, all keys are accepted.
func New[K comparable, V any](name string, policy RangePolicy[K]) *Registry[K, V] {
	return &Registry[K, V]{
		entries: make(map[K]V),
		policy:  policy,
		name:    name,
	}
}

// Register adds a key-value pair to the registry.
//
// Returns ErrAlreadyRegistered if the key exists.
// Returns ErrKeyInReservedRange if the policy rejects the key.
// Returns ErrRegistryFrozen if the registry has been frozen.
//
// This is safe for concurrent use.
func (r *Registry[K, V]) Register(key K, value V) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.frozen {
		return fmt.Errorf("%w: %s", ErrRegistryFrozen, r.name)
	}

	if r.policy != nil {
		if err := r.policy(key); err != nil {
			return err
		}
	}

	if _, exists := r.entries[key]; exists {
		return fmt.Errorf("%w: %s key %v", ErrAlreadyRegistered, r.name, key)
	}

	r.entries[key] = value
	return nil
}

// MustRegister is like Register but panics on error.
// Use only during package initialization (init functions).
func (r *Registry[K, V]) MustRegister(key K, value V) {
	if err := r.Register(key, value); err != nil {
		panic(fmt.Sprintf("registry.MustRegister: %v", err))
	}
}

// Lookup retrieves the value for a key.
//
// Returns the value and true if found, zero value and false otherwise.
// This is safe for concurrent use.
func (r *Registry[K, V]) Lookup(key K) (V, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.entries[key]
	return v, ok
}

// Range calls fn for each entry in the registry.
//
// If fn returns false, iteration stops. The iteration order is not
// guaranteed. This is safe for concurrent use (holds read lock).
func (r *Registry[K, V]) Range(fn func(K, V) bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for k, v := range r.entries {
		if !fn(k, v) {
			return
		}
	}
}

// Len returns the number of entries in the registry.
func (r *Registry[K, V]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// Freeze prevents further modifications to the registry.
//
// Call this after populating initial/core values during package init.
// Extension registrations after freeze will return ErrRegistryFrozen.
//
// Per RFC 9669 §7.3: "prior group definitions cannot be modified
// post-registration." Freezing enforces this invariant.
func (r *Registry[K, V]) Freeze() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
}

// IsFrozen returns whether the registry has been frozen.
func (r *Registry[K, V]) IsFrozen() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.frozen
}

// Keys returns all registered keys as a slice.
// The order is not guaranteed.
func (r *Registry[K, V]) Keys() []K {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]K, 0, len(r.entries))
	for k := range r.entries {
		keys = append(keys, k)
	}
	return keys
}

// Name returns the registry name.
func (r *Registry[K, V]) Name() string {
	return r.name
}
