// Package health provides service health monitoring and uptime tracking.
// It polls /health endpoints from all Kingdom services and maintains
// uptime history for the dashboard.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"unheaded/pkg/logger"
)

var (
	// ErrNilConfig indicates nil configuration
	ErrNilConfig = errors.New("config cannot be nil")
	// ErrInvalidInterval indicates invalid poll interval
	ErrInvalidInterval = errors.New("poll interval must be > 0")
	// ErrServiceNotFound indicates service not found
	ErrServiceNotFound = errors.New("service not found")
	// ErrMonitorStopped indicates monitor has been stopped
	ErrMonitorStopped = errors.New("monitor has been stopped")
)

// Status represents health status
type Status string

const (
	// StatusHealthy indicates service is healthy
	StatusHealthy Status = "healthy"
	// StatusDegraded indicates service is degraded
	StatusDegraded Status = "degraded"
	// StatusUnhealthy indicates service is unhealthy
	StatusUnhealthy Status = "unhealthy"
	// StatusUnknown indicates unknown status
	StatusUnknown Status = "unknown"
)

// Config holds health monitor configuration
type Config struct {
	// PollInterval is how often to poll health endpoints
	PollInterval time.Duration
	// PollTimeout is timeout for each health check request
	PollTimeout time.Duration
	// RetentionPeriod is how long to keep health history
	RetentionPeriod time.Duration
	// MaxHistory is maximum history entries per service
	MaxHistory int
	// FailureThreshold is consecutive failures before unhealthy
	FailureThreshold int
	// RecoveryThreshold is consecutive successes before healthy
	RecoveryThreshold int
}

// DefaultConfig returns default health monitor configuration
func DefaultConfig() *Config {
	return &Config{
		PollInterval:      15 * time.Second,
		PollTimeout:       5 * time.Second,
		RetentionPeriod:   24 * time.Hour,
		MaxHistory:        1000,
		FailureThreshold:  3,
		RecoveryThreshold: 2,
	}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.PollInterval <= 0 {
		return ErrInvalidInterval
	}
	if c.PollTimeout == 0 {
		c.PollTimeout = 5 * time.Second
	}
	if c.RetentionPeriod == 0 {
		c.RetentionPeriod = 24 * time.Hour
	}
	if c.MaxHistory == 0 {
		c.MaxHistory = 1000
	}
	if c.FailureThreshold == 0 {
		c.FailureThreshold = 3
	}
	if c.RecoveryThreshold == 0 {
		c.RecoveryThreshold = 2
	}
	return nil
}

// ServiceTarget represents a service to monitor
type ServiceTarget struct {
	// Name is the service name (e.g., "timeguru", "captain")
	Name string
	// HealthURL is the full URL to the /health endpoint
	HealthURL string
	// ReadyURL is the full URL to the /ready endpoint
	ReadyURL string
	// Tags for categorizing services
	Tags []string
	// Enabled indicates if monitoring is enabled
	Enabled bool
}

// CheckResult represents a single health check result
type CheckResult struct {
	Service    string        `json:"service"`
	Status     Status        `json:"status"`
	Message    string        `json:"message,omitempty"`
	StatusCode int           `json:"status_code,omitempty"`
	Duration   time.Duration `json:"duration"`
	Timestamp  time.Time     `json:"timestamp"`
	Error      string        `json:"error,omitempty"`
	Ready      bool          `json:"ready"`
	Healthy    bool          `json:"healthy"`
	Details    interface{}   `json:"details,omitempty"`
}

// ServiceHealth represents current health state of a service
type ServiceHealth struct {
	Name                 string        `json:"name"`
	Status               Status        `json:"status"`
	LastCheck            time.Time     `json:"last_check"`
	LastHealthy          time.Time     `json:"last_healthy"`
	LastUnhealthy        time.Time     `json:"last_unhealthy,omitempty"`
	Uptime               time.Duration `json:"uptime"`
	UptimePercent        float64       `json:"uptime_percent"`
	ConsecutiveSuccesses int           `json:"consecutive_successes"`
	ConsecutiveFailures  int           `json:"consecutive_failures"`
	TotalChecks          int64         `json:"total_checks"`
	SuccessfulChecks     int64         `json:"successful_checks"`
	FailedChecks         int64         `json:"failed_checks"`
	AverageLatency       time.Duration `json:"average_latency"`
	RecentResults        []CheckResult `json:"recent_results,omitempty"`
	Tags                 []string      `json:"tags,omitempty"`
}

