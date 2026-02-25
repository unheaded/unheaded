package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the health status of an endpoint.
type HealthStatus int

const (
	HealthUnknown HealthStatus = iota
	HealthHealthy
	HealthUnhealthy
)

func (s HealthStatus) String() string {
	switch s {
	case HealthHealthy:
		return "healthy"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthCheckConfig configures health checking.
type HealthCheckConfig struct {
	// Check interval
	Interval time.Duration

	// Timeouts
	Timeout time.Duration

	// Thresholds
	HealthyThreshold   int // consecutive successes to mark healthy
	UnhealthyThreshold int // consecutive failures to mark unhealthy

	// HTTP check settings
	Path           string
	ExpectedStatus int
	Host           string

	// TCP check settings
	TCPOnly bool
}

// DefaultHealthCheckConfig returns default health check configuration.
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Interval:           10 * time.Second,
		Timeout:            5 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		Path:               "/health",
		ExpectedStatus:     200,
	}
}

// HealthChecker performs health checks on endpoints.
type HealthChecker struct {
	config     *HealthCheckConfig
	httpClient *http.Client

	mu       sync.RWMutex
	checks   map[string]*healthCheck
	done     chan struct{}
	onChange func(address string, status HealthStatus)
}

type healthCheck struct {
	address           string
	status            HealthStatus
	consecutiveOK     int
	consecutiveFail   int
	lastCheck         time.Time
	lastError         error
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(config *HealthCheckConfig) *HealthChecker {
	if config == nil {
		config = DefaultHealthCheckConfig()
	}

	return &HealthChecker{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				DisableKeepAlives: true, // Fresh connection each check
			},
		},
		checks: make(map[string]*healthCheck),
		done:   make(chan struct{}),
	}
}

// Start begins health checking for an endpoint.
func (hc *HealthChecker) Start(address string) {
	hc.mu.Lock()
	if _, exists := hc.checks[address]; exists {
		hc.mu.Unlock()
		return
	}

	check := &healthCheck{
		address: address,
		status:  HealthUnknown,
	}
	hc.checks[address] = check
	hc.mu.Unlock()

	// Start checking goroutine
	go hc.checkLoop(address)
}

// Stop stops health checking for an endpoint.
func (hc *HealthChecker) Stop(address string) {
	hc.mu.Lock()
	delete(hc.checks, address)
	hc.mu.Unlock()
}

// GetStatus returns the health status of an endpoint.
func (hc *HealthChecker) GetStatus(address string) HealthStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	if check, ok := hc.checks[address]; ok {
		return check.status
	}
	return HealthUnknown
}

// IsHealthy returns true if the endpoint is healthy.
func (hc *HealthChecker) IsHealthy(address string) bool {
	return hc.GetStatus(address) == HealthHealthy
}

// OnChange sets a callback for health status changes.
func (hc *HealthChecker) OnChange(fn func(address string, status HealthStatus)) {
	hc.mu.Lock()
	hc.onChange = fn
	hc.mu.Unlock()
}

// Close stops all health checks.
func (hc *HealthChecker) Close() {
	close(hc.done)
}

// GetAllStatus returns status for all tracked endpoints.
func (hc *HealthChecker) GetAllStatus() map[string]HealthStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	result := make(map[string]HealthStatus, len(hc.checks))
	for addr, check := range hc.checks {
		result[addr] = check.status
	}
	return result
}

func (hc *HealthChecker) checkLoop(address string) {
	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	// Initial check
	hc.performCheck(address)

	for {
		select {
		case <-hc.done:
			return
		case <-ticker.C:
			hc.mu.RLock()
			_, exists := hc.checks[address]
			hc.mu.RUnlock()
			if !exists {
				return
			}
			hc.performCheck(address)
		}
	}
}

func (hc *HealthChecker) performCheck(address string) {
	var err error

	if hc.config.TCPOnly {
		err = hc.tcpCheck(address)
	} else {
		err = hc.httpCheck(address)
	}

	hc.updateStatus(address, err)
}

