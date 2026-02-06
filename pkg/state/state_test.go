package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// MOCK IMPLEMENTATIONS
// ============================================================================

// mockLXDClient is a configurable mock for the LXDClient interface.
type mockLXDClient struct {
	mu sync.Mutex

	containers []*ContainerActual
	networks   []*NetworkActual

	// Per-method error injection
	listContainersErr   error
	getContainerErr     error
	createContainerErr  error
	startContainerErr   error
	stopContainerErr    error
	restartContainerErr error
	deleteContainerErr  error
	updateContainerErr  error
	checkHealthErr      error
	listNetworksErr     error
	createNetworkErr    error
	deleteNetworkErr    error

	// Call tracking
	createCalls  []string
	startCalls   []string
	stopCalls    []string
	restartCalls []string
	deleteCalls  []string
	updateCalls  []string

	// Behaviour overrides
	healthResult HealthStatus

	// For simulating transient failures that succeed after N attempts
	failCreateNTimes int
	createAttempts   int
}

func newMockLXD() *mockLXDClient {
	return &mockLXDClient{
		containers:   make([]*ContainerActual, 0),
		networks:     make([]*NetworkActual, 0),
		createCalls:  make([]string, 0),
		startCalls:   make([]string, 0),
		stopCalls:    make([]string, 0),
		restartCalls: make([]string, 0),
		deleteCalls:  make([]string, 0),
		updateCalls:  make([]string, 0),
		healthResult: HealthHealthy,
	}
}

func (m *mockLXDClient) ListContainers(_ context.Context) ([]*ContainerActual, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listContainersErr != nil {
		return nil, m.listContainersErr
	}
	result := make([]*ContainerActual, len(m.containers))
	copy(result, m.containers)
	return result, nil
}

func (m *mockLXDClient) GetContainer(_ context.Context, name string) (*ContainerActual, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getContainerErr != nil {
		return nil, m.getContainerErr
	}
	for _, c := range m.containers {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, ErrContainerNotFound
}

func (m *mockLXDClient) CreateContainer(_ context.Context, spec *ContainerDesired) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls = append(m.createCalls, spec.Name)
	if m.failCreateNTimes > 0 && m.createAttempts < m.failCreateNTimes {
		m.createAttempts++
		return errors.New("transient create failure")
	}
	if m.createContainerErr != nil {
		return m.createContainerErr
	}
	return nil
}

func (m *mockLXDClient) StartContainer(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalls = append(m.startCalls, name)
	if m.startContainerErr != nil {
		return m.startContainerErr
	}
	return nil
}

func (m *mockLXDClient) StopContainer(_ context.Context, name string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls = append(m.stopCalls, name)
	if m.stopContainerErr != nil {
		return m.stopContainerErr
	}
	return nil
}

func (m *mockLXDClient) RestartContainer(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartCalls = append(m.restartCalls, name)
	if m.restartContainerErr != nil {
		return m.restartContainerErr
	}
	return nil
}

func (m *mockLXDClient) DeleteContainer(_ context.Context, name string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls = append(m.deleteCalls, name)
	if m.deleteContainerErr != nil {
		return m.deleteContainerErr
	}
	return nil
}

func (m *mockLXDClient) UpdateContainer(_ context.Context, name string, _ *ContainerDesired) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls = append(m.updateCalls, name)
	if m.updateContainerErr != nil {
		return m.updateContainerErr
	}
	return nil
}

func (m *mockLXDClient) CheckHealth(_ context.Context, _ string, _ *HealthCheck) (HealthStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.checkHealthErr != nil {
		return HealthUnknown, m.checkHealthErr
	}
	return m.healthResult, nil
}

func (m *mockLXDClient) ListNetworks(_ context.Context) ([]*NetworkActual, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listNetworksErr != nil {
		return nil, m.listNetworksErr
	}
	result := make([]*NetworkActual, len(m.networks))
	copy(result, m.networks)
	return result, nil
}

func (m *mockLXDClient) CreateNetwork(_ context.Context, _ *NetworkDesired) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createNetworkErr
}

func (m *mockLXDClient) DeleteNetwork(_ context.Context, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleteNetworkErr
}

// mockBusboyClient tracks published events.
type mockBusboyClient struct {
	mu       sync.Mutex
	events   [][]byte
	topics   []string
	err      error
	callCount int64
}

func newMockBusboy() *mockBusboyClient {
	return &mockBusboyClient{
		events: make([][]byte, 0),
		topics: make([]string, 0),
	}
}

func (m *mockBusboyClient) Publish(_ context.Context, topic string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	atomic.AddInt64(&m.callCount, 1)
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, payload)
	m.topics = append(m.topics, topic)
	return nil
}

func (m *mockBusboyClient) eventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockBusboyClient) lastEvent() *ReconciliationEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return nil
	}
	var ev ReconciliationEvent
	if err := json.Unmarshal(m.events[len(m.events)-1], &ev); err != nil {
		return nil
	}
	return &ev
}

// silentLogger discards all log output (useful for tests).
type silentLogger struct{}

func (l *silentLogger) Debug(_ string, _ map[string]interface{}) {}
func (l *silentLogger) Info(_ string, _ map[string]interface{})  {}
func (l *silentLogger) Warn(_ string, _ map[string]interface{})  {}
func (l *silentLogger) Error(_ string, _ map[string]interface{}) {}

// ============================================================================
// HELPER CONSTRUCTORS
// ============================================================================

func testConfig() ReconcilerConfig {
	cfg := DefaultReconcilerConfig()
	cfg.MaxActionsPerSecond = 1000 // fast for tests
	cfg.RetryBackoff = 1 * time.Millisecond
	cfg.ActionTimeout = 5 * time.Second
	cfg.MaxRetries = 2
	cfg.MaxHistorySize = 50
	return cfg
}

func newTestReconciler(t *testing.T) (*Reconciler, *mockLXDClient, *mockBusboyClient) {
	t.Helper()
	lxd := newMockLXD()
	bus := newMockBusboy()
	r, err := NewReconciler(testConfig(), lxd, bus, &silentLogger{})
	if err != nil {
		t.Fatalf("NewReconciler: %v", err)
	}
	return r, lxd, bus
}

