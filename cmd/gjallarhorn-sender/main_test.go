// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net"
	"testing"
)

// ─── parseHexErr ────────────────────────────────────────────────────────────

func TestParseHexErr_AcceptsLowerAndUpperPrefix(t *testing.T) {
	t.Parallel()
	cases := map[string]uint64{
		"0xCAFE":     0xCAFE,
		"0XCAFE":     0xCAFE,
		"CAFE":       0xCAFE,
		"cafe":       0xCAFE,
		"0xCAFEBABE": 0xCAFEBABE,
		"0":          0,
		"FFFFFFFFFFFFFFFF": 0xFFFFFFFFFFFFFFFF, // u64 max
	}
	for input, want := range cases {
		got, err := parseHexErr(input)
		if err != nil {
			t.Errorf("parseHexErr(%q) errored: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("parseHexErr(%q) = 0x%X, want 0x%X", input, got, want)
		}
	}
}

func TestParseHexErr_RejectsInvalidHex(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"",
		"0x",        // prefix only — empty after strip
		"GHIJ",      // non-hex chars
		"0xZZZZ",    // non-hex chars w/ prefix
		"0x10000000000000000", // overflows u64 (>16 hex digits)
		"-1",        // negative not supported
	} {
		_, err := parseHexErr(input)
		if err == nil {
			t.Errorf("parseHexErr(%q) should error, got nil", input)
		}
	}
}

func TestParseHexErr_StripsBothPrefixCases(t *testing.T) {
	t.Parallel()
	// Pin the documented behavior: 0x AND 0X both strip; double-prefix
	// gets only one strip.
	a, _ := parseHexErr("0x123")
	b, _ := parseHexErr("0X123")
	c, _ := parseHexErr("123")
	if a != b || b != c || a != 0x123 {
		t.Errorf("0x/0X/bare prefix mismatch: 0x=%X 0X=%X bare=%X", a, b, c)
	}
}

// ─── Multicast destination constants ────────────────────────────────────────

func TestGjallarhornMcast_IsValidIPv6(t *testing.T) {
	t.Parallel()
	ip := net.ParseIP(gjallarhornMcast)
	if ip == nil {
		t.Fatalf("gjallarhornMcast %q is not a valid IP", gjallarhornMcast)
	}
	if ip.To16() == nil || ip.To4() != nil {
		t.Errorf("gjallarhornMcast %q should be IPv6, not IPv4", gjallarhornMcast)
	}
	if !ip.IsMulticast() {
		t.Errorf("gjallarhornMcast %q is not a multicast address", gjallarhornMcast)
	}
}

func TestGjallarhornMcast_IsLinkLocalScope(t *testing.T) {
	t.Parallel()
	// ff02::/16 is the IPv6 link-local multicast scope. Per the source
	// comment, gjallarhornMcast lives in the "variable scope multicast"
	// range starting with ff02. Pin that scope so accidentally bumping
	// to a wider scope (ff05 site-local, ff0e global) is intentional.
	ip := net.ParseIP(gjallarhornMcast)
	if ip == nil || len(ip) != 16 {
		t.Fatalf("invalid IPv6 ip: %v", ip)
	}
	if ip[0] != 0xFF || ip[1]&0x0F != 0x02 {
		t.Errorf("gjallarhornMcast %q not in ff02::/16 link-local scope (got bytes %02X %02X)",
			gjallarhornMcast, ip[0], ip[1])
	}
}

func TestGjallarhornPort_InDoomRange(t *testing.T) {
	t.Parallel()
	if gjallarhornPort < 16666 || gjallarhornPort > 26666 {
		t.Errorf("gjallarhornPort %d outside Doom Range (16666-26666)", gjallarhornPort)
	}
}
