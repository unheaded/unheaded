// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestNewRateLimiter verifies rate limiter creation
func TestNewRateLimiter(t *testing.T) {
	r := rate.Limit(10)
	burst := 20
	cleanup := 1 * time.Minute

	rl := NewRateLimiter(r, burst, cleanup)

	if rl == nil {
		t.Fatal("NewRateLimiter() returned nil")
	}
	if rl.rate != r {
		t.Errorf("rate = %v, want %v", rl.rate, r)
	}
	if rl.burst != burst {
		t.Errorf("burst = %d, want %d", rl.burst, burst)
	}
	if rl.cleanup != cleanup {
		t.Errorf("cleanup = %v, want %v", rl.cleanup, cleanup)
	}
	if rl.limiters == nil {
		t.Error("limiters map is nil")
	}

	// Cleanup for goroutine
	time.Sleep(10 * time.Millisecond)
}

// TestGetLimiter_NewClient tests getting limiter for new client
func TestGetLimiter_NewClient(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 20, 1*time.Minute)

	limiter := rl.GetLimiter("client1")

	if limiter == nil {
		t.Fatal("GetLimiter() returned nil")
	}

	// Verify limiter was stored
	rl.mu.RLock()
	stored, exists := rl.limiters["client1"]
	rl.mu.RUnlock()

	if !exists {
		t.Error("Limiter not stored in map")
	}
	if stored != limiter {
		t.Error("Stored limiter is not the same instance")
	}
}

// TestGetLimiter_ExistingClient tests getting limiter for existing client
func TestGetLimiter_ExistingClient(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 20, 1*time.Minute)

	limiter1 := rl.GetLimiter("client1")
	limiter2 := rl.GetLimiter("client1")

	if limiter1 != limiter2 {
		t.Error("GetLimiter returned different instances for same client")
	}

	// Verify only one entry in map
	rl.mu.RLock()
	count := len(rl.limiters)
	rl.mu.RUnlock()

	if count != 1 {
		t.Errorf("limiters map has %d entries, want 1", count)
	}
}

// TestGetLimiter_DoubleCheckLocking tests concurrent GetLimiter calls
func TestGetLimiter_DoubleCheckLocking(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 20, 1*time.Minute)

	var wg sync.WaitGroup
	limiters := make([]*rate.Limiter, 100)

	// Many goroutines try to get limiter for same client
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			limiters[idx] = rl.GetLimiter("client1")
		}(i)
	}

	wg.Wait()

	// All should get the same instance
	first := limiters[0]
	for i, limiter := range limiters {
		if limiter != first {
			t.Errorf("Limiter[%d] is different instance", i)
		}
	}

	// Should only have one entry in map
	rl.mu.RLock()
	count := len(rl.limiters)
	rl.mu.RUnlock()

	if count != 1 {
		t.Errorf("limiters map has %d entries, want 1", count)
	}
}

// TestGetLimiter_MultipleClients tests limiters for different clients
func TestGetLimiter_MultipleClients(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 20, 1*time.Minute)

	limiter1 := rl.GetLimiter("client1")
	limiter2 := rl.GetLimiter("client2")
	limiter3 := rl.GetLimiter("client3")

	// All should be different instances
	if limiter1 == limiter2 || limiter1 == limiter3 || limiter2 == limiter3 {
		t.Error("Different clients got same limiter instance")
	}

	// Verify all stored
	rl.mu.RLock()
	count := len(rl.limiters)
	rl.mu.RUnlock()

	if count != 3 {
		t.Errorf("limiters map has %d entries, want 3", count)
	}
}

// TestAllow_UnderLimit tests Allow when under rate limit
func TestAllow_UnderLimit(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 10, 1*time.Minute)

	// Should allow first requests
	for i := 0; i < 10; i++ {
		if !rl.Allow("client1") {
			t.Errorf("Allow() = false for request %d, want true", i+1)
		}
	}
}

// TestAllow_OverLimit tests Allow when over rate limit
func TestAllow_OverLimit(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 5, 1*time.Minute)

	// Exhaust burst
	for i := 0; i < 5; i++ {
		rl.Allow("client1")
	}

	// Next should be denied
	if rl.Allow("client1") {
		t.Error("Allow() = true when over limit, want false")
	}
}

