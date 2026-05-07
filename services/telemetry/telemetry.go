// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package telemetry provides the telemetry aggregation service for the
// Unheaded Kingdom. It subscribes to per-hop and per-path telemetry
// events from Wotan, computes rolling 1-second window aggregates per
// service pair, and exposes the results via a REST API.
//
// Port: 19019 (ports.TelemetryAgg)
package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
)

// ── Metric types ─────────────────────────────────────────────────────────

// HopMetric represents a single per-hop telemetry sample.
type HopMetric struct {
	TraceID    string        `json:"trace_id"`
	HopIndex   uint8         `json:"hop_index"`
	Timestamp  time.Time     `json:"timestamp"`
	Latency    time.Duration `json:"latency_ns"`
	QueueDepth uint32        `json:"queue_depth"`
	LinkUtil   float64       `json:"link_util"`
	ServiceID  uint8         `json:"service_id"`
}

// PathMetric represents an aggregated path built from per-hop metrics.
type PathMetric struct {
	TraceID      string        `json:"trace_id"`
	Hops         []HopMetric   `json:"hops"`
	TotalLatency time.Duration `json:"total_latency_ns"`
	HopCount     int           `json:"hop_count"`
	Timestamp    time.Time     `json:"timestamp"`
}

// ServicePairAggregate holds rolling statistics for a (src, dst) service pair.
type ServicePairAggregate struct {
	SrcSvcID      uint8         `json:"src_svc_id"`
	DstSvcID      uint8         `json:"dst_svc_id"`
	Window        time.Time     `json:"window"`
	LatencyP50    time.Duration `json:"latency_p50_ns"`
	LatencyP95    time.Duration `json:"latency_p95_ns"`
	LatencyP99    time.Duration `json:"latency_p99_ns"`
	QueueDepthAvg float64       `json:"queue_depth_avg"`
	PacketCount   int64         `json:"packet_count"`
}

// pairKey is a composite key for service-pair maps.
type pairKey struct {
	Src uint8
	Dst uint8
}

// windowBucket accumulates samples for a single 1-second window.
type windowBucket struct {
	latencies   []time.Duration
	queueSum    float64
	sampleCount int64
	window      time.Time
}

// ── Service ──────────────────────────────────────────────────────────────

// Service is the telemetry aggregation service.
type Service struct {
	log   *logger.Logger
	wotan *wotanClient.Client

	// Per-hop metric store (ring buffer, last N minutes).
	hopsMu  sync.RWMutex
	hops    []HopMetric
	maxHops int

	// Per-path metric store.
	pathsMu  sync.RWMutex
	paths    []PathMetric
	maxPaths int

	// Service-pair aggregation state.
	aggMu      sync.RWMutex
	windows    map[pairKey]*windowBucket
	aggregates []ServicePairAggregate
	maxAggs    int

	// Counters.
	hopsReceived  int64
	pathsReceived int64
	aggsPublished int64

	// Lifecycle.
	httpServer *http.Server
}

// NewService creates a new telemetry aggregation service.
func NewService(log *logger.Logger, wotan *wotanClient.Client) *Service {
	return &Service{
		log:        log,
		wotan:      wotan,
		hops:       make([]HopMetric, 0, 10000),
		maxHops:    100000,
		paths:      make([]PathMetric, 0, 1000),
		maxPaths:   10000,
		windows:    make(map[pairKey]*windowBucket),
		aggregates: make([]ServicePairAggregate, 0, 1000),
		maxAggs:    50000,
	}
}

