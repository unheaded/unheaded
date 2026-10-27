// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"unheaded/pkg/auth"
	"unheaded/pkg/wotansig"
	"unheaded/services/wotan/internal/signing"
)

// TopicConfig holds topic-level configuration for Wotan.
//
// Auto-approval (S-IRON-GATE Phase 7, ADR-103 §6): with AUTH_ENABLED=true
// the caller's verified service-token subject decides, matched against
// auto_approve; the self-reported display_name is a label only. With auth
// off, the display name decides, matched against auto_approve and
// legacy_display_names.
type TopicConfig struct {
	Topics struct {
		// AutoApprove lists the service identities (the sub of their
		// EdDSA service tokens, the names their keys are filed under)
		// whose subscriptions and publishes are approved at once.
		AutoApprove []string `yaml:"auto_approve"`
		// LegacyDisplayNames are self-reported display names approved
		// only while auth is off (pre-Phase-7 clients, and the Appendix A3
		// rollback). A verified identity never matches them.
		LegacyDisplayNames []string `yaml:"legacy_display_names"`
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
	// legacySet is built from LegacyDisplayNames.
	legacySet map[string]struct{}
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

	// Build lookup sets
	cfg.allowSet = make(map[string]struct{}, len(cfg.Topics.AutoApprove))
	for _, name := range cfg.Topics.AutoApprove {
		cfg.allowSet[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	cfg.legacySet = make(map[string]struct{}, len(cfg.Topics.LegacyDisplayNames))
	for _, name := range cfg.Topics.LegacyDisplayNames {
		cfg.legacySet[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	return &cfg, nil
}

// IsAutoApproved reports whether a caller is on the auto-approve
// allowlist. With a verified identity (auth on) only its subject counts,
// and only for an EdDSA service token: a user token or an API key named
// like a service is not a service, and displayName is ignored. With no
// identity (auth off) displayName is matched against auto_approve and
// legacy_display_names, with a one-time warning: a self-reported name is
// not authentication (ADR-103). Matching is case-insensitive; "*" in
// auto_approve matches every name and every service identity.
func (c *TopicConfig) IsAutoApproved(identity *auth.Identity, displayName string) bool {
	if c == nil || c.allowSet == nil {
		return false
	}
	if identity != nil {
		if identity.Extra["auth_method"] != "eddsa" {
			return false
		}
		if _, ok := c.allowSet["*"]; ok {
			return true
		}
		_, ok := c.allowSet[strings.ToLower(strings.TrimSpace(identity.Subject))]
		return ok
	}

	displayNameWarn.Do(func() {
		log.Warn().Msg("wotan auto-approve is matching self-reported display names: AUTH_ENABLED is off, so callers are not authenticated (ADR-103 §6, S-IRON-GATE Phase 7)")
	})
	if _, ok := c.allowSet["*"]; ok {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(displayName))
	if _, ok := c.allowSet[name]; ok {
		return true
	}
	_, ok := c.legacySet[name]
	return ok
}

// displayNameWarn makes the auth-off fallback warn once per process.
var displayNameWarn sync.Once

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
		if wotansig.Covers(sp.Prefix, topic) {
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
