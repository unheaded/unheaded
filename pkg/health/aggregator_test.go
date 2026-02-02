// Package health provides a health check aggregator for the Unheaded Kingdom infrastructure.
package health

import (
	"sync"
	"testing"
	"time"
)

func TestHealthStatusConstants(t *testing.T) {
	statuses := []HealthStatus{
		StatusHealthy,
		StatusDegraded,
		StatusUnhealthy,
		StatusUnknown,
	}

	for _, s := range statuses {
		if s == "" {
			t.Error("health status should not be empty")
		}
	}
}

func TestCheckTypeConstants(t *testing.T) {
	checkTypes := []CheckType{
		CheckTypeHTTP,
		CheckTypeTCP,
		CheckTypeGRPC,
		CheckTypeExec,
		CheckTypeBusboy,
	}

	for _, ct := range checkTypes {
		if ct == "" {
			t.Error("check type should not be empty")
		}
	}
}

func TestHealthCheck(t *testing.T) {
	check := &HealthCheck{
		Name:             "test-service",
		Type:             CheckTypeHTTP,
		Target:           "http://localhost:8080/health",
		Timeout:          5 * time.Second,
		Interval:         30 * time.Second,
		SuccessThreshold: 1,
		FailureThreshold: 3,
		Tags:             []string{"critical", "api"},
		Metadata: map[string]string{
			"team": "platform",
		},
		HTTPConfig: &HTTPCheckConfig{
			Method:         "GET",
			ExpectedStatus: 200,
			Headers: map[string]string{
				"Accept": "application/json",
			},
		},
	}

	if check.Name != "test-service" {
		t.Errorf("expected name 'test-service', got '%s'", check.Name)
	}
	if check.Type != CheckTypeHTTP {
		t.Errorf("expected type HTTP, got '%s'", check.Type)
	}
	if check.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", check.Timeout)
	}
	if len(check.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(check.Tags))
	}
}

func TestHTTPCheckConfig(t *testing.T) {
	config := &HTTPCheckConfig{
		Method:          "POST",
		Headers:         map[string]string{"Content-Type": "application/json"},
		ExpectedStatus:  201,
		ExpectedBody:    `{"status":"ok"}`,
		FollowRedirects: true,
	}

	if config.Method != "POST" {
		t.Errorf("expected method POST, got '%s'", config.Method)
	}
	if config.ExpectedStatus != 201 {
		t.Errorf("expected status 201, got %d", config.ExpectedStatus)
	}
	if !config.FollowRedirects {
		t.Error("expected FollowRedirects to be true")
	}
}

func TestGRPCCheckConfig(t *testing.T) {
	config := &GRPCCheckConfig{
		Service: "grpc.health.v1.Health",
		UseTLS:  true,
	}

	if config.Service != "grpc.health.v1.Health" {
		t.Errorf("expected service 'grpc.health.v1.Health', got '%s'", config.Service)
	}
	if !config.UseTLS {
		t.Error("expected UseTLS to be true")
	}
}

func TestExecCheckConfig(t *testing.T) {
	config := &ExecCheckConfig{
		Args:             []string{"-c", "test"},
		Env:              []string{"DEBUG=1"},
		ExpectedExitCode: 0,
	}

	if len(config.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(config.Args))
	}
	if config.ExpectedExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", config.ExpectedExitCode)
	}
}

func TestHealthResult(t *testing.T) {
	result := &HealthResult{
		Name:                 "test-service",
		Status:               StatusHealthy,
		Message:              "Service is healthy",
		Duration:             50 * time.Millisecond,
		Timestamp:            time.Now(),
		ConsecutiveSuccesses: 5,
		ConsecutiveFailures:  0,
		Metadata: map[string]interface{}{
			"response_code": 200,
		},
	}

	if result.Status != StatusHealthy {
		t.Errorf("expected status healthy, got '%s'", result.Status)
	}
	if result.ConsecutiveSuccesses != 5 {
		t.Errorf("expected 5 consecutive successes, got %d", result.ConsecutiveSuccesses)
	}
}