func validDesiredState() *DesiredState {
	return &DesiredState{
		Version: "1.0",
		Name:    "test-stack",
		Containers: map[string]*ContainerDesired{
			"web": {
				Name:    "web",
				Image:   "ubuntu:22.04",
				Type:    "service",
				Enabled: true,
				CPU:     2,
				Memory:  512,
			},
			"api": {
				Name:    "api",
				Image:   "golang:1.21",
				Type:    "service",
				Enabled: true,
				CPU:     4,
				Memory:  1024,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func matchingActualState() *ActualState {
	return &ActualState{
		Containers: map[string]*ContainerActual{
			"web": {
				ID:     "c1",
				Name:   "web",
				Status: StatusRunning,
				Image:  "ubuntu:22.04",
				CPU:    2,
				Memory: 512,
				Health: HealthHealthy,
			},
			"api": {
				ID:     "c2",
				Name:   "api",
				Status: StatusRunning,
				Image:  "golang:1.21",
				CPU:    4,
				Memory: 1024,
				Health: HealthHealthy,
			},
		},
		ObservedAt: time.Now(),
	}
}

// ============================================================================
// TEST: NewReconciler
// ============================================================================

func TestNewReconciler(t *testing.T) {
	tests := []struct {
		name      string
		lxd       LXDClient
		busboy    BusboyClient
		logger    Logger
		wantErr   error
	}{
		{
			name:    "nil LXD client returns error",
			lxd:     nil,
			busboy:  newMockBusboy(),
			logger:  &silentLogger{},
			wantErr: ErrLXDClientNil,
		},
		{
			name:    "nil busboy is allowed",
			lxd:     newMockLXD(),
			busboy:  nil,
			logger:  &silentLogger{},
			wantErr: nil,
		},
		{
			name:    "nil logger uses default",
			lxd:     newMockLXD(),
			busboy:  newMockBusboy(),
			logger:  nil,
			wantErr: nil,
		},
		{
			name:    "all provided",
			lxd:     newMockLXD(),
			busboy:  newMockBusboy(),
			logger:  &silentLogger{},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewReconciler(testConfig(), tt.lxd, tt.busboy, tt.logger)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				if r != nil {
					t.Error("expected nil reconciler on error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if r == nil {
					t.Fatal("expected non-nil reconciler")
				}
			}
		})
	}
}

// ============================================================================
// TEST: DesiredState Validation
// ============================================================================

func TestDesiredState_Validate(t *testing.T) {
	tests := []struct {
		name    string
		state   *DesiredState
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil state",
			state:   nil,
			wantErr: true,
			errMsg:  "invalid desired state",
		},
		{
			name:    "empty name",
			state:   &DesiredState{Name: "", Containers: map[string]*ContainerDesired{"x": {Name: "x", Image: "img"}}},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name:    "no containers",
			state:   &DesiredState{Name: "test", Containers: map[string]*ContainerDesired{}},
			wantErr: true,
			errMsg:  "at least one container",
		},
		{
			name: "nil container entry",
			state: &DesiredState{
				Name:       "test",
				Containers: map[string]*ContainerDesired{"x": nil},
			},
			wantErr: true,
			errMsg:  "is nil",
		},
		{
			name: "container missing name",
			state: &DesiredState{
				Name:       "test",
				Containers: map[string]*ContainerDesired{"x": {Image: "img"}},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "container missing image",
			state: &DesiredState{
				Name:       "test",
				Containers: map[string]*ContainerDesired{"x": {Name: "x"}},
			},
			wantErr: true,
			errMsg:  "image is required",
		},
		{
			name: "container negative CPU",
			state: &DesiredState{
				Name:       "test",
				Containers: map[string]*ContainerDesired{"x": {Name: "x", Image: "img", CPU: -1}},
			},
			wantErr: true,
			errMsg:  "CPU cannot be negative",
		},
		{
			name: "container negative memory",
			state: &DesiredState{
				Name:       "test",
				Containers: map[string]*ContainerDesired{"x": {Name: "x", Image: "img", Memory: -100}},
			},
			wantErr: true,
			errMsg:  "memory cannot be negative",
		},
		{
			name:    "valid state",
			state:   validDesiredState(),
			wantErr: false,
		},
		{
			name: "valid with zero CPU/memory (optional resources)",
			state: &DesiredState{
				Name: "test",
				Containers: map[string]*ContainerDesired{
					"x": {Name: "x", Image: "img", CPU: 0, Memory: 0},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && !containsStr(err.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ============================================================================
// TEST: SetDesiredState / GetDesiredState
// ============================================================================

func TestSetDesiredState(t *testing.T) {
	r, _, _ := newTestReconciler(t)

	t.Run("valid state", func(t *testing.T) {
		ds := validDesiredState()
		if err := r.SetDesiredState(ds); err != nil {
			t.Fatalf("SetDesiredState: %v", err)
		}
		got := r.GetDesiredState()
		if got == nil {
			t.Fatal("expected non-nil desired state")
		}
		if got.Name != ds.Name {
			t.Errorf("name = %q, want %q", got.Name, ds.Name)
		}
	})

	t.Run("invalid state rejected", func(t *testing.T) {
		err := r.SetDesiredState(&DesiredState{})
		if err == nil {
			t.Fatal("expected error for invalid state")
		}
	})

	t.Run("updates metrics", func(t *testing.T) {
		ds := validDesiredState()
		_ = r.SetDesiredState(ds)
		m := r.GetMetrics()
		if m.ContainersDesired != len(ds.Containers) {
			t.Errorf("ContainersDesired = %d, want %d", m.ContainersDesired, len(ds.Containers))
		}
	})
}

// ============================================================================
// TEST: LoadDesiredState from file
// ============================================================================

func TestLoadDesiredState(t *testing.T) {
	r, _, _ := newTestReconciler(t)

	t.Run("empty path", func(t *testing.T) {
		_, err := r.LoadDesiredState("")
		if !errors.Is(err, ErrInvalidStatePath) {
			t.Errorf("expected ErrInvalidStatePath, got %v", err)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := r.LoadDesiredState("/nonexistent/path/state.yaml")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})

	t.Run("load valid YAML", func(t *testing.T) {
		content := `
version: "1.0"
name: "yaml-test"
containers:
  svc:
    name: "svc"
    image: "alpine:3.18"
    type: "service"
    enabled: true
    cpu_cores: 1
    memory_mb: 256
    network_mode: "bridge"
`
		dir := t.TempDir()
		path := filepath.Join(dir, "state.yaml")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		ds, err := r.LoadDesiredState(path)
		if err != nil {
			t.Fatalf("LoadDesiredState: %v", err)
		}
		if ds.Name != "yaml-test" {
			t.Errorf("name = %q, want %q", ds.Name, "yaml-test")
		}
		if len(ds.Containers) != 1 {
			t.Errorf("containers = %d, want 1", len(ds.Containers))
		}
	})

	t.Run("load valid JSON", func(t *testing.T) {
		ds := validDesiredState()
		data, _ := json.Marshal(ds)
		dir := t.TempDir()
		path := filepath.Join(dir, "state.json")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}

		got, err := r.LoadDesiredState(path)
		if err != nil {
			t.Fatalf("LoadDesiredState: %v", err)
		}
		if got.Name != ds.Name {
			t.Errorf("name = %q, want %q", got.Name, ds.Name)
		}
	})

	t.Run("invalid content", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")
		// Write content that parses as YAML but fails validation (no containers)
		if err := os.WriteFile(path, []byte(`name: "test"`), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := r.LoadDesiredState(path)
		if err == nil {
			t.Fatal("expected error for state without containers")
		}
	})
}

// ============================================================================
// TEST: GetActualState
// ============================================================================

func TestGetActualState(t *testing.T) {
	t.Run("returns containers and networks from LXD", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Health: HealthUnhealthy},
		}
		lxd.networks = []*NetworkActual{
			{Name: "lxdbr0", Subnet: "10.10.10.0/24"},
		}

		actual, err := r.GetActualState(context.Background())
		if err != nil {
			t.Fatalf("GetActualState: %v", err)
		}
		if len(actual.Containers) != 2 {
			t.Errorf("containers = %d, want 2", len(actual.Containers))
		}
		if len(actual.Networks) != 1 {
			t.Errorf("networks = %d, want 1", len(actual.Networks))
		}

		m := r.GetMetrics()
		if m.ContainersActual != 2 {
			t.Errorf("ContainersActual = %d, want 2", m.ContainersActual)
		}
		if m.ContainersHealthy != 1 {
			t.Errorf("ContainersHealthy = %d, want 1", m.ContainersHealthy)
		}
		if m.ContainersUnhealthy != 1 {
			t.Errorf("ContainersUnhealthy = %d, want 1", m.ContainersUnhealthy)
		}
	})

	t.Run("list containers error", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		lxd.listContainersErr = errors.New("connection refused")
		_, err := r.GetActualState(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("list networks error is non-fatal", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		lxd.containers = []*ContainerActual{{ID: "c1", Name: "web", Status: StatusRunning}}
		lxd.listNetworksErr = errors.New("network error")
		actual, err := r.GetActualState(context.Background())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(actual.Networks) != 0 {
			t.Errorf("expected empty networks on error, got %d", len(actual.Networks))
		}
	})
}

// ============================================================================
// TEST: Drift Detection (Pleroma vs Kenoma pattern)
// ============================================================================

func TestDetectDrift(t *testing.T) {
	tests := []struct {
		name           string
		desired        *DesiredState
		actual         *ActualState
		wantErr        error
		wantDriftCount int
		wantDriftTypes []DriftType
		checkSummary   func(t *testing.T, s DriftSummary)
	}{
		{
			name:    "nil desired state",
			desired: nil,
			actual:  matchingActualState(),
			wantErr: ErrInvalidDesiredState,
		},
		{
			name:    "nil actual state",
			desired: validDesiredState(),
			actual:  nil,
			wantErr: ErrInvalidActualState,
		},
		{
			name:           "no drift when states match (Pleroma = Kenoma)",
			desired:        validDesiredState(),
			actual:         matchingActualState(),
			wantDriftCount: 0,
		},
		{
			name:    "missing container detected (Kenoma lacks element from Pleroma)",
			desired: validDesiredState(),
			actual: &ActualState{
				Containers: map[string]*ContainerActual{
					"web": {ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
					// "api" is missing
				},
				ObservedAt: time.Now(),
			},
			wantDriftCount: 1,
			wantDriftTypes: []DriftType{DriftMissing},
			checkSummary: func(t *testing.T, s DriftSummary) {
				if s.Missing != 1 {
					t.Errorf("Missing = %d, want 1", s.Missing)
				}
				if s.HighCount != 1 {
					t.Errorf("HighCount = %d, want 1", s.HighCount)
				}
			},
		},
		{
			name:    "orphaned container detected (Kenoma has element not in Pleroma)",
			desired: validDesiredState(),
			actual: func() *ActualState {
				as := matchingActualState()
				as.Containers["orphan"] = &ContainerActual{
					ID: "c3", Name: "orphan", Status: StatusRunning,
				}
				return as
			}(),
			wantDriftCount: 1,
			wantDriftTypes: []DriftType{DriftOrphaned},
			checkSummary: func(t *testing.T, s DriftSummary) {
				if s.Orphaned != 1 {
					t.Errorf("Orphaned = %d, want 1", s.Orphaned)
				}
			},
		},
		{
			name:    "status mismatch - stopped container",
			desired: validDesiredState(),
			actual: func() *ActualState {
				as := matchingActualState()
				as.Containers["web"].Status = StatusStopped
				return as
			}(),
			wantDriftCount: 1,
			wantDriftTypes: []DriftType{DriftStatusMismatch},
			checkSummary: func(t *testing.T, s DriftSummary) {
				if s.StatusMismatch != 1 {
					t.Errorf("StatusMismatch = %d, want 1", s.StatusMismatch)
				}
			},
		},
		{
			name:    "status mismatch - error container (critical severity)",
			desired: validDesiredState(),
			actual: func() *ActualState {
				as := matchingActualState()
				as.Containers["api"].Status = StatusError
				return as
			}(),
			wantDriftCount: 1,
			wantDriftTypes: []DriftType{DriftStatusMismatch},
			checkSummary: func(t *testing.T, s DriftSummary) {
				if s.CriticalCount != 1 {
					t.Errorf("CriticalCount = %d, want 1", s.CriticalCount)
				}
			},
		},
		{
			name:    "health degraded",
			desired: validDesiredState(),
			actual: func() *ActualState {
				as := matchingActualState()
				as.Containers["web"].Health = HealthUnhealthy
				return as
			}(),
			wantDriftCount: 1,
			wantDriftTypes: []DriftType{DriftHealthDegraded},
			checkSummary: func(t *testing.T, s DriftSummary) {
				if s.HealthDegraded != 1 {
					t.Errorf("HealthDegraded = %d, want 1", s.HealthDegraded)
				}
				if s.MediumCount != 1 {
					t.Errorf("MediumCount = %d, want 1", s.MediumCount)
				}
			},
		},
		{
			name:    "image config mismatch",
			desired: validDesiredState(),
			actual: func() *ActualState {
				as := matchingActualState()
				as.Containers["web"].Image = "ubuntu:20.04"
				return as
			}(),
			wantDriftCount: 1,
			wantDriftTypes: []DriftType{DriftConfigMismatch},
			checkSummary: func(t *testing.T, s DriftSummary) {
				if s.ConfigMismatch != 1 {
					t.Errorf("ConfigMismatch = %d, want 1", s.ConfigMismatch)
				}
			},
		},
		{
			name:    "CPU resource drift",
			desired: validDesiredState(),
			actual: func() *ActualState {
				as := matchingActualState()
				as.Containers["web"].CPU = 1 // desired is 2
				return as
			}(),
			wantDriftCount: 1,
			wantDriftTypes: []DriftType{DriftResourceDrift},
			checkSummary: func(t *testing.T, s DriftSummary) {
				if s.ResourceDrift != 1 {
					t.Errorf("ResourceDrift = %d, want 1", s.ResourceDrift)
				}
			},
		},
		{
			name:    "memory resource drift",
			desired: validDesiredState(),
			actual: func() *ActualState {
				as := matchingActualState()
				as.Containers["api"].Memory = 2048 // desired is 1024
				return as
			}(),
			wantDriftCount: 1,
			wantDriftTypes: []DriftType{DriftResourceDrift},
		},
		{
			name: "disabled container not flagged as missing",
			desired: func() *DesiredState {
				ds := validDesiredState()
				ds.Containers["api"].Enabled = false
				return ds
			}(),
			actual: &ActualState{
				Containers: map[string]*ContainerActual{
					"web": {ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
				},
				ObservedAt: time.Now(),
			},
			wantDriftCount: 0,
		},
		{
			name:    "multiple drifts compound",
			desired: validDesiredState(),
			actual: &ActualState{
				Containers: map[string]*ContainerActual{
					"web": {
						ID: "c1", Name: "web", Status: StatusStopped,
						Image: "ubuntu:20.04", CPU: 1, Memory: 256,
						Health: HealthUnhealthy,
					},
					// "api" missing
					"orphan": {ID: "c3", Name: "orphan", Status: StatusRunning},
				},
				ObservedAt: time.Now(),
			},
			// web: status_mismatch + health_degraded + config_mismatch + cpu_drift + memory_drift = 5
			// api: missing = 1
			// orphan: orphaned = 1
			// Total = 7
			wantDriftCount: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _, _ := newTestReconciler(t)
			report, err := r.DetectDrift(tt.desired, tt.actual)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if report.DriftCount != tt.wantDriftCount {
				t.Errorf("DriftCount = %d, want %d", report.DriftCount, tt.wantDriftCount)
				for _, d := range report.Drifts {
					t.Logf("  drift: type=%s container=%s field=%s", d.DriftType, d.ContainerID, d.Field)
				}
			}
			if tt.wantDriftTypes != nil {
				for _, wantType := range tt.wantDriftTypes {
					found := false
					for _, d := range report.Drifts {
						if d.DriftType == wantType {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected drift type %s not found", wantType)
					}
				}
			}
			if tt.checkSummary != nil {
				tt.checkSummary(t, report.Summary)
			}
		})
	}
}

func TestDetectDrift_EmitsEvent(t *testing.T) {
	r, _, bus := newTestReconciler(t)
	desired := validDesiredState()
	actual := &ActualState{
		Containers: map[string]*ContainerActual{}, // all missing
		ObservedAt: time.Now(),
	}
	_, err := r.DetectDrift(desired, actual)
	if err != nil {
		t.Fatal(err)
	}
	if bus.eventCount() == 0 {
		t.Error("expected at least one event to be emitted")
	}
	ev := bus.lastEvent()
	if ev == nil {
		t.Fatal("expected event")
	}
	if ev.Type != EventDriftDetected {
		t.Errorf("event type = %s, want %s", ev.Type, EventDriftDetected)
	}
}

func TestDetectDrift_NoDriftNoEvent(t *testing.T) {
	r, _, bus := newTestReconciler(t)
	_, err := r.DetectDrift(validDesiredState(), matchingActualState())
	if err != nil {
		t.Fatal(err)
	}
	if bus.eventCount() != 0 {
		t.Error("expected no events when there is no drift")
	}
}

func TestDetectDrift_UpdatesMetrics(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	actual := &ActualState{
		Containers: map[string]*ContainerActual{},
		ObservedAt: time.Now(),
	}
	_, err := r.DetectDrift(validDesiredState(), actual)
	if err != nil {
		t.Fatal(err)
	}
	m := r.GetMetrics()
	if m.DriftDetectedTotal != 1 {
		t.Errorf("DriftDetectedTotal = %d, want 1", m.DriftDetectedTotal)
	}
	if m.LastDriftCount < 1 {
		t.Errorf("LastDriftCount = %d, want >= 1", m.LastDriftCount)
	}
}

// ============================================================================
// TEST: Action Generation
// ============================================================================

func TestGenerateActions(t *testing.T) {
	t.Run("nil drift report returns error", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		_, err := r.GenerateActions(nil)
		if err == nil {
			t.Fatal("expected error for nil drift report")
		}
	})

	t.Run("missing drift creates ActionCreate", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{ContainerID: "web", DriftType: DriftMissing, Severity: SeverityHigh},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) != 1 {
			t.Fatalf("actions = %d, want 1", len(actions))
		}
		if actions[0].ActionType != ActionCreate {
			t.Errorf("ActionType = %s, want %s", actions[0].ActionType, ActionCreate)
		}
	})

	t.Run("status mismatch with Running expected creates ActionStart", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{
					ContainerID: "web",
					DriftType:   DriftStatusMismatch,
					Severity:    SeverityHigh,
					Expected:    StatusRunning,
					Actual:      StatusStopped,
				},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) != 1 {
			t.Fatalf("actions = %d, want 1", len(actions))
		}
		if actions[0].ActionType != ActionStart {
			t.Errorf("ActionType = %s, want %s", actions[0].ActionType, ActionStart)
		}
	})

	t.Run("status mismatch with Stopped expected creates ActionStop", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{
					ContainerID: "web",
					DriftType:   DriftStatusMismatch,
					Severity:    SeverityMedium,
					Expected:    StatusStopped,
					Actual:      StatusRunning,
				},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if actions[0].ActionType != ActionStop {
			t.Errorf("ActionType = %s, want %s", actions[0].ActionType, ActionStop)
		}
	})

	t.Run("status mismatch with unexpected expected value creates ActionRestart", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{
					ContainerID: "web",
					DriftType:   DriftStatusMismatch,
					Severity:    SeverityMedium,
					Expected:    StatusCreating, // unusual expected status
					Actual:      StatusStopped,
				},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if actions[0].ActionType != ActionRestart {
			t.Errorf("ActionType = %s, want %s", actions[0].ActionType, ActionRestart)
		}
	})

	t.Run("status mismatch with non-ContainerStatus expected defaults to start", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{
					ContainerID: "web",
					DriftType:   DriftStatusMismatch,
					Severity:    SeverityHigh,
					Expected:    "some-string", // not a ContainerStatus
					Actual:      StatusStopped,
				},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		// When type assertion fails, expectedStatus defaults to StatusRunning -> ActionStart
		if actions[0].ActionType != ActionStart {
			t.Errorf("ActionType = %s, want %s", actions[0].ActionType, ActionStart)
		}
	})

	t.Run("health degraded creates ActionRestart", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{ContainerID: "web", DriftType: DriftHealthDegraded, Severity: SeverityMedium},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if actions[0].ActionType != ActionRestart {
			t.Errorf("ActionType = %s, want %s", actions[0].ActionType, ActionRestart)
		}
	})

	t.Run("config mismatch creates ActionUpdate", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{ContainerID: "web", DriftType: DriftConfigMismatch, Severity: SeverityMedium, Field: "image"},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if actions[0].ActionType != ActionUpdate {
			t.Errorf("ActionType = %s, want %s", actions[0].ActionType, ActionUpdate)
		}
		if actions[0].Params == nil || actions[0].Params["field"] != "image" {
			t.Error("expected params with field=image")
		}
	})

	t.Run("resource drift creates ActionUpdate", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{ContainerID: "web", DriftType: DriftResourceDrift, Severity: SeverityLow, Field: "cpu"},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if actions[0].ActionType != ActionUpdate {
			t.Errorf("ActionType = %s, want %s", actions[0].ActionType, ActionUpdate)
		}
	})

	t.Run("orphaned creates ActionDelete with low priority", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{ContainerID: "orphan", DriftType: DriftOrphaned, Severity: SeverityLow},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if actions[0].ActionType != ActionDelete {
			t.Errorf("ActionType = %s, want %s", actions[0].ActionType, ActionDelete)
		}
		if actions[0].Priority != 100 {
			t.Errorf("Priority = %d, want 100", actions[0].Priority)
		}
		if actions[0].MaxRetries != 1 {
			t.Errorf("MaxRetries = %d, want 1", actions[0].MaxRetries)
		}
	})

	t.Run("actions sorted by priority", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{ContainerID: "low", DriftType: DriftOrphaned, Severity: SeverityLow},
				{ContainerID: "critical", DriftType: DriftMissing, Severity: SeverityCritical},
				{ContainerID: "high", DriftType: DriftMissing, Severity: SeverityHigh},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) < 3 {
			t.Fatalf("expected at least 3 actions, got %d", len(actions))
		}
		for i := 0; i < len(actions)-1; i++ {
			if actions[i].Priority > actions[i+1].Priority {
				t.Errorf("actions not sorted: priority[%d]=%d > priority[%d]=%d",
					i, actions[i].Priority, i+1, actions[i+1].Priority)
			}
		}
	})

	t.Run("DriftNone type produces no action", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		report := &DriftReport{
			Drifts: []DriftItem{
				{ContainerID: "web", DriftType: DriftNone, Severity: SeverityLow},
			},
		}
		actions, err := r.GenerateActions(report)
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) != 0 {
			t.Errorf("expected 0 actions for DriftNone, got %d", len(actions))
		}
	})
}

