// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package version provides the Version Translation Service for the Unheaded Kingdom.
// It manages translation rules between service protocol versions, enabling seamless
// rolling upgrades across the mesh. Translation rules include field masks, offset maps,
// width deltas, and CRC recalculation — all designed to match the Monad wire format.
// The service tracks per-version statistics and publishes rollout events to Wotan.
package version

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

// TranslationRule defines how to translate packets between protocol versions.
type TranslationRule struct {
	SrcVersion    string    `json:"src_version"`
	DstVersion    string    `json:"dst_version"`
	FieldMask     uint16    `json:"field_mask"`
	OffsetMap     [8]byte   `json:"offset_map"`
	WidthDeltas   [4]int8   `json:"width_deltas"`
	DefaultValues [4]byte   `json:"default_values"`
	CRCRecalc     bool      `json:"crc_recalc"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ruleKey produces a canonical key from src and dst versions.
func ruleKey(src, dst string) string {
	return src + "->" + dst
}

// ServiceVersionInfo tracks the current version and capabilities of a service.
type ServiceVersionInfo struct {
	ServiceID      string    `json:"service_id"`
	CurrentVersion string    `json:"current_version"`
	Capabilities   uint8     `json:"capabilities"`
	LastUpdated    time.Time `json:"last_updated"`
}

// TranslationStats holds aggregate translation statistics.
type TranslationStats struct {
	TotalTranslations     uint64            `json:"total_translations"`
	TranslationsByVersion map[string]uint64 `json:"translations_by_version"`
	ErrorCount            uint64            `json:"error_count"`
	AvgLatencyUS          float64           `json:"avg_latency_us"`
	LastUpdated           time.Time         `json:"last_updated"`
}

// VersionHeatmapEntry represents a single entry in the version distribution heatmap.
type VersionHeatmapEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Version     string    `json:"version"`
	PacketCount uint64    `json:"packet_count"`
}

// Service is the main Version Translation service.
type Service struct {
	log   *logger.Logger
	wotan *wotanClient.Client

	mu               sync.RWMutex
	translationRules map[string]*TranslationRule    // key: "src->dst"
	serviceVersions  map[string]*ServiceVersionInfo // key: service_id
	translationStats *TranslationStats
	heatmap          []VersionHeatmapEntry

	// Alert handling
	alertsCh chan *wotanClient.Message

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64

	// HTTP server reference for graceful shutdown
	httpServer *http.Server
}

// NewService creates a new Version Translation service.
func NewService(log *logger.Logger, wotan *wotanClient.Client) *Service {
	return &Service{
		log:              log,
		wotan:            wotan,
		translationRules: make(map[string]*TranslationRule),
		serviceVersions:  make(map[string]*ServiceVersionInfo),
		translationStats: &TranslationStats{
			TranslationsByVersion: make(map[string]uint64),
		},
		heatmap:  make([]VersionHeatmapEntry, 0),
		alertsCh: make(chan *wotanClient.Message, 100),
	}
}

// Start initializes Wotan subscriptions, the HTTP server, and background goroutines.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Version Translation Service starting")

	// Subscribe to Wotan topics
	if s.wotan != nil {
		topics := []string{
			"monad.translation.events",
			"monad.translation.errors",
			"monad.version.rollout",
			"alerts.critical",
		}
		for _, topic := range topics {
			if _, err := s.wotan.Subscribe(ctx, topic, "version-service"); err != nil {
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

	// Start stats aggregation goroutine
	go s.statsAggregationLoop(ctx)

	// Start HTTP server
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	s.httpServer = &http.Server{
		Addr:           ports.DefaultAddr(ports.VersionSvc),
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		s.log.Info().Int("port", ports.VersionSvc).Msg("Version HTTP server listening")
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Version Translation Service stopping")

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
	// Translation rules management
	mux.HandleFunc("/api/v1/translations/rules", s.handleTranslationRules)
	mux.HandleFunc("/api/v1/translations/rules/", s.handleTranslationRuleByVersions)

	// Service version registry
	mux.HandleFunc("/api/v1/services/versions", s.handleServiceVersions)
	mux.HandleFunc("/api/v1/services/", s.handleServiceByID)

	// Statistics
	mux.HandleFunc("/api/v1/translations/stats", s.handleTranslationStats)
	mux.HandleFunc("/api/v1/translations/heatmap", s.handleHeatmap)

	// Health endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)
}

// handleTranslationRules handles GET (list) and POST (create/update) on /api/v1/translations/rules.
func (s *Service) handleTranslationRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTranslationRules(w, r)
	case http.MethodPost:
		s.createOrUpdateTranslationRule(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTranslationRuleByVersions handles GET and DELETE on /api/v1/translations/rules/{src}/{dst}.
func (s *Service) handleTranslationRuleByVersions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/translations/rules/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "src and dst version required", http.StatusBadRequest)
		return
	}

	src := parts[0]
	dst := parts[1]

	switch r.Method {
	case http.MethodGet:
		s.getTranslationRule(w, r, src, dst)
	case http.MethodDelete:
		s.deleteTranslationRule(w, r, src, dst)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listTranslationRules returns all translation rules.
func (s *Service) listTranslationRules(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	rules := make([]*TranslationRule, 0, len(s.translationRules))
	for _, rule := range s.translationRules {
		rules = append(rules, rule)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rules": rules,
		"count": len(rules),
	})
}

// getTranslationRule returns a specific translation rule.
func (s *Service) getTranslationRule(w http.ResponseWriter, _ *http.Request, src, dst string) {
	s.mu.RLock()
	rule, ok := s.translationRules[ruleKey(src, dst)]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "translation rule not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

// createOrUpdateTranslationRule creates or updates a translation rule.
func (s *Service) createOrUpdateTranslationRule(w http.ResponseWriter, r *http.Request) {
	var rule TranslationRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if rule.SrcVersion == "" || rule.DstVersion == "" {
		http.Error(w, "src_version and dst_version required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	key := ruleKey(rule.SrcVersion, rule.DstVersion)
	now := time.Now()
	_, exists := s.translationRules[key]
	if !exists {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	s.translationRules[key] = &rule
	s.mu.Unlock()

	s.log.Info().
		Str("src", rule.SrcVersion).
		Str("dst", rule.DstVersion).
		Bool("crc_recalc", rule.CRCRecalc).
		Msg("translation rule created/updated")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rule)
}

// deleteTranslationRule removes a translation rule.
func (s *Service) deleteTranslationRule(w http.ResponseWriter, _ *http.Request, src, dst string) {
	s.mu.Lock()
	key := ruleKey(src, dst)
	if _, ok := s.translationRules[key]; !ok {
		s.mu.Unlock()
		http.Error(w, "translation rule not found", http.StatusNotFound)
		return
	}
	delete(s.translationRules, key)
	s.mu.Unlock()

	s.log.Info().
		Str("src", src).
		Str("dst", dst).
		Msg("translation rule deleted")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
	})
}

// handleServiceVersions handles GET on /api/v1/services/versions.
func (s *Service) handleServiceVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	versions := make([]*ServiceVersionInfo, 0, len(s.serviceVersions))
	for _, v := range s.serviceVersions {
		versions = append(versions, v)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"services": versions,
		"count":    len(versions),
	})
}

// handleServiceByID handles POST on /api/v1/services/{id}/version.
func (s *Service) handleServiceByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] != "version" {
		http.Error(w, "invalid path, expected /api/v1/services/{id}/version", http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serviceID := parts[0]

	var req struct {
		Version      string `json:"version"`
		Capabilities uint8  `json:"capabilities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Version == "" {
		http.Error(w, "version required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.serviceVersions[serviceID] = &ServiceVersionInfo{
		ServiceID:      serviceID,
		CurrentVersion: req.Version,
		Capabilities:   req.Capabilities,
		LastUpdated:    time.Now(),
	}
	s.mu.Unlock()

	s.log.Info().
		Str("service_id", serviceID).
		Str("version", req.Version).
		Msg("service version updated")

	// Publish rollout event
	s.publishEvent(r.Context(), "monad.version.rollout", map[string]interface{}{
		"service_id": serviceID,
		"version":    req.Version,
		"timestamp":  time.Now().UnixMilli(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.serviceVersions[serviceID])
}

// handleTranslationStats returns translation statistics.
func (s *Service) handleTranslationStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	stats := *s.translationStats
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleHeatmap returns version distribution data.
func (s *Service) handleHeatmap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	heatmap := make([]VersionHeatmapEntry, len(s.heatmap))
	copy(heatmap, s.heatmap)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": heatmap,
		"count":   len(heatmap),
	})
}

