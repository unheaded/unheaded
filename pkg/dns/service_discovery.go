// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package dns provides enhanced DNS-based service discovery for the Unheaded mesh.
package dns

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Service discovery errors
var (
	ErrServiceNotFound    = errors.New("dns: service not found")
	ErrNoHealthyEndpoints = errors.New("dns: no healthy endpoints available")
	ErrInvalidService     = errors.New("dns: invalid service configuration")
)

// LoadBalancingPolicy defines how endpoints are selected
type LoadBalancingPolicy int

const (
	// LoadBalanceRoundRobin distributes requests evenly across all endpoints
	LoadBalanceRoundRobin LoadBalancingPolicy = iota
	// LoadBalanceWeighted distributes based on endpoint weights
	LoadBalanceWeighted
	// LoadBalanceRandom selects endpoints randomly
	LoadBalanceRandom
	// LoadBalanceFirstHealthy always returns the first healthy endpoint
	LoadBalanceFirstHealthy
)

func (p LoadBalancingPolicy) String() string {
	switch p {
	case LoadBalanceRoundRobin:
		return "round-robin"
	case LoadBalanceWeighted:
		return "weighted"
	case LoadBalanceRandom:
		return "random"
	case LoadBalanceFirstHealthy:
		return "first-healthy"
	default:
		return "unknown"
	}
}

// HealthState represents the health state of an endpoint
type HealthState int

const (
	HealthStateUnknown HealthState = iota
	HealthStateHealthy
	HealthStateUnhealthy
	HealthStateDegraded
)

