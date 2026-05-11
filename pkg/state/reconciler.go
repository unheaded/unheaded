// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package state provides the State Reconciliation Engine for the Cuirass control plane.
// This is the core of the control plane - detecting drift between desired and actual state,
// then reconciling to bring the system back to the desired state.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrReconcilerClosed    = errors.New("reconciler is closed")
	ErrInvalidDesiredState = errors.New("invalid desired state")
	ErrInvalidActualState  = errors.New("invalid actual state")
	ErrActionFailed        = errors.New("reconciliation action failed")
	ErrRateLimited         = errors.New("rate limited")
	ErrDryRunMode          = errors.New("action skipped in dry-run mode")
	ErrLXDClientNil        = errors.New("LXD client is nil")
	ErrWotanClientNil      = errors.New("wotan client is nil")
	ErrInvalidStatePath    = errors.New("invalid state file path")
	ErrContainerNotFound   = errors.New("container not found")
	ErrMaxRetriesExceeded  = errors.New("max retries exceeded")
)

// ============================================================================
// CONTAINER TYPES
// ============================================================================

// ContainerStatus represents the status of a container
type ContainerStatus string

const (
	StatusPending  ContainerStatus = "pending"
	StatusCreating ContainerStatus = "creating"
	StatusRunning  ContainerStatus = "running"
	StatusStopped  ContainerStatus = "stopped"
	StatusError    ContainerStatus = "error"
	StatusDegraded ContainerStatus = "degraded"
	StatusDeleting ContainerStatus = "deleting"
	StatusNotFound ContainerStatus = "not_found"
)

// HealthStatus represents the health of a container
type HealthStatus string

const (
	HealthUnknown   HealthStatus = "unknown"
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthStarting  HealthStatus = "starting"
)

// ============================================================================
// DESIRED STATE TYPES
// ============================================================================

// DesiredState represents the complete desired state of the system
type DesiredState struct {
	Version     string                       `json:"version" yaml:"version"`
	Name        string                       `json:"name" yaml:"name"`
	Description string                       `json:"description,omitempty" yaml:"description,omitempty"`
	Containers  map[string]*ContainerDesired `json:"containers" yaml:"containers"`
	Networks    map[string]*NetworkDesired   `json:"networks,omitempty" yaml:"networks,omitempty"`
	CreatedAt   time.Time                    `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time                    `json:"updated_at" yaml:"updated_at"`
}

// ContainerDesired represents the desired state of a single container
type ContainerDesired struct {
	Name          string            `json:"name" yaml:"name"`
	Image         string            `json:"image" yaml:"image"`
	Type          string            `json:"type" yaml:"type"` // service, gateway, worker
	Enabled       bool              `json:"enabled" yaml:"enabled"`
	CPU           int               `json:"cpu_cores" yaml:"cpu_cores"`
	Memory        int64             `json:"memory_mb" yaml:"memory_mb"`
	DiskSize      int64             `json:"disk_size_gb,omitempty" yaml:"disk_size_gb,omitempty"`
	NetworkMode   string            `json:"network_mode" yaml:"network_mode"`
	Network       string            `json:"network,omitempty" yaml:"network,omitempty"`
	IPAddress     string            `json:"ip_address,omitempty" yaml:"ip_address,omitempty"`
	Ports         []PortMapping     `json:"ports,omitempty" yaml:"ports,omitempty"`
	Env           map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Labels        map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Volumes       []VolumeMount     `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	HealthCheck   *HealthCheck      `json:"health_check,omitempty" yaml:"health_check,omitempty"`
	Dependencies  []string          `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	RestartPolicy string            `json:"restart_policy,omitempty" yaml:"restart_policy,omitempty"`
	Priority      int               `json:"priority,omitempty" yaml:"priority,omitempty"`
}

// PortMapping represents a port mapping
type PortMapping struct {
	ContainerPort int    `json:"container_port" yaml:"container_port"`
	HostPort      int    `json:"host_port,omitempty" yaml:"host_port,omitempty"`
	Protocol      string `json:"protocol" yaml:"protocol"` // tcp, udp
}

// VolumeMount represents a volume mount
type VolumeMount struct {
	Source   string `json:"source" yaml:"source"`
	Target   string `json:"target" yaml:"target"`
	ReadOnly bool   `json:"read_only,omitempty" yaml:"read_only,omitempty"`
}

// HealthCheck represents a health check configuration
type HealthCheck struct {
	Endpoint string        `json:"endpoint" yaml:"endpoint"`
	Interval time.Duration `json:"interval" yaml:"interval"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout"`
	Retries  int           `json:"retries" yaml:"retries"`
}

// NetworkDesired represents the desired state of a network
type NetworkDesired struct {
	Name    string `json:"name" yaml:"name"`
	Subnet  string `json:"subnet" yaml:"subnet"`
	Gateway string `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	Bridge  string `json:"bridge,omitempty" yaml:"bridge,omitempty"`
}

// Validate validates the desired state
func (d *DesiredState) Validate() error {
	if d == nil {
		return ErrInvalidDesiredState
	}
	if d.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidDesiredState)
	}
	if len(d.Containers) == 0 {
		return fmt.Errorf("%w: at least one container is required", ErrInvalidDesiredState)
	}
	for id, container := range d.Containers {
		if err := container.Validate(id); err != nil {
			return err
		}
	}
	return nil
}

// Validate validates a container desired state
func (c *ContainerDesired) Validate(id string) error {
	if c == nil {
		return fmt.Errorf("%w: container %s is nil", ErrInvalidDesiredState, id)
	}
	if c.Name == "" {
		return fmt.Errorf("%w: container %s name is required", ErrInvalidDesiredState, id)
	}
	if c.Image == "" {
		return fmt.Errorf("%w: container %s image is required", ErrInvalidDesiredState, id)
	}
	if c.CPU < 0 {
		return fmt.Errorf("%w: container %s CPU cannot be negative", ErrInvalidDesiredState, id)
	}
	if c.Memory < 0 {
		return fmt.Errorf("%w: container %s memory cannot be negative", ErrInvalidDesiredState, id)
	}
	return nil
}

// ============================================================================
// ACTUAL STATE TYPES
// ============================================================================

// ActualState represents the current observed state of the system
type ActualState struct {
	Containers map[string]*ContainerActual `json:"containers"`
	Networks   map[string]*NetworkActual   `json:"networks,omitempty"`
	ObservedAt time.Time                   `json:"observed_at"`
}

// ContainerActual represents the actual observed state of a container
type ContainerActual struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Status      ContainerStatus    `json:"status"`
	Image       string             `json:"image"`
	IPAddress   string             `json:"ip_address"`
	CPU         int                `json:"cpu_cores"`
	Memory      int64              `json:"memory_mb"`
	CPUUsage    float64            `json:"cpu_usage_percent"`
	MemoryUsage int64              `json:"memory_usage_mb"`
	DiskUsage   int64              `json:"disk_usage_mb"`
	Uptime      time.Duration      `json:"uptime"`
	Health      HealthStatus       `json:"health"`
	Ports       []PortMapping      `json:"ports,omitempty"`
	Env         map[string]string  `json:"env,omitempty"`
	Labels      map[string]string  `json:"labels,omitempty"`
	LastSeen    time.Time          `json:"last_seen"`
	CreatedAt   time.Time          `json:"created_at"`
	Metrics     map[string]float64 `json:"metrics,omitempty"`
}

// NetworkActual represents the actual observed state of a network
type NetworkActual struct {
	Name    string   `json:"name"`
	Subnet  string   `json:"subnet"`
	Gateway string   `json:"gateway"`
	Bridge  string   `json:"bridge"`
	Members []string `json:"members"`
}

// ============================================================================
// DRIFT DETECTION TYPES
// ============================================================================

// DriftType represents the type of configuration drift
type DriftType string

const (
	DriftNone           DriftType = "none"
	DriftMissing        DriftType = "missing"         // Desired but not present
	DriftOrphaned       DriftType = "orphaned"        // Present but not desired
	DriftStatusMismatch DriftType = "status_mismatch" // Wrong status (stopped vs running)
	DriftConfigMismatch DriftType = "config_mismatch" // Configuration differs
	DriftHealthDegraded DriftType = "health_degraded" // Unhealthy
	DriftResourceDrift  DriftType = "resource_drift"  // CPU/Memory mismatch
	DriftNetworkDrift   DriftType = "network_drift"   // Network configuration differs
)

// DriftSeverity represents the severity of drift
type DriftSeverity string

const (
	SeverityLow      DriftSeverity = "low"
	SeverityMedium   DriftSeverity = "medium"
	SeverityHigh     DriftSeverity = "high"
	SeverityCritical DriftSeverity = "critical"
)

// DriftItem represents a single drift detection
type DriftItem struct {
	ContainerID string            `json:"container_id"`
	DriftType   DriftType         `json:"drift_type"`
	Severity    DriftSeverity     `json:"severity"`
	Field       string            `json:"field,omitempty"`
	Expected    interface{}       `json:"expected,omitempty"`
	Actual      interface{}       `json:"actual,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
	DetectedAt  time.Time         `json:"detected_at"`
}