// HealthHistory holds historical check results for a service
type HealthHistory struct {
	Name     string
	Results  []CheckResult
	MaxSize  int
	mu       sync.RWMutex
}

// Add adds a result to the history
func (h *HealthHistory) Add(result CheckResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Results = append(h.Results, result)
	if len(h.Results) > h.MaxSize {
		h.Results = h.Results[len(h.Results)-h.MaxSize:]
	}
}

// GetRecent returns the most recent N results
func (h *HealthHistory) GetRecent(n int) []CheckResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if n > len(h.Results) {
		n = len(h.Results)
	}
	if n == 0 {
		return nil
	}

	results := make([]CheckResult, n)
	start := len(h.Results) - n
	copy(results, h.Results[start:])
	return results
}

// GetSince returns results since a given time
func (h *HealthHistory) GetSince(since time.Time) []CheckResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var results []CheckResult
	for _, r := range h.Results {
		if r.Timestamp.After(since) {
			results = append(results, r)
		}
	}
	return results
}

// Cleanup removes results older than cutoff
func (h *HealthHistory) Cleanup(cutoff time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var valid []CheckResult
	for _, r := range h.Results {
		if r.Timestamp.After(cutoff) {
			valid = append(valid, r)
		}
	}
	h.Results = valid
}

// CalculateUptime calculates uptime percentage for a duration
func (h *HealthHistory) CalculateUptime(duration time.Duration) float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	since := time.Now().Add(-duration)
	var total, successful int

	for _, r := range h.Results {
		if r.Timestamp.After(since) {
			total++
			if r.Status == StatusHealthy {
				successful++
			}
		}
	}

	if total == 0 {
		return 100.0 // No data means we assume healthy
	}

	return float64(successful) / float64(total) * 100.0
}

// SystemHealth represents overall system health
type SystemHealth struct {
	Status         Status                    `json:"status"`
	Services       map[string]*ServiceHealth `json:"services"`
	TotalServices  int                       `json:"total_services"`
	HealthyCount   int                       `json:"healthy_count"`
	DegradedCount  int                       `json:"degraded_count"`
	UnhealthyCount int                       `json:"unhealthy_count"`
	UnknownCount   int                       `json:"unknown_count"`
	LastUpdated    time.Time                 `json:"last_updated"`
	Message        string                    `json:"message,omitempty"`
}

// HealthEvent represents a health state change event
type HealthEvent struct {
	Service     string    `json:"service"`
	OldStatus   Status    `json:"old_status"`
	NewStatus   Status    `json:"new_status"`
	Timestamp   time.Time `json:"timestamp"`
	Message     string    `json:"message,omitempty"`
}

// Monitor monitors health of Kingdom services
type Monitor struct {
	config    *Config
	log       *logger.Logger
	client    *http.Client
	targets   map[string]*ServiceTarget
	health    map[string]*ServiceHealth
	history   map[string]*HealthHistory

	targetsMu sync.RWMutex
	healthMu  sync.RWMutex
	historyMu sync.RWMutex

	running  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	runMu    sync.RWMutex
	wg       sync.WaitGroup

	// Callbacks
	onStatusChange func(event HealthEvent)
}

// NewMonitor creates a new health monitor
func NewMonitor(config *Config, log *logger.Logger) (*Monitor, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if log == nil {
		log = logger.New(nil)
	}

	return &Monitor{
		config:  config,
		log:     log,
		client: &http.Client{
			Timeout: config.PollTimeout,
		},
		targets:  make(map[string]*ServiceTarget),
		health:   make(map[string]*ServiceHealth),
		history:  make(map[string]*HealthHistory),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}, nil
}

// RegisterTarget registers a service target for monitoring
func (m *Monitor) RegisterTarget(target *ServiceTarget) error {
	if target == nil {
		return errors.New("target cannot be nil")
	}
	if target.Name == "" {
		return errors.New("target name cannot be empty")
	}
	if target.HealthURL == "" {
		return errors.New("target health URL cannot be empty")
	}

	m.targetsMu.Lock()
	defer m.targetsMu.Unlock()

	target.Enabled = true
	m.targets[target.Name] = target

	// Initialize health state
	m.healthMu.Lock()
	m.health[target.Name] = &ServiceHealth{
		Name:   target.Name,
		Status: StatusUnknown,
		Tags:   target.Tags,
	}
	m.healthMu.Unlock()

	// Initialize history
	m.historyMu.Lock()
	m.history[target.Name] = &HealthHistory{
		Name:    target.Name,
		MaxSize: m.config.MaxHistory,
		Results: make([]CheckResult, 0, m.config.MaxHistory),
	}
	m.historyMu.Unlock()

	m.log.Info().
		Str("service", target.Name).
		Str("health_url", target.HealthURL).
		Msg("registered health target")

	return nil
}

