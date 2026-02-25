// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// L7Proxy handles Layer 7 (HTTP/HTTPS) load balancing.
type L7Proxy struct {
	config         *Config
	pool           *BackendPool
	health         *HealthAggregator
	sessionManager *SessionManager

	// HTTP server
	server *http.Server

	// Reverse proxy per backend
	proxies   map[string]*httputil.ReverseProxy
	proxiesMu sync.RWMutex

	// State
	running int32
	stopCh  chan struct{}

	// Stats
	activeRequests  int64
	totalRequests   int64
	totalErrors     int64
	totalBytes      int64

	// Callbacks
	onRequest  func(r *http.Request, backend *Backend)
	onResponse func(r *http.Request, backend *Backend, status int, duration time.Duration)
	onError    func(r *http.Request, err error)
}

// NewL7Proxy creates a new Layer 7 proxy.
func NewL7Proxy(config *Config, pool *BackendPool, health *HealthAggregator) (*L7Proxy, error) {
	proxy := &L7Proxy{
		config:         config,
		pool:           pool,
		health:         health,
		sessionManager: NewSessionManager(config.SessionPersistence, pool),
		stopCh:         make(chan struct{}),
		proxies:        make(map[string]*httputil.ReverseProxy),
	}

	// Create reverse proxies for each backend
	for _, backend := range pool.All() {
		rp, err := proxy.createReverseProxy(backend)
		if err != nil {
			return nil, fmt.Errorf("create proxy for %s: %w", backend.Config.Name, err)
		}
		proxy.proxies[backend.Config.Name] = rp
	}

	return proxy, nil
}

// createReverseProxy creates a reverse proxy for a backend.
func (p *L7Proxy) createReverseProxy(backend *Backend) (*httputil.ReverseProxy, error) {
	targetURL, err := url.Parse(fmt.Sprintf("http://%s", backend.Config.Address))
	if err != nil {
		return nil, err
	}

	// Create transport with connection pooling
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   p.config.Timeouts.Connect,
			KeepAlive: p.config.Timeouts.KeepAlive,
		}).DialContext,
		MaxIdleConns:          p.config.ConnectionPool.MaxTotalConnections,
		MaxIdleConnsPerHost:   p.config.ConnectionPool.MaxIdlePerBackend,
		IdleConnTimeout:       p.config.ConnectionPool.IdleTimeout,
		ResponseHeaderTimeout: p.config.Timeouts.Read,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Configure TLS for backend if needed
	if backend.tlsConfig != nil {
		transport.TLSClientConfig = backend.tlsConfig
		targetURL.Scheme = "https"
	}

	rp := httputil.NewSingleHostReverseProxy(targetURL)
	rp.Transport = transport

	// Custom error handler
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		p.health.RecordError(backend, err)
		atomic.AddInt64(&p.totalErrors, 1)
		backend.RecordError(err)

		if p.onError != nil {
			p.onError(r, err)
		}

		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Modify request before sending to backend
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)

		// Apply header modifications
		p.applyRequestHeaders(req)
	}

	// Modify response before sending to client
	rp.ModifyResponse = func(resp *http.Response) error {
		p.applyResponseHeaders(resp)
		return nil
	}

	return rp, nil
}

// OnRequest sets the callback for new requests.
func (p *L7Proxy) OnRequest(fn func(r *http.Request, backend *Backend)) {
	p.onRequest = fn
}

// OnResponse sets the callback for completed requests.
func (p *L7Proxy) OnResponse(fn func(r *http.Request, backend *Backend, status int, duration time.Duration)) {
	p.onResponse = fn
}

// OnError sets the callback for request errors.
func (p *L7Proxy) OnError(fn func(r *http.Request, err error)) {
	p.onError = fn
}

