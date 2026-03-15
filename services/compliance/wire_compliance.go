// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package compliance provides wire-level policy enforcement and audit capabilities.
// Wire Compliance is the Kingdom's regulatory framework - ensuring service-to-service
// communication adheres to security policies, data classification rules, geographic
// zone restrictions, and encryption requirements. Every violation is audited.
package compliance

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

// PolicyAction defines the action taken on policy match.
type PolicyAction string

const (
	ActionAllow PolicyAction = "ALLOW"
	ActionBlock PolicyAction = "BLOCK"
)

// DataClassification defines the sensitivity level of data.
type DataClassification int

const (
	ClassPublic       DataClassification = 0
	ClassInternal     DataClassification = 1
	ClassSensitive    DataClassification = 2
	ClassSovereignPII DataClassification = 3
)

// String returns the human-readable classification name.
func (dc DataClassification) String() string {
	switch dc {
	case ClassPublic:
		return "PUBLIC"
	case ClassInternal:
		return "INTERNAL"
	case ClassSensitive:
		return "SENSITIVE"
	case ClassSovereignPII:
		return "SOVEREIGN_PII"
	default:
		return "UNKNOWN"
	}
}

// ViolationType categorizes the type of compliance violation.
type ViolationType string

const (
	ViolationPolicy     ViolationType = "POLICY"
	ViolationPIIEgress  ViolationType = "PII_EGRESS"
	ViolationCircuit    ViolationType = "CIRCUIT"
)

// ServicePolicy defines a service-to-service communication policy.
type ServicePolicy struct {
	SrcServiceID      string         `json:"src_service_id"`
	DstServiceID      string         `json:"dst_service_id"`
	Action            PolicyAction   `json:"action"`
	AuditLevel        string         `json:"audit_level"`
	MinQoS            int            `json:"min_qos"`
	MaxLatencyMS      float64        `json:"max_latency_ms"`
	RequiresEncryption bool          `json:"requires_encryption"`
	TraceSampling     float64        `json:"trace_sampling"` // 0.0-1.0
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// policyKey returns a canonical key for a service-to-service policy.
func policyKey(src, dst string) string {
	return src + "->" + dst
}

// Violation records a compliance violation event.
type Violation struct {
	Timestamp    time.Time          `json:"timestamp"`
	SrcServiceID string             `json:"src_service_id"`
	DstServiceID string             `json:"dst_service_id"`
	ViolationType ViolationType     `json:"violation_type"`
	Classification DataClassification `json:"classification"`
	ActionTaken  PolicyAction       `json:"action_taken"`
	TraceID      string             `json:"trace_id"`
	ZoneID       string             `json:"zone_id"`
	Description  string             `json:"description,omitempty"`
}

// ComplianceScore represents the overall compliance posture.
type ComplianceScore struct {
	TotalPackets     int64   `json:"total_packets"`
	Allowed          int64   `json:"allowed"`
	Blocked          int64   `json:"blocked"`
	Violations       int64   `json:"violations"`
	AuditCompleteness float64 `json:"audit_completeness"` // 0.0-1.0
	ViolationRate    float64 `json:"violation_rate"`       // violations per total
	ScorePct         float64 `json:"score_pct"`            // overall percentage
}

// GeoZone defines a geographic compliance zone.
type GeoZone struct {
	ZoneID          string   `json:"zone_id"`
	Name            string   `json:"name"`
	AllowedExitZones []string `json:"allowed_exit_zones"`
	RequiresVPN     bool     `json:"requires_vpn"`
	PIIVault        bool     `json:"pii_vault"`
}

// AuditEntry records an auditable event.
type AuditEntry struct {
	Timestamp    time.Time     `json:"timestamp"`
	TraceID      string        `json:"trace_id"`
	SrcServiceID string        `json:"src_service_id"`
	DstServiceID string        `json:"dst_service_id"`
	Action       PolicyAction  `json:"action"`
	ViolationType ViolationType `json:"violation_type,omitempty"`
	Classification DataClassification `json:"classification"`
	ZoneID       string        `json:"zone_id"`
	Details      string        `json:"details,omitempty"`
}

// Config holds Wire Compliance service configuration.
type Config struct {
	AggregationWindow time.Duration `json:"aggregation_window"`
	MaxViolations     int           `json:"max_violations"`
	MaxAuditEntries   int           `json:"max_audit_entries"`
	WotanTopic        string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		AggregationWindow: 10 * time.Second,
		MaxViolations:     10000,
		MaxAuditEntries:   50000,
		WotanTopic:        "compliance.violations",
	}
}

