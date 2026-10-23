// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"strings"
	"testing"
)

func scanConfigFixtures(t *testing.T) []Asset {
	t.Helper()
	assets, err := scanConfig(fixtureTree(t, "config"))
	if err != nil {
		t.Fatalf("scanConfig: %v", err)
	}
	return assets
}

func TestScanConfig_Classification(t *testing.T) {
	assets := scanConfigFixtures(t)
	tests := []struct {
		name   string
		path   string
		line   int
		kind   string
		status string
	}{
		{"postgres 16 (one ECDH curve)", "docker-compose.yml", 5, kindPostgresImage, pqClassical},
		{"postgres 18 with hybrid ssl_groups", "deploy/pg18/compose.yml", 3, kindPostgresImage, pqHybrid},
		{"postgresql.conf with TLS on, default groups", "db/postgresql.conf", 3, kindPostgresConf, pqClassical},
		{"ssl_groups with a hybrid first", "db/postgresql.pg18.conf.d/10-pq-groups.conf", 2, kindPostgresConf, pqHybrid},
		{"traefik edge (TLS off by decision)", "docker-compose.yml", 3, kindProxyImage, pqUnknown},
		{"sshd classical KEX", "runbooks/security/ssh.yaml", 4, kindSSHDKex, pqClassical},
		{"sshd hybrid first plus classical", "deploy/ssh/sshd_config", 2, kindSSHDKex, pqHybridOpp},
		{"sshd hybrid only (= form)", "deploy/ssh/sshd_strict.conf", 1, kindSSHDKex, pqHybridStrict},
		{"patch: only the added line counts", "nix/patches/0001-sshd.patch", 6, kindSSHDKex, pqClassical},
		{"WireGuard with a static PSK", "scripts/wg/setup.sh", 5, kindWireGuard, pqPSKStatic},
		{"WireGuard peer without a PSK", "scripts/wg/nopsk.conf", 4, kindWireGuard, pqClassical},
		{"GODEBUG kill switch in a unit", "deploy/units/app.service", 2, kindGodebug, pqClassical},
		{"go directive below 1.24", "go.mod", 3, kindGoDirective, pqClassical},
		{"go directive 1.26 with a godebug kill switch", "tools/new/go.mod", 5, kindGodebug, pqClassical},
		{"go directive 1.26", "tools/new/go.mod", 3, kindGoDirective, pqHybrid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := one(t, assets, tt.path, tt.line, tt.kind)
			if a.PQStatus != tt.status {
				t.Errorf("%s: pq-status = %q, want %q (detail %q)", a.Ref(), a.PQStatus, tt.status, a.Detail)
			}
			if a.Scanner != "config" {
				t.Errorf("%s: scanner = %q", a.Ref(), a.Scanner)
			}
		})
	}
}

func TestScanConfig_PatchRemovedLineAndCommentIgnored(t *testing.T) {
	got := at(scanConfigFixtures(t), "nix/patches/0001-sshd.patch", 0, kindSSHDKex)
	if len(got) != 1 {
		t.Fatalf("patch: want only the added KexAlgorithms line, got %d assets", len(got))
	}
}

func TestScanConfig_EdgeIsDisabledByConfig(t *testing.T) {
	a := one(t, scanConfigFixtures(t), "docker-compose.yml", 3, kindProxyImage)
	if a.LinkState != linkDisabled {
		t.Errorf("traefik link-state = %q, want %q", a.LinkState, linkDisabled)
	}
}

func TestScanConfig_RecordsKexList(t *testing.T) {
	a := one(t, scanConfigFixtures(t), "deploy/ssh/sshd_config", 2, kindSSHDKex)
	if strings.Join(a.Groups, ",") != "mlkem768x25519-sha256,curve25519-sha256" {
		t.Errorf("groups = %v", a.Groups)
	}
}

func TestClassifySSHKex(t *testing.T) {
	tests := []struct {
		list []string
		want string
	}{
		{[]string{"curve25519-sha256"}, pqClassical},
		{[]string{"sntrup761x25519-sha512@openssh.com", "curve25519-sha256"}, pqHybridOpp},
		{[]string{"curve25519-sha256", "mlkem768x25519-sha256"}, pqHybridOpp},
		{[]string{"mlkem768x25519-sha256", "sntrup761x25519-sha512"}, pqHybridStrict},
		{[]string{"+mlkem768x25519-sha256"}, pqUnknown}, // relative to the compiled-in default: cannot tell
		{nil, pqUnknown},
	}
	for _, tt := range tests {
		if got := classifySSHKex(tt.list); got != tt.want {
			t.Errorf("classifySSHKex(%v) = %q, want %q", tt.list, got, tt.want)
		}
	}
}