// Start initializes Wotan subscriptions, starts the HTTP server, and
// begins the 1-second aggregation loop.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().
		Str("service", "telemetry-aggregator").
		Int("port", ports.TelemetryAgg).
		Msg("telemetry aggregation service starting")

	// Subscribe to Wotan topics (best-effort).
	if s.wotan != nil {
		topics := []string{"telemetry.hop", "telemetry.path", "telemetry.aggregate", "alerts.critical"}
		for _, topic := range topics {
			if _, err := s.wotan.Subscribe(ctx, topic, "telemetry-agg"); err != nil {
				s.log.Warn().Err(err).Str("topic", topic).Msg("failed to subscribe")
			} else {
				s.log.Info().Str("topic", topic).Msg("subscribed")
			}
		}

		// Start message listeners.
		go s.listenHopMetrics(ctx)
		go s.listenPathMetrics(ctx)
		go s.listenAlerts(ctx)
	} else {
		s.log.Warn().Msg("Wotan not configured, running without subscriptions")
	}

	// Start 1-second aggregation loop.
	go s.aggregationLoop(ctx)

	// Start HTTP server.
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := ports.DefaultAddr(ports.TelemetryAgg)
	s.httpServer = &http.Server{
		Addr:           addr,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		s.log.Info().Str("addr", addr).Msg("HTTP server starting")
		if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("telemetry aggregation service stopping")

	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.log.Error().Err(err).Msg("HTTP server shutdown error")
			return err
		}
	}

	s.log.Info().Msg("telemetry aggregation service stopped")
	return nil
}

// ── Wotan listeners ──────────────────────────────────────────────────────

func (s *Service) listenHopMetrics(ctx context.Context) {
	msgCh, err := s.wotan.StreamMessages(ctx, "telemetry.hop")
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to stream telemetry.hop")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			var hop HopMetric
			if err := json.Unmarshal([]byte(msg.Payload), &hop); err != nil {
				s.log.Warn().Err(err).Msg("failed to decode hop metric")
				continue
			}
			s.ingestHop(hop)
		}
	}
}

func (s *Service) listenPathMetrics(ctx context.Context) {
	msgCh, err := s.wotan.StreamMessages(ctx, "telemetry.path")
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to stream telemetry.path")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			var path PathMetric
			if err := json.Unmarshal([]byte(msg.Payload), &path); err != nil {
				s.log.Warn().Err(err).Msg("failed to decode path metric")
				continue
			}
			s.ingestPath(path)
		}
	}
}

func (s *Service) listenAlerts(ctx context.Context) {
	msgCh, err := s.wotan.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to stream alerts.critical")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			s.log.Warn().
				Str("message_id", msg.MessageID).
				Msg("received critical alert")
		}
	}
}

// ── Ingestion ────────────────────────────────────────────────────────────

// ingestHop stores a hop metric and feeds the aggregation windows.
func (s *Service) ingestHop(hop HopMetric) {
	if hop.Timestamp.IsZero() {
		hop.Timestamp = time.Now()
	}

	// Store in hop ring buffer.
	s.hopsMu.Lock()
	s.hops = append(s.hops, hop)
	if len(s.hops) > s.maxHops {
		trim := s.maxHops / 4
		s.hops = s.hops[trim:]
	}
	s.hopsMu.Unlock()

	atomic.AddInt64(&s.hopsReceived, 1)

	// Feed into aggregation window. We use hop index 0 as the "source"
	// side of a service pair.
	s.feedAggregation(hop)
}

// ingestPath stores a path metric.
func (s *Service) ingestPath(path PathMetric) {
	if path.Timestamp.IsZero() {
		path.Timestamp = time.Now()
	}
	path.HopCount = len(path.Hops)

	// Calculate total latency if not provided.
	if path.TotalLatency == 0 && len(path.Hops) >= 2 {
		first := path.Hops[0]
		last := path.Hops[len(path.Hops)-1]
		if !first.Timestamp.IsZero() && !last.Timestamp.IsZero() {
			path.TotalLatency = last.Timestamp.Sub(first.Timestamp)
		}
	}

	s.pathsMu.Lock()
	s.paths = append(s.paths, path)
	if len(s.paths) > s.maxPaths {
		trim := s.maxPaths / 4
		s.paths = s.paths[trim:]
	}
	s.pathsMu.Unlock()

	atomic.AddInt64(&s.pathsReceived, 1)
}

// IngestHopForTest exposes ingestHop for testing.
func (s *Service) IngestHopForTest(hop HopMetric) {
	s.ingestHop(hop)
}

// IngestPathForTest exposes ingestPath for testing.
func (s *Service) IngestPathForTest(path PathMetric) {
	s.ingestPath(path)
}

// ── Aggregation ──────────────────────────────────────────────────────────

