// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package proxy provides L4/L7 proxy functionality for THE HAUBERK.
package proxy

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrPoolClosed indicates the pool is closed.
	ErrPoolClosed = errors.New("connection pool closed")
	// ErrPoolExhausted indicates no connections available.
	ErrPoolExhausted = errors.New("connection pool exhausted")
	// ErrConnUnusable indicates the connection is unusable.
	ErrConnUnusable = errors.New("connection unusable")
)

// PooledConn wraps a connection for pool management.
type PooledConn struct {
	net.Conn
	pool       *ConnectionPool
	createdAt  time.Time
	lastUsedAt time.Time
	usageCount int64
	unusable   bool
}

// Close returns the connection to the pool or closes it.
func (pc *PooledConn) Close() error {
	if pc.unusable {
		return pc.Conn.Close()
	}
	return pc.pool.put(pc)
}

// MarkUnusable marks the connection as unusable.
func (pc *PooledConn) MarkUnusable() {
	pc.unusable = true
}

// ConnectionPool manages a pool of connections to a backend.
type ConnectionPool struct {
	mu sync.Mutex

	// Configuration
	address     string
	maxSize     int
	minSize     int
	maxLifetime time.Duration
	maxIdleTime time.Duration
	dialTimeout time.Duration

	// Dial function
	dial func(network, address string, timeout time.Duration) (net.Conn, error)

	// Pool state
	conns     []*PooledConn
	numOpen   int32
	waitQueue []chan *PooledConn
	closed    bool

	// Stats
	hits      uint64
	misses    uint64
	timeouts  uint64
	evictions uint64
}

// PoolConfig configures a connection pool.
type PoolConfig struct {
	Address     string
	MaxSize     int
	MinSize     int
	MaxLifetime time.Duration
	MaxIdleTime time.Duration
	DialTimeout time.Duration
	Dial        func(network, address string, timeout time.Duration) (net.Conn, error)
}

// DefaultPoolConfig returns a default pool configuration.
func DefaultPoolConfig(address string) *PoolConfig {
	return &PoolConfig{
		Address:     address,
		MaxSize:     100,
		MinSize:     5,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 5 * time.Minute,
		DialTimeout: 5 * time.Second,
	}
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(config *PoolConfig) *ConnectionPool {
	dial := config.Dial
	if dial == nil {
		// Default dialer for the connection pool. The address is the
		// configured backend address — operator-supplied via PoolConfig,
		// not user-supplied at request time. SSRF surface is gated upstream
		// at the load-balancer / mesh-policy layer, not here.
		dial = func(network, address string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout(network, address, timeout) // #nosec G704 -- configured backend address, not user-controlled
		}
	}

	pool := &ConnectionPool{
		address:     config.Address,
		maxSize:     config.MaxSize,
		minSize:     config.MinSize,
		maxLifetime: config.MaxLifetime,
		maxIdleTime: config.MaxIdleTime,
		dialTimeout: config.DialTimeout,
		dial:        dial,
		conns:       make([]*PooledConn, 0, config.MaxSize),
		waitQueue:   make([]chan *PooledConn, 0),
	}

	// Pre-populate with minimum connections
	go pool.ensureMinConnections()

	return pool
}

// Get retrieves a connection from the pool.
func (p *ConnectionPool) Get() (*PooledConn, error) {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}

	// Try to get an idle connection
	for len(p.conns) > 0 {
		// Pop from the end (LIFO for better cache locality)
		n := len(p.conns) - 1
		conn := p.conns[n]
		p.conns = p.conns[:n]

		// Check if connection is still valid
		if p.isValid(conn) {
			conn.lastUsedAt = time.Now()
			atomic.AddInt64(&conn.usageCount, 1)
			atomic.AddUint64(&p.hits, 1)
			p.mu.Unlock()
			return conn, nil
		}

		// Connection expired, close it
		_ = conn.Conn.Close()
		atomic.AddInt32(&p.numOpen, -1)
		atomic.AddUint64(&p.evictions, 1)
	}

	// No idle connections available
	atomic.AddUint64(&p.misses, 1)

	// Check if we can create a new connection
	if int(atomic.LoadInt32(&p.numOpen)) < p.maxSize {
		atomic.AddInt32(&p.numOpen, 1)
		p.mu.Unlock()
		return p.dialNew()
	}

	// Pool exhausted, wait for a connection
	ch := make(chan *PooledConn, 1)
	p.waitQueue = append(p.waitQueue, ch)
	p.mu.Unlock()

	select {
	case conn := <-ch:
		if conn == nil {
			return nil, ErrPoolClosed
		}
		return conn, nil
	case <-time.After(p.dialTimeout):
		atomic.AddUint64(&p.timeouts, 1)
		// Remove from wait queue
		p.mu.Lock()
		for i, c := range p.waitQueue {
			if c == ch {
				p.waitQueue = append(p.waitQueue[:i], p.waitQueue[i+1:]...)
				break
			}
		}
		p.mu.Unlock()
		return nil, ErrPoolExhausted
	}
}

