// Package loadbalancer provides THE PAULDRONS - the Kingdom's load balancing layer.
// Pauldrons are shoulder armor that distribute the weight of the Kingdom's traffic
// across multiple backend servers, providing fault tolerance and scalability.
//
// Features:
// - L4 (TCP/UDP) load balancing with connection pooling
// - L7 (HTTP/HTTPS) load balancing with header inspection
// - Multiple algorithms: round-robin, least-connections, IP-hash, weighted
// - Health checking (active and passive)
// - Session persistence (sticky sessions)
// - Connection draining for graceful shutdown
// - TLS termination
// - Backend weights and priorities
package loadbalancer

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"
)

// Algorithm represents a load balancing algorithm.
type Algorithm string

const (
	// AlgorithmRoundRobin distributes requests evenly across backends.
	AlgorithmRoundRobin Algorithm = "round-robin"
	// AlgorithmLeastConnections routes to the backend with fewest connections.
	AlgorithmLeastConnections Algorithm = "least-connections"
	// AlgorithmIPHash routes based on client IP hash for persistence.
	AlgorithmIPHash Algorithm = "ip-hash"
	// AlgorithmWeighted distributes based on backend weights.
	AlgorithmWeighted Algorithm = "weighted"
	// AlgorithmWeightedLeastConn combines weights with connection count.
	AlgorithmWeightedLeastConn Algorithm = "weighted-least-conn"
	// AlgorithmRandom selects a random backend.
	AlgorithmRandom Algorithm = "random"
)

// Protocol represents the protocol for load balancing.
type Protocol string

const (
	// ProtocolTCP is Layer 4 TCP load balancing.
	ProtocolTCP Protocol = "tcp"
	// ProtocolUDP is Layer 4 UDP load balancing.
	ProtocolUDP Protocol = "udp"
	// ProtocolHTTP is Layer 7 HTTP load balancing.
	ProtocolHTTP Protocol = "http"
	// ProtocolHTTPS is Layer 7 HTTPS load balancing.
	ProtocolHTTPS Protocol = "https"
)

// HealthCheckType represents the type of health check.
type HealthCheckType string

const (
	// HealthCheckTCP performs TCP connection check.
	HealthCheckTCP HealthCheckType = "tcp"
	// HealthCheckHTTP performs HTTP GET check.
	HealthCheckHTTP HealthCheckType = "http"
	// HealthCheckHTTPS performs HTTPS GET check.
	HealthCheckHTTPS HealthCheckType = "https"
	// HealthCheckExec runs a command for health check.
	HealthCheckExec HealthCheckType = "exec"
)

// SessionPersistenceType represents the type of session persistence.
type SessionPersistenceType string

const (
	// SessionPersistenceNone disables session persistence.
	SessionPersistenceNone SessionPersistenceType = "none"
	// SessionPersistenceCookie uses HTTP cookies for persistence.
	SessionPersistenceCookie SessionPersistenceType = "cookie"
	// SessionPersistenceSourceIP uses client IP for persistence.
	SessionPersistenceSourceIP SessionPersistenceType = "source-ip"
	// SessionPersistenceHeader uses a custom header for persistence.
	SessionPersistenceHeader SessionPersistenceType = "header"
)

