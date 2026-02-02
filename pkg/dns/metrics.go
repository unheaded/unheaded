// Package dns provides DNS service discovery with metrics collection.
package dns

import (
	"sync"
	"sync/atomic"
	"time"
)

// DiscoveryMetrics collects metrics for DNS service discovery
type DiscoveryMetrics struct {
	// Query metrics
	totalQueries      uint64
	cacheHits         uint64
	cacheMisses       uint64
	srvQueries        uint64
	aQueries          uint64
	aaaaQueries       uint64
	txtQueries        uint64
	ptrQueries        uint64
	nxdomainResponses uint64

	// Service metrics
	serviceRegistrations   uint64
	serviceDeregistrations uint64
	endpointAdditions      uint64
	endpointRemovals       uint64
	healthChanges          uint64

	// Latency tracking
	mu            sync.RWMutex
	queryLatency  *latencyTracker
	serviceHealth map[string]*serviceMetrics
	queryByType   map[uint16]uint64
	queryByRcode  map[uint8]uint64

	// Time tracking
	startTime time.Time
}

type serviceMetrics struct {
	name            string
	endpointCount   int
	healthyCount    int
	totalQueries    uint64
	registeredAt    time.Time
	lastHealthCheck time.Time
}

type latencyTracker struct {
	mu      sync.Mutex
	buckets []uint64 // Histogram buckets
	sum     uint64
	count   uint64
	min     time.Duration
	max     time.Duration
}

// Latency bucket boundaries in microseconds
var latencyBuckets = []int64{
	100,    // 100us
	500,    // 500us
	1000,   // 1ms
	5000,   // 5ms
	10000,  // 10ms
	50000,  // 50ms
	100000, // 100ms
	500000, // 500ms
}

// NewDiscoveryMetrics creates a new metrics collector
func NewDiscoveryMetrics() *DiscoveryMetrics {
	return &DiscoveryMetrics{
		queryLatency:  newLatencyTracker(),
		serviceHealth: make(map[string]*serviceMetrics),
		queryByType:   make(map[uint16]uint64),
		queryByRcode:  make(map[uint8]uint64),
		startTime:     time.Now(),
	}
}

func newLatencyTracker() *latencyTracker {
	return &latencyTracker{
		buckets: make([]uint64, len(latencyBuckets)+1),
		min:     time.Hour, // Will be replaced with first observation
	}
}

// RecordQuery records a DNS query
func (m *DiscoveryMetrics) RecordQuery(qtype uint16, rcode uint8, latency time.Duration, cacheHit bool) {
	atomic.AddUint64(&m.totalQueries, 1)

	if cacheHit {
		atomic.AddUint64(&m.cacheHits, 1)
	} else {
		atomic.AddUint64(&m.cacheMisses, 1)
	}

	// Record by type
	switch qtype {
	case TypeSRV:
		atomic.AddUint64(&m.srvQueries, 1)
	case TypeA:
		atomic.AddUint64(&m.aQueries, 1)
	case TypeAAAA:
		atomic.AddUint64(&m.aaaaQueries, 1)
	case TypeTXT:
		atomic.AddUint64(&m.txtQueries, 1)
	case TypePTR:
		atomic.AddUint64(&m.ptrQueries, 1)
	}

	// Record NXDOMAIN
	if rcode == RcodeNameError {
		atomic.AddUint64(&m.nxdomainResponses, 1)
	}

	// Record detailed stats
	m.mu.Lock()
	m.queryByType[qtype]++
	m.queryByRcode[rcode]++
	m.mu.Unlock()

	// Record latency
	m.queryLatency.record(latency)
}

// RecordServiceQuery records a query for a specific service
func (m *DiscoveryMetrics) RecordServiceQuery(serviceName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if svc, ok := m.serviceHealth[serviceName]; ok {
		atomic.AddUint64(&svc.totalQueries, 1)
	}
}

// ServiceRegistered records a service registration
func (m *DiscoveryMetrics) ServiceRegistered(name string) {
	atomic.AddUint64(&m.serviceRegistrations, 1)

	m.mu.Lock()
	m.serviceHealth[name] = &serviceMetrics{
		name:         name,
		registeredAt: time.Now(),
	}
	m.mu.Unlock()
}

