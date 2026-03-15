// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package firewall provides firewall policy management and connection tracking
// for the Unheaded Kingdom. It aggregates conntrack statistics, manages policy
// rules, and publishes periodic snapshots to the Wotan message bus.
package firewall

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"
)

// Policy action constants.
const (
	ActionAllow = "ALLOW"
	ActionBlock = "BLOCK"
)

// Connection states tracked by conntrack.
const (
	StateNew         = "NEW"
	StateEstablished = "ESTABLISHED"
	StateClosing     = "CLOSING"
	StateClosed      = "CLOSED"
)

// Default timeout values (seconds).
const (
	DefaultTimeoutNew         = 30
	DefaultTimeoutEstablished = 600
	DefaultTimeoutClosing     = 10
)

// conntrackAggregationInterval is the window for conntrack stats snapshots.
const conntrackAggregationInterval = 5 * time.Second

// maxConnectionsPage is the maximum number of connections returned per page.
const maxConnectionsPage = 100

// PolicyRule defines a single firewall rule.
type PolicyRule struct {
	SrcService         string   `json:"src_service"`
	DstService         string   `json:"dst_service"`
	Ports              []uint16 `json:"ports"`
	Protocol           string   `json:"protocol"`
	Action             string   `json:"action"`
	RequiresEncryption bool     `json:"requires_encryption"`
}

// FirewallPolicy holds the complete firewall policy.
type FirewallPolicy struct {
	DefaultAction      string       `json:"default_action"`
	TimeoutNew         int          `json:"timeout_new"`
	TimeoutEstablished int          `json:"timeout_established"`
	TimeoutClosing     int          `json:"timeout_closing"`
	Rules              []PolicyRule `json:"rules"`
}

// ConntrackEntry represents a single tracked connection.
type ConntrackEntry struct {
	SrcIP       string    `json:"src_ip"`
	DstIP       string    `json:"dst_ip"`
	SrcPort     uint16    `json:"src_port"`
	DstPort     uint16    `json:"dst_port"`
	Protocol    string    `json:"protocol"`
	State       string    `json:"state"`
	PacketCount uint64    `json:"packet_count"`
	ByteCount   uint64    `json:"byte_count"`
	CreatedAt   time.Time `json:"created_at"`
	LastSeen    time.Time `json:"last_seen"`
}

// ConntrackStats holds aggregated conntrack statistics.
type ConntrackStats struct {
	TotalConnections int               `json:"total_connections"`
	ActiveByState    map[string]int    `json:"active_by_state"`
	TopSrcIPs        []IPCount         `json:"top_src_ips"`
	TopDstServices   []ServiceCount    `json:"top_dst_services"`
	BlockedCount     int64             `json:"blocked_count"`
	AllowedCount     int64             `json:"allowed_count"`
	Timestamp        time.Time         `json:"timestamp"`
}

// IPCount tracks connection count per IP.
type IPCount struct {
	IP    string `json:"ip"`
	Count int    `json:"count"`
}

// ServiceCount tracks connection count per service.
type ServiceCount struct {
	Service string `json:"service"`
	Count   int    `json:"count"`
}

// BlockStats holds block-rate and throughput information for the stats endpoint.
type BlockStats struct {
	BlockRate       float64 `json:"block_rate"`
	AllowRate       float64 `json:"allow_rate"`
	TotalThroughput int64   `json:"total_throughput_bytes"`
	CacheHits       int64   `json:"cache_hits"`
	CacheMisses     int64   `json:"cache_misses"`
}

// Service is the Firewall Policy Manager service.
type Service struct {
	log   *logger.Logger
	wotan *wotanClient.Client

	mu             sync.RWMutex
	policy         *FirewallPolicy
	connections    map[string]*ConntrackEntry
	conntrackStats *ConntrackStats

	// Counters
	blockedCount int64
	allowedCount int64
	cacheHits    int64
	cacheMisses  int64

	// Alert handling
	alertsCh   chan *wotanClient.Message
	wotanDrops int64

	// HTTP server
	server   *http.Server
	listener net.Listener
}

