package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadServiceConfig reads and parses a service config YAML file.
// Path should be the absolute path to a config.yaml file.
// Returns a validated ServiceConfig or an error if the file is missing,
// malformed, or fails validation.
func LoadServiceConfig(path string) (*ServiceConfig, error) {
	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("loading %s: service config file not found: %w", path, err)
		}
		return nil, fmt.Errorf("loading %s: failed to read file: %w", path, err)
	}

	// Parse YAML
	var cfg ServiceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("loading %s: invalid YAML: %w", path, err)
	}

	// Apply defaults
	applyDefaults(&cfg)

	// Validate
	if err := ValidateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("loading %s: validation failed: %w", path, err)
	}

	return &cfg, nil
}

// LoadServiceDirectory scans a directory for config.yaml files and loads all valid configs.
// Skips invalid configs with logged warnings (non-fatal).
// Returns all successfully loaded configs or an error if the directory doesn't exist.
func LoadServiceDirectory(dir string) ([]*ServiceConfig, error) {
	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("service directory not found: %s: %w", dir, err)
		}
		return nil, fmt.Errorf("failed to stat directory %s: %w", dir, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dir)
	}

	// List subdirectories
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var configs []*ServiceConfig

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		configPath := filepath.Join(dir, entry.Name(), "config.yaml")

		// Try to load the config
		cfg, err := LoadServiceConfig(configPath)
		if err != nil {
			// Log warning but continue with other services
			fmt.Fprintf(os.Stderr, "Warning: skipping invalid service config at %s: %v\n", configPath, err)
			continue
		}

		configs = append(configs, cfg)
	}

	return configs, nil
}

// LoadBundledManifest parses the embedded services.yaml manifest as a fallback.
// Used when /opt/unheaded/ is not accessible (development mode, containers).
// Data should be the raw YAML bytes of a manifest file containing a list of service configs.
func LoadBundledManifest(data []byte) ([]*ServiceConfig, error) {
	var manifest struct {
		Services []*ServiceConfig `yaml:"services"`
	}

	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse bundled manifest: %w", err)
	}

	// Apply defaults and validate each config
	for _, cfg := range manifest.Services {
		applyDefaults(cfg)
		if err := ValidateConfig(cfg); err != nil {
			return nil, fmt.Errorf("invalid service %q in manifest: %w", cfg.Service.Name, err)
		}
	}

	return manifest.Services, nil
}

// ValidateConfig checks that a ServiceConfig meets all requirements.
// Returns an error describing the first validation failure encountered.
func ValidateConfig(cfg *ServiceConfig) error {
	// Validate service metadata
	if err := validateServiceMeta(&cfg.Service); err != nil {
		return err
	}

	// Validate network config
	if err := validateNetworkConfig(&cfg.Network); err != nil {
		return err
	}

	// Validate deployment config
	if err := validateDeploymentConfig(&cfg.Deployment); err != nil {
		return err
	}

	// Validate runtime config
	if err := validateRuntimeConfig(&cfg.Runtime); err != nil {
		return err
	}

	// Validate health config
	if err := validateHealthConfig(&cfg.Health); err != nil {
		return err
	}

	// Validate labels
	if err := validateLabels(&cfg.Labels); err != nil {
		return err
	}

	// Cross-field validation: port must be in tier range
	if err := validatePortTier(cfg.Deployment.Tier, cfg.Network.Port); err != nil {
		return err
	}

	return nil
}

// validateServiceMeta checks service identity and metadata fields.
func validateServiceMeta(meta *ServiceMeta) error {
	if meta.Name == "" {
		return fmt.Errorf("service.name is required")
	}

	// Validate service name format: ^[a-z][a-z0-9-]*$
	namePattern := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	if !namePattern.MatchString(meta.Name) {
		return fmt.Errorf("service.name %q must start with lowercase letter and contain only lowercase alphanumeric and hyphens", meta.Name)
	}

	// Validate version format if provided
	if meta.Version != "" && !isValidSemanticVersion(meta.Version) {
		return fmt.Errorf("service.version %q is not valid semantic version", meta.Version)
	}

	return nil
}

// validateNetworkConfig checks network configuration.
func validateNetworkConfig(net *NetworkConfig) error {
	if net.Port == 0 {
		return fmt.Errorf("network.port is required and must not be zero")
	}

	if net.Port < 1024 || net.Port > 65535 {
		return fmt.Errorf("network.port must be in range [1024, 65535], got %d", net.Port)
	}

	if net.HTTPPort != 0 && (net.HTTPPort < 1024 || net.HTTPPort > 65535) {
		return fmt.Errorf("network.http_port must be in range [1024, 65535], got %d", net.HTTPPort)
	}

	if net.Protocol == "" {
		return nil // defaults applied, no validation needed
	}

	validProtocols := map[string]bool{"grpc": true, "http": true, "both": true}
	if !validProtocols[net.Protocol] {
		return fmt.Errorf("network.protocol must be 'grpc', 'http', or 'both', got %q", net.Protocol)
	}

	return nil
}