// handleHealth returns service health status.
func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "version",
	})
}

// handleReady returns service readiness status.
func (s *Service) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":   true,
		"service": "version",
	})
}

// handleMetrics returns Prometheus-format metrics.
func (s *Service) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	ruleCount := len(s.translationRules)
	serviceCount := len(s.serviceVersions)
	totalTranslations := s.translationStats.TotalTranslations
	errorCount := s.translationStats.ErrorCount
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP version_translation_rules_total Total translation rules\n")
	fmt.Fprintf(w, "# TYPE version_translation_rules_total gauge\n")
	fmt.Fprintf(w, "version_translation_rules_total %d\n", ruleCount)
	fmt.Fprintf(w, "# HELP version_services_total Total tracked services\n")
	fmt.Fprintf(w, "# TYPE version_services_total gauge\n")
	fmt.Fprintf(w, "version_services_total %d\n", serviceCount)
	fmt.Fprintf(w, "# HELP version_translations_total Total translations performed\n")
	fmt.Fprintf(w, "# TYPE version_translations_total counter\n")
	fmt.Fprintf(w, "version_translations_total %d\n", totalTranslations)
	fmt.Fprintf(w, "# HELP version_translation_errors_total Total translation errors\n")
	fmt.Fprintf(w, "# TYPE version_translation_errors_total counter\n")
	fmt.Fprintf(w, "version_translation_errors_total %d\n", errorCount)
	fmt.Fprintf(w, "# HELP version_wotan_drops_total Messages dropped due to Wotan unavailability\n")
	fmt.Fprintf(w, "# TYPE version_wotan_drops_total counter\n")
	fmt.Fprintf(w, "version_wotan_drops_total %d\n", s.WotanDrops())
}

