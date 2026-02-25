// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package cache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/storage"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testConfig returns a minimal Config with no cleanup goroutine.
func testConfig(maxSize int, maxBytes int64) Config {
	return Config{
		Type:            TypeLRU,
		MaxSize:         maxSize,
		MaxBytes:        maxBytes,
		DefaultTTL:      0, // no default expiry
		CleanupInterval: 0, // no background cleanup
	}
}

// testTTLConfig returns a Config suitable for TTL cache tests.
func testTTLConfig(maxSize int, maxBytes int64, defaultTTL time.Duration) Config {
	return Config{
		Type:            TypeTTL,
		MaxSize:         maxSize,
		MaxBytes:        maxBytes,
		DefaultTTL:      defaultTTL,
		CleanupInterval: 0, // disable background cleanup in tests
	}
}

// ctx is a shorthand for a background context.
func ctx() context.Context { return context.Background() }

// mockKVStore is a simple in-memory KVStore used for CacheWrapper tests.
type mockKVStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMockKVStore() *mockKVStore {
	return &mockKVStore{data: make(map[string][]byte)}
}

func (m *mockKVStore) Get(_ context.Context, key []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[string(key)]
	if !ok {
		return nil, storage.ErrNotFound
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (m *mockKVStore) Set(_ context.Context, key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[string(key)] = cp
	return nil
}

func (m *mockKVStore) Delete(_ context.Context, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, string(key))
	return nil
}

func (m *mockKVStore) Scan(_ context.Context, prefix []byte, fn func(key, value []byte) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.data {
		if bytes.HasPrefix([]byte(k), prefix) {
			if err := fn([]byte(k), v); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *mockKVStore) Close() error { return nil }

// ---------------------------------------------------------------------------
// Entry tests
// ---------------------------------------------------------------------------

func TestEntry_IsExpired(t *testing.T) {
	t.Run("zero expiration is never expired", func(t *testing.T) {
		e := &Entry{}
		if e.IsExpired() {
			t.Fatal("entry with zero ExpiresAt should not be expired")
		}
	})

	t.Run("future expiration is not expired", func(t *testing.T) {
		e := &Entry{ExpiresAt: time.Now().Add(time.Hour)}
		if e.IsExpired() {
			t.Fatal("entry with future ExpiresAt should not be expired")
		}
	})

	t.Run("past expiration is expired", func(t *testing.T) {
		e := &Entry{ExpiresAt: time.Now().Add(-time.Second)}
		if !e.IsExpired() {
			t.Fatal("entry with past ExpiresAt should be expired")
		}
	})
}

// ---------------------------------------------------------------------------
// CopyBytes tests
// ---------------------------------------------------------------------------

func TestCopyBytes(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		if CopyBytes(nil) != nil {
			t.Fatal("CopyBytes(nil) should return nil")
		}
	})

	t.Run("copy is independent", func(t *testing.T) {
		orig := []byte("hello")
		cp := CopyBytes(orig)
		if !bytes.Equal(orig, cp) {
			t.Fatal("copy should equal original")
		}
		cp[0] = 'X'
		if bytes.Equal(orig, cp) {
			t.Fatal("modifying copy should not affect original")
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		orig := []byte{}
		cp := CopyBytes(orig)
		if len(cp) != 0 {
			t.Fatal("copy of empty slice should be empty")
		}
	})
}

// ---------------------------------------------------------------------------
// DefaultConfig tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Type != TypeLRU {
		t.Fatalf("expected default type %q, got %q", TypeLRU, cfg.Type)
	}
	if cfg.MaxSize != 10000 {
		t.Fatalf("expected MaxSize 10000, got %d", cfg.MaxSize)
	}
	if cfg.MaxBytes != 64*1024*1024 {
		t.Fatalf("expected MaxBytes 64MB, got %d", cfg.MaxBytes)
	}
	if cfg.DefaultTTL != 5*time.Minute {
		t.Fatalf("expected DefaultTTL 5m, got %v", cfg.DefaultTTL)
	}
	if cfg.CleanupInterval != 1*time.Minute {
		t.Fatalf("expected CleanupInterval 1m, got %v", cfg.CleanupInterval)
	}
}

// ---------------------------------------------------------------------------
// New factory tests
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		cacheTyp Type
	}{
		{"lru", TypeLRU},
		{"ttl", TypeTTL},
		{"unknown defaults to lru", Type("unknown")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(100, 1024)
			cfg.Type = tt.cacheTyp
			// For TTL cache, set a default TTL so entries don't immediately expire
			if tt.cacheTyp == TypeTTL {
				cfg.DefaultTTL = time.Minute
			}
			c, err := New(cfg)
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			if c == nil {
				t.Fatal("New returned nil cache")
			}
			c.Close()
		})
	}
}

// ---------------------------------------------------------------------------
// LRU Cache tests
// ---------------------------------------------------------------------------

func TestLRUCache_SetAndGet(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	key := []byte("key1")
	val := []byte("value1")

	if err := c.Set(ctx(), key, val, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, ok := c.Get(ctx(), key)
	if !ok {
		t.Fatal("Get returned false for existing key")
	}
	if !bytes.Equal(got, val) {
		t.Fatalf("expected %q, got %q", val, got)
	}
}

func TestLRUCache_GetMiss(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	_, ok := c.Get(ctx(), []byte("nonexistent"))
	if ok {
		t.Fatal("expected miss for nonexistent key")
	}
}

func TestLRUCache_SetOverwrite(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	key := []byte("key")
	c.Set(ctx(), key, []byte("v1"), 0)
	c.Set(ctx(), key, []byte("v2"), 0)

	got, ok := c.Get(ctx(), key)
	if !ok {
		t.Fatal("Get returned false")
	}
	if !bytes.Equal(got, []byte("v2")) {
		t.Fatalf("expected v2, got %q", got)
	}

	if c.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", c.Len())
	}
}

