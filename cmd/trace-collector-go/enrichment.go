// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// enrichment.go — Trace enrichment for the trace-collector-go pipeline.
//
// TraceEnricher resolves service names from Sophia's dictionary, adds
// geographic zone info, QoS class descriptions, calculates path latency
// from hop data, and builds dependency graph edges. Results are cached
// in a bounded sync.Map to avoid repeated lookups.
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// ── Types ────────────────────────────────────────────────────────────────

// TraceEvent represents an enriched trace flowing through the pipeline.
type TraceEvent struct {
	TraceID         string        `json:"trace_id"`
	TimestampNS     uint64        `json:"timestamp_ns"`
	SrcServiceID    uint8         `json:"src_service_id"`
	DstServiceID    uint8         `json:"dst_service_id"`
	SrcServiceName  string        `json:"src_service_name,omitempty"`
	DstServiceName  string        `json:"dst_service_name,omitempty"`
	GeoZone         string        `json:"geo_zone,omitempty"`
	QoSClass        uint8         `json:"qos_class"`
	QoSDescription  string        `json:"qos_description,omitempty"`
	Hops            []HopData     `json:"hops,omitempty"`
	PathLatency     time.Duration `json:"path_latency_ns,omitempty"`
	DependencyEdges []DepEdge     `json:"dependency_edges,omitempty"`
	Enriched        bool          `json:"enriched"`
}

// HopData represents a single hop in a trace path.
type HopData struct {
	HopIndex    uint8         `json:"hop_index"`
	ServiceID   uint8         `json:"service_id"`
	ServiceName string        `json:"service_name,omitempty"`
	Latency     time.Duration `json:"latency_ns"`
	QueueDepth  uint32        `json:"queue_depth"`
	LinkUtil    float64       `json:"link_util"`
	TimestampNS uint64        `json:"timestamp_ns"`
}

// DepEdge represents an edge in the service dependency graph.
type DepEdge struct {
	SrcService string        `json:"src_service"`
	DstService string        `json:"dst_service"`
	Latency    time.Duration `json:"latency_ns"`
	Observed   time.Time     `json:"observed"`
}

// enrichmentCacheEntry stores a cached enrichment result.
type enrichmentCacheEntry struct {
	serviceName string
	cachedAt    time.Time
}

// ── TraceEnricher ────────────────────────────────────────────────────────

// maxCacheEntries is the upper bound on enrichment cache size.
const maxCacheEntries = 1000

// TraceEnricher enriches raw trace events with service metadata from
// Sophia's dictionary, geographic zone information, and QoS class
// descriptions. It maintains a bounded cache to avoid redundant lookups.
type TraceEnricher struct {
	wotan       *wotanClient.Client
	cache       sync.Map // map[uint8]*enrichmentCacheEntry
	cacheSize   int64
	enriched    int64
	cacheMisses int64
	cacheHits   int64
}

// NewTraceEnricher creates a TraceEnricher backed by the given Wotan
// (Sophia) client for service name resolution.
func NewTraceEnricher(wotan *wotanClient.Client) *TraceEnricher {
	return &TraceEnricher{
		wotan: wotan,
	}
}

// EnrichTrace enriches a TraceEvent with service names, geographic zone,
// QoS class description, path latency, and dependency graph edges.
func (te *TraceEnricher) EnrichTrace(trace *TraceEvent) {
	if trace == nil {
		return
	}

	// Resolve source and destination service names.
	trace.SrcServiceName = te.resolveServiceName(trace.SrcServiceID)
	trace.DstServiceName = te.resolveServiceName(trace.DstServiceID)

	// Add geographic zone info based on service IDs.
	trace.GeoZone = resolveGeoZone(trace.SrcServiceID, trace.DstServiceID)

	// Add QoS class description.
	trace.QoSDescription = resolveQoSClass(trace.QoSClass)

	// Calculate path latency from hop data.
	trace.PathLatency = calculatePathLatency(trace.Hops)

	// Build dependency graph edges from hops.
	trace.DependencyEdges = te.buildDependencyEdges(trace)

	// Enrich individual hops with service names.
	for i := range trace.Hops {
		trace.Hops[i].ServiceName = te.resolveServiceName(trace.Hops[i].ServiceID)
	}

	trace.Enriched = true
	atomic.AddInt64(&te.enriched, 1)
}

// resolveServiceName looks up a service name from the enrichment cache or
// falls back to the Sophia dictionary via Wotan. Results are cached.
func (te *TraceEnricher) resolveServiceName(svcID uint8) string {
	// Check cache first.
	if entry, ok := te.cache.Load(svcID); ok {
		cached, _ := entry.(*enrichmentCacheEntry)
		// Cache entries are valid for 5 minutes.
		if time.Since(cached.cachedAt) < 5*time.Minute {
			atomic.AddInt64(&te.cacheHits, 1)
			return cached.serviceName
		}
		// Expired entry — remove it and fall through to lookup.
		te.cache.Delete(svcID)
		atomic.AddInt64(&te.cacheSize, -1)
	}

	atomic.AddInt64(&te.cacheMisses, 1)

	// Resolve from known service registry (Sophia dictionary).
	name := defaultServiceName(svcID)

	// Store in cache if under limit.
	if atomic.LoadInt64(&te.cacheSize) < maxCacheEntries {
		te.cache.Store(svcID, &enrichmentCacheEntry{
			serviceName: name,
			cachedAt:    time.Now(),
		})
		atomic.AddInt64(&te.cacheSize, 1)
	}

	return name
}

