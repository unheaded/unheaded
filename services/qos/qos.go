// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package qos provides Quality of Service policy management for the Unheaded
// Kingdom. It manages per-service QoS policies, aggregates queue statistics at
// 10 Hz (100ms intervals), and publishes them to the Wotan message bus.
// CoDel-inspired active queue management parameters are configurable per service.
package qos

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
)

// CoDel constants (Controlled Delay active queue management).
const (
	DefaultTargetMS   = 5
	DefaultIntervalMS = 100
)

// Circuit breaker states for congestion detection.
const (
	CircuitClosed   = "CLOSED"
	CircuitOpen     = "OPEN"
	CircuitHalfOpen = "HALF_OPEN"
)

// statsAggregationInterval controls how often queue stats are aggregated and
// published (10 Hz = every 100ms).
const statsAggregationInterval = 100 * time.Millisecond

// QoSPolicy defines a per-service QoS configuration.
type QoSPolicy struct {
	ServiceID      string  `json:"service_id"`
	Class          uint8   `json:"class"`
	Weight         uint8   `json:"weight"`
	RateLimitMbps  float64 `json:"rate_limit_mbps"`
	BurstBytes     uint64  `json:"burst_bytes"`
	TargetLatencyMS int    `json:"target_latency_ms"`
	IntervalMS     int     `json:"interval_ms"`
}

// Validate checks the policy for correctness.
func (p *QoSPolicy) Validate() error {
	if p.ServiceID == "" {
		return fmt.Errorf("service_id is required")
	}
	if p.Weight < 1 || p.Weight > 16 {
		return fmt.Errorf("weight must be between 1 and 16, got %d", p.Weight)
	}
	if p.RateLimitMbps < 0 {
		return fmt.Errorf("rate_limit_mbps cannot be negative")
	}
	if p.TargetLatencyMS <= 0 {
		p.TargetLatencyMS = DefaultTargetMS
	}
	if p.IntervalMS <= 0 {
		p.IntervalMS = DefaultIntervalMS
	}
	return nil
}

// QueueStats holds per-service queue statistics.
type QueueStats struct {
	ServiceID       string  `json:"service_id"`
	Packets         uint64  `json:"packets"`
	Drops           uint64  `json:"drops"`
	QueueDepth      int     `json:"queue_depth"`
	DropProbability float64 `json:"drop_probability"`
	LatencyP50      float64 `json:"latency_p50_ms"`
	LatencyP99      float64 `json:"latency_p99_ms"`
	LatencyP999     float64 `json:"latency_p999_ms"`
	Timestamp       time.Time `json:"timestamp"`
}

// CongestionInfo reports the congestion state for a service.
type CongestionInfo struct {
	ServiceID    string `json:"service_id"`
	CircuitState string `json:"circuit_state"`
	Suggestion   string `json:"suggestion"`
}

// Service is the QoS Policy Manager service.
type Service struct {
	log   *logger.Logger
	wotan *wotanClient.Client

	mu       sync.RWMutex
	policies map[string]*QoSPolicy
	stats    map[string]*QueueStats

	// Alert handling
	alertsCh   chan *wotanClient.Message
	wotanDrops int64

	// HTTP server
	server   *http.Server
	listener net.Listener
}

// NewService creates a new QoS Policy Manager service.
func NewService(log *logger.Logger, wotan *wotanClient.Client) *Service {
	return &Service{
		log:      log,
		wotan:    wotan,
		policies: make(map[string]*QoSPolicy),
		stats:    make(map[string]*QueueStats),
		alertsCh: make(chan *wotanClient.Message, 100),
	}
}

// Start initialises Wotan subscriptions, starts the HTTP server, and begins
// statistics aggregation at 10 Hz (every 100ms).
func (s *Service) Start(ctx context.Context) error {
	// Subscribe to Wotan topics.
	if s.wotan != nil {
		topics := []string{
			"qos.policy.updates",
			"qos.statistics",
			"qos.decisions",
			"alerts.critical",
		}
		for _, topic := range topics {
			if _, err := s.wotan.Subscribe(ctx, topic, "qos-service"); err != nil {
				s.log.Warn().Str("topic", topic).Err(err).Msg("failed to subscribe to topic")
			} else {
				s.log.Info().Str("topic", topic).Msg("subscribed to topic")
			}
		}
		go s.listenForAlerts(ctx)
	} else {
		s.log.Info().Msg("Wotan not configured, skipping subscriptions")
	}

	// Start stats aggregation (10 Hz).
	go s.aggregateStats(ctx)

	// Start HTTP server.
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := ports.DefaultAddr(ports.QoS)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("qos: listen on %s: %w", addr, err)
	}
	s.listener = ln

	s.server = &http.Server{
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	s.log.Info().Str("addr", addr).Int("port", ports.QoS).Msg("qos service starting")

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("qos HTTP server error")
		}
	}()

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("qos service stopping")

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(ctx); err != nil {
			s.log.Warn().Err(err).Msg("qos HTTP server shutdown error")
		}
	}

	close(s.alertsCh)
	return nil
}

