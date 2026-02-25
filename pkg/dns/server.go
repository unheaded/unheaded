// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package dns

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Server errors
var (
	ErrServerClosed  = errors.New("dns: server closed")
	ErrServerRunning = errors.New("dns: server already running")
)

// Server is a DNS server supporting UDP and TCP
type Server struct {
	mu           sync.RWMutex
	config       *ServerConfig
	resolver     *Resolver
	zones        *ZoneManager
	udpListener  *net.UDPConn
	tcpListener  net.Listener
	running      atomic.Bool
	shutdown     chan struct{}
	wg           sync.WaitGroup
	stats        ServerStats
	handler      Handler
	middlewares  []Middleware
	logger       *log.Logger
}

// ServerConfig holds server configuration
type ServerConfig struct {
	UDPAddr        string        // UDP listen address (e.g., ":53")
	TCPAddr        string        // TCP listen address (e.g., ":53")
	ReadTimeout    time.Duration // Read timeout for TCP connections
	WriteTimeout   time.Duration // Write timeout for TCP connections
	MaxTCPConns    int           // Maximum concurrent TCP connections
	MaxUDPSize     int           // Maximum UDP message size
	EnableTCP      bool          // Enable TCP listener
	EnableUDP      bool          // Enable UDP listener
	ResolverConfig *ResolverConfig
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		UDPAddr:        ":53",
		TCPAddr:        ":53",
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxTCPConns:    100,
		MaxUDPSize:     4096,
		EnableTCP:      true,
		EnableUDP:      true,
		ResolverConfig: DefaultResolverConfig(),
	}
}

// ServerStats holds server statistics
type ServerStats struct {
	UDPQueries   uint64
	TCPQueries   uint64
	UDPErrors    uint64
	TCPErrors    uint64
	TruncatedUDP uint64
}

// Handler handles DNS queries
type Handler interface {
	ServeDNS(ctx context.Context, w ResponseWriter, r *Message)
}

// HandlerFunc is a function that implements Handler
type HandlerFunc func(ctx context.Context, w ResponseWriter, r *Message)

// ServeDNS implements Handler
func (f HandlerFunc) ServeDNS(ctx context.Context, w ResponseWriter, r *Message) {
	f(ctx, w, r)
}

// ResponseWriter writes DNS responses
type ResponseWriter interface {
	WriteMsg(*Message) error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	IsTCP() bool
	Close() error
}

// Middleware wraps a handler
type Middleware func(Handler) Handler

// NewServer creates a new DNS server
func NewServer(config *ServerConfig) *Server {
	if config == nil {
		config = DefaultServerConfig()
	}

	zones := NewZoneManager()
	resolver := NewResolver(config.ResolverConfig, zones)

	s := &Server{
		config:   config,
		resolver: resolver,
		zones:    zones,
		shutdown: make(chan struct{}),
	}

	// Default handler
	s.handler = HandlerFunc(s.defaultHandler)

	return s
}

// SetHandler sets the query handler
func (s *Server) SetHandler(h Handler) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

// Use adds middleware to the server
func (s *Server) Use(m Middleware) {
	s.mu.Lock()
	s.middlewares = append(s.middlewares, m)
	s.mu.Unlock()
}

// SetLogger sets the server logger
func (s *Server) SetLogger(l *log.Logger) {
	s.mu.Lock()
	s.logger = l
	s.mu.Unlock()
}

// Zones returns the zone manager
func (s *Server) Zones() *ZoneManager {
	return s.zones
}

// Resolver returns the resolver
func (s *Server) Resolver() *Resolver {
	return s.resolver
}

// Start starts the DNS server
func (s *Server) Start() error {
	if s.running.Load() {
		return ErrServerRunning
	}

	s.running.Store(true)
	s.shutdown = make(chan struct{})

	var errUDP, errTCP error

	if s.config.EnableUDP {
		errUDP = s.startUDP()
	}

	if s.config.EnableTCP {
		errTCP = s.startTCP()
	}

	if errUDP != nil {
		return errUDP
	}
	if errTCP != nil {
		return errTCP
	}

	return nil
}

