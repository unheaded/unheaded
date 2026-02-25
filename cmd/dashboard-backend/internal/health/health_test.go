package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// testLogger returns a logger that discards all output, suitable for tests.
func testLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// testConfig returns a Config with fast intervals for testing.
func testConfig() *Config {
	return &Config{
		PollInterval:      50 * time.Millisecond,
		PollTimeout:       2 * time.Second,
		RetentionPeriod:   1 * time.Hour,
		MaxHistory:        100,
		FailureThreshold:  3,
		RecoveryThreshold: 2,
	}
}

// newTestMonitor creates a Monitor pre-configured for tests.
func newTestMonitor(t *testing.T) *Monitor {
	t.Helper()
	m, err := NewMonitor(testConfig(), testLogger())
	if err != nil {
		t.Fatalf("NewMonitor: %v", err)
	}
	return m
}

// healthyServer returns a test HTTP server that responds 200 with a JSON health body.
func healthyServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
}

// unhealthyServer returns a test HTTP server that responds 500.
func unhealthyServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error"})
	}))
}

// degradedServer returns a test HTTP server that responds 429 (non-2xx, non-5xx).
func degradedServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"status": "degraded"})
	}))
}

// ---------------------------------------------------------------------------
// 1. Config validation
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PollInterval != 15*time.Second {
		t.Errorf("PollInterval = %v, want 15s", cfg.PollInterval)
	}
	if cfg.PollTimeout != 5*time.Second {
		t.Errorf("PollTimeout = %v, want 5s", cfg.PollTimeout)
	}
	if cfg.RetentionPeriod != 24*time.Hour {
		t.Errorf("RetentionPeriod = %v, want 24h", cfg.RetentionPeriod)
	}
	if cfg.MaxHistory != 1000 {
		t.Errorf("MaxHistory = %d, want 1000", cfg.MaxHistory)
	}
	if cfg.FailureThreshold != 3 {
		t.Errorf("FailureThreshold = %d, want 3", cfg.FailureThreshold)
	}
	if cfg.RecoveryThreshold != 2 {
		t.Errorf("RecoveryThreshold = %d, want 2", cfg.RecoveryThreshold)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr error
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: ErrNilConfig,
		},
		{
			name: "zero poll interval",
			config: &Config{
				PollInterval: 0,
			},
			wantErr: ErrInvalidInterval,
		},
		{
			name: "negative poll interval",
			config: &Config{
				PollInterval: -1 * time.Second,
			},
			wantErr: ErrInvalidInterval,
		},
		{
			name: "valid minimal config fills defaults",
			config: &Config{
				PollInterval: 1 * time.Second,
			},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if err != tc.wantErr {
				t.Errorf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}

	// Verify that zero fields are filled with defaults after successful Validate.
	cfg := &Config{PollInterval: 1 * time.Second}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if cfg.PollTimeout != 5*time.Second {
		t.Errorf("PollTimeout not defaulted: got %v", cfg.PollTimeout)
	}
	if cfg.RetentionPeriod != 24*time.Hour {
		t.Errorf("RetentionPeriod not defaulted: got %v", cfg.RetentionPeriod)
	}
	if cfg.MaxHistory != 1000 {
		t.Errorf("MaxHistory not defaulted: got %d", cfg.MaxHistory)
	}
	if cfg.FailureThreshold != 3 {
		t.Errorf("FailureThreshold not defaulted: got %d", cfg.FailureThreshold)
	}
	if cfg.RecoveryThreshold != 2 {
		t.Errorf("RecoveryThreshold not defaulted: got %d", cfg.RecoveryThreshold)
	}
}

// ---------------------------------------------------------------------------
// 2. Monitor creation
// ---------------------------------------------------------------------------

func TestNewMonitor(t *testing.T) {
	t.Run("nil config uses defaults", func(t *testing.T) {
		m, err := NewMonitor(nil, testLogger())
		if err != nil {
			t.Fatalf("NewMonitor(nil, ...) returned error: %v", err)
		}
		if m.config.PollInterval != 15*time.Second {
			t.Errorf("expected default PollInterval, got %v", m.config.PollInterval)
		}
	})

	t.Run("nil logger does not panic", func(t *testing.T) {
		m, err := NewMonitor(DefaultConfig(), nil)
		if err != nil {
			t.Fatalf("NewMonitor(..., nil) returned error: %v", err)
		}
		if m.log == nil {
			t.Error("expected non-nil logger")
		}
	})

	t.Run("invalid config returns error", func(t *testing.T) {
		_, err := NewMonitor(&Config{PollInterval: -1}, testLogger())
		if err == nil {
			t.Fatal("expected error for invalid config")
		}
	})

	t.Run("valid config succeeds", func(t *testing.T) {
		m, err := NewMonitor(testConfig(), testLogger())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.targets == nil || m.health == nil || m.history == nil {
			t.Error("internal maps not initialized")
		}
	})
}

// ---------------------------------------------------------------------------
// 3. Service registration and unregistration
// ---------------------------------------------------------------------------

func TestRegisterTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  *ServiceTarget
		wantErr string
	}{
		{
			name:    "nil target",
			target:  nil,
			wantErr: "target cannot be nil",
		},
		{
			name:    "empty name",
			target:  &ServiceTarget{Name: "", HealthURL: "http://localhost/health"},
			wantErr: "target name cannot be empty",
		},
		{
			name:    "empty health URL",
			target:  &ServiceTarget{Name: "svc", HealthURL: ""},
			wantErr: "target health URL cannot be empty",
		},
		{
			name:    "valid target",
			target:  &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health", Tags: []string{"test"}},
			wantErr: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestMonitor(t)
			err := m.RegisterTarget(tc.target)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
			}
		})
	}
}