// DriftReport represents the complete drift analysis
type DriftReport struct {
	ID              string        `json:"id"`
	DesiredState    string        `json:"desired_state_version"`
	TotalContainers int           `json:"total_containers"`
	DriftCount      int           `json:"drift_count"`
	Drifts          []DriftItem   `json:"drifts"`
	Summary         DriftSummary  `json:"summary"`
	DetectedAt      time.Time     `json:"detected_at"`
	Duration        time.Duration `json:"duration"`
}

// DriftSummary provides a summary of drift
type DriftSummary struct {
	Missing        int `json:"missing"`
	Orphaned       int `json:"orphaned"`
	StatusMismatch int `json:"status_mismatch"`
	ConfigMismatch int `json:"config_mismatch"`
	HealthDegraded int `json:"health_degraded"`
	ResourceDrift  int `json:"resource_drift"`
	CriticalCount  int `json:"critical_count"`
	HighCount      int `json:"high_count"`
	MediumCount    int `json:"medium_count"`
	LowCount       int `json:"low_count"`
}

// ============================================================================
// RECONCILIATION ACTION TYPES
// ============================================================================

// ActionType represents the type of reconciliation action
type ActionType string

const (
	ActionCreate  ActionType = "create"
	ActionStart   ActionType = "start"
	ActionStop    ActionType = "stop"
	ActionRestart ActionType = "restart"
	ActionDelete  ActionType = "delete"
	ActionUpdate  ActionType = "update"
	ActionRepair  ActionType = "repair"
	ActionScale   ActionType = "scale"
)

// ActionStatus represents the status of an action
type ActionStatus string

const (
	ActionStatusPending    ActionStatus = "pending"
	ActionStatusInProgress ActionStatus = "in_progress"
	ActionStatusCompleted  ActionStatus = "completed"
	ActionStatusFailed     ActionStatus = "failed"
	ActionStatusSkipped    ActionStatus = "skipped"
	ActionStatusRetrying   ActionStatus = "retrying"
)

