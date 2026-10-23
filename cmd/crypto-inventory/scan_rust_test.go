// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"strings"
	"testing"
)

func scanRustFixtures(t *testing.T) []Asset {
	t.Helper()
	assets, err := scanRust(fixtureTree(t, "rust"))
	if err != nil {
		t.Fatalf("scanRust: %v", err)
	}
	return assets
}

func TestScanRust_Classification(t *testing.T) {
	assets := scanRustFixtures(t)
	tests := []struct {
		name   string
		path   string
		line   int
		kind   string
		status string
		link   string
	}{
		{"ring provider call", "crates/ringy/src/lib.rs", 9, kindRustProvider, pqClassical, linkEnabled},
		{"aws-lc-rs provider call", "crates/awsy/src/lib.rs", 5, kindRustProvider, pqHybrid, linkEnabled},
		{"builder on the process default, ring-only crate", "crates/ringy/src/lib.rs", 5, kindRustBuilder, pqClassical, linkEnabled},
		{"ring-only rustls dependency", "crates/ringy/Cargo.toml", 7, kindRustDep, pqClassical, linkEnabled},
		{"default rustls features (aws-lc-rs)", "crates/awsy/Cargo.toml", 6, kindRustDep, pqHybrid, linkEnabled},
		{"both providers compiled", "crates/bothy/Cargo.toml", 6, kindRustDep, pqNA, linkEnabled},
		{"tonic tls feature never used", "crates/tonicy/Cargo.toml", 6, kindRustDep, pqClassical, linkUnusedFeature},
		{"reqwest rustls-tls never used", "crates/reqy/Cargo.toml", 6, kindRustDep, pqClassical, linkUnusedFeature},
		{"lockfile providers", "Cargo.lock", 0, kindRustLock, pqNA, linkEnabled},
		{"pqcrypto ML-KEM in the lockfile", "Cargo.lock", 0, kindKEMImport, pqNA, linkEnabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := one(t, assets, tt.path, tt.line, tt.kind)
			if a.PQStatus != tt.status {
				t.Errorf("%s: pq-status = %q, want %q (detail %q)", a.Ref(), a.PQStatus, tt.status, a.Detail)
			}
			if a.LinkState != tt.link {
				t.Errorf("%s: link-state = %q, want %q", a.Ref(), a.LinkState, tt.link)
			}
			if a.Scanner != "rust" {
				t.Errorf("%s: scanner = %q", a.Ref(), a.Scanner)
			}
		})
	}
}

func TestScanRust_CommentIsNotACallSite(t *testing.T) {
	if got := at(scanRustFixtures(t), "crates/bothy/src/lib.rs", 0, kindRustProvider); len(got) != 0 {
		t.Errorf("a provider named in a comment was inventoried: %v", got)
	}
}

func TestScanRust_ProviderBuilderNotCountedTwice(t *testing.T) {
	// builder_with_provider sits on the line after the provider call; the
	// provider call is the site, the builder line is not a second one.
	if got := at(scanRustFixtures(t), "crates/awsy/src/lib.rs", 6, kindRustBuilder); len(got) != 0 {
		t.Errorf("builder_with_provider counted as a process-default builder: %v", got)
	}
}

func TestScanRust_LockDetail(t *testing.T) {
	a := one(t, scanRustFixtures(t), "Cargo.lock", 0, kindRustLock)
	for _, want := range []string{"rustls 0.23.46", "aws-lc-rs", "ring"} {
		if !strings.Contains(a.Detail+" "+a.Name, want) {
			t.Errorf("lock asset %q / %q lacks %q", a.Name, a.Detail, want)
		}
	}
}

func TestVersionLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"0.9.1", "0.23.0", true}, {"0.23.0", "0.9.1", false}, {"0.23.46", "0.23.46", false},
		{"0.23", "0.23.1", true}, {"1.0.0-rc1", "1.0.0-rc2", true},
	}
	for _, tt := range tests {
		if got := versionLess(tt.a, tt.b); got != tt.want {
			t.Errorf("versionLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRustlsVersionAtLeast(t *testing.T) {
	tests := []struct {
		v    string
		want bool
	}{
		{"0.23.46", true}, {"0.23.27", true}, {"0.23.26", false}, {"0.22.4", false},
		{"0.24.0", true}, {"1.0.0", true}, {"garbage", false}, {"", false}, {"0.23", false},
	}
	for _, tt := range tests {
		if got := rustlsHasHybridDefault(tt.v); got != tt.want {
			t.Errorf("rustlsHasHybridDefault(%q) = %v, want %v", tt.v, got, tt.want)
		}
	}
}
