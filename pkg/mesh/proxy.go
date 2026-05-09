// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package mesh provides THE HAUBERK - the service mesh implementation for the Unheaded Kingdom.
// This file implements the transparent sidecar proxy for inbound and outbound traffic.
package mesh

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Proxy errors.
var (
	ErrProxyNotStarted = errors.New("proxy not started")
	ErrProxyClosed     = errors.New("proxy closed")
	ErrNoBackend       = errors.New("no backend available")
	ErrProxyTimeout    = errors.New("proxy timeout")
)

// Request represents a mesh request.
type Request struct {
	Method    string
	Path      string
	Service   string
	Namespace string
	Headers   http.Header
	Body      io.Reader
	Timeout   time.Duration
	TraceID   string
	ClientIP  string
}

// Response represents a mesh response.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Endpoint   *Endpoint
	Duration   time.Duration
}

// ProxyRequest represents a proxied request.
type ProxyRequest struct {
	Method     string
	Path       string
	Host       string
	Headers    http.Header
	Body       io.Reader
	RemoteAddr string
	TLS        *tls.ConnectionState
	StartTime  time.Time
}

// ProxyResponse represents a proxied response.
type ProxyResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Backend    string
	Duration   time.Duration
	Error      error
}

// InboundProxyConfig configures the inbound proxy.
type InboundProxyConfig struct {
	Address         string
	Port            int
	TLSConfig       *tls.Config
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	MaxHeaderBytes  int
	ReadBufferSize  int
	WriteBufferSize int
	Handler         func(context.Context, *ProxyRequest) (*ProxyResponse, error)
}

// DefaultInboundProxyConfig returns default inbound proxy configuration.
func DefaultInboundProxyConfig() *InboundProxyConfig {
	return &InboundProxyConfig{
		Address:         "127.0.0.1",
		Port:            15001,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     90 * time.Second,
		MaxHeaderBytes:  1 << 20, // 1MB
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
	}
}

// InboundProxy handles inbound connections to the sidecar.
type InboundProxy struct {
	config   *InboundProxyConfig
	listener net.Listener
	server   *http.Server

	mu      sync.RWMutex
	started bool
	closed  bool
	wg      sync.WaitGroup

	// Stats
	activeConns   int64
	totalConns    uint64
	totalRequests uint64
	totalBytes    uint64
}

// NewInboundProxy creates a new inbound proxy.
func NewInboundProxy(config *InboundProxyConfig) *InboundProxy {
	if config == nil {
		config = DefaultInboundProxyConfig()
	}

	p := &InboundProxy{
		config: config,
	}

	// Create HTTP server
	p.server = &http.Server{
		Handler:        http.HandlerFunc(p.handleHTTP),
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		IdleTimeout:    config.IdleTimeout,
		MaxHeaderBytes: config.MaxHeaderBytes,
	}

	return p
}

// Start starts the inbound proxy.
func (p *InboundProxy) Start() error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return errors.New("proxy already started")
	}
	if p.closed {
		p.mu.Unlock()
		return ErrProxyClosed
	}
	p.mu.Unlock()

	// Create listener
	addr := net.JoinHostPort(p.config.Address, strconv.Itoa(p.config.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// Wrap with TLS if configured
	if p.config.TLSConfig != nil {
		listener = tls.NewListener(listener, p.config.TLSConfig)
	}

	p.listener = listener

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	// Start serving
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.server.Serve(listener)
	}()

	return nil
}

// Stop stops the inbound proxy.
func (p *InboundProxy) Stop() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.started = false
	p.mu.Unlock()

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := p.server.Shutdown(ctx); err != nil {
		p.server.Close()
	}

	p.wg.Wait()
	return nil
}

