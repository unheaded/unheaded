// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"context"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DNSDiscoverer implements service discovery using DNS.
type DNSDiscoverer struct {
	resolver    *net.Resolver
	domain      string
	defaultPort int
	refreshRate time.Duration

	mu       sync.RWMutex
	cache    map[string]*dnsCache
	watchers map[string][]chan *Service
	done     chan struct{}
}

type dnsCache struct {
	service   *Service
	expiresAt time.Time
}

// DNSConfig configures DNS-based discovery.
type DNSConfig struct {
	// Domain suffix for service names (e.g., "svc.cluster.local")
	Domain string
	// DefaultPort when not using SRV records
	DefaultPort int
	// RefreshRate for DNS polling
	RefreshRate time.Duration
	// Resolver to use (nil for default)
	Resolver *net.Resolver
}

// DefaultDNSConfig returns a default DNS configuration.
func DefaultDNSConfig() *DNSConfig {
	return &DNSConfig{
		Domain:      "svc.unheaded.local",
		DefaultPort: 8080,
		RefreshRate: 30 * time.Second,
	}
}

// NewDNSDiscoverer creates a new DNS-based discoverer.
func NewDNSDiscoverer(config *DNSConfig) *DNSDiscoverer {
	if config == nil {
		config = DefaultDNSConfig()
	}

	resolver := config.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	d := &DNSDiscoverer{
		resolver:    resolver,
		domain:      config.Domain,
		defaultPort: config.DefaultPort,
		refreshRate: config.RefreshRate,
		cache:       make(map[string]*dnsCache),
		watchers:    make(map[string][]chan *Service),
		done:        make(chan struct{}),
	}

	// Start background refresh
	go d.refreshLoop()

	return d
}

// Discover resolves a service using DNS.
func (d *DNSDiscoverer) Discover(ctx context.Context, service, namespace string) (*Service, error) {
	key := namespace + "/" + service

	// Check cache
	d.mu.RLock()
	if cached, ok := d.cache[key]; ok && time.Now().Before(cached.expiresAt) {
		d.mu.RUnlock()
		return cached.service, nil
	}
	d.mu.RUnlock()

	// Build DNS name
	dnsName := d.buildDNSName(service, namespace)

	// Try SRV records first
	svc, err := d.resolveSRV(ctx, dnsName, service, namespace)
	if err != nil {
		// Fall back to A/AAAA records
		svc, err = d.resolveA(ctx, dnsName, service, namespace)
		if err != nil {
			return nil, err
		}
	}

	// Update cache
	d.mu.Lock()
	d.cache[key] = &dnsCache{
		service:   svc,
		expiresAt: time.Now().Add(d.refreshRate),
	}
	d.mu.Unlock()

	return svc, nil
}

// Watch returns a channel for service updates.
func (d *DNSDiscoverer) Watch(ctx context.Context, service, namespace string) (<-chan *Service, error) {
	key := namespace + "/" + service
	ch := make(chan *Service, 10)

	d.mu.Lock()
	d.watchers[key] = append(d.watchers[key], ch)
	d.mu.Unlock()

	// Send current state
	go func() {
		svc, err := d.Discover(ctx, service, namespace)
		if err == nil {
			select {
			case ch <- svc:
			case <-ctx.Done():
			}
		}
	}()

	// Clean up on context cancellation
	go func() {
		<-ctx.Done()
		d.mu.Lock()
		watchers := d.watchers[key]
		for i, w := range watchers {
			if w == ch {
				d.watchers[key] = append(watchers[:i], watchers[i+1:]...)
				close(ch)
				break
			}
		}
		d.mu.Unlock()
	}()

	return ch, nil
}

// Close shuts down the DNS discoverer.
func (d *DNSDiscoverer) Close() error {
	close(d.done)
	return nil
}

func (d *DNSDiscoverer) buildDNSName(service, namespace string) string {
	// Format: service.namespace.domain
	return service + "." + namespace + "." + d.domain
}

func (d *DNSDiscoverer) resolveSRV(ctx context.Context, name, service, namespace string) (*Service, error) {
	// SRV format: _service._tcp.name
	srvName := "_" + service + "._tcp." + name

	_, addrs, err := d.resolver.LookupSRV(ctx, service, "tcp", name)
	if err != nil {
		// Try without underscore prefix
		_, addrs, err = d.resolver.LookupSRV(ctx, "", "", srvName)
		if err != nil {
			return nil, err
		}
	}

	if len(addrs) == 0 {
		return nil, ErrNoEndpoints
	}

	endpoints := make([]*Endpoint, 0, len(addrs))
	for _, addr := range addrs {
		// Resolve the target hostname
		ips, err := d.resolver.LookupIPAddr(ctx, strings.TrimSuffix(addr.Target, "."))
		if err != nil {
			continue
		}

		for _, ip := range ips {
			endpoints = append(endpoints, &Endpoint{
				Address:  ip.String(),
				Port:     int(addr.Port),
				Weight:   int(addr.Weight),
				Healthy:  true,
				LastSeen: time.Now(),
			})
		}
	}

	if len(endpoints) == 0 {
		return nil, ErrNoEndpoints
	}

	return &Service{
		Name:      service,
		Namespace: namespace,
		Endpoints: endpoints,
		Protocol:  "tcp",
		UpdatedAt: time.Now(),
	}, nil
}

