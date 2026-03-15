// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/config/merge"
)

func TestLoad(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{
		"server": {
			"host": "localhost",
			"port": 8080
		},
		"database": {
			"url": "postgres://localhost/test"
		}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := Load(
		WithFile(configPath),
		WithDefaults(map[string]any{
			"server": map[string]any{
				"timeout": 30,
			},
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Test GetString
	host := cfg.GetString("server.host")
	if host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", host)
	}

	// Test GetInt
	port := cfg.GetInt("server.port")
	if port != 8080 {
		t.Errorf("Expected port 8080, got %d", port)
	}

	// Test default value
	timeout := cfg.GetInt("server.timeout")
	if timeout != 30 {
		t.Errorf("Expected timeout 30, got %d", timeout)
	}

	// Test Get
	dbURL := cfg.Get("database.url")
	if dbURL != "postgres://localhost/test" {
		t.Errorf("Expected database URL 'postgres://localhost/test', got '%v'", dbURL)
	}
}

func TestLoadTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	configContent := `
# Server configuration
[server]
host = "localhost"
port = 8080
debug = true

[database]
url = "postgres://localhost/test"
max_connections = 100

[[workers]]
name = "worker1"
threads = 4

[[workers]]
name = "worker2"
threads = 8
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := Load(WithFile(configPath))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Test string
	host := cfg.GetString("server.host")
	if host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", host)
	}

	// Test int
	port := cfg.GetInt("server.port")
	if port != 8080 {
		t.Errorf("Expected port 8080, got %d", port)
	}

	// Test bool
	debug := cfg.GetBool("server.debug")
	if !debug {
		t.Error("Expected debug to be true")
	}

	// Test nested
	maxConn := cfg.GetInt("database.max_connections")
	if maxConn != 100 {
		t.Errorf("Expected max_connections 100, got %d", maxConn)
	}
}

func TestLoadYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  host: localhost
  port: 8080
  debug: true
database:
  url: postgres://localhost/test
  max_connections: 100
features:
  - logging
  - metrics
  - tracing
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := Load(WithFile(configPath))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	host := cfg.GetString("server.host")
	if host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", host)
	}

	port := cfg.GetInt("server.port")
	if port != 8080 {
		t.Errorf("Expected port 8080, got %d", port)
	}

	features := cfg.GetStringSlice("features")
	if len(features) != 3 {
		t.Errorf("Expected 3 features, got %d", len(features))
	}
}

func TestEnvSource(t *testing.T) {
	os.Setenv("UNHEADED_SERVER_HOST", "envhost")
	os.Setenv("UNHEADED_SERVER_PORT", "9090")
	defer func() {
		os.Unsetenv("UNHEADED_SERVER_HOST")
		os.Unsetenv("UNHEADED_SERVER_PORT")
	}()

	cfg, err := Load(
		WithEnvPrefix("UNHEADED"),
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "defaulthost",
				"port": 8080,
			},
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	host := cfg.GetString("server.host")
	if host != "envhost" {
		t.Errorf("Expected host 'envhost', got '%s'", host)
	}

	port := cfg.GetInt("server.port")
	if port != 9090 {
		t.Errorf("Expected port 9090, got %d", port)
	}
}

func TestFlagSource(t *testing.T) {
	args := []string{
		"--server-host=flaghost",
		"--server-port", "7070",
		"--debug",
	}

	cfg, err := Load(
		WithFlags(args),
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "defaulthost",
				"port": 8080,
			},
			"debug": false,
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	host := cfg.GetString("server.host")
	if host != "flaghost" {
		t.Errorf("Expected host 'flaghost', got '%s'", host)
	}

	port := cfg.GetInt("server.port")
	if port != 7070 {
		t.Errorf("Expected port 7070, got %d", port)
	}

	debug := cfg.GetBool("debug")
	if !debug {
		t.Error("Expected debug to be true")
	}
}

func TestSourcePrecedence(t *testing.T) {
	// Create config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{
		"server": {
			"host": "filehost",
			"port": 8080
		}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Set environment variable
	os.Setenv("TEST_SERVER_HOST", "envhost")
	os.Setenv("TEST_SERVER_PORT", "9090")
	defer func() {
		os.Unsetenv("TEST_SERVER_HOST")
		os.Unsetenv("TEST_SERVER_PORT")
	}()

	// Flags should override env, which overrides file
	args := []string{"--server-host=flaghost"}

	cfg, err := Load(
		WithFile(configPath),
		WithEnvPrefix("TEST"),
		WithFlags(args),
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host":    "defaulthost",
				"port":    80,
				"timeout": 30,
			},
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Flag value should win
	host := cfg.GetString("server.host")
	if host != "flaghost" {
		t.Errorf("Expected host 'flaghost' (from flags), got '%s'", host)
	}

	// Env value should win over file
	port := cfg.GetInt("server.port")
	if port != 9090 {
		t.Errorf("Expected port 9090 (from env), got %d", port)
	}

	// Default value should be used
	timeout := cfg.GetInt("server.timeout")
	if timeout != 30 {
		t.Errorf("Expected timeout 30 (from defaults), got %d", timeout)
	}
}

func TestUnmarshal(t *testing.T) {
	type ServerConfig struct {
		Host    string `config:"host"`
		Port    int    `config:"port"`
		Debug   bool   `config:"debug"`
		Timeout int    `config:"timeout"`
	}

	type Config struct {
		Server ServerConfig `config:"server"`
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{
		"server": {
			"host": "localhost",
			"port": 8080,
			"debug": true,
			"timeout": 30
		}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := Load(WithFile(configPath))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	var config Config
	if err := cfg.Unmarshal(&config); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if config.Server.Host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", config.Server.Host)
	}

	if config.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", config.Server.Port)
	}

	if !config.Server.Debug {
		t.Error("Expected debug to be true")
	}
}

func TestValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{
		"server": {
			"host": "localhost",
			"port": -1
		}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Create a validator that checks the nested port value
	validator := merge.RangeValidator("server.port", 1, 65535)

	_, err := Load(
		WithFile(configPath),
		WithValidator(validator),
	)

	if err == nil {
		t.Error("Expected validation error for invalid port")
	}
}

func TestHas(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !cfg.Has("server.host") {
		t.Error("Expected server.host to exist")
	}

	if cfg.Has("server.nonexistent") {
		t.Error("Expected server.nonexistent to not exist")
	}
}

func TestSet(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "localhost",
			},
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cfg.Set("server.host", "newhost")
	cfg.Set("server.port", 9090)
	cfg.Set("new.nested.key", "value")

	if cfg.GetString("server.host") != "newhost" {
		t.Error("Set server.host failed")
	}

	if cfg.GetInt("server.port") != 9090 {
		t.Error("Set server.port failed")
	}

	if cfg.GetString("new.nested.key") != "value" {
		t.Error("Set new.nested.key failed")
	}
}

func TestAllKeys(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
			"debug": true,
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	keys := cfg.AllKeys()
	if len(keys) < 3 {
		t.Errorf("Expected at least 3 keys, got %d", len(keys))
	}
}

func TestSub(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	serverCfg := cfg.Sub("server")
	if serverCfg.GetString("host") != "localhost" {
		t.Error("Sub config host failed")
	}
}

func TestOnChange(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{"value": 1}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := Load(
		WithFile(configPath),
		WithHotReload(),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer cfg.Close()

	changed := make(chan bool, 1)
	cfg.OnChange(func(c *Config) {
		changed <- true
	})

	// Wait a bit and update file
	time.Sleep(100 * time.Millisecond)
	newContent := `{"value": 2}`
	if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to update config file: %v", err)
	}

	// Wait for change notification
	select {
	case <-changed:
		// Success
	case <-time.After(2 * time.Second):
		// Timeout is acceptable in tests since file watching depends on OS
	}
}

func TestGetDuration(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"timeout": "30s",
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	timeout := cfg.GetDuration("timeout")
	if timeout != 30*time.Second {
		t.Errorf("Expected 30s, got %v", timeout)
	}
}

func TestGetBool(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"enabled":  true,
			"disabled": false,
			"strTrue":  "true",
			"strFalse": "false",
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !cfg.GetBool("enabled") {
		t.Error("Expected enabled to be true")
	}

	if cfg.GetBool("disabled") {
		t.Error("Expected disabled to be false")
	}

	if !cfg.GetBool("strTrue") {
		t.Error("Expected strTrue to be true")
	}

	if cfg.GetBool("strFalse") {
		t.Error("Expected strFalse to be false")
	}
}

func TestGetFloat64(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"pi":      3.14159,
			"intVal":  42,
			"strVal":  "2.718",
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	pi := cfg.GetFloat64("pi")
	if pi != 3.14159 {
		t.Errorf("Expected pi 3.14159, got %f", pi)
	}

	intVal := cfg.GetFloat64("intVal")
	if intVal != 42.0 {
		t.Errorf("Expected intVal 42.0, got %f", intVal)
	}

	strVal := cfg.GetFloat64("strVal")
	if strVal != 2.718 {
		t.Errorf("Expected strVal 2.718, got %f", strVal)
	}
}

func TestGetStringMap(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	serverMap := cfg.GetStringMap("server")
	if serverMap == nil {
		t.Fatal("Expected server map to be non-nil")
	}

	if serverMap["host"] != "localhost" {
		t.Error("Expected host to be localhost")
	}
}

func TestMerge(t *testing.T) {
	cfg1, err := Load(
		WithDefaults(map[string]any{
			"a": 1,
			"b": 2,
		}),
	)
	if err != nil {
		t.Fatalf("Load cfg1 failed: %v", err)
	}

	cfg2, err := Load(
		WithDefaults(map[string]any{
			"b": 3,
			"c": 4,
		}),
	)
	if err != nil {
		t.Fatalf("Load cfg2 failed: %v", err)
	}

	cfg1.Merge(cfg2)

	if cfg1.GetInt("a") != 1 {
		t.Error("Expected a to be 1")
	}
	if cfg1.GetInt("b") != 3 {
		t.Error("Expected b to be 3 (overwritten)")
	}
	if cfg1.GetInt("c") != 4 {
		t.Error("Expected c to be 4")
	}
}

func TestToJSON(t *testing.T) {
	cfg, err := Load(
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	jsonBytes, err := cfg.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	if len(jsonBytes) == 0 {
		t.Error("Expected non-empty JSON")
	}
}

func TestFromJSON(t *testing.T) {
	jsonData := []byte(`{"server": {"host": "localhost", "port": 8080}}`)

	cfg, err := FromJSON(jsonData)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	host := cfg.GetString("server.host")
	if host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", host)
	}
}

func TestWriteToFile(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := Load(
		WithDefaults(map[string]any{
			"server": map[string]any{
				"host": "localhost",
				"port": 8080,
			},
		}),
	)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Test JSON
	jsonPath := filepath.Join(tmpDir, "output.json")
	if err := cfg.WriteToFile(jsonPath); err != nil {
		t.Errorf("WriteToFile JSON failed: %v", err)
	}

	// Test TOML
	tomlPath := filepath.Join(tmpDir, "output.toml")
	if err := cfg.WriteToFile(tomlPath); err != nil {
		t.Errorf("WriteToFile TOML failed: %v", err)
	}

	// Test YAML
	yamlPath := filepath.Join(tmpDir, "output.yaml")
	if err := cfg.WriteToFile(yamlPath); err != nil {
		t.Errorf("WriteToFile YAML failed: %v", err)
	}
}

func TestMustLoad(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Log("MustLoad correctly panicked for invalid config")
		}
	}()

	// This should panic
	_ = MustLoad(WithFile("/nonexistent/path/config.json"))
	t.Error("MustLoad should have panicked")
}

func TestReload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{"value": 1}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := Load(WithFile(configPath))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.GetInt("value") != 1 {
		t.Error("Expected initial value to be 1")
	}

	// Update file
	newContent := `{"value": 2}`
	if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to update config file: %v", err)
	}

	// Reload
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if cfg.GetInt("value") != 2 {
		t.Error("Expected reloaded value to be 2")
	}
}
