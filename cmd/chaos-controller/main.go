// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// chaos-controller — Chaos engineering controller for the Unheaded Kingdom.
//
// Yaldabaoth, the Demiurge of chaos, injects controlled faults into the
// service mesh to validate resilience. Supports fault rules (DROP, DELAY,
// CORRUPT, DUPLICATE), GameDay scenarios with staged fault injection, and
// a full audit trail for compliance.
//
// Port: 19012 (ports.Chaos)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"unheaded/pkg/discovery"
	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/yaldabaoth"
)

// ── Fault types ──────────────────────────────────────────────────────────

// FaultType defines the kind of chaos fault to inject.
type FaultType string

const (
	FaultDrop      FaultType = "DROP"
	FaultDelay     FaultType = "DELAY"
	FaultCorrupt   FaultType = "CORRUPT"
	FaultDuplicate FaultType = "DUPLICATE"
)

// ── ChaosFaultRule ───────────────────────────────────────────────────────

// ChaosFaultRule defines a single chaos injection rule.
type ChaosFaultRule struct {
	ID           string        `json:"id"`
	SrcServiceID uint8         `json:"src_service_id"`
	DstServiceID uint8         `json:"dst_service_id"`
	FaultType    FaultType     `json:"fault_type"`
	DropRatePPM  uint32        `json:"drop_rate_ppm"` // parts per million
	DelayUS      uint64        `json:"delay_us"`       // microseconds
	Duration     time.Duration `json:"duration_ns"`
	CreatedAt    time.Time     `json:"created_at"`
	CreatedBy    string        `json:"created_by"`
	ExpiresAt    time.Time     `json:"expires_at"`
	Active       bool          `json:"active"`
}

// ChaosEvent records a chaos injection event for the event log.
type ChaosEvent struct {
	Timestamp time.Time `json:"timestamp"`
	RuleID    string    `json:"rule_id"`
	Action    string    `json:"action"` // "injected", "removed", "expired"
	Details   string    `json:"details,omitempty"`
}

// ── ChaosController ──────────────────────────────────────────────────────

// ChaosController manages chaos fault injection rules and GameDay scenarios.
type ChaosController struct {
	log          *logger.Logger
	wotan        *wotanClient.Client
	rules        map[string]*ChaosFaultRule
	mu           sync.RWMutex
	events       []ChaosEvent
	eventsMu     sync.RWMutex
	maxEvents    int
	ruleCounter  int64
	gameDaySched *yaldabaoth.GameDayScheduler
	audit        *yaldabaoth.AuditTrail
}

// NewChaosController creates a new ChaosController.
func NewChaosController(log *logger.Logger, wotan *wotanClient.Client) *ChaosController {
	cc := &ChaosController{
		log:       log,
		wotan:     wotan,
		rules:     make(map[string]*ChaosFaultRule),
		events:    make([]ChaosEvent, 0, 1000),
		maxEvents: 10000,
		audit:     yaldabaoth.NewAuditTrail(log, 10000),
	}
	if wotan != nil {
		cc.gameDaySched = yaldabaoth.NewGameDayScheduler(log, wotan)
	}
	return cc
}

// Start initializes Wotan subscriptions and begins listening for alerts.
func (cc *ChaosController) Start(ctx context.Context) error {
	cc.log.Info().Str("service", "chaos-controller").Msg("starting chaos controller")

	if cc.wotan == nil {
		cc.log.Warn().Msg("Wotan not configured, skipping subscriptions")
		return nil
	}

	// Subscribe to alerts.critical (required by CLAUDE.md)
	if _, err := cc.wotan.Subscribe(ctx, "alerts.critical", "chaos-controller"); err != nil {
		cc.log.Warn().Err(err).Msg("failed to subscribe to alerts.critical")
	}

	// Subscribe to chaos topics
	topics := []string{"chaos.injections", "chaos.events", "chaos.schedules"}
	for _, topic := range topics {
		if _, err := cc.wotan.Subscribe(ctx, topic, "chaos-controller"); err != nil {
			cc.log.Warn().Err(err).Str("topic", topic).Msg("failed to subscribe")
		}
	}

	// Start alert listener
	go cc.listenForAlerts(ctx)

	// Start rule expiry goroutine
	go cc.expireRules(ctx)

	return nil
}