func (d *DNSDiscoverer) resolveA(ctx context.Context, name, service, namespace string) (*Service, error) {
	ips, err := d.resolver.LookupIPAddr(ctx, name)
	if err != nil {
		return nil, err
	}

	if len(ips) == 0 {
		return nil, ErrNoEndpoints
	}

	endpoints := make([]*Endpoint, 0, len(ips))
	for _, ip := range ips {
		endpoints = append(endpoints, &Endpoint{
			Address:  ip.String(),
			Port:     d.defaultPort,
			Weight:   1,
			Healthy:  true,
			LastSeen: time.Now(),
		})
	}

	return &Service{
		Name:      service,
		Namespace: namespace,
		Endpoints: endpoints,
		Protocol:  "tcp",
		UpdatedAt: time.Now(),
	}, nil
}

func (d *DNSDiscoverer) refreshLoop() {
	ticker := time.NewTicker(d.refreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-d.done:
			return
		case <-ticker.C:
			d.refreshAll()
		}
	}
}

func (d *DNSDiscoverer) refreshAll() {
	d.mu.RLock()
	keys := make([]string, 0, len(d.watchers))
	for key := range d.watchers {
		keys = append(keys, key)
	}
	d.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, key := range keys {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			continue
		}
		namespace, service := parts[0], parts[1]

		svc, err := d.Discover(ctx, service, namespace)
		if err != nil {
			continue
		}

		// Notify watchers
		d.mu.RLock()
		watchers := d.watchers[key]
		d.mu.RUnlock()

		for _, ch := range watchers {
			select {
			case ch <- svc:
			default:
			}
		}
	}
}

// HeadlessServiceDiscoverer handles headless service DNS patterns.
type HeadlessServiceDiscoverer struct {
	*DNSDiscoverer
}

// NewHeadlessServiceDiscoverer creates a discoverer for headless services.
func NewHeadlessServiceDiscoverer(config *DNSConfig) *HeadlessServiceDiscoverer {
	return &HeadlessServiceDiscoverer{
		DNSDiscoverer: NewDNSDiscoverer(config),
	}
}

// DiscoverPod discovers a specific pod by its DNS name.
func (h *HeadlessServiceDiscoverer) DiscoverPod(ctx context.Context, podName, service, namespace string) (*Endpoint, error) {
	// Format: pod.service.namespace.domain
	dnsName := podName + "." + service + "." + namespace + "." + h.domain

	ips, err := h.resolver.LookupIPAddr(ctx, dnsName)
	if err != nil {
		return nil, err
	}

	if len(ips) == 0 {
		return nil, ErrNoEndpoints
	}

	return &Endpoint{
		Address:  ips[0].String(),
		Port:     h.defaultPort,
		Weight:   1,
		Healthy:  true,
		LastSeen: time.Now(),
	}, nil
}

// ExternalNameDiscoverer handles ExternalName service patterns.
type ExternalNameDiscoverer struct {
	*DNSDiscoverer
	externalNames map[string]string // service -> external DNS name
	mu            sync.RWMutex
}

// NewExternalNameDiscoverer creates a discoverer for external name services.
func NewExternalNameDiscoverer(config *DNSConfig) *ExternalNameDiscoverer {
	return &ExternalNameDiscoverer{
		DNSDiscoverer: NewDNSDiscoverer(config),
		externalNames: make(map[string]string),
	}
}

// SetExternalName maps a service to an external DNS name.
func (e *ExternalNameDiscoverer) SetExternalName(service, namespace, externalName string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := namespace + "/" + service
	e.externalNames[key] = externalName
}

// Discover resolves using external name if configured.
func (e *ExternalNameDiscoverer) Discover(ctx context.Context, service, namespace string) (*Service, error) {
	key := namespace + "/" + service

	e.mu.RLock()
	externalName, ok := e.externalNames[key]
	e.mu.RUnlock()

	if ok {
		// Resolve external name
		return e.resolveA(ctx, externalName, service, namespace)
	}

	// Fall back to standard DNS
	return e.DNSDiscoverer.Discover(ctx, service, namespace)
}

// SRVEndpoint represents a parsed SRV record.
type SRVEndpoint struct {
	Target   string
	Port     uint16
	Priority uint16
	Weight   uint16
}

// ParseSRVRecord parses an SRV record string.
func ParseSRVRecord(record string) (*SRVEndpoint, error) {
	// Format: priority weight port target
	parts := strings.Fields(record)
	if len(parts) < 4 {
		return nil, ErrNoEndpoints
	}

	priority, err := strconv.ParseUint(parts[0], 10, 16)
	if err != nil {
		return nil, err
	}

	weight, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return nil, err
	}

	port, err := strconv.ParseUint(parts[2], 10, 16)
	if err != nil {
		return nil, err
	}

	return &SRVEndpoint{
		Target:   strings.TrimSuffix(parts[3], "."),
		Port:     uint16(port),
		Priority: uint16(priority),
		Weight:   uint16(weight),
	}, nil
}

// SortSRVByPriority sorts SRV endpoints by priority (lower is better).
func SortSRVByPriority(endpoints []*SRVEndpoint) {
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Priority == endpoints[j].Priority {
			return endpoints[i].Weight > endpoints[j].Weight
		}
		return endpoints[i].Priority < endpoints[j].Priority
	})
}
