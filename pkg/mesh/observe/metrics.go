// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package observe provides observability features for THE HAUBERK.
package observe

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects request metrics.
type Metrics struct {
	mu sync.RWMutex

	// Request counters
	totalRequests   uint64
	successRequests uint64
	failedRequests  uint64

	// Status code counters
	statusCodes map[int]uint64

	// Latency histogram
	latencyBuckets []uint64
	bucketBounds   []float64 // in milliseconds

	// Per-service metrics
	services map[string]*ServiceMetrics

	// Connection metrics
	activeConnections int64
	totalConnections  uint64
	connectionErrors  uint64

	// Byte counters
	bytesIn  uint64
	bytesOut uint64
}

// ServiceMetrics holds metrics for a specific service.
type ServiceMetrics struct {
	mu sync.RWMutex

	Name      string
	Namespace string

	// Request metrics
	TotalRequests   uint64
	SuccessRequests uint64
	FailedRequests  uint64
	Retries         uint64

	// Latency
	LatencySum   uint64 // microseconds
	LatencyCount uint64
	LatencyMin   uint64
	LatencyMax   uint64

	// Circuit breaker
	CircuitOpens uint64
	CircuitState string

	// Last update
	LastRequestAt time.Time
}

// NewMetrics creates a new metrics collector.
func NewMetrics() *Metrics {
	// Default latency buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
	buckets := []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

	return &Metrics{
		statusCodes:    make(map[int]uint64),
		latencyBuckets: make([]uint64, len(buckets)+1), // +1 for overflow bucket
		bucketBounds:   buckets,
		services:       make(map[string]*ServiceMetrics),
	}
}

// RecordRequest records a request.
func (m *Metrics) RecordRequest(service, namespace string, statusCode int, latency time.Duration, err error) {
	atomic.AddUint64(&m.totalRequests, 1)

	if err != nil || statusCode >= 500 {
		atomic.AddUint64(&m.failedRequests, 1)
	} else {
		atomic.AddUint64(&m.successRequests, 1)
	}

	// Record status code
	m.mu.Lock()
	m.statusCodes[statusCode]++
	m.mu.Unlock()

	// Record latency
	m.recordLatency(latency)

	// Record per-service metrics
	m.recordServiceRequest(service, namespace, statusCode, latency, err)
}

// RecordConnection records a connection event.
func (m *Metrics) RecordConnection(opened bool, err error) {
	if opened {
		atomic.AddInt64(&m.activeConnections, 1)
		atomic.AddUint64(&m.totalConnections, 1)
	} else {
		atomic.AddInt64(&m.activeConnections, -1)
	}

	if err != nil {
		atomic.AddUint64(&m.connectionErrors, 1)
	}
}

// RecordBytes records bytes transferred.
func (m *Metrics) RecordBytes(in, out uint64) {
	atomic.AddUint64(&m.bytesIn, in)
	atomic.AddUint64(&m.bytesOut, out)
}

// RecordRetry records a retry attempt.
func (m *Metrics) RecordRetry(service, namespace string) {
	key := namespace + "/" + service

	m.mu.Lock()
	if svc, ok := m.services[key]; ok {
		atomic.AddUint64(&svc.Retries, 1)
	}
	m.mu.Unlock()
}

// RecordCircuitOpen records a circuit breaker opening.
func (m *Metrics) RecordCircuitOpen(service, namespace string) {
	key := namespace + "/" + service

	m.mu.Lock()
	if svc, ok := m.services[key]; ok {
		atomic.AddUint64(&svc.CircuitOpens, 1)
		svc.CircuitState = "open"
	}
	m.mu.Unlock()
}

// UpdateCircuitState updates the circuit breaker state.
func (m *Metrics) UpdateCircuitState(service, namespace, state string) {
	key := namespace + "/" + service

	m.mu.Lock()
	if svc, ok := m.services[key]; ok {
		svc.CircuitState = state
	}
	m.mu.Unlock()
}

