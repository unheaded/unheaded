// Package config provides configuration management for the Cuirass control plane
// THE ARMOR'S DESIGN - How the Core Heart is forged
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrConfigNotFound   = errors.New("configuration file not found")
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrMissingRequired  = errors.New("missing required configuration")
)

// ============================================================================
// CONFIG TYPES - THE ARMOR'S BLUEPRINT
// ============================================================================

// Config represents the complete daemon configuration
type Config struct {
	// Identity
	NodeID   string `json:"node_id"`
	NodeName string `json:"node_name"`
	Region   string `json:"region"`
	Zone     string `json:"zone"`

	// Server
	Server ServerConfig `json:"server"`

	// LXD
	LXD LXDConfig `json:"lxd"`

	// eBPF
	EBPF EBPFConfig `json:"ebpf"`

	// State
	State StateConfig `json:"state"`

	// Busboy (Message Bus)
	Busboy BusboyConfig `json:"busboy"`

	// Logging
	Logging LoggingConfig `json:"logging"`

	// Security
	Security SecurityConfig `json:"security"`

	// Metrics
	Metrics MetricsConfig `json:"metrics"`
}

// ServerConfig holds HTTP/gRPC server settings
type ServerConfig struct {
	HTTPAddr     string        `json:"http_addr"`
	GRPCAddr     string        `json:"grpc_addr"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
	TLSCert      string        `json:"tls_cert"`
	TLSKey       string        `json:"tls_key"`
	TLSCA        string        `json:"tls_ca"`
}

// LXDConfig holds LXD client settings
type LXDConfig struct {
	Socket      string        `json:"socket"`
	HTTPSAddr   string        `json:"https_addr"`
	TLSCert     string        `json:"tls_cert"`
	TLSKey      string        `json:"tls_key"`
	TLSCA       string        `json:"tls_ca"`
	Timeout     time.Duration `json:"timeout"`
	SkipVerify  bool          `json:"skip_verify"`
	DefaultPool string        `json:"default_pool"`
	DefaultNet  string        `json:"default_network"`
}

// EBPFConfig holds eBPF loader settings
type EBPFConfig struct {
	Enabled    bool     `json:"enabled"`
	BTFPath    string   `json:"btf_path"`
	PinPath    string   `json:"pin_path"`
	Programs   []string `json:"programs"`
	AutoAttach bool     `json:"auto_attach"`
}

// StateConfig holds state management settings
type StateConfig struct {
	PollInterval      time.Duration `json:"poll_interval"`
	DriftCheckInterval time.Duration `json:"drift_check_interval"`
	SnapshotInterval  time.Duration `json:"snapshot_interval"`
	SnapshotPath      string        `json:"snapshot_path"`
	MaxDriftReports   int           `json:"max_drift_reports"`
}

// BusboyConfig holds message bus settings
type BusboyConfig struct {
	Addr         string        `json:"addr"`
	Topics       []string      `json:"topics"`
	BatchSize    int           `json:"batch_size"`
	FlushTimeout time.Duration `json:"flush_timeout"`
	TLSEnabled   bool          `json:"tls_enabled"`
	TLSCert      string        `json:"tls_cert"`
	TLSKey       string        `json:"tls_key"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level      string `json:"level"`      // debug, info, warn, error
	Format     string `json:"format"`     // json, text
	Output     string `json:"output"`     // stdout, stderr, file path
	MaxSize    int    `json:"max_size"`   // Max size in MB before rotation
	MaxBackups int    `json:"max_backups"`
	MaxAge     int    `json:"max_age"`    // Days
}

// SecurityConfig holds security settings
type SecurityConfig struct {
	// mTLS
	MTLSEnabled bool   `json:"mtls_enabled"`
	CertPath    string `json:"cert_path"`
	KeyPath     string `json:"key_path"`
	CAPath      string `json:"ca_path"`

	// Seccomp
	SeccompEnabled bool   `json:"seccomp_enabled"`
	SeccompProfile string `json:"seccomp_profile"`

	// Capabilities
	DropCaps []string `json:"drop_caps"`
	AddCaps  []string `json:"add_caps"`

	// The Sacred Law: Zero customer data access
	// This is architectural, not policy
	DataIsolation DataIsolationConfig `json:"data_isolation"`
}

