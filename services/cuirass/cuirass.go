// Package cuirass provides control plane and IDP orchestration capabilities.
// Cuirass the Control Plane is the Kingdom's heart - managing all services.
// Implements service registration, health monitoring, and orchestration.
package cuirass

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
	"unheaded/pkg/logger"
)

// ServiceRegistration represents a registered service.
type ServiceRegistration struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Address     string            `json:"address"`
	Port        int               `json:"port"`
	Protocol    string            `json:"protocol"`
	HealthURL   string            `json:"health_url,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Status      ServiceStatus     `json:"status"`
	LastSeen    time.Time         `json:"last_seen"`
	RegisteredAt time.Time        `json:"registered_at"`
}

// ServiceStatus tracks service health.
type ServiceStatus string

const (
	StatusHealthy   ServiceStatus = "healthy"
	StatusUnhealthy ServiceStatus = "unhealthy"
	StatusUnknown   ServiceStatus = "unknown"
	StatusDraining  ServiceStatus = "draining"
)

// HealthCheck represents a health check result.
type HealthCheck struct {
	ServiceID   string    `json:"service_id"`
	Status      bool      `json:"status"`
	Latency     int64     `json:"latency_ms"`
	Message     string    `json:"message,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

// Cluster represents a managed cluster.
type Cluster struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Region   string            `json:"region"`
	Nodes    []Node            `json:"nodes"`
	Status   string            `json:"status"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Node represents a cluster node.
type Node struct {
	ID       string            `json:"id"`
	Address  string            `json:"address"`
	Role     string            `json:"role"` // master, worker
	Status   string            `json:"status"`
	Capacity NodeCapacity      `json:"capacity"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// NodeCapacity represents node resources.
type NodeCapacity struct {
	CPU     int   `json:"cpu_cores"`
	Memory  int64 `json:"memory_bytes"`
	Storage int64 `json:"storage_bytes"`
}

// Service is the main Cuirass control plane service.
type Service struct {
	log    *logger.Logger
	busboy *busboyClient.Client
	config *Config

	mu       sync.RWMutex
	services map[string]*ServiceRegistration
	clusters map[string]*Cluster
	health   map[string]*HealthCheck
}

