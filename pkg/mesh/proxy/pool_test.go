// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package proxy

import (
	"net"
	"sync"
	"testing"
	"time"
)

// ==================== PoolConfig Tests ====================

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig("backend:8080")

	if cfg.Address != "backend:8080" {
		t.Errorf("Address = %q, want %q", cfg.Address, "backend:8080")
	}
	if cfg.MaxSize != 100 {
		t.Errorf("MaxSize = %d, want 100", cfg.MaxSize)
	}
	if cfg.MinSize != 5 {
		t.Errorf("MinSize = %d, want 5", cfg.MinSize)
	}
	if cfg.MaxLifetime != 30*time.Minute {
		t.Errorf("MaxLifetime = %v, want %v", cfg.MaxLifetime, 30*time.Minute)
	}
	if cfg.MaxIdleTime != 5*time.Minute {
		t.Errorf("MaxIdleTime = %v, want %v", cfg.MaxIdleTime, 5*time.Minute)
	}
	if cfg.DialTimeout != 5*time.Second {
		t.Errorf("DialTimeout = %v, want %v", cfg.DialTimeout, 5*time.Second)
	}
}

// ==================== ConnectionPool Tests ====================

// mockDialer returns a dial function that creates pipe connections
func mockDialer(t *testing.T) (func(network, address string, timeout time.Duration) (net.Conn, error), func()) {
	t.Helper()
	var mu sync.Mutex
	var serverConns []net.Conn

	dial := func(network, address string, timeout time.Duration) (net.Conn, error) {
		client, server := net.Pipe()
		mu.Lock()
		serverConns = append(serverConns, server)
		mu.Unlock()
		return client, nil
	}

	cleanup := func() {
		mu.Lock()
		for _, c := range serverConns {
			c.Close()
		}
		mu.Unlock()
	}

	return dial, cleanup
}

func TestNewConnectionPool(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0, // disable pre-populating for simpler test
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	if pool.address != "test:8080" {
		t.Errorf("address = %q, want %q", pool.address, "test:8080")
	}
	if pool.maxSize != 10 {
		t.Errorf("maxSize = %d, want 10", pool.maxSize)
	}
}

func TestConnectionPool_GetAndClose(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	conn, err := pool.Get()
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if conn == nil {
		t.Fatal("Get returned nil connection")
	}

	// Close should return to pool
	err = conn.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	stats := pool.Stats()
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}
}

func TestConnectionPool_GetFromPool(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	// Get and return a connection
	conn1, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}
	conn1.Close() // returns to pool

	// Get again - should be a cache hit
	conn2, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()

	stats := pool.Stats()
	if stats.Hits != 1 {
		t.Errorf("Hits = %d, want 1", stats.Hits)
	}
}

func TestConnectionPool_MarkUnusable(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	conn, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}

	conn.MarkUnusable()
	conn.Close() // should actually close, not return to pool

	stats := pool.Stats()
	if stats.NumIdle != 0 {
		t.Errorf("NumIdle = %d, want 0 (unusable conn should not return to pool)", stats.NumIdle)
	}
}

func TestConnectionPool_ClosedPool(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	pool.Close()

	_, err := pool.Get()
	if err != ErrPoolClosed {
		t.Errorf("Get after Close returned %v, want ErrPoolClosed", err)
	}

	// Double close should be fine
	err = pool.Close()
	if err != nil {
		t.Errorf("double Close returned error: %v", err)
	}
}

func TestConnectionPool_PutToClosedPool(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)

	conn, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}

	pool.Close()

	// Putting connection back to closed pool should close the connection
	err = conn.Close()
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestConnectionPool_ExpiredLifetime(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 1 * time.Millisecond, // very short lifetime
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	conn, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}
	conn.Close() // return to pool

	// Wait for connection to expire
	time.Sleep(5 * time.Millisecond)

	// Next Get should create a new connection (expired one evicted)
	conn2, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()

	stats := pool.Stats()
	if stats.Evictions == 0 {
		t.Error("expected at least 1 eviction for expired connection")
	}
}

func TestConnectionPool_ExpiredIdleTime(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 1 * time.Millisecond, // very short idle time
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	conn, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	time.Sleep(5 * time.Millisecond)

	conn2, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()

	stats := pool.Stats()
	if stats.Evictions == 0 {
		t.Error("expected eviction for idle-expired connection")
	}
}

func TestConnectionPool_Stats(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	stats := pool.Stats()
	if stats.NumOpen != 0 {
		t.Errorf("initial NumOpen = %d, want 0", stats.NumOpen)
	}

	conn, _ := pool.Get()
	stats = pool.Stats()
	if stats.NumOpen != 1 {
		t.Errorf("NumOpen after Get = %d, want 1", stats.NumOpen)
	}
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}

	conn.Close()
	stats = pool.Stats()
	if stats.NumIdle != 1 {
		t.Errorf("NumIdle after put = %d, want 1", stats.NumIdle)
	}
}

func TestConnectionPool_ConcurrentAccess(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     20,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := pool.Get()
			if err != nil {
				return
			}
			time.Sleep(time.Millisecond)
			conn.Close()
		}()
	}
	wg.Wait()

	stats := pool.Stats()
	if stats.Misses+stats.Hits == 0 {
		t.Error("expected some gets to be recorded")
	}
}

// ==================== PoolStats Tests ====================

