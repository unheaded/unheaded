// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package anomaly provides the Anomaly Detection Service for the Unheaded Kingdom.
// It operates on flow-level data from the Sophia BPF programs, computing anomaly scores
// using decision-tree models. The service maintains per-service baselines, adaptive
// thresholds (mean + 2*stddev), and publishes high-confidence findings as alerts to Wotan.
// Background goroutines handle baseline learning (every 5 minutes) and threshold updates
// (pushed to Sophia BPF maps).
package anomaly

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
)

// FlowKey uniquely identifies a network flow.
type FlowKey struct {
	SrcIP   string `json:"src_ip"`
	DstIP   string `json:"dst_ip"`
	SrcPort uint16 `json:"src_port"`
	DstPort uint16 `json:"dst_port"`
	Proto   uint8  `json:"proto"`
}

// String returns a human-readable representation of the FlowKey.
func (fk FlowKey) String() string {
	return fmt.Sprintf("%s:%d->%s:%d/%d", fk.SrcIP, fk.SrcPort, fk.DstIP, fk.DstPort, fk.Proto)
}

// FlowScore holds the anomaly score for a single flow.
type FlowScore struct {
	FlowKey      FlowKey   `json:"flow_key"`
	AnomalyScore int       `json:"anomaly_score"` // 0-100
	PacketCount  uint64    `json:"packet_count"`
	LastSeen     time.Time `json:"last_seen"`
	ServiceID    string    `json:"service_id"`
}

// AnomalyAlert represents a detected anomaly event.
type AnomalyAlert struct {
	FlowKey   FlowKey   `json:"flow_key"`
	Score     int       `json:"score"`
	Reason    string    `json:"reason"`
	Severity  string    `json:"severity"` // low, medium, high, critical
	Timestamp time.Time `json:"timestamp"`
	TraceID   string    `json:"trace_id"`
}

// ServiceBaseline holds the learned baseline statistics for a service.
type ServiceBaseline struct {
	ServiceID          string    `json:"service_id"`
	PacketSizeMean     float64   `json:"packet_size_mean"`
	PacketSizeStdDev   float64   `json:"packet_size_std_dev"`
	InterArrivalMeanMS float64   `json:"inter_arrival_mean_ms"`
	EntropyMean        float64   `json:"entropy_mean"`
	EntropyStdDev      float64   `json:"entropy_std_dev"`
	ConnRatePerSec     float64   `json:"conn_rate_per_sec"`
	SampleCount        uint64    `json:"sample_count"`
	LastUpdated        time.Time `json:"last_updated"`
}

// AdaptiveThreshold holds the computed threshold for a service.
type AdaptiveThreshold struct {
	ServiceID   string    `json:"service_id"`
	Mean        float64   `json:"mean"`
	StdDev      float64   `json:"std_dev"`
	Threshold   float64   `json:"threshold"` // mean + 2*stddev
	LastUpdated time.Time `json:"last_updated"`
}

