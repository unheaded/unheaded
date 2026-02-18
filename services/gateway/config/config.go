// Package config provides configuration management for the API Gateway.
package config

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Config holds all gateway configuration.
type Config struct {
	Server        ServerConfig        `json:"server"`
	Routes        []RouteConfig       `json:"routes"`
	RateLimit     RateLimitConfig     `json:"rate_limit"`
	Auth          AuthConfig          `json:"auth"`
	CORS          CORSConfig          `json:"cors"`
	Circuit       CircuitConfig       `json:"circuit"`
	Wotan        WotanConfig        `json:"wotan"`
	Metrics       MetricsConfig       `json:"metrics"`
	AlphaServices AlphaServicesConfig `json:"alpha_services"`
}

// AlphaServicesConfig holds configuration for Alpha platform services.
type AlphaServicesConfig struct {
	TimeGuruHost     string `json:"timeguru_host"`
	CaptainHost      string `json:"captain_host"`
	ArchitectHost    string `json:"architect_host"`
	MicromanagerHost string `json:"micromanager_host"`
	MonadHost        string `json:"monad_host"`
	SophiaHost       string `json:"sophia_host"`
}

// ServerConfig holds server-specific settings.
type ServerConfig struct {
	HTTPPort         int           `json:"http_port"`
	HTTP3Port        int           `json:"http3_port"`
	ReadTimeout      time.Duration `json:"read_timeout"`
	WriteTimeout     time.Duration `json:"write_timeout"`
	IdleTimeout      time.Duration `json:"idle_timeout"`
	ShutdownTimeout  time.Duration `json:"shutdown_timeout"`
	MaxHeaderBytes   int           `json:"max_header_bytes"`
	EnableHTTP3      bool          `json:"enable_http3"`
	TLSCertFile      string        `json:"tls_cert_file"`
	TLSKeyFile       string        `json:"tls_key_file"`
}

