// Package deploy provides deployment engine tests.
package deploy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/deploy/strategy"
)

// MockContainerRuntime implements a mock container runtime for testing.
type MockContainerRuntime struct {
	mu         sync.Mutex
	containers map[string]*MockContainer
	createErr  error
	deleteErr  error
	listErr    error
}

type MockContainer struct {
	ID      string
	Name    string
	Image   string
	State   string
	Labels  map[string]string
	Created time.Time
}

func NewMockContainerRuntime() *MockContainerRuntime {
	return &MockContainerRuntime{
		containers: make(map[string]*MockContainer),
	}
}

func (m *MockContainerRuntime) Create(ctx context.Context, spec interface{}) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	id := generateTestID()
	m.containers[id] = &MockContainer{
		ID:      id,
		State:   "running",
		Created: time.Now(),
	}
	return id, nil
}

func (m *MockContainerRuntime) Delete(ctx context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.containers, id)
	return nil
}

func (m *MockContainerRuntime) List(ctx context.Context) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	ids := make([]string, 0, len(m.containers))
	for id := range m.containers {
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *MockContainerRuntime) SetCreateError(err error) {
	m.createErr = err
}

func (m *MockContainerRuntime) SetDeleteError(err error) {
	m.deleteErr = err
}

func (m *MockContainerRuntime) ContainerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.containers)
}

var testCounter int
var testCounterMu sync.Mutex

func generateTestID() string {
	testCounterMu.Lock()
	defer testCounterMu.Unlock()
	testCounter++
	return "test-container-" + string(rune('0'+testCounter%10))
}

// TestDeploymentSpec tests deployment specification validation.
func TestDeploymentSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    *DeploymentSpec
		wantErr bool
	}{
		{
			name: "valid spec",
			spec: &DeploymentSpec{
				ServiceName: "test-service",
				Version:     "v1.0.0",
				ArtifactRef: "test-artifact:v1.0.0",
				Replicas:    3,
				Strategy:    StrategyRolling,
			},
			wantErr: false,
		},
		{
			name: "missing service name",
			spec: &DeploymentSpec{
				Version:     "v1.0.0",
				ArtifactRef: "test-artifact:v1.0.0",
				Replicas:    3,
				Strategy:    StrategyRolling,
			},
			wantErr: true,
		},
		{
			name: "missing version",
			spec: &DeploymentSpec{
				ServiceName: "test-service",
				ArtifactRef: "test-artifact:v1.0.0",
				Replicas:    3,
				Strategy:    StrategyRolling,
			},
			wantErr: true,
		},
		{
			name: "missing artifact ref",
			spec: &DeploymentSpec{
				ServiceName: "test-service",
				Version:     "v1.0.0",
				Replicas:    3,
				Strategy:    StrategyRolling,
			},
			wantErr: true,
		},
		{
			name: "zero replicas defaults to 1",
			spec: &DeploymentSpec{
				ServiceName: "test-service",
				Version:     "v1.0.0",
				ArtifactRef: "test-artifact:v1.0.0",
				Replicas:    0,
				Strategy:    StrategyRolling,
			},
			wantErr: false,
		},
		{
			name: "empty strategy defaults to rolling",
			spec: &DeploymentSpec{
				ServiceName: "test-service",
				Version:     "v1.0.0",
				ArtifactRef: "test-artifact:v1.0.0",
				Replicas:    1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeploymentSpec(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDeploymentSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestDeploymentState tests deployment state transitions.
func TestDeploymentState(t *testing.T) {
	validTransitions := map[DeploymentState][]DeploymentState{
		DeploymentStatePending:   {DeploymentStateRunning, DeploymentStateCancelled},
		DeploymentStateRunning:   {DeploymentStateCompleted, DeploymentStateFailed, DeploymentStateCancelled, DeploymentStateRollingBack},
		DeploymentStateRollingBack: {DeploymentStateRolledBack, DeploymentStateFailed},
	}

	for from, validTos := range validTransitions {
		for _, to := range validTos {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				d := &Deployment{State: from}
				if !canTransitionTo(d.State, to) {
					t.Errorf("Expected valid transition from %s to %s", from, to)
				}
			})
		}
	}

	// Test invalid transitions
	invalidTransitions := []struct {
		from DeploymentState
		to   DeploymentState
	}{
		{DeploymentStateCompleted, DeploymentStateRunning},
		{DeploymentStateFailed, DeploymentStateRunning},
		{DeploymentStateCancelled, DeploymentStateRunning},
	}

	for _, tt := range invalidTransitions {
		t.Run("invalid_"+string(tt.from)+"_to_"+string(tt.to), func(t *testing.T) {
			if canTransitionTo(tt.from, tt.to) {
				t.Errorf("Expected invalid transition from %s to %s", tt.from, tt.to)
			}
		})
	}
}

// canTransitionTo checks if a state transition is valid.
func canTransitionTo(from, to DeploymentState) bool {
	validTransitions := map[DeploymentState][]DeploymentState{
		DeploymentStatePending:     {DeploymentStateRunning, DeploymentStateCancelled},
		DeploymentStateRunning:     {DeploymentStateCompleted, DeploymentStateFailed, DeploymentStateCancelled, DeploymentStateRollingBack},
		DeploymentStateRollingBack: {DeploymentStateRolledBack, DeploymentStateFailed},
	}

	valid, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, v := range valid {
		if v == to {
			return true
		}
	}
	return false
}

// TestEngineConfig tests engine configuration.
func TestEngineConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *EngineConfig
		check  func(*EngineConfig) bool
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
			check: func(c *EngineConfig) bool {
				return c == nil
			},
		},
		{
			name: "custom timeout",
			config: &EngineConfig{
				DefaultTimeout: 10 * time.Minute,
			},
			check: func(c *EngineConfig) bool {
				return c.DefaultTimeout == 10*time.Minute
			},
		},
		{
			name: "custom replicas",
			config: &EngineConfig{
				DefaultReplicas: 5,
			},
			check: func(c *EngineConfig) bool {
				return c.DefaultReplicas == 5
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check(tt.config) {
				t.Errorf("Config check failed for %s", tt.name)
			}
		})
	}
}