// DataIsolationConfig enforces The Sacred Law
type DataIsolationConfig struct {
	// Separate namespaces for customer vs operator data
	CustomerDataNamespace string `json:"customer_data_namespace"`
	OperatorDataNamespace string `json:"operator_data_namespace"`

	// Encryption at rest keys (separate for customer data)
	CustomerKeyPath  string `json:"customer_key_path"`
	OperatorKeyPath  string `json:"operator_key_path"`

	// Audit logging for all data access
	AuditEnabled bool   `json:"audit_enabled"`
	AuditPath    string `json:"audit_path"`

	// PII Containment — GDPR/ePrivacy compliance
	// "PII is radioactive." — hnbad (https://news.ycombinator.com/user?id=hnbad)
	PIIContainmentEnabled bool   `json:"pii_containment_enabled"`
	PIIMode               string `json:"pii_mode"` // off, eu, strict
}

// MetricsConfig holds metrics/observability settings
type MetricsConfig struct {
	Enabled        bool          `json:"enabled"`
	PrometheusAddr string        `json:"prometheus_addr"`
	PushGateway    string        `json:"push_gateway"`
	PushInterval   time.Duration `json:"push_interval"`
	Labels         map[string]string `json:"labels"`
}

// ============================================================================
// DEFAULT CONFIGURATION
// ============================================================================

// DefaultConfig returns sensible defaults for the daemon
func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	return &Config{
		NodeID:   generateNodeID(),
		NodeName: hostname,
		Region:   "default",
		Zone:     "default",

		Server: ServerConfig{
			HTTPAddr:     ":8080",
			GRPCAddr:     ":9090",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},

		LXD: LXDConfig{
			Socket:      "/var/lib/lxd/unix.socket",
			Timeout:     30 * time.Second,
			DefaultPool: "default",
			DefaultNet:  "lxdbr0",
		},

		EBPF: EBPFConfig{
			Enabled:    true,
			BTFPath:    "/sys/kernel/btf/vmlinux",
			PinPath:    "/sys/fs/bpf/unheaded",
			Programs:   []string{"packet_marker", "flow_tracker", "latency_probe"},
			AutoAttach: true,
		},

		State: StateConfig{
			PollInterval:       30 * time.Second,
			DriftCheckInterval: 60 * time.Second,
			SnapshotInterval:   5 * time.Minute,
			SnapshotPath:       "/var/lib/unheaded/state",
			MaxDriftReports:    1000,
		},

		Busboy: BusboyConfig{
			Addr:         "localhost:5555",
			Topics:       []string{"cuirass.events", "cuirass.drift", "cuirass.metrics"},
			BatchSize:    100,
			FlushTimeout: time.Second,
		},

		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			Output:     "stdout",
			MaxSize:    100,
			MaxBackups: 3,
			MaxAge:     7,
		},

		Security: SecurityConfig{
			MTLSEnabled:    true,
			SeccompEnabled: true,
			DropCaps: []string{
				"CAP_NET_RAW", "CAP_MKNOD", "CAP_AUDIT_WRITE",
				"CAP_SETFCAP", "CAP_MAC_OVERRIDE", "CAP_MAC_ADMIN",
			},
			AddCaps: []string{
				"CAP_SYS_ADMIN", // Needed for eBPF
				"CAP_BPF",       // BPF operations
				"CAP_NET_ADMIN", // Network management
			},
			DataIsolation: DataIsolationConfig{
				CustomerDataNamespace: "customer",
				OperatorDataNamespace: "operator",
				AuditEnabled:          true,
				AuditPath:             "/var/log/unheaded/audit.log",
			},
		},

		Metrics: MetricsConfig{
			Enabled:        true,
			PrometheusAddr: ":9100",
			PushInterval:   10 * time.Second,
			Labels: map[string]string{
				"service": "cuirass",
				"tier":    "control_plane",
			},
		},
	}
}

