// Package lxd provides LXD orchestration for the Unheaded control plane
// REAL CLIENT IMPLEMENTATION - Production LXD REST API client
package lxd

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ============================================================================
// PROMETHEUS METRICS - OBSERVABILITY FOR THE FORGE
// ============================================================================

var (
	lxdRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unheaded_lxd_requests_total",
			Help: "Total LXD API requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	lxdRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "unheaded_lxd_request_duration_seconds",
			Help:    "LXD API request latency",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "endpoint"},
	)

	lxdConnectionErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "unheaded_lxd_connection_errors_total",
			Help: "Total LXD connection errors",
		},
	)

	lxdOperationsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "unheaded_lxd_operations_active",
			Help: "Currently active LXD operations",
		},
	)

	lxdContainersTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "unheaded_lxd_containers_total",
			Help: "Total managed LXD containers",
		},
	)

	lxdRetryAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unheaded_lxd_retry_attempts_total",
			Help: "Total LXD request retry attempts",
		},
		[]string{"endpoint"},
	)
)

// ============================================================================
// LXD API RESPONSE TYPES
// ============================================================================

// APIResponse represents the standard LXD API response envelope
type APIResponse struct {
	Type       string          `json:"type"`
	Status     string          `json:"status"`
	StatusCode int             `json:"status_code"`
	Operation  string          `json:"operation,omitempty"`
	ErrorCode  int             `json:"error_code,omitempty"`
	Error      string          `json:"error,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

// APIOperation represents an LXD async operation from the API
type APIOperation struct {
	ID          string                 `json:"id"`
	Class       string                 `json:"class"`
	Description string                 `json:"description"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Status      string                 `json:"status"`
	StatusCode  int                    `json:"status_code"`
	Resources   map[string][]string    `json:"resources"`
	Metadata    map[string]interface{} `json:"metadata"`
	MayCancel   bool                   `json:"may_cancel"`
	Err         string                 `json:"err"`
	Location    string                 `json:"location"`
}

// APIServer represents LXD server info from the API
type APIServer struct {
	APIExtensions []string               `json:"api_extensions"`
	APIStatus     string                 `json:"api_status"`
	APIVersion    string                 `json:"api_version"`
	Auth          string                 `json:"auth"`
	Public        bool                   `json:"public"`
	AuthMethods   []string               `json:"auth_methods"`
	Environment   map[string]interface{} `json:"environment"`
	Config        map[string]interface{} `json:"config"`
}

// APIContainer represents container info from the API
type APIContainer struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Status       string                 `json:"status"`
	StatusCode   int                    `json:"status_code"`
	Architecture string                 `json:"architecture"`
	CreatedAt    time.Time              `json:"created_at"`
	LastUsedAt   time.Time              `json:"last_used_at"`
	Profiles     []string               `json:"profiles"`
	Config       map[string]string      `json:"config"`
	Devices      map[string]interface{} `json:"devices"`
	Ephemeral    bool                   `json:"ephemeral"`
	Stateful     bool                   `json:"stateful"`
	Location     string                 `json:"location"`
	Type         string                 `json:"type"`
}

// APIContainerState represents container runtime state from the API
type APIContainerState struct {
	Status     string                    `json:"status"`
	StatusCode int                       `json:"status_code"`
	CPU        APICPUState               `json:"cpu"`
	Memory     APIMemoryState            `json:"memory"`
	Network    map[string]APINetState    `json:"network"`
	Disk       map[string]APIDiskState   `json:"disk"`
	Pid        int64                     `json:"pid"`
	Processes  int64                     `json:"processes"`
}

// APICPUState represents CPU usage from the API
type APICPUState struct {
	Usage int64 `json:"usage"`
}

// APIMemoryState represents memory usage from the API
type APIMemoryState struct {
	Usage         int64 `json:"usage"`
	UsagePeak     int64 `json:"usage_peak"`
	SwapUsage     int64 `json:"swap_usage"`
	SwapUsagePeak int64 `json:"swap_usage_peak"`
}

// APINetState represents network state from the API
type APINetState struct {
	Addresses []APIAddress   `json:"addresses"`
	Counters  APINetCounters `json:"counters"`
	Hwaddr    string         `json:"hwaddr"`
	Mtu       int            `json:"mtu"`
	State     string         `json:"state"`
	Type      string         `json:"type"`
}

// APIAddress represents an IP address from the API
type APIAddress struct {
	Address string `json:"address"`
	Family  string `json:"family"`
	Netmask string `json:"netmask"`
	Scope   string `json:"scope"`
}

