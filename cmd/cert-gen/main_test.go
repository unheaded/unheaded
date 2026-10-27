// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		if svc.IP == "" {
			continue // no bridge address: compose-only services
		}
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
		if svc.IP == "" {
			continue // no bridge address: compose-only services
		}
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

// S-IRON-GATE Phase 8: a certificate for every first-party compose service
// (the 13 that check-service-auth counts, by compose name: the name a
// caller dials and so the SAN TLS verifies), the CLI's client cert, and
// the legacy gateway.
func TestDefaultServices_CoverFirstPartyComposeServices(t *testing.T) {
	t.Parallel()
	want := []string{"wotan", "timeguru", "captain", "architect", "micromanager", "akira", "monad",
		"sophia", "dashboard-backend", "kanban-app", "cuirass", "auth", "sword", "unheaded-cli"}
	have := map[string]bool{}
	for _, s := range defaultServices {
		have[s.Name] = true
	}
	for _, n := range want {
		if !have[n] {
			t.Errorf("no default certificate for %s", n)
		}
	}
}

func TestGenerate_PerServiceLayoutAndReuseCA(t *testing.T) {
	out := t.TempDir()
	o := options{out: out, services: "auth,cuirass", ttl: time.Hour, layout: "per-service", reuseCA: true}
	if err := generate(o, io.Discard); err != nil {
		t.Fatal(err)
	}
	ca1, err := os.ReadFile(filepath.Join(out, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range []string{"auth", "cuirass"} {
		dir := filepath.Join(out, svc)
		for _, f := range []string{svc + ".pem", svc + "-key.pem", "ca.pem"} {
			if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
				t.Errorf("%s/%s: %v", svc, f, err)
			}
		}
		st, err := os.Stat(filepath.Join(dir, svc+"-key.pem"))
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("%s key mode = %v (%v), want 0600", svc, st, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "ca-key.pem")); err == nil {
			t.Errorf("%s: the CA key was copied into a service directory", svc)
		}
	}
	// SANs: the compose name and the auth identity.
	certPEM, _ := os.ReadFile(filepath.Join(out, "cuirass", "cuirass.pem"))
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cuirass", "unheaded-daemon", "localhost"} {
		if err := cert.VerifyHostname(name); err != nil {
			t.Errorf("cuirass cert does not cover %s: %v", name, err)
		}
	}
	if err := cert.VerifyHostname("127.0.0.1"); err != nil {
		t.Errorf("cuirass cert does not cover 127.0.0.1: %v", err)
	}

	// A re-run keeps the CA, so services already running keep trusting it.
	if err := generate(o, io.Discard); err != nil {
		t.Fatal(err)
	}
	ca2, _ := os.ReadFile(filepath.Join(out, "ca.pem"))
	if string(ca1) != string(ca2) {
		t.Fatal("-reuse-ca made a new CA")
	}
	o.reuseCA = false
	if err := generate(o, io.Discard); err != nil {
		t.Fatal(err)
	}
	ca3, _ := os.ReadFile(filepath.Join(out, "ca.pem"))
	if string(ca1) == string(ca3) {
		t.Fatal("without -reuse-ca the CA was kept")
	}
}

func TestGenerate_RejectsBadLayoutAndNames(t *testing.T) {
	if err := generate(options{out: t.TempDir(), layout: "nested"}, io.Discard); err == nil {
		t.Error("unknown layout accepted")
	}
	if err := generate(options{out: t.TempDir(), layout: "per-service", services: "../etc"}, io.Discard); err == nil {
		t.Error("a path in a service name was accepted")
	}
}
