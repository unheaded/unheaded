// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package transport

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Type != Auto {
		t.Errorf("DefaultConfig().Type = %q, want %q", cfg.Type, Auto)
	}
	if cfg.WotanGRPCAddr != "localhost:18001" {
		t.Errorf("DefaultConfig().WotanGRPCAddr = %q, want %q", cfg.WotanGRPCAddr, "localhost:18001")
	}
	if cfg.WotanHTTPAddr != "http://localhost:18000" {
		t.Errorf("DefaultConfig().WotanHTTPAddr = %q, want %q", cfg.WotanHTTPAddr, "http://localhost:18000")
	}
	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("DefaultConfig().ConnectTimeout = %v, want %v", cfg.ConnectTimeout, 5*time.Second)
	}
	if !cfg.FallbackEnabled {
		t.Error("DefaultConfig().FallbackEnabled = false, want true")
	}
}

func TestTypeConstants(t *testing.T) {
	tests := []struct {
		typ  Type
		want string
	}{
		{GRPC, "grpc"},
		{HTTP, "http"},
		{Auto, "auto"},
	}
	for _, tt := range tests {
		if string(tt.typ) != tt.want {
			t.Errorf("Type constant %v = %q, want %q", tt.typ, string(tt.typ), tt.want)
		}
	}
}

func TestConfigFieldsIndependent(t *testing.T) {
	cfg1 := DefaultConfig()
	cfg2 := DefaultConfig()
	cfg1.Type = GRPC
	cfg1.WotanGRPCAddr = "other:1234"

	if cfg2.Type != Auto {
		t.Error("modifying one Config affected another")
	}
	if cfg2.WotanGRPCAddr != "localhost:18001" {
		t.Error("modifying one Config affected another")
	}
}
