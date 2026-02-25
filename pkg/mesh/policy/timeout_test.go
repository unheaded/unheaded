// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package policy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// ==================== TimeoutConfig Tests ====================

func TestDefaultTimeoutConfig(t *testing.T) {
	config := DefaultTimeoutConfig()

	if config.Request != 30*time.Second {
		t.Errorf("expected Request 30s, got %v", config.Request)
	}
	if config.Connect != 5*time.Second {
		t.Errorf("expected Connect 5s, got %v", config.Connect)
	}
	if config.Idle != 90*time.Second {
		t.Errorf("expected Idle 90s, got %v", config.Idle)
	}
	if config.Read != 30*time.Second {
		t.Errorf("expected Read 30s, got %v", config.Read)
	}
	if config.Write != 30*time.Second {
		t.Errorf("expected Write 30s, got %v", config.Write)
	}
	if config.TLSShake != 10*time.Second {
		t.Errorf("expected TLSShake 10s, got %v", config.TLSShake)
	}
	if config.KeepAlive != 30*time.Second {
		t.Errorf("expected KeepAlive 30s, got %v", config.KeepAlive)
	}
}

// ==================== TimeoutController Tests ====================

func TestNewTimeoutController(t *testing.T) {
	t.Run("WithNilConfig", func(t *testing.T) {
		tc := NewTimeoutController(nil)
		if tc == nil {
			t.Fatal("expected non-nil timeout controller")
		}
		// Should use defaults
		if tc.config.Request != 30*time.Second {
			t.Errorf("expected default Request 30s, got %v", tc.config.Request)
		}
	})

	t.Run("WithCustomConfig", func(t *testing.T) {
		config := &TimeoutConfig{
			Request: 60 * time.Second,
			Connect: 10 * time.Second,
		}
		tc := NewTimeoutController(config)

		if tc.config.Request != 60*time.Second {
			t.Errorf("expected Request 60s, got %v", tc.config.Request)
		}
		if tc.config.Connect != 10*time.Second {
			t.Errorf("expected Connect 10s, got %v", tc.config.Connect)
		}
	})
}

func TestTimeoutController_WithRequestTimeout(t *testing.T) {
	config := &TimeoutConfig{Request: 100 * time.Millisecond}
	tc := NewTimeoutController(config)

	ctx, cancel := tc.WithRequestTimeout(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("expected deadline to be set")
	}

	expected := time.Now().Add(100 * time.Millisecond)
	if deadline.After(expected.Add(10 * time.Millisecond)) {
		t.Error("deadline too far in future")
	}
}

func TestTimeoutController_WithConnectTimeout(t *testing.T) {
	config := &TimeoutConfig{Connect: 50 * time.Millisecond}
	tc := NewTimeoutController(config)

	ctx, cancel := tc.WithConnectTimeout(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("expected deadline to be set")
	}

	expected := time.Now().Add(50 * time.Millisecond)
	if deadline.After(expected.Add(10 * time.Millisecond)) {
		t.Error("deadline too far in future")
	}
}

func TestTimeoutController_Execute(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		config := &TimeoutConfig{Request: 100 * time.Millisecond}
		tc := NewTimeoutController(config)
		ctx := context.Background()

		executed := false
		err := tc.Execute(ctx, func(ctx context.Context) error {
			executed = true
			return nil
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !executed {
			t.Error("operation was not executed")
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		config := &TimeoutConfig{Request: 50 * time.Millisecond}
		tc := NewTimeoutController(config)
		ctx := context.Background()

		err := tc.Execute(ctx, func(ctx context.Context) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})

		if err != ErrTimeout {
			t.Errorf("expected ErrTimeout, got %v", err)
		}
	})

	t.Run("OperationError", func(t *testing.T) {
		config := &TimeoutConfig{Request: 100 * time.Millisecond}
		tc := NewTimeoutController(config)
		ctx := context.Background()
		expectedErr := errors.New("operation failed")

		err := tc.Execute(ctx, func(ctx context.Context) error {
			return expectedErr
		})

		if err != expectedErr {
			t.Errorf("expected operation error, got %v", err)
		}
	})
}

// ==================== TimeoutConn Tests ====================

func TestTimeoutConn(t *testing.T) {
	// Create a pipe for testing
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tc := NewTimeoutConn(client, 100*time.Millisecond, 100*time.Millisecond)

	t.Run("Read_Success", func(t *testing.T) {
		go func() {
			server.Write([]byte("hello"))
		}()

		buf := make([]byte, 10)
		n, err := tc.Read(buf)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if n != 5 {
			t.Errorf("expected 5 bytes, got %d", n)
		}
		if string(buf[:n]) != "hello" {
			t.Errorf("expected 'hello', got '%s'", string(buf[:n]))
		}
	})

	t.Run("Write_Success", func(t *testing.T) {
		go func() {
			buf := make([]byte, 10)
			server.Read(buf)
		}()

		n, err := tc.Write([]byte("world"))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if n != 5 {
			t.Errorf("expected 5 bytes written, got %d", n)
		}
	})

	t.Run("SetTimeouts", func(t *testing.T) {
		tc.SetTimeouts(50*time.Millisecond, 50*time.Millisecond)
		if tc.readTimeout != 50*time.Millisecond {
			t.Errorf("expected read timeout 50ms, got %v", tc.readTimeout)
		}
		if tc.writeTimeout != 50*time.Millisecond {
			t.Errorf("expected write timeout 50ms, got %v", tc.writeTimeout)
		}
	})
}

