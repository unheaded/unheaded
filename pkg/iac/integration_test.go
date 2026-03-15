// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package iac

import (
	"strings"
	"testing"
)

// multiServiceConfigs returns a realistic set of Unheaded service configs
// for integration testing.
func multiServiceConfigs() []ServiceConfig {
	return []ServiceConfig{
		{
			Name:       "timeguru",
			Port:       19000,
			GRPCPort:   0,
			Image:      "unheaded/timeguru:v0.1.0",
			Binary:     "/opt/unheaded/bin/timeguru",
			HealthPath: "/health",
			ReadyPath:  "/ready",
			Replicas:   2,
			Environment: map[string]string{
				"LOG_LEVEL": "info",
			},
			Resources: ResourceSpec{CPULimit: "500m", MemLimit: "256Mi", CPURequest: "100m", MemRequest: "64Mi"},
			Hardening: HardeningSpec{ReadOnlyFS: true, NoNewPrivs: true, DropAllCaps: true, AllowedCaps: []string{"NET_BIND_SERVICE"}, SeccompPolicy: "runtime/default", RunAsUser: 1000, RunAsGroup: 1000},
		},
		{
			Name:       "wotan",
			Port:       18000,
			GRPCPort:   18001,
			Image:      "unheaded/wotan:v0.1.0",
			Binary:     "/opt/unheaded/bin/wotan",
			HealthPath: "/health",
			ReadyPath:  "/ready",
			Replicas:   3,
			Environment: map[string]string{
				"LOG_LEVEL":    "info",
				"RING_SIZE":    "4096",
				"AUTH_ENABLED": "true",
			},
			Resources: ResourceSpec{CPULimit: "1", MemLimit: "512Mi", CPURequest: "250m", MemRequest: "128Mi"},
			Hardening: HardeningSpec{ReadOnlyFS: true, NoNewPrivs: true, DropAllCaps: true, AllowedCaps: []string{"NET_BIND_SERVICE"}, SeccompPolicy: "runtime/default", RunAsUser: 1000, RunAsGroup: 1000},
		},
		{
			Name:       "captain",
			Port:       19002,
			HealthPath: "/health",
			ReadyPath:  "/ready",
			Replicas:   1,
			Resources:  ResourceSpec{CPULimit: "500m", MemLimit: "256Mi", CPURequest: "100m", MemRequest: "64Mi"},
			Hardening:  HardeningSpec{ReadOnlyFS: true, NoNewPrivs: true, DropAllCaps: true, SeccompPolicy: "runtime/default", RunAsUser: 1000, RunAsGroup: 1000},
		},
		DefaultServiceConfig("architect", 19001),
		DefaultServiceConfig("micromanager", 19003),
	}
}

// --- Task 3: Integration tests for Ansible + Terraform renderers ---

func TestAnsibleRenderer_RenderAll_MultiService(t *testing.T) {
	configs := multiServiceConfigs()
	r := &AnsibleRenderer{}
	out, err := r.RenderAll(configs)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}

	// Should have site.yml and inventory
	if _, ok := out.Files["site.yml"]; !ok {
		t.Error("missing site.yml playbook")
	}
	if _, ok := out.Files["inventory/hosts.yml"]; !ok {
		t.Error("missing inventory/hosts.yml")
	}

	// Each service should have tasks, defaults, handlers, systemd template
	for _, cfg := range configs {
		tasksKey := "roles/" + cfg.Name + "/tasks/main.yml"
		if tasks, ok := out.Files[tasksKey]; !ok {
			t.Errorf("missing %s", tasksKey)
		} else {
			if !strings.Contains(tasks, cfg.Name) {
				t.Errorf("tasks for %s should contain service name", cfg.Name)
			}
			if !strings.Contains(tasks, cfg.HealthPath) {
				t.Errorf("tasks for %s should contain health path %s", cfg.Name, cfg.HealthPath)
			}
		}
	}

	// Site playbook should reference all services
	site := out.Files["site.yml"]
	for _, cfg := range configs {
		if !strings.Contains(site, cfg.Name) {
			t.Errorf("site.yml should reference %s", cfg.Name)
		}
	}

	// Inventory should contain ports
	inv := out.Files["inventory/hosts.yml"]
	if !strings.Contains(inv, "19000") {
		t.Error("inventory should contain timeguru port 19000")
	}
	if !strings.Contains(inv, "18000") {
		t.Error("inventory should contain wotan port 18000")
	}
}