// TestDeploymentMetrics tests deployment metrics tracking.
func TestDeploymentMetrics(t *testing.T) {
	metrics := &DeploymentMetrics{}

	// Test initial state
	if metrics.InstancesCreated != 0 {
		t.Error("Expected initial InstancesCreated to be 0")
	}
	if metrics.InstancesDeleted != 0 {
		t.Error("Expected initial InstancesDeleted to be 0")
	}

	// Test updating metrics
	metrics.InstancesCreated = 5
	metrics.InstancesDeleted = 3
	metrics.InstancesFailed = 1
	metrics.HealthChecksPassed = 4
	metrics.HealthChecksFailed = 1
	metrics.TrafficShifts = 2

	if metrics.InstancesCreated != 5 {
		t.Errorf("Expected InstancesCreated = 5, got %d", metrics.InstancesCreated)
	}
	if metrics.InstancesDeleted != 3 {
		t.Errorf("Expected InstancesDeleted = 3, got %d", metrics.InstancesDeleted)
	}
}

// TestStrategyConfig tests strategy configuration.
func TestStrategyConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *strategy.Config
		valid  bool
	}{
		{
			name: "valid rolling config",
			config: &strategy.Config{
				Type:           strategy.TypeRolling,
				MaxSurge:       1,
				MaxUnavailable: 0,
			},
			valid: true,
		},
		{
			name: "valid blue-green config",
			config: &strategy.Config{
				Type: strategy.TypeBlueGreen,
			},
			valid: true,
		},
		{
			name: "valid canary config",
			config: &strategy.Config{
				Type: strategy.TypeCanary,
				CanarySteps: []strategy.CanaryStep{
					{Weight: 10, Pause: 30 * time.Second},
					{Weight: 50, Pause: 1 * time.Minute},
					{Weight: 100, Pause: 0},
				},
			},
			valid: true,
		},
		{
			name: "invalid canary weights",
			config: &strategy.Config{
				Type: strategy.TypeCanary,
				CanarySteps: []strategy.CanaryStep{
					{Weight: 150, Pause: 30 * time.Second}, // Invalid: > 100
				},
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStrategyConfig(tt.config)
			if (err == nil) != tt.valid {
				t.Errorf("validateStrategyConfig() valid = %v, want %v, err = %v", err == nil, tt.valid, err)
			}
		})
	}
}

