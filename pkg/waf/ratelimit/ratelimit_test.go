// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSlidingWindow_Allow(t *testing.T) {
	limiter := NewSlidingWindow(5, time.Second)
	defer limiter.Close()

	key := "test-client"

	// Should allow first 5 requests
	for i := 0; i < 5; i++ {
		if !limiter.Allow(key) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be blocked
	if limiter.Allow(key) {
		t.Error("6th request should be blocked")
	}

	// Different key should be allowed
	if !limiter.Allow("other-client") {
		t.Error("Different client should be allowed")
	}
}

func TestSlidingWindow_AllowN(t *testing.T) {
	limiter := NewSlidingWindow(10, time.Second)
	defer limiter.Close()

	key := "test-client"

	// Allow 5 requests
	if !limiter.AllowN(key, 5) {
		t.Error("AllowN(5) should succeed")
	}

	// Allow 3 more
	if !limiter.AllowN(key, 3) {
		t.Error("AllowN(3) should succeed")
	}

	// Allow 2 more
	if !limiter.AllowN(key, 2) {
		t.Error("AllowN(2) should succeed")
	}

	// 1 more should fail
	if limiter.AllowN(key, 1) {
		t.Error("AllowN(1) should fail when at limit")
	}
}

func TestSlidingWindow_Reset(t *testing.T) {
	limiter := NewSlidingWindow(5, time.Second)
	defer limiter.Close()

	key := "test-client"

	// Exhaust the limit
	for i := 0; i < 5; i++ {
		limiter.Allow(key)
	}

	// Should be blocked
	if limiter.Allow(key) {
		t.Error("Should be blocked after exhausting limit")
	}

	// Reset
	limiter.Reset(key)

	// Should be allowed again
	if !limiter.Allow(key) {
		t.Error("Should be allowed after reset")
	}
}

func TestSlidingWindow_GetRemaining(t *testing.T) {
	limiter := NewSlidingWindow(10, time.Second)
	defer limiter.Close()

	key := "test-client"

	// Initial remaining should be full
	remaining := limiter.GetRemaining(key)
	if remaining != 10 {
		t.Errorf("Initial remaining = %d, want 10", remaining)
	}

	// Use 3 requests
	for i := 0; i < 3; i++ {
		limiter.Allow(key)
	}

	remaining = limiter.GetRemaining(key)
	if remaining != 7 {
		t.Errorf("Remaining after 3 requests = %d, want 7", remaining)
	}
}