func TestTimeoutConn_NoTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tc := NewTimeoutConn(client, 0, 0) // No timeouts

	go func() {
		server.Write([]byte("test"))
	}()

	buf := make([]byte, 10)
	_, err := tc.Read(buf)
	if err != nil {
		t.Errorf("unexpected error with no timeout: %v", err)
	}
}

// ==================== DeadlineManager Tests ====================

func TestDeadlineManager(t *testing.T) {
	t.Run("NewDeadlineManager", func(t *testing.T) {
		dm := NewDeadlineManager()
		defer dm.Stop()

		if dm == nil {
			t.Fatal("expected non-nil deadline manager")
		}
	})

	t.Run("Set_and_Get", func(t *testing.T) {
		dm := NewDeadlineManager()
		defer dm.Stop()

		deadline := time.Now().Add(1 * time.Hour)
		dm.Set("request-1", deadline)

		got, ok := dm.Get("request-1")
		if !ok {
			t.Error("expected to find deadline")
		}
		if !got.Equal(deadline) {
			t.Errorf("expected deadline %v, got %v", deadline, got)
		}
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		dm := NewDeadlineManager()
		defer dm.Stop()

		_, ok := dm.Get("unknown")
		if ok {
			t.Error("expected not to find deadline for unknown request")
		}
	})

	t.Run("IsExpired", func(t *testing.T) {
		dm := NewDeadlineManager()
		defer dm.Stop()

		// Set past deadline
		dm.Set("expired", time.Now().Add(-1*time.Hour))
		if !dm.IsExpired("expired") {
			t.Error("expected deadline to be expired")
		}

		// Set future deadline
		dm.Set("valid", time.Now().Add(1*time.Hour))
		if dm.IsExpired("valid") {
			t.Error("expected deadline to not be expired")
		}

		// Unknown request
		if dm.IsExpired("unknown") {
			t.Error("expected unknown request to not be expired")
		}
	})

	t.Run("Remove", func(t *testing.T) {
		dm := NewDeadlineManager()
		defer dm.Stop()

		dm.Set("request-1", time.Now().Add(1*time.Hour))
		dm.Remove("request-1")

		_, ok := dm.Get("request-1")
		if ok {
			t.Error("expected deadline to be removed")
		}
	})

	t.Run("RemainingTime", func(t *testing.T) {
		dm := NewDeadlineManager()
		defer dm.Stop()

		// Future deadline
		dm.Set("future", time.Now().Add(1*time.Hour))
		remaining := dm.RemainingTime("future")
		if remaining < 59*time.Minute {
			t.Errorf("expected ~1 hour remaining, got %v", remaining)
		}

		// Past deadline
		dm.Set("past", time.Now().Add(-1*time.Hour))
		remaining = dm.RemainingTime("past")
		if remaining != 0 {
			t.Errorf("expected 0 remaining for past deadline, got %v", remaining)
		}

		// Unknown
		remaining = dm.RemainingTime("unknown")
		if remaining != 0 {
			t.Errorf("expected 0 remaining for unknown, got %v", remaining)
		}
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		dm := NewDeadlineManager()
		defer dm.Stop()

		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				id := "request-" + string(rune('a'+idx%10))
				dm.Set(id, time.Now().Add(time.Duration(idx)*time.Second))
				dm.Get(id)
				dm.IsExpired(id)
				dm.RemainingTime(id)
			}(i)
		}
		wg.Wait()
	})
}

// ==================== IdleTracker Tests ====================