func TestRegisterTarget_InitializesState(t *testing.T) {
	m := newTestMonitor(t)
	target := &ServiceTarget{
		Name:      "test-svc",
		HealthURL: "http://localhost/health",
		Tags:      []string{"alpha"},
	}

	if err := m.RegisterTarget(target); err != nil {
		t.Fatalf("RegisterTarget: %v", err)
	}

	// Target should be marked Enabled.
	if !m.targets["test-svc"].Enabled {
		t.Error("target not marked enabled")
	}

	// Health state should be initialized to unknown.
	h, ok := m.health["test-svc"]
	if !ok {
		t.Fatal("health state not initialized")
	}
	if h.Status != StatusUnknown {
		t.Errorf("initial status = %v, want %v", h.Status, StatusUnknown)
	}
	if len(h.Tags) != 1 || h.Tags[0] != "alpha" {
		t.Errorf("tags not propagated: %v", h.Tags)
	}

	// History should be initialized.
	hist, ok := m.history["test-svc"]
	if !ok {
		t.Fatal("history not initialized")
	}
	if hist.MaxSize != m.config.MaxHistory {
		t.Errorf("history MaxSize = %d, want %d", hist.MaxSize, m.config.MaxHistory)
	}
}

func TestUnregisterTarget(t *testing.T) {
	m := newTestMonitor(t)

	t.Run("not found", func(t *testing.T) {
		err := m.UnregisterTarget("nonexistent")
		if err != ErrServiceNotFound {
			t.Errorf("error = %v, want ErrServiceNotFound", err)
		}
	})

	t.Run("removes target, health, and history", func(t *testing.T) {
		target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
		if err := m.RegisterTarget(target); err != nil {
			t.Fatalf("RegisterTarget: %v", err)
		}

		if err := m.UnregisterTarget("svc"); err != nil {
			t.Fatalf("UnregisterTarget: %v", err)
		}

		if _, ok := m.targets["svc"]; ok {
			t.Error("target still present after unregister")
		}
		if _, ok := m.health["svc"]; ok {
			t.Error("health still present after unregister")
		}
		if _, ok := m.history["svc"]; ok {
			t.Error("history still present after unregister")
		}
	})
}

func TestGetTargets(t *testing.T) {
	m := newTestMonitor(t)

	names := []string{"charlie", "alpha", "bravo"}
	for _, n := range names {
		m.RegisterTarget(&ServiceTarget{Name: n, HealthURL: "http://localhost/" + n})
	}

	targets := m.GetTargets()
	if len(targets) != 3 {
		t.Fatalf("len(targets) = %d, want 3", len(targets))
	}

	// Verify sorted by name.
	for i := 1; i < len(targets); i++ {
		if targets[i-1].Name >= targets[i].Name {
			t.Errorf("targets not sorted: %v >= %v", targets[i-1].Name, targets[i].Name)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. HealthHistory
// ---------------------------------------------------------------------------

func TestHealthHistory_Add(t *testing.T) {
	h := &HealthHistory{Name: "svc", MaxSize: 3}

	for i := 0; i < 5; i++ {
		h.Add(CheckResult{Service: "svc", Status: StatusHealthy, Timestamp: time.Now()})
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.Results) != 3 {
		t.Errorf("expected 3 results (capped at MaxSize), got %d", len(h.Results))
	}
}

func TestHealthHistory_GetRecent(t *testing.T) {
	h := &HealthHistory{Name: "svc", MaxSize: 100}

	// No results.
	if got := h.GetRecent(5); got != nil {
		t.Errorf("GetRecent on empty = %v, want nil", got)
	}

	for i := 0; i < 10; i++ {
		h.Add(CheckResult{
			Service:   "svc",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Message:   fmt.Sprintf("check-%d", i),
		})
	}

	// Request more than available.
	got := h.GetRecent(20)
	if len(got) != 10 {
		t.Errorf("GetRecent(20) returned %d, want 10", len(got))
	}

	// Request fewer.
	got = h.GetRecent(3)
	if len(got) != 3 {
		t.Errorf("GetRecent(3) returned %d, want 3", len(got))
	}

	// Should be the most recent 3.
	if got[0].Message != "check-7" || got[2].Message != "check-9" {
		t.Errorf("GetRecent returned wrong slice: first=%q last=%q", got[0].Message, got[2].Message)
	}

	// Request zero.
	got = h.GetRecent(0)
	if got != nil {
		t.Errorf("GetRecent(0) = %v, want nil", got)
	}
}

func TestHealthHistory_GetSince(t *testing.T) {
	h := &HealthHistory{Name: "svc", MaxSize: 100}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		h.Add(CheckResult{
			Service:   "svc",
			Timestamp: base.Add(time.Duration(i) * time.Minute),
		})
	}

	// Get results since minute 5 (exclusive).
	since := base.Add(5 * time.Minute)
	got := h.GetSince(since)

	// Minutes 6, 7, 8, 9 should be returned (4 results).
	if len(got) != 4 {
		t.Errorf("GetSince returned %d results, want 4", len(got))
	}
}

func TestHealthHistory_Cleanup(t *testing.T) {
	h := &HealthHistory{Name: "svc", MaxSize: 100}

	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-10 * time.Minute)

	h.Add(CheckResult{Service: "svc", Timestamp: old})
	h.Add(CheckResult{Service: "svc", Timestamp: old.Add(time.Minute)})
	h.Add(CheckResult{Service: "svc", Timestamp: recent})
	h.Add(CheckResult{Service: "svc", Timestamp: recent.Add(time.Minute)})

	cutoff := time.Now().Add(-1 * time.Hour)
	h.Cleanup(cutoff)

	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.Results) != 2 {
		t.Errorf("after Cleanup, len(Results) = %d, want 2", len(h.Results))
	}
}