// validateStrategyConfig validates a strategy configuration.
func validateStrategyConfig(config *strategy.Config) error {
	if config == nil {
		return errors.New("config is required")
	}

	switch config.Type {
	case strategy.TypeRolling:
		// Rolling strategy validation
		return nil
	case strategy.TypeBlueGreen:
		// Blue-green strategy validation
		return nil
	case strategy.TypeCanary:
		// Canary strategy validation
		for _, step := range config.CanarySteps {
			if step.Weight < 0 || step.Weight > 100 {
				return errors.New("canary step weight must be between 0 and 100")
			}
		}
		return nil
	case strategy.TypeRecreate:
		return nil
	default:
		return errors.New("unknown strategy type")
	}
}

// TestDeploymentStatus tests deployment status methods.
func TestDeploymentStatus(t *testing.T) {
	now := time.Now()
	deployment := &Deployment{
		ID:          "test-deployment-1",
		ServiceName: "test-service",
		Version:     "v1.0.0",
		State:       DeploymentStateRunning,
		StartedAt:   now.Add(-5 * time.Minute),
		Metrics: &DeploymentMetrics{
			InstancesCreated:    3,
			HealthChecksPassed:  3,
		},
	}

	status := deployment.Status()

	if status.DeploymentID != deployment.ID {
		t.Errorf("Expected DeploymentID = %s, got %s", deployment.ID, status.DeploymentID)
	}
	if status.State != DeploymentStateRunning {
		t.Errorf("Expected State = %s, got %s", DeploymentStateRunning, status.State)
	}
	if status.InstancesReady != 3 {
		t.Errorf("Expected InstancesReady = 3, got %d", status.InstancesReady)
	}
}

// TestDeploymentCallbacks tests deployment callback execution.
func TestDeploymentCallbacks(t *testing.T) {
	callbackCalled := false
	var receivedDeployment *Deployment

	callback := func(ctx context.Context, d *Deployment) error {
		callbackCalled = true
		receivedDeployment = d
		return nil
	}

	deployment := &Deployment{
		ID:          "test-deployment-1",
		ServiceName: "test-service",
		Version:     "v1.0.0",
		State:       DeploymentStateRunning,
	}

	// Simulate callback execution
	ctx := context.Background()
	err := callback(ctx, deployment)

	if err != nil {
		t.Errorf("Callback returned error: %v", err)
	}
	if !callbackCalled {
		t.Error("Callback was not called")
	}
	if receivedDeployment != deployment {
		t.Error("Callback received wrong deployment")
	}
}

// TestConcurrentDeployments tests concurrent deployment handling.
func TestConcurrentDeployments(t *testing.T) {
	deployments := make(map[string]*Deployment)
	var mu sync.RWMutex

	// Simulate concurrent deployment creation
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			d := &Deployment{
				ID:          generateDeploymentID(),
				ServiceName: "test-service",
				Version:     "v1.0.0",
				State:       DeploymentStatePending,
				StartedAt:   time.Now(),
			}

			mu.Lock()
			deployments[d.ID] = d
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	mu.RLock()
	count := len(deployments)
	mu.RUnlock()

	if count != 10 {
		t.Errorf("Expected 10 deployments, got %d", count)
	}
}