func TestIdleTracker(t *testing.T) {
	t.Run("NewIdleTracker", func(t *testing.T) {
		it := NewIdleTracker(30 * time.Second)
		if it == nil {
			t.Fatal("expected non-nil idle tracker")
		}
		if it.timeout != 30*time.Second {
			t.Errorf("expected timeout 30s, got %v", it.timeout)
		}
	})

	t.Run("Touch_and_IsIdle", func(t *testing.T) {
		it := NewIdleTracker(50 * time.Millisecond)

		it.Touch("conn-1")

		// Should not be idle immediately
		if it.IsIdle("conn-1") {
			t.Error("connection should not be idle immediately after touch")
		}

		// Wait for idle timeout
		time.Sleep(100 * time.Millisecond)

		if !it.IsIdle("conn-1") {
			t.Error("connection should be idle after timeout")
		}
	})

	t.Run("Touch_Resets_Idle", func(t *testing.T) {
		it := NewIdleTracker(50 * time.Millisecond)

		it.Touch("conn-1")
		time.Sleep(30 * time.Millisecond)
		it.Touch("conn-1") // Reset
		time.Sleep(30 * time.Millisecond)

		// Should not be idle yet (only 30ms since last touch)
		if it.IsIdle("conn-1") {
			t.Error("connection should not be idle after touch reset")
		}
	})

	t.Run("IsIdle_Unknown", func(t *testing.T) {
		it := NewIdleTracker(30 * time.Second)

		if !it.IsIdle("unknown") {
			t.Error("unknown connection should be considered idle")
		}
	})

	t.Run("Remove", func(t *testing.T) {
		it := NewIdleTracker(30 * time.Second)

		it.Touch("conn-1")
		it.Remove("conn-1")

		// After remove, should be considered idle (unknown)
		if !it.IsIdle("conn-1") {
			t.Error("removed connection should be considered idle")
		}
	})

	t.Run("GetIdleConnections", func(t *testing.T) {
		it := NewIdleTracker(50 * time.Millisecond)

		it.Touch("conn-1")
		it.Touch("conn-2")
		it.Touch("conn-3")

		// Initially none idle
		idle := it.GetIdleConnections()
		if len(idle) != 0 {
			t.Errorf("expected no idle connections initially, got %d", len(idle))
		}

		// Wait for timeout
		time.Sleep(100 * time.Millisecond)

		idle = it.GetIdleConnections()
		if len(idle) != 3 {
			t.Errorf("expected 3 idle connections, got %d", len(idle))
		}
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		it := NewIdleTracker(30 * time.Second)

		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				id := "conn-" + string(rune('a'+idx%10))
				it.Touch(id)
				it.IsIdle(id)
			}(i)
		}
		wg.Wait()
	})
}

// ==================== TimeoutReader Tests ====================

func TestTimeoutReader(t *testing.T) {
	t.Run("Read_Success", func(t *testing.T) {
		data := []byte("hello world")
		reader := bytes.NewReader(data)
		tr := NewTimeoutReader(reader, 100*time.Millisecond)

		buf := make([]byte, len(data))
		n, err := tr.Read(buf)
		if err != nil && err != io.EOF {
			t.Errorf("unexpected error: %v", err)
		}
		if n != len(data) {
			t.Errorf("expected %d bytes, got %d", len(data), n)
		}
		if string(buf) != "hello world" {
			t.Errorf("expected 'hello world', got '%s'", string(buf))
		}
	})

	t.Run("Read_NoTimeout", func(t *testing.T) {
		data := []byte("test")
		reader := bytes.NewReader(data)
		tr := NewTimeoutReader(reader, 0) // No timeout

		buf := make([]byte, len(data))
		n, err := tr.Read(buf)
		if err != nil && err != io.EOF {
			t.Errorf("unexpected error: %v", err)
		}
		if n != len(data) {
			t.Errorf("expected %d bytes, got %d", len(data), n)
		}
	})

	t.Run("Read_Timeout", func(t *testing.T) {
		// Create a slow reader that blocks
		slowReader := &slowReader{delay: 200 * time.Millisecond}
		tr := NewTimeoutReader(slowReader, 50*time.Millisecond)

		buf := make([]byte, 10)
		_, err := tr.Read(buf)
		if err != ErrTimeout {
			t.Errorf("expected ErrTimeout, got %v", err)
		}
	})
}

type slowReader struct {
	delay time.Duration
}

func (s *slowReader) Read(p []byte) (n int, err error) {
	time.Sleep(s.delay)
	copy(p, []byte("data"))
	return 4, nil
}

// ==================== Error Tests ====================

func TestTimeoutErrors(t *testing.T) {
	t.Run("ErrTimeout", func(t *testing.T) {
		if ErrTimeout.Error() != "operation timed out" {
			t.Errorf("unexpected error message: %s", ErrTimeout.Error())
		}
	})

	t.Run("ErrDeadlineExceeded", func(t *testing.T) {
		if ErrDeadlineExceeded.Error() != "deadline exceeded" {
			t.Errorf("unexpected error message: %s", ErrDeadlineExceeded.Error())
		}
	})
}

// ==================== Benchmark Tests ====================

func BenchmarkTimeoutController_Execute(b *testing.B) {
	config := &TimeoutConfig{Request: 30 * time.Second}
	tc := NewTimeoutController(config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.Execute(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkDeadlineManager_Set(b *testing.B) {
	dm := NewDeadlineManager()
	defer dm.Stop()

	deadline := time.Now().Add(1 * time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dm.Set("request", deadline)
	}
}

func BenchmarkDeadlineManager_Get(b *testing.B) {
	dm := NewDeadlineManager()
	defer dm.Stop()
	dm.Set("request", time.Now().Add(1*time.Hour))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dm.Get("request")
	}
}

func BenchmarkIdleTracker_Touch(b *testing.B) {
	it := NewIdleTracker(30 * time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it.Touch("conn-1")
	}
}

func BenchmarkIdleTracker_IsIdle(b *testing.B) {
	it := NewIdleTracker(30 * time.Second)
	it.Touch("conn-1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it.IsIdle("conn-1")
	}
}
