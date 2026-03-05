// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package selfheal provides self-healing network infrastructure with circuit breakers.
// Self-Heal the Restoration Engine is the Kingdom's immune system - continuously
// monitoring endpoint health, triggering failovers when degradation is detected,
// and orchestrating recovery when services return to healthy state.
package selfheal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState string

const (
	CircuitClosed   CircuitState = "CLOSED"
	CircuitOpen     CircuitState = "OPEN"
	CircuitHalfOpen CircuitState = "HALF_OPEN"
)

// Health score weight constants.
const (
	weightLatency = 0.40
	weightErrors  = 0.30
	weightLoss    = 0.20
	weightCircuit = 0.10
)

// Circuit breaker thresholds.
const (
	circuitOpenThreshold    = 50  // health score below which circuit opens
	circuitOpenFailCount    = 3   // minimum fail count to open circuit
	circuitCloseThreshold   = 75  // health score above which circuit closes
	circuitCloseConsecutive = 5   // consecutive healthy checks to close
)

// EndpointHealth represents the health status of a monitored endpoint.
type EndpointHealth struct {
	EndpointID     string       `json:"endpoint_id"`
	HealthScore    int          `json:"health_score"` // 0-100
	FailCount      int          `json:"fail_count"`
	SuccessStreak  int          `json:"success_streak"`
	LastSeen       time.Time    `json:"last_seen"`
	LatencyBuckets [5]uint64    `json:"latency_buckets"` // lt1ms, 1-5ms, 5-10ms, 10-50ms, gt50ms
	CircuitState   CircuitState `json:"circuit_state"`
	LastFailover   *time.Time   `json:"last_failover,omitempty"`
	LastRecovery   *time.Time   `json:"last_recovery,omitempty"`
}

// BackupRoster maps a primary endpoint to its backup list.
type BackupRoster struct {
	PrimaryID string   `json:"primary_id"`
	Backups   []string `json:"backups"`
	Count     int      `json:"count"`
}

// FailoverEvent records when a failover is triggered.
type FailoverEvent struct {
	Timestamp      time.Time `json:"timestamp"`
	EndpointID     string    `json:"endpoint_id"`
	BackupID       string    `json:"backup_id"`
	Reason         string    `json:"reason"`
	HealthScore    int       `json:"health_score"`
	AffectedFlows  int       `json:"affected_flows"`
	BlastRadiusPct float64   `json:"blast_radius_pct"`
}

// RecoveryEvent records when a recovery is detected.
type RecoveryEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	EndpointID   string    `json:"endpoint_id"`
	HealthScore  int       `json:"health_score"`
	DowntimeMs   int64     `json:"downtime_ms"`
}

// Config holds Self-Heal service configuration.
type Config struct {
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	DefaultHealthScore  int           `json:"default_health_score"`
	WotanTopic          string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		HealthCheckInterval: 5 * time.Second,
		DefaultHealthScore:  100,
		WotanTopic:          "network.failover",
	}
}

// Service is the main Self-Healing service.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu        sync.RWMutex
	endpoints map[string]*EndpointHealth // endpoint_id -> health
	circuits  map[string]CircuitState    // endpoint_id -> circuit state
	backups   map[string]*BackupRoster   // primary_id -> roster
	events    []FailoverEvent            // recent failover events

	httpServer *http.Server
	started    int32

	// Alert handling
	alertsCh chan *wotanClient.Message

	// Degraded mode counter
	wotanDrops int64
}

// NewService creates a new Self-Healing service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:       log,
		wotan:     wotan,
		config:    cfg,
		endpoints: make(map[string]*EndpointHealth),
		circuits:  make(map[string]CircuitState),
		backups:   make(map[string]*BackupRoster),
		events:    make([]FailoverEvent, 0),
		alertsCh:  make(chan *wotanClient.Message, 100),
	}
}

