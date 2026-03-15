// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package dns provides DNS service discovery with mesh integration.
package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// MeshIntegration provides integration between DNS and service mesh
type MeshIntegration struct {
	sd         *EnhancedServiceDiscovery
	registry   ServiceRegistry
	healthAgg  HealthAggregator
	config     *MeshIntegrationConfig

	mu         sync.RWMutex
	watchers   map[string]context.CancelFunc
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// ServiceRegistry is the interface for a service registry
type ServiceRegistry interface {
	// Discover returns service information
	Discover(ctx context.Context, name, namespace string) (*MeshService, error)
	// Watch watches for service changes
	Watch(ctx context.Context, name, namespace string) (<-chan *MeshService, error)
	// Register registers a service
	Register(ctx context.Context, svc *MeshService) error
	// Deregister removes a service
	Deregister(ctx context.Context, name, namespace string) error
	// ListServices lists all services
	ListServices(ctx context.Context, namespace string) ([]string, error)
}

// HealthAggregator provides aggregated health status
type HealthAggregator interface {
	// GetHealth returns the health of an endpoint
	GetHealth(address string) EndpointHealth
	// OnHealthChange registers a callback for health changes
	OnHealthChange(callback func(address string, health EndpointHealth))
	// IsHealthy returns true if the endpoint is healthy
	IsHealthy(address string) bool
}

// EndpointHealth represents endpoint health from the aggregator
type EndpointHealth struct {
	Healthy      bool
	Status       string
	LastCheck    time.Time
	FailureCount int
	ResponseTime time.Duration
}

// MeshService represents a service from the mesh registry
type MeshService struct {
	Name      string
	Namespace string
	Protocol  string
	Port      uint16
	Instances []*MeshInstance
	Metadata  map[string]string
	Version   string
}

// MeshInstance represents a service instance
type MeshInstance struct {
	ID        string
	Address   string
	Port      int
	Health    string
	Weight    int
	Tags      []string
	Metadata  map[string]string
	LastSeen  time.Time
}

// MeshIntegrationConfig configures mesh integration
type MeshIntegrationConfig struct {
	Namespace         string
	SyncInterval      time.Duration
	HealthSyncEnabled bool
	AutoRegister      bool
	DefaultTTL        uint32
	DefaultProtocol   string
}

// DefaultMeshIntegrationConfig returns default configuration
func DefaultMeshIntegrationConfig() *MeshIntegrationConfig {
	return &MeshIntegrationConfig{
		Namespace:         "default",
		SyncInterval:      30 * time.Second,
		HealthSyncEnabled: true,
		AutoRegister:      true,
		DefaultTTL:        30,
		DefaultProtocol:   "tcp",
	}
}

// NewMeshIntegration creates a new mesh integration
func NewMeshIntegration(
	sd *EnhancedServiceDiscovery,
	registry ServiceRegistry,
	healthAgg HealthAggregator,
	config *MeshIntegrationConfig,
) *MeshIntegration {
	if config == nil {
		config = DefaultMeshIntegrationConfig()
	}

	mi := &MeshIntegration{
		sd:        sd,
		registry:  registry,
		healthAgg: healthAgg,
		config:    config,
		watchers:  make(map[string]context.CancelFunc),
		stopCh:    make(chan struct{}),
	}

	// Register health change callback
	if healthAgg != nil && config.HealthSyncEnabled {
		healthAgg.OnHealthChange(mi.handleHealthChange)
	}

	return mi
}

// Start starts the mesh integration
func (mi *MeshIntegration) Start(ctx context.Context) error {
	// Initial sync
	if err := mi.syncAll(ctx); err != nil {
		return fmt.Errorf("initial sync: %w", err)
	}

	// Start periodic sync
	mi.wg.Add(1)
	go mi.syncLoop(ctx)

	return nil
}

// Stop stops the mesh integration
func (mi *MeshIntegration) Stop() {
	close(mi.stopCh)

	mi.mu.Lock()
	for _, cancel := range mi.watchers {
		cancel()
	}
	mi.watchers = make(map[string]context.CancelFunc)
	mi.mu.Unlock()

	mi.wg.Wait()
}

// syncLoop periodically syncs services from the registry
func (mi *MeshIntegration) syncLoop(ctx context.Context) {
	defer mi.wg.Done()

	ticker := time.NewTicker(mi.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mi.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			mi.syncAll(ctx)
		}
	}
}

// syncAll syncs all services from the registry
func (mi *MeshIntegration) syncAll(ctx context.Context) error {
	if mi.registry == nil {
		return nil
	}

	services, err := mi.registry.ListServices(ctx, mi.config.Namespace)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	for _, name := range services {
		if err := mi.syncService(ctx, name); err != nil {
			// Log error but continue
			continue
		}
	}

	return nil
}