// ============================================================================
// TEST: Action Execution
// ============================================================================

func TestExecuteActions(t *testing.T) {
	t.Run("dry run skips execution", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionStart, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Fatalf("results = %d, want 1", len(results))
		}
		if !results[0].Success {
			t.Error("dry run result should report success")
		}
		if !errors.Is(results[0].Error, ErrDryRunMode) {
			t.Error("dry run result should have ErrDryRunMode error")
		}
		if len(lxd.startCalls) != 0 {
			t.Error("LXD should not be called in dry run")
		}
	})

	t.Run("start action calls LXD", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionStart, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if !results[0].Success {
			t.Errorf("expected success, got error: %v", results[0].Error)
		}
		if len(lxd.startCalls) != 1 {
			t.Errorf("startCalls = %d, want 1", len(lxd.startCalls))
		}
	})

	t.Run("stop action calls LXD", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionStop, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if !results[0].Success {
			t.Errorf("expected success: %v", results[0].Error)
		}
		if len(lxd.stopCalls) != 1 {
			t.Errorf("stopCalls = %d, want 1", len(lxd.stopCalls))
		}
	})

	t.Run("restart action calls LXD", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionRestart, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if !results[0].Success {
			t.Errorf("expected success: %v", results[0].Error)
		}
		if len(lxd.restartCalls) != 1 {
			t.Errorf("restartCalls = %d, want 1", len(lxd.restartCalls))
		}
	})

	t.Run("delete action calls LXD", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionDelete, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if !results[0].Success {
			t.Errorf("expected success: %v", results[0].Error)
		}
		if len(lxd.deleteCalls) != 1 {
			t.Errorf("deleteCalls = %d, want 1", len(lxd.deleteCalls))
		}
	})

	t.Run("create action calls create then start", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionCreate, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if !results[0].Success {
			t.Errorf("expected success: %v", results[0].Error)
		}
		if len(lxd.createCalls) != 1 {
			t.Errorf("createCalls = %d, want 1", len(lxd.createCalls))
		}
		if len(lxd.startCalls) != 1 {
			t.Errorf("startCalls = %d, want 1 (start after create)", len(lxd.startCalls))
		}
	})

	t.Run("update action calls LXD", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionUpdate, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if !results[0].Success {
			t.Errorf("expected success: %v", results[0].Error)
		}
		if len(lxd.updateCalls) != 1 {
			t.Errorf("updateCalls = %d, want 1", len(lxd.updateCalls))
		}
	})

	t.Run("repair action calls restart", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionRepair, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if !results[0].Success {
			t.Errorf("expected success: %v", results[0].Error)
		}
		if len(lxd.restartCalls) != 1 {
			t.Errorf("restartCalls = %d, want 1 (repair -> restart)", len(lxd.restartCalls))
		}
	})

	t.Run("unknown action type fails", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: "teleport", Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if results[0].Success {
			t.Error("expected failure for unknown action type")
		}
	})

	t.Run("create with nil container spec fails", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		// Set desired state without the container we reference
		ds := validDesiredState()
		_ = r.SetDesiredState(ds)
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "nonexistent", ActionType: ActionCreate, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if results[0].Success {
			t.Error("expected failure when container spec not found")
		}
	})

	t.Run("update with nil container spec fails", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		ds := validDesiredState()
		_ = r.SetDesiredState(ds)
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "nonexistent", ActionType: ActionUpdate, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if results[0].Success {
			t.Error("expected failure when container spec not found")
		}
	})

	t.Run("nil LXD client returns error", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		r.lxd = nil
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionStart},
		}
		_, err := r.ExecuteActions(context.Background(), actions, false)
		if !errors.Is(err, ErrLXDClientNil) {
			t.Errorf("expected ErrLXDClientNil, got %v", err)
		}
	})

	t.Run("context cancellation stops execution", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionStart, Status: ActionStatusPending, MaxRetries: 0},
		}
		_, err := r.ExecuteActions(ctx, actions, false)
		if err == nil {
			t.Fatal("expected error on cancelled context")
		}
	})

	t.Run("action retries on transient failure", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		// Fail the first create attempt, succeed on retry
		lxd.failCreateNTimes = 1

		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionCreate, Status: ActionStatusPending, MaxRetries: 2},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if !results[0].Success {
			t.Errorf("expected eventual success after retry, got error: %v", results[0].Error)
		}
		if results[0].RetryCount < 1 {
			t.Errorf("RetryCount = %d, expected >= 1", results[0].RetryCount)
		}
	})

	t.Run("action fails after max retries exhausted", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.createContainerErr = errors.New("permanent failure")

		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionCreate, Status: ActionStatusPending, MaxRetries: 1},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if results[0].Success {
			t.Error("expected failure after exhausted retries")
		}
		if actions[0].Status != ActionStatusFailed {
			t.Errorf("action status = %s, want %s", actions[0].Status, ActionStatusFailed)
		}
	})

	t.Run("LXD start error propagates", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.startContainerErr = errors.New("cannot start")
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionStart, Status: ActionStatusPending, MaxRetries: 0},
		}
		results, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		if results[0].Success {
			t.Error("expected failure")
		}
	})
}

