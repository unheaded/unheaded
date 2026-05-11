// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net"
	"strings"
	"testing"
)

// ─── lookupDefaultIP ─────────────────────────────────────────────────────────

func TestLookupDefaultIP_KnownServiceReturnsBridgeIP(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"timeguru":          "10.10.10.20",
		"wotan":             "10.10.10.10",
		"gateway":           "10.10.10.100",
		"dashboard-backend": "10.10.10.26",
	}
	for name, want := range cases {
		if got := lookupDefaultIP(name); got != want {
			t.Errorf("lookupDefaultIP(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestLookupDefaultIP_UnknownServiceReturnsEmpty(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"unknown", "", "TIMEGURU", "kingdom"} {
		if got := lookupDefaultIP(name); got != "" {
			t.Errorf("lookupDefaultIP(%q) = %q, want empty", name, got)
		}
	}
}

// ─── defaultServices invariants ─────────────────────────────────────────────

func TestDefaultServices_NamesUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]int)
	for _, svc := range defaultServices {
		seen[svc.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("duplicate Name in defaultServices: %s appears %d times", name, count)
		}
	}
}

func TestDefaultServices_IPsUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]string)
	for _, svc := range defaultServices {
		if existing, ok := seen[svc.IP]; ok {
			t.Errorf("duplicate IP %s: assigned to both %s and %s", svc.IP, existing, svc.Name)
		}
		seen[svc.IP] = svc.Name
	}
}

func TestDefaultServices_AllIPsInLXDBridgeSubnet(t *testing.T) {
	t.Parallel()
	// CLAUDE.md "Network Design": Bridge lxdbr0 = 10.10.10.0/24.
	// All cert SANs MUST sit in that subnet to be reachable over the bridge.
	_, subnet, err := net.ParseCIDR("10.10.10.0/24")
	if err != nil {
		t.Fatalf("parse subnet: %v", err)
	}
	for _, svc := range defaultServices {
		ip := net.ParseIP(svc.IP)
		if ip == nil {
			t.Errorf("invalid IP for %s: %q", svc.Name, svc.IP)
			continue
		}
		if !subnet.Contains(ip) {
			t.Errorf("%s IP %s not in lxdbr0 subnet %s", svc.Name, svc.IP, subnet)
		}
	}
}

func TestDefaultServices_NamesAreLowercaseHyphenated(t *testing.T) {
	t.Parallel()
	// Convention enforced for cert SAN consistency. Catches accidental
	// CamelCase or under_score additions.
	for _, svc := range defaultServices {
		for _, ch := range svc.Name {
			if !(ch == '-' || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) { //nolint:staticcheck // QF1001: De Morgan would invert allow-list to forbid-list — less readable for char validation
				t.Errorf("%s contains non-[a-z0-9-] char %q", svc.Name, ch)
				break
			}
		}
		if strings.Contains(svc.Name, "_") {
			t.Errorf("%s uses underscore — convention is hyphen", svc.Name)
		}
	}
}

func TestDefaultServices_AlphaCoverageHas10Services(t *testing.T) {
	t.Parallel()
	// CLAUDE.md S36 declares 10 core services. defaultServices should
	// match — pinning the count surfaces accidental drops or adds for
	// review. Update both this test AND CLAUDE.md if the count changes.
	if got, want := len(defaultServices), 10; got != want {
		t.Errorf("defaultServices count = %d, want %d (sync with CLAUDE.md S36)", got, want)
	}
}
