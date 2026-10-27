// SPDX-License-Identifier: GPL-3.0-or-later
package signing

import (
	"errors"
	"testing"
)

func TestRequiresSignature_BuiltinPrefixes(t *testing.T) {
	for topic, want := range map[string]bool{
		"config.baseline":        true,
		"drift.detected.node-01": true,
		"drift.":                 true,
		"drift":                  true, // a prefix covers its stem (wotansig.Covers)
		"driftwood.x":            false,
		"kenoma.drift.detected":  false,
		"state.drift.detected":   false,
		"cuirass.drift":          false,
		"logs.wotan.info":        false,
	} {
		if got := RequiresSignature(topic); got != want {
			t.Errorf("RequiresSignature(%q) = %v, want %v", topic, got, want)
		}
	}
}

func TestValidatePrefix(t *testing.T) {
	for _, ok := range []string{"drift.", "drift.detected.", "audit.v1."} {
		if err := ValidatePrefix(ok); err != nil {
			t.Errorf("ValidatePrefix(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "drift", "drift.*", "drift.>", "dri ft.", "drift..", "config.", "config.sub.", "drift.\x00."} {
		if err := ValidatePrefix(bad); err == nil {
			t.Errorf("ValidatePrefix(%q) accepted", bad)
		}
	}
}

func TestNewPolicy_Scoping(t *testing.T) {
	cfgKey := generateTestSigner(t)
	driftKey := generateTestSigner(t)
	p, err := NewPolicy([][]byte{cfgKey.pubKey}, PrefixKeys{Prefix: "drift.", Keys: [][]byte{driftKey.pubKey}})
	if err != nil {
		t.Fatal(err)
	}

	verify := func(topic string, ts *TopicSigner) error {
		v, ok := p.VerifierFor(topic)
		if !ok {
			t.Fatalf("no verifier for %s", topic)
		}
		sg := sign(t, ts, topic, "s", "p")
		return v.Verify(topic, "s", []byte("p"), sg)
	}
	if err := verify("drift.detected.n1", driftKey); err != nil {
		t.Errorf("drift key on drift.*: %v", err)
	}
	if err := verify("config.kingdom", cfgKey); err != nil {
		t.Errorf("config key on config.*: %v", err)
	}
	if err := verify("drift.detected.n1", cfgKey); !errors.Is(err, ErrUntrustedKey) {
		t.Errorf("config key on drift.*: %v, want ErrUntrustedKey", err)
	}
	if err := verify("config.kingdom", driftKey); !errors.Is(err, ErrUntrustedKey) {
		t.Errorf("drift key on config.*: %v, want ErrUntrustedKey", err)
	}
	if _, ok := p.VerifierFor("logs.x"); ok {
		t.Error("logs.x has a verifier")
	}
}

// drift. is required even when the config names no keys for it: every
// drift.* publish is refused (fail closed until keys are enrolled, E5).
func TestNewPolicy_DriftFailsClosedWithoutKeys(t *testing.T) {
	p, err := NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.RequiresSignature("drift.detected.n1") {
		t.Fatal("drift.* not required")
	}
	v, ok := p.VerifierFor("drift.detected.n1")
	if !ok || v == nil {
		t.Fatal("no verifier for drift.*")
	}
	ts := generateTestSigner(t)
	if err := v.Verify("drift.detected.n1", "s", []byte("p"), sign(t, ts, "drift.detected.n1", "s", "p")); !errors.Is(err, ErrNoTrustedKeys) {
		t.Fatalf("err = %v, want ErrNoTrustedKeys", err)
	}
}

func TestNewPolicy_LongestPrefixWins(t *testing.T) {
	general := generateTestSigner(t)
	specific := generateTestSigner(t)
	p, err := NewPolicy(nil,
		PrefixKeys{Prefix: "drift.", Keys: [][]byte{general.pubKey}},
		PrefixKeys{Prefix: "drift.detected.", Keys: [][]byte{specific.pubKey}},
	)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := p.VerifierFor("drift.detected.n1")
	if err := v.Verify("drift.detected.n1", "s", []byte("p"), sign(t, general, "drift.detected.n1", "s", "p")); !errors.Is(err, ErrUntrustedKey) {
		t.Errorf("general key on the specific prefix: %v", err)
	}
	if err := v.Verify("drift.detected.n1", "s", []byte("p"), sign(t, specific, "drift.detected.n1", "s", "p")); err != nil {
		t.Errorf("specific key: %v", err)
	}
	v, _ = p.VerifierFor("drift.other")
	if err := v.Verify("drift.other", "s", []byte("p"), sign(t, general, "drift.other", "s", "p")); err != nil {
		t.Errorf("general key on drift.other: %v", err)
	}
}

func TestNewPolicy_Refusals(t *testing.T) {
	k := generateTestSigner(t).pubKey
	if _, err := NewPolicy(nil, PrefixKeys{Prefix: "drift"}); err == nil {
		t.Error("prefix without a trailing dot accepted")
	}
	if _, err := NewPolicy(nil, PrefixKeys{Prefix: ""}); err == nil {
		t.Error("empty prefix accepted")
	}
	if _, err := NewPolicy(nil, PrefixKeys{Prefix: "drift.", Keys: [][]byte{k}}, PrefixKeys{Prefix: "drift."}); err == nil {
		t.Error("duplicate prefix accepted")
	}
	if _, err := NewPolicy(nil, PrefixKeys{Prefix: "config."}); err == nil {
		t.Error("config. in signed prefixes accepted (its keys are topics.signing_keys)")
	}
}

// A nil policy (an embedding caller that never set one) still requires
// signatures on the built-in prefixes and verifies nothing: fail closed.
func TestPolicy_NilFailsClosed(t *testing.T) {
	var p *Policy
	if !p.RequiresSignature("drift.x") || !p.RequiresSignature("config.x") {
		t.Fatal("nil policy does not require built-in prefixes")
	}
	if v, ok := p.VerifierFor("drift.x"); ok || v != nil {
		t.Fatal("nil policy has a verifier")
	}
}

func TestPolicy_PrefixFor(t *testing.T) {
	p, _ := NewPolicy(nil, PrefixKeys{Prefix: "audit."})
	for topic, want := range map[string]string{"audit.x": "audit.", "drift.y": "drift.", "config.z": "config.", "logs.q": ""} {
		if got := p.PrefixFor(topic); got != want {
			t.Errorf("PrefixFor(%q) = %q, want %q", topic, got, want)
		}
	}
}