// handleHTTP handles HTTP requests.
func (p *InboundProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&p.totalRequests, 1)
	startTime := time.Now()

	// Build proxy request
	req := &ProxyRequest{
		Method:     r.Method,
		Path:       r.URL.Path,
		Host:       r.Host,
		Headers:    r.Header.Clone(),
		Body:       r.Body,
		RemoteAddr: r.RemoteAddr,
		TLS:        r.TLS,
		StartTime:  startTime,
	}

	// Call handler
	if p.config.Handler == nil {
		http.Error(w, "no handler configured", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	resp, err := p.config.Handler(ctx, req)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Write response headers
	for k, v := range resp.Headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Write body
	if len(resp.Body) > 0 {
		w.Write(resp.Body)
		atomic.AddUint64(&p.totalBytes, uint64(len(resp.Body)))
	}
}

// Stats returns proxy statistics.
func (p *InboundProxy) Stats() InboundProxyStats {
	return InboundProxyStats{
		ActiveConnections: atomic.LoadInt64(&p.activeConns),
		TotalConnections:  atomic.LoadUint64(&p.totalConns),
		TotalRequests:     atomic.LoadUint64(&p.totalRequests),
		TotalBytes:        atomic.LoadUint64(&p.totalBytes),
	}
}

// InboundProxyStats holds inbound proxy statistics.
type InboundProxyStats struct {
	ActiveConnections int64
	TotalConnections  uint64
	TotalRequests     uint64
	TotalBytes        uint64
}

// OutboundProxyConfig configures the outbound proxy.
type OutboundProxyConfig struct {
	Address         string
	Port            int
	TLSConfig       *tls.Config
	ConnectTimeout  time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ReadBufferSize  int
	WriteBufferSize int
	Handler         func(context.Context, *ProxyRequest) (*ProxyResponse, error)
}

// DefaultOutboundProxyConfig returns default outbound proxy configuration.
func DefaultOutboundProxyConfig() *OutboundProxyConfig {
	return &OutboundProxyConfig{
		Address:         "127.0.0.1",
		Port:            15006,
		ConnectTimeout:  5 * time.Second,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     90 * time.Second,
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
	}
}

// OutboundProxy handles outbound connections from the sidecar.
type OutboundProxy struct {
	config   *OutboundProxyConfig
	listener net.Listener

	mu      sync.RWMutex
	started bool
	closed  bool
	wg      sync.WaitGroup

	// Connection pool
	pools   map[string]*ConnectionPool
	poolsMu sync.RWMutex

	// Stats
	activeConns   int64
	totalConns    uint64
	totalRequests uint64
	totalBytes    uint64
}

// NewOutboundProxy creates a new outbound proxy.
func NewOutboundProxy(config *OutboundProxyConfig) *OutboundProxy {
	if config == nil {
		config = DefaultOutboundProxyConfig()
	}

	return &OutboundProxy{
		config: config,
		pools:  make(map[string]*ConnectionPool),
	}
}

// Start starts the outbound proxy.
func (p *OutboundProxy) Start() error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return errors.New("proxy already started")
	}
	if p.closed {
		p.mu.Unlock()
		return ErrProxyClosed
	}
	p.mu.Unlock()

	// Create listener
	addr := net.JoinHostPort(p.config.Address, strconv.Itoa(p.config.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	p.listener = listener

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	// Start accepting connections
	p.wg.Add(1)
	go p.acceptLoop()

	return nil
}

// Stop stops the outbound proxy.
func (p *OutboundProxy) Stop() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.started = false
	p.mu.Unlock()

	// Close listener
	if p.listener != nil {
		p.listener.Close()
	}

	// Close all connection pools
	p.poolsMu.Lock()
	for _, pool := range p.pools {
		pool.Close()
	}
	p.poolsMu.Unlock()

	p.wg.Wait()
	return nil
}

// acceptLoop accepts incoming connections.
func (p *OutboundProxy) acceptLoop() {
	defer p.wg.Done()

	for {
		conn, err := p.listener.Accept()
		if err != nil {
			p.mu.RLock()
			closed := p.closed
			p.mu.RUnlock()
			if closed {
				return
			}
			continue
		}

		atomic.AddInt64(&p.activeConns, 1)
		atomic.AddUint64(&p.totalConns, 1)

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer atomic.AddInt64(&p.activeConns, -1)
			p.handleConnection(conn)
		}()
	}
}

