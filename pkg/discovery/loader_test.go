// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"testing"
)

func TestServiceConfigLoader_CRUD(t *testing.T) {
	loader := NewServiceConfigLoader()

	if loader.Count() != 0 {
		t.Errorf("expected 0 configs, got %d", loader.Count())
	}

	// Add a config
	cfg := &ServiceConfig{
		Service: ServiceMeta{
			Name:    "test-svc",
			Version: "1.0.0",
		},
		Network: NetworkConfig{
			Port:     19000,
			Protocol: "grpc",
		},
		Deployment: DeploymentConfig{
			Tier:     "presentation",
			Replicas: 1,
		},
		Runtime: RuntimeConfig{
			Binary: "test-svc",
		},
	}

	loader.UpdateConfig("test-svc", cfg)

	if loader.Count() != 1 {
		t.Errorf("expected 1 config, got %d", loader.Count())
	}

	got := loader.GetConfig("test-svc")
	if got == nil {
		t.Fatal("expected config, got nil")
	}
	if got.Service.Name != "test-svc" {
		t.Errorf("expected name test-svc, got %s", got.Service.Name)
	}

	// Get non-existent
	if loader.GetConfig("nope") != nil {
		t.Error("expected nil for non-existent service")
	}

	// GetAllConfigs returns copy
	all := loader.GetAllConfigs()
	if len(all) != 1 {
		t.Errorf("expected 1 in GetAllConfigs, got %d", len(all))
	}

	// Delete
	if !loader.DeleteConfig("test-svc") {
		t.Error("expected delete to return true")
	}
	if loader.DeleteConfig("test-svc") {
		t.Error("expected delete of already-deleted to return false")
	}
	if loader.Count() != 0 {
		t.Errorf("expected 0 after delete, got %d", loader.Count())
	}
}

func TestMarshalConfig(t *testing.T) {
	cfg := &ServiceConfig{
		Service: ServiceMeta{
			Name:    "marshal-test",
			Version: "0.1.0",
		},
		Network: NetworkConfig{
			Port:     19500,
			Protocol: "http",
		},
		Deployment: DeploymentConfig{
			Tier:     "presentation",
			Replicas: 1,
		},
		Runtime: RuntimeConfig{
			Binary: "marshal-test",
		},
	}

	data, err := MarshalConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalConfig failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty YAML output")
	}
}