func TestExecuteActions_MetricsUpdated(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	_ = r.SetDesiredState(validDesiredState())
	actions := []*ReconciliationAction{
		{ID: "a1", ContainerID: "web", ActionType: ActionStart, Status: ActionStatusPending, MaxRetries: 0},
	}
	_, err := r.ExecuteActions(context.Background(), actions, false)
	if err != nil {
		t.Fatal(err)
	}
	m := r.GetMetrics()
	if m.ActionsExecutedTotal != 1 {
		t.Errorf("ActionsExecutedTotal = %d, want 1", m.ActionsExecutedTotal)
	}
	if m.ActionsSuccessTotal != 1 {
		t.Errorf("ActionsSuccessTotal = %d, want 1", m.ActionsSuccessTotal)
	}
}

// ============================================================================
// TEST: Rate Limiter
// ============================================================================

func TestRateLimiter(t *testing.T) {
	t.Run("allows initial burst", func(t *testing.T) {
		rl := NewRateLimiter(5.0)
		allowed := 0
		for i := 0; i < 5; i++ {
			if rl.Allow() {
				allowed++
			}
		}
		if allowed != 5 {
			t.Errorf("allowed = %d, want 5", allowed)
		}
	})

	t.Run("rejects when tokens exhausted", func(t *testing.T) {
		rl := NewRateLimiter(1.0)
		// First call consumes the single token
		if !rl.Allow() {
			t.Fatal("first call should be allowed")
		}
		// Second call should be denied (no time has passed)
		if rl.Allow() {
			t.Error("second call should be rejected (tokens exhausted)")
		}
	})

	t.Run("tokens replenish over time", func(t *testing.T) {
		rl := NewRateLimiter(100.0) // 100 per second
		// Exhaust all tokens
		for i := 0; i < 100; i++ {
			rl.Allow()
		}
		// Wait for replenishment
		time.Sleep(50 * time.Millisecond) // should add ~5 tokens
		if !rl.Allow() {
			t.Error("expected token replenishment after sleep")
		}
	})

	t.Run("Wait respects context cancellation", func(t *testing.T) {
		rl := NewRateLimiter(0.01) // very slow: 1 per 100 seconds
		rl.Allow()                 // exhaust the 1 token

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := rl.Wait(ctx)
		if err == nil {
			t.Error("expected context deadline error")
		}
	})

	t.Run("Wait returns nil when token available", func(t *testing.T) {
		rl := NewRateLimiter(1000.0)
		err := rl.Wait(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("tokens do not exceed max", func(t *testing.T) {
		rl := NewRateLimiter(2.0)
		time.Sleep(100 * time.Millisecond)
		// Even after sleeping, max tokens should be capped at maxPerSecond (2)
		allowed := 0
		for i := 0; i < 10; i++ {
			if rl.Allow() {
				allowed++
			}
		}
		if allowed > 2 {
			t.Errorf("allowed = %d, should be capped at 2", allowed)
		}
	})
}

// ============================================================================
// TEST: Rate Limiter - Concurrent
// ============================================================================

func TestRateLimiter_Concurrent(t *testing.T) {
	rl := NewRateLimiter(100.0)
	var wg sync.WaitGroup
	var totalAllowed int64

	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if rl.Allow() {
					atomic.AddInt64(&totalAllowed, 1)
				}
			}
		}()
	}
	wg.Wait()

	// We started with 100 tokens and 10 goroutines doing 50 calls each (500 total).
	// The allowed count should be at most ~100 plus a small replenishment amount.
	if totalAllowed > 200 {
		t.Errorf("totalAllowed = %d, seems too high for rate limit of 100/s", totalAllowed)
	}
	if totalAllowed == 0 {
		t.Error("expected some requests to be allowed")
	}
}

