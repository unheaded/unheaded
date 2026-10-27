// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotansig

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/gungnir"
)

// Signing-key environment variables (S-IRON-GATE Phase 9, M-053). Each
// names an ML-DSA-65 seed file (32 bytes, gungnir seed format, made by
// unheaded-keygen -algo mldsa65) owned by the producer's account, mode
// 0400: the same owner rules as the service keys. Each key signs one
// family only; its public half is enrolled under that family's prefix in
// Wotan's topics.signed_prefixes, never under another (ADR-043 #4).
const (
	// AuditKeyEnv signs audit.* (no first-party producer yet).
	AuditKeyEnv = "AUDIT_SIGNING_KEY_FILE"
	// HealthKeyEnv signs Akira's system.health.consensus and
	// system.health.remediate.
	HealthKeyEnv = "HEALTH_SIGNING_KEY_FILE"
)

// SignedTopicPrefixes are the prefixes a first-party producer never
// publishes unsigned (RequiresSignature): the built-in config. and drift.
// plus the families configs/wotan.yaml signs. Wotan enforces its own
// config; this list is the producer's floor, so a publisher without a key
// gets an error instead of a message Wotan refuses (or, on a Wotan that
// was never told, takes unsigned).
var SignedTopicPrefixes = []string{
	"config.",
	"drift.",
	"audit.",
	"alerts.critical.",
	"system.health.consensus.",
	"system.health.remediate.",
}

// ErrSignatureRequired refuses an unsigned publish to a signed topic.
var ErrSignatureRequired = errors.New("signing: topic takes only ML-DSA-65 signed messages")

// Covers reports whether a signed prefix covers topic: the topics under
// it, and its stem, the prefix without its final dot ("alerts.critical."
// covers the topic alerts.critical itself). A prefix that does not end in
// "." covers nothing.
func Covers(prefix, topic string) bool {
	if len(prefix) < 2 || !strings.HasSuffix(prefix, ".") {
		return false
	}
	return strings.HasPrefix(topic, prefix) || topic == prefix[:len(prefix)-1]
}

// RequiresSignature reports whether topic is under SignedTopicPrefixes.
func RequiresSignature(topic string) bool {
	for _, p := range SignedTopicPrefixes {
		if Covers(p, topic) {
			return true
		}
	}
	return false
}

// RefuseUnsigned is the producer-side check before an unsigned publish:
// ErrSignatureRequired (naming the topic) for a signed topic, else nil.
func RefuseUnsigned(topic string) error {
	if RequiresSignature(topic) {
		return fmt.Errorf("%w: %s (publish it signed; the producer's signing key is not configured)", ErrSignatureRequired, topic)
	}
	return nil
}

// LoadSignerFile loads an ML-DSA-65 topic signer from a gungnir seed file,
// refusing one that group or others can read (0400 or 0600 only).
func LoadSignerFile(path string) (*TopicSigner, error) {
	if path == "" {
		return nil, errors.New("signing: empty key path")
	}
	priv, err := gungnir.LoadSeedFile(path)
	if err != nil {
		return nil, fmt.Errorf("signing key: %w", err)
	}
	s, err := pqc.NewMLDSASignerFromPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("signing key %s: %w", path, err)
	}
	return NewTopicSigner(s), nil
}

// SignerFromEnv loads the signer the environment variable env names. Unset
// or empty is (nil, nil): the producer has no key and must refuse its
// signed topics (never publish them unsigned). A key file that is named but
// cannot be used is an error, so a misconfigured producer fails loudly.
func SignerFromEnv(env string) (*TopicSigner, error) {
	path := strings.TrimSpace(os.Getenv(env))
	if path == "" {
		return nil, nil
	}
	s, err := LoadSignerFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", env, err)
	}
	return s, nil
}
