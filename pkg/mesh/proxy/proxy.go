// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrProxyClosed indicates the proxy is closed.
	ErrProxyClosed = errors.New("proxy closed")
	// ErrNoBackend indicates no backend is available.
	ErrNoBackend = errors.New("no backend available")
	// ErrTimeout indicates a timeout occurred.
	ErrTimeout = errors.New("operation timed out")
)

// Proxy is the L4/L7 proxy implementation.
type Proxy struct {
	config *Config

	// Listeners
	inboundListener  net.Listener
	outboundListener net.Listener

	// Connection management
	poolManager *PoolManager

	// TLS
	tlsConfig       *tls.Config
	clientTLSConfig *tls.Config

	// Backend selection
	selectBackend func(ctx context.Context) (string, error)

	// Callbacks
	onRequest  func(*Request)
	onResponse func(*Response)

	// State
	mu       sync.RWMutex
	running  bool
	closed   bool
	wg       sync.WaitGroup

	// Stats
	activeConns     int64
	totalConns      uint64
	totalRequests   uint64
	totalBytes      uint64
}

// Config configures the proxy.
type Config struct {
	// Inbound settings (receiving connections from apps)
	InboundAddr string
	InboundPort int

	// Outbound settings (for transparent proxy)
	OutboundAddr string
	OutboundPort int

	// Timeouts
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration

	// Buffer sizes
	ReadBufferSize  int
	WriteBufferSize int

	// Connection pool
	MaxIdleConns        int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration

	// TLS
	TLSConfig       *tls.Config // For inbound
	ClientTLSConfig *tls.Config // For outbound

	// Features
	EnableHTTP2 bool
}

// DefaultConfig returns a default proxy configuration.
func DefaultConfig() *Config {
	return &Config{
		InboundAddr:         "127.0.0.1",
		InboundPort:         15001,
		OutboundAddr:        "127.0.0.1",
		OutboundPort:        15006,
		ConnectTimeout:      5 * time.Second,
		ReadTimeout:         30 * time.Second,
		WriteTimeout:        30 * time.Second,
		IdleTimeout:         90 * time.Second,
		ReadBufferSize:      32 * 1024,
		WriteBufferSize:     32 * 1024,
		MaxIdleConns:        100,
		MaxConnsPerHost:     10,
		IdleConnTimeout:     90 * time.Second,
	}
}

// NewProxy creates a new proxy.
func NewProxy(config *Config) *Proxy {
	if config == nil {
		config = DefaultConfig()
	}

	poolConfig := &PoolConfig{
		MaxSize:     config.MaxConnsPerHost,
		MinSize:     2,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: config.IdleConnTimeout,
		DialTimeout: config.ConnectTimeout,
	}

	return &Proxy{
		config:          config,
		poolManager:     NewPoolManager(poolConfig, config.MaxIdleConns),
		tlsConfig:       config.TLSConfig,
		clientTLSConfig: config.ClientTLSConfig,
	}
}

// SetBackendSelector sets the backend selection function.
func (p *Proxy) SetBackendSelector(fn func(ctx context.Context) (string, error)) {
	p.mu.Lock()
	p.selectBackend = fn
	p.mu.Unlock()
}

// SetTLSConfig sets the TLS configuration for inbound connections.
func (p *Proxy) SetTLSConfig(config *tls.Config) {
	p.mu.Lock()
	p.tlsConfig = config
	p.mu.Unlock()
}

// SetClientTLSConfig sets the TLS configuration for outbound connections.
func (p *Proxy) SetClientTLSConfig(config *tls.Config) {
	p.mu.Lock()
	p.clientTLSConfig = config
	p.mu.Unlock()
}

// OnRequest sets a callback for incoming requests.
func (p *Proxy) OnRequest(fn func(*Request)) {
	p.mu.Lock()
	p.onRequest = fn
	p.mu.Unlock()
}

// OnResponse sets a callback for responses.
func (p *Proxy) OnResponse(fn func(*Response)) {
	p.mu.Lock()
	p.onResponse = fn
	p.mu.Unlock()
}

// Start starts the proxy listeners.
func (p *Proxy) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return errors.New("proxy already running")
	}
	p.running = true
	p.mu.Unlock()

	// Start inbound listener
	inboundAddr := net.JoinHostPort(p.config.InboundAddr, itoa(p.config.InboundPort))
	listener, err := net.Listen("tcp", inboundAddr)
	if err != nil {
		return err
	}
	p.inboundListener = listener

	// Start outbound listener
	outboundAddr := net.JoinHostPort(p.config.OutboundAddr, itoa(p.config.OutboundPort))
	outListener, err := net.Listen("tcp", outboundAddr)
	if err != nil {
		p.inboundListener.Close()
		return err
	}
	p.outboundListener = outListener

	// Accept connections
	p.wg.Add(2)
	go p.acceptLoop(p.inboundListener, true)
	go p.acceptLoop(p.outboundListener, false)

	return nil
}

