// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package bpfschema

import "testing"

// S-TRUE-WIRE Step 386: FlowActionLegacy reads one byte from pre-sprint
// producers. All 256 values: only the old numbering's 0x01-0x05 decode, and
// each decodes to a distinct registered action with a registry name.
func TestFlowActionLegacy_AllBytes(t *testing.T) {
	seen := map[uint8]uint8{}
	for v := 0; v < 256; v++ {
		a, ok := FlowActionLegacy(uint8(v))
		if ok != (v >= 0x01 && v <= 0x05) {
			t.Fatalf("FlowActionLegacy(0x%02x) ok = %v", v, ok)
		}
		if !ok {
			continue
		}
		if prev, dup := seen[a]; dup {
			t.Fatalf("legacy 0x%02x and 0x%02x both decode to 0x%02x", prev, v, a)
		}
		seen[a] = uint8(v)
		if name := FlowActionName(a); len(name) >= 7 && name[:7] == "unknown" {
			t.Fatalf("legacy 0x%02x decodes to unregistered 0x%02x", v, a)
		}
	}
}
