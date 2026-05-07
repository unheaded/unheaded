// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package nfv provides NFV (Network Function Virtualization) chain management.
// The NFV Chain Manager orchestrates BPF function chaining, managing up to 64
// active chains with 10 functions per chain (BPF tail-call depth limit).
// It tracks per-chain telemetry, supports hot-reload of BPF functions, and
// publishes chain lifecycle events to Wotan.
package nfv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
)

// Chain limits — derived from BPF tail-call architecture.
const (
	// MaxFunctionsPerChain is the BPF tail-call depth limit.
	MaxFunctionsPerChain = 10
	// MaxActiveChains is the maximum number of active chains (6-bit CHAIN_INDEX).
	MaxActiveChains = 64
)

// AuthLevel defines the authorization level for chain operations.
type AuthLevel int

const (
	// AuthNormal is the default authorization level.
	AuthNormal AuthLevel = iota
	// AuthPriority grants elevated priority for chain operations.
	AuthPriority
	// AuthReserved is reserved for system-internal use.
	AuthReserved
	// AuthExperimental allows experimental chain configurations.
	AuthExperimental
)

// String returns the string representation of an AuthLevel.
func (a AuthLevel) String() string {
	switch a {
	case AuthNormal:
		return "NORMAL"
	case AuthPriority:
		return "PRIORITY"
	case AuthReserved:
		return "RESERVED"
	case AuthExperimental:
		return "EXPERIMENTAL"
	default:
		return "UNKNOWN"
	}
}