// WireComplianceService is the main Wire Compliance service.
type WireComplianceService struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu         sync.RWMutex
	policies   map[string]*ServicePolicy // "src->dst" -> policy
	violations []Violation               // violation store
	auditLog   []AuditEntry              // audit trail
	zones      map[string]*GeoZone       // zone_id -> zone
	score      ComplianceScore           // running score

	httpServer *http.Server
	started    int32

	// Alert handling
	alertsCh chan *wotanClient.Message

	// Degraded mode counter
	wotanDrops int64
}

// NewService creates a new Wire Compliance service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *WireComplianceService {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &WireComplianceService{
		log:        log,
		wotan:      wotan,
		config:     cfg,
		policies:   make(map[string]*ServicePolicy),
		violations: make([]Violation, 0),
		auditLog:   make([]AuditEntry, 0),
		zones:      make(map[string]*GeoZone),
		alertsCh:   make(chan *wotanClient.Message, 100),
	}
}

// Start initializes Wotan subscriptions, HTTP server, and violation aggregation.
func (s *WireComplianceService) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		return fmt.Errorf("wire compliance service already started")
	}

	s.log.Info().Msg("wire compliance service starting")

	// Subscribe to Wotan topics
	if s.wotan != nil {
		topics := []string{
			"compliance.violations",
			"compliance.audit_complete",
			"policies.updates",
			"alerts.critical",
		}
		for _, topic := range topics {
			if _, err := s.wotan.Subscribe(ctx, topic, "compliance-service"); err != nil {
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
		Addr:           ports.DefaultAddr(ports.Compliance),
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

	// Start violation aggregation loop
	go s.aggregationLoop(ctx)

	s.log.Info().Int("port", ports.Compliance).Msg("wire compliance service started")
	return nil
}

// Stop gracefully shuts down the service.
func (s *WireComplianceService) Stop() error {
	s.log.Info().Msg("wire compliance service stopping")

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
	s.log.Info().Msg("wire compliance service stopped")
	return nil
}

// registerRoutes sets up all HTTP API endpoints.
func (s *WireComplianceService) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/api/v1/compliance/score", s.handleComplianceScore)
	mux.HandleFunc("/api/v1/compliance/violations", s.handleViolations)
	mux.HandleFunc("/api/v1/compliance/violations/heatmap", s.handleViolationHeatmap)
	mux.HandleFunc("/api/v1/compliance/policies", s.handlePolicies)
	mux.HandleFunc("/api/v1/compliance/audit", s.handleAudit)
	mux.HandleFunc("/api/v1/compliance/zones", s.handleZones)
}

// handleHealth responds with service health status.
func (s *WireComplianceService) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "wire-compliance",
		"timestamp": time.Now(),
	})
}

// handleReady responds with readiness status.
func (s *WireComplianceService) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":   atomic.LoadInt32(&s.started) == 1,
		"service": "wire-compliance",
	})
}