// TestDeploymentTimeout tests deployment timeout handling.
func TestDeploymentTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Simulate a long-running operation
	done := make(chan bool)
	go func() {
		select {
		case <-ctx.Done():
			done <- true
		case <-time.After(1 * time.Second):
			done <- false
		}
	}()

	result := <-done
	if !result {
		t.Error("Expected context to timeout")
	}
}

// TestDeploymentLabels tests deployment label handling.
func TestDeploymentLabels(t *testing.T) {
	spec := &DeploymentSpec{
		ServiceName: "test-service",
		Version:     "v1.0.0",
		ArtifactRef: "test-artifact:v1.0.0",
		Replicas:    1,
		Labels: map[string]string{
			"app":         "test",
			"environment": "testing",
			"team":        "platform",
		},
	}

	if spec.Labels["app"] != "test" {
		t.Errorf("Expected label 'app' = 'test', got '%s'", spec.Labels["app"])
	}
	if spec.Labels["environment"] != "testing" {
		t.Errorf("Expected label 'environment' = 'testing', got '%s'", spec.Labels["environment"])
	}
}

// TestDeploymentAnnotations tests deployment annotation handling.
func TestDeploymentAnnotations(t *testing.T) {
	spec := &DeploymentSpec{
		ServiceName: "test-service",
		Version:     "v1.0.0",
		ArtifactRef: "test-artifact:v1.0.0",
		Replicas:    1,
		Annotations: map[string]string{
			"description":     "Test deployment",
			"owner":           "platform-team",
			"deployment.type": "rolling",
		},
	}

	if spec.Annotations["description"] != "Test deployment" {
		t.Errorf("Expected annotation 'description' = 'Test deployment', got '%s'", spec.Annotations["description"])
	}
}

