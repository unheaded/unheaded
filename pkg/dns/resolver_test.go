package dns

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolver_NewResolver(t *testing.T) {
	// Test with nil config
	r := NewResolver(nil, nil)
	if r == nil {
		t.Fatal("NewResolver returned nil with nil config")
	}
	if len(r.upstreams) != 2 {
		t.Errorf("Expected 2 default upstreams, got %d", len(r.upstreams))
	}
	r.Close()

	// Test with custom config
	config := &ResolverConfig{
		Upstreams:   []string{"1.1.1.1:53"},
		Timeout:     10 * time.Second,
		MaxRetries:  5,
		CacheConfig: DefaultCacheConfig(),
	}
	r2 := NewResolver(config, nil)
	if len(r2.upstreams) != 1 {
		t.Errorf("Expected 1 upstream, got %d", len(r2.upstreams))
	}
	if r2.timeout != 10*time.Second {
		t.Errorf("Expected 10s timeout, got %v", r2.timeout)
	}
	r2.Close()
}

func TestResolver_ResolveQuery(t *testing.T) {
	zones := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))
	zones.AddZone(zone)

	config := &ResolverConfig{
		Upstreams:   nil, // No upstreams - local only
		CacheConfig: DefaultCacheConfig(),
	}
	r := NewResolver(config, zones)
	defer r.Close()

	ctx := context.Background()
	resp, err := r.ResolveQuery(ctx, "www.test.local", TypeA)
	if err != nil {
		t.Fatalf("ResolveQuery error: %v", err)
	}

	if len(resp.Answers) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(resp.Answers))
	}

	ip := resp.Answers[0].GetIP()
	if !ip.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("Expected IP 10.0.0.1, got %v", ip)
	}
}

func TestResolver_LocalZoneLookup(t *testing.T) {
	zones := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("example.local"))
	zone.AddRecord(NewARecord("api.example.local", 60, net.ParseIP("192.168.1.100")))
	zone.AddRecord(NewAAAARecord("api.example.local", 60, net.ParseIP("::1")))
	zone.AddRecord(NewTXTRecord("api.example.local", 60, "version=1.0"))
	zones.AddZone(zone)

	r := NewResolver(&ResolverConfig{CacheConfig: DefaultCacheConfig()}, zones)
	defer r.Close()

	tests := []struct {
		name   string
		qtype  uint16
		expect int
	}{
		{"A record", TypeA, 1},
		{"AAAA record", TypeAAAA, 1},
		{"TXT record", TypeTXT, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewQuery("api.example.local", tt.qtype)
			resp, err := r.Resolve(context.Background(), msg)
			if err != nil {
				t.Fatalf("Resolve error: %v", err)
			}
			if len(resp.Answers) != tt.expect {
				t.Errorf("Expected %d answers, got %d", tt.expect, len(resp.Answers))
			}
		})
	}
}

func TestResolver_NXDOMAIN(t *testing.T) {
	zones := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zones.AddZone(zone)

	r := NewResolver(&ResolverConfig{CacheConfig: DefaultCacheConfig()}, zones)
	defer r.Close()

	msg := NewQuery("nonexistent.test.local", TypeA)
	resp, err := r.Resolve(context.Background(), msg)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	if resp.Header.Rcode() != RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got rcode %d", resp.Header.Rcode())
	}

	// Should have SOA in authority section
	if len(resp.Authority) == 0 {
		t.Error("Expected SOA in authority section for NXDOMAIN")
	}
}