// syncService syncs a single service from the registry
func (mi *MeshIntegration) syncService(ctx context.Context, name string) error {
	meshSvc, err := mi.registry.Discover(ctx, name, mi.config.Namespace)
	if err != nil {
		return fmt.Errorf("discover %s: %w", name, err)
	}

	// Convert to DNS service definition
	dnsSvc := mi.convertToDNSService(meshSvc)

	// Check if service exists
	existing, err := mi.sd.GetService(name)
	if err == ErrServiceNotFound {
		// Register new service
		return mi.sd.RegisterService(dnsSvc)
	} else if err != nil {
		return err
	}

	// Update existing service
	return mi.updateService(existing, dnsSvc, meshSvc)
}

// convertToDNSService converts a mesh service to DNS service definition
func (mi *MeshIntegration) convertToDNSService(meshSvc *MeshService) *ServiceDefinition {
	protocol := meshSvc.Protocol
	if protocol == "" {
		protocol = mi.config.DefaultProtocol
	}

	svc := &ServiceDefinition{
		Name:          meshSvc.Name,
		Namespace:     meshSvc.Namespace,
		Protocol:      protocol,
		Port:          meshSvc.Port,
		Metadata:      meshSvc.Metadata,
		TTL:           mi.config.DefaultTTL,
		LoadBalancing: LoadBalanceRoundRobin,
		Endpoints:     make([]*ServiceEndpoint, 0, len(meshSvc.Instances)),
	}

	for _, inst := range meshSvc.Instances {
		ep := mi.convertToEndpoint(inst)
		if ep != nil {
			svc.Endpoints = append(svc.Endpoints, ep)
		}
	}

	return svc
}

// convertToEndpoint converts a mesh instance to a service endpoint
func (mi *MeshIntegration) convertToEndpoint(inst *MeshInstance) *ServiceEndpoint {
	ip := net.ParseIP(inst.Address)
	if ip == nil {
		// Try to resolve hostname
		ips, err := net.LookupIP(inst.Address)
		if err != nil || len(ips) == 0 {
			return nil
		}
		ip = ips[0]
	}

	weight := uint16(inst.Weight)
	if weight == 0 {
		weight = 100
	}

	health := HealthStateUnknown
	switch strings.ToLower(inst.Health) {
	case "passing", "healthy":
		health = HealthStateHealthy
	case "warning", "degraded":
		health = HealthStateDegraded
	case "critical", "unhealthy":
		health = HealthStateUnhealthy
	}

	return &ServiceEndpoint{
		ID:       inst.ID,
		Address:  ip,
		Port:     uint16(inst.Port),
		Weight:   weight,
		Health:   health,
		Metadata: inst.Metadata,
		LastSeen: inst.LastSeen,
	}
}

// updateService updates an existing service with new data
func (mi *MeshIntegration) updateService(existing, new *ServiceDefinition, meshSvc *MeshService) error {
	// Build map of existing endpoints
	existingEps := make(map[string]*ServiceEndpoint)
	for _, ep := range existing.Endpoints {
		existingEps[ep.ID] = ep
	}

	// Build map of new endpoints
	newEps := make(map[string]*ServiceEndpoint)
	for _, ep := range new.Endpoints {
		newEps[ep.ID] = ep
	}

	// Remove endpoints that no longer exist
	for id := range existingEps {
		if _, exists := newEps[id]; !exists {
			mi.sd.RemoveEndpoint(existing.Name, id)
		}
	}

	// Add or update endpoints
	for id, ep := range newEps {
		if existingEp, exists := existingEps[id]; !exists {
			// Add new endpoint
			mi.sd.AddEndpoint(existing.Name, ep)
		} else {
			// Update health if changed
			if existingEp.Health != ep.Health {
				mi.sd.UpdateEndpointHealth(existing.Name, id, ep.Health)
			}
			// Update last seen
			mi.sd.Heartbeat(existing.Name, id)
		}
	}

	return nil
}

// handleHealthChange handles health changes from the aggregator
func (mi *MeshIntegration) handleHealthChange(address string, health EndpointHealth) {
	// Find endpoint by address
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	services := mi.sd.ListServices()
	for _, svcName := range services {
		svc, err := mi.sd.GetService(svcName)
		if err != nil {
			continue
		}

		for _, ep := range svc.Endpoints {
			epAddr := fmt.Sprintf("%s:%d", ep.Address.String(), ep.Port)
			if epAddr == address || ep.Address.String() == address {
				var newHealth HealthState
				if health.Healthy {
					newHealth = HealthStateHealthy
				} else if health.FailureCount > 0 && health.FailureCount < 3 {
					newHealth = HealthStateDegraded
				} else {
					newHealth = HealthStateUnhealthy
				}
				mi.sd.UpdateEndpointHealth(svcName, ep.ID, newHealth)
				return
			}
		}
	}
}