// ============================================================================
// TEST: Event Emission
// ============================================================================

func TestEventEmission(t *testing.T) {
	t.Run("nil busboy client does not panic", func(t *testing.T) {
		lxd := newMockLXD()
		r, err := NewReconciler(testConfig(), lxd, nil, &silentLogger{})
		if err != nil {
			t.Fatal(err)
		}
		// This should not panic
		r.emitEvent(context.Background(), ReconciliationEvent{
			Type:    EventDriftDetected,
			Message: "test",
		})
	})

	t.Run("busboy error is logged but does not fail", func(t *testing.T) {
		r, _, bus := newTestReconciler(t)
		bus.err = errors.New("publish failed")
		// Should not panic or return error
		r.emitEvent(context.Background(), ReconciliationEvent{
			Type:    EventDriftDetected,
			Message: "test event",
		})
	})

	t.Run("events published with correct topic", func(t *testing.T) {
		r, _, bus := newTestReconciler(t)
		r.emitEvent(context.Background(), ReconciliationEvent{
			Type:    EventReconcileStarted,
			Message: "starting",
		})
		if len(bus.topics) != 1 {
			t.Fatalf("topics = %d, want 1", len(bus.topics))
		}
		if bus.topics[0] != r.config.EventTopic {
			t.Errorf("topic = %q, want %q", bus.topics[0], r.config.EventTopic)
		}
	})

	t.Run("action execution emits start and complete events", func(t *testing.T) {
		r, _, bus := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionStart, Status: ActionStatusPending, MaxRetries: 0},
		}
		_, err := r.ExecuteActions(context.Background(), actions, false)
		if err != nil {
			t.Fatal(err)
		}
		// Should have: reconcile.started, action.started, container.started, action.completed
		if bus.eventCount() < 3 {
			t.Errorf("expected >= 3 events, got %d", bus.eventCount())
		}
	})

	t.Run("failed action emits failure event", func(t *testing.T) {
		r, lxd, bus := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.startContainerErr = errors.New("nope")
		actions := []*ReconciliationAction{
			{ID: "a1", ContainerID: "web", ActionType: ActionStart, Status: ActionStatusPending, MaxRetries: 0},
		}
		_, _ = r.ExecuteActions(context.Background(), actions, false)

		foundFailed := false
		bus.mu.Lock()
		for _, raw := range bus.events {
			var ev ReconciliationEvent
			if err := json.Unmarshal(raw, &ev); err == nil {
				if ev.Type == EventActionFailed {
					foundFailed = true
					break
				}
			}
		}
		bus.mu.Unlock()
		if !foundFailed {
			t.Error("expected action.failed event to be emitted")
		}
	})
}

// ============================================================================
// TEST: Full Reconciliation Cycle
// ============================================================================