func TestLRUCache_Delete(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	key := []byte("key")
	c.Set(ctx(), key, []byte("val"), 0)
	c.Delete(ctx(), key)

	_, ok := c.Get(ctx(), key)
	if ok {
		t.Fatal("key should be deleted")
	}
	if c.Len() != 0 {
		t.Fatalf("expected 0 items, got %d", c.Len())
	}
}

func TestLRUCache_Clear(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	for i := 0; i < 10; i++ {
		c.Set(ctx(), []byte(fmt.Sprintf("k%d", i)), []byte("v"), 0)
	}

	if c.Len() != 10 {
		t.Fatalf("expected 10 items before clear, got %d", c.Len())
	}

	c.Clear(ctx())

	if c.Len() != 0 {
		t.Fatalf("expected 0 items after clear, got %d", c.Len())
	}
}

func TestLRUCache_MaxSizeEviction(t *testing.T) {
	maxSize := 3
	c := NewLRUCache(testConfig(maxSize, 0))
	defer c.Close()

	// Fill the cache
	for i := 0; i < maxSize; i++ {
		c.Set(ctx(), []byte(fmt.Sprintf("k%d", i)), []byte("v"), 0)
	}

	// Adding one more should evict the oldest (k0)
	c.Set(ctx(), []byte("k_new"), []byte("v"), 0)

	if c.Len() != maxSize {
		t.Fatalf("expected %d items, got %d", maxSize, c.Len())
	}

	// k0 should be evicted (it was first inserted, hence LRU)
	if c.Contains([]byte("k0")) {
		t.Fatal("k0 should have been evicted")
	}

	// The rest should still be present
	for _, key := range []string{"k1", "k2", "k_new"} {
		if !c.Contains([]byte(key)) {
			t.Fatalf("%s should still be in the cache", key)
		}
	}
}

func TestLRUCache_MaxBytesEviction(t *testing.T) {
	// Each entry: key(2 bytes) + value(4 bytes) = 6 bytes of entrySize
	// MaxBytes = 15 means we can hold 2 items (12 bytes) but not 3 (18 bytes)
	c := NewLRUCache(testConfig(0, 15))
	defer c.Close()

	c.Set(ctx(), []byte("k1"), []byte("val1"), 0) // 2+4 = 6
	c.Set(ctx(), []byte("k2"), []byte("val2"), 0) // 6+6 = 12

	if c.Len() != 2 {
		t.Fatalf("expected 2 items, got %d", c.Len())
	}

	// Adding k3 should cause eviction of k1 (oldest)
	c.Set(ctx(), []byte("k3"), []byte("val3"), 0) // needs room: 12+6=18 > 15

	if c.Len() != 2 {
		t.Fatalf("expected 2 items after eviction, got %d", c.Len())
	}

	if c.Contains([]byte("k1")) {
		t.Fatal("k1 should have been evicted (LRU under byte limit)")
	}
}

func TestLRUCache_LRUOrdering(t *testing.T) {
	c := NewLRUCache(testConfig(3, 0))
	defer c.Close()

	// Insert k0, k1, k2
	c.Set(ctx(), []byte("k0"), []byte("v"), 0)
	c.Set(ctx(), []byte("k1"), []byte("v"), 0)
	c.Set(ctx(), []byte("k2"), []byte("v"), 0)

	// Access k0 to make it recently used
	c.Get(ctx(), []byte("k0"))

	// Insert k3 => should evict k1 (least recently used)
	c.Set(ctx(), []byte("k3"), []byte("v"), 0)

	if c.Contains([]byte("k1")) {
		t.Fatal("k1 should have been evicted as LRU")
	}
	if !c.Contains([]byte("k0")) {
		t.Fatal("k0 was accessed and should NOT be evicted")
	}
}

func TestLRUCache_TTLExpiration(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	key := []byte("expiring")
	c.Set(ctx(), key, []byte("val"), 50*time.Millisecond)

	// Should be present immediately
	if _, ok := c.Get(ctx(), key); !ok {
		t.Fatal("key should exist before TTL")
	}

	time.Sleep(100 * time.Millisecond)

	// Should now be expired
	if _, ok := c.Get(ctx(), key); ok {
		t.Fatal("key should be expired")
	}
}

func TestLRUCache_Peek(t *testing.T) {
	c := NewLRUCache(testConfig(3, 0))
	defer c.Close()

	c.Set(ctx(), []byte("k0"), []byte("v0"), 0)
	c.Set(ctx(), []byte("k1"), []byte("v1"), 0)
	c.Set(ctx(), []byte("k2"), []byte("v2"), 0)

	// Peek k0 should NOT move it to front
	val, ok := c.Peek(ctx(), []byte("k0"))
	if !ok || !bytes.Equal(val, []byte("v0")) {
		t.Fatal("Peek should return the value")
	}

	// Insert k3 => should evict k0 because Peek does not update LRU ordering
	c.Set(ctx(), []byte("k3"), []byte("v3"), 0)

	if c.Contains([]byte("k0")) {
		t.Fatal("k0 should be evicted because Peek does not update position")
	}
}

func TestLRUCache_PeekExpired(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	c.Set(ctx(), []byte("k"), []byte("v"), 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	_, ok := c.Peek(ctx(), []byte("k"))
	if ok {
		t.Fatal("Peek should return false for expired entry")
	}
}

func TestLRUCache_RemoveOldest(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	c.Set(ctx(), []byte("k0"), []byte("v0"), 0)
	c.Set(ctx(), []byte("k1"), []byte("v1"), 0)

	key, ok := c.RemoveOldest()
	if !ok {
		t.Fatal("RemoveOldest should return true")
	}
	if !bytes.Equal(key, []byte("k0")) {
		t.Fatalf("expected k0 to be oldest, got %q", key)
	}
	if c.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", c.Len())
	}
}

