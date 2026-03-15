// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dns

import (
	"net"
	"testing"
	"time"
)

func TestCache_SetAndGet(t *testing.T) {
	config := &CacheConfig{
		MaxSize:         100,
		NegativeTTL:     time.Minute,
		CleanupInterval: time.Hour, // Long interval for testing
	}
	cache := NewCache(config)
	defer cache.Stop()

	// Create a response message
	msg := &Message{
		Header: Header{Flags: FlagQR | FlagAA},
		Questions: []Question{
			{Name: "example.com", Type: TypeA, Class: ClassIN},
		},
		Answers: []ResourceRecord{
			*NewARecord("example.com", 300, net.ParseIP("1.2.3.4")),
		},
	}

	// Set in cache
	cache.Set(msg)

	// Get from cache
	cached, ok := cache.Get("example.com", TypeA, ClassIN)
	if !ok {
		t.Fatal("Get() returned false, expected true")
	}

	if len(cached.Answers) != 1 {
		t.Errorf("Cached message has %d answers, want 1", len(cached.Answers))
	}

	// Check that TTL is adjusted
	if cached.Answers[0].TTL > 300 {
		t.Errorf("TTL = %d, should be <= 300", cached.Answers[0].TTL)
	}
}

func TestCache_NegativeCaching(t *testing.T) {
	config := &CacheConfig{
		MaxSize:         100,
		NegativeTTL:     time.Minute,
		CleanupInterval: time.Hour,
	}
	cache := NewCache(config)
	defer cache.Stop()

	// Create NXDOMAIN response
	msg := &Message{
		Header: Header{Flags: FlagQR | FlagAA},
		Questions: []Question{
			{Name: "nonexistent.com", Type: TypeA, Class: ClassIN},
		},
	}
	msg.Header.SetRcode(RcodeNameError)

	// Add SOA to authority section
	msg.Authority = []ResourceRecord{
		*NewSOARecord("com", 60, "ns1.com", "admin.com", 1, 3600, 600, 86400, 30),
	}

	// Set in cache (NXDOMAIN should be cached)
	cache.Set(msg)

	// Get from cache
	cached, ok := cache.Get("nonexistent.com", TypeA, ClassIN)
	if !ok {
		t.Fatal("Negative response not cached")
	}

	if cached.Header.Rcode() != RcodeNameError {
		t.Errorf("Cached rcode = %d, want %d (NXDOMAIN)", cached.Header.Rcode(), RcodeNameError)
	}
}

func TestCache_SetNegative(t *testing.T) {
	config := DefaultCacheConfig()
	config.CleanupInterval = time.Hour
	cache := NewCache(config)
	defer cache.Stop()

	// Directly set negative cache entry
	cache.SetNegative("notfound.local", TypeA, ClassIN, time.Minute)

	cached, ok := cache.Get("notfound.local", TypeA, ClassIN)
	if !ok {
		t.Fatal("SetNegative entry not cached")
	}

	if cached.Header.Rcode() != RcodeNameError {
		t.Errorf("Cached rcode = %d, want NXDOMAIN", cached.Header.Rcode())
	}
}

func TestCache_Expiration(t *testing.T) {
	config := &CacheConfig{
		MaxSize:         100,
		NegativeTTL:     50 * time.Millisecond,
		CleanupInterval: time.Hour,
	}
	cache := NewCache(config)
	defer cache.Stop()

	// Create response with very short TTL
	msg := &Message{
		Header: Header{Flags: FlagQR | FlagAA},
		Questions: []Question{
			{Name: "expire.test", Type: TypeA, Class: ClassIN},
		},
		Answers: []ResourceRecord{
			*NewARecord("expire.test", 1, net.ParseIP("1.2.3.4")), // 1 second TTL
		},
	}
	// Manually set very short TTL in cache
	cache.Set(msg)

	// Immediately available
	_, ok := cache.Get("expire.test", TypeA, ClassIN)
	if !ok {
		t.Fatal("Entry should be cached immediately")
	}

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get("expire.test", TypeA, ClassIN)
	if ok {
		t.Error("Entry should have expired")
	}
}

func TestCache_Delete(t *testing.T) {
	config := DefaultCacheConfig()
	config.CleanupInterval = time.Hour
	cache := NewCache(config)
	defer cache.Stop()

	msg := &Message{
		Header: Header{Flags: FlagQR},
		Questions: []Question{
			{Name: "delete.test", Type: TypeA, Class: ClassIN},
		},
		Answers: []ResourceRecord{
			*NewARecord("delete.test", 300, net.ParseIP("1.2.3.4")),
		},
	}
	cache.Set(msg)

	// Verify it's cached
	_, ok := cache.Get("delete.test", TypeA, ClassIN)
	if !ok {
		t.Fatal("Entry should be cached")
	}

	// Delete
	cache.Delete("delete.test", TypeA, ClassIN)

	// Should be gone
	_, ok = cache.Get("delete.test", TypeA, ClassIN)
	if ok {
		t.Error("Entry should be deleted")
	}
}