func TestReconcile(t *testing.T) {
	t.Run("no desired state returns error", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		err := r.Reconcile(context.Background(), false)
		if err == nil {
			t.Fatal("expected error when no desired state")
		}
	})

	t.Run("closed reconciler returns error", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		r.Close()
		err := r.Reconcile(context.Background(), false)
		if !errors.Is(err, ErrReconcilerClosed) {
			t.Errorf("expected ErrReconcilerClosed, got %v", err)
		}
	})

	t.Run("full reconcile with no drift", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		ds := validDesiredState()
		_ = r.SetDesiredState(ds)

		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		}

		err := r.Reconcile(context.Background(), false)
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}

		run := r.GetLastRun()
		if run == nil {
			t.Fatal("expected last run")
		}
		if run.DriftReport.DriftCount != 0 {
			t.Errorf("DriftCount = %d, want 0", run.DriftReport.DriftCount)
		}
		if run.ActionsTotal != 0 {
			t.Errorf("ActionsTotal = %d, want 0", run.ActionsTotal)
		}
		if !run.Success {
			t.Error("expected success")
		}
	})

	t.Run("full reconcile with drift creates and executes actions", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		ds := validDesiredState()
		_ = r.SetDesiredState(ds)

		// Only web exists, api is missing
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
		}

		err := r.Reconcile(context.Background(), false)
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}

		run := r.GetLastRun()
		if run == nil {
			t.Fatal("expected last run")
		}
		if run.DriftReport.DriftCount == 0 {
			t.Error("expected drift")
		}
		if run.ActionsTotal == 0 {
			t.Error("expected actions")
		}
		if !run.Success {
			t.Error("expected success")
		}
	})

	t.Run("dry run reconcile does not modify state", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		ds := validDesiredState()
		_ = r.SetDesiredState(ds)

		lxd.containers = []*ContainerActual{} // all missing

		err := r.Reconcile(context.Background(), true)
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}

		if len(lxd.createCalls) != 0 {
			t.Errorf("createCalls = %d, want 0 in dry run", len(lxd.createCalls))
		}
	})

	t.Run("LXD list error stops reconciliation", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.listContainersErr = errors.New("connection refused")

		err := r.Reconcile(context.Background(), false)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("reconcile updates metrics", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		}

		_ = r.Reconcile(context.Background(), false)
		m := r.GetMetrics()
		if m.ReconcileTotal != 1 {
			t.Errorf("ReconcileTotal = %d, want 1", m.ReconcileTotal)
		}
		if m.ReconcileSuccessTotal != 1 {
			t.Errorf("ReconcileSuccessTotal = %d, want 1", m.ReconcileSuccessTotal)
		}
	})
}

// ============================================================================
// TEST: History
// ============================================================================

func TestHistory(t *testing.T) {
	t.Run("empty history", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		if r.GetLastRun() != nil {
			t.Error("expected nil last run on empty history")
		}
		hist := r.GetHistory(10)
		if len(hist) != 0 {
			t.Errorf("history = %d, want 0", len(hist))
		}
	})

	t.Run("history returns most recent first", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		}

		// Run reconcile 3 times
		for i := 0; i < 3; i++ {
			_ = r.Reconcile(context.Background(), false)
		}

		hist := r.GetHistory(2)
		if len(hist) != 2 {
			t.Fatalf("history = %d, want 2", len(hist))
		}
		// Most recent should be first
		if hist[0].StartedAt.Before(hist[1].StartedAt) {
			t.Error("history not ordered most recent first")
		}
	})

	t.Run("history trimmed to max size", func(t *testing.T) {
		cfg := testConfig()
		cfg.MaxHistorySize = 3
		lxd := newMockLXD()
		bus := newMockBusboy()
		r, err := NewReconciler(cfg, lxd, bus, &silentLogger{})
		if err != nil {
			t.Fatal(err)
		}
		_ = r.SetDesiredState(validDesiredState())
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		}

		for i := 0; i < 10; i++ {
			_ = r.Reconcile(context.Background(), false)
		}

		hist := r.GetHistory(0)
		if len(hist) > 3 {
			t.Errorf("history = %d, want <= 3 (max size)", len(hist))
		}
	})

	t.Run("GetHistory with limit 0 returns all", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		}
		for i := 0; i < 5; i++ {
			_ = r.Reconcile(context.Background(), false)
		}
		hist := r.GetHistory(0)
		if len(hist) != 5 {
			t.Errorf("history = %d, want 5", len(hist))
		}
	})

	t.Run("GetHistory with limit > history returns all", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		}
		_ = r.Reconcile(context.Background(), false)
		hist := r.GetHistory(100)
		if len(hist) != 1 {
			t.Errorf("history = %d, want 1", len(hist))
		}
	})
}

// ============================================================================
// TEST: Lifecycle (Close / IsClosed)
// ============================================================================

func TestLifecycle(t *testing.T) {
	t.Run("new reconciler is not closed", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		if r.IsClosed() {
			t.Error("new reconciler should not be closed")
		}
	})

	t.Run("Close marks reconciler as closed", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		err := r.Close()
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
		if !r.IsClosed() {
			t.Error("expected reconciler to be closed")
		}
	})

	t.Run("double Close is safe", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
	})
}

// ============================================================================
// TEST: ReconciliationLoop
// ============================================================================

func TestReconciliationLoop(t *testing.T) {
	t.Run("stops on context cancellation", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := r.ReconciliationLoop(ctx, 50*time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	})

	t.Run("stops on Close", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		}

		done := make(chan error, 1)
		go func() {
			done <- r.ReconciliationLoop(context.Background(), 50*time.Millisecond)
		}()

		time.Sleep(100 * time.Millisecond)
		r.Close()

		select {
		case err := <-done:
			if !errors.Is(err, ErrReconcilerClosed) {
				t.Errorf("expected ErrReconcilerClosed, got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("loop did not stop after Close")
		}
	})

	t.Run("uses default interval when 0", func(t *testing.T) {
		r, lxd, _ := newTestReconciler(t)
		_ = r.SetDesiredState(validDesiredState())
		lxd.containers = []*ContainerActual{
			{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
			{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		// Should not panic with interval=0
		_ = r.ReconciliationLoop(ctx, 0)
	})
}

// ============================================================================
// TEST: Metrics
// ============================================================================

func TestGetMetrics(t *testing.T) {
	t.Run("returns a copy, not a reference", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		m1 := r.GetMetrics()
		m1.ReconcileTotal = 999
		m2 := r.GetMetrics()
		if m2.ReconcileTotal == 999 {
			t.Error("GetMetrics should return a copy")
		}
	})

	t.Run("initial metrics are zero", func(t *testing.T) {
		r, _, _ := newTestReconciler(t)
		m := r.GetMetrics()
		if m.ReconcileTotal != 0 {
			t.Errorf("ReconcileTotal = %d, want 0", m.ReconcileTotal)
		}
		if m.DriftDetectedTotal != 0 {
			t.Errorf("DriftDetectedTotal = %d, want 0", m.DriftDetectedTotal)
		}
	})
}

func TestNewReconcilerMetrics(t *testing.T) {
	m := NewReconcilerMetrics()
	if m.DriftByType == nil {
		t.Error("DriftByType should be initialized")
	}
	if m.DriftBySeverity == nil {
		t.Error("DriftBySeverity should be initialized")
	}
	if m.ActionsByType == nil {
		t.Error("ActionsByType should be initialized")
	}
}

// ============================================================================
// TEST: GetLastDriftReport / GetActualStateSnapshot
// ============================================================================

func TestGetLastDriftReport(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	if r.GetLastDriftReport() != nil {
		t.Error("expected nil before any drift detection")
	}

	_, _ = r.DetectDrift(validDesiredState(), matchingActualState())
	report := r.GetLastDriftReport()
	if report == nil {
		t.Error("expected non-nil drift report after detection")
	}
}

func TestGetActualStateSnapshot(t *testing.T) {
	r, lxd, _ := newTestReconciler(t)
	if r.GetActualStateSnapshot() != nil {
		t.Error("expected nil before GetActualState")
	}

	lxd.containers = []*ContainerActual{
		{ID: "c1", Name: "web", Status: StatusRunning},
	}
	_, _ = r.GetActualState(context.Background())
	snap := r.GetActualStateSnapshot()
	if snap == nil {
		t.Error("expected non-nil snapshot after GetActualState")
	}
	if len(snap.Containers) != 1 {
		t.Errorf("containers = %d, want 1", len(snap.Containers))
	}
}

// ============================================================================
// TEST: DefaultReconcilerConfig
// ============================================================================

func TestDefaultReconcilerConfig(t *testing.T) {
	cfg := DefaultReconcilerConfig()
	if cfg.PollInterval == 0 {
		t.Error("PollInterval should have a default")
	}
	if cfg.MaxActionsPerSecond == 0 {
		t.Error("MaxActionsPerSecond should have a default")
	}
	if cfg.MaxRetries == 0 {
		t.Error("MaxRetries should have a default")
	}
	if cfg.EventTopic == "" {
		t.Error("EventTopic should have a default")
	}
	if cfg.MaxHistorySize == 0 {
		t.Error("MaxHistorySize should have a default")
	}
}

// ============================================================================
// TEST: DefaultLogger
// ============================================================================

func TestDefaultLogger(t *testing.T) {
	// This primarily ensures no panics. We capture stdout below.
	logger := NewDefaultLogger("debug")
	logger.Debug("debug msg", map[string]interface{}{"key": "value"})
	logger.Info("info msg", nil)
	logger.Warn("warn msg", map[string]interface{}{"count": 42})
	logger.Error("error msg", map[string]interface{}{})

	// Non-debug level should suppress Debug
	loggerInfo := NewDefaultLogger("info")
	loggerInfo.Debug("should be suppressed", nil)
}

// ============================================================================
// TEST: Helper Methods
// ============================================================================

func TestSeverityForStatus(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	tests := []struct {
		status   ContainerStatus
		expected DriftSeverity
	}{
		{StatusError, SeverityCritical},
		{StatusStopped, SeverityHigh},
		{StatusDegraded, SeverityMedium},
		{StatusPending, SeverityLow},
		{StatusCreating, SeverityLow},
		{StatusRunning, SeverityLow},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := r.severityForStatus(tt.status)
			if got != tt.expected {
				t.Errorf("severityForStatus(%s) = %s, want %s", tt.status, got, tt.expected)
			}
		})
	}
}

