// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dns

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Resolver errors
var (
	ErrNoUpstream     = errors.New("dns: no upstream servers configured")
	ErrAllUpstreamsFailed = errors.New("dns: all upstream servers failed")
	ErrTimeout        = errors.New("dns: query timeout")
	ErrCanceled       = errors.New("dns: query canceled")
)

// Resolver handles DNS resolution with forwarding and caching
type Resolver struct {
	mu          sync.RWMutex
	zones       *ZoneManager
	cache       *Cache
	upstreams   []string
	timeout     time.Duration
	maxRetries  int
	roundRobin  uint32
	useTCP      bool
	pool        *connectionPool
	stats       ResolverStats
}

// ResolverConfig holds resolver configuration
type ResolverConfig struct {
	Upstreams   []string      // Upstream DNS servers
	Timeout     time.Duration // Query timeout
	MaxRetries  int           // Maximum retry attempts
	CacheConfig *CacheConfig  // Cache configuration
	UseTCP      bool          // Prefer TCP over UDP
}

// DefaultResolverConfig returns default resolver configuration
func DefaultResolverConfig() *ResolverConfig {
	return &ResolverConfig{
		Upstreams:   []string{"8.8.8.8:53", "8.8.4.4:53"},
		Timeout:     5 * time.Second,
		MaxRetries:  2,
		CacheConfig: DefaultCacheConfig(),
		UseTCP:      false,
	}
}

// ResolverStats holds resolver statistics
type ResolverStats struct {
	Queries      uint64
	CacheHits    uint64
	CacheMisses  uint64
	LocalHits    uint64
	Forwards     uint64
	ForwardFails uint64
	NXDOMAIN     uint64
}

// NewResolver creates a new DNS resolver
func NewResolver(config *ResolverConfig, zones *ZoneManager) *Resolver {
	if config == nil {
		config = DefaultResolverConfig()
	}

	r := &Resolver{
		zones:      zones,
		cache:      NewCache(config.CacheConfig),
		upstreams:  config.Upstreams,
		timeout:    config.Timeout,
		maxRetries: config.MaxRetries,
		useTCP:     config.UseTCP,
		pool:       newConnectionPool(10, config.Timeout),
	}

	return r
}

// Resolve resolves a DNS query
func (r *Resolver) Resolve(ctx context.Context, msg *Message) (*Message, error) {
	atomic.AddUint64(&r.stats.Queries, 1)

	if len(msg.Questions) == 0 {
		return r.errorResponse(msg, RcodeFormatError), nil
	}

	q := msg.Questions[0]

	// Check cache first
	if cached, ok := r.cache.Get(q.Name, q.Type, q.Class); ok {
		atomic.AddUint64(&r.stats.CacheHits, 1)
		cached.Header.ID = msg.Header.ID
		return cached, nil
	}
	atomic.AddUint64(&r.stats.CacheMisses, 1)

	// Try local zones
	if r.zones != nil {
		records, zone, exists := r.zones.Lookup(q.Name, q.Type)
		if zone != nil {
			atomic.AddUint64(&r.stats.LocalHits, 1)
			return r.buildLocalResponse(msg, records, zone, exists), nil
		}
	}

	// Forward to upstream
	return r.forward(ctx, msg)
}

// ResolveQuery resolves a simple query by name and type
func (r *Resolver) ResolveQuery(ctx context.Context, name string, qtype uint16) (*Message, error) {
	msg := NewQuery(name, qtype)
	return r.Resolve(ctx, msg)
}

// buildLocalResponse builds a response from local zone data
func (r *Resolver) buildLocalResponse(query *Message, records []*ResourceRecord, zone *Zone, exists bool) *Message {
	resp := NewResponse(query)

	if !exists {
		// NXDOMAIN
		resp.Header.SetRcode(RcodeNameError)
		atomic.AddUint64(&r.stats.NXDOMAIN, 1)

		// Add SOA to authority section
		if zone != nil {
			resp.Authority = append(resp.Authority, *zone.GetSOA())
		}

		// Cache negative response
		r.cache.Set(resp)
		return resp
	}

	if len(records) == 0 {
		// NODATA - name exists but no records of requested type
		if zone != nil {
			resp.Authority = append(resp.Authority, *zone.GetSOA())
		}
		return resp
	}

	// Add answers
	for _, rr := range records {
		resp.Answers = append(resp.Answers, *rr)
	}

	// Cache positive response
	r.cache.Set(resp)

	return resp
}