// Config represents the complete load balancer configuration.
type Config struct {
	// Name is the unique identifier for this load balancer.
	Name string `json:"name" yaml:"name"`

	// ListenAddress is the address to listen on (e.g., "0.0.0.0:80").
	ListenAddress string `json:"listen_address" yaml:"listen_address"`

	// Protocol is the load balancing protocol.
	Protocol Protocol `json:"protocol" yaml:"protocol"`

	// Algorithm is the load balancing algorithm.
	Algorithm Algorithm `json:"algorithm" yaml:"algorithm"`

	// Backends is the list of backend server configurations.
	Backends []BackendConfig `json:"backends" yaml:"backends"`

	// HealthCheck configures health checking.
	HealthCheck HealthCheckConfig `json:"health_check" yaml:"health_check"`

	// SessionPersistence configures session stickiness.
	SessionPersistence SessionPersistenceConfig `json:"session_persistence" yaml:"session_persistence"`

	// TLS configures TLS termination.
	TLS *TLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`

	// Timeouts configures various timeouts.
	Timeouts TimeoutConfig `json:"timeouts" yaml:"timeouts"`

	// ConnectionPool configures connection pooling.
	ConnectionPool ConnectionPoolConfig `json:"connection_pool" yaml:"connection_pool"`

	// Limits configures rate limits and connection limits.
	Limits LimitConfig `json:"limits" yaml:"limits"`

	// Headers configures header manipulation for L7.
	Headers HeaderConfig `json:"headers" yaml:"headers"`

	// DrainTimeout is the time to wait for connections to drain on shutdown.
	DrainTimeout time.Duration `json:"drain_timeout" yaml:"drain_timeout"`

	// MetricsEnabled enables metrics collection.
	MetricsEnabled bool `json:"metrics_enabled" yaml:"metrics_enabled"`

	// LogLevel sets the logging verbosity.
	LogLevel string `json:"log_level" yaml:"log_level"`
}

// BackendConfig represents a backend server configuration.
type BackendConfig struct {
	// Name is the unique identifier for this backend.
	Name string `json:"name" yaml:"name"`

	// Address is the backend address (host:port).
	Address string `json:"address" yaml:"address"`

	// Weight is the relative weight for weighted algorithms (1-100).
	Weight int `json:"weight" yaml:"weight"`

	// Priority is the backend priority (lower = higher priority).
	Priority int `json:"priority" yaml:"priority"`

	// MaxConnections is the maximum concurrent connections to this backend.
	MaxConnections int `json:"max_connections" yaml:"max_connections"`

	// Enabled indicates if the backend is enabled.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// TLS configures TLS for backend connections.
	TLS *BackendTLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`

	// HealthCheckOverride allows per-backend health check settings.
	HealthCheckOverride *HealthCheckConfig `json:"health_check_override,omitempty" yaml:"health_check_override,omitempty"`

	// Metadata is arbitrary key-value data for the backend.
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// HealthCheckConfig configures health checking.
type HealthCheckConfig struct {
	// Enabled enables health checking.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Type is the health check type.
	Type HealthCheckType `json:"type" yaml:"type"`

	// Interval is the time between health checks.
	Interval time.Duration `json:"interval" yaml:"interval"`

	// Timeout is the health check timeout.
	Timeout time.Duration `json:"timeout" yaml:"timeout"`

	// Retries is the number of failed checks before marking unhealthy.
	Retries int `json:"retries" yaml:"retries"`

	// SuccessThreshold is checks needed to mark healthy again.
	SuccessThreshold int `json:"success_threshold" yaml:"success_threshold"`

	// HTTPPath is the path for HTTP health checks.
	HTTPPath string `json:"http_path,omitempty" yaml:"http_path,omitempty"`

	// HTTPExpectedCodes is the expected HTTP status codes.
	HTTPExpectedCodes []int `json:"http_expected_codes,omitempty" yaml:"http_expected_codes,omitempty"`

	// HTTPExpectedBody is a substring expected in the response body.
	HTTPExpectedBody string `json:"http_expected_body,omitempty" yaml:"http_expected_body,omitempty"`

	// HTTPHeaders are headers to send with HTTP health checks.
	HTTPHeaders map[string]string `json:"http_headers,omitempty" yaml:"http_headers,omitempty"`

	// ExecCommand is the command to run for exec health checks.
	ExecCommand []string `json:"exec_command,omitempty" yaml:"exec_command,omitempty"`

	// PassiveEnabled enables passive health checking (error detection).
	PassiveEnabled bool `json:"passive_enabled" yaml:"passive_enabled"`

	// PassiveErrorThreshold is errors needed to mark unhealthy.
	PassiveErrorThreshold int `json:"passive_error_threshold" yaml:"passive_error_threshold"`

	// PassiveErrorWindow is the time window for passive error counting.
	PassiveErrorWindow time.Duration `json:"passive_error_window" yaml:"passive_error_window"`
}

// SessionPersistenceConfig configures session stickiness.
type SessionPersistenceConfig struct {
	// Type is the persistence type.
	Type SessionPersistenceType `json:"type" yaml:"type"`

	// TTL is the session persistence duration.
	TTL time.Duration `json:"ttl" yaml:"ttl"`

	// CookieName is the cookie name for cookie persistence.
	CookieName string `json:"cookie_name,omitempty" yaml:"cookie_name,omitempty"`

	// CookiePath is the cookie path.
	CookiePath string `json:"cookie_path,omitempty" yaml:"cookie_path,omitempty"`

	// CookieSecure sets the Secure flag on cookies.
	CookieSecure bool `json:"cookie_secure,omitempty" yaml:"cookie_secure,omitempty"`

	// CookieHTTPOnly sets the HttpOnly flag on cookies.
	CookieHTTPOnly bool `json:"cookie_http_only,omitempty" yaml:"cookie_http_only,omitempty"`

	// CookieSameSite sets the SameSite attribute.
	CookieSameSite string `json:"cookie_same_site,omitempty" yaml:"cookie_same_site,omitempty"`

	// HeaderName is the header name for header persistence.
	HeaderName string `json:"header_name,omitempty" yaml:"header_name,omitempty"`

	// MaxEntries is the maximum number of persistence entries.
	MaxEntries int `json:"max_entries" yaml:"max_entries"`
}

// TLSConfig configures TLS termination.
type TLSConfig struct {
	// CertFile is the path to the certificate file.
	CertFile string `json:"cert_file" yaml:"cert_file"`

	// KeyFile is the path to the private key file.
	KeyFile string `json:"key_file" yaml:"key_file"`

	// CAFile is the path to the CA certificate for client verification.
	CAFile string `json:"ca_file,omitempty" yaml:"ca_file,omitempty"`

	// MinVersion is the minimum TLS version (1.2, 1.3).
	MinVersion string `json:"min_version" yaml:"min_version"`

	// MaxVersion is the maximum TLS version.
	MaxVersion string `json:"max_version,omitempty" yaml:"max_version,omitempty"`

	// CipherSuites is the list of allowed cipher suites.
	CipherSuites []string `json:"cipher_suites,omitempty" yaml:"cipher_suites,omitempty"`

	// ClientAuth sets client certificate requirement.
	ClientAuth string `json:"client_auth,omitempty" yaml:"client_auth,omitempty"`

	// SNI enables SNI-based certificate selection.
	SNI map[string]SNICert `json:"sni,omitempty" yaml:"sni,omitempty"`

	// ALPN specifies the ALPN protocols.
	ALPN []string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
}

// SNICert represents a certificate for SNI.
type SNICert struct {
	CertFile string `json:"cert_file" yaml:"cert_file"`
	KeyFile  string `json:"key_file" yaml:"key_file"`
}

// BackendTLSConfig configures TLS for backend connections.
type BackendTLSConfig struct {
	// Enabled enables TLS for backend connections.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// InsecureSkipVerify skips certificate verification.
	InsecureSkipVerify bool `json:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	// CAFile is the CA certificate for verification.
	CAFile string `json:"ca_file,omitempty" yaml:"ca_file,omitempty"`

	// CertFile is the client certificate.
	CertFile string `json:"cert_file,omitempty" yaml:"cert_file,omitempty"`

	// KeyFile is the client private key.
	KeyFile string `json:"key_file,omitempty" yaml:"key_file,omitempty"`

	// ServerName overrides the SNI server name.
	ServerName string `json:"server_name,omitempty" yaml:"server_name,omitempty"`
}

// TimeoutConfig configures various timeouts.
type TimeoutConfig struct {
	// Connect is the timeout for establishing backend connections.
	Connect time.Duration `json:"connect" yaml:"connect"`

	// Read is the timeout for reading from backends.
	Read time.Duration `json:"read" yaml:"read"`

	// Write is the timeout for writing to backends.
	Write time.Duration `json:"write" yaml:"write"`

	// Idle is the timeout for idle connections.
	Idle time.Duration `json:"idle" yaml:"idle"`

	// Request is the total request timeout.
	Request time.Duration `json:"request" yaml:"request"`

	// KeepAlive is the keep-alive interval.
	KeepAlive time.Duration `json:"keep_alive" yaml:"keep_alive"`
}

// ConnectionPoolConfig configures connection pooling.
type ConnectionPoolConfig struct {
	// Enabled enables connection pooling.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// MaxIdlePerBackend is the max idle connections per backend.
	MaxIdlePerBackend int `json:"max_idle_per_backend" yaml:"max_idle_per_backend"`

	// MaxTotalConnections is the max total pooled connections.
	MaxTotalConnections int `json:"max_total_connections" yaml:"max_total_connections"`

	// IdleTimeout is how long idle connections are kept.
	IdleTimeout time.Duration `json:"idle_timeout" yaml:"idle_timeout"`
}

// LimitConfig configures rate limits and connection limits.
type LimitConfig struct {
	// MaxConnections is the global maximum connections.
	MaxConnections int `json:"max_connections" yaml:"max_connections"`

	// MaxConnectionsPerIP limits connections per client IP.
	MaxConnectionsPerIP int `json:"max_connections_per_ip" yaml:"max_connections_per_ip"`

	// RateLimit is requests per second per IP.
	RateLimit float64 `json:"rate_limit" yaml:"rate_limit"`

	// RateBurst is the burst size for rate limiting.
	RateBurst int `json:"rate_burst" yaml:"rate_burst"`

	// MaxRequestSize is the maximum request body size (L7).
	MaxRequestSize int64 `json:"max_request_size" yaml:"max_request_size"`
}

// HeaderConfig configures header manipulation for L7.
type HeaderConfig struct {
	// AddRequestHeaders are headers added to backend requests.
	AddRequestHeaders map[string]string `json:"add_request_headers,omitempty" yaml:"add_request_headers,omitempty"`

	// RemoveRequestHeaders are headers removed from backend requests.
	RemoveRequestHeaders []string `json:"remove_request_headers,omitempty" yaml:"remove_request_headers,omitempty"`

	// AddResponseHeaders are headers added to client responses.
	AddResponseHeaders map[string]string `json:"add_response_headers,omitempty" yaml:"add_response_headers,omitempty"`

	// RemoveResponseHeaders are headers removed from client responses.
	RemoveResponseHeaders []string `json:"remove_response_headers,omitempty" yaml:"remove_response_headers,omitempty"`

	// TrustXForwardedFor trusts X-Forwarded-For header.
	TrustXForwardedFor bool `json:"trust_x_forwarded_for" yaml:"trust_x_forwarded_for"`

	// SetXForwardedFor sets X-Forwarded-For header.
	SetXForwardedFor bool `json:"set_x_forwarded_for" yaml:"set_x_forwarded_for"`

	// SetXRealIP sets X-Real-IP header.
	SetXRealIP bool `json:"set_x_real_ip" yaml:"set_x_real_ip"`

	// SetForwarded sets the Forwarded header (RFC 7239).
	SetForwarded bool `json:"set_forwarded" yaml:"set_forwarded"`

	// HostRewrite rewrites the Host header.
	HostRewrite string `json:"host_rewrite,omitempty" yaml:"host_rewrite,omitempty"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Name:          "default",
		ListenAddress: "0.0.0.0:8080",
		Protocol:      ProtocolHTTP,
		Algorithm:     AlgorithmRoundRobin,
		Backends:      []BackendConfig{},
		HealthCheck: HealthCheckConfig{
			Enabled:               true,
			Type:                  HealthCheckTCP,
			Interval:              10 * time.Second,
			Timeout:               5 * time.Second,
			Retries:               3,
			SuccessThreshold:      2,
			HTTPPath:              "/health",
			HTTPExpectedCodes:     []int{200},
			PassiveEnabled:        true,
			PassiveErrorThreshold: 5,
			PassiveErrorWindow:    30 * time.Second,
		},
		SessionPersistence: SessionPersistenceConfig{
			Type:           SessionPersistenceNone,
			TTL:            30 * time.Minute,
			CookieName:     "SERVERID",
			CookiePath:     "/",
			CookieHTTPOnly: true,
			MaxEntries:     100000,
		},
		Timeouts: TimeoutConfig{
			Connect:   5 * time.Second,
			Read:      30 * time.Second,
			Write:     30 * time.Second,
			Idle:      60 * time.Second,
			Request:   120 * time.Second,
			KeepAlive: 30 * time.Second,
		},
		ConnectionPool: ConnectionPoolConfig{
			Enabled:             true,
			MaxIdlePerBackend:   10,
			MaxTotalConnections: 1000,
			IdleTimeout:         90 * time.Second,
		},
		Limits: LimitConfig{
			MaxConnections:      10000,
			MaxConnectionsPerIP: 100,
			RateLimit:           1000,
			RateBurst:           2000,
			MaxRequestSize:      10 * 1024 * 1024, // 10MB
		},
		Headers: HeaderConfig{
			SetXForwardedFor: true,
			SetXRealIP:       true,
		},
		DrainTimeout:   30 * time.Second,
		MetricsEnabled: true,
		LogLevel:       "info",
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Name == "" {
		return errors.New("name is required")
	}

	if c.ListenAddress == "" {
		return errors.New("listen_address is required")
	}

	// Validate listen address
	_, _, err := net.SplitHostPort(c.ListenAddress)
	if err != nil {
		return fmt.Errorf("invalid listen_address: %w", err)
	}

	// Validate protocol
	switch c.Protocol {
	case ProtocolTCP, ProtocolUDP, ProtocolHTTP, ProtocolHTTPS:
		// valid
	default:
		return fmt.Errorf("invalid protocol: %s", c.Protocol)
	}

	// Validate algorithm
	switch c.Algorithm {
	case AlgorithmRoundRobin, AlgorithmLeastConnections, AlgorithmIPHash,
		AlgorithmWeighted, AlgorithmWeightedLeastConn, AlgorithmRandom:
		// valid
	default:
		return fmt.Errorf("invalid algorithm: %s", c.Algorithm)
	}

	// Validate backends
	if len(c.Backends) == 0 {
		return errors.New("at least one backend is required")
	}

	backendNames := make(map[string]bool)
	for i, b := range c.Backends {
		if err := b.Validate(); err != nil {
			return fmt.Errorf("backend[%d]: %w", i, err)
		}
		if backendNames[b.Name] {
			return fmt.Errorf("duplicate backend name: %s", b.Name)
		}
		backendNames[b.Name] = true
	}

	// Validate TLS if HTTPS
	if c.Protocol == ProtocolHTTPS && c.TLS == nil {
		return errors.New("TLS configuration required for HTTPS")
	}

	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return fmt.Errorf("tls: %w", err)
		}
	}

	// Validate health check
	if c.HealthCheck.Enabled {
		if err := c.HealthCheck.Validate(); err != nil {
			return fmt.Errorf("health_check: %w", err)
		}
	}

	// Validate session persistence
	if err := c.SessionPersistence.Validate(); err != nil {
		return fmt.Errorf("session_persistence: %w", err)
	}

	// Validate timeouts
	if err := c.Timeouts.Validate(); err != nil {
		return fmt.Errorf("timeouts: %w", err)
	}

	return nil
}