// ModelConfig holds metadata about the active anomaly detection model.
type ModelConfig struct {
	Type             string     `json:"type"` // decision_tree
	Depth            int        `json:"depth"`
	NodeCount        int        `json:"node_count"`
	TrainingAccuracy float64    `json:"training_accuracy"`
	Version          string     `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	Nodes            []TreeNode `json:"nodes,omitempty"`
}

// TreeNode represents a node in the decision tree model.
type TreeNode struct {
	NodeID     int   `json:"node_id"`
	Threshold  int16 `json:"threshold"`
	FeatureIdx int   `json:"feature_idx"`
	LeftChild  int   `json:"left_child"`
	RightChild int   `json:"right_child"`
	IsLeaf     bool  `json:"is_leaf"`
	LeafScore  int16 `json:"leaf_score"`
}

// HeatmapCell represents a single cell in the flow anomaly heatmap.
type HeatmapCell struct {
	SrcService string  `json:"src_service"`
	DstService string  `json:"dst_service"`
	Score      float64 `json:"score"`
	FlowCount  int     `json:"flow_count"`
}

// Service is the main Anomaly Detection service.
type Service struct {
	log   *logger.Logger
	wotan *wotanClient.Client

	mu          sync.RWMutex
	flowScores  map[string]*FlowScore         // key: FlowKey.String()
	baselines   map[string]*ServiceBaseline   // key: service_id
	thresholds  map[string]*AdaptiveThreshold // key: service_id
	modelConfig *ModelConfig
	alerts      []AnomalyAlert

	// Alert handling
	alertsCh chan *wotanClient.Message

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64

	// HTTP server reference for graceful shutdown
	httpServer *http.Server
}

// NewService creates a new Anomaly Detection service.
func NewService(log *logger.Logger, wotan *wotanClient.Client) *Service {
	return &Service{
		log:        log,
		wotan:      wotan,
		flowScores: make(map[string]*FlowScore),
		baselines:  make(map[string]*ServiceBaseline),
		thresholds: make(map[string]*AdaptiveThreshold),
		modelConfig: &ModelConfig{
			Type:             "decision_tree",
			Depth:            5,
			NodeCount:        0,
			TrainingAccuracy: 0.0,
			Version:          "0.0.0",
			CreatedAt:        time.Now(),
		},
		alerts:   make([]AnomalyAlert, 0),
		alertsCh: make(chan *wotanClient.Message, 100),
	}
}

// Start initializes Wotan subscriptions, the HTTP server, and background goroutines.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Anomaly Detection Service starting")

	// Subscribe to Wotan topics
	if s.wotan != nil {
		topics := []string{
			"anomaly.scores",
			"anomaly.alerts",
			"anomaly.baseline",
			"alerts.critical",
		}
		for _, topic := range topics {
			if _, err := s.wotan.Subscribe(ctx, topic, "anomaly-service"); err != nil {
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

	// Start baseline learning goroutine (every 5 minutes)
	go s.baselineLearningLoop(ctx)

	// Start threshold update goroutine (push to Sophia BPF maps)
	go s.thresholdUpdateLoop(ctx)

	// Start HTTP server
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	s.httpServer = &http.Server{
		Addr:           ports.DefaultAddr(ports.Anomaly),
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		s.log.Info().Int("port", ports.Anomaly).Msg("Anomaly HTTP server listening")
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Anomaly Detection Service stopping")

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.log.Error().Err(err).Msg("HTTP server shutdown error")
		}
	}

	close(s.alertsCh)
	return nil
}

// WotanDrops returns the total number of messages dropped because Wotan was nil.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// RegisterRoutes registers all HTTP routes on the provided mux.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	// Anomaly endpoints
	mux.HandleFunc("/api/v1/anomalies/top", s.handleTopAnomalies)
	mux.HandleFunc("/api/v1/anomalies/heatmap", s.handleHeatmap)
	mux.HandleFunc("/api/v1/anomalies/baseline/", s.handleBaseline)
	mux.HandleFunc("/api/v1/anomalies/model-config", s.handleModelConfig)
	mux.HandleFunc("/api/v1/anomalies/model", s.handleModelUpdate)
	mux.HandleFunc("/api/v1/anomalies/threshold/", s.handleThresholdOverride)
	mux.HandleFunc("/api/v1/anomalies/alerts", s.handleAlerts)

	// Health endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)
}

// handleTopAnomalies returns the top suspicious flows, optionally filtered by service_id.
func (s *Service) handleTopAnomalies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	serviceIDFilter := r.URL.Query().Get("service_id")

	s.mu.RLock()
	scores := make([]*FlowScore, 0, len(s.flowScores))
	for _, score := range s.flowScores {
		if serviceIDFilter != "" && score.ServiceID != serviceIDFilter {
			continue
		}
		scores = append(scores, score)
	}
	s.mu.RUnlock()

	// Sort by anomaly score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].AnomalyScore > scores[j].AnomalyScore
	})

	if len(scores) > limit {
		scores = scores[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"flows": scores,
		"count": len(scores),
	})
}

// handleHeatmap returns the flow grid [src_svc x dst_svc x score] for the dashboard.
func (s *Service) handleHeatmap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	// Aggregate flow scores into a service-to-service heatmap
	cellMap := make(map[string]*HeatmapCell)
	for _, score := range s.flowScores {
		// Use service_id as src, derive dst from flow key IP
		src := score.ServiceID
		if src == "" {
			src = "unknown"
		}
		dst := score.FlowKey.DstIP
		key := src + "|" + dst
		cell, ok := cellMap[key]
		if !ok {
			cell = &HeatmapCell{SrcService: src, DstService: dst}
			cellMap[key] = cell
		}
		cell.Score += float64(score.AnomalyScore)
		cell.FlowCount++
	}
	s.mu.RUnlock()

	// Normalize scores
	cells := make([]HeatmapCell, 0, len(cellMap))
	for _, cell := range cellMap {
		if cell.FlowCount > 0 {
			cell.Score /= float64(cell.FlowCount)
		}
		cells = append(cells, *cell)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cells": cells,
		"count": len(cells),
	})
}

// handleBaseline returns the current baseline and threshold stats for a service.
func (s *Service) handleBaseline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serviceID := strings.TrimPrefix(r.URL.Path, "/api/v1/anomalies/baseline/")
	if serviceID == "" {
		http.Error(w, "service_id required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	baseline, baselineOk := s.baselines[serviceID]
	threshold, thresholdOk := s.thresholds[serviceID]
	s.mu.RUnlock()

	if !baselineOk {
		http.Error(w, "baseline not found for service", http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"baseline": baseline,
	}
	if thresholdOk {
		response["threshold"] = threshold
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleModelConfig returns the active model metadata.
func (s *Service) handleModelConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	config := *s.modelConfig
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// handleModelUpdate updates the model weights (push to BPF map).
func (s *Service) handleModelUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var config ModelConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if config.Type == "" {
		config.Type = "decision_tree"
	}
	if config.Version == "" {
		http.Error(w, "model version required", http.StatusBadRequest)
		return
	}

	config.CreatedAt = time.Now()

	s.mu.Lock()
	s.modelConfig = &config
	s.mu.Unlock()

	s.log.Info().
		Str("type", config.Type).
		Str("version", config.Version).
		Int("depth", config.Depth).
		Int("nodes", config.NodeCount).
		Msg("model updated")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// handleThresholdOverride allows manual threshold override for a service.
func (s *Service) handleThresholdOverride(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serviceID := strings.TrimPrefix(r.URL.Path, "/api/v1/anomalies/threshold/")
	if serviceID == "" {
		http.Error(w, "service_id required", http.StatusBadRequest)
		return
	}

	var req struct {
		Threshold float64 `json:"threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.thresholds[serviceID] = &AdaptiveThreshold{
		ServiceID:   serviceID,
		Threshold:   req.Threshold,
		LastUpdated: time.Now(),
	}
	s.mu.Unlock()

	s.log.Info().
		Str("service_id", serviceID).
		Float64("threshold", req.Threshold).
		Msg("threshold manually overridden")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.thresholds[serviceID])
}

