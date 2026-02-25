// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package pauldrons provides load balancing and traffic distribution.
// Pauldrons the Load Balancer is the Kingdom's shoulders - bearing the weight of traffic.
// Implements L4/L7 load balancing, Maglev hashing, and session persistence.
package pauldrons

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
)

// Backend represents an upstream server.
type Backend struct {
	ID          string          `json:"id"`
	Address     string          `json:"address"`
	Port        int             `json:"port"`
	Weight      int             `json:"weight"`
	MaxConns    int             `json:"max_conns,omitempty"`
	ActiveConns int64           `json:"active_conns"`
	Healthy     bool            `json:"healthy"`
	Status      BackendStatus   `json:"status"`
	Stats       BackendStats    `json:"stats"`
	LastChecked time.Time       `json:"last_checked"`
}

// BackendStatus tracks backend state.
type BackendStatus string

const (
	BackendUp       BackendStatus = "up"
	BackendDown     BackendStatus = "down"
	BackendDraining BackendStatus = "draining"
)

// BackendStats tracks backend performance.
type BackendStats struct {
	TotalRequests int64         `json:"total_requests"`
	TotalErrors   int64         `json:"total_errors"`
	AvgLatency    time.Duration `json:"avg_latency"`
	P99Latency    time.Duration `json:"p99_latency"`
}

// Pool represents a backend pool.
type Pool struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Backends  []*Backend        `json:"backends"`
	Algorithm LBAlgorithm       `json:"algorithm"`
	Config    PoolConfig        `json:"config"`
	Sticky    *SessionSticky    `json:"sticky,omitempty"`
	Index     uint64            // For round-robin
}

// PoolConfig holds pool settings.
type PoolConfig struct {
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	HealthCheckTimeout  time.Duration `json:"health_check_timeout"`
	ConnectionTimeout   time.Duration `json:"connection_timeout"`
	IdleTimeout         time.Duration `json:"idle_timeout"`
}

// LBAlgorithm defines load balancing algorithm.
type LBAlgorithm string

const (
	AlgoRoundRobin   LBAlgorithm = "round_robin"
	AlgoLeastConn    LBAlgorithm = "least_conn"
	AlgoRandom       LBAlgorithm = "random"
	AlgoIPHash       LBAlgorithm = "ip_hash"
	AlgoMaglev       LBAlgorithm = "maglev"
	AlgoWeighted     LBAlgorithm = "weighted"
)

// SessionSticky defines session persistence.
type SessionSticky struct {
	Enabled  bool          `json:"enabled"`
	Type     string        `json:"type"` // cookie, source_ip
	TTL      time.Duration `json:"ttl"`
	CookieName string      `json:"cookie_name,omitempty"`
}

// VirtualServer represents a frontend listener.
type VirtualServer struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Address  string     `json:"address"`
	Port     int        `json:"port"`
	Protocol string     `json:"protocol"` // tcp, http, https
	PoolID   string     `json:"pool_id"`
	Rules    []LBRule   `json:"rules,omitempty"`
}

// LBRule defines routing rules.
type LBRule struct {
	Name      string            `json:"name"`
	Match     RuleMatch         `json:"match"`
	PoolID    string            `json:"pool_id"`
	Priority  int               `json:"priority"`
}

// RuleMatch defines match conditions.
type RuleMatch struct {
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Host    string            `json:"host,omitempty"`
}

// Service is the main Pauldrons load balancer service.
type Service struct {
	log    *logger.Logger
	wotan *wotanClient.Client
	config *Config

	mu             sync.RWMutex
	pools          map[string]*Pool
	virtualServers map[string]*VirtualServer
	sessions       map[string]string // Session ID -> Backend ID
	maglevTable    map[string][]int  // Pool ID -> lookup table
}

// Config holds Pauldrons service configuration.
type Config struct {
	DefaultAlgorithm    LBAlgorithm   `json:"default_algorithm"`
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	SessionTTL          time.Duration `json:"session_ttl"`
	MaglevTableSize     int           `json:"maglev_table_size"`
	WotanTopic         string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DefaultAlgorithm:    AlgoRoundRobin,
		HealthCheckInterval: 5 * time.Second,
		SessionTTL:          30 * time.Minute,
		MaglevTableSize:     65537, // Prime number for better distribution
		WotanTopic:         "pauldrons.lb",
	}
}

