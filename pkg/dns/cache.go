// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package dns

import (
	"strings"
	"sync"
	"time"
)

// CacheEntry represents a cached DNS response
type CacheEntry struct {
	Message    *Message
	Expiration time.Time
	Negative   bool // True if this is a negative cache entry (NXDOMAIN)
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.Expiration)
}

// RemainingTTL returns the remaining TTL in seconds
func (e *CacheEntry) RemainingTTL() uint32 {
	remaining := time.Until(e.Expiration)
	if remaining <= 0 {
		return 0
	}
	return uint32(remaining.Seconds())
}

// Cache is a DNS response cache with TTL awareness
type Cache struct {
	mu              sync.RWMutex
	entries         map[string]*CacheEntry
	maxSize         int
	negativeTTL     time.Duration
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	MaxSize         int           // Maximum number of entries
	NegativeTTL     time.Duration // TTL for negative (NXDOMAIN) responses
	CleanupInterval time.Duration // Interval for cleanup of expired entries
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxSize:         10000,
		NegativeTTL:     5 * time.Minute,
		CleanupInterval: time.Minute,
	}
}

// NewCache creates a new DNS cache
func NewCache(config *CacheConfig) *Cache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	c := &Cache{
		entries:         make(map[string]*CacheEntry),
		maxSize:         config.MaxSize,
		negativeTTL:     config.NegativeTTL,
		cleanupInterval: config.CleanupInterval,
		stopCleanup:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go c.cleanupLoop()

	return c
}

// cacheKey generates a cache key for a query
func cacheKey(name string, qtype, qclass uint16) string {
	return strings.ToLower(name) + "|" + TypeToString(qtype) + "|" + ClassToString(qclass)
}

// Get retrieves a cached response
func (c *Cache) Get(name string, qtype, qclass uint16) (*Message, bool) {
	key := cacheKey(name, qtype, qclass)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists || entry.IsExpired() {
		if exists {
			// Remove expired entry
			c.mu.Lock()
			delete(c.entries, key)
			c.mu.Unlock()
		}
		return nil, false
	}

	// Clone the message and adjust TTLs
	return c.cloneWithAdjustedTTL(entry), true
}

// Set caches a DNS response
func (c *Cache) Set(msg *Message) {
	if msg == nil || len(msg.Questions) == 0 {
		return
	}

	// Don't cache if no answers and not NXDOMAIN
	if len(msg.Answers) == 0 && msg.Header.Rcode() != RcodeNameError {
		return
	}

	q := msg.Questions[0]
	key := cacheKey(q.Name, q.Type, q.Class)

	// Calculate TTL
	var ttl time.Duration
	negative := false

	if msg.Header.Rcode() == RcodeNameError {
		// Negative caching
		ttl = c.negativeTTL
		negative = true

		// Use SOA minimum TTL if available
		for _, rr := range msg.Authority {
			if soa, ok := rr.Data.(*SOARecord); ok {
				soaTTL := time.Duration(soa.Minimum) * time.Second
				if soaTTL < ttl {
					ttl = soaTTL
				}
				break
			}
		}
	} else {
		// Use minimum TTL from answers
		ttl = c.getMinTTL(msg.Answers)
		if ttl == 0 {
			return // Don't cache if TTL is 0
		}
	}

	entry := &CacheEntry{
		Message:    msg,
		Expiration: time.Now().Add(ttl),
		Negative:   negative,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = entry
}

// SetNegative caches a negative (NXDOMAIN) response
func (c *Cache) SetNegative(name string, qtype, qclass uint16, ttl time.Duration) {
	key := cacheKey(name, qtype, qclass)

	if ttl <= 0 {
		ttl = c.negativeTTL
	}

	entry := &CacheEntry{
		Message: &Message{
			Header: Header{
				Flags: FlagQR | FlagAA,
			},
			Questions: []Question{{Name: name, Type: qtype, Class: qclass}},
		},
		Expiration: time.Now().Add(ttl),
		Negative:   true,
	}
	entry.Message.Header.SetRcode(RcodeNameError)

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = entry
}

// Delete removes an entry from the cache
func (c *Cache) Delete(name string, qtype, qclass uint16) {
	key := cacheKey(name, qtype, qclass)

	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// DeleteByName removes all entries for a name
func (c *Cache) DeleteByName(name string) {
	name = strings.ToLower(name)

	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.entries {
		if strings.HasPrefix(key, name+"|") {
			delete(c.entries, key)
		}
	}
}

// Flush clears the entire cache
func (c *Cache) Flush() {
	c.mu.Lock()
	c.entries = make(map[string]*CacheEntry)
	c.mu.Unlock()
}

// Size returns the number of cached entries
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Stop stops the cache cleanup goroutine
func (c *Cache) Stop() {
	close(c.stopCleanup)
}

// Stats returns cache statistics
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		Size:     len(c.entries),
		MaxSize:  c.maxSize,
		Positive: 0,
		Negative: 0,
		Expired:  0,
	}

	now := time.Now()
	for _, entry := range c.entries {
		if entry.Expiration.Before(now) {
			stats.Expired++
		} else if entry.Negative {
			stats.Negative++
		} else {
			stats.Positive++
		}
	}

	return stats
}

// CacheStats holds cache statistics
type CacheStats struct {
	Size     int
	MaxSize  int
	Positive int // Positive cache entries
	Negative int // Negative cache entries (NXDOMAIN)
	Expired  int // Expired entries not yet cleaned
}

