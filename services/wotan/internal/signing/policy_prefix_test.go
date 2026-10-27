// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package signing

import (
	"errors"
	"testing"
)

// S-IRON-GATE Phase 9 (M-053): audit.*, Akira's health outputs and
// alerts.critical join the signed-prefix table, each prefix with its own
// keys (ADR-043 #4 key separation, extended past config./drift.).
var phase9Prefixes = []string{
	"audit.",
	"system.health.consensus.",
	"system.health.remediate.",
	"alerts.critical.",
}

func TestPolicy_Phase9PrefixesSignedAndSeparated(t *testing.T) {
	keys := map[string]*TopicSigner{}
	var entries []PrefixKeys
	for _, p := range phase9Prefixes {
		keys[p] = generateTestSigner(t)
		entries = append(entries, PrefixKeys{Prefix: p, Keys: [][]byte{keys[p].pubKey}})
	}
	pol, err := NewPolicy(nil, entries...)
	if err != nil {
		t.Fatal(err)
	}
	// One real topic per prefix: alerts.critical and Akira's two topics
	// are the prefix's own stem (no trailing segment).
	topics := map[string]string{
		"audit.":                   "audit.login",
		"system.health.consensus.": "system.health.consensus",
		"system.health.remediate.": "system.health.remediate",
		"alerts.critical.":         "alerts.critical",
	}
	for prefix, topic := range topics {
		if got := pol.PrefixFor(topic); got != prefix {
			t.Errorf("PrefixFor(%q) = %q, want %q", topic, got, prefix)
		}
		v, ok := pol.VerifierFor(topic)
		if !ok {
			t.Fatalf("no verifier for %s", topic)
		}
		if err := v.Verify(topic, "akira", []byte("p"), sign(t, keys[prefix], topic, "akira", "p")); err != nil {
			t.Errorf("%s key on %s: %v", prefix, topic, err)
		}
		for other, k := range keys {
			if other == prefix {
				continue
			}
			if err := v.Verify(topic, "akira", []byte("p"), sign(t, k, topic, "akira", "p")); !errors.Is(err, ErrUntrustedKey) {
				t.Errorf("%s key on %s: %v, want ErrUntrustedKey", other, topic, err)
			}
		}
	}
}

// keys: [] in configs/wotan.yaml: the prefix is signed and every publish
// under it is refused until a key is enrolled (E2 pattern).
func TestPolicy_Phase9PrefixesFailClosedWithoutKeys(t *testing.T) {
	var entries []PrefixKeys
	for _, p := range phase9Prefixes {
		entries = append(entries, PrefixKeys{Prefix: p})
	}
	pol, err := NewPolicy(nil, entries...)
	if err != nil {
		t.Fatal(err)
	}
	ts := generateTestSigner(t)
	for _, topic := range []string{"audit.x", "system.health.consensus", "system.health.remediate", "alerts.critical"} {
		if !pol.RequiresSignature(topic) {
			t.Errorf("%s not signed", topic)
			continue
		}
		v, ok := pol.VerifierFor(topic)
		if !ok {
			t.Fatalf("no verifier for %s", topic)
		}
		if err := v.Verify(topic, "s", []byte("p"), sign(t, ts, topic, "s", "p")); !errors.Is(err, ErrNoTrustedKeys) {
			t.Errorf("%s: %v, want ErrNoTrustedKeys", topic, err)
		}
	}
}

// A prefix covers its stem (the prefix without its final dot) and the
// topics under it, nothing else: alerts.critical is the topic every
// service subscribes to, alerts.warning stays unsigned.
func TestPolicy_PrefixCoversItsStem(t *testing.T) {
	pol, err := NewPolicy(nil, PrefixKeys{Prefix: "alerts.critical."}, PrefixKeys{Prefix: "system.health.consensus."})
	if err != nil {
		t.Fatal(err)
	}
	for topic, want := range map[string]bool{
		"alerts.critical":           true,
		"alerts.critical.disk":      true,
		"alerts.criticality":        false,
		"alerts.warning":            false,
		"alerts":                    false,
		"system.health.consensus":   true,
		"system.health.reports":     false,
		"system.health.consensusx":  false,
		"system.health.remediate":   false,
		"logs.alerts.critical.info": false,
	} {
		if got := pol.RequiresSignature(topic); got != want {
			t.Errorf("RequiresSignature(%q) = %v, want %v", topic, got, want)
		}
	}
}

// [DECIDE] Step 270: only alerts.critical. is signed. alerts.* carries
// unsigned service alerts today; a prefix that broad is refused.
func TestValidatePrefix_AlertsScope(t *testing.T) {
	for _, ok := range append([]string{"alerts.critical."}, phase9Prefixes...) {
		if err := ValidatePrefix(ok); err != nil {
			t.Errorf("ValidatePrefix(%q) = %v", ok, err)
		}
	}
	if err := ValidatePrefix("alerts."); err == nil {
		t.Error(`ValidatePrefix("alerts.") accepted: too broad, alerts.* carries unsigned service alerts`)
	}
	if _, err := NewPolicy(nil, PrefixKeys{Prefix: "alerts."}); err == nil {
		t.Error("NewPolicy accepted alerts.")
	}
}