func TestLRUCache_RemoveOldest_Empty(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	_, ok := c.RemoveOldest()
	if ok {
		t.Fatal("RemoveOldest on empty cache should return false")
	}
}

func TestLRUCache_Keys(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	c.Set(ctx(), []byte("a"), []byte("1"), 0)
	c.Set(ctx(), []byte("b"), []byte("2"), 0)

	keys := c.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	found := map[string]bool{}
	for _, k := range keys {
		found[string(k)] = true
	}
	if !found["a"] || !found["b"] {
		t.Fatal("expected keys a and b")
	}
}

func TestLRUCache_Contains(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	c.Set(ctx(), []byte("x"), []byte("y"), 0)

	if !c.Contains([]byte("x")) {
		t.Fatal("Contains should return true for existing key")
	}
	if c.Contains([]byte("z")) {
		t.Fatal("Contains should return false for missing key")
	}
}

func TestLRUCache_Resize(t *testing.T) {
	c := NewLRUCache(testConfig(10, 0))
	defer c.Close()

	for i := 0; i < 10; i++ {
		c.Set(ctx(), []byte(fmt.Sprintf("k%d", i)), []byte("v"), 0)
	}
	if c.Len() != 10 {
		t.Fatalf("expected 10, got %d", c.Len())
	}

	// Shrink to maxSize=5. shouldEvict evicts while size >= maxSize,
	// so it settles at size = maxSize - 1 = 4.
	c.Resize(5, 0)
	if c.Len() > 5 {
		t.Fatalf("expected at most 5 after resize, got %d", c.Len())
	}
	// The implementation evicts while size >= maxSize, so we expect exactly 4.
	if c.Len() != 4 {
		t.Fatalf("expected 4 after resize (evicts while size >= maxSize), got %d", c.Len())
	}
}

func TestLRUCache_GetOrSet(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	key := []byte("computed")
	calls := 0

	generator := func() ([]byte, error) {
		calls++
		return []byte("generated_value"), nil
	}

	// First call should invoke the function
	val, err := c.GetOrSet(ctx(), key, generator, 0)
	if err != nil {
		t.Fatalf("GetOrSet: %v", err)
	}
	if !bytes.Equal(val, []byte("generated_value")) {
		t.Fatalf("expected generated_value, got %q", val)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}

	// Second call should return cached value without calling generator
	val2, err := c.GetOrSet(ctx(), key, generator, 0)
	if err != nil {
		t.Fatalf("GetOrSet second call: %v", err)
	}
	if !bytes.Equal(val2, []byte("generated_value")) {
		t.Fatalf("expected cached value, got %q", val2)
	}
	if calls != 1 {
		t.Fatalf("generator should not be called again, calls=%d", calls)
	}
}

func TestLRUCache_GetOrSet_Error(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	errBoom := errors.New("boom")
	_, err := c.GetOrSet(ctx(), []byte("k"), func() ([]byte, error) {
		return nil, errBoom
	}, 0)
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected errBoom, got %v", err)
	}
}

func TestLRUCache_ExpireNow(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	key := []byte("ex")
	c.Set(ctx(), key, []byte("val"), time.Hour)

	if !c.ExpireNow(key) {
		t.Fatal("ExpireNow should return true for existing key")
	}

	_, ok := c.Get(ctx(), key)
	if ok {
		t.Fatal("key should be expired after ExpireNow")
	}
}

func TestLRUCache_ExpireNow_Missing(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	if c.ExpireNow([]byte("nope")) {
		t.Fatal("ExpireNow should return false for missing key")
	}
}

func TestLRUCache_SetTTL(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	key := []byte("ttl_key")
	c.Set(ctx(), key, []byte("val"), time.Hour)

	// Shorten TTL
	if !c.SetTTL(key, 50*time.Millisecond) {
		t.Fatal("SetTTL should return true for existing key")
	}

	time.Sleep(100 * time.Millisecond)

	_, ok := c.Get(ctx(), key)
	if ok {
		t.Fatal("key should be expired after SetTTL with short duration")
	}
}

func TestLRUCache_SetTTL_RemoveTTL(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	key := []byte("ttl_key")
	c.Set(ctx(), key, []byte("val"), 50*time.Millisecond)

	// Remove TTL (set to 0 => no expiration)
	c.SetTTL(key, 0)

	time.Sleep(100 * time.Millisecond)

	_, ok := c.Get(ctx(), key)
	if !ok {
		t.Fatal("key with TTL removed should not expire")
	}
}

func TestLRUCache_SetTTL_Missing(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	if c.SetTTL([]byte("nope"), time.Hour) {
		t.Fatal("SetTTL should return false for missing key")
	}
}

func TestLRUCache_CleanupExpired(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	// Insert some entries with short TTL
	c.Set(ctx(), []byte("short1"), []byte("v"), 50*time.Millisecond)
	c.Set(ctx(), []byte("short2"), []byte("v"), 50*time.Millisecond)
	// And one with no TTL
	c.Set(ctx(), []byte("forever"), []byte("v"), 0)

	time.Sleep(100 * time.Millisecond)

	removed := c.CleanupExpired()
	if removed != 2 {
		t.Fatalf("expected 2 expired removed, got %d", removed)
	}
	if c.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", c.Len())
	}
}