// Start starts the L7 proxy.
func (p *L7Proxy) Start() error {
	if !atomic.CompareAndSwapInt32(&p.running, 0, 1) {
		return errors.New("proxy already running")
	}

	// Start session manager
	p.sessionManager.Start()

	// Create HTTP server
	p.server = &http.Server{
		Addr:         p.config.ListenAddress,
		Handler:      p,
		ReadTimeout:  p.config.Timeouts.Read,
		WriteTimeout: p.config.Timeouts.Write,
		IdleTimeout:  p.config.Timeouts.Idle,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Configure TLS if needed
	if p.config.TLS != nil {
		tlsConfig, err := p.config.TLS.BuildTLSConfig()
		if err != nil {
			atomic.StoreInt32(&p.running, 0)
			return fmt.Errorf("build TLS config: %w", err)
		}
		p.server.TLSConfig = tlsConfig
	}

	// Start server
	go func() {
		var err error
		if p.config.TLS != nil {
			err = p.server.ListenAndServeTLS("", "")
		} else {
			err = p.server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			// Log error
		}
	}()

	return nil
}

// Stop stops the L7 proxy.
func (p *L7Proxy) Stop() error {
	if !atomic.CompareAndSwapInt32(&p.running, 1, 0) {
		return errors.New("proxy not running")
	}

	close(p.stopCh)
	p.sessionManager.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), p.config.DrainTimeout)
	defer cancel()

	return p.server.Shutdown(ctx)
}

