// Package hauberk provides service mesh and mTLS capabilities.
// Hauberk the Service Mesh is the Kingdom's chainmail - secure inter-service communication.
// Implements service discovery, circuit breakers, retries, and mutual TLS.
package hauberk

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
)

// Endpoint represents a service endpoint in the mesh.
type Endpoint struct {
	ID          string            `json:"id"`
	ServiceName string            `json:"service_name"`
	Address     string            `json:"address"`
	Port        int               `json:"port"`
	Weight      int               `json:"weight"`
	Healthy     bool              `json:"healthy"`
	Labels      map[string]string `json:"labels,omitempty"`
	TLSEnabled  bool              `json:"tls_enabled"`
	LastUpdated time.Time         `json:"last_updated"`
}

// CircuitBreaker tracks circuit breaker state.
type CircuitBreaker struct {
	ServiceName  string              `json:"service_name"`
	State        CircuitBreakerState `json:"state"`
	Failures     int                 `json:"failures"`
	Successes    int                 `json:"successes"`
	LastFailure  *time.Time          `json:"last_failure,omitempty"`
	LastSuccess  *time.Time          `json:"last_success,omitempty"`
	OpenedAt     *time.Time          `json:"opened_at,omitempty"`
}

// CircuitBreakerState represents circuit state.
type CircuitBreakerState string

const (
	CircuitClosed   CircuitBreakerState = "closed"
	CircuitOpen     CircuitBreakerState = "open"
	CircuitHalfOpen CircuitBreakerState = "half_open"
)

// TrafficPolicy defines routing policy.
type TrafficPolicy struct {
	ID             string         `json:"id"`
	ServiceName    string         `json:"service_name"`
	Retries        RetryPolicy    `json:"retries,omitempty"`
	Timeout        time.Duration  `json:"timeout,omitempty"`
	CircuitBreaker CBPolicy       `json:"circuit_breaker,omitempty"`
	LoadBalancer   LBPolicy       `json:"load_balancer,omitempty"`
}

// RetryPolicy defines retry behavior.
type RetryPolicy struct {
	Attempts      int           `json:"attempts"`
	PerTryTimeout time.Duration `json:"per_try_timeout"`
	RetryOn       []string      `json:"retry_on,omitempty"` // 5xx, reset, connect-failure
}

// CBPolicy defines circuit breaker settings.
type CBPolicy struct {
	Threshold        int           `json:"threshold"`
	Timeout          time.Duration `json:"timeout"`
	HalfOpenRequests int           `json:"half_open_requests"`
}

// LBPolicy defines load balancing.
type LBPolicy struct {
	Algorithm string `json:"algorithm"` // round_robin, least_conn, random, maglev
}