// handleMetrics exposes Prometheus-style metrics.
func (s *WireComplianceService) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sc := s.score
	policyCount := len(s.policies)
	violationCount := len(s.violations)
	auditCount := len(s.auditLog)
	zoneCount := len(s.zones)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP compliance_score_pct Overall compliance score percentage\n")
	fmt.Fprintf(w, "# TYPE compliance_score_pct gauge\n")
	fmt.Fprintf(w, "compliance_score_pct %.2f\n", sc.ScorePct)
	fmt.Fprintf(w, "# HELP compliance_total_packets Total packets evaluated\n")
	fmt.Fprintf(w, "# TYPE compliance_total_packets counter\n")
	fmt.Fprintf(w, "compliance_total_packets %d\n", sc.TotalPackets)
	fmt.Fprintf(w, "# HELP compliance_violations_total Total violations recorded\n")
	fmt.Fprintf(w, "# TYPE compliance_violations_total counter\n")
	fmt.Fprintf(w, "compliance_violations_total %d\n", violationCount)
	fmt.Fprintf(w, "# HELP compliance_policies_total Total active policies\n")
	fmt.Fprintf(w, "# TYPE compliance_policies_total gauge\n")
	fmt.Fprintf(w, "compliance_policies_total %d\n", policyCount)
	fmt.Fprintf(w, "# HELP compliance_audit_entries_total Total audit log entries\n")
	fmt.Fprintf(w, "# TYPE compliance_audit_entries_total counter\n")
	fmt.Fprintf(w, "compliance_audit_entries_total %d\n", auditCount)
	fmt.Fprintf(w, "# HELP compliance_zones_total Total geographic zones\n")
	fmt.Fprintf(w, "# TYPE compliance_zones_total gauge\n")
	fmt.Fprintf(w, "compliance_zones_total %d\n", zoneCount)
	fmt.Fprintf(w, "# HELP compliance_wotan_drops_total Messages dropped due to unavailable Wotan\n")
	fmt.Fprintf(w, "# TYPE compliance_wotan_drops_total counter\n")
	fmt.Fprintf(w, "compliance_wotan_drops_total %d\n", atomic.LoadInt64(&s.wotanDrops))
}

// handleComplianceScore returns the overall compliance score.
func (s *WireComplianceService) handleComplianceScore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	sc := s.score
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sc)
}

// handleViolations handles GET for /api/v1/compliance/violations (paginated).
func (s *WireComplianceService) handleViolations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	violations := make([]Violation, len(s.violations))
	copy(violations, s.violations)
	s.mu.RUnlock()

	// Simple pagination via query params
	offset := 0
	limit := 100
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}

	total := len(violations)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := violations[offset:end]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"violations": page,
		"total":      total,
		"offset":     offset,
		"limit":      limit,
	})
}