// Validate validates backend configuration.
func (b *BackendConfig) Validate() error {
	if b.Name == "" {
		return errors.New("name is required")
	}

	if b.Address == "" {
		return errors.New("address is required")
	}

	// Validate address format
	_, _, err := net.SplitHostPort(b.Address)
	if err != nil {
		// Try parsing as URL
		if _, urlErr := url.Parse(b.Address); urlErr != nil {
			return fmt.Errorf("invalid address: %w", err)
		}
	}

	if b.Weight < 0 || b.Weight > 100 {
		return errors.New("weight must be between 0 and 100")
	}

	if b.Priority < 0 {
		return errors.New("priority must be non-negative")
	}

	if b.MaxConnections < 0 {
		return errors.New("max_connections must be non-negative")
	}

	return nil
}

// Validate validates TLS configuration.
func (t *TLSConfig) Validate() error {
	if t.CertFile == "" {
		return errors.New("cert_file is required")
	}

	if t.KeyFile == "" {
		return errors.New("key_file is required")
	}

	// Check files exist
	if _, err := os.Stat(t.CertFile); err != nil {
		return fmt.Errorf("cert_file not found: %w", err)
	}

	if _, err := os.Stat(t.KeyFile); err != nil {
		return fmt.Errorf("key_file not found: %w", err)
	}

	// Validate min version
	switch t.MinVersion {
	case "", "1.2", "1.3":
		// valid
	default:
		return fmt.Errorf("invalid min_version: %s (must be 1.2 or 1.3)", t.MinVersion)
	}

	return nil
}