// APINetCounters represents network counters from the API
type APINetCounters struct {
	BytesReceived          int64 `json:"bytes_received"`
	BytesSent              int64 `json:"bytes_sent"`
	PacketsReceived        int64 `json:"packets_received"`
	PacketsSent            int64 `json:"packets_sent"`
	ErrorsReceived         int64 `json:"errors_received"`
	ErrorsSent             int64 `json:"errors_sent"`
	PacketsDroppedInbound  int64 `json:"packets_dropped_inbound"`
	PacketsDroppedOutbound int64 `json:"packets_dropped_outbound"`
}

// APIDiskState represents disk usage from the API
type APIDiskState struct {
	Usage int64 `json:"usage"`
}

// APISnapshot represents a snapshot from the API
type APISnapshot struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	CreatedAt    time.Time         `json:"created_at"`
	ExpiresAt    time.Time         `json:"expires_at"`
	Stateful     bool              `json:"stateful"`
	Config       map[string]string `json:"config"`
}

// ============================================================================
// REAL LXD CLIENT - THE PRODUCTION FORGE
// ============================================================================

// RealClient implements the Client interface for actual LXD connections
type RealClient struct {
	mu         sync.RWMutex
	httpClient *http.Client
	baseURL    string
	connected  int32 // atomic
	config     ClientConfig
	logger     zerolog.Logger

	// Connection pool settings
	maxRetries     int
	retryDelay     time.Duration
	operationCache map[string]*Operation
	opCacheMu      sync.RWMutex

	// Active operations tracking
	activeOps sync.Map
}

// NewRealClient creates a new real LXD client
func NewRealClient(cfg ClientConfig) (*RealClient, error) {
	logger := log.With().Str("component", "lxd-client").Logger()

	// Set defaults
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Socket == "" && cfg.HTTPS == "" {
		cfg.Socket = "/var/lib/lxd/unix.socket"
	}

	client := &RealClient{
		config:         cfg,
		logger:         logger,
		maxRetries:     3,
		retryDelay:     time.Second,
		operationCache: make(map[string]*Operation),
	}

	return client, nil
}

// Connect establishes connection to LXD
func (c *RealClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info().Msg("connecting to LXD")

	var transport *http.Transport

	if c.config.Socket != "" {
		// Unix socket connection
		c.logger.Debug().Str("socket", c.config.Socket).Msg("using unix socket")

		transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := net.Dialer{
					Timeout:   c.config.Timeout,
					KeepAlive: 30 * time.Second,
				}
				return dialer.DialContext(ctx, "unix", c.config.Socket)
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
		}

		c.baseURL = "http://unix.socket"

	} else if c.config.HTTPS != "" {
		// HTTPS connection with TLS
		c.logger.Debug().Str("https", c.config.HTTPS).Msg("using HTTPS connection")

		tlsConfig, err := c.buildTLSConfig()
		if err != nil {
			lxdConnectionErrors.Inc()
			return fmt.Errorf("build TLS config: %w", err)
		}

		transport = &http.Transport{
			TLSClientConfig: tlsConfig,
			DialContext: (&net.Dialer{
				Timeout:   c.config.Timeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableCompression:    false,
		}

		c.baseURL = c.config.HTTPS
		if !strings.HasPrefix(c.baseURL, "https://") {
			c.baseURL = "https://" + c.baseURL
		}

	} else {
		lxdConnectionErrors.Inc()
		return fmt.Errorf("no connection method specified (socket or HTTPS)")
	}

	c.httpClient = &http.Client{
		Transport: transport,
		Timeout:   c.config.Timeout,
	}

	// Test connection
	if err := c.testConnection(ctx); err != nil {
		lxdConnectionErrors.Inc()
		return fmt.Errorf("connection test failed: %w", err)
	}

	atomic.StoreInt32(&c.connected, 1)
	c.logger.Info().Msg("successfully connected to LXD")

	return nil
}

// buildTLSConfig creates TLS configuration for HTTPS connections
func (c *RealClient) buildTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if c.config.SkipVerify {
		c.logger.Warn().Msg("TLS verification disabled - not recommended for production")
		tlsConfig.InsecureSkipVerify = true
	}

	// Load client certificate if provided
	if c.config.TLSCert != "" && c.config.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(c.config.TLSCert, c.config.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		c.logger.Debug().Str("cert", c.config.TLSCert).Msg("loaded client certificate")
	}

	// Load CA certificate if provided
	if c.config.TLSCA != "" {
		caCert, err := os.ReadFile(c.config.TLSCA)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
		c.logger.Debug().Str("ca", c.config.TLSCA).Msg("loaded CA certificate")
	}

	return tlsConfig, nil
}

