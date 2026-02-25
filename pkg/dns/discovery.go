package dns

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ServiceDiscovery provides DNS-based service discovery
type ServiceDiscovery struct {
	mu            sync.RWMutex
	zone          *DynamicZone
	services      map[string]*Service
	healthChecker *HealthChecker
	defaultTTL    uint32
	roundRobinIdx map[string]*uint32
}

// Service represents a discoverable service
type Service struct {
	Name        string              // Service name (e.g., "api", "database")
	Domain      string              // Full domain (e.g., "api.kingdom.local")
	Protocol    string              // Protocol (tcp, udp)
	Port        uint16              // Service port
	Endpoints   []*Endpoint         // Service endpoints
	Metadata    map[string]string   // Service metadata as TXT records
	Priority    uint16              // SRV priority
	Weight      uint16              // SRV weight
	TTL         uint32              // Record TTL
	HealthCheck *HealthCheckConfig  // Health check configuration
}

// Endpoint represents a service endpoint
type Endpoint struct {
	ID       string            // Unique endpoint ID
	Address  net.IP            // Endpoint IP address
	Port     uint16            // Endpoint port (may differ from service port)
	Healthy  bool              // Health status
	Weight   uint16            // Endpoint weight for load balancing
	Metadata map[string]string // Endpoint-specific metadata
	LastSeen time.Time         // Last health check time
}

// HealthCheckConfig holds health check configuration
type HealthCheckConfig struct {
	Type        string        // Health check type (tcp, http, dns)
	Interval    time.Duration // Check interval
	Timeout     time.Duration // Check timeout
	Retries     int           // Number of retries before unhealthy
	Path        string        // HTTP health check path
	ExpectedTXT string        // Expected TXT record for DNS check
}

// NewServiceDiscovery creates a new service discovery instance
func NewServiceDiscovery(config *ServiceDiscoveryConfig) (*ServiceDiscovery, error) {
	if config == nil {
		config = DefaultServiceDiscoveryConfig()
	}

	zoneConfig := DefaultZoneConfig(config.Zone)
	zoneConfig.DefaultTTL = config.DefaultTTL

	zone, err := NewDynamicZone(zoneConfig)
	if err != nil {
		return nil, err
	}

	sd := &ServiceDiscovery{
		zone:          zone,
		services:      make(map[string]*Service),
		healthChecker: NewHealthChecker(config.HealthCheckInterval),
		defaultTTL:    config.DefaultTTL,
		roundRobinIdx: make(map[string]*uint32),
	}

	// Add zone update hook
	zone.AddUpdateHook(func(z *Zone, action string, record *ResourceRecord) {
		// Could trigger notifications here
	})

	return sd, nil
}

// Zone returns the underlying zone
func (sd *ServiceDiscovery) Zone() *DynamicZone {
	return sd.zone
}

// RegisterService registers a new service
func (sd *ServiceDiscovery) RegisterService(svc *Service) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Normalize service name
	svc.Name = strings.ToLower(svc.Name)
	if svc.Domain == "" {
		svc.Domain = svc.Name + "." + sd.zone.Name
	}

	if svc.TTL == 0 {
		svc.TTL = sd.defaultTTL
	}

	if svc.Protocol == "" {
		svc.Protocol = "tcp"
	}

	// Store service
	sd.services[svc.Name] = svc

	// Initialize round-robin index
	idx := uint32(0)
	sd.roundRobinIdx[svc.Name] = &idx

	// Create DNS records
	if err := sd.updateServiceRecords(svc); err != nil {
		return err
	}

	// Start health checking if configured
	if svc.HealthCheck != nil {
		sd.healthChecker.AddService(svc, func() {
			sd.mu.Lock()
			defer sd.mu.Unlock()
			sd.updateServiceRecords(svc)
		})
	}

	return nil
}

