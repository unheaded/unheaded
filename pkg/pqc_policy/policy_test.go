// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc_policy

import (
	"context"
	"testing"
	"time"
)

func TestNISTLevel_MLDSA(t *testing.T) {
	tests := []struct {
		name     string
		algoID   uint8
		paramSet uint8
		want     NISTSecurityLevel
	}{
		{"ML-DSA-44", 0x02, 0x01, NISTLevel2},
		{"ML-DSA-65", 0x02, 0x02, NISTLevel3},
		{"ML-DSA-87", 0x02, 0x03, NISTLevel5},
		{"ML-KEM-512", 0x04, 0x01, NISTLevel1},
		{"ML-KEM-768", 0x04, 0x02, NISTLevel3},
		{"ML-KEM-1024", 0x04, 0x03, NISTLevel5},
		{"SLH-DSA-128", 0x01, 0x01, NISTLevel1},
		{"HQC-128", 0x05, 0x01, NISTLevel1},
		{"unknown", 0xFF, 0x01, NISTLevel0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NISTLevel(tt.algoID, tt.paramSet)
			if got != tt.want {
				t.Errorf("NISTLevel(0x%02x, 0x%02x) = %s, want %s", tt.algoID, tt.paramSet, got, tt.want)
			}
		})
	}
}

func TestMeetsMinLevel(t *testing.T) {
	if !MeetsMinLevel(0x02, 0x03, NISTLevel5) {
		t.Error("ML-DSA-87 should meet NIST Level 5")
	}
	if MeetsMinLevel(0x02, 0x01, NISTLevel5) {
		t.Error("ML-DSA-44 should not meet NIST Level 5")
	}
}

func TestChecker_7PointVerification(t *testing.T) {
	c := NewChecker()
	ctx := context.Background()

	c.SetPolicy(&AppPolicy{
		ServiceID:    100,
		ServiceName:  "test-service",
		AllowedAlgos: []uint8{0x02}, // ML-DSA only
		MinNISTLevel: NISTLevel3,
		MaxKeyAgeSec: 3600,
		RequirePQC:   true,
	})

	// Step 1: PQC not verified but required.
	r := c.Check(ctx, PolicyCheckRequest{
		ServiceID:   100,
		PQCVerified: false,
	})
	if r.Allowed {
		t.Fatal("should fail: PQC required but not verified")
	}
	if r.FailStep != 1 {
		t.Fatalf("expected fail step 1, got %d", r.FailStep)
	}

	// Step 4: Algorithm not allowed.
	r = c.Check(ctx, PolicyCheckRequest{
		ServiceID:   100,
		AlgoID:      0x01, // SLH-DSA not in allowed list
		ParamSet:    0x01,
		PQCVerified: true,
	})
	if r.Allowed {
		t.Fatal("should fail: SLH-DSA not allowed")
	}
	if r.FailStep != 4 {
		t.Fatalf("expected fail step 4, got %d", r.FailStep)
	}

	// Step 5: NIST level too low.
	r = c.Check(ctx, PolicyCheckRequest{
		ServiceID:   100,
		AlgoID:      0x02,
		ParamSet:    0x01, // ML-DSA-44 = Level 2, min is Level 3
		PQCVerified: true,
		KeyCreatedAt: time.Now(),
	})
	if r.Allowed {
		t.Fatal("should fail: NIST level too low")
	}
	if r.FailStep != 5 {
		t.Fatalf("expected fail step 5, got %d", r.FailStep)
	}

	// Step 7: Key too old.
	r = c.Check(ctx, PolicyCheckRequest{
		ServiceID:    100,
		AlgoID:       0x02,
		ParamSet:     0x02, // ML-DSA-65 = Level 3
		PQCVerified:  true,
		KeyCreatedAt: time.Now().Add(-2 * time.Hour), // > 3600s
	})
	if r.Allowed {
		t.Fatal("should fail: key too old")
	}
	if r.FailStep != 7 {
		t.Fatalf("expected fail step 7, got %d", r.FailStep)
	}

	// All pass.
	r = c.Check(ctx, PolicyCheckRequest{
		ServiceID:    100,
		AlgoID:       0x02,
		ParamSet:     0x02,
		PQCVerified:  true,
		KeyCreatedAt: time.Now(),
	})
	if !r.Allowed {
		t.Fatalf("should pass all checks, failed at step %d: %s", r.FailStep, r.FailReason)
	}
	if r.NISTLevel != NISTLevel3 {
		t.Fatalf("expected NIST Level 3, got %s", r.NISTLevel)
	}
}