func TestCache_DeleteByName(t *testing.T) {
	config := DefaultCacheConfig()
	config.CleanupInterval = time.Hour
	cache := NewCache(config)
	defer cache.Stop()

	// Add multiple record types for same name
	aMsg := &Message{
		Header:    Header{Flags: FlagQR},
		Questions: []Question{{Name: "multi.test", Type: TypeA, Class: ClassIN}},
		Answers:   []ResourceRecord{*NewARecord("multi.test", 300, net.ParseIP("1.2.3.4"))},
	}
	cache.Set(aMsg)

	txtMsg := &Message{
		Header:    Header{Flags: FlagQR},
		Questions: []Question{{Name: "multi.test", Type: TypeTXT, Class: ClassIN}},
		Answers:   []ResourceRecord{*NewTXTRecord("multi.test", 300, "test")},
	}
	cache.Set(txtMsg)

	// Both should be cached
	if _, ok := cache.Get("multi.test", TypeA, ClassIN); !ok {
		t.Fatal("A record should be cached")
	}
	if _, ok := cache.Get("multi.test", TypeTXT, ClassIN); !ok {
		t.Fatal("TXT record should be cached")
	}

	// Delete by name
	cache.DeleteByName("multi.test")

	// Both should be gone
	if _, ok := cache.Get("multi.test", TypeA, ClassIN); ok {
		t.Error("A record should be deleted")
	}
	if _, ok := cache.Get("multi.test", TypeTXT, ClassIN); ok {
		t.Error("TXT record should be deleted")
	}
}

func TestCache_Flush(t *testing.T) {
	config := DefaultCacheConfig()
	config.CleanupInterval = time.Hour
	cache := NewCache(config)
	defer cache.Stop()

	// Add multiple entries
	for i := 0; i < 10; i++ {
		msg := &Message{
			Header:    Header{Flags: FlagQR},
			Questions: []Question{{Name: "flush" + string(rune('0'+i)) + ".test", Type: TypeA, Class: ClassIN}},
			Answers:   []ResourceRecord{*NewARecord("flush.test", 300, net.ParseIP("1.2.3.4"))},
		}
		cache.Set(msg)
	}

	if cache.Size() == 0 {
		t.Fatal("Cache should have entries")
	}

	cache.Flush()

	if cache.Size() != 0 {
		t.Errorf("Cache size after flush = %d, want 0", cache.Size())
	}
}

func TestCache_Stats(t *testing.T) {
	config := DefaultCacheConfig()
	config.CleanupInterval = time.Hour
	cache := NewCache(config)
	defer cache.Stop()

	// Add positive entry
	msg := &Message{
		Header:    Header{Flags: FlagQR},
		Questions: []Question{{Name: "stats.test", Type: TypeA, Class: ClassIN}},
		Answers:   []ResourceRecord{*NewARecord("stats.test", 300, net.ParseIP("1.2.3.4"))},
	}
	cache.Set(msg)

	// Add negative entry
	cache.SetNegative("neg.test", TypeA, ClassIN, time.Minute)

	stats := cache.Stats()
	if stats.Size != 2 {
		t.Errorf("Stats.Size = %d, want 2", stats.Size)
	}
	if stats.Positive != 1 {
		t.Errorf("Stats.Positive = %d, want 1", stats.Positive)
	}
	if stats.Negative != 1 {
		t.Errorf("Stats.Negative = %d, want 1", stats.Negative)
	}
}

func TestCache_MaxSize(t *testing.T) {
	config := &CacheConfig{
		MaxSize:         5,
		NegativeTTL:     time.Minute,
		CleanupInterval: time.Hour,
	}
	cache := NewCache(config)
	defer cache.Stop()

	// Add more entries than max size
	for i := 0; i < 10; i++ {
		msg := &Message{
			Header:    Header{Flags: FlagQR},
			Questions: []Question{{Name: "max" + string(rune('0'+i)) + ".test", Type: TypeA, Class: ClassIN}},
			Answers:   []ResourceRecord{*NewARecord("max.test", 300, net.ParseIP("1.2.3.4"))},
		}
		cache.Set(msg)
	}

	if cache.Size() > config.MaxSize {
		t.Errorf("Cache size = %d, exceeds max %d", cache.Size(), config.MaxSize)
	}
}

func TestCache_TTLAdjustment(t *testing.T) {
	config := DefaultCacheConfig()
	config.CleanupInterval = time.Hour
	cache := NewCache(config)
	defer cache.Stop()

	originalTTL := uint32(300)
	msg := &Message{
		Header:    Header{Flags: FlagQR},
		Questions: []Question{{Name: "ttl.test", Type: TypeA, Class: ClassIN}},
		Answers:   []ResourceRecord{*NewARecord("ttl.test", originalTTL, net.ParseIP("1.2.3.4"))},
	}
	cache.Set(msg)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Get and check TTL is reduced
	cached, ok := cache.Get("ttl.test", TypeA, ClassIN)
	if !ok {
		t.Fatal("Entry should be cached")
	}

	if cached.Answers[0].TTL >= originalTTL {
		t.Errorf("TTL should be reduced, got %d (original %d)", cached.Answers[0].TTL, originalTTL)
	}
}