// Stop stops the proxy.
func (p *Proxy) Stop() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.running = false
	p.mu.Unlock()

	// Close listeners
	if p.inboundListener != nil {
		p.inboundListener.Close()
	}
	if p.outboundListener != nil {
		p.outboundListener.Close()
	}

	// Close connection pools
	p.poolManager.Close()

	// Wait for all connections to finish
	p.wg.Wait()

	return nil
}

// Stats returns proxy statistics.
func (p *Proxy) Stats() ProxyStats {
	return ProxyStats{
		ActiveConnections: atomic.LoadInt64(&p.activeConns),
		TotalConnections:  atomic.LoadUint64(&p.totalConns),
		TotalRequests:     atomic.LoadUint64(&p.totalRequests),
		TotalBytes:        atomic.LoadUint64(&p.totalBytes),
		PoolStats:         p.poolManager.Stats(),
	}
}

func (p *Proxy) acceptLoop(listener net.Listener, inbound bool) {
	defer p.wg.Done()

	for {
		conn, err := listener.Accept()
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

			if inbound {
				p.handleInbound(conn)
			} else {
				p.handleOutbound(conn)
			}
		}()
	}
}

func (p *Proxy) handleInbound(conn net.Conn) {
	defer conn.Close()

	// Wrap with TLS if configured
	p.mu.RLock()
	tlsConfig := p.tlsConfig
	p.mu.RUnlock()

	if tlsConfig != nil {
		tlsConn := tls.Server(conn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return
		}
		conn = tlsConn
	}

	// Set timeouts
	conn.SetDeadline(time.Now().Add(p.config.IdleTimeout))

	// Proxy the connection
	p.proxyConnection(conn, true)
}

func (p *Proxy) handleOutbound(conn net.Conn) {
	defer conn.Close()

	// Set timeouts
	conn.SetDeadline(time.Now().Add(p.config.IdleTimeout))

	// Proxy the connection
	p.proxyConnection(conn, false)
}

func (p *Proxy) proxyConnection(clientConn net.Conn, inbound bool) {
	// Try to detect protocol
	br := bufio.NewReaderSize(clientConn, p.config.ReadBufferSize)
	peek, err := br.Peek(1)
	if err != nil {
		return
	}

	// Check if HTTP
	if isHTTPMethod(peek[0]) {
		p.proxyHTTP(clientConn, br, inbound)
		return
	}

	// Fall back to L4 proxy
	p.proxyL4(clientConn, br, inbound)
}

func (p *Proxy) proxyHTTP(clientConn net.Conn, reader *bufio.Reader, inbound bool) {
	for {
		// Set read deadline
		clientConn.SetReadDeadline(time.Now().Add(p.config.ReadTimeout))

		// Parse HTTP request
		req, err := http.ReadRequest(reader)
		if err != nil {
			if err != io.EOF {
				// Log error
			}
			return
		}

		atomic.AddUint64(&p.totalRequests, 1)

		// Create request wrapper
		request := &Request{
			Method:     req.Method,
			Path:       req.URL.Path,
			Host:       req.Host,
			Headers:    req.Header,
			RemoteAddr: clientConn.RemoteAddr().String(),
			StartTime:  time.Now(),
			Inbound:    inbound,
		}

		// Callback
		p.mu.RLock()
		onRequest := p.onRequest
		p.mu.RUnlock()
		if onRequest != nil {
			onRequest(request)
		}

		// Select backend
		p.mu.RLock()
		selectBackend := p.selectBackend
		p.mu.RUnlock()

		var backendAddr string
		if selectBackend != nil {
			ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectTimeout)
			backendAddr, err = selectBackend(ctx)
			cancel()
			if err != nil {
				p.sendHTTPError(clientConn, http.StatusServiceUnavailable)
				continue
			}
		} else {
			backendAddr = req.Host
		}

		// Get backend connection
		backendConn, err := p.getBackendConn(backendAddr)
		if err != nil {
			p.sendHTTPError(clientConn, http.StatusBadGateway)
			continue
		}

		// Forward request
		backendConn.SetWriteDeadline(time.Now().Add(p.config.WriteTimeout))
		if err := req.Write(backendConn); err != nil {
			backendConn.Close()
			p.sendHTTPError(clientConn, http.StatusBadGateway)
			continue
		}

		// Read response
		backendConn.SetReadDeadline(time.Now().Add(p.config.ReadTimeout))
		backendReader := bufio.NewReaderSize(backendConn, p.config.ReadBufferSize)
		resp, err := http.ReadResponse(backendReader, req)
		if err != nil {
			backendConn.Close()
			p.sendHTTPError(clientConn, http.StatusBadGateway)
			continue
		}

		// Create response wrapper
		response := &Response{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Duration:   time.Since(request.StartTime),
			Backend:    backendAddr,
		}

		// Callback
		p.mu.RLock()
		onResponse := p.onResponse
		p.mu.RUnlock()
		if onResponse != nil {
			onResponse(response)
		}

		// Forward response
		clientConn.SetWriteDeadline(time.Now().Add(p.config.WriteTimeout))
		if err := resp.Write(clientConn); err != nil {
			backendConn.Close()
			return
		}

		// Return connection to pool if keep-alive
		if resp.Close || req.Close {
			backendConn.Close()
		} else {
			if pc, ok := backendConn.(*PooledConn); ok {
				pc.pool.put(pc)
			} else {
				backendConn.Close()
			}
		}

		// Check if we should continue
		if req.Close {
			return
		}
	}
}

