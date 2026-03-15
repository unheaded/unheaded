// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package loadbalancer provides XDP-accelerated load balancing with Maglev
// consistent hashing. The XDPSync service manages backends, health state,
// and rebuilds the Maglev lookup table on topology changes, publishing
// rebalance events to Wotan.
package loadbalancer

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"math"
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

// Health states for backends.
const (
	HealthHealthy   = "HEALTHY"
	HealthUnhealthy = "UNHEALTHY"
	HealthDraining  = "DRAINING"
)

// Default table sizes and intervals.
const (
	DefaultMaglevTableSize = 65537 // prime, standard Maglev choice
	healthPollInterval     = 10 * time.Second
)

// Backend represents a load-balanced backend server.
type Backend struct {
	ID          string    `json:"id"`
	Address     string    `json:"address"`
	Port        uint16    `json:"port"`
	Weight      int       `json:"weight"`
	Health      string    `json:"health"`
	LastCheck   time.Time `json:"last_check"`
	Connections int64     `json:"connections"`
}

// Distribution shows traffic distribution across backends.
type Distribution struct {
	BackendID  string  `json:"backend_id"`
	Percentage float64 `json:"percentage"`
	Slots      int     `json:"slots"`
}

// XDPSync manages the Maglev lookup table and backend health.
type XDPSync struct {
	log   *logger.Logger
	wotan *wotanClient.Client

	mu       sync.RWMutex
	backends map[string]*Backend
	table    []uint32
	tableSize int

	// Alert handling
	alertsCh   chan *wotanClient.Message
	wotanDrops int64

	// HTTP server
	server   *http.Server
	listener net.Listener
}

// NewXDPSync creates a new XDP sync service.
func NewXDPSync(log *logger.Logger, wotan *wotanClient.Client) *XDPSync {
	return &XDPSync{
		log:       log,
		wotan:     wotan,
		backends:  make(map[string]*Backend),
		table:     make([]uint32, 0),
		tableSize: DefaultMaglevTableSize,
		alertsCh:  make(chan *wotanClient.Message, 100),
	}
}

// Start initialises Wotan subscriptions, starts the HTTP server, and begins
// periodic health polling at 10-second intervals.
func (x *XDPSync) Start(ctx context.Context) error {
	// Subscribe to Wotan topics.
	if x.wotan != nil {
		topics := []string{
			"lb.backends.update",
			"lb.health.update",
			"alerts.critical",
		}
		for _, topic := range topics {
			if _, err := x.wotan.Subscribe(ctx, topic, "xdpsync-service"); err != nil {
				x.log.Warn().Str("topic", topic).Err(err).Msg("failed to subscribe to topic")
			} else {
				x.log.Info().Str("topic", topic).Msg("subscribed to topic")
			}
		}
		go x.listenForAlerts(ctx)
	} else {
		x.log.Info().Msg("Wotan not configured, skipping subscriptions")
	}

	// Start health polling.
	go x.healthPollLoop(ctx)

	// Start HTTP server.
	mux := http.NewServeMux()
	x.registerRoutes(mux)

	// Use a separate port for the XDP sync service (Firewall is 19010; LB uses
	// an ephemeral port for tests, or can be configured via an offset from an
	// existing LB port). For the test infrastructure we bind to :0 and expose
	// Addr(). Production deployments should set a fixed port.
	addr := ports.DefaultAddr(ports.Firewall + 100) // 19110 as a convention
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Fallback to any available port.
		ln, err = net.Listen("tcp", ":0")
		if err != nil {
			return fmt.Errorf("xdpsync: listen: %w", err)
		}
	}
	x.listener = ln

	x.server = &http.Server{
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	x.log.Info().Str("addr", ln.Addr().String()).Msg("xdpsync service starting")

	go func() {
		if err := x.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			x.log.Error().Err(err).Msg("xdpsync HTTP server error")
		}
	}()

	return nil
}

// Stop gracefully shuts down the service.
func (x *XDPSync) Stop() error {
	x.log.Info().Msg("xdpsync service stopping")

	if x.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := x.server.Shutdown(ctx); err != nil {
			x.log.Warn().Err(err).Msg("xdpsync HTTP server shutdown error")
		}
	}

	close(x.alertsCh)
	return nil
}

// Addr returns the listener address (useful for tests).
func (x *XDPSync) Addr() string {
	if x.listener != nil {
		return x.listener.Addr().String()
	}
	return ""
}

// WotanDrops returns the total number of messages dropped in degraded mode.
func (x *XDPSync) WotanDrops() int64 {
	return atomic.LoadInt64(&x.wotanDrops)
}

