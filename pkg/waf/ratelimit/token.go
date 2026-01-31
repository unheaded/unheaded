// Package ratelimit provides token bucket rate limiting
package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	rate       float64       // Tokens per second
	capacity   int           // Maximum tokens
	buckets    map[string]*bucket
	mu         sync.RWMutex
	cleanupInt time.Duration
	stopCh     chan struct{}
}

type bucket struct {
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(rate float64, capacity int) *TokenBucket {
	tb := &TokenBucket{
		rate:       rate,
		capacity:   capacity,
		buckets:    make(map[string]*bucket),
		cleanupInt: time.Minute * 5,
		stopCh:     make(chan struct{}),
	}

	go tb.cleanup()
	return tb
}

// Allow checks if a request should be allowed
func (tb *TokenBucket) Allow(key string) bool {
	return tb.AllowN(key, 1)
}

// AllowN checks if N requests should be allowed
func (tb *TokenBucket) AllowN(key string, n int) bool {
	b := tb.getBucket(key)

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	tb.refill(b, now)

	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}

	return false
}

// getBucket gets or creates a bucket for a key
func (tb *TokenBucket) getBucket(key string) *bucket {
	tb.mu.RLock()
	b, ok := tb.buckets[key]
	tb.mu.RUnlock()

	if ok {
		return b
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Double-check
	if b, ok = tb.buckets[key]; ok {
		return b
	}

	b = &bucket{
		tokens:     float64(tb.capacity),
		lastUpdate: time.Now(),
	}
	tb.buckets[key] = b
	return b
}

// refill adds tokens based on elapsed time
func (tb *TokenBucket) refill(b *bucket, now time.Time) {
	elapsed := now.Sub(b.lastUpdate).Seconds()
	tokensToAdd := elapsed * tb.rate

	b.tokens += tokensToAdd
	if b.tokens > float64(tb.capacity) {
		b.tokens = float64(tb.capacity)
	}

	b.lastUpdate = now
}

// Reset resets the bucket for a key
func (tb *TokenBucket) Reset(key string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	delete(tb.buckets, key)
}

// GetTokens returns the current number of tokens for a key
func (tb *TokenBucket) GetTokens(key string) float64 {
	tb.mu.RLock()
	b, ok := tb.buckets[key]
	tb.mu.RUnlock()

	if !ok {
		return float64(tb.capacity)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	tb.refill(b, time.Now())
	return b.tokens
}

// Close stops the cleanup goroutine
func (tb *TokenBucket) Close() error {
	close(tb.stopCh)
	return nil
}

// cleanup periodically removes stale entries
func (tb *TokenBucket) cleanup() {
	ticker := time.NewTicker(tb.cleanupInt)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tb.doCleanup()
		case <-tb.stopCh:
			return
		}
	}
}

// doCleanup removes entries that are at full capacity
func (tb *TokenBucket) doCleanup() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	for key, b := range tb.buckets {
		b.mu.Lock()
		// If enough time has passed to refill to capacity, remove entry
		elapsed := now.Sub(b.lastUpdate).Seconds()
		if b.tokens+elapsed*tb.rate >= float64(tb.capacity) {
			delete(tb.buckets, key)
		}
		b.mu.Unlock()
	}
}

// SetRate changes the refill rate
func (tb *TokenBucket) SetRate(rate float64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.rate = rate
}

// SetCapacity changes the bucket capacity
func (tb *TokenBucket) SetCapacity(capacity int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.capacity = capacity
}

// Stats returns statistics about the rate limiter
func (tb *TokenBucket) Stats() map[string]interface{} {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["keys"] = len(tb.buckets)
	stats["rate"] = tb.rate
	stats["capacity"] = tb.capacity

	return stats
}

// WaitTime returns the duration to wait for n tokens
func (tb *TokenBucket) WaitTime(key string, n int) time.Duration {
	b := tb.getBucket(key)

	b.mu.Lock()
	defer b.mu.Unlock()

	tb.refill(b, time.Now())

	if b.tokens >= float64(n) {
		return 0
	}

	needed := float64(n) - b.tokens
	seconds := needed / tb.rate
	return time.Duration(seconds * float64(time.Second))
}

// Reserve reserves n tokens and returns the wait time
func (tb *TokenBucket) Reserve(key string, n int) time.Duration {
	b := tb.getBucket(key)

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	tb.refill(b, now)

	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return 0
	}

	needed := float64(n) - b.tokens
	waitDuration := time.Duration(needed / tb.rate * float64(time.Second))

	// Reserve the tokens (go negative)
	b.tokens -= float64(n)

	return waitDuration
}