func (p *Proxy) proxyL4(clientConn net.Conn, reader *bufio.Reader, inbound bool) {
	// Select backend
	p.mu.RLock()
	selectBackend := p.selectBackend
	p.mu.RUnlock()

	var backendAddr string
	var err error

	if selectBackend != nil {
		ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectTimeout)
		backendAddr, err = selectBackend(ctx)
		cancel()
		if err != nil {
			return
		}
	} else {
		// Need original destination for transparent proxy
		// This would require SO_ORIGINAL_DST socket option
		return
	}

	// Connect to backend
	backendConn, err := p.getBackendConn(backendAddr)
	if err != nil {
		return
	}
	defer backendConn.Close()

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend
	go func() {
		defer wg.Done()
		n, _ := io.Copy(backendConn, reader)
		atomic.AddUint64(&p.totalBytes, uint64(n))
		// Signal EOF to backend
		if tc, ok := backendConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		n, _ := io.Copy(clientConn, backendConn)
		atomic.AddUint64(&p.totalBytes, uint64(n))
		// Signal EOF to client
		if tc, ok := clientConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	wg.Wait()
}

func (p *Proxy) getBackendConn(address string) (net.Conn, error) {
	// Try to get from pool
	conn, err := p.poolManager.Get(address)
	if err != nil {
		return nil, err
	}

	// Wrap with TLS if configured
	p.mu.RLock()
	tlsConfig := p.clientTLSConfig
	p.mu.RUnlock()

	if tlsConfig != nil {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, err
		}
		return tlsConn, nil
	}

	return conn, nil
}

func (p *Proxy) sendHTTPError(conn net.Conn, status int) {
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

func isHTTPMethod(b byte) bool {
	// HTTP methods start with uppercase letters
	// GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS, CONNECT, TRACE
	switch b {
	case 'G', 'P', 'D', 'H', 'O', 'C', 'T':
		return true
	}
	return false
}

// Request represents a proxied request.
type Request struct {
	Method     string
	Path       string
	Host       string
	Headers    http.Header
	RemoteAddr string
	StartTime  time.Time
	Inbound    bool
}

// Response represents a proxied response.
type Response struct {
	StatusCode int
	Headers    http.Header
	Duration   time.Duration
	Backend    string
	Error      error
}

// ProxyStats holds proxy statistics.
type ProxyStats struct {
	ActiveConnections int64
	TotalConnections  uint64
	TotalRequests     uint64
	TotalBytes        uint64
	PoolStats         map[string]PoolStats
}

// TransparentProxy handles iptables-redirected traffic.
type TransparentProxy struct {
	*Proxy
}

// NewTransparentProxy creates a transparent proxy.
func NewTransparentProxy(config *Config) *TransparentProxy {
	return &TransparentProxy{
		Proxy: NewProxy(config),
	}
}

// GetOriginalDst gets the original destination from a redirected connection.
// This requires platform-specific implementation using SO_ORIGINAL_DST.
func (tp *TransparentProxy) GetOriginalDst(conn net.Conn) (string, error) {
	// This is a simplified version. In production, you'd use:
	// - syscall.GetsockoptIPv6Mreq for IPv6
	// - syscall.GetsockoptIPMreq for IPv4
	// With SO_ORIGINAL_DST option
	return "", errors.New("not implemented - requires syscall")
}