// DeregisterService removes a service
func (sd *ServiceDiscovery) DeregisterService(name string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	name = strings.ToLower(name)
	svc, exists := sd.services[name]
	if !exists {
		return fmt.Errorf("service not found: %s", name)
	}

	// Stop health checks
	sd.healthChecker.RemoveService(name)

	// Remove DNS records
	sd.zone.RemoveRecordsByName(svc.Domain, TypeANY)

	// Remove SRV record
	srvName := fmt.Sprintf("_%s._%s.%s", svc.Name, svc.Protocol, sd.zone.Name)
	sd.zone.RemoveRecordsByName(srvName, TypeSRV)

	// Remove PTR record for browsing
	sd.zone.RemoveRecordsByName("_services._dns-sd._udp."+sd.zone.Name, TypePTR)

	delete(sd.services, name)
	delete(sd.roundRobinIdx, name)

	return nil
}

// AddEndpoint adds an endpoint to a service
func (sd *ServiceDiscovery) AddEndpoint(serviceName string, endpoint *Endpoint) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	svc, exists := sd.services[serviceName]
	if !exists {
		return fmt.Errorf("service not found: %s", serviceName)
	}

	// Generate ID if not provided
	if endpoint.ID == "" {
		endpoint.ID = fmt.Sprintf("%s-%s-%d", serviceName, endpoint.Address.String(), endpoint.Port)
	}

	// Check for duplicate
	for _, ep := range svc.Endpoints {
		if ep.ID == endpoint.ID {
			return fmt.Errorf("endpoint already exists: %s", endpoint.ID)
		}
	}

	endpoint.Healthy = true
	endpoint.LastSeen = time.Now()
	svc.Endpoints = append(svc.Endpoints, endpoint)

	return sd.updateServiceRecords(svc)
}

// RemoveEndpoint removes an endpoint from a service
func (sd *ServiceDiscovery) RemoveEndpoint(serviceName, endpointID string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	svc, exists := sd.services[serviceName]
	if !exists {
		return fmt.Errorf("service not found: %s", serviceName)
	}

	var remaining []*Endpoint
	found := false
	for _, ep := range svc.Endpoints {
		if ep.ID == endpointID {
			found = true
		} else {
			remaining = append(remaining, ep)
		}
	}

	if !found {
		return fmt.Errorf("endpoint not found: %s", endpointID)
	}

	svc.Endpoints = remaining
	return sd.updateServiceRecords(svc)
}

// SetEndpointHealth sets the health status of an endpoint
func (sd *ServiceDiscovery) SetEndpointHealth(serviceName, endpointID string, healthy bool) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	svc, exists := sd.services[serviceName]
	if !exists {
		return fmt.Errorf("service not found: %s", serviceName)
	}

	for _, ep := range svc.Endpoints {
		if ep.ID == endpointID {
			if ep.Healthy != healthy {
				ep.Healthy = healthy
				ep.LastSeen = time.Now()
				return sd.updateServiceRecords(svc)
			}
			return nil
		}
	}

	return fmt.Errorf("endpoint not found: %s", endpointID)
}