func (hc *HealthChecker) tcpCheck(address string) error {
	conn, err := net.DialTimeout("tcp", address, hc.config.Timeout)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

func (hc *HealthChecker) httpCheck(address string) error {
	url := "http://" + address + hc.config.Path

	ctx, cancel := context.WithTimeout(context.Background(), hc.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	if hc.config.Host != "" {
		req.Host = hc.config.Host
	}

	resp, err := hc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Drain body to reuse connection
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != hc.config.ExpectedStatus {
		return &HealthCheckError{
			StatusCode: resp.StatusCode,
			Expected:   hc.config.ExpectedStatus,
		}
	}

	return nil
}

func (hc *HealthChecker) updateStatus(address string, err error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	check, ok := hc.checks[address]
	if !ok {
		return
	}

	check.lastCheck = time.Now()
	check.lastError = err
	oldStatus := check.status

	if err == nil {
		check.consecutiveOK++
		check.consecutiveFail = 0

		if check.consecutiveOK >= hc.config.HealthyThreshold {
			check.status = HealthHealthy
		}
	} else {
		check.consecutiveFail++
		check.consecutiveOK = 0

		if check.consecutiveFail >= hc.config.UnhealthyThreshold {
			check.status = HealthUnhealthy
		}
	}

	// Notify on status change
	if oldStatus != check.status && hc.onChange != nil {
		go hc.onChange(address, check.status)
	}
}

// HealthCheckError represents a failed health check.
type HealthCheckError struct {
	StatusCode int
	Expected   int
}

func (e *HealthCheckError) Error() string {
	return "health check failed: got " + itoa(e.StatusCode) + ", expected " + itoa(e.Expected)
}

// ActiveHealthChecker performs active health checks with configurable probes.
type ActiveHealthChecker struct {
	*HealthChecker
	probes map[string]HealthProbe
	mu     sync.RWMutex
}

// HealthProbe defines a custom health check probe.
type HealthProbe interface {
	Check(ctx context.Context, address string) error
}

// NewActiveHealthChecker creates an active health checker.
func NewActiveHealthChecker(config *HealthCheckConfig) *ActiveHealthChecker {
	return &ActiveHealthChecker{
		HealthChecker: NewHealthChecker(config),
		probes:        make(map[string]HealthProbe),
	}
}

// RegisterProbe registers a custom probe for an address.
func (ahc *ActiveHealthChecker) RegisterProbe(address string, probe HealthProbe) {
	ahc.mu.Lock()
	ahc.probes[address] = probe
	ahc.mu.Unlock()
}

// Check performs a health check using the registered probe.
func (ahc *ActiveHealthChecker) Check(ctx context.Context, address string) error {
	ahc.mu.RLock()
	probe, ok := ahc.probes[address]
	ahc.mu.RUnlock()

	if ok {
		return probe.Check(ctx, address)
	}

	// Fall back to default check
	if ahc.config.TCPOnly {
		return ahc.tcpCheck(address)
	}
	return ahc.httpCheck(address)
}

// PassiveHealthChecker tracks health based on request outcomes.
type PassiveHealthChecker struct {
	mu        sync.RWMutex
	endpoints map[string]*passiveEndpoint
	config    *PassiveHealthConfig
	onChange  func(address string, status HealthStatus)
}

type passiveEndpoint struct {
	address        string
	status         HealthStatus
	failures       int
	successes      int
	lastFailure    time.Time
	lastSuccess    time.Time
	ejectedUntil   time.Time
}

// PassiveHealthConfig configures passive health checking.
type PassiveHealthConfig struct {
	FailureThreshold   int           // failures before ejection
	SuccessThreshold   int           // successes before recovery
	EjectionTime       time.Duration // how long to eject
	MaxEjectionPercent float64       // max percentage of hosts to eject
	WindowDuration     time.Duration // rolling window for failures
}

// DefaultPassiveHealthConfig returns default passive health config.
func DefaultPassiveHealthConfig() *PassiveHealthConfig {
	return &PassiveHealthConfig{
		FailureThreshold:   5,
		SuccessThreshold:   2,
		EjectionTime:       30 * time.Second,
		MaxEjectionPercent: 50.0,
		WindowDuration:     time.Minute,
	}
}

// NewPassiveHealthChecker creates a passive health checker.
func NewPassiveHealthChecker(config *PassiveHealthConfig) *PassiveHealthChecker {
	if config == nil {
		config = DefaultPassiveHealthConfig()
	}

	return &PassiveHealthChecker{
		endpoints: make(map[string]*passiveEndpoint),
		config:    config,
	}
}

// RecordSuccess records a successful request.
func (phc *PassiveHealthChecker) RecordSuccess(address string) {
	phc.mu.Lock()
	defer phc.mu.Unlock()

	ep := phc.getOrCreateEndpoint(address)
	ep.successes++
	ep.failures = 0
	ep.lastSuccess = time.Now()

	// Check for recovery
	if ep.status == HealthUnhealthy && ep.successes >= phc.config.SuccessThreshold {
		ep.status = HealthHealthy
		if phc.onChange != nil {
			go phc.onChange(address, HealthHealthy)
		}
	}
}

// RecordFailure records a failed request.
func (phc *PassiveHealthChecker) RecordFailure(address string) {
	phc.mu.Lock()
	defer phc.mu.Unlock()

	ep := phc.getOrCreateEndpoint(address)
	ep.failures++
	ep.successes = 0
	ep.lastFailure = time.Now()

	// Check for ejection
	if ep.status == HealthHealthy && ep.failures >= phc.config.FailureThreshold {
		// Check max ejection percent
		if phc.canEject() {
			ep.status = HealthUnhealthy
			ep.ejectedUntil = time.Now().Add(phc.config.EjectionTime)
			if phc.onChange != nil {
				go phc.onChange(address, HealthUnhealthy)
			}
		}
	}
}

// GetStatus returns the health status.
func (phc *PassiveHealthChecker) GetStatus(address string) HealthStatus {
	phc.mu.RLock()
	defer phc.mu.RUnlock()

	if ep, ok := phc.endpoints[address]; ok {
		// Check if ejection period has passed
		if ep.status == HealthUnhealthy && time.Now().After(ep.ejectedUntil) {
			return HealthHealthy // Give another chance
		}
		return ep.status
	}
	return HealthHealthy // Unknown is healthy by default
}

// OnChange sets a callback for health changes.
func (phc *PassiveHealthChecker) OnChange(fn func(address string, status HealthStatus)) {
	phc.mu.Lock()
	phc.onChange = fn
	phc.mu.Unlock()
}

func (phc *PassiveHealthChecker) getOrCreateEndpoint(address string) *passiveEndpoint {
	if ep, ok := phc.endpoints[address]; ok {
		return ep
	}

	ep := &passiveEndpoint{
		address: address,
		status:  HealthHealthy,
	}
	phc.endpoints[address] = ep
	return ep
}

func (phc *PassiveHealthChecker) canEject() bool {
	total := len(phc.endpoints)
	if total == 0 {
		return true
	}

	ejected := 0
	for _, ep := range phc.endpoints {
		if ep.status == HealthUnhealthy {
			ejected++
		}
	}

	ejectedPercent := float64(ejected+1) / float64(total) * 100
	return ejectedPercent <= phc.config.MaxEjectionPercent
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