func TestHealthHistory(t *testing.T) {
	history := &HealthHistory{
		Name:    "test-service",
		Results: []HealthResult{},
		MaxSize: 10,
	}

	// Add results
	for i := 0; i < 15; i++ {
		result := HealthResult{
			Name:      "test-service",
			Status:    StatusHealthy,
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		history.Add(result)
	}

	// Should be capped at MaxSize
	if len(history.Results) != 10 {
		t.Errorf("expected 10 results (MaxSize), got %d", len(history.Results))
	}

	// Most recent should be first
	if history.Results[0].Timestamp.Before(history.Results[1].Timestamp) {
		t.Error("expected most recent result first")
	}
}

func TestHealthHistoryGetSince(t *testing.T) {
	history := &HealthHistory{
		Name:    "test-service",
		Results: []HealthResult{},
		MaxSize: 100,
	}

	baseTime := time.Now()

	// Add results with different timestamps
	for i := 0; i < 10; i++ {
		result := HealthResult{
			Name:      "test-service",
			Status:    StatusHealthy,
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
		}
		history.Add(result)
	}

	// Get results since 5 minutes ago
	since := baseTime.Add(4 * time.Minute)
	results := history.GetSince(since)

	// Should get results from minute 5-9 (5 results)
	if len(results) != 5 {
		t.Errorf("expected 5 results since %v, got %d", since, len(results))
	}
}

func TestHealthHistoryGetDuration(t *testing.T) {
	history := &HealthHistory{
		Name:    "test-service",
		Results: []HealthResult{},
		MaxSize: 100,
	}

	// Add a recent result
	result := HealthResult{
		Name:      "test-service",
		Status:    StatusHealthy,
		Timestamp: time.Now(),
	}
	history.Add(result)

	// Get results from last hour
	results := history.GetDuration(1 * time.Hour)

	if len(results) != 1 {
		t.Errorf("expected 1 result in last hour, got %d", len(results))
	}
}

func TestHealthHistoryConcurrency(t *testing.T) {
	history := &HealthHistory{
		Name:    "test-service",
		Results: []HealthResult{},
		MaxSize: 100,
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result := HealthResult{
				Name:      "test-service",
				Status:    StatusHealthy,
				Timestamp: time.Now(),
			}
			history.Add(result)
		}(i)
	}
	wg.Wait()

	// Should have added 100 results but capped at MaxSize
	if len(history.Results) != 100 {
		t.Errorf("expected 100 results, got %d", len(history.Results))
	}
}

func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, 2, 30*time.Second)

	if cb.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", cb.Name)
	}
	if cb.FailureThreshold != 3 {
		t.Errorf("expected failure threshold 3, got %d", cb.FailureThreshold)
	}
	if cb.SuccessThreshold != 2 {
		t.Errorf("expected success threshold 2, got %d", cb.SuccessThreshold)
	}
	if cb.State != "closed" {
		t.Errorf("expected initial state 'closed', got '%s'", cb.State)
	}
}

func TestCircuitBreakerClosed(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, 2, 30*time.Second)

	// Should allow requests when closed
	if !cb.Allow() {
		t.Error("expected Allow() to return true when closed")
	}

	// Record success should keep it closed
	cb.RecordSuccess()
	if cb.GetState() != "closed" {
		t.Errorf("expected state 'closed' after success, got '%s'", cb.GetState())
	}

	// Record failures but not enough to open
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.GetState() != "closed" {
		t.Errorf("expected state 'closed' after 2 failures, got '%s'", cb.GetState())
	}
}