// updateServiceRecords updates DNS records for a service
func (sd *ServiceDiscovery) updateServiceRecords(svc *Service) error {
	// Get healthy endpoints
	var healthyEndpoints []*Endpoint
	for _, ep := range svc.Endpoints {
		if ep.Healthy {
			healthyEndpoints = append(healthyEndpoints, ep)
		}
	}

	// Update A/AAAA records
	var aRecords, aaaaRecords []*ResourceRecord
	for _, ep := range healthyEndpoints {
		if ep.Address.To4() != nil {
			aRecords = append(aRecords, NewARecord(svc.Domain, svc.TTL, ep.Address))
		} else {
			aaaaRecords = append(aaaaRecords, NewAAAARecord(svc.Domain, svc.TTL, ep.Address))
		}
	}

	sd.zone.ReplaceRecords(svc.Domain, TypeA, aRecords)
	sd.zone.ReplaceRecords(svc.Domain, TypeAAAA, aaaaRecords)

	// Update SRV records
	srvName := fmt.Sprintf("_%s._%s.%s", svc.Name, svc.Protocol, sd.zone.Name)
	var srvRecords []*ResourceRecord
	for _, ep := range healthyEndpoints {
		port := svc.Port
		if ep.Port != 0 {
			port = ep.Port
		}
		weight := svc.Weight
		if ep.Weight != 0 {
			weight = ep.Weight
		}
		srvRecords = append(srvRecords, NewSRVRecord(srvName, svc.TTL, svc.Priority, weight, port, svc.Domain))
	}
	sd.zone.ReplaceRecords(srvName, TypeSRV, srvRecords)

	// Update TXT records (metadata)
	if len(svc.Metadata) > 0 {
		var txtStrings []string
		for k, v := range svc.Metadata {
			txtStrings = append(txtStrings, k+"="+v)
		}
		sort.Strings(txtStrings)
		sd.zone.ReplaceRecords(svc.Domain, TypeTXT, []*ResourceRecord{
			NewTXTRecord(svc.Domain, svc.TTL, txtStrings...),
		})
	}

	// Update PTR for DNS-SD browsing
	browseName := "_services._dns-sd._udp." + sd.zone.Name
	servicePTR := fmt.Sprintf("_%s._%s.%s", svc.Name, svc.Protocol, sd.zone.Name)

	// Get existing PTR records
	existing := sd.zone.GetRecords(browseName, TypePTR)
	found := false
	for _, rr := range existing {
		if rr.GetTarget() == servicePTR {
			found = true
			break
		}
	}
	if !found {
		sd.zone.AddRecord(NewPTRRecord(browseName, svc.TTL, servicePTR))
	}

	// Add PTR from service type to service instances
	instancePTR := svc.Name + "." + svc.Domain
	sd.zone.AddRecord(NewPTRRecord(servicePTR, svc.TTL, instancePTR))

	return nil
}

// LookupService returns healthy endpoints for a service
func (sd *ServiceDiscovery) LookupService(name string) ([]*Endpoint, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	name = strings.ToLower(name)
	svc, exists := sd.services[name]
	if !exists {
		return nil, fmt.Errorf("service not found: %s", name)
	}

	var healthy []*Endpoint
	for _, ep := range svc.Endpoints {
		if ep.Healthy {
			healthy = append(healthy, ep)
		}
	}

	return healthy, nil
}

// LookupServiceRoundRobin returns the next healthy endpoint in round-robin order
func (sd *ServiceDiscovery) LookupServiceRoundRobin(name string) (*Endpoint, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	name = strings.ToLower(name)
	svc, exists := sd.services[name]
	if !exists {
		return nil, fmt.Errorf("service not found: %s", name)
	}

	var healthy []*Endpoint
	for _, ep := range svc.Endpoints {
		if ep.Healthy {
			healthy = append(healthy, ep)
		}
	}

	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy endpoints for service: %s", name)
	}

	idx := sd.roundRobinIdx[name]
	current := atomic.AddUint32(idx, 1)
	return healthy[int(current)%len(healthy)], nil
}

// GetService returns service information
func (sd *ServiceDiscovery) GetService(name string) (*Service, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	name = strings.ToLower(name)
	svc, exists := sd.services[name]
	if !exists {
		return nil, fmt.Errorf("service not found: %s", name)
	}

	// Return a copy
	copy := *svc
	copy.Endpoints = make([]*Endpoint, len(svc.Endpoints))
	for i, ep := range svc.Endpoints {
		epCopy := *ep
		copy.Endpoints[i] = &epCopy
	}

	return &copy, nil
}