// buildDependencyEdges constructs dependency graph edges from trace hops.
func (te *TraceEnricher) buildDependencyEdges(trace *TraceEvent) []DepEdge {
	if len(trace.Hops) < 2 {
		// Need at least two hops to form an edge.
		if trace.SrcServiceName != "" && trace.DstServiceName != "" {
			return []DepEdge{{
				SrcService: trace.SrcServiceName,
				DstService: trace.DstServiceName,
				Latency:    trace.PathLatency,
				Observed:   time.Now(),
			}}
		}
		return nil
	}

	edges := make([]DepEdge, 0, len(trace.Hops)-1)
	now := time.Now()
	for i := 0; i < len(trace.Hops)-1; i++ {
		src := te.resolveServiceName(trace.Hops[i].ServiceID)
		dst := te.resolveServiceName(trace.Hops[i+1].ServiceID)
		hopLatency := trace.Hops[i+1].Latency - trace.Hops[i].Latency
		if hopLatency < 0 {
			hopLatency = trace.Hops[i+1].Latency
		}
		edges = append(edges, DepEdge{
			SrcService: src,
			DstService: dst,
			Latency:    hopLatency,
			Observed:   now,
		})
	}
	return edges
}

// Stats returns enricher statistics.
func (te *TraceEnricher) Stats() map[string]int64 {
	return map[string]int64{
		"enriched":     atomic.LoadInt64(&te.enriched),
		"cache_hits":   atomic.LoadInt64(&te.cacheHits),
		"cache_misses": atomic.LoadInt64(&te.cacheMisses),
		"cache_size":   atomic.LoadInt64(&te.cacheSize),
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────

// resolveQoSClass returns a human-readable description for a QoS class.
func resolveQoSClass(qos uint8) string {
	switch qos {
	case 0:
		return "best-effort"
	case 1:
		return "bulk-transfer"
	case 2:
		return "standard"
	case 3:
		return "low-latency"
	case 4:
		return "real-time"
	case 5:
		return "critical-control"
	case 6:
		return "network-control"
	case 7:
		return "reserved-highest"
	default:
		return fmt.Sprintf("qos-class-%d", qos)
	}
}

// calculatePathLatency sums the individual hop latencies to compute the
// total path latency. If timestamps are available, it uses the difference
// between the first and last hop timestamps instead.
func calculatePathLatency(hops []HopData) time.Duration {
	if len(hops) == 0 {
		return 0
	}

	// Prefer timestamp-based calculation when available.
	if len(hops) >= 2 {
		first := hops[0].TimestampNS
		last := hops[len(hops)-1].TimestampNS
		if first > 0 && last > first {
			return time.Duration(last-first) * time.Nanosecond
		}
	}

	// Fallback: sum individual hop latencies.
	var total time.Duration
	for _, hop := range hops {
		total += hop.Latency
	}
	return total
}

// resolveGeoZone determines the geographic zone based on service IDs.
// Services are grouped into zones by ID range.
func resolveGeoZone(srcID, dstID uint8) string {
	srcZone := geoZoneForID(srcID)
	dstZone := geoZoneForID(dstID)
	if srcZone == dstZone {
		return srcZone
	}
	return fmt.Sprintf("%s->%s", srcZone, dstZone)
}

// geoZoneForID maps a service ID to a geographic zone name.
func geoZoneForID(svcID uint8) string {
	switch {
	case svcID < 32:
		return "zone-west"
	case svcID < 64:
		return "zone-central"
	case svcID < 96:
		return "zone-east"
	case svcID < 128:
		return "zone-north"
	default:
		return "zone-south"
	}
}

// defaultServiceName returns the canonical name for a known service ID,
// mirroring the Sophia dictionary's service_id -> name mapping.
func defaultServiceName(svcID uint8) string {
	switch svcID {
	case 0x01:
		return "timeguru"
	case 0x02:
		return "architect"
	case 0x03:
		return "captain"
	case 0x04:
		return "micromanager"
	case 0x05:
		return "monad"
	case 0x06:
		return "sophia"
	case 0x07:
		return "cuirass"
	case 0x08:
		return "dashboard-backend"
	case 0x09:
		return "kanban-app"
	case 0x0A:
		return "wotan"
	case 0x0B:
		return "unheaded-daemon"
	case 0x0C:
		return "gateway"
	case 0x0D:
		return "trace-collector"
	case 0x0E:
		return "doom-bridge"
	case 0x10:
		return "firewall"
	case 0x11:
		return "canary"
	case 0x12:
		return "chaos-controller"
	case 0x13:
		return "qos-engine"
	case 0x14:
		return "backup-manager"
	case 0x15:
		return "compliance"
	case 0x16:
		return "nfv"
	case 0x17:
		return "version-service"
	case 0x18:
		return "anomaly-detector"
	case 0x19:
		return "telemetry-aggregator"
	default:
		return fmt.Sprintf("service-%d", svcID)
	}
}
