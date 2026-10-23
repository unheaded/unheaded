// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/wotansig"
	"unheaded/services/wotan/internal/signing"
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
		// SignedPrefixes are further signed topic prefixes, each with the
		// keys trusted for it alone (ADR-105). drift. is signed whether or
		// not it is listed; listed with no keys, or not listed, every
		// drift.* publish is refused.
		SignedPrefixes []SignedPrefix `yaml:"signed_prefixes"`
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
	// Refuse a config whose signing policy is broken here, at load, so
	// Wotan never starts with a prefix unsigned or a key silently dropped.
	if _, err := cfg.SigningPolicy(); err != nil {
		return nil, err
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

// SignedPrefix is one topics.signed_prefixes entry.
type SignedPrefix struct {
	Prefix string   `yaml:"prefix"`
	Keys   []string `yaml:"keys"`
}

// UnmarshalYAML refuses unknown fields: a misspelt "keys" would otherwise
// leave the prefix with no trusted key and no error.
func (sp *SignedPrefix) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a signed_prefixes entry is a mapping {prefix, keys}", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		switch k := n.Content[i].Value; k {
		case "prefix", "keys":
		default:
			return fmt.Errorf("line %d: unknown field %q in a signed_prefixes entry", n.Content[i].Line, k)
		}
	}
	type plain SignedPrefix
	return n.Decode((*plain)(sp))
}

// RequiresSignature reports whether topic falls under a built-in or
// configured signed prefix; HTTP refuses every such topic. A nil config
// knows only the built-in prefixes.
func (c *TopicConfig) RequiresSignature(topic string) bool {
	if signing.RequiresSignature(topic) {
		return true
	}
	if c == nil {
		return false
	}
	for _, sp := range c.Topics.SignedPrefixes {
		if sp.Prefix != "" && strings.HasPrefix(topic, sp.Prefix) {
			return true
		}
	}
	return false
}

// SigningPolicy builds the publish-time signing policy: config. trusts
// topics.signing_keys; each signed_prefixes entry trusts its own keys,
// which must be valid ML-DSA-65 public keys.
func (c *TopicConfig) SigningPolicy() (*signing.Policy, error) {
	cfgKeys, err := c.TrustedSigningKeys()
	if err != nil {
		return nil, err
	}
	var entries []signing.PrefixKeys
	if c != nil {
		for i, sp := range c.Topics.SignedPrefixes {
			if err := signing.ValidatePrefix(sp.Prefix); err != nil {
				return nil, fmt.Errorf("topics.signed_prefixes[%d]: %w", i, err)
			}
			keys := make([][]byte, 0, len(sp.Keys))
			seen := make(map[string]bool, len(sp.Keys))
			for j, k := range sp.Keys {
				b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(k))
				if err != nil || !wotansig.ValidPublicKey(b) {
					return nil, fmt.Errorf("topics.signed_prefixes[%d].keys[%d]: not a base64 ML-DSA-65 public key", i, j)
				}
				if seen[string(b)] {
					return nil, fmt.Errorf("topics.signed_prefixes[%d].keys[%d]: listed twice", i, j)
				}
				seen[string(b)] = true
				keys = append(keys, b)
			}
			entries = append(entries, signing.PrefixKeys{Prefix: sp.Prefix, Keys: keys})
		}
	}
	p, err := signing.NewPolicy(cfgKeys, entries...)
	if err != nil {
		return nil, fmt.Errorf("topics.signed_prefixes: %w", err)
	}
	return p, nil
}