func TestPriorityForSeverity(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	tests := []struct {
		severity DriftSeverity
		expected int
	}{
		{SeverityCritical, 1},
		{SeverityHigh, 10},
		{SeverityMedium, 50},
		{SeverityLow, 100},
		{"unknown_severity", 50}, // default
	}
	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			got := r.priorityForSeverity(tt.severity)
			if got != tt.expected {
				t.Errorf("priorityForSeverity(%s) = %d, want %d", tt.severity, got, tt.expected)
			}
		})
	}
}

func TestHighestSeverity(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	tests := []struct {
		name     string
		drifts   []DriftItem
		expected DriftSeverity
	}{
		{
			name:     "single low",
			drifts:   []DriftItem{{Severity: SeverityLow}},
			expected: SeverityLow,
		},
		{
			name: "mixed severities returns critical",
			drifts: []DriftItem{
				{Severity: SeverityLow},
				{Severity: SeverityCritical},
				{Severity: SeverityMedium},
			},
			expected: SeverityCritical,
		},
		{
			name:     "empty drifts returns low",
			drifts:   []DriftItem{},
			expected: SeverityLow,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.highestSeverity(tt.drifts)
			if got != tt.expected {
				t.Errorf("highestSeverity = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestIncrementSeverityCount(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	summary := &DriftSummary{}
	r.incrementSeverityCount(summary, SeverityCritical)
	r.incrementSeverityCount(summary, SeverityHigh)
	r.incrementSeverityCount(summary, SeverityMedium)
	r.incrementSeverityCount(summary, SeverityLow)

	if summary.CriticalCount != 1 {
		t.Errorf("CriticalCount = %d", summary.CriticalCount)
	}
	if summary.HighCount != 1 {
		t.Errorf("HighCount = %d", summary.HighCount)
	}
	if summary.MediumCount != 1 {
		t.Errorf("MediumCount = %d", summary.MediumCount)
	}
	if summary.LowCount != 1 {
		t.Errorf("LowCount = %d", summary.LowCount)
	}
}

func TestGetContainerName(t *testing.T) {
	r, _, _ := newTestReconciler(t)

	t.Run("returns ID when no state loaded", func(t *testing.T) {
		name := r.getContainerName("unknown-id")
		if name != "unknown-id" {
			t.Errorf("name = %q, want %q", name, "unknown-id")
		}
	})

	t.Run("returns name from desired state", func(t *testing.T) {
		_ = r.SetDesiredState(validDesiredState())
		name := r.getContainerName("web")
		if name != "web" {
			t.Errorf("name = %q, want %q", name, "web")
		}
	})

	t.Run("falls back to actual state", func(t *testing.T) {
		r.mu.Lock()
		r.actualState = &ActualState{
			Containers: map[string]*ContainerActual{
				"orphan-id": {ID: "orphan-id", Name: "orphan-id"},
			},
		}
		r.mu.Unlock()

		name := r.getContainerName("orphan-id")
		if name != "orphan-id" {
			t.Errorf("name = %q, want %q", name, "orphan-id")
		}
	})

	t.Run("returns containerID as fallback", func(t *testing.T) {
		name := r.getContainerName("absolutely-unknown")
		if name != "absolutely-unknown" {
			t.Errorf("name = %q, want %q", name, "absolutely-unknown")
		}
	})
}

func TestSortActionsByPriority(t *testing.T) {
	r, _, _ := newTestReconciler(t)

	t.Run("sorts ascending by priority", func(t *testing.T) {
		actions := []*ReconciliationAction{
			{ID: "a1", Priority: 100},
			{ID: "a2", Priority: 1},
			{ID: "a3", Priority: 50},
		}
		r.sortActionsByPriority(actions)
		if actions[0].Priority != 1 || actions[1].Priority != 50 || actions[2].Priority != 100 {
			t.Errorf("not sorted: %d, %d, %d", actions[0].Priority, actions[1].Priority, actions[2].Priority)
		}
	})

	t.Run("empty slice does not panic", func(t *testing.T) {
		r.sortActionsByPriority(nil)
		r.sortActionsByPriority([]*ReconciliationAction{})
	})

	t.Run("single element", func(t *testing.T) {
		actions := []*ReconciliationAction{{ID: "a1", Priority: 42}}
		r.sortActionsByPriority(actions)
		if actions[0].Priority != 42 {
			t.Errorf("single element sort failed")
		}
	})
}

// ============================================================================
// TEST: Concurrent Safety (Race Detector)
// ============================================================================

func TestConcurrentReconcilerAccess(t *testing.T) {
	r, lxd, _ := newTestReconciler(t)
	ds := validDesiredState()
	_ = r.SetDesiredState(ds)

	lxd.mu.Lock()
	lxd.containers = []*ContainerActual{
		{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
		{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
	}
	lxd.mu.Unlock()

	var wg sync.WaitGroup

	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = r.GetDesiredState()
				_ = r.GetActualStateSnapshot()
				_ = r.GetLastDriftReport()
				_ = r.GetMetrics()
				_ = r.GetHistory(5)
				_ = r.GetLastRun()
				_ = r.IsClosed()
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_ = r.Reconcile(context.Background(), false)
			}
		}()
	}

	wg.Wait()
}

func TestConcurrentSetDesiredState(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ds := validDesiredState()
			ds.Name = fmt.Sprintf("state-%d", n)
			_ = r.SetDesiredState(ds)
		}(i)
	}
	wg.Wait()

	got := r.GetDesiredState()
	if got == nil {
		t.Fatal("expected non-nil desired state after concurrent sets")
	}
}

func TestConcurrentDetectDrift(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	desired := validDesiredState()
	actual := matchingActualState()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.DetectDrift(desired, actual)
		}()
	}
	wg.Wait()
}

// ============================================================================
// TEST: Pleroma vs Kenoma State Reconciliation Pattern
// ============================================================================

