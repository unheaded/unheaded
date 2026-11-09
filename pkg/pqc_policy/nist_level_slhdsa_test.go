// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc_policy_test

import (
	"strings"
	"testing"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/pqc_policy"
)

// slhdsaSets is every SLH-DSA parameter set pkg/crypto/pqc can sign with.
// NISTLevel must use the same ids, so a new or renumbered set fails here.
var slhdsaSets = []pqc.ParameterSet{
	pqc.SLHDSA_SHAKE_128s, pqc.SLHDSA_SHAKE_128f,
	pqc.SLHDSA_SHAKE_192s, pqc.SLHDSA_SHAKE_192f,
	pqc.SLHDSA_SHAKE_256s, pqc.SLHDSA_SHAKE_256f,
	pqc.SLHDSA_SHA2_128s, pqc.SLHDSA_SHA2_128f,
	pqc.SLHDSA_SHA2_192s, pqc.SLHDSA_SHA2_192f,
	pqc.SLHDSA_SHA2_256s, pqc.SLHDSA_SHA2_256f,
}

// levelFromName reads FIPS 205's security category from the set's name:
// n=16 (128) is category 1, n=24 (192) is 3, n=32 (256) is 5.
func levelFromName(t *testing.T, name string) pqc_policy.NISTSecurityLevel {
	t.Helper()
	switch {
	case strings.Contains(name, "-128"):
		return pqc_policy.NISTLevel1
	case strings.Contains(name, "-192"):
		return pqc_policy.NISTLevel3
	case strings.Contains(name, "-256"):
		return pqc_policy.NISTLevel5
	}
	t.Fatalf("cannot read a security category from %q", name)
	return pqc_policy.NISTLevel0
}

func TestNISTLevel_SLHDSA_MatchesSigningRegistry(t *testing.T) {
	for _, ps := range slhdsaSets {
		name, _, _, _, ok := pqc.SLHDSAParamInfo(ps)
		if !ok {
			t.Fatalf("pkg/crypto/pqc has no info for SLH-DSA set 0x%02x", uint8(ps))
		}
		want := levelFromName(t, name)
		got := pqc_policy.NISTLevel(uint8(pqc.AlgoSLHDSA), uint8(ps))
		if got != want {
			t.Errorf("NISTLevel(SLH-DSA, 0x%02x %s) = %s, want %s", uint8(ps), name, got, want)
		}
	}
}

// The old 0x01-0x03 ids named no real set; they must stay unknown (level 0)
// so a policy with any minimum refuses them.
func TestNISTLevel_SLHDSA_OldIDsAreUnknown(t *testing.T) {
	for _, id := range []uint8{0x00, 0x01, 0x02, 0x03, 0x06, 0x0D, 0x0F, 0x16, 0xFF} {
		if got := pqc_policy.NISTLevel(uint8(pqc.AlgoSLHDSA), id); got != pqc_policy.NISTLevel0 {
			t.Errorf("NISTLevel(SLH-DSA, 0x%02x) = %s, want Unknown", id, got)
		}
	}
}