// handleViolationHeatmap returns service matrix data for violations.
func (s *WireComplianceService) handleViolationHeatmap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	heatmap := make(map[string]int) // "src->dst" -> count
	for _, v := range s.violations {
		key := policyKey(v.SrcServiceID, v.DstServiceID)
		heatmap[key]++
	}
	s.mu.RUnlock()

	// Convert to structured response
	type heatmapEntry struct {
		SrcServiceID string `json:"src_service_id"`
		DstServiceID string `json:"dst_service_id"`
		Count        int    `json:"count"`
	}

	entries := make([]heatmapEntry, 0, len(heatmap))
	for key, count := range heatmap {
		parts := strings.SplitN(key, "->", 2)
		if len(parts) == 2 {
			entries = append(entries, heatmapEntry{
				SrcServiceID: parts[0],
				DstServiceID: parts[1],
				Count:        count,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"heatmap": entries,
		"count":   len(entries),
	})
}

// handlePolicies handles GET (list) and POST (update) for policies.
func (s *WireComplianceService) handlePolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPolicies(w, r)
	case http.MethodPost:
		s.updatePolicy(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listPolicies returns all service-to-service policies.
func (s *WireComplianceService) listPolicies(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	policies := make([]*ServicePolicy, 0, len(s.policies))
	for _, p := range s.policies {
		policies = append(policies, p)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"policies": policies,
		"count":    len(policies),
	})
}

// updatePolicy creates or updates a service-to-service policy.
func (s *WireComplianceService) updatePolicy(w http.ResponseWriter, r *http.Request) {
	var policy ServicePolicy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if policy.SrcServiceID == "" || policy.DstServiceID == "" {
		http.Error(w, "src_service_id and dst_service_id are required", http.StatusBadRequest)
		return
	}

	now := time.Now()
	key := policyKey(policy.SrcServiceID, policy.DstServiceID)

	s.mu.Lock()
	existing, exists := s.policies[key]
	if exists {
		policy.CreatedAt = existing.CreatedAt
	} else {
		policy.CreatedAt = now
	}
	policy.UpdatedAt = now
	s.policies[key] = &policy
	s.mu.Unlock()

	s.log.Info().
		Str("src", policy.SrcServiceID).
		Str("dst", policy.DstServiceID).
		Str("action", string(policy.Action)).
		Msg("policy updated")

	s.publishEvent(context.Background(), "policies.updates", map[string]interface{}{
		"src_service_id": policy.SrcServiceID,
		"dst_service_id": policy.DstServiceID,
		"action":         policy.Action,
		"timestamp":      now.UnixMilli(),
	})

	status := http.StatusOK
	if !exists {
		status = http.StatusCreated
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(&policy)
}

// handleAudit handles GET for /api/v1/compliance/audit with search filters.
func (s *WireComplianceService) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	traceID := r.URL.Query().Get("trace_id")
	violationTypeFilter := r.URL.Query().Get("violation_type")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	s.mu.RLock()
	results := make([]AuditEntry, 0)
	for _, entry := range s.auditLog {
		if traceID != "" && entry.TraceID != traceID {
			continue
		}
		if violationTypeFilter != "" && string(entry.ViolationType) != violationTypeFilter {
			continue
		}
		if startStr != "" {
			if start, err := time.Parse(time.RFC3339, startStr); err == nil {
				if entry.Timestamp.Before(start) {
					continue
				}
			}
		}
		if endStr != "" {
			if end, err := time.Parse(time.RFC3339, endStr); err == nil {
				if entry.Timestamp.After(end) {
					continue
				}
			}
		}
		results = append(results, entry)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"audit_entries": results,
		"count":         len(results),
	})
}

// handleZones returns geographic zone configuration.
func (s *WireComplianceService) handleZones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	zones := make([]*GeoZone, 0, len(s.zones))
	for _, z := range s.zones {
		zones = append(zones, z)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"zones": zones,
		"count": len(zones),
	})
}

// aggregationLoop runs violation aggregation at the configured window interval.
func (s *WireComplianceService) aggregationLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.AggregationWindow)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.aggregateViolations(ctx)
		}
	}
}

// aggregateViolations recalculates the compliance score.
func (s *WireComplianceService) aggregateViolations(_ context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	totalPackets := s.score.TotalPackets
	if totalPackets == 0 {
		s.score.ScorePct = 100.0
		s.score.AuditCompleteness = 1.0
		return
	}

	violations := int64(len(s.violations))
	s.score.Violations = violations

	if totalPackets > 0 {
		s.score.ViolationRate = float64(violations) / float64(totalPackets)
		s.score.ScorePct = (1.0 - s.score.ViolationRate) * 100.0
		if s.score.ScorePct < 0 {
			s.score.ScorePct = 0
		}
	}

	// Audit completeness: ratio of audit entries to total packets
	auditEntries := int64(len(s.auditLog))
	if totalPackets > 0 {
		s.score.AuditCompleteness = float64(auditEntries) / float64(totalPackets)
		if s.score.AuditCompleteness > 1.0 {
			s.score.AuditCompleteness = 1.0
		}
	}

	// Trim violations if over max
	if len(s.violations) > s.config.MaxViolations {
		s.violations = s.violations[len(s.violations)-s.config.MaxViolations:]
	}
	if len(s.auditLog) > s.config.MaxAuditEntries {
		s.auditLog = s.auditLog[len(s.auditLog)-s.config.MaxAuditEntries:]
	}
}