func TestPleromaKenoma_FullAlignment(t *testing.T) {
	// Pleroma = desired (full/complete), Kenoma = actual (observed/material)
	// When Pleroma == Kenoma, system is in equilibrium
	r, lxd, _ := newTestReconciler(t)
	ds := validDesiredState()
	_ = r.SetDesiredState(ds)

	lxd.containers = []*ContainerActual{
		{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
		{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
	}

	err := r.Reconcile(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}

	run := r.GetLastRun()
	if run.DriftReport.DriftCount != 0 {
		t.Errorf("fully aligned state should have 0 drifts, got %d", run.DriftReport.DriftCount)
	}
	if run.ActionsTotal != 0 {
		t.Error("no actions should be generated when states are aligned")
	}
}

func TestPleromaKenoma_KenomaDegradation(t *testing.T) {
	// When Kenoma degrades from Pleroma, drift represents the gap
	r, lxd, _ := newTestReconciler(t)
	ds := validDesiredState()
	_ = r.SetDesiredState(ds)

	// Kenoma is degraded: web is stopped, api has wrong image
	lxd.containers = []*ContainerActual{
		{ID: "c1", Name: "web", Status: StatusStopped, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthUnhealthy},
		{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.20", CPU: 4, Memory: 1024, Health: HealthHealthy},
	}

	err := r.Reconcile(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}

	run := r.GetLastRun()
	if run.DriftReport.DriftCount == 0 {
		t.Error("expected drifts when Kenoma is degraded from Pleroma")
	}
	// Reconciliation should attempt to bring Kenoma back toward Pleroma
	if run.ActionsTotal == 0 {
		t.Error("expected reconciliation actions")
	}
}

func TestPleromaKenoma_KenomaSurplus(t *testing.T) {
	// Kenoma has elements not in Pleroma (orphaned containers)
	r, lxd, _ := newTestReconciler(t)
	ds := validDesiredState()
	_ = r.SetDesiredState(ds)

	lxd.containers = []*ContainerActual{
		{ID: "c1", Name: "web", Status: StatusRunning, Image: "ubuntu:22.04", CPU: 2, Memory: 512, Health: HealthHealthy},
		{ID: "c2", Name: "api", Status: StatusRunning, Image: "golang:1.21", CPU: 4, Memory: 1024, Health: HealthHealthy},
		{ID: "c3", Name: "rogue-service", Status: StatusRunning, Health: HealthHealthy},
	}

	err := r.Reconcile(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}

	run := r.GetLastRun()
	foundOrphan := false
	for _, d := range run.DriftReport.Drifts {
		if d.DriftType == DriftOrphaned {
			foundOrphan = true
			break
		}
	}
	if !foundOrphan {
		t.Error("expected orphaned drift for surplus Kenoma element")
	}
}

func TestPleromaKenoma_CompleteVoid(t *testing.T) {
	// Kenoma is completely empty - all Pleroma elements missing
	r, lxd, _ := newTestReconciler(t)
	ds := validDesiredState()
	_ = r.SetDesiredState(ds)

	lxd.containers = []*ContainerActual{} // empty Kenoma

	err := r.Reconcile(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}

	run := r.GetLastRun()
	if run.DriftReport.Summary.Missing != len(ds.Containers) {
		t.Errorf("Missing = %d, want %d (all containers)", run.DriftReport.Summary.Missing, len(ds.Containers))
	}
}

// ============================================================================
// TEST: Edge Cases
// ============================================================================

func TestContainerDesired_Validate_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		c       *ContainerDesired
		id      string
		wantErr bool
	}{
		{
			name:    "nil container",
			c:       nil,
			id:      "test",
			wantErr: true,
		},
		{
			name:    "zero CPU is valid",
			c:       &ContainerDesired{Name: "x", Image: "img", CPU: 0},
			id:      "x",
			wantErr: false,
		},
		{
			name:    "zero memory is valid",
			c:       &ContainerDesired{Name: "x", Image: "img", Memory: 0},
			id:      "x",
			wantErr: false,
		},
		{
			name:    "large CPU is valid",
			c:       &ContainerDesired{Name: "x", Image: "img", CPU: 128},
			id:      "x",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c.Validate(tt.id)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDetectDrift_EmptyActualState(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	desired := validDesiredState()
	actual := &ActualState{
		Containers: map[string]*ContainerActual{},
		ObservedAt: time.Now(),
	}
	report, err := r.DetectDrift(desired, actual)
	if err != nil {
		t.Fatal(err)
	}

	// Count enabled containers
	enabled := 0
	for _, c := range desired.Containers {
		if c.Enabled {
			enabled++
		}
	}
	if report.DriftCount != enabled {
		t.Errorf("DriftCount = %d, want %d (all enabled containers missing)", report.DriftCount, enabled)
	}
}

func TestDetectDrift_DisabledContainersIgnored(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	desired := &DesiredState{
		Version: "1.0",
		Name:    "test",
		Containers: map[string]*ContainerDesired{
			"enabled-svc": {Name: "enabled-svc", Image: "img", Enabled: true},
			"disabled-svc": {Name: "disabled-svc", Image: "img", Enabled: false},
		},
	}
	actual := &ActualState{
		Containers: map[string]*ContainerActual{},
		ObservedAt: time.Now(),
	}
	report, err := r.DetectDrift(desired, actual)
	if err != nil {
		t.Fatal(err)
	}
	if report.DriftCount != 1 {
		t.Errorf("DriftCount = %d, want 1 (only enabled container)", report.DriftCount)
	}
}

func TestReconcile_WithActionFailure(t *testing.T) {
	r, lxd, _ := newTestReconciler(t)
	ds := validDesiredState()
	_ = r.SetDesiredState(ds)

	lxd.containers = []*ContainerActual{} // all missing
	lxd.createContainerErr = errors.New("disk full")

	err := r.Reconcile(context.Background(), false)
	// Reconcile itself doesn't return error for individual action failures
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	run := r.GetLastRun()
	if run.Success {
		t.Error("expected run to be marked as failed when actions fail")
	}
	if run.ActionsFailed == 0 {
		t.Error("expected at least one failed action")
	}

	m := r.GetMetrics()
	if m.ReconcileFailedTotal != 1 {
		t.Errorf("ReconcileFailedTotal = %d, want 1", m.ReconcileFailedTotal)
	}
}

func TestDriftReport_IDsAreUnique(t *testing.T) {
	r, _, _ := newTestReconciler(t)
	desired := validDesiredState()
	actual := &ActualState{
		Containers: map[string]*ContainerActual{},
		ObservedAt: time.Now(),
	}

	ids := make(map[string]bool)
	for i := 0; i < 5; i++ {
		report, _ := r.DetectDrift(desired, actual)
		if ids[report.ID] {
			t.Errorf("duplicate report ID: %s", report.ID)
		}
		ids[report.ID] = true
	}
}

// ============================================================================
// TEST: Status and Health Constants
// ============================================================================

func TestContainerStatusConstants(t *testing.T) {
	statuses := []ContainerStatus{
		StatusPending, StatusCreating, StatusRunning, StatusStopped,
		StatusError, StatusDegraded, StatusDeleting, StatusNotFound,
	}
	seen := make(map[ContainerStatus]bool)
	for _, s := range statuses {
		if s == "" {
			t.Error("empty status constant")
		}
		if seen[s] {
			t.Errorf("duplicate status: %s", s)
		}
		seen[s] = true
	}
}

func TestDriftTypeConstants(t *testing.T) {
	types := []DriftType{
		DriftNone, DriftMissing, DriftOrphaned, DriftStatusMismatch,
		DriftConfigMismatch, DriftHealthDegraded, DriftResourceDrift, DriftNetworkDrift,
	}
	seen := make(map[DriftType]bool)
	for _, dt := range types {
		if dt == "" {
			t.Error("empty drift type constant")
		}
		if seen[dt] {
			t.Errorf("duplicate drift type: %s", dt)
		}
		seen[dt] = true
	}
}

func TestActionTypeConstants(t *testing.T) {
	types := []ActionType{
		ActionCreate, ActionStart, ActionStop, ActionRestart,
		ActionDelete, ActionUpdate, ActionRepair, ActionScale,
	}
	seen := make(map[ActionType]bool)
	for _, at := range types {
		if at == "" {
			t.Error("empty action type constant")
		}
		if seen[at] {
			t.Errorf("duplicate action type: %s", at)
		}
		seen[at] = true
	}
}

func TestEventTypeConstants(t *testing.T) {
	types := []EventType{
		EventDriftDetected, EventReconcileStarted, EventReconcileCompleted,
		EventReconcileFailed, EventActionStarted, EventActionCompleted,
		EventActionFailed, EventContainerCreated, EventContainerStarted,
		EventContainerStopped, EventContainerDeleted, EventContainerHealthy,
		EventContainerUnhealthy,
	}
	seen := make(map[EventType]bool)
	for _, et := range types {
		if et == "" {
			t.Error("empty event type constant")
		}
		if seen[et] {
			t.Errorf("duplicate event type: %s", et)
		}
		seen[et] = true
	}
}

// ============================================================================
// UTILITY
// ============================================================================

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
