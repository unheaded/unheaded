// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dns

import (
	"testing"
	"time"
)

func TestDiscoveryMetrics_New(t *testing.T) {
	m := NewDiscoveryMetrics()
	if m == nil {
		t.Fatal("NewDiscoveryMetrics returned nil")
	}

	if m.queryLatency == nil {
		t.Error("queryLatency should be initialized")
	}
	if m.serviceHealth == nil {
		t.Error("serviceHealth should be initialized")
	}
	if m.queryByType == nil {
		t.Error("queryByType should be initialized")
	}
	if m.queryByRcode == nil {
		t.Error("queryByRcode should be initialized")
	}
}

func TestDiscoveryMetrics_RecordQuery(t *testing.T) {
	m := NewDiscoveryMetrics()

	// Record queries of different types
	m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, true)
	m.RecordQuery(TypeAAAA, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypeSRV, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypeTXT, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypePTR, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypeA, RcodeNameError, time.Millisecond, false) // NXDOMAIN

	snap := m.Snapshot()

	if snap.TotalQueries != 7 {
		t.Errorf("TotalQueries = %d, want 7", snap.TotalQueries)
	}
	if snap.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", snap.CacheHits)
	}
	if snap.CacheMisses != 6 {
		t.Errorf("CacheMisses = %d, want 6", snap.CacheMisses)
	}
	if snap.AQueries != 3 {
		t.Errorf("AQueries = %d, want 3", snap.AQueries)
	}
	if snap.AAAAQueries != 1 {
		t.Errorf("AAAAQueries = %d, want 1", snap.AAAAQueries)
	}
	if snap.SRVQueries != 1 {
		t.Errorf("SRVQueries = %d, want 1", snap.SRVQueries)
	}
	if snap.TXTQueries != 1 {
		t.Errorf("TXTQueries = %d, want 1", snap.TXTQueries)
	}
	if snap.PTRQueries != 1 {
		t.Errorf("PTRQueries = %d, want 1", snap.PTRQueries)
	}
	if snap.NXDOMAINResponses != 1 {
		t.Errorf("NXDOMAINResponses = %d, want 1", snap.NXDOMAINResponses)
	}
}

func TestDiscoveryMetrics_CacheHitRate(t *testing.T) {
	m := NewDiscoveryMetrics()

	// 3 cache hits, 7 misses = 30% hit rate
	for i := 0; i < 3; i++ {
		m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, true)
	}
	for i := 0; i < 7; i++ {
		m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, false)
	}

	snap := m.Snapshot()

	if snap.CacheHitRate != 30.0 {
		t.Errorf("CacheHitRate = %.1f, want 30.0", snap.CacheHitRate)
	}
}

func TestDiscoveryMetrics_ServiceRegistration(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.ServiceRegistered("api")
	m.ServiceRegistered("web")
	m.ServiceDeregistered("web")

	snap := m.Snapshot()

	if snap.ServiceRegistrations != 2 {
		t.Errorf("ServiceRegistrations = %d, want 2", snap.ServiceRegistrations)
	}
	if snap.ServiceDeregistrations != 1 {
		t.Errorf("ServiceDeregistrations = %d, want 1", snap.ServiceDeregistrations)
	}

	// Check service exists in map
	if _, ok := snap.Services["api"]; !ok {
		t.Error("api service should exist in snapshot")
	}
	if _, ok := snap.Services["web"]; ok {
		t.Error("web service should not exist after deregistration")
	}
}

func TestDiscoveryMetrics_EndpointChanges(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.ServiceRegistered("test")
	m.EndpointAdded("test", "ep-1")
	m.EndpointAdded("test", "ep-2")
	m.EndpointRemoved("test", "ep-1")

	snap := m.Snapshot()

	if snap.EndpointAdditions != 2 {
		t.Errorf("EndpointAdditions = %d, want 2", snap.EndpointAdditions)
	}
	if snap.EndpointRemovals != 1 {
		t.Errorf("EndpointRemovals = %d, want 1", snap.EndpointRemovals)
	}

	svcSnap := snap.Services["test"]
	if svcSnap.EndpointCount != 1 {
		t.Errorf("EndpointCount = %d, want 1", svcSnap.EndpointCount)
	}
}