// forward forwards a query to upstream servers
func (r *Resolver) forward(ctx context.Context, msg *Message) (*Message, error) {
	r.mu.RLock()
	upstreams := r.upstreams
	r.mu.RUnlock()

	if len(upstreams) == 0 {
		return r.errorResponse(msg, RcodeServerFailure), ErrNoUpstream
	}

	atomic.AddUint64(&r.stats.Forwards, 1)

	// Pack the query
	queryData, err := msg.Pack()
	if err != nil {
		return r.errorResponse(msg, RcodeServerFailure), err
	}

	// Round-robin selection with fallback
	startIdx := int(atomic.AddUint32(&r.roundRobin, 1)) % len(upstreams)

	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		for i := 0; i < len(upstreams); i++ {
			idx := (startIdx + i) % len(upstreams)
			upstream := upstreams[idx]

			resp, err := r.forwardTo(ctx, upstream, queryData)
			if err != nil {
				lastErr = err
				continue
			}

			// Cache the response
			r.cache.Set(resp)

			if resp.Header.Rcode() == RcodeNameError {
				atomic.AddUint64(&r.stats.NXDOMAIN, 1)
			}

			return resp, nil
		}
	}

	atomic.AddUint64(&r.stats.ForwardFails, 1)
	return r.errorResponse(msg, RcodeServerFailure), lastErr
}

// forwardTo forwards a query to a specific upstream server
func (r *Resolver) forwardTo(ctx context.Context, upstream string, queryData []byte) (*Message, error) {
	if r.useTCP {
		return r.forwardTCP(ctx, upstream, queryData)
	}
	return r.forwardUDP(ctx, upstream, queryData)
}

// forwardUDP forwards a query via UDP
func (r *Resolver) forwardUDP(ctx context.Context, upstream string, queryData []byte) (*Message, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// Dial upstream
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", upstream)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Set deadline
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Send query
	if _, err := conn.Write(queryData); err != nil {
		return nil, err
	}

	// Read response
	respData := make([]byte, MaxUDPSize)
	n, err := conn.Read(respData)
	if err != nil {
		return nil, err
	}

	// Parse response
	resp := &Message{}
	if err := resp.Unpack(respData[:n]); err != nil {
		return nil, err
	}

	// Check for truncation
	if resp.Header.Flags&FlagTC != 0 {
		// Retry with TCP
		return r.forwardTCP(ctx, upstream, queryData)
	}

	return resp, nil
}

// forwardTCP forwards a query via TCP
func (r *Resolver) forwardTCP(ctx context.Context, upstream string, queryData []byte) (*Message, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// Dial upstream
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", upstream)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Set deadline
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// TCP DNS uses 2-byte length prefix
	tcpData := make([]byte, 2+len(queryData))
	tcpData[0] = byte(len(queryData) >> 8)
	tcpData[1] = byte(len(queryData))
	copy(tcpData[2:], queryData)

	// Send query
	if _, err := conn.Write(tcpData); err != nil {
		return nil, err
	}

	// Read length prefix
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}

	respLen := int(lenBuf[0])<<8 | int(lenBuf[1])
	if respLen > 65535 {
		return nil, ErrInvalidMessage
	}

	// Read response
	respData := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respData); err != nil {
		return nil, err
	}

	// Parse response
	resp := &Message{}
	if err := resp.Unpack(respData); err != nil {
		return nil, err
	}

	return resp, nil
}

// errorResponse creates an error response
func (r *Resolver) errorResponse(query *Message, rcode uint8) *Message {
	resp := NewResponse(query)
	resp.Header.SetRcode(rcode)
	return resp
}

// SetUpstreams updates the upstream servers
func (r *Resolver) SetUpstreams(upstreams []string) {
	r.mu.Lock()
	r.upstreams = upstreams
	r.mu.Unlock()
}

// GetUpstreams returns the current upstream servers
func (r *Resolver) GetUpstreams() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, len(r.upstreams))
	copy(result, r.upstreams)
	return result
}

// Stats returns resolver statistics
func (r *Resolver) Stats() ResolverStats {
	return ResolverStats{
		Queries:      atomic.LoadUint64(&r.stats.Queries),
		CacheHits:    atomic.LoadUint64(&r.stats.CacheHits),
		CacheMisses:  atomic.LoadUint64(&r.stats.CacheMisses),
		LocalHits:    atomic.LoadUint64(&r.stats.LocalHits),
		Forwards:     atomic.LoadUint64(&r.stats.Forwards),
		ForwardFails: atomic.LoadUint64(&r.stats.ForwardFails),
		NXDOMAIN:     atomic.LoadUint64(&r.stats.NXDOMAIN),
	}
}

// FlushCache flushes the resolver cache
func (r *Resolver) FlushCache() {
	r.cache.Flush()
}

// Close closes the resolver
func (r *Resolver) Close() error {
	r.cache.Stop()
	r.pool.close()
	return nil
}

// connectionPool manages reusable connections
type connectionPool struct {
	mu       sync.Mutex
	conns    map[string][]net.Conn
	maxConns int
	timeout  time.Duration
}

func newConnectionPool(maxConns int, timeout time.Duration) *connectionPool {
	return &connectionPool{
		conns:    make(map[string][]net.Conn),
		maxConns: maxConns,
		timeout:  timeout,
	}
}