// Start initializes Wotan subscriptions, HTTP server, and health monitoring.
func (s *Service) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		return fmt.Errorf("selfheal service already started")
	}

	s.log.Info().Msg("self-healing service starting")

	// Subscribe to Wotan topics
	if s.wotan != nil {
		topics := []string{
			"network.failover",
			"network.recovery",
			"network.circuit_state",
			"alerts.critical",
		}
		for _, topic := range topics {
			if _, err := s.wotan.Subscribe(ctx, topic, "selfheal-service"); err != nil {
				s.log.Warn().Err(err).Str("topic", topic).Msg("failed to subscribe")
			} else {
				s.log.Info().Str("topic", topic).Msg("subscribed")
			}
		}

		// Start alert listener
		go s.listenForAlerts(ctx)
	} else {
		s.log.Info().Msg("Wotan not configured, skipping subscriptions")
	}

	// Start HTTP server
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:           ports.DefaultAddr(ports.BackupMgr),
		Handler:        mux,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Start health monitoring goroutine
	go s.healthMonitorLoop(ctx)

	s.log.Info().Int("port", ports.BackupMgr).Msg("self-healing service started")
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("self-healing service stopping")

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.log.Error().Err(err).Msg("HTTP server shutdown error")
			return err
		}
	}

	close(s.alertsCh)
	atomic.StoreInt32(&s.started, 0)
	s.log.Info().Msg("self-healing service stopped")
	return nil
}

// registerRoutes sets up all HTTP API endpoints.
func (s *Service) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/api/v1/endpoints", s.handleEndpoints)
	mux.HandleFunc("/api/v1/endpoints/", s.handleEndpointByID)
	mux.HandleFunc("/api/v1/backups", s.handleBackups)
	mux.HandleFunc("/api/v1/circuit-breakers", s.handleCircuitBreakers)
}

// handleHealth responds with service health status.
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "selfheal",
		"timestamp": time.Now(),
	})
}

// handleReady responds with readiness status.
func (s *Service) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":   atomic.LoadInt32(&s.started) == 1,
		"service": "selfheal",
	})
}

// handleMetrics exposes Prometheus-style metrics.
func (s *Service) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	endpointCount := len(s.endpoints)
	var healthy, degraded, failed int
	var openCircuits, halfOpenCircuits, closedCircuits int
	for _, ep := range s.endpoints {
		switch {
		case ep.HealthScore >= 75:
			healthy++
		case ep.HealthScore >= 50:
			degraded++
		default:
			failed++
		}
		switch ep.CircuitState {
		case CircuitClosed:
			closedCircuits++
		case CircuitOpen:
			openCircuits++
		case CircuitHalfOpen:
			halfOpenCircuits++
		}
	}
	eventCount := len(s.events)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP selfheal_endpoints_total Total monitored endpoints\n")
	fmt.Fprintf(w, "# TYPE selfheal_endpoints_total gauge\n")
	fmt.Fprintf(w, "selfheal_endpoints_total %d\n", endpointCount)
	fmt.Fprintf(w, "# HELP selfheal_endpoints_by_health Endpoints by health category\n")
	fmt.Fprintf(w, "# TYPE selfheal_endpoints_by_health gauge\n")
	fmt.Fprintf(w, "selfheal_endpoints_by_health{category=\"healthy\"} %d\n", healthy)
	fmt.Fprintf(w, "selfheal_endpoints_by_health{category=\"degraded\"} %d\n", degraded)
	fmt.Fprintf(w, "selfheal_endpoints_by_health{category=\"failed\"} %d\n", failed)
	fmt.Fprintf(w, "# HELP selfheal_circuit_breakers Circuit breaker states\n")
	fmt.Fprintf(w, "# TYPE selfheal_circuit_breakers gauge\n")
	fmt.Fprintf(w, "selfheal_circuit_breakers{state=\"CLOSED\"} %d\n", closedCircuits)
	fmt.Fprintf(w, "selfheal_circuit_breakers{state=\"OPEN\"} %d\n", openCircuits)
	fmt.Fprintf(w, "selfheal_circuit_breakers{state=\"HALF_OPEN\"} %d\n", halfOpenCircuits)
	fmt.Fprintf(w, "# HELP selfheal_failover_events_total Total failover events\n")
	fmt.Fprintf(w, "# TYPE selfheal_failover_events_total counter\n")
	fmt.Fprintf(w, "selfheal_failover_events_total %d\n", eventCount)
	fmt.Fprintf(w, "# HELP selfheal_wotan_drops_total Messages dropped due to unavailable Wotan\n")
	fmt.Fprintf(w, "# TYPE selfheal_wotan_drops_total counter\n")
	fmt.Fprintf(w, "selfheal_wotan_drops_total %d\n", atomic.LoadInt64(&s.wotanDrops))
}