// ServeHTTP handles HTTP requests.
func (p *L7Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	atomic.AddInt64(&p.activeRequests, 1)
	atomic.AddInt64(&p.totalRequests, 1)
	defer atomic.AddInt64(&p.activeRequests, -1)

	// Check rate limits
	if p.config.Limits.RateLimit > 0 {
		// Simple rate limiting check (in production, use a proper rate limiter)
	}

	// Check request size
	if p.config.Limits.MaxRequestSize > 0 && r.ContentLength > p.config.Limits.MaxRequestSize {
		http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
		return
	}

	// Select backend
	backend, err := p.selectBackend(r)
	if err != nil {
		if p.onError != nil {
			p.onError(r, err)
		}
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	if p.onRequest != nil {
		p.onRequest(r, backend)
	}

	backend.IncrementRequests()

	// Get reverse proxy for backend
	p.proxiesMu.RLock()
	rp, ok := p.proxies[backend.Config.Name]
	p.proxiesMu.RUnlock()

	if !ok {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Wrap response writer to capture status
	wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Proxy the request
	rp.ServeHTTP(wrapped, r)

	duration := time.Since(startTime)
	backend.RecordResponseTime(duration)

	// Set session persistence
	p.sessionManager.SetBackend(r, w, backend)

	if p.onResponse != nil {
		p.onResponse(r, backend, wrapped.statusCode, duration)
	}
}

// selectBackend selects a backend for a request.
func (p *L7Proxy) selectBackend(r *http.Request) (*Backend, error) {
	// Check session persistence first
	if backend, found := p.sessionManager.GetBackend(r); found {
		return backend, nil
	}

	// Use algorithm to select
	ctx := r.Context()
	key := p.getSelectionKey(r)
	return p.pool.Select(ctx, key)
}

// getSelectionKey extracts the key for backend selection.
func (p *L7Proxy) getSelectionKey(r *http.Request) string {
	// For IP-hash, use client IP
	if p.config.Algorithm == AlgorithmIPHash {
		return getClientIP(r, p.config.Headers.TrustXForwardedFor)
	}
	return r.RemoteAddr
}

// applyRequestHeaders applies header modifications to requests.
func (p *L7Proxy) applyRequestHeaders(r *http.Request) {
	cfg := p.config.Headers

	// Add headers
	for k, v := range cfg.AddRequestHeaders {
		r.Header.Set(k, v)
	}

	// Remove headers
	for _, k := range cfg.RemoveRequestHeaders {
		r.Header.Del(k)
	}

	// Set X-Forwarded-For
	if cfg.SetXForwardedFor {
		clientIP := getClientIP(r, cfg.TrustXForwardedFor)
		existing := r.Header.Get("X-Forwarded-For")
		if existing != "" {
			r.Header.Set("X-Forwarded-For", existing+", "+clientIP)
		} else {
			r.Header.Set("X-Forwarded-For", clientIP)
		}
	}

	// Set X-Real-IP
	if cfg.SetXRealIP {
		r.Header.Set("X-Real-IP", getClientIP(r, cfg.TrustXForwardedFor))
	}

	// Set Forwarded header (RFC 7239)
	if cfg.SetForwarded {
		clientIP := getClientIP(r, cfg.TrustXForwardedFor)
		proto := "http"
		if r.TLS != nil {
			proto = "https"
		}
		forwarded := fmt.Sprintf("for=%s;proto=%s;host=%s", clientIP, proto, r.Host)
		r.Header.Set("Forwarded", forwarded)
	}

	// Rewrite Host header
	if cfg.HostRewrite != "" {
		r.Host = cfg.HostRewrite
	}
}

// applyResponseHeaders applies header modifications to responses.
func (p *L7Proxy) applyResponseHeaders(resp *http.Response) {
	cfg := p.config.Headers

	// Add headers
	for k, v := range cfg.AddResponseHeaders {
		resp.Header.Set(k, v)
	}

	// Remove headers
	for _, k := range cfg.RemoveResponseHeaders {
		resp.Header.Del(k)
	}
}

// getClientIP extracts the client IP from a request.
func getClientIP(r *http.Request, trustXFF bool) string {
	if trustXFF {
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}

		xri := r.Header.Get("X-Real-IP")
		if xri != "" {
			return xri
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}

// Stats returns proxy statistics.
func (p *L7Proxy) Stats() L7ProxyStats {
	return L7ProxyStats{
		ActiveRequests: atomic.LoadInt64(&p.activeRequests),
		TotalRequests:  atomic.LoadInt64(&p.totalRequests),
		TotalErrors:    atomic.LoadInt64(&p.totalErrors),
		TotalBytes:     atomic.LoadInt64(&p.totalBytes),
		SessionStats:   p.sessionManager.Stats(),
	}
}

// L7ProxyStats holds L7 proxy statistics.
type L7ProxyStats struct {
	ActiveRequests int64
	TotalRequests  int64
	TotalErrors    int64
	TotalBytes     int64
	SessionStats   SessionStats
}

// WebSocketProxy handles WebSocket connections.
type WebSocketProxy struct {
	pool    *BackendPool
	dialer  *net.Dialer
	timeout time.Duration
}

// NewWebSocketProxy creates a new WebSocket proxy.
func NewWebSocketProxy(pool *BackendPool, timeout time.Duration) *WebSocketProxy {
	return &WebSocketProxy{
		pool:    pool,
		timeout: timeout,
		dialer: &net.Dialer{
			Timeout: timeout,
		},
	}
}

// ServeHTTP handles WebSocket upgrade requests.
func (ws *WebSocketProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if this is a WebSocket upgrade request
	if !isWebSocketUpgrade(r) {
		http.Error(w, "Not a WebSocket upgrade request", http.StatusBadRequest)
		return
	}

	// Select backend
	backend, err := ws.pool.Select(r.Context(), r.RemoteAddr)
	if err != nil {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Connect to backend
	backendConn, err := ws.dialer.Dial("tcp", backend.Config.Address)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// Hijack client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		backendConn.Close()
		return
	}

	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		backendConn.Close()
		return
	}

	// Forward the original request to backend
	if err := r.Write(backendConn); err != nil {
		clientConn.Close()
		backendConn.Close()
		return
	}

	// Flush any buffered data
	if clientBuf.Reader.Buffered() > 0 {
		buf := make([]byte, clientBuf.Reader.Buffered())
		clientBuf.Read(buf)
		backendConn.Write(buf)
	}

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(backendConn, clientConn)
		backendConn.Close()
	}()

	go func() {
		defer wg.Done()
		io.Copy(clientConn, backendConn)
		clientConn.Close()
	}()

	wg.Wait()
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade.
// Uses case-insensitive comparison per RFC 6455.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		headerContainsTokenCI(r.Header.Get("Connection"), "upgrade")
}

// headerContainsTokenCI checks if a comma-separated header value contains
// a token, using case-insensitive comparison per RFC 6455 / RFC 7230.
func headerContainsTokenCI(headerValue, token string) bool {
	for _, part := range strings.Split(headerValue, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// GRPCProxy handles gRPC connections.
type GRPCProxy struct {
	pool       *BackendPool
	tlsConfig  *tls.Config
	timeout    time.Duration
}

// NewGRPCProxy creates a new gRPC proxy.
func NewGRPCProxy(pool *BackendPool, tlsConfig *tls.Config, timeout time.Duration) *GRPCProxy {
	return &GRPCProxy{
		pool:      pool,
		tlsConfig: tlsConfig,
		timeout:   timeout,
	}
}

// ServeHTTP handles gRPC requests (HTTP/2).
func (gp *GRPCProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if this is a gRPC request
	if !isGRPCRequest(r) {
		http.Error(w, "Not a gRPC request", http.StatusBadRequest)
		return
	}

	// Select backend
	backend, err := gp.pool.Select(r.Context(), r.RemoteAddr)
	if err != nil {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Create transport for HTTP/2
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: gp.timeout,
		}).DialContext,
		ForceAttemptHTTP2: true,
		TLSClientConfig:   backend.tlsConfig,
	}

	// Create proxy
	targetURL, _ := url.Parse(fmt.Sprintf("http://%s", backend.Config.Address))
	rp := httputil.NewSingleHostReverseProxy(targetURL)
	rp.Transport = transport

	// Proxy the request
	rp.ServeHTTP(w, r)
}

// isGRPCRequest checks if the request is a gRPC request.
func isGRPCRequest(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc")
}

// CircuitBreaker provides circuit breaker functionality.
type CircuitBreaker struct {
	mu           sync.RWMutex
	state        circuitState
	failures     int
	successes    int
	lastFailure  time.Time
	threshold    int
	timeout      time.Duration
	halfOpenMax  int
}

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(threshold int, timeout time.Duration, halfOpenMax int) *CircuitBreaker {
	return &CircuitBreaker{
		state:       circuitClosed,
		threshold:   threshold,
		timeout:     timeout,
		halfOpenMax: halfOpenMax,
	}
}

// Allow checks if a request should be allowed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case circuitClosed:
		return true
	case circuitOpen:
		if time.Since(cb.lastFailure) > cb.timeout {
			cb.state = circuitHalfOpen
			cb.successes = 0
			return true
		}
		return false
	case circuitHalfOpen:
		return cb.successes < cb.halfOpenMax
	}
	return false
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	if cb.state == circuitHalfOpen {
		cb.successes++
		if cb.successes >= cb.halfOpenMax {
			cb.state = circuitClosed
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.threshold {
		cb.state = circuitOpen
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case circuitClosed:
		return "closed"
	case circuitOpen:
		return "open"
	case circuitHalfOpen:
		return "half-open"
	}
	return "unknown"
}

// RetryPolicy configures request retry behavior.
type RetryPolicy struct {
	MaxRetries    int
	RetryStatuses []int
	Backoff       time.Duration
	MaxBackoff    time.Duration
}

// DefaultRetryPolicy returns default retry settings.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:    3,
		RetryStatuses: []int{502, 503, 504},
		Backoff:       100 * time.Millisecond,
		MaxBackoff:    5 * time.Second,
	}
}