// UnregisterTarget removes a service target
func (m *Monitor) UnregisterTarget(name string) error {
	m.targetsMu.Lock()
	defer m.targetsMu.Unlock()

	if _, exists := m.targets[name]; !exists {
		return ErrServiceNotFound
	}

	delete(m.targets, name)

	m.healthMu.Lock()
	delete(m.health, name)
	m.healthMu.Unlock()

	m.historyMu.Lock()
	delete(m.history, name)
	m.historyMu.Unlock()

	m.log.Info().
		Str("service", name).
		Msg("unregistered health target")

	return nil
}

// OnStatusChange sets a callback for status changes
func (m *Monitor) OnStatusChange(fn func(HealthEvent)) {
	m.onStatusChange = fn
}

// Start starts the health monitor
func (m *Monitor) Start(ctx context.Context) error {
	m.runMu.Lock()
	if m.running {
		m.runMu.Unlock()
		return errors.New("monitor already running")
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})
	m.runMu.Unlock()

	m.wg.Add(2)
	go m.pollLoop(ctx)
	go m.cleanupLoop(ctx)

	m.log.Info().
		Dur("interval", m.config.PollInterval).
		Msg("health monitor started")

	return nil
}

// Stop stops the health monitor
func (m *Monitor) Stop() error {
	m.runMu.Lock()
	if !m.running {
		m.runMu.Unlock()
		return nil
	}
	m.running = false
	close(m.stopCh)
	m.runMu.Unlock()

	m.wg.Wait()
	close(m.doneCh)

	m.log.Info().Msg("health monitor stopped")
	return nil
}

// pollLoop runs the periodic health polling
func (m *Monitor) pollLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.PollInterval)
	defer ticker.Stop()

	// Initial poll
	m.pollAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.pollAll(ctx)
		}
	}
}

// cleanupLoop runs periodic cleanup of old history
func (m *Monitor) cleanupLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.RetentionPeriod / 10)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

// pollAll polls all registered targets
func (m *Monitor) pollAll(ctx context.Context) {
	m.targetsMu.RLock()
	targets := make([]*ServiceTarget, 0, len(m.targets))
	for _, t := range m.targets {
		if t.Enabled {
			targets = append(targets, t)
		}
	}
	m.targetsMu.RUnlock()

	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(t *ServiceTarget) {
			defer wg.Done()
			m.pollTarget(ctx, t)
		}(target)
	}
	wg.Wait()
}

// pollTarget polls a single target
func (m *Monitor) pollTarget(ctx context.Context, target *ServiceTarget) {
	start := time.Now()
	result := CheckResult{
		Service:   target.Name,
		Timestamp: start,
	}

	// Check health endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", target.HealthURL, nil)
	if err != nil {
		result.Status = StatusUnhealthy
		result.Error = fmt.Sprintf("create request: %v", err)
		result.Duration = time.Since(start)
		m.processResult(target, result)
		return
	}

	resp, err := m.client.Do(req)
	if err != nil {
		result.Status = StatusUnhealthy
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Duration = time.Since(start)
		m.processResult(target, result)
		return
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Duration = time.Since(start)

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Status = StatusDegraded
		result.Error = fmt.Sprintf("read body: %v", err)
		m.processResult(target, result)
		return
	}

	// Parse health response
	var healthResp map[string]interface{}
	if err := json.Unmarshal(body, &healthResp); err == nil {
		result.Details = healthResp
		if status, ok := healthResp["status"].(string); ok {
			result.Message = status
		}
	}

	// Determine status based on HTTP code
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		result.Status = StatusHealthy
		result.Healthy = true
	case resp.StatusCode >= 500:
		result.Status = StatusUnhealthy
		result.Healthy = false
	default:
		result.Status = StatusDegraded
		result.Healthy = false
	}

	// Check ready endpoint if available
	if target.ReadyURL != "" {
		readyReq, err := http.NewRequestWithContext(ctx, "GET", target.ReadyURL, nil)
		if err == nil {
			readyResp, err := m.client.Do(readyReq)
			if err == nil {
				result.Ready = readyResp.StatusCode >= 200 && readyResp.StatusCode < 300
				readyResp.Body.Close()
			}
		}
	} else {
		result.Ready = result.Healthy
	}

	m.processResult(target, result)
}