func TestPoolStats_HitRate(t *testing.T) {
	tests := []struct {
		name   string
		stats  PoolStats
		expect float64
	}{
		{"no traffic", PoolStats{Hits: 0, Misses: 0}, 1.0},
		{"all hits", PoolStats{Hits: 10, Misses: 0}, 1.0},
		{"all misses", PoolStats{Hits: 0, Misses: 10}, 0.0},
		{"50/50", PoolStats{Hits: 5, Misses: 5}, 0.5},
		{"80/20", PoolStats{Hits: 80, Misses: 20}, 0.8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stats.HitRate()
			if got != tt.expect {
				t.Errorf("HitRate() = %f, want %f", got, tt.expect)
			}
		})
	}
}

// ==================== PooledConn Tests ====================

func TestPooledConn_Close_ReturnsToPool(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	conn, _ := pool.Get()

	// Check initial state
	if conn.unusable {
		t.Error("new connection should not be unusable")
	}

	// Close returns to pool
	conn.Close()

	stats := pool.Stats()
	if stats.NumIdle != 1 {
		t.Errorf("NumIdle = %d, want 1 after returning conn", stats.NumIdle)
	}
}

func TestPooledConn_MarkUnusable_ThenClose(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	conn, _ := pool.Get()
	conn.MarkUnusable()

	if !conn.unusable {
		t.Error("connection should be marked unusable")
	}

	conn.Close()

	stats := pool.Stats()
	if stats.NumIdle != 0 {
		t.Errorf("NumIdle = %d, want 0 (unusable conn should be closed)", stats.NumIdle)
	}
}

// ==================== PoolManager Tests ====================

func TestNewPoolManager(t *testing.T) {
	cfg := &PoolConfig{
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
	}

	pm := NewPoolManager(cfg, 5)
	defer pm.Close()

	if pm.maxPools != 5 {
		t.Errorf("maxPools = %d, want 5", pm.maxPools)
	}
}

func TestPoolManager_GetPool(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pm := NewPoolManager(cfg, 10)
	defer pm.Close()

	pool1 := pm.GetPool("addr1:8080")
	pool2 := pm.GetPool("addr1:8080")

	if pool1 != pool2 {
		t.Error("GetPool should return same pool for same address")
	}

	pool3 := pm.GetPool("addr2:8080")
	if pool1 == pool3 {
		t.Error("GetPool should return different pool for different address")
	}
}

func TestPoolManager_Get(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pm := NewPoolManager(cfg, 10)
	defer pm.Close()

	conn, err := pm.Get("addr1:8080")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if conn == nil {
		t.Fatal("Get returned nil")
	}
	conn.Close()
}

func TestPoolManager_EvictsOldest(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pm := NewPoolManager(cfg, 2) // max 2 pools
	defer pm.Close()

	// Create 2 pools
	pm.GetPool("addr1:8080")
	pm.GetPool("addr2:8080")

	// Creating a 3rd should evict one
	pm.GetPool("addr3:8080")

	stats := pm.Stats()
	if len(stats) > 2 {
		t.Errorf("expected at most 2 pools, got %d", len(stats))
	}
}

func TestPoolManager_Close(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pm := NewPoolManager(cfg, 10)

	pm.GetPool("addr1:8080")
	pm.GetPool("addr2:8080")

	err := pm.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	stats := pm.Stats()
	if len(stats) != 0 {
		t.Errorf("expected 0 pools after close, got %d", len(stats))
	}
}

func TestPoolManager_Stats(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		MaxSize:     10,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pm := NewPoolManager(cfg, 10)
	defer pm.Close()

	pm.GetPool("addr1:8080")
	pm.GetPool("addr2:8080")

	stats := pm.Stats()
	if len(stats) != 2 {
		t.Errorf("Stats returned %d entries, want 2", len(stats))
	}
	if _, ok := stats["addr1:8080"]; !ok {
		t.Error("Stats missing addr1:8080")
	}
	if _, ok := stats["addr2:8080"]; !ok {
		t.Error("Stats missing addr2:8080")
	}
}

func TestPoolManager_ConcurrentAccess(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		MaxSize:     20,
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 1 * time.Second,
		Dial:        dial,
	}

	pm := NewPoolManager(cfg, 50)
	defer pm.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			addr := "addr" + itoa(idx%5) + ":8080"
			conn, err := pm.Get(addr)
			if err != nil {
				return
			}
			conn.Close()
		}(i)
	}
	wg.Wait()
}

// ==================== Error Variables Tests ====================

func TestPoolErrorVariables(t *testing.T) {
	if ErrPoolClosed.Error() != "connection pool closed" {
		t.Errorf("ErrPoolClosed = %q", ErrPoolClosed.Error())
	}
	if ErrPoolExhausted.Error() != "connection pool exhausted" {
		t.Errorf("ErrPoolExhausted = %q", ErrPoolExhausted.Error())
	}
	if ErrConnUnusable.Error() != "connection unusable" {
		t.Errorf("ErrConnUnusable = %q", ErrConnUnusable.Error())
	}
}

// ==================== ConnectionPool Waiter Tests ====================

func TestConnectionPool_WaiterGetsConnection(t *testing.T) {
	dial, cleanup := mockDialer(t)
	defer cleanup()

	cfg := &PoolConfig{
		Address:     "test:8080",
		MaxSize:     1, // only 1 connection allowed
		MinSize:     0,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 2 * time.Second,
		Dial:        dial,
	}

	pool := NewConnectionPool(cfg)
	defer pool.Close()

	// Get the only connection
	conn1, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}

	// Start a goroutine that waits for a connection
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn2, err := pool.Get()
		if err != nil {
			t.Errorf("waiter Get failed: %v", err)
			return
		}
		conn2.Close()
	}()

	// Return first connection after a short delay
	time.Sleep(50 * time.Millisecond)
	conn1.Close()

	select {
	case <-done:
		// Success
	case <-time.After(3 * time.Second):
		t.Error("waiter did not get connection in time")
	}
}