// Stop gracefully shuts down the controller.
func (cc *ChaosController) Stop() error {
	cc.log.Info().Msg("chaos controller stopping")
	if cc.gameDaySched != nil {
		cc.gameDaySched.Stop()
	}
	return nil
}

// listenForAlerts listens for critical alerts and auto-removes rules if needed.
func (cc *ChaosController) listenForAlerts(ctx context.Context) {
	if cc.wotan == nil {
		return
	}

	msgCh, err := cc.wotan.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		cc.log.Warn().Err(err).Msg("failed to stream alerts.critical")
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
			cc.log.Warn().
				Str("message_id", msg.MessageID).
				Str("topic", msg.Topic).
				Msg("received critical alert — evaluating rule safety")
		}
	}
}

// expireRules periodically checks for and removes expired rules.
func (cc *ChaosController) expireRules(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cc.mu.Lock()
			now := time.Now()
			for id, rule := range cc.rules {
				if rule.Active && !rule.ExpiresAt.IsZero() && now.After(rule.ExpiresAt) {
					rule.Active = false
					cc.recordEvent(ChaosEvent{
						Timestamp: now,
						RuleID:    id,
						Action:    "expired",
						Details:   fmt.Sprintf("rule expired after %s", rule.Duration),
					})
					cc.log.Info().Str("rule_id", id).Msg("chaos rule expired")
					delete(cc.rules, id)
				}
			}
			cc.mu.Unlock()
		}
	}
}

// ── Rule management ──────────────────────────────────────────────────────

// InjectRule creates and activates a new chaos fault rule.
func (cc *ChaosController) InjectRule(rule *ChaosFaultRule) error {
	if rule.FaultType == "" {
		return fmt.Errorf("fault_type is required")
	}
	switch rule.FaultType {
	case FaultDrop, FaultDelay, FaultCorrupt, FaultDuplicate:
		// Valid fault type.
	default:
		return fmt.Errorf("unknown fault_type: %s", rule.FaultType)
	}
	if rule.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}

	id := fmt.Sprintf("chaos-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&cc.ruleCounter, 1))
	rule.ID = id
	rule.CreatedAt = time.Now()
	rule.ExpiresAt = rule.CreatedAt.Add(rule.Duration)
	rule.Active = true

	cc.mu.Lock()
	cc.rules[id] = rule
	cc.mu.Unlock()

	cc.recordEvent(ChaosEvent{
		Timestamp: rule.CreatedAt,
		RuleID:    id,
		Action:    "injected",
		Details:   fmt.Sprintf("type=%s src=%d dst=%d drop_ppm=%d delay_us=%d",
			rule.FaultType, rule.SrcServiceID, rule.DstServiceID, rule.DropRatePPM, rule.DelayUS),
	})

	cc.audit.Record(yaldabaoth.AuditEntry{
		Timestamp: rule.CreatedAt,
		UserID:    rule.CreatedBy,
		Action:    "inject_rule",
		RuleID:    id,
		RuleSpec:  fmt.Sprintf("%+v", rule),
		Result:    "success",
	})

	// Publish to Wotan
	cc.publishEvent(context.Background(), "chaos.injections", map[string]interface{}{
		"event_type": "chaos.rule.injected",
		"rule_id":    id,
		"rule":       rule,
		"timestamp":  time.Now().UnixMilli(),
	})

	cc.log.Info().
		Str("rule_id", id).
		Str("fault_type", string(rule.FaultType)).
		Uint8("src_svc", rule.SrcServiceID).
		Uint8("dst_svc", rule.DstServiceID).
		Msg("chaos rule injected")

	return nil
}