func (s HealthState) String() string {
	switch s {
	case HealthStateHealthy:
		return "healthy"
	case HealthStateUnhealthy:
		return "unhealthy"
	case HealthStateDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// ServiceEndpoint represents a single service endpoint with health information
type ServiceEndpoint struct {
	ID           string            // Unique endpoint identifier
	Address      net.IP            // Endpoint IP address
	Port         uint16            // Endpoint port
	Weight       uint16            // Load balancing weight (0-65535)
	Priority     uint16            // SRV priority (lower is better)
	Health       HealthState       // Current health state
	Metadata     map[string]string // Key-value metadata
	LastCheck    time.Time         // Last health check time
	LastSeen     time.Time         // Last heartbeat time
	FailCount    int               // Consecutive failure count
	SuccessCount int               // Consecutive success count
	ResponseTime time.Duration     // Average response time
}

// IsHealthy returns true if the endpoint is considered healthy
func (e *ServiceEndpoint) IsHealthy() bool {
	return e.Health == HealthStateHealthy || e.Health == HealthStateDegraded
}

// EffectiveWeight returns the weight adjusted for health state
func (e *ServiceEndpoint) EffectiveWeight() uint16 {
	if e.Health == HealthStateUnhealthy {
		return 0
	}
	if e.Health == HealthStateDegraded {
		return e.Weight / 2
	}
	return e.Weight
}

// ServiceDefinition defines a discoverable service
type ServiceDefinition struct {
	Name           string                // Service name (e.g., "api")
	Namespace      string                // Service namespace (e.g., "production")
	Protocol       string                // Protocol (tcp, udp, http, grpc)
	Port           uint16                // Default service port
	Endpoints      []*ServiceEndpoint    // Service endpoints
	Metadata       map[string]string     // Service metadata (TXT records)
	TTL            uint32                // DNS record TTL
	LoadBalancing  LoadBalancingPolicy   // Load balancing policy
	HealthCheck    *ServiceHealthCheck   // Health check configuration
	CircuitBreaker *CircuitBreakerConfig // Circuit breaker settings
	RetryPolicy    *RetryPolicyConfig    // Retry configuration
}

// FQDN returns the fully qualified domain name for DNS-SD
func (s *ServiceDefinition) FQDN(domain string) string {
	return fmt.Sprintf("_%s._%s.%s", s.Name, s.Protocol, domain)
}

// InstanceName returns the instance name for DNS-SD
func (s *ServiceDefinition) InstanceName(domain string) string {
	if s.Namespace != "" {
		return fmt.Sprintf("%s._%s._%s.%s", s.Namespace, s.Name, s.Protocol, domain)
	}
	return fmt.Sprintf("_%s._%s.%s", s.Name, s.Protocol, domain)
}

// ServiceHealthCheck configures health checking for a service
type ServiceHealthCheck struct {
	Type               string        // Check type: tcp, http, https, grpc, dns
	Interval           time.Duration // Check interval
	Timeout            time.Duration // Check timeout
	Path               string        // HTTP path for health check
	ExpectedStatus     int           // Expected HTTP status
	ExpectedBody       string        // Expected response body substring
	HealthyThreshold   int           // Checks required to mark healthy
	UnhealthyThreshold int           // Checks required to mark unhealthy
}

// CircuitBreakerConfig configures circuit breaker behavior
type CircuitBreakerConfig struct {
	Enabled          bool
	ErrorThreshold   int           // Errors before circuit opens
	ErrorWindow      time.Duration // Time window for errors
	OpenDuration     time.Duration // How long circuit stays open
	HalfOpenRequests int           // Requests allowed in half-open state
}

// RetryPolicyConfig configures retry behavior
type RetryPolicyConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
}

// EnhancedServiceDiscovery provides advanced DNS-based service discovery
type EnhancedServiceDiscovery struct {
	mu              sync.RWMutex
	zone            *DynamicZone
	services        map[string]*ServiceDefinition
	endpointIndex   map[string]map[string]*ServiceEndpoint // service -> endpoint ID -> endpoint
	roundRobinState map[string]*uint32
	weightedState   map[string]*weightedSelector
	healthChecker   *EnhancedHealthChecker
	metrics         *DiscoveryMetrics
	config          *ServiceDiscoveryConfig
	onChange        []ServiceChangeCallback
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// ServiceChangeCallback is called when a service changes
type ServiceChangeCallback func(serviceName string, event ServiceEvent)

// ServiceEvent represents a service change event
type ServiceEvent int

const (
	ServiceEventRegistered ServiceEvent = iota
	ServiceEventDeregistered
	ServiceEventEndpointAdded
	ServiceEventEndpointRemoved
	ServiceEventHealthChanged
)

func (e ServiceEvent) String() string {
	switch e {
	case ServiceEventRegistered:
		return "registered"
	case ServiceEventDeregistered:
		return "deregistered"
	case ServiceEventEndpointAdded:
		return "endpoint_added"
	case ServiceEventEndpointRemoved:
		return "endpoint_removed"
	case ServiceEventHealthChanged:
		return "health_changed"
	default:
		return "unknown"
	}
}

// weightedSelector implements weighted random selection
type weightedSelector struct {
	mu        sync.Mutex
	endpoints []*ServiceEndpoint
	weights   []int
	total     int
}

func newWeightedSelector() *weightedSelector {
	return &weightedSelector{
		endpoints: make([]*ServiceEndpoint, 0),
		weights:   make([]int, 0),
	}
}

func (ws *weightedSelector) update(endpoints []*ServiceEndpoint) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.endpoints = make([]*ServiceEndpoint, 0, len(endpoints))
	ws.weights = make([]int, 0, len(endpoints))
	ws.total = 0

	for _, ep := range endpoints {
		if ep.IsHealthy() {
			weight := int(ep.EffectiveWeight())
			if weight > 0 {
				ws.endpoints = append(ws.endpoints, ep)
				ws.weights = append(ws.weights, weight)
				ws.total += weight
			}
		}
	}
}

func (ws *weightedSelector) select_() *ServiceEndpoint {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.total == 0 || len(ws.endpoints) == 0 {
		return nil
	}

	r := rand.Intn(ws.total)
	for i, w := range ws.weights {
		r -= w
		if r < 0 {
			return ws.endpoints[i]
		}
	}

	return ws.endpoints[len(ws.endpoints)-1]
}

// ServiceDiscoveryConfig configures the enhanced service discovery
type ServiceDiscoveryConfig struct {
	Zone                 string        // DNS zone name
	DefaultTTL           uint32        // Default record TTL
	HealthCheckInterval  time.Duration // Default health check interval
	HealthCheckTimeout   time.Duration // Default health check timeout
	EnableHealthChecks   bool          // Enable automatic health checking
	EnableMetrics        bool          // Enable discovery metrics
	StaleEndpointTimeout time.Duration // Remove endpoints not seen after this
	HealthyThreshold     int           // Default healthy threshold
	UnhealthyThreshold   int           // Default unhealthy threshold
}

// DefaultServiceDiscoveryConfig returns sensible defaults
func DefaultServiceDiscoveryConfig() *ServiceDiscoveryConfig {
	return &ServiceDiscoveryConfig{
		Zone:                 "service.local",
		DefaultTTL:           30,
		HealthCheckInterval:  10 * time.Second,
		HealthCheckTimeout:   5 * time.Second,
		EnableHealthChecks:   true,
		EnableMetrics:        true,
		StaleEndpointTimeout: 5 * time.Minute,
		HealthyThreshold:     2,
		UnhealthyThreshold:   3,
	}
}

// NewEnhancedServiceDiscovery creates a new enhanced service discovery instance
func NewEnhancedServiceDiscovery(config *ServiceDiscoveryConfig) (*EnhancedServiceDiscovery, error) {
	if config == nil {
		config = DefaultServiceDiscoveryConfig()
	}

	zoneConfig := DefaultZoneConfig(config.Zone)
	zoneConfig.DefaultTTL = config.DefaultTTL

	zone, err := NewDynamicZone(zoneConfig)
	if err != nil {
		return nil, fmt.Errorf("create zone: %w", err)
	}

	sd := &EnhancedServiceDiscovery{
		zone:            zone,
		services:        make(map[string]*ServiceDefinition),
		endpointIndex:   make(map[string]map[string]*ServiceEndpoint),
		roundRobinState: make(map[string]*uint32),
		weightedState:   make(map[string]*weightedSelector),
		config:          config,
		onChange:        make([]ServiceChangeCallback, 0),
		stopCh:          make(chan struct{}),
	}

	if config.EnableMetrics {
		sd.metrics = NewDiscoveryMetrics()
	}

	if config.EnableHealthChecks {
		sd.healthChecker = NewEnhancedHealthChecker(config, sd)
		sd.healthChecker.Start()
	}

	// Start maintenance goroutine
	sd.wg.Add(1)
	go sd.maintenanceLoop()

	return sd, nil
}

// Zone returns the underlying dynamic zone
func (sd *EnhancedServiceDiscovery) Zone() *DynamicZone {
	return sd.zone
}

// RegisterService registers a new service
func (sd *EnhancedServiceDiscovery) RegisterService(svc *ServiceDefinition) error {
	if svc == nil || svc.Name == "" {
		return ErrInvalidService
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Normalize service name
	svc.Name = strings.ToLower(svc.Name)
	if svc.Protocol == "" {
		svc.Protocol = "tcp"
	}
	if svc.TTL == 0 {
		svc.TTL = sd.config.DefaultTTL
	}
	if svc.LoadBalancing == 0 {
		svc.LoadBalancing = LoadBalanceRoundRobin
	}

	// Store service
	sd.services[svc.Name] = svc
	sd.endpointIndex[svc.Name] = make(map[string]*ServiceEndpoint)

	// Initialize load balancing state
	idx := uint32(0)
	sd.roundRobinState[svc.Name] = &idx
	ws := newWeightedSelector()
	sd.weightedState[svc.Name] = ws

	// Register existing endpoints
	for _, ep := range svc.Endpoints {
		sd.endpointIndex[svc.Name][ep.ID] = ep
	}

	// Initialize weighted selector with endpoints
	ws.update(svc.Endpoints)

	// Update DNS records
	sd.updateServiceDNS(svc)

	// Start health checking if configured
	if sd.healthChecker != nil && svc.HealthCheck != nil {
		sd.healthChecker.AddService(svc)
	}

	// Notify callbacks
	sd.notifyChange(svc.Name, ServiceEventRegistered)

	if sd.metrics != nil {
		sd.metrics.ServiceRegistered(svc.Name)
	}

	return nil
}

// DeregisterService removes a service
func (sd *EnhancedServiceDiscovery) DeregisterService(name string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	name = strings.ToLower(name)
	svc, exists := sd.services[name]
	if !exists {
		return ErrServiceNotFound
	}

	// Stop health checking
	if sd.healthChecker != nil {
		sd.healthChecker.RemoveService(name)
	}

	// Remove DNS records
	sd.removeDNSRecords(svc)

	// Cleanup state
	delete(sd.services, name)
	delete(sd.endpointIndex, name)
	delete(sd.roundRobinState, name)
	delete(sd.weightedState, name)

	// Notify callbacks
	sd.notifyChange(name, ServiceEventDeregistered)

	if sd.metrics != nil {
		sd.metrics.ServiceDeregistered(name)
	}

	return nil
}

// AddEndpoint adds an endpoint to a service
func (sd *EnhancedServiceDiscovery) AddEndpoint(serviceName string, ep *ServiceEndpoint) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	svc, exists := sd.services[serviceName]
	if !exists {
		return ErrServiceNotFound
	}

	// Generate ID if not provided
	if ep.ID == "" {
		ep.ID = fmt.Sprintf("%s-%s-%d", serviceName, ep.Address.String(), ep.Port)
	}

	// Set defaults
	if ep.Weight == 0 {
		ep.Weight = 100
	}
	if ep.Health == HealthStateUnknown {
		ep.Health = HealthStateHealthy
	}
	ep.LastSeen = time.Now()

	// Check for duplicate
	if _, exists := sd.endpointIndex[serviceName][ep.ID]; exists {
		return fmt.Errorf("endpoint %s already exists", ep.ID)
	}

	// Add endpoint
	svc.Endpoints = append(svc.Endpoints, ep)
	sd.endpointIndex[serviceName][ep.ID] = ep

	// Update weighted selector
	if ws, ok := sd.weightedState[serviceName]; ok {
		ws.update(svc.Endpoints)
	}

	// Update DNS records
	sd.updateServiceDNS(svc)

	// Notify callbacks
	sd.notifyChange(serviceName, ServiceEventEndpointAdded)

	if sd.metrics != nil {
		sd.metrics.EndpointAdded(serviceName, ep.ID)
	}

	return nil
}

// RemoveEndpoint removes an endpoint from a service
func (sd *EnhancedServiceDiscovery) RemoveEndpoint(serviceName, endpointID string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	svc, exists := sd.services[serviceName]
	if !exists {
		return ErrServiceNotFound
	}

	if _, exists := sd.endpointIndex[serviceName][endpointID]; !exists {
		return fmt.Errorf("endpoint %s not found", endpointID)
	}

	// Remove from endpoints slice
	newEndpoints := make([]*ServiceEndpoint, 0, len(svc.Endpoints)-1)
	for _, ep := range svc.Endpoints {
		if ep.ID != endpointID {
			newEndpoints = append(newEndpoints, ep)
		}
	}
	svc.Endpoints = newEndpoints

	// Remove from index
	delete(sd.endpointIndex[serviceName], endpointID)

	// Update weighted selector
	if ws, ok := sd.weightedState[serviceName]; ok {
		ws.update(svc.Endpoints)
	}

	// Update DNS records
	sd.updateServiceDNS(svc)

	// Notify callbacks
	sd.notifyChange(serviceName, ServiceEventEndpointRemoved)

	if sd.metrics != nil {
		sd.metrics.EndpointRemoved(serviceName, endpointID)
	}

	return nil
}

// UpdateEndpointHealth updates the health state of an endpoint
func (sd *EnhancedServiceDiscovery) UpdateEndpointHealth(serviceName, endpointID string, health HealthState) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	svc, exists := sd.services[serviceName]
	if !exists {
		return ErrServiceNotFound
	}

	ep, exists := sd.endpointIndex[serviceName][endpointID]
	if !exists {
		return fmt.Errorf("endpoint %s not found", endpointID)
	}

	if ep.Health == health {
		return nil // No change
	}

	ep.Health = health
	ep.LastCheck = time.Now()

	// Update weighted selector
	if ws, ok := sd.weightedState[serviceName]; ok {
		ws.update(svc.Endpoints)
	}

	// Update DNS records (only healthy endpoints)
	sd.updateServiceDNS(svc)

	// Notify callbacks
	sd.notifyChange(serviceName, ServiceEventHealthChanged)

	if sd.metrics != nil {
		sd.metrics.HealthChanged(serviceName, endpointID, health)
	}

	return nil
}

