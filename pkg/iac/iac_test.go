// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package iac

import (
	"strings"
	"testing"
)

func testServiceConfig() ServiceConfig {
	return ServiceConfig{
		Name:     "timeguru",
		Port:     19000,
		GRPCPort: 19001,
		Image:    "unheaded/timeguru:v0.1.0",
		Binary:   "/opt/unheaded/bin/timeguru",
		Environment: map[string]string{
			"LOG_LEVEL":  "info",
			"LOG_FORMAT": "json",
		},
		Dependencies: []string{"wotan"},
		HealthPath:   "/health",
		ReadyPath:    "/ready",
		MetricsPath:  "/metrics",
		Replicas:     2,
		Resources: ResourceSpec{
			CPULimit:   "500m",
			MemLimit:   "256Mi",
			CPURequest: "100m",
			MemRequest: "64Mi",
		},
		Hardening: HardeningSpec{
			ReadOnlyFS:    true,
			NoNewPrivs:    true,
			DropAllCaps:   true,
			AllowedCaps:   []string{"NET_BIND_SERVICE"},
			SeccompPolicy: "runtime/default",
			RunAsUser:     1000,
			RunAsGroup:    1000,
		},
		Network: NetworkSpec{
			IngressPorts: []int{19000, 19001},
			InternalOnly: true,
		},
	}
}

func TestServiceConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := testServiceConfig()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		cfg := testServiceConfig()
		cfg.Name = ""
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		cfg := testServiceConfig()
		cfg.Port = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for port 0")
		}
	})

	t.Run("port too high", func(t *testing.T) {
		cfg := testServiceConfig()
		cfg.Port = 70000
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for port > 65535")
		}
	})

	t.Run("defaults filled", func(t *testing.T) {
		cfg := ServiceConfig{Name: "test", Port: 8080}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.HealthPath != "/health" {
			t.Errorf("expected /health, got %s", cfg.HealthPath)
		}
		if cfg.Replicas != 1 {
			t.Errorf("expected 1 replica, got %d", cfg.Replicas)
		}
	})
}

func TestDefaultServiceConfig(t *testing.T) {
	cfg := DefaultServiceConfig("wotan", 18000)
	if cfg.Name != "wotan" {
		t.Errorf("expected wotan, got %s", cfg.Name)
	}
	if cfg.Port != 18000 {
		t.Errorf("expected 18000, got %d", cfg.Port)
	}
	if !cfg.Hardening.ReadOnlyFS {
		t.Error("expected ReadOnlyFS true")
	}
	if cfg.Hardening.RunAsUser != 1000 {
		t.Errorf("expected RunAsUser 1000, got %d", cfg.Hardening.RunAsUser)
	}
}

func TestAllBackends(t *testing.T) {
	backends := AllBackends()
	if len(backends) != 6 {
		t.Fatalf("expected 6 backends, got %d", len(backends))
	}
}

func TestNewRenderer_AllBackends(t *testing.T) {
	for _, b := range AllBackends() {
		r, err := NewRenderer(b)
		if err != nil {
			t.Errorf("NewRenderer(%s) error: %v", b, err)
			continue
		}
		if r.Backend() != b {
			t.Errorf("expected backend %s, got %s", b, r.Backend())
		}
	}
}

