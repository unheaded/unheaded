// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package api

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// TopicConfig holds topic-level configuration for Wotan.
//
// TODO(BlackMage): Harden auto-approval with API key identity verification.
// Currently auto-approve is based on display_name which is self-reported by
// the subscriber. A malicious service could claim any display_name. Phase 2
// should tie auto-approval to verified API keys or mTLS client certificates.
type TopicConfig struct {
	Topics struct {
		AutoApprove []string `yaml:"auto_approve"`
	} `yaml:"topics"`

	// allowSet is built from AutoApprove for O(1) lookups
	allowSet map[string]struct{}
}

// LoadTopicConfig reads topic configuration from a YAML file.
// Returns a default (deny-all) config if the file does not exist.
func LoadTopicConfig(path string) (*TopicConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file — deny all auto-approvals
			return &TopicConfig{allowSet: make(map[string]struct{})}, nil
		}
		return nil, fmt.Errorf("read topic config: %w", err)
	}

	var cfg TopicConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse topic config: %w", err)
	}

	// Build lookup set
	cfg.allowSet = make(map[string]struct{}, len(cfg.Topics.AutoApprove))
	for _, name := range cfg.Topics.AutoApprove {
		cfg.allowSet[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	return &cfg, nil
}

// IsAutoApproved returns true if the given display name is in the auto-approve
// allowlist. Matching is case-insensitive. The wildcard "*" matches all names.
func (c *TopicConfig) IsAutoApproved(displayName string) bool {
	if c == nil || c.allowSet == nil {
		return false
	}

	// Wildcard: approve everyone
	if _, ok := c.allowSet["*"]; ok {
		return true
	}

	_, ok := c.allowSet[strings.ToLower(strings.TrimSpace(displayName))]
	return ok
}
