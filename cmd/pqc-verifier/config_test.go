// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"testing"
	"time"

	"unheaded/pkg/ports"
)

func TestDefaultConfig_PinnedDefaults(t *testing.T) {
	t.Parallel()
	c := DefaultConfig()
	if c.Port != ports.PQCVerifier {
		t.Errorf("Port: got %d, want ports.PQCVerifier=%d", c.Port, ports.PQCVerifier)
	}
	if c.WotanAddr != ports.DefaultWotanGRPC() {
		t.Errorf("WotanAddr: got %q, want DefaultWotanGRPC()", c.WotanAddr)
	}
	if c.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout: got %v, want 10s", c.ShutdownTimeout)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel: got %q, want info", c.LogLevel)
	}
	if c.DefaultTier != 0x01 {
		t.Errorf("DefaultTier: got 0x%02X, want 0x01 (STANDARD)", c.DefaultTier)
	}
	if c.AuthEnabled {
		t.Errorf("AuthEnabled: default should be false")
	}
}

func TestLoadFromEnv_OverridesPort(t *testing.T) {
	t.Setenv("PQC_VERIFIER_PORT", "29999")
	c := DefaultConfig()
	c.LoadFromEnv()
	if c.Port != 29999 {
		t.Errorf("Port: got %d, want 29999", c.Port)
	}
}

func TestLoadFromEnv_RejectsNonNumericPort(t *testing.T) {
	t.Setenv("PQC_VERIFIER_PORT", "not-a-port")
	c := DefaultConfig()
	want := c.Port
	c.LoadFromEnv()
	if c.Port != want {
		t.Errorf("non-numeric port should be ignored; got %d, want default %d", c.Port, want)
	}
}

func TestLoadFromEnv_OverridesWotanAddr(t *testing.T) {
	t.Setenv("WOTAN_ADDR", "wotan.example:18999")
	c := DefaultConfig()
	c.LoadFromEnv()
	if c.WotanAddr != "wotan.example:18999" {
		t.Errorf("WotanAddr: got %q, want wotan.example:18999", c.WotanAddr)
	}
}

func TestLoadFromEnv_OverridesLogLevelAndPolicyFile(t *testing.T) {
	t.Setenv("PQC_LOG_LEVEL", "debug")
	t.Setenv("PQC_POLICY_FILE", "/etc/unheaded/pqc-policy.yaml")
	c := DefaultConfig()
	c.LoadFromEnv()
	if c.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want debug", c.LogLevel)
	}
	if c.PolicyFile != "/etc/unheaded/pqc-policy.yaml" {
		t.Errorf("PolicyFile: got %q, want /etc/unheaded/pqc-policy.yaml", c.PolicyFile)
	}
}

func TestLoadFromEnv_DefaultTierRespectsByteWidth(t *testing.T) {
	t.Setenv("PQC_DEFAULT_TIER", "3")
	c := DefaultConfig()
	c.LoadFromEnv()
	if c.DefaultTier != 3 {
		t.Errorf("DefaultTier: got 0x%02X, want 0x03", c.DefaultTier)
	}
}

func TestLoadFromEnv_AuthEnabledAcceptsTrueAndOne(t *testing.T) {
	for _, val := range []string{"true", "1"} {
		t.Run("val="+val, func(t *testing.T) {
			t.Setenv("PQC_AUTH_ENABLED", val)
			c := DefaultConfig()
			c.LoadFromEnv()
			if !c.AuthEnabled {
				t.Errorf("PQC_AUTH_ENABLED=%q should set AuthEnabled=true", val)
			}
		})
	}
}

func TestLoadFromEnv_AuthEnabledRejectsOtherValues(t *testing.T) {
	for _, val := range []string{"yes", "TRUE", "0", "false", "enabled"} {
		t.Run("val="+val, func(t *testing.T) {
			t.Setenv("PQC_AUTH_ENABLED", val)
			c := DefaultConfig()
			c.LoadFromEnv()
			if c.AuthEnabled {
				t.Errorf("PQC_AUTH_ENABLED=%q should NOT enable auth (only 'true' or '1' do)", val)
			}
		})
	}
}