func TestDiscoveryMetrics_HealthChanges(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.ServiceRegistered("test")
	m.HealthChanged("test", "ep-1", HealthStateHealthy)
	m.HealthChanged("test", "ep-1", HealthStateUnhealthy)

	snap := m.Snapshot()

	if snap.HealthChanges != 2 {
		t.Errorf("HealthChanges = %d, want 2", snap.HealthChanges)
	}

	svcSnap := snap.Services["test"]
	if svcSnap.LastHealthCheck.IsZero() {
		t.Error("LastHealthCheck should be set")
	}
}

func TestDiscoveryMetrics_UpdateServiceHealth(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.ServiceRegistered("test")
	m.UpdateServiceHealth("test", 5, 3) // 5 total, 3 healthy

	snap := m.Snapshot()
	svcSnap := snap.Services["test"]

	if svcSnap.EndpointCount != 5 {
		t.Errorf("EndpointCount = %d, want 5", svcSnap.EndpointCount)
	}
	if svcSnap.HealthyCount != 3 {
		t.Errorf("HealthyCount = %d, want 3", svcSnap.HealthyCount)
	}
}

func TestDiscoveryMetrics_RecordServiceQuery(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.ServiceRegistered("test")
	m.RecordServiceQuery("test")
	m.RecordServiceQuery("test")
	m.RecordServiceQuery("nonexistent") // Should not crash

	snap := m.Snapshot()
	svcSnap := snap.Services["test"]

	if svcSnap.TotalQueries != 2 {
		t.Errorf("TotalQueries = %d, want 2", svcSnap.TotalQueries)
	}
}

func TestDiscoveryMetrics_Latency(t *testing.T) {
	m := NewDiscoveryMetrics()

	// Record some queries with different latencies
	m.RecordQuery(TypeA, RcodeSuccess, 100*time.Microsecond, false)
	m.RecordQuery(TypeA, RcodeSuccess, 1*time.Millisecond, false)
	m.RecordQuery(TypeA, RcodeSuccess, 10*time.Millisecond, false)
	m.RecordQuery(TypeA, RcodeSuccess, 100*time.Millisecond, false)

	snap := m.Snapshot()

	if snap.QueryLatency == nil {
		t.Fatal("QueryLatency should not be nil")
	}

	if snap.QueryLatency.Count != 4 {
		t.Errorf("Count = %d, want 4", snap.QueryLatency.Count)
	}
	if snap.QueryLatency.Min <= 0 {
		t.Error("Min should be > 0")
	}
	if snap.QueryLatency.Max <= 0 {
		t.Error("Max should be > 0")
	}
	if snap.QueryLatency.Mean <= 0 {
		t.Error("Mean should be > 0")
	}

	// Check histogram buckets
	if len(snap.QueryLatency.Buckets) == 0 {
		t.Error("Buckets should not be empty")
	}
}

func TestDiscoveryMetrics_Reset(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, false)
	m.ServiceRegistered("test")

	m.Reset()

	snap := m.Snapshot()

	if snap.TotalQueries != 0 {
		t.Errorf("TotalQueries after reset = %d, want 0", snap.TotalQueries)
	}
	// Service health data is kept
}

func TestDiscoveryMetrics_Uptime(t *testing.T) {
	m := NewDiscoveryMetrics()

	time.Sleep(10 * time.Millisecond)

	snap := m.Snapshot()

	if snap.Uptime < 10*time.Millisecond {
		t.Errorf("Uptime = %v, want >= 10ms", snap.Uptime)
	}
}