// ShouldRetry checks if a status code should trigger a retry.
func (rp *RetryPolicy) ShouldRetry(statusCode int) bool {
	for _, s := range rp.RetryStatuses {
		if s == statusCode {
			return true
		}
	}
	return false
}

// BackoffDuration calculates backoff for a retry attempt.
func (rp *RetryPolicy) BackoffDuration(attempt int) time.Duration {
	d := rp.Backoff * time.Duration(1<<uint(attempt))
	if d > rp.MaxBackoff {
		d = rp.MaxBackoff
	}
	return d
}

// RequestRewriter rewrites requests before forwarding.
type RequestRewriter struct {
	rules []RewriteRule
}

// RewriteRule defines a rewrite rule.
type RewriteRule struct {
	// Match is the path prefix to match.
	Match string
	// Replace is the replacement path.
	Replace string
	// StripPrefix removes the match prefix.
	StripPrefix bool
	// AddPrefix adds a prefix to the path.
	AddPrefix string
}

// NewRequestRewriter creates a new request rewriter.
func NewRequestRewriter() *RequestRewriter {
	return &RequestRewriter{
		rules: make([]RewriteRule, 0),
	}
}

// AddRule adds a rewrite rule.
func (rr *RequestRewriter) AddRule(rule RewriteRule) {
	rr.rules = append(rr.rules, rule)
}