// feedAggregation adds a hop sample to the current 1-second window for
// the (src, dst) service pair. The src is derived from the hop's ServiceID
// and a synthetic dst based on the trace flow.
func (s *Service) feedAggregation(hop HopMetric) {
	// Use service ID pairs: current hop -> next expected service.
	// For simplicity, we pair (serviceID, serviceID+1) unless there's
	// a better signal. Real deployments would extract pairs from path data.
	key := pairKey{Src: hop.ServiceID, Dst: hop.ServiceID + 1}
	windowStart := hop.Timestamp.Truncate(time.Second)

	s.aggMu.Lock()
	defer s.aggMu.Unlock()

	bucket, ok := s.windows[key]
	if !ok || !bucket.window.Equal(windowStart) {
		// New window — flush old bucket if it exists.
		if ok && bucket.sampleCount > 0 {
			s.flushBucket(key, bucket)
		}
		bucket = &windowBucket{
			latencies: make([]time.Duration, 0, 128),
			window:    windowStart,
		}
		s.windows[key] = bucket
	}

	bucket.latencies = append(bucket.latencies, hop.Latency)
	bucket.queueSum += float64(hop.QueueDepth)
	bucket.sampleCount++
}

// aggregationLoop runs every second and flushes all expired windows.
func (s *Service) aggregationLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flushExpiredWindows()
		}
	}
}

// flushExpiredWindows flushes all window buckets older than the current second.
func (s *Service) flushExpiredWindows() {
	now := time.Now().Truncate(time.Second)

	s.aggMu.Lock()
	defer s.aggMu.Unlock()

	for key, bucket := range s.windows {
		if bucket.window.Before(now) && bucket.sampleCount > 0 {
			s.flushBucket(key, bucket)
			delete(s.windows, key)
		}
	}
}

// flushBucket computes percentiles and creates an aggregate entry.
// MUST be called with aggMu held.
func (s *Service) flushBucket(key pairKey, bucket *windowBucket) {
	agg := ServicePairAggregate{
		SrcSvcID:      key.Src,
		DstSvcID:      key.Dst,
		Window:        bucket.window,
		PacketCount:   bucket.sampleCount,
		QueueDepthAvg: bucket.queueSum / float64(bucket.sampleCount),
	}

	// Sort latencies for percentile calculation.
	sorted := make([]time.Duration, len(bucket.latencies))
	copy(sorted, bucket.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	agg.LatencyP50 = percentile(sorted, 50)
	agg.LatencyP95 = percentile(sorted, 95)
	agg.LatencyP99 = percentile(sorted, 99)

	s.aggregates = append(s.aggregates, agg)
	if len(s.aggregates) > s.maxAggs {
		trim := s.maxAggs / 4
		s.aggregates = s.aggregates[trim:]
	}

	atomic.AddInt64(&s.aggsPublished, 1)

	// Publish to Wotan (best-effort, async).
	if s.wotan != nil {
		go s.publishAggregate(agg)
	}
}

// publishAggregate publishes a service-pair aggregate to Wotan.
func (s *Service) publishAggregate(agg ServicePairAggregate) {
	payload, err := json.Marshal(map[string]interface{}{
		"event_type":      "telemetry.aggregate",
		"src_svc_id":      agg.SrcSvcID,
		"dst_svc_id":      agg.DstSvcID,
		"window":          agg.Window.UnixMilli(),
		"latency_p50_ns":  agg.LatencyP50.Nanoseconds(),
		"latency_p95_ns":  agg.LatencyP95.Nanoseconds(),
		"latency_p99_ns":  agg.LatencyP99.Nanoseconds(),
		"queue_depth_avg": agg.QueueDepthAvg,
		"packet_count":    agg.PacketCount,
		"timestamp":       time.Now().UnixMilli(),
	})
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to marshal aggregate")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.wotan.Publish(ctx, "telemetry.aggregate", payload); err != nil {
		s.log.Warn().Err(err).Msg("failed to publish aggregate")
	}
}

// percentile returns the p-th percentile of a sorted duration slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	rank := (p / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}

	// Linear interpolation between adjacent ranks.
	frac := rank - float64(lower)
	return time.Duration(float64(sorted[lower])*(1-frac) + float64(sorted[upper])*frac)
}

// ── Query helpers ────────────────────────────────────────────────────────

