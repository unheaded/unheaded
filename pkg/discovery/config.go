// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"time"
)

// ServiceConfig represents the YAML configuration for a single Unheaded service.
// It is loaded from /opt/unheaded/<service>/config.yaml and defines how the service
// is deployed, monitored, and managed within the Unheaded platform.
type ServiceConfig struct {
	Service    ServiceMeta       `yaml:"service" json:"service"`
	Network    NetworkConfig     `yaml:"network" json:"network"`
	Deployment DeploymentConfig  `yaml:"deployment" json:"deployment"`
	Runtime    RuntimeConfig     `yaml:"runtime" json:"runtime"`
	Health     HealthConfig      `yaml:"health" json:"health"`
	Labels     ServiceLabels     `yaml:"labels" json:"labels"`
}

// ServiceMeta contains service identity and metadata.
type ServiceMeta struct {
	// Name is the unique service identifier (lowercase, alphanumeric with hyphens).
	// Used as directory name and in service discovery.
	// Must match regex: ^[a-z][a-z0-9-]*$
	Name string `yaml:"name" json:"name" validate:"required,hostname_rfc952"`

	// DisplayName is the human-readable service name for UI display.
	// Defaults to Name with title casing if not provided.
	DisplayName string `yaml:"display_name" json:"display_name"`

	// Description is a brief explanation of the service's purpose and responsibilities.
	Description string `yaml:"description" json:"description"`

	// Version is the semantic version of this service's API and behavior.
	// Defaults to "0.1.0" if not provided.
	Version string `yaml:"version" json:"version"`
}

// NetworkConfig defines service networking, ports, and communication protocols.
type NetworkConfig struct {
	// Port is the primary service port.
	// For gRPC services, this is the gRPC port.
	// For HTTP services, this is the HTTP port.
	// Must be in the appropriate port tier range (17000-17999 for control, etc.)
	// Must be in range [1024, 65535].
	Port int `yaml:"port" json:"port" validate:"required,min=1024,max=65535"`

	// HTTPPort is the HTTP fallback or secondary port.
	// Used for health checks, metrics, and HTTP bridges.
	// Defaults to Port + 100 if not specified.
	HTTPPort int `yaml:"http_port" json:"http_port" validate:"omitempty,min=1024,max=65535"`

	// Protocol is the communication protocol used by this service.
	// Must be one of: "grpc", "http", or "both".
	// Defaults to "grpc" if not specified.
	Protocol string `yaml:"protocol" json:"protocol" validate:"omitempty,oneof=grpc http both"`

	// HealthEndpoint is the HTTP endpoint path for health checks.
	// Responds with 200 OK if healthy.
	// Defaults to "/health" if not specified.
	HealthEndpoint string `yaml:"health_endpoint" json:"health_endpoint"`

	// MetricsEndpoint is the HTTP endpoint path for Prometheus-format metrics.
	// Must be accessible via HTTPPort.
	// Defaults to "/metrics" if not specified.
	MetricsEndpoint string `yaml:"metrics_endpoint" json:"metrics_endpoint"`
}

// DeploymentConfig configures how the service is deployed and managed.
type DeploymentConfig struct {
	// Tier is the deployment tier, determining resource constraints and scheduling.
	// Must be one of: "control", "infrastructure", "presentation", or "application".
	// - "control": Control plane services (port 17000–17999), single instance, critical.
	// - "infrastructure": Core services (port 18000–18999), highly available, mesh-integrated.
	// - "presentation": Frontend services (port 19000–19999), multiple replicas, stateless.
	// - "application": User workloads (port 20000–20999), customer applications.
	Tier string `yaml:"tier" json:"tier" validate:"required,oneof=control infrastructure presentation application"`

	// Replicas is the desired number of running instances.
	// Control plane services typically use 1 or 3 (for HA).
	// Defaults to 1 if not specified.
	Replicas int `yaml:"replicas" json:"replicas" validate:"omitempty,min=1,max=100"`

	// RestartPolicy is the restart behavior when the service process exits.
	// Must be one of: "always", "on-failure", or "never".
	// - "always": Automatically restart on any exit (recommended for production).
	// - "on-failure": Restart only on non-zero exit code.
	// - "never": No automatic restart.
	// Defaults to "always" if not specified.
	RestartPolicy string `yaml:"restart_policy" json:"restart_policy" validate:"omitempty,oneof=always on-failure never"`

	// DependsOn is a list of service names that must be healthy before this service starts.
	// Service discovery ensures dependency ordering during startup.
	DependsOn []string `yaml:"depends_on" json:"depends_on"`
}