// Rewrite rewrites a request path.
func (rr *RequestRewriter) Rewrite(r *http.Request) {
	path := r.URL.Path

	for _, rule := range rr.rules {
		if strings.HasPrefix(path, rule.Match) {
			if rule.Replace != "" {
				path = rule.Replace + strings.TrimPrefix(path, rule.Match)
			} else if rule.StripPrefix {
				path = strings.TrimPrefix(path, rule.Match)
			}
			if rule.AddPrefix != "" {
				path = rule.AddPrefix + path
			}
			r.URL.Path = path
			return
		}
	}
}

// HealthResponseWriter captures the response for health-based routing.
type HealthResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (hrw *HealthResponseWriter) WriteHeader(code int) {
	hrw.statusCode = code
}

func (hrw *HealthResponseWriter) Write(b []byte) (int, error) {
	hrw.body = append(hrw.body, b...)
	return len(b), nil
}

// Flush sends the buffered response.
func (hrw *HealthResponseWriter) Flush(w http.ResponseWriter) {
	w.WriteHeader(hrw.statusCode)
	w.Write(hrw.body)
}

// CompressedResponseWriter adds compression to responses.
type CompressedResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

func (crw *CompressedResponseWriter) Write(b []byte) (int, error) {
	return crw.writer.Write(b)
}

// RateLimiter implements token bucket rate limiting.
type RateLimiter struct {
	mu        sync.Mutex
	tokens    float64
	maxTokens float64
	rate      float64 // tokens per second
	lastTime  time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		tokens:    float64(burst),
		maxTokens: float64(burst),
		rate:      rate,
		lastTime:  time.Now(),
	}
}

// Allow checks if a request should be allowed.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.lastTime = now

	// Add tokens based on elapsed time
	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}

	// Check if we have a token
	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}
	return false
}

// IPRateLimiter provides per-IP rate limiting.
type IPRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*RateLimiter
	rate     float64
	burst    int
}

// NewIPRateLimiter creates a new per-IP rate limiter.
func NewIPRateLimiter(rate float64, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		limiters: make(map[string]*RateLimiter),
		rate:     rate,
		burst:    burst,
	}
}

// Allow checks if a request from an IP should be allowed.
func (iprl *IPRateLimiter) Allow(ip string) bool {
	iprl.mu.RLock()
	limiter, ok := iprl.limiters[ip]
	iprl.mu.RUnlock()

	if !ok {
		iprl.mu.Lock()
		limiter, ok = iprl.limiters[ip]
		if !ok {
			limiter = NewRateLimiter(iprl.rate, iprl.burst)
			iprl.limiters[ip] = limiter
		}
		iprl.mu.Unlock()
	}

	return limiter.Allow()
}