// testConnection verifies the LXD connection works
func (c *RealClient) testConnection(ctx context.Context) error {
	resp, err := c.doRequest(ctx, "GET", "/1.0", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// Disconnect closes the LXD connection
func (c *RealClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}

	atomic.StoreInt32(&c.connected, 0)
	c.logger.Info().Msg("disconnected from LXD")

	return nil
}

// IsConnected returns connection status
func (c *RealClient) IsConnected() bool {
	return atomic.LoadInt32(&c.connected) == 1
}

// ============================================================================
// HTTP REQUEST HELPERS
// ============================================================================

// doRequest executes an HTTP request with retry logic
func (c *RealClient) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader

	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	return c.doRequestWithRetry(ctx, method, path, bodyReader)
}

// doRequestWithRetry executes a request with retry logic
func (c *RealClient) doRequestWithRetry(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			lxdRetryAttempts.WithLabelValues(path).Inc()
			c.logger.Debug().
				Int("attempt", attempt).
				Str("path", path).
				Msg("retrying request")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.retryDelay * time.Duration(attempt)):
			}

			// Re-create body reader if needed for retry
			if body != nil {
				if seeker, ok := body.(io.Seeker); ok {
					if _, err := seeker.Seek(0, io.SeekStart); err != nil {
						return nil, fmt.Errorf("reset body for retry: %w", err)
					}
				}
			}
		}

		resp, err := c.doSingleRequest(ctx, method, path, body)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				continue
			}
			return nil, err
		}

		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doSingleRequest executes a single HTTP request
func (c *RealClient) doSingleRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	start := time.Now()

	reqURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	// Record metrics
	status := "error"
	if err == nil {
		status = strconv.Itoa(resp.StatusCode)
	}

	lxdRequestsTotal.WithLabelValues(method, path, status).Inc()
	lxdRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())

	if err != nil {
		c.logger.Error().
			Err(err).
			Str("method", method).
			Str("path", path).
			Dur("duration", duration).
			Msg("request failed")
		return nil, fmt.Errorf("execute request: %w", err)
	}

	c.logger.Debug().
		Str("method", method).
		Str("path", path).
		Int("status", resp.StatusCode).
		Dur("duration", duration).
		Msg("request completed")

	return resp, nil
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Network errors are retryable
	if _, ok := err.(net.Error); ok {
		return true
	}

	// Context deadline exceeded is not retryable
	if err == context.DeadlineExceeded {
		return false
	}

	// Context canceled is not retryable
	if err == context.Canceled {
		return false
	}

	errStr := err.Error()
	retryablePatterns := []string{
		"connection reset",
		"connection refused",
		"no such host",
		"i/o timeout",
		"temporary failure",
		"EOF",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// parseAPIResponse parses an LXD API response
func (c *RealClient) parseAPIResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return fmt.Errorf("unmarshal API response: %w", err)
	}

	// Check for API errors
	if apiResp.Error != "" {
		return c.mapAPIError(apiResp.ErrorCode, apiResp.Error)
	}

	if apiResp.StatusCode >= 400 {
		return fmt.Errorf("LXD API error: %s (code: %d)", apiResp.Status, apiResp.StatusCode)
	}

	// Parse metadata if target is provided
	if target != nil && apiResp.Metadata != nil {
		if err := json.Unmarshal(apiResp.Metadata, target); err != nil {
			return fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return nil
}

// mapAPIError maps LXD API error codes to package errors
func (c *RealClient) mapAPIError(code int, msg string) error {
	switch code {
	case 404:
		if strings.Contains(msg, "container") || strings.Contains(msg, "instance") {
			return ErrContainerNotFound
		}
		if strings.Contains(msg, "image") {
			return ErrImageNotFound
		}
		if strings.Contains(msg, "snapshot") {
			return ErrSnapshotNotFound
		}
		return fmt.Errorf("not found: %s", msg)
	case 409:
		if strings.Contains(msg, "already exists") {
			return ErrContainerExists
		}
		return fmt.Errorf("conflict: %s", msg)
	case 400:
		return fmt.Errorf("%w: %s", ErrInvalidConfig, msg)
	default:
		return fmt.Errorf("LXD error (code %d): %s", code, msg)
	}
}

// parseOperationResponse parses an async operation response
func (c *RealClient) parseOperationResponse(resp *http.Response) (*Operation, error) {
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal API response: %w", err)
	}

	if apiResp.Error != "" {
		return nil, c.mapAPIError(apiResp.ErrorCode, apiResp.Error)
	}

	// For async operations, the operation URL is in the response
	if apiResp.Type == "async" && apiResp.Operation != "" {
		opID := filepath.Base(apiResp.Operation)

		// Fetch operation details
		return c.getOperation(context.Background(), opID)
	}

	// For sync operations, parse metadata directly
	var apiOp APIOperation
	if apiResp.Metadata != nil {
		if err := json.Unmarshal(apiResp.Metadata, &apiOp); err != nil {
			return nil, fmt.Errorf("unmarshal operation: %w", err)
		}
	}

	op := c.convertAPIOperation(&apiOp)
	lxdOperationsActive.Inc()

	return op, nil
}