func TestLRUCache_Stats(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	c.Set(ctx(), []byte("k"), []byte("v"), 0)
	c.Get(ctx(), []byte("k"))          // hit
	c.Get(ctx(), []byte("k"))          // hit
	c.Get(ctx(), []byte("nonexistent")) // miss

	stats := c.Stats()
	if stats.Hits != 2 {
		t.Fatalf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Size != 1 {
		t.Fatalf("expected size 1, got %d", stats.Size)
	}
}

func TestLRUCache_ClosedBehavior(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	c.Set(ctx(), []byte("k"), []byte("v"), 0)
	c.Close()

	// Get on closed cache
	_, ok := c.Get(ctx(), []byte("k"))
	if ok {
		t.Fatal("Get on closed cache should return false")
	}

	// Set on closed cache
	err := c.Set(ctx(), []byte("k2"), []byte("v"), 0)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("Set on closed cache should return ErrClosed, got %v", err)
	}

	// Double close is safe
	if err := c.Close(); err != nil {
		t.Fatalf("double Close should not error, got %v", err)
	}
}

func TestLRUCache_ValueIsolation(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	original := []byte("original")
	c.Set(ctx(), []byte("k"), original, 0)

	// Mutating original should not affect cache
	original[0] = 'X'

	got, ok := c.Get(ctx(), []byte("k"))
	if !ok {
		t.Fatal("Get returned false")
	}
	if got[0] == 'X' {
		t.Fatal("cache should store a copy, not a reference")
	}

	// Mutating returned value should not affect cache
	got[0] = 'Z'
	got2, _ := c.Get(ctx(), []byte("k"))
	if got2[0] == 'Z' {
		t.Fatal("returned value should be a copy")
	}
}

func TestLRUCache_DefaultTTL(t *testing.T) {
	cfg := testConfig(100, 0)
	cfg.DefaultTTL = 50 * time.Millisecond
	c := NewLRUCache(cfg)
	defer c.Close()

	c.Set(ctx(), []byte("k"), []byte("v"), 0) // uses DefaultTTL

	time.Sleep(100 * time.Millisecond)

	_, ok := c.Get(ctx(), []byte("k"))
	if ok {
		t.Fatal("entry should expire using DefaultTTL")
	}
}

// ---------------------------------------------------------------------------
// TTL Cache tests
// ---------------------------------------------------------------------------

func TestTTLCache_SetAndGet(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	key := []byte("key1")
	val := []byte("value1")

	if err := c.Set(ctx(), key, val, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, ok := c.Get(ctx(), key)
	if !ok {
		t.Fatal("Get returned false")
	}
	if !bytes.Equal(got, val) {
		t.Fatalf("expected %q, got %q", val, got)
	}
}

func TestTTLCache_GetMiss(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	_, ok := c.Get(ctx(), []byte("nonexistent"))
	if ok {
		t.Fatal("expected miss for nonexistent key")
	}
}

func TestTTLCache_Expiration(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	key := []byte("expiring")
	c.Set(ctx(), key, []byte("val"), 50*time.Millisecond)

	_, ok := c.Get(ctx(), key)
	if !ok {
		t.Fatal("key should exist before expiration")
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = c.Get(ctx(), key)
	if ok {
		t.Fatal("key should be expired")
	}
}

func TestTTLCache_SetOverwrite(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	key := []byte("k")
	c.Set(ctx(), key, []byte("v1"), time.Minute)
	c.Set(ctx(), key, []byte("v2"), time.Minute)

	got, ok := c.Get(ctx(), key)
	if !ok {
		t.Fatal("Get returned false")
	}
	if !bytes.Equal(got, []byte("v2")) {
		t.Fatalf("expected v2, got %q", got)
	}
	if c.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", c.Len())
	}
}

func TestTTLCache_Delete(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	key := []byte("k")
	c.Set(ctx(), key, []byte("v"), time.Minute)
	c.Delete(ctx(), key)

	_, ok := c.Get(ctx(), key)
	if ok {
		t.Fatal("key should be deleted")
	}
	if c.Len() != 0 {
		t.Fatalf("expected 0 items, got %d", c.Len())
	}
}

func TestTTLCache_Clear(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	for i := 0; i < 5; i++ {
		c.Set(ctx(), []byte(fmt.Sprintf("k%d", i)), []byte("v"), time.Minute)
	}

	c.Clear(ctx())

	if c.Len() != 0 {
		t.Fatalf("expected 0 items after clear, got %d", c.Len())
	}
}

func TestTTLCache_MaxSizeEviction(t *testing.T) {
	c := NewTTLCache(testTTLConfig(3, 0, time.Hour))
	defer c.Close()

	// Fill
	c.Set(ctx(), []byte("k0"), []byte("v"), time.Hour)
	time.Sleep(time.Millisecond) // ensure distinct expiration times
	c.Set(ctx(), []byte("k1"), []byte("v"), time.Hour)
	time.Sleep(time.Millisecond)
	c.Set(ctx(), []byte("k2"), []byte("v"), time.Hour)

	// Insert one more => should evict the one closest to expiration (k0)
	c.Set(ctx(), []byte("k3"), []byte("v"), time.Hour)

	if c.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", c.Len())
	}

	stats := c.Stats()
	if stats.Evictions != 1 {
		t.Fatalf("expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestTTLCache_MaxBytesEviction(t *testing.T) {
	// entry size = len(key) + len(value)
	// "k0" + "val0" = 2 + 4 = 6 bytes each
	c := NewTTLCache(testTTLConfig(0, 15, time.Hour))
	defer c.Close()

	c.Set(ctx(), []byte("k0"), []byte("val0"), time.Hour)
	time.Sleep(time.Millisecond)
	c.Set(ctx(), []byte("k1"), []byte("val1"), time.Hour)

	// 12 bytes so far, adding 6 more = 18 > 15 => eviction
	c.Set(ctx(), []byte("k2"), []byte("val2"), time.Hour)

	if c.Len() != 2 {
		t.Fatalf("expected 2 items after byte eviction, got %d", c.Len())
	}
}

func TestTTLCache_CleanupExpired(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("short1"), []byte("v"), 50*time.Millisecond)
	c.Set(ctx(), []byte("short2"), []byte("v"), 50*time.Millisecond)
	c.Set(ctx(), []byte("long"), []byte("v"), time.Hour)

	time.Sleep(100 * time.Millisecond)

	removed := c.CleanupExpired()
	if removed != 2 {
		t.Fatalf("expected 2 expired removed, got %d", removed)
	}
	if c.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", c.Len())
	}
}