// RuntimeConfig specifies how the service binary is executed.
type RuntimeConfig struct {
	// Binary is the name or absolute path to the executable.
	// Resolved in PATH or as an absolute path during service startup.
	// Example: "wotan" or "/usr/local/bin/wotan"
	Binary string `yaml:"binary" json:"binary" validate:"required"`

	// Args is a list of command-line arguments passed to the binary at startup.
	// Empty list if not specified.
	Args []string `yaml:"args" json:"args"`

	// Env is a map of environment variables set for the process.
	// Values are merged with system environment; service env vars override system vars of the same name.
	Env map[string]string `yaml:"env" json:"env"`
}

// HealthConfig configures service health checking and monitoring.
type HealthConfig struct {
	// CheckInterval is the time between health checks.
	// Go duration format (e.g., "10s", "1m", "500ms").
	// Defaults to "10s" if not specified.
	CheckInterval string `yaml:"check_interval" json:"check_interval"`

	// Timeout is the maximum time to wait for a health check response.
	// Go duration format (e.g., "5s", "100ms").
	// Defaults to "5s" if not specified.
	Timeout string `yaml:"timeout" json:"timeout"`

	// Retries is the number of consecutive failed checks before marking the service as unhealthy.
	// Defaults to 3 if not specified.
	Retries int `yaml:"retries" json:"retries" validate:"omitempty,min=1,max=10"`
}

// ServiceLabels contains arbitrary key-value labels for categorization and monitoring.
// Labels can be used for filtering, grouping, and custom metadata.
type ServiceLabels struct {
	// KingdomTier categorizes the service using Unheaded's armor metaphor.
	// Must be one of: "armory", "gnostic", "royal-court", or "presentation" if specified.
	// - "armory": Core runtime services (binary, build, package management).
	// - "gnostic": Knowledge and state services (discovery, metrics, logging, observability).
	// - "royal-court": Command and coordination services (orchestration, scheduling, networking).
	// - "presentation": User-facing and application services (dashboard, APIs, load balancers).
	KingdomTier string `yaml:"kingdom_tier" json:"kingdom_tier" validate:"omitempty,oneof=armory gnostic royal-court presentation"`

	// ArmorPiece identifies which armor piece (service) this represents.
	// Example: "wotan", "helm", "boots", "gloves".
	ArmorPiece string `yaml:"armor_piece" json:"armor_piece"`

	// Custom labels beyond the standard ones can be added and will be preserved.
	// Map[string]string for extensibility.
	Custom map[string]string `yaml:"custom" json:"custom"`
}

// ServiceStatus represents the runtime health status of a service.
// This is populated by the discovery service during periodic health checks.
type ServiceStatus struct {
	// Name is the service identifier (matches ServiceConfig.Service.Name).
	Name string `json:"name"`

	// Healthy indicates whether the service is currently healthy.
	// Determined by consecutive successful health checks vs. Retries threshold.
	Healthy bool `json:"healthy"`

	// LastCheck is the timestamp of the most recent health check.
	LastCheck time.Time `json:"last_check"`

	// Latency is the round-trip time of the most recent health check.
	// Measured from request start to response received.
	Latency time.Duration `json:"latency"`

	// Message is an optional status message (e.g., error reason if unhealthy).
	Message string `json:"message"`

	// ConsecutiveFailures is the number of consecutive failed health checks.
	// Exposed for debugging and monitoring.
	ConsecutiveFailures int `json:"consecutive_failures"`
}

// ServiceInfo combines configuration and runtime status for a single service.
// Returned by the service discovery API.
type ServiceInfo struct {
	// Config is the static configuration for this service.
	Config ServiceConfig `json:"config"`

	// Status is the current runtime health and status.
	Status ServiceStatus `json:"status"`
}

// ServiceList is the API response for GET /api/v1/services.
// Provides a summary of all services and their health.
type ServiceList struct {
	// Services is a list of all discovered services with their config and status.
	Services []ServiceInfo `json:"services"`

	// Total is the total number of services discovered.
	Total int `json:"total"`

	// Healthy is the number of services currently in healthy state.
	Healthy int `json:"healthy"`

	// Unhealthy is the number of services currently in unhealthy state.
	Unhealthy int `json:"unhealthy"`

	// UpdatedAt is the timestamp when this list was last refreshed.
	UpdatedAt time.Time `json:"updated_at"`
}

// ServiceMetrics contains metrics for monitoring service health at aggregate level.
type ServiceMetrics struct {
	// TotalServices is the total count of configured services.
	TotalServices int `json:"total_services"`

	// HealthyServices is the count of services in healthy state.
	HealthyServices int `json:"healthy_services"`

	// UnhealthyServices is the count of services in unhealthy state.
	UnhealthyServices int `json:"unhealthy_services"`

	// AverageLatency is the average health check latency across all services.
	AverageLatency time.Duration `json:"average_latency"`

	// TotalRestarts is the aggregate count of service restarts across the platform.
	TotalRestarts int64 `json:"total_restarts"`

	// LastUpdated is when these metrics were computed.
	LastUpdated time.Time `json:"last_updated"`
}