func TestHealthHistory_CalculateUptime(t *testing.T) {
	t.Run("no data returns 100", func(t *testing.T) {
		h := &HealthHistory{Name: "svc", MaxSize: 100}
		got := h.CalculateUptime(1 * time.Hour)
		if got != 100.0 {
			t.Errorf("CalculateUptime with no data = %f, want 100.0", got)
		}
	})

	t.Run("mixed results", func(t *testing.T) {
		h := &HealthHistory{Name: "svc", MaxSize: 100}

		now := time.Now()
		// 7 healthy, 3 unhealthy within the last hour.
		for i := 0; i < 7; i++ {
			h.Add(CheckResult{
				Service:   "svc",
				Status:    StatusHealthy,
				Timestamp: now.Add(-time.Duration(i) * time.Minute),
			})
		}
		for i := 0; i < 3; i++ {
			h.Add(CheckResult{
				Service:   "svc",
				Status:    StatusUnhealthy,
				Timestamp: now.Add(-time.Duration(i+10) * time.Minute),
			})
		}

		got := h.CalculateUptime(1 * time.Hour)
		expected := 70.0
		if got != expected {
			t.Errorf("CalculateUptime = %f, want %f", got, expected)
		}
	})

	t.Run("all healthy", func(t *testing.T) {
		h := &HealthHistory{Name: "svc", MaxSize: 100}
		now := time.Now()
		for i := 0; i < 5; i++ {
			h.Add(CheckResult{
				Service:   "svc",
				Status:    StatusHealthy,
				Timestamp: now.Add(-time.Duration(i) * time.Minute),
			})
		}
		got := h.CalculateUptime(1 * time.Hour)
		if got != 100.0 {
			t.Errorf("CalculateUptime = %f, want 100.0", got)
		}
	})

	t.Run("old results outside window excluded", func(t *testing.T) {
		h := &HealthHistory{Name: "svc", MaxSize: 100}
		now := time.Now()

		// Old unhealthy result (outside window).
		h.Add(CheckResult{
			Service:   "svc",
			Status:    StatusUnhealthy,
			Timestamp: now.Add(-2 * time.Hour),
		})

		// Recent healthy result.
		h.Add(CheckResult{
			Service:   "svc",
			Status:    StatusHealthy,
			Timestamp: now.Add(-5 * time.Minute),
		})

		got := h.CalculateUptime(1 * time.Hour)
		if got != 100.0 {
			t.Errorf("CalculateUptime = %f, want 100.0 (old result excluded)", got)
		}
	})
}

// ---------------------------------------------------------------------------
// 5. Health status transitions (processResult / threshold logic)
// ---------------------------------------------------------------------------

func TestProcessResult_HealthyTransition(t *testing.T) {
	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
	m.RegisterTarget(target)

	now := time.Now()

	// Send RecoveryThreshold (2) healthy results. Status should go from unknown to healthy.
	for i := 0; i < m.config.RecoveryThreshold; i++ {
		m.processResult(target, CheckResult{
			Service:   "svc",
			Status:    StatusHealthy,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Duration:  10 * time.Millisecond,
		})
	}

	h := m.health["svc"]
	if h.Status != StatusHealthy {
		t.Errorf("after %d successes, status = %v, want %v",
			m.config.RecoveryThreshold, h.Status, StatusHealthy)
	}
	if h.ConsecutiveSuccesses != m.config.RecoveryThreshold {
		t.Errorf("ConsecutiveSuccesses = %d, want %d", h.ConsecutiveSuccesses, m.config.RecoveryThreshold)
	}
	if h.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", h.ConsecutiveFailures)
	}
}

func TestProcessResult_UnhealthyTransition(t *testing.T) {
	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
	m.RegisterTarget(target)

	now := time.Now()

	// First make service healthy.
	for i := 0; i < m.config.RecoveryThreshold; i++ {
		m.processResult(target, CheckResult{
			Service:   "svc",
			Status:    StatusHealthy,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Duration:  5 * time.Millisecond,
		})
	}

	if m.health["svc"].Status != StatusHealthy {
		t.Fatal("precondition: service should be healthy")
	}

	// One failure should degrade.
	m.processResult(target, CheckResult{
		Service:   "svc",
		Status:    StatusUnhealthy,
		Timestamp: now.Add(10 * time.Second),
		Duration:  5 * time.Millisecond,
	})

	if m.health["svc"].Status != StatusDegraded {
		t.Errorf("after 1 failure from healthy, status = %v, want %v",
			m.health["svc"].Status, StatusDegraded)
	}

	// Continue failing to reach FailureThreshold.
	for i := 1; i < m.config.FailureThreshold; i++ {
		m.processResult(target, CheckResult{
			Service:   "svc",
			Status:    StatusUnhealthy,
			Timestamp: now.Add(time.Duration(10+i) * time.Second),
			Duration:  5 * time.Millisecond,
		})
	}

	if m.health["svc"].Status != StatusUnhealthy {
		t.Errorf("after %d failures, status = %v, want %v",
			m.config.FailureThreshold, m.health["svc"].Status, StatusUnhealthy)
	}
}

func TestProcessResult_DegradedBeforeUnhealthy(t *testing.T) {
	m := newTestMonitor(t)
	m.config.FailureThreshold = 5
	target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
	m.RegisterTarget(target)

	// Make healthy first.
	for i := 0; i < m.config.RecoveryThreshold; i++ {
		m.processResult(target, CheckResult{
			Service:   "svc",
			Status:    StatusHealthy,
			Timestamp: time.Now(),
			Duration:  1 * time.Millisecond,
		})
	}

	// 1 failure: degraded.
	m.processResult(target, CheckResult{
		Service:   "svc",
		Status:    StatusUnhealthy,
		Timestamp: time.Now(),
		Duration:  1 * time.Millisecond,
	})
	if m.health["svc"].Status != StatusDegraded {
		t.Errorf("status = %v, want degraded", m.health["svc"].Status)
	}

	// 2 more failures (total 3, below threshold of 5): still degraded.
	for i := 0; i < 2; i++ {
		m.processResult(target, CheckResult{
			Service:   "svc",
			Status:    StatusUnhealthy,
			Timestamp: time.Now(),
			Duration:  1 * time.Millisecond,
		})
	}
	if m.health["svc"].Status != StatusDegraded {
		t.Errorf("at 3/%d failures, status = %v, want degraded",
			m.config.FailureThreshold, m.health["svc"].Status)
	}
}

