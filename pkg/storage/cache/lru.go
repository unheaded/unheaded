package cache

import (
	"container/list"
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unheaded/unheaded/pkg/logger"
	"github.com/unheaded/unheaded/pkg/storage"
)

// LRUCache is a Least Recently Used cache implementation.
type LRUCache struct {
	cfg      Config
	log      *logger.Logger
	mu       sync.RWMutex
	items    map[string]*list.Element
	evictList *list.List
	size     int64
	bytes    int64
	hits     int64
	misses   int64
	evictions int64
	closed   bool
}

// lruEntry is an entry in the LRU cache.
type lruEntry struct {
	key       string
	value     []byte
	size      int64
	expiresAt time.Time
}

// NewLRUCache creates a new LRU cache.
func NewLRUCache(cfg Config) *LRUCache {
	log := cfg.Logger
	if log == nil {
		log = logger.New(os.Stdout)
	}

	cache := &LRUCache{
		cfg:       cfg,
		log:       log.With().Str("cache", "lru").Logger(),
		items:     make(map[string]*list.Element),
		evictList: list.New(),
	}

	cache.log.Info().
		Int("max_size", cfg.MaxSize).
		Int64("max_bytes", cfg.MaxBytes).
		Msg("lru cache initialized")

	return cache
}

// Get retrieves a cached value.
func (c *LRUCache) Get(ctx context.Context, key []byte) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, false
	}

	elem, ok := c.items[string(key)]
	if !ok {
		atomic.AddInt64(&c.misses, 1)
		recordMiss("lru")
		return nil, false
	}

	entry := elem.Value.(*lruEntry)

	// Check expiration
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		c.removeElement(elem)
		atomic.AddInt64(&c.misses, 1)
		recordMiss("lru")
		return nil, false
	}

	// Move to front (most recently used)
	c.evictList.MoveToFront(elem)

	atomic.AddInt64(&c.hits, 1)
	recordHit("lru")

	return CopyBytes(entry.value), true
}

// Set stores a value with optional TTL.
func (c *LRUCache) Set(ctx context.Context, key, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return storage.ErrClosed
	}

	keyStr := string(key)
	entrySize := int64(len(key) + len(value))

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	} else if c.cfg.DefaultTTL > 0 {
		expiresAt = time.Now().Add(c.cfg.DefaultTTL)
	}

	// Check if key exists
	if elem, ok := c.items[keyStr]; ok {
		// Update existing entry
		entry := elem.Value.(*lruEntry)
		c.bytes -= entry.size
		entry.value = CopyBytes(value)
		entry.size = entrySize
		entry.expiresAt = expiresAt
		c.bytes += entrySize
		c.evictList.MoveToFront(elem)
		updateSize("lru", c.size, c.bytes)
		return nil
	}

	// Evict if necessary
	for c.shouldEvict(entrySize) {
		c.evictOldest()
	}

	// Add new entry
	entry := &lruEntry{
		key:       keyStr,
		value:     CopyBytes(value),
		size:      entrySize,
		expiresAt: expiresAt,
	}

	elem := c.evictList.PushFront(entry)
	c.items[keyStr] = elem
	c.size++
	c.bytes += entrySize

	updateSize("lru", c.size, c.bytes)
	return nil
}

// Delete removes a cached value.
func (c *LRUCache) Delete(ctx context.Context, key []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	if elem, ok := c.items[string(key)]; ok {
		c.removeElement(elem)
		updateSize("lru", c.size, c.bytes)
	}
}

// Clear removes all cached values.
func (c *LRUCache) Clear(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.items = make(map[string]*list.Element)
	c.evictList.Init()
	c.size = 0
	c.bytes = 0

	updateSize("lru", 0, 0)
	c.log.Debug().Msg("cache cleared")
}

// Stats returns cache statistics.
func (c *LRUCache) Stats() storage.CacheStats {
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
func (c *LRUCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	c.items = nil
	c.evictList = nil
	c.log.Info().Msg("lru cache closed")
	return nil
}

// shouldEvict returns true if eviction is needed.
func (c *LRUCache) shouldEvict(newSize int64) bool {
	if c.evictList.Len() == 0 {
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

// evictOldest removes the oldest entry.
func (c *LRUCache) evictOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
		atomic.AddInt64(&c.evictions, 1)
		recordEviction("lru")
	}
}

// removeElement removes an element from the cache.
func (c *LRUCache) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	entry := elem.Value.(*lruEntry)
	delete(c.items, entry.key)
	c.size--
	c.bytes -= entry.size
}

// Keys returns all cache keys.
func (c *LRUCache) Keys() [][]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([][]byte, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, []byte(k))
	}
	return keys
}

// Len returns the number of items in the cache.
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return int(c.size)
}

// Contains returns true if the key exists (even if expired).
func (c *LRUCache) Contains(key []byte) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.items[string(key)]
	return ok
}

// Peek retrieves a value without updating its position.
func (c *LRUCache) Peek(ctx context.Context, key []byte) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, false
	}

	elem, ok := c.items[string(key)]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*lruEntry)

	// Check expiration
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return CopyBytes(entry.value), true
}

// RemoveOldest removes the oldest entry and returns its key.
func (c *LRUCache) RemoveOldest() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem := c.evictList.Back()
	if elem == nil {
		return nil, false
	}

	entry := elem.Value.(*lruEntry)
	key := []byte(entry.key)
	c.removeElement(elem)
	updateSize("lru", c.size, c.bytes)

	return key, true
}

// Resize changes the cache capacity.
func (c *LRUCache) Resize(maxSize int, maxBytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cfg.MaxSize = maxSize
	c.cfg.MaxBytes = maxBytes

	// Evict if necessary
	for c.shouldEvict(0) {
		c.evictOldest()
	}

	updateSize("lru", c.size, c.bytes)
	c.log.Info().Int("max_size", maxSize).Int64("max_bytes", maxBytes).Msg("cache resized")
}

// GetOrSet retrieves a value or sets it if not present.
func (c *LRUCache) GetOrSet(ctx context.Context, key []byte, fn func() ([]byte, error), ttl time.Duration) ([]byte, error) {
	// Try to get first
	if value, ok := c.Get(ctx, key); ok {
		return value, nil
	}

	// Generate value
	value, err := fn()
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.Set(ctx, key, value, ttl)

	return value, nil
}

// ExpireNow marks an entry as expired.
func (c *LRUCache) ExpireNow(key []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[string(key)]
	if !ok {
		return false
	}

	entry := elem.Value.(*lruEntry)
	entry.expiresAt = time.Now().Add(-time.Second)
	return true
}

// SetTTL updates the TTL for an entry.
func (c *LRUCache) SetTTL(key []byte, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[string(key)]
	if !ok {
		return false
	}

	entry := elem.Value.(*lruEntry)
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	} else {
		entry.expiresAt = time.Time{}
	}
	return true
}

// CleanupExpired removes all expired entries.
func (c *LRUCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0
	}

	now := time.Now()
	removed := 0

	for elem := c.evictList.Back(); elem != nil; {
		prev := elem.Prev()
		entry := elem.Value.(*lruEntry)
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			c.removeElement(elem)
			removed++
		}
		elem = prev
	}

	if removed > 0 {
		updateSize("lru", c.size, c.bytes)
		c.log.Debug().Int("removed", removed).Msg("cleaned up expired entries")
	}

	return removed
}
