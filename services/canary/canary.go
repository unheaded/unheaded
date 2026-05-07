// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package canary provides canary deployment management with automated SLO evaluation.
// Canary the Deployment Guardian is the Kingdom's progressive delivery engine -
// gradually shifting traffic to new versions while continuously monitoring service
// health. If SLOs breach, traffic is instantly rolled back to the stable version.
package canary

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

// CanaryState represents the lifecycle phase of a canary deployment.
type CanaryState string

const (
	StateProbing  CanaryState = "PROBING"
	StateRamp     CanaryState = "RAMP"
	StateStable   CanaryState = "STABLE"
	StateRollback CanaryState = "ROLLBACK"
)

// SLOThreshold defines the acceptable bounds for a canary's service level objectives.
type SLOThreshold struct {
	MaxErrorRatePPM  int64   `json:"max_error_rate_ppm"` // Parts per million
	MaxLatencyP99MS  float64 `json:"max_latency_p99_ms"` // P99 latency ceiling
	MinThroughputPPS float64 `json:"min_throughput_pps"` // Packets per second floor
}

// DefaultSLOThreshold returns conservative SLO defaults.
func DefaultSLOThreshold() *SLOThreshold {
	return &SLOThreshold{
		MaxErrorRatePPM:  5000,  // 0.5%
		MaxLatencyP99MS:  500.0, // 500ms
		MinThroughputPPS: 10.0,  // 10 req/s
	}
}

// CanaryDeployment describes a single canary deployment lifecycle.
type CanaryDeployment struct {
	ServiceID       string        `json:"service_id"`
	CanaryVersion   string        `json:"canary_version"`
	StableVersion   string        `json:"stable_version"`
	TrafficSplitPct int           `json:"traffic_split_pct"`
	State           CanaryState   `json:"state"`
	ProbeStartTime  time.Time     `json:"probe_start_time"`
	LastEvaluation  time.Time     `json:"last_evaluation"`
	LastRampTime    time.Time     `json:"last_ramp_time"`
	SLO             *SLOThreshold `json:"slo"`
	CreatedAt       time.Time     `json:"created_at"`
	CompletedAt     *time.Time    `json:"completed_at,omitempty"`
	RollbackReason  string        `json:"rollback_reason,omitempty"`
	EvalCount       int64         `json:"eval_count"`
}

// CanaryMetrics holds observed metrics for a specific version.
type CanaryMetrics struct {
	ServiceID   string  `json:"service_id"`
	Version     string  `json:"version"`
	ErrorRate   float64 `json:"error_rate"`  // errors per million
	LatencyP99  float64 `json:"latency_p99"` // ms
	Throughput  float64 `json:"throughput"`  // requests per second
	SampleCount int64   `json:"sample_count"`
}

// MetricsComparison holds canary vs stable metrics for a service.
type MetricsComparison struct {
	ServiceID     string         `json:"service_id"`
	CanaryMetrics *CanaryMetrics `json:"canary_metrics"`
	StableMetrics *CanaryMetrics `json:"stable_metrics"`
	SLO           *SLOThreshold  `json:"slo"`
	Healthy       bool           `json:"healthy"`
	Timestamp     time.Time      `json:"timestamp"`
}

// Config holds Canary service configuration.
type Config struct {
	EvalInterval    time.Duration `json:"eval_interval"`
	ProbeDuration   time.Duration `json:"probe_duration"`
	RampInterval    time.Duration `json:"ramp_interval"`
	RampStepPct     int           `json:"ramp_step_pct"`
	InitialSplitPct int           `json:"initial_split_pct"`
	WotanTopic      string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		EvalInterval:    5 * time.Second,
		ProbeDuration:   5 * time.Minute,
		RampInterval:    2 * time.Minute,
		RampStepPct:     10,
		InitialSplitPct: 5,
		WotanTopic:      "monad.canary.metrics",
	}
}

// Service is the main Canary Deployment service.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu          sync.RWMutex
	deployments map[string]*CanaryDeployment  // service_id -> deployment
	metrics     map[string]*MetricsComparison // service_id -> latest comparison

	httpServer *http.Server
	started    int32

	// Alert handling
	alertsCh chan *wotanClient.Message

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64
}