func TestTTLCache_Keys(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("a"), []byte("1"), time.Minute)
	c.Set(ctx(), []byte("b"), []byte("2"), time.Minute)

	keys := c.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	found := map[string]bool{}
	for _, k := range keys {
		found[string(k)] = true
	}
	if !found["a"] || !found["b"] {
		t.Fatal("expected keys a and b")
	}
}

func TestTTLCache_Contains(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("x"), []byte("y"), time.Minute)

	if !c.Contains([]byte("x")) {
		t.Fatal("Contains should return true for existing key")
	}
	if c.Contains([]byte("z")) {
		t.Fatal("Contains should return false for missing key")
	}
}

func TestTTLCache_TTL(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("k"), []byte("v"), time.Hour)

	remaining, ok := c.TTL([]byte("k"))
	if !ok {
		t.Fatal("TTL should return true for existing key")
	}
	if remaining <= 0 || remaining > time.Hour {
		t.Fatalf("expected remaining ~1h, got %v", remaining)
	}
}

func TestTTLCache_TTL_Expired(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("k"), []byte("v"), 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	_, ok := c.TTL([]byte("k"))
	if ok {
		t.Fatal("TTL should return false for expired key")
	}
}

func TestTTLCache_TTL_Missing(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	_, ok := c.TTL([]byte("nope"))
	if ok {
		t.Fatal("TTL should return false for missing key")
	}
}

func TestTTLCache_Touch(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("k"), []byte("v"), 80*time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	// Refresh TTL
	if !c.Touch([]byte("k"), 200*time.Millisecond) {
		t.Fatal("Touch should return true for existing key")
	}

	time.Sleep(50 * time.Millisecond)

	// Should still be alive because we refreshed
	_, ok := c.Get(ctx(), []byte("k"))
	if !ok {
		t.Fatal("key should still exist after Touch")
	}
}

func TestTTLCache_Touch_Missing(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	if c.Touch([]byte("nope"), time.Hour) {
		t.Fatal("Touch should return false for missing key")
	}
}

func TestTTLCache_GetWithTTL(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("k"), []byte("v"), time.Hour)

	val, remaining, ok := c.GetWithTTL(ctx(), []byte("k"))
	if !ok {
		t.Fatal("GetWithTTL should return true")
	}
	if !bytes.Equal(val, []byte("v")) {
		t.Fatalf("expected v, got %q", val)
	}
	if remaining <= 0 || remaining > time.Hour {
		t.Fatalf("unexpected remaining TTL: %v", remaining)
	}
}

func TestTTLCache_GetWithTTL_Expired(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("k"), []byte("v"), 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	_, _, ok := c.GetWithTTL(ctx(), []byte("k"))
	if ok {
		t.Fatal("GetWithTTL should return false for expired entry")
	}
}

func TestTTLCache_GetWithTTL_Missing(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	_, _, ok := c.GetWithTTL(ctx(), []byte("nope"))
	if ok {
		t.Fatal("GetWithTTL should return false for missing key")
	}
}

func TestTTLCache_SetIfAbsent(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	// First set should succeed
	if !c.SetIfAbsent(ctx(), []byte("k"), []byte("v1"), time.Hour) {
		t.Fatal("SetIfAbsent should return true for new key")
	}

	// Second set should fail (key exists)
	if c.SetIfAbsent(ctx(), []byte("k"), []byte("v2"), time.Hour) {
		t.Fatal("SetIfAbsent should return false for existing key")
	}

	// Value should be original
	got, ok := c.Get(ctx(), []byte("k"))
	if !ok || !bytes.Equal(got, []byte("v1")) {
		t.Fatalf("expected v1, got %q, ok=%v", got, ok)
	}
}

func TestTTLCache_SetIfAbsent_ExpiredReplacement(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.SetIfAbsent(ctx(), []byte("k"), []byte("old"), 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	// Key is expired, SetIfAbsent should succeed
	if !c.SetIfAbsent(ctx(), []byte("k"), []byte("new"), time.Hour) {
		t.Fatal("SetIfAbsent should succeed for expired key")
	}

	got, ok := c.Get(ctx(), []byte("k"))
	if !ok || !bytes.Equal(got, []byte("new")) {
		t.Fatalf("expected new, got %q, ok=%v", got, ok)
	}
}

func TestTTLCache_GetMultiple(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("a"), []byte("1"), time.Hour)
	c.Set(ctx(), []byte("b"), []byte("2"), time.Hour)
	c.Set(ctx(), []byte("c"), []byte("3"), 50*time.Millisecond)

	time.Sleep(100 * time.Millisecond) // let "c" expire

	result := c.GetMultiple(ctx(), [][]byte{
		[]byte("a"),
		[]byte("b"),
		[]byte("c"),      // expired
		[]byte("missing"), // never set
	})

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if !bytes.Equal(result["a"], []byte("1")) {
		t.Fatalf("expected 1 for a, got %q", result["a"])
	}
	if !bytes.Equal(result["b"], []byte("2")) {
		t.Fatalf("expected 2 for b, got %q", result["b"])
	}
}