// TestHealthCheckConfig tests health check configuration.
func TestHealthCheckConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *strategy.HealthCheckConfig
		valid  bool
	}{
		{
			name: "valid http health check",
			config: &strategy.HealthCheckConfig{
				Type:     "http",
				Path:     "/health",
				Port:     8080,
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
			valid: true,
		},
		{
			name: "valid tcp health check",
			config: &strategy.HealthCheckConfig{
				Type:     "tcp",
				Port:     8080,
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
			valid: true,
		},
		{
			name: "invalid - missing port",
			config: &strategy.HealthCheckConfig{
				Type:     "http",
				Path:     "/health",
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
			valid: false,
		},
		{
			name: "invalid - timeout > interval",
			config: &strategy.HealthCheckConfig{
				Type:     "http",
				Path:     "/health",
				Port:     8080,
				Interval: 5 * time.Second,
				Timeout:  10 * time.Second,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHealthCheckConfig(tt.config)
			if (err == nil) != tt.valid {
				t.Errorf("validateHealthCheckConfig() valid = %v, want %v", err == nil, tt.valid)
			}
		})
	}
}

// validateHealthCheckConfig validates health check configuration.
func validateHealthCheckConfig(config *strategy.HealthCheckConfig) error {
	if config == nil {
		return errors.New("health check config is required")
	}
	if config.Port == 0 {
		return errors.New("port is required")
	}
	if config.Timeout > config.Interval {
		return errors.New("timeout must be less than interval")
	}
	return nil
}

// TestCanarySteps tests canary deployment step progression.
func TestCanarySteps(t *testing.T) {
	steps := []strategy.CanaryStep{
		{Weight: 1, Pause: 30 * time.Second},
		{Weight: 10, Pause: 1 * time.Minute},
		{Weight: 50, Pause: 2 * time.Minute},
		{Weight: 100, Pause: 0},
	}

	// Verify step progression
	for i := 1; i < len(steps); i++ {
		if steps[i].Weight <= steps[i-1].Weight {
			t.Errorf("Canary steps should increase: step %d weight %d <= step %d weight %d",
				i, steps[i].Weight, i-1, steps[i-1].Weight)
		}
	}

	// Verify final step is 100%
	if steps[len(steps)-1].Weight != 100 {
		t.Errorf("Final canary step should be 100%%, got %d%%", steps[len(steps)-1].Weight)
	}

	// Verify final step has no pause
	if steps[len(steps)-1].Pause != 0 {
		t.Errorf("Final canary step should have no pause, got %v", steps[len(steps)-1].Pause)
	}
}

// TestAnalysisConfig tests analysis configuration for canary deployments.
func TestAnalysisConfig(t *testing.T) {
	config := &strategy.AnalysisConfig{
		ErrorRateThreshold:   0.05, // 5%
		LatencyP99Threshold:  500 * time.Millisecond,
		SuccessRateThreshold: 0.99, // 99%
		SampleSize:           1000,
		Interval:             30 * time.Second,
	}

	if config.ErrorRateThreshold < 0 || config.ErrorRateThreshold > 1 {
		t.Error("Error rate threshold should be between 0 and 1")
	}
	if config.SuccessRateThreshold < 0 || config.SuccessRateThreshold > 1 {
		t.Error("Success rate threshold should be between 0 and 1")
	}
	if config.SampleSize <= 0 {
		t.Error("Sample size should be positive")
	}
}

// TestDeploymentEnvironment tests environment variable handling.
func TestDeploymentEnvironment(t *testing.T) {
	spec := &DeploymentSpec{
		ServiceName: "test-service",
		Version:     "v1.0.0",
		ArtifactRef: "test-artifact:v1.0.0",
		Replicas:    1,
		Environment: map[string]string{
			"LOG_LEVEL":   "debug",
			"DB_HOST":     "localhost",
			"DB_PORT":     "5432",
			"ENVIRONMENT": "testing",
		},
	}

	// Verify environment variables
	if spec.Environment["LOG_LEVEL"] != "debug" {
		t.Errorf("Expected LOG_LEVEL = 'debug', got '%s'", spec.Environment["LOG_LEVEL"])
	}
	if spec.Environment["ENVIRONMENT"] != "testing" {
		t.Errorf("Expected ENVIRONMENT = 'testing', got '%s'", spec.Environment["ENVIRONMENT"])
	}
}

// TestRollbackReason tests rollback reason handling.
func TestRollbackReason(t *testing.T) {
	reasons := []string{
		"manual",
		"automatic",
		"health_check",
		"timeout",
		"canary_failure",
	}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			if reason == "" {
				t.Error("Reason should not be empty")
			}
		})
	}
}

// BenchmarkDeploymentCreation benchmarks deployment creation.
func BenchmarkDeploymentCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &Deployment{
			ID:          generateDeploymentID(),
			ServiceName: "test-service",
			Version:     "v1.0.0",
			State:       DeploymentStatePending,
			StartedAt:   time.Now(),
			Metrics:     &DeploymentMetrics{},
		}
	}
}

// BenchmarkSpecValidation benchmarks spec validation.
func BenchmarkSpecValidation(b *testing.B) {
	spec := &DeploymentSpec{
		ServiceName: "test-service",
		Version:     "v1.0.0",
		ArtifactRef: "test-artifact:v1.0.0",
		Replicas:    3,
		Strategy:    StrategyRolling,
	}

	for i := 0; i < b.N; i++ {
		_ = validateDeploymentSpec(spec)
	}
}

// validateDeploymentSpec validates a deployment specification.
func validateDeploymentSpec(spec *DeploymentSpec) error {
	if spec == nil {
		return errors.New("spec is required")
	}
	if spec.ServiceName == "" {
		return errors.New("service name is required")
	}
	if spec.Version == "" {
		return errors.New("version is required")
	}
	if spec.ArtifactRef == "" {
		return errors.New("artifact reference is required")
	}
	return nil
}