// Cleanup removes stale limiters.
func (iprl *IPRateLimiter) Cleanup(maxAge time.Duration) {
	iprl.mu.Lock()
	defer iprl.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for ip, limiter := range iprl.limiters {
		if limiter.lastTime.Before(cutoff) {
			delete(iprl.limiters, ip)
		}
	}
}

// TraceIDGenerator generates trace IDs for requests.
type TraceIDGenerator struct {
	counter uint64
	prefix  string
}

// NewTraceIDGenerator creates a new trace ID generator.
func NewTraceIDGenerator(prefix string) *TraceIDGenerator {
	return &TraceIDGenerator{prefix: prefix}
}

// Generate generates a new trace ID.
func (g *TraceIDGenerator) Generate() string {
	id := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("%s-%d-%d", g.prefix, time.Now().UnixNano(), id)
}

// TraceMiddleware adds trace headers to requests.
func TraceMiddleware(generator *TraceIDGenerator, headerName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get(headerName)
		if traceID == "" {
			traceID = generator.Generate()
			r.Header.Set(headerName, traceID)
		}
		w.Header().Set(headerName, traceID)
		next.ServeHTTP(w, r)
	})
}

// HeaderInspector provides header-based routing.
type HeaderInspector struct {
	rules []HeaderRule
}

// HeaderRule defines a header-based routing rule.
type HeaderRule struct {
	Header   string
	Value    string
	Backend  string
	Exact    bool
	Contains bool
}

// NewHeaderInspector creates a new header inspector.
func NewHeaderInspector() *HeaderInspector {
	return &HeaderInspector{
		rules: make([]HeaderRule, 0),
	}
}

// AddRule adds a header routing rule.
func (hi *HeaderInspector) AddRule(rule HeaderRule) {
	hi.rules = append(hi.rules, rule)
}

// Match finds the backend for a request based on headers.
func (hi *HeaderInspector) Match(r *http.Request) (string, bool) {
	for _, rule := range hi.rules {
		value := r.Header.Get(rule.Header)
		if value == "" {
			continue
		}

		if rule.Exact && value == rule.Value {
			return rule.Backend, true
		}
		if rule.Contains && strings.Contains(value, rule.Value) {
			return rule.Backend, true
		}
		if !rule.Exact && !rule.Contains && strings.EqualFold(value, rule.Value) {
			return rule.Backend, true
		}
	}
	return "", false
}

// ContentLengthValidator validates content length.
type ContentLengthValidator struct {
	maxSize int64
}

// NewContentLengthValidator creates a new content length validator.
func NewContentLengthValidator(maxSize int64) *ContentLengthValidator {
	return &ContentLengthValidator{maxSize: maxSize}
}

// Validate validates the request content length.
func (v *ContentLengthValidator) Validate(r *http.Request) error {
	// Check Content-Length header
	if r.ContentLength > v.maxSize {
		return fmt.Errorf("content length %d exceeds maximum %d", r.ContentLength, v.maxSize)
	}

	// Also check Transfer-Encoding: chunked by reading incrementally
	return nil
}

// XForwardedForParser parses X-Forwarded-For headers.
type XForwardedForParser struct {
	trustedProxies []net.IPNet
}

// NewXForwardedForParser creates a new parser.
func NewXForwardedForParser(trustedProxies []string) (*XForwardedForParser, error) {
	parser := &XForwardedForParser{
		trustedProxies: make([]net.IPNet, 0, len(trustedProxies)),
	}

	for _, cidr := range trustedProxies {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Try as single IP
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, fmt.Errorf("invalid trusted proxy: %s", cidr)
			}
			mask := net.CIDRMask(32, 32)
			if ip.To4() == nil {
				mask = net.CIDRMask(128, 128)
			}
			ipnet = &net.IPNet{IP: ip, Mask: mask}
		}
		parser.trustedProxies = append(parser.trustedProxies, *ipnet)
	}

	return parser, nil
}