func TestCircuitBreakerOpens(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, 2, 30*time.Second)

	// Record enough failures to open the circuit
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.GetState() != "open" {
		t.Errorf("expected state 'open' after 3 failures, got '%s'", cb.GetState())
	}

	// Should not allow requests when open
	if cb.Allow() {
		t.Error("expected Allow() to return false when open")
	}
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, 2, 1*time.Millisecond)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for reset timeout
	time.Sleep(5 * time.Millisecond)

	// Should transition to half-open and allow
	if !cb.Allow() {
		t.Error("expected Allow() to return true after reset timeout")
	}

	if cb.GetState() != "half-open" {
		t.Errorf("expected state 'half-open', got '%s'", cb.GetState())
	}
}

func TestCircuitBreakerClosesFromHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, 2, 1*time.Millisecond)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait and transition to half-open
	time.Sleep(5 * time.Millisecond)
	cb.Allow()

	// Record enough successes to close
	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.GetState() != "closed" {
		t.Errorf("expected state 'closed' after successes in half-open, got '%s'", cb.GetState())
	}
}

func TestCircuitBreakerReopensFromHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, 2, 1*time.Millisecond)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait and transition to half-open
	time.Sleep(5 * time.Millisecond)
	cb.Allow()

	// Failure in half-open should reopen
	cb.RecordFailure()

	if cb.GetState() != "open" {
		t.Errorf("expected state 'open' after failure in half-open, got '%s'", cb.GetState())
	}
}

func TestValidateExecTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{
			name:    "valid path in /usr/bin",
			target:  "/usr/bin/check-health",
			wantErr: false,
		},
		{
			name:    "valid path in /usr/local/bin",
			target:  "/usr/local/bin/healthcheck",
			wantErr: false,
		},
		{
			name:    "valid path in /opt/unheaded/bin",
			target:  "/opt/unheaded/bin/check",
			wantErr: false,
		},
		{
			name:    "valid path in /opt/unheaded/healthchecks",
			target:  "/opt/unheaded/healthchecks/mycheck",
			wantErr: false,
		},
		{
			name:    "relative path",
			target:  "check-health",
			wantErr: true,
		},
		{
			name:    "path traversal",
			target:  "/usr/bin/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "not in allowed directory",
			target:  "/tmp/check",
			wantErr: true,
		},
		{
			name:    "shell metacharacters",
			target:  "/usr/bin/check;rm -rf /",
			wantErr: true,
		},
		{
			name:    "spaces not allowed",
			target:  "/usr/bin/check health",
			wantErr: true,
		},
		{
			name:    "backticks not allowed",
			target:  "/usr/bin/`whoami`",
			wantErr: true,
		},
		{
			name:    "dollar sign not allowed",
			target:  "/usr/bin/$USER",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExecTarget(tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateExecTarget(%q) error = %v, wantErr %v", tt.target, err, tt.wantErr)
			}
		})
	}
}

func TestSystemHealth(t *testing.T) {
	health := &SystemHealth{
		Status: StatusHealthy,
		Checks: map[string]*HealthResult{
			"service1": {Name: "service1", Status: StatusHealthy},
			"service2": {Name: "service2", Status: StatusHealthy},
			"service3": {Name: "service3", Status: StatusDegraded},
		},
		HealthyCount:   2,
		DegradedCount:  1,
		UnhealthyCount: 0,
	}

	if health.Status != StatusHealthy {
		t.Errorf("expected status healthy, got '%s'", health.Status)
	}
	if len(health.Checks) != 3 {
		t.Errorf("expected 3 checks, got %d", len(health.Checks))
	}
	if health.HealthyCount != 2 {
		t.Errorf("expected healthy count 2, got %d", health.HealthyCount)
	}
}

func TestErrors(t *testing.T) {
	errors := []error{
		ErrCheckNotFound,
		ErrCheckAlreadyExists,
		ErrInvalidCheckType,
		ErrCheckTimeout,
		ErrCircuitOpen,
		ErrAggregatorStopped,
		ErrInvalidExecTarget,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("error should not be nil")
		}
		if err.Error() == "" {
			t.Error("error message should not be empty")
		}
	}
}