func (m *Metrics) recordLatency(latency time.Duration) {
	ms := float64(latency.Microseconds()) / 1000.0

	// Find bucket
	bucketIdx := len(m.bucketBounds) // overflow bucket
	for i, bound := range m.bucketBounds {
		if ms <= bound {
			bucketIdx = i
			break
		}
	}

	atomic.AddUint64(&m.latencyBuckets[bucketIdx], 1)
}

func (m *Metrics) recordServiceRequest(service, namespace string, statusCode int, latency time.Duration, err error) {
	key := namespace + "/" + service

	m.mu.Lock()
	svc, ok := m.services[key]
	if !ok {
		svc = &ServiceMetrics{
			Name:       service,
			Namespace:  namespace,
			LatencyMin: ^uint64(0), // Max uint64
		}
		m.services[key] = svc
	}
	m.mu.Unlock()

	atomic.AddUint64(&svc.TotalRequests, 1)
	if err != nil || statusCode >= 500 {
		atomic.AddUint64(&svc.FailedRequests, 1)
	} else {
		atomic.AddUint64(&svc.SuccessRequests, 1)
	}

	// Latency stats
	latencyUs := uint64(latency.Microseconds()) // #nosec G115 -- bounded by construction; see the surrounding guard
	atomic.AddUint64(&svc.LatencySum, latencyUs)
	atomic.AddUint64(&svc.LatencyCount, 1)

	// Update min/max (not perfectly atomic but good enough for metrics)
	svc.mu.Lock()
	if latencyUs < svc.LatencyMin {
		svc.LatencyMin = latencyUs
	}
	if latencyUs > svc.LatencyMax {
		svc.LatencyMax = latencyUs
	}
	svc.LastRequestAt = time.Now()
	svc.mu.Unlock()
}

// GetOrCreateServiceMetrics gets or creates service metrics.
func (m *Metrics) GetOrCreateServiceMetrics(service, namespace string) *ServiceMetrics {
	key := namespace + "/" + service

	m.mu.RLock()
	svc, ok := m.services[key]
	m.mu.RUnlock()

	if ok {
		return svc
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if svc, ok := m.services[key]; ok {
		return svc
	}

	svc = &ServiceMetrics{
		Name:       service,
		Namespace:  namespace,
		LatencyMin: ^uint64(0),
	}
	m.services[key] = svc
	return svc
}

// Snapshot returns a point-in-time snapshot of metrics.
func (m *Metrics) Snapshot() *MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Copy status codes
	statusCodes := make(map[int]uint64, len(m.statusCodes))
	for k, v := range m.statusCodes {
		statusCodes[k] = v
	}

	// Copy latency buckets
	latencyBuckets := make([]uint64, len(m.latencyBuckets))
	copy(latencyBuckets, m.latencyBuckets)

	// Copy service metrics
	services := make(map[string]*ServiceMetricsSnapshot, len(m.services))
	for key, svc := range m.services {
		svc.mu.RLock()
		services[key] = &ServiceMetricsSnapshot{
			Name:            svc.Name,
			Namespace:       svc.Namespace,
			TotalRequests:   atomic.LoadUint64(&svc.TotalRequests),
			SuccessRequests: atomic.LoadUint64(&svc.SuccessRequests),
			FailedRequests:  atomic.LoadUint64(&svc.FailedRequests),
			Retries:         atomic.LoadUint64(&svc.Retries),
			LatencyAvgUs:    m.calculateAvgLatency(svc),
			LatencyMinUs:    svc.LatencyMin,
			LatencyMaxUs:    svc.LatencyMax,
			CircuitOpens:    atomic.LoadUint64(&svc.CircuitOpens),
			CircuitState:    svc.CircuitState,
			LastRequestAt:   svc.LastRequestAt,
		}
		svc.mu.RUnlock()
	}

	return &MetricsSnapshot{
		Timestamp:           time.Now(),
		TotalRequests:       atomic.LoadUint64(&m.totalRequests),
		SuccessRequests:     atomic.LoadUint64(&m.successRequests),
		FailedRequests:      atomic.LoadUint64(&m.failedRequests),
		StatusCodes:         statusCodes,
		LatencyBuckets:      latencyBuckets,
		LatencyBucketBounds: m.bucketBounds,
		ActiveConnections:   atomic.LoadInt64(&m.activeConnections),
		TotalConnections:    atomic.LoadUint64(&m.totalConnections),
		ConnectionErrors:    atomic.LoadUint64(&m.connectionErrors),
		BytesIn:             atomic.LoadUint64(&m.bytesIn),
		BytesOut:            atomic.LoadUint64(&m.bytesOut),
		Services:            services,
	}
}

