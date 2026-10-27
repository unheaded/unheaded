// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the GNU General Public License v3.0 or later.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package main is the entry point for the unheaded-daemon
// THE CUIRASS - The Core Heart of the Unheaded Kingdom
//
// "The armor protects the Knight. The Knight serves the Kingdom.
//
//	The Kingdom endures forever." - The Sacred Chronicles
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/discovery"
	"unheaded/pkg/healthreport"
	"unheaded/pkg/httputil"
	"unheaded/pkg/logagg"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
	"unheaded/pkg/probenet"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"

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

// Wotan topic constants
const (
	TopicCuirassDrift   = "cuirass.drift"
	TopicCuirassMetrics = "cuirass.metrics"

	// wotanDisplayName must be on configs/wotan.yaml topics.auto_approve.
	wotanDisplayName = "cuirass"
)

// Daemon represents the unheaded-daemon control plane
type Daemon struct {
	config *Config
	log    *logger.Logger

	// State management
	stateManager *StateManager

	// Wotan client for event publishing
	wotanClient *wotanClient.Client
	wotanReady  bool
	wotanMu     sync.RWMutex

	// LXD client for container operations
	lxdClient LXDClient
	lxdUp     atomic.Bool // the last list succeeded

	// store persists the desired state; nil refuses container writes.
	store *desiredStore

	// authEnabled: the container write API (PUT/DELETE) answers only behind
	// auth. Without it anyone who reaches the port could ask for deletes.
	authEnabled bool

	// eBPF Loader: intentionally disabled in the control plane.
	// Real eBPF loading requires kernel support (see internal/ebpf/loader.go).
	// When RealLoader is complete, enable here:
	//   ebpfLoader, err := ebpf.NewLoader(ebpf.LoaderConfig{...})
	//   if err != nil { log.Warn().Err(err).Msg("eBPF loader failed (optional)") }

	// Transport
	transportCfg transport.Config
	healthSrv    *transport.HealthServer

	// HTTP server
	httpServer *http.Server

	// Shutdown coordination
	shutdown chan struct{}
	wg       sync.WaitGroup

	// registry produces the /metrics body; this daemon used to assemble that
	// text with fmt.Fprintf calls.
	registry *metrics.Registry

	// logPublisher, when set, has its log-forwarding counters registered in
	// registry (SetLogPublisher); kept so a swap can unregister them.
	logPublisherMu sync.Mutex
	logPublisher   *logagg.Publisher

	// chaosPublish sends a cuirass.chaos event; nil means publishToWotan.
	chaosPublish func(ctx context.Context, payload []byte) error

	// POST /api/v1/reconcile runs at most once a second (reconcile_now.go).
	reconcileNowMu   sync.Mutex
	lastReconcileNow time.Time

	// Health probing (health_probe.go); health is guarded by stateManager.mu.
	prober      healthProber
	health      map[string]*healthRecord
	healthGauge *metrics.GaugeVec

	actionsTotal *metrics.CounterVec
}

// initMetrics registers every series this daemon publishes. Each reads the
// state manager at scrape time; the caller holds stateManager.mu, so the
// closures must not take it again.
func (d *Daemon) initMetrics() {
	d.registry = metrics.NewRegistry()

	gauge := func(name, help string, read func() int) {
		d.registry.MustRegister(metrics.NewFuncGauge(name, help, nil, func() float64 {
			return float64(read())
		}))
	}
	gauge("cuirass_containers_desired", "Number of desired containers", func() int { return len(d.stateManager.desired) })
	gauge("cuirass_containers_actual", "Number of actual containers", func() int { return len(d.stateManager.actual) })
	gauge("cuirass_drift_count", "Number of detected drifts", func() int { return len(d.stateManager.drifts) })
	gauge("cuirass_lxd_up", "1 if the last LXD instance list succeeded", func() int {
		if d.lxdUp.Load() {
			return 1
		}
		return 0
	})
	d.healthGauge = metrics.NewGaugeVec("cuirass_container_health",
		"Health probe verdict per managed instance with a health spec: 1 healthy, 0 unhealthy, -1 not yet known",
		nil, []string{"id"})
	d.registry.MustRegister(d.healthGauge)
	d.actionsTotal = metrics.NewCounterVec("cuirass_reconcile_actions_total",
		"Reconcile actions by action and result (done, failed, planned in observe mode, refused)",
		nil, []string{"action", "result"})
	d.registry.MustRegister(d.actionsTotal)
}

// countAction records one reconcile action's outcome.
func (d *Daemon) countAction(action, result string) {
	if d.actionsTotal != nil {
		d.actionsTotal.WithLabelValues(action, result).Inc()
	}
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
	// UpdateProxy sets (or, with nil, removes) the unheaded-proxy device of a
	// managed instance, leaving every other device alone.
	UpdateProxy(ctx context.Context, name string, p *ProxySpec) error
	// ChaosStop stops (force=false) or kills (force=true) a managed instance
	// labelled chaos=allowed, checking both itself (chaos_action.go).
	ChaosStop(ctx context.Context, name string, force bool) error
	// ListContainers returns all containers. An error means actual state
	// is unknown, never that it is empty.
	ListContainers(ctx context.Context) (map[string]*ContainerState, error)
}