// TestAllow_ClientIsolation tests that clients have separate limits
func TestAllow_ClientIsolation(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 2, 1*time.Minute)

	// Exhaust client1's limit
	rl.Allow("client1")
	rl.Allow("client1")

	// client1 should be blocked
	if rl.Allow("client1") {
		t.Error("client1 allowed when over limit")
	}

	// client2 should still be allowed
	if !rl.Allow("client2") {
		t.Error("client2 blocked when should be allowed")
	}
}

// TestWait_Success tests Wait with available tokens
func TestWait_Success(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 10, 1*time.Minute)

	ctx := context.Background()
	err := rl.Wait(ctx, "client1")

	if err != nil {
		t.Errorf("Wait() returned error: %v", err)
	}
}

// TestWait_ContextCancelled tests Wait with cancelled context
func TestWait_ContextCancelled(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1, 1*time.Minute)

	// Exhaust tokens
	rl.Allow("client1")

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rl.Wait(ctx, "client1")

	if err == nil {
		t.Error("Wait() returned nil error with cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("Wait() error = %v, want context.Canceled", err)
	}
}

// TestWait_Timeout tests Wait with timeout
func TestWait_Timeout(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1, 1*time.Minute)

	// Exhaust tokens
	rl.Allow("client1")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := rl.Wait(ctx, "client1")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Wait() returned nil error with timeout")
	}
	// rate.Limiter returns a wrapped deadline exceeded error
	if err != context.DeadlineExceeded && err.Error() != "rate: Wait(n=1) would exceed context deadline" {
		t.Errorf("Wait() error = %v, want context deadline error", err)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("Wait took %v, expected ~10ms timeout", elapsed)
	}
}

// TestReserve_Available tests Reserve with available tokens
func TestReserve_Available(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 10, 1*time.Minute)

	delay := rl.Reserve("client1")

	if delay != 0 {
		t.Errorf("Reserve() delay = %v, want 0", delay)
	}
}

// TestReserve_RequiresWait tests Reserve when tokens exhausted
func TestReserve_RequiresWait(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 5, 1*time.Minute)

	// Exhaust burst
	for i := 0; i < 5; i++ {
		rl.Reserve("client1")
	}

	// Next reserve should require waiting
	delay := rl.Reserve("client1")

	if delay == 0 {
		t.Error("Reserve() delay = 0, expected > 0")
	}

	// Delay should be reasonable (roughly 100ms for 10/sec rate)
	if delay > 200*time.Millisecond {
		t.Errorf("Reserve() delay = %v, expected ~100ms", delay)
	}
}

// TestReserve_InfRate tests Reserve with infinite rate
func TestReserve_InfRate(t *testing.T) {
	rl := NewRateLimiter(rate.Inf, 0, 1*time.Minute)

	delay := rl.Reserve("client1")

	// With infinite rate and zero burst, reservation should fail
	if delay != 0 {
		t.Errorf("Reserve() delay = %v, want 0 (failed reservation)", delay)
	}
}

// TestMiddleware_AllowedRequest tests middleware with allowed request
func TestMiddleware_AllowedRequest(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 10, 1*time.Minute)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "success" {
		t.Errorf("Body = %s, want 'success'", rec.Body.String())
	}
}

// TestMiddleware_RateLimitExceeded tests middleware blocking over-limit requests
func TestMiddleware_RateLimitExceeded(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 2, 1*time.Minute)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	// Exhaust limit
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)

	// Third request should be blocked
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req)

	if rec3.Code != http.StatusTooManyRequests {
		t.Errorf("Status = %d, want %d", rec3.Code, http.StatusTooManyRequests)
	}

	body := rec3.Body.String()
	expectedBody := "{\"error\":\"rate_limit_exceeded\",\"message\":\"Too many requests\"}\n"
	if body != expectedBody {
		t.Errorf("Body = %q, want %q", body, expectedBody)
	}
}

// TestMiddleware_DifferentClients tests middleware with different client IPs
func TestMiddleware_DifferentClients(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1, 1*time.Minute)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Client 1 exhausts limit
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Client 1 second request blocked
	rec1_2 := httptest.NewRecorder()
	handler.ServeHTTP(rec1_2, req1)
	if rec1_2.Code != http.StatusTooManyRequests {
		t.Error("Client 1 second request should be blocked")
	}

	// Client 2 should still be allowed
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.2:12345"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("Client 2 status = %d, want %d", rec2.Code, http.StatusOK)
	}
}