func (m *Metrics) calculateAvgLatency(svc *ServiceMetrics) uint64 {
	count := atomic.LoadUint64(&svc.LatencyCount)
	if count == 0 {
		return 0
	}
	sum := atomic.LoadUint64(&svc.LatencySum)
	return sum / count
}

// Reset resets all metrics.
func (m *Metrics) Reset() {
	atomic.StoreUint64(&m.totalRequests, 0)
	atomic.StoreUint64(&m.successRequests, 0)
	atomic.StoreUint64(&m.failedRequests, 0)
	atomic.StoreUint64(&m.totalConnections, 0)
	atomic.StoreUint64(&m.connectionErrors, 0)
	atomic.StoreUint64(&m.bytesIn, 0)
	atomic.StoreUint64(&m.bytesOut, 0)

	m.mu.Lock()
	m.statusCodes = make(map[int]uint64)
	for i := range m.latencyBuckets {
		m.latencyBuckets[i] = 0
	}
	m.services = make(map[string]*ServiceMetrics)
	m.mu.Unlock()
}

// MetricsSnapshot is a point-in-time snapshot of metrics.
type MetricsSnapshot struct {
	Timestamp time.Time

	// Request metrics
	TotalRequests   uint64
	SuccessRequests uint64
	FailedRequests  uint64

	// Status codes
	StatusCodes map[int]uint64

	// Latency histogram
	LatencyBuckets      []uint64
	LatencyBucketBounds []float64

	// Connections
	ActiveConnections int64
	TotalConnections  uint64
	ConnectionErrors  uint64

	// Bytes
	BytesIn  uint64
	BytesOut uint64

	// Per-service
	Services map[string]*ServiceMetricsSnapshot
}

// ServiceMetricsSnapshot is a point-in-time snapshot of service metrics.
type ServiceMetricsSnapshot struct {
	Name            string
	Namespace       string
	TotalRequests   uint64
	SuccessRequests uint64
	FailedRequests  uint64
	Retries         uint64
	LatencyAvgUs    uint64
	LatencyMinUs    uint64
	LatencyMaxUs    uint64
	CircuitOpens    uint64
	CircuitState    string
	LastRequestAt   time.Time
}

// SuccessRate returns the success rate as a percentage.
func (s *MetricsSnapshot) SuccessRate() float64 {
	if s.TotalRequests == 0 {
		return 100.0
	}
	return float64(s.SuccessRequests) / float64(s.TotalRequests) * 100.0
}

// LatencyPercentile estimates a latency percentile from the histogram.
func (s *MetricsSnapshot) LatencyPercentile(percentile float64) float64 {
	var total uint64
	for _, count := range s.LatencyBuckets {
		total += count
	}

	if total == 0 {
		return 0
	}

	threshold := uint64(float64(total) * percentile / 100.0)
	var cumulative uint64

	for i, count := range s.LatencyBuckets {
		cumulative += count
		if cumulative >= threshold {
			if i < len(s.LatencyBucketBounds) {
				return s.LatencyBucketBounds[i]
			}
			// Overflow bucket
			return s.LatencyBucketBounds[len(s.LatencyBucketBounds)-1] * 2
		}
	}

	return 0
}