// handleConnection handles a single connection.
func (p *OutboundProxy) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set timeouts
	conn.SetDeadline(time.Now().Add(p.config.IdleTimeout))

	// Read and proxy data
	reader := bufio.NewReaderSize(conn, p.config.ReadBufferSize)

	// Detect protocol
	peek, err := reader.Peek(1)
	if err != nil {
		return
	}

	if isHTTPMethod(peek[0]) {
		p.proxyHTTP(conn, reader)
	} else {
		p.proxyL4(conn, reader)
	}
}

// proxyHTTP handles HTTP proxying.
func (p *OutboundProxy) proxyHTTP(conn net.Conn, reader *bufio.Reader) {
	for {
		conn.SetReadDeadline(time.Now().Add(p.config.ReadTimeout))

		req, err := http.ReadRequest(reader)
		if err != nil {
			if err != io.EOF {
				// Log error
			}
			return
		}

		atomic.AddUint64(&p.totalRequests, 1)
		startTime := time.Now()

		// Build proxy request
		proxyReq := &ProxyRequest{
			Method:     req.Method,
			Path:       req.URL.Path,
			Host:       req.Host,
			Headers:    req.Header.Clone(),
			Body:       req.Body,
			RemoteAddr: conn.RemoteAddr().String(),
			StartTime:  startTime,
		}

		// Call handler
		if p.config.Handler == nil {
			sendHTTPError(conn, http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), p.config.ReadTimeout)
		resp, err := p.config.Handler(ctx, proxyReq)
		cancel()

		if err != nil {
			sendHTTPError(conn, http.StatusBadGateway)
			continue
		}

		// Build HTTP response
		httpResp := &http.Response{
			StatusCode: resp.StatusCode,
			Status:     http.StatusText(resp.StatusCode),
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     resp.Headers,
		}

		if httpResp.Header == nil {
			httpResp.Header = make(http.Header)
		}

		// Write response
		conn.SetWriteDeadline(time.Now().Add(p.config.WriteTimeout))

		if len(resp.Body) > 0 {
			httpResp.Header.Set("Content-Length", strconv.Itoa(len(resp.Body)))
			httpResp.ContentLength = int64(len(resp.Body))
		}

		if err := httpResp.Write(conn); err != nil {
			return
		}

		if len(resp.Body) > 0 {
			conn.Write(resp.Body)
			atomic.AddUint64(&p.totalBytes, uint64(len(resp.Body)))
		}

		// Check if connection should be closed
		if req.Close {
			return
		}
	}
}

// proxyL4 handles L4 (TCP) proxying.
func (p *OutboundProxy) proxyL4(conn net.Conn, reader *bufio.Reader) {
	// For L4 proxy, we need original destination
	// This requires SO_ORIGINAL_DST or similar platform-specific code
	// For now, just close the connection
}

// Stats returns proxy statistics.
func (p *OutboundProxy) Stats() OutboundProxyStats {
	return OutboundProxyStats{
		ActiveConnections: atomic.LoadInt64(&p.activeConns),
		TotalConnections:  atomic.LoadUint64(&p.totalConns),
		TotalRequests:     atomic.LoadUint64(&p.totalRequests),
		TotalBytes:        atomic.LoadUint64(&p.totalBytes),
	}
}

// OutboundProxyStats holds outbound proxy statistics.
type OutboundProxyStats struct {
	ActiveConnections int64
	TotalConnections  uint64
	TotalRequests     uint64
	TotalBytes        uint64
}

// ConnectionPool manages a pool of connections to a backend.
type ConnectionPool struct {
	config  *ConnectionPoolConfig
	address string

	mu    sync.Mutex
	conns chan *pooledConn

	// Stats
	activeConns int64
	totalConns  uint64
	totalHits   uint64
	totalMisses uint64
	totalErrors uint64
	closed      bool
}

// ConnectionPoolConfig configures a connection pool.
type ConnectionPoolConfig struct {
	MaxSize     int
	MinSize     int
	MaxLifetime time.Duration
	MaxIdleTime time.Duration
	DialTimeout time.Duration
	TLSConfig   *tls.Config
}