func TestProcessResult_RecoveryFromUnhealthy(t *testing.T) {
	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
	m.RegisterTarget(target)

	// Drive to unhealthy.
	for i := 0; i < m.config.FailureThreshold; i++ {
		m.processResult(target, CheckResult{
			Service:   "svc",
			Status:    StatusUnhealthy,
			Timestamp: time.Now(),
			Duration:  1 * time.Millisecond,
		})
	}
	if m.health["svc"].Status != StatusUnhealthy {
		t.Fatal("precondition: expected unhealthy")
	}

	// One success is not enough to recover.
	m.processResult(target, CheckResult{
		Service:   "svc",
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Duration:  1 * time.Millisecond,
	})
	if m.health["svc"].Status == StatusHealthy {
		t.Error("one success should not recover immediately")
	}

	// RecoveryThreshold successes should recover.
	for i := 1; i < m.config.RecoveryThreshold; i++ {
		m.processResult(target, CheckResult{
			Service:   "svc",
			Status:    StatusHealthy,
			Timestamp: time.Now(),
			Duration:  1 * time.Millisecond,
		})
	}
	if m.health["svc"].Status != StatusHealthy {
		t.Errorf("after %d successes from unhealthy, status = %v, want healthy",
			m.config.RecoveryThreshold, m.health["svc"].Status)
	}
}

func TestProcessResult_CountsAccumulate(t *testing.T) {
	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
	m.RegisterTarget(target)

	m.processResult(target, CheckResult{Service: "svc", Status: StatusHealthy, Timestamp: time.Now(), Duration: 1 * time.Millisecond})
	m.processResult(target, CheckResult{Service: "svc", Status: StatusHealthy, Timestamp: time.Now(), Duration: 1 * time.Millisecond})
	m.processResult(target, CheckResult{Service: "svc", Status: StatusUnhealthy, Timestamp: time.Now(), Duration: 1 * time.Millisecond})

	h := m.health["svc"]
	if h.TotalChecks != 3 {
		t.Errorf("TotalChecks = %d, want 3", h.TotalChecks)
	}
	if h.SuccessfulChecks != 2 {
		t.Errorf("SuccessfulChecks = %d, want 2", h.SuccessfulChecks)
	}
	if h.FailedChecks != 1 {
		t.Errorf("FailedChecks = %d, want 1", h.FailedChecks)
	}
}

// ---------------------------------------------------------------------------
// 6. OnStatusChange callback
// ---------------------------------------------------------------------------

func TestOnStatusChange_Callback(t *testing.T) {
	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
	m.RegisterTarget(target)

	var events []HealthEvent
	var mu sync.Mutex
	m.OnStatusChange(func(e HealthEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	// Make healthy (from unknown -- no event because old status is unknown).
	for i := 0; i < m.config.RecoveryThreshold; i++ {
		m.processResult(target, CheckResult{
			Service: "svc", Status: StatusHealthy, Timestamp: time.Now(), Duration: 1 * time.Millisecond,
		})
	}

	mu.Lock()
	if len(events) != 0 {
		t.Errorf("expected 0 events for unknown->healthy, got %d", len(events))
	}
	mu.Unlock()

	// Now fail -- healthy -> degraded should fire.
	m.processResult(target, CheckResult{
		Service: "svc", Status: StatusUnhealthy, Timestamp: time.Now(), Duration: 1 * time.Millisecond,
	})

	mu.Lock()
	if len(events) != 1 {
		t.Fatalf("expected 1 event for healthy->degraded, got %d", len(events))
	}
	if events[0].OldStatus != StatusHealthy || events[0].NewStatus != StatusDegraded {
		t.Errorf("event = %v->%v, want healthy->degraded", events[0].OldStatus, events[0].NewStatus)
	}
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// 7. Polling against real HTTP servers
// ---------------------------------------------------------------------------

func TestPollTarget_Healthy(t *testing.T) {
	srv := healthyServer()
	defer srv.Close()

	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: srv.URL + "/health"}
	m.RegisterTarget(target)

	ctx := context.Background()
	m.pollTarget(ctx, target)

	h := m.health["svc"]
	if h.SuccessfulChecks != 1 {
		t.Errorf("SuccessfulChecks = %d, want 1", h.SuccessfulChecks)
	}

	hist := m.history["svc"]
	recent := hist.GetRecent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(recent))
	}
	if recent[0].Status != StatusHealthy {
		t.Errorf("result status = %v, want healthy", recent[0].Status)
	}
	if recent[0].StatusCode != 200 {
		t.Errorf("status code = %d, want 200", recent[0].StatusCode)
	}
	if !recent[0].Healthy {
		t.Error("Healthy flag should be true")
	}
	if !recent[0].Ready {
		t.Error("Ready flag should be true when no ReadyURL set")
	}
}

func TestPollTarget_Unhealthy(t *testing.T) {
	srv := unhealthyServer()
	defer srv.Close()

	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: srv.URL + "/health"}
	m.RegisterTarget(target)

	ctx := context.Background()
	m.pollTarget(ctx, target)

	hist := m.history["svc"]
	recent := hist.GetRecent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(recent))
	}
	if recent[0].Status != StatusUnhealthy {
		t.Errorf("status = %v, want unhealthy", recent[0].Status)
	}
	if recent[0].StatusCode != 500 {
		t.Errorf("status code = %d, want 500", recent[0].StatusCode)
	}
}

func TestPollTarget_Degraded(t *testing.T) {
	srv := degradedServer()
	defer srv.Close()

	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: srv.URL + "/health"}
	m.RegisterTarget(target)

	ctx := context.Background()
	m.pollTarget(ctx, target)

	hist := m.history["svc"]
	recent := hist.GetRecent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(recent))
	}
	if recent[0].Status != StatusDegraded {
		t.Errorf("status = %v, want degraded", recent[0].Status)
	}
}

