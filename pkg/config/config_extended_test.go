// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetInt64(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"int64val":   int64(9999999999),
			"intval":     42,
			"floatval":   3.14,
			"strval":     "12345",
			"invalidstr": "notanumber",
			"boolval":    true,
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetInt64("int64val"); v != 9999999999 {
		t.Errorf("int64val = %d, want 9999999999", v)
	}
	if v := cfg.GetInt64("intval"); v != 42 {
		t.Errorf("intval = %d, want 42", v)
	}
	if v := cfg.GetInt64("floatval"); v != 3 {
		t.Errorf("floatval = %d, want 3", v)
	}
	if v := cfg.GetInt64("strval"); v != 12345 {
		t.Errorf("strval = %d, want 12345", v)
	}
	if v := cfg.GetInt64("invalidstr"); v != 0 {
		t.Errorf("invalidstr = %d, want 0", v)
	}
	if v := cfg.GetInt64("boolval"); v != 0 {
		t.Errorf("boolval = %d, want 0 (unsupported type)", v)
	}
	if v := cfg.GetInt64("missing"); v != 0 {
		t.Errorf("missing = %d, want 0", v)
	}
}

func TestGetString_Missing(t *testing.T) {
	cfg, err := Load(WithDefaults(map[string]any{}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetString("nonexistent"); v != "" {
		t.Errorf("expected empty string, got %q", v)
	}
}

func TestGetInt_Missing(t *testing.T) {
	cfg, err := Load(WithDefaults(map[string]any{}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetInt("nonexistent"); v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

func TestGetBool_IntConversion(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"nonzero": 1,
			"zero":    0,
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.GetBool("nonzero") {
		t.Error("expected nonzero int to be true")
	}
	if cfg.GetBool("zero") {
		t.Error("expected zero int to be false")
	}
	if cfg.GetBool("missing") {
		t.Error("expected missing to be false")
	}
}

func TestGetDuration_IntConversion(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"duration_int":   1000000000, // 1 second in nanoseconds
			"duration_int64": int64(2000000000),
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetDuration("duration_int"); v != time.Second {
		t.Errorf("duration_int = %v, want 1s", v)
	}
	if v := cfg.GetDuration("duration_int64"); v != 2*time.Second {
		t.Errorf("duration_int64 = %v, want 2s", v)
	}
	if v := cfg.GetDuration("missing"); v != 0 {
		t.Errorf("missing = %v, want 0", v)
	}
}

func TestGetFloat64_Missing(t *testing.T) {
	cfg, err := Load(WithDefaults(map[string]any{}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetFloat64("missing"); v != 0 {
		t.Errorf("expected 0, got %f", v)
	}
}

func TestGetStringSlice_Missing(t *testing.T) {
	cfg, err := Load(WithDefaults(map[string]any{}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetStringSlice("missing"); v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}

func TestGetStringMap_Missing(t *testing.T) {
	cfg, err := Load(WithDefaults(map[string]any{}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetStringMap("missing"); v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}

func TestUnmarshalKey(t *testing.T) {
	type ServerConfig struct {
		Host string `config:"host"`
		Port int    `config:"port"`
	}

	cfg, err := Load(
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var server ServerConfig
	if err := cfg.UnmarshalKey("server", &server); err != nil {
		t.Fatalf("UnmarshalKey: %v", err)
	}

	if server.Host != "localhost" {
		t.Errorf("Host = %q, want localhost", server.Host)
	}
	if server.Port != 8080 {
		t.Errorf("Port = %d, want 8080", server.Port)
	}

	// Test missing key
	if err := cfg.UnmarshalKey("nonexistent", &server); err == nil {
		t.Error("expected error for missing key")
	}

	// Test non-map key
	cfg.Set("scalar", "value")
	if err := cfg.UnmarshalKey("scalar", &server); err == nil {
		t.Error("expected error for non-map key")
	}
}

func TestAllSettings(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"key1": "value1",
			"key2": 42,
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	settings := cfg.AllSettings()
	if settings == nil {
		t.Fatal("AllSettings returned nil")
	}
	if settings["key1"] != "value1" {
		t.Errorf("key1 = %v, want value1", settings["key1"])
	}

	// Verify deep copy (modifying settings should not affect config)
	settings["key1"] = "modified"
	if cfg.GetString("key1") != "value1" {
		t.Error("AllSettings should return a deep copy")
	}
}

func TestUnmarshalWithSliceAndMap(t *testing.T) {
	type Config struct {
		Tags   []string          `config:"tags"`
		Labels map[string]string `config:"labels"`
	}

	cfg, err := Load(
		WithDefaults(map[string]any{
			"tags":   []any{"tag1", "tag2", "tag3"},
			"labels": map[string]any{"env": "prod", "team": "infra"},
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var c Config
	if err := cfg.Unmarshal(&c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(c.Tags) != 3 {
		t.Errorf("Tags len = %d, want 3", len(c.Tags))
	}
	if c.Labels["env"] != "prod" {
		t.Errorf("Labels[env] = %q, want prod", c.Labels["env"])
	}
}

func TestUnmarshalWithUint(t *testing.T) {
	type Config struct {
		Port uint16 `config:"port"`
	}

	cfg, err := Load(
		WithDefaults(map[string]any{
			"port": 8080,
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var c Config
	if err := cfg.Unmarshal(&c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080", c.Port)
	}
}

func TestUnmarshalWithFloat(t *testing.T) {
	type Config struct {
		Rate float64 `config:"rate"`
	}

	cfg, err := Load(
		WithDefaults(map[string]any{
			"rate": 0.95,
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var c Config
	if err := cfg.Unmarshal(&c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if c.Rate != 0.95 {
		t.Errorf("Rate = %f, want 0.95", c.Rate)
	}
}

func TestUnmarshalWithBool(t *testing.T) {
	type Config struct {
		Enabled bool `config:"enabled"`
	}

	cfg, err := Load(
		WithDefaults(map[string]any{
			"enabled": true,
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var c Config
	if err := cfg.Unmarshal(&c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !c.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestWriteToFile_Formats(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := Load(
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host":    "localhost",
				"port":    8080,
				"debug":   true,
				"timeout": 30,
			},
			"tags": []any{"a", "b"},
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Write and read back each format
	formats := []string{"config.json", "config.toml", "config.yaml", "config.yml"}
	for _, name := range formats {
		path := filepath.Join(tmpDir, name)
		if err := cfg.WriteToFile(path); err != nil {
			t.Errorf("WriteToFile(%s): %v", name, err)
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("ReadFile(%s): %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("WriteToFile(%s) produced empty file", name)
		}
	}

	// Unknown extension
	unknownPath := filepath.Join(tmpDir, "config.xyz")
	if err := cfg.WriteToFile(unknownPath); err == nil {
		t.Error("expected error for unknown extension")
	}
}

func TestClose_NoWatcher(t *testing.T) {
	cfg, err := Load(WithDefaults(map[string]any{"k": "v"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Close without a watcher should not error
	if err := cfg.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestFromJSON_Invalid(t *testing.T) {
	_, err := FromJSON([]byte(`{invalid json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMustLoad_Success(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("MustLoad should not panic with valid config: %v", r)
		}
	}()

	cfg := MustLoad(WithDefaults(map[string]any{"k": "v"}))
	if cfg.GetString("k") != "v" {
		t.Error("MustLoad returned wrong config")
	}
}

func TestSubNonExistent(t *testing.T) {
	cfg, err := Load(WithDefaults(map[string]any{"k": "v"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	sub := cfg.Sub("nonexistent")
	if sub.GetString("anything") != "" {
		t.Error("sub of nonexistent key should return empty config")
	}
}

func TestGetFloat64_Int64Conversion(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"int64val": int64(42),
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetFloat64("int64val"); v != 42.0 {
		t.Errorf("int64val = %f, want 42.0", v)
	}
}

func TestGetBool_Default(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"floatval": 3.14,
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// float64 is not a supported bool conversion → should return false
	if cfg.GetBool("floatval") {
		t.Error("expected false for float value")
	}
}

func TestGetDuration_Default(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"floatval": 3.14,
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// float64 is not a supported duration conversion → should return 0
	if v := cfg.GetDuration("floatval"); v != 0 {
		t.Errorf("expected 0 for float value, got %v", v)
	}
}

func TestGetFloat64_Default(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"boolval": true,
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// bool is not a supported float conversion → should return 0
	if v := cfg.GetFloat64("boolval"); v != 0 {
		t.Errorf("expected 0 for bool value, got %f", v)
	}
}

func TestGetStringSlice_NonSliceType(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"scalar": "notaslice",
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetStringSlice("scalar"); v != nil {
		t.Errorf("expected nil for non-slice value, got %v", v)
	}
}

func TestGetStringMap_NonMapType(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"scalar": "notamap",
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v := cfg.GetStringMap("scalar"); v != nil {
		t.Errorf("expected nil for non-map value, got %v", v)
	}
}