// MatchPolicy evaluates a request against policies and returns the matching policy.
func (s *WireComplianceService) MatchPolicy(srcServiceID, dstServiceID string) (*ServicePolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := policyKey(srcServiceID, dstServiceID)
	policy, ok := s.policies[key]
	return policy, ok
}

// RecordViolation records a compliance violation.
func (s *WireComplianceService) RecordViolation(v Violation) {
	s.mu.Lock()
	s.violations = append(s.violations, v)
	s.score.Blocked++
	s.score.TotalPackets++

	// Also add to audit log
	s.auditLog = append(s.auditLog, AuditEntry{
		Timestamp:      v.Timestamp,
		TraceID:        v.TraceID,
		SrcServiceID:   v.SrcServiceID,
		DstServiceID:   v.DstServiceID,
		Action:         v.ActionTaken,
		ViolationType:  v.ViolationType,
		Classification: v.Classification,
		ZoneID:         v.ZoneID,
		Details:        v.Description,
	})
	s.mu.Unlock()

	s.publishEvent(context.Background(), "compliance.violations", map[string]interface{}{
		"src_service_id":  v.SrcServiceID,
		"dst_service_id":  v.DstServiceID,
		"violation_type":  v.ViolationType,
		"classification":  v.Classification,
		"action_taken":    v.ActionTaken,
		"trace_id":        v.TraceID,
		"timestamp":       v.Timestamp.UnixMilli(),
	})
}

// RecordAllowed records a successful policy check (allowed traffic).
func (s *WireComplianceService) RecordAllowed() {
	s.mu.Lock()
	s.score.Allowed++
	s.score.TotalPackets++
	s.mu.Unlock()
}

// AddZone adds a geographic compliance zone.
func (s *WireComplianceService) AddZone(zone *GeoZone) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zones[zone.ZoneID] = zone
}

// GetZone returns a zone by ID.
func (s *WireComplianceService) GetZone(zoneID string) (*GeoZone, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	z, ok := s.zones[zoneID]
	return z, ok
}

// listenForAlerts listens for critical alerts from Wotan.
func (s *WireComplianceService) listenForAlerts(ctx context.Context) {
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
func (s *WireComplianceService) handleCriticalAlert(_ context.Context, msg *wotanClient.Message) {
	if msg == nil {
		return
	}

	s.log.Warn().
		Str("message_id", msg.MessageID).
		Str("topic", msg.Topic).
		Msg("received critical alert — auditing compliance posture")
}

// publishEvent publishes an event to Wotan.
func (s *WireComplianceService) publishEvent(ctx context.Context, topic string, event map[string]interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		s.log.Warn().Str("topic", topic).Msg("wotan unavailable — event dropped (degraded mode)")
		return
	}

	event["service"] = "wire-compliance"
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
func (s *WireComplianceService) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// GetScore returns the current compliance score.
func (s *WireComplianceService) GetScore() ComplianceScore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.score
}

// ListViolations returns all recorded violations.
func (s *WireComplianceService) ListViolations() []Violation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Violation, len(s.violations))
	copy(result, s.violations)
	return result
}

// ListPolicies returns all policies.
func (s *WireComplianceService) ListPolicies() []*ServicePolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*ServicePolicy, 0, len(s.policies))
	for _, p := range s.policies {
		result = append(result, p)
	}
	return result
}

// Stats returns service statistics.
func (s *WireComplianceService) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"total_policies":      len(s.policies),
		"total_violations":    len(s.violations),
		"total_audit_entries": len(s.auditLog),
		"total_zones":         len(s.zones),
		"compliance_score":    s.score.ScorePct,
		"total_packets":       s.score.TotalPackets,
		"allowed_packets":     s.score.Allowed,
		"blocked_packets":     s.score.Blocked,
		"wotan_drops":         atomic.LoadInt64(&s.wotanDrops),
	}
}