func TestDNSServerMetrics_New(t *testing.T) {
	m := NewDNSServerMetrics()
	if m == nil {
		t.Fatal("NewDNSServerMetrics returned nil")
	}

	if m.DiscoveryMetrics == nil {
		t.Error("DiscoveryMetrics should be embedded")
	}
}

func TestDNSServerMetrics_RecordQueries(t *testing.T) {
	m := NewDNSServerMetrics()

	m.RecordUDPQuery()
	m.RecordUDPQuery()
	m.RecordTCPQuery()
	m.RecordUDPError()
	m.RecordTCPError()
	m.RecordTruncated()
	m.RecordRefused()
	m.RecordRecursive()
	m.RecordForwarded()
	m.RecordForwardError()

	snap := m.ServerSnapshot()

	if snap.UDPQueries != 2 {
		t.Errorf("UDPQueries = %d, want 2", snap.UDPQueries)
	}
	if snap.TCPQueries != 1 {
		t.Errorf("TCPQueries = %d, want 1", snap.TCPQueries)
	}
	if snap.UDPErrors != 1 {
		t.Errorf("UDPErrors = %d, want 1", snap.UDPErrors)
	}
	if snap.TCPErrors != 1 {
		t.Errorf("TCPErrors = %d, want 1", snap.TCPErrors)
	}
	if snap.TruncatedQueries != 1 {
		t.Errorf("TruncatedQueries = %d, want 1", snap.TruncatedQueries)
	}
	if snap.RefusedQueries != 1 {
		t.Errorf("RefusedQueries = %d, want 1", snap.RefusedQueries)
	}
	if snap.RecursiveQueries != 1 {
		t.Errorf("RecursiveQueries = %d, want 1", snap.RecursiveQueries)
	}
	if snap.ForwardedQueries != 1 {
		t.Errorf("ForwardedQueries = %d, want 1", snap.ForwardedQueries)
	}
	if snap.ForwardErrors != 1 {
		t.Errorf("ForwardErrors = %d, want 1", snap.ForwardErrors)
	}
}

func TestDNSServerMetrics_InheritedFunctionality(t *testing.T) {
	m := NewDNSServerMetrics()

	// Use inherited functionality from DiscoveryMetrics
	m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, false)
	m.ServiceRegistered("test")

	snap := m.ServerSnapshot()

	if snap.TotalQueries != 1 {
		t.Errorf("TotalQueries = %d, want 1", snap.TotalQueries)
	}
	if snap.ServiceRegistrations != 1 {
		t.Errorf("ServiceRegistrations = %d, want 1", snap.ServiceRegistrations)
	}
}

func TestLatencyTracker_EmptySnapshot(t *testing.T) {
	lt := newLatencyTracker()

	snap := lt.snapshot()

	if snap.Count != 0 {
		t.Errorf("Count = %d, want 0", snap.Count)
	}
	if snap.Sum != 0 {
		t.Errorf("Sum = %v, want 0", snap.Sum)
	}
}

func TestLatencyTracker_MinMax(t *testing.T) {
	lt := newLatencyTracker()

	lt.record(100 * time.Microsecond)
	lt.record(10 * time.Millisecond)
	lt.record(1 * time.Millisecond)

	snap := lt.snapshot()

	if snap.Min != 100*time.Microsecond {
		t.Errorf("Min = %v, want 100us", snap.Min)
	}
	if snap.Max != 10*time.Millisecond {
		t.Errorf("Max = %v, want 10ms", snap.Max)
	}
}

func TestLatencyTracker_Buckets(t *testing.T) {
	lt := newLatencyTracker()

	// Record values in different buckets
	lt.record(50 * time.Microsecond)   // < 100us
	lt.record(200 * time.Microsecond)  // 100-500us
	lt.record(2 * time.Millisecond)    // 1-5ms
	lt.record(1 * time.Second)         // overflow bucket

	snap := lt.snapshot()

	if len(snap.Buckets) == 0 {
		t.Fatal("Buckets should not be empty")
	}

	// Check overflow bucket has the 1s value
	overflowBucket := snap.Buckets[len(snap.Buckets)-1]
	if overflowBucket.Count == 0 {
		t.Error("Overflow bucket should have count > 0")
	}
}