// ReconciliationAction represents a single action to reconcile drift
type ReconciliationAction struct {
	ID          string                 `json:"id"`
	ContainerID string                 `json:"container_id"`
	ActionType  ActionType             `json:"action_type"`
	Priority    int                    `json:"priority"`
	DriftItem   *DriftItem             `json:"drift_item,omitempty"`
	Params      map[string]interface{} `json:"params,omitempty"`
	Status      ActionStatus           `json:"status"`
	Retries     int                    `json:"retries"`
	MaxRetries  int                    `json:"max_retries"`
	Error       string                 `json:"error,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Duration    time.Duration          `json:"duration,omitempty"`
}

// ActionResult represents the result of executing an action
type ActionResult struct {
	ActionID    string        `json:"action_id"`
	Success     bool          `json:"success"`
	Error       error         `json:"error,omitempty"`
	Duration    time.Duration `json:"duration"`
	RetryCount  int           `json:"retry_count"`
	CompletedAt time.Time     `json:"completed_at"`
}

// ============================================================================
// RECONCILIATION HISTORY
// ============================================================================

// ReconciliationRun represents a single reconciliation run
type ReconciliationRun struct {
	ID             string                  `json:"id"`
	StartedAt      time.Time               `json:"started_at"`
	CompletedAt    *time.Time              `json:"completed_at,omitempty"`
	Duration       time.Duration           `json:"duration,omitempty"`
	DriftReport    *DriftReport            `json:"drift_report"`
	Actions        []*ReconciliationAction `json:"actions"`
	Results        []*ActionResult         `json:"results"`
	DryRun         bool                    `json:"dry_run"`
	Success        bool                    `json:"success"`
	Error          string                  `json:"error,omitempty"`
	ActionsTotal   int                     `json:"actions_total"`
	ActionsSuccess int                     `json:"actions_success"`
	ActionsFailed  int                     `json:"actions_failed"`
	ActionsSkipped int                     `json:"actions_skipped"`
}

// ============================================================================
// LXD CLIENT INTERFACE
// ============================================================================

// LXDClient defines the interface for LXD operations
type LXDClient interface {
	// Container operations
	ListContainers(ctx context.Context) ([]*ContainerActual, error)
	GetContainer(ctx context.Context, name string) (*ContainerActual, error)
	CreateContainer(ctx context.Context, spec *ContainerDesired) error
	StartContainer(ctx context.Context, name string) error
	StopContainer(ctx context.Context, name string, force bool) error
	RestartContainer(ctx context.Context, name string) error
	DeleteContainer(ctx context.Context, name string, force bool) error
	UpdateContainer(ctx context.Context, name string, spec *ContainerDesired) error

	// Health operations
	CheckHealth(ctx context.Context, name string, check *HealthCheck) (HealthStatus, error)

	// Network operations
	ListNetworks(ctx context.Context) ([]*NetworkActual, error)
	CreateNetwork(ctx context.Context, spec *NetworkDesired) error
	DeleteNetwork(ctx context.Context, name string) error
}

// ============================================================================
// EVENT TYPES FOR WOTAN
// ============================================================================

// EventType represents the type of reconciliation event
type EventType string

const (
	EventDriftDetected      EventType = "drift.detected"
	EventReconcileStarted   EventType = "reconcile.started"
	EventReconcileCompleted EventType = "reconcile.completed"
	EventReconcileFailed    EventType = "reconcile.failed"
	EventActionStarted      EventType = "action.started"
	EventActionCompleted    EventType = "action.completed"
	EventActionFailed       EventType = "action.failed"
	EventContainerCreated   EventType = "container.created"
	EventContainerStarted   EventType = "container.started"
	EventContainerStopped   EventType = "container.stopped"
	EventContainerDeleted   EventType = "container.deleted"
	EventContainerHealthy   EventType = "container.healthy"
	EventContainerUnhealthy EventType = "container.unhealthy"
)

// ReconciliationEvent represents an event to publish to Wotan
type ReconciliationEvent struct {
	Type        EventType              `json:"type"`
	Timestamp   time.Time              `json:"timestamp"`
	ReconcileID string                 `json:"reconcile_id,omitempty"`
	ContainerID string                 `json:"container_id,omitempty"`
	ActionID    string                 `json:"action_id,omitempty"`
	Severity    DriftSeverity          `json:"severity,omitempty"`
	Message     string                 `json:"message"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// WotanClient defines the interface for event publishing
type WotanClient interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// ============================================================================
// METRICS
// ============================================================================

// ReconcilerMetrics holds Prometheus metrics
type ReconcilerMetrics struct {
	mu sync.RWMutex

	// Drift metrics
	DriftDetectedTotal int64                   `json:"drift_detected_total"`
	DriftByType        map[DriftType]int64     `json:"drift_by_type"`
	DriftBySeverity    map[DriftSeverity]int64 `json:"drift_by_severity"`
	LastDriftCount     int                     `json:"last_drift_count"`
	LastDriftCheckTime time.Time               `json:"last_drift_check_time"`
	DriftCheckDuration time.Duration           `json:"drift_check_duration"`

	// Action metrics
	ActionsExecutedTotal int64                `json:"actions_executed_total"`
	ActionsSuccessTotal  int64                `json:"actions_success_total"`
	ActionsFailedTotal   int64                `json:"actions_failed_total"`
	ActionsSkippedTotal  int64                `json:"actions_skipped_total"`
	ActionsByType        map[ActionType]int64 `json:"actions_by_type"`
	ActionDuration       time.Duration        `json:"action_duration"`

	// Reconciliation metrics
	ReconcileTotal        int64         `json:"reconcile_total"`
	ReconcileSuccessTotal int64         `json:"reconcile_success_total"`
	ReconcileFailedTotal  int64         `json:"reconcile_failed_total"`
	ReconcileDuration     time.Duration `json:"reconcile_duration"`
	LastReconcileTime     time.Time     `json:"last_reconcile_time"`

	// Container metrics
	ContainersDesired   int `json:"containers_desired"`
	ContainersActual    int `json:"containers_actual"`
	ContainersHealthy   int `json:"containers_healthy"`
	ContainersUnhealthy int `json:"containers_unhealthy"`
}

// NewReconcilerMetrics creates a new metrics instance
func NewReconcilerMetrics() *ReconcilerMetrics {
	return &ReconcilerMetrics{
		DriftByType:     make(map[DriftType]int64),
		DriftBySeverity: make(map[DriftSeverity]int64),
		ActionsByType:   make(map[ActionType]int64),
	}
}

// ============================================================================
// RATE LIMITER
// ============================================================================

// RateLimiter controls the rate of reconciliation actions
type RateLimiter struct {
	mu           sync.Mutex
	maxPerSecond float64
	tokens       float64
	lastUpdate   time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxPerSecond float64) *RateLimiter {
	return &RateLimiter{
		maxPerSecond: maxPerSecond,
		tokens:       maxPerSecond,
		lastUpdate:   time.Now(),
	}
}

// Allow checks if an action is allowed and consumes a token
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastUpdate).Seconds()
	r.tokens += elapsed * r.maxPerSecond
	if r.tokens > r.maxPerSecond {
		r.tokens = r.maxPerSecond
	}
	r.lastUpdate = now

	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

// Wait waits until an action is allowed
func (r *RateLimiter) Wait(ctx context.Context) error {
	for {
		if r.Allow() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}
}

// ============================================================================
// RECONCILER CONFIGURATION
// ============================================================================

