// Package state provides state management for the Cuirass control plane
// THE CORE HEART - Maintains desired vs actual state and enforces convergence
package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrNilState       = errors.New("state cannot be nil")
	ErrEmptyID        = errors.New("id cannot be empty")
	ErrContainerExists = errors.New("container already exists")
	ErrContainerNotFound = errors.New("container not found")
	ErrInvalidStatus  = errors.New("invalid container status")
	ErrStateClosed    = errors.New("state manager is closed")
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
)

// ContainerSpec represents the desired state of a container
type ContainerSpec struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Type        string            `json:"type"` // service, gateway, worker
	CPU         int               `json:"cpu_cores"`
	Memory      int64             `json:"memory_mb"`
	NetworkMode string            `json:"network_mode"`
	Ports       []PortMapping     `json:"ports"`
	Env         map[string]string `json:"env"`
	Labels      map[string]string `json:"labels"`
	HealthCheck *HealthCheck      `json:"health_check,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// PortMapping represents a port mapping
type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol"` // tcp, udp
}

// HealthCheck represents a health check configuration
type HealthCheck struct {
	Endpoint string        `json:"endpoint"`
	Interval time.Duration `json:"interval"`
	Timeout  time.Duration `json:"timeout"`
	Retries  int           `json:"retries"`
}

// ContainerState represents the actual observed state of a container
type ContainerState struct {
	ID          string            `json:"id"`
	Status      ContainerStatus   `json:"status"`
	IPAddress   string            `json:"ip_address"`
	CPUUsage    float64           `json:"cpu_usage_percent"`
	MemoryUsage int64             `json:"memory_usage_mb"`
	Uptime      time.Duration     `json:"uptime"`
	LastSeen    time.Time         `json:"last_seen"`
	Health      HealthStatus      `json:"health"`
	Metrics     map[string]float64 `json:"metrics,omitempty"`
}

// HealthStatus represents the health of a container
type HealthStatus string