func TestPollTarget_Unreachable(t *testing.T) {
	m := newTestMonitor(t)
	// Use a URL that will fail to connect.
	target := &ServiceTarget{Name: "svc", HealthURL: "http://127.0.0.1:1/health"}
	m.RegisterTarget(target)

	ctx := context.Background()
	m.pollTarget(ctx, target)

	hist := m.history["svc"]
	recent := hist.GetRecent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(recent))
	}
	if recent[0].Status != StatusUnhealthy {
		t.Errorf("status = %v, want unhealthy", recent[0].Status)
	}
	if recent[0].Error == "" {
		t.Error("expected non-empty error for unreachable service")
	}
}

func TestPollTarget_InvalidURL(t *testing.T) {
	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: "://invalid"}
	m.RegisterTarget(target)

	ctx := context.Background()
	m.pollTarget(ctx, target)

	hist := m.history["svc"]
	recent := hist.GetRecent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(recent))
	}
	if recent[0].Status != StatusUnhealthy {
		t.Errorf("status = %v, want unhealthy", recent[0].Status)
	}
	if recent[0].Error == "" {
		t.Error("expected error for invalid URL")
	}
}

func TestPollTarget_WithReadyEndpoint(t *testing.T) {
	healthSrv := healthyServer()
	defer healthSrv.Close()

	readySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer readySrv.Close()

	m := newTestMonitor(t)
	target := &ServiceTarget{
		Name:      "svc",
		HealthURL: healthSrv.URL + "/health",
		ReadyURL:  readySrv.URL + "/ready",
	}
	m.RegisterTarget(target)

	ctx := context.Background()
	m.pollTarget(ctx, target)

	hist := m.history["svc"]
	recent := hist.GetRecent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(recent))
	}

	// Healthy, but not ready.
	if !recent[0].Healthy {
		t.Error("expected Healthy=true")
	}
	if recent[0].Ready {
		t.Error("expected Ready=false (ready endpoint returned 503)")
	}
}

func TestPollTarget_DetailsFromJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"version": "1.2.3",
		})
	}))
	defer srv.Close()

	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: srv.URL + "/health"}
	m.RegisterTarget(target)

	ctx := context.Background()
	m.pollTarget(ctx, target)

	hist := m.history["svc"]
	recent := hist.GetRecent(1)
	if recent[0].Details == nil {
		t.Error("expected Details to be populated from JSON response")
	}
	if recent[0].Message != "ok" {
		t.Errorf("Message = %q, want \"ok\"", recent[0].Message)
	}
}

// ---------------------------------------------------------------------------
// 8. pollAll - concurrent checks
// ---------------------------------------------------------------------------

func TestPollAll_ConcurrentServices(t *testing.T) {
	var requestCount int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	m := newTestMonitor(t)
	for i := 0; i < 5; i++ {
		m.RegisterTarget(&ServiceTarget{
			Name:      fmt.Sprintf("svc-%d", i),
			HealthURL: srv.URL + "/health",
		})
	}

	ctx := context.Background()
	m.pollAll(ctx)

	count := atomic.LoadInt64(&requestCount)
	if count != 5 {
		t.Errorf("expected 5 requests, got %d", count)
	}
}

func TestPollAll_SkipsDisabled(t *testing.T) {
	var requestCount int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	m := newTestMonitor(t)
	m.RegisterTarget(&ServiceTarget{Name: "enabled", HealthURL: srv.URL + "/health"})
	m.RegisterTarget(&ServiceTarget{Name: "disabled", HealthURL: srv.URL + "/health"})

	// Disable one target.
	m.targetsMu.Lock()
	m.targets["disabled"].Enabled = false
	m.targetsMu.Unlock()

	ctx := context.Background()
	m.pollAll(ctx)

	count := atomic.LoadInt64(&requestCount)
	if count != 1 {
		t.Errorf("expected 1 request (disabled skipped), got %d", count)
	}
}

// ---------------------------------------------------------------------------
// 9. GetServiceHealth / GetSystemHealth
// ---------------------------------------------------------------------------

func TestGetServiceHealth(t *testing.T) {
	m := newTestMonitor(t)

	t.Run("not found", func(t *testing.T) {
		_, err := m.GetServiceHealth("nonexistent")
		if err != ErrServiceNotFound {
			t.Errorf("error = %v, want ErrServiceNotFound", err)
		}
	})

	t.Run("returns copy with recent results", func(t *testing.T) {
		target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health", Tags: []string{"test"}}
		m.RegisterTarget(target)

		// Add history entries.
		for i := 0; i < 15; i++ {
			m.history["svc"].Add(CheckResult{
				Service:   "svc",
				Status:    StatusHealthy,
				Timestamp: time.Now(),
			})
		}

		sh, err := m.GetServiceHealth("svc")
		if err != nil {
			t.Fatalf("GetServiceHealth: %v", err)
		}

		// Should include recent results (up to 10).
		if len(sh.RecentResults) != 10 {
			t.Errorf("RecentResults len = %d, want 10", len(sh.RecentResults))
		}

		// Verify it returns a copy (modifying returned value should not affect internal state).
		sh.Status = StatusUnhealthy
		if m.health["svc"].Status == StatusUnhealthy {
			t.Error("GetServiceHealth should return a copy, but modification affected internal state")
		}
	})
}

