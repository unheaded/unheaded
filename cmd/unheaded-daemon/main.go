// Package main is the entry point for the unheaded-daemon
// THE CUIRASS - The Core Heart of the Unheaded Kingdom
//
// "The armor protects the Knight. The Knight serves the Kingdom.
//  The Kingdom endures forever." - The Sacred Chronicles
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
	"unheaded/pkg/logger"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// THE CORE HEART BANNER
// ============================================================================

const banner = `
╔══════════════════════════════════════════════════════════════════════════════╗
║                                                                              ║
║   ⚔️  THE CUIRASS AWAKENS - Core Heart of the Unheaded Kingdom  ⚔️           ║
║                                                                              ║
║   "The armor protects the Knight. The Knight serves the Kingdom."            ║
║                                                                              ║
║   🛡️  Control Plane Active                                                   ║
║   🔮  Whispering Void (eBPF) Loading...                                      ║
║   📦  Citadel Orchestration Ready                                            ║
║   💎  Crystal Grotto (Secrets) Sealed                                        ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝
`

// ============================================================================
// VERSION INFO
// ============================================================================

var (
	version   = "0.1.0-alpha"
	gitCommit = "dev"
	buildTime = "unknown"
)

// ============================================================================
// DAEMON STRUCTURE
// ============================================================================

// Busboy topic constants
const (
	TopicCuirassDrift   = "cuirass.drift"
	TopicCuirassMetrics = "cuirass.metrics"
)

// Daemon represents the unheaded-daemon control plane
type Daemon struct {
	mu     sync.RWMutex
	config *Config
	log    *logger.Logger

	// State management
	stateManager *StateManager

	// Busboy client for event publishing
	busboyClient *busboyClient.Client
	busboyReady  bool
	busboyMu     sync.RWMutex

	// LXD client for container operations
	lxdClient LXDClient

	// eBPF loader (mock for now)
	// ebpfLoader ebpf.Loader

	// HTTP server
	httpServer *http.Server

	// Shutdown coordination
	shutdown chan struct{}
	wg       sync.WaitGroup
}

// ============================================================================
// LXD CLIENT INTERFACE AND MOCK IMPLEMENTATION
// ============================================================================

// LXDClient defines the interface for LXD container operations
type LXDClient interface {
	// CreateContainer creates a new container from the spec
	CreateContainer(ctx context.Context, spec *ContainerSpec) error
	// StartContainer starts a stopped container
	StartContainer(ctx context.Context, name string) error
	// StopContainer stops a running container
	StopContainer(ctx context.Context, name string, force bool) error
	// RestartContainer restarts a container
	RestartContainer(ctx context.Context, name string) error
	// DeleteContainer removes a container
	DeleteContainer(ctx context.Context, name string, force bool) error
	// GetContainer returns a container's state
	GetContainer(name string) (*ContainerState, bool)
	// ListContainers returns all containers
	ListContainers() map[string]*ContainerState
}

// MockLXDClient is a mock implementation of LXDClient for testing
type MockLXDClient struct {
	mu         sync.RWMutex
	containers map[string]*ContainerState
	log        *logger.Logger
}

// NewMockLXDClient creates a new mock LXD client
func NewMockLXDClient(log *logger.Logger) *MockLXDClient {
	return &MockLXDClient{
		containers: make(map[string]*ContainerState),
		log:        log,
	}
}

// CreateContainer creates a mock container
func (m *MockLXDClient) CreateContainer(ctx context.Context, spec *ContainerSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.containers[spec.ID]; exists {
		return fmt.Errorf("container %s already exists", spec.ID)
	}

	m.containers[spec.ID] = &ContainerState{
		ID:       spec.ID,
		Status:   "stopped",
		Health:   "starting",
		CPUUsage: 0,
	}

	m.log.Info().
		Str("id", spec.ID).
		Str("name", spec.Name).
		Str("image", spec.Image).
		Msg("MockLXD: Created container")
	return nil
}

// StartContainer starts a mock container
func (m *MockLXDClient) StartContainer(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	container, exists := m.containers[name]
	if !exists {
		return fmt.Errorf("container %s not found", name)
	}

	container.Status = "running"
	container.Health = "healthy"
	m.log.Info().Str("id", name).Msg("MockLXD: Started container")
	return nil
}