// TestCleanupRoutine tests that inactive limiters are removed
func TestCleanupRoutine(t *testing.T) {
	cleanup := 50 * time.Millisecond
	rl := NewRateLimiter(rate.Limit(10), 10, cleanup)

	// Create limiter and use it
	rl.Allow("client1")

	// Wait for tokens to refill (limiter becomes inactive)
	time.Sleep(200 * time.Millisecond)

	// Wait for cleanup to run
	time.Sleep(100 * time.Millisecond)

	rl.mu.RLock()
	count := len(rl.limiters)
	rl.mu.RUnlock()

	// Limiter should have been cleaned up
	if count != 0 {
		t.Errorf("limiters map has %d entries after cleanup, want 0", count)
	}
}

// TestCleanupRoutine_ActiveLimiters tests that active limiters are not removed
func TestCleanupRoutine_ActiveLimiters(t *testing.T) {
	cleanup := 50 * time.Millisecond
	rl := NewRateLimiter(rate.Limit(1), 5, cleanup)

	// Exhaust tokens
	for i := 0; i < 5; i++ {
		rl.Allow("client1")
	}

	// Wait for cleanup to run
	time.Sleep(100 * time.Millisecond)

	rl.mu.RLock()
	count := len(rl.limiters)
	rl.mu.RUnlock()

	// Active limiter (tokens not full) should not be cleaned up
	if count != 1 {
		t.Errorf("limiters map has %d entries, want 1 (active limiter)", count)
	}
}

// TestConcurrent_Allow tests concurrent Allow calls
func TestConcurrent_Allow(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(100), 100, 1*time.Minute)

	var wg sync.WaitGroup
	allowed := 0
	var mu sync.Mutex

	// Many concurrent calls
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.Allow("client1") {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Should allow exactly burst amount (100)
	if allowed != 100 {
		t.Errorf("Allowed %d requests, want 100", allowed)
	}
}

// TestConcurrent_MultipleClients tests concurrent access from multiple clients
func TestConcurrent_MultipleClients(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 10, 1*time.Minute)

	var wg sync.WaitGroup

	// Multiple clients making concurrent requests
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			key := string(rune('A' + clientID))
			for j := 0; j < 5; j++ {
				rl.Allow(key)
			}
		}(i)
	}

	wg.Wait()

	// Should have limiters for all clients
	rl.mu.RLock()
	count := len(rl.limiters)
	rl.mu.RUnlock()

	if count != 10 {
		t.Errorf("limiters map has %d entries, want 10", count)
	}
}

// TestConcurrent_GetLimiterAndAllow tests concurrent GetLimiter and Allow
func TestConcurrent_GetLimiterAndAllow(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(100), 50, 1*time.Minute)

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.GetLimiter("client1")
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow("client1")
		}()
	}

	wg.Wait()

	// Should only have one limiter
	rl.mu.RLock()
	count := len(rl.limiters)
	rl.mu.RUnlock()

	if count != 1 {
		t.Errorf("limiters map has %d entries, want 1", count)
	}
}

// TestRateLimiter_TokenRefill tests that tokens refill over time
func TestRateLimiter_TokenRefill(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 5, 1*time.Minute)

	// Exhaust burst
	for i := 0; i < 5; i++ {
		rl.Allow("client1")
	}

	// Should be denied
	if rl.Allow("client1") {
		t.Error("Should be denied after exhausting burst")
	}

	// Wait for tokens to refill (100ms for 1 token at 10/sec)
	time.Sleep(150 * time.Millisecond)

	// Should now be allowed
	if !rl.Allow("client1") {
		t.Error("Should be allowed after token refill")
	}
}

// TestRateLimiter_BurstCapacity tests burst capacity
func TestRateLimiter_BurstCapacity(t *testing.T) {
	burst := 15
	rl := NewRateLimiter(rate.Limit(1), burst, 1*time.Minute)

	// Should allow exactly burst requests immediately
	allowed := 0
	for i := 0; i < burst+5; i++ {
		if rl.Allow("client1") {
			allowed++
		}
	}

	if allowed != burst {
		t.Errorf("Allowed %d requests, want %d (burst capacity)", allowed, burst)
	}
}

// TestMiddleware_HandlerCalled tests that handler is called on allowed requests
func TestMiddleware_HandlerCalled(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 10, 1*time.Minute)

	called := false
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("Handler was not called")
	}
}

// TestMiddleware_HandlerNotCalled tests that handler is not called when rate limited
func TestMiddleware_HandlerNotCalled(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1, 1*time.Minute)

	called := 0
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	// First request should call handler
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)

	// Second request should not call handler
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)

	if called != 1 {
		t.Errorf("Handler called %d times, want 1", called)
	}
}