// cloneWithAdjustedTTL clones a message and adjusts TTLs
func (c *Cache) cloneWithAdjustedTTL(entry *CacheEntry) *Message {
	remaining := entry.RemainingTTL()

	msg := &Message{
		Header:    entry.Message.Header,
		Questions: make([]Question, len(entry.Message.Questions)),
	}
	copy(msg.Questions, entry.Message.Questions)

	// Clone and adjust answers
	msg.Answers = make([]ResourceRecord, len(entry.Message.Answers))
	for i, rr := range entry.Message.Answers {
		clone := *rr.Clone()
		if clone.TTL > remaining {
			clone.TTL = remaining
		}
		msg.Answers[i] = clone
	}

	// Clone and adjust authority
	msg.Authority = make([]ResourceRecord, len(entry.Message.Authority))
	for i, rr := range entry.Message.Authority {
		clone := *rr.Clone()
		if clone.TTL > remaining {
			clone.TTL = remaining
		}
		msg.Authority[i] = clone
	}

	// Clone and adjust additional
	msg.Additional = make([]ResourceRecord, len(entry.Message.Additional))
	for i, rr := range entry.Message.Additional {
		clone := *rr.Clone()
		if clone.TTL > remaining {
			clone.TTL = remaining
		}
		msg.Additional[i] = clone
	}

	return msg
}

// getMinTTL finds the minimum TTL in a set of records
func (c *Cache) getMinTTL(records []ResourceRecord) time.Duration {
	if len(records) == 0 {
		return 0
	}

	minTTL := records[0].TTL
	for _, rr := range records[1:] {
		if rr.TTL < minTTL {
			minTTL = rr.TTL
		}
	}

	return time.Duration(minTTL) * time.Second
}

// evictOldest removes the oldest/most-expired entries
func (c *Cache) evictOldest() {
	// Find and remove expired entries first
	now := time.Now()
	for key, entry := range c.entries {
		if entry.Expiration.Before(now) {
			delete(c.entries, key)
		}
	}

	// If still over capacity, remove entries closest to expiration
	if len(c.entries) >= c.maxSize {
		var oldestKey string
		var oldestTime time.Time

		for key, entry := range c.entries {
			if oldestKey == "" || entry.Expiration.Before(oldestTime) {
				oldestKey = key
				oldestTime = entry.Expiration
			}
		}

		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
}

// cleanupLoop periodically removes expired entries
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanup removes all expired entries
func (c *Cache) cleanup() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if entry.Expiration.Before(now) {
			delete(c.entries, key)
		}
	}
}

// Prefetch attempts to refresh a cache entry before it expires
func (c *Cache) Prefetch(name string, qtype, qclass uint16, refreshThreshold time.Duration) bool {
	key := cacheKey(name, qtype, qclass)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists || entry.Negative {
		return false
	}

	// Check if within refresh threshold
	timeToExpire := time.Until(entry.Expiration)
	return timeToExpire <= refreshThreshold && timeToExpire > 0
}

// GetAll returns all non-expired cache entries (for debugging)
func (c *Cache) GetAll() map[string]*CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*CacheEntry)
	now := time.Now()

	for key, entry := range c.entries {
		if entry.Expiration.After(now) {
			result[key] = entry
		}
	}

	return result
}

// RecordCache is a specialized cache for individual records
type RecordCache struct {
	mu      sync.RWMutex
	records map[string]map[uint16][]*CachedRecord
	maxAge  time.Duration
}

// CachedRecord wraps a ResourceRecord with expiration
type CachedRecord struct {
	Record     *ResourceRecord
	Expiration time.Time
}

// NewRecordCache creates a new record cache
func NewRecordCache(maxAge time.Duration) *RecordCache {
	return &RecordCache{
		records: make(map[string]map[uint16][]*CachedRecord),
		maxAge:  maxAge,
	}
}

// Add adds a record to the cache
func (rc *RecordCache) Add(rr *ResourceRecord) {
	name := strings.ToLower(rr.Name)
	ttl := time.Duration(rr.TTL) * time.Second
	if ttl > rc.maxAge {
		ttl = rc.maxAge
	}

	cached := &CachedRecord{
		Record:     rr.Clone(),
		Expiration: time.Now().Add(ttl),
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.records[name] == nil {
		rc.records[name] = make(map[uint16][]*CachedRecord)
	}

	rc.records[name][rr.Type] = append(rc.records[name][rr.Type], cached)
}

// Get retrieves records from the cache
func (rc *RecordCache) Get(name string, qtype uint16) []*ResourceRecord {
	name = strings.ToLower(name)
	now := time.Now()

	rc.mu.RLock()
	defer rc.mu.RUnlock()

	typeMap, exists := rc.records[name]
	if !exists {
		return nil
	}

	cached := typeMap[qtype]
	if len(cached) == 0 {
		return nil
	}

	var result []*ResourceRecord
	for _, c := range cached {
		if c.Expiration.After(now) {
			clone := c.Record.Clone()
			remaining := uint32(time.Until(c.Expiration).Seconds())
			if remaining < clone.TTL {
				clone.TTL = remaining
			}
			result = append(result, clone)
		}
	}

	return result
}

// Remove removes records from the cache
func (rc *RecordCache) Remove(name string, qtype uint16) {
	name = strings.ToLower(name)

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if typeMap, exists := rc.records[name]; exists {
		delete(typeMap, qtype)
		if len(typeMap) == 0 {
			delete(rc.records, name)
		}
	}
}

// Cleanup removes expired records
func (rc *RecordCache) Cleanup() {
	now := time.Now()

	rc.mu.Lock()
	defer rc.mu.Unlock()

	for name, typeMap := range rc.records {
		for qtype, cached := range typeMap {
			var valid []*CachedRecord
			for _, c := range cached {
				if c.Expiration.After(now) {
					valid = append(valid, c)
				}
			}
			if len(valid) == 0 {
				delete(typeMap, qtype)
			} else {
				typeMap[qtype] = valid
			}
		}
		if len(typeMap) == 0 {
			delete(rc.records, name)
		}
	}
}