// handleAlerts returns recent alerts.
func (s *Service) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	s.mu.RLock()
	alerts := make([]AnomalyAlert, len(s.alerts))
	copy(alerts, s.alerts)
	s.mu.RUnlock()

	// Return most recent first
	if len(alerts) > limit {
		alerts = alerts[len(alerts)-limit:]
	}
	// Reverse for most recent first
	for i, j := 0, len(alerts)-1; i < j; i, j = i+1, j-1 {
		alerts[i], alerts[j] = alerts[j], alerts[i]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alerts": alerts,
		"count":  len(alerts),
	})
}

// handleHealth returns service health status.
func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "anomaly",
	})
}

// handleReady returns service readiness status.
func (s *Service) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":   true,
		"service": "anomaly",
	})
}

// handleMetrics returns Prometheus-format metrics.
func (s *Service) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	flowCount := len(s.flowScores)
	baselineCount := len(s.baselines)
	thresholdCount := len(s.thresholds)
	alertCount := len(s.alerts)
	modelVersion := s.modelConfig.Version
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP anomaly_tracked_flows Total tracked flows\n")
	fmt.Fprintf(w, "# TYPE anomaly_tracked_flows gauge\n")
	fmt.Fprintf(w, "anomaly_tracked_flows %d\n", flowCount)
	fmt.Fprintf(w, "# HELP anomaly_baselines_total Total service baselines\n")
	fmt.Fprintf(w, "# TYPE anomaly_baselines_total gauge\n")
	fmt.Fprintf(w, "anomaly_baselines_total %d\n", baselineCount)
	fmt.Fprintf(w, "# HELP anomaly_thresholds_total Total adaptive thresholds\n")
	fmt.Fprintf(w, "# TYPE anomaly_thresholds_total gauge\n")
	fmt.Fprintf(w, "anomaly_thresholds_total %d\n", thresholdCount)
	fmt.Fprintf(w, "# HELP anomaly_alerts_total Total alerts generated\n")
	fmt.Fprintf(w, "# TYPE anomaly_alerts_total counter\n")
	fmt.Fprintf(w, "anomaly_alerts_total %d\n", alertCount)
	fmt.Fprintf(w, "# HELP anomaly_model_info Model version metadata\n")
	fmt.Fprintf(w, "# TYPE anomaly_model_info gauge\n")
	fmt.Fprintf(w, "anomaly_model_info{version=%q} 1\n", modelVersion)
	fmt.Fprintf(w, "# HELP anomaly_wotan_drops_total Messages dropped due to Wotan unavailability\n")
	fmt.Fprintf(w, "# TYPE anomaly_wotan_drops_total counter\n")
	fmt.Fprintf(w, "anomaly_wotan_drops_total %d\n", s.WotanDrops())
}

