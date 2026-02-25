package cache

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/storage"
)

// TTLCache is a Time To Live based cache implementation.
type TTLCache struct {
	cfg           Config
	log           *logger.Logger
	mu            sync.RWMutex
	items         map[string]*ttlEntry
	size          int64
	bytes         int64
	hits          int64
	misses        int64
	evictions     int64
	closed        bool
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// ttlEntry is an entry in the TTL cache.
type ttlEntry struct {
	key        string
	value      []byte
	size       int64
	expiresAt  time.Time
	accessedAt time.Time
	createdAt  time.Time
}

// NewTTLCache creates a new TTL cache.
func NewTTLCache(cfg Config) *TTLCache {
	log := cfg.Logger
	if log == nil {
		log = logger.New(os.Stdout)
	}

	cache := &TTLCache{
		cfg:         cfg,
		log:         log.With().Str("cache", "ttl").Logger(),
		items:       make(map[string]*ttlEntry),
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	if cfg.CleanupInterval > 0 {
		cache.cleanupTicker = time.NewTicker(cfg.CleanupInterval)
		go cache.cleanupLoop()
	}

	cache.log.Info().
		Int("max_size", cfg.MaxSize).
		Int64("max_bytes", cfg.MaxBytes).
		Dur("default_ttl", cfg.DefaultTTL).
		Dur("cleanup_interval", cfg.CleanupInterval).
		Msg("ttl cache initialized")

	return cache
}

// cleanupLoop periodically removes expired entries.
func (c *TTLCache) cleanupLoop() {
	for {
		select {
		case <-c.cleanupTicker.C:
			c.CleanupExpired()
		case <-c.stopCleanup:
			return
		}
	}
}

// Get retrieves a cached value.
func (c *TTLCache) Get(ctx context.Context, key []byte) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, false
	}

	entry, ok := c.items[string(key)]
	if !ok {
		atomic.AddInt64(&c.misses, 1)
		recordMiss("ttl")
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.expiresAt) {
		c.removeEntry(string(key))
		atomic.AddInt64(&c.misses, 1)
		recordMiss("ttl")
		return nil, false
	}

	// Update access time
	entry.accessedAt = time.Now()

	atomic.AddInt64(&c.hits, 1)
	recordHit("ttl")

	return CopyBytes(entry.value), true
}

// Set stores a value with TTL.
func (c *TTLCache) Set(ctx context.Context, key, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return storage.ErrClosed
	}

	keyStr := string(key)
	entrySize := int64(len(key) + len(value))

	// Use provided TTL or default
	if ttl <= 0 {
		ttl = c.cfg.DefaultTTL
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute // Fallback default
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	// Check if key exists
	if existing, ok := c.items[keyStr]; ok {
		// Update existing entry
		c.bytes -= existing.size
		existing.value = CopyBytes(value)
		existing.size = entrySize
		existing.expiresAt = expiresAt
		existing.accessedAt = now
		c.bytes += entrySize
		updateSize("ttl", c.size, c.bytes)
		return nil
	}

	// Evict if necessary
	for c.shouldEvict(entrySize) {
		c.evictOne()
	}

	// Add new entry
	entry := &ttlEntry{
		key:        keyStr,
		value:      CopyBytes(value),
		size:       entrySize,
		expiresAt:  expiresAt,
		accessedAt: now,
		createdAt:  now,
	}

	c.items[keyStr] = entry
	c.size++
	c.bytes += entrySize

	updateSize("ttl", c.size, c.bytes)
	return nil
}

// Delete removes a cached value.
func (c *TTLCache) Delete(ctx context.Context, key []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.removeEntry(string(key))
	updateSize("ttl", c.size, c.bytes)
}

// Clear removes all cached values.
func (c *TTLCache) Clear(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.items = make(map[string]*ttlEntry)
	c.size = 0
	c.bytes = 0

	updateSize("ttl", 0, 0)
	c.log.Debug().Msg("cache cleared")
}

// Stats returns cache statistics.
func (c *TTLCache) Stats() storage.CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return storage.CacheStats{
		Hits:      atomic.LoadInt64(&c.hits),
		Misses:    atomic.LoadInt64(&c.misses),
		Size:      c.size,
		Bytes:     c.bytes,
		Evictions: atomic.LoadInt64(&c.evictions),
	}
}

// Close releases resources.
func (c *TTLCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
		close(c.stopCleanup)
	}

	c.items = nil
	c.log.Info().Msg("ttl cache closed")
	return nil
}

// shouldEvict returns true if eviction is needed.
func (c *TTLCache) shouldEvict(newSize int64) bool {
	if len(c.items) == 0 {
		return false
	}

	if c.cfg.MaxSize > 0 && c.size >= int64(c.cfg.MaxSize) {
		return true
	}

	if c.cfg.MaxBytes > 0 && c.bytes+newSize > c.cfg.MaxBytes {
		return true
	}

	return false
}

// evictOne removes one entry (oldest by expiration).
func (c *TTLCache) evictOne() {
	if len(c.items) == 0 {
		return
	}

	// Find the entry closest to expiration
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range c.items {
		if first || entry.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiresAt
			first = false
		}
	}

	if oldestKey != "" {
		c.removeEntry(oldestKey)
		atomic.AddInt64(&c.evictions, 1)
		recordEviction("ttl")
	}
}

// removeEntry removes an entry from the cache.
func (c *TTLCache) removeEntry(key string) {
	if entry, ok := c.items[key]; ok {
		delete(c.items, key)
		c.size--
		c.bytes -= entry.size
	}
}

