// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc_policy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PolicyCheckRequest contains the data needed for a policy check.
type PolicyCheckRequest struct {
	ServiceID      uint16
	AlgoID         uint8
	ParamSet       uint8
	PQCVerified    bool
	KeyFingerprint [4]byte
	KeyCreatedAt   time.Time
}

// PolicyCheckResult holds the result of a policy check.
type PolicyCheckResult struct {
	Allowed    bool
	FailStep   int
	FailReason string
	NISTLevel  NISTSecurityLevel
}

// PolicyChecker is the interface for checking PQC application policies.
type PolicyChecker interface {
	Check(ctx context.Context, req PolicyCheckRequest) PolicyCheckResult
}

// AppPolicy defines a per-application PQC policy.
type AppPolicy struct {
	ServiceID       uint16            `yaml:"service_id" json:"service_id"`
	ServiceName     string            `yaml:"service_name" json:"service_name"`
	AllowedAlgos    []uint8           `yaml:"allowed_algos" json:"allowed_algos"`
	MinNISTLevel    NISTSecurityLevel `yaml:"min_nist_level" json:"min_nist_level"`
	PinnedKeys      [][4]byte         `yaml:"-" json:"-"`
	PinnedKeysHex   []string          `yaml:"pinned_keys" json:"pinned_keys,omitempty"`
	MaxKeyAgeSec    uint32            `yaml:"max_key_age_sec" json:"max_key_age_sec"`
	RequirePQC      bool              `yaml:"require_pqc" json:"require_pqc"`
}

// DefaultAppPolicy returns a permissive default policy.
func DefaultAppPolicy() AppPolicy {
	return AppPolicy{
		AllowedAlgos: []uint8{algoSLHDSA, algoMLDSA, algoFNDSA},
		MinNISTLevel: NISTLevel1,
		MaxKeyAgeSec: 86400, // 24 hours
		RequirePQC:   false,
	}
}

// Checker implements PolicyChecker with per-application policy lookup.
type Checker struct {
	mu       sync.RWMutex
	policies map[uint16]*AppPolicy
}

// NewChecker creates a new policy checker.
func NewChecker() *Checker {
	return &Checker{
		policies: make(map[uint16]*AppPolicy),
	}
}

// LoadPolicies loads policies from a parsed policy file.
func (c *Checker) LoadPolicies(pf *PolicyFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range pf.Policies {
		p := &pf.Policies[i]
		c.policies[p.ServiceID] = p
	}
}

// SetPolicy sets a single application policy.
func (c *Checker) SetPolicy(p *AppPolicy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.policies[p.ServiceID] = p
}

// GetPolicy retrieves a policy by service ID.
func (c *Checker) GetPolicy(serviceID uint16) (*AppPolicy, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.policies[serviceID]
	return p, ok
}

// Check performs 7-point Layer 2 policy verification.
//
// The 7 checks are:
//  1. PQC verified flag — is the packet PQC-authenticated?
//  2. Algorithm ID — is the algorithm known?
//  3. App policy lookup — does a policy exist for this service?
//  4. Allowed algorithms — is this algorithm permitted by policy?
//  5. NIST security level — does the algo/params meet minimum level?
//  6. Key fingerprint pinning — if pinned, does the key match?
//  7. Key age — is the signing key within the allowed age?
func (c *Checker) Check(_ context.Context, req PolicyCheckRequest) PolicyCheckResult {
	result := PolicyCheckResult{}

	// Step 1: PQC verified flag.
	pol := c.lookupPolicy(req.ServiceID)
	if pol.RequirePQC && !req.PQCVerified {
		result.FailStep = 1
		result.FailReason = "pqc_policy: packet not PQC-verified"
		return result
	}
	if !req.PQCVerified {
		// No PQC, but not required — allow.
		result.Allowed = true
		return result
	}

	// Step 2: Algorithm ID check.
	if req.AlgoID == 0 {
		result.FailStep = 2
		result.FailReason = "pqc_policy: unknown algorithm ID 0x00"
		return result
	}

	// Step 3: App policy lookup (already done above, pol is always valid).

	// Step 4: Allowed algorithms.
	if len(pol.AllowedAlgos) > 0 {
		allowed := false
		for _, a := range pol.AllowedAlgos {
			if a == req.AlgoID {
				allowed = true
				break
			}
		}
		if !allowed {
			result.FailStep = 4
			result.FailReason = fmt.Sprintf("pqc_policy: algorithm 0x%02x not allowed for service %d", req.AlgoID, req.ServiceID)
			return result
		}
	}

	// Step 5: NIST security level.
	level := NISTLevel(req.AlgoID, req.ParamSet)
	result.NISTLevel = level
	if level < pol.MinNISTLevel {
		result.FailStep = 5
		result.FailReason = fmt.Sprintf("pqc_policy: NIST level %s below minimum %s", level, pol.MinNISTLevel)
		return result
	}

	// Step 6: Key fingerprint pinning.
	if len(pol.PinnedKeys) > 0 {
		pinned := false
		for _, pk := range pol.PinnedKeys {
			if pk == req.KeyFingerprint {
				pinned = true
				break
			}
		}
		if !pinned {
			result.FailStep = 6
			result.FailReason = fmt.Sprintf("pqc_policy: key fingerprint %x not pinned for service %d", req.KeyFingerprint, req.ServiceID)
			return result
		}
	}

	// Step 7: Key age.
	if pol.MaxKeyAgeSec > 0 && !req.KeyCreatedAt.IsZero() {
		maxAge := time.Duration(pol.MaxKeyAgeSec) * time.Second
		if time.Since(req.KeyCreatedAt) > maxAge {
			result.FailStep = 7
			result.FailReason = fmt.Sprintf("pqc_policy: key age exceeds maximum %s", maxAge)
			return result
		}
	}

	result.Allowed = true
	return result
}

// lookupPolicy returns the policy for a service, or the default policy.
func (c *Checker) lookupPolicy(serviceID uint16) *AppPolicy {
	c.mu.RLock()
	p, ok := c.policies[serviceID]
	c.mu.RUnlock()
	if ok {
		return p
	}
	def := DefaultAppPolicy()
	return &def
}