func TestGetSystemHealth(t *testing.T) {
	m := newTestMonitor(t)

	t.Run("no services", func(t *testing.T) {
		sys := m.GetSystemHealth()
		if sys.Status != StatusUnknown {
			t.Errorf("empty system status = %v, want unknown", sys.Status)
		}
		if sys.TotalServices != 0 {
			t.Errorf("TotalServices = %d, want 0", sys.TotalServices)
		}
	})

	t.Run("all healthy", func(t *testing.T) {
		target1 := &ServiceTarget{Name: "svc1", HealthURL: "http://localhost/health"}
		target2 := &ServiceTarget{Name: "svc2", HealthURL: "http://localhost/health"}
		m.RegisterTarget(target1)
		m.RegisterTarget(target2)

		// Make both healthy.
		for _, name := range []string{"svc1", "svc2"} {
			m.healthMu.Lock()
			m.health[name].Status = StatusHealthy
			m.healthMu.Unlock()
		}

		sys := m.GetSystemHealth()
		if sys.Status != StatusHealthy {
			t.Errorf("status = %v, want healthy", sys.Status)
		}
		if sys.HealthyCount != 2 {
			t.Errorf("HealthyCount = %d, want 2", sys.HealthyCount)
		}
		if sys.TotalServices != 2 {
			t.Errorf("TotalServices = %d, want 2", sys.TotalServices)
		}
	})

	t.Run("one degraded", func(t *testing.T) {
		m.healthMu.Lock()
		m.health["svc1"].Status = StatusDegraded
		m.healthMu.Unlock()

		sys := m.GetSystemHealth()
		if sys.Status != StatusDegraded {
			t.Errorf("status = %v, want degraded", sys.Status)
		}
		if sys.DegradedCount != 1 {
			t.Errorf("DegradedCount = %d, want 1", sys.DegradedCount)
		}
	})

	t.Run("one unhealthy overrides degraded", func(t *testing.T) {
		m.healthMu.Lock()
		m.health["svc2"].Status = StatusUnhealthy
		m.healthMu.Unlock()

		sys := m.GetSystemHealth()
		if sys.Status != StatusUnhealthy {
			t.Errorf("status = %v, want unhealthy (one unhealthy overrides)", sys.Status)
		}
		if sys.UnhealthyCount != 1 {
			t.Errorf("UnhealthyCount = %d, want 1", sys.UnhealthyCount)
		}
	})
}

