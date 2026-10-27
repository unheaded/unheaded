// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package probenet

import (
	"strings"
	"testing"
)

func TestParse_DefaultIsTheLXDBridge(t *testing.T) {
	got, err := ParseCSV("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].String() != "10.10.10.0/24" {
		t.Fatalf("default %v, want [10.10.10.0/24]", got)
	}
}

func TestParse_Lists(t *testing.T) {
	got, err := ParseCSV(" 10.10.20.0/24, fd42:474b:622d:259d::/64 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].String() != "10.10.20.0/24" || got[1].String() != "fd42:474b:622d:259d::/64" {
		t.Fatalf("got %v", got)
	}
	// Host bits are masked off, not an error: 10.10.20.1/24 is the bridge's
	// own address as LXD prints it.
	got, err = Parse([]string{"10.10.20.1/24"})
	if err != nil || got[0].String() != "10.10.20.0/24" {
		t.Fatalf("10.10.20.1/24: %v %v", got, err)
	}
}

func TestParse_Refuses(t *testing.T) {
	for _, bad := range [][]string{
		{},                 // an empty list must not mean "nothing restricts"
		{"0.0.0.0/0"},      // everything
		{"::/0"},           // everything
		{"10.0.0.0/8"},     // broader than a bridge
		{"fd00::/32"},      // broader than a bridge
		{"10.10.10.5"},     // not a prefix
		{"lxdbr0"},         // a name
		{"127.0.0.0/24"},   // loopback
		{"169.254.0.0/16"}, // link-local
		{"fe80::/64"},      // link-local
		{"224.0.0.0/24"},   // multicast
	} {
		if got, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) = %v, want an error", bad, got)
		}
	}
}

func TestAllowed(t *testing.T) {
	subnets, err := ParseCSV("10.10.20.0/24,fd42:474b:622d:259d::/64")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ip   string
		want string // "" = refused
	}{
		{"10.10.20.128", "10.10.20.128"},
		{"::ffff:10.10.20.66", "10.10.20.66"}, // v4-mapped is the v4 address
		{"fd42:474b:622d:259d::42", "fd42:474b:622d:259d::42"},
		{"192.168.13.1", ""},  // the P2P peer (east)
		{"10.10.21.5", ""},    // next subnet over
		{"fe80::1", ""},       // link-local
		{"fe80::1%eth0", ""},  // zoned
		{"127.0.0.1", ""},     // loopback
		{"10.10.20.0/24", ""}, // not an address
		{"", ""},
		{"not-an-ip", ""},
	} {
		got, err := Allowed(subnets, tc.ip)
		if tc.want == "" {
			if err == nil {
				t.Errorf("Allowed(%q) = %q, want refused", tc.ip, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("Allowed(%q) = %q, %v; want %q", tc.ip, got, err, tc.want)
		}
	}
}

func TestAllowed_NoSubnetsRefusesEverything(t *testing.T) {
	if _, err := Allowed(nil, "10.10.10.5"); err == nil || !strings.Contains(err.Error(), "no probe subnets") {
		t.Fatalf("nil subnets: %v, want refusal", err)
	}
}