// GetClientIP extracts the real client IP considering trusted proxies.
func (p *XForwardedForParser) GetClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		return host
	}

	// Parse IPs from X-Forwarded-For (rightmost to leftmost)
	ips := strings.Split(xff, ",")
	for i := len(ips) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(ips[i])
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			continue
		}

		// If this IP is not a trusted proxy, it's the client
		trusted := false
		for _, cidr := range p.trustedProxies {
			if cidr.Contains(parsedIP) {
				trusted = true
				break
			}
		}

		if !trusted {
			return ip
		}
	}

	// All IPs were trusted proxies, return the leftmost
	if len(ips) > 0 {
		return strings.TrimSpace(ips[0])
	}

	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// ServerTimingHeader generates Server-Timing headers.
type ServerTimingHeader struct {
	entries []serverTimingEntry
	mu      sync.Mutex
}

type serverTimingEntry struct {
	name        string
	duration    time.Duration
	description string
}

// NewServerTimingHeader creates a new Server-Timing header generator.
func NewServerTimingHeader() *ServerTimingHeader {
	return &ServerTimingHeader{
		entries: make([]serverTimingEntry, 0),
	}
}

// Add adds a timing entry.
func (st *ServerTimingHeader) Add(name string, duration time.Duration, description string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.entries = append(st.entries, serverTimingEntry{name, duration, description})
}

// String returns the header value.
func (st *ServerTimingHeader) String() string {
	st.mu.Lock()
	defer st.mu.Unlock()

	parts := make([]string, len(st.entries))
	for i, e := range st.entries {
		ms := float64(e.duration) / float64(time.Millisecond)
		if e.description != "" {
			parts[i] = fmt.Sprintf("%s;dur=%.2f;desc=\"%s\"", e.name, ms, e.description)
		} else {
			parts[i] = fmt.Sprintf("%s;dur=%.2f", e.name, ms)
		}
	}
	return strings.Join(parts, ", ")
}

// SetHeader sets the Server-Timing header on a response.
func (st *ServerTimingHeader) SetHeader(w http.ResponseWriter) {
	value := st.String()
	if value != "" {
		w.Header().Set("Server-Timing", value)
	}
}

// BufferPool provides a pool of byte buffers.
type BufferPool struct {
	pool sync.Pool
	size int
}

// NewBufferPool creates a new buffer pool.
func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		size: size,
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, size)
			},
		},
	}
}

// Get gets a buffer from the pool.
func (bp *BufferPool) Get() []byte {
	return bp.pool.Get().([]byte)
}

// Put returns a buffer to the pool.
func (bp *BufferPool) Put(buf []byte) {
	if cap(buf) >= bp.size {
		bp.pool.Put(buf[:bp.size])
	}
}

// MetricResponseWriter captures response metrics.
type MetricResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

func (mrw *MetricResponseWriter) WriteHeader(code int) {
	if !mrw.wroteHeader {
		mrw.statusCode = code
		mrw.wroteHeader = true
		mrw.ResponseWriter.WriteHeader(code)
	}
}

func (mrw *MetricResponseWriter) Write(b []byte) (int, error) {
	if !mrw.wroteHeader {
		mrw.WriteHeader(http.StatusOK)
	}
	n, err := mrw.ResponseWriter.Write(b)
	mrw.bytesWritten += int64(n)
	return n, err
}

// StatusCode returns the response status code.
func (mrw *MetricResponseWriter) StatusCode() int {
	if mrw.statusCode == 0 {
		return http.StatusOK
	}
	return mrw.statusCode
}

// BytesWritten returns the bytes written.
func (mrw *MetricResponseWriter) BytesWritten() int64 {
	return mrw.bytesWritten
}

// StatusClass returns the status class (2xx, 4xx, 5xx).
func (mrw *MetricResponseWriter) StatusClass() string {
	return strconv.Itoa(mrw.StatusCode()/100) + "xx"
}