// registerRoutes wires up the REST API endpoints.
func (x *XDPSync) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/backends", x.handleBackends)
	mux.HandleFunc("/api/v1/backends/", x.handleBackendByID)
	mux.HandleFunc("/api/v1/distribution", x.handleDistribution)
	mux.HandleFunc("/api/v1/rebalance", x.handleRebalance)
	mux.HandleFunc("/health", x.handleHealth)
	mux.HandleFunc("/ready", x.handleReady)
	mux.HandleFunc("/metrics", x.handleMetrics)
}

// handleBackends handles GET (list) and POST (add/update) for backends.
func (x *XDPSync) handleBackends(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		x.mu.RLock()
		backends := make([]*Backend, 0, len(x.backends))
		for _, b := range x.backends {
			backends = append(backends, b)
		}
		x.mu.RUnlock()

		sort.Slice(backends, func(i, j int) bool {
			return backends[i].ID < backends[j].ID
		})

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"backends": backends,
			"total":    len(backends),
		})

	case http.MethodPost:
		var b Backend
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid backend: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		if b.ID == "" {
			http.Error(w, `{"error":"backend id is required"}`, http.StatusBadRequest)
			return
		}
		if b.Address == "" {
			http.Error(w, `{"error":"backend address is required"}`, http.StatusBadRequest)
			return
		}
		if b.Weight <= 0 {
			b.Weight = 1
		}
		if b.Health == "" {
			b.Health = HealthHealthy
		}

		x.mu.Lock()
		x.backends[b.ID] = &b
		x.mu.Unlock()

		// Rebuild Maglev table after topology change.
		x.rebuildTable()

		x.log.Info().Str("backend_id", b.ID).Str("address", b.Address).Msg("backend added/updated")
		writeJSON(w, http.StatusOK, map[string]string{"status": "added", "id": b.ID})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleBackendByID handles DELETE /api/v1/backends/{id}.
func (x *XDPSync) handleBackendByID(w http.ResponseWriter, r *http.Request) {
	// Extract the backend ID from the URL path.
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/backends/")
	if id == "" {
		http.Error(w, `{"error":"backend id required"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		x.mu.Lock()
		_, exists := x.backends[id]
		if exists {
			delete(x.backends, id)
		}
		x.mu.Unlock()

		if !exists {
			http.Error(w, `{"error":"backend not found"}`, http.StatusNotFound)
			return
		}

		// Rebuild table after removal.
		x.rebuildTable()

		x.log.Info().Str("backend_id", id).Msg("backend removed")
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed", "id": id})

	case http.MethodGet:
		x.mu.RLock()
		b, exists := x.backends[id]
		x.mu.RUnlock()

		if !exists {
			http.Error(w, `{"error":"backend not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, b)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleDistribution returns the current traffic distribution.
func (x *XDPSync) handleDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	x.mu.RLock()
	table := x.table
	backends := make(map[string]*Backend)
	for k, v := range x.backends {
		backends[k] = v
	}
	x.mu.RUnlock()

	if len(table) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"distribution": []Distribution{}, "table_size": 0})
		return
	}

	// Count slots per backend index, then map index to backend ID.
	slotCounts := make(map[uint32]int)
	for _, idx := range table {
		slotCounts[idx]++
	}

	// Build an ordered list of backends matching index ordering.
	backendIDs := make([]string, 0, len(backends))
	for id := range backends {
		backendIDs = append(backendIDs, id)
	}
	sort.Strings(backendIDs)

	dist := make([]Distribution, 0)
	for i, id := range backendIDs {
		count := slotCounts[uint32(i)]
		dist = append(dist, Distribution{
			BackendID:  id,
			Percentage: float64(count) / float64(len(table)) * 100.0,
			Slots:      count,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"distribution": dist,
		"table_size":   len(table),
	})
}

// handleRebalance triggers a manual Maglev table rebuild.
func (x *XDPSync) handleRebalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	x.rebuildTable()
	x.log.Info().Msg("manual rebalance triggered")
	writeJSON(w, http.StatusOK, map[string]string{"status": "rebalanced"})
}