// WatchService starts watching a service for changes
func (mi *MeshIntegration) WatchService(ctx context.Context, name string) error {
	if mi.registry == nil {
		return fmt.Errorf("no registry configured")
	}

	mi.mu.Lock()
	if _, exists := mi.watchers[name]; exists {
		mi.mu.Unlock()
		return nil // Already watching
	}

	watchCtx, cancel := context.WithCancel(ctx)
	mi.watchers[name] = cancel
	mi.mu.Unlock()

	mi.wg.Add(1)
	go mi.watchLoop(watchCtx, name)

	return nil
}

// StopWatching stops watching a service
func (mi *MeshIntegration) StopWatching(name string) {
	mi.mu.Lock()
	if cancel, exists := mi.watchers[name]; exists {
		cancel()
		delete(mi.watchers, name)
	}
	mi.mu.Unlock()
}

// watchLoop watches a service for changes
func (mi *MeshIntegration) watchLoop(ctx context.Context, name string) {
	defer mi.wg.Done()

	ch, err := mi.registry.Watch(ctx, name, mi.config.Namespace)
	if err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-mi.stopCh:
			return
		case meshSvc, ok := <-ch:
			if !ok {
				return
			}
			mi.handleServiceUpdate(meshSvc)
		}
	}
}

// handleServiceUpdate handles a service update from the registry
func (mi *MeshIntegration) handleServiceUpdate(meshSvc *MeshService) {
	dnsSvc := mi.convertToDNSService(meshSvc)

	existing, err := mi.sd.GetService(meshSvc.Name)
	if err == ErrServiceNotFound {
		mi.sd.RegisterService(dnsSvc)
		return
	}

	mi.updateService(existing, dnsSvc, meshSvc)
}

// ExportToDNS exports a DNS service to the mesh registry
func (mi *MeshIntegration) ExportToDNS(ctx context.Context, svc *ServiceDefinition) error {
	if mi.registry == nil || !mi.config.AutoRegister {
		return nil
	}

	meshSvc := mi.convertToMeshService(svc)
	return mi.registry.Register(ctx, meshSvc)
}

// convertToMeshService converts a DNS service to mesh service
func (mi *MeshIntegration) convertToMeshService(svc *ServiceDefinition) *MeshService {
	meshSvc := &MeshService{
		Name:      svc.Name,
		Namespace: svc.Namespace,
		Protocol:  svc.Protocol,
		Port:      svc.Port,
		Metadata:  svc.Metadata,
		Instances: make([]*MeshInstance, 0, len(svc.Endpoints)),
	}

	for _, ep := range svc.Endpoints {
		health := "unknown"
		switch ep.Health {
		case HealthStateHealthy:
			health = "passing"
		case HealthStateDegraded:
			health = "warning"
		case HealthStateUnhealthy:
			health = "critical"
		}

		meshSvc.Instances = append(meshSvc.Instances, &MeshInstance{
			ID:       ep.ID,
			Address:  ep.Address.String(),
			Port:     int(ep.Port),
			Health:   health,
			Weight:   int(ep.Weight),
			Metadata: ep.Metadata,
			LastSeen: ep.LastSeen,
		})
	}

	return meshSvc
}

// DNSHandler returns a handler that integrates with mesh health
type DNSHandler struct {
	sd       *EnhancedServiceDiscovery
	mi       *MeshIntegration
	resolver *Resolver
	zones    *ZoneManager
	metrics  *DNSServerMetrics
}

// NewDNSHandler creates a new DNS handler with mesh integration
func NewDNSHandler(
	sd *EnhancedServiceDiscovery,
	mi *MeshIntegration,
	resolver *Resolver,
) *DNSHandler {
	return &DNSHandler{
		sd:       sd,
		mi:       mi,
		resolver: resolver,
		zones:    NewZoneManager(),
		metrics:  NewDNSServerMetrics(),
	}
}

// ServeDNS handles a DNS request with mesh awareness
func (h *DNSHandler) ServeDNS(ctx context.Context, w ResponseWriter, r *Message) {
	start := time.Now()

	if len(r.Questions) == 0 {
		resp := NewResponse(r)
		resp.Header.SetRcode(RcodeFormatError)
		w.WriteMsg(resp)
		return
	}

	q := r.Questions[0]

	// Try service discovery first
	if h.sd != nil {
		resp, handled := h.handleServiceQuery(r, q)
		if handled {
			w.WriteMsg(resp)
			h.recordMetrics(q.Type, resp.Header.Rcode(), time.Since(start), false)
			return
		}
	}

	// Fall back to resolver
	if h.resolver != nil {
		resp, err := h.resolver.Resolve(ctx, r)
		if err == nil && resp != nil {
			w.WriteMsg(resp)
			h.recordMetrics(q.Type, resp.Header.Rcode(), time.Since(start), true)
			return
		}
	}

	// NXDOMAIN
	resp := NewResponse(r)
	resp.Header.SetRcode(RcodeNameError)
	w.WriteMsg(resp)
	h.recordMetrics(q.Type, RcodeNameError, time.Since(start), false)
}