func generateNodeID() string {
	// Use hostname + timestamp for uniqueness
	hostname, _ := os.Hostname()
	return fmt.Sprintf("citadel-%s-%d", hostname, time.Now().Unix()%10000)
}

// ============================================================================
// LOADING AND VALIDATION
// ============================================================================

// Load loads configuration from a file
func Load(path string) (*Config, error) {
	if path == "" {
		return nil, ErrConfigNotFound
	}

	// Expand home directory
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[1:])
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, path)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	cfg := DefaultConfig()

	// Override with environment variables
	if v := os.Getenv("UNHEADED_NODE_ID"); v != "" {
		cfg.NodeID = v
	}
	if v := os.Getenv("UNHEADED_NODE_NAME"); v != "" {
		cfg.NodeName = v
	}
	if v := os.Getenv("UNHEADED_REGION"); v != "" {
		cfg.Region = v
	}
	if v := os.Getenv("UNHEADED_ZONE"); v != "" {
		cfg.Zone = v
	}

	// Server
	if v := os.Getenv("UNHEADED_HTTP_ADDR"); v != "" {
		cfg.Server.HTTPAddr = v
	}
	if v := os.Getenv("UNHEADED_GRPC_ADDR"); v != "" {
		cfg.Server.GRPCAddr = v
	}

	// LXD
	if v := os.Getenv("LXD_SOCKET"); v != "" {
		cfg.LXD.Socket = v
	}

	// Busboy
	if v := os.Getenv("BUSBOY_ADDR"); v != "" {
		cfg.Busboy.Addr = v
	}

	// Logging
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}

	// eBPF
	if v := os.Getenv("EBPF_ENABLED"); v == "false" {
		cfg.EBPF.Enabled = false
	}

	// PII containment
	if v := os.Getenv("UNHEADED_PII_MODE"); v != "" {
		cfg.Security.DataIsolation.PIIContainmentEnabled = v != "off"
		cfg.Security.DataIsolation.PIIMode = v
	}

	return cfg
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c == nil {
		return ErrInvalidConfig
	}

	// Required fields
	if c.NodeID == "" {
		return fmt.Errorf("%w: node_id", ErrMissingRequired)
	}

	// Server validation
	if c.Server.HTTPAddr == "" && c.Server.GRPCAddr == "" {
		return fmt.Errorf("%w: at least one of http_addr or grpc_addr required", ErrMissingRequired)
	}

	// Validate timeouts are positive
	if c.Server.ReadTimeout < 0 || c.Server.WriteTimeout < 0 || c.Server.IdleTimeout < 0 {
		return fmt.Errorf("%w: timeouts must be positive", ErrInvalidConfig)
	}

	// Validate log level
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("%w: invalid log level %q", ErrInvalidConfig, c.Logging.Level)
	}

	// Validate log format
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("%w: invalid log format %q", ErrInvalidConfig, c.Logging.Format)
	}

	// Validate PII mode if containment is enabled
	if c.Security.DataIsolation.PIIContainmentEnabled {
		validPIIModes := map[string]bool{"eu": true, "strict": true}
		if !validPIIModes[c.Security.DataIsolation.PIIMode] {
			return fmt.Errorf("%w: invalid pii_mode %q (must be eu or strict)", ErrInvalidConfig, c.Security.DataIsolation.PIIMode)
		}
	}

	return nil
}

// Save saves configuration to a file
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// ============================================================================
// ENVIRONMENT HELPERS
// ============================================================================

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return os.Getenv("UNHEADED_ENV") == "development"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	env := os.Getenv("UNHEADED_ENV")
	return env == "production" || env == ""
}

// GetEnv returns the current environment name
func (c *Config) GetEnv() string {
	env := os.Getenv("UNHEADED_ENV")
	if env == "" {
		return "production"
	}
	return env
}