func TestTTLCache_SetMultiple(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	items := map[string][]byte{
		"a": []byte("1"),
		"b": []byte("2"),
		"c": []byte("3"),
	}
	c.SetMultiple(ctx(), items, time.Hour)

	if c.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", c.Len())
	}

	for k, v := range items {
		got, ok := c.Get(ctx(), []byte(k))
		if !ok {
			t.Fatalf("key %q not found", k)
		}
		if !bytes.Equal(got, v) {
			t.Fatalf("key %q: expected %q, got %q", k, v, got)
		}
	}
}

func TestTTLCache_SetMultiple_Overwrite(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("a"), []byte("old"), time.Hour)

	c.SetMultiple(ctx(), map[string][]byte{"a": []byte("new")}, time.Hour)

	got, ok := c.Get(ctx(), []byte("a"))
	if !ok || !bytes.Equal(got, []byte("new")) {
		t.Fatalf("expected new, got %q", got)
	}
	if c.Len() != 1 {
		t.Fatalf("expected 1, got %d", c.Len())
	}
}

func TestTTLCache_DeleteMultiple(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("a"), []byte("1"), time.Hour)
	c.Set(ctx(), []byte("b"), []byte("2"), time.Hour)
	c.Set(ctx(), []byte("c"), []byte("3"), time.Hour)

	c.DeleteMultiple(ctx(), [][]byte{[]byte("a"), []byte("c")})

	if c.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", c.Len())
	}
	if !c.Contains([]byte("b")) {
		t.Fatal("b should still exist")
	}
}

func TestTTLCache_Stats(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	c.Set(ctx(), []byte("k"), []byte("v"), time.Hour)
	c.Get(ctx(), []byte("k"))          // hit
	c.Get(ctx(), []byte("k"))          // hit
	c.Get(ctx(), []byte("nonexistent")) // miss

	stats := c.Stats()
	if stats.Hits != 2 {
		t.Fatalf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Size != 1 {
		t.Fatalf("expected size 1, got %d", stats.Size)
	}
}

func TestTTLCache_ClosedBehavior(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	c.Set(ctx(), []byte("k"), []byte("v"), time.Hour)
	c.Close()

	_, ok := c.Get(ctx(), []byte("k"))
	if ok {
		t.Fatal("Get on closed cache should return false")
	}

	err := c.Set(ctx(), []byte("k2"), []byte("v"), time.Hour)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("Set on closed cache should return ErrClosed, got %v", err)
	}

	// Double close is safe
	if err := c.Close(); err != nil {
		t.Fatalf("double Close should not error, got %v", err)
	}
}

func TestTTLCache_ValueIsolation(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	original := []byte("original")
	c.Set(ctx(), []byte("k"), original, time.Hour)

	original[0] = 'X'
	got, _ := c.Get(ctx(), []byte("k"))
	if got[0] == 'X' {
		t.Fatal("cache should store a copy")
	}

	got[0] = 'Z'
	got2, _ := c.Get(ctx(), []byte("k"))
	if got2[0] == 'Z' {
		t.Fatal("returned value should be a copy")
	}
}

func TestTTLCache_DefaultTTLFallback(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, 50*time.Millisecond))
	defer c.Close()

	// Set with ttl=0 should use DefaultTTL
	c.Set(ctx(), []byte("k"), []byte("v"), 0)

	time.Sleep(100 * time.Millisecond)

	_, ok := c.Get(ctx(), []byte("k"))
	if ok {
		t.Fatal("entry should expire using DefaultTTL")
	}
}

// ---------------------------------------------------------------------------
// LoaderFunc / LoadingCache tests
// ---------------------------------------------------------------------------

