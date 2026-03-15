// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Verify default values
	if cfg.Region != "default" {
		t.Errorf("expected region 'default', got %q", cfg.Region)
	}
	if cfg.Zone != "default" {
		t.Errorf("expected zone 'default', got %q", cfg.Zone)
	}
	if cfg.Server.HTTPAddr != ":17000" {
		t.Errorf("expected HTTP addr ':17000', got %q", cfg.Server.HTTPAddr)
	}
	if cfg.Server.GRPCAddr != ":17001" {
		t.Errorf("expected gRPC addr ':17001', got %q", cfg.Server.GRPCAddr)
	}
	if cfg.LXD.Socket != "/var/lib/lxd/unix.socket" {
		t.Errorf("expected LXD socket path, got %q", cfg.LXD.Socket)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected log level 'info', got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected log format 'json', got %q", cfg.Logging.Format)
	}
	if cfg.EBPF.Enabled != true {
		t.Error("expected eBPF enabled by default")
	}
	if cfg.NodeID == "" {
		t.Error("expected non-empty NodeID from DefaultConfig")
	}
}

func TestDefaultConfigValidates(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig should validate: %v", err)
	}
}

func TestValidate_NilConfig(t *testing.T) {
	var cfg *Config
	err := cfg.Validate()
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig for nil config, got %v", err)
	}
}

func TestValidate_MissingNodeID(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NodeID = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing node_id")
	}
}

func TestValidate_MissingServerAddr(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.HTTPAddr = ""
	cfg.Server.GRPCAddr = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when both HTTPAddr and GRPCAddr are empty")
	}
}

func TestValidate_NegativeTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.ReadTimeout = -1 * time.Second
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		valid bool
	}{
		{"debug", "debug", true},
		{"info", "info", true},
		{"warn", "warn", true},
		{"error", "error", true},
		{"invalid", "trace", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Logging.Level = tt.level
			err := cfg.Validate()
			if tt.valid && err != nil {
				t.Errorf("expected log level %q to be valid, got error: %v", tt.level, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected log level %q to be invalid", tt.level)
			}
		})
	}
}

func TestValidate_InvalidLogFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		valid  bool
	}{
		{"json", "json", true},
		{"text", "text", true},
		{"invalid", "yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Logging.Format = tt.format
			err := cfg.Validate()
			if tt.valid && err != nil {
				t.Errorf("expected format %q to be valid, got error: %v", tt.format, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected format %q to be invalid", tt.format)
			}
		})
	}
}

func TestValidate_PIIContainment(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		mode    string
		valid   bool
	}{
		{"disabled", false, "", true},
		{"eu_mode", true, "eu", true},
		{"strict_mode", true, "strict", true},
		{"invalid_mode", true, "off", false},
		{"empty_mode", true, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Security.DataIsolation.PIIContainmentEnabled = tt.enabled
			cfg.Security.DataIsolation.PIIMode = tt.mode
			err := cfg.Validate()
			if tt.valid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("expected error for invalid PII config")
			}
		})
	}
}

func TestLoad_EmptyPath(t *testing.T) {
	_, err := Load("")
	if err != ErrConfigNotFound {
		t.Errorf("expected ErrConfigNotFound for empty path, got %v", err)
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	// Create a temp config file
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.NodeID = "test-node-123"
	cfg.Region = "us-east-1"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.NodeID != "test-node-123" {
		t.Errorf("expected node_id 'test-node-123', got %q", loaded.NodeID)
	}
	if loaded.Region != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got %q", loaded.Region)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_PartialOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Only override region, other fields should use defaults
	partial := `{"region": "eu-west-1"}`
	if err := os.WriteFile(path, []byte(partial), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Region != "eu-west-1" {
		t.Errorf("expected overridden region 'eu-west-1', got %q", loaded.Region)
	}
	// Defaults should still be present
	if loaded.Server.HTTPAddr != ":17000" {
		t.Errorf("expected default HTTP addr, got %q", loaded.Server.HTTPAddr)
	}
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.json")

	cfg := DefaultConfig()
	cfg.NodeID = "save-test"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file exists and is valid
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}

	if loaded.NodeID != "save-test" {
		t.Errorf("expected node_id 'save-test', got %q", loaded.NodeID)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set environment variables
	t.Setenv("UNHEADED_NODE_ID", "env-node-42")
	t.Setenv("UNHEADED_NODE_NAME", "env-test-host")
	t.Setenv("UNHEADED_REGION", "ap-southeast-1")
	t.Setenv("UNHEADED_HTTP_ADDR", ":9999")
	t.Setenv("WOTAN_ADDR", "wotan.local:5555")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("EBPF_ENABLED", "false")

	cfg := LoadFromEnv()

	if cfg.NodeID != "env-node-42" {
		t.Errorf("expected node_id from env, got %q", cfg.NodeID)
	}
	if cfg.NodeName != "env-test-host" {
		t.Errorf("expected node_name from env, got %q", cfg.NodeName)
	}
	if cfg.Region != "ap-southeast-1" {
		t.Errorf("expected region from env, got %q", cfg.Region)
	}
	if cfg.Server.HTTPAddr != ":9999" {
		t.Errorf("expected HTTP addr from env, got %q", cfg.Server.HTTPAddr)
	}
	if cfg.Wotan.Addr != "wotan.local:5555" {
		t.Errorf("expected wotan addr from env, got %q", cfg.Wotan.Addr)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level from env, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("expected log format from env, got %q", cfg.Logging.Format)
	}
	if cfg.EBPF.Enabled {
		t.Error("expected eBPF disabled from env")
	}
}

func TestLoadFromEnv_PIIMode(t *testing.T) {
	t.Setenv("UNHEADED_PII_MODE", "eu")

	cfg := LoadFromEnv()
	if !cfg.Security.DataIsolation.PIIContainmentEnabled {
		t.Error("expected PII containment enabled")
	}
	if cfg.Security.DataIsolation.PIIMode != "eu" {
		t.Errorf("expected PII mode 'eu', got %q", cfg.Security.DataIsolation.PIIMode)
	}
}

func TestLoadFromEnv_PIIModeOff(t *testing.T) {
	t.Setenv("UNHEADED_PII_MODE", "off")

	cfg := LoadFromEnv()
	if cfg.Security.DataIsolation.PIIContainmentEnabled {
		t.Error("expected PII containment disabled for 'off'")
	}
}

func TestEnvironmentHelpers(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("default_is_production", func(t *testing.T) {
		t.Setenv("UNHEADED_ENV", "")
		if !cfg.IsProduction() {
			t.Error("expected production when env is empty")
		}
		if cfg.IsDevelopment() {
			t.Error("expected not development when env is empty")
		}
		if cfg.GetEnv() != "production" {
			t.Errorf("expected 'production', got %q", cfg.GetEnv())
		}
	})

	t.Run("development", func(t *testing.T) {
		t.Setenv("UNHEADED_ENV", "development")
		if !cfg.IsDevelopment() {
			t.Error("expected development")
		}
		if cfg.IsProduction() {
			t.Error("expected not production in development")
		}
		if cfg.GetEnv() != "development" {
			t.Errorf("expected 'development', got %q", cfg.GetEnv())
		}
	})

	t.Run("production_explicit", func(t *testing.T) {
		t.Setenv("UNHEADED_ENV", "production")
		if !cfg.IsProduction() {
			t.Error("expected production")
		}
	})
}