// ServiceDeregistered records a service deregistration
func (m *DiscoveryMetrics) ServiceDeregistered(name string) {
	atomic.AddUint64(&m.serviceDeregistrations, 1)

	m.mu.Lock()
	delete(m.serviceHealth, name)
	m.mu.Unlock()
}

// EndpointAdded records an endpoint addition
func (m *DiscoveryMetrics) EndpointAdded(serviceName, endpointID string) {
	atomic.AddUint64(&m.endpointAdditions, 1)

	m.mu.Lock()
	if svc, ok := m.serviceHealth[serviceName]; ok {
		svc.endpointCount++
		svc.healthyCount++ // Assume healthy initially
	}
	m.mu.Unlock()
}

// EndpointRemoved records an endpoint removal
func (m *DiscoveryMetrics) EndpointRemoved(serviceName, endpointID string) {
	atomic.AddUint64(&m.endpointRemovals, 1)

	m.mu.Lock()
	if svc, ok := m.serviceHealth[serviceName]; ok {
		svc.endpointCount--
		if svc.healthyCount > svc.endpointCount {
			svc.healthyCount = svc.endpointCount
		}
	}
	m.mu.Unlock()
}

// HealthChanged records a health state change
func (m *DiscoveryMetrics) HealthChanged(serviceName, endpointID string, newHealth HealthState) {
	atomic.AddUint64(&m.healthChanges, 1)

	m.mu.Lock()
	if svc, ok := m.serviceHealth[serviceName]; ok {
		svc.lastHealthCheck = time.Now()
		// Health count is recomputed on snapshot
	}
	m.mu.Unlock()
}

// UpdateServiceHealth updates the healthy endpoint count for a service
func (m *DiscoveryMetrics) UpdateServiceHealth(serviceName string, total, healthy int) {
	m.mu.Lock()
	if svc, ok := m.serviceHealth[serviceName]; ok {
		svc.endpointCount = total
		svc.healthyCount = healthy
		svc.lastHealthCheck = time.Now()
	}
	m.mu.Unlock()
}

// Snapshot returns a copy of the current metrics
func (m *DiscoveryMetrics) Snapshot() *MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := &MetricsSnapshot{
		Uptime: time.Since(m.startTime),

		TotalQueries:      atomic.LoadUint64(&m.totalQueries),
		CacheHits:         atomic.LoadUint64(&m.cacheHits),
		CacheMisses:       atomic.LoadUint64(&m.cacheMisses),
		SRVQueries:        atomic.LoadUint64(&m.srvQueries),
		AQueries:          atomic.LoadUint64(&m.aQueries),
		AAAAQueries:       atomic.LoadUint64(&m.aaaaQueries),
		TXTQueries:        atomic.LoadUint64(&m.txtQueries),
		PTRQueries:        atomic.LoadUint64(&m.ptrQueries),
		NXDOMAINResponses: atomic.LoadUint64(&m.nxdomainResponses),

		ServiceRegistrations:   atomic.LoadUint64(&m.serviceRegistrations),
		ServiceDeregistrations: atomic.LoadUint64(&m.serviceDeregistrations),
		EndpointAdditions:      atomic.LoadUint64(&m.endpointAdditions),
		EndpointRemovals:       atomic.LoadUint64(&m.endpointRemovals),
		HealthChanges:          atomic.LoadUint64(&m.healthChanges),

		QueryByType:  make(map[uint16]uint64),
		QueryByRcode: make(map[uint8]uint64),
		Services:     make(map[string]*ServiceMetricsSnapshot),
	}

	// Calculate cache hit rate
	total := snapshot.CacheHits + snapshot.CacheMisses
	if total > 0 {
		snapshot.CacheHitRate = float64(snapshot.CacheHits) / float64(total) * 100
	}

	// Copy query type distribution
	for k, v := range m.queryByType {
		snapshot.QueryByType[k] = v
	}

	// Copy rcode distribution
	for k, v := range m.queryByRcode {
		snapshot.QueryByRcode[k] = v
	}

	// Copy service metrics
	for name, svc := range m.serviceHealth {
		snapshot.Services[name] = &ServiceMetricsSnapshot{
			Name:            svc.name,
			EndpointCount:   svc.endpointCount,
			HealthyCount:    svc.healthyCount,
			TotalQueries:    atomic.LoadUint64(&svc.totalQueries),
			RegisteredAt:    svc.registeredAt,
			LastHealthCheck: svc.lastHealthCheck,
		}
	}

	// Copy latency stats
	snapshot.QueryLatency = m.queryLatency.snapshot()

	return snapshot
}

