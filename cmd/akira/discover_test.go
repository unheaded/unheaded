// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Discovery is off unless the config or --discover-subnets names subnets
// (ADR-147 Phase 9); the flag wins; a bad prefix is an error.
func TestDiscoverSubnets(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	plain := write("plain.yaml", "targets:\n  - name: wotan\n    host: localhost\n    port: 18000\n")
	on := write("on.yaml", "targets: []\ndiscover:\n  subnets: [\"172.28.1.0/24\"]\n")
	bad := write("bad.yaml", "discover:\n  subnets: [\"not-a-prefix\"]\n")

	if got, err := discoverSubnets("", ""); err != nil || got != nil {
		t.Fatalf("no config, no flag: %v %v, want off", got, err)
	}
	if got, err := discoverSubnets(plain, ""); err != nil || got != nil {
		t.Fatalf("config without discover: %v %v, want off", got, err)
	}
	if got, err := discoverSubnets(on, ""); err != nil || len(got) != 1 || got[0].String() != "172.28.1.0/24" {
		t.Fatalf("config: %v %v", got, err)
	}
	if got, err := discoverSubnets(on, "10.10.20.0/24,192.168.13.0/30"); err != nil || len(got) != 2 || got[0].String() != "10.10.20.0/24" {
		t.Fatalf("flag wins: %v %v", got, err)
	}
	if _, err := discoverSubnets(bad, ""); err == nil {
		t.Fatal("a bad prefix was accepted")
	}
	if _, err := discoverSubnets("", "0.0.0.0/0"); err == nil {
		t.Fatal("a /0 was accepted")
	}
}