// MockLXDClient is the default LXDClient (LXD_BACKEND unset or "mock"):
// container operations are tracked in memory only. lxdREST is the real one.
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
		Managed:  true, // a real client sets ManagedLabel=true at create
		Proxy:    copyProxy(spec.Proxy),
		Labels:   copyLabels(spec.Labels),
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

// UpdateProxy sets or removes a mock container's proxy; like the real client
// it refuses an instance that is not Unheaded's.
func (m *MockLXDClient) UpdateProxy(_ context.Context, name string, p *ProxySpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[name]
	if !ok {
		return fmt.Errorf("container %s not found", name)
	}
	if !c.Managed {
		return fmt.Errorf("%s: %w", name, errLXDNotManaged)
	}
	c.Proxy = copyProxy(p)
	return nil
}

// ListContainers returns all containers
func (m *MockLXDClient) ListContainers(context.Context) (map[string]*ContainerState, error) {
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
			Managed:  v.Managed,
			Proxy:    copyProxy(v.Proxy),
			Labels:   copyLabels(v.Labels),
		}
	}
	return result, nil
}

func copyProxy(p *ProxySpec) *ProxySpec {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func copyLabels(l map[string]string) map[string]string {
	if l == nil {
		return nil
	}
	c := make(map[string]string, len(l))
	for k, v := range l {
		c[k] = v
	}
	return c
}

// ============================================================================
// CONFIG AND STATE TYPES
// ============================================================================

// Config holds daemon configuration (simplified inline for now)
type Config struct {
	NodeID        string
	NodeName      string
	HTTPAddr      string
	GRPCAddr      string
	WotanAddr     string
	WotanGRPCAddr string
	PollInterval  time.Duration
	LogLevel      string

	// Enforce lets reconciliation act on LXD. Off (the default), Cuirass
	// observes: it plans and reports actions and takes none.
	Enforce bool
	// Prune lets enforcement delete managed instances nobody desires.
	// Unmanaged instances are never deleted, pruning or not.
	Prune bool
	// Chaos enables POST /api/v1/containers/{id}/chaos (CUIRASS_CHAOS=true),
	// for game-day windows only. Off by default.
	Chaos bool

	// LXDBackend is "mock" (the default), "unix" (LXDSocket) or "https"
	// (LXDURL with a client certificate and the pinned server certificate).
	LXDBackend    string
	LXDSocket     string
	LXDURL        string
	LXDProject    string
	LXDClientCert string
	LXDClientKey  string
	LXDServerCert string

	// StateDir holds desired.json, the desired state. Empty refuses writes.
	StateDir string

	// ProxyListenAddrs: lab addresses a proxy device may listen on besides
	// 127.0.0.1 (CUIRASS_PROXY_LISTEN_ADDRS, comma separated IPs).
	ProxyListenAddrs []string

	// ProbeSubnets: the only subnets whose instance-reported addresses the
	// health probe will contact (CUIRASS_PROBE_SUBNETS, comma separated
	// CIDRs; default pkg/probenet.DefaultCIDR, setup-host.sh's lxdbr0).
	// S-CLEAN-SLATE M-039.
	ProbeSubnets []netip.Prefix
}

// StateManager manages desired vs actual state
type StateManager struct {
	mu      sync.RWMutex
	desired map[string]*ContainerSpec
	actual  map[string]*ContainerState
	drifts  []DriftReport
}

// ContainerSpec represents desired container state
type ContainerSpec struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	CPU    int               `json:"cpu"`
	Memory int64             `json:"memory"`
	Labels map[string]string `json:"labels"`
	// Health, when set, is probed on the instance each sync (health_probe.go).
	Health *HealthSpec `json:"health,omitempty"`
	// Proxy, when set, is the instance's unheaded-proxy device, the
	// service's stable endpoint (proxy_device.go).
	Proxy *ProxySpec `json:"proxy,omitempty"`
}

// validate checks the parts of a spec that are checked before it is stored.
func (s *ContainerSpec) validate(proxyListen []string) error {
	if s.Health != nil {
		if err := s.Health.validate(); err != nil {
			return err
		}
	}
	if s.Proxy != nil {
		if err := s.Proxy.validate(proxyListen); err != nil {
			return err
		}
	}
	return nil
}

// ManagedLabel marks an LXD instance as Unheaded's. Cuirass sets it on
// every instance it creates and never starts, restarts or deletes one
// without it: west's LXD also runs pihole, which is not ours.
const ManagedLabel = "user.unheaded.managed"

