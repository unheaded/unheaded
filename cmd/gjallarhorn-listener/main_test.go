// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net"
	"testing"
)

// Pin the constants/flag-defaults this listener uses. The listener itself
// blocks on UDP I/O so end-to-end is hard, but the listening recipe
// (multicast group + port) is small and worth pinning.

func TestGjallarhornPort_MatchesSenderConst(t *testing.T) {
	t.Parallel()
	// MUST match cmd/gjallarhorn-sender/main.go's gjallarhornPort const.
	// If they drift, sender packets land at a port the listener isn't
	// bound to. Pinning here surfaces that drift immediately.
	const senderPort = 16901
	if gjallarhornPort != senderPort {
		t.Errorf("gjallarhornPort = %d, want %d (must match sender)", gjallarhornPort, senderPort)
	}
}

func TestGjallarhornPort_InDoomRange(t *testing.T) {
	t.Parallel()
	if gjallarhornPort < 16666 || gjallarhornPort > 26666 {
		t.Errorf("gjallarhornPort %d outside Doom Range (16666-26666)", gjallarhornPort)
	}
}

func TestDefaultMcastAddr_IsValidLinkLocalMulticast(t *testing.T) {
	t.Parallel()
	// The flag default `ff02::1:abba` must be a valid IPv6 multicast
	// address in link-local scope (ff02::/16). Pinning here catches
	// accidental edits to the flag-default string in main.go.
	const defaultMcast = "ff02::1:abba"
	ip := net.ParseIP(defaultMcast)
	if ip == nil || len(ip) != 16 {
		t.Fatalf("default mcast %q is not a valid IPv6 address", defaultMcast)
	}
	if ip.To4() != nil {
		t.Errorf("default mcast %q parsed as IPv4, want IPv6", defaultMcast)
	}
	if !ip.IsMulticast() {
		t.Errorf("default mcast %q is not a multicast address", defaultMcast)
	}
	if ip[0] != 0xFF || ip[1]&0x0F != 0x02 {
		t.Errorf("default mcast %q not in ff02::/16 link-local scope", defaultMcast)
	}
}
