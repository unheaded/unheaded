// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package iac provides the Infrastructure as Code renderer framework.
//
// Unheaded maintains a single desired-state model for each managed service.
// IaC backends are interchangeable output renderers that transform this model
// into configuration artifacts for the user's preferred toolchain.
//
// Supported backends:
//   - Ansible (playbooks, roles, inventory)
//   - Terraform (HCL modules, providers)
//   - Puppet (manifests, Hiera data)
//   - Kubernetes (manifests, Helm charts)
//   - Chef (cookbooks, recipes)
//   - Salt (states, pillars)
//
// Usage:
//
//	svc := iac.ServiceConfig{Name: "timeguru", Port: 19000, ...}
//	renderer := iac.NewRenderer(iac.BackendAnsible)
//	output, err := renderer.Render(svc)
package iac

import (
	"errors"
	"fmt"
)

// Backend identifies an IaC output backend.
type Backend string

const (
	BackendAnsible    Backend = "ansible"
	BackendTerraform  Backend = "terraform"
	BackendPuppet     Backend = "puppet"
	BackendKubernetes Backend = "kubernetes"
	BackendChef       Backend = "chef"
	BackendSalt       Backend = "salt"
)

// AllBackends returns all supported backend identifiers.
func AllBackends() []Backend {
	return []Backend{
		BackendAnsible,
		BackendTerraform,
		BackendPuppet,
		BackendKubernetes,
		BackendChef,
		BackendSalt,
	}
}

// Common errors.
var (
	ErrUnknownBackend = errors.New("iac: unknown backend")
	ErrInvalidConfig  = errors.New("iac: invalid service config")
)

// ServiceConfig is the desired-state model for a managed service.
// All renderers consume this same model and produce backend-specific output.
type ServiceConfig struct {
	// Name is the service identifier (e.g. "timeguru", "wotan").
	Name string

	// Port is the primary HTTP listen port.
	Port int

	// GRPCPort is the gRPC listen port (0 if not applicable).
	GRPCPort int

	// Image is the container image reference (for K8s/Docker backends).
	Image string

	// Binary is the path to the service binary (for bare-metal backends).
	Binary string

	// Environment contains key-value environment variables.
	Environment map[string]string

	// Volumes maps host paths to container mount paths.
	Volumes map[string]string

	// Dependencies lists other services this service depends on.
	Dependencies []string

	// HealthPath is the HTTP health check endpoint (default: /health).
	HealthPath string

	// ReadyPath is the HTTP readiness endpoint (default: /ready).
	ReadyPath string

	// MetricsPath is the Prometheus metrics endpoint (default: /metrics).
	MetricsPath string

	// Replicas is the desired replica count (for K8s/orchestrated backends).
	Replicas int

	// Resources specifies resource limits.
	Resources ResourceSpec

	// Hardening contains security hardening options.
	Hardening HardeningSpec

	// Network contains network policy options.
	Network NetworkSpec
}

// ResourceSpec defines resource limits for the service.
type ResourceSpec struct {
	CPULimit    string // e.g. "500m", "2"
	MemLimit    string // e.g. "256Mi", "1Gi"
	CPURequest  string
	MemRequest  string
}

// HardeningSpec defines security hardening options.
type HardeningSpec struct {
	ReadOnlyFS    bool
	NoNewPrivs    bool
	DropAllCaps   bool
	AllowedCaps   []string // e.g. "NET_BIND_SERVICE"
	SeccompPolicy string   // e.g. "runtime/default"
	RunAsUser     int      // 0 = root (avoid), >0 = non-root
	RunAsGroup    int
}

// NetworkSpec defines network policy options.
type NetworkSpec struct {
	IngressPorts  []int    // ports accepting external traffic
	EgressAllowed []string // allowed egress destinations (CIDR or service names)
	InternalOnly  bool     // only accessible from 10.10.10.0/24
}

// Validate checks that the ServiceConfig has all required fields.
func (sc *ServiceConfig) Validate() error {
	if sc.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidConfig)
	}
	if sc.Port <= 0 || sc.Port > 65535 {
		return fmt.Errorf("%w: port must be 1-65535, got %d", ErrInvalidConfig, sc.Port)
	}
	if sc.HealthPath == "" {
		sc.HealthPath = "/health"
	}
	if sc.ReadyPath == "" {
		sc.ReadyPath = "/ready"
	}
	if sc.MetricsPath == "" {
		sc.MetricsPath = "/metrics"
	}
	if sc.Replicas <= 0 {
		sc.Replicas = 1
	}
	return nil
}

// RenderOutput contains the rendered IaC artifacts.
type RenderOutput struct {
	// Backend identifies which renderer produced this output.
	Backend Backend

	// Files maps relative file paths to their rendered content.
	// e.g. "roles/timeguru/tasks/main.yml" → YAML content
	Files map[string]string
}

// Renderer transforms a ServiceConfig into backend-specific IaC artifacts.
type Renderer interface {
	// Backend returns the backend identifier.
	Backend() Backend

	// Render produces IaC artifacts from the service config.
	Render(config ServiceConfig) (*RenderOutput, error)

	// RenderAll produces IaC artifacts for multiple services.
	RenderAll(configs []ServiceConfig) (*RenderOutput, error)
}

// NewRenderer creates a renderer for the given backend.
func NewRenderer(backend Backend) (Renderer, error) {
	switch backend {
	case BackendAnsible:
		return &AnsibleRenderer{}, nil
	case BackendTerraform:
		return &TerraformRenderer{}, nil
	case BackendPuppet:
		return &PuppetRenderer{}, nil
	case BackendKubernetes:
		return &KubernetesRenderer{}, nil
	case BackendChef:
		return &ChefRenderer{}, nil
	case BackendSalt:
		return &SaltRenderer{}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownBackend, backend)
	}
}

// DefaultServiceConfig returns a ServiceConfig with hardened defaults
// matching the Unheaded security baseline from CLAUDE.md.
func DefaultServiceConfig(name string, port int) ServiceConfig {
	return ServiceConfig{
		Name:        name,
		Port:        port,
		HealthPath:  "/health",
		ReadyPath:   "/ready",
		MetricsPath: "/metrics",
		Replicas:    1,
		Environment: map[string]string{
			"LOG_LEVEL":    "info",
			"LOG_FORMAT":   "json",
			"AUTH_ENABLED": "true",
		},
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
			IngressPorts: []int{port},
			InternalOnly: true,
		},
	}
}
