// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"context"
	"fmt"
	"net"
	"time"
)

// Resolver implements three-layer service name resolution:
//  1. Wotan registry (live announcements)
//  2. Port scan (is the expected port open?)
//  3. Convention scan (filesystem config.yaml)
//  4. Static fallback (configs/services.yaml)
type Resolver struct {
	registry *Registry     // Layer 1: Wotan-based (may be nil)
	scanner  *Scanner      // Layer 3: Convention-based (may be nil)
	static   *StaticConfig // Layer 4: Static fallback (may be nil)
}

// NewResolver creates a resolver with the given discovery layers.
// Any layer may be nil; nil layers are skipped during resolution.
func NewResolver(registry *Registry, scanner *Scanner, static *StaticConfig) *Resolver {
	return &Resolver{
		registry: registry,
		scanner:  scanner,
		static:   static,
	}
}

// Resolve looks up a service by name using the resolution cascade.
func (r *Resolver) Resolve(ctx context.Context, name string) (*ServiceEntry, error) {
	// Layer 1: Wotan registry
	if r.registry != nil {
		if entry, err := r.registry.Resolve(name); err == nil {
			return entry, nil
		}
	}

	// Layer 2: Port scan — check if the static/convention-known port is open.
	// We need to know the expected port first, so try static config.
	if r.static != nil {
		if entry, err := r.static.Resolve(name); err == nil {
			if isPortOpen(entry.Address) {
				entry.Health = "healthy"
				return entry, nil
			}
		}
	}

	// Layer 3: Convention scan
	if r.scanner != nil {
		if entry, err := r.scanner.ScanService(name); err == nil {
			return entry, nil
		}
	}

	// Layer 4: Static fallback (return even without port check)
	if r.static != nil {
		if entry, err := r.static.Resolve(name); err == nil {
			return entry, nil
		}
	}

	return nil, fmt.Errorf("%w: %s (all resolution layers exhausted)", ErrServiceNotFound, name)
}

// ResolveAll returns all known services from all layers, deduplicated by name.
// Wotan entries take priority, then convention, then static.
func (r *Resolver) ResolveAll(ctx context.Context) []ServiceEntry {
	seen := make(map[string]struct{})
	var result []ServiceEntry

	// Layer 1: Wotan registry
	if r.registry != nil {
		for _, entry := range r.registry.ResolveAll() {
			if _, ok := seen[entry.Name]; !ok {
				seen[entry.Name] = struct{}{}
				result = append(result, entry)
			}
		}
	}

	// Layer 3: Convention scan
	if r.scanner != nil {
		if entries, err := r.scanner.Scan(); err == nil {
			for _, entry := range entries {
				if _, ok := seen[entry.Name]; !ok {
					seen[entry.Name] = struct{}{}
					result = append(result, entry)
				}
			}
		}
	}

	// Layer 4: Static fallback
	if r.static != nil {
		for _, entry := range r.static.ResolveAll() {
			if _, ok := seen[entry.Name]; !ok {
				seen[entry.Name] = struct{}{}
				result = append(result, entry)
			}
		}
	}

	return result
}

// isPortOpen checks if a TCP connection can be established to the given address.
func isPortOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