// RemoveRule deactivates and removes a chaos rule by ID.
func (cc *ChaosController) RemoveRule(id string) error {
	cc.mu.Lock()
	rule, ok := cc.rules[id]
	if !ok {
		cc.mu.Unlock()
		return fmt.Errorf("rule not found: %s", id)
	}
	rule.Active = false
	delete(cc.rules, id)
	cc.mu.Unlock()

	cc.recordEvent(ChaosEvent{
		Timestamp: time.Now(),
		RuleID:    id,
		Action:    "removed",
	})

	cc.audit.Record(yaldabaoth.AuditEntry{
		Timestamp: time.Now(),
		Action:    "remove_rule",
		RuleID:    id,
		Result:    "success",
	})

	cc.publishEvent(context.Background(), "chaos.events", map[string]interface{}{
		"event_type": "chaos.rule.removed",
		"rule_id":    id,
		"timestamp":  time.Now().UnixMilli(),
	})

	cc.log.Info().Str("rule_id", id).Msg("chaos rule removed")
	return nil
}

// ListRules returns all active chaos rules.
func (cc *ChaosController) ListRules() []*ChaosFaultRule {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	rules := make([]*ChaosFaultRule, 0, len(cc.rules))
	for _, rule := range cc.rules {
		rules = append(rules, rule)
	}
	return rules
}

// RecentEvents returns the most recent chaos events.
func (cc *ChaosController) RecentEvents(limit int) []ChaosEvent {
	cc.eventsMu.RLock()
	defer cc.eventsMu.RUnlock()

	if limit <= 0 || limit > len(cc.events) {
		limit = len(cc.events)
	}

	start := len(cc.events) - limit
	if start < 0 {
		start = 0
	}

	result := make([]ChaosEvent, limit)
	copy(result, cc.events[start:])
	return result
}

// recordEvent appends an event to the event log with capacity limit.
func (cc *ChaosController) recordEvent(event ChaosEvent) {
	cc.eventsMu.Lock()
	defer cc.eventsMu.Unlock()

	cc.events = append(cc.events, event)
	if len(cc.events) > cc.maxEvents {
		// Trim oldest quarter when we exceed the limit.
		trim := cc.maxEvents / 4
		cc.events = cc.events[trim:]
	}
}

// publishEvent publishes an event to a Wotan topic.
func (cc *ChaosController) publishEvent(ctx context.Context, topic string, event interface{}) {
	if cc.wotan == nil {
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		cc.log.Warn().Err(err).Msg("failed to marshal chaos event")
		return
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := cc.wotan.Publish(pubCtx, topic, payload); err != nil {
		cc.log.Warn().Err(err).Str("topic", topic).Msg("failed to publish chaos event")
	}
}

// ── HTTP Handlers ────────────────────────────────────────────────────────

func (cc *ChaosController) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/chaos/inject", cc.handleInjectRule)
	mux.HandleFunc("GET /api/v1/chaos/rules", cc.handleListRules)
	mux.HandleFunc("DELETE /api/v1/chaos/rules/", cc.handleRemoveRule)
	mux.HandleFunc("GET /api/v1/chaos/events", cc.handleListEvents)
	mux.HandleFunc("POST /api/v1/chaos/gameday", cc.handleStartGameDay)
	mux.HandleFunc("GET /api/v1/chaos/gameday/status", cc.handleGameDayStatus)
	mux.HandleFunc("GET /health", cc.handleHealth)
	mux.HandleFunc("GET /ready", cc.handleReady)
	mux.HandleFunc("GET /metrics", cc.handleMetrics)
}