// handleEndpoints handles GET on /api/v1/endpoints.
func (s *Service) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	endpoints := make([]*EndpointHealth, 0, len(s.endpoints))
	for _, ep := range s.endpoints {
		endpoints = append(endpoints, ep)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"endpoints": endpoints,
		"count":     len(endpoints),
	})
}

// handleEndpointByID dispatches requests for /api/v1/endpoints/{id}/*.
func (s *Service) handleEndpointByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/endpoints/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "endpoint id required", http.StatusBadRequest)
		return
	}

	endpointID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	switch {
	case action == "health" && r.Method == http.MethodGet:
		s.getEndpointHealth(w, r, endpointID)
	case action == "failover" && r.Method == http.MethodPost:
		s.triggerFailover(w, r, endpointID)
	case action == "recover" && r.Method == http.MethodPost:
		s.triggerRecovery(w, r, endpointID)
	case action == "" && r.Method == http.MethodGet:
		s.getEndpointHealth(w, r, endpointID)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// getEndpointHealth returns detailed health for an endpoint.
func (s *Service) getEndpointHealth(w http.ResponseWriter, _ *http.Request, endpointID string) {
	s.mu.RLock()
	ep, ok := s.endpoints[endpointID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "endpoint not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ep)
}

// triggerFailover performs manual failover for an endpoint.
func (s *Service) triggerFailover(w http.ResponseWriter, _ *http.Request, endpointID string) {
	s.mu.Lock()
	ep, ok := s.endpoints[endpointID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "endpoint not found", http.StatusNotFound)
		return
	}

	roster, hasBackup := s.backups[endpointID]
	if !hasBackup || len(roster.Backups) == 0 {
		s.mu.Unlock()
		http.Error(w, "no backup available for endpoint", http.StatusConflict)
		return
	}

	backupID := roster.Backups[0]
	now := time.Now()
	ep.CircuitState = CircuitOpen
	ep.LastFailover = &now

	event := FailoverEvent{
		Timestamp:      now,
		EndpointID:     endpointID,
		BackupID:       backupID,
		Reason:         "manual_failover",
		HealthScore:    ep.HealthScore,
		AffectedFlows:  0,
		BlastRadiusPct: 0.0,
	}
	s.events = append(s.events, event)
	s.circuits[endpointID] = CircuitOpen
	s.mu.Unlock()

	s.log.Warn().
		Str("endpoint_id", endpointID).
		Str("backup_id", backupID).
		Msg("manual failover triggered")

	s.publishEvent(context.Background(), "network.failover", map[string]interface{}{
		"endpoint_id": endpointID,
		"backup_id":   backupID,
		"reason":      "manual_failover",
		"timestamp":   now.UnixMilli(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
}

// triggerRecovery performs manual recovery for an endpoint.
func (s *Service) triggerRecovery(w http.ResponseWriter, _ *http.Request, endpointID string) {
	s.mu.Lock()
	ep, ok := s.endpoints[endpointID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "endpoint not found", http.StatusNotFound)
		return
	}

	now := time.Now()
	ep.CircuitState = CircuitClosed
	ep.HealthScore = s.config.DefaultHealthScore
	ep.FailCount = 0
	ep.SuccessStreak = circuitCloseConsecutive
	ep.LastRecovery = &now
	s.circuits[endpointID] = CircuitClosed
	s.mu.Unlock()

	s.log.Info().
		Str("endpoint_id", endpointID).
		Msg("manual recovery triggered")

	s.publishEvent(context.Background(), "network.recovery", map[string]interface{}{
		"endpoint_id":  endpointID,
		"health_score": s.config.DefaultHealthScore,
		"timestamp":    now.UnixMilli(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"endpoint_id":  endpointID,
		"health_score": ep.HealthScore,
		"circuit":      ep.CircuitState,
		"recovered_at": now,
	})
}

// handleBackups handles GET (list) and POST (update) on /api/v1/backups.
func (s *Service) handleBackups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listBackups(w, r)
	case http.MethodPost:
		s.updateBackup(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listBackups returns the full backup roster.
func (s *Service) listBackups(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	rosters := make([]*BackupRoster, 0, len(s.backups))
	for _, r := range s.backups {
		rosters = append(rosters, r)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"backups": rosters,
		"count":   len(rosters),
	})
}

// updateBackupRequest is the payload for updating a backup roster.
type updateBackupRequest struct {
	PrimaryID string   `json:"primary_id"`
	Backups   []string `json:"backups"`
}

// updateBackup creates or updates a backup roster entry.
func (s *Service) updateBackup(w http.ResponseWriter, r *http.Request) {
	var req updateBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.PrimaryID == "" {
		http.Error(w, "primary_id is required", http.StatusBadRequest)
		return
	}

	roster := &BackupRoster{
		PrimaryID: req.PrimaryID,
		Backups:   req.Backups,
		Count:     len(req.Backups),
	}

	s.mu.Lock()
	s.backups[req.PrimaryID] = roster
	s.mu.Unlock()

	s.log.Info().
		Str("primary_id", req.PrimaryID).
		Int("backup_count", len(req.Backups)).
		Msg("backup roster updated")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(roster)
}

// handleCircuitBreakers returns all circuit breaker states.
func (s *Service) handleCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	breakers := make(map[string]CircuitState, len(s.circuits))
	for id, state := range s.circuits {
		breakers[id] = state
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"circuit_breakers": breakers,
		"count":            len(breakers),
	})
}

// healthMonitorLoop runs health checks every HealthCheckInterval.
func (s *Service) healthMonitorLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runHealthChecks(ctx)
		}
	}
}