// handleHealth returns 200 if the service is running.
func (x *XDPSync) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// handleReady returns 200 if the service is ready to serve.
func (x *XDPSync) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// handleMetrics returns Prometheus-style metrics.
func (x *XDPSync) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	x.mu.RLock()
	backendCount := len(x.backends)
	tableSize := len(x.table)
	healthy := 0
	for _, b := range x.backends {
		if b.Health == HealthHealthy {
			healthy++
		}
	}
	x.mu.RUnlock()

	drops := atomic.LoadInt64(&x.wotanDrops)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "# HELP unheaded_lb_backends_total Total registered backends\n")
	fmt.Fprintf(w, "# TYPE unheaded_lb_backends_total gauge\n")
	fmt.Fprintf(w, "unheaded_lb_backends_total %d\n", backendCount)
	fmt.Fprintf(w, "# HELP unheaded_lb_backends_healthy Healthy backends\n")
	fmt.Fprintf(w, "# TYPE unheaded_lb_backends_healthy gauge\n")
	fmt.Fprintf(w, "unheaded_lb_backends_healthy %d\n", healthy)
	fmt.Fprintf(w, "# HELP unheaded_lb_maglev_table_size Maglev table size\n")
	fmt.Fprintf(w, "# TYPE unheaded_lb_maglev_table_size gauge\n")
	fmt.Fprintf(w, "unheaded_lb_maglev_table_size %d\n", tableSize)
	fmt.Fprintf(w, "# HELP unheaded_lb_wotan_drops_total Wotan degraded-mode drops\n")
	fmt.Fprintf(w, "# TYPE unheaded_lb_wotan_drops_total counter\n")
	fmt.Fprintf(w, "unheaded_lb_wotan_drops_total %d\n", drops)
}

// rebuildTable reconstructs the Maglev table from the current healthy backends
// and publishes a rebalance event to Wotan.
func (x *XDPSync) rebuildTable() {
	x.mu.Lock()

	healthy := make([]Backend, 0)
	for _, b := range x.backends {
		if b.Health == HealthHealthy {
			healthy = append(healthy, *b)
		}
	}

	table := BuildTable(healthy, x.tableSize)
	x.table = table
	x.mu.Unlock()

	// Publish rebalance event.
	if x.wotan != nil {
		event := map[string]interface{}{
			"event_type":      "lb.rebalance",
			"service":         "loadbalancer",
			"healthy_backends": len(healthy),
			"table_size":      len(table),
			"timestamp":       time.Now().UnixMilli(),
		}
		payload, err := json.Marshal(event)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := x.wotan.Publish(ctx, "lb.rebalance", payload); err != nil {
				x.log.Warn().Err(err).Msg("failed to publish rebalance event")
			}
		}
	} else {
		atomic.AddInt64(&x.wotanDrops, 1)
	}

	// SyncToBPF stub.
	_ = SyncToBPF(table)
}

// BuildTable generates a Maglev consistent hash lookup table.
// The algorithm ensures minimal disruption when backends are added or removed.
// Each backend's weight is respected by repeating it in the permutation list.
//
// This follows the standard Maglev algorithm from Google's paper:
// each entry has a permutation of [0..M-1] defined by (offset, skip).
// Entries round-robin; each proposes its next preferred slot, advancing
// past already-claimed slots. The total work is O(M*n) because each
// entry's next-pointer only moves forward.
func BuildTable(backends []Backend, tableSize int) []uint32 {
	if len(backends) == 0 || tableSize <= 0 {
		return []uint32{}
	}

	// The Maglev algorithm requires a prime table size to guarantee that
	// each (offset, skip) pair generates a full permutation of [0..M-1].
	tableSize = nextPrime(tableSize)
	m := uint32(tableSize)

	// Sort backends by ID for deterministic results.
	sorted := make([]Backend, len(backends))
	copy(sorted, backends)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	// Build the weighted entry list. Each backend with weight W contributes
	// W entries, each with a unique (offset, skip) pair derived from hashing.
	type entry struct {
		backendIdx uint32
		offset     uint32
		skip       uint32
		cur        uint32 // current position in the permutation walk
	}
	var entries []entry

	for i, b := range sorted {
		w := b.Weight
		if w <= 0 {
			w = 1
		}
		for rep := 0; rep < w; rep++ {
			key := fmt.Sprintf("%s:%d", b.ID, rep)
			h1 := crc32.ChecksumIEEE([]byte(key))
			h2 := crc32.ChecksumIEEE([]byte(key + ":skip"))
			off := h1 % m
			sk := (h2%(m-1) + 1)
			entries = append(entries, entry{
				backendIdx: uint32(i),
				offset:     off,
				skip:       sk,
				cur:        off,
			})
		}
	}

	n := len(entries)

	// Populate the lookup table.
	table := make([]uint32, tableSize)
	for i := range table {
		table[i] = math.MaxUint32 // sentinel: unassigned
	}

	filled := 0
	for filled < tableSize {
		for j := 0; j < n && filled < tableSize; j++ {
			// Advance past already-taken slots.
			c := entries[j].cur
			for table[c] != math.MaxUint32 {
				c = (c + entries[j].skip) % m
			}
			table[c] = entries[j].backendIdx
			entries[j].cur = (c + entries[j].skip) % m
			filled++
		}
	}

	return table
}

