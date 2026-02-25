// Package container provides a unified container runtime interface for Unheaded
package container

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// CONTAINER SPEC TESTS
// ============================================================================

func TestContainerSpecValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    *ContainerSpec
		wantErr bool
	}{
		{
			name: "valid spec",
			spec: &ContainerSpec{
				Name:  "test-container",
				Image: "nixos:latest",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			spec: &ContainerSpec{
				Image: "nixos:latest",
			},
			wantErr: true,
		},
		{
			name: "missing image",
			spec: &ContainerSpec{
				Name: "test-container",
			},
			wantErr: true,
		},
		{
			name: "invalid name with special chars",
			spec: &ContainerSpec{
				Name:  "test@container",
				Image: "nixos:latest",
			},
			wantErr: true,
		},
		{
			name: "name with dashes and underscores",
			spec: &ContainerSpec{
				Name:  "test-container_v1",
				Image: "nixos:latest",
			},
			wantErr: false,
		},
		{
			name: "full spec with all fields",
			spec: &ContainerSpec{
				Name:     "full-container",
				Image:    "nixos:latest",
				Hostname: "full-host",
				Labels: map[string]string{
					"app": "test",
				},
				Command:    []string{"/bin/bash"},
				WorkingDir: "/home",
				Env: map[string]string{
					"ENV_VAR": "value",
				},
				Resources: ResourceSpec{
					Limits: ResourceLimits{
						CPU:    "2",
						Memory: "1Gi",
					},
				},
				Security: SecuritySpec{
					NoNewPrivileges: true,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ContainerSpec.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHealthCheckSpecValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    *HealthCheckSpec
		wantErr bool
	}{
		{
			name: "valid health check",
			spec: &HealthCheckSpec{
				Command:  []string{"curl", "-f", "http://localhost/health"},
				Interval: 30 * time.Second,
				Timeout:  5 * time.Second,
				Retries:  3,
			},
			wantErr: false,
		},
		{
			name: "missing command",
			spec: &HealthCheckSpec{
				Interval: 30 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative interval",
			spec: &HealthCheckSpec{
				Command:  []string{"curl", "-f", "http://localhost/health"},
				Interval: -1 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			spec: &HealthCheckSpec{
				Command: []string{"curl", "-f", "http://localhost/health"},
				Timeout: -1 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative retries",
			spec: &HealthCheckSpec{
				Command: []string{"curl", "-f", "http://localhost/health"},
				Retries: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("HealthCheckSpec.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// CONTAINER STATE TESTS
// ============================================================================

func TestContainerIsRunning(t *testing.T) {
	tests := []struct {
		name  string
		state ContainerState
		want  bool
	}{
		{"running", StateRunning, true},
		{"stopped", StateStopped, false},
		{"paused", StatePaused, false},
		{"exited", StateExited, false},
		{"created", StateCreated, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Container{State: tt.state}
			if got := c.IsRunning(); got != tt.want {
				t.Errorf("Container.IsRunning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerIsStopped(t *testing.T) {
	tests := []struct {
		name  string
		state ContainerState
		want  bool
	}{
		{"running", StateRunning, false},
		{"stopped", StateStopped, true},
		{"paused", StatePaused, false},
		{"exited", StateExited, true},
		{"created", StateCreated, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Container{State: tt.state}
			if got := c.IsStopped(); got != tt.want {
				t.Errorf("Container.IsStopped() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// FILTER TESTS
// ============================================================================

func TestContainerMatch(t *testing.T) {
	container := &Container{
		ID:    "abc123",
		Name:  "test-container",
		Image: "nixos:latest",
		State: StateRunning,
		Labels: map[string]string{
			"app":  "test",
			"tier": "frontend",
		},
	}

	tests := []struct {
		name   string
		filter *Filter
		want   bool
	}{
		{
			name:   "nil filter matches all",
			filter: nil,
			want:   true,
		},
		{
			name:   "empty filter matches all",
			filter: &Filter{},
			want:   true,
		},
		{
			name: "matching ID",
			filter: &Filter{
				ID: "abc123",
			},
			want: true,
		},
		{
			name: "non-matching ID",
			filter: &Filter{
				ID: "xyz789",
			},
			want: false,
		},
		{
			name: "matching name",
			filter: &Filter{
				Name: "test-container",
			},
			want: true,
		},
		{
			name: "non-matching name",
			filter: &Filter{
				Name: "other-container",
			},
			want: false,
		},
		{
			name: "matching image",
			filter: &Filter{
				Image: "nixos:latest",
			},
			want: true,
		},
		{
			name: "matching state",
			filter: &Filter{
				State: []ContainerState{StateRunning},
			},
			want: true,
		},
		{
			name: "non-matching state",
			filter: &Filter{
				State: []ContainerState{StateStopped},
			},
			want: false,
		},
		{
			name: "matching multiple states",
			filter: &Filter{
				State: []ContainerState{StateRunning, StatePaused},
			},
			want: true,
		},
		{
			name: "matching label",
			filter: &Filter{
				Labels: map[string]string{
					"app": "test",
				},
			},
			want: true,
		},
		{
			name: "non-matching label value",
			filter: &Filter{
				Labels: map[string]string{
					"app": "production",
				},
			},
			want: false,
		},
		{
			name: "missing label",
			filter: &Filter{
				Labels: map[string]string{
					"missing": "label",
				},
			},
			want: false,
		},
		{
			name: "multiple matching labels",
			filter: &Filter{
				Labels: map[string]string{
					"app":  "test",
					"tier": "frontend",
				},
			},
			want: true,
		},
		{
			name: "combined filter - matching",
			filter: &Filter{
				Name:  "test-container",
				Image: "nixos:latest",
				State: []ContainerState{StateRunning},
				Labels: map[string]string{
					"app": "test",
				},
			},
			want: true,
		},
		{
			name: "combined filter - non-matching",
			filter: &Filter{
				Name:  "test-container",
				Image: "nixos:latest",
				State: []ContainerState{StateStopped}, // This won't match
				Labels: map[string]string{
					"app": "test",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := container.Match(tt.filter); got != tt.want {
				t.Errorf("Container.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// TYPE TESTS
// ============================================================================

func TestContainerStateValues(t *testing.T) {
	states := []ContainerState{
		StateCreated,
		StateRunning,
		StatePaused,
		StateStopped,
		StateExited,
		StateRemoving,
		StateRestarting,
		StateDead,
		StateUnknown,
	}

	// Ensure all states have valid string representations
	for _, s := range states {
		if string(s) == "" {
			t.Errorf("ContainerState has empty string value")
		}
	}
}

func TestHealthStateValues(t *testing.T) {
	states := []HealthState{
		HealthStateHealthy,
		HealthStateUnhealthy,
		HealthStateStarting,
		HealthStateNone,
	}

	// Ensure all states have valid string representations
	for _, s := range states {
		if string(s) == "" {
			t.Errorf("HealthState has empty string value")
		}
	}
}

func TestMountTypeValues(t *testing.T) {
	types := []MountType{
		MountTypeBind,
		MountTypeVolume,
		MountTypeTmpfs,
		MountTypeDisk,
	}

	// Ensure all types have valid string representations
	for _, mt := range types {
		if string(mt) == "" {
			t.Errorf("MountType has empty string value")
		}
	}
}

func TestNetworkModeValues(t *testing.T) {
	modes := []NetworkMode{
		NetworkModeBridge,
		NetworkModeHost,
		NetworkModeNone,
		NetworkModeManaged,
	}

	// Ensure all modes have valid string representations
	for _, m := range modes {
		if string(m) == "" {
			t.Errorf("NetworkMode has empty string value")
		}
	}

	// Default mode can be empty
	if NetworkModeDefault != "" {
		t.Errorf("NetworkModeDefault should be empty string, got %q", NetworkModeDefault)
	}
}

func TestRestartPolicyTypeValues(t *testing.T) {
	policies := []RestartPolicyType{
		RestartPolicyNone,
		RestartPolicyAlways,
		RestartPolicyOnFailure,
		RestartPolicyUnlessStopped,
	}

	// Ensure all policies have valid string representations
	for _, p := range policies {
		if string(p) == "" {
			t.Errorf("RestartPolicyType has empty string value")
		}
	}
}

func TestBackendTypeValues(t *testing.T) {
	backends := []BackendType{
		BackendLXD,
		BackendDocker,
		BackendPodman,
	}

	// Ensure all backends have valid string representations
	for _, b := range backends {
		if string(b) == "" {
			t.Errorf("BackendType has empty string value")
		}
	}
}

// ============================================================================
// MOCK RUNTIME FOR TESTING
// ============================================================================

// MockRuntime implements Runtime interface for testing
type MockRuntime struct {
	containers map[string]*Container
}

// NewMockRuntime creates a new mock runtime
func NewMockRuntime() *MockRuntime {
	return &MockRuntime{
		containers: make(map[string]*Container),
	}
}

func (m *MockRuntime) Create(ctx context.Context, spec *ContainerSpec) (*Container, error) {
	if spec == nil {
		return nil, ErrInvalidSpec
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	if _, exists := m.containers[spec.Name]; exists {
		return nil, ErrContainerExists
	}

	c := &Container{
		ID:        spec.Name,
		Name:      spec.Name,
		Image:     spec.Image,
		State:     StateCreated,
		CreatedAt: time.Now(),
		Spec:      spec,
		Backend:   BackendLXD,
	}

	m.containers[spec.Name] = c
	return c, nil
}

func (m *MockRuntime) Start(ctx context.Context, id string) error {
	c, exists := m.containers[id]
	if !exists {
		return ErrContainerNotFound
	}
	c.State = StateRunning
	c.StartedAt = time.Now()
	return nil
}

func (m *MockRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	c, exists := m.containers[id]
	if !exists {
		return ErrContainerNotFound
	}
	c.State = StateStopped
	c.FinishedAt = time.Now()
	return nil
}

func (m *MockRuntime) Restart(ctx context.Context, id string, timeout time.Duration) error {
	if err := m.Stop(ctx, id, timeout); err != nil {
		return err
	}
	return m.Start(ctx, id)
}

func (m *MockRuntime) Pause(ctx context.Context, id string) error {
	c, exists := m.containers[id]
	if !exists {
		return ErrContainerNotFound
	}
	c.State = StatePaused
	return nil
}

func (m *MockRuntime) Unpause(ctx context.Context, id string) error {
	c, exists := m.containers[id]
	if !exists {
		return ErrContainerNotFound
	}
	c.State = StateRunning
	return nil
}

func (m *MockRuntime) Delete(ctx context.Context, id string, force bool) error {
	if _, exists := m.containers[id]; !exists {
		return ErrContainerNotFound
	}
	delete(m.containers, id)
	return nil
}

func (m *MockRuntime) Get(ctx context.Context, id string) (*Container, error) {
	c, exists := m.containers[id]
	if !exists {
		return nil, ErrContainerNotFound
	}
	return c, nil
}

func (m *MockRuntime) List(ctx context.Context, filter *Filter) ([]*Container, error) {
	result := make([]*Container, 0, len(m.containers))
	for _, c := range m.containers {
		if c.Match(filter) {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *MockRuntime) Exists(ctx context.Context, id string) (bool, error) {
	_, exists := m.containers[id]
	return exists, nil
}

func (m *MockRuntime) Exec(ctx context.Context, id string, cmd *ExecConfig) (*ExecResult, error) {
	if _, exists := m.containers[id]; !exists {
		return nil, ErrContainerNotFound
	}
	return &ExecResult{ExitCode: 0, Stdout: []byte("mock output")}, nil
}

func (m *MockRuntime) Logs(ctx context.Context, id string, opts *LogOptions) (interface{ Close() error }, error) {
	if _, exists := m.containers[id]; !exists {
		return nil, ErrContainerNotFound
	}
	return nil, nil
}

func (m *MockRuntime) Attach(ctx context.Context, id string, opts *AttachOptions) (*AttachResult, error) {
	return nil, nil
}

func (m *MockRuntime) Stats(ctx context.Context, id string) (*Stats, error) {
	if _, exists := m.containers[id]; !exists {
		return nil, ErrContainerNotFound
	}
	return &Stats{ContainerID: id, Timestamp: time.Now()}, nil
}

func (m *MockRuntime) StatsStream(ctx context.Context, id string) (<-chan *Stats, error) {
	return nil, nil
}

func (m *MockRuntime) HealthCheck(ctx context.Context, id string) (*HealthStatus, error) {
	return &HealthStatus{Status: HealthStateHealthy}, nil
}

func (m *MockRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	return &RuntimeInfo{Backend: BackendLXD, Version: "mock"}, nil
}

func (m *MockRuntime) Version(ctx context.Context) (*RuntimeVersion, error) {
	return &RuntimeVersion{Version: "mock"}, nil
}

func (m *MockRuntime) Close() error {
	return nil
}

// ============================================================================
// MOCK RUNTIME TESTS
// ============================================================================

func TestMockRuntimeLifecycle(t *testing.T) {
	ctx := context.Background()
	runtime := NewMockRuntime()

	// Create container
	spec := &ContainerSpec{
		Name:  "test-container",
		Image: "nixos:latest",
	}

	c, err := runtime.Create(ctx, spec)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if c.State != StateCreated {
		t.Errorf("Container state = %v, want %v", c.State, StateCreated)
	}

	// Start container
	err = runtime.Start(ctx, c.ID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c, _ = runtime.Get(ctx, c.ID)
	if c.State != StateRunning {
		t.Errorf("Container state = %v, want %v", c.State, StateRunning)
	}

	// Pause container
	err = runtime.Pause(ctx, c.ID)
	if err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	c, _ = runtime.Get(ctx, c.ID)
	if c.State != StatePaused {
		t.Errorf("Container state = %v, want %v", c.State, StatePaused)
	}

	// Unpause container
	err = runtime.Unpause(ctx, c.ID)
	if err != nil {
		t.Fatalf("Unpause() error = %v", err)
	}
	c, _ = runtime.Get(ctx, c.ID)
	if c.State != StateRunning {
		t.Errorf("Container state = %v, want %v", c.State, StateRunning)
	}

	// Stop container
	err = runtime.Stop(ctx, c.ID, 10*time.Second)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	c, _ = runtime.Get(ctx, c.ID)
	if c.State != StateStopped {
		t.Errorf("Container state = %v, want %v", c.State, StateStopped)
	}

	// Delete container
	err = runtime.Delete(ctx, c.ID, false)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	exists, _ := runtime.Exists(ctx, c.ID)
	if exists {
		t.Error("Container should not exist after deletion")
	}
}

func TestMockRuntimeErrors(t *testing.T) {
	ctx := context.Background()
	runtime := NewMockRuntime()

	// Create container
	spec := &ContainerSpec{
		Name:  "test-container",
		Image: "nixos:latest",
	}
	_, err := runtime.Create(ctx, spec)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Try to create duplicate
	_, err = runtime.Create(ctx, spec)
	if err != ErrContainerExists {
		t.Errorf("Create() duplicate error = %v, want %v", err, ErrContainerExists)
	}

	// Try operations on non-existent container
	_, err = runtime.Get(ctx, "non-existent")
	if err != ErrContainerNotFound {
		t.Errorf("Get() non-existent error = %v, want %v", err, ErrContainerNotFound)
	}

	err = runtime.Start(ctx, "non-existent")
	if err != ErrContainerNotFound {
		t.Errorf("Start() non-existent error = %v, want %v", err, ErrContainerNotFound)
	}

	err = runtime.Stop(ctx, "non-existent", 10*time.Second)
	if err != ErrContainerNotFound {
		t.Errorf("Stop() non-existent error = %v, want %v", err, ErrContainerNotFound)
	}

	err = runtime.Delete(ctx, "non-existent", false)
	if err != ErrContainerNotFound {
		t.Errorf("Delete() non-existent error = %v, want %v", err, ErrContainerNotFound)
	}
}