// startUDP starts the UDP listener
func (s *Server) startUDP() error {
	addr, err := net.ResolveUDPAddr("udp", s.config.UDPAddr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	s.udpListener = conn

	s.wg.Add(1)
	go s.serveUDP()

	s.log("DNS UDP server listening on %s", s.config.UDPAddr)
	return nil
}

// startTCP starts the TCP listener
func (s *Server) startTCP() error {
	listener, err := net.Listen("tcp", s.config.TCPAddr)
	if err != nil {
		return err
	}

	s.tcpListener = listener

	s.wg.Add(1)
	go s.serveTCP()

	s.log("DNS TCP server listening on %s", s.config.TCPAddr)
	return nil
}

// serveUDP handles UDP requests
func (s *Server) serveUDP() {
	defer s.wg.Done()

	buf := make([]byte, s.config.MaxUDPSize)

	for {
		select {
		case <-s.shutdown:
			return
		default:
		}

		s.udpListener.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := s.udpListener.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			select {
			case <-s.shutdown:
				return
			default:
				atomic.AddUint64(&s.stats.UDPErrors, 1)
				continue
			}
		}

		atomic.AddUint64(&s.stats.UDPQueries, 1)

		// Handle in goroutine for concurrency
		data := make([]byte, n)
		copy(data, buf[:n])

		go s.handleUDP(data, addr)
	}
}

// handleUDP processes a single UDP request
func (s *Server) handleUDP(data []byte, addr *net.UDPAddr) {
	msg := &Message{}
	if err := msg.Unpack(data); err != nil {
		atomic.AddUint64(&s.stats.UDPErrors, 1)
		return
	}

	w := &udpResponseWriter{
		conn:       s.udpListener,
		remoteAddr: addr,
		localAddr:  s.udpListener.LocalAddr(),
	}

	ctx := context.Background()
	s.serve(ctx, w, msg)
}

// serveTCP handles TCP connections
func (s *Server) serveTCP() {
	defer s.wg.Done()

	semaphore := make(chan struct{}, s.config.MaxTCPConns)

	for {
		select {
		case <-s.shutdown:
			return
		default:
		}

		s.tcpListener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
		conn, err := s.tcpListener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			select {
			case <-s.shutdown:
				return
			default:
				atomic.AddUint64(&s.stats.TCPErrors, 1)
				continue
			}
		}

		atomic.AddUint64(&s.stats.TCPQueries, 1)

		// Limit concurrent connections
		select {
		case semaphore <- struct{}{}:
			go func() {
				defer func() { <-semaphore }()
				s.handleTCP(conn)
			}()
		default:
			conn.Close()
		}
	}
}

// handleTCP processes TCP requests on a connection
func (s *Server) handleTCP(conn net.Conn) {
	defer conn.Close()

	for {
		conn.SetReadDeadline(time.Now().Add(s.config.ReadTimeout))

		// Read 2-byte length prefix
		lenBuf := make([]byte, 2)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return
		}

		msgLen := int(lenBuf[0])<<8 | int(lenBuf[1])
		if msgLen > 65535 {
			return
		}

		// Read message
		data := make([]byte, msgLen)
		if _, err := io.ReadFull(conn, data); err != nil {
			return
		}

		msg := &Message{}
		if err := msg.Unpack(data); err != nil {
			atomic.AddUint64(&s.stats.TCPErrors, 1)
			return
		}

		w := &tcpResponseWriter{
			conn:       conn,
			localAddr:  conn.LocalAddr(),
			remoteAddr: conn.RemoteAddr(),
			timeout:    s.config.WriteTimeout,
		}

		ctx := context.Background()
		s.serve(ctx, w, msg)
	}
}

// serve handles a DNS request through the middleware chain
func (s *Server) serve(ctx context.Context, w ResponseWriter, r *Message) {
	s.mu.RLock()
	handler := s.handler
	middlewares := s.middlewares
	s.mu.RUnlock()

	// Build middleware chain
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	handler.ServeDNS(ctx, w, r)
}

// defaultHandler is the default query handler
func (s *Server) defaultHandler(ctx context.Context, w ResponseWriter, r *Message) {
	resp, err := s.resolver.Resolve(ctx, r)
	if err != nil {
		s.log("resolver error: %v", err)
	}

	if resp != nil {
		// Check for truncation on UDP
		if !w.IsTCP() {
			data, _ := resp.Pack()
			if len(data) > MaxUDPSize {
				atomic.AddUint64(&s.stats.TruncatedUDP, 1)
				resp.Header.Flags |= FlagTC
				// Truncate answers
				if len(resp.Answers) > 0 {
					resp.Answers = resp.Answers[:1]
				}
				resp.Authority = nil
				resp.Additional = nil
			}
		}

		if err := w.WriteMsg(resp); err != nil {
			s.log("write error: %v", err)
		}
	}
}