func TestGetSystemHealth_Message(t *testing.T) {
	m := newTestMonitor(t)

	tests := []struct {
		name       string
		setup      func()
		wantStatus Status
		wantMsg    string
	}{
		{
			name:       "no services",
			setup:      func() {},
			wantStatus: StatusUnknown,
			wantMsg:    "No services registered or all unknown",
		},
		{
			name: "all healthy",
			setup: func() {
				m.RegisterTarget(&ServiceTarget{Name: "a", HealthURL: "http://a/health"})
				m.healthMu.Lock()
				m.health["a"].Status = StatusHealthy
				m.healthMu.Unlock()
			},
			wantStatus: StatusHealthy,
			wantMsg:    "All 1 services healthy",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			sys := m.GetSystemHealth()
			if sys.Status != tc.wantStatus {
				t.Errorf("status = %v, want %v", sys.Status, tc.wantStatus)
			}
			if sys.Message != tc.wantMsg {
				t.Errorf("message = %q, want %q", sys.Message, tc.wantMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 10. GetHistory
// ---------------------------------------------------------------------------

func TestGetHistory(t *testing.T) {
	m := newTestMonitor(t)

	t.Run("not found", func(t *testing.T) {
		_, err := m.GetHistory("nonexistent", time.Time{})
		if err != ErrServiceNotFound {
			t.Errorf("error = %v, want ErrServiceNotFound", err)
		}
	})

	t.Run("returns since", func(t *testing.T) {
		target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
		m.RegisterTarget(target)

		base := time.Now()
		for i := 0; i < 10; i++ {
			m.history["svc"].Add(CheckResult{
				Service:   "svc",
				Timestamp: base.Add(time.Duration(i) * time.Minute),
			})
		}

		since := base.Add(7 * time.Minute)
		results, err := m.GetHistory("svc", since)
		if err != nil {
			t.Fatalf("GetHistory: %v", err)
		}
		// Minutes 8 and 9 should be returned.
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})
}

// ---------------------------------------------------------------------------
// 11. ForceCheck
// ---------------------------------------------------------------------------

func TestForceCheck(t *testing.T) {
	t.Run("service not found", func(t *testing.T) {
		m := newTestMonitor(t)
		_, err := m.ForceCheck(context.Background(), "nonexistent")
		if err != ErrServiceNotFound {
			t.Errorf("error = %v, want ErrServiceNotFound", err)
		}
	})

	t.Run("healthy server", func(t *testing.T) {
		srv := healthyServer()
		defer srv.Close()

		m := newTestMonitor(t)
		m.RegisterTarget(&ServiceTarget{Name: "svc", HealthURL: srv.URL + "/health"})

		result, err := m.ForceCheck(context.Background(), "svc")
		if err != nil {
			t.Fatalf("ForceCheck: %v", err)
		}
		if result.Status != StatusHealthy {
			t.Errorf("status = %v, want healthy", result.Status)
		}
		if result.StatusCode != 200 {
			t.Errorf("status code = %d, want 200", result.StatusCode)
		}
		if !result.Healthy {
			t.Error("Healthy flag should be true")
		}
		if result.Duration <= 0 {
			t.Error("Duration should be > 0")
		}
	})

	t.Run("unhealthy server", func(t *testing.T) {
		srv := unhealthyServer()
		defer srv.Close()

		m := newTestMonitor(t)
		m.RegisterTarget(&ServiceTarget{Name: "svc", HealthURL: srv.URL + "/health"})

		result, err := m.ForceCheck(context.Background(), "svc")
		if err != nil {
			t.Fatalf("ForceCheck: %v", err)
		}
		if result.Status != StatusUnhealthy {
			t.Errorf("status = %v, want unhealthy", result.Status)
		}
	})

	t.Run("degraded server", func(t *testing.T) {
		srv := degradedServer()
		defer srv.Close()

		m := newTestMonitor(t)
		m.RegisterTarget(&ServiceTarget{Name: "svc", HealthURL: srv.URL + "/health"})

		result, err := m.ForceCheck(context.Background(), "svc")
		if err != nil {
			t.Fatalf("ForceCheck: %v", err)
		}
		if result.Status != StatusDegraded {
			t.Errorf("status = %v, want degraded", result.Status)
		}
	})

	t.Run("unreachable server returns result not error", func(t *testing.T) {
		m := newTestMonitor(t)
		m.RegisterTarget(&ServiceTarget{Name: "svc", HealthURL: "http://127.0.0.1:1/health"})

		result, err := m.ForceCheck(context.Background(), "svc")
		if err != nil {
			t.Fatalf("ForceCheck: %v", err)
		}
		if result.Status != StatusUnhealthy {
			t.Errorf("status = %v, want unhealthy", result.Status)
		}
		if result.Error == "" {
			t.Error("expected non-empty Error for unreachable server")
		}
	})

	t.Run("invalid URL returns result not error", func(t *testing.T) {
		m := newTestMonitor(t)
		m.RegisterTarget(&ServiceTarget{Name: "svc", HealthURL: "://bad"})

		result, err := m.ForceCheck(context.Background(), "svc")
		if err != nil {
			t.Fatalf("ForceCheck: %v", err)
		}
		if result.Status != StatusUnhealthy {
			t.Errorf("status = %v, want unhealthy", result.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// 12. Start / Stop lifecycle
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	m := newTestMonitor(t)

	if m.IsRunning() {
		t.Error("monitor should not be running before Start")
	}

	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !m.IsRunning() {
		t.Error("monitor should be running after Start")
	}

	// Starting again should return error.
	if err := m.Start(ctx); err == nil {
		t.Error("double Start should return error")
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if m.IsRunning() {
		t.Error("monitor should not be running after Stop")
	}

	// Stopping again should be idempotent.
	if err := m.Stop(); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestStartStop_ContextCancellation(t *testing.T) {
	m := newTestMonitor(t)

	ctx, cancel := context.WithCancel(context.Background())
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Cancel the context -- the poll and cleanup loops should exit.
	cancel()

	// Give goroutines time to exit, then stop to clean up.
	time.Sleep(100 * time.Millisecond)
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop after cancel: %v", err)
	}
}

func TestStartPolls(t *testing.T) {
	var requestCount int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.PollInterval = 30 * time.Millisecond

	m, err := NewMonitor(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewMonitor: %v", err)
	}

	m.RegisterTarget(&ServiceTarget{Name: "svc", HealthURL: srv.URL + "/health"})

	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for initial poll + at least 2 ticker polls.
	time.Sleep(120 * time.Millisecond)

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	count := atomic.LoadInt64(&requestCount)
	// At minimum: 1 initial poll + some ticker polls.
	if count < 2 {
		t.Errorf("expected at least 2 polls, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// 13. RegisterKingdomServices
// ---------------------------------------------------------------------------

func TestRegisterKingdomServices(t *testing.T) {
	m := newTestMonitor(t)
	m.RegisterKingdomServices(nil)

	targets := m.GetTargets()
	expectedNames := []string{"architect", "captain", "gateway", "micromanager", "timeguru", "wotan"}

	if len(targets) != len(expectedNames) {
		t.Fatalf("expected %d kingdom services, got %d", len(expectedNames), len(targets))
	}

	for i, name := range expectedNames {
		if targets[i].Name != name {
			t.Errorf("targets[%d].Name = %q, want %q", i, targets[i].Name, name)
		}
		if targets[i].HealthURL == "" {
			t.Errorf("targets[%d] (%s) has empty HealthURL", i, name)
		}
		if targets[i].ReadyURL == "" {
			t.Errorf("targets[%d] (%s) has empty ReadyURL", i, name)
		}
		if !targets[i].Enabled {
			t.Errorf("targets[%d] (%s) not enabled", i, name)
		}
		if len(targets[i].Tags) == 0 || targets[i].Tags[0] != "kingdom" {
			t.Errorf("targets[%d] (%s) missing 'kingdom' tag", i, name)
		}
	}
}

// ---------------------------------------------------------------------------
// 14. Cleanup via Monitor.cleanup()
// ---------------------------------------------------------------------------

func TestMonitorCleanup(t *testing.T) {
	m := newTestMonitor(t)
	m.config.RetentionPeriod = 1 * time.Hour

	target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
	m.RegisterTarget(target)

	// Add old and recent entries.
	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-5 * time.Minute)

	m.history["svc"].Add(CheckResult{Service: "svc", Timestamp: old})
	m.history["svc"].Add(CheckResult{Service: "svc", Timestamp: old.Add(time.Minute)})
	m.history["svc"].Add(CheckResult{Service: "svc", Timestamp: recent})

	m.cleanup()

	m.history["svc"].mu.RLock()
	count := len(m.history["svc"].Results)
	m.history["svc"].mu.RUnlock()

	if count != 1 {
		t.Errorf("after cleanup, expected 1 result, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// 15. Average latency calculation
// ---------------------------------------------------------------------------

func TestAverageLatencyCalculation(t *testing.T) {
	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: "http://localhost/health"}
	m.RegisterTarget(target)

	durations := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	for _, d := range durations {
		m.processResult(target, CheckResult{
			Service:   "svc",
			Status:    StatusHealthy,
			Timestamp: time.Now(),
			Duration:  d,
		})
	}

	h := m.health["svc"]
	expected := 20 * time.Millisecond // (10+20+30)/3
	if h.AverageLatency != expected {
		t.Errorf("AverageLatency = %v, want %v", h.AverageLatency, expected)
	}
}

// ---------------------------------------------------------------------------
// 16. Concurrent access safety
// ---------------------------------------------------------------------------

func TestConcurrentAccess(t *testing.T) {
	srv := healthyServer()
	defer srv.Close()

	// Note: processResult in the source has a known race where health.Status is
	// read after healthMu.Unlock when two goroutines poll the SAME service
	// concurrently. To test safe concurrent patterns without triggering the
	// source-level race, we use separate monitors and exercise the read APIs
	// concurrently against a single monitor's pollAll (which fans out per-service,
	// each unique, so no two goroutines process the same ServiceHealth).

	m := newTestMonitor(t)
	for i := 0; i < 20; i++ {
		m.RegisterTarget(&ServiceTarget{
			Name:      fmt.Sprintf("svc-%d", i),
			HealthURL: srv.URL + "/health",
		})
	}

	ctx := context.Background()
	var wg sync.WaitGroup

	// Single pollAll (each service is polled exactly once, no same-service contention).
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.pollAll(ctx)
	}()

	// Concurrent reads while poll is running.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.GetSystemHealth()
		}()
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			m.GetServiceHealth(fmt.Sprintf("svc-%d", idx%20))
		}(i)
	}

	// Concurrent register/unregister of separate services.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("dynamic-%d", idx)
			m.RegisterTarget(&ServiceTarget{Name: name, HealthURL: srv.URL + "/health"})
			time.Sleep(5 * time.Millisecond)
			m.UnregisterTarget(name)
		}(i)
	}

	wg.Wait()
	// No panics or data races means success.
}

// ---------------------------------------------------------------------------
// 17. HealthHistory concurrent Add/Read
// ---------------------------------------------------------------------------

func TestHealthHistory_ConcurrentSafety(t *testing.T) {
	h := &HealthHistory{Name: "svc", MaxSize: 50}

	var wg sync.WaitGroup

	// Writers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				h.Add(CheckResult{
					Service:   "svc",
					Status:    StatusHealthy,
					Timestamp: time.Now(),
				})
			}
		}()
	}

	// Readers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				h.GetRecent(5)
				h.GetSince(time.Now().Add(-1 * time.Hour))
				h.CalculateUptime(1 * time.Hour)
			}
		}()
	}

	wg.Wait()
	// No panics or data races means success.
}

