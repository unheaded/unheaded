// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package signing

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// BuiltinPrefixes are always signed, whatever the config says (ADR-043 #2,
// ADR-105): config.* with topics.signing_keys, drift.* with the keys
// topics.signed_prefixes names for it. A built-in prefix with no keys
// refuses every publish.
var BuiltinPrefixes = []string{"config.", "drift."}

// maxPrefixLen bounds a configured prefix; topic names are short.
const maxPrefixLen = 256

// PrefixKeys trusts Keys to sign topics under Prefix.
type PrefixKeys struct {
	Prefix string
	Keys   [][]byte
}

// Policy maps signed topic prefixes to the keys trusted for each: a key
// trusted for one prefix verifies nothing under another (ADR-043 #4 key
// separation). Lookup is by longest matching prefix. A nil *Policy still
// requires signatures on the built-in prefixes and verifies nothing.
type Policy struct {
	prefixes  []string // longest first
	verifiers map[string]*TopicVerifier
}

// ValidatePrefix accepts a topic prefix for signed_prefixes: non-empty,
// ending in ".", no empty segment, no wildcard, whitespace or control
// characters, and not under config. (whose keys are topics.signing_keys).
func ValidatePrefix(prefix string) error {
	switch {
	case prefix == "" || prefix == ".":
		return errors.New("empty prefix")
	case len(prefix) > maxPrefixLen:
		return fmt.Errorf("prefix longer than %d bytes", maxPrefixLen)
	case !strings.HasSuffix(prefix, "."):
		return fmt.Errorf("prefix %q must end in \".\"", prefix)
	case strings.HasPrefix(prefix, ".") || strings.Contains(prefix, ".."):
		return fmt.Errorf("prefix %q has an empty segment", prefix)
	case strings.ContainsAny(prefix, "*>"):
		return fmt.Errorf("prefix %q contains a wildcard", prefix)
	case strings.IndexFunc(prefix, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0:
		return fmt.Errorf("prefix %q contains whitespace or control characters", prefix)
	case strings.HasPrefix(prefix, "config."):
		return fmt.Errorf("prefix %q: config.* keys are topics.signing_keys", prefix)
	}
	return nil
}

// NewPolicy builds the signing policy: config. trusts configKeys, every
// other built-in prefix trusts the keys an entry names for it (none if no
// entry does), and each further entry adds a signed prefix.
func NewPolicy(configKeys [][]byte, entries ...PrefixKeys) (*Policy, error) {
	keys := map[string][][]byte{"config.": configKeys}
	for _, e := range entries {
		if err := ValidatePrefix(e.Prefix); err != nil {
			return nil, err
		}
		if _, dup := keys[e.Prefix]; dup {
			return nil, fmt.Errorf("prefix %q listed twice", e.Prefix)
		}
		keys[e.Prefix] = e.Keys
	}
	for _, b := range BuiltinPrefixes {
		if _, ok := keys[b]; !ok {
			keys[b] = nil
		}
	}
	p := &Policy{verifiers: make(map[string]*TopicVerifier, len(keys))}
	for prefix, k := range keys {
		v, err := NewTopicVerifier(k...)
		if err != nil {
			return nil, err
		}
		p.verifiers[prefix] = v
		p.prefixes = append(p.prefixes, prefix)
	}
	sort.Slice(p.prefixes, func(i, j int) bool {
		if len(p.prefixes[i]) != len(p.prefixes[j]) {
			return len(p.prefixes[i]) > len(p.prefixes[j])
		}
		return p.prefixes[i] < p.prefixes[j]
	})
	return p, nil
}

// PrefixFor returns the signed prefix a topic falls under (longest match),
// or "" when the topic is not signed.
func (p *Policy) PrefixFor(topic string) string {
	if p == nil {
		for _, b := range BuiltinPrefixes {
			if strings.HasPrefix(topic, b) {
				return b
			}
		}
		return ""
	}
	for _, prefix := range p.prefixes {
		if strings.HasPrefix(topic, prefix) {
			return prefix
		}
	}
	return ""
}

// RequiresSignature reports whether topic takes only signed messages.
func (p *Policy) RequiresSignature(topic string) bool {
	return p.PrefixFor(topic) != ""
}

// VerifierFor returns the verifier for topic's signed prefix. ok is false
// for an unsigned topic, and for every topic on a nil policy.
func (p *Policy) VerifierFor(topic string) (*TopicVerifier, bool) {
	if p == nil {
		return nil, false
	}
	prefix := p.PrefixFor(topic)
	if prefix == "" {
		return nil, false
	}
	v, ok := p.verifiers[prefix]
	return v, ok && v != nil
}