// NewService creates a new Canary deployment service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:         log,
		wotan:       wotan,
		config:      cfg,
		deployments: make(map[string]*CanaryDeployment),
		metrics:     make(map[string]*MetricsComparison),
		alertsCh:    make(chan *wotanClient.Message, 100),
	}
}

// Start initializes Wotan subscriptions, HTTP server, and the evaluation loop.
func (s *Service) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		return fmt.Errorf("canary service already started")
	}

	s.log.Info().Msg("canary deployment service starting")

	// Subscribe to Wotan topics
	if s.wotan != nil {
		topics := []string{
			"monad.canary.metrics",
			"monad.canary.promotion",
			"monad.canary.rollback",
			"monad.canary.circuit_breaker",
			"alerts.critical",
		}
		for _, topic := range topics {
			if _, err := s.wotan.Subscribe(ctx, topic, "canary-service"); err != nil {
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
		Addr:           ports.DefaultAddr(ports.Canary),
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

	// Start canary evaluation loop
	go s.evaluationLoop(ctx)

	s.log.Info().Int("port", ports.Canary).Msg("canary deployment service started")
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("canary deployment service stopping")

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
	s.log.Info().Msg("canary deployment service stopped")
	return nil
}

// registerRoutes sets up all HTTP API endpoints.
func (s *Service) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/api/v1/canaries", s.handleCanaries)
	mux.HandleFunc("/api/v1/canaries/", s.handleCanaryByID)
}

// handleHealth responds with service health status.
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "canary",
		"timestamp": time.Now(),
	})
}

// handleReady responds with readiness status.
func (s *Service) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":   atomic.LoadInt32(&s.started) == 1,
		"service": "canary",
	})
}

// handleMetrics exposes Prometheus-style metrics.
func (s *Service) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	activeCount := len(s.deployments)
	var probingCount, rampCount, stableCount, rollbackCount int
	for _, d := range s.deployments {
		switch d.State {
		case StateProbing:
			probingCount++
		case StateRamp:
			rampCount++
		case StateStable:
			stableCount++
		case StateRollback:
			rollbackCount++
		}
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP canary_deployments_active Total active canary deployments\n")
	fmt.Fprintf(w, "# TYPE canary_deployments_active gauge\n")
	fmt.Fprintf(w, "canary_deployments_active %d\n", activeCount)
	fmt.Fprintf(w, "# HELP canary_deployments_by_state Canary deployments by state\n")
	fmt.Fprintf(w, "# TYPE canary_deployments_by_state gauge\n")
	fmt.Fprintf(w, "canary_deployments_by_state{state=\"PROBING\"} %d\n", probingCount)
	fmt.Fprintf(w, "canary_deployments_by_state{state=\"RAMP\"} %d\n", rampCount)
	fmt.Fprintf(w, "canary_deployments_by_state{state=\"STABLE\"} %d\n", stableCount)
	fmt.Fprintf(w, "canary_deployments_by_state{state=\"ROLLBACK\"} %d\n", rollbackCount)
	fmt.Fprintf(w, "# HELP canary_wotan_drops_total Messages dropped due to unavailable Wotan\n")
	fmt.Fprintf(w, "# TYPE canary_wotan_drops_total counter\n")
	fmt.Fprintf(w, "canary_wotan_drops_total %d\n", atomic.LoadInt64(&s.wotanDrops))
}