func TestChecker_DefaultPolicy(t *testing.T) {
	c := NewChecker()
	ctx := context.Background()

	// Unknown service gets default permissive policy.
	r := c.Check(ctx, PolicyCheckRequest{
		ServiceID:    9999,
		AlgoID:       0x02,
		ParamSet:     0x01,
		PQCVerified:  true,
		KeyCreatedAt: time.Now(),
	})
	if !r.Allowed {
		t.Fatalf("default policy should allow, failed at step %d: %s", r.FailStep, r.FailReason)
	}
}

func TestChecker_NoPQCNotRequired(t *testing.T) {
	c := NewChecker()
	ctx := context.Background()

	// Default policy does not require PQC.
	r := c.Check(ctx, PolicyCheckRequest{
		ServiceID:   9999,
		PQCVerified: false,
	})
	if !r.Allowed {
		t.Fatal("should allow non-PQC packets when not required")
	}
}

func TestChecker_KeyPinning(t *testing.T) {
	c := NewChecker()
	ctx := context.Background()

	pinnedFP := [4]byte{0xDE, 0xAD, 0xBE, 0xEF}
	c.SetPolicy(&AppPolicy{
		ServiceID:    200,
		AllowedAlgos: []uint8{0x02},
		MinNISTLevel: NISTLevel1,
		PinnedKeys:   [][4]byte{pinnedFP},
		MaxKeyAgeSec: 86400,
		RequirePQC:   true,
	})

	// Wrong fingerprint.
	r := c.Check(ctx, PolicyCheckRequest{
		ServiceID:      200,
		AlgoID:         0x02,
		ParamSet:       0x01,
		PQCVerified:    true,
		KeyFingerprint: [4]byte{0x00, 0x00, 0x00, 0x00},
		KeyCreatedAt:   time.Now(),
	})
	if r.Allowed {
		t.Fatal("should fail: key fingerprint not pinned")
	}
	if r.FailStep != 6 {
		t.Fatalf("expected fail step 6, got %d", r.FailStep)
	}

	// Correct fingerprint.
	r = c.Check(ctx, PolicyCheckRequest{
		ServiceID:      200,
		AlgoID:         0x02,
		ParamSet:       0x01,
		PQCVerified:    true,
		KeyFingerprint: pinnedFP,
		KeyCreatedAt:   time.Now(),
	})
	if !r.Allowed {
		t.Fatalf("should pass with correct fingerprint, failed: %s", r.FailReason)
	}
}

func TestParsePolicyBytes(t *testing.T) {
	yamlData := []byte(`
version: "1.0"
policies:
  - service_id: 100
    service_name: "test-service"
    allowed_algos: [2, 3]
    min_nist_level: 3
    max_key_age_sec: 3600
    require_pqc: true
  - service_id: 200
    service_name: "another-service"
    allowed_algos: [1, 2]
    min_nist_level: 1
    max_key_age_sec: 86400
    require_pqc: false
`)

	pf, err := ParsePolicyBytes(yamlData)
	if err != nil {
		t.Fatalf("ParsePolicyBytes: %v", err)
	}
	if pf.Version != "1.0" {
		t.Fatalf("version = %q, want 1.0", pf.Version)
	}
	if len(pf.Policies) != 2 {
		t.Fatalf("policy count = %d, want 2", len(pf.Policies))
	}
	if pf.Policies[0].ServiceID != 100 {
		t.Fatalf("first policy service ID = %d, want 100", pf.Policies[0].ServiceID)
	}
	if !pf.Policies[0].RequirePQC {
		t.Fatal("first policy should require PQC")
	}
}

func TestCheckerLoadPolicies(t *testing.T) {
	yamlData := []byte(`
version: "1.0"
policies:
  - service_id: 300
    service_name: "loaded-service"
    allowed_algos: [2]
    min_nist_level: 3
    max_key_age_sec: 1800
    require_pqc: true
`)

	pf, err := ParsePolicyBytes(yamlData)
	if err != nil {
		t.Fatalf("ParsePolicyBytes: %v", err)
	}

	c := NewChecker()
	c.LoadPolicies(pf)

	p, ok := c.GetPolicy(300)
	if !ok {
		t.Fatal("policy for service 300 should exist")
	}
	if p.ServiceName != "loaded-service" {
		t.Fatalf("service name = %q, want loaded-service", p.ServiceName)
	}
}

func TestNISTLevel_String(t *testing.T) {
	if NISTLevel1.String() != "NIST Level 1" {
		t.Fatalf("NISTLevel1.String() = %q", NISTLevel1.String())
	}
	if NISTLevel0.String() != "Unknown" {
		t.Fatalf("NISTLevel0.String() = %q", NISTLevel0.String())
	}
}