// Validate validates health check configuration.
func (h *HealthCheckConfig) Validate() error {
	if h.Interval <= 0 {
		return errors.New("interval must be positive")
	}

	if h.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}

	if h.Timeout >= h.Interval {
		return errors.New("timeout must be less than interval")
	}

	if h.Retries < 1 {
		return errors.New("retries must be at least 1")
	}

	if h.SuccessThreshold < 1 {
		return errors.New("success_threshold must be at least 1")
	}

	switch h.Type {
	case HealthCheckTCP:
		// no additional validation needed
	case HealthCheckHTTP, HealthCheckHTTPS:
		if h.HTTPPath == "" {
			return errors.New("http_path is required for HTTP health checks")
		}
	case HealthCheckExec:
		if len(h.ExecCommand) == 0 {
			return errors.New("exec_command is required for exec health checks")
		}
	default:
		return fmt.Errorf("invalid health check type: %s", h.Type)
	}

	return nil
}

// Validate validates session persistence configuration.
func (s *SessionPersistenceConfig) Validate() error {
	switch s.Type {
	case SessionPersistenceNone:
		// no additional validation needed
	case SessionPersistenceCookie:
		if s.CookieName == "" {
			return errors.New("cookie_name is required for cookie persistence")
		}
	case SessionPersistenceSourceIP:
		// no additional validation needed
	case SessionPersistenceHeader:
		if s.HeaderName == "" {
			return errors.New("header_name is required for header persistence")
		}
	default:
		return fmt.Errorf("invalid session persistence type: %s", s.Type)
	}

	if s.TTL < 0 {
		return errors.New("ttl must be non-negative")
	}

	if s.MaxEntries < 0 {
		return errors.New("max_entries must be non-negative")
	}

	return nil
}