// DefaultConnectionPoolConfig returns default connection pool configuration.
func DefaultConnectionPoolConfig() *ConnectionPoolConfig {
	return &ConnectionPoolConfig{
		MaxSize:     10,
		MinSize:     2,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 90 * time.Second,
		DialTimeout: 5 * time.Second,
	}
}

type pooledConn struct {
	net.Conn
	pool      *ConnectionPool
	createdAt time.Time
	lastUsed  time.Time
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(address string, config *ConnectionPoolConfig) *ConnectionPool {
	if config == nil {
		config = DefaultConnectionPoolConfig()
	}

	return &ConnectionPool{
		config:  config,
		address: address,
		conns:   make(chan *pooledConn, config.MaxSize),
	}
}

// Get gets a connection from the pool.
func (p *ConnectionPool) Get(ctx context.Context) (net.Conn, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrProxyClosed
	}
	p.mu.Unlock()

	// Try to get from pool
	select {
	case pc := <-p.conns:
		// Check if connection is still valid
		if time.Since(pc.createdAt) > p.config.MaxLifetime ||
			time.Since(pc.lastUsed) > p.config.MaxIdleTime {
			pc.Conn.Close()
			atomic.AddUint64(&p.totalMisses, 1)
		} else {
			pc.lastUsed = time.Now()
			atomic.AddUint64(&p.totalHits, 1)
			atomic.AddInt64(&p.activeConns, 1)
			return pc, nil
		}
	default:
		atomic.AddUint64(&p.totalMisses, 1)
	}

	// Create new connection
	conn, err := p.dial(ctx)
	if err != nil {
		atomic.AddUint64(&p.totalErrors, 1)
		return nil, err
	}

	atomic.AddUint64(&p.totalConns, 1)
	atomic.AddInt64(&p.activeConns, 1)

	return &pooledConn{
		Conn:      conn,
		pool:      p,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}, nil
}

// Put returns a connection to the pool.
func (p *ConnectionPool) Put(conn net.Conn) {
	pc, ok := conn.(*pooledConn)
	if !ok {
		conn.Close()
		return
	}

	atomic.AddInt64(&p.activeConns, -1)

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		pc.Conn.Close()
		return
	}
	p.mu.Unlock()

	// Check if connection is still valid
	if time.Since(pc.createdAt) > p.config.MaxLifetime {
		pc.Conn.Close()
		return
	}

	pc.lastUsed = time.Now()

	// Try to return to pool
	select {
	case p.conns <- pc:
	default:
		pc.Conn.Close()
	}
}

// dial creates a new connection.
func (p *ConnectionPool) dial(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: p.config.DialTimeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", p.address)
	if err != nil {
		return nil, err
	}

	// Wrap with TLS if configured
	if p.config.TLSConfig != nil {
		tlsConn := tls.Client(conn, p.config.TLSConfig)
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, err
		}
		return tlsConn, nil
	}

	return conn, nil
}

// Close closes the connection pool.
func (p *ConnectionPool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	close(p.conns)
	for pc := range p.conns {
		pc.Conn.Close()
	}
}

// Cleanup removes idle connections.
func (p *ConnectionPool) Cleanup() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	// Drain and filter connections
	var toReturn []*pooledConn

	for {
		select {
		case pc := <-p.conns:
			if time.Since(pc.lastUsed) < p.config.MaxIdleTime &&
				time.Since(pc.createdAt) < p.config.MaxLifetime {
				toReturn = append(toReturn, pc)
			} else {
				pc.Conn.Close()
			}
		default:
			goto done
		}
	}

done:
	// Return valid connections
	for _, pc := range toReturn {
		select {
		case p.conns <- pc:
		default:
			pc.Conn.Close()
		}
	}
}

// Size returns the number of connections in the pool.
func (p *ConnectionPool) Size() int {
	return len(p.conns)
}

