// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package sophia

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// AppPolicy defines PQC policy for an application/service.
type AppPolicy struct {
	ServiceID    uint16   `yaml:"service_id" json:"service_id"`
	ServiceName  string   `yaml:"service_name" json:"service_name"`
	Mode         string   `yaml:"mode" json:"mode"`
	RequiredTier uint8    `yaml:"required_tier" json:"required_tier"`
	AllowedAlgos []uint8  `yaml:"allowed_algos" json:"allowed_algos"`
	MaxSigAgeSec uint32   `yaml:"max_sig_age_sec" json:"max_sig_age_sec"`
	GraceSec     uint32   `yaml:"grace_sec" json:"grace_sec"`
}

// PolicyConfig holds a YAML policy file.
type PolicyConfig struct {
	Version  string      `yaml:"version"`
	Policies []AppPolicy `yaml:"policies"`
}

// PolicyMap manages per-app PQC policies with an in-memory cache.
type PolicyMap struct {
	mu       sync.RWMutex
	policies map[uint16]*AppPolicy // serviceID → policy
	raw      []byte                // last loaded raw YAML
}

// NewPolicyMap creates a new policy map.
func NewPolicyMap() *PolicyMap {
	return &PolicyMap{
		policies: make(map[uint16]*AppPolicy),
	}
}

// LoadYAML parses and caches a YAML policy configuration.
func (pm *PolicyMap) LoadYAML(data []byte) error {
	var config PolicyConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("sophia: policy_map: %w", err)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.raw = make([]byte, len(data))
	copy(pm.raw, data)

	for i := range config.Policies {
		p := &config.Policies[i]
		pm.policies[p.ServiceID] = p
	}
	return nil
}

// Get returns the policy for a service.
func (pm *PolicyMap) Get(serviceID uint16) (*AppPolicy, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.policies[serviceID]
	return p, ok
}

// Set stores a policy.
func (pm *PolicyMap) Set(p *AppPolicy) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.policies[p.ServiceID] = p
}

// Delete removes a policy.
func (pm *PolicyMap) Delete(serviceID uint16) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	_, ok := pm.policies[serviceID]
	if ok {
		delete(pm.policies, serviceID)
	}
	return ok
}

// List returns all policies.
func (pm *PolicyMap) List() []*AppPolicy {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]*AppPolicy, 0, len(pm.policies))
	for _, p := range pm.policies {
		result = append(result, p)
	}
	return result
}

// Count returns the number of policies.
func (pm *PolicyMap) Count() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.policies)
}