// runHealthChecks evaluates all endpoints and updates circuit breakers.
func (s *Service) runHealthChecks(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, ep := range s.endpoints {
		s.evaluateEndpoint(ctx, id, ep)
	}
}

// evaluateEndpoint applies health scoring and circuit breaker logic to one endpoint.
func (s *Service) evaluateEndpoint(ctx context.Context, id string, ep *EndpointHealth) {
	ep.LastSeen = time.Now()

	// Calculate health score: weighted(latency 40%, errors 30%, loss 20%, circuit 10%)
	score := s.calculateHealthScore(ep)
	ep.HealthScore = score

	// Circuit breaker state machine
	switch ep.CircuitState {
	case CircuitClosed:
		// CLOSED -> OPEN: health < 50 && fails > 3
		if score < circuitOpenThreshold && ep.FailCount > circuitOpenFailCount {
			ep.CircuitState = CircuitOpen
			ep.SuccessStreak = 0
			s.circuits[id] = CircuitOpen

			s.log.Warn().
				Str("endpoint_id", id).
				Int("health_score", score).
				Int("fail_count", ep.FailCount).
				Msg("circuit breaker OPENED")

			// Trigger automatic failover
			s.autoFailover(ctx, id, ep)
		}

	case CircuitOpen:
		// OPEN -> HALF_OPEN: any success (score improvement)
		if score > circuitOpenThreshold {
			ep.CircuitState = CircuitHalfOpen
			ep.SuccessStreak = 1
			s.circuits[id] = CircuitHalfOpen

			s.log.Info().
				Str("endpoint_id", id).
				Int("health_score", score).
				Msg("circuit breaker moved to HALF_OPEN")
		}

	case CircuitHalfOpen:
		if score > circuitCloseThreshold {
			ep.SuccessStreak++
			// HALF_OPEN -> CLOSED: health > 75 for 5 consecutive checks
			if ep.SuccessStreak >= circuitCloseConsecutive {
				ep.CircuitState = CircuitClosed
				ep.FailCount = 0
				s.circuits[id] = CircuitClosed
				now := time.Now()
				ep.LastRecovery = &now

				s.log.Info().
					Str("endpoint_id", id).
					Int("health_score", score).
					Msg("circuit breaker CLOSED — recovery complete")

				go s.publishEvent(ctx, "network.recovery", map[string]interface{}{
					"endpoint_id":  id,
					"health_score": score,
					"timestamp":    now.UnixMilli(),
				})
			}
		} else {
			// Failed check — back to OPEN
			ep.CircuitState = CircuitOpen
			ep.SuccessStreak = 0
			s.circuits[id] = CircuitOpen

			s.log.Warn().
				Str("endpoint_id", id).
				Int("health_score", score).
				Msg("circuit breaker reopened from HALF_OPEN")
		}
	}
}