// Stats returns pool statistics.
func (p *ConnectionPool) Stats() ConnectionPoolStats {
	return ConnectionPoolStats{
		Address:     p.address,
		MaxSize:     p.config.MaxSize,
		CurrentSize: len(p.conns),
		ActiveConns: atomic.LoadInt64(&p.activeConns),
		TotalConns:  atomic.LoadUint64(&p.totalConns),
		TotalHits:   atomic.LoadUint64(&p.totalHits),
		TotalMisses: atomic.LoadUint64(&p.totalMisses),
		TotalErrors: atomic.LoadUint64(&p.totalErrors),
	}
}

// ConnectionPoolStats holds connection pool statistics.
type ConnectionPoolStats struct {
	Address     string
	MaxSize     int
	CurrentSize int
	ActiveConns int64
	TotalConns  uint64
	TotalHits   uint64
	TotalMisses uint64
	TotalErrors uint64
}

// HitRate returns the cache hit rate.
func (s ConnectionPoolStats) HitRate() float64 {
	total := s.TotalHits + s.TotalMisses
	if total == 0 {
		return 0
	}
	return float64(s.TotalHits) / float64(total) * 100
}

// TransparentProxy handles transparent proxying with iptables redirection.
type TransparentProxy struct {
	config   *TransparentProxyConfig
	listener net.Listener

	mu      sync.RWMutex
	started bool
	closed  bool
	wg      sync.WaitGroup

	// Stats
	activeConns int64
	totalConns  uint64
	totalBytes  uint64
}

// TransparentProxyConfig configures the transparent proxy.
type TransparentProxyConfig struct {
	Address        string
	Port           int
	TLSConfig      *tls.Config
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	BufferSize     int
	Backend        func(ctx context.Context, originalDst string) (string, error)
}

// NewTransparentProxy creates a new transparent proxy.
func NewTransparentProxy(config *TransparentProxyConfig) *TransparentProxy {
	if config == nil {
		config = &TransparentProxyConfig{
			Address:        "0.0.0.0",
			Port:           15001,
			ConnectTimeout: 5 * time.Second,
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			BufferSize:     32 * 1024,
		}
	}

	return &TransparentProxy{
		config: config,
	}
}

// Start starts the transparent proxy.
func (p *TransparentProxy) Start() error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return errors.New("proxy already started")
	}
	p.mu.Unlock()

	addr := net.JoinHostPort(p.config.Address, strconv.Itoa(p.config.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	p.listener = listener

	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	p.wg.Add(1)
	go p.acceptLoop()

	return nil
}

// Stop stops the transparent proxy.
func (p *TransparentProxy) Stop() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.started = false
	p.mu.Unlock()

	if p.listener != nil {
		p.listener.Close()
	}

	p.wg.Wait()
	return nil
}

// acceptLoop accepts incoming connections.
func (p *TransparentProxy) acceptLoop() {
	defer p.wg.Done()

	for {
		conn, err := p.listener.Accept()
		if err != nil {
			p.mu.RLock()
			closed := p.closed
			p.mu.RUnlock()
			if closed {
				return
			}
			continue
		}

		atomic.AddInt64(&p.activeConns, 1)
		atomic.AddUint64(&p.totalConns, 1)

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer atomic.AddInt64(&p.activeConns, -1)
			p.handleConnection(conn)
		}()
	}
}

// handleConnection handles a single connection.
func (p *TransparentProxy) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// Get original destination
	// This requires platform-specific code (SO_ORIGINAL_DST on Linux)
	originalDst := p.getOriginalDst(clientConn)
	if originalDst == "" {
		return
	}

	// Get backend address
	var backendAddr string
	if p.config.Backend != nil {
		ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectTimeout)
		var err error
		backendAddr, err = p.config.Backend(ctx, originalDst)
		cancel()
		if err != nil {
			return
		}
	} else {
		backendAddr = originalDst
	}

	// Connect to backend
	dialer := &net.Dialer{
		Timeout: p.config.ConnectTimeout,
	}

	backendConn, err := dialer.Dial("tcp", backendAddr)
	if err != nil {
		return
	}
	defer backendConn.Close()

	// Wrap with TLS if configured
	if p.config.TLSConfig != nil {
		tlsConn := tls.Client(backendConn, p.config.TLSConfig)
		if err := tlsConn.Handshake(); err != nil {
			return
		}
		backendConn = tlsConn
	}

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend
	go func() {
		defer wg.Done()
		n, _ := io.Copy(backendConn, clientConn)
		atomic.AddUint64(&p.totalBytes, uint64(n))
		if tc, ok := backendConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		n, _ := io.Copy(clientConn, backendConn)
		atomic.AddUint64(&p.totalBytes, uint64(n))
		if tc, ok := clientConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	wg.Wait()
}