// ResolveService returns healthy endpoints for a service using the configured load balancing
func (sd *EnhancedServiceDiscovery) ResolveService(name string) ([]*ServiceEndpoint, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	name = strings.ToLower(name)
	svc, exists := sd.services[name]
	if !exists {
		return nil, ErrServiceNotFound
	}

	// Filter healthy endpoints
	healthy := make([]*ServiceEndpoint, 0)
	for _, ep := range svc.Endpoints {
		if ep.IsHealthy() {
			healthy = append(healthy, ep)
		}
	}

	if len(healthy) == 0 {
		return nil, ErrNoHealthyEndpoints
	}

	// Sort by priority (lower is better)
	sort.Slice(healthy, func(i, j int) bool {
		return healthy[i].Priority < healthy[j].Priority
	})

	return healthy, nil
}

// ResolveServiceSingle returns a single healthy endpoint using load balancing
func (sd *EnhancedServiceDiscovery) ResolveServiceSingle(name string) (*ServiceEndpoint, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	name = strings.ToLower(name)
	svc, exists := sd.services[name]
	if !exists {
		return nil, ErrServiceNotFound
	}

	return sd.selectEndpoint(svc)
}

// selectEndpoint selects an endpoint based on the load balancing policy
func (sd *EnhancedServiceDiscovery) selectEndpoint(svc *ServiceDefinition) (*ServiceEndpoint, error) {
	switch svc.LoadBalancing {
	case LoadBalanceRoundRobin:
		return sd.selectRoundRobin(svc)
	case LoadBalanceWeighted:
		return sd.selectWeighted(svc)
	case LoadBalanceRandom:
		return sd.selectRandom(svc)
	case LoadBalanceFirstHealthy:
		return sd.selectFirstHealthy(svc)
	default:
		return sd.selectRoundRobin(svc)
	}
}