// NewService creates a new Firewall Policy Manager service.
func NewService(log *logger.Logger, wotan *wotanClient.Client) *Service {
	return &Service{
		log:   log,
		wotan: wotan,
		policy: &FirewallPolicy{
			DefaultAction:      ActionBlock,
			TimeoutNew:         DefaultTimeoutNew,
			TimeoutEstablished: DefaultTimeoutEstablished,
			TimeoutClosing:     DefaultTimeoutClosing,
			Rules:              make([]PolicyRule, 0),
		},
		connections: make(map[string]*ConntrackEntry),
		conntrackStats: &ConntrackStats{
			ActiveByState:  make(map[string]int),
			TopSrcIPs:      make([]IPCount, 0),
			TopDstServices: make([]ServiceCount, 0),
			Timestamp:      time.Now(),
		},
		alertsCh: make(chan *wotanClient.Message, 100),
	}
}

// Start initialises Wotan subscriptions, starts the HTTP server, and begins
// conntrack aggregation on 5-second windows.
func (s *Service) Start(ctx context.Context) error {
	// Subscribe to Wotan topics
	if s.wotan != nil {
		topics := []string{
			"firewall.events",
			"firewall.blocks",
			"firewall.conntrack",
			"alerts.critical",
		}
		for _, topic := range topics {
			if _, err := s.wotan.Subscribe(ctx, topic, "firewall-service"); err != nil {
				s.log.Warn().Str("topic", topic).Err(err).Msg("failed to subscribe to topic")
			} else {
				s.log.Info().Str("topic", topic).Msg("subscribed to topic")
			}
		}
		go s.listenForAlerts(ctx)
	} else {
		s.log.Info().Msg("Wotan not configured, skipping subscriptions")
	}

	// Start conntrack aggregation
	go s.aggregateConntrackStats(ctx)

	// Start HTTP server
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := ports.DefaultAddr(ports.Firewall)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("firewall: listen on %s: %w", addr, err)
	}
	s.listener = ln

	s.server = &http.Server{
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	s.log.Info().Str("addr", addr).Int("port", ports.Firewall).Msg("firewall service starting")

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("firewall HTTP server error")
		}
	}()

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("firewall service stopping")

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(ctx); err != nil {
			s.log.Warn().Err(err).Msg("firewall HTTP server shutdown error")
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
	mux.HandleFunc("/api/v1/connections", s.handleConnections)
	mux.HandleFunc("/api/v1/stats", s.handleStats)
	mux.HandleFunc("/api/v1/policy/reload", s.handlePolicyReload)
	mux.HandleFunc("/api/v1/policy", s.handlePolicy)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)
}

// handleConnections returns the top connections with pagination (offset/limit).
func (s *Service) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > maxConnectionsPage {
		limit = maxConnectionsPage
	}
	if offset < 0 {
		offset = 0
	}

	s.mu.RLock()
	all := make([]*ConntrackEntry, 0, len(s.connections))
	for _, entry := range s.connections {
		all = append(all, entry)
	}
	s.mu.RUnlock()

	// Sort by packet count descending (top talkers first).
	sort.Slice(all, func(i, j int) bool {
		return all[i].PacketCount > all[j].PacketCount
	})

	// Apply pagination.
	if offset >= len(all) {
		all = nil
	} else {
		end := offset + limit
		if end > len(all) {
			end = len(all)
		}
		all = all[offset:end]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connections": all,
		"offset":      offset,
		"limit":       limit,
		"total":       len(s.connections),
	})
}

// handleStats returns block rate, throughput, and cache statistics.
func (s *Service) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	blocked := atomic.LoadInt64(&s.blockedCount)
	allowed := atomic.LoadInt64(&s.allowedCount)
	total := blocked + allowed
	var blockRate, allowRate float64
	if total > 0 {
		blockRate = float64(blocked) / float64(total)
		allowRate = float64(allowed) / float64(total)
	}

	s.mu.RLock()
	var totalBytes int64
	for _, entry := range s.connections {
		totalBytes += int64(entry.ByteCount)
	}
	s.mu.RUnlock()

	stats := BlockStats{
		BlockRate:       blockRate,
		AllowRate:       allowRate,
		TotalThroughput: totalBytes,
		CacheHits:       atomic.LoadInt64(&s.cacheHits),
		CacheMisses:     atomic.LoadInt64(&s.cacheMisses),
	}
	writeJSON(w, http.StatusOK, stats)
}