func (p *connectionPool) get(addr string) net.Conn {
	p.mu.Lock()
	defer p.mu.Unlock()

	if conns := p.conns[addr]; len(conns) > 0 {
		conn := conns[len(conns)-1]
		p.conns[addr] = conns[:len(conns)-1]
		return conn
	}
	return nil
}

func (p *connectionPool) put(addr string, conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.conns[addr]) >= p.maxConns {
		conn.Close()
		return
	}
	p.conns[addr] = append(p.conns[addr], conn)
}

func (p *connectionPool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conns := range p.conns {
		for _, conn := range conns {
			conn.Close()
		}
	}
	p.conns = make(map[string][]net.Conn)
}

// RecursiveResolver performs recursive DNS resolution
type RecursiveResolver struct {
	*Resolver
	rootServers []string
	maxDepth    int
}

// NewRecursiveResolver creates a new recursive resolver
func NewRecursiveResolver(zones *ZoneManager) *RecursiveResolver {
	config := DefaultResolverConfig()
	config.Upstreams = nil // No upstreams for recursive resolver

	return &RecursiveResolver{
		Resolver: NewResolver(config, zones),
		rootServers: []string{
			"198.41.0.4:53",   // a.root-servers.net
			"199.9.14.201:53", // b.root-servers.net
			"192.33.4.12:53",  // c.root-servers.net
			"199.7.91.13:53",  // d.root-servers.net
		},
		maxDepth: 10,
	}
}

// ResolveRecursive performs a recursive DNS lookup
func (rr *RecursiveResolver) ResolveRecursive(ctx context.Context, name string, qtype uint16) (*Message, error) {
	return rr.resolveRecursiveWithDepth(ctx, name, qtype, rr.rootServers, 0)
}

func (rr *RecursiveResolver) resolveRecursiveWithDepth(ctx context.Context, name string, qtype uint16, servers []string, depth int) (*Message, error) {
	if depth > rr.maxDepth {
		return nil, errors.New("dns: max recursion depth exceeded")
	}

	// Check context
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Build query
	query := NewQuery(name, qtype)
	queryData, err := query.Pack()
	if err != nil {
		return nil, err
	}

	// Try each server
	for _, server := range servers {
		resp, err := rr.forwardUDP(ctx, server, queryData)
		if err != nil {
			continue
		}

		// Check for answer
		if len(resp.Answers) > 0 {
			return resp, nil
		}

		// Check for NXDOMAIN
		if resp.Header.Rcode() == RcodeNameError {
			return resp, nil
		}

		// Get referral from authority section
		var nextServers []string
		for _, rr := range resp.Authority {
			if rr.Type == TypeNS {
				// Look for glue records
				target := rr.GetTarget()
				for _, ar := range resp.Additional {
					if ar.Type == TypeA && ar.Name == target {
						nextServers = append(nextServers, ar.GetIP().String()+":53")
					}
				}
			}
		}

		if len(nextServers) > 0 {
			return rr.resolveRecursiveWithDepth(ctx, name, qtype, nextServers, depth+1)
		}
	}

	return nil, ErrAllUpstreamsFailed
}

// ParallelResolver resolves queries in parallel across multiple upstreams
type ParallelResolver struct {
	*Resolver
	parallelism int
}

// NewParallelResolver creates a new parallel resolver
func NewParallelResolver(config *ResolverConfig, zones *ZoneManager, parallelism int) *ParallelResolver {
	return &ParallelResolver{
		Resolver:    NewResolver(config, zones),
		parallelism: parallelism,
	}
}

// ResolveParallel resolves a query using multiple upstreams in parallel
func (pr *ParallelResolver) ResolveParallel(ctx context.Context, msg *Message) (*Message, error) {
	pr.mu.RLock()
	upstreams := pr.upstreams
	pr.mu.RUnlock()

	if len(upstreams) == 0 {
		return pr.errorResponse(msg, RcodeServerFailure), ErrNoUpstream
	}

	// Check cache first
	if len(msg.Questions) > 0 {
		q := msg.Questions[0]
		if cached, ok := pr.cache.Get(q.Name, q.Type, q.Class); ok {
			cached.Header.ID = msg.Header.ID
			return cached, nil
		}
	}

	queryData, err := msg.Pack()
	if err != nil {
		return pr.errorResponse(msg, RcodeServerFailure), err
	}

	// Query upstreams in parallel
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan *Message, len(upstreams))
	errs := make(chan error, len(upstreams))

	for _, upstream := range upstreams {
		go func(us string) {
			resp, err := pr.forwardUDP(ctx, us, queryData)
			if err != nil {
				errs <- err
				return
			}
			results <- resp
		}(upstream)
	}

	// Wait for first successful response
	var lastErr error
	for i := 0; i < len(upstreams); i++ {
		select {
		case resp := <-results:
			pr.cache.Set(resp)
			return resp, nil
		case err := <-errs:
			lastErr = err
		case <-ctx.Done():
			return pr.errorResponse(msg, RcodeServerFailure), ctx.Err()
		}
	}

	return pr.errorResponse(msg, RcodeServerFailure), lastErr
}