// AddTranslationRule adds a translation rule programmatically.
func (s *Service) AddTranslationRule(rule *TranslationRule) error {
	if rule.SrcVersion == "" || rule.DstVersion == "" {
		return fmt.Errorf("src_version and dst_version required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := ruleKey(rule.SrcVersion, rule.DstVersion)
	now := time.Now()
	if _, exists := s.translationRules[key]; !exists {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	s.translationRules[key] = rule

	return nil
}

// GetTranslationRule retrieves a translation rule by src and dst version.
func (s *Service) GetTranslationRule(src, dst string) (*TranslationRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rule, ok := s.translationRules[ruleKey(src, dst)]
	return rule, ok
}

// DeleteTranslationRule deletes a translation rule.
func (s *Service) DeleteTranslationRule(src, dst string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := ruleKey(src, dst)
	if _, ok := s.translationRules[key]; !ok {
		return fmt.Errorf("translation rule not found: %s -> %s", src, dst)
	}
	delete(s.translationRules, key)
	return nil
}

// ListTranslationRules returns all translation rules.
func (s *Service) ListTranslationRules() []*TranslationRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := make([]*TranslationRule, 0, len(s.translationRules))
	for _, rule := range s.translationRules {
		rules = append(rules, rule)
	}
	return rules
}

// UpdateServiceVersion updates or registers a service version.
func (s *Service) UpdateServiceVersion(ctx context.Context, serviceID, version string, capabilities uint8) {
	s.mu.Lock()
	s.serviceVersions[serviceID] = &ServiceVersionInfo{
		ServiceID:      serviceID,
		CurrentVersion: version,
		Capabilities:   capabilities,
		LastUpdated:    time.Now(),
	}
	s.mu.Unlock()

	s.publishEvent(ctx, "monad.version.rollout", map[string]interface{}{
		"service_id": serviceID,
		"version":    version,
		"timestamp":  time.Now().UnixMilli(),
	})
}

// GetServiceVersion retrieves version info for a service.
func (s *Service) GetServiceVersion(serviceID string) (*ServiceVersionInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, ok := s.serviceVersions[serviceID]
	return info, ok
}

// ListServiceVersions returns all tracked service versions.
func (s *Service) ListServiceVersions() []*ServiceVersionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions := make([]*ServiceVersionInfo, 0, len(s.serviceVersions))
	for _, v := range s.serviceVersions {
		versions = append(versions, v)
	}
	return versions
}

// RecordTranslation records a successful translation for stats tracking.
func (s *Service) RecordTranslation(version string, latencyUS float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.translationStats.TotalTranslations++
	s.translationStats.TranslationsByVersion[version]++

	// Running average
	total := float64(s.translationStats.TotalTranslations)
	s.translationStats.AvgLatencyUS = s.translationStats.AvgLatencyUS*(total-1)/total + latencyUS/total
	s.translationStats.LastUpdated = time.Now()
}

// RecordTranslationError records a translation error.
func (s *Service) RecordTranslationError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.translationStats.ErrorCount++
}

// GetTranslationStats returns translation statistics.
func (s *Service) GetTranslationStats() TranslationStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.translationStats
}

// GetHeatmap returns the version distribution heatmap.
func (s *Service) GetHeatmap() []VersionHeatmapEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]VersionHeatmapEntry, len(s.heatmap))
	copy(entries, s.heatmap)
	return entries
}

// AddHeatmapEntry adds an entry to the heatmap.
func (s *Service) AddHeatmapEntry(entry VersionHeatmapEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heatmap = append(s.heatmap, entry)
}

// Stats returns service-level statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"translation_rules":  len(s.translationRules),
		"tracked_services":   len(s.serviceVersions),
		"total_translations": s.translationStats.TotalTranslations,
		"error_count":        s.translationStats.ErrorCount,
		"heatmap_entries":    len(s.heatmap),
		"wotan_drops":        s.WotanDrops(),
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

// statsAggregationLoop periodically generates heatmap entries from service versions.
func (s *Service) statsAggregationLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.generateHeatmapSnapshot()
		}
	}
}

// generateHeatmapSnapshot creates a point-in-time snapshot for the heatmap.
func (s *Service) generateHeatmapSnapshot() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	// Count packets per version from translation stats
	for version, count := range s.translationStats.TranslationsByVersion {
		s.heatmap = append(s.heatmap, VersionHeatmapEntry{
			Timestamp:   now,
			Version:     version,
			PacketCount: count,
		})
	}

	// Keep heatmap bounded (max 1000 entries)
	if len(s.heatmap) > 1000 {
		s.heatmap = s.heatmap[len(s.heatmap)-1000:]
	}

	s.log.Debug().
		Int("rules", len(s.translationRules)).
		Int("services", len(s.serviceVersions)).
		Msg("heatmap snapshot generated")
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
		traceID = fmt.Sprintf("version-%d", time.Now().UnixNano())
	}

	event["trace_id"] = traceID
	event["service"] = "version"

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