func TestTerraformRenderer_RenderAll_MultiService(t *testing.T) {
	configs := multiServiceConfigs()
	r := &TerraformRenderer{}
	out, err := r.RenderAll(configs)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}

	// Should have root main.tf and versions.tf
	if _, ok := out.Files["main.tf"]; !ok {
		t.Error("missing root main.tf")
	}
	if _, ok := out.Files["versions.tf"]; !ok {
		t.Error("missing versions.tf")
	}

	// Each service should have module files
	for _, cfg := range configs {
		mainKey := "modules/" + cfg.Name + "/main.tf"
		if main, ok := out.Files[mainKey]; !ok {
			t.Errorf("missing %s", mainKey)
		} else {
			if !strings.Contains(main, cfg.Name) {
				t.Errorf("main.tf for %s should contain service name", cfg.Name)
			}
			if !strings.Contains(main, cfg.HealthPath) {
				t.Errorf("main.tf for %s should contain health path", cfg.Name)
			}
		}

		varsKey := "modules/" + cfg.Name + "/variables.tf"
		if _, ok := out.Files[varsKey]; !ok {
			t.Errorf("missing %s", varsKey)
		}
	}

	// Root main.tf should reference all modules
	root := out.Files["main.tf"]
	for _, cfg := range configs {
		if !strings.Contains(root, cfg.Name) {
			t.Errorf("root main.tf should reference module %s", cfg.Name)
		}
	}

	// Wotan should have gRPC port in root
	if !strings.Contains(root, "18001") {
		t.Error("root main.tf should contain wotan gRPC port 18001")
	}
}

func TestTerraformRenderer_RenderOutput_PortInHealthcheck(t *testing.T) {
	cfg := DefaultServiceConfig("monad", 19004)
	cfg.GRPCPort = 19005
	r := &TerraformRenderer{}
	out, err := r.Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	main := out.Files["modules/monad/main.tf"]
	if !strings.Contains(main, "19004") {
		t.Error("healthcheck should reference correct port")
	}
	if !strings.Contains(main, "grpc_port") {
		t.Error("should have gRPC port variable")
	}
}

// --- Validate method tests ---

func TestAllRenderers_Validate(t *testing.T) {
	cfg := testServiceConfig()

	for _, b := range AllBackends() {
		t.Run(string(b), func(t *testing.T) {
			r, err := NewRenderer(b)
			if err != nil {
				t.Fatalf("NewRenderer: %v", err)
			}

			if err := r.Validate(cfg); err != nil {
				t.Errorf("Validate should pass for valid config: %v", err)
			}
		})
	}
}

func TestAllRenderers_Validate_InvalidConfig(t *testing.T) {
	invalid := ServiceConfig{Name: "", Port: 0}

	for _, b := range AllBackends() {
		t.Run(string(b), func(t *testing.T) {
			r, err := NewRenderer(b)
			if err != nil {
				t.Fatalf("NewRenderer: %v", err)
			}

			if err := r.Validate(invalid); err == nil {
				t.Error("Validate should fail for invalid config")
			}
		})
	}
}

func TestAnsibleRenderer_Validate_ContentCheck(t *testing.T) {
	cfg := DefaultServiceConfig("sophia", 19005)
	r := &AnsibleRenderer{}

	if err := r.Validate(cfg); err != nil {
		t.Errorf("Validate sophia: %v", err)
	}
}

func TestTerraformRenderer_Validate_ContentCheck(t *testing.T) {
	cfg := DefaultServiceConfig("dashboard-backend", 16667)
	r := &TerraformRenderer{}

	if err := r.Validate(cfg); err != nil {
		t.Errorf("Validate dashboard-backend: %v", err)
	}
}

// --- Diff method tests ---

func TestAllRenderers_Diff(t *testing.T) {
	current := DefaultServiceConfig("timeguru", 19000)
	desired := DefaultServiceConfig("timeguru", 19000)
	desired.Replicas = 3
	desired.Resources.CPULimit = "1"

	for _, b := range AllBackends() {
		t.Run(string(b), func(t *testing.T) {
			r, err := NewRenderer(b)
			if err != nil {
				t.Fatalf("NewRenderer: %v", err)
			}

			diff, err := r.Diff(current, desired)
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}

			if diff == "" {
				t.Error("expected non-empty diff for changed replicas and CPU")
			}
			if !strings.Contains(diff, "replicas") {
				t.Error("diff should mention replicas change")
			}
			if !strings.Contains(diff, "cpu_limit") {
				t.Error("diff should mention CPU limit change")
			}
		})
	}
}