// handleCanaries handles GET (list) and POST (create) on /api/v1/canaries.
func (s *Service) handleCanaries(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCanaries(w, r)
	case http.MethodPost:
		s.createCanary(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listCanaries returns all active canary deployments.
func (s *Service) listCanaries(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	deployments := make([]*CanaryDeployment, 0, len(s.deployments))
	for _, d := range s.deployments {
		deployments = append(deployments, d)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"canaries": deployments,
		"count":    len(deployments),
	})
}

// createCanaryRequest is the payload for creating a canary deployment.
type createCanaryRequest struct {
	ServiceID       string `json:"service_id"`
	CanaryVersion   string `json:"canary_version"`
	StableVersion   string `json:"stable_version"`
	TrafficSplitPct int    `json:"traffic_split_pct"`
}

// createCanary creates a new canary deployment.
func (s *Service) createCanary(w http.ResponseWriter, r *http.Request) {
	var req createCanaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.ServiceID == "" || req.CanaryVersion == "" || req.StableVersion == "" {
		http.Error(w, "service_id, canary_version, and stable_version are required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if _, exists := s.deployments[req.ServiceID]; exists {
		s.mu.Unlock()
		http.Error(w, fmt.Sprintf("canary deployment already exists for service %s", req.ServiceID), http.StatusConflict)
		return
	}

	splitPct := req.TrafficSplitPct
	if splitPct <= 0 {
		splitPct = s.config.InitialSplitPct
	}

	now := time.Now()
	deployment := &CanaryDeployment{
		ServiceID:       req.ServiceID,
		CanaryVersion:   req.CanaryVersion,
		StableVersion:   req.StableVersion,
		TrafficSplitPct: splitPct,
		State:           StateProbing,
		ProbeStartTime:  now,
		LastEvaluation:  now,
		LastRampTime:    now,
		SLO:             DefaultSLOThreshold(),
		CreatedAt:       now,
	}

	s.deployments[req.ServiceID] = deployment
	s.mu.Unlock()

	s.log.Info().
		Str("service_id", req.ServiceID).
		Str("canary_version", req.CanaryVersion).
		Str("stable_version", req.StableVersion).
		Int("traffic_split_pct", splitPct).
		Msg("canary deployment created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(deployment)
}

// handleCanaryByID dispatches requests for /api/v1/canaries/{service_id}/*.
func (s *Service) handleCanaryByID(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/v1/canaries/{service_id}[/action]
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/canaries/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "service_id required", http.StatusBadRequest)
		return
	}

	serviceID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	switch {
	case action == "metrics" && r.Method == http.MethodGet:
		s.getCanaryMetrics(w, r, serviceID)
	case action == "promote" && r.Method == http.MethodPost:
		s.promoteCanary(w, r, serviceID)
	case action == "rollback" && r.Method == http.MethodPost:
		s.rollbackCanary(w, r, serviceID)
	case action == "" && r.Method == http.MethodDelete:
		s.abortCanary(w, r, serviceID)
	case action == "" && r.Method == http.MethodGet:
		s.getCanary(w, r, serviceID)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// getCanary returns a single canary deployment.
func (s *Service) getCanary(w http.ResponseWriter, _ *http.Request, serviceID string) {
	s.mu.RLock()
	deployment, ok := s.deployments[serviceID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "canary deployment not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployment)
}

// getCanaryMetrics returns canary vs stable metrics comparison.
func (s *Service) getCanaryMetrics(w http.ResponseWriter, _ *http.Request, serviceID string) {
	s.mu.RLock()
	deployment, ok := s.deployments[serviceID]
	comparison := s.metrics[serviceID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "canary deployment not found", http.StatusNotFound)
		return
	}

	if comparison == nil {
		comparison = &MetricsComparison{
			ServiceID: serviceID,
			CanaryMetrics: &CanaryMetrics{
				ServiceID: serviceID,
				Version:   deployment.CanaryVersion,
			},
			StableMetrics: &CanaryMetrics{
				ServiceID: serviceID,
				Version:   deployment.StableVersion,
			},
			SLO:       deployment.SLO,
			Healthy:   true,
			Timestamp: time.Now(),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comparison)
}

// promoteCanary force-promotes a canary to stable (100% traffic).
func (s *Service) promoteCanary(w http.ResponseWriter, _ *http.Request, serviceID string) {
	s.mu.Lock()
	deployment, ok := s.deployments[serviceID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "canary deployment not found", http.StatusNotFound)
		return
	}

	now := time.Now()
	deployment.State = StateStable
	deployment.TrafficSplitPct = 100
	deployment.LastEvaluation = now
	deployment.CompletedAt = &now
	s.mu.Unlock()

	s.log.Info().Str("service_id", serviceID).Msg("canary force-promoted to stable")
	s.publishEvent(context.Background(), "monad.canary.promotion", map[string]interface{}{
		"service_id":     serviceID,
		"canary_version": deployment.CanaryVersion,
		"action":         "force_promote",
		"timestamp":      now.UnixMilli(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployment)
}

// rollbackCanary forces an immediate rollback to stable.
func (s *Service) rollbackCanary(w http.ResponseWriter, _ *http.Request, serviceID string) {
	s.mu.Lock()
	deployment, ok := s.deployments[serviceID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "canary deployment not found", http.StatusNotFound)
		return
	}

	now := time.Now()
	deployment.State = StateRollback
	deployment.TrafficSplitPct = 0
	deployment.LastEvaluation = now
	deployment.CompletedAt = &now
	deployment.RollbackReason = "manual_rollback"
	s.mu.Unlock()

	s.log.Warn().Str("service_id", serviceID).Msg("canary force-rolled back")
	s.publishEvent(context.Background(), "monad.canary.rollback", map[string]interface{}{
		"service_id":     serviceID,
		"canary_version": deployment.CanaryVersion,
		"reason":         "manual_rollback",
		"timestamp":      now.UnixMilli(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployment)
}

// abortCanary aborts a canary deployment and performs rollback.
func (s *Service) abortCanary(w http.ResponseWriter, _ *http.Request, serviceID string) {
	s.mu.Lock()
	deployment, ok := s.deployments[serviceID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "canary deployment not found", http.StatusNotFound)
		return
	}

	now := time.Now()
	deployment.State = StateRollback
	deployment.TrafficSplitPct = 0
	deployment.CompletedAt = &now
	deployment.RollbackReason = "abort"
	delete(s.deployments, serviceID)
	delete(s.metrics, serviceID)
	s.mu.Unlock()

	s.log.Warn().Str("service_id", serviceID).Msg("canary deployment aborted")
	s.publishEvent(context.Background(), "monad.canary.rollback", map[string]interface{}{
		"service_id":     serviceID,
		"canary_version": deployment.CanaryVersion,
		"reason":         "abort",
		"timestamp":      now.UnixMilli(),
	})

	w.WriteHeader(http.StatusNoContent)
}

// evaluationLoop runs the canary evaluation every EvalInterval.
func (s *Service) evaluationLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.EvalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evaluateAll(ctx)
		}
	}
}

// evaluateAll evaluates all active canary deployments.
func (s *Service) evaluateAll(ctx context.Context) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.deployments))
	for id := range s.deployments {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	for _, id := range ids {
		s.mu.RLock()
		deployment, ok := s.deployments[id]
		s.mu.RUnlock()
		if !ok {
			continue
		}
		// Skip completed deployments
		if deployment.State == StateStable || deployment.State == StateRollback {
			continue
		}
		s.evaluateCanary(ctx, deployment)
	}
}