// MetricsSnapshot is a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	Uptime time.Duration

	// Query counters
	TotalQueries      uint64
	CacheHits         uint64
	CacheMisses       uint64
	CacheHitRate      float64
	SRVQueries        uint64
	AQueries          uint64
	AAAAQueries       uint64
	TXTQueries        uint64
	PTRQueries        uint64
	NXDOMAINResponses uint64

	// Service counters
	ServiceRegistrations   uint64
	ServiceDeregistrations uint64
	EndpointAdditions      uint64
	EndpointRemovals       uint64
	HealthChanges          uint64

	// Distributions
	QueryByType  map[uint16]uint64
	QueryByRcode map[uint8]uint64
	Services     map[string]*ServiceMetricsSnapshot

	// Latency
	QueryLatency *LatencySnapshot
}

// ServiceMetricsSnapshot contains metrics for a single service
type ServiceMetricsSnapshot struct {
	Name            string
	EndpointCount   int
	HealthyCount    int
	TotalQueries    uint64
	RegisteredAt    time.Time
	LastHealthCheck time.Time
}

// LatencySnapshot contains latency statistics
type LatencySnapshot struct {
	Count   uint64
	Sum     time.Duration
	Mean    time.Duration
	Min     time.Duration
	Max     time.Duration
	Buckets []LatencyBucket
}

// LatencyBucket represents a latency histogram bucket
type LatencyBucket struct {
	UpperBound time.Duration
	Count      uint64
}

func (lt *latencyTracker) record(d time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	us := d.Microseconds()
	lt.sum += uint64(us)
	lt.count++

	if d < lt.min {
		lt.min = d
	}
	if d > lt.max {
		lt.max = d
	}

	// Find bucket
	for i, boundary := range latencyBuckets {
		if us <= boundary {
			lt.buckets[i]++
			return
		}
	}
	lt.buckets[len(latencyBuckets)]++ // Overflow bucket
}

func (lt *latencyTracker) snapshot() *LatencySnapshot {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if lt.count == 0 {
		return &LatencySnapshot{}
	}

	snap := &LatencySnapshot{
		Count:   lt.count,
		Sum:     time.Duration(lt.sum) * time.Microsecond,
		Mean:    time.Duration(lt.sum/lt.count) * time.Microsecond,
		Min:     lt.min,
		Max:     lt.max,
		Buckets: make([]LatencyBucket, len(latencyBuckets)+1),
	}

	for i, boundary := range latencyBuckets {
		snap.Buckets[i] = LatencyBucket{
			UpperBound: time.Duration(boundary) * time.Microsecond,
			Count:      lt.buckets[i],
		}
	}
	snap.Buckets[len(latencyBuckets)] = LatencyBucket{
		UpperBound: 0, // Infinity
		Count:      lt.buckets[len(latencyBuckets)],
	}

	return snap
}

// Reset resets all metrics
func (m *DiscoveryMetrics) Reset() {
	atomic.StoreUint64(&m.totalQueries, 0)
	atomic.StoreUint64(&m.cacheHits, 0)
	atomic.StoreUint64(&m.cacheMisses, 0)
	atomic.StoreUint64(&m.srvQueries, 0)
	atomic.StoreUint64(&m.aQueries, 0)
	atomic.StoreUint64(&m.aaaaQueries, 0)
	atomic.StoreUint64(&m.txtQueries, 0)
	atomic.StoreUint64(&m.ptrQueries, 0)
	atomic.StoreUint64(&m.nxdomainResponses, 0)
	atomic.StoreUint64(&m.serviceRegistrations, 0)
	atomic.StoreUint64(&m.serviceDeregistrations, 0)
	atomic.StoreUint64(&m.endpointAdditions, 0)
	atomic.StoreUint64(&m.endpointRemovals, 0)
	atomic.StoreUint64(&m.healthChanges, 0)

	m.mu.Lock()
	m.queryLatency = newLatencyTracker()
	m.queryByType = make(map[uint16]uint64)
	m.queryByRcode = make(map[uint8]uint64)
	// Keep service health data
	m.startTime = time.Now()
	m.mu.Unlock()
}

