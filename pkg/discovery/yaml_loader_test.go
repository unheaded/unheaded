// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadServiceConfig tests loading a single service config file.
func TestLoadServiceConfig(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantErr   bool
		errSubstr string
		validate  func(t *testing.T, cfg *ServiceConfig)
	}{
		{
			name: "valid_grpc_service",
			yaml: `
service:
  name: wotan
  display_name: "Wotan"
  description: "Memory model service"
  version: "0.2.1"
network:
  port: 18001
  http_port: 18101
  protocol: grpc
  health_endpoint: /health
  metrics_endpoint: /metrics
deployment:
  tier: infrastructure
  replicas: 2
  restart_policy: always
  depends_on:
    - service-discovery
runtime:
  binary: wotan
  args: ["--config", "/opt/unheaded/wotan/config.yaml"]
  env:
    LOG_LEVEL: info
    WOTAN_RING_SIZE: "1024"
health:
  check_interval: 10s
  timeout: 5s
  retries: 3
labels:
  kingdom_tier: gnostic
  armor_piece: wotan
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *ServiceConfig) {
				if cfg.Service.Name != "wotan" {
					t.Errorf("expected name 'wotan', got %q", cfg.Service.Name)
				}
				if cfg.Network.Port != 18001 {
					t.Errorf("expected port 18001, got %d", cfg.Network.Port)
				}
				if cfg.Deployment.Replicas != 2 {
					t.Errorf("expected 2 replicas, got %d", cfg.Deployment.Replicas)
				}
				if len(cfg.Runtime.Env) != 2 {
					t.Errorf("expected 2 env vars, got %d", len(cfg.Runtime.Env))
				}
			},
		},
		{
			name: "valid_http_service",
			yaml: `
service:
  name: dashboard-backend
network:
  port: 17200
  protocol: http
deployment:
  tier: control
runtime:
  binary: dashboard-backend
health:
  check_interval: 5s
  timeout: 3s
  retries: 2
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *ServiceConfig) {
				if cfg.Network.Protocol != "http" {
					t.Errorf("expected protocol 'http', got %q", cfg.Network.Protocol)
				}
				if cfg.Deployment.Tier != "control" {
					t.Errorf("expected tier 'control', got %q", cfg.Deployment.Tier)
				}
			},
		},
		{
			name: "missing_service_name",
			yaml: `
network:
  port: 18001
deployment:
  tier: infrastructure
runtime:
  binary: test
`,
			wantErr:   true,
			errSubstr: "service.name is required",
		},
		{
			name: "missing_network_port",
			yaml: `
service:
  name: test-service
deployment:
  tier: infrastructure
runtime:
  binary: test
`,
			wantErr:   true,
			errSubstr: "network.port is required",
		},
		{
			name: "missing_deployment_tier",
			yaml: `
service:
  name: test-service
network:
  port: 18001
runtime:
  binary: test
`,
			wantErr:   true,
			errSubstr: "deployment.tier is required",
		},
		{
			name: "missing_runtime_binary",
			yaml: `
service:
  name: test-service
network:
  port: 18001
deployment:
  tier: infrastructure
`,
			wantErr:   true,
			errSubstr: "runtime.binary is required",
		},
		{
			name: "invalid_service_name",
			yaml: `
service:
  name: "Invalid-Service-Name"
network:
  port: 18001
deployment:
  tier: infrastructure
runtime:
  binary: test
`,
			wantErr:   true,
			errSubstr: "service.name",
		},
		{
			name: "port_out_of_range",
			yaml: `
service:
  name: test-service
network:
  port: 100
deployment:
  tier: infrastructure
runtime:
  binary: test
`,
			wantErr:   true,
			errSubstr: "network.port must be in range",
		},
		{
			name: "port_outside_tier_range",
			yaml: `
service:
  name: test-service
network:
  port: 20001
deployment:
  tier: control
runtime:
  binary: test
`,
			wantErr:   true,
			errSubstr: "outside the valid range",
		},
		{
			name: "invalid_tier",
			yaml: `
service:
  name: test-service
network:
  port: 18001
deployment:
  tier: invalid-tier
runtime:
  binary: test
`,
			wantErr:   true,
			errSubstr: "deployment.tier",
		},
		{
			name: "invalid_protocol",
			yaml: `
service:
  name: test-service
network:
  port: 18001
  protocol: websocket
deployment:
  tier: infrastructure
runtime:
  binary: test
`,
			wantErr:   true,
			errSubstr: "network.protocol",
		},
		{
			name: "invalid_restart_policy",
			yaml: `
service:
  name: test-service
network:
  port: 18001
deployment:
  tier: infrastructure
  restart_policy: maybe
runtime:
  binary: test
`,
			wantErr:   true,
			errSubstr: "restart_policy",
		},
		{
			name: "invalid_kingdom_tier",
			yaml: `
service:
  name: test-service
network:
  port: 18001
deployment:
  tier: infrastructure
runtime:
  binary: test
labels:
  kingdom_tier: invalid
`,
			wantErr:   true,
			errSubstr: "kingdom_tier",
		},
		{
			name: "defaults_applied",
			yaml: `
service:
  name: test-service
network:
  port: 18001
deployment:
  tier: infrastructure
runtime:
  binary: test
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *ServiceConfig) {
				if cfg.Service.DisplayName == "" {
					t.Error("display_name should have default value")
				}
				if cfg.Network.HTTPPort != 18101 {
					t.Errorf("expected default http_port 18101, got %d", cfg.Network.HTTPPort)
				}
				if cfg.Network.Protocol != "grpc" {
					t.Errorf("expected default protocol 'grpc', got %q", cfg.Network.Protocol)
				}
				if cfg.Deployment.Replicas != 1 {
					t.Errorf("expected default replicas 1, got %d", cfg.Deployment.Replicas)
				}
				if cfg.Deployment.RestartPolicy != "always" {
					t.Errorf("expected default restart_policy 'always', got %q", cfg.Deployment.RestartPolicy)
				}
				if cfg.Health.CheckInterval != "10s" {
					t.Errorf("expected default check_interval '10s', got %q", cfg.Health.CheckInterval)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory and file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			if err := os.WriteFile(configPath, []byte(tt.yaml), 0644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			// Load the config
			cfg, err := LoadServiceConfig(configPath)

			// Check error expectations
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error=%v, got error=%v: %v", tt.wantErr, err != nil, err)
			}

			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got %v", tt.errSubstr, err)
				}
			}

			// Run custom validation
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

// TestLoadServiceDirectory tests loading all configs from a directory.
func TestLoadServiceDirectory(t *testing.T) {
	tests := []struct {
		name       string
		setupFunc  func(t *testing.T, dir string)
		wantCount  int
		wantErr    bool
		errSubstr  string
	}{
		{
			name: "empty_directory",
			setupFunc: func(t *testing.T, dir string) {
				// Don't create any subdirectories
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "directory_with_valid_configs",
			setupFunc: func(t *testing.T, dir string) {
				services := []struct {
					name   string
					config string
				}{
					{
						name: "wotan",
						config: `
service:
  name: wotan
network:
  port: 18001
deployment:
  tier: infrastructure
runtime:
  binary: wotan
`,
					},
					{
						name: "captain",
						config: `
service:
  name: captain
network:
  port: 18002
deployment:
  tier: infrastructure
runtime:
  binary: captain
`,
					},
				}

				for _, svc := range services {
					serviceDir := filepath.Join(dir, svc.name)
					if err := os.Mkdir(serviceDir, 0755); err != nil {
						t.Fatalf("failed to create service dir: %v", err)
					}

					configPath := filepath.Join(serviceDir, "config.yaml")
					if err := os.WriteFile(configPath, []byte(svc.config), 0644); err != nil {
						t.Fatalf("failed to write config: %v", err)
					}
				}
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "directory_with_mixed_valid_invalid",
			setupFunc: func(t *testing.T, dir string) {
				// Valid service
				validDir := filepath.Join(dir, "valid-service")
				os.Mkdir(validDir, 0755)
				validConfig := `
service:
  name: valid-service
network:
  port: 18001
deployment:
  tier: infrastructure
runtime:
  binary: valid
`
				os.WriteFile(filepath.Join(validDir, "config.yaml"), []byte(validConfig), 0644)

				// Invalid service (missing required field)
				invalidDir := filepath.Join(dir, "invalid-service")
				os.Mkdir(invalidDir, 0755)
				invalidConfig := `
service:
  name: invalid-service
network:
  port: 18002
deployment:
  tier: infrastructure
`
				os.WriteFile(filepath.Join(invalidDir, "config.yaml"), []byte(invalidConfig), 0644)
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "nonexistent_directory",
			setupFunc: func(t *testing.T, dir string) {
				// Don't create anything
			},
			wantCount: 0,
			wantErr:   true,
			errSubstr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.name != "nonexistent_directory" {
				tt.setupFunc(t, tmpDir)
			} else {
				tmpDir = filepath.Join(tmpDir, "nonexistent")
			}

			configs, err := LoadServiceDirectory(tmpDir)

			if (err != nil) != tt.wantErr {
				t.Errorf("expected error=%v, got error=%v: %v", tt.wantErr, err != nil, err)
			}

			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got %v", tt.errSubstr, err)
				}
			}

			if len(configs) != tt.wantCount {
				t.Errorf("expected %d configs, got %d", tt.wantCount, len(configs))
			}
		})
	}
}

// TestValidateConfig tests each validation rule independently.
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name         string
		cfg          ServiceConfig
		skipDefaults bool
		wantErr      bool
		errSubstr    string
	}{
		{
			name: "valid_config",
			cfg: ServiceConfig{
				Service: ServiceMeta{
					Name:    "test",
					Version: "1.0.0",
				},
				Network: NetworkConfig{
					Port:     18001,
					Protocol: "grpc",
				},
				Deployment: DeploymentConfig{
					Tier:     "infrastructure",
					Replicas: 1,
				},
				Runtime: RuntimeConfig{
					Binary: "test",
				},
				Health: HealthConfig{
					CheckInterval: "10s",
					Timeout:       "5s",
					Retries:       3,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_service_name_uppercase",
			cfg: ServiceConfig{
				Service: ServiceMeta{
					Name: "Test-Service",
				},
				Network: NetworkConfig{
					Port: 18001,
				},
				Deployment: DeploymentConfig{
					Tier: "infrastructure",
				},
				Runtime: RuntimeConfig{
					Binary: "test",
				},
			},
			wantErr:   true,
			errSubstr: "service.name",
		},
		{
			name: "invalid_port_too_low",
			cfg: ServiceConfig{
				Service: ServiceMeta{
					Name: "test",
				},
				Network: NetworkConfig{
					Port: 100,
				},
				Deployment: DeploymentConfig{
					Tier: "infrastructure",
				},
				Runtime: RuntimeConfig{
					Binary: "test",
				},
			},
			wantErr:   true,
			errSubstr: "network.port",
		},
		{
			name: "invalid_port_too_high",
			cfg: ServiceConfig{
				Service: ServiceMeta{
					Name: "test",
				},
				Network: NetworkConfig{
					Port: 70000,
				},
				Deployment: DeploymentConfig{
					Tier: "infrastructure",
				},
				Runtime: RuntimeConfig{
					Binary: "test",
				},
			},
			wantErr:   true,
			errSubstr: "network.port",
		},
		{
			name: "invalid_replicas_zero",
			cfg: ServiceConfig{
				Service: ServiceMeta{
					Name: "test",
				},
				Network: NetworkConfig{
					Port: 18001,
				},
				Deployment: DeploymentConfig{
					Tier:     "infrastructure",
					Replicas: 0,
				},
				Runtime: RuntimeConfig{
					Binary: "test",
				},
			},
			skipDefaults: true,
			wantErr:      true,
			errSubstr:    "replicas",
		},
		{
			name: "invalid_check_interval_duration",
			cfg: ServiceConfig{
				Service: ServiceMeta{
					Name: "test",
				},
				Network: NetworkConfig{
					Port: 18001,
				},
				Deployment: DeploymentConfig{
					Tier: "infrastructure",
				},
				Runtime: RuntimeConfig{
					Binary: "test",
				},
				Health: HealthConfig{
					CheckInterval: "invalid-duration",
				},
			},
			wantErr:   true,
			errSubstr: "check_interval",
		},
		{
			name: "invalid_timeout_duration",
			cfg: ServiceConfig{
				Service: ServiceMeta{
					Name: "test",
				},
				Network: NetworkConfig{
					Port: 18001,
				},
				Deployment: DeploymentConfig{
					Tier: "infrastructure",
				},
				Runtime: RuntimeConfig{
					Binary: "test",
				},
				Health: HealthConfig{
					Timeout: "bad-timeout",
				},
			},
			wantErr:   true,
			errSubstr: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply defaults unless explicitly skipped (e.g., to test that
			// the validator rejects zero-values that applyDefaults would mask)
			if !tt.skipDefaults {
				applyDefaults(&tt.cfg)
			}

			err := ValidateConfig(&tt.cfg)

			if (err != nil) != tt.wantErr {
				t.Errorf("expected error=%v, got error=%v: %v", tt.wantErr, err != nil, err)
			}

			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got %v", tt.errSubstr, err)
				}
			}
		})
	}
}

// TestLoadBundledManifest tests loading a manifest file.
func TestLoadBundledManifest(t *testing.T) {
	tests := []struct {
		name      string
		manifest  string
		wantCount int
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid_manifest",
			manifest: `
services:
  - service:
      name: wotan
    network:
      port: 18001
    deployment:
      tier: infrastructure
    runtime:
      binary: wotan
  - service:
      name: captain
    network:
      port: 18002
    deployment:
      tier: infrastructure
    runtime:
      binary: captain
`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "manifest_with_invalid_service",
			manifest: `
services:
  - service:
      name: wotan
    network:
      port: 18001
    deployment:
      tier: infrastructure
    runtime:
      binary: wotan
  - service:
      name: bad-service
    network:
      port: 18002
    deployment:
      tier: invalid-tier
    runtime:
      binary: bad
`,
			wantCount: 0,
			wantErr:   true,
			errSubstr: "invalid-tier",
		},
		{
			name:      "empty_manifest",
			manifest:  "services: []",
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := LoadBundledManifest([]byte(tt.manifest))

			if (err != nil) != tt.wantErr {
				t.Errorf("expected error=%v, got error=%v: %v", tt.wantErr, err != nil, err)
			}

			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got %v", tt.errSubstr, err)
				}
			}

			if len(configs) != tt.wantCount {
				t.Errorf("expected %d configs, got %d", tt.wantCount, len(configs))
			}
		})
	}
}

// TestHealthCheckDefaults tests that health check defaults are applied correctly.
func TestHealthCheckDefaults(t *testing.T) {
	cfg := ServiceConfig{
		Service: ServiceMeta{
			Name: "test",
		},
		Network: NetworkConfig{
			Port: 18001,
		},
		Deployment: DeploymentConfig{
			Tier: "infrastructure",
		},
		Runtime: RuntimeConfig{
			Binary: "test",
		},
		Health: HealthConfig{},
	}

	applyDefaults(&cfg)

	if cfg.Health.CheckInterval != "10s" {
		t.Errorf("expected default CheckInterval '10s', got %q", cfg.Health.CheckInterval)
	}

	if cfg.Health.Timeout != "5s" {
		t.Errorf("expected default Timeout '5s', got %q", cfg.Health.Timeout)
	}

	if cfg.Health.Retries != 3 {
		t.Errorf("expected default Retries 3, got %d", cfg.Health.Retries)
	}
}

// TestServiceStatus tests the ServiceStatus type.
func TestServiceStatus(t *testing.T) {
	status := ServiceStatus{
		Name:                "test-service",
		Healthy:             true,
		LastCheck:           time.Now(),
		Latency:             100 * time.Millisecond,
		Message:             "OK",
		ConsecutiveFailures: 0,
	}

	if status.Name != "test-service" {
		t.Errorf("expected name 'test-service', got %q", status.Name)
	}

	if !status.Healthy {
		t.Error("expected service to be healthy")
	}

	if status.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 consecutive failures, got %d", status.ConsecutiveFailures)
	}
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