// Addr returns the listener address (useful for tests).
func (s *Service) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// WotanDrops returns the total number of messages dropped in degraded mode.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// registerRoutes wires up the REST API endpoints.
func (s *Service) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/policy", s.handlePolicies)
	mux.HandleFunc("/api/v1/policy/", s.handlePolicyByService)
	mux.HandleFunc("/api/v1/stats", s.handleStats)
	mux.HandleFunc("/api/v1/stats/", s.handleStatsByService)
	mux.HandleFunc("/api/v1/congestion", s.handleCongestion)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)
}

// handlePolicies returns all QoS policies (GET).
func (s *Service) handlePolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	policies := make([]*QoSPolicy, 0, len(s.policies))
	for _, p := range s.policies {
		policies = append(policies, p)
	}
	s.mu.RUnlock()

	sort.Slice(policies, func(i, j int) bool {
		return policies[i].ServiceID < policies[j].ServiceID
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"policies": policies,
		"total":    len(policies),
	})
}

// handlePolicyByService handles GET and POST for /api/v1/policy/{service_id}.
func (s *Service) handlePolicyByService(w http.ResponseWriter, r *http.Request) {
	serviceID := strings.TrimPrefix(r.URL.Path, "/api/v1/policy/")
	if serviceID == "" {
		http.Error(w, `{"error":"service_id required"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		p, ok := s.policies[serviceID]
		s.mu.RUnlock()

		if !ok {
			http.Error(w, `{"error":"policy not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, p)

	case http.MethodPost:
		var p QoSPolicy
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid policy: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		// Ensure the service_id in the body matches the URL.
		p.ServiceID = serviceID

		if err := p.Validate(); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		s.policies[serviceID] = &p
		s.mu.Unlock()

		s.log.Info().
			Str("service_id", serviceID).
			Uint8("class", p.Class).
			Uint8("weight", p.Weight).
			Msg("qos policy updated")

		// Publish update to Wotan.
		s.publishPolicyUpdate(serviceID, &p)

		writeJSON(w, http.StatusOK, map[string]string{
			"status":     "updated",
			"service_id": serviceID,
		})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleStats returns queue statistics for all services.
func (s *Service) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	stats := make([]*QueueStats, 0, len(s.stats))
	for _, st := range s.stats {
		stats = append(stats, st)
	}
	s.mu.RUnlock()

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].ServiceID < stats[j].ServiceID
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stats": stats,
		"total": len(stats),
	})
}