func TestServiceMetricsSnapshot(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.ServiceRegistered("test-svc")
	m.EndpointAdded("test-svc", "ep-1")
	m.EndpointAdded("test-svc", "ep-2")
	m.RecordServiceQuery("test-svc")

	snap := m.Snapshot()
	svcSnap, ok := snap.Services["test-svc"]
	if !ok {
		t.Fatal("Service snapshot should exist")
	}

	if svcSnap.Name != "test-svc" {
		t.Errorf("Name = %s, want test-svc", svcSnap.Name)
	}
	if svcSnap.EndpointCount != 2 {
		t.Errorf("EndpointCount = %d, want 2", svcSnap.EndpointCount)
	}
	if svcSnap.TotalQueries != 1 {
		t.Errorf("TotalQueries = %d, want 1", svcSnap.TotalQueries)
	}
	if svcSnap.RegisteredAt.IsZero() {
		t.Error("RegisteredAt should be set")
	}
}

func TestQueryByTypeDistribution(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypeAAAA, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypeMX, RcodeSuccess, time.Millisecond, false)

	snap := m.Snapshot()

	if snap.QueryByType[TypeA] != 2 {
		t.Errorf("TypeA count = %d, want 2", snap.QueryByType[TypeA])
	}
	if snap.QueryByType[TypeAAAA] != 1 {
		t.Errorf("TypeAAAA count = %d, want 1", snap.QueryByType[TypeAAAA])
	}
	if snap.QueryByType[TypeMX] != 1 {
		t.Errorf("TypeMX count = %d, want 1", snap.QueryByType[TypeMX])
	}
}

func TestQueryByRcodeDistribution(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypeA, RcodeSuccess, time.Millisecond, false)
	m.RecordQuery(TypeA, RcodeNameError, time.Millisecond, false)
	m.RecordQuery(TypeA, RcodeServerFailure, time.Millisecond, false)

	snap := m.Snapshot()

	if snap.QueryByRcode[RcodeSuccess] != 2 {
		t.Errorf("NOERROR count = %d, want 2", snap.QueryByRcode[RcodeSuccess])
	}
	if snap.QueryByRcode[RcodeNameError] != 1 {
		t.Errorf("NXDOMAIN count = %d, want 1", snap.QueryByRcode[RcodeNameError])
	}
	if snap.QueryByRcode[RcodeServerFailure] != 1 {
		t.Errorf("SERVFAIL count = %d, want 1", snap.QueryByRcode[RcodeServerFailure])
	}
}

func TestEndpointRemoved_HealthyCountUpdate(t *testing.T) {
	m := NewDiscoveryMetrics()

	m.ServiceRegistered("test")
	m.EndpointAdded("test", "ep-1")
	m.EndpointAdded("test", "ep-2")
	m.EndpointAdded("test", "ep-3")

	// Initially all healthy (added as healthy)
	snap := m.Snapshot()
	if snap.Services["test"].HealthyCount != 3 {
		t.Errorf("Initial HealthyCount = %d, want 3", snap.Services["test"].HealthyCount)
	}

	// Remove an endpoint
	m.EndpointRemoved("test", "ep-1")

	snap = m.Snapshot()
	if snap.Services["test"].EndpointCount != 2 {
		t.Errorf("EndpointCount = %d, want 2", snap.Services["test"].EndpointCount)
	}
	// Healthy count should be capped at endpoint count
	if snap.Services["test"].HealthyCount > snap.Services["test"].EndpointCount {
		t.Error("HealthyCount should not exceed EndpointCount")
	}
}