// RouteConfig defines a route to a backend service.
type RouteConfig struct {
	Name           string        `json:"name"`
	PathPrefix     string        `json:"path_prefix"`
	StripPrefix    bool          `json:"strip_prefix"`
	Backends       []string      `json:"backends"`
	LoadBalancer   string        `json:"load_balancer"` // round_robin, random, least_conn
	HealthCheckURL string        `json:"health_check_url"`
	Timeout        time.Duration `json:"timeout"`
	RetryCount     int           `json:"retry_count"`
	RetryDelay     time.Duration `json:"retry_delay"`
	RateLimit      *RateLimitConfig `json:"rate_limit,omitempty"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	Enabled       bool          `json:"enabled"`
	RequestsPerSec float64      `json:"requests_per_sec"`
	BurstSize     int           `json:"burst_size"`
	CleanupPeriod time.Duration `json:"cleanup_period"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	Enabled        bool     `json:"enabled"`
	JWTSecret      string   `json:"jwt_secret"`
	JWTIssuer      string   `json:"jwt_issuer"`
	APIKeyHeader   string   `json:"api_key_header"`
	APIKeys        []string `json:"api_keys"`
	ExcludedPaths  []string `json:"excluded_paths"`
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	Enabled          bool     `json:"enabled"`
	AllowedOrigins   []string `json:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers"`
	ExposedHeaders   []string `json:"exposed_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
	MaxAge           int      `json:"max_age"`
}

// CircuitConfig holds circuit breaker settings.
type CircuitConfig struct {
	Enabled          bool          `json:"enabled"`
	FailureThreshold int           `json:"failure_threshold"`
	SuccessThreshold int           `json:"success_threshold"`
	Timeout          time.Duration `json:"timeout"`
	HalfOpenRequests int           `json:"half_open_requests"`
}

// WotanConfig holds event bus client settings.
type WotanConfig struct {
	Enabled     bool   `json:"enabled"`
	URL         string `json:"url"`
	Topic       string `json:"topic"`
	ServiceName string `json:"service_name"`
}

// MetricsConfig holds metrics settings.
type MetricsConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort:        8080,
			HTTP3Port:       8443,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
			MaxHeaderBytes:  1 << 20, // 1MB
			EnableHTTP3:     false,
		},
		RateLimit: RateLimitConfig{
			Enabled:        true,
			RequestsPerSec: 100,
			BurstSize:      200,
			CleanupPeriod:  time.Minute,
		},
		Auth: AuthConfig{
			Enabled:      false,
			APIKeyHeader: "X-API-Key",
		},
		CORS: CORSConfig{
			Enabled:          true,
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
			AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Request-ID"},
			ExposedHeaders:   []string{"X-Request-ID", "X-Trace-ID"},
			AllowCredentials: false,
			MaxAge:           86400,
		},
		Circuit: CircuitConfig{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 2,
			Timeout:          30 * time.Second,
			HalfOpenRequests: 3,
		},
		Wotan: WotanConfig{
			Enabled:     true,
			URL:         "http://localhost:8081",
			Topic:       "gateway.requests",
			ServiceName: "api-gateway",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		AlphaServices: AlphaServicesConfig{
			TimeGuruHost:     "timeguru:8000",
			CaptainHost:      "captain:8001",
			ArchitectHost:    "architect:8002",
			MicromanagerHost: "micromanager:8003",
			MonadHost:        "monad:8004",
			SophiaHost:       "sophia:8005",
		},
	}
}

// LoadFromFile loads configuration from a JSON file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadFromEnv loads configuration from environment variables.
func LoadFromEnv() *Config {
	cfg := DefaultConfig()

	if port := os.Getenv("GATEWAY_HTTP_PORT"); port != "" {
		var p int
		if err := json.Unmarshal([]byte(port), &p); err == nil {
			cfg.Server.HTTPPort = p
		}
	}

	if secret := os.Getenv("GATEWAY_JWT_SECRET"); secret != "" {
		cfg.Auth.JWTSecret = secret
		cfg.Auth.Enabled = true
	}

	if wotanURL := os.Getenv("WOTAN_URL"); wotanURL != "" {
		cfg.Wotan.URL = wotanURL
	}

	if tlsCert := os.Getenv("GATEWAY_TLS_CERT"); tlsCert != "" {
		cfg.Server.TLSCertFile = tlsCert
	}

	if tlsKey := os.Getenv("GATEWAY_TLS_KEY"); tlsKey != "" {
		cfg.Server.TLSKeyFile = tlsKey
	}

	// CORS configuration
	if origins := os.Getenv("GATEWAY_CORS_ORIGINS"); origins != "" {
		cfg.CORS.AllowedOrigins = splitAndTrim(origins, ",")
	}

	if corsEnabled := os.Getenv("GATEWAY_CORS_ENABLED"); corsEnabled == "false" {
		cfg.CORS.Enabled = false
	}

	// Tracing service name
	if serviceName := os.Getenv("GATEWAY_SERVICE_NAME"); serviceName != "" {
		cfg.Wotan.ServiceName = serviceName
	}

	// Alpha Services host configuration
	if host := os.Getenv("TIMEGURU_HOST"); host != "" {
		cfg.AlphaServices.TimeGuruHost = host
	}
	if host := os.Getenv("CAPTAIN_HOST"); host != "" {
		cfg.AlphaServices.CaptainHost = host
	}
	if host := os.Getenv("ARCHITECT_HOST"); host != "" {
		cfg.AlphaServices.ArchitectHost = host
	}
	if host := os.Getenv("MICROMANAGER_HOST"); host != "" {
		cfg.AlphaServices.MicromanagerHost = host
	}
	if host := os.Getenv("MONAD_HOST"); host != "" {
		cfg.AlphaServices.MonadHost = host
	}
	if host := os.Getenv("SOPHIA_HOST"); host != "" {
		cfg.AlphaServices.SophiaHost = host
	}

	return cfg
}

// splitAndTrim splits a string by delimiter and trims whitespace from each part.
func splitAndTrim(s, delim string) []string {
	parts := strings.Split(s, delim)
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Port 0 is valid - it tells the OS to assign any available port (useful for tests).
	// Only reject negative ports or ports greater than 65535.
	if c.Server.HTTPPort < 0 || c.Server.HTTPPort > 65535 {
		return &ConfigError{Field: "server.http_port", Message: "invalid port number"}
	}

	if c.Auth.Enabled && c.Auth.JWTSecret == "" && len(c.Auth.APIKeys) == 0 {
		return &ConfigError{Field: "auth", Message: "JWT secret or API keys required when auth enabled"}
	}

	return nil
}

// ConfigError represents a configuration validation error.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config error: " + e.Field + ": " + e.Message
}