// calculateHealthScore computes a weighted health score (0-100) for an endpoint.
func (s *Service) calculateHealthScore(ep *EndpointHealth) int {
	var totalSamples uint64
	for _, count := range ep.LatencyBuckets {
		totalSamples += count
	}

	// Latency score (40%): proportion of fast requests (< 10ms)
	latencyScore := 100.0
	if totalSamples > 0 {
		fast := float64(ep.LatencyBuckets[0] + ep.LatencyBuckets[1] + ep.LatencyBuckets[2])
		latencyScore = (fast / float64(totalSamples)) * 100.0
	}

	// Error score (30%): inverse of fail count (max 10 fails = 0 score)
	errorScore := 100.0
	if ep.FailCount > 0 {
		errorScore = 100.0 - float64(ep.FailCount)*10.0
		if errorScore < 0 {
			errorScore = 0
		}
	}

	// Loss score (20%): based on latency bucket distribution (heavy tail = loss)
	lossScore := 100.0
	if totalSamples > 0 {
		slowRatio := float64(ep.LatencyBuckets[4]) / float64(totalSamples) // gt50ms
		lossScore = (1.0 - slowRatio) * 100.0
	}

	// Circuit score (10%): penalize open/half-open states
	circuitScore := 100.0
	switch ep.CircuitState {
	case CircuitOpen:
		circuitScore = 0.0
	case CircuitHalfOpen:
		circuitScore = 50.0
	}

	weighted := latencyScore*weightLatency +
		errorScore*weightErrors +
		lossScore*weightLoss +
		circuitScore*weightCircuit

	score := int(weighted)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// autoFailover triggers automatic failover when circuit opens.
func (s *Service) autoFailover(ctx context.Context, endpointID string, ep *EndpointHealth) {
	roster, ok := s.backups[endpointID]
	if !ok || len(roster.Backups) == 0 {
		s.log.Warn().
			Str("endpoint_id", endpointID).
			Msg("no backup roster for failover")
		return
	}

	backupID := s.selectBackup(roster)
	now := time.Now()
	ep.LastFailover = &now

	event := FailoverEvent{
		Timestamp:      now,
		EndpointID:     endpointID,
		BackupID:       backupID,
		Reason:         "health_score_breach",
		HealthScore:    ep.HealthScore,
		AffectedFlows:  0,
		BlastRadiusPct: 0.0,
	}
	s.events = append(s.events, event)

	s.log.Warn().
		Str("endpoint_id", endpointID).
		Str("backup_id", backupID).
		Int("health_score", ep.HealthScore).
		Msg("automatic failover triggered")

	go s.publishEvent(ctx, "network.failover", map[string]interface{}{
		"endpoint_id":  endpointID,
		"backup_id":    backupID,
		"reason":       "health_score_breach",
		"health_score": ep.HealthScore,
		"timestamp":    now.UnixMilli(),
	})
}

// selectBackup chooses the best backup from a roster (first available).
func (s *Service) selectBackup(roster *BackupRoster) string {
	for _, backupID := range roster.Backups {
		// Prefer backups whose circuits are not open
		if state, ok := s.circuits[backupID]; ok && state == CircuitOpen {
			continue
		}
		return backupID
	}
	// Fallback: return first backup regardless of state
	if len(roster.Backups) > 0 {
		return roster.Backups[0]
	}
	return ""
}

// listenForAlerts listens for critical alerts from Wotan.
func (s *Service) listenForAlerts(ctx context.Context) {
	if s.wotan == nil {
		s.log.Warn().Msg("wotan unavailable — alert listener disabled (degraded mode)")
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
			s.handleCriticalAlert(ctx, msg)
		}
	}
}