// RecordFlowScore records or updates a flow score.
func (s *Service) RecordFlowScore(score *FlowScore) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := score.FlowKey.String()
	score.LastSeen = time.Now()
	s.flowScores[key] = score

	// Check if this score exceeds the threshold for the service
	if score.AnomalyScore > 80 {
		alert := AnomalyAlert{
			FlowKey:   score.FlowKey,
			Score:     score.AnomalyScore,
			Reason:    "anomaly_score_exceeded_threshold",
			Severity:  classifySeverity(score.AnomalyScore),
			Timestamp: time.Now(),
			TraceID:   fmt.Sprintf("anomaly-%d", time.Now().UnixNano()),
		}
		s.alerts = append(s.alerts, alert)

		// Keep alerts bounded (max 10000)
		if len(s.alerts) > 10000 {
			s.alerts = s.alerts[len(s.alerts)-10000:]
		}
	}
}

// classifySeverity maps an anomaly score to a severity level.
func classifySeverity(score int) string {
	switch {
	case score >= 95:
		return "critical"
	case score >= 85:
		return "high"
	case score >= 70:
		return "medium"
	default:
		return "low"
	}
}

// GetFlowScores returns all flow scores, optionally filtered by service.
func (s *Service) GetFlowScores(serviceID string) []*FlowScore {
	s.mu.RLock()
	defer s.mu.RUnlock()

	scores := make([]*FlowScore, 0, len(s.flowScores))
	for _, score := range s.flowScores {
		if serviceID != "" && score.ServiceID != serviceID {
			continue
		}
		scores = append(scores, score)
	}
	return scores
}

// GetAlerts returns recent alerts.
func (s *Service) GetAlerts() []AnomalyAlert {
	s.mu.RLock()
	defer s.mu.RUnlock()
	alerts := make([]AnomalyAlert, len(s.alerts))
	copy(alerts, s.alerts)
	return alerts
}

// SetBaseline sets the baseline for a service.
func (s *Service) SetBaseline(baseline *ServiceBaseline) {
	s.mu.Lock()
	defer s.mu.Unlock()
	baseline.LastUpdated = time.Now()
	s.baselines[baseline.ServiceID] = baseline
}

// GetBaseline retrieves the baseline for a service.
func (s *Service) GetBaseline(serviceID string) (*ServiceBaseline, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	baseline, ok := s.baselines[serviceID]
	return baseline, ok
}

// SetThreshold sets the threshold for a service.
func (s *Service) SetThreshold(threshold *AdaptiveThreshold) {
	s.mu.Lock()
	defer s.mu.Unlock()
	threshold.LastUpdated = time.Now()
	s.thresholds[threshold.ServiceID] = threshold
}

// GetThreshold retrieves the threshold for a service.
func (s *Service) GetThreshold(serviceID string) (*AdaptiveThreshold, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	threshold, ok := s.thresholds[serviceID]
	return threshold, ok
}

// GetModelConfig retrieves the active model configuration.
func (s *Service) GetModelConfig() ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.modelConfig
}

// UpdateModel updates the model configuration.
func (s *Service) UpdateModel(config *ModelConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config.CreatedAt = time.Now()
	s.modelConfig = config
}

// learnBaseline computes running statistics from a set of flow score samples.
func learnBaseline(serviceID string, samples []FlowScore) ServiceBaseline {
	baseline := ServiceBaseline{
		ServiceID:   serviceID,
		SampleCount: uint64(len(samples)),
		LastUpdated: time.Now(),
	}

	if len(samples) == 0 {
		return baseline
	}

	// Compute means
	var scoreSum, packetSum float64
	for _, s := range samples {
		scoreSum += float64(s.AnomalyScore)
		packetSum += float64(s.PacketCount)
	}
	n := float64(len(samples))
	scoreMean := scoreSum / n
	packetMean := packetSum / n

	// Compute standard deviations
	var scoreVarSum, packetVarSum float64
	for _, s := range samples {
		scoreDiff := float64(s.AnomalyScore) - scoreMean
		scoreVarSum += scoreDiff * scoreDiff
		packetDiff := float64(s.PacketCount) - packetMean
		packetVarSum += packetDiff * packetDiff
	}

	baseline.EntropyMean = scoreMean
	baseline.EntropyStdDev = math.Sqrt(scoreVarSum / n)
	baseline.PacketSizeMean = packetMean
	baseline.PacketSizeStdDev = math.Sqrt(packetVarSum / n)

	// Inter-arrival and connection rate are estimated from flow timing
	if len(samples) > 1 {
		first := samples[0].LastSeen
		last := samples[len(samples)-1].LastSeen
		duration := last.Sub(first)
		if duration > 0 {
			baseline.InterArrivalMeanMS = float64(duration.Milliseconds()) / n
			baseline.ConnRatePerSec = n / duration.Seconds()
		}
	}

	return baseline
}

