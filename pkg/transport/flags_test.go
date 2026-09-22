// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package transport

import (
	"flag"
	"os"
	"testing"
	"time"
)

func TestRegisterFlags(t *testing.T) {
	cfg := DefaultConfig()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	RegisterFlags(fs, &cfg)

	// Verify all flags exist
	expectedFlags := []string{"transport", "wotan-grpc-addr", "wotan-http-addr", "connect-timeout", "fallback"}
	for _, name := range expectedFlags {
		if fs.Lookup(name) == nil {
			t.Errorf("flag %q not registered", name)
		}
	}
}

func TestRegisterFlags_Parsing(t *testing.T) {
	cfg := DefaultConfig()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	RegisterFlags(fs, &cfg)

	err := fs.Parse([]string{
		"-transport", "grpc",
		"-wotan-grpc-addr", "wotan.local:18001",
		"-wotan-http-addr", "http://wotan.local:18000",
		"-connect-timeout", "10s",
		"-fallback=false",
	})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if cfg.Type != GRPC {
		t.Errorf("Type = %q, want %q", cfg.Type, GRPC)
	}
	if cfg.WotanGRPCAddr != "wotan.local:18001" {
		t.Errorf("WotanGRPCAddr = %q, want %q", cfg.WotanGRPCAddr, "wotan.local:18001")
	}
	if cfg.WotanHTTPAddr != "http://wotan.local:18000" {
		t.Errorf("WotanHTTPAddr = %q, want %q", cfg.WotanHTTPAddr, "http://wotan.local:18000")
	}
	if cfg.ConnectTimeout != 10*time.Second {
		t.Errorf("ConnectTimeout = %v, want %v", cfg.ConnectTimeout, 10*time.Second)
	}
	if cfg.FallbackEnabled {
		t.Error("FallbackEnabled = true, want false")
	}
}

func TestConfigFromEnv(t *testing.T) {
	cfg := DefaultConfig()

	// Set env vars
	os.Setenv("TRANSPORT_TYPE", "http")
	os.Setenv("WOTAN_GRPC_ADDR", "grpc.test:9999")
	os.Setenv("WOTAN_HTTP_ADDR", "http://http.test:8888")
	os.Setenv("CONNECT_TIMEOUT", "15s")
	defer func() {
		os.Unsetenv("TRANSPORT_TYPE")
		os.Unsetenv("WOTAN_GRPC_ADDR")
		os.Unsetenv("WOTAN_HTTP_ADDR")
		os.Unsetenv("CONNECT_TIMEOUT")
	}()

	ConfigFromEnv(&cfg)

	if cfg.Type != HTTP {
		t.Errorf("Type = %q, want %q", cfg.Type, HTTP)
	}
	if cfg.WotanGRPCAddr != "grpc.test:9999" {
		t.Errorf("WotanGRPCAddr = %q, want %q", cfg.WotanGRPCAddr, "grpc.test:9999")
	}
	if cfg.WotanHTTPAddr != "http://http.test:8888" {
		t.Errorf("WotanHTTPAddr = %q, want %q", cfg.WotanHTTPAddr, "http://http.test:8888")
	}
	if cfg.ConnectTimeout != 15*time.Second {
		t.Errorf("ConnectTimeout = %v, want %v", cfg.ConnectTimeout, 15*time.Second)
	}
}

func TestConfigFromEnv_EmptyDoesNotOverride(t *testing.T) {
	cfg := DefaultConfig()

	// Clear env vars
	os.Unsetenv("TRANSPORT_TYPE")
	os.Unsetenv("WOTAN_GRPC_ADDR")
	os.Unsetenv("WOTAN_HTTP_ADDR")
	os.Unsetenv("CONNECT_TIMEOUT")

	ConfigFromEnv(&cfg)

	// Should still have defaults
	if cfg.Type != Auto {
		t.Errorf("Type = %q, want %q (default)", cfg.Type, Auto)
	}
	if cfg.WotanGRPCAddr != "localhost:18001" {
		t.Errorf("WotanGRPCAddr = %q, want default", cfg.WotanGRPCAddr)
	}
}

func TestConfigFromEnv_InvalidDuration(t *testing.T) {
	cfg := DefaultConfig()
	os.Setenv("CONNECT_TIMEOUT", "not-a-duration")
	defer os.Unsetenv("CONNECT_TIMEOUT")

	ConfigFromEnv(&cfg)

	// Invalid duration should be silently ignored, keeping default
	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("ConnectTimeout = %v, want %v (default, invalid duration ignored)", cfg.ConnectTimeout, 5*time.Second)
	}
}

// WOTAN_ADDR is what docker-compose sets. ConfigFromEnv only read
// WOTAN_HTTP_ADDR, which nothing sets, so the HTTP address silently stayed
// at the localhost default inside every container.
func TestConfigFromEnv_WotanAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		httpAddr string
		want     string
	}{
		{"bare host:port gets a scheme", "wotan:18000", "", "http://wotan:18000"},
		{"url passes through", "http://wotan:18000", "", "http://wotan:18000"},
		{"https preserved", "https://wotan:18000", "", "https://wotan:18000"},
		{"explicit WOTAN_HTTP_ADDR wins", "wotan:18000", "http://other:9999", "http://other:9999"},
		{"unset leaves the default", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WOTAN_ADDR", tt.addr)
			t.Setenv("WOTAN_HTTP_ADDR", tt.httpAddr)

			cfg := DefaultConfig()
			def := cfg.WotanHTTPAddr
			ConfigFromEnv(&cfg)

			want := tt.want
			if want == "" {
				want = def
			}
			if cfg.WotanHTTPAddr != want {
				t.Errorf("WotanHTTPAddr = %q, want %q", cfg.WotanHTTPAddr, want)
			}
		})
	}
}