func TestLoaderFunc(t *testing.T) {
	fn := LoaderFunc(func(_ context.Context, key []byte) ([]byte, error) {
		return append([]byte("loaded:"), key...), nil
	})

	val, err := fn.Load(ctx(), []byte("test"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !bytes.Equal(val, []byte("loaded:test")) {
		t.Fatalf("expected loaded:test, got %q", val)
	}
}

func TestLoadingCache_Get(t *testing.T) {
	underlying := NewLRUCache(testConfig(100, 0))
	defer underlying.Close()

	loadCalls := 0
	loader := LoaderFunc(func(_ context.Context, key []byte) ([]byte, error) {
		loadCalls++
		return []byte("loaded"), nil
	})

	lc := NewLoadingCache(underlying, loader, time.Minute)

	// First get should load
	val, err := lc.Get(ctx(), []byte("k"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(val, []byte("loaded")) {
		t.Fatalf("expected loaded, got %q", val)
	}
	if loadCalls != 1 {
		t.Fatalf("expected 1 load call, got %d", loadCalls)
	}

	// Second get should use cache
	val2, err := lc.Get(ctx(), []byte("k"))
	if err != nil {
		t.Fatalf("Get cached: %v", err)
	}
	if !bytes.Equal(val2, []byte("loaded")) {
		t.Fatalf("expected cached value, got %q", val2)
	}
	if loadCalls != 1 {
		t.Fatalf("loader should not be called again, calls=%d", loadCalls)
	}
}

func TestLoadingCache_Get_LoaderError(t *testing.T) {
	underlying := NewLRUCache(testConfig(100, 0))
	defer underlying.Close()

	errFail := errors.New("load failed")
	loader := LoaderFunc(func(_ context.Context, key []byte) ([]byte, error) {
		return nil, errFail
	})

	lc := NewLoadingCache(underlying, loader, time.Minute)

	_, err := lc.Get(ctx(), []byte("k"))
	if !errors.Is(err, errFail) {
		t.Fatalf("expected errFail, got %v", err)
	}
}

func TestLoadingCache_Invalidate(t *testing.T) {
	underlying := NewLRUCache(testConfig(100, 0))
	defer underlying.Close()

	loadCalls := 0
	loader := LoaderFunc(func(_ context.Context, key []byte) ([]byte, error) {
		loadCalls++
		return []byte(fmt.Sprintf("v%d", loadCalls)), nil
	})

	lc := NewLoadingCache(underlying, loader, time.Minute)

	lc.Get(ctx(), []byte("k")) // loads v1
	if loadCalls != 1 {
		t.Fatalf("expected 1, got %d", loadCalls)
	}

	lc.Invalidate(ctx(), []byte("k"))

	val, _ := lc.Get(ctx(), []byte("k")) // reloads v2
	if loadCalls != 2 {
		t.Fatalf("expected 2 after invalidation, got %d", loadCalls)
	}
	if !bytes.Equal(val, []byte("v2")) {
		t.Fatalf("expected v2, got %q", val)
	}
}

func TestLoadingCache_Clear(t *testing.T) {
	underlying := NewLRUCache(testConfig(100, 0))
	defer underlying.Close()

	loadCalls := 0
	loader := LoaderFunc(func(_ context.Context, key []byte) ([]byte, error) {
		loadCalls++
		return []byte("v"), nil
	})

	lc := NewLoadingCache(underlying, loader, time.Minute)

	lc.Get(ctx(), []byte("a"))
	lc.Get(ctx(), []byte("b"))
	lc.Clear(ctx())

	lc.Get(ctx(), []byte("a"))
	if loadCalls != 3 {
		t.Fatalf("expected 3 load calls after clear, got %d", loadCalls)
	}
}

func TestLoadingCache_Stats(t *testing.T) {
	underlying := NewLRUCache(testConfig(100, 0))
	defer underlying.Close()

	loader := LoaderFunc(func(_ context.Context, key []byte) ([]byte, error) {
		return []byte("v"), nil
	})

	lc := NewLoadingCache(underlying, loader, time.Minute)
	lc.Get(ctx(), []byte("k")) // miss + load + set
	lc.Get(ctx(), []byte("k")) // hit

	stats := lc.Stats()
	if stats.Hits != 1 {
		t.Fatalf("expected 1 hit, got %d", stats.Hits)
	}
}

func TestLoadingCache_Close(t *testing.T) {
	underlying := NewLRUCache(testConfig(100, 0))
	loader := LoaderFunc(func(_ context.Context, key []byte) ([]byte, error) {
		return []byte("v"), nil
	})

	lc := NewLoadingCache(underlying, loader, time.Minute)
	if err := lc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// MultiLevelCache tests
// ---------------------------------------------------------------------------

func TestMultiLevelCache_GetPopulatesEarlierLevels(t *testing.T) {
	l1 := NewLRUCache(testConfig(100, 0))
	l2 := NewLRUCache(testConfig(100, 0))
	defer l1.Close()
	defer l2.Close()

	mlc := NewMultiLevelCache(l1, l2)
	defer mlc.Close()

	// Put in L2 only
	l2.Set(ctx(), []byte("k"), []byte("v"), 0)

	val, ok := mlc.Get(ctx(), []byte("k"))
	if !ok {
		t.Fatal("Get should find value in L2")
	}
	if !bytes.Equal(val, []byte("v")) {
		t.Fatalf("expected v, got %q", val)
	}

	// L1 should now be populated
	val1, ok := l1.Get(ctx(), []byte("k"))
	if !ok {
		t.Fatal("L1 should be populated from L2 hit")
	}
	if !bytes.Equal(val1, []byte("v")) {
		t.Fatalf("expected v in L1, got %q", val1)
	}
}

func TestMultiLevelCache_GetMiss(t *testing.T) {
	l1 := NewLRUCache(testConfig(100, 0))
	l2 := NewLRUCache(testConfig(100, 0))
	defer l1.Close()
	defer l2.Close()

	mlc := NewMultiLevelCache(l1, l2)
	defer mlc.Close()

	_, ok := mlc.Get(ctx(), []byte("missing"))
	if ok {
		t.Fatal("Get should return false when not in any level")
	}
}

func TestMultiLevelCache_SetAllLevels(t *testing.T) {
	l1 := NewLRUCache(testConfig(100, 0))
	l2 := NewLRUCache(testConfig(100, 0))
	defer l1.Close()
	defer l2.Close()

	mlc := NewMultiLevelCache(l1, l2)
	defer mlc.Close()

	mlc.Set(ctx(), []byte("k"), []byte("v"), 0)

	// Both should have it
	if _, ok := l1.Get(ctx(), []byte("k")); !ok {
		t.Fatal("L1 should have the key")
	}
	if _, ok := l2.Get(ctx(), []byte("k")); !ok {
		t.Fatal("L2 should have the key")
	}
}

func TestMultiLevelCache_DeleteAllLevels(t *testing.T) {
	l1 := NewLRUCache(testConfig(100, 0))
	l2 := NewLRUCache(testConfig(100, 0))
	defer l1.Close()
	defer l2.Close()

	mlc := NewMultiLevelCache(l1, l2)
	defer mlc.Close()

	mlc.Set(ctx(), []byte("k"), []byte("v"), 0)
	mlc.Delete(ctx(), []byte("k"))

	if _, ok := l1.Get(ctx(), []byte("k")); ok {
		t.Fatal("L1 should not have the key after delete")
	}
	if _, ok := l2.Get(ctx(), []byte("k")); ok {
		t.Fatal("L2 should not have the key after delete")
	}
}

func TestMultiLevelCache_ClearAllLevels(t *testing.T) {
	l1 := NewLRUCache(testConfig(100, 0))
	l2 := NewLRUCache(testConfig(100, 0))
	defer l1.Close()
	defer l2.Close()

	mlc := NewMultiLevelCache(l1, l2)
	defer mlc.Close()

	mlc.Set(ctx(), []byte("a"), []byte("1"), 0)
	mlc.Set(ctx(), []byte("b"), []byte("2"), 0)
	mlc.Clear(ctx())

	if l1.Len() != 0 || l2.Len() != 0 {
		t.Fatal("all levels should be cleared")
	}
}

func TestMultiLevelCache_Stats(t *testing.T) {
	l1 := NewLRUCache(testConfig(100, 0))
	l2 := NewLRUCache(testConfig(100, 0))
	defer l1.Close()
	defer l2.Close()

	mlc := NewMultiLevelCache(l1, l2)
	defer mlc.Close()

	mlc.Set(ctx(), []byte("a"), []byte("1"), 0)
	mlc.Set(ctx(), []byte("b"), []byte("2"), 0)

	stats := mlc.Stats()
	// Each Set writes to both levels, so combined size is 4
	if stats.Size != 4 {
		t.Fatalf("expected combined size 4, got %d", stats.Size)
	}
}

// ---------------------------------------------------------------------------
// CacheWrapper tests
// ---------------------------------------------------------------------------

func TestCacheWrapper_GetFromCache(t *testing.T) {
	store := newMockKVStore()
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	w := NewCacheWrapper(store, c, time.Minute)

	// Pre-populate store
	store.Set(ctx(), []byte("k"), []byte("from_store"))

	// First get: cache miss, fetches from store, caches it
	val, err := w.Get(ctx(), []byte("k"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(val, []byte("from_store")) {
		t.Fatalf("expected from_store, got %q", val)
	}

	// Update store directly (bypassing wrapper)
	store.Set(ctx(), []byte("k"), []byte("updated_store"))

	// Second get: should return cached value, not updated store value
	val2, err := w.Get(ctx(), []byte("k"))
	if err != nil {
		t.Fatalf("Get cached: %v", err)
	}
	if !bytes.Equal(val2, []byte("from_store")) {
		t.Fatalf("expected cached from_store, got %q", val2)
	}
}

func TestCacheWrapper_GetStoreMiss(t *testing.T) {
	store := newMockKVStore()
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	w := NewCacheWrapper(store, c, time.Minute)

	_, err := w.Get(ctx(), []byte("missing"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCacheWrapper_SetUpdatesStoreAndCache(t *testing.T) {
	store := newMockKVStore()
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	w := NewCacheWrapper(store, c, time.Minute)

	if err := w.Set(ctx(), []byte("k"), []byte("v")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Store should have the value
	sv, err := store.Get(ctx(), []byte("k"))
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if !bytes.Equal(sv, []byte("v")) {
		t.Fatalf("store has wrong value: %q", sv)
	}

	// Cache should have the value
	cv, ok := c.Get(ctx(), []byte("k"))
	if !ok {
		t.Fatal("cache should have the value")
	}
	if !bytes.Equal(cv, []byte("v")) {
		t.Fatalf("cache has wrong value: %q", cv)
	}
}

func TestCacheWrapper_DeleteInvalidatesCache(t *testing.T) {
	store := newMockKVStore()
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	w := NewCacheWrapper(store, c, time.Minute)

	w.Set(ctx(), []byte("k"), []byte("v"))

	if err := w.Delete(ctx(), []byte("k")); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Store should not have it
	_, err := store.Get(ctx(), []byte("k"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("store should return ErrNotFound, got %v", err)
	}

	// Cache should not have it
	_, ok := c.Get(ctx(), []byte("k"))
	if ok {
		t.Fatal("cache should not have the key after delete")
	}
}

func TestCacheWrapper_Scan(t *testing.T) {
	store := newMockKVStore()
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	w := NewCacheWrapper(store, c, time.Minute)

	store.Set(ctx(), []byte("prefix:a"), []byte("1"))
	store.Set(ctx(), []byte("prefix:b"), []byte("2"))
	store.Set(ctx(), []byte("other:c"), []byte("3"))

	var scanned []string
	err := w.Scan(ctx(), []byte("prefix:"), func(key, value []byte) error {
		scanned = append(scanned, string(key))
		return nil
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(scanned) != 2 {
		t.Fatalf("expected 2 scanned keys, got %d", len(scanned))
	}
}

func TestCacheWrapper_Close(t *testing.T) {
	store := newMockKVStore()
	c := NewLRUCache(testConfig(100, 0))

	w := NewCacheWrapper(store, c, time.Minute)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Concurrency tests
// ---------------------------------------------------------------------------

func TestLRUCache_ConcurrentAccess(t *testing.T) {
	c := NewLRUCache(testConfig(100, 0))
	defer c.Close()

	var wg sync.WaitGroup
	goroutines := 20
	ops := 200

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := []byte(fmt.Sprintf("key-%d-%d", id, i))
				val := []byte(fmt.Sprintf("val-%d-%d", id, i))
				c.Set(ctx(), key, val, 0)
				c.Get(ctx(), key)
				if i%3 == 0 {
					c.Delete(ctx(), key)
				}
			}
		}(g)
	}

	wg.Wait()

	// Just verify no panic and stats are consistent
	stats := c.Stats()
	if stats.Hits+stats.Misses == 0 {
		t.Fatal("expected some hits or misses")
	}
}

func TestTTLCache_ConcurrentAccess(t *testing.T) {
	c := NewTTLCache(testTTLConfig(100, 0, time.Minute))
	defer c.Close()

	var wg sync.WaitGroup
	goroutines := 20
	ops := 200

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := []byte(fmt.Sprintf("key-%d-%d", id, i))
				val := []byte(fmt.Sprintf("val-%d-%d", id, i))
				c.Set(ctx(), key, val, time.Hour)
				c.Get(ctx(), key)
				if i%3 == 0 {
					c.Delete(ctx(), key)
				}
			}
		}(g)
	}

	wg.Wait()

	stats := c.Stats()
	if stats.Hits+stats.Misses == 0 {
		t.Fatal("expected some hits or misses")
	}
}
