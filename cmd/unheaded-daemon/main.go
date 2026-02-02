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
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
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

// Daemon represents the unheaded-daemon control plane
type Daemon struct {
	mu     sync.RWMutex
	config *Config

	// State management
	stateManager *StateManager

	// LXD client (mock for now)
	// lxdClient lxd.Client

	// eBPF loader (mock for now)
	// ebpfLoader ebpf.Loader

	// HTTP server
	httpServer *http.Server

	// Shutdown coordination
	shutdown chan struct{}
	wg       sync.WaitGroup
}

// Config holds daemon configuration (simplified inline for now)
type Config struct {
	NodeID       string
	NodeName     string
	HTTPAddr     string
	GRPCAddr     string
	LXDSocket    string
	BusboyAddr   string
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

	// Create daemon
	daemon := NewDaemon(cfg)

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start daemon
	if err := daemon.Start(); err != nil {
		log.Fatalf("Failed to start daemon: %v", err)
	}

	log.Printf("🛡️  Cuirass Control Plane active on %s", cfg.HTTPAddr)
	log.Println("📡 Listening for state changes...")

	// Wait for shutdown signal
	sig := <-sigCh
	log.Printf("\n🔻 Received signal %v, initiating graceful shutdown...", sig)

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := daemon.Shutdown(ctx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("✅ Cuirass Control Plane shut down gracefully")
}

// ============================================================================
// DAEMON LIFECYCLE
// ============================================================================

// NewDaemon creates a new daemon instance
func NewDaemon(cfg *Config) *Daemon {
	return &Daemon{
		config: cfg,
		stateManager: &StateManager{
			desired: make(map[string]*ContainerSpec),
			actual:  make(map[string]*ContainerState),
			drifts:  make([]DriftReport, 0),
		},
		shutdown: make(chan struct{}),
	}
}

// Start starts the daemon
func (d *Daemon) Start() error {
	// Setup HTTP server
	mux := http.NewServeMux()
	d.registerHandlers(mux)

	d.httpServer = &http.Server{
		Addr:         d.config.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

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
		"status": "healthy",
		"component": "cuirass",
		"hollow": "crystal_grotto",
	})
}

func (d *Daemon) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready": true,
		"services": map[string]bool{
			"state_manager": true,
			"lxd_client":    false, // TODO: implement
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
	// TODO: implement single container operations
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "not implemented",
	})
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

	// Compare desired vs actual and take action
	for id, desired := range d.stateManager.desired {
		actual, exists := d.stateManager.actual[id]

		if !exists {
			// Container should exist but doesn't - create it
			log.Printf("📦 Container %s (%s) missing, scheduling creation", id, desired.Name)
			// TODO: d.lxdClient.CreateContainer(desired)
			continue
		}

		if actual.Status != "running" {
			// Container should be running - start it
			log.Printf("▶️  Container %s (%s) not running (status: %s), scheduling start", id, desired.Name, actual.Status)
			// TODO: d.lxdClient.StartContainer(desired)
			continue
		}

		if actual.Health == "unhealthy" {
			// Container unhealthy - restart it
			log.Printf("🔄 Container %s (%s) unhealthy, scheduling restart", id, desired.Name)
			// TODO: d.lxdClient.RestartContainer(desired)
			continue
		}
	}

	// Check for orphaned containers (actual but not desired)
	for id := range d.stateManager.actual {
		if _, exists := d.stateManager.desired[id]; !exists {
			log.Printf("🗑️  Container %s orphaned, scheduling removal", id)
			// TODO: d.lxdClient.DeleteContainer(...)
		}
	}
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

	drifts := make([]DriftReport, 0)

	// Check for missing containers
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

	// Check for orphaned containers
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

	// Check for status drift
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
		}
	}

	if len(drifts) > 0 {
		log.Printf("🔍 Detected %d drift(s)", len(drifts))
		// TODO: Publish to Busboy
	}

	d.stateManager.drifts = drifts
}

// ============================================================================
// HELPERS
// ============================================================================

func loadConfig(path string) *Config {
	cfg := &Config{
		NodeID:       getEnvOrDefault("UNHEADED_NODE_ID", fmt.Sprintf("citadel-%s", getHostname())),
		NodeName:     getEnvOrDefault("UNHEADED_NODE_NAME", getHostname()),
		HTTPAddr:     getEnvOrDefault("HTTP_ADDR", ":8080"),
		GRPCAddr:     getEnvOrDefault("GRPC_ADDR", ":9090"),
		LXDSocket:    getEnvOrDefault("LXD_SOCKET", "/var/lib/lxd/unix.socket"),
		BusboyAddr:   getEnvOrDefault("BUSBOY_ADDR", "localhost:5555"),
		PollInterval: 30 * time.Second,
		LogLevel:     getEnvOrDefault("LOG_LEVEL", "info"),
	}

	// TODO: Load from file if path provided
	if path != "" {
		log.Printf("📁 Loading config from %s (not yet implemented)", path)
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