// evaluateCanary checks SLO compliance and advances state or triggers rollback.
func (s *Service) evaluateCanary(ctx context.Context, deployment *CanaryDeployment) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	deployment.LastEvaluation = now
	deployment.EvalCount++

	// Generate simulated metrics for evaluation (in production, these come from Wotan)
	canaryMetrics := &CanaryMetrics{
		ServiceID:   deployment.ServiceID,
		Version:     deployment.CanaryVersion,
		ErrorRate:   100.0, // 100 ppm = 0.01% — healthy
		LatencyP99:  45.0,  // 45ms — healthy
		Throughput:  250.0, // 250 rps — healthy
		SampleCount: deployment.EvalCount * 100,
	}

	stableMetrics := &CanaryMetrics{
		ServiceID:   deployment.ServiceID,
		Version:     deployment.StableVersion,
		ErrorRate:   80.0,
		LatencyP99:  40.0,
		Throughput:  300.0,
		SampleCount: deployment.EvalCount * 500,
	}

	healthy := s.checkSLO(canaryMetrics, deployment.SLO)

	s.metrics[deployment.ServiceID] = &MetricsComparison{
		ServiceID:     deployment.ServiceID,
		CanaryMetrics: canaryMetrics,
		StableMetrics: stableMetrics,
		SLO:           deployment.SLO,
		Healthy:       healthy,
		Timestamp:     now,
	}

	if !healthy {
		// SLO breach: instant rollback
		deployment.State = StateRollback
		deployment.TrafficSplitPct = 0
		deployment.CompletedAt = &now
		deployment.RollbackReason = "slo_breach"

		s.log.Warn().
			Str("service_id", deployment.ServiceID).
			Str("state", string(deployment.State)).
			Msg("canary SLO breached — rolling back")

		go s.publishEvent(ctx, "monad.canary.rollback", map[string]interface{}{
			"service_id":     deployment.ServiceID,
			"canary_version": deployment.CanaryVersion,
			"reason":         "slo_breach",
			"timestamp":      now.UnixMilli(),
		})
		return
	}

	// State machine transitions
	switch deployment.State {
	case StateProbing:
		// PROBING: hold at initial split for ProbeDuration
		if now.Sub(deployment.ProbeStartTime) >= s.config.ProbeDuration {
			deployment.State = StateRamp
			deployment.LastRampTime = now
			s.log.Info().
				Str("service_id", deployment.ServiceID).
				Msg("canary probe complete — entering ramp phase")
		}

	case StateRamp:
		// RAMP: increase traffic by RampStepPct every RampInterval
		if now.Sub(deployment.LastRampTime) >= s.config.RampInterval {
			newSplit := deployment.TrafficSplitPct + s.config.RampStepPct
			if newSplit >= 100 {
				deployment.TrafficSplitPct = 100
				deployment.State = StateStable
				deployment.CompletedAt = &now

				s.log.Info().
					Str("service_id", deployment.ServiceID).
					Msg("canary promoted to stable — 100% traffic")

				go s.publishEvent(ctx, "monad.canary.promotion", map[string]interface{}{
					"service_id":     deployment.ServiceID,
					"canary_version": deployment.CanaryVersion,
					"action":         "auto_promote",
					"timestamp":      now.UnixMilli(),
				})
			} else {
				deployment.TrafficSplitPct = newSplit
				deployment.LastRampTime = now
				s.log.Info().
					Str("service_id", deployment.ServiceID).
					Int("traffic_split_pct", newSplit).
					Msg("canary traffic ramped")
			}
		}
	}
}