func (sd *EnhancedServiceDiscovery) selectRoundRobin(svc *ServiceDefinition) (*ServiceEndpoint, error) {
	healthy := sd.getHealthyEndpoints(svc)
	if len(healthy) == 0 {
		return nil, ErrNoHealthyEndpoints
	}

	idx := sd.roundRobinState[svc.Name]
	next := atomic.AddUint32(idx, 1)
	return healthy[int(next)%len(healthy)], nil
}

func (sd *EnhancedServiceDiscovery) selectWeighted(svc *ServiceDefinition) (*ServiceEndpoint, error) {
	ws, ok := sd.weightedState[svc.Name]
	if !ok {
		return nil, ErrNoHealthyEndpoints
	}

	ep := ws.select_()
	if ep == nil {
		return nil, ErrNoHealthyEndpoints
	}
	return ep, nil
}

func (sd *EnhancedServiceDiscovery) selectRandom(svc *ServiceDefinition) (*ServiceEndpoint, error) {
	healthy := sd.getHealthyEndpoints(svc)
	if len(healthy) == 0 {
		return nil, ErrNoHealthyEndpoints
	}
	return healthy[rand.Intn(len(healthy))], nil
}

func (sd *EnhancedServiceDiscovery) selectFirstHealthy(svc *ServiceDefinition) (*ServiceEndpoint, error) {
	// Sort by priority first
	endpoints := make([]*ServiceEndpoint, len(svc.Endpoints))
	copy(endpoints, svc.Endpoints)
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Priority < endpoints[j].Priority
	})

	for _, ep := range endpoints {
		if ep.IsHealthy() {
			return ep, nil
		}
	}
	return nil, ErrNoHealthyEndpoints
}