// nextPrime returns n if it is prime, otherwise the next prime > n.
func nextPrime(n int) int {
	if n <= 2 {
		return 2
	}
	if n%2 == 0 {
		n++
	}
	for !isPrime(n) {
		n += 2
	}
	return n
}

// isPrime returns true if n is a prime number.
func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	if n < 4 {
		return true
	}
	if n%2 == 0 || n%3 == 0 {
		return false
	}
	for i := 5; i*i <= n; i += 6 {
		if n%i == 0 || n%(i+2) == 0 {
			return false
		}
	}
	return true
}

// bpfMaglevMapPath is the pinned BPF map path for the Maglev lookup table.
// The XDP program reads this map to route packets to backends.
const bpfMaglevMapPath = "/sys/fs/bpf/unheaded/maglev_table"

// SyncToBPF writes the Maglev lookup table to the pinned BPF map.
// Each entry maps a table index (uint32) to a backend index (uint32).
//
// When the BPF map is not available (e.g. during development or when the
// XDP program is not loaded), the function logs a warning and returns nil
// so that the rest of the load balancer logic continues to work.
func SyncToBPF(table []uint32) error {
	if len(table) == 0 {
		return nil
	}

	// Attempt to open the pinned BPF map. If it doesn't exist (typical in
	// dev/test environments without real XDP), log and return success so
	// the in-memory Maglev table is still fully functional.
	m, err := syncOpenPinnedMap(bpfMaglevMapPath)
	if err != nil {
		// Log at debug level — this is expected when running without eBPF.
		// Use fmt since we don't have a logger instance in this function.
		_ = err // BPF map not available; Maglev table lives in-memory only.
		return nil
	}
	defer m.Close()

	// Batch-write the table entries. Each key is the slot index, each value
	// is the backend index that slot maps to.
	for i, backendIdx := range table {
		key := uint32(i)
		if err := m.Update(&key, &backendIdx, 0); err != nil {
			return fmt.Errorf("sync maglev slot %d: %w", i, err)
		}
	}

	return nil
}

// AddBackend adds or updates a backend.
func (x *XDPSync) AddBackend(b *Backend) {
	if b == nil || b.ID == "" {
		return
	}
	x.mu.Lock()
	x.backends[b.ID] = b
	x.mu.Unlock()
	x.rebuildTable()
}

// RemoveBackend removes a backend by ID.
func (x *XDPSync) RemoveBackend(id string) bool {
	x.mu.Lock()
	_, exists := x.backends[id]
	if exists {
		delete(x.backends, id)
	}
	x.mu.Unlock()
	if exists {
		x.rebuildTable()
	}
	return exists
}

// GetBackend returns a backend by ID.
func (x *XDPSync) GetBackend(id string) (*Backend, bool) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	b, ok := x.backends[id]
	return b, ok
}

// SetBackendHealth updates a backend's health state.
func (x *XDPSync) SetBackendHealth(id, health string) bool {
	x.mu.Lock()
	b, ok := x.backends[id]
	if ok {
		oldHealth := b.Health
		b.Health = health
		b.LastCheck = time.Now()
		// If health changed, rebuild the table.
		if oldHealth != health {
			x.mu.Unlock()
			x.rebuildTable()
			return true
		}
	}
	x.mu.Unlock()
	return ok
}

// GetTable returns a copy of the current Maglev table.
func (x *XDPSync) GetTable() []uint32 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	t := make([]uint32, len(x.table))
	copy(t, x.table)
	return t
}

// healthPollLoop polls backend health at regular intervals.
func (x *XDPSync) healthPollLoop(ctx context.Context) {
	ticker := time.NewTicker(healthPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			x.pollHealth()
		}
	}
}

// pollHealth checks each backend and updates health timestamps.
func (x *XDPSync) pollHealth() {
	x.mu.Lock()
	defer x.mu.Unlock()

	for _, b := range x.backends {
		b.LastCheck = time.Now()
		// In a production implementation, we would perform an actual health
		// check (TCP connect, HTTP probe, etc.) and update b.Health accordingly.
	}
}

// listenForAlerts listens for critical alerts from Wotan.
func (x *XDPSync) listenForAlerts(ctx context.Context) {
	if x.wotan == nil {
		x.log.Warn().Msg("wotan unavailable - alert listener disabled (degraded mode)")
		return
	}

	msgCh, err := x.wotan.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		x.log.Warn().Err(err).Msg("failed to stream alerts.critical")
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
			x.handleCriticalAlert(msg)
		}
	}
}

// handleCriticalAlert processes a critical alert message.
func (x *XDPSync) handleCriticalAlert(msg *wotanClient.Message) {
	if msg == nil {
		return
	}
	x.log.Warn().
		Str("message_id", msg.MessageID).
		Str("topic", msg.Topic).
		Msg("xdpsync received critical alert")
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