func TestCache_Prefetch(t *testing.T) {
	config := DefaultCacheConfig()
	config.CleanupInterval = time.Hour
	cache := NewCache(config)
	defer cache.Stop()

	// Add entry with short TTL
	msg := &Message{
		Header:    Header{Flags: FlagQR},
		Questions: []Question{{Name: "prefetch.test", Type: TypeA, Class: ClassIN}},
		Answers:   []ResourceRecord{*NewARecord("prefetch.test", 2, net.ParseIP("1.2.3.4"))},
	}
	cache.Set(msg)

	// Should not need prefetch immediately
	if cache.Prefetch("prefetch.test", TypeA, ClassIN, time.Second) {
		t.Error("Prefetch should return false when TTL is high")
	}

	// Wait until close to expiration
	time.Sleep(1200 * time.Millisecond)

	// Now should need prefetch
	if !cache.Prefetch("prefetch.test", TypeA, ClassIN, time.Second) {
		t.Error("Prefetch should return true when TTL is low")
	}
}

func TestRecordCache_AddAndGet(t *testing.T) {
	cache := NewRecordCache(time.Minute)

	rr := NewARecord("record.test", 60, net.ParseIP("1.2.3.4"))
	cache.Add(rr)

	records := cache.Get("record.test", TypeA)
	if len(records) != 1 {
		t.Fatalf("Get() returned %d records, want 1", len(records))
	}

	if !records[0].GetIP().Equal(net.ParseIP("1.2.3.4")) {
		t.Errorf("Got IP %v, want 1.2.3.4", records[0].GetIP())
	}
}

func TestRecordCache_TTLCapping(t *testing.T) {
	cache := NewRecordCache(10 * time.Second) // Max 10 seconds

	// Add record with long TTL
	rr := NewARecord("ttlcap.test", 3600, net.ParseIP("1.2.3.4")) // 1 hour
	cache.Add(rr)

	// The cache should cap the TTL
	records := cache.Get("ttlcap.test", TypeA)
	if len(records) != 1 {
		t.Fatal("Record not cached")
	}

	// TTL should be capped
	if records[0].TTL > 10 {
		t.Errorf("TTL = %d, should be capped at 10", records[0].TTL)
	}
}

func TestRecordCache_Remove(t *testing.T) {
	cache := NewRecordCache(time.Minute)

	cache.Add(NewARecord("remove.test", 60, net.ParseIP("1.2.3.4")))
	cache.Add(NewAAAARecord("remove.test", 60, net.ParseIP("::1")))

	// Remove A records
	cache.Remove("remove.test", TypeA)

	// A should be gone
	if len(cache.Get("remove.test", TypeA)) != 0 {
		t.Error("A records should be removed")
	}

	// AAAA should still be there
	if len(cache.Get("remove.test", TypeAAAA)) != 1 {
		t.Error("AAAA records should still exist")
	}
}

func TestRecordCache_Cleanup(t *testing.T) {
	cache := NewRecordCache(50 * time.Millisecond)

	cache.Add(NewARecord("cleanup.test", 1, net.ParseIP("1.2.3.4")))

	// Immediately available
	if len(cache.Get("cleanup.test", TypeA)) == 0 {
		t.Fatal("Record should be cached")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Cleanup
	cache.Cleanup()

	// Should be gone after cleanup
	if len(cache.Get("cleanup.test", TypeA)) != 0 {
		t.Error("Expired record should be cleaned up")
	}
}

func TestCacheEntry_RemainingTTL(t *testing.T) {
	entry := &CacheEntry{
		Expiration: time.Now().Add(10 * time.Second),
	}

	ttl := entry.RemainingTTL()
	if ttl < 9 || ttl > 10 {
		t.Errorf("RemainingTTL() = %d, want ~10", ttl)
	}

	// Test expired entry
	expiredEntry := &CacheEntry{
		Expiration: time.Now().Add(-time.Second),
	}
	if expiredEntry.RemainingTTL() != 0 {
		t.Errorf("Expired entry RemainingTTL() = %d, want 0", expiredEntry.RemainingTTL())
	}
}

func TestCacheEntry_IsExpired(t *testing.T) {
	// Not expired
	entry := &CacheEntry{
		Expiration: time.Now().Add(time.Hour),
	}
	if entry.IsExpired() {
		t.Error("Entry should not be expired")
	}

	// Expired
	expiredEntry := &CacheEntry{
		Expiration: time.Now().Add(-time.Second),
	}
	if !expiredEntry.IsExpired() {
		t.Error("Entry should be expired")
	}
}
