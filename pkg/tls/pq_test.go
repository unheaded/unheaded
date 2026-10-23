// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package tls

import (
	"crypto/tls"
	"slices"
	"testing"
)

// mlkem1024 is crypto/tls MLKEM1024 (IANA 0x0202). The constant needs go1.27
// under vet while go.mod says 1.26; pq_go127_test.go pins the two together.
const mlkem1024 = tls.CurveID(0x0202)

func TestStrictGroupsAreHybridOnly(t *testing.T) {
	got := StrictKEX()
	want := []tls.CurveID{tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024}
	if !slices.Equal(got, want) {
		t.Fatalf("StrictKEX() = %v, want %v", got, want)
	}
	for _, id := range got {
		if !IsHybrid(id) {
			t.Errorf("StrictKEX() contains %v, which is not hybrid", id)
		}
	}
}

func TestStrictKEXReturnsAFreshSlice(t *testing.T) {
	a := StrictKEX()
	a[0] = tls.X25519
	if b := StrictKEX(); b[0] != tls.X25519MLKEM768 {
		t.Fatalf("mutating one StrictKEX() result changed the next: %v", b)
	}
}

func TestIsHybrid(t *testing.T) {
	tests := []struct {
		id   tls.CurveID
		want bool
	}{
		{tls.X25519MLKEM768, true},
		{tls.SecP256r1MLKEM768, true},
		{tls.SecP384r1MLKEM1024, true},
		{tls.X25519, false},
		{tls.CurveP256, false},
		{tls.CurveP384, false},
		{tls.CurveP521, false},
		{mlkem1024, false}, // post-quantum but not hybrid: the CBOM calls it pq-only
		{0, false},
		{tls.CurveID(0xFFFF), false},
	}
	for _, tt := range tests {
		if got := IsHybrid(tt.id); got != tt.want {
			t.Errorf("IsHybrid(%v) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestApplyStrictSetsTLS13(t *testing.T) {
	cfg := &tls.Config{}
	if got := ApplyStrict(cfg); got != cfg {
		t.Fatal("ApplyStrict must return the config it was given")
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %#x, want TLS 1.3", cfg.MinVersion)
	}
	if !slices.Equal(cfg.CurvePreferences, StrictKEX()) {
		t.Errorf("CurvePreferences = %v, want %v", cfg.CurvePreferences, StrictKEX())
	}
}

func TestApplyStrictReplacesAClassicalPin(t *testing.T) {
	cfg := &tls.Config{CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256}}
	ApplyStrict(cfg)
	for _, id := range cfg.CurvePreferences {
		if !IsHybrid(id) {
			t.Fatalf("classical group %v survived ApplyStrict", id)
		}
	}
}

func TestApplyPreferredLeavesDefaults(t *testing.T) {
	cfg := &tls.Config{CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256}}
	if got := ApplyPreferred(cfg); got != cfg {
		t.Fatal("ApplyPreferred must return the config it was given")
	}
	if cfg.CurvePreferences != nil {
		t.Errorf("CurvePreferences = %v, want nil (Go default: hybrids first)", cfg.CurvePreferences)
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %#x, want TLS 1.3", cfg.MinVersion)
	}
}

func TestApplyStrictRefusesWeakerOverride(t *testing.T) {
	for name, apply := range map[string]func(*tls.Config) *tls.Config{"strict": ApplyStrict, "preferred": ApplyPreferred} {
		t.Run(name, func(t *testing.T) {
			cfg := &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12}
			apply(cfg)
			if cfg.MinVersion != tls.VersionTLS13 {
				t.Errorf("MinVersion = %#x, want raised to TLS 1.3", cfg.MinVersion)
			}
			if cfg.MaxVersion != 0 && cfg.MaxVersion < tls.VersionTLS13 {
				t.Errorf("MaxVersion = %#x still caps below TLS 1.3", cfg.MaxVersion)
			}
		})
	}
}

func TestApplyNilAllocates(t *testing.T) {
	for name, apply := range map[string]func(*tls.Config) *tls.Config{"strict": ApplyStrict, "preferred": ApplyPreferred} {
		cfg := apply(nil)
		if cfg == nil || cfg.MinVersion != tls.VersionTLS13 {
			t.Errorf("%s(nil) = %+v, want a fresh TLS 1.3 config", name, cfg)
		}
	}
}

func TestNewServerTLSConfigIsPreferred(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "svc"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}
	cfg, err := NewServerTLSConfig(ServerConfig{CertPEM: sc.CertPEM, KeyPEM: sc.KeyPEM})
	if err != nil {
		t.Fatalf("NewServerTLSConfig: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS13 || cfg.CurvePreferences != nil {
		t.Fatalf("server config MinVersion=%#x CurvePreferences=%v, want 1.3 and nil", cfg.MinVersion, cfg.CurvePreferences)
	}
}