// RecentHops returns hop metrics from the last N minutes.
func (s *Service) RecentHops(minutes int) []HopMetric {
	if minutes <= 0 {
		minutes = 5
	}
	cutoff := time.Now().Add(-time.Duration(minutes) * time.Minute)

	s.hopsMu.RLock()
	defer s.hopsMu.RUnlock()

	result := make([]HopMetric, 0, 256)
	for _, hop := range s.hops {
		if hop.Timestamp.After(cutoff) {
			result = append(result, hop)
		}
	}
	return result
}

// RecentPaths returns aggregated path data.
func (s *Service) RecentPaths(limit int) []PathMetric {
	if limit <= 0 {
		limit = 100
	}

	s.pathsMu.RLock()
	defer s.pathsMu.RUnlock()

	start := len(s.paths) - limit
	if start < 0 {
		start = 0
	}

	result := make([]PathMetric, len(s.paths)-start)
	copy(result, s.paths[start:])
	return result
}

// ServiceLatency returns latency percentiles for a specific service over
// the recent aggregate windows.
func (s *Service) ServiceLatency(svcID uint8) []ServicePairAggregate {
	s.aggMu.RLock()
	defer s.aggMu.RUnlock()

	result := make([]ServicePairAggregate, 0, 64)
	for _, agg := range s.aggregates {
		if agg.SrcSvcID == svcID || agg.DstSvcID == svcID {
			result = append(result, agg)
		}
	}
	return result
}

// RecentAggregates returns service-pair aggregates.
func (s *Service) RecentAggregates(limit int) []ServicePairAggregate {
	if limit <= 0 {
		limit = 100
	}

	s.aggMu.RLock()
	defer s.aggMu.RUnlock()

	start := len(s.aggregates) - limit
	if start < 0 {
		start = 0
	}

	result := make([]ServicePairAggregate, len(s.aggregates)-start)
	copy(result, s.aggregates[start:])
	return result
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.hopsMu.RLock()
	hopCount := len(s.hops)
	s.hopsMu.RUnlock()

	s.pathsMu.RLock()
	pathCount := len(s.paths)
	s.pathsMu.RUnlock()

	s.aggMu.RLock()
	aggCount := len(s.aggregates)
	windowCount := len(s.windows)
	s.aggMu.RUnlock()

	return map[string]interface{}{
		"hops_stored":    hopCount,
		"paths_stored":   pathCount,
		"aggregates":     aggCount,
		"active_windows": windowCount,
		"hops_received":  atomic.LoadInt64(&s.hopsReceived),
		"paths_received": atomic.LoadInt64(&s.pathsReceived),
		"aggs_published": atomic.LoadInt64(&s.aggsPublished),
	}
}

// ── HTTP handlers ────────────────────────────────────────────────────────

func (s *Service) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/telemetry/hops", s.handleHops)
	mux.HandleFunc("GET /api/v1/telemetry/paths", s.handlePaths)
	mux.HandleFunc("GET /api/v1/telemetry/services/", s.handleServiceLatency)
	mux.HandleFunc("GET /api/v1/telemetry/aggregate", s.handleAggregate)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
}

func (s *Service) handleHops(w http.ResponseWriter, _ *http.Request) {
	hops := s.RecentHops(5)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hops":  hops,
		"count": len(hops),
	})
}

func (s *Service) handlePaths(w http.ResponseWriter, _ *http.Request) {
	paths := s.RecentPaths(100)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"paths": paths,
		"count": len(paths),
	})
}

func (s *Service) handleServiceLatency(w http.ResponseWriter, r *http.Request) {
	// Extract service ID from path: /api/v1/telemetry/services/{id}/latency
	path := r.URL.Path
	var svcID uint8
	n, err := fmt.Sscanf(path, "/api/v1/telemetry/services/%d/latency", &svcID)
	if err != nil || n != 1 {
		http.Error(w, `{"error":"invalid service ID"}`, http.StatusBadRequest)
		return
	}

	aggs := s.ServiceLatency(svcID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service_id": svcID,
		"aggregates": aggs,
		"count":      len(aggs),
	})
}

func (s *Service) handleAggregate(w http.ResponseWriter, _ *http.Request) {
	aggs := s.RecentAggregates(100)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"aggregates": aggs,
		"count":      len(aggs),
	})
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Service) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (s *Service) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	stats := s.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