// handleCriticalAlert processes critical alerts.
func (s *Service) handleCriticalAlert(_ context.Context, msg *wotanClient.Message) {
	if msg == nil {
		return
	}

	s.log.Warn().
		Str("message_id", msg.MessageID).
		Str("topic", msg.Topic).
		Msg("received critical alert — checking endpoint health")
}

// publishEvent publishes an event to Wotan.
func (s *Service) publishEvent(ctx context.Context, topic string, event map[string]interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		s.log.Warn().Str("topic", topic).Msg("wotan unavailable — event dropped (degraded mode)")
		return
	}

	event["service"] = "selfheal"
	payload, err := json.Marshal(event)
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to marshal event")
		return
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.wotan.Publish(pubCtx, topic, payload); err != nil {
		s.log.Warn().Err(err).Str("topic", topic).Msg("failed to publish event")
	}
}

// WotanDrops returns the total number of messages dropped because Wotan was nil.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// RegisterEndpoint adds or updates an endpoint in the health map.
func (s *Service) RegisterEndpoint(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.endpoints[id]; !exists {
		s.endpoints[id] = &EndpointHealth{
			EndpointID:   id,
			HealthScore:  s.config.DefaultHealthScore,
			CircuitState: CircuitClosed,
			LastSeen:     time.Now(),
		}
		s.circuits[id] = CircuitClosed
	}
}

// GetEndpoint returns an endpoint's health status.
func (s *Service) GetEndpoint(id string) (*EndpointHealth, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ep, ok := s.endpoints[id]
	return ep, ok
}

// ListEndpoints returns all endpoints.
func (s *Service) ListEndpoints() []*EndpointHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*EndpointHealth, 0, len(s.endpoints))
	for _, ep := range s.endpoints {
		result = append(result, ep)
	}
	return result
}

// GetEvents returns recent failover events.
func (s *Service) GetEvents() []FailoverEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]FailoverEvent, len(s.events))
	copy(result, s.events)
	return result
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var healthy, degraded, failed int
	var open, halfOpen, closed int
	for _, ep := range s.endpoints {
		switch {
		case ep.HealthScore >= 75:
			healthy++
		case ep.HealthScore >= 50:
			degraded++
		default:
			failed++
		}
		switch ep.CircuitState {
		case CircuitClosed:
			closed++
		case CircuitOpen:
			open++
		case CircuitHalfOpen:
			halfOpen++
		}
	}

	return map[string]interface{}{
		"total_endpoints":      len(s.endpoints),
		"healthy_endpoints":    healthy,
		"degraded_endpoints":   degraded,
		"failed_endpoints":     failed,
		"circuit_closed":       closed,
		"circuit_open":         open,
		"circuit_half_open":    halfOpen,
		"total_backup_rosters": len(s.backups),
		"total_failover_events": len(s.events),
		"wotan_drops":          atomic.LoadInt64(&s.wotanDrops),
	}
}