// ChainDefinition describes an NFV chain.
type ChainDefinition struct {
	ChainID          int           `json:"chain_id"` // 0-63
	Name             string        `json:"name"`
	Description      string        `json:"description,omitempty"`
	Functions        []FunctionRef `json:"functions"`
	NextOnDrop       bool          `json:"next_on_drop"`
	TelemetryEnabled bool          `json:"telemetry_enabled"`
	AuthLevel        AuthLevel     `json:"auth_level"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

// FunctionRef references a BPF function within a chain.
type FunctionRef struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	ProgFD   int    `json:"prog_fd"`
	Required bool   `json:"required"`
}

// ChainStats holds per-chain telemetry data.
type ChainStats struct {
	ChainID          int            `json:"chain_id"`
	PacketsProcessed uint64         `json:"packets_processed"`
	Drops            uint64         `json:"drops"`
	TotalLatencyUS   uint64         `json:"total_latency_us"`
	FunctionStats    []FunctionStat `json:"function_stats"`
	LastUpdated      time.Time      `json:"last_updated"`
}

// FunctionStat holds per-function telemetry within a chain.
type FunctionStat struct {
	Name         string  `json:"name"`
	Invocations  uint64  `json:"invocations"`
	AvgLatencyUS float64 `json:"avg_latency_us"`
	DropCount    uint64  `json:"drop_count"`
}

// FunctionEntry represents a registered BPF function.
type FunctionEntry struct {
	Name     string    `json:"name"`
	ProgFD   int       `json:"prog_fd"`
	LoadedAt time.Time `json:"loaded_at"`
	RefCount int       `json:"ref_count"`
}

// Service is the main NFV Chain Manager service.
type Service struct {
	log   *logger.Logger
	wotan *wotanClient.Client

	mu               sync.RWMutex
	chains           map[int]*ChainDefinition
	functionRegistry map[string]*FunctionEntry
	chainStats       map[int]*ChainStats

	// Alert handling
	alertsCh chan *wotanClient.Message

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64

	// HTTP server reference for graceful shutdown
	httpServer *http.Server
}

// NewService creates a new NFV Chain Manager service.
func NewService(log *logger.Logger, wotan *wotanClient.Client) *Service {
	return &Service{
		log:              log,
		wotan:            wotan,
		chains:           make(map[int]*ChainDefinition),
		functionRegistry: make(map[string]*FunctionEntry),
		chainStats:       make(map[int]*ChainStats),
		alertsCh:         make(chan *wotanClient.Message, 100),
	}
}

// Start initializes Wotan subscriptions, the HTTP server, and background goroutines.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("NFV Chain Manager starting")

	// Subscribe to Wotan topics
	if s.wotan != nil {
		topics := []string{
			"nfv.chain.created",
			"nfv.chain.deleted",
			"nfv.function.loaded",
			"nfv.stats.per_chain",
			"nfv.circuit_breaker",
			"alerts.critical",
		}
		for _, topic := range topics {
			if _, err := s.wotan.Subscribe(ctx, topic, "nfv-service"); err != nil {
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
		Addr:           ports.DefaultAddr(ports.NFV),
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		s.log.Info().Int("port", ports.NFV).Msg("NFV HTTP server listening")
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("NFV Chain Manager stopping")

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
	// Chain management
	mux.HandleFunc("/api/v1/chains", s.handleChains)
	mux.HandleFunc("/api/v1/chains/", s.handleChainByID)

	// Function management
	mux.HandleFunc("/api/v1/functions", s.handleFunctions)
	mux.HandleFunc("/api/v1/functions/reload", s.handleFunctionReload)

	// Health endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)
}

// handleChains handles GET (list) and POST (create/update) on /api/v1/chains.
func (s *Service) handleChains(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listChains(w, r)
	case http.MethodPost:
		s.createOrUpdateChain(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleChainByID handles GET, DELETE on /api/v1/chains/{chain_id} and
// GET on /api/v1/chains/{chain_id}/telemetry.
func (s *Service) handleChainByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/chains/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "chain_id required", http.StatusBadRequest)
		return
	}

	chainID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "invalid chain_id", http.StatusBadRequest)
		return
	}

	// Check for telemetry sub-path
	if len(parts) == 2 && parts[1] == "telemetry" {
		s.getChainTelemetry(w, r, chainID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getChain(w, r, chainID)
	case http.MethodDelete:
		s.deleteChain(w, r, chainID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listChains returns all chain definitions.
func (s *Service) listChains(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	chains := make([]*ChainDefinition, 0, len(s.chains))
	for _, chain := range s.chains {
		chains = append(chains, chain)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"chains": chains,
		"count":  len(chains),
	})
}

// getChain returns a specific chain by ID.
func (s *Service) getChain(w http.ResponseWriter, _ *http.Request, chainID int) {
	s.mu.RLock()
	chain, ok := s.chains[chainID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "chain not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chain)
}

// createOrUpdateChain creates or updates a chain definition.
func (s *Service) createOrUpdateChain(w http.ResponseWriter, r *http.Request) {
	var chain ChainDefinition
	if err := json.NewDecoder(r.Body).Decode(&chain); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate chain ID range
	if chain.ChainID < 0 || chain.ChainID >= MaxActiveChains {
		http.Error(w, fmt.Sprintf("chain_id must be 0-%d", MaxActiveChains-1), http.StatusBadRequest)
		return
	}

	// Validate function count
	if len(chain.Functions) > MaxFunctionsPerChain {
		http.Error(w, fmt.Sprintf("max %d functions per chain", MaxFunctionsPerChain), http.StatusBadRequest)
		return
	}

	// Validate chain name
	if chain.Name == "" {
		http.Error(w, "chain name required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	// Check max active chains if this is a new chain
	_, exists := s.chains[chain.ChainID]
	if !exists && len(s.chains) >= MaxActiveChains {
		s.mu.Unlock()
		http.Error(w, fmt.Sprintf("max %d active chains reached", MaxActiveChains), http.StatusConflict)
		return
	}

	now := time.Now()
	if !exists {
		chain.CreatedAt = now
	}
	chain.UpdatedAt = now
	s.chains[chain.ChainID] = &chain
	s.mu.Unlock()

	s.log.Info().
		Int("chain_id", chain.ChainID).
		Str("name", chain.Name).
		Int("functions", len(chain.Functions)).
		Msg("chain created/updated")

	// Publish event to Wotan
	s.publishEvent(r.Context(), "nfv.chain.created", map[string]interface{}{
		"chain_id":  chain.ChainID,
		"name":      chain.Name,
		"functions": len(chain.Functions),
		"timestamp": now.UnixMilli(),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(chain)
}

// deleteChain removes a chain definition.
func (s *Service) deleteChain(w http.ResponseWriter, r *http.Request, chainID int) {
	s.mu.Lock()
	chain, ok := s.chains[chainID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "chain not found", http.StatusNotFound)
		return
	}
	delete(s.chains, chainID)
	delete(s.chainStats, chainID)
	s.mu.Unlock()

	s.log.Info().
		Int("chain_id", chainID).
		Str("name", chain.Name).
		Msg("chain deleted")

	// Publish event to Wotan
	s.publishEvent(r.Context(), "nfv.chain.deleted", map[string]interface{}{
		"chain_id":  chainID,
		"name":      chain.Name,
		"timestamp": time.Now().UnixMilli(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
	})
}

// getChainTelemetry returns per-function stats for a chain.
func (s *Service) getChainTelemetry(w http.ResponseWriter, _ *http.Request, chainID int) {
	s.mu.RLock()
	_, chainExists := s.chains[chainID]
	stats, statsExist := s.chainStats[chainID]
	s.mu.RUnlock()

	if !chainExists {
		http.Error(w, "chain not found", http.StatusNotFound)
		return
	}

	if !statsExist {
		stats = &ChainStats{ChainID: chainID}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleFunctions handles GET on /api/v1/functions.
func (s *Service) handleFunctions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	functions := make([]*FunctionEntry, 0, len(s.functionRegistry))
	for _, fn := range s.functionRegistry {
		functions = append(functions, fn)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"functions": functions,
		"count":     len(functions),
	})
}

// handleFunctionReload handles POST on /api/v1/functions/reload.
// Hot-reloads a BPF function with a 200ms grace period.
func (s *Service) handleFunctionReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name   string `json:"name"`
		ProgFD int    `json:"prog_fd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "function name required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	entry, exists := s.functionRegistry[req.Name]
	if !exists {
		entry = &FunctionEntry{
			Name:     req.Name,
			LoadedAt: time.Now(),
		}
		s.functionRegistry[req.Name] = entry
	}
	entry.ProgFD = req.ProgFD
	entry.LoadedAt = time.Now()

	// Update all chains referencing this function
	for _, chain := range s.chains {
		for i := range chain.Functions {
			if chain.Functions[i].Name == req.Name {
				chain.Functions[i].ProgFD = req.ProgFD
			}
		}
	}
	s.mu.Unlock()

	// 200ms grace period for BPF program swap
	time.Sleep(200 * time.Millisecond)

	s.log.Info().
		Str("function", req.Name).
		Int("prog_fd", req.ProgFD).
		Msg("function hot-reloaded")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "reloaded",
		"name":         req.Name,
		"prog_fd":      req.ProgFD,
		"grace_period": "200ms",
	})
}