// handlePolicyReload reloads the policy (triggered by POST).
func (s *Service) handlePolicyReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var newPolicy FirewallPolicy
	if err := json.NewDecoder(r.Body).Decode(&newPolicy); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid policy: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Validate
	if newPolicy.DefaultAction != ActionAllow && newPolicy.DefaultAction != ActionBlock {
		http.Error(w, `{"error":"default_action must be ALLOW or BLOCK"}`, http.StatusBadRequest)
		return
	}
	for i, rule := range newPolicy.Rules {
		if rule.Action != ActionAllow && rule.Action != ActionBlock {
			http.Error(w, fmt.Sprintf(`{"error":"rule %d has invalid action"}`, i), http.StatusBadRequest)
			return
		}
	}

	if newPolicy.TimeoutNew <= 0 {
		newPolicy.TimeoutNew = DefaultTimeoutNew
	}
	if newPolicy.TimeoutEstablished <= 0 {
		newPolicy.TimeoutEstablished = DefaultTimeoutEstablished
	}
	if newPolicy.TimeoutClosing <= 0 {
		newPolicy.TimeoutClosing = DefaultTimeoutClosing
	}

	s.mu.Lock()
	s.policy = &newPolicy
	s.mu.Unlock()

	s.log.Info().Int("rules", len(newPolicy.Rules)).Msg("firewall policy reloaded")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "reloaded",
		"rules":  len(newPolicy.Rules),
	})
}

// handlePolicy returns the current policy.
func (s *Service) handlePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	policy := s.policy
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, policy)
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
	blocked := atomic.LoadInt64(&s.blockedCount)
	allowed := atomic.LoadInt64(&s.allowedCount)
	drops := atomic.LoadInt64(&s.wotanDrops)

	s.mu.RLock()
	connCount := len(s.connections)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "# HELP unheaded_firewall_blocked_total Total blocked connections\n")
	fmt.Fprintf(w, "# TYPE unheaded_firewall_blocked_total counter\n")
	fmt.Fprintf(w, "unheaded_firewall_blocked_total %d\n", blocked)
	fmt.Fprintf(w, "# HELP unheaded_firewall_allowed_total Total allowed connections\n")
	fmt.Fprintf(w, "# TYPE unheaded_firewall_allowed_total counter\n")
	fmt.Fprintf(w, "unheaded_firewall_allowed_total %d\n", allowed)
	fmt.Fprintf(w, "# HELP unheaded_firewall_connections Active connections\n")
	fmt.Fprintf(w, "# TYPE unheaded_firewall_connections gauge\n")
	fmt.Fprintf(w, "unheaded_firewall_connections %d\n", connCount)
	fmt.Fprintf(w, "# HELP unheaded_firewall_wotan_drops_total Wotan degraded-mode drops\n")
	fmt.Fprintf(w, "# TYPE unheaded_firewall_wotan_drops_total counter\n")
	fmt.Fprintf(w, "unheaded_firewall_wotan_drops_total %d\n", drops)
}

