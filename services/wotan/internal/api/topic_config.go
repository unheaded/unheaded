// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// TopicConfig holds topic-level configuration for Wotan.
//
// Security note: Auto-approval uses display_name which is self-reported.
// When mTLS is enabled (pkg/auth), approval should be tied to verified
// client certificates instead. See: pkg/auth/mtls.go for cert validation.
type TopicConfig struct {
	Topics struct {
		AutoApprove []string `yaml:"auto_approve"`
		// SigningKeys are the base64 ML-DSA-65 public keys trusted to sign
		// config.* messages (ADR-043 #2). None: config.* is closed.
		SigningKeys []string `yaml:"signing_keys"`
	} `yaml:"topics"`

	// allowSet is built from AutoApprove for O(1) lookups
	allowSet map[string]struct{}
}

// LoadTopicConfig reads topic configuration from a YAML file.
// Returns a default (deny-all) config if the file does not exist.
func LoadTopicConfig(path string) (*TopicConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied configuration file path
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

// TrustedSigningKeys decodes topics.signing_keys. A key that is not valid
// base64 is an error rather than skipped: a typo must not silently leave a
// signer untrusted while the operator believes it is.
func (c *TopicConfig) TrustedSigningKeys() ([][]byte, error) {
	if c == nil {
		return nil, nil
	}
	keys := make([][]byte, 0, len(c.Topics.SigningKeys))
	for i, k := range c.Topics.SigningKeys {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(k))
		if err != nil || len(b) == 0 {
			return nil, fmt.Errorf("topics.signing_keys[%d]: not a base64 public key", i)
		}
		keys = append(keys, b)
	}
	return keys, nil
}