func (sd *EnhancedServiceDiscovery) getHealthyEndpoints(svc *ServiceDefinition) []*ServiceEndpoint {
	healthy := make([]*ServiceEndpoint, 0)
	for _, ep := range svc.Endpoints {
		if ep.IsHealthy() {
			healthy = append(healthy, ep)
		}
	}
	return healthy
}

// updateServiceDNS updates all DNS records for a service
func (sd *EnhancedServiceDiscovery) updateServiceDNS(svc *ServiceDefinition) {
	domain := sd.zone.Name

	// Get healthy endpoints only for DNS
	healthyEndpoints := sd.getHealthyEndpoints(svc)

	// Build service domain name
	serviceDomain := fmt.Sprintf("%s.%s", svc.Name, domain)

	// A/AAAA records for service domain (healthy endpoints only)
	var aRecords, aaaaRecords []*ResourceRecord
	for _, ep := range healthyEndpoints {
		if ep.Address.To4() != nil {
			aRecords = append(aRecords, NewARecord(serviceDomain, svc.TTL, ep.Address))
		} else {
			aaaaRecords = append(aaaaRecords, NewAAAARecord(serviceDomain, svc.TTL, ep.Address))
		}
	}
	sd.zone.ReplaceRecords(serviceDomain, TypeA, aRecords)
	sd.zone.ReplaceRecords(serviceDomain, TypeAAAA, aaaaRecords)

	// SRV records - _service._protocol.domain
	srvName := svc.FQDN(domain)
	var srvRecords []*ResourceRecord
	for _, ep := range healthyEndpoints {
		port := svc.Port
		if ep.Port != 0 {
			port = ep.Port
		}
		srvRecords = append(srvRecords, NewSRVRecord(
			srvName,
			svc.TTL,
			ep.Priority,
			ep.EffectiveWeight(),
			port,
			serviceDomain,
		))
	}
	sd.zone.ReplaceRecords(srvName, TypeSRV, srvRecords)

	// TXT records for service metadata
	if len(svc.Metadata) > 0 {
		var txtParts []string
		for k, v := range svc.Metadata {
			txtParts = append(txtParts, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(txtParts)
		sd.zone.ReplaceRecords(srvName, TypeTXT, []*ResourceRecord{
			NewTXTRecord(srvName, svc.TTL, txtParts...),
		})
	}

	// DNS-SD browsing records
	sd.updateDNSSDRecords(svc)
}

// updateDNSSDRecords updates DNS-SD browsing records
func (sd *EnhancedServiceDiscovery) updateDNSSDRecords(svc *ServiceDefinition) {
	domain := sd.zone.Name

	// Service types enumeration: _services._dns-sd._udp.domain
	browseZone := "_services._dns-sd._udp." + domain
	serviceType := svc.FQDN(domain)

	// Add PTR from browse zone to service type
	existing := sd.zone.GetRecords(browseZone, TypePTR)
	found := false
	for _, rr := range existing {
		if rr.GetTarget() == serviceType {
			found = true
			break
		}
	}
	if !found {
		sd.zone.AddRecord(NewPTRRecord(browseZone, svc.TTL, serviceType))
	}

	// Add PTR from service type to service instance
	instanceName := svc.InstanceName(domain)
	sd.zone.AddRecord(NewPTRRecord(serviceType, svc.TTL, instanceName))

	// Instance TXT record with full metadata
	if len(svc.Metadata) > 0 || len(svc.Endpoints) > 0 {
		var txtParts []string
		for k, v := range svc.Metadata {
			txtParts = append(txtParts, fmt.Sprintf("%s=%s", k, v))
		}
		// Add endpoint count
		txtParts = append(txtParts, fmt.Sprintf("endpoints=%d", len(svc.Endpoints)))
		healthyCount := len(sd.getHealthyEndpoints(svc))
		txtParts = append(txtParts, fmt.Sprintf("healthy=%d", healthyCount))

		sort.Strings(txtParts)
		sd.zone.ReplaceRecords(instanceName, TypeTXT, []*ResourceRecord{
			NewTXTRecord(instanceName, svc.TTL, txtParts...),
		})
	}
}

// removeDNSRecords removes all DNS records for a service
func (sd *EnhancedServiceDiscovery) removeDNSRecords(svc *ServiceDefinition) {
	domain := sd.zone.Name
	serviceDomain := fmt.Sprintf("%s.%s", svc.Name, domain)
	srvName := svc.FQDN(domain)
	instanceName := svc.InstanceName(domain)

	sd.zone.RemoveRecordsByName(serviceDomain, TypeA)
	sd.zone.RemoveRecordsByName(serviceDomain, TypeAAAA)
	sd.zone.RemoveRecordsByName(srvName, TypeSRV)
	sd.zone.RemoveRecordsByName(srvName, TypeTXT)
	sd.zone.RemoveRecordsByName(instanceName, TypeTXT)

	// Note: We don't remove the browse PTR records as they may be shared
}

// OnChange registers a callback for service changes
func (sd *EnhancedServiceDiscovery) OnChange(callback ServiceChangeCallback) {
	sd.mu.Lock()
	sd.onChange = append(sd.onChange, callback)
	sd.mu.Unlock()
}

func (sd *EnhancedServiceDiscovery) notifyChange(serviceName string, event ServiceEvent) {
	for _, cb := range sd.onChange {
		go cb(serviceName, event)
	}
}

// ListServices returns all registered service names
func (sd *EnhancedServiceDiscovery) ListServices() []string {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	names := make([]string, 0, len(sd.services))
	for name := range sd.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetService returns a service definition
func (sd *EnhancedServiceDiscovery) GetService(name string) (*ServiceDefinition, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	name = strings.ToLower(name)
	svc, exists := sd.services[name]
	if !exists {
		return nil, ErrServiceNotFound
	}

	// Return a copy
	copy := *svc
	copy.Endpoints = make([]*ServiceEndpoint, len(svc.Endpoints))
	for i, ep := range svc.Endpoints {
		epCopy := *ep
		copy.Endpoints[i] = &epCopy
	}
	return &copy, nil
}

// maintenanceLoop performs periodic maintenance
func (sd *EnhancedServiceDiscovery) maintenanceLoop() {
	defer sd.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sd.stopCh:
			return
		case <-ticker.C:
			sd.removeStaleEndpoints()
		}
	}
}

// removeStaleEndpoints removes endpoints that haven't been seen recently
func (sd *EnhancedServiceDiscovery) removeStaleEndpoints() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	cutoff := time.Now().Add(-sd.config.StaleEndpointTimeout)

	for serviceName, svc := range sd.services {
		var staleIDs []string
		for _, ep := range svc.Endpoints {
			if !ep.LastSeen.IsZero() && ep.LastSeen.Before(cutoff) {
				staleIDs = append(staleIDs, ep.ID)
			}
		}

		// Remove stale endpoints (without lock since we already hold it)
		for _, id := range staleIDs {
			// Remove from endpoints slice
			newEndpoints := make([]*ServiceEndpoint, 0)
			for _, ep := range svc.Endpoints {
				if ep.ID != id {
					newEndpoints = append(newEndpoints, ep)
				}
			}
			svc.Endpoints = newEndpoints
			delete(sd.endpointIndex[serviceName], id)

			if sd.metrics != nil {
				sd.metrics.EndpointRemoved(serviceName, id)
			}
		}

		if len(staleIDs) > 0 {
			// Update weighted selector
			if ws, ok := sd.weightedState[serviceName]; ok {
				ws.update(svc.Endpoints)
			}
			sd.updateServiceDNS(svc)
		}
	}
}