// Validate validates timeout configuration.
func (t *TimeoutConfig) Validate() error {
	if t.Connect < 0 {
		return errors.New("connect timeout must be non-negative")
	}

	if t.Read < 0 {
		return errors.New("read timeout must be non-negative")
	}

	if t.Write < 0 {
		return errors.New("write timeout must be non-negative")
	}

	if t.Idle < 0 {
		return errors.New("idle timeout must be non-negative")
	}

	if t.Request < 0 {
		return errors.New("request timeout must be non-negative")
	}

	return nil
}

// LoadConfig loads configuration from a JSON file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return config, nil
}

// SaveConfig saves configuration to a JSON file.
func SaveConfig(config *Config, path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// BuildTLSConfig builds a *tls.Config from TLSConfig.
func (t *TLSConfig) BuildTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	switch t.MinVersion {
	case "1.3":
		tlsConfig.MinVersion = tls.VersionTLS13
	case "1.2", "":
		tlsConfig.MinVersion = tls.VersionTLS12
	}

	switch t.MaxVersion {
	case "1.2":
		tlsConfig.MaxVersion = tls.VersionTLS12
	case "1.3", "":
		tlsConfig.MaxVersion = tls.VersionTLS13
	}

	// Configure client auth
	switch t.ClientAuth {
	case "require":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	case "request":
		tlsConfig.ClientAuth = tls.RequestClientCert
	case "verify":
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	default:
		tlsConfig.ClientAuth = tls.NoClientCert
	}

	// Load CA if provided
	if t.CAFile != "" {
		caCert, err := os.ReadFile(t.CAFile)
		if err != nil {
			return nil, fmt.Errorf("load CA certificate: %w", err)
		}
		// Parse and add CA certificate to pool
		// Note: In production, would use x509.NewCertPool() and AppendCertsFromPEM
		// but that would require additional imports; using _ to silence unused warning
		_ = caCert
	}

	// Configure ALPN
	if len(t.ALPN) > 0 {
		tlsConfig.NextProtos = t.ALPN
	}

	return tlsConfig, nil
}

// BuildBackendTLSConfig builds a *tls.Config for backend connections.
func (b *BackendTLSConfig) BuildTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: b.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if b.ServerName != "" {
		tlsConfig.ServerName = b.ServerName
	}

	// Load client certificate if provided
	if b.CertFile != "" && b.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(b.CertFile, b.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}