// processResult processes a health check result
func (m *Monitor) processResult(target *ServiceTarget, result CheckResult) {
	// Store in history
	m.historyMu.RLock()
	hist := m.history[target.Name]
	m.historyMu.RUnlock()

	if hist != nil {
		hist.Add(result)
	}

	// Update health state
	m.healthMu.Lock()
	health, exists := m.health[target.Name]
	if !exists {
		health = &ServiceHealth{
			Name:   target.Name,
			Status: StatusUnknown,
			Tags:   target.Tags,
		}
		m.health[target.Name] = health
	}

	oldStatus := health.Status
	health.LastCheck = result.Timestamp
	health.TotalChecks++

	// Update consecutive counts and status
	if result.Status == StatusHealthy {
		health.SuccessfulChecks++
		health.ConsecutiveSuccesses++
		health.ConsecutiveFailures = 0
		health.LastHealthy = result.Timestamp

		if health.ConsecutiveSuccesses >= m.config.RecoveryThreshold {
			health.Status = StatusHealthy
		}
	} else {
		health.FailedChecks++
		health.ConsecutiveFailures++
		health.ConsecutiveSuccesses = 0

		if result.Status == StatusUnhealthy {
			health.LastUnhealthy = result.Timestamp
		}

		if health.ConsecutiveFailures >= m.config.FailureThreshold {
			health.Status = StatusUnhealthy
		} else if health.Status == StatusHealthy {
			health.Status = StatusDegraded
		}
	}

	// Calculate uptime
	if hist != nil {
		health.UptimePercent = hist.CalculateUptime(m.config.RetentionPeriod)
	}

	// Calculate average latency (simple moving average from last 100)
	if hist != nil {
		recent := hist.GetRecent(100)
		if len(recent) > 0 {
			var total time.Duration
			for _, r := range recent {
				total += r.Duration
			}
			health.AverageLatency = total / time.Duration(len(recent))
		}
	}

	// Calculate uptime duration
	if health.LastHealthy.After(health.LastUnhealthy) {
		health.Uptime = time.Since(health.LastUnhealthy)
	} else if !health.LastHealthy.IsZero() {
		health.Uptime = time.Since(health.LastHealthy)
	}

	m.healthMu.Unlock()

	// Emit status change event
	if oldStatus != health.Status && oldStatus != StatusUnknown {
		event := HealthEvent{
			Service:   target.Name,
			OldStatus: oldStatus,
			NewStatus: health.Status,
			Timestamp: result.Timestamp,
			Message:   result.Message,
		}

		m.log.Info().
			Str("service", target.Name).
			Str("old_status", string(oldStatus)).
			Str("new_status", string(health.Status)).
			Msg("health status changed")

		if m.onStatusChange != nil {
			m.onStatusChange(event)
		}
	}
}

// cleanup removes old history entries
func (m *Monitor) cleanup() {
	cutoff := time.Now().Add(-m.config.RetentionPeriod)

	m.historyMu.RLock()
	histories := make([]*HealthHistory, 0, len(m.history))
	for _, h := range m.history {
		histories = append(histories, h)
	}
	m.historyMu.RUnlock()

	for _, h := range histories {
		h.Cleanup(cutoff)
	}

	m.log.Debug().
		Time("cutoff", cutoff).
		Msg("health history cleanup complete")
}

// GetServiceHealth returns health for a specific service
func (m *Monitor) GetServiceHealth(name string) (*ServiceHealth, error) {
	m.healthMu.RLock()
	defer m.healthMu.RUnlock()

	health, exists := m.health[name]
	if !exists {
		return nil, ErrServiceNotFound
	}

	// Make a copy with recent results
	result := *health

	m.historyMu.RLock()
	if hist, ok := m.history[name]; ok {
		result.RecentResults = hist.GetRecent(10)
	}
	m.historyMu.RUnlock()

	return &result, nil
}