// handleServiceQuery handles a query for a service
func (h *DNSHandler) handleServiceQuery(query *Message, q Question) (*Message, bool) {
	name := strings.ToLower(q.Name)
	zone := h.sd.Zone()

	// Check if in our zone
	if !strings.HasSuffix(name, "."+zone.Name) && name != zone.Name {
		return nil, false
	}

	resp := NewResponse(query)

	switch q.Type {
	case TypeSRV:
		return h.handleSRVQuery(resp, q)
	case TypeA, TypeAAAA:
		return h.handleAddressQuery(resp, q)
	case TypeTXT:
		return h.handleTXTQuery(resp, q)
	case TypePTR:
		return h.handlePTRQuery(resp, q)
	default:
		return nil, false
	}
}

func (h *DNSHandler) handleSRVQuery(resp *Message, q Question) (*Message, bool) {
	name := strings.ToLower(q.Name)
	zone := h.sd.Zone()

	// Parse service type from name: _service._proto.domain
	parts := strings.Split(name, ".")
	if len(parts) < 3 {
		return nil, false
	}

	if !strings.HasPrefix(parts[0], "_") || !strings.HasPrefix(parts[1], "_") {
		return nil, false
	}

	serviceName := strings.TrimPrefix(parts[0], "_")
	protocol := strings.TrimPrefix(parts[1], "_")

	results, err := h.sd.LookupSRV(context.Background(), serviceName, protocol)
	if err != nil {
		resp.Header.SetRcode(RcodeNameError)
		resp.Authority = append(resp.Authority, *zone.GetSOA())
		return resp, true
	}

	for _, r := range results {
		resp.Answers = append(resp.Answers, *NewSRVRecord(
			q.Name,
			30,
			r.Priority,
			r.Weight,
			r.Port,
			r.Target,
		))

		// Add A record in additional section
		if r.Address != nil {
			if r.Address.To4() != nil {
				resp.Additional = append(resp.Additional, *NewARecord(r.Target, 30, r.Address))
			} else {
				resp.Additional = append(resp.Additional, *NewAAAARecord(r.Target, 30, r.Address))
			}
		}
	}

	return resp, true
}

func (h *DNSHandler) handleAddressQuery(resp *Message, q Question) (*Message, bool) {
	name := strings.ToLower(q.Name)
	zone := h.sd.Zone()

	// Extract service name from domain
	suffix := "." + zone.Name
	if !strings.HasSuffix(name, suffix) {
		return nil, false
	}

	serviceName := strings.TrimSuffix(name, suffix)
	if serviceName == "" {
		return nil, false
	}

	endpoints, err := h.sd.ResolveService(serviceName)
	if err != nil {
		resp.Header.SetRcode(RcodeNameError)
		resp.Authority = append(resp.Authority, *zone.GetSOA())
		return resp, true
	}

	for _, ep := range endpoints {
		if q.Type == TypeA && ep.Address.To4() != nil {
			resp.Answers = append(resp.Answers, *NewARecord(q.Name, 30, ep.Address))
		} else if q.Type == TypeAAAA && ep.Address.To4() == nil {
			resp.Answers = append(resp.Answers, *NewAAAARecord(q.Name, 30, ep.Address))
		}
	}

	return resp, true
}

func (h *DNSHandler) handleTXTQuery(resp *Message, q Question) (*Message, bool) {
	zone := h.sd.Zone()
	records := zone.GetRecords(q.Name, TypeTXT)

	if len(records) == 0 {
		return nil, false
	}

	for _, rr := range records {
		resp.Answers = append(resp.Answers, *rr)
	}

	return resp, true
}

func (h *DNSHandler) handlePTRQuery(resp *Message, q Question) (*Message, bool) {
	zone := h.sd.Zone()
	records := zone.GetRecords(q.Name, TypePTR)

	if len(records) == 0 {
		return nil, false
	}

	for _, rr := range records {
		resp.Answers = append(resp.Answers, *rr)
	}

	return resp, true
}

func (h *DNSHandler) recordMetrics(qtype uint16, rcode uint8, latency time.Duration, cached bool) {
	if h.metrics != nil {
		h.metrics.RecordQuery(qtype, rcode, latency, cached)
	}
}