// convertAPIOperation converts API operation to package Operation type
func (c *RealClient) convertAPIOperation(apiOp *APIOperation) *Operation {
	return &Operation{
		ID:          apiOp.ID,
		Type:        apiOp.Class,
		Status:      apiOp.Status,
		StatusCode:  apiOp.StatusCode,
		Resources:   apiOp.Resources,
		Metadata:    apiOp.Metadata,
		MayCancel:   apiOp.MayCancel,
		CreatedAt:   apiOp.CreatedAt,
		UpdatedAt:   apiOp.UpdatedAt,
		Description: apiOp.Description,
		Err:         apiOp.Err,
	}
}

// getOperation fetches operation details by ID
func (c *RealClient) getOperation(ctx context.Context, opID string) (*Operation, error) {
	resp, err := c.doRequest(ctx, "GET", "/1.0/operations/"+opID, nil)
	if err != nil {
		return nil, err
	}

	var apiOp APIOperation
	if err := c.parseAPIResponse(resp, &apiOp); err != nil {
		return nil, err
	}

	return c.convertAPIOperation(&apiOp), nil
}

// ============================================================================
// SERVER INFORMATION
// ============================================================================

// ServerInfo returns LXD server information
func (c *RealClient) ServerInfo(ctx context.Context) (*ServerInfo, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	resp, err := c.doRequest(ctx, "GET", "/1.0", nil)
	if err != nil {
		return nil, err
	}

	var apiServer APIServer
	if err := c.parseAPIResponse(resp, &apiServer); err != nil {
		return nil, err
	}

	// Convert environment map
	env := make(map[string]string)
	for k, v := range apiServer.Environment {
		if s, ok := v.(string); ok {
			env[k] = s
		}
	}

	return &ServerInfo{
		APIVersion:  apiServer.APIVersion,
		Auth:        apiServer.Auth,
		Public:      apiServer.Public,
		AuthMethods: apiServer.AuthMethods,
		Environment: env,
		Config:      apiServer.Config,
	}, nil
}

// ============================================================================
// CONTAINER LIFECYCLE
// ============================================================================