// ContainerState represents actual container state
type ContainerState struct {
	ID       string  `json:"id"`
	Status   string  `json:"status"`
	IP       string  `json:"ip"`
	CPUUsage float64 `json:"cpu_usage"`
	Health   string  `json:"health"`
	Managed  bool    `json:"managed"` // carries ManagedLabel=true
	// Proxy is the instance's unheaded-proxy device, if any; other devices
	// are not reported.
	Proxy *ProxySpec `json:"proxy,omitempty"`
	// Labels are the instance's user.unheaded.label.* config keys.
	Labels map[string]string `json:"labels,omitempty"`
	// HostVeth is the host-side veth of the instance's bridged NIC (LXD
	// state network.<nic>.host_name). Empty for a non-bridged instance,
	// which is then not a chaos-eligible netem target (S-REAL-STEEL D-21).
	HostVeth string `json:"host_veth,omitempty"`
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
	// Planned: observe mode decided this action and did not take it.
	Planned bool `json:"planned,omitempty"`

	spec *ContainerSpec // what to create, for "create"
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

	// Initialize transport config
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	if cfg.WotanGRPCAddr != "" {
		transportCfg.WotanGRPCAddr = cfg.WotanGRPCAddr
	}
	if cfg.WotanAddr != "" {
		transportCfg.WotanHTTPAddr = "http://" + cfg.WotanAddr
	}

	// Initialize logger
	log := logger.New(os.Stdout)
	level, _ := logger.ParseLevel(cfg.LogLevel)
	log.SetLevel(level)

	// Create transport health server
	healthSrv := transport.NewHealthServer("unheaded-daemon")

	// Log aggregation publisher — forwards structured logs to Wotan
	var logConn transport.Connection
	if cfg.WotanAddr != "" {
		var err error
		if logConn, err = logagg.Connect(context.Background(), transportCfg, "unheaded-daemon"); err != nil {
			// best-effort: logs stay local, but say why so a nil here is not silent
			log.Warn().Err(err).Msg("log aggregation: no transport connection; logs not forwarded")
			logConn = nil
		}
	}
	logPublisher := logagg.NewPublisher("unheaded-daemon", logConn)
	log.AddHook(logPublisher.LoggerHook())
	defer logPublisher.Close()

	// Service discovery: announced on Wotan's system.discovery topic (best-effort)
	discoveryCtx, discoveryCancel := context.WithCancel(context.Background())
	defer discoveryCancel()
	discovery.Start(discoveryCtx, transportCfg, "unheaded-daemon", ports.PortOf(cfg.HTTPAddr, ports.DaemonHTTP))
	healthreport.StartService(discoveryCtx, transportCfg, "unheaded-daemon", healthreport.HealthURL(cfg.HTTPAddr, ports.PortOf(cfg.HTTPAddr, ports.DaemonHTTP))) // ADR-029: a vote on Akira's ballot

	// Create daemon
	daemon := NewDaemon(cfg, log, transportCfg, healthSrv)
	if err := daemon.SetLogPublisher(logPublisher); err != nil {
		log.Warn().Err(err).Msg("log forwarding counters not served on /metrics")
	}
	lxdClient, err := newLXDClient(cfg, log)
	if err != nil {
		log.Fatal().Err(err).Str("backend", cfg.LXDBackend).Msg("LXD client configuration")
	}
	daemon.lxdClient = lxdClient
	if cfg.StateDir == "" {
		log.Warn().Msg("No state dir: container writes are refused (desired state could not survive a restart)")
	} else {
		daemon.store = newDesiredStore(cfg.StateDir)
		desired, err := daemon.store.load()
		if err != nil {
			log.Fatal().Err(err).Msg("Desired state unreadable; refusing to start with an empty one")
		}
		daemon.stateManager.desired = desired
		log.Info().Int("containers", len(desired)).Str("path", daemon.store.path).Msg("Desired state loaded")
	}

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
func NewDaemon(cfg *Config, log *logger.Logger, transportCfg transport.Config, healthSrv *transport.HealthServer) *Daemon {
	d := &Daemon{
		config: cfg,
		log:    log,
		stateManager: &StateManager{
			desired: make(map[string]*ContainerSpec),
			actual:  make(map[string]*ContainerState),
			drifts:  make([]DriftReport, 0),
		},
		lxdClient:    NewMockLXDClient(log), // main swaps in newLXDClient's choice
		transportCfg: transportCfg,
		healthSrv:    healthSrv,
		shutdown:     make(chan struct{}),
	}
	d.initMetrics()
	return d
}

// Start starts the daemon
func (d *Daemon) Start() error {
	// Initialize Wotan client
	if err := d.initWotan(); err != nil {
		d.log.Warn().Err(err).Msg("Wotan connection failed, will retry")
		d.healthSrv.SetGRPCStatus(false)
	}

	d.log.Info().Str("backend", d.config.LXDBackend).Str("project", d.config.LXDProject).
		Bool("enforce", d.config.Enforce).Bool("prune", d.config.Prune).
		Msg("LXD client initialized")

	// Setup HTTP server
	mux := http.NewServeMux()
	d.registerHandlers(mux)

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("unheaded-daemon")
	d.authEnabled = authCfg.Enabled
	if !d.authEnabled {
		d.log.Warn().Msg("AUTH_ENABLED is not true: container write API refused (read-only)")
	}
	httpHandler := d.wrapAuth(mux, authCfg)
	httpHandler = httputil.SecurityHeaders(httputil.NewServiceMetrics("cuirass").Instrument(mux, httpHandler))

	d.httpServer = &http.Server{
		Addr:           d.config.HTTPAddr,
		Handler:        httpHandler,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
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

	// Start Wotan connection manager (handles reconnection)
	d.wg.Add(1)
	go d.wotanConnectionLoop()

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

	// Close Wotan client
	d.wotanMu.Lock()
	if d.wotanClient != nil {
		if err := d.wotanClient.Close(); err != nil {
			d.log.Warn().Err(err).Msg("Error closing Wotan client")
		}
		d.wotanClient = nil
		d.wotanReady = false
	}
	d.wotanMu.Unlock()

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
// WOTAN INTEGRATION
// ============================================================================

// initWotan initializes the Wotan client and subscribes to required topics
func (d *Daemon) initWotan() error {
	client, err := wotanClient.NewClient(d.config.WotanAddr)
	if err != nil {
		return fmt.Errorf("create wotan client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Join as "cuirass", the name on configs/wotan.yaml topics.auto_approve.
	// "cuirass-<node id>" embedded the container id, so no allowlist could
	// match it and both subscriptions sat pending: nothing was published.
	// Subscribe to drift topic
	_, err = client.Subscribe(ctx, TopicCuirassDrift, wotanDisplayName)
	if err != nil {
		client.Close() // #nosec G104 -- close on a read/cleanup path; nothing was buffered that a close error could lose
		return fmt.Errorf("subscribe to %s: %w", TopicCuirassDrift, err)
	}

	// Subscribe to metrics topic
	_, err = client.Subscribe(ctx, TopicCuirassMetrics, wotanDisplayName)
	if err != nil {
		client.Close() // #nosec G104 -- close on a read/cleanup path; nothing was buffered that a close error could lose
		return fmt.Errorf("subscribe to %s: %w", TopicCuirassMetrics, err)
	}

	// Join the chaos topic (to publish chaos requests on it)
	_, err = client.Subscribe(ctx, TopicCuirassChaos, wotanDisplayName)
	if err != nil {
		client.Close() // #nosec G104 -- close on a read/cleanup path; nothing was buffered that a close error could lose
		return fmt.Errorf("subscribe to %s: %w", TopicCuirassChaos, err)
	}

	d.wotanMu.Lock()
	d.wotanClient = client
	d.wotanReady = true
	d.wotanMu.Unlock()

	d.log.Info().Str("addr", d.config.WotanAddr).Msg("Wotan connected")
	return nil
}

// wotanConnectionLoop manages Wotan connection with automatic reconnection
func (d *Daemon) wotanConnectionLoop() {
	defer d.wg.Done()

	reconnectInterval := 5 * time.Second
	maxReconnectInterval := 60 * time.Second

	for {
		select {
		case <-d.shutdown:
			return
		default:
		}

		d.wotanMu.RLock()
		isReady := d.wotanReady
		d.wotanMu.RUnlock()

		if !isReady {
			d.log.Info().Str("addr", d.config.WotanAddr).Msg("Attempting Wotan reconnection")
			if err := d.initWotan(); err != nil {
				d.log.Warn().Err(err).Dur("retry_in", reconnectInterval).Msg("Wotan reconnection failed")
				d.healthSrv.SetGRPCStatus(false)

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
			d.healthSrv.SetGRPCStatus(true)
		}

		// Check connection health periodically
		select {
		case <-d.shutdown:
			return
		case <-time.After(30 * time.Second):
		}
	}
}

// metricsPublishLoop publishes metrics to Wotan periodically
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

// publishMetrics publishes current metrics to Wotan
func (d *Daemon) publishMetrics() {
	d.wotanMu.RLock()
	client := d.wotanClient
	ready := d.wotanReady
	d.wotanMu.RUnlock()

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
		if err == wotanClient.ErrNotConnected || err == wotanClient.ErrSubscriptionPending {
			d.wotanMu.Lock()
			d.wotanReady = false
			d.wotanMu.Unlock()
			d.healthSrv.SetGRPCStatus(false)
		}
		d.log.Error().Err(err).Msg("Error publishing metrics to Wotan")
		return
	}

	d.log.Debug().Str("topic", TopicCuirassMetrics).Msg("Published metrics")
}

// publishDriftEvents publishes drift events to Wotan
func (d *Daemon) publishDriftEvents(drifts []DriftReport) {
	if len(drifts) == 0 {
		return
	}

	d.wotanMu.RLock()
	client := d.wotanClient
	ready := d.wotanReady
	d.wotanMu.RUnlock()

	if !ready || client == nil {
		d.log.Warn().Msg("Wotan not ready, skipping drift event publish")
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
		if err == wotanClient.ErrNotConnected || err == wotanClient.ErrSubscriptionPending {
			d.wotanMu.Lock()
			d.wotanReady = false
			d.wotanMu.Unlock()
			d.healthSrv.SetGRPCStatus(false)
		}
		d.log.Error().Err(err).Msg("Error publishing drift events to Wotan")
		return
	}

	d.log.Info().Int("count", len(drifts)).Str("topic", TopicCuirassDrift).Msg("Published drift events")
}

// ============================================================================
// HTTP HANDLERS
// ============================================================================

// wrapAuth puts the auth middleware in front of mux. The read-only state
// endpoints (/api/v1/info, /api/v1/state and below) stay open like /health:
// Sword's readiness ping and steel-echo's discover mode read them without a
// key, and they carry infrastructure state only (never user data). Every
// container write (PUT/DELETE/chaos under /api/v1/containers) and the
// reconcile trigger stay behind the key; the handlers refuse them as well
// when auth is off (authEnabled).
//
// Behind authentication sits RBAC (S-IRON-GATE Phase 5): a write needs role
// admin, operator or service (an EdDSA service token carries service), so an
// observer key can read the keyed endpoints but not change anything.
func (d *Daemon) wrapAuth(mux *http.ServeMux, cfg auth.ServiceAuthConfig) http.Handler {
	authn := auth.SetupMiddleware(cfg)
	if authn == nil {
		return mux
	}
	rbac := auth.RBACMiddleware(auth.DefaultServicePolicies(cfg.ServiceName))
	authz := func(next http.Handler) http.Handler { return authn(rbac(next)) }
	return auth.SkipAuthPaths(authz, "/api/v1/info", "/api/v1/state")(mux)
}

func (d *Daemon) registerHandlers(mux *http.ServeMux) {
	// Health endpoints (transport-based dual-protocol health)
	d.healthSrv.RegisterHTTP(mux)

	// State endpoints
	mux.HandleFunc("/api/v1/state", d.handleGetState)
	mux.HandleFunc("/api/v1/state/desired", d.handleDesiredState)
	mux.HandleFunc("/api/v1/state/actual", d.handleActualState)
	mux.HandleFunc("/api/v1/state/drift", d.handleDrift)

	// Container endpoints
	mux.HandleFunc("/api/v1/containers", d.handleContainers)
	mux.HandleFunc("/api/v1/containers/", d.handleContainer)
	mux.HandleFunc("/api/v1/reconcile", d.handleReconcileNow)

	// Metrics
	mux.HandleFunc("/metrics", d.handleMetrics)

	// (No /health or /ready here: healthSrv.RegisterHTTP above already owns
	// both, and registering either twice makes http.ServeMux panic — the daemon
	// died on every start. Same resolution as micromanager. The removed local
	// handlers reported a hardcoded "status":"healthy" that could never go
	// unhealthy; the transport version reports real dual-protocol state. If
	// richer probe logic is wanted, add it to healthSrv rather than here.)

	// Info
	mux.HandleFunc("/api/v1/info", d.handleInfo)
}

func (d *Daemon) handleGetState(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
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
	json.NewEncoder(w).Encode(d.stateManager.desired) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

func (d *Daemon) handleActualState(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.stateManager.actual) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

func (d *Daemon) handleDrift(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.stateManager.drifts) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
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
	json.NewEncoder(w).Encode(containers) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

func (d *Daemon) handleContainer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract container ID from URL path: /api/v1/containers/{id}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/containers/")
	if target, ok := strings.CutSuffix(id, "/chaos"); ok {
		d.handleChaos(w, r, target)
		return
	}
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"error": "container id is required",
		})
		return
	}

	if (r.Method == http.MethodPut || r.Method == http.MethodDelete) && !d.authEnabled {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"error": "container writes need AUTH_ENABLED=true",
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
			json.NewEncoder(w).Encode(map[string]string{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
				"error": "container not found",
			})
			return
		}

		d.log.Info().Str("id", id).Msg("Retrieved container state")
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
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
			json.NewEncoder(w).Encode(map[string]string{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
				"error": fmt.Sprintf("invalid request body: %v", err),
			})
			return
		}

		// Ensure the spec ID matches the URL path ID; it becomes the LXD
		// instance name.
		if err := validLXDName(id); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			return
		}
		spec.ID = id
		if err := spec.validate(d.config.ProxyListenAddrs); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			return
		}

		d.stateManager.mu.Lock()
		if owner := d.proxyListenOwner(id, spec.Proxy); owner != "" {
			d.stateManager.mu.Unlock()
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
				"error": fmt.Sprintf("proxy listen %s is already %s's", spec.Proxy.Listen, owner),
			})
			return
		}
		next := cloneDesired(d.stateManager.desired)
		next[id] = &spec
		err := d.commitDesired(next)
		d.stateManager.mu.Unlock()
		if err != nil {
			d.writeStoreError(w, id, err)
			return
		}

		d.log.Info().Str("id", id).Str("name", spec.Name).Str("image", spec.Image).Msg("Updated desired state for container")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
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
			json.NewEncoder(w).Encode(map[string]string{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
				"error": "container not found in desired state",
			})
			return
		}
		next := cloneDesired(d.stateManager.desired)
		delete(next, id)
		err := d.commitDesired(next)
		d.stateManager.mu.Unlock()
		if err != nil {
			d.writeStoreError(w, id, err)
			return
		}

		d.log.Info().Str("id", id).Msg("Removed container from desired state, reconciliation will clean up")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"message": "container removed from desired state",
			"id":      id,
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"error": fmt.Sprintf("method %s not allowed", r.Method),
		})
	}
}