// CleanupExpired removes all expired entries.
func (c *TTLCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0
	}

	now := time.Now()
	removed := 0

	for key, entry := range c.items {
		if now.After(entry.expiresAt) {
			c.removeEntry(key)
			removed++
		}
	}

	if removed > 0 {
		updateSize("ttl", c.size, c.bytes)
		c.log.Debug().Int("removed", removed).Msg("cleaned up expired entries")
	}

	return removed
}

// Keys returns all cache keys.
func (c *TTLCache) Keys() [][]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([][]byte, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, []byte(k))
	}
	return keys
}

// Len returns the number of items in the cache.
func (c *TTLCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Contains returns true if the key exists (even if expired).
func (c *TTLCache) Contains(key []byte) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.items[string(key)]
	return ok
}

// TTL returns the remaining TTL for a key.
func (c *TTLCache) TTL(key []byte) (time.Duration, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.items[string(key)]
	if !ok {
		return 0, false
	}

	remaining := time.Until(entry.expiresAt)
	if remaining < 0 {
		return 0, false
	}

	return remaining, true
}

// Touch refreshes the TTL for a key.
func (c *TTLCache) Touch(key []byte, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[string(key)]
	if !ok {
		return false
	}

	if ttl <= 0 {
		ttl = c.cfg.DefaultTTL
	}

	entry.expiresAt = time.Now().Add(ttl)
	entry.accessedAt = time.Now()
	return true
}

// GetWithTTL retrieves a value and its remaining TTL.
func (c *TTLCache) GetWithTTL(ctx context.Context, key []byte) ([]byte, time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, 0, false
	}

	entry, ok := c.items[string(key)]
	if !ok {
		atomic.AddInt64(&c.misses, 1)
		recordMiss("ttl")
		return nil, 0, false
	}

	now := time.Now()
	remaining := entry.expiresAt.Sub(now)

	// Check expiration
	if remaining <= 0 {
		c.removeEntry(string(key))
		atomic.AddInt64(&c.misses, 1)
		recordMiss("ttl")
		return nil, 0, false
	}

	// Update access time
	entry.accessedAt = now

	atomic.AddInt64(&c.hits, 1)
	recordHit("ttl")

	return CopyBytes(entry.value), remaining, true
}

// SetIfAbsent stores a value only if the key doesn't exist.
func (c *TTLCache) SetIfAbsent(ctx context.Context, key, value []byte, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return false
	}

	keyStr := string(key)

	// Check if key exists and is not expired
	if entry, ok := c.items[keyStr]; ok {
		if time.Now().Before(entry.expiresAt) {
			return false
		}
		// Entry exists but is expired, remove it
		c.removeEntry(keyStr)
	}

	// Key doesn't exist, set it
	entrySize := int64(len(key) + len(value))

	// Use provided TTL or default
	if ttl <= 0 {
		ttl = c.cfg.DefaultTTL
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	now := time.Now()

	// Evict if necessary
	for c.shouldEvict(entrySize) {
		c.evictOne()
	}

	entry := &ttlEntry{
		key:        keyStr,
		value:      CopyBytes(value),
		size:       entrySize,
		expiresAt:  now.Add(ttl),
		accessedAt: now,
		createdAt:  now,
	}

	c.items[keyStr] = entry
	c.size++
	c.bytes += entrySize

	updateSize("ttl", c.size, c.bytes)
	return true
}

// GetMultiple retrieves multiple values.
func (c *TTLCache) GetMultiple(ctx context.Context, keys [][]byte) map[string][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	result := make(map[string][]byte, len(keys))
	now := time.Now()

	for _, key := range keys {
		keyStr := string(key)
		entry, ok := c.items[keyStr]
		if !ok {
			atomic.AddInt64(&c.misses, 1)
			recordMiss("ttl")
			continue
		}

		if now.After(entry.expiresAt) {
			c.removeEntry(keyStr)
			atomic.AddInt64(&c.misses, 1)
			recordMiss("ttl")
			continue
		}

		entry.accessedAt = now
		result[keyStr] = CopyBytes(entry.value)
		atomic.AddInt64(&c.hits, 1)
		recordHit("ttl")
	}

	return result
}

// SetMultiple stores multiple values.
func (c *TTLCache) SetMultiple(ctx context.Context, items map[string][]byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	if ttl <= 0 {
		ttl = c.cfg.DefaultTTL
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	for key, value := range items {
		entrySize := int64(len(key) + len(value))

		// Update existing or evict and add
		if existing, ok := c.items[key]; ok {
			c.bytes -= existing.size
			existing.value = CopyBytes(value)
			existing.size = entrySize
			existing.expiresAt = expiresAt
			existing.accessedAt = now
			c.bytes += entrySize
		} else {
			for c.shouldEvict(entrySize) {
				c.evictOne()
			}

			entry := &ttlEntry{
				key:        key,
				value:      CopyBytes(value),
				size:       entrySize,
				expiresAt:  expiresAt,
				accessedAt: now,
				createdAt:  now,
			}

			c.items[key] = entry
			c.size++
			c.bytes += entrySize
		}
	}

	updateSize("ttl", c.size, c.bytes)
}

// DeleteMultiple removes multiple keys.
func (c *TTLCache) DeleteMultiple(ctx context.Context, keys [][]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	for _, key := range keys {
		c.removeEntry(string(key))
	}

	updateSize("ttl", c.size, c.bytes)
}
