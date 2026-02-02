package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ==================== TimeoutConfig Tests ====================

func TestDefaultTimeoutConfig(t *testing.T) {
	config := DefaultTimeoutConfig()

	if config.Default != 30*time.Second {
		t.Errorf("expected Default 30s, got %v", config.Default)
	}
	if config.Read != 10*time.Second {
		t.Errorf("expected Read 10s, got %v", config.Read)
	}
	if config.Write != 10*time.Second {
		t.Errorf("expected Write 10s, got %v", config.Write)
	}
	if config.Idle != 60*time.Second {
		t.Errorf("expected Idle 60s, got %v", config.Idle)
	}
}

// ==================== TimeoutManager Tests ====================

func TestNewTimeoutManager(t *testing.T) {
	t.Run("WithValidConfig", func(t *testing.T) {
		config := TimeoutConfig{
			Default: 15 * time.Second,
			Read:    5 * time.Second,
			Write:   5 * time.Second,
			Idle:    30 * time.Second,
		}

		tm := NewTimeoutManager(config)
		if tm == nil {
			t.Fatal("expected non-nil timeout manager")
		}
	})

	t.Run("WithInvalidConfig_SetsDefaults", func(t *testing.T) {
		config := TimeoutConfig{
			Default: 0, // Invalid
			Read:    0, // Invalid
			Write:   0, // Invalid
			Idle:    0, // Invalid
		}

		tm := NewTimeoutManager(config)

		if tm.config.Default != 30*time.Second {
			t.Errorf("expected default Default 30s, got %v", tm.config.Default)
		}
		if tm.config.Read != 10*time.Second {
			t.Errorf("expected default Read 10s, got %v", tm.config.Read)
		}
		if tm.config.Write != 10*time.Second {
			t.Errorf("expected default Write 10s, got %v", tm.config.Write)
		}
		if tm.config.Idle != 60*time.Second {
			t.Errorf("expected default Idle 60s, got %v", tm.config.Idle)
		}
	})
}

func TestTimeoutManager_SetServiceTimeout(t *testing.T) {
	tm := NewTimeoutManager(DefaultTimeoutConfig())

	tm.SetServiceTimeout("api-service", 5*time.Second)
	timeout := tm.GetServiceTimeout("api-service")

	if timeout != 5*time.Second {
		t.Errorf("expected timeout 5s for api-service, got %v", timeout)
	}
}

func TestTimeoutManager_GetServiceTimeout(t *testing.T) {
	t.Run("ExistingService", func(t *testing.T) {
		tm := NewTimeoutManager(DefaultTimeoutConfig())
		tm.SetServiceTimeout("api-service", 10*time.Second)

		timeout := tm.GetServiceTimeout("api-service")
		if timeout != 10*time.Second {
			t.Errorf("expected 10s, got %v", timeout)
		}
	})

	t.Run("NonExistingService_ReturnsDefault", func(t *testing.T) {
		tm := NewTimeoutManager(DefaultTimeoutConfig())

		timeout := tm.GetServiceTimeout("unknown-service")
		if timeout != 30*time.Second {
			t.Errorf("expected default 30s, got %v", timeout)
		}
	})
}

func TestTimeoutManager_WithTimeout(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		tm := NewTimeoutManager(DefaultTimeoutConfig())
		ctx := context.Background()
		executed := false

		err := tm.WithTimeout(ctx, 100*time.Millisecond, func(ctx context.Context) error {
			executed = true
			return nil
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !executed {
			t.Error("function was not executed")
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		tm := NewTimeoutManager(DefaultTimeoutConfig())
		ctx := context.Background()

		err := tm.WithTimeout(ctx, 50*time.Millisecond, func(ctx context.Context) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})

		if err != ErrTimeout {
			t.Errorf("expected ErrTimeout, got %v", err)
		}
	})

	t.Run("UsesDefaultTimeout_WhenZero", func(t *testing.T) {
		config := TimeoutConfig{Default: 50 * time.Millisecond}
		tm := NewTimeoutManager(config)
		ctx := context.Background()

		err := tm.WithTimeout(ctx, 0, func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})

		if err != ErrTimeout {
			t.Errorf("expected ErrTimeout with default timeout, got %v", err)
		}
	})

	t.Run("OnTimeoutCallback", func(t *testing.T) {
		var callbackCalled bool
		var callbackOp string
		var callbackDuration time.Duration

		config := TimeoutConfig{
			Default: 50 * time.Millisecond,
			OnTimeout: func(operation string, duration time.Duration) {
				callbackCalled = true
				callbackOp = operation
				callbackDuration = duration
			},
		}
		tm := NewTimeoutManager(config)
		ctx := context.Background()

		tm.WithTimeout(ctx, 50*time.Millisecond, func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})

		if !callbackCalled {
			t.Error("expected OnTimeout callback to be called")
		}
		if callbackOp != "request" {
			t.Errorf("expected operation 'request', got '%s'", callbackOp)
		}
		if callbackDuration != 50*time.Millisecond {
			t.Errorf("expected duration 50ms, got %v", callbackDuration)
		}
	})

	t.Run("FunctionError", func(t *testing.T) {
		tm := NewTimeoutManager(DefaultTimeoutConfig())
		ctx := context.Background()
		expectedErr := errors.New("function error")

		err := tm.WithTimeout(ctx, 100*time.Millisecond, func(ctx context.Context) error {
			return expectedErr
		})

		if err != expectedErr {
			t.Errorf("expected function error, got %v", err)
		}
	})
}