// ReconcilerConfig holds configuration for the reconciler
type ReconcilerConfig struct {
	// Polling configuration
	PollInterval        time.Duration `json:"poll_interval" yaml:"poll_interval"`
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`

	// Rate limiting
	MaxActionsPerSecond  float64 `json:"max_actions_per_second" yaml:"max_actions_per_second"`
	MaxConcurrentActions int     `json:"max_concurrent_actions" yaml:"max_concurrent_actions"`

	// Retry configuration
	MaxRetries    int           `json:"max_retries" yaml:"max_retries"`
	RetryBackoff  time.Duration `json:"retry_backoff" yaml:"retry_backoff"`
	ActionTimeout time.Duration `json:"action_timeout" yaml:"action_timeout"`

	// History configuration
	MaxHistorySize int `json:"max_history_size" yaml:"max_history_size"`

	// Event topics
	EventTopic string `json:"event_topic" yaml:"event_topic"`

	// Logging
	LogLevel string `json:"log_level" yaml:"log_level"`
}

// DefaultReconcilerConfig returns sensible defaults
func DefaultReconcilerConfig() ReconcilerConfig {
	return ReconcilerConfig{
		PollInterval:         30 * time.Second,
		HealthCheckInterval:  10 * time.Second,
		MaxActionsPerSecond:  5.0,
		MaxConcurrentActions: 3,
		MaxRetries:           3,
		RetryBackoff:         5 * time.Second,
		ActionTimeout:        2 * time.Minute,
		MaxHistorySize:       100,
		EventTopic:           "cuirass.reconciliation",
		LogLevel:             "info",
	}
}

// ============================================================================
// LOGGER INTERFACE
// ============================================================================

// Logger defines the logging interface
type Logger interface {
	Debug(msg string, fields map[string]interface{})
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, fields map[string]interface{})
}

// DefaultLogger is a simple default logger implementation
type DefaultLogger struct {
	level string
}

// NewDefaultLogger creates a new default logger
func NewDefaultLogger(level string) *DefaultLogger {
	return &DefaultLogger{level: level}
}

func (l *DefaultLogger) log(level, msg string, fields map[string]interface{}) {
	data, _ := json.Marshal(fields)
	fmt.Printf(`{"level":"%s","msg":"%s","fields":%s,"time":"%s"}`+"\n",
		level, msg, string(data), time.Now().Format(time.RFC3339))
}

func (l *DefaultLogger) Debug(msg string, fields map[string]interface{}) {
	if l.level == "debug" {
		l.log("debug", msg, fields)
	}
}

func (l *DefaultLogger) Info(msg string, fields map[string]interface{}) {
	l.log("info", msg, fields)
}

func (l *DefaultLogger) Warn(msg string, fields map[string]interface{}) {
	l.log("warn", msg, fields)
}

func (l *DefaultLogger) Error(msg string, fields map[string]interface{}) {
	l.log("error", msg, fields)
}

// ============================================================================
// RECONCILER
// ============================================================================

// Reconciler is the main state reconciliation engine
type Reconciler struct {
	mu sync.RWMutex

	// Configuration
	config ReconcilerConfig

	// Clients
	lxd    LXDClient
	wotan  WotanClient
	logger Logger

	// State
	desiredState *DesiredState
	actualState  *ActualState
	lastDrift    *DriftReport

	// Rate limiting
	rateLimiter *RateLimiter

	// History
	history   []*ReconciliationRun
	historyMu sync.RWMutex

	// Metrics
	metrics *ReconcilerMetrics

	// Lifecycle
	closed  bool
	closeCh chan struct{}
	wg      sync.WaitGroup
}

// NewReconciler creates a new reconciler instance
func NewReconciler(config ReconcilerConfig, lxd LXDClient, wotan WotanClient, logger Logger) (*Reconciler, error) {
	if lxd == nil {
		return nil, ErrLXDClientNil
	}

	if logger == nil {
		logger = NewDefaultLogger(config.LogLevel)
	}

	return &Reconciler{
		config:      config,
		lxd:         lxd,
		wotan:       wotan,
		logger:      logger,
		rateLimiter: NewRateLimiter(config.MaxActionsPerSecond),
		history:     make([]*ReconciliationRun, 0, config.MaxHistorySize),
		metrics:     NewReconcilerMetrics(),
		closeCh:     make(chan struct{}),
	}, nil
}

// ============================================================================
// STATE LOADING
// ============================================================================

// LoadDesiredState loads desired state from a file (YAML or JSON)
func (r *Reconciler) LoadDesiredState(path string) (*DesiredState, error) {
	if path == "" {
		return nil, ErrInvalidStatePath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var state DesiredState

	// Try YAML first, then JSON
	if err := yaml.Unmarshal(data, &state); err != nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("parse state file: %w", err)
		}
	}

	if err := state.Validate(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.desiredState = &state
	r.metrics.mu.Lock()
	r.metrics.ContainersDesired = len(state.Containers)
	r.metrics.mu.Unlock()
	r.mu.Unlock()

	r.logger.Info("loaded desired state", map[string]interface{}{
		"path":       path,
		"version":    state.Version,
		"containers": len(state.Containers),
	})

	return &state, nil
}

// SetDesiredState sets the desired state directly
func (r *Reconciler) SetDesiredState(state *DesiredState) error {
	if err := state.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	r.desiredState = state
	r.metrics.mu.Lock()
	r.metrics.ContainersDesired = len(state.Containers)
	r.metrics.mu.Unlock()
	r.mu.Unlock()

	return nil
}

// GetDesiredState returns the current desired state
func (r *Reconciler) GetDesiredState() *DesiredState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.desiredState
}

// ============================================================================
// ACTUAL STATE RETRIEVAL
// ============================================================================

// GetActualState queries LXD to get the current actual state
func (r *Reconciler) GetActualState(ctx context.Context) (*ActualState, error) {
	if r.lxd == nil {
		return nil, ErrLXDClientNil
	}

	startTime := time.Now()

	containers, err := r.lxd.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	networks, err := r.lxd.ListNetworks(ctx)
	if err != nil {
		r.logger.Warn("failed to list networks", map[string]interface{}{
			"error": err.Error(),
		})
		networks = nil
	}

	state := &ActualState{
		Containers: make(map[string]*ContainerActual),
		Networks:   make(map[string]*NetworkActual),
		ObservedAt: time.Now(),
	}

	for _, c := range containers {
		state.Containers[c.Name] = c
	}

	for _, n := range networks {
		state.Networks[n.Name] = n
	}

	r.mu.Lock()
	r.actualState = state
	r.metrics.mu.Lock()
	r.metrics.ContainersActual = len(state.Containers)

	// Count healthy/unhealthy
	healthy := 0
	unhealthy := 0
	for _, c := range state.Containers {
		switch c.Health {
		case HealthHealthy:
			healthy++
		case HealthUnhealthy:
			unhealthy++
		}
	}
	r.metrics.ContainersHealthy = healthy
	r.metrics.ContainersUnhealthy = unhealthy
	r.metrics.mu.Unlock()
	r.mu.Unlock()

	duration := time.Since(startTime)
	r.logger.Debug("retrieved actual state", map[string]interface{}{
		"containers": len(containers),
		"networks":   len(networks),
		"duration":   duration.String(),
	})

	return state, nil
}

// ============================================================================
// DRIFT DETECTION
// ============================================================================

// DetectDrift compares desired and actual state to detect drift
func (r *Reconciler) DetectDrift(desired *DesiredState, actual *ActualState) (*DriftReport, error) {
	if desired == nil {
		return nil, ErrInvalidDesiredState
	}
	if actual == nil {
		return nil, ErrInvalidActualState
	}

	startTime := time.Now()
	drifts := make([]DriftItem, 0)
	summary := DriftSummary{}

	// Check for missing or misconfigured containers
	for id, desiredContainer := range desired.Containers {
		if !desiredContainer.Enabled {
			continue // Skip disabled containers
		}

		actualContainer, exists := actual.Containers[desiredContainer.Name]
		if !exists {
			// Container is missing
			drift := DriftItem{
				ContainerID: id,
				DriftType:   DriftMissing,
				Severity:    SeverityHigh,
				Expected:    StatusRunning,
				Actual:      StatusNotFound,
				Details:     map[string]string{"reason": "container not found"},
				DetectedAt:  time.Now(),
			}
			drifts = append(drifts, drift)
			summary.Missing++
			summary.HighCount++
			continue
		}

		// Check status drift
		if actualContainer.Status != StatusRunning {
			severity := r.severityForStatus(actualContainer.Status)
			drift := DriftItem{
				ContainerID: id,
				DriftType:   DriftStatusMismatch,
				Severity:    severity,
				Field:       "status",
				Expected:    StatusRunning,
				Actual:      actualContainer.Status,
				Details:     map[string]string{"current_status": string(actualContainer.Status)},
				DetectedAt:  time.Now(),
			}
			drifts = append(drifts, drift)
			summary.StatusMismatch++
			r.incrementSeverityCount(&summary, severity)
		}

		// Check health drift
		if actualContainer.Health == HealthUnhealthy {
			drift := DriftItem{
				ContainerID: id,
				DriftType:   DriftHealthDegraded,
				Severity:    SeverityMedium,
				Field:       "health",
				Expected:    HealthHealthy,
				Actual:      actualContainer.Health,
				Details:     map[string]string{"health_status": string(actualContainer.Health)},
				DetectedAt:  time.Now(),
			}
			drifts = append(drifts, drift)
			summary.HealthDegraded++
			summary.MediumCount++
		}

		// Check configuration drift (image)
		if desiredContainer.Image != "" && actualContainer.Image != desiredContainer.Image {
			drift := DriftItem{
				ContainerID: id,
				DriftType:   DriftConfigMismatch,
				Severity:    SeverityMedium,
				Field:       "image",
				Expected:    desiredContainer.Image,
				Actual:      actualContainer.Image,
				DetectedAt:  time.Now(),
			}
			drifts = append(drifts, drift)
			summary.ConfigMismatch++
			summary.MediumCount++
		}

		// Check resource drift (CPU)
		if desiredContainer.CPU > 0 && actualContainer.CPU != desiredContainer.CPU {
			drift := DriftItem{
				ContainerID: id,
				DriftType:   DriftResourceDrift,
				Severity:    SeverityLow,
				Field:       "cpu",
				Expected:    desiredContainer.CPU,
				Actual:      actualContainer.CPU,
				DetectedAt:  time.Now(),
			}
			drifts = append(drifts, drift)
			summary.ResourceDrift++
			summary.LowCount++
		}

		// Check resource drift (Memory)
		if desiredContainer.Memory > 0 && actualContainer.Memory != desiredContainer.Memory {
			drift := DriftItem{
				ContainerID: id,
				DriftType:   DriftResourceDrift,
				Severity:    SeverityLow,
				Field:       "memory",
				Expected:    desiredContainer.Memory,
				Actual:      actualContainer.Memory,
				DetectedAt:  time.Now(),
			}
			drifts = append(drifts, drift)
			summary.ResourceDrift++
			summary.LowCount++
		}
	}

	// Check for orphaned containers
	for name, actualContainer := range actual.Containers {
		found := false
		for _, desiredContainer := range desired.Containers {
			if desiredContainer.Name == name {
				found = true
				break
			}
		}
		if !found {
			drift := DriftItem{
				ContainerID: actualContainer.ID,
				DriftType:   DriftOrphaned,
				Severity:    SeverityLow,
				Details:     map[string]string{"name": name, "reason": "container exists but not in desired state"},
				DetectedAt:  time.Now(),
			}
			drifts = append(drifts, drift)
			summary.Orphaned++
			summary.LowCount++
		}
	}

	duration := time.Since(startTime)
	report := &DriftReport{
		ID:              fmt.Sprintf("drift-%d", time.Now().UnixNano()),
		DesiredState:    desired.Version,
		TotalContainers: len(desired.Containers),
		DriftCount:      len(drifts),
		Drifts:          drifts,
		Summary:         summary,
		DetectedAt:      time.Now(),
		Duration:        duration,
	}

	// Update metrics
	r.metrics.mu.Lock()
	r.metrics.DriftDetectedTotal++
	r.metrics.LastDriftCount = len(drifts)
	r.metrics.LastDriftCheckTime = time.Now()
	r.metrics.DriftCheckDuration = duration
	for _, d := range drifts {
		r.metrics.DriftByType[d.DriftType]++
		r.metrics.DriftBySeverity[d.Severity]++
	}
	r.metrics.mu.Unlock()

	r.mu.Lock()
	r.lastDrift = report
	r.mu.Unlock()

	// Emit event to Wotan
	if len(drifts) > 0 {
		r.emitEvent(context.Background(), ReconciliationEvent{
			Type:      EventDriftDetected,
			Timestamp: time.Now(),
			Severity:  r.highestSeverity(drifts),
			Message:   fmt.Sprintf("Detected %d drift(s)", len(drifts)),
			Data: map[string]interface{}{
				"drift_count":     len(drifts),
				"missing":         summary.Missing,
				"orphaned":        summary.Orphaned,
				"status_mismatch": summary.StatusMismatch,
				"health_degraded": summary.HealthDegraded,
			},
		})
	}

	r.logger.Info("drift detection completed", map[string]interface{}{
		"drift_count":     len(drifts),
		"missing":         summary.Missing,
		"orphaned":        summary.Orphaned,
		"status_mismatch": summary.StatusMismatch,
		"duration":        duration.String(),
	})

	return report, nil
}

// ============================================================================
// ACTION GENERATION
// ============================================================================

// GenerateActions generates reconciliation actions from a drift report
func (r *Reconciler) GenerateActions(drift *DriftReport) ([]*ReconciliationAction, error) {
	if drift == nil {
		return nil, errors.New("drift report is nil")
	}

	actions := make([]*ReconciliationAction, 0, len(drift.Drifts))

	for _, d := range drift.Drifts {
		var action *ReconciliationAction

		switch d.DriftType {
		case DriftMissing:
			action = &ReconciliationAction{
				ID:          fmt.Sprintf("action-%d", time.Now().UnixNano()),
				ContainerID: d.ContainerID,
				ActionType:  ActionCreate,
				Priority:    r.priorityForSeverity(d.Severity),
				DriftItem:   &d,
				Status:      ActionStatusPending,
				MaxRetries:  r.config.MaxRetries,
				CreatedAt:   time.Now(),
			}

		case DriftStatusMismatch:
			expectedStatus, ok := d.Expected.(ContainerStatus)
			if !ok {
				expectedStatus = StatusRunning
			}

			var actionType ActionType
			switch expectedStatus {
			case StatusRunning:
				actionType = ActionStart
			case StatusStopped:
				actionType = ActionStop
			default:
				actionType = ActionRestart
			}

			action = &ReconciliationAction{
				ID:          fmt.Sprintf("action-%d", time.Now().UnixNano()),
				ContainerID: d.ContainerID,
				ActionType:  actionType,
				Priority:    r.priorityForSeverity(d.Severity),
				DriftItem:   &d,
				Status:      ActionStatusPending,
				MaxRetries:  r.config.MaxRetries,
				CreatedAt:   time.Now(),
			}

		case DriftHealthDegraded:
			action = &ReconciliationAction{
				ID:          fmt.Sprintf("action-%d", time.Now().UnixNano()),
				ContainerID: d.ContainerID,
				ActionType:  ActionRestart,
				Priority:    r.priorityForSeverity(d.Severity),
				DriftItem:   &d,
				Status:      ActionStatusPending,
				MaxRetries:  r.config.MaxRetries,
				CreatedAt:   time.Now(),
			}

		case DriftConfigMismatch, DriftResourceDrift:
			action = &ReconciliationAction{
				ID:          fmt.Sprintf("action-%d", time.Now().UnixNano()),
				ContainerID: d.ContainerID,
				ActionType:  ActionUpdate,
				Priority:    r.priorityForSeverity(d.Severity),
				DriftItem:   &d,
				Params: map[string]interface{}{
					"field":    d.Field,
					"expected": d.Expected,
					"actual":   d.Actual,
				},
				Status:     ActionStatusPending,
				MaxRetries: r.config.MaxRetries,
				CreatedAt:  time.Now(),
			}

		case DriftOrphaned:
			// For orphaned containers, we create a delete action but with low priority
			action = &ReconciliationAction{
				ID:          fmt.Sprintf("action-%d", time.Now().UnixNano()),
				ContainerID: d.ContainerID,
				ActionType:  ActionDelete,
				Priority:    100, // Low priority for deletion
				DriftItem:   &d,
				Status:      ActionStatusPending,
				MaxRetries:  1, // Only try once for deletion
				CreatedAt:   time.Now(),
			}
		}

		if action != nil {
			actions = append(actions, action)
		}
	}

	// Sort by priority (lower number = higher priority)
	r.sortActionsByPriority(actions)

	r.logger.Info("generated reconciliation actions", map[string]interface{}{
		"action_count": len(actions),
		"drift_count":  len(drift.Drifts),
	})

	return actions, nil
}

// ============================================================================
// ACTION EXECUTION
// ============================================================================

// ExecuteActions executes reconciliation actions with rate limiting
func (r *Reconciler) ExecuteActions(ctx context.Context, actions []*ReconciliationAction, dryRun bool) ([]*ActionResult, error) {
	if r.lxd == nil {
		return nil, ErrLXDClientNil
	}

	results := make([]*ActionResult, 0, len(actions))

	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	r.emitEvent(ctx, ReconciliationEvent{
		Type:        EventReconcileStarted,
		Timestamp:   time.Now(),
		ReconcileID: runID,
		Message:     fmt.Sprintf("Starting reconciliation with %d actions (dry_run=%v)", len(actions), dryRun),
		Data: map[string]interface{}{
			"action_count": len(actions),
			"dry_run":      dryRun,
		},
	})

	for _, action := range actions {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		// Rate limit
		if err := r.rateLimiter.Wait(ctx); err != nil {
			return results, fmt.Errorf("rate limiter: %w", err)
		}

		result := r.executeAction(ctx, action, dryRun)
		results = append(results, result)

		// Update metrics
		r.metrics.mu.Lock()
		r.metrics.ActionsExecutedTotal++
		if result.Success {
			r.metrics.ActionsSuccessTotal++
		} else if action.Status == ActionStatusSkipped {
			r.metrics.ActionsSkippedTotal++
		} else {
			r.metrics.ActionsFailedTotal++
		}
		r.metrics.ActionsByType[action.ActionType]++
		r.metrics.mu.Unlock()
	}

	return results, nil
}

// executeAction executes a single reconciliation action
func (r *Reconciler) executeAction(ctx context.Context, action *ReconciliationAction, dryRun bool) *ActionResult {
	startTime := time.Now()
	action.Status = ActionStatusInProgress
	action.StartedAt = &startTime

	r.emitEvent(ctx, ReconciliationEvent{
		Type:        EventActionStarted,
		Timestamp:   time.Now(),
		ContainerID: action.ContainerID,
		ActionID:    action.ID,
		Message:     fmt.Sprintf("Starting action %s on %s", action.ActionType, action.ContainerID),
		Data: map[string]interface{}{
			"action_type": action.ActionType,
			"dry_run":     dryRun,
		},
	})

	if dryRun {
		action.Status = ActionStatusSkipped
		completedAt := time.Now()
		action.CompletedAt = &completedAt
		action.Duration = time.Since(startTime)

		r.logger.Info("dry run: action skipped", map[string]interface{}{
			"action_id":    action.ID,
			"action_type":  action.ActionType,
			"container_id": action.ContainerID,
		})

		return &ActionResult{
			ActionID:    action.ID,
			Success:     true,
			Error:       ErrDryRunMode,
			Duration:    action.Duration,
			CompletedAt: completedAt,
		}
	}

	// Execute with retries
	var lastErr error
	for attempt := 0; attempt <= action.MaxRetries; attempt++ {
		if attempt > 0 {
			action.Status = ActionStatusRetrying
			action.Retries = attempt

			// Bail early if ctx is cancelled — the bare `break` only escaped
			// the select before this fix, so executeActionOnce would still
			// be invoked and overwrite lastErr.
			select {
			case <-ctx.Done():
				lastErr = ctx.Err()
				goto exhausted
			case <-time.After(r.config.RetryBackoff * time.Duration(attempt)):
			}
		}

		err := r.executeActionOnce(ctx, action)
		if err == nil {
			action.Status = ActionStatusCompleted
			completedAt := time.Now()
			action.CompletedAt = &completedAt
			action.Duration = time.Since(startTime)

			r.emitEvent(ctx, ReconciliationEvent{
				Type:        EventActionCompleted,
				Timestamp:   time.Now(),
				ContainerID: action.ContainerID,
				ActionID:    action.ID,
				Message:     fmt.Sprintf("Completed action %s on %s", action.ActionType, action.ContainerID),
				Data: map[string]interface{}{
					"action_type": action.ActionType,
					"duration":    action.Duration.String(),
					"retries":     action.Retries,
				},
			})

			r.logger.Info("action completed successfully", map[string]interface{}{
				"action_id":    action.ID,
				"action_type":  action.ActionType,
				"container_id": action.ContainerID,
				"duration":     action.Duration.String(),
				"retries":      action.Retries,
			})

			return &ActionResult{
				ActionID:    action.ID,
				Success:     true,
				Duration:    action.Duration,
				RetryCount:  action.Retries,
				CompletedAt: completedAt,
			}
		}

		lastErr = err
		r.logger.Warn("action attempt failed", map[string]interface{}{
			"action_id":    action.ID,
			"action_type":  action.ActionType,
			"container_id": action.ContainerID,
			"attempt":      attempt + 1,
			"max_retries":  action.MaxRetries,
			"error":        err.Error(),
		})
	}

	// All retries exhausted (or ctx cancelled mid-backoff via goto)
exhausted:
	action.Status = ActionStatusFailed
	action.Error = lastErr.Error()
	completedAt := time.Now()
	action.CompletedAt = &completedAt
	action.Duration = time.Since(startTime)

	r.emitEvent(ctx, ReconciliationEvent{
		Type:        EventActionFailed,
		Timestamp:   time.Now(),
		ContainerID: action.ContainerID,
		ActionID:    action.ID,
		Severity:    SeverityHigh,
		Message:     fmt.Sprintf("Failed action %s on %s: %v", action.ActionType, action.ContainerID, lastErr),
		Data: map[string]interface{}{
			"action_type": action.ActionType,
			"error":       lastErr.Error(),
			"retries":     action.Retries,
		},
	})

	r.logger.Error("action failed after retries", map[string]interface{}{
		"action_id":    action.ID,
		"action_type":  action.ActionType,
		"container_id": action.ContainerID,
		"error":        lastErr.Error(),
		"retries":      action.Retries,
	})

	return &ActionResult{
		ActionID:    action.ID,
		Success:     false,
		Error:       lastErr,
		Duration:    action.Duration,
		RetryCount:  action.Retries,
		CompletedAt: completedAt,
	}
}

// executeActionOnce executes a single action attempt
func (r *Reconciler) executeActionOnce(ctx context.Context, action *ReconciliationAction) error {
	actionCtx, cancel := context.WithTimeout(ctx, r.config.ActionTimeout)
	defer cancel()

	// Get container spec from desired state
	r.mu.RLock()
	var containerSpec *ContainerDesired
	if r.desiredState != nil {
		containerSpec = r.desiredState.Containers[action.ContainerID]
	}
	r.mu.RUnlock()

	switch action.ActionType {
	case ActionCreate:
		if containerSpec == nil {
			return fmt.Errorf("container spec not found for %s", action.ContainerID)
		}
		if err := r.lxd.CreateContainer(actionCtx, containerSpec); err != nil {
			return fmt.Errorf("create container: %w", err)
		}
		// Also start the container after creation
		if err := r.lxd.StartContainer(actionCtx, containerSpec.Name); err != nil {
			return fmt.Errorf("start container after create: %w", err)
		}
		r.emitEvent(ctx, ReconciliationEvent{
			Type:        EventContainerCreated,
			Timestamp:   time.Now(),
			ContainerID: action.ContainerID,
			Message:     fmt.Sprintf("Created container %s", containerSpec.Name),
		})

	case ActionStart:
		name := r.getContainerName(action.ContainerID)
		if err := r.lxd.StartContainer(actionCtx, name); err != nil {
			return fmt.Errorf("start container: %w", err)
		}
		r.emitEvent(ctx, ReconciliationEvent{
			Type:        EventContainerStarted,
			Timestamp:   time.Now(),
			ContainerID: action.ContainerID,
			Message:     fmt.Sprintf("Started container %s", name),
		})

	case ActionStop:
		name := r.getContainerName(action.ContainerID)
		if err := r.lxd.StopContainer(actionCtx, name, false); err != nil {
			return fmt.Errorf("stop container: %w", err)
		}
		r.emitEvent(ctx, ReconciliationEvent{
			Type:        EventContainerStopped,
			Timestamp:   time.Now(),
			ContainerID: action.ContainerID,
			Message:     fmt.Sprintf("Stopped container %s", name),
		})

	case ActionRestart:
		name := r.getContainerName(action.ContainerID)
		if err := r.lxd.RestartContainer(actionCtx, name); err != nil {
			return fmt.Errorf("restart container: %w", err)
		}
		r.emitEvent(ctx, ReconciliationEvent{
			Type:        EventContainerStarted,
			Timestamp:   time.Now(),
			ContainerID: action.ContainerID,
			Message:     fmt.Sprintf("Restarted container %s", name),
		})

	case ActionDelete:
		name := r.getContainerName(action.ContainerID)
		if err := r.lxd.DeleteContainer(actionCtx, name, true); err != nil {
			return fmt.Errorf("delete container: %w", err)
		}
		r.emitEvent(ctx, ReconciliationEvent{
			Type:        EventContainerDeleted,
			Timestamp:   time.Now(),
			ContainerID: action.ContainerID,
			Message:     fmt.Sprintf("Deleted container %s", name),
		})

	case ActionUpdate:
		if containerSpec == nil {
			return fmt.Errorf("container spec not found for %s", action.ContainerID)
		}
		if err := r.lxd.UpdateContainer(actionCtx, containerSpec.Name, containerSpec); err != nil {
			return fmt.Errorf("update container: %w", err)
		}

	case ActionRepair:
		// Repair typically means restart for now
		name := r.getContainerName(action.ContainerID)
		if err := r.lxd.RestartContainer(actionCtx, name); err != nil {
			return fmt.Errorf("repair (restart) container: %w", err)
		}

	default:
		return fmt.Errorf("unknown action type: %s", action.ActionType)
	}

	return nil
}

// ============================================================================
// RECONCILIATION LOOP
// ============================================================================

// ReconciliationLoop runs the continuous reconciliation loop
func (r *Reconciler) ReconciliationLoop(ctx context.Context, interval time.Duration) error {
	if interval == 0 {
		interval = r.config.PollInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.logger.Info("starting reconciliation loop", map[string]interface{}{
		"interval": interval.String(),
	})

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("reconciliation loop stopped", map[string]interface{}{
				"reason": ctx.Err().Error(),
			})
			return ctx.Err()
		case <-r.closeCh:
			r.logger.Info("reconciliation loop stopped", map[string]interface{}{
				"reason": "reconciler closed",
			})
			return ErrReconcilerClosed
		case <-ticker.C:
			if err := r.Reconcile(ctx, false); err != nil {
				r.logger.Error("reconciliation failed", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}
	}
}

// Reconcile performs a single reconciliation cycle
func (r *Reconciler) Reconcile(ctx context.Context, dryRun bool) error {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return ErrReconcilerClosed
	}
	desiredState := r.desiredState
	r.mu.RUnlock()

	if desiredState == nil {
		return errors.New("no desired state loaded")
	}

	startTime := time.Now()
	run := &ReconciliationRun{
		ID:        fmt.Sprintf("run-%d", time.Now().UnixNano()),
		StartedAt: startTime,
		DryRun:    dryRun,
		Actions:   make([]*ReconciliationAction, 0),
		Results:   make([]*ActionResult, 0),
	}

	// Step 1: Get actual state
	actualState, err := r.GetActualState(ctx)
	if err != nil {
		run.Error = err.Error()
		run.Success = false
		r.addToHistory(run)
		return fmt.Errorf("get actual state: %w", err)
	}

	// Step 2: Detect drift
	driftReport, err := r.DetectDrift(desiredState, actualState)
	if err != nil {
		run.Error = err.Error()
		run.Success = false
		r.addToHistory(run)
		return fmt.Errorf("detect drift: %w", err)
	}
	run.DriftReport = driftReport

	// Step 3: Generate actions
	if driftReport.DriftCount > 0 {
		actions, err := r.GenerateActions(driftReport)
		if err != nil {
			run.Error = err.Error()
			run.Success = false
			r.addToHistory(run)
			return fmt.Errorf("generate actions: %w", err)
		}
		run.Actions = actions
		run.ActionsTotal = len(actions)

		// Step 4: Execute actions
		results, err := r.ExecuteActions(ctx, actions, dryRun)
		if err != nil {
			run.Error = err.Error()
			run.Success = false
			r.addToHistory(run)
			return fmt.Errorf("execute actions: %w", err)
		}
		run.Results = results

		// Count results
		for _, result := range results {
			if result.Success {
				run.ActionsSuccess++
			} else if result.Error == ErrDryRunMode {
				run.ActionsSkipped++
			} else {
				run.ActionsFailed++
			}
		}
	}

	// Complete the run
	completedAt := time.Now()
	run.CompletedAt = &completedAt
	run.Duration = time.Since(startTime)
	run.Success = run.ActionsFailed == 0

	// Update metrics
	r.metrics.mu.Lock()
	r.metrics.ReconcileTotal++
	if run.Success {
		r.metrics.ReconcileSuccessTotal++
	} else {
		r.metrics.ReconcileFailedTotal++
	}
	r.metrics.ReconcileDuration = run.Duration
	r.metrics.LastReconcileTime = completedAt
	r.metrics.mu.Unlock()

	// Add to history
	r.addToHistory(run)

	// Emit completion event
	eventType := EventReconcileCompleted
	if !run.Success {
		eventType = EventReconcileFailed
	}
	r.emitEvent(ctx, ReconciliationEvent{
		Type:        eventType,
		Timestamp:   completedAt,
		ReconcileID: run.ID,
		Message:     fmt.Sprintf("Reconciliation completed: %d drifts, %d actions, %d success, %d failed", driftReport.DriftCount, run.ActionsTotal, run.ActionsSuccess, run.ActionsFailed),
		Data: map[string]interface{}{
			"drift_count":     driftReport.DriftCount,
			"actions_total":   run.ActionsTotal,
			"actions_success": run.ActionsSuccess,
			"actions_failed":  run.ActionsFailed,
			"actions_skipped": run.ActionsSkipped,
			"duration":        run.Duration.String(),
			"dry_run":         dryRun,
		},
	})

	r.logger.Info("reconciliation cycle completed", map[string]interface{}{
		"run_id":          run.ID,
		"drift_count":     driftReport.DriftCount,
		"actions_total":   run.ActionsTotal,
		"actions_success": run.ActionsSuccess,
		"actions_failed":  run.ActionsFailed,
		"duration":        run.Duration.String(),
		"dry_run":         dryRun,
	})

	return nil
}

// ============================================================================
// HISTORY
// ============================================================================

// addToHistory adds a reconciliation run to history
func (r *Reconciler) addToHistory(run *ReconciliationRun) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()

	r.history = append(r.history, run)

	// Trim history if needed
	if len(r.history) > r.config.MaxHistorySize {
		r.history = r.history[len(r.history)-r.config.MaxHistorySize:]
	}
}

// GetHistory returns reconciliation history
func (r *Reconciler) GetHistory(limit int) []*ReconciliationRun {
	r.historyMu.RLock()
	defer r.historyMu.RUnlock()

	if limit <= 0 || limit > len(r.history) {
		limit = len(r.history)
	}

	// Return most recent first
	result := make([]*ReconciliationRun, limit)
	for i := 0; i < limit; i++ {
		result[i] = r.history[len(r.history)-1-i]
	}
	return result
}

// GetLastRun returns the last reconciliation run
func (r *Reconciler) GetLastRun() *ReconciliationRun {
	r.historyMu.RLock()
	defer r.historyMu.RUnlock()

	if len(r.history) == 0 {
		return nil
	}
	return r.history[len(r.history)-1]
}

// ============================================================================
// METRICS
// ============================================================================

// GetMetrics returns current reconciler metrics
func (r *Reconciler) GetMetrics() *ReconcilerMetrics {
	r.metrics.mu.RLock()
	defer r.metrics.mu.RUnlock()

	// Return a copy
	m := &ReconcilerMetrics{
		DriftDetectedTotal:    r.metrics.DriftDetectedTotal,
		DriftByType:           make(map[DriftType]int64),
		DriftBySeverity:       make(map[DriftSeverity]int64),
		LastDriftCount:        r.metrics.LastDriftCount,
		LastDriftCheckTime:    r.metrics.LastDriftCheckTime,
		DriftCheckDuration:    r.metrics.DriftCheckDuration,
		ActionsExecutedTotal:  r.metrics.ActionsExecutedTotal,
		ActionsSuccessTotal:   r.metrics.ActionsSuccessTotal,
		ActionsFailedTotal:    r.metrics.ActionsFailedTotal,
		ActionsSkippedTotal:   r.metrics.ActionsSkippedTotal,
		ActionsByType:         make(map[ActionType]int64),
		ActionDuration:        r.metrics.ActionDuration,
		ReconcileTotal:        r.metrics.ReconcileTotal,
		ReconcileSuccessTotal: r.metrics.ReconcileSuccessTotal,
		ReconcileFailedTotal:  r.metrics.ReconcileFailedTotal,
		ReconcileDuration:     r.metrics.ReconcileDuration,
		LastReconcileTime:     r.metrics.LastReconcileTime,
		ContainersDesired:     r.metrics.ContainersDesired,
		ContainersActual:      r.metrics.ContainersActual,
		ContainersHealthy:     r.metrics.ContainersHealthy,
		ContainersUnhealthy:   r.metrics.ContainersUnhealthy,
	}

	for k, v := range r.metrics.DriftByType {
		m.DriftByType[k] = v
	}
	for k, v := range r.metrics.DriftBySeverity {
		m.DriftBySeverity[k] = v
	}
	for k, v := range r.metrics.ActionsByType {
		m.ActionsByType[k] = v
	}

	return m
}

// ============================================================================
// LIFECYCLE
// ============================================================================

// Close closes the reconciler and stops the reconciliation loop
func (r *Reconciler) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	close(r.closeCh)
	r.mu.Unlock()

	r.wg.Wait()

	r.logger.Info("reconciler closed", nil)
	return nil
}

// IsClosed returns whether the reconciler is closed
func (r *Reconciler) IsClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.closed
}

// ============================================================================
// HELPER METHODS
// ============================================================================

// emitEvent publishes an event to Wotan
func (r *Reconciler) emitEvent(ctx context.Context, event ReconciliationEvent) {
	if r.wotan == nil {
		r.logger.Warn("wotan unavailable — reconciliation event dropped (degraded mode)", nil)
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		r.logger.Warn("failed to marshal event", map[string]interface{}{
			"error": err.Error(),
			"type":  event.Type,
		})
		return
	}

	if err := r.wotan.Publish(ctx, r.config.EventTopic, data); err != nil {
		r.logger.Warn("failed to publish event", map[string]interface{}{
			"error": err.Error(),
			"type":  event.Type,
			"topic": r.config.EventTopic,
		})
	}
}

// severityForStatus returns the severity for a container status
func (r *Reconciler) severityForStatus(status ContainerStatus) DriftSeverity {
	switch status {
	case StatusError:
		return SeverityCritical
	case StatusStopped:
		return SeverityHigh
	case StatusDegraded:
		return SeverityMedium
	default:
		return SeverityLow
	}
}

// priorityForSeverity returns the action priority for a severity
func (r *Reconciler) priorityForSeverity(severity DriftSeverity) int {
	switch severity {
	case SeverityCritical:
		return 1
	case SeverityHigh:
		return 10
	case SeverityMedium:
		return 50
	case SeverityLow:
		return 100
	default:
		return 50
	}
}

// highestSeverity returns the highest severity from a list of drifts
func (r *Reconciler) highestSeverity(drifts []DriftItem) DriftSeverity {
	highest := SeverityLow
	for _, d := range drifts {
		if r.priorityForSeverity(d.Severity) < r.priorityForSeverity(highest) {
			highest = d.Severity
		}
	}
	return highest
}

// incrementSeverityCount increments the appropriate severity counter
func (r *Reconciler) incrementSeverityCount(summary *DriftSummary, severity DriftSeverity) {
	switch severity {
	case SeverityCritical:
		summary.CriticalCount++
	case SeverityHigh:
		summary.HighCount++
	case SeverityMedium:
		summary.MediumCount++
	case SeverityLow:
		summary.LowCount++
	}
}

// sortActionsByPriority sorts actions by priority (lower number = higher priority)
func (r *Reconciler) sortActionsByPriority(actions []*ReconciliationAction) {
	for i := 0; i < len(actions)-1; i++ {
		for j := i + 1; j < len(actions); j++ {
			if actions[i].Priority > actions[j].Priority {
				actions[i], actions[j] = actions[j], actions[i]
			}
		}
	}
}

// getContainerName returns the container name from the desired state
func (r *Reconciler) getContainerName(containerID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.desiredState == nil {
		return containerID
	}

	if spec, ok := r.desiredState.Containers[containerID]; ok {
		return spec.Name
	}

	// Also check actual state
	if r.actualState != nil {
		if actual, ok := r.actualState.Containers[containerID]; ok {
			return actual.Name
		}
	}

	return containerID
}

// GetLastDriftReport returns the last drift report
func (r *Reconciler) GetLastDriftReport() *DriftReport {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastDrift
}

// GetActualStateSnapshot returns the current actual state
func (r *Reconciler) GetActualStateSnapshot() *ActualState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.actualState
}