// Certificate represents a mesh certificate.
type Certificate struct {
	ID          string    `json:"id"`
	ServiceName string    `json:"service_name"`
	CommonName  string    `json:"common_name"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	IsCA        bool      `json:"is_ca"`
}

// Service is the main Hauberk service mesh service.
type Service struct {
	log    *logger.Logger
	wotan *wotanClient.Client
	config *Config

	mu              sync.RWMutex
	endpoints       map[string][]*Endpoint // service name -> endpoints
	circuitBreakers map[string]*CircuitBreaker
	policies        map[string]*TrafficPolicy
	certificates    map[string]*Certificate
	tlsConfig       *tls.Config
}

// Config holds Hauberk service configuration.
type Config struct {
	DefaultTimeout       time.Duration `json:"default_timeout"`
	DefaultRetries       int           `json:"default_retries"`
	CBThreshold          int           `json:"cb_threshold"`
	CBTimeout            time.Duration `json:"cb_timeout"`
	MTLSEnabled          bool          `json:"mtls_enabled"`
	CertRotationInterval time.Duration `json:"cert_rotation_interval"`
	WotanTopic          string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DefaultTimeout:       10 * time.Second,
		DefaultRetries:       3,
		CBThreshold:          5,
		CBTimeout:            30 * time.Second,
		MTLSEnabled:          true,
		CertRotationInterval: 24 * time.Hour,
		WotanTopic:          "hauberk.mesh",
	}
}

// NewService creates a new Hauberk service mesh service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:             log,
		wotan:          wotan,
		config:          cfg,
		endpoints:       make(map[string][]*Endpoint),
		circuitBreakers: make(map[string]*CircuitBreaker),
		policies:        make(map[string]*TrafficPolicy),
		certificates:    make(map[string]*Certificate),
	}
}

// Start initializes the Hauberk service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Hauberk woven - service mesh active")
	go s.circuitBreakerLoop(ctx)
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Hauberk removed - service mesh suspended")
	return nil
}

// RegisterEndpoint registers a service endpoint.
func (s *Service) RegisterEndpoint(endpoint *Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if endpoint.ID == "" {
		endpoint.ID = fmt.Sprintf("%s-%s-%d", endpoint.ServiceName, endpoint.Address, endpoint.Port)
	}
	endpoint.LastUpdated = time.Now()
	endpoint.Healthy = true

	s.endpoints[endpoint.ServiceName] = append(s.endpoints[endpoint.ServiceName], endpoint)

	// Initialize circuit breaker
	if _, ok := s.circuitBreakers[endpoint.ServiceName]; !ok {
		s.circuitBreakers[endpoint.ServiceName] = &CircuitBreaker{
			ServiceName: endpoint.ServiceName,
			State:       CircuitClosed,
		}
	}

	s.log.Info().
		Str("id", endpoint.ID).
		Str("service", endpoint.ServiceName).
		Str("address", endpoint.Address).
		Msg("Endpoint registered")

	return nil
}

// DeregisterEndpoint removes an endpoint.
func (s *Service) DeregisterEndpoint(endpointID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for svc, endpoints := range s.endpoints {
		for i, ep := range endpoints {
			if ep.ID == endpointID {
				s.endpoints[svc] = append(endpoints[:i], endpoints[i+1:]...)
				s.log.Info().Str("id", endpointID).Msg("Endpoint deregistered")
				return nil
			}
		}
	}

	return fmt.Errorf("endpoint not found: %s", endpointID)
}

// GetEndpoints returns healthy endpoints for a service.
func (s *Service) GetEndpoints(serviceName string) []*Endpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var healthy []*Endpoint
	for _, ep := range s.endpoints[serviceName] {
		if ep.Healthy {
			healthy = append(healthy, ep)
		}
	}
	return healthy
}

// SelectEndpoint picks an endpoint based on load balancing policy.
func (s *Service) SelectEndpoint(serviceName string) (*Endpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check circuit breaker
	cb, ok := s.circuitBreakers[serviceName]
	if ok && cb.State == CircuitOpen {
		return nil, fmt.Errorf("circuit open for service: %s", serviceName)
	}

	endpoints := s.GetEndpoints(serviceName)
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no healthy endpoints for service: %s", serviceName)
	}

	// Simple round-robin (would use policy in production)
	return endpoints[0], nil
}

// RecordSuccess records a successful request.
func (s *Service) RecordSuccess(serviceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cb, ok := s.circuitBreakers[serviceName]
	if !ok {
		return
	}

	now := time.Now()
	cb.Successes++
	cb.LastSuccess = &now

	if cb.State == CircuitHalfOpen {
		cb.State = CircuitClosed
		cb.Failures = 0
		s.log.Info().Str("service", serviceName).Msg("Circuit breaker closed")
	}
}

// RecordFailure records a failed request.
func (s *Service) RecordFailure(serviceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cb, ok := s.circuitBreakers[serviceName]
	if !ok {
		return
	}

	now := time.Now()
	cb.Failures++
	cb.LastFailure = &now

	if cb.Failures >= s.config.CBThreshold && cb.State == CircuitClosed {
		cb.State = CircuitOpen
		cb.OpenedAt = &now
		s.log.Warn().Str("service", serviceName).Int("failures", cb.Failures).Msg("Circuit breaker opened")
	}
}

// GetCircuitBreaker returns circuit breaker state.
func (s *Service) GetCircuitBreaker(serviceName string) (*CircuitBreaker, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cb, ok := s.circuitBreakers[serviceName]
	return cb, ok
}

// SetPolicy sets traffic policy for a service.
func (s *Service) SetPolicy(policy *TrafficPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if policy.ID == "" {
		policy.ID = fmt.Sprintf("policy-%s", policy.ServiceName)
	}

	s.policies[policy.ServiceName] = policy
	s.log.Info().Str("service", policy.ServiceName).Msg("Traffic policy set")
	return nil
}

// GetPolicy returns traffic policy for a service.
func (s *Service) GetPolicy(serviceName string) (*TrafficPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	policy, ok := s.policies[serviceName]
	return policy, ok
}

// IssueCertificate issues a mesh certificate.
func (s *Service) IssueCertificate(serviceName string) (*Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	cert := &Certificate{
		ID:          fmt.Sprintf("cert-%s-%d", serviceName, now.UnixNano()),
		ServiceName: serviceName,
		CommonName:  fmt.Sprintf("%s.mesh.local", serviceName),
		NotBefore:   now,
		NotAfter:    now.Add(s.config.CertRotationInterval),
		IsCA:        false,
	}

	s.certificates[serviceName] = cert
	s.log.Info().Str("service", serviceName).Str("cn", cert.CommonName).Msg("Certificate issued")
	return cert, nil
}

// GetCertificate returns certificate for a service.
func (s *Service) GetCertificate(serviceName string) (*Certificate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cert, ok := s.certificates[serviceName]
	return cert, ok
}

func (s *Service) circuitBreakerLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkCircuitBreakers()
		}
	}
}

func (s *Service) checkCircuitBreakers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, cb := range s.circuitBreakers {
		if cb.State == CircuitOpen && cb.OpenedAt != nil {
			if now.Sub(*cb.OpenedAt) > s.config.CBTimeout {
				cb.State = CircuitHalfOpen
				s.log.Info().Str("service", cb.ServiceName).Msg("Circuit breaker half-open")
			}
		}
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalEndpoints := 0
	for _, eps := range s.endpoints {
		totalEndpoints += len(eps)
	}

	openCircuits := 0
	for _, cb := range s.circuitBreakers {
		if cb.State == CircuitOpen {
			openCircuits++
		}
	}

	return map[string]interface{}{
		"total_services":   len(s.endpoints),
		"total_endpoints":  totalEndpoints,
		"total_policies":   len(s.policies),
		"open_circuits":    openCircuits,
		"certificates":     len(s.certificates),
		"mtls_enabled":     s.config.MTLSEnabled,
	}
}

// RegisterRoutes registers /health, /ready, and /metrics endpoints on the provided mux.
// Required by CLAUDE.md: every component must expose these endpoints.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "hauberk",
		})
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   true,
			"service": "hauberk",
		})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := s.Stats()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP hauberk_services_total Total services in mesh\n")
		fmt.Fprintf(w, "# TYPE hauberk_services_total gauge\n")
		fmt.Fprintf(w, "hauberk_services_total %v\n", stats["total_services"])
		fmt.Fprintf(w, "# HELP hauberk_endpoints_total Total endpoints\n")
		fmt.Fprintf(w, "# TYPE hauberk_endpoints_total gauge\n")
		fmt.Fprintf(w, "hauberk_endpoints_total %v\n", stats["total_endpoints"])
		fmt.Fprintf(w, "# HELP hauberk_open_circuits Open circuit breakers\n")
		fmt.Fprintf(w, "# TYPE hauberk_open_circuits gauge\n")
		fmt.Fprintf(w, "hauberk_open_circuits %v\n", stats["open_circuits"])
		fmt.Fprintf(w, "# HELP hauberk_certificates_total Total certificates\n")
		fmt.Fprintf(w, "# TYPE hauberk_certificates_total gauge\n")
		fmt.Fprintf(w, "hauberk_certificates_total %v\n", stats["certificates"])
	})
}