// DNSServerMetrics extends DiscoveryMetrics for DNS server metrics
type DNSServerMetrics struct {
	*DiscoveryMetrics

	// Server-specific metrics
	udpQueries       uint64
	tcpQueries       uint64
	udpErrors        uint64
	tcpErrors        uint64
	truncatedQueries uint64
	refusedQueries   uint64
	recursiveQueries uint64
	forwardedQueries uint64
	forwardErrors    uint64
}

// NewDNSServerMetrics creates new server metrics
func NewDNSServerMetrics() *DNSServerMetrics {
	return &DNSServerMetrics{
		DiscoveryMetrics: NewDiscoveryMetrics(),
	}
}

// RecordUDPQuery records a UDP query
func (m *DNSServerMetrics) RecordUDPQuery() {
	atomic.AddUint64(&m.udpQueries, 1)
}

// RecordTCPQuery records a TCP query
func (m *DNSServerMetrics) RecordTCPQuery() {
	atomic.AddUint64(&m.tcpQueries, 1)
}

// RecordUDPError records a UDP error
func (m *DNSServerMetrics) RecordUDPError() {
	atomic.AddUint64(&m.udpErrors, 1)
}

// RecordTCPError records a TCP error
func (m *DNSServerMetrics) RecordTCPError() {
	atomic.AddUint64(&m.tcpErrors, 1)
}

// RecordTruncated records a truncated response
func (m *DNSServerMetrics) RecordTruncated() {
	atomic.AddUint64(&m.truncatedQueries, 1)
}

// RecordRefused records a refused query
func (m *DNSServerMetrics) RecordRefused() {
	atomic.AddUint64(&m.refusedQueries, 1)
}

// RecordRecursive records a recursive query
func (m *DNSServerMetrics) RecordRecursive() {
	atomic.AddUint64(&m.recursiveQueries, 1)
}

// RecordForwarded records a forwarded query
func (m *DNSServerMetrics) RecordForwarded() {
	atomic.AddUint64(&m.forwardedQueries, 1)
}

// RecordForwardError records a forward error
func (m *DNSServerMetrics) RecordForwardError() {
	atomic.AddUint64(&m.forwardErrors, 1)
}

// ServerSnapshot returns a snapshot including server metrics
func (m *DNSServerMetrics) ServerSnapshot() *ServerMetricsSnapshot {
	base := m.DiscoveryMetrics.Snapshot()

	return &ServerMetricsSnapshot{
		MetricsSnapshot:  base,
		UDPQueries:       atomic.LoadUint64(&m.udpQueries),
		TCPQueries:       atomic.LoadUint64(&m.tcpQueries),
		UDPErrors:        atomic.LoadUint64(&m.udpErrors),
		TCPErrors:        atomic.LoadUint64(&m.tcpErrors),
		TruncatedQueries: atomic.LoadUint64(&m.truncatedQueries),
		RefusedQueries:   atomic.LoadUint64(&m.refusedQueries),
		RecursiveQueries: atomic.LoadUint64(&m.recursiveQueries),
		ForwardedQueries: atomic.LoadUint64(&m.forwardedQueries),
		ForwardErrors:    atomic.LoadUint64(&m.forwardErrors),
	}
}

// ServerMetricsSnapshot includes both base and server-specific metrics
type ServerMetricsSnapshot struct {
	*MetricsSnapshot
	UDPQueries       uint64
	TCPQueries       uint64
	UDPErrors        uint64
	TCPErrors        uint64
	TruncatedQueries uint64
	RefusedQueries   uint64
	RecursiveQueries uint64
	ForwardedQueries uint64
	ForwardErrors    uint64
}