// Stop stops the DNS server gracefully
func (s *Server) Stop() error {
	if !s.running.Load() {
		return ErrServerClosed
	}

	close(s.shutdown)

	if s.udpListener != nil {
		s.udpListener.Close()
	}

	if s.tcpListener != nil {
		s.tcpListener.Close()
	}

	s.wg.Wait()
	s.running.Store(false)

	s.resolver.Close()

	return nil
}

// Stats returns server statistics
func (s *Server) Stats() ServerStats {
	return ServerStats{
		UDPQueries:   atomic.LoadUint64(&s.stats.UDPQueries),
		TCPQueries:   atomic.LoadUint64(&s.stats.TCPQueries),
		UDPErrors:    atomic.LoadUint64(&s.stats.UDPErrors),
		TCPErrors:    atomic.LoadUint64(&s.stats.TCPErrors),
		TruncatedUDP: atomic.LoadUint64(&s.stats.TruncatedUDP),
	}
}

// log writes a log message
func (s *Server) log(format string, args ...interface{}) {
	s.mu.RLock()
	logger := s.logger
	s.mu.RUnlock()

	if logger != nil {
		logger.Printf(format, args...)
	}
}

// udpResponseWriter implements ResponseWriter for UDP
type udpResponseWriter struct {
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	localAddr  net.Addr
}

func (w *udpResponseWriter) WriteMsg(msg *Message) error {
	data, err := msg.Pack()
	if err != nil {
		return err
	}
	_, err = w.conn.WriteToUDP(data, w.remoteAddr)
	return err
}

func (w *udpResponseWriter) LocalAddr() net.Addr {
	return w.localAddr
}

func (w *udpResponseWriter) RemoteAddr() net.Addr {
	return w.remoteAddr
}

func (w *udpResponseWriter) IsTCP() bool {
	return false
}

func (w *udpResponseWriter) Close() error {
	return nil // UDP is connectionless
}

// tcpResponseWriter implements ResponseWriter for TCP
type tcpResponseWriter struct {
	conn       net.Conn
	localAddr  net.Addr
	remoteAddr net.Addr
	timeout    time.Duration
}

func (w *tcpResponseWriter) WriteMsg(msg *Message) error {
	data, err := msg.Pack()
	if err != nil {
		return err
	}

	// TCP DNS uses 2-byte length prefix
	tcpData := make([]byte, 2+len(data))
	tcpData[0] = byte(len(data) >> 8)
	tcpData[1] = byte(len(data))
	copy(tcpData[2:], data)

	w.conn.SetWriteDeadline(time.Now().Add(w.timeout))
	_, err = w.conn.Write(tcpData)
	return err
}

func (w *tcpResponseWriter) LocalAddr() net.Addr {
	return w.localAddr
}

func (w *tcpResponseWriter) RemoteAddr() net.Addr {
	return w.remoteAddr
}

func (w *tcpResponseWriter) IsTCP() bool {
	return true
}

func (w *tcpResponseWriter) Close() error {
	return w.conn.Close()
}

// LoggingMiddleware logs DNS queries
func LoggingMiddleware(logger *log.Logger) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
			start := time.Now()
			next.ServeDNS(ctx, w, r)
			elapsed := time.Since(start)

			if len(r.Questions) > 0 {
				q := r.Questions[0]
				proto := "UDP"
				if w.IsTCP() {
					proto = "TCP"
				}
				logger.Printf("[%s] %s %s %s from %s (%v)",
					proto, q.Name, TypeToString(q.Type),
					ClassToString(q.Class), w.RemoteAddr(), elapsed)
			}
		})
	}
}