func TestNewRenderer_UnknownBackend(t *testing.T) {
	_, err := NewRenderer("unknown")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestAllRenderers_Render(t *testing.T) {
	cfg := testServiceConfig()

	for _, b := range AllBackends() {
		t.Run(string(b), func(t *testing.T) {
			r, err := NewRenderer(b)
			if err != nil {
				t.Fatalf("NewRenderer: %v", err)
			}

			out, err := r.Render(cfg)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			if out.Backend != b {
				t.Errorf("expected backend %s, got %s", b, out.Backend)
			}
			if len(out.Files) == 0 {
				t.Error("expected at least one output file")
			}

			for path, content := range out.Files {
				if path == "" {
					t.Error("empty file path")
				}
				if content == "" {
					t.Errorf("empty content for %s", path)
				}
				if !strings.Contains(path, string(cfg.Name)) {
					// Most files should contain the service name in the path
					t.Logf("note: %s does not contain service name", path)
				}
			}
		})
	}
}

func TestAllRenderers_RenderAll(t *testing.T) {
	configs := []ServiceConfig{
		DefaultServiceConfig("timeguru", 19000),
		DefaultServiceConfig("wotan", 18000),
		DefaultServiceConfig("captain", 19002),
	}

	for _, b := range AllBackends() {
		t.Run(string(b), func(t *testing.T) {
			r, err := NewRenderer(b)
			if err != nil {
				t.Fatalf("NewRenderer: %v", err)
			}

			out, err := r.RenderAll(configs)
			if err != nil {
				t.Fatalf("RenderAll: %v", err)
			}

			if len(out.Files) < 3 {
				t.Errorf("expected at least 3 files for 3 services, got %d", len(out.Files))
			}
		})
	}
}

func TestAllRenderers_InvalidConfig(t *testing.T) {
	invalid := ServiceConfig{Name: "", Port: 0}

	for _, b := range AllBackends() {
		t.Run(string(b), func(t *testing.T) {
			r, err := NewRenderer(b)
			if err != nil {
				t.Fatalf("NewRenderer: %v", err)
			}

			_, err = r.Render(invalid)
			if err == nil {
				t.Error("expected error for invalid config")
			}
		})
	}
}

func TestAnsibleRenderer_ContentCheck(t *testing.T) {
	cfg := testServiceConfig()
	r := &AnsibleRenderer{}
	out, err := r.Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Check tasks file exists and contains service name
	tasksKey := "roles/timeguru/tasks/main.yml"
	tasks, ok := out.Files[tasksKey]
	if !ok {
		t.Fatalf("missing %s", tasksKey)
	}
	if !strings.Contains(tasks, "timeguru") {
		t.Error("tasks should contain service name")
	}
	if !strings.Contains(tasks, "systemd") {
		t.Error("tasks should reference systemd")
	}

	// Check systemd template
	unitKey := "roles/timeguru/templates/timeguru.service.j2"
	unit, ok := out.Files[unitKey]
	if !ok {
		t.Fatalf("missing %s", unitKey)
	}
	if !strings.Contains(unit, "NoNewPrivileges=true") {
		t.Error("systemd unit should have NoNewPrivileges")
	}
}

func TestTerraformRenderer_ContentCheck(t *testing.T) {
	cfg := testServiceConfig()
	r := &TerraformRenderer{}
	out, err := r.Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	mainKey := "modules/timeguru/main.tf"
	main, ok := out.Files[mainKey]
	if !ok {
		t.Fatalf("missing %s", mainKey)
	}
	if !strings.Contains(main, "docker_container") {
		t.Error("main.tf should contain docker_container resource")
	}
	if !strings.Contains(main, "healthcheck") {
		t.Error("main.tf should contain healthcheck")
	}

	varsKey := "modules/timeguru/variables.tf"
	if _, ok := out.Files[varsKey]; !ok {
		t.Fatalf("missing %s", varsKey)
	}
}

func TestKubernetesRenderer_ContentCheck(t *testing.T) {
	cfg := testServiceConfig()
	r := &KubernetesRenderer{}
	out, err := r.Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	deployKey := "timeguru/deployment.yaml"
	deploy, ok := out.Files[deployKey]
	if !ok {
		t.Fatalf("missing %s", deployKey)
	}
	if !strings.Contains(deploy, "kind: Deployment") {
		t.Error("should be a Deployment")
	}
	if !strings.Contains(deploy, "replicas: 2") {
		t.Error("should have 2 replicas")
	}
	if !strings.Contains(deploy, "readOnlyRootFilesystem: true") {
		t.Error("should have readOnlyRootFilesystem")
	}
	if !strings.Contains(deploy, "allowPrivilegeEscalation: false") {
		t.Error("should have allowPrivilegeEscalation: false")
	}
	if !strings.Contains(deploy, "livenessProbe") {
		t.Error("should have livenessProbe")
	}

	svcKey := "timeguru/service.yaml"
	svc, ok := out.Files[svcKey]
	if !ok {
		t.Fatalf("missing %s", svcKey)
	}
	if !strings.Contains(svc, "kind: Service") {
		t.Error("should be a Service")
	}

	npKey := "timeguru/networkpolicy.yaml"
	np, ok := out.Files[npKey]
	if !ok {
		t.Fatalf("missing %s", npKey)
	}
	if !strings.Contains(np, "kind: NetworkPolicy") {
		t.Error("should be a NetworkPolicy")
	}
}

func TestKubernetesRenderer_RenderAll_Kustomization(t *testing.T) {
	configs := []ServiceConfig{
		DefaultServiceConfig("timeguru", 19000),
		DefaultServiceConfig("wotan", 18000),
	}
	r := &KubernetesRenderer{}
	out, err := r.RenderAll(configs)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}

	kust, ok := out.Files["kustomization.yaml"]
	if !ok {
		t.Fatal("missing kustomization.yaml")
	}
	if !strings.Contains(kust, "timeguru/deployment.yaml") {
		t.Error("kustomization should reference timeguru deployment")
	}
	if !strings.Contains(kust, "namespace.yaml") {
		t.Error("kustomization should reference namespace")
	}
}

func TestSaltRenderer_ContentCheck(t *testing.T) {
	cfg := testServiceConfig()
	r := &SaltRenderer{}
	out, err := r.Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	stateKey := "salt/timeguru/init.sls"
	state, ok := out.Files[stateKey]
	if !ok {
		t.Fatalf("missing %s", stateKey)
	}
	if !strings.Contains(state, "service.running") {
		t.Error("state should contain service.running")
	}

	pillarKey := "pillar/timeguru/init.sls"
	pillar, ok := out.Files[pillarKey]
	if !ok {
		t.Fatalf("missing %s", pillarKey)
	}
	if !strings.Contains(pillar, "port: 19000") {
		t.Error("pillar should contain port")
	}
}