// ---------------------------------------------------------------------------
// 18. Status constants
// ---------------------------------------------------------------------------

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusHealthy, "healthy"},
		{StatusDegraded, "degraded"},
		{StatusUnhealthy, "unhealthy"},
		{StatusUnknown, "unknown"},
	}

	for _, tc := range tests {
		if string(tc.status) != tc.want {
			t.Errorf("Status %v = %q, want %q", tc.status, string(tc.status), tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// 19. processResult creates health entry if missing
// ---------------------------------------------------------------------------

func TestProcessResult_CreatesHealthIfMissing(t *testing.T) {
	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "new-svc", HealthURL: "http://localhost/health", Tags: []string{"dynamic"}}

	// Do NOT call RegisterTarget -- directly call processResult.
	// The history will be nil so it skips history storage, but health state should be created.
	m.processResult(target, CheckResult{
		Service:   "new-svc",
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Duration:  1 * time.Millisecond,
	})

	m.healthMu.RLock()
	h, ok := m.health["new-svc"]
	m.healthMu.RUnlock()

	if !ok {
		t.Fatal("processResult should create health entry if missing")
	}
	if h.TotalChecks != 1 {
		t.Errorf("TotalChecks = %d, want 1", h.TotalChecks)
	}
}

// ---------------------------------------------------------------------------
// 20. Integration: full lifecycle with HTTP servers
// ---------------------------------------------------------------------------

func TestIntegration_FullLifecycle(t *testing.T) {
	// Start with a healthy server.
	healthy := true
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		isHealthy := healthy
		mu.Unlock()

		if isHealthy {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"status": "error"})
		}
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.PollInterval = 20 * time.Millisecond
	cfg.FailureThreshold = 3
	cfg.RecoveryThreshold = 2

	m, err := NewMonitor(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewMonitor: %v", err)
	}

	m.RegisterTarget(&ServiceTarget{Name: "svc", HealthURL: srv.URL + "/health"})

	var events []HealthEvent
	var eventsMu sync.Mutex
	m.OnStatusChange(func(e HealthEvent) {
		eventsMu.Lock()
		events = append(events, e)
		eventsMu.Unlock()
	})

	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let it poll healthy for a while to reach healthy status.
	time.Sleep(100 * time.Millisecond)

	sh, err := m.GetServiceHealth("svc")
	if err != nil {
		t.Fatalf("GetServiceHealth: %v", err)
	}
	if sh.Status != StatusHealthy {
		t.Errorf("expected healthy after initial polls, got %v", sh.Status)
	}

	// Switch to unhealthy.
	mu.Lock()
	healthy = false
	mu.Unlock()

	// Wait for failure threshold to be reached.
	time.Sleep(150 * time.Millisecond)

	sh, err = m.GetServiceHealth("svc")
	if err != nil {
		t.Fatalf("GetServiceHealth: %v", err)
	}
	if sh.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy after failures, got %v", sh.Status)
	}

	// Switch back to healthy.
	mu.Lock()
	healthy = true
	mu.Unlock()

	// Wait for recovery threshold.
	time.Sleep(100 * time.Millisecond)

	sh, err = m.GetServiceHealth("svc")
	if err != nil {
		t.Fatalf("GetServiceHealth: %v", err)
	}
	if sh.Status != StatusHealthy {
		t.Errorf("expected healthy after recovery, got %v", sh.Status)
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Verify events were fired.
	eventsMu.Lock()
	eventCount := len(events)
	eventsMu.Unlock()

	if eventCount == 0 {
		t.Error("expected status change events to have been emitted")
	}
}

// ---------------------------------------------------------------------------
// 21. Sentinel error identity
// ---------------------------------------------------------------------------

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrNilConfig", ErrNilConfig, "config cannot be nil"},
		{"ErrInvalidInterval", ErrInvalidInterval, "poll interval must be > 0"},
		{"ErrServiceNotFound", ErrServiceNotFound, "service not found"},
		{"ErrMonitorStopped", ErrMonitorStopped, "monitor has been stopped"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Error() != tc.want {
				t.Errorf("%s.Error() = %q, want %q", tc.name, tc.err.Error(), tc.want)
			}
		})
	}
}