// RateLimitMiddleware limits queries per client
func RateLimitMiddleware(rps int, burst int) Middleware {
	limiters := sync.Map{}

	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
			addr := w.RemoteAddr().String()
			host, _, _ := net.SplitHostPort(addr)

			// Get or create limiter for this client
			val, _ := limiters.LoadOrStore(host, newRateLimiter(rps, burst))
			limiter := val.(*rateLimiter)

			if !limiter.allow() {
				// Send REFUSED response
				resp := NewResponse(r)
				resp.Header.SetRcode(RcodeRefused)
				w.WriteMsg(resp)
				return
			}

			next.ServeDNS(ctx, w, r)
		})
	}
}

// rateLimiter implements a simple token bucket rate limiter
type rateLimiter struct {
	mu       sync.Mutex
	tokens   int
	maxBurst int
	rate     int
	lastTime time.Time
}

func newRateLimiter(rate, burst int) *rateLimiter {
	return &rateLimiter{
		tokens:   burst,
		maxBurst: burst,
		rate:     rate,
		lastTime: time.Now(),
	}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime)
	rl.lastTime = now

	// Add tokens based on elapsed time
	rl.tokens += int(elapsed.Seconds() * float64(rl.rate))
	if rl.tokens > rl.maxBurst {
		rl.tokens = rl.maxBurst
	}

	if rl.tokens <= 0 {
		return false
	}

	rl.tokens--
	return true
}

// ACLMiddleware filters queries by source IP
func ACLMiddleware(allowedNets []*net.IPNet, deniedNets []*net.IPNet) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
			addr := w.RemoteAddr().String()
			host, _, _ := net.SplitHostPort(addr)
			ip := net.ParseIP(host)

			if ip == nil {
				resp := NewResponse(r)
				resp.Header.SetRcode(RcodeRefused)
				w.WriteMsg(resp)
				return
			}

			// Check denied first
			for _, net := range deniedNets {
				if net.Contains(ip) {
					resp := NewResponse(r)
					resp.Header.SetRcode(RcodeRefused)
					w.WriteMsg(resp)
					return
				}
			}

			// Check allowed if specified
			if len(allowedNets) > 0 {
				allowed := false
				for _, net := range allowedNets {
					if net.Contains(ip) {
						allowed = true
						break
					}
				}
				if !allowed {
					resp := NewResponse(r)
					resp.Header.SetRcode(RcodeRefused)
					w.WriteMsg(resp)
					return
				}
			}

			next.ServeDNS(ctx, w, r)
		})
	}
}

// RecoveryMiddleware recovers from panics in handlers
func RecoveryMiddleware(logger *log.Logger) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
			defer func() {
				if err := recover(); err != nil {
					if logger != nil {
						logger.Printf("panic in DNS handler: %v", err)
					}
					resp := NewResponse(r)
					resp.Header.SetRcode(RcodeServerFailure)
					w.WriteMsg(resp)
				}
			}()
			next.ServeDNS(ctx, w, r)
		})
	}
}

// MetricsMiddleware collects query metrics
type MetricsMiddleware struct {
	mu         sync.RWMutex
	queryTypes map[uint16]uint64
	clients    map[string]uint64
	rcodes     map[uint8]uint64
}

// NewMetricsMiddleware creates a new metrics middleware
func NewMetricsMiddleware() *MetricsMiddleware {
	return &MetricsMiddleware{
		queryTypes: make(map[uint16]uint64),
		clients:    make(map[string]uint64),
		rcodes:     make(map[uint8]uint64),
	}
}

// Middleware returns the middleware function
func (m *MetricsMiddleware) Middleware() Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, w ResponseWriter, r *Message) {
			// Record query type
			if len(r.Questions) > 0 {
				m.mu.Lock()
				m.queryTypes[r.Questions[0].Type]++
				m.mu.Unlock()
			}

			// Record client
			addr := w.RemoteAddr().String()
			host, _, _ := net.SplitHostPort(addr)
			m.mu.Lock()
			m.clients[host]++
			m.mu.Unlock()

			next.ServeDNS(ctx, w, r)
		})
	}
}

// GetQueryTypes returns query type statistics
func (m *MetricsMiddleware) GetQueryTypes() map[uint16]uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[uint16]uint64)
	for k, v := range m.queryTypes {
		result[k] = v
	}
	return result
}

// GetClients returns client statistics
func (m *MetricsMiddleware) GetClients() map[string]uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]uint64)
	for k, v := range m.clients {
		result[k] = v
	}
	return result
}