const (
	HealthUnknown   HealthStatus = "unknown"
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// Validate validates a ContainerSpec
func (c *ContainerSpec) Validate() error {
	if c == nil {
		return ErrNilState
	}
	if c.ID == "" {
		return ErrEmptyID
	}
	if c.Name == "" {
		return errors.New("container name cannot be empty")
	}
	if c.Image == "" {
		return errors.New("container image cannot be empty")
	}
	if c.CPU < 0 {
		return errors.New("CPU cores cannot be negative")
	}
	if c.Memory < 0 {
		return errors.New("memory cannot be negative")
	}
	return nil
}

// ============================================================================
// DRIFT DETECTION
// ============================================================================

// DriftType represents the type of configuration drift
type DriftType string

const (
	DriftNone       DriftType = "none"
	DriftConfig     DriftType = "config"
	DriftStatus     DriftType = "status"
	DriftMissing    DriftType = "missing"
	DriftOrphaned   DriftType = "orphaned"
	DriftDegraded   DriftType = "degraded"
)

// DriftReport represents detected drift between desired and actual state
type DriftReport struct {
	ContainerID string            `json:"container_id"`
	DriftType   DriftType         `json:"drift_type"`
	Desired     *ContainerSpec    `json:"desired,omitempty"`
	Actual      *ContainerState   `json:"actual,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
	DetectedAt  time.Time         `json:"detected_at"`
	Severity    string            `json:"severity"` // low, medium, high, critical
}

// ============================================================================
// STATE MANAGER
// ============================================================================

// Manager manages desired and actual state with drift detection
type Manager struct {
	mu            sync.RWMutex
	desired       map[string]*ContainerSpec
	actual        map[string]*ContainerState
	driftReports  []DriftReport
	closed        bool
	pollInterval  time.Duration
	onDrift       func(DriftReport)
	metrics       *StateMetrics
}

// StateMetrics tracks state management metrics
type StateMetrics struct {
	mu                 sync.RWMutex
	ContainersDesired  int
	ContainersActual   int
	DriftsDetected     int
	LastPollTime       time.Time
	LastDriftCheckTime time.Time
}

// ManagerConfig holds Manager configuration
type ManagerConfig struct {
	PollInterval time.Duration
	OnDrift      func(DriftReport)
}

// NewManager creates a new state manager
func NewManager(cfg ManagerConfig) *Manager {
	interval := cfg.PollInterval
	if interval == 0 {
		interval = 30 * time.Second
	}

	return &Manager{
		desired:      make(map[string]*ContainerSpec),
		actual:       make(map[string]*ContainerState),
		driftReports: make([]DriftReport, 0),
		pollInterval: interval,
		onDrift:      cfg.OnDrift,
		metrics:      &StateMetrics{},
	}
}

// ============================================================================
// DESIRED STATE OPERATIONS
// ============================================================================

// SetDesiredState sets the desired state for a container
func (m *Manager) SetDesiredState(ctx context.Context, spec *ContainerSpec) error {
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("validate spec: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrStateClosed
	}

	now := time.Now()
	spec.UpdatedAt = now
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = now
	}

	m.desired[spec.ID] = spec

	m.metrics.mu.Lock()
	m.metrics.ContainersDesired = len(m.desired)
	m.metrics.mu.Unlock()

	return nil
}

// RemoveDesiredState removes a container from desired state
func (m *Manager) RemoveDesiredState(ctx context.Context, id string) error {
	if id == "" {
		return ErrEmptyID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrStateClosed
	}

	delete(m.desired, id)

	m.metrics.mu.Lock()
	m.metrics.ContainersDesired = len(m.desired)
	m.metrics.mu.Unlock()

	return nil
}

// GetDesiredState gets the desired state for a container
func (m *Manager) GetDesiredState(ctx context.Context, id string) (*ContainerSpec, error) {
	if id == "" {
		return nil, ErrEmptyID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrStateClosed
	}

	spec, ok := m.desired[id]
	if !ok {
		return nil, ErrContainerNotFound
	}

	return spec, nil
}

// ListDesiredStates returns all desired container specs
func (m *Manager) ListDesiredStates(ctx context.Context) ([]*ContainerSpec, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrStateClosed
	}

	specs := make([]*ContainerSpec, 0, len(m.desired))
	for _, spec := range m.desired {
		specs = append(specs, spec)
	}

	return specs, nil
}

// ============================================================================
// ACTUAL STATE OPERATIONS
// ============================================================================

// UpdateActualState updates the observed state of a container
func (m *Manager) UpdateActualState(ctx context.Context, state *ContainerState) error {
	if state == nil {
		return ErrNilState
	}
	if state.ID == "" {
		return ErrEmptyID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrStateClosed
	}

	state.LastSeen = time.Now()
	m.actual[state.ID] = state

	m.metrics.mu.Lock()
	m.metrics.ContainersActual = len(m.actual)
	m.metrics.mu.Unlock()

	return nil
}

// GetActualState gets the observed state of a container
func (m *Manager) GetActualState(ctx context.Context, id string) (*ContainerState, error) {
	if id == "" {
		return nil, ErrEmptyID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrStateClosed
	}

	state, ok := m.actual[id]
	if !ok {
		return nil, ErrContainerNotFound
	}

	return state, nil
}

// ListActualStates returns all observed container states
func (m *Manager) ListActualStates(ctx context.Context) ([]*ContainerState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrStateClosed
	}

	states := make([]*ContainerState, 0, len(m.actual))
	for _, state := range m.actual {
		states = append(states, state)
	}

	return states, nil
}

// ============================================================================
// DRIFT DETECTION
// ============================================================================

// DetectDrift compares desired vs actual state and reports drift
func (m *Manager) DetectDrift(ctx context.Context) ([]DriftReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrStateClosed
	}

	reports := make([]DriftReport, 0)
	now := time.Now()

	// Check for missing containers (desired but not actual)
	for id, desired := range m.desired {
		actual, exists := m.actual[id]

		if !exists {
			report := DriftReport{
				ContainerID: id,
				DriftType:   DriftMissing,
				Desired:     desired,
				Actual:      nil,
				Details:     map[string]string{"reason": "container not found in actual state"},
				DetectedAt:  now,
				Severity:    "high",
			}
			reports = append(reports, report)
			continue
		}

		// Check status drift
		if actual.Status != StatusRunning {
			report := DriftReport{
				ContainerID: id,
				DriftType:   DriftStatus,
				Desired:     desired,
				Actual:      actual,
				Details:     map[string]string{"status": string(actual.Status)},
				DetectedAt:  now,
				Severity:    severityForStatus(actual.Status),
			}
			reports = append(reports, report)
		}

		// Check health drift
		if actual.Health == HealthUnhealthy {
			report := DriftReport{
				ContainerID: id,
				DriftType:   DriftDegraded,
				Desired:     desired,
				Actual:      actual,
				Details:     map[string]string{"health": string(actual.Health)},
				DetectedAt:  now,
				Severity:    "medium",
			}
			reports = append(reports, report)
		}
	}

	// Check for orphaned containers (actual but not desired)
	for id, actual := range m.actual {
		if _, exists := m.desired[id]; !exists {
			report := DriftReport{
				ContainerID: id,
				DriftType:   DriftOrphaned,
				Desired:     nil,
				Actual:      actual,
				Details:     map[string]string{"reason": "container exists but not in desired state"},
				DetectedAt:  now,
				Severity:    "low",
			}
			reports = append(reports, report)
		}
	}

	m.driftReports = reports
	m.metrics.mu.Lock()
	m.metrics.DriftsDetected = len(reports)
	m.metrics.LastDriftCheckTime = now
	m.metrics.mu.Unlock()

	// Call drift callback for each report
	if m.onDrift != nil {
		for _, report := range reports {
			m.onDrift(report)
		}
	}

	return reports, nil
}

// GetDriftReports returns the latest drift reports
func (m *Manager) GetDriftReports(ctx context.Context) ([]DriftReport, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrStateClosed
	}

	reports := make([]DriftReport, len(m.driftReports))
	copy(reports, m.driftReports)

	return reports, nil
}

// ============================================================================
// METRICS
// ============================================================================

// GetMetrics returns current state metrics
func (m *Manager) GetMetrics(ctx context.Context) (*StateMetrics, error) {
	m.metrics.mu.RLock()
	defer m.metrics.mu.RUnlock()

	return &StateMetrics{
		ContainersDesired:  m.metrics.ContainersDesired,
		ContainersActual:   m.metrics.ContainersActual,
		DriftsDetected:     m.metrics.DriftsDetected,
		LastPollTime:       m.metrics.LastPollTime,
		LastDriftCheckTime: m.metrics.LastDriftCheckTime,
	}, nil
}

// ============================================================================
// LIFECYCLE
// ============================================================================

// Close closes the state manager
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	return nil
}

// IsClosed returns whether the manager is closed
func (m *Manager) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// ============================================================================
// SERIALIZATION
// ============================================================================

// ExportState exports the complete state as JSON
func (m *Manager) ExportState(ctx context.Context) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrStateClosed
	}

	export := struct {
		Desired map[string]*ContainerSpec  `json:"desired"`
		Actual  map[string]*ContainerState `json:"actual"`
		Drifts  []DriftReport              `json:"drifts"`
	}{
		Desired: m.desired,
		Actual:  m.actual,
		Drifts:  m.driftReports,
	}

	return json.MarshalIndent(export, "", "  ")
}

// ============================================================================
// HELPERS
// ============================================================================

func severityForStatus(status ContainerStatus) string {
	switch status {
	case StatusError:
		return "critical"
	case StatusStopped:
		return "high"
	case StatusDegraded:
		return "medium"
	default:
		return "low"
	}
}