// Heartbeat updates the LastSeen time for an endpoint
func (sd *EnhancedServiceDiscovery) Heartbeat(serviceName, endpointID string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	ep, exists := sd.endpointIndex[serviceName][endpointID]
	if !exists {
		return fmt.Errorf("endpoint %s not found", endpointID)
	}

	ep.LastSeen = time.Now()
	return nil
}

// Close stops the service discovery
func (sd *EnhancedServiceDiscovery) Close() error {
	close(sd.stopCh)
	sd.wg.Wait()

	if sd.healthChecker != nil {
		sd.healthChecker.Stop()
	}

	return nil
}

// BrowseServices returns all service types available via DNS-SD
func (sd *EnhancedServiceDiscovery) BrowseServices(ctx context.Context) ([]string, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	var services []string
	for name, svc := range sd.services {
		services = append(services, fmt.Sprintf("_%s._%s", name, svc.Protocol))
	}
	sort.Strings(services)
	return services, nil
}

// LookupSRV performs an SRV lookup for a service
func (sd *EnhancedServiceDiscovery) LookupSRV(ctx context.Context, service, proto string) ([]*SRVResult, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	service = strings.TrimPrefix(strings.ToLower(service), "_")
	proto = strings.TrimPrefix(strings.ToLower(proto), "_")

	svc, exists := sd.services[service]
	if !exists || svc.Protocol != proto {
		return nil, ErrServiceNotFound
	}

	var results []*SRVResult
	for _, ep := range svc.Endpoints {
		if ep.IsHealthy() {
			port := svc.Port
			if ep.Port != 0 {
				port = ep.Port
			}
			results = append(results, &SRVResult{
				Target:   fmt.Sprintf("%s.%s", svc.Name, sd.zone.Name),
				Port:     port,
				Priority: ep.Priority,
				Weight:   ep.EffectiveWeight(),
				Address:  ep.Address,
			})
		}
	}

	// Sort by priority then weight
	sort.Slice(results, func(i, j int) bool {
		if results[i].Priority != results[j].Priority {
			return results[i].Priority < results[j].Priority
		}
		return results[i].Weight > results[j].Weight
	})

	return results, nil
}

// SRVResult represents the result of an SRV lookup
type SRVResult struct {
	Target   string
	Port     uint16
	Priority uint16
	Weight   uint16
	Address  net.IP
}

// Metrics returns the discovery metrics
func (sd *EnhancedServiceDiscovery) Metrics() *DiscoveryMetrics {
	return sd.metrics
}