// handleHealth returns service health status.
func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "nfv",
	})
}

// handleReady returns service readiness status.
func (s *Service) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":   true,
		"service": "nfv",
	})
}

// handleMetrics returns Prometheus-format metrics.
func (s *Service) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	chainCount := len(s.chains)
	functionCount := len(s.functionRegistry)
	var totalPackets, totalDrops uint64
	for _, stats := range s.chainStats {
		totalPackets += stats.PacketsProcessed
		totalDrops += stats.Drops
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP nfv_chains_total Total active chains\n")
	fmt.Fprintf(w, "# TYPE nfv_chains_total gauge\n")
	fmt.Fprintf(w, "nfv_chains_total %d\n", chainCount)
	fmt.Fprintf(w, "# HELP nfv_functions_total Total registered functions\n")
	fmt.Fprintf(w, "# TYPE nfv_functions_total gauge\n")
	fmt.Fprintf(w, "nfv_functions_total %d\n", functionCount)
	fmt.Fprintf(w, "# HELP nfv_packets_processed_total Total packets processed across all chains\n")
	fmt.Fprintf(w, "# TYPE nfv_packets_processed_total counter\n")
	fmt.Fprintf(w, "nfv_packets_processed_total %d\n", totalPackets)
	fmt.Fprintf(w, "# HELP nfv_drops_total Total packets dropped across all chains\n")
	fmt.Fprintf(w, "# TYPE nfv_drops_total counter\n")
	fmt.Fprintf(w, "nfv_drops_total %d\n", totalDrops)
	fmt.Fprintf(w, "# HELP nfv_wotan_drops_total Messages dropped due to Wotan unavailability\n")
	fmt.Fprintf(w, "# TYPE nfv_wotan_drops_total counter\n")
	fmt.Fprintf(w, "nfv_wotan_drops_total %d\n", s.WotanDrops())
}