// put returns a connection to the pool.
func (p *ConnectionPool) put(conn *PooledConn) error {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		atomic.AddInt32(&p.numOpen, -1)
		return conn.Conn.Close()
	}

	// Check if connection is still valid
	if !p.isValid(conn) {
		p.mu.Unlock()
		atomic.AddInt32(&p.numOpen, -1)
		return conn.Conn.Close()
	}

	// Check if there are waiters
	if len(p.waitQueue) > 0 {
		ch := p.waitQueue[0]
		p.waitQueue = p.waitQueue[1:]
		p.mu.Unlock()
		conn.lastUsedAt = time.Now()
		ch <- conn
		return nil
	}

	// Return to pool
	conn.lastUsedAt = time.Now()
	p.conns = append(p.conns, conn)
	p.mu.Unlock()

	return nil
}

// Close closes all connections and the pool.
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	// Close all idle connections
	for _, conn := range p.conns {
		_ = conn.Conn.Close()
	}
	p.conns = nil

	// Wake up all waiters
	for _, ch := range p.waitQueue {
		close(ch)
	}
	p.waitQueue = nil

	return nil
}

// Stats returns pool statistics.
func (p *ConnectionPool) Stats() PoolStats {
	p.mu.Lock()
	idle := len(p.conns)
	waiting := len(p.waitQueue)
	p.mu.Unlock()

	return PoolStats{
		NumOpen:   int(atomic.LoadInt32(&p.numOpen)),
		NumIdle:   idle,
		NumWait:   waiting,
		Hits:      atomic.LoadUint64(&p.hits),
		Misses:    atomic.LoadUint64(&p.misses),
		Timeouts:  atomic.LoadUint64(&p.timeouts),
		Evictions: atomic.LoadUint64(&p.evictions),
	}
}

func (p *ConnectionPool) dialNew() (*PooledConn, error) {
	conn, err := p.dial("tcp", p.address, p.dialTimeout)
	if err != nil {
		atomic.AddInt32(&p.numOpen, -1)
		return nil, err
	}

	return &PooledConn{
		Conn:       conn,
		pool:       p,
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
		usageCount: 1,
	}, nil
}

func (p *ConnectionPool) isValid(conn *PooledConn) bool {
	now := time.Now()

	// Check lifetime
	if p.maxLifetime > 0 && now.Sub(conn.createdAt) > p.maxLifetime {
		return false
	}

	// Check idle time
	if p.maxIdleTime > 0 && now.Sub(conn.lastUsedAt) > p.maxIdleTime {
		return false
	}

	return !conn.unusable
}

func (p *ConnectionPool) ensureMinConnections() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	for i := 0; i < p.minSize; i++ {
		p.mu.Lock()
		if p.closed || int(atomic.LoadInt32(&p.numOpen)) >= p.minSize {
			p.mu.Unlock()
			break
		}
		atomic.AddInt32(&p.numOpen, 1)
		p.mu.Unlock()

		conn, err := p.dialNew()
		if err != nil {
			atomic.AddInt32(&p.numOpen, -1)
			continue
		}

		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			_ = conn.Conn.Close()
			atomic.AddInt32(&p.numOpen, -1)
			break
		}
		p.conns = append(p.conns, conn)
		p.mu.Unlock()
	}
}

// PoolStats holds pool statistics.
type PoolStats struct {
	NumOpen   int
	NumIdle   int
	NumWait   int
	Hits      uint64
	Misses    uint64
	Timeouts  uint64
	Evictions uint64
}

// HitRate returns the cache hit rate.
func (s PoolStats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 1.0
	}
	return float64(s.Hits) / float64(total)
}

// PoolManager manages connection pools for multiple backends.
type PoolManager struct {
	mu       sync.RWMutex
	pools    map[string]*ConnectionPool
	config   *PoolConfig
	maxPools int
}

// NewPoolManager creates a new pool manager.
func NewPoolManager(defaultConfig *PoolConfig, maxPools int) *PoolManager {
	return &PoolManager{
		pools:    make(map[string]*ConnectionPool),
		config:   defaultConfig,
		maxPools: maxPools,
	}
}

// Get gets a connection from the pool for the given address.
func (pm *PoolManager) Get(address string) (*PooledConn, error) {
	pool := pm.getOrCreatePool(address)
	return pool.Get()
}

// GetPool returns the pool for an address.
func (pm *PoolManager) GetPool(address string) *ConnectionPool {
	return pm.getOrCreatePool(address)
}

func (pm *PoolManager) getOrCreatePool(address string) *ConnectionPool {
	pm.mu.RLock()
	pool, ok := pm.pools[address]
	pm.mu.RUnlock()

	if ok {
		return pool
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double-check after acquiring write lock
	if pool, ok := pm.pools[address]; ok {
		return pool
	}

	// Evict oldest pool if at limit
	if pm.maxPools > 0 && len(pm.pools) >= pm.maxPools {
		pm.evictOldest()
	}

	// Create new pool
	config := *pm.config
	config.Address = address
	pool = NewConnectionPool(&config)
	pm.pools[address] = pool

	return pool
}

func (pm *PoolManager) evictOldest() {
	// Simple eviction: just remove the first one
	// In production, you'd want LRU or similar
	for addr, pool := range pm.pools {
		_ = pool.Close()
		delete(pm.pools, addr)
		break
	}
}

// Close closes all pools.
func (pm *PoolManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for addr, pool := range pm.pools {
		_ = pool.Close()
		delete(pm.pools, addr)
	}

	return nil
}

// Stats returns statistics for all pools.
func (pm *PoolManager) Stats() map[string]PoolStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make(map[string]PoolStats, len(pm.pools))
	for addr, pool := range pm.pools {
		stats[addr] = pool.Stats()
	}

	return stats
}