// checkSLO evaluates whether canary metrics are within SLO thresholds.
func (s *Service) checkSLO(metrics *CanaryMetrics, slo *SLOThreshold) bool {
	if slo == nil {
		return true
	}

	if metrics.ErrorRate > float64(slo.MaxErrorRatePPM) {
		s.log.Warn().
			Str("service_id", metrics.ServiceID).
			Float64("error_rate", metrics.ErrorRate).
			Int64("max_allowed", slo.MaxErrorRatePPM).
			Msg("SLO breach: error rate exceeded")
		return false
	}

	if metrics.LatencyP99 > slo.MaxLatencyP99MS {
		s.log.Warn().
			Str("service_id", metrics.ServiceID).
			Float64("latency_p99", metrics.LatencyP99).
			Float64("max_allowed", slo.MaxLatencyP99MS).
			Msg("SLO breach: latency exceeded")
		return false
	}

	if slo.MinThroughputPPS > 0 && metrics.Throughput < slo.MinThroughputPPS {
		s.log.Warn().
			Str("service_id", metrics.ServiceID).
			Float64("throughput", metrics.Throughput).
			Float64("min_required", slo.MinThroughputPPS).
			Msg("SLO breach: throughput below minimum")
		return false
	}

	return true
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
		Msg("received critical alert — evaluating canary impact")
}

// publishEvent publishes an event to Wotan.
func (s *Service) publishEvent(ctx context.Context, topic string, event map[string]interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		s.log.Warn().Str("topic", topic).Msg("wotan unavailable — event dropped (degraded mode)")
		return
	}

	event["service"] = "canary"
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

// GetDeployment returns a canary deployment by service ID.
func (s *Service) GetDeployment(serviceID string) (*CanaryDeployment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.deployments[serviceID]
	return d, ok
}

// ListDeployments returns all canary deployments.
func (s *Service) ListDeployments() []*CanaryDeployment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*CanaryDeployment, 0, len(s.deployments))
	for _, d := range s.deployments {
		result = append(result, d)
	}
	return result
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var probing, ramp, stable, rollback int
	for _, d := range s.deployments {
		switch d.State {
		case StateProbing:
			probing++
		case StateRamp:
			ramp++
		case StateStable:
			stable++
		case StateRollback:
			rollback++
		}
	}

	return map[string]interface{}{
		"total_deployments":    len(s.deployments),
		"probing_deployments":  probing,
		"ramp_deployments":     ramp,
		"stable_deployments":   stable,
		"rollback_deployments": rollback,
		"wotan_drops":          atomic.LoadInt64(&s.wotanDrops),
	}
}