func TestSlidingWindow_WindowExpiration(t *testing.T) {
	limiter := NewSlidingWindow(5, 100*time.Millisecond)
	defer limiter.Close()

	key := "test-client"

	// Exhaust the limit
	for i := 0; i < 5; i++ {
		limiter.Allow(key)
	}

	// Should be blocked
	if limiter.Allow(key) {
		t.Error("Should be blocked")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	if !limiter.Allow(key) {
		t.Error("Should be allowed after window expiration")
	}
}

func TestSlidingWindow_ConcurrentAccess(t *testing.T) {
	limiter := NewSlidingWindow(100, time.Second)
	defer limiter.Close()

	key := "test-client"
	var wg sync.WaitGroup
	var allowed int64
	var mu sync.Mutex

	// 200 concurrent requests
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.Allow(key) {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if allowed > 100 {
		t.Errorf("Allowed %d requests, want <= 100", allowed)
	}
}

func TestSlidingWindowWithPrecision(t *testing.T) {
	limiter := NewSlidingWindowWithPrecision(10, time.Second, 20)
	defer limiter.Close()

	key := "test-client"

	// Should work the same as regular sliding window
	for i := 0; i < 10; i++ {
		if !limiter.Allow(key) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	if limiter.Allow(key) {
		t.Error("11th request should be blocked")
	}
}

func TestSlidingWindow_Stats(t *testing.T) {
	limiter := NewSlidingWindow(10, time.Second)
	defer limiter.Close()

	// Generate some activity
	for i := 0; i < 5; i++ {
		limiter.Allow("client-" + string(rune(i+'0')))
	}

	stats := limiter.Stats()

	if stats["keys"] != 5 {
		t.Errorf("Stats keys = %d, want 5", stats["keys"])
	}

	if stats["rate"] != 10 {
		t.Errorf("Stats rate = %d, want 10", stats["rate"])
	}
}

func TestTokenBucket_Allow(t *testing.T) {
	limiter := NewTokenBucket(10, 5) // 10 tokens/sec, max 5
	defer limiter.Close()

	key := "test-client"

	// Should allow burst of 5
	for i := 0; i < 5; i++ {
		if !limiter.Allow(key) {
			t.Errorf("Request %d should be allowed (burst)", i+1)
		}
	}

	// 6th should be blocked (bucket empty)
	if limiter.Allow(key) {
		t.Error("6th request should be blocked")
	}
}

func TestTokenBucket_AllowN(t *testing.T) {
	limiter := NewTokenBucket(10, 10)
	defer limiter.Close()

	key := "test-client"

	// Allow 5 tokens
	if !limiter.AllowN(key, 5) {
		t.Error("AllowN(5) should succeed")
	}

	// Allow 5 more
	if !limiter.AllowN(key, 5) {
		t.Error("AllowN(5) should succeed again")
	}

	// Should fail now
	if limiter.AllowN(key, 1) {
		t.Error("AllowN(1) should fail")
	}
}

func TestTokenBucket_Reset(t *testing.T) {
	limiter := NewTokenBucket(1, 5)
	defer limiter.Close()

	key := "test-client"

	// Exhaust tokens
	for i := 0; i < 5; i++ {
		limiter.Allow(key)
	}

	// Should be blocked
	if limiter.Allow(key) {
		t.Error("Should be blocked")
	}

	// Reset
	limiter.Reset(key)

	// Should be allowed (tokens restored)
	if !limiter.Allow(key) {
		t.Error("Should be allowed after reset")
	}
}

func TestTokenBucket_GetTokens(t *testing.T) {
	limiter := NewTokenBucket(10, 10)
	defer limiter.Close()

	key := "test-client"

	// Initial tokens should be full
	tokens := limiter.GetTokens(key)
	if tokens != 10 {
		t.Errorf("Initial tokens = %f, want 10", tokens)
	}

	// Use 3 tokens
	limiter.AllowN(key, 3)

	tokens = limiter.GetTokens(key)
	if tokens < 6 || tokens > 8 { // Allow some refill
		t.Errorf("Tokens after using 3 = %f, want ~7", tokens)
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	limiter := NewTokenBucket(100, 5) // 100 tokens/sec, max 5
	defer limiter.Close()

	key := "test-client"

	// Exhaust all tokens
	for i := 0; i < 5; i++ {
		limiter.Allow(key)
	}

	// Wait for some refill
	time.Sleep(50 * time.Millisecond)

	// Should have some tokens now
	tokens := limiter.GetTokens(key)
	if tokens < 1 {
		t.Error("Expected tokens to refill")
	}
}

func TestTokenBucket_WaitTime(t *testing.T) {
	limiter := NewTokenBucket(10, 5)
	defer limiter.Close()

	key := "test-client"

	// Exhaust tokens
	for i := 0; i < 5; i++ {
		limiter.Allow(key)
	}

	// Check wait time
	waitTime := limiter.WaitTime(key, 1)
	if waitTime <= 0 {
		t.Error("Expected positive wait time when tokens exhausted")
	}
}

func TestTokenBucket_Reserve(t *testing.T) {
	limiter := NewTokenBucket(10, 5)
	defer limiter.Close()

	key := "test-client"

	// Reserve some tokens
	wait := limiter.Reserve(key, 3)
	if wait != 0 {
		t.Error("Expected no wait for first reserve")
	}

	// Reserve more
	wait = limiter.Reserve(key, 5)
	if wait == 0 {
		t.Error("Expected wait time when reserving more than available")
	}
}

func TestTokenBucket_SetRate(t *testing.T) {
	limiter := NewTokenBucket(10, 5)
	defer limiter.Close()

	limiter.SetRate(20)

	stats := limiter.Stats()
	if stats["rate"].(float64) != 20 {
		t.Errorf("Rate = %v, want 20", stats["rate"])
	}
}

func TestTokenBucket_SetCapacity(t *testing.T) {
	limiter := NewTokenBucket(10, 5)
	defer limiter.Close()

	limiter.SetCapacity(20)

	stats := limiter.Stats()
	if stats["capacity"].(int) != 20 {
		t.Errorf("Capacity = %v, want 20", stats["capacity"])
	}
}

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	limiter := NewTokenBucket(100, 50)
	defer limiter.Close()

	key := "test-client"
	var wg sync.WaitGroup
	var allowed int64
	var mu sync.Mutex

	// 100 concurrent requests
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.Allow(key) {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if allowed > 50 {
		t.Errorf("Allowed %d requests, want <= 50", allowed)
	}
}

func TestMultiLimiter_Allow(t *testing.T) {
	limiter1 := NewSlidingWindow(10, time.Second)
	limiter2 := NewTokenBucket(10, 5)

	multi := NewMultiLimiter(limiter1, limiter2)
	defer multi.Close() // Closes all underlying limiters

	key := "test-client"

	// Should allow up to the most restrictive limit (5 from token bucket)
	for i := 0; i < 5; i++ {
		if !multi.Allow(key) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th should be blocked by token bucket
	if multi.Allow(key) {
		t.Error("6th request should be blocked")
	}
}

func TestMultiLimiter_AllowN(t *testing.T) {
	limiter1 := NewSlidingWindow(10, time.Second)
	limiter2 := NewSlidingWindow(20, time.Second)

	multi := NewMultiLimiter(limiter1, limiter2)
	defer multi.Close() // Closes all underlying limiters

	key := "test-client"

	// Should allow based on most restrictive
	if !multi.AllowN(key, 10) {
		t.Error("AllowN(10) should succeed")
	}

	// Should be blocked by limiter1
	if multi.AllowN(key, 1) {
		t.Error("Should be blocked after 10 requests")
	}
}

func TestMultiLimiter_Reset(t *testing.T) {
	limiter1 := NewSlidingWindow(5, time.Second)
	limiter2 := NewSlidingWindow(5, time.Second)

	multi := NewMultiLimiter(limiter1, limiter2)
	defer multi.Close() // Closes all underlying limiters

	key := "test-client"

	// Exhaust limits
	for i := 0; i < 5; i++ {
		multi.Allow(key)
	}

	// Reset both
	multi.Reset(key)

	// Should be allowed
	if !multi.Allow(key) {
		t.Error("Should be allowed after reset")
	}
}

func TestMultiLimiter_AddLimiter(t *testing.T) {
	limiter1 := NewSlidingWindow(10, time.Second)
	defer limiter1.Close()

	multi := NewMultiLimiter(limiter1)

	limiter2 := NewSlidingWindow(5, time.Second)
	defer limiter2.Close()

	multi.AddLimiter(limiter2)

	key := "test-client"

	// Now limited to 5 (from limiter2)
	for i := 0; i < 5; i++ {
		if !multi.Allow(key) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	if multi.Allow(key) {
		t.Error("6th request should be blocked")
	}
}

func TestEndpointLimiter_Allow(t *testing.T) {
	factory := func() Limiter {
		return NewSlidingWindow(5, time.Second)
	}

	limiter := NewEndpointLimiter(factory)
	defer limiter.Close()

	key := "test-client"

	// Each endpoint should have its own limit
	for i := 0; i < 5; i++ {
		if !limiter.Allow("/api/v1/users", key) {
			t.Error("Request to /api/v1/users should be allowed")
		}
	}

	// 6th request to same endpoint should be blocked
	if limiter.Allow("/api/v1/users", key) {
		t.Error("6th request to /api/v1/users should be blocked")
	}

	// Different endpoint should still be allowed
	if !limiter.Allow("/api/v1/products", key) {
		t.Error("Request to different endpoint should be allowed")
	}
}

func TestEndpointLimiter_AllowN(t *testing.T) {
	factory := func() Limiter {
		return NewSlidingWindow(10, time.Second)
	}

	limiter := NewEndpointLimiter(factory)
	defer limiter.Close()

	key := "test-client"

	if !limiter.AllowN("/api/v1/users", key, 5) {
		t.Error("AllowN(5) should succeed")
	}

	if !limiter.AllowN("/api/v1/users", key, 5) {
		t.Error("AllowN(5) should succeed again")
	}

	if limiter.AllowN("/api/v1/users", key, 1) {
		t.Error("AllowN(1) should fail")
	}
}

func TestEndpointLimiter_Reset(t *testing.T) {
	factory := func() Limiter {
		return NewSlidingWindow(5, time.Second)
	}

	limiter := NewEndpointLimiter(factory)
	defer limiter.Close()

	key := "test-client"
	endpoint := "/api/v1/users"

	// Exhaust limit
	for i := 0; i < 5; i++ {
		limiter.Allow(endpoint, key)
	}

	// Reset
	limiter.Reset(endpoint, key)

	// Should be allowed again
	if !limiter.Allow(endpoint, key) {
		t.Error("Should be allowed after reset")
	}
}

func TestRateLimitMiddleware_Check(t *testing.T) {
	limiter := NewSlidingWindow(5, time.Second)
	defer limiter.Close()

	keyExtractor := func(ctx context.Context) string {
		if key, ok := ctx.Value("client_ip").(string); ok {
			return key
		}
		return "unknown"
	}

	var limitReachedCount int
	onLimitReached := func(ctx context.Context, key string) {
		limitReachedCount++
	}

	middleware := NewRateLimitMiddleware(limiter, keyExtractor)
	middleware.SetLimitReachedHandler(onLimitReached)

	ctx := context.WithValue(context.Background(), "client_ip", "1.2.3.4")

	// Should allow first 5
	for i := 0; i < 5; i++ {
		if !middleware.Check(ctx) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th should trigger limit reached
	if middleware.Check(ctx) {
		t.Error("6th request should be blocked")
	}

	if limitReachedCount != 1 {
		t.Errorf("onLimitReached called %d times, want 1", limitReachedCount)
	}
}

func BenchmarkSlidingWindow_Allow(b *testing.B) {
	limiter := NewSlidingWindow(1000000, time.Minute)
	defer limiter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("test-key")
	}
}

func BenchmarkSlidingWindow_AllowConcurrent(b *testing.B) {
	limiter := NewSlidingWindow(1000000, time.Minute)
	defer limiter.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow("test-key")
		}
	})
}

func BenchmarkTokenBucket_Allow(b *testing.B) {
	limiter := NewTokenBucket(1000000, 1000000)
	defer limiter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("test-key")
	}
}

func BenchmarkTokenBucket_AllowConcurrent(b *testing.B) {
	limiter := NewTokenBucket(1000000, 1000000)
	defer limiter.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow("test-key")
		}
	})
}

func BenchmarkMultiLimiter_Allow(b *testing.B) {
	limiter1 := NewSlidingWindow(1000000, time.Minute)
	limiter2 := NewTokenBucket(1000000, 1000000)
	defer limiter1.Close()
	defer limiter2.Close()

	multi := NewMultiLimiter(limiter1, limiter2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		multi.Allow("test-key")
	}
}

func BenchmarkEndpointLimiter_Allow(b *testing.B) {
	factory := func() Limiter {
		return NewSlidingWindow(1000000, time.Minute)
	}

	limiter := NewEndpointLimiter(factory)
	defer limiter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("/api/v1/users", "test-key")
	}
}
