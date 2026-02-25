// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStaticConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "services.yaml")
	content := `services:
  wotan:
    host: localhost
    port: 18001
    proto: grpc
  timeguru:
    host: localhost
    port: 19000
    proto: http
`
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := LoadStaticConfig(path)
	if err != nil {
		t.Fatalf("LoadStaticConfig: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Errorf("got %d services, want 2", len(cfg.Services))
	}
	if cfg.Services["wotan"].Port != 18001 {
		t.Errorf("wotan port = %d, want 18001", cfg.Services["wotan"].Port)
	}
}

func TestLoadStaticConfig_FileNotFound(t *testing.T) {
	_, err := LoadStaticConfig("/nonexistent/services.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadStaticConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "services.yaml")
	os.WriteFile(path, []byte("{{not yaml"), 0644)

	_, err := LoadStaticConfig(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestStaticConfig_Resolve(t *testing.T) {
	cfg := &StaticConfig{
		Services: map[string]StaticEntry{
			"timeguru": {Host: "localhost", Port: 19000, Proto: "http"},
		},
	}

	entry, err := cfg.Resolve("timeguru")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if entry.Port != 19000 {
		t.Errorf("port = %d, want 19000", entry.Port)
	}
	if entry.Address != "localhost:19000" {
		t.Errorf("address = %q, want %q", entry.Address, "localhost:19000")
	}
}

func TestStaticConfig_ResolveNotFound(t *testing.T) {
	cfg := &StaticConfig{Services: map[string]StaticEntry{}}

	_, err := cfg.Resolve("nonexistent")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}
}

func TestStaticConfig_ResolveAll(t *testing.T) {
	cfg := &StaticConfig{
		Services: map[string]StaticEntry{
			"svc-a": {Host: "localhost", Port: 19000, Proto: "http"},
			"svc-b": {Host: "localhost", Port: 19001, Proto: "grpc"},
		},
	}

	all := cfg.ResolveAll()
	if len(all) != 2 {
		t.Errorf("got %d services, want 2", len(all))
	}
}