// handleStatsByService returns stats for a specific service.
func (s *Service) handleStatsByService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	serviceID := strings.TrimPrefix(r.URL.Path, "/api/v1/stats/")
	if serviceID == "" {
		http.Error(w, `{"error":"service_id required"}`, http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	st, ok := s.stats[serviceID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, `{"error":"stats not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleCongestion returns the congestion map for all services.
func (s *Service) handleCongestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	congestion := make([]CongestionInfo, 0, len(s.policies))
	for serviceID := range s.policies {
		info := s.detectCongestion(serviceID)
		congestion = append(congestion, info)
	}
	s.mu.RUnlock()

	sort.Slice(congestion, func(i, j int) bool {
		return congestion[i].ServiceID < congestion[j].ServiceID
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"congestion": congestion,
		"total":      len(congestion),
	})
}

// handleHealth returns 200 if the service is running.
func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// handleReady returns 200 if the service is ready to serve.
func (s *Service) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// handleMetrics returns Prometheus-style metrics.
func (s *Service) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	policyCount := len(s.policies)
	statsCount := len(s.stats)

	var totalPackets, totalDrops uint64
	for _, st := range s.stats {
		totalPackets += st.Packets
		totalDrops += st.Drops
	}
	s.mu.RUnlock()

	drops := atomic.LoadInt64(&s.wotanDrops)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "# HELP unheaded_qos_policies_total Total QoS policies\n")
	fmt.Fprintf(w, "# TYPE unheaded_qos_policies_total gauge\n")
	fmt.Fprintf(w, "unheaded_qos_policies_total %d\n", policyCount)
	fmt.Fprintf(w, "# HELP unheaded_qos_services_monitored Services with stats\n")
	fmt.Fprintf(w, "# TYPE unheaded_qos_services_monitored gauge\n")
	fmt.Fprintf(w, "unheaded_qos_services_monitored %d\n", statsCount)
	fmt.Fprintf(w, "# HELP unheaded_qos_packets_total Total packets processed\n")
	fmt.Fprintf(w, "# TYPE unheaded_qos_packets_total counter\n")
	fmt.Fprintf(w, "unheaded_qos_packets_total %d\n", totalPackets)
	fmt.Fprintf(w, "# HELP unheaded_qos_drops_total Total packets dropped\n")
	fmt.Fprintf(w, "# TYPE unheaded_qos_drops_total counter\n")
	fmt.Fprintf(w, "unheaded_qos_drops_total %d\n", totalDrops)
	fmt.Fprintf(w, "# HELP unheaded_qos_wotan_drops_total Wotan degraded-mode drops\n")
	fmt.Fprintf(w, "# TYPE unheaded_qos_wotan_drops_total counter\n")
	fmt.Fprintf(w, "unheaded_qos_wotan_drops_total %d\n", drops)
}

// SetPolicy adds or updates a QoS policy for a service.
func (s *Service) SetPolicy(p *QoSPolicy) error {
	if p == nil {
		return fmt.Errorf("policy cannot be nil")
	}
	if err := p.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	s.policies[p.ServiceID] = p
	s.mu.Unlock()
	return nil
}

// GetPolicy returns the policy for a given service.
func (s *Service) GetPolicy(serviceID string) (*QoSPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[serviceID]
	return p, ok
}

// UpdateStats records queue statistics for a service.
func (s *Service) UpdateStats(st *QueueStats) {
	if st == nil || st.ServiceID == "" {
		return
	}
	st.Timestamp = time.Now()
	s.mu.Lock()
	s.stats[st.ServiceID] = st
	s.mu.Unlock()
}

// GetStats returns stats for a given service.
func (s *Service) GetStats(serviceID string) (*QueueStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.stats[serviceID]
	return st, ok
}

// DetectCongestion evaluates the congestion state for a service.
// Must be called with mu held (at least RLock).
func (s *Service) detectCongestion(serviceID string) CongestionInfo {
	info := CongestionInfo{
		ServiceID:    serviceID,
		CircuitState: CircuitClosed,
		Suggestion:   "traffic normal",
	}

	st, ok := s.stats[serviceID]
	if !ok {
		return info
	}

	policy, ok := s.policies[serviceID]
	if !ok {
		return info
	}

	targetMS := float64(policy.TargetLatencyMS)
	if targetMS <= 0 {
		targetMS = float64(DefaultTargetMS)
	}

	// Evaluate congestion based on latency vs target.
	if st.LatencyP99 > targetMS*5 {
		info.CircuitState = CircuitOpen
		info.Suggestion = fmt.Sprintf("p99 latency %.1fms exceeds 5x target %.1fms - circuit OPEN, shed traffic", st.LatencyP99, targetMS)
	} else if st.LatencyP99 > targetMS*2 {
		info.CircuitState = CircuitHalfOpen
		info.Suggestion = fmt.Sprintf("p99 latency %.1fms exceeds 2x target %.1fms - circuit HALF_OPEN, reduce rate", st.LatencyP99, targetMS)
	}

	// Also check drop probability.
	if st.DropProbability > 0.1 {
		if info.CircuitState == CircuitClosed {
			info.CircuitState = CircuitHalfOpen
		}
		info.Suggestion += fmt.Sprintf("; drop probability %.2f%% is elevated", st.DropProbability*100)
	}

	return info
}

// DetectCongestionExported is the exported wrapper for congestion detection (for tests).
func (s *Service) DetectCongestionExported(serviceID string) CongestionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.detectCongestion(serviceID)
}

// publishPolicyUpdate publishes a policy update event to Wotan.
func (s *Service) publishPolicyUpdate(serviceID string, p *QoSPolicy) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		return
	}

	event := map[string]interface{}{
		"event_type": "qos.policy.updated",
		"service":    "qos",
		"service_id": serviceID,
		"policy":     p,
		"timestamp":  time.Now().UnixMilli(),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to marshal policy update event")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.wotan.Publish(ctx, "qos.policy.updates", payload); err != nil {
		s.log.Warn().Err(err).Msg("failed to publish policy update")
	}
}

// aggregateStats runs at 10 Hz (every 100ms) to publish stats snapshots.
func (s *Service) aggregateStats(ctx context.Context) {
	ticker := time.NewTicker(statsAggregationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.publishStatsSnapshot(ctx)
		}
	}
}

// publishStatsSnapshot publishes the current queue stats to Wotan.
func (s *Service) publishStatsSnapshot(ctx context.Context) {
	s.mu.RLock()
	allStats := make([]*QueueStats, 0, len(s.stats))
	for _, st := range s.stats {
		allStats = append(allStats, st)
	}
	s.mu.RUnlock()

	if len(allStats) == 0 {
		return
	}

	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		return
	}

	event := map[string]interface{}{
		"event_type": "qos.statistics",
		"service":    "qos",
		"stats":      allStats,
		"timestamp":  time.Now().UnixMilli(),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to marshal stats snapshot")
		return
	}

	pubCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.wotan.Publish(pubCtx, "qos.statistics", payload); err != nil {
		s.log.Warn().Err(err).Msg("failed to publish stats snapshot")
	}
}

// listenForAlerts listens for critical alerts from Wotan.
func (s *Service) listenForAlerts(ctx context.Context) {
	if s.wotan == nil {
		s.log.Warn().Msg("wotan unavailable - alert listener disabled (degraded mode)")
		return
	}

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
			s.handleCriticalAlert(msg)
		}
	}
}

// handleCriticalAlert processes a critical alert message.
func (s *Service) handleCriticalAlert(msg *wotanClient.Message) {
	if msg == nil {
		return
	}
	s.log.Warn().
		Str("message_id", msg.MessageID).
		Str("topic", msg.Topic).
		Msg("qos received critical alert")
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