func TestResolver_CacheHit(t *testing.T) {
	zones := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("cache.test"))
	zone.AddRecord(NewARecord("cached.cache.test", 300, net.ParseIP("10.0.0.1")))
	zones.AddZone(zone)

	r := NewResolver(&ResolverConfig{CacheConfig: DefaultCacheConfig()}, zones)
	defer r.Close()

	ctx := context.Background()

	// First query - should be cache miss
	msg1 := NewQuery("cached.cache.test", TypeA)
	_, _ = r.Resolve(ctx, msg1)

	initialMisses := atomic.LoadUint64(&r.stats.CacheMisses)

	// Second query - should be cache hit
	msg2 := NewQuery("cached.cache.test", TypeA)
	resp, _ := r.Resolve(ctx, msg2)

	if len(resp.Answers) != 1 {
		t.Error("Expected cached response with 1 answer")
	}

	hits := atomic.LoadUint64(&r.stats.CacheHits)
	if hits == 0 {
		t.Error("Expected cache hit on second query")
	}

	// Cache misses should not have increased
	currentMisses := atomic.LoadUint64(&r.stats.CacheMisses)
	if currentMisses > initialMisses {
		t.Error("Cache miss counter increased on cached query")
	}
}

func TestResolver_NegativeCaching(t *testing.T) {
	zones := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("negcache.test"))
	zones.AddZone(zone)

	config := &ResolverConfig{
		CacheConfig: &CacheConfig{
			MaxSize:         100,
			NegativeTTL:     time.Minute,
			CleanupInterval: time.Hour,
		},
	}
	r := NewResolver(config, zones)
	defer r.Close()

	ctx := context.Background()

	// First query for non-existent name
	msg1 := NewQuery("noexist.negcache.test", TypeA)
	resp1, _ := r.Resolve(ctx, msg1)
	if resp1.Header.Rcode() != RcodeNameError {
		t.Error("Expected NXDOMAIN on first query")
	}

	// Second query should be served from negative cache
	msg2 := NewQuery("noexist.negcache.test", TypeA)
	resp2, _ := r.Resolve(ctx, msg2)
	if resp2.Header.Rcode() != RcodeNameError {
		t.Error("Expected NXDOMAIN on cached query")
	}

	// Check that NXDOMAIN counter increased
	nxcount := atomic.LoadUint64(&r.stats.NXDOMAIN)
	if nxcount == 0 {
		t.Error("NXDOMAIN counter should be > 0")
	}
}

func TestResolver_Stats(t *testing.T) {
	zones := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("stats.test"))
	zone.AddRecord(NewARecord("host.stats.test", 300, net.ParseIP("10.0.0.1")))
	zones.AddZone(zone)

	r := NewResolver(&ResolverConfig{CacheConfig: DefaultCacheConfig()}, zones)
	defer r.Close()

	ctx := context.Background()

	// Make several queries
	for i := 0; i < 5; i++ {
		msg := NewQuery("host.stats.test", TypeA)
		r.Resolve(ctx, msg)
	}

	stats := r.Stats()
	if stats.Queries != 5 {
		t.Errorf("Expected 5 queries, got %d", stats.Queries)
	}
	if stats.LocalHits != 1 {
		t.Errorf("Expected 1 local hit (first query), got %d", stats.LocalHits)
	}
	if stats.CacheHits != 4 {
		t.Errorf("Expected 4 cache hits, got %d", stats.CacheHits)
	}
}

func TestResolver_SetUpstreams(t *testing.T) {
	r := NewResolver(nil, nil)
	defer r.Close()

	newUpstreams := []string{"9.9.9.9:53", "1.0.0.1:53"}
	r.SetUpstreams(newUpstreams)

	upstreams := r.GetUpstreams()
	if len(upstreams) != 2 {
		t.Fatalf("Expected 2 upstreams, got %d", len(upstreams))
	}
	if upstreams[0] != "9.9.9.9:53" {
		t.Errorf("Expected first upstream 9.9.9.9:53, got %s", upstreams[0])
	}
}

func TestResolver_FlushCache(t *testing.T) {
	zones := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("flush.test"))
	zone.AddRecord(NewARecord("host.flush.test", 300, net.ParseIP("10.0.0.1")))
	zones.AddZone(zone)

	r := NewResolver(&ResolverConfig{CacheConfig: DefaultCacheConfig()}, zones)
	defer r.Close()

	// Prime the cache
	msg := NewQuery("host.flush.test", TypeA)
	r.Resolve(context.Background(), msg)

	// Flush cache
	r.FlushCache()

	// Next query should be a cache miss
	initialMisses := atomic.LoadUint64(&r.stats.CacheMisses)
	r.Resolve(context.Background(), msg)
	newMisses := atomic.LoadUint64(&r.stats.CacheMisses)

	if newMisses <= initialMisses {
		t.Error("Expected cache miss after flush")
	}
}

