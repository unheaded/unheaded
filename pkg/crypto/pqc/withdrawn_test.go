// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import "testing"

// DP-411: FN-DSA left the registry; its id stays burned.
func TestAlgoWithdrawn03(t *testing.T) {
	if _, ok := Registry(AlgoID(0x03)); ok {
		t.Fatal("Registry(0x03) found an entry: FN-DSA was withdrawn, 0x03 must not resolve")
	}
	if got := AlgoID(0x03).String(); got != "unknown" {
		t.Fatalf("AlgoID(0x03).String() = %q, want unknown", got)
	}
	if info, ok := Registry(AlgoHQC); !ok || info.Available {
		t.Fatalf("HQC must stay reserved: ok=%v available=%v", ok, info.Available)
	}
	if info, ok := Registry(AlgoSLHDSA); !ok || !info.Available {
		t.Fatalf("SLH-DSA must be available: ok=%v available=%v", ok, info.Available)
	}
}