// proxyListenOwner returns the other desired container whose proxy already
// listens where p would, or ""; the caller holds stateManager.mu.
func (d *Daemon) proxyListenOwner(id string, p *ProxySpec) string {
	if p == nil {
		return ""
	}
	for other, spec := range d.stateManager.desired {
		if other != id && spec.Proxy != nil && spec.Proxy.Listen == p.Listen {
			return other
		}
	}
	return ""
}

var errNoStateStore = errors.New("no state dir: desired state cannot be persisted, so container writes are refused")

// commitDesired persists next as the desired state, then adopts it; the
// caller holds stateManager.mu. With no store, or a failed write, the
// desired state is unchanged.
func (d *Daemon) commitDesired(next map[string]*ContainerSpec) error {
	if d.store == nil {
		return errNoStateStore
	}
	if err := d.store.save(next); err != nil {
		return fmt.Errorf("persist desired state: %w", err)
	}
	d.stateManager.desired = next
	return nil
}

func cloneDesired(m map[string]*ContainerSpec) map[string]*ContainerSpec {
	out := make(map[string]*ContainerSpec, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (d *Daemon) writeStoreError(w http.ResponseWriter, id string, err error) {
	d.log.Error().Err(err).Str("id", id).Msg("Desired state not changed")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

func (d *Daemon) handleMetrics(w http.ResponseWriter, r *http.Request) {
	d.stateManager.mu.RLock()
	defer d.stateManager.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	// A Daemon built as a struct literal has no registry. Guard rather than
	// dereference: a nil-pointer panic here takes the whole endpoint down.
	// #nosec G104 -- response already committed; a write failure here means
	// the client went away and nothing further can be sent.
	//
	// The default registry rides along: go_*, process_* and what linked
	// libraries register at load (pkg/wotan-client) never reached this
	// scrape before ADR-094 step 5. GatherAll skips a nil registry.
	// The log publisher's counters are in d.registry (SetLogPublisher).
	_ = metrics.GatherAll(w, d.registry, metrics.DefaultRegistry)
}

// SetLogPublisher attaches a logagg publisher and registers its counters in
// the registry /metrics serves, replacing a previous publisher's. Optional:
// nil removes them.
func (d *Daemon) SetLogPublisher(p *logagg.Publisher) error {
	d.logPublisherMu.Lock()
	defer d.logPublisherMu.Unlock()
	if old := d.logPublisher; old != nil {
		old.UnregisterCollectors(d.registry)
		d.logPublisher = nil
	}
	if p == nil {
		return nil
	}
	if err := p.RegisterCollectors(d.registry); err != nil {
		return err
	}
	d.logPublisher = p
	return nil
}

func (d *Daemon) handleInfo(w http.ResponseWriter, r *http.Request) {
	backend := d.config.LXDBackend
	if backend == "" {
		backend = "mock"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		// Which LXD and which modes are live: mock and real look alike
		// otherwise without reading the environment.
		"lxd_backend": backend,
		"lxd_project": d.config.LXDProject,
		"enforce":     d.config.Enforce,
		"prune":       d.config.Prune,
		"chaos":       d.config.Chaos,
		"component":   "cuirass",
		"version":     version,
		"git_commit":  gitCommit,
		"build_time":  buildTime,
		"node_id":     d.config.NodeID,
		"node_name":   d.config.NodeName,
		"hollow":      "crystal_grotto",
		"role":        "control_plane",
		"kingdom":     "unheaded",
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
	_, _ = d.reconcileOnce()
}

// reconcileOnce runs one sync, plan and apply cycle and returns what it
// decided. The error is set when the cycle could not run (no LXD client,
// LXD list failed).
func (d *Daemon) reconcileOnce() ([]ReconcileAction, error) {
	d.stateManager.mu.Lock()
	defer d.stateManager.mu.Unlock()

	// Check if LXD client is available
	if d.lxdClient == nil {
		d.log.Warn().Msg("LXD client not available, skipping reconciliation")
		return nil, errors.New("no LXD client")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := d.syncActualStateFromLXD(ctx); err != nil {
		d.log.Error().Err(err).Msg("LXD list failed; skipping reconcile (actual state unknown)")
		return nil, fmt.Errorf("LXD list failed: %w", err)
	}
	d.probeHealth(ctx)
	actions := proxyRemovalsFirst(d.planReconcile())
	for i := range actions {
		d.applyAction(ctx, &actions[i])
	}
	if err := d.syncActualStateFromLXD(ctx); err != nil {
		d.log.Error().Err(err).Msg("LXD list after reconcile failed")
	}

	// Publish reconciliation events if any actions were decided
	if len(actions) > 0 {
		go d.publishReconcileEvents(actions)
	}
	return actions, nil
}

// planReconcile decides what reconciliation would do; the caller holds
// stateManager.mu. It never plans to touch an unmanaged instance: one that
// shares a desired ID is refused, one nobody desires is left alone, and a
// managed one nobody desires is deleted only when pruning.
func (d *Daemon) planReconcile() []ReconcileAction {
	var actions []ReconcileAction
	plan := func(id, action, reason string) *ReconcileAction {
		actions = append(actions, ReconcileAction{ContainerID: id, Action: action, Reason: reason, Timestamp: time.Now()})
		return &actions[len(actions)-1]
	}
	ids := make([]string, 0, len(d.stateManager.desired))
	for id := range d.stateManager.desired {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		desired := d.stateManager.desired[id]
		actual, exists := d.stateManager.actual[id]
		switch {
		case !exists:
			plan(id, "create", "container missing from actual state").spec = desired
		case !actual.Managed:
			a := plan(id, "refuse", "an instance with this name exists and is not Unheaded's (no "+ManagedLabel+"=true)")
			a.Error = "not managed by Unheaded; not touching it"
		case actual.Status != "running":
			plan(id, "start", fmt.Sprintf("container status is '%s', expected 'running'", actual.Status))
		case actual.Health == "unhealthy":
			plan(id, "restart", "container health status is 'unhealthy'")
		}
		if exists && actual.Managed && !proxyEqual(desired.Proxy, actual.Proxy) {
			plan(id, "update-proxy", "unheaded-proxy device differs from the desired one").spec = desired
		}
	}
	if !d.config.Prune {
		return actions
	}
	orphans := make([]string, 0)
	for id, actual := range d.stateManager.actual {
		if _, desired := d.stateManager.desired[id]; !desired && actual.Managed {
			orphans = append(orphans, id)
		}
	}
	sort.Strings(orphans)
	for _, id := range orphans {
		plan(id, "delete", "managed container is not in desired state (prune)")
	}
	return actions
}

// applyAction takes one planned action, or in observe mode only records
// that it would have.
func (d *Daemon) applyAction(ctx context.Context, a *ReconcileAction) {
	if a.Error != "" { // refused while planning
		d.countAction(a.Action, "refused")
		d.log.Warn().Str("id", a.ContainerID).Str("reason", a.Reason).Msg("Reconcile refused")
		return
	}
	if !d.config.Enforce {
		d.countAction(a.Action, "planned")
		a.Planned = true
		d.log.Info().Str("id", a.ContainerID).Str("action", a.Action).Str("reason", a.Reason).
			Msg("Observe mode: would act (CUIRASS_ENFORCE is not true)")
		return
	}
	var err error
	switch a.Action {
	case "create":
		if err = d.lxdClient.CreateContainer(ctx, a.spec); err == nil {
			if err = d.lxdClient.StartContainer(ctx, a.ContainerID); err != nil {
				err = fmt.Errorf("created but failed to start: %w", err)
			}
		}
	case "start":
		err = d.lxdClient.StartContainer(ctx, a.ContainerID)
	case "restart":
		err = d.lxdClient.RestartContainer(ctx, a.ContainerID)
	case "delete":
		err = d.lxdClient.DeleteContainer(ctx, a.ContainerID, true)
	case "update-proxy":
		if a.spec == nil {
			err = errors.New("update-proxy without a spec")
		} else {
			err = d.lxdClient.UpdateProxy(ctx, a.ContainerID, a.spec.Proxy)
		}
	default:
		err = fmt.Errorf("unknown action %q", a.Action)
	}
	if err != nil {
		d.countAction(a.Action, "failed")
		d.log.Error().Err(err).Str("id", a.ContainerID).Str("action", a.Action).Msg("Reconcile action failed")
		a.Error = err.Error()
		return
	}
	d.countAction(a.Action, "done")
	d.log.Info().Str("id", a.ContainerID).Str("action", a.Action).Msg("Reconcile action done")
	a.Success = true
	if a.Action == "create" || a.Action == "restart" || a.Action == "delete" {
		d.resetHealth(a.ContainerID)
	}
}

// syncActualStateFromLXD updates the actual state from the LXD client. On
// error it keeps the previous state: an unreachable LXD is not an empty one.
func (d *Daemon) syncActualStateFromLXD(ctx context.Context) error {
	if d.lxdClient == nil {
		return errors.New("no LXD client")
	}
	actual, err := d.lxdClient.ListContainers(ctx)
	d.lxdUp.Store(err == nil)
	if err != nil {
		return err
	}
	// Replace, not merge: an instance deleted outside Cuirass must leave
	// the actual state too.
	d.stateManager.actual = actual
	// LXD reports no health; carry over the probe verdicts.
	for id, rec := range d.health {
		if a, ok := actual[id]; ok && rec.status != "" {
			a.Health = rec.status
		}
	}
	return nil
}

// publishReconcileEvents publishes reconciliation action events to Wotan
func (d *Daemon) publishReconcileEvents(actions []ReconcileAction) {
	d.wotanMu.RLock()
	client := d.wotanClient
	ready := d.wotanReady
	d.wotanMu.RUnlock()

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
		if err == wotanClient.ErrNotConnected || err == wotanClient.ErrSubscriptionPending {
			d.wotanMu.Lock()
			d.wotanReady = false
			d.wotanMu.Unlock()
			d.healthSrv.SetGRPCStatus(false)
		}
		d.log.Error().Err(err).Msg("Error publishing reconcile events to Wotan")
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := d.syncActualStateFromLXD(ctx); err != nil {
		d.log.Error().Err(err).Msg("LXD list failed; drift not re-evaluated")
		return
	}

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

	// Check for orphaned containers (actual but not desired). Only managed
	// ones: the rest are simply not ours.
	for id, actual := range d.stateManager.actual {
		if _, exists := d.stateManager.desired[id]; !exists && actual.Managed {
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
			if !actual.Managed {
				drifts = append(drifts, DriftReport{
					ContainerID: id,
					DriftType:   "unmanaged",
					Severity:    "high",
					DetectedAt:  time.Now(),
				})
				continue
			}
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
		// Publish drift events to Wotan
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
	NodeID        string `yaml:"node_id"`
	NodeName      string `yaml:"node_name"`
	HTTPAddr      string `yaml:"http_addr"`
	GRPCAddr      string `yaml:"grpc_addr"`
	LXDBackend    string `yaml:"lxd_backend"`
	LXDSocket     string `yaml:"lxd_socket"`
	LXDURL        string `yaml:"lxd_url"`
	LXDProject    string `yaml:"lxd_project"`
	LXDClientCert string `yaml:"lxd_client_cert"`
	LXDClientKey  string `yaml:"lxd_client_key"`
	LXDServerCert string `yaml:"lxd_server_cert"`
	StateDir      string `yaml:"state_dir"`
	WotanAddr     string `yaml:"wotan_addr"`
	WotanGRPCAddr string `yaml:"wotan_grpc_addr"`
	PollInterval  string `yaml:"poll_interval"`
	LogLevel      string `yaml:"log_level"`
	Enforce       *bool  `yaml:"enforce"`
	Prune         *bool  `yaml:"prune"`
}

func loadConfig(path string) *Config {
	// Start with defaults from environment variables
	cfg := &Config{
		NodeID:           getEnvOrDefault("UNHEADED_NODE_ID", fmt.Sprintf("citadel-%s", getHostname())),
		NodeName:         getEnvOrDefault("UNHEADED_NODE_NAME", getHostname()),
		HTTPAddr:         getEnvOrDefault("HTTP_ADDR", ports.DefaultAddr(ports.DaemonHTTP)),
		GRPCAddr:         getEnvOrDefault("GRPC_ADDR", ports.DefaultAddr(ports.DaemonGRPC)),
		LXDBackend:       getEnvOrDefault("LXD_BACKEND", "mock"),
		LXDSocket:        getEnvOrDefault("LXD_SOCKET", "/var/snap/lxd/common/lxd/unix.socket"),
		LXDURL:           os.Getenv("LXD_URL"),
		LXDProject:       os.Getenv("LXD_PROJECT"),
		LXDClientCert:    os.Getenv("LXD_CLIENT_CERT"),
		LXDClientKey:     os.Getenv("LXD_CLIENT_KEY"),
		LXDServerCert:    os.Getenv("LXD_SERVER_CERT"),
		StateDir:         getEnvOrDefault("CUIRASS_STATE_DIR", "/var/lib/unheaded/cuirass"),
		WotanAddr:        getEnvOrDefault("WOTAN_ADDR", fmt.Sprintf("localhost:%d", ports.WotanHTTP)), // HTTP; 18001 is gRPC
		WotanGRPCAddr:    getEnvOrDefault("WOTAN_GRPC_ADDR", ""),
		PollInterval:     30 * time.Second,
		LogLevel:         getEnvOrDefault("LOG_LEVEL", "info"),
		Enforce:          os.Getenv("CUIRASS_ENFORCE") == "true",
		Prune:            os.Getenv("CUIRASS_PRUNE") == "true",
		Chaos:            os.Getenv("CUIRASS_CHAOS") == "true",
		ProxyListenAddrs: parseIPList(os.Getenv("CUIRASS_PROXY_LISTEN_ADDRS")),
	}
	subnets, err := probeSubnetsFromEnv(os.Getenv("CUIRASS_PROBE_SUBNETS"))
	if err != nil {
		// Fail loud: a bad list must not become "probe anything".
		fmt.Fprintf(os.Stderr, "Error: CUIRASS_PROBE_SUBNETS: %v\n", err)
		os.Exit(1)
	}
	cfg.ProbeSubnets = subnets

	// Load from YAML file if path provided, overriding defaults
	if path != "" {
		data, err := os.ReadFile(path) // #nosec G304 -- operator-configured path; no G304 site in this tree derives from an HTTP request (verified)
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
		for _, o := range []struct {
			dst *string
			v   string
		}{
			{&cfg.LXDBackend, yc.LXDBackend}, {&cfg.LXDSocket, yc.LXDSocket}, {&cfg.LXDURL, yc.LXDURL},
			{&cfg.LXDProject, yc.LXDProject}, {&cfg.LXDClientCert, yc.LXDClientCert},
			{&cfg.LXDClientKey, yc.LXDClientKey}, {&cfg.LXDServerCert, yc.LXDServerCert},
			{&cfg.StateDir, yc.StateDir},
		} {
			if o.v != "" {
				*o.dst = o.v
			}
		}
		if yc.WotanAddr != "" {
			cfg.WotanAddr = yc.WotanAddr
		}
		if yc.WotanGRPCAddr != "" {
			cfg.WotanGRPCAddr = yc.WotanGRPCAddr
		}
		if yc.LogLevel != "" {
			cfg.LogLevel = yc.LogLevel
		}
		if yc.Enforce != nil {
			cfg.Enforce = *yc.Enforce
		}
		if yc.Prune != nil {
			cfg.Prune = *yc.Prune
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

// probeSubnetsFromEnv parses CUIRASS_PROBE_SUBNETS (empty: the default).
func probeSubnetsFromEnv(v string) ([]netip.Prefix, error) {
	return probenet.ParseCSV(v)
}

// parseIPList keeps the comma-separated entries that are specific IP
// literals; names, wildcards and garbage are dropped (a proxy must never
// listen on every interface).
func parseIPList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if ip := net.ParseIP(f); ip != nil && !ip.IsUnspecified() {
			out = append(out, ip.String())
		} else if f != "" {
			fmt.Fprintf(os.Stderr, "Warning: CUIRASS_PROXY_LISTEN_ADDRS entry %q is not a specific IP; ignored\n", f)
		}
	}
	return out
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