func TestResolver_EmptyQuery(t *testing.T) {
	r := NewResolver(nil, nil)
	defer r.Close()

	msg := &Message{
		Header:    Header{ID: 1234},
		Questions: []Question{}, // Empty questions
	}

	resp, err := r.Resolve(context.Background(), msg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.Header.Rcode() != RcodeFormatError {
		t.Errorf("Expected FORMERR for empty query, got %d", resp.Header.Rcode())
	}
}

func TestResolver_NoUpstreams(t *testing.T) {
	config := &ResolverConfig{
		Upstreams:   []string{}, // No upstreams
		CacheConfig: DefaultCacheConfig(),
	}
	r := NewResolver(config, nil)
	defer r.Close()

	msg := NewQuery("example.com", TypeA)
	resp, err := r.Resolve(context.Background(), msg)

	// Should return error and SERVFAIL response
	if err != ErrNoUpstream {
		t.Errorf("Expected ErrNoUpstream, got %v", err)
	}
	if resp.Header.Rcode() != RcodeServerFailure {
		t.Errorf("Expected SERVFAIL, got %d", resp.Header.Rcode())
	}
}

func TestRecursiveResolver_New(t *testing.T) {
	rr := NewRecursiveResolver(nil)
	if rr == nil {
		t.Fatal("NewRecursiveResolver returned nil")
	}
	if len(rr.rootServers) == 0 {
		t.Error("Expected root servers to be configured")
	}
	if rr.maxDepth != 10 {
		t.Errorf("Expected max depth 10, got %d", rr.maxDepth)
	}
	rr.Close()
}

func TestParallelResolver_New(t *testing.T) {
	config := &ResolverConfig{
		Upstreams:   []string{"8.8.8.8:53", "8.8.4.4:53"},
		CacheConfig: DefaultCacheConfig(),
	}
	pr := NewParallelResolver(config, nil, 2)
	if pr == nil {
		t.Fatal("NewParallelResolver returned nil")
	}
	if pr.parallelism != 2 {
		t.Errorf("Expected parallelism 2, got %d", pr.parallelism)
	}
	pr.Close()
}

func TestConnectionPool(t *testing.T) {
	pool := newConnectionPool(2, time.Second)
	defer pool.close()

	// Get from empty pool
	conn := pool.get("test:53")
	if conn != nil {
		t.Error("Expected nil from empty pool")
	}

	// Create mock connections using pipe
	serverConn1, clientConn1 := net.Pipe()
	defer serverConn1.Close()

	serverConn2, clientConn2 := net.Pipe()
	defer serverConn2.Close()

	serverConn3, clientConn3 := net.Pipe()
	defer serverConn3.Close()

	// Put connections
	pool.put("test:53", clientConn1)
	pool.put("test:53", clientConn2)
	pool.put("test:53", clientConn3) // Should close this one (over limit)

	// Get connections back
	got1 := pool.get("test:53")
	if got1 == nil {
		t.Error("Expected connection from pool")
	}

	got2 := pool.get("test:53")
	if got2 == nil {
		t.Error("Expected second connection from pool")
	}

	// Should be empty now
	got3 := pool.get("test:53")
	if got3 != nil {
		t.Error("Expected nil from empty pool")
	}
}

func TestDefaultResolverConfig(t *testing.T) {
	config := DefaultResolverConfig()

	if len(config.Upstreams) != 2 {
		t.Errorf("Expected 2 upstreams, got %d", len(config.Upstreams))
	}
	if config.Timeout != 5*time.Second {
		t.Errorf("Expected 5s timeout, got %v", config.Timeout)
	}
	if config.MaxRetries != 2 {
		t.Errorf("Expected 2 max retries, got %d", config.MaxRetries)
	}
	if config.CacheConfig == nil {
		t.Error("Expected CacheConfig to be set")
	}
	if config.UseTCP {
		t.Error("Expected UseTCP to be false by default")
	}
}