// Config holds Cuirass service configuration.
type Config struct {
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	ServiceTTL          time.Duration `json:"service_ttl"`
	DeregisterAfter     time.Duration `json:"deregister_after"`
	BusboyTopic         string        `json:"busboy_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		HealthCheckInterval: 10 * time.Second,
		ServiceTTL:          30 * time.Second,
		DeregisterAfter:     60 * time.Second,
		BusboyTopic:         "cuirass.control",
	}
}

// NewService creates a new Cuirass control plane service.
func NewService(log *logger.Logger, busboy *busboyClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:      log,
		busboy:   busboy,
		config:   cfg,
		services: make(map[string]*ServiceRegistration),
		clusters: make(map[string]*Cluster),
		health:   make(map[string]*HealthCheck),
	}
}

// Start initializes the Cuirass service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Cuirass donned - control plane active")
	go s.healthCheckLoop(ctx)
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Cuirass removed - control plane suspended")
	return nil
}

// Register registers a new service.
func (s *Service) Register(ctx context.Context, svc *ServiceRegistration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if svc.ID == "" {
		svc.ID = fmt.Sprintf("%s-%d", svc.Name, time.Now().UnixNano())
	}

	svc.RegisteredAt = time.Now()
	svc.LastSeen = time.Now()
	svc.Status = StatusUnknown

	s.services[svc.ID] = svc

	s.log.Info().
		Str("id", svc.ID).
		Str("name", svc.Name).
		Str("address", svc.Address).
		Msg("Service registered")

	return nil
}

// Deregister removes a service.
func (s *Service) Deregister(ctx context.Context, serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.services[serviceID]; !ok {
		return fmt.Errorf("service not found: %s", serviceID)
	}

	delete(s.services, serviceID)
	delete(s.health, serviceID)

	s.log.Info().Str("id", serviceID).Msg("Service deregistered")
	return nil
}

// GetService retrieves a service by ID.
func (s *Service) GetService(serviceID string) (*ServiceRegistration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	svc, ok := s.services[serviceID]
	return svc, ok
}

// GetServiceByName retrieves services by name.
func (s *Service) GetServiceByName(name string) []*ServiceRegistration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*ServiceRegistration
	for _, svc := range s.services {
		if svc.Name == name && svc.Status == StatusHealthy {
			results = append(results, svc)
		}
	}
	return results
}

// ListServices returns all registered services.
func (s *Service) ListServices() []*ServiceRegistration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]*ServiceRegistration, 0, len(s.services))
	for _, svc := range s.services {
		results = append(results, svc)
	}
	return results
}

// Heartbeat updates service last seen time.
func (s *Service) Heartbeat(ctx context.Context, serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	svc, ok := s.services[serviceID]
	if !ok {
		return fmt.Errorf("service not found: %s", serviceID)
	}

	svc.LastSeen = time.Now()
	return nil
}

// RecordHealthCheck records a health check result.
func (s *Service) RecordHealthCheck(check *HealthCheck) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.health[check.ServiceID] = check

	svc, ok := s.services[check.ServiceID]
	if ok {
		if check.Status {
			svc.Status = StatusHealthy
		} else {
			svc.Status = StatusUnhealthy
		}
		svc.LastSeen = time.Now()
	}
}

// GetHealthCheck retrieves latest health check for a service.
func (s *Service) GetHealthCheck(serviceID string) (*HealthCheck, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	check, ok := s.health[serviceID]
	return check, ok
}

// RegisterCluster registers a new cluster.
func (s *Service) RegisterCluster(cluster *Cluster) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cluster.ID == "" {
		cluster.ID = fmt.Sprintf("cluster-%d", time.Now().UnixNano())
	}

	s.clusters[cluster.ID] = cluster
	s.log.Info().Str("id", cluster.ID).Str("name", cluster.Name).Msg("Cluster registered")
	return nil
}

// GetCluster retrieves a cluster by ID.
func (s *Service) GetCluster(clusterID string) (*Cluster, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cluster, ok := s.clusters[clusterID]
	return cluster, ok
}

// ListClusters returns all clusters.
func (s *Service) ListClusters() []*Cluster {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]*Cluster, 0, len(s.clusters))
	for _, c := range s.clusters {
		results = append(results, c)
	}
	return results
}

func (s *Service) healthCheckLoop(ctx context.Context) {
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

func (s *Service) runHealthChecks(ctx context.Context) {
	s.mu.RLock()
	services := make([]*ServiceRegistration, 0, len(s.services))
	for _, svc := range s.services {
		services = append(services, svc)
	}
	s.mu.RUnlock()

	for _, svc := range services {
		// Check for stale services
		if time.Since(svc.LastSeen) > s.config.DeregisterAfter {
			s.log.Warn().Str("id", svc.ID).Msg("Service stale, deregistering")
			s.Deregister(ctx, svc.ID)
			continue
		}

		// Mark as unhealthy if past TTL
		if time.Since(svc.LastSeen) > s.config.ServiceTTL {
			s.mu.Lock()
			svc.Status = StatusUnhealthy
			s.mu.Unlock()
		}
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statusCounts := make(map[string]int)
	for _, svc := range s.services {
		statusCounts[string(svc.Status)]++
	}

	return map[string]interface{}{
		"total_services": len(s.services),
		"total_clusters": len(s.clusters),
		"by_status":      statusCounts,
	}
}

// RegisterRoutes registers /health, /ready, and /metrics endpoints on the provided mux.
// Required by CLAUDE.md: every component must expose these endpoints.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "cuirass",
		})
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   true,
			"service": "cuirass",
		})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := s.Stats()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP cuirass_services_total Total registered services\n")
		fmt.Fprintf(w, "# TYPE cuirass_services_total gauge\n")
		fmt.Fprintf(w, "cuirass_services_total %v\n", stats["total_services"])
		fmt.Fprintf(w, "# HELP cuirass_clusters_total Total clusters\n")
		fmt.Fprintf(w, "# TYPE cuirass_clusters_total gauge\n")
		fmt.Fprintf(w, "cuirass_clusters_total %v\n", stats["total_clusters"])
	})
}