// GetSystemHealth returns overall system health
func (m *Monitor) GetSystemHealth() *SystemHealth {
	m.healthMu.RLock()
	defer m.healthMu.RUnlock()

	system := &SystemHealth{
		Services:    make(map[string]*ServiceHealth),
		LastUpdated: time.Now(),
	}

	for name, health := range m.health {
		// Make a copy
		h := *health

		// Add recent results
		m.historyMu.RLock()
		if hist, ok := m.history[name]; ok {
			h.RecentResults = hist.GetRecent(5)
		}
		m.historyMu.RUnlock()

		system.Services[name] = &h
		system.TotalServices++

		switch health.Status {
		case StatusHealthy:
			system.HealthyCount++
		case StatusDegraded:
			system.DegradedCount++
		case StatusUnhealthy:
			system.UnhealthyCount++
		default:
			system.UnknownCount++
		}
	}

	// Determine overall status
	if system.UnhealthyCount > 0 {
		system.Status = StatusUnhealthy
		system.Message = fmt.Sprintf("%d/%d services unhealthy", system.UnhealthyCount, system.TotalServices)
	} else if system.DegradedCount > 0 {
		system.Status = StatusDegraded
		system.Message = fmt.Sprintf("%d/%d services degraded", system.DegradedCount, system.TotalServices)
	} else if system.HealthyCount == system.TotalServices && system.TotalServices > 0 {
		system.Status = StatusHealthy
		system.Message = fmt.Sprintf("All %d services healthy", system.TotalServices)
	} else {
		system.Status = StatusUnknown
		system.Message = "No services registered or all unknown"
	}

	return system
}

// GetHistory returns health history for a service
func (m *Monitor) GetHistory(name string, since time.Time) ([]CheckResult, error) {
	m.historyMu.RLock()
	defer m.historyMu.RUnlock()

	hist, exists := m.history[name]
	if !exists {
		return nil, ErrServiceNotFound
	}

	return hist.GetSince(since), nil
}

// GetTargets returns all registered targets
func (m *Monitor) GetTargets() []*ServiceTarget {
	m.targetsMu.RLock()
	defer m.targetsMu.RUnlock()

	targets := make([]*ServiceTarget, 0, len(m.targets))
	for _, t := range m.targets {
		targets = append(targets, t)
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})

	return targets
}

// IsRunning returns whether the monitor is running
func (m *Monitor) IsRunning() bool {
	m.runMu.RLock()
	defer m.runMu.RUnlock()
	return m.running
}

// ForceCheck forces an immediate health check for a service
func (m *Monitor) ForceCheck(ctx context.Context, name string) (*CheckResult, error) {
	m.targetsMu.RLock()
	target, exists := m.targets[name]
	m.targetsMu.RUnlock()

	if !exists {
		return nil, ErrServiceNotFound
	}

	start := time.Now()
	result := CheckResult{
		Service:   target.Name,
		Timestamp: start,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", target.HealthURL, nil)
	if err != nil {
		result.Status = StatusUnhealthy
		result.Error = fmt.Sprintf("create request: %v", err)
		result.Duration = time.Since(start)
		return &result, nil
	}

	resp, err := m.client.Do(req)
	if err != nil {
		result.Status = StatusUnhealthy
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Duration = time.Since(start)
		return &result, nil
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Duration = time.Since(start)

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		result.Status = StatusHealthy
		result.Healthy = true
	case resp.StatusCode >= 500:
		result.Status = StatusUnhealthy
	default:
		result.Status = StatusDegraded
	}

	return &result, nil
}

// DefaultServiceEndpoints returns the default LXD bridge addresses for Kingdom services.
var DefaultServiceEndpoints = map[string]string{
	"timeguru":     "10.10.10.20:8000",
	"captain":      "10.10.10.21:8001",
	"architect":    "10.10.10.22:8002",
	"micromanager": "10.10.10.23:8003",
	"busboy":       "10.10.10.10:9090",
	"gateway":      "10.10.10.100:8080",
}

// RegisterKingdomServices registers the standard Kingdom services for monitoring.
// If overrides is non-nil, entries override the default addresses (format: "host:port").
func (m *Monitor) RegisterKingdomServices(overrides map[string]string) {
	endpoints := make(map[string]string, len(DefaultServiceEndpoints))
	for k, v := range DefaultServiceEndpoints {
		endpoints[k] = v
	}
	for k, v := range overrides {
		endpoints[k] = v
	}

	for name, hostPort := range endpoints {
		target := &ServiceTarget{
			Name:      name,
			HealthURL: fmt.Sprintf("http://%s/health", hostPort),
			ReadyURL:  fmt.Sprintf("http://%s/ready", hostPort),
			Tags:      []string{"kingdom"},
		}
		m.RegisterTarget(target)
	}
}