// NewService creates a new Pauldrons load balancer service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:            log,
		wotan:         wotan,
		config:         cfg,
		pools:          make(map[string]*Pool),
		virtualServers: make(map[string]*VirtualServer),
		sessions:       make(map[string]string),
		maglevTable:    make(map[string][]int),
	}
}

// Start initializes the Pauldrons service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Pauldrons equipped - load balancer active")
	go s.healthCheckLoop(ctx)
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Pauldrons removed - load balancer suspended")
	return nil
}

// CreatePool creates a new backend pool.
func (s *Service) CreatePool(pool *Pool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pool.ID == "" {
		pool.ID = fmt.Sprintf("pool-%d", time.Now().UnixNano())
	}

	if pool.Algorithm == "" {
		pool.Algorithm = s.config.DefaultAlgorithm
	}

	s.pools[pool.ID] = pool

	// Build Maglev table if using that algorithm
	if pool.Algorithm == AlgoMaglev {
		s.buildMaglevTable(pool)
	}

	s.log.Info().Str("id", pool.ID).Str("name", pool.Name).Msg("Pool created")
	return nil
}

// AddBackend adds a backend to a pool.
func (s *Service) AddBackend(poolID string, backend *Backend) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pool, ok := s.pools[poolID]
	if !ok {
		return fmt.Errorf("pool not found: %s", poolID)
	}

	if backend.ID == "" {
		backend.ID = fmt.Sprintf("%s:%d", backend.Address, backend.Port)
	}
	backend.Healthy = true
	backend.Status = BackendUp

	pool.Backends = append(pool.Backends, backend)

	// Rebuild Maglev table
	if pool.Algorithm == AlgoMaglev {
		s.buildMaglevTable(pool)
	}

	s.log.Info().Str("pool", poolID).Str("backend", backend.ID).Msg("Backend added")
	return nil
}

// RemoveBackend removes a backend from a pool.
func (s *Service) RemoveBackend(poolID, backendID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pool, ok := s.pools[poolID]
	if !ok {
		return fmt.Errorf("pool not found: %s", poolID)
	}

	for i, b := range pool.Backends {
		if b.ID == backendID {
			pool.Backends = append(pool.Backends[:i], pool.Backends[i+1:]...)
			s.log.Info().Str("pool", poolID).Str("backend", backendID).Msg("Backend removed")

			if pool.Algorithm == AlgoMaglev {
				s.buildMaglevTable(pool)
			}
			return nil
		}
	}

	return fmt.Errorf("backend not found: %s", backendID)
}

// SelectBackend picks a backend for the request.
func (s *Service) SelectBackend(poolID string, key string) (*Backend, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pool, ok := s.pools[poolID]
	if !ok {
		return nil, fmt.Errorf("pool not found: %s", poolID)
	}

	// Check session persistence
	if pool.Sticky != nil && pool.Sticky.Enabled {
		if backendID, ok := s.sessions[key]; ok {
			for _, b := range pool.Backends {
				if b.ID == backendID && b.Healthy {
					return b, nil
				}
			}
		}
	}

	// Get healthy backends
	var healthy []*Backend
	for _, b := range pool.Backends {
		if b.Healthy {
			healthy = append(healthy, b)
		}
	}

	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy backends in pool: %s", poolID)
	}

	var selected *Backend

	switch pool.Algorithm {
	case AlgoRoundRobin:
		idx := atomic.AddUint64(&pool.Index, 1)
		selected = healthy[int(idx)%len(healthy)]

	case AlgoLeastConn:
		minConns := int64(^uint64(0) >> 1)
		for _, b := range healthy {
			if atomic.LoadInt64(&b.ActiveConns) < minConns {
				minConns = atomic.LoadInt64(&b.ActiveConns)
				selected = b
			}
		}

	case AlgoIPHash:
		h := fnv.New32a()
		h.Write([]byte(key))
		idx := h.Sum32() % uint32(len(healthy))
		selected = healthy[idx]

	case AlgoMaglev:
		table := s.maglevTable[poolID]
		if len(table) > 0 {
			h := fnv.New32a()
			h.Write([]byte(key))
			idx := table[int(h.Sum32())%len(table)]
			if idx < len(healthy) {
				selected = healthy[idx]
			}
		}
		if selected == nil {
			selected = healthy[0]
		}

	case AlgoWeighted:
		totalWeight := 0
		for _, b := range healthy {
			totalWeight += b.Weight
		}
		if totalWeight == 0 {
			selected = healthy[0]
		} else {
			h := fnv.New32a()
			h.Write([]byte(key))
			target := int(h.Sum32()) % totalWeight
			for _, b := range healthy {
				target -= b.Weight
				if target < 0 {
					selected = b
					break
				}
			}
		}

	default:
		selected = healthy[0]
	}

	// Record session if sticky enabled
	if pool.Sticky != nil && pool.Sticky.Enabled {
		s.sessions[key] = selected.ID
	}

	return selected, nil
}