func TestAllRenderers_Diff_NoDifference(t *testing.T) {
	cfg := DefaultServiceConfig("wotan", 18000)

	for _, b := range AllBackends() {
		t.Run(string(b), func(t *testing.T) {
			r, err := NewRenderer(b)
			if err != nil {
				t.Fatalf("NewRenderer: %v", err)
			}

			diff, err := r.Diff(cfg, cfg)
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}

			if diff != "" {
				t.Errorf("expected empty diff for identical configs, got: %s", diff)
			}
		})
	}
}

func TestAllRenderers_Diff_InvalidConfig(t *testing.T) {
	valid := DefaultServiceConfig("test", 8080)
	invalid := ServiceConfig{Name: "", Port: 0}

	for _, b := range AllBackends() {
		t.Run(string(b), func(t *testing.T) {
			r, err := NewRenderer(b)
			if err != nil {
				t.Fatalf("NewRenderer: %v", err)
			}

			_, err = r.Diff(invalid, valid)
			if err == nil {
				t.Error("Diff should fail with invalid current config")
			}

			_, err = r.Diff(valid, invalid)
			if err == nil {
				t.Error("Diff should fail with invalid desired config")
			}
		})
	}
}

// --- Factory pattern tests ---

func TestNewRenderer_FactoryReturnsCorrectType(t *testing.T) {
	tests := []struct {
		backend  Backend
		typeName string
	}{
		{BackendAnsible, "ansible"},
		{BackendTerraform, "terraform"},
		{BackendPuppet, "puppet"},
		{BackendKubernetes, "kubernetes"},
		{BackendChef, "chef"},
		{BackendSalt, "salt"},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			r, err := NewRenderer(tt.backend)
			if err != nil {
				t.Fatalf("NewRenderer(%s): %v", tt.backend, err)
			}
			if r.Backend() != tt.backend {
				t.Errorf("expected Backend() == %s, got %s", tt.backend, r.Backend())
			}

			// Ensure Render, RenderAll, Validate, Diff all work
			cfg := DefaultServiceConfig("test-svc", 9999)
			if _, err := r.Render(cfg); err != nil {
				t.Errorf("Render failed: %v", err)
			}
			if _, err := r.RenderAll([]ServiceConfig{cfg}); err != nil {
				t.Errorf("RenderAll failed: %v", err)
			}
			if err := r.Validate(cfg); err != nil {
				t.Errorf("Validate failed: %v", err)
			}
			if _, err := r.Diff(cfg, cfg); err != nil {
				t.Errorf("Diff failed: %v", err)
			}
		})
	}
}

// --- ValidateRenderedOutput tests ---

func TestValidateRenderedOutput_NilOutput(t *testing.T) {
	if err := ValidateRenderedOutput(nil, ServiceConfig{Name: "test"}); err == nil {
		t.Error("expected error for nil output")
	}
}

func TestValidateRenderedOutput_EmptyFiles(t *testing.T) {
	out := &RenderOutput{Backend: BackendAnsible, Files: map[string]string{}}
	if err := ValidateRenderedOutput(out, ServiceConfig{Name: "test"}); err == nil {
		t.Error("expected error for empty files")
	}
}

func TestValidateRenderedOutput_EmptyContent(t *testing.T) {
	out := &RenderOutput{
		Backend: BackendAnsible,
		Files:   map[string]string{"test.yml": "   "},
	}
	if err := ValidateRenderedOutput(out, ServiceConfig{Name: "test"}); err == nil {
		t.Error("expected error for whitespace-only content")
	}
}

// --- DiffConfigs tests ---

func TestDiffConfigs_EnvironmentChanges(t *testing.T) {
	current := DefaultServiceConfig("test", 8080)
	desired := DefaultServiceConfig("test", 8080)
	desired.Environment["NEW_VAR"] = "new_value"
	delete(desired.Environment, "AUTH_ENABLED")

	diff := DiffConfigs(current, desired)
	if diff == "" {
		t.Error("expected non-empty diff for env changes")
	}
	if !strings.Contains(diff, "NEW_VAR") {
		t.Error("diff should mention added NEW_VAR")
	}
	if !strings.Contains(diff, "AUTH_ENABLED") {
		t.Error("diff should mention removed AUTH_ENABLED")
	}
}

func TestDiffConfigs_PortChange(t *testing.T) {
	current := DefaultServiceConfig("test", 8080)
	desired := DefaultServiceConfig("test", 9090)

	diff := DiffConfigs(current, desired)
	if !strings.Contains(diff, "8080") || !strings.Contains(diff, "9090") {
		t.Errorf("diff should show port change, got: %s", diff)
	}
}
