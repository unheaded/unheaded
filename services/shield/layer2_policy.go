// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package shield

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// AppPQCPolicy defines per-application PQC verification requirements.
type AppPQCPolicy struct {
	ServiceID    uint16   `yaml:"service_id" json:"service_id"`
	ServiceName  string   `yaml:"service_name" json:"service_name"`
	Mode         string   `yaml:"mode" json:"mode"`                 // "pessimistic" or "optimistic"
	RequiredTier uint8    `yaml:"required_tier" json:"required_tier"`
	AllowedAlgos []uint8  `yaml:"allowed_algos" json:"allowed_algos"`
	MaxSigAgeSec uint32   `yaml:"max_sig_age_sec" json:"max_sig_age_sec"`
	GraceSec     uint32   `yaml:"grace_sec" json:"grace_sec"`
	Exempt       bool     `yaml:"exempt" json:"exempt"` // Skip PQC for this service
}

// Layer2PolicyConfig holds the YAML-driven per-app policy configuration.
type Layer2PolicyConfig struct {
	Version  string          `yaml:"version"`
	Policies []AppPQCPolicy  `yaml:"policies"`
}

// Layer2PolicyEngine manages per-application PQC policies.
type Layer2PolicyEngine struct {
	mu       sync.RWMutex
	policies map[uint16]*AppPQCPolicy // serviceID → policy
}

// NewLayer2PolicyEngine creates a new policy engine.
func NewLayer2PolicyEngine() *Layer2PolicyEngine {
	return &Layer2PolicyEngine{
		policies: make(map[uint16]*AppPQCPolicy),
	}
}

// LoadFromYAML parses a YAML policy configuration.
func (e *Layer2PolicyEngine) LoadFromYAML(data []byte) error {
	var config Layer2PolicyConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("shield: policy yaml: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range config.Policies {
		p := &config.Policies[i]
		e.policies[p.ServiceID] = p
	}
	return nil
}

// GetPolicy returns the PQC policy for a service.
func (e *Layer2PolicyEngine) GetPolicy(serviceID uint16) (*AppPQCPolicy, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p, ok := e.policies[serviceID]
	return p, ok
}

// SetPolicy sets a policy for a service.
func (e *Layer2PolicyEngine) SetPolicy(p *AppPQCPolicy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[p.ServiceID] = p
}

// IsExempt checks if a service is exempt from PQC verification.
func (e *Layer2PolicyEngine) IsExempt(serviceID uint16) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p, ok := e.policies[serviceID]
	return ok && p.Exempt
}

// PolicyCount returns the number of loaded policies.
func (e *Layer2PolicyEngine) PolicyCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.policies)
}
