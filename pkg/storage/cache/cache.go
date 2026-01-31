// Package cache provides caching implementations for the Kingdom.
// This package supports LRU and TTL-based caching strategies.
package cache

import (
	"context"
	"os"
	"time"

	"github.com/unheaded/unheaded/pkg/logger"
	"github.com/unheaded/unheaded/pkg/metrics"
	"github.com/unheaded/unheaded/pkg/storage"
)

// Type represents a cache type.
type Type string

const (
	// TypeLRU is a Least Recently Used cache.
	TypeLRU Type = "lru"
	// TypeTTL is a Time To Live based cache.
	TypeTTL Type = "ttl"
)

// Config holds cache configuration.
type Config struct {
	// Type is the cache type.
	Type Type `json:"type" yaml:"type"`

	// MaxSize is the maximum number of items.
	MaxSize int `json:"max_size" yaml:"max_size"`

	// MaxBytes is the maximum size in bytes.
	MaxBytes int64 `json:"max_bytes" yaml:"max_bytes"`

	// DefaultTTL is the default TTL for items.
	DefaultTTL time.Duration `json:"default_ttl" yaml:"default_ttl"`

	// CleanupInterval is how often to run cleanup.
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`

	// Logger is the logger instance.
	Logger *logger.Logger

	// Metrics is the metrics registry.
	Metrics *metrics.Registry
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Type:            TypeLRU,
		MaxSize:         10000,
		MaxBytes:        64 * 1024 * 1024, // 64MB
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
}

// New creates a new cache based on configuration.
func New(cfg Config) (storage.Cache, error) {
	switch cfg.Type {
	case TypeLRU:
		return NewLRUCache(cfg), nil
	case TypeTTL:
		return NewTTLCache(cfg), nil
	default:
		return NewLRUCache(cfg), nil
	}
}

// Metrics for cache operations.
var (
	cacheHitsTotal = metrics.NewCounterVec(
		"unheaded_cache_hits_total",
		"Total cache hits",
		nil,
		[]string{"cache"},
	)

	cacheMissesTotal = metrics.NewCounterVec(
		"unheaded_cache_misses_total",
		"Total cache misses",
		nil,
		[]string{"cache"},
	)

	cacheEvictionsTotal = metrics.NewCounterVec(
		"unheaded_cache_evictions_total",
		"Total cache evictions",
		nil,
		[]string{"cache"},
	)

	cacheSizeItems = metrics.NewGaugeVec(
		"unheaded_cache_size_items",
		"Current number of items in cache",
		nil,
		[]string{"cache"},
	)

	cacheSizeBytes = metrics.NewGaugeVec(
		"unheaded_cache_size_bytes",
		"Current size of cache in bytes",
		nil,
		[]string{"cache"},
	)
)

// recordHit records a cache hit.
func recordHit(cacheType string) {
	cacheHitsTotal.WithLabels(metrics.Labels{"cache": cacheType}).Inc()
}

// recordMiss records a cache miss.
func recordMiss(cacheType string) {
	cacheMissesTotal.WithLabels(metrics.Labels{"cache": cacheType}).Inc()
}

// recordEviction records a cache eviction.
func recordEviction(cacheType string) {
	cacheEvictionsTotal.WithLabels(metrics.Labels{"cache": cacheType}).Inc()
}

// updateSize updates the size metrics.
func updateSize(cacheType string, items int64, bytes int64) {
	cacheSizeItems.WithLabels(metrics.Labels{"cache": cacheType}).Set(float64(items))
	cacheSizeBytes.WithLabels(metrics.Labels{"cache": cacheType}).Set(float64(bytes))
}

// CacheStats contains cache statistics.
type CacheStats = storage.CacheStats

// Entry represents a cache entry.
type Entry struct {
	// Key is the cache key.
	Key []byte
	// Value is the cached value.
	Value []byte
	// Size is the size in bytes.
	Size int64
	// ExpiresAt is when the entry expires.
	ExpiresAt time.Time
	// AccessedAt is when the entry was last accessed.
	AccessedAt time.Time
	// CreatedAt is when the entry was created.
	CreatedAt time.Time
}

// IsExpired returns true if the entry has expired.
func (e *Entry) IsExpired() bool {
	if e.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(e.ExpiresAt)
}

// CopyBytes creates a copy of a byte slice.
func CopyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}

// CacheLoader loads values that are not in cache.
type CacheLoader interface {
	// Load loads a value by key.
	Load(ctx context.Context, key []byte) ([]byte, error)
}

// LoaderFunc is a function adapter for CacheLoader.
type LoaderFunc func(ctx context.Context, key []byte) ([]byte, error)

// Load implements CacheLoader.
func (f LoaderFunc) Load(ctx context.Context, key []byte) ([]byte, error) {
	return f(ctx, key)
}

// LoadingCache wraps a cache with automatic loading.
type LoadingCache struct {
	cache  storage.Cache
	loader CacheLoader
	ttl    time.Duration
}

// NewLoadingCache creates a new loading cache.
func NewLoadingCache(cache storage.Cache, loader CacheLoader, ttl time.Duration) *LoadingCache {
	return &LoadingCache{
		cache:  cache,
		loader: loader,
		ttl:    ttl,
	}
}

// Get retrieves a value, loading it if not cached.
func (l *LoadingCache) Get(ctx context.Context, key []byte) ([]byte, error) {
	// Try cache first
	if value, ok := l.cache.Get(ctx, key); ok {
		return value, nil
	}

	// Load from source
	value, err := l.loader.Load(ctx, key)
	if err != nil {
		return nil, err
	}

	// Store in cache
	if err := l.cache.Set(ctx, key, value, l.ttl); err != nil {
		// Log but don't fail
		return value, nil
	}

	return value, nil
}

// Invalidate removes a key from the cache.
func (l *LoadingCache) Invalidate(ctx context.Context, key []byte) {
	l.cache.Delete(ctx, key)
}

// Clear removes all entries from the cache.
func (l *LoadingCache) Clear(ctx context.Context) {
	l.cache.Clear(ctx)
}

// Stats returns cache statistics.
func (l *LoadingCache) Stats() CacheStats {
	return l.cache.Stats()
}

// Close releases resources.
func (l *LoadingCache) Close() error {
	return l.cache.Close()
}

// MultiLevelCache chains multiple caches.
type MultiLevelCache struct {
	caches []storage.Cache
	log    *logger.Logger
}

// NewMultiLevelCache creates a new multi-level cache.
func NewMultiLevelCache(caches ...storage.Cache) *MultiLevelCache {
	return &MultiLevelCache{
		caches: caches,
		log:    logger.New(os.Stdout),
	}
}

// Get retrieves a value from the cache chain.
func (m *MultiLevelCache) Get(ctx context.Context, key []byte) ([]byte, bool) {
	for i, cache := range m.caches {
		if value, ok := cache.Get(ctx, key); ok {
			// Populate earlier caches
			for j := 0; j < i; j++ {
				m.caches[j].Set(ctx, key, value, 0)
			}
			return value, true
		}
	}
	return nil, false
}

// Set stores a value in all cache levels.
func (m *MultiLevelCache) Set(ctx context.Context, key, value []byte, ttl time.Duration) error {
	for _, cache := range m.caches {
		if err := cache.Set(ctx, key, value, ttl); err != nil {
			m.log.Warn().Err(err).Msg("failed to set in cache level")
		}
	}
	return nil
}

// Delete removes a key from all cache levels.
func (m *MultiLevelCache) Delete(ctx context.Context, key []byte) {
	for _, cache := range m.caches {
		cache.Delete(ctx, key)
	}
}

// Clear removes all entries from all cache levels.
func (m *MultiLevelCache) Clear(ctx context.Context) {
	for _, cache := range m.caches {
		cache.Clear(ctx)
	}
}

// Stats returns combined cache statistics.
func (m *MultiLevelCache) Stats() CacheStats {
	var combined CacheStats
	for _, cache := range m.caches {
		stats := cache.Stats()
		combined.Hits += stats.Hits
		combined.Misses += stats.Misses
		combined.Size += stats.Size
		combined.Bytes += stats.Bytes
		combined.Evictions += stats.Evictions
	}
	return combined
}

// Close releases resources for all cache levels.
func (m *MultiLevelCache) Close() error {
	for _, cache := range m.caches {
		cache.Close()
	}
	return nil
}

// CacheWrapper wraps a KVStore with caching.
type CacheWrapper struct {
	store storage.KVStore
	cache storage.Cache
	ttl   time.Duration
}

// NewCacheWrapper creates a cached KVStore wrapper.
func NewCacheWrapper(store storage.KVStore, cache storage.Cache, ttl time.Duration) *CacheWrapper {
	return &CacheWrapper{
		store: store,
		cache: cache,
		ttl:   ttl,
	}
}

// Get retrieves a value, checking cache first.
func (w *CacheWrapper) Get(ctx context.Context, key []byte) ([]byte, error) {
	// Check cache
	if value, ok := w.cache.Get(ctx, key); ok {
		return value, nil
	}

	// Get from store
	value, err := w.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	// Cache the result
	w.cache.Set(ctx, key, value, w.ttl)

	return value, nil
}

// Set stores a value and invalidates cache.
func (w *CacheWrapper) Set(ctx context.Context, key, value []byte) error {
	// Update store
	if err := w.store.Set(ctx, key, value); err != nil {
		return err
	}

	// Update cache
	w.cache.Set(ctx, key, value, w.ttl)

	return nil
}

// Delete removes a key and invalidates cache.
func (w *CacheWrapper) Delete(ctx context.Context, key []byte) error {
	// Delete from store
	if err := w.store.Delete(ctx, key); err != nil {
		return err
	}

	// Invalidate cache
	w.cache.Delete(ctx, key)

	return nil
}

// Scan iterates over keys (bypasses cache).
func (w *CacheWrapper) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	return w.store.Scan(ctx, prefix, fn)
}

// Close releases resources.
func (w *CacheWrapper) Close() error {
	w.cache.Close()
	return w.store.Close()
}