// StopContainer stops a mock container
func (m *MockLXDClient) StopContainer(ctx context.Context, name string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	container, exists := m.containers[name]
	if !exists {
		return fmt.Errorf("container %s not found", name)
	}

	container.Status = "stopped"
	container.Health = "unknown"
	m.log.Info().Str("id", name).Bool("force", force).Msg("MockLXD: Stopped container")
	return nil
}

// RestartContainer restarts a mock container
func (m *MockLXDClient) RestartContainer(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	container, exists := m.containers[name]
	if !exists {
		return fmt.Errorf("container %s not found", name)
	}

	container.Status = "running"
	container.Health = "healthy"
	m.log.Info().Str("id", name).Msg("MockLXD: Restarted container")
	return nil
}

// DeleteContainer removes a mock container
func (m *MockLXDClient) DeleteContainer(ctx context.Context, name string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.containers[name]; !exists {
		return fmt.Errorf("container %s not found", name)
	}

	delete(m.containers, name)
	m.log.Info().Str("id", name).Bool("force", force).Msg("MockLXD: Deleted container")
	return nil
}

// GetContainer returns a container's state
func (m *MockLXDClient) GetContainer(name string) (*ContainerState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, exists := m.containers[name]
	if !exists {
		return nil, false
	}
	// Return a copy
	return &ContainerState{
		ID:       c.ID,
		Status:   c.Status,
		IP:       c.IP,
		CPUUsage: c.CPUUsage,
		Health:   c.Health,
	}, true
}

// ListContainers returns all containers
func (m *MockLXDClient) ListContainers() map[string]*ContainerState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*ContainerState)
	for k, v := range m.containers {
		result[k] = &ContainerState{
			ID:       v.ID,
			Status:   v.Status,
			IP:       v.IP,
			CPUUsage: v.CPUUsage,
			Health:   v.Health,
		}
	}
	return result
}

// ============================================================================
// CONFIG AND STATE TYPES
// ============================================================================

// Config holds daemon configuration (simplified inline for now)
type Config struct {
	NodeID       string
	NodeName     string
	HTTPAddr     string
	GRPCAddr     string
	LXDSocket    string
	BusboyAddr   string
	BusboyGRPCAddr string
	PollInterval time.Duration
	LogLevel     string
}

// StateManager manages desired vs actual state
type StateManager struct {
	mu       sync.RWMutex
	desired  map[string]*ContainerSpec
	actual   map[string]*ContainerState
	drifts   []DriftReport
}

// ContainerSpec represents desired container state
type ContainerSpec struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	CPU    int               `json:"cpu"`
	Memory int64             `json:"memory"`
	Labels map[string]string `json:"labels"`
}

// ContainerState represents actual container state
type ContainerState struct {
	ID       string  `json:"id"`
	Status   string  `json:"status"`
	IP       string  `json:"ip"`
	CPUUsage float64 `json:"cpu_usage"`
	Health   string  `json:"health"`
}

// DriftReport represents detected drift
type DriftReport struct {
	ContainerID string    `json:"container_id"`
	DriftType   string    `json:"drift_type"`
	Severity    string    `json:"severity"`
	DetectedAt  time.Time `json:"detected_at"`
}