// MatchRule evaluates whether a given connection matches a policy rule.
// Returns the action to take (ALLOW/BLOCK) based on the first matching rule,
// or the default policy action if no rule matches.
func (s *Service) MatchRule(srcService, dstService string, dstPort uint16, protocol string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rule := range s.policy.Rules {
		if rule.SrcService != "" && rule.SrcService != srcService {
			continue
		}
		if rule.DstService != "" && rule.DstService != dstService {
			continue
		}
		if rule.Protocol != "" && rule.Protocol != protocol {
			continue
		}
		if len(rule.Ports) > 0 {
			found := false
			for _, p := range rule.Ports {
				if p == dstPort {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		// Matched
		if rule.Action == ActionAllow {
			atomic.AddInt64(&s.allowedCount, 1)
		} else {
			atomic.AddInt64(&s.blockedCount, 1)
		}
		return rule.Action
	}

	// No matching rule: apply default action.
	if s.policy.DefaultAction == ActionAllow {
		atomic.AddInt64(&s.allowedCount, 1)
	} else {
		atomic.AddInt64(&s.blockedCount, 1)
	}
	return s.policy.DefaultAction
}

// AddConnection adds or updates a conntrack entry.
func (s *Service) AddConnection(entry *ConntrackEntry) {
	if entry == nil {
		return
	}
	key := connKey(entry.SrcIP, entry.DstIP, entry.SrcPort, entry.DstPort, entry.Protocol)
	s.mu.Lock()
	s.connections[key] = entry
	s.mu.Unlock()
}

// GetPolicy returns a copy of the current policy.
func (s *Service) GetPolicy() FirewallPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := *s.policy
	rules := make([]PolicyRule, len(p.Rules))
	copy(rules, p.Rules)
	p.Rules = rules
	return p
}

// SetPolicy atomically replaces the current policy.
func (s *Service) SetPolicy(policy *FirewallPolicy) {
	if policy == nil {
		return
	}
	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
}

// GetConntrackStats returns the latest aggregated conntrack stats.
func (s *Service) GetConntrackStats() ConntrackStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.conntrackStats
}

// aggregateConntrackStats runs a 5-second aggregation loop and publishes
// snapshots to the Wotan "firewall.conntrack" topic.
func (s *Service) aggregateConntrackStats(ctx context.Context) {
	ticker := time.NewTicker(conntrackAggregationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.computeConntrackStats(ctx)
		}
	}
}

// computeConntrackStats builds a stats snapshot from the current connections.
func (s *Service) computeConntrackStats(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := &ConntrackStats{
		TotalConnections: len(s.connections),
		ActiveByState:    make(map[string]int),
		BlockedCount:     atomic.LoadInt64(&s.blockedCount),
		AllowedCount:     atomic.LoadInt64(&s.allowedCount),
		Timestamp:        time.Now(),
	}

	srcIPCounts := make(map[string]int)
	dstSvcCounts := make(map[string]int)

	for _, entry := range s.connections {
		stats.ActiveByState[entry.State]++
		srcIPCounts[entry.SrcIP]++
		// Use DstIP:DstPort as a proxy for destination service.
		dstKey := fmt.Sprintf("%s:%d", entry.DstIP, entry.DstPort)
		dstSvcCounts[dstKey]++
	}

	// Top source IPs (top 10).
	stats.TopSrcIPs = topNIPs(srcIPCounts, 10)

	// Top destination services (top 10).
	stats.TopDstServices = topNServices(dstSvcCounts, 10)

	s.conntrackStats = stats

	// Publish to Wotan.
	if s.wotan != nil {
		payload, err := json.Marshal(map[string]interface{}{
			"event_type": "firewall.conntrack",
			"service":    "firewall",
			"stats":      stats,
			"timestamp":  time.Now().UnixMilli(),
		})
		if err == nil {
			pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := s.wotan.Publish(pubCtx, "firewall.conntrack", payload); err != nil {
				s.log.Warn().Err(err).Msg("failed to publish conntrack stats")
			}
		}
	} else {
		atomic.AddInt64(&s.wotanDrops, 1)
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
		Msg("firewall received critical alert - entering degraded mode check")
}

// connKey generates a unique key for a connection.
func connKey(srcIP, dstIP string, srcPort, dstPort uint16, protocol string) string {
	return fmt.Sprintf("%s:%d->%s:%d/%s", srcIP, srcPort, dstIP, dstPort, protocol)
}

// topNIPs returns the top N source IPs by connection count.
func topNIPs(counts map[string]int, n int) []IPCount {
	items := make([]IPCount, 0, len(counts))
	for ip, count := range counts {
		items = append(items, IPCount{IP: ip, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})
	if len(items) > n {
		items = items[:n]
	}
	return items
}

// topNServices returns the top N destination services by connection count.
func topNServices(counts map[string]int, n int) []ServiceCount {
	items := make([]ServiceCount, 0, len(counts))
	for svc, count := range counts {
		items = append(items, ServiceCount{Service: svc, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Count > items[j].Count
	})
	if len(items) > n {
		items = items[:n]
	}
	return items
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