func TestTimeoutManager_WithServiceTimeout(t *testing.T) {
	tm := NewTimeoutManager(DefaultTimeoutConfig())
	tm.SetServiceTimeout("fast-service", 50*time.Millisecond)

	ctx := context.Background()

	err := tm.WithServiceTimeout(ctx, "fast-service", func(ctx context.Context) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	if err != ErrTimeout {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

func TestTimeoutManager_ReadWriteIdleTimeouts(t *testing.T) {
	config := TimeoutConfig{
		Default: 30 * time.Second,
		Read:    5 * time.Second,
		Write:   10 * time.Second,
		Idle:    60 * time.Second,
	}
	tm := NewTimeoutManager(config)

	if tm.ReadTimeout() != 5*time.Second {
		t.Errorf("expected ReadTimeout 5s, got %v", tm.ReadTimeout())
	}
	if tm.WriteTimeout() != 10*time.Second {
		t.Errorf("expected WriteTimeout 10s, got %v", tm.WriteTimeout())
	}
	if tm.IdleTimeout() != 60*time.Second {
		t.Errorf("expected IdleTimeout 60s, got %v", tm.IdleTimeout())
	}
}

func TestTimeoutManager_TimeoutContext(t *testing.T) {
	config := TimeoutConfig{Default: 100 * time.Millisecond}
	tm := NewTimeoutManager(config)

	ctx, cancel := tm.TimeoutContext(context.Background())
	defer cancel()

	// Verify deadline is set
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("expected deadline to be set")
	}

	expectedDeadline := time.Now().Add(100 * time.Millisecond)
	if deadline.Before(time.Now()) || deadline.After(expectedDeadline.Add(10*time.Millisecond)) {
		t.Errorf("deadline %v not within expected range", deadline)
	}
}

func TestTimeoutManager_ConcurrentAccess(t *testing.T) {
	tm := NewTimeoutManager(DefaultTimeoutConfig())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			service := "service-" + string(rune('a'+idx%5))
			tm.SetServiceTimeout(service, time.Duration(idx)*time.Millisecond)
		}(i)
		go func(idx int) {
			defer wg.Done()
			service := "service-" + string(rune('a'+idx%5))
			tm.GetServiceTimeout(service)
		}(i)
	}
	wg.Wait()
}

// ==================== Deadline Tests ====================

func TestDeadline(t *testing.T) {
	t.Run("NewDeadline", func(t *testing.T) {
		d := NewDeadline(100 * time.Millisecond)
		if d == nil {
			t.Fatal("expected non-nil deadline")
		}
		if d.timeout != 100*time.Millisecond {
			t.Errorf("expected timeout 100ms, got %v", d.timeout)
		}
	})

	t.Run("Start", func(t *testing.T) {
		d := NewDeadline(50 * time.Millisecond)
		ch := d.Start()

		select {
		case <-ch:
			// OK - timer fired
		case <-time.After(100 * time.Millisecond):
			t.Error("deadline timer did not fire")
		}
	})

	t.Run("Stop", func(t *testing.T) {
		d := NewDeadline(100 * time.Millisecond)
		ch := d.Start()
		d.Stop()

		select {
		case <-ch:
			t.Error("stopped timer should not fire")
		case <-time.After(150 * time.Millisecond):
			// OK - timer was stopped
		}
	})

	t.Run("Reset", func(t *testing.T) {
		d := NewDeadline(50 * time.Millisecond)
		ch := d.Start()

		// Reset before it fires
		time.Sleep(30 * time.Millisecond)
		d.Reset()

		// Should fire ~50ms after reset, not after original start
		start := time.Now()
		<-ch
		elapsed := time.Since(start)

		// Should be roughly 50ms from the reset point
		if elapsed < 40*time.Millisecond || elapsed > 70*time.Millisecond {
			t.Errorf("expected ~50ms after reset, got %v", elapsed)
		}
	})

	t.Run("Stop_BeforeStart", func(t *testing.T) {
		d := NewDeadline(100 * time.Millisecond)
		// Should not panic
		d.Stop()
	})

	t.Run("Reset_BeforeStart", func(t *testing.T) {
		d := NewDeadline(100 * time.Millisecond)
		// Should not panic
		d.Reset()
	})
}

// ==================== Error Tests ====================

func TestErrTimeout(t *testing.T) {
	if ErrTimeout.Error() != "request timeout" {
		t.Errorf("unexpected error message: %s", ErrTimeout.Error())
	}
}

// ==================== Benchmark Tests ====================

func BenchmarkTimeoutManager_WithTimeout_Fast(b *testing.B) {
	tm := NewTimeoutManager(DefaultTimeoutConfig())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.WithTimeout(ctx, 1*time.Second, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkTimeoutManager_GetServiceTimeout(b *testing.B) {
	tm := NewTimeoutManager(DefaultTimeoutConfig())
	tm.SetServiceTimeout("api-service", 5*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.GetServiceTimeout("api-service")
	}
}

func BenchmarkTimeoutManager_SetServiceTimeout(b *testing.B) {
	tm := NewTimeoutManager(DefaultTimeoutConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.SetServiceTimeout("service", time.Duration(i)*time.Millisecond)
	}
}