// ReconcileAction represents an action taken during reconciliation
type ReconcileAction struct {
	ContainerID string    `json:"container_id"`
	Action      string    `json:"action"`
	Reason      string    `json:"reason"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// ============================================================================
// MAIN
// ============================================================================

func main() {
	// Parse flags
	configPath := flag.String("config", "", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("unheaded-daemon %s (commit: %s, built: %s)\n", version, gitCommit, buildTime)
		os.Exit(0)
	}

	// Print banner
	fmt.Print(banner)
	fmt.Printf("Version: %s | Node: %s\n", version, getHostname())
	fmt.Println()

	// Load configuration
	cfg := loadConfig(*configPath)

	// Initialize logger
	log := logger.New(os.Stdout)
	level, _ := logger.ParseLevel(cfg.LogLevel)
	log.SetLevel(level)

	// Create daemon
	daemon := NewDaemon(cfg, log)

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start daemon
	if err := daemon.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start daemon")
	}

	log.Info().Str("addr", cfg.HTTPAddr).Msg("Cuirass Control Plane active")
	log.Info().Msg("Listening for state changes...")

	// Wait for shutdown signal
	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("Received signal, initiating graceful shutdown...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := daemon.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Error during shutdown")
	}

	log.Info().Msg("Cuirass Control Plane shut down gracefully")
}

// ============================================================================
// DAEMON LIFECYCLE
// ============================================================================

// NewDaemon creates a new daemon instance
func NewDaemon(cfg *Config, log *logger.Logger) *Daemon {
	return &Daemon{
		config: cfg,
		log:    log,
		stateManager: &StateManager{
			desired: make(map[string]*ContainerSpec),
			actual:  make(map[string]*ContainerState),
			drifts:  make([]DriftReport, 0),
		},
		lxdClient: NewMockLXDClient(log), // Use mock client for now
		shutdown:  make(chan struct{}),
	}
}

// Start starts the daemon
func (d *Daemon) Start() error {
	// Initialize Busboy client
	if err := d.initBusboy(); err != nil {
		d.log.Warn().Err(err).Msg("Busboy connection failed, will retry")
	}

	// Log LXD client status
	d.log.Info().Msg("LXD client initialized (mock mode)")

	// Setup HTTP server
	mux := http.NewServeMux()
	d.registerHandlers(mux)

	d.httpServer = &http.Server{
		Addr:         d.config.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Start HTTP server
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			d.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Start Busboy connection manager (handles reconnection)
	d.wg.Add(1)
	go d.busboyConnectionLoop()

	// Start metrics publishing loop
	d.wg.Add(1)
	go d.metricsPublishLoop()

	// Start state reconciliation loop
	d.wg.Add(1)
	go d.reconciliationLoop()

	// Start drift detection loop
	d.wg.Add(1)
	go d.driftDetectionLoop()

	return nil
}

// Shutdown gracefully shuts down the daemon
func (d *Daemon) Shutdown(ctx context.Context) error {
	close(d.shutdown)

	if d.httpServer != nil {
		if err := d.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
	}

	// Close Busboy client
	d.busboyMu.Lock()
	if d.busboyClient != nil {
		if err := d.busboyClient.Close(); err != nil {
			d.log.Warn().Err(err).Msg("Error closing Busboy client")
		}
		d.busboyClient = nil
		d.busboyReady = false
	}
	d.busboyMu.Unlock()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ============================================================================
// BUSBOY INTEGRATION
// ============================================================================

// initBusboy initializes the Busboy client and subscribes to required topics
func (d *Daemon) initBusboy() error {
	client, err := busboyClient.NewClient(d.config.BusboyAddr)
	if err != nil {
		return fmt.Errorf("create busboy client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Subscribe to drift topic
	_, err = client.Subscribe(ctx, TopicCuirassDrift, fmt.Sprintf("cuirass-%s", d.config.NodeID))
	if err != nil {
		client.Close()
		return fmt.Errorf("subscribe to %s: %w", TopicCuirassDrift, err)
	}

	// Subscribe to metrics topic
	_, err = client.Subscribe(ctx, TopicCuirassMetrics, fmt.Sprintf("cuirass-%s", d.config.NodeID))
	if err != nil {
		client.Close()
		return fmt.Errorf("subscribe to %s: %w", TopicCuirassMetrics, err)
	}

	d.busboyMu.Lock()
	d.busboyClient = client
	d.busboyReady = true
	d.busboyMu.Unlock()

	d.log.Info().Str("addr", d.config.BusboyAddr).Msg("Busboy connected")
	return nil
}

// busboyConnectionLoop manages Busboy connection with automatic reconnection
func (d *Daemon) busboyConnectionLoop() {
	defer d.wg.Done()

	reconnectInterval := 5 * time.Second
	maxReconnectInterval := 60 * time.Second

	for {
		select {
		case <-d.shutdown:
			return
		default:
		}

		d.busboyMu.RLock()
		isReady := d.busboyReady
		d.busboyMu.RUnlock()

		if !isReady {
			d.log.Info().Str("addr", d.config.BusboyAddr).Msg("Attempting Busboy reconnection")
			if err := d.initBusboy(); err != nil {
				d.log.Warn().Err(err).Dur("retry_in", reconnectInterval).Msg("Busboy reconnection failed")

				select {
				case <-d.shutdown:
					return
				case <-time.After(reconnectInterval):
				}

				// Exponential backoff
				reconnectInterval = reconnectInterval * 2
				if reconnectInterval > maxReconnectInterval {
					reconnectInterval = maxReconnectInterval
				}
				continue
			}
			// Reset backoff on successful connection
			reconnectInterval = 5 * time.Second
		}

		// Check connection health periodically
		select {
		case <-d.shutdown:
			return
		case <-time.After(30 * time.Second):
		}
	}
}

// metricsPublishLoop publishes metrics to Busboy periodically
func (d *Daemon) metricsPublishLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.shutdown:
			return
		case <-ticker.C:
			d.publishMetrics()
		}
	}
}

// publishMetrics publishes current metrics to Busboy
func (d *Daemon) publishMetrics() {
	d.busboyMu.RLock()
	client := d.busboyClient
	ready := d.busboyReady
	d.busboyMu.RUnlock()

	if !ready || client == nil {
		return
	}

	d.stateManager.mu.RLock()
	metrics := map[string]interface{}{
		"node_id":       d.config.NodeID,
		"node_name":     d.config.NodeName,
		"timestamp":     time.Now().UnixMilli(),
		"desired_count": len(d.stateManager.desired),
		"actual_count":  len(d.stateManager.actual),
		"drift_count":   len(d.stateManager.drifts),
		"component":     "cuirass",
	}
	d.stateManager.mu.RUnlock()

	payload, err := json.Marshal(metrics)
	if err != nil {
		d.log.Error().Err(err).Msg("Error marshaling metrics")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Publish(ctx, TopicCuirassMetrics, payload); err != nil {
		// Mark connection as unhealthy for reconnection
		if err == busboyClient.ErrNotConnected || err == busboyClient.ErrSubscriptionPending {
			d.busboyMu.Lock()
			d.busboyReady = false
			d.busboyMu.Unlock()
		}
		d.log.Error().Err(err).Msg("Error publishing metrics to Busboy")
		return
	}

	d.log.Debug().Str("topic", TopicCuirassMetrics).Msg("Published metrics")
}

// publishDriftEvents publishes drift events to Busboy
func (d *Daemon) publishDriftEvents(drifts []DriftReport) {
	if len(drifts) == 0 {
		return
	}

	d.busboyMu.RLock()
	client := d.busboyClient
	ready := d.busboyReady
	d.busboyMu.RUnlock()

	if !ready || client == nil {
		d.log.Warn().Msg("Busboy not ready, skipping drift event publish")
		return
	}

	event := map[string]interface{}{
		"node_id":     d.config.NodeID,
		"node_name":   d.config.NodeName,
		"timestamp":   time.Now().UnixMilli(),
		"drift_count": len(drifts),
		"drifts":      drifts,
		"component":   "cuirass",
	}

	payload, err := json.Marshal(event)
	if err != nil {
		d.log.Error().Err(err).Msg("Error marshaling drift events")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Publish(ctx, TopicCuirassDrift, payload); err != nil {
		// Mark connection as unhealthy for reconnection
		if err == busboyClient.ErrNotConnected || err == busboyClient.ErrSubscriptionPending {
			d.busboyMu.Lock()
			d.busboyReady = false
			d.busboyMu.Unlock()
		}
		d.log.Error().Err(err).Msg("Error publishing drift events to Busboy")
		return
	}

	d.log.Info().Int("count", len(drifts)).Str("topic", TopicCuirassDrift).Msg("Published drift events")
}

// ============================================================================
// HTTP HANDLERS
// ============================================================================

func (d *Daemon) registerHandlers(mux *http.ServeMux) {
	// Health endpoints
	mux.HandleFunc("/health", d.handleHealth)
	mux.HandleFunc("/ready", d.handleReady)

	// State endpoints
	mux.HandleFunc("/api/v1/state", d.handleGetState)
	mux.HandleFunc("/api/v1/state/desired", d.handleDesiredState)
	mux.HandleFunc("/api/v1/state/actual", d.handleActualState)
	mux.HandleFunc("/api/v1/state/drift", d.handleDrift)

	// Container endpoints
	mux.HandleFunc("/api/v1/containers", d.handleContainers)
	mux.HandleFunc("/api/v1/containers/", d.handleContainer)

	// Metrics
	mux.HandleFunc("/metrics", d.handleMetrics)

	// Info
	mux.HandleFunc("/api/v1/info", d.handleInfo)
}

func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service":   "cuirass",
		"status":    "healthy",
		"version":   "0.1.0",
		"timestamp": time.Now().UTC(),
		"component": "cuirass",
		"hollow":    "crystal_grotto",
	})
}

func (d *Daemon) handleReady(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	lxdReady := d.lxdClient != nil
	d.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready": true,
		"services": map[string]bool{
			"state_manager": true,
			"lxd_client":    lxdReady,
			"ebpf_loader":   false, // TODO: implement
		},
	})
}

func (d *Daemon) handleGetState(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"desired_count": len(d.stateManager.desired),
		"actual_count":  len(d.stateManager.actual),
		"drift_count":   len(d.stateManager.drifts),
		"timestamp":     time.Now(),
	})
}

func (d *Daemon) handleDesiredState(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.stateManager.desired)
}

func (d *Daemon) handleActualState(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.stateManager.actual)
}

func (d *Daemon) handleDrift(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.stateManager.drifts)
}

func (d *Daemon) handleContainers(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	containers := make([]map[string]interface{}, 0)
	for id, spec := range d.stateManager.desired {
		actual := d.stateManager.actual[id]
		container := map[string]interface{}{
			"id":      id,
			"name":    spec.Name,
			"image":   spec.Image,
			"desired": spec,
			"actual":  actual,
		}
		containers = append(containers, container)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(containers)
}

func (d *Daemon) handleContainer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract container ID from URL path: /api/v1/containers/{id}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/containers/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "container id is required",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.stateManager.mu.RLock()
		desired, desiredExists := d.stateManager.desired[id]
		actual, actualExists := d.stateManager.actual[id]
		d.stateManager.mu.RUnlock()

		if !desiredExists && !actualExists {
			d.log.Warn().Str("id", id).Msg("Container not found")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "container not found",
			})
			return
		}

		d.log.Info().Str("id", id).Msg("Retrieved container state")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      id,
			"desired": desired,
			"actual":  actual,
		})

	case http.MethodPut:
		var spec ContainerSpec
		// SECURITY: Enforce body size limit (1MB) to prevent denial-of-service
		if err := json.NewDecoder(io.LimitReader(r.Body, 1*1024*1024)).Decode(&spec); err != nil {
			d.log.Error().Err(err).Str("id", id).Msg("Invalid request body for container update")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("invalid request body: %v", err),
			})
			return
		}

		// Ensure the spec ID matches the URL path ID
		spec.ID = id

		d.stateManager.mu.Lock()
		d.stateManager.desired[id] = &spec
		d.stateManager.mu.Unlock()

		d.log.Info().Str("id", id).Str("name", spec.Name).Str("image", spec.Image).Msg("Updated desired state for container")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "desired state updated",
			"id":      id,
			"desired": &spec,
		})

	case http.MethodDelete:
		d.stateManager.mu.Lock()
		_, exists := d.stateManager.desired[id]
		if !exists {
			d.stateManager.mu.Unlock()
			d.log.Warn().Str("id", id).Msg("Container not found in desired state for deletion")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "container not found in desired state",
			})
			return
		}
		delete(d.stateManager.desired, id)
		d.stateManager.mu.Unlock()

		d.log.Info().Str("id", id).Msg("Removed container from desired state, reconciliation will clean up")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "container removed from desired state",
			"id":      id,
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("method %s not allowed", r.Method),
		})
	}
}

func (d *Daemon) handleMetrics(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	// Prometheus format
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# HELP cuirass_containers_desired Number of desired containers\n")
	fmt.Fprintf(w, "# TYPE cuirass_containers_desired gauge\n")
	fmt.Fprintf(w, "cuirass_containers_desired %d\n", len(d.stateManager.desired))
	fmt.Fprintf(w, "# HELP cuirass_containers_actual Number of actual containers\n")
	fmt.Fprintf(w, "# TYPE cuirass_containers_actual gauge\n")
	fmt.Fprintf(w, "cuirass_containers_actual %d\n", len(d.stateManager.actual))
	fmt.Fprintf(w, "# HELP cuirass_drift_count Number of detected drifts\n")
	fmt.Fprintf(w, "# TYPE cuirass_drift_count gauge\n")
	fmt.Fprintf(w, "cuirass_drift_count %d\n", len(d.stateManager.drifts))
}

func (d *Daemon) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"component":  "cuirass",
		"version":    version,
		"git_commit": gitCommit,
		"build_time": buildTime,
		"node_id":    d.config.NodeID,
		"node_name":  d.config.NodeName,
		"hollow":     "crystal_grotto",
		"role":       "control_plane",
		"kingdom":    "unheaded",
	})
}

// ============================================================================
// RECONCILIATION LOOP
// ============================================================================

func (d *Daemon) reconciliationLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.shutdown:
			return
		case <-ticker.C:
			d.reconcile()
		}
	}
}

func (d *Daemon) reconcile() {
	d.stateManager.mu.Lock()
	defer d.stateManager.mu.Unlock()

	// Check if LXD client is available
	if d.lxdClient == nil {
		d.log.Warn().Msg("LXD client not available, skipping reconciliation")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var actions []ReconcileAction

	// Compare desired vs actual and take action
	for id, desired := range d.stateManager.desired {
		actual, exists := d.stateManager.actual[id]

		if !exists {
			// Container should exist but doesn't - create it
			// DRIFT TYPE: "missing"
			d.log.Info().Str("id", id).Str("name", desired.Name).Str("image", desired.Image).Msg("Container missing, creating...")

			action := ReconcileAction{
				ContainerID: id,
				Action:      "create",
				Reason:      "container missing from desired state",
				Timestamp:   time.Now(),
			}

			// Create the container
			if err := d.lxdClient.CreateContainer(ctx, desired); err != nil {
				d.log.Error().Err(err).Str("id", id).Msg("Failed to create container")
				action.Success = false
				action.Error = err.Error()
				actions = append(actions, action)
				continue
			}

			// Start the container after creation
			if err := d.lxdClient.StartContainer(ctx, id); err != nil {
				d.log.Error().Err(err).Str("id", id).Msg("Failed to start container after creation")
				action.Success = false
				action.Error = fmt.Sprintf("created but failed to start: %v", err)
				actions = append(actions, action)
				continue
			}

			d.log.Info().Str("id", id).Msg("Container created and started successfully")
			action.Success = true
			actions = append(actions, action)

			// Update actual state with the new container
			if state, ok := d.lxdClient.GetContainer(id); ok {
				d.stateManager.actual[id] = state
			}
			continue
		}

		if actual.Status != "running" {
			// Container should be running but isn't - start it
			// DRIFT TYPE: "status"
			d.log.Info().Str("id", id).Str("name", desired.Name).Str("current_status", actual.Status).Msg("Container not running, starting...")

			action := ReconcileAction{
				ContainerID: id,
				Action:      "start",
				Reason:      fmt.Sprintf("container status is '%s', expected 'running'", actual.Status),
				Timestamp:   time.Now(),
			}

			if err := d.lxdClient.StartContainer(ctx, id); err != nil {
				d.log.Error().Err(err).Str("id", id).Msg("Failed to start container")
				action.Success = false
				action.Error = err.Error()
				actions = append(actions, action)
				continue
			}

			d.log.Info().Str("id", id).Msg("Container started successfully")
			action.Success = true
			actions = append(actions, action)

			// Update actual state
			if state, ok := d.lxdClient.GetContainer(id); ok {
				d.stateManager.actual[id] = state
			}
			continue
		}

		if actual.Health == "unhealthy" {
			// Container is unhealthy - restart it
			// DRIFT TYPE: "degraded" (health degraded / config mismatch)
			d.log.Info().Str("id", id).Str("name", desired.Name).Msg("Container unhealthy, restarting...")

			action := ReconcileAction{
				ContainerID: id,
				Action:      "restart",
				Reason:      "container health status is 'unhealthy'",
				Timestamp:   time.Now(),
			}

			if err := d.lxdClient.RestartContainer(ctx, id); err != nil {
				d.log.Error().Err(err).Str("id", id).Msg("Failed to restart container")
				action.Success = false
				action.Error = err.Error()
				actions = append(actions, action)
				continue
			}

			d.log.Info().Str("id", id).Msg("Container restarted successfully")
			action.Success = true
			actions = append(actions, action)

			// Update actual state
			if state, ok := d.lxdClient.GetContainer(id); ok {
				d.stateManager.actual[id] = state
			}
			continue
		}
	}

	// Check for orphaned containers (actual but not desired)
	// DRIFT TYPE: "orphaned"
	for id := range d.stateManager.actual {
		if _, exists := d.stateManager.desired[id]; !exists {
			d.log.Info().Str("id", id).Msg("Container orphaned, deleting...")

			// Safety check: only delete if container is not in desired state
			// This is an important safety measure to prevent accidental deletion
			action := ReconcileAction{
				ContainerID: id,
				Action:      "delete",
				Reason:      "container exists but is not in desired state (orphaned)",
				Timestamp:   time.Now(),
			}

			// Perform safety check - confirm it's really not in desired state
			// We need to release and re-acquire the lock for the safety check
			d.stateManager.mu.Unlock()
			d.stateManager.mu.RLock()
			if _, stillDesired := d.stateManager.desired[id]; stillDesired {
				d.log.Warn().Str("id", id).Msg("Safety check failed: container appeared in desired state during delete")
				d.stateManager.mu.RUnlock()
				d.stateManager.mu.Lock()
				action.Success = false
				action.Error = "safety check failed: container appeared in desired state"
				actions = append(actions, action)
				continue
			}
			d.stateManager.mu.RUnlock()
			d.stateManager.mu.Lock()

			// Delete the orphaned container (force=true to handle any state)
			if err := d.lxdClient.DeleteContainer(ctx, id, true); err != nil {
				d.log.Error().Err(err).Str("id", id).Msg("Failed to delete orphaned container")
				action.Success = false
				action.Error = err.Error()
				actions = append(actions, action)
				continue
			}

			d.log.Info().Str("id", id).Msg("Orphaned container deleted successfully")
			action.Success = true
			actions = append(actions, action)

			// Remove from actual state
			delete(d.stateManager.actual, id)
		}
	}

	// Sync actual state from LXD client to ensure consistency
	d.syncActualStateFromLXD()

	// Publish reconciliation events if any actions were taken
	if len(actions) > 0 {
		go d.publishReconcileEvents(actions)
	}
}

// syncActualStateFromLXD updates the actual state from the LXD client
func (d *Daemon) syncActualStateFromLXD() {
	if d.lxdClient == nil {
		return
	}

	containers := d.lxdClient.ListContainers()
	for id, state := range containers {
		d.stateManager.actual[id] = state
	}
}

// publishReconcileEvents publishes reconciliation action events to Busboy
func (d *Daemon) publishReconcileEvents(actions []ReconcileAction) {
	d.busboyMu.RLock()
	client := d.busboyClient
	ready := d.busboyReady
	d.busboyMu.RUnlock()

	if !ready || client == nil {
		return
	}

	// Count successful and failed actions
	successCount := 0
	failedCount := 0
	for _, a := range actions {
		if a.Success {
			successCount++
		} else {
			failedCount++
		}
	}

	event := map[string]interface{}{
		"node_id":       d.config.NodeID,
		"node_name":     d.config.NodeName,
		"timestamp":     time.Now().UnixMilli(),
		"action_count":  len(actions),
		"success_count": successCount,
		"failed_count":  failedCount,
		"actions":       actions,
		"component":     "cuirass",
		"event_type":    "reconcile",
	}

	payload, err := json.Marshal(event)
	if err != nil {
		d.log.Error().Err(err).Msg("Error marshaling reconcile events")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Publish reconcile events to drift topic (related to state changes)
	if err := client.Publish(ctx, TopicCuirassDrift, payload); err != nil {
		if err == busboyClient.ErrNotConnected || err == busboyClient.ErrSubscriptionPending {
			d.busboyMu.Lock()
			d.busboyReady = false
			d.busboyMu.Unlock()
		}
		d.log.Error().Err(err).Msg("Error publishing reconcile events to Busboy")
		return
	}

	d.log.Info().Int("total", len(actions)).Int("success", successCount).Int("failed", failedCount).Str("topic", TopicCuirassDrift).Msg("Published reconcile actions")
}

// ============================================================================
// DRIFT DETECTION LOOP
// ============================================================================

func (d *Daemon) driftDetectionLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.PollInterval * 2)
	defer ticker.Stop()

	for {
		select {
		case <-d.shutdown:
			return
		case <-ticker.C:
			d.detectDrift()
		}
	}
}

func (d *Daemon) detectDrift() {
	d.stateManager.mu.Lock()
	defer d.stateManager.mu.Unlock()

	// Sync actual state from LXD before detecting drift
	d.syncActualStateFromLXD()

	drifts := make([]DriftReport, 0)

	// Check for missing containers (desired but not actual)
	for id := range d.stateManager.desired {
		if _, exists := d.stateManager.actual[id]; !exists {
			drifts = append(drifts, DriftReport{
				ContainerID: id,
				DriftType:   "missing",
				Severity:    "high",
				DetectedAt:  time.Now(),
			})
		}
	}

	// Check for orphaned containers (actual but not desired)
	for id := range d.stateManager.actual {
		if _, exists := d.stateManager.desired[id]; !exists {
			drifts = append(drifts, DriftReport{
				ContainerID: id,
				DriftType:   "orphaned",
				Severity:    "low",
				DetectedAt:  time.Now(),
			})
		}
	}

	// Check for status drift (container exists but not running)
	for id, actual := range d.stateManager.actual {
		if _, exists := d.stateManager.desired[id]; exists {
			if actual.Status != "running" {
				drifts = append(drifts, DriftReport{
					ContainerID: id,
					DriftType:   "status",
					Severity:    "medium",
					DetectedAt:  time.Now(),
				})
			}
			// Check for health degradation
			if actual.Health == "unhealthy" {
				drifts = append(drifts, DriftReport{
					ContainerID: id,
					DriftType:   "degraded",
					Severity:    "medium",
					DetectedAt:  time.Now(),
				})
			}
		}
	}

	if len(drifts) > 0 {
		d.log.Info().Int("count", len(drifts)).Msg("Detected drifts")
		// Publish drift events to Busboy
		go d.publishDriftEvents(drifts)
	}

	d.stateManager.drifts = drifts
}

// ============================================================================
// HELPERS
// ============================================================================

// yamlConfig represents the YAML file structure for daemon configuration.
// Fields map to the inline Config struct used by the daemon.
type yamlConfig struct {
	NodeID       string `yaml:"node_id"`
	NodeName     string `yaml:"node_name"`
	HTTPAddr     string `yaml:"http_addr"`
	GRPCAddr     string `yaml:"grpc_addr"`
	LXDSocket    string `yaml:"lxd_socket"`
	BusboyAddr   string `yaml:"busboy_addr"`
	BusboyGRPCAddr string `yaml:"busboy_grpc_addr"`
	PollInterval string `yaml:"poll_interval"`
	LogLevel     string `yaml:"log_level"`
}

func loadConfig(path string) *Config {
	// Start with defaults from environment variables
	cfg := &Config{
		NodeID:       getEnvOrDefault("UNHEADED_NODE_ID", fmt.Sprintf("citadel-%s", getHostname())),
		NodeName:     getEnvOrDefault("UNHEADED_NODE_NAME", getHostname()),
		HTTPAddr:     getEnvOrDefault("HTTP_ADDR", ":8080"),
		GRPCAddr:     getEnvOrDefault("GRPC_ADDR", ":9090"),
		LXDSocket:    getEnvOrDefault("LXD_SOCKET", "/var/lib/lxd/unix.socket"),
		BusboyAddr:   getEnvOrDefault("BUSBOY_ADDR", "localhost:5555"),
		BusboyGRPCAddr: getEnvOrDefault("BUSBOY_GRPC_ADDR", ""),
		PollInterval: 30 * time.Second,
		LogLevel:     getEnvOrDefault("LOG_LEVEL", "info"),
	}

	// Load from YAML file if path provided, overriding defaults
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading config file %s: %v\n", path, err)
			os.Exit(1)
		}

		var yc yamlConfig
		if err := yaml.Unmarshal(data, &yc); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing config file %s: %v\n", path, err)
			os.Exit(1)
		}

		// Apply non-empty values from YAML, overriding env/defaults
		if yc.NodeID != "" {
			cfg.NodeID = yc.NodeID
		}
		if yc.NodeName != "" {
			cfg.NodeName = yc.NodeName
		}
		if yc.HTTPAddr != "" {
			cfg.HTTPAddr = yc.HTTPAddr
		}
		if yc.GRPCAddr != "" {
			cfg.GRPCAddr = yc.GRPCAddr
		}
		if yc.LXDSocket != "" {
			cfg.LXDSocket = yc.LXDSocket
		}
		if yc.BusboyAddr != "" {
			cfg.BusboyAddr = yc.BusboyAddr
		}
		if yc.BusboyGRPCAddr != "" {
			cfg.BusboyGRPCAddr = yc.BusboyGRPCAddr
		}
		if yc.LogLevel != "" {
			cfg.LogLevel = yc.LogLevel
		}
		if yc.PollInterval != "" {
			if d, err := time.ParseDuration(yc.PollInterval); err == nil {
				cfg.PollInterval = d
			} else {
				fmt.Fprintf(os.Stderr, "Warning: invalid poll_interval %q in config, using default\n", yc.PollInterval)
			}
		}

		fmt.Printf("Loaded config from %s\n", path)
	}

	return cfg
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