// getOriginalDst gets the original destination address.
// This is a placeholder - real implementation requires syscalls.
func (p *TransparentProxy) getOriginalDst(conn net.Conn) string {
	// On Linux, this would use SO_ORIGINAL_DST socket option
	// For now, return empty string
	return ""
}

// Stats returns proxy statistics.
func (p *TransparentProxy) Stats() TransparentProxyStats {
	return TransparentProxyStats{
		ActiveConnections: atomic.LoadInt64(&p.activeConns),
		TotalConnections:  atomic.LoadUint64(&p.totalConns),
		TotalBytes:        atomic.LoadUint64(&p.totalBytes),
	}
}

// TransparentProxyStats holds transparent proxy statistics.
type TransparentProxyStats struct {
	ActiveConnections int64
	TotalConnections  uint64
	TotalBytes        uint64
}

// Helper functions

// isHTTPMethod checks if a byte is the start of an HTTP method.
func isHTTPMethod(b byte) bool {
	switch b {
	case 'G', 'P', 'D', 'H', 'O', 'C', 'T': // GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS, CONNECT, TRACE
		return true
	}
	return false
}

// sendHTTPError sends an HTTP error response.
func sendHTTPError(conn net.Conn, status int) {
	resp := &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	resp.Header.Set("Content-Length", "0")
	resp.Header.Set("Connection", "close")
	resp.Write(conn)
}

// PoolManager manages multiple connection pools.
type PoolManager struct {
	config   *ConnectionPoolConfig
	mu       sync.RWMutex
	pools    map[string]*ConnectionPool
	maxPools int
}

// NewPoolManager creates a new pool manager.
func NewPoolManager(config *ConnectionPoolConfig, maxPools int) *PoolManager {
	if config == nil {
		config = DefaultConnectionPoolConfig()
	}
	if maxPools <= 0 {
		maxPools = 100
	}

	return &PoolManager{
		config:   config,
		pools:    make(map[string]*ConnectionPool),
		maxPools: maxPools,
	}
}

// Get gets a connection for an address.
func (pm *PoolManager) Get(ctx context.Context, address string) (net.Conn, error) {
	pool := pm.getOrCreatePool(address)
	return pool.Get(ctx)
}

// Put returns a connection to its pool.
func (pm *PoolManager) Put(conn net.Conn) {
	if pc, ok := conn.(*pooledConn); ok {
		pc.pool.Put(conn)
	} else {
		conn.Close()
	}
}

// getOrCreatePool gets or creates a pool for an address.
func (pm *PoolManager) getOrCreatePool(address string) *ConnectionPool {
	pm.mu.RLock()
	pool, ok := pm.pools[address]
	pm.mu.RUnlock()

	if ok {
		return pool
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double-check
	if pool, ok := pm.pools[address]; ok {
		return pool
	}

	// Create new pool
	pool = NewConnectionPool(address, pm.config)
	pm.pools[address] = pool
	return pool
}

// Close closes all pools.
func (pm *PoolManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, pool := range pm.pools {
		pool.Close()
	}
	pm.pools = make(map[string]*ConnectionPool)
}

// Cleanup cleans up idle connections in all pools.
func (pm *PoolManager) Cleanup() {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, pool := range pm.pools {
		pool.Cleanup()
	}
}

// Stats returns statistics for all pools.
func (pm *PoolManager) Stats() map[string]ConnectionPoolStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make(map[string]ConnectionPoolStats, len(pm.pools))
	for addr, pool := range pm.pools {
		stats[addr] = pool.Stats()
	}
	return stats
}