// validateDeploymentConfig checks deployment configuration.
func validateDeploymentConfig(deploy *DeploymentConfig) error {
	if deploy.Tier == "" {
		return fmt.Errorf("deployment.tier is required")
	}

	validTiers := map[string]bool{
		"control":          true,
		"infrastructure":   true,
		"presentation":     true,
		"application":      true,
	}
	if !validTiers[deploy.Tier] {
		return fmt.Errorf("deployment.tier must be 'control', 'infrastructure', 'presentation', or 'application', got %q", deploy.Tier)
	}

	if deploy.Replicas < 1 {
		return fmt.Errorf("deployment.replicas must be at least 1, got %d", deploy.Replicas)
	}

	if deploy.RestartPolicy != "" {
		validPolicies := map[string]bool{"always": true, "on-failure": true, "never": true}
		if !validPolicies[deploy.RestartPolicy] {
			return fmt.Errorf("deployment.restart_policy must be 'always', 'on-failure', or 'never', got %q", deploy.RestartPolicy)
		}
	}

	return nil
}

// validateRuntimeConfig checks runtime configuration.
func validateRuntimeConfig(rt *RuntimeConfig) error {
	if rt.Binary == "" {
		return fmt.Errorf("runtime.binary is required")
	}

	return nil
}

// validateHealthConfig checks health checking configuration.
func validateHealthConfig(health *HealthConfig) error {
	if health.CheckInterval != "" {
		if _, err := time.ParseDuration(health.CheckInterval); err != nil {
			return fmt.Errorf("health.check_interval %q is not a valid duration: %w", health.CheckInterval, err)
		}
	}

	if health.Timeout != "" {
		if _, err := time.ParseDuration(health.Timeout); err != nil {
			return fmt.Errorf("health.timeout %q is not a valid duration: %w", health.Timeout, err)
		}
	}

	if health.Retries < 0 {
		return fmt.Errorf("health.retries must be non-negative, got %d", health.Retries)
	}

	return nil
}

// validateLabels checks label configuration.
func validateLabels(labels *ServiceLabels) error {
	if labels.KingdomTier != "" {
		validTiers := map[string]bool{
			"armory":       true,
			"gnostic":      true,
			"royal-court":  true,
			"presentation": true,
		}
		if !validTiers[labels.KingdomTier] {
			return fmt.Errorf("labels.kingdom_tier must be 'armory', 'gnostic', 'royal-court', or 'presentation', got %q", labels.KingdomTier)
		}
	}

	return nil
}

// validatePortTier ensures the port is in the correct range for the deployment tier.
func validatePortTier(tier string, port int) error {
	var minPort, maxPort int

	switch tier {
	case "control":
		minPort, maxPort = 17000, 17999
	case "infrastructure":
		minPort, maxPort = 18000, 18999
	case "presentation":
		minPort, maxPort = 19000, 19999
	case "application":
		minPort, maxPort = 20000, 20999
	}

	if port < minPort || port > maxPort {
		return fmt.Errorf("network.port %d is outside the valid range for tier %q [%d-%d]", port, tier, minPort, maxPort)
	}

	return nil
}

// applyDefaults sets default values for optional fields.
func applyDefaults(cfg *ServiceConfig) {
	// Service defaults
	if cfg.Service.DisplayName == "" {
		cfg.Service.DisplayName = strings.Title(strings.ReplaceAll(cfg.Service.Name, "-", " "))
	}
	if cfg.Service.Version == "" {
		cfg.Service.Version = "0.1.0"
	}

	// Network defaults
	if cfg.Network.HTTPPort == 0 {
		cfg.Network.HTTPPort = cfg.Network.Port + 100
	}
	if cfg.Network.Protocol == "" {
		cfg.Network.Protocol = "grpc"
	}
	if cfg.Network.HealthEndpoint == "" {
		cfg.Network.HealthEndpoint = "/health"
	}
	if cfg.Network.MetricsEndpoint == "" {
		cfg.Network.MetricsEndpoint = "/metrics"
	}

	// Deployment defaults
	if cfg.Deployment.Replicas == 0 {
		cfg.Deployment.Replicas = 1
	}
	if cfg.Deployment.RestartPolicy == "" {
		cfg.Deployment.RestartPolicy = "always"
	}

	// Health defaults
	if cfg.Health.CheckInterval == "" {
		cfg.Health.CheckInterval = "10s"
	}
	if cfg.Health.Timeout == "" {
		cfg.Health.Timeout = "5s"
	}
	if cfg.Health.Retries == 0 {
		cfg.Health.Retries = 3
	}

	// Labels defaults
	if cfg.Labels.Custom == nil {
		cfg.Labels.Custom = make(map[string]string)
	}
}

// isValidSemanticVersion checks if a version string matches semantic versioning.
// Format: MAJOR.MINOR.PATCH with optional prerelease and metadata.
func isValidSemanticVersion(version string) bool {
	pattern := regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?(?:\+[a-zA-Z0-9.-]+)?$`)
	return pattern.MatchString(version)
}