// CreateContainer creates a new container
func (c *RealClient) CreateContainer(ctx context.Context, cfg ContainerConfig) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	if cfg.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidConfig)
	}

	c.logger.Info().Str("name", cfg.Name).Str("image", cfg.Image).Msg("creating container")

	// Build container creation request
	reqBody := map[string]interface{}{
		"name":      cfg.Name,
		"ephemeral": cfg.Ephemeral,
		"profiles":  cfg.Profiles,
		"config":    cfg.Config,
		"source": map[string]interface{}{
			"type":  "image",
			"alias": cfg.Image,
		},
	}

	if cfg.Architecture != "" {
		reqBody["architecture"] = cfg.Architecture
	}

	// Convert devices
	if cfg.Devices != nil {
		devices := make(map[string]interface{})
		for name, dev := range cfg.Devices {
			deviceMap := map[string]interface{}{
				"type": dev.Type,
			}
			for k, v := range dev.Properties {
				deviceMap[k] = v
			}
			devices[name] = deviceMap
		}
		reqBody["devices"] = devices
	}

	resp, err := c.doRequest(ctx, "POST", "/1.0/instances", reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// DeleteContainer deletes a container
func (c *RealClient) DeleteContainer(ctx context.Context, name string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().Str("name", name).Msg("deleting container")

	resp, err := c.doRequest(ctx, "DELETE", "/1.0/instances/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// StartContainer starts a container
func (c *RealClient) StartContainer(ctx context.Context, name string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().Str("name", name).Msg("starting container")

	reqBody := map[string]interface{}{
		"action": "start",
	}

	resp, err := c.doRequest(ctx, "PUT", "/1.0/instances/"+url.PathEscape(name)+"/state", reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// StopContainer stops a container
func (c *RealClient) StopContainer(ctx context.Context, name string, force bool, timeout int) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().Str("name", name).Bool("force", force).Int("timeout", timeout).Msg("stopping container")

	reqBody := map[string]interface{}{
		"action":  "stop",
		"force":   force,
		"timeout": timeout,
	}

	resp, err := c.doRequest(ctx, "PUT", "/1.0/instances/"+url.PathEscape(name)+"/state", reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// RestartContainer restarts a container
func (c *RealClient) RestartContainer(ctx context.Context, name string, force bool, timeout int) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().Str("name", name).Bool("force", force).Int("timeout", timeout).Msg("restarting container")

	reqBody := map[string]interface{}{
		"action":  "restart",
		"force":   force,
		"timeout": timeout,
	}

	resp, err := c.doRequest(ctx, "PUT", "/1.0/instances/"+url.PathEscape(name)+"/state", reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// FreezeContainer freezes a container
func (c *RealClient) FreezeContainer(ctx context.Context, name string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().Str("name", name).Msg("freezing container")

	reqBody := map[string]interface{}{
		"action": "freeze",
	}

	resp, err := c.doRequest(ctx, "PUT", "/1.0/instances/"+url.PathEscape(name)+"/state", reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// UnfreezeContainer unfreezes a container
func (c *RealClient) UnfreezeContainer(ctx context.Context, name string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().Str("name", name).Msg("unfreezing container")

	reqBody := map[string]interface{}{
		"action": "unfreeze",
	}

	resp, err := c.doRequest(ctx, "PUT", "/1.0/instances/"+url.PathEscape(name)+"/state", reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// ============================================================================
// CONTAINER INFORMATION
// ============================================================================

// GetContainer returns container information
func (c *RealClient) GetContainer(ctx context.Context, name string) (*ContainerInfo, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	resp, err := c.doRequest(ctx, "GET", "/1.0/instances/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}

	var apiContainer APIContainer
	if err := c.parseAPIResponse(resp, &apiContainer); err != nil {
		return nil, err
	}

	return c.convertAPIContainer(&apiContainer), nil
}

// convertAPIContainer converts API container to package ContainerInfo type
func (c *RealClient) convertAPIContainer(apiC *APIContainer) *ContainerInfo {
	// Convert devices
	devices := make(map[string]Device)
	for name, devRaw := range apiC.Devices {
		if devMap, ok := devRaw.(map[string]interface{}); ok {
			dev := Device{
				Properties: make(map[string]string),
			}
			for k, v := range devMap {
				if k == "type" {
					if t, ok := v.(string); ok {
						dev.Type = t
					}
				} else {
					if s, ok := v.(string); ok {
						dev.Properties[k] = s
					}
				}
			}
			devices[name] = dev
		}
	}

	return &ContainerInfo{
		Name:         apiC.Name,
		Status:       apiC.Status,
		StatusCode:   apiC.StatusCode,
		Architecture: apiC.Architecture,
		CreatedAt:    apiC.CreatedAt,
		LastUsedAt:   apiC.LastUsedAt,
		Profiles:     apiC.Profiles,
		Config:       apiC.Config,
		Devices:      devices,
		Ephemeral:    apiC.Ephemeral,
		Stateful:     apiC.Stateful,
	}
}

// GetContainerState returns container runtime state
func (c *RealClient) GetContainerState(ctx context.Context, name string) (*ContainerState, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	resp, err := c.doRequest(ctx, "GET", "/1.0/instances/"+url.PathEscape(name)+"/state", nil)
	if err != nil {
		return nil, err
	}

	var apiState APIContainerState
	if err := c.parseAPIResponse(resp, &apiState); err != nil {
		return nil, err
	}

	return c.convertAPIContainerState(&apiState), nil
}

// convertAPIContainerState converts API state to package ContainerState type
func (c *RealClient) convertAPIContainerState(apiS *APIContainerState) *ContainerState {
	// Convert network state
	network := make(map[string]NetState)
	for iface, netRaw := range apiS.Network {
		addresses := make([]Address, len(netRaw.Addresses))
		for i, addr := range netRaw.Addresses {
			addresses[i] = Address{
				Address: addr.Address,
				Family:  addr.Family,
				Netmask: addr.Netmask,
				Scope:   addr.Scope,
			}
		}
		network[iface] = NetState{
			Addresses: addresses,
			Counters: NetCounters{
				BytesReceived:   netRaw.Counters.BytesReceived,
				BytesSent:       netRaw.Counters.BytesSent,
				PacketsReceived: netRaw.Counters.PacketsReceived,
				PacketsSent:     netRaw.Counters.PacketsSent,
				ErrorsReceived:  netRaw.Counters.ErrorsReceived,
				ErrorsSent:      netRaw.Counters.ErrorsSent,
			},
			Hwaddr: netRaw.Hwaddr,
			Mtu:    netRaw.Mtu,
			State:  netRaw.State,
			Type:   netRaw.Type,
		}
	}

	// Convert disk state
	disk := make(map[string]DiskState)
	for name, diskRaw := range apiS.Disk {
		disk[name] = DiskState{
			Usage: diskRaw.Usage,
		}
	}

	return &ContainerState{
		Status:     apiS.Status,
		StatusCode: apiS.StatusCode,
		CPU: CPUState{
			Usage: apiS.CPU.Usage,
		},
		Memory: MemoryState{
			Usage:     apiS.Memory.Usage,
			UsagePeak: apiS.Memory.UsagePeak,
			SwapUsage: apiS.Memory.SwapUsage,
		},
		Network:   network,
		Disk:      disk,
		Pid:       apiS.Pid,
		Processes: apiS.Processes,
	}
}

// ListContainers returns all containers
func (c *RealClient) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	// Get full instance details with recursion
	resp, err := c.doRequest(ctx, "GET", "/1.0/instances?recursion=1", nil)
	if err != nil {
		return nil, err
	}

	var apiContainers []APIContainer
	if err := c.parseAPIResponse(resp, &apiContainers); err != nil {
		return nil, err
	}

	containers := make([]ContainerInfo, len(apiContainers))
	for i, apiC := range apiContainers {
		containers[i] = *c.convertAPIContainer(&apiC)
	}

	lxdContainersTotal.Set(float64(len(containers)))

	return containers, nil
}

// ContainerExists checks if a container exists
func (c *RealClient) ContainerExists(ctx context.Context, name string) (bool, error) {
	if !c.IsConnected() {
		return false, ErrClientNotConnected
	}

	_, err := c.GetContainer(ctx, name)
	if err != nil {
		if err == ErrContainerNotFound {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// ============================================================================
// CONTAINER CONFIGURATION
// ============================================================================

// UpdateContainer updates container configuration
func (c *RealClient) UpdateContainer(ctx context.Context, name string, cfg ContainerConfig) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().Str("name", name).Msg("updating container")

	// Get current container
	current, err := c.GetContainer(ctx, name)
	if err != nil {
		return nil, err
	}

	// Build update request with merged config
	reqBody := map[string]interface{}{
		"profiles":    cfg.Profiles,
		"config":      cfg.Config,
		"description": "",
	}

	// Preserve architecture if not specified
	if cfg.Architecture != "" {
		reqBody["architecture"] = cfg.Architecture
	} else {
		reqBody["architecture"] = current.Architecture
	}

	// Convert devices
	if cfg.Devices != nil {
		devices := make(map[string]interface{})
		for devName, dev := range cfg.Devices {
			deviceMap := map[string]interface{}{
				"type": dev.Type,
			}
			for k, v := range dev.Properties {
				deviceMap[k] = v
			}
			devices[devName] = deviceMap
		}
		reqBody["devices"] = devices
	}

	resp, err := c.doRequest(ctx, "PUT", "/1.0/instances/"+url.PathEscape(name), reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// RenameContainer renames a container
func (c *RealClient) RenameContainer(ctx context.Context, name, newName string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().Str("name", name).Str("newName", newName).Msg("renaming container")

	reqBody := map[string]interface{}{
		"name": newName,
	}

	resp, err := c.doRequest(ctx, "POST", "/1.0/instances/"+url.PathEscape(name), reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// ============================================================================
// SNAPSHOTS
// ============================================================================

// CreateSnapshot creates a container snapshot
func (c *RealClient) CreateSnapshot(ctx context.Context, container, snapshot string, stateful bool) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().
		Str("container", container).
		Str("snapshot", snapshot).
		Bool("stateful", stateful).
		Msg("creating snapshot")

	reqBody := map[string]interface{}{
		"name":     snapshot,
		"stateful": stateful,
	}

	resp, err := c.doRequest(ctx, "POST", "/1.0/instances/"+url.PathEscape(container)+"/snapshots", reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// DeleteSnapshot deletes a snapshot
func (c *RealClient) DeleteSnapshot(ctx context.Context, container, snapshot string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().
		Str("container", container).
		Str("snapshot", snapshot).
		Msg("deleting snapshot")

	path := "/1.0/instances/" + url.PathEscape(container) + "/snapshots/" + url.PathEscape(snapshot)
	resp, err := c.doRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// RestoreSnapshot restores from a snapshot
func (c *RealClient) RestoreSnapshot(ctx context.Context, container, snapshot string) (*Operation, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Info().
		Str("container", container).
		Str("snapshot", snapshot).
		Msg("restoring snapshot")

	reqBody := map[string]interface{}{
		"restore": snapshot,
	}

	resp, err := c.doRequest(ctx, "PUT", "/1.0/instances/"+url.PathEscape(container), reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseOperationResponse(resp)
}

// ListSnapshots lists container snapshots
func (c *RealClient) ListSnapshots(ctx context.Context, container string) ([]Snapshot, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	path := "/1.0/instances/" + url.PathEscape(container) + "/snapshots?recursion=1"
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var apiSnapshots []APISnapshot
	if err := c.parseAPIResponse(resp, &apiSnapshots); err != nil {
		return nil, err
	}

	snapshots := make([]Snapshot, len(apiSnapshots))
	for i, apiS := range apiSnapshots {
		snapshots[i] = Snapshot{
			Name:      apiS.Name,
			Stateful:  apiS.Stateful,
			CreatedAt: apiS.CreatedAt,
			ExpiresAt: apiS.ExpiresAt,
			Config:    apiS.Config,
		}
	}

	return snapshots, nil
}

// ============================================================================
// EXECUTION
// ============================================================================

// ExecContainer executes a command in a container
func (c *RealClient) ExecContainer(ctx context.Context, name string, cmd []string, env map[string]string) (*ExecResult, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Debug().
		Str("container", name).
		Strs("command", cmd).
		Msg("executing command in container")

	// Build environment
	environment := make(map[string]string)
	for k, v := range env {
		environment[k] = v
	}
	// Add defaults
	if _, ok := environment["PATH"]; !ok {
		environment["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	if _, ok := environment["HOME"]; !ok {
		environment["HOME"] = "/root"
	}

	reqBody := map[string]interface{}{
		"command":              cmd,
		"environment":          environment,
		"wait-for-websocket":   false,
		"record-output":        true,
		"interactive":          false,
		"width":                80,
		"height":               24,
	}

	resp, err := c.doRequest(ctx, "POST", "/1.0/instances/"+url.PathEscape(name)+"/exec", reqBody)
	if err != nil {
		return nil, err
	}

	// Parse operation
	op, err := c.parseOperationResponse(resp)
	if err != nil {
		return nil, err
	}

	// Wait for operation to complete
	if err := c.WaitOperation(ctx, op); err != nil {
		return nil, fmt.Errorf("exec wait: %w", err)
	}

	// Get final operation status with output
	finalOp, err := c.getOperation(ctx, op.ID)
	if err != nil {
		return nil, fmt.Errorf("get exec result: %w", err)
	}

	result := &ExecResult{
		ExitCode: 0,
	}

	// Extract exit code from metadata
	if finalOp.Metadata != nil {
		if retval, ok := finalOp.Metadata["return"].(float64); ok {
			result.ExitCode = int(retval)
		}
	}

	// Get output from logs
	if finalOp.Metadata != nil {
		if output, ok := finalOp.Metadata["output"].(map[string]interface{}); ok {
			if stdout, ok := output["1"].(string); ok {
				result.Stdout = []byte(stdout)
			}
			if stderr, ok := output["2"].(string); ok {
				result.Stderr = []byte(stderr)
			}
		}
	}

	return result, nil
}

// ============================================================================
// FILE OPERATIONS
// ============================================================================

// PushFile pushes a file to a container
func (c *RealClient) PushFile(ctx context.Context, container, destPath string, content []byte, mode int) error {
	if !c.IsConnected() {
		return ErrClientNotConnected
	}

	c.logger.Debug().
		Str("container", container).
		Str("path", destPath).
		Int("size", len(content)).
		Msg("pushing file to container")

	// Build request with raw body
	path := "/1.0/instances/" + url.PathEscape(container) + "/files?path=" + url.QueryEscape(destPath)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-LXD-type", "file")
	if mode > 0 {
		req.Header.Set("X-LXD-mode", fmt.Sprintf("%04o", mode))
	}
	req.Header.Set("X-LXD-uid", "0")
	req.Header.Set("X-LXD-gid", "0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push file failed: %s", string(bodyBytes))
	}

	return nil
}

// PullFile pulls a file from a container
func (c *RealClient) PullFile(ctx context.Context, container, srcPath string) ([]byte, error) {
	if !c.IsConnected() {
		return nil, ErrClientNotConnected
	}

	c.logger.Debug().
		Str("container", container).
		Str("path", srcPath).
		Msg("pulling file from container")

	path := "/1.0/instances/" + url.PathEscape(container) + "/files?path=" + url.QueryEscape(srcPath)

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("file not found: %s", srcPath)
	}

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pull file failed: %s", string(bodyBytes))
	}

	// Check if it's a directory
	if resp.Header.Get("X-LXD-type") == "directory" {
		return nil, fmt.Errorf("path is a directory, not a file: %s", srcPath)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file content: %w", err)
	}

	return content, nil
}

// ============================================================================
// OPERATIONS MANAGEMENT
// ============================================================================

// WaitOperation waits for an async operation to complete
func (c *RealClient) WaitOperation(ctx context.Context, op *Operation) error {
	if op == nil {
		return fmt.Errorf("nil operation")
	}

	c.logger.Debug().Str("operation", op.ID).Msg("waiting for operation")

	// Track active operations
	c.activeOps.Store(op.ID, op)
	defer func() {
		c.activeOps.Delete(op.ID)
		lxdOperationsActive.Dec()
	}()

	// Use LXD's wait endpoint with timeout
	waitPath := "/1.0/operations/" + op.ID + "/wait"

	// Add timeout parameter
	timeout := c.config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	waitPath += fmt.Sprintf("?timeout=%d", int(timeout.Seconds()))

	// Create context with timeout
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := c.doRequest(waitCtx, "GET", waitPath, nil)
	if err != nil {
		if waitCtx.Err() == context.DeadlineExceeded {
			return ErrOperationTimeout
		}
		return fmt.Errorf("wait operation: %w", err)
	}

	var finalOp APIOperation
	if err := c.parseAPIResponse(resp, &finalOp); err != nil {
		return err
	}

	// Check operation status
	if finalOp.StatusCode >= 400 {
		if finalOp.Err != "" {
			return fmt.Errorf("operation failed: %s", finalOp.Err)
		}
		return fmt.Errorf("operation failed with status: %s", finalOp.Status)
	}

	c.logger.Debug().
		Str("operation", op.ID).
		Str("status", finalOp.Status).
		Msg("operation completed")

	return nil
}

// CancelOperation cancels an async operation
func (c *RealClient) CancelOperation(ctx context.Context, op *Operation) error {
	if op == nil {
		return fmt.Errorf("nil operation")
	}

	if !c.IsConnected() {
		return ErrClientNotConnected
	}

	if !op.MayCancel {
		return fmt.Errorf("operation %s cannot be cancelled", op.ID)
	}

	c.logger.Info().Str("operation", op.ID).Msg("cancelling operation")

	resp, err := c.doRequest(ctx, "DELETE", "/1.0/operations/"+op.ID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel operation failed: %s", string(bodyBytes))
	}

	return nil
}

// ============================================================================
// CONVENIENCE METHODS
// ============================================================================

// WaitForContainerState waits for a container to reach a specific state
func (c *RealClient) WaitForContainerState(ctx context.Context, name, targetState string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return ErrOperationTimeout
			}

			state, err := c.GetContainerState(ctx, name)
			if err != nil {
				return err
			}

			if strings.EqualFold(state.Status, targetState) {
				return nil
			}
		}
	}
}

// ExecuteWithOutput is a convenience method that executes a command and returns combined output
func (c *RealClient) ExecuteWithOutput(ctx context.Context, name string, cmd ...string) (string, error) {
	result, err := c.ExecContainer(ctx, name, cmd, nil)
	if err != nil {
		return "", err
	}

	output := string(result.Stdout)
	if len(result.Stderr) > 0 {
		output += "\n" + string(result.Stderr)
	}

	if result.ExitCode != 0 {
		return output, fmt.Errorf("command exited with code %d: %s", result.ExitCode, output)
	}

	return strings.TrimSpace(output), nil
}

// PushFileString is a convenience method to push a string as a file
func (c *RealClient) PushFileString(ctx context.Context, container, destPath, content string, mode int) error {
	return c.PushFile(ctx, container, destPath, []byte(content), mode)
}

// PushFileBase64 pushes a base64-encoded file to a container
func (c *RealClient) PushFileBase64(ctx context.Context, container, destPath, base64Content string, mode int) error {
	content, err := base64.StdEncoding.DecodeString(base64Content)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	return c.PushFile(ctx, container, destPath, content, mode)
}

// ============================================================================
// FACTORY FUNCTION
// ============================================================================

// NewRealClientFromConfig creates a new real LXD client from configuration
func NewRealClientFromConfig(cfg ClientConfig) (Client, error) {
	client, err := NewRealClient(cfg)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// MustNewRealClient creates a new real LXD client and panics on error
func MustNewRealClient(cfg ClientConfig) *RealClient {
	client, err := NewRealClient(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to create LXD client: %v", err))
	}
	return client
}

// ============================================================================
// HEALTH CHECK
// ============================================================================

// HealthCheck performs a health check on the LXD connection
func (c *RealClient) HealthCheck(ctx context.Context) error {
	if !c.IsConnected() {
		return ErrClientNotConnected
	}

	info, err := c.ServerInfo(ctx)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	if info.APIVersion == "" {
		return fmt.Errorf("health check failed: invalid API response")
	}

	return nil
}

// ============================================================================
// INTERFACE COMPLIANCE
// ============================================================================

// Ensure RealClient implements Client interface
var _ Client = (*RealClient)(nil)