// CreateChain creates a new chain programmatically.
func (s *Service) CreateChain(ctx context.Context, chain *ChainDefinition) error {
	if chain.ChainID < 0 || chain.ChainID >= MaxActiveChains {
		return fmt.Errorf("chain_id must be 0-%d", MaxActiveChains-1)
	}
	if len(chain.Functions) > MaxFunctionsPerChain {
		return fmt.Errorf("max %d functions per chain", MaxFunctionsPerChain)
	}
	if chain.Name == "" {
		return fmt.Errorf("chain name required")
	}

	s.mu.Lock()
	if _, exists := s.chains[chain.ChainID]; !exists && len(s.chains) >= MaxActiveChains {
		s.mu.Unlock()
		return fmt.Errorf("max %d active chains reached", MaxActiveChains)
	}
	now := time.Now()
	chain.CreatedAt = now
	chain.UpdatedAt = now
	s.chains[chain.ChainID] = chain
	s.mu.Unlock()

	s.publishEvent(ctx, "nfv.chain.created", map[string]interface{}{
		"chain_id":  chain.ChainID,
		"name":      chain.Name,
		"timestamp": now.UnixMilli(),
	})

	return nil
}

// DeleteChain deletes a chain programmatically.
func (s *Service) DeleteChain(ctx context.Context, chainID int) error {
	s.mu.Lock()
	chain, ok := s.chains[chainID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("chain not found: %d", chainID)
	}
	delete(s.chains, chainID)
	delete(s.chainStats, chainID)
	s.mu.Unlock()

	s.publishEvent(ctx, "nfv.chain.deleted", map[string]interface{}{
		"chain_id":  chainID,
		"name":      chain.Name,
		"timestamp": time.Now().UnixMilli(),
	})

	return nil
}

// GetChain retrieves a chain by ID.
func (s *Service) GetChain(chainID int) (*ChainDefinition, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chain, ok := s.chains[chainID]
	return chain, ok
}

// ListChains returns all chain definitions.
func (s *Service) ListChains() []*ChainDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chains := make([]*ChainDefinition, 0, len(s.chains))
	for _, chain := range s.chains {
		chains = append(chains, chain)
	}
	return chains
}

// RegisterFunction registers a BPF function in the function registry.
func (s *Service) RegisterFunction(name string, progFD int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.functionRegistry[name] = &FunctionEntry{
		Name:     name,
		ProgFD:   progFD,
		LoadedAt: time.Now(),
	}
}

// ListFunctions returns all registered functions.
func (s *Service) ListFunctions() []*FunctionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	functions := make([]*FunctionEntry, 0, len(s.functionRegistry))
	for _, fn := range s.functionRegistry {
		functions = append(functions, fn)
	}
	return functions
}

// GetChainStats retrieves stats for a chain.
func (s *Service) GetChainStats(chainID int) (*ChainStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats, ok := s.chainStats[chainID]
	return stats, ok
}

// RecordStats records stats for a chain (used by telemetry ingestion).
func (s *Service) RecordStats(stats *ChainStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats.LastUpdated = time.Now()
	s.chainStats[stats.ChainID] = stats
}

// Stats returns service-level statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalPackets, totalDrops uint64
	for _, stats := range s.chainStats {
		totalPackets += stats.PacketsProcessed
		totalDrops += stats.Drops
	}

	return map[string]interface{}{
		"active_chains":        len(s.chains),
		"registered_functions": len(s.functionRegistry),
		"total_packets":        totalPackets,
		"total_drops":          totalDrops,
		"wotan_drops":          s.WotanDrops(),
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

// statsAggregationLoop aggregates chain stats every 10 seconds.
func (s *Service) statsAggregationLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.aggregateStats()
		}
	}
}

// aggregateStats aggregates chain statistics.
func (s *Service) aggregateStats() {
	s.mu.RLock()
	chainCount := len(s.chains)
	var totalPackets uint64
	for _, stats := range s.chainStats {
		totalPackets += stats.PacketsProcessed
	}
	s.mu.RUnlock()

	s.log.Debug().
		Int("chains", chainCount).
		Uint64("total_packets", totalPackets).
		Msg("stats aggregated")
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
		traceID = tid.(string)
	}
	if traceID == "" {
		traceID = fmt.Sprintf("nfv-%d", time.Now().UnixNano())
	}

	event["trace_id"] = traceID
	event["service"] = "nfv"

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