// updateThresholds recalculates adaptive thresholds from baselines.
func (s *Service) updateThresholds() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for serviceID, baseline := range s.baselines {
		mean := baseline.EntropyMean
		stddev := baseline.EntropyStdDev
		threshold := mean + 2*stddev

		s.thresholds[serviceID] = &AdaptiveThreshold{
			ServiceID:   serviceID,
			Mean:        mean,
			StdDev:      stddev,
			Threshold:   threshold,
			LastUpdated: time.Now(),
		}
	}
}

// Stats returns service-level statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"tracked_flows": len(s.flowScores),
		"baselines":     len(s.baselines),
		"thresholds":    len(s.thresholds),
		"alerts":        len(s.alerts),
		"model_version": s.modelConfig.Version,
		"model_type":    s.modelConfig.Type,
		"wotan_drops":   s.WotanDrops(),
	}
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
			s.handleCriticalAlert(msg)
		}
	}
}

// handleCriticalAlert processes critical alerts.
func (s *Service) handleCriticalAlert(msg *wotanClient.Message) {
	if msg == nil {
		return
	}
	s.log.Warn().
		Str("message_id", msg.MessageID).
		Str("topic", msg.Topic).
		Msg("received critical alert")
}

// baselineLearningLoop runs baseline learning every 5 minutes.
func (s *Service) baselineLearningLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.learnBaselinesFromFlows()
		}
	}
}

// learnBaselinesFromFlows computes baselines from current flow scores.
func (s *Service) learnBaselinesFromFlows() {
	s.mu.RLock()
	// Group flows by service
	serviceFlows := make(map[string][]FlowScore)
	for _, score := range s.flowScores {
		sid := score.ServiceID
		if sid == "" {
			sid = "unknown"
		}
		serviceFlows[sid] = append(serviceFlows[sid], *score)
	}
	s.mu.RUnlock()

	// Compute baselines per service
	for serviceID, flows := range serviceFlows {
		baseline := learnBaseline(serviceID, flows)
		s.SetBaseline(&baseline)
	}

	s.log.Debug().
		Int("services", len(serviceFlows)).
		Msg("baselines learned from flows")
}

// thresholdUpdateLoop updates thresholds and pushes to Sophia BPF maps.
func (s *Service) thresholdUpdateLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateThresholds()
			s.pushThresholdsToBPF(ctx)
		}
	}
}

// pushThresholdsToBPF publishes updated thresholds to Wotan for Sophia to consume.
func (s *Service) pushThresholdsToBPF(ctx context.Context) {
	s.mu.RLock()
	thresholds := make(map[string]*AdaptiveThreshold, len(s.thresholds))
	for k, v := range s.thresholds {
		thresholds[k] = v
	}
	s.mu.RUnlock()

	if len(thresholds) == 0 {
		return
	}

	s.publishEvent(ctx, "anomaly.alerts", map[string]interface{}{
		"event_type": "threshold_update",
		"thresholds": thresholds,
		"timestamp":  time.Now().UnixMilli(),
	})
}

// publishEvent publishes an event to Wotan with trace_id.
func (s *Service) publishEvent(ctx context.Context, topic string, event map[string]interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		s.log.Warn().Str("topic", topic).Msg("wotan unavailable — event dropped (degraded mode)")
		return
	}

	// Extract trace_id from context or generate one
	traceID := ""
	if tid := ctx.Value("trace_id"); tid != nil {
		traceID, _ = tid.(string)
	}
	if traceID == "" {
		traceID = fmt.Sprintf("anomaly-%d", time.Now().UnixNano())
	}

	event["trace_id"] = traceID
	event["service"] = "anomaly"

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