// ListServices returns all registered services
func (sd *ServiceDiscovery) ListServices() []string {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	names := make([]string, 0, len(sd.services))
	for name := range sd.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Close stops the service discovery
func (sd *ServiceDiscovery) Close() error {
	sd.healthChecker.Stop()
	return nil
}

// HealthChecker performs health checks on service endpoints
type HealthChecker struct {
	mu       sync.RWMutex
	services map[string]*healthCheckEntry
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
}

type healthCheckEntry struct {
	service  *Service
	callback func()
	stop     chan struct{}
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(defaultInterval time.Duration) *HealthChecker {
	return &HealthChecker{
		services: make(map[string]*healthCheckEntry),
		interval: defaultInterval,
		stop:     make(chan struct{}),
	}
}

// AddService adds a service for health checking
func (hc *HealthChecker) AddService(svc *Service, callback func()) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	entry := &healthCheckEntry{
		service:  svc,
		callback: callback,
		stop:     make(chan struct{}),
	}

	hc.services[svc.Name] = entry

	hc.wg.Add(1)
	go hc.checkLoop(entry)
}

// RemoveService removes a service from health checking
func (hc *HealthChecker) RemoveService(name string) {
	hc.mu.Lock()
	entry, exists := hc.services[name]
	if exists {
		close(entry.stop)
		delete(hc.services, name)
	}
	hc.mu.Unlock()
}

// Stop stops all health checks
func (hc *HealthChecker) Stop() {
	hc.mu.Lock()
	for _, entry := range hc.services {
		close(entry.stop)
	}
	hc.services = make(map[string]*healthCheckEntry)
	hc.mu.Unlock()

	hc.wg.Wait()
}

// checkLoop runs health checks for a service
func (hc *HealthChecker) checkLoop(entry *healthCheckEntry) {
	defer hc.wg.Done()

	interval := hc.interval
	if entry.service.HealthCheck != nil && entry.service.HealthCheck.Interval > 0 {
		interval = entry.service.HealthCheck.Interval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-entry.stop:
			return
		case <-ticker.C:
			changed := hc.checkService(entry.service)
			if changed && entry.callback != nil {
				entry.callback()
			}
		}
	}
}

// checkService checks all endpoints of a service
func (hc *HealthChecker) checkService(svc *Service) bool {
	if svc.HealthCheck == nil {
		return false
	}

	changed := false
	for _, ep := range svc.Endpoints {
		wasHealthy := ep.Healthy
		ep.Healthy = hc.checkEndpoint(svc, ep)
		ep.LastSeen = time.Now()
		if ep.Healthy != wasHealthy {
			changed = true
		}
	}

	return changed
}

// checkEndpoint performs a health check on an endpoint
func (hc *HealthChecker) checkEndpoint(svc *Service, ep *Endpoint) bool {
	config := svc.HealthCheck
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	retries := config.Retries
	if retries == 0 {
		retries = 1
	}

	for i := 0; i < retries; i++ {
		if hc.doCheck(config.Type, ep.Address, ep.Port, timeout, config) {
			return true
		}
	}

	return false
}

// doCheck performs the actual health check
func (hc *HealthChecker) doCheck(checkType string, addr net.IP, port uint16, timeout time.Duration, config *HealthCheckConfig) bool {
	switch checkType {
	case "tcp":
		return hc.tcpCheck(addr, port, timeout)
	case "http":
		return hc.httpCheck(addr, port, timeout, config.Path)
	default:
		return hc.tcpCheck(addr, port, timeout)
	}
}

