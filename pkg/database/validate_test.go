// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"strings"
	"testing"
)

// ─── ValidateConfig ──────────────────────────────────────────────────────────

func TestValidateConfig_RejectsEmptyPassword(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Host:    "localhost",
		Port:    "5432",
		User:    "u",
		DBName:  "d",
		SSLMode: "disable",
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected empty-password rejection, got nil")
	}
	if !strings.Contains(err.Error(), "POSTGRES_PASSWORD") {
		t.Errorf("expected POSTGRES_PASSWORD in error, got: %v", err)
	}
}

func TestValidateConfig_AllowsDefaultPasswordOnPrivateHostWithSSLDisabled(t *testing.T) {
	t.Parallel()
	// dev-mode default: password is weak BUT host is local + SSL disabled.
	// The current policy only blocks when SSL is ENABLED with a default
	// password (mismatch — looks like prod but isn't real). On disable it
	// passes.
	for _, pw := range []string{"unheaded_dev", "postgres", "password"} {
		cfg := Config{
			Host:     "localhost",
			Port:     "5432",
			User:     "u",
			Password: pw,
			DBName:   "d",
			SSLMode:  "disable",
		}
		if err := ValidateConfig(cfg); err != nil {
			t.Errorf("default pw %q on localhost+disable should pass, got: %v", pw, err)
		}
	}
}

func TestValidateConfig_BlocksDefaultPasswordWithSSL(t *testing.T) {
	t.Parallel()
	for _, pw := range []string{"unheaded_dev", "postgres", "password"} {
		cfg := Config{
			Host:     "localhost",
			Port:     "5432",
			User:     "u",
			Password: pw,
			DBName:   "d",
			SSLMode:  "require",
		}
		err := ValidateConfig(cfg)
		if err == nil {
			t.Errorf("default pw %q with SSL=require should be rejected", pw)
		}
		if err != nil && !strings.Contains(err.Error(), "default password") {
			t.Errorf("expected 'default password' in error for pw %q, got: %v", pw, err)
		}
	}
}

func TestValidateConfig_BlocksSSLDisableOnPublicHost(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Host:     "the-well.bellis.tech",
		Port:     "5432",
		User:     "u",
		Password: "real-strong-password-not-default",
		DBName:   "d",
		SSLMode:  "disable",
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected SSL-disabled-on-public-host rejection, got nil")
	}
	if !strings.Contains(err.Error(), "SSL disabled") {
		t.Errorf("expected 'SSL disabled' in error, got: %v", err)
	}
}

func TestValidateConfig_AcceptsHardenedConfig(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Host:     "the-well.bellis.tech",
		Port:     "5432",
		User:     "unheaded",
		Password: "k7QvGv-real-prod-secret",
		DBName:   "the_well",
		SSLMode:  "require",
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("hardened config should validate, got: %v", err)
	}
}

// ─── isPrivateHost ───────────────────────────────────────────────────────────

func TestIsPrivateHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"10.10.10.20", true},  // Wotan IP per CLAUDE.md
		{"172.16.0.1", true},   // RFC 1918
		{"192.168.1.1", true},  // home LAN
		{"8.8.8.8", false},     // public DNS
		{"the-well.bellis.tech", false},
		{"github.com", false},
		{"", false},
		{"100.64.0.1", false},   // CGNAT — current impl doesn't recognize this
	}
	for _, c := range cases {
		if got := isPrivateHost(c.host); got != c.want {
			t.Errorf("isPrivateHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

// ─── DSN ─────────────────────────────────────────────────────────────────────

func TestConfig_DSN_FormatsAllSixFields(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Host:     "h",
		Port:     "5432",
		User:     "u",
		Password: "p",
		DBName:   "d",
		SSLMode:  "require",
	}
	dsn := cfg.DSN()
	for _, want := range []string{
		"host=h", "port=5432", "user=u", "password=p", "dbname=d", "sslmode=require",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN missing %q: %q", want, dsn)
		}
	}
}