// GetPool returns a pool by ID.
func (s *Service) GetPool(poolID string) (*Pool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pool, ok := s.pools[poolID]
	return pool, ok
}

// ListPools returns all pools.
func (s *Service) ListPools() []*Pool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pools := make([]*Pool, 0, len(s.pools))
	for _, p := range s.pools {
		pools = append(pools, p)
	}
	return pools
}

// CreateVirtualServer creates a frontend.
func (s *Service) CreateVirtualServer(vs *VirtualServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if vs.ID == "" {
		vs.ID = fmt.Sprintf("vs-%d", time.Now().UnixNano())
	}

	s.virtualServers[vs.ID] = vs
	s.log.Info().Str("id", vs.ID).Str("address", vs.Address).Int("port", vs.Port).Msg("VirtualServer created")
	return nil
}

// GetVirtualServer returns a virtual server by ID.
func (s *Service) GetVirtualServer(vsID string) (*VirtualServer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vs, ok := s.virtualServers[vsID]
	return vs, ok
}

func (s *Service) buildMaglevTable(pool *Pool) {
	if len(pool.Backends) == 0 {
		s.maglevTable[pool.ID] = nil
		return
	}

	size := s.config.MaglevTableSize
	table := make([]int, size)

	// Simplified Maglev - real implementation would be more sophisticated
	for i := 0; i < size; i++ {
		table[i] = i % len(pool.Backends)
	}

	s.maglevTable[pool.ID] = table
}

func (s *Service) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runHealthChecks()
		}
	}
}

func (s *Service) runHealthChecks() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pool := range s.pools {
		for _, backend := range pool.Backends {
			backend.LastChecked = time.Now()
			// In production, would actually check health
		}
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalBackends := 0
	healthyBackends := 0
	for _, pool := range s.pools {
		for _, b := range pool.Backends {
			totalBackends++
			if b.Healthy {
				healthyBackends++
			}
		}
	}

	return map[string]interface{}{
		"total_pools":        len(s.pools),
		"total_backends":     totalBackends,
		"healthy_backends":   healthyBackends,
		"virtual_servers":    len(s.virtualServers),
		"active_sessions":    len(s.sessions),
	}
}

// RegisterRoutes registers /health, /ready, and /metrics endpoints on the provided mux.
// Required by CLAUDE.md: every component must expose these endpoints.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "pauldrons",
		})
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   true,
			"service": "pauldrons",
		})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := s.Stats()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP pauldrons_pools_total Total backend pools\n")
		fmt.Fprintf(w, "# TYPE pauldrons_pools_total gauge\n")
		fmt.Fprintf(w, "pauldrons_pools_total %v\n", stats["total_pools"])
		fmt.Fprintf(w, "# HELP pauldrons_backends_total Total backends\n")
		fmt.Fprintf(w, "# TYPE pauldrons_backends_total gauge\n")
		fmt.Fprintf(w, "pauldrons_backends_total %v\n", stats["total_backends"])
		fmt.Fprintf(w, "# HELP pauldrons_healthy_backends Healthy backends\n")
		fmt.Fprintf(w, "# TYPE pauldrons_healthy_backends gauge\n")
		fmt.Fprintf(w, "pauldrons_healthy_backends %v\n", stats["healthy_backends"])
		fmt.Fprintf(w, "# HELP pauldrons_active_sessions Active sessions\n")
		fmt.Fprintf(w, "# TYPE pauldrons_active_sessions gauge\n")
		fmt.Fprintf(w, "pauldrons_active_sessions %v\n", stats["active_sessions"])
	})
}
