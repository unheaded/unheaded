// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package api

import (
	"errors"
	"testing"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/wotansig"
	"unheaded/services/wotan/internal/signing"
)

// The checked-in configs/wotan.yaml signs audit.*, Akira's two health
// outputs and alerts.critical (S-IRON-GATE Phase 9) with keys: [] (enrolled
// per deployment): Wotan refuses every publish there until a key is listed,
// and HTTP refuses those topics outright.
func TestRepoWotanYAML_Phase9SignedPrefixes(t *testing.T) {
	cfg, err := LoadTopicConfig("../../../../configs/wotan.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, sp := range cfg.Topics.SignedPrefixes {
		got[sp.Prefix] = len(sp.Keys)
	}
	for _, p := range []string{"drift.", "audit.", "alerts.critical.", "system.health.consensus.", "system.health.remediate."} {
		n, ok := got[p]
		if !ok {
			t.Errorf("configs/wotan.yaml signed_prefixes lacks %q", p)
			continue
		}
		if n != 0 {
			t.Errorf("%q has %d keys checked in: keys are enrolled per deployment, never committed", p, n)
		}
	}
	// Every prefix a first-party producer refuses to send unsigned is
	// signed by the checked-in Wotan config too (no producer/config drift).
	for _, p := range wotansig.SignedTopicPrefixes {
		if p == "config." {
			continue // topics.signing_keys
		}
		if _, ok := got[p]; !ok {
			t.Errorf("wotansig.SignedTopicPrefixes has %q, configs/wotan.yaml does not", p)
		}
	}

	pol, err := cfg.SigningPolicy()
	if err != nil {
		t.Fatal(err)
	}
	_, s, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	ts := wotansig.NewTopicSigner(s)
	for _, topic := range []string{"audit.login", "alerts.critical", "system.health.consensus", "system.health.remediate"} {
		if !cfg.RequiresSignature(topic) {
			t.Errorf("HTTP path takes %s unsigned", topic)
		}
		v, ok := pol.VerifierFor(topic)
		if !ok {
			t.Fatalf("no verifier for %s", topic)
		}
		sg, err := ts.Sign(topic, "akira", []byte("p"))
		if err != nil {
			t.Fatal(err)
		}
		if err := v.Verify(topic, "akira", []byte("p"), sg); !errors.Is(err, signing.ErrNoTrustedKeys) {
			t.Errorf("%s with no enrolled key: %v, want ErrNoTrustedKeys", topic, err)
		}
	}
	for _, topic := range []string{"system.health.reports", "alerts.warning", "logs.akira.info"} {
		if cfg.RequiresSignature(topic) || pol.RequiresSignature(topic) {
			t.Errorf("%s signed; it has unsigned producers", topic)
		}
	}
}