// tcpCheck performs a TCP health check
func (hc *HealthChecker) tcpCheck(addr net.IP, port uint16, timeout time.Duration) bool {
	var address string
	if addr.To4() != nil {
		address = fmt.Sprintf("%s:%d", addr.String(), port)
	} else {
		address = fmt.Sprintf("[%s]:%d", addr.String(), port)
	}
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// httpCheck performs an HTTP health check
func (hc *HealthChecker) httpCheck(addr net.IP, port uint16, timeout time.Duration, path string) bool {
	if path == "" {
		path = "/health"
	}

	// Simple TCP check for now (full HTTP would need http package)
	return hc.tcpCheck(addr, port, timeout)
}

// DNSServiceBrowser provides DNS-SD browsing capabilities
type DNSServiceBrowser struct {
	resolver *Resolver
	domain   string
}

// NewDNSServiceBrowser creates a new DNS service browser
func NewDNSServiceBrowser(resolver *Resolver, domain string) *DNSServiceBrowser {
	return &DNSServiceBrowser{
		resolver: resolver,
		domain:   domain,
	}
}

// Browse lists all available service types
func (b *DNSServiceBrowser) Browse(ctx context.Context) ([]string, error) {
	browseName := "_services._dns-sd._udp." + b.domain

	resp, err := b.resolver.ResolveQuery(ctx, browseName, TypePTR)
	if err != nil {
		return nil, err
	}

	var services []string
	for _, rr := range resp.Answers {
		if rr.Type == TypePTR {
			services = append(services, rr.GetTarget())
		}
	}

	return services, nil
}

// LookupService looks up instances of a service type
func (b *DNSServiceBrowser) LookupService(ctx context.Context, serviceType string) ([]*ServiceInstance, error) {
	// Get PTR records for service instances
	resp, err := b.resolver.ResolveQuery(ctx, serviceType, TypePTR)
	if err != nil {
		return nil, err
	}

	var instances []*ServiceInstance
	for _, rr := range resp.Answers {
		if rr.Type == TypePTR {
			instanceName := rr.GetTarget()
			instance, err := b.lookupInstance(ctx, instanceName)
			if err != nil {
				continue
			}
			instances = append(instances, instance)
		}
	}

	return instances, nil
}

// ServiceInstance represents a discovered service instance
type ServiceInstance struct {
	Name      string
	Target    string
	Port      uint16
	Priority  uint16
	Weight    uint16
	Addresses []net.IP
	TXT       map[string]string
}

// lookupInstance looks up a service instance
func (b *DNSServiceBrowser) lookupInstance(ctx context.Context, name string) (*ServiceInstance, error) {
	instance := &ServiceInstance{
		Name: name,
		TXT:  make(map[string]string),
	}

	// Get SRV record
	srvResp, err := b.resolver.ResolveQuery(ctx, name, TypeSRV)
	if err == nil && len(srvResp.Answers) > 0 {
		if srv, ok := srvResp.Answers[0].Data.(*SRVRecord); ok {
			instance.Target = srv.Target
			instance.Port = srv.Port
			instance.Priority = srv.Priority
			instance.Weight = srv.Weight
		}
	}

	// Get TXT records
	txtResp, err := b.resolver.ResolveQuery(ctx, name, TypeTXT)
	if err == nil {
		for _, rr := range txtResp.Answers {
			if txt, ok := rr.Data.(*TXTRecord); ok {
				for _, t := range txt.Text {
					if idx := strings.Index(t, "="); idx != -1 {
						instance.TXT[t[:idx]] = t[idx+1:]
					}
				}
			}
		}
	}

	// Get A/AAAA records for target
	if instance.Target != "" {
		aResp, _ := b.resolver.ResolveQuery(ctx, instance.Target, TypeA)
		for _, rr := range aResp.Answers {
			if ip := rr.GetIP(); ip != nil {
				instance.Addresses = append(instance.Addresses, ip)
			}
		}

		aaaaResp, _ := b.resolver.ResolveQuery(ctx, instance.Target, TypeAAAA)
		for _, rr := range aaaaResp.Answers {
			if ip := rr.GetIP(); ip != nil {
				instance.Addresses = append(instance.Addresses, ip)
			}
		}
	}

	return instance, nil
}

// ReverseDiscovery provides reverse DNS lookup for service discovery
type ReverseDiscovery struct {
	sd *ServiceDiscovery
}

// NewReverseDiscovery creates a reverse discovery helper
func NewReverseDiscovery(sd *ServiceDiscovery) *ReverseDiscovery {
	return &ReverseDiscovery{sd: sd}
}

// RegisterReverse registers reverse DNS (PTR) records for a service
func (rd *ReverseDiscovery) RegisterReverse(serviceName string) error {
	svc, err := rd.sd.GetService(serviceName)
	if err != nil {
		return err
	}

	for _, ep := range svc.Endpoints {
		ptrName := rd.reverseIP(ep.Address)
		if ptrName == "" {
			continue
		}

		rd.sd.zone.AddRecord(NewPTRRecord(ptrName, svc.TTL, svc.Domain))
	}

	return nil
}

// reverseIP creates a reverse DNS name for an IP
func (rd *ReverseDiscovery) reverseIP(ip net.IP) string {
	if ip4 := ip.To4(); ip4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa",
			ip4[3], ip4[2], ip4[1], ip4[0])
	}

	// IPv6 reverse DNS
	if ip6 := ip.To16(); ip6 != nil {
		var parts []string
		for i := len(ip6) - 1; i >= 0; i-- {
			parts = append(parts, fmt.Sprintf("%x", ip6[i]&0x0f))
			parts = append(parts, fmt.Sprintf("%x", ip6[i]>>4))
		}
		return strings.Join(parts, ".") + ".ip6.arpa"
	}

	return ""
}