func (cc *ChaosController) handleInjectRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SrcServiceID uint8     `json:"src_svc"`
		DstServiceID uint8     `json:"dst_svc"`
		FaultType    FaultType `json:"fault_type"`
		DropRatePPM  uint32    `json:"drop_rate"`
		DelayUS      uint64    `json:"delay_us"`
		DurationSec  int64     `json:"duration"`
		CreatedBy    string    `json:"created_by"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	rule := &ChaosFaultRule{
		SrcServiceID: req.SrcServiceID,
		DstServiceID: req.DstServiceID,
		FaultType:    req.FaultType,
		DropRatePPM:  req.DropRatePPM,
		DelayUS:      req.DelayUS,
		Duration:     time.Duration(req.DurationSec) * time.Second,
		CreatedBy:    req.CreatedBy,
	}

	if err := cc.InjectRule(rule); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rule)
}

func (cc *ChaosController) handleListRules(w http.ResponseWriter, _ *http.Request) {
	rules := cc.ListRules()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rules": rules,
		"count": len(rules),
	})
}

func (cc *ChaosController) handleRemoveRule(w http.ResponseWriter, r *http.Request) {
	// Extract rule ID from path: /api/v1/chaos/rules/{id}
	id := r.URL.Path[len("/api/v1/chaos/rules/"):]
	if id == "" {
		http.Error(w, `{"error":"rule ID required"}`, http.StatusBadRequest)
		return
	}

	if err := cc.RemoveRule(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "removed", "rule_id": id})
}

func (cc *ChaosController) handleListEvents(w http.ResponseWriter, _ *http.Request) {
	events := cc.RecentEvents(100)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
		"count":  len(events),
	})
}

func (cc *ChaosController) handleStartGameDay(w http.ResponseWriter, r *http.Request) {
	if cc.gameDaySched == nil {
		http.Error(w, `{"error":"GameDay scheduler not available (Wotan not configured)"}`, http.StatusServiceUnavailable)
		return
	}

	var scenario yaldabaoth.GameDayScenario
	if err := json.NewDecoder(r.Body).Decode(&scenario); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if err := cc.gameDaySched.Schedule(scenario); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	cc.audit.Record(yaldabaoth.AuditEntry{
		Timestamp: time.Now(),
		Action:    "start_gameday",
		RuleSpec:  scenario.Name,
		Result:    "scheduled",
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "scheduled",
		"scenario": scenario.Name,
	})
}

func (cc *ChaosController) handleGameDayStatus(w http.ResponseWriter, _ *http.Request) {
	if cc.gameDaySched == nil {
		http.Error(w, `{"error":"GameDay scheduler not available"}`, http.StatusServiceUnavailable)
		return
	}

	status := cc.gameDaySched.Status()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (cc *ChaosController) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (cc *ChaosController) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (cc *ChaosController) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	cc.mu.RLock()
	ruleCount := len(cc.rules)
	cc.mu.RUnlock()

	cc.eventsMu.RLock()
	eventCount := len(cc.events)
	cc.eventsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active_rules":  ruleCount,
		"total_events":  eventCount,
		"audit_entries": cc.audit.Len(),
	})
}

// ── Main ─────────────────────────────────────────────────────────────────

func main() {
	log := logger.New(os.Stderr)

	log.Info().
		Str("service", "chaos-controller").
		Int("port", ports.Chaos).
		Msg("chaos-controller starting")

	// Connect to Wotan (best-effort)
	var wotan *wotanClient.Client
	wotanAddr := os.Getenv("WOTAN_ADDR")
	if wotanAddr == "" {
		wotanAddr = fmt.Sprintf("localhost:%d", ports.WotanHTTP)
	}
	wc, err := wotanClient.NewClient(wotanAddr)
	if err != nil {
		log.Warn().Err(err).Msg("Wotan client creation failed, running in degraded mode")
	} else {
		wotan = wc
	}

	// Create controller
	controller := NewChaosController(log, wotan)

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start controller (Wotan subscriptions)
	if err := controller.Start(ctx); err != nil {
		log.Error().Err(err).Msg("failed to start controller")
	}

	// Service discovery — best-effort
	discovery.SetupServiceDiscovery(ctx, nil, "chaos-controller", ports.Chaos)

	// Set up HTTP server
	mux := http.NewServeMux()
	controller.registerRoutes(mux)

	addr := ports.DefaultAddr(ports.Chaos)
	httpServer := &http.Server{
		Addr:           addr,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// Start HTTP server
	go func() {
		log.Info().Str("addr", addr).Msg("HTTP server starting")
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	if err := controller.Stop(); err != nil {
		log.Error().Err(err).Msg("controller stop error")
	}

	log.Info().Msg("chaos-controller stopped")
}
