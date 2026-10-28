// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanerr

import "testing"

// S-TRUE-WIRE Step 386: the header and bracket parsers read peer input
// (X-Wotan-Error, x-wotan-error). Neither may panic, and anything they
// accept must survive its own encoding.

func FuzzDecode(f *testing.F) {
	for _, v := range []uint32{0, 0x44010001, 0xFFFFFFFF, 0x0000FF00} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v uint32) {
		c, err := Decode(v)
		if err != nil {
			return
		}
		if c.Encode() != v {
			t.Fatalf("Decode(0x%08X).Encode() = 0x%08X", v, c.Encode())
		}
		if back, err := ParseHex(c.Hex()); err != nil || back != c {
			t.Fatalf("ParseHex(%q) = %v, %v; want %v", c.Hex(), back, err, c)
		}
	})
}

func FuzzParseHex(f *testing.F) {
	for _, s := range []string{"0x44010001", "0x", "0xZZZZZZZZ", "44010001", "0x4401000100"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c, err := ParseHex(s)
		if err != nil {
			return
		}
		if len(s) != 10 {
			t.Fatalf("ParseHex accepted %q (length %d)", s, len(s))
		}
		if _, err := Decode(c.Encode()); err != nil {
			t.Fatalf("ParseHex(%q) returned %v that Decode refuses: %v", s, c, err)
		}
	})
}

func FuzzParseBracket(f *testing.F) {
	for _, s := range []string{"[ERROR, WOTAN_GRPC, ACCESS_CONTROL, 0x01]", "[WARN, L1, BOUNDS, 0x01]", "[", "[,,,]", "[a, b, c, d, e]"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c, err := ParseBracket(s)
		if err != nil {
			return
		}
		if err := c.Validate(); err != nil {
			t.Fatalf("ParseBracket(%q) returned an invalid code %v: %v", s, c, err)
		}
	})
}
