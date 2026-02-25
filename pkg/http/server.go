// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the Business Source License 1.1 (BSL 1.1).
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

package http

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"unheaded/pkg/ports"
)

// Server wraps the standard http.Server with additional functionality
type Server struct {
	// Router is the HTTP router
	Router *Router

	// server is the underlying http.Server
	server *http.Server

	// listener is the network listener
	listener net.Listener

	// shutdownTimeout is the timeout for graceful shutdown
	shutdownTimeout time.Duration

	// running indicates if the server is running
	running bool

	// mu protects the running state
	mu sync.Mutex

	// onShutdown is called when the server is shutting down
	onShutdown []func()

	// onStart is called when the server starts
	onStart []func()

	// logger is the server logger
	logger *log.Logger
}

// ServerConfig holds server configuration
type ServerConfig struct {
	// Addr is the address to listen on (default: ":20000")
	Addr string

	// ReadTimeout is the maximum duration for reading the entire request
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the response
	WriteTimeout time.Duration

	// IdleTimeout is the maximum amount of time to wait for the next request
	IdleTimeout time.Duration

	// ReadHeaderTimeout is the amount of time allowed to read request headers
	ReadHeaderTimeout time.Duration

	// MaxHeaderBytes is the maximum number of bytes the server will read parsing the request header
	MaxHeaderBytes int

	// ShutdownTimeout is the timeout for graceful shutdown (default: 30s)
	ShutdownTimeout time.Duration

	// TLSConfig is the optional TLS configuration
	TLSConfig *tls.Config

	// Logger is the optional logger (default: log.Default())
	Logger *log.Logger
}

// DefaultServerConfig returns a ServerConfig with sensible defaults
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Addr:              ports.DefaultAddr(ports.DashboardBackend),
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
		ShutdownTimeout:   30 * time.Second,
	}
}

// NewServer creates a new Server with the given router
func NewServer(router *Router) *Server {
	config := DefaultServerConfig()
	return NewServerWithConfig(router, config)
}

// NewServerWithConfig creates a new Server with the given router and config
func NewServerWithConfig(router *Router, config ServerConfig) *Server {
	// Set defaults
	if config.Addr == "" {
		config.Addr = ports.DefaultAddr(ports.DashboardBackend)
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 30 * time.Second
	}
	if config.Logger == nil {
		config.Logger = log.Default()
	}

	srv := &http.Server{
		Addr:              config.Addr,
		Handler:           router,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		MaxHeaderBytes:    config.MaxHeaderBytes,
		TLSConfig:         config.TLSConfig,
	}

	return &Server{
		Router:          router,
		server:          srv,
		shutdownTimeout: config.ShutdownTimeout,
		logger:          config.Logger,
	}
}

// ListenAndServe starts the HTTP server
func (s *Server) ListenAndServe(addr ...string) error {
	if len(addr) > 0 && addr[0] != "" {
		s.server.Addr = addr[0]
	}

	// Create listener
	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = ln

	return s.serve()
}

// ListenAndServeTLS starts the HTTPS server
func (s *Server) ListenAndServeTLS(addr, certFile, keyFile string) error {
	if addr != "" {
		s.server.Addr = addr
	}

	// Create listener
	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = ln

	// Load TLS certificate
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	if s.server.TLSConfig == nil {
		s.server.TLSConfig = &tls.Config{}
	}
	s.server.TLSConfig.Certificates = []tls.Certificate{cert}

	// Wrap listener with TLS
	tlsListener := tls.NewListener(ln, s.server.TLSConfig)
	s.listener = tlsListener

	return s.serve()
}

// Serve starts serving on an existing listener
func (s *Server) Serve(ln net.Listener) error {
	s.listener = ln
	return s.serve()
}

// serve is the internal serve method
func (s *Server) serve() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("server is already running")
	}
	s.running = true
	s.mu.Unlock()

	// Call onStart callbacks
	for _, fn := range s.onStart {
		fn()
	}

	s.logger.Printf("Server listening on %s", s.listener.Addr().String())

	err := s.server.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Println("Shutting down server...")

	// Call onShutdown callbacks
	for _, fn := range s.onShutdown {
		fn()
	}

	return s.server.Shutdown(ctx)
}

// GracefulShutdown shuts down the server with the configured timeout
func (s *Server) GracefulShutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	return s.Shutdown(ctx)
}

// ForceShutdown immediately closes the server
func (s *Server) ForceShutdown() error {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	return s.server.Close()
}

// OnShutdown registers a callback to be called on shutdown
func (s *Server) OnShutdown(fn func()) {
	s.onShutdown = append(s.onShutdown, fn)
}

// OnStart registers a callback to be called on start
func (s *Server) OnStart(fn func()) {
	s.onStart = append(s.onStart, fn)
}

// Running returns whether the server is running
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Addr returns the server's address
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.server.Addr
}

// --- Convenience Methods on Router ---

// ListenAndServe starts an HTTP server with the router
func (r *Router) ListenAndServe(addr string) error {
	server := NewServer(r)
	return server.ListenAndServeWithGracefulShutdown(addr)
}

// ListenAndServeTLS starts an HTTPS server with the router
func (r *Router) ListenAndServeTLS(addr, certFile, keyFile string) error {
	server := NewServer(r)
	return server.ListenAndServeTLSWithGracefulShutdown(addr, certFile, keyFile)
}

// Run starts the server (alias for ListenAndServe)
func (r *Router) Run(addr ...string) error {
	address := ports.DefaultAddr(ports.DashboardBackend)
	if len(addr) > 0 {
		address = addr[0]
	}
	return r.ListenAndServe(address)
}

// --- Graceful Shutdown with Signal Handling ---

// ListenAndServeWithGracefulShutdown starts the server with automatic graceful shutdown
func (s *Server) ListenAndServeWithGracefulShutdown(addr ...string) error {
	if len(addr) > 0 && addr[0] != "" {
		s.server.Addr = addr[0]
	}

	// Create channel for shutdown signal
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Create channel for server errors
	serverErr := make(chan error, 1)

	// Start server in goroutine
	go func() {
		if err := s.ListenAndServe(); err != nil {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case sig := <-shutdown:
		s.logger.Printf("Received signal %v, initiating graceful shutdown...", sig)
		return s.GracefulShutdown()
	case err := <-serverErr:
		return err
	}
}

// ListenAndServeTLSWithGracefulShutdown starts the TLS server with automatic graceful shutdown
func (s *Server) ListenAndServeTLSWithGracefulShutdown(addr, certFile, keyFile string) error {
	// Create channel for shutdown signal
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Create channel for server errors
	serverErr := make(chan error, 1)

	// Start server in goroutine
	go func() {
		if err := s.ListenAndServeTLS(addr, certFile, keyFile); err != nil {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case sig := <-shutdown:
		s.logger.Printf("Received signal %v, initiating graceful shutdown...", sig)
		return s.GracefulShutdown()
	case err := <-serverErr:
		return err
	}
}

// --- HTTP/2 Support ---

// EnableHTTP2 enables HTTP/2 support on the server
func (s *Server) EnableHTTP2() {
	// HTTP/2 is enabled by default when using TLS in Go 1.6+
	// This method exists for explicit configuration
	if s.server.TLSConfig == nil {
		s.server.TLSConfig = &tls.Config{}
	}

	// Ensure HTTP/2 is in the NextProtos list
	hasH2 := false
	for _, proto := range s.server.TLSConfig.NextProtos {
		if proto == "h2" {
			hasH2 = true
			break
		}
	}
	if !hasH2 {
		s.server.TLSConfig.NextProtos = append(s.server.TLSConfig.NextProtos, "h2")
	}
}

// --- Health Check Support ---

// HealthCheck represents a health check function
type HealthCheck func() error

// HealthCheckConfig holds health check configuration
type HealthCheckConfig struct {
	// Path is the health check endpoint path (default: "/health")
	Path string

	// LivenessPath is the liveness probe path (default: "/health/live")
	LivenessPath string

	// ReadinessPath is the readiness probe path (default: "/health/ready")
	ReadinessPath string

	// Checks are the health check functions
	Checks map[string]HealthCheck
}

// RegisterHealthChecks registers health check endpoints on the router
func (r *Router) RegisterHealthChecks(config HealthCheckConfig) {
	// Set defaults
	if config.Path == "" {
		config.Path = "/health"
	}
	if config.LivenessPath == "" {
		config.LivenessPath = "/health/live"
	}
	if config.ReadinessPath == "" {
		config.ReadinessPath = "/health/ready"
	}

	// Main health check
	r.GET(config.Path, func(c *Context) {
		c.JSON(http.StatusOK, map[string]string{
			"status": "healthy",
		})
	})

	// Liveness probe (is the server alive?)
	r.GET(config.LivenessPath, func(c *Context) {
		c.JSON(http.StatusOK, map[string]string{
			"status": "alive",
		})
	})

	// Readiness probe (is the server ready to serve traffic?)
	r.GET(config.ReadinessPath, func(c *Context) {
		results := make(map[string]string)
		allHealthy := true

		for name, check := range config.Checks {
			if err := check(); err != nil {
				results[name] = err.Error()
				allHealthy = false
			} else {
				results[name] = "ok"
			}
		}

		status := http.StatusOK
		statusText := "ready"
		if !allHealthy {
			status = http.StatusServiceUnavailable
			statusText = "not_ready"
		}

		c.JSON(status, map[string]any{
			"status": statusText,
			"checks": results,
		})
	})
}

// --- Metrics Support ---

// Metrics holds server metrics
type Metrics struct {
	mu              sync.RWMutex
	RequestCount    int64
	ErrorCount      int64
	TotalLatency    time.Duration
	ActiveRequests  int64
	RequestsByPath  map[string]int64
	StatusCodes     map[int]int64
	BytesSent       int64
	BytesReceived   int64
}

// NewMetrics creates a new Metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		RequestsByPath: make(map[string]int64),
		StatusCodes:    make(map[int]int64),
	}
}

// Record records a request
func (m *Metrics) Record(path string, statusCode int, latency time.Duration, bytesIn, bytesOut int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.RequestCount++
	m.TotalLatency += latency
	m.RequestsByPath[path]++
	m.StatusCodes[statusCode]++
	m.BytesSent += bytesOut
	m.BytesReceived += bytesIn

	if statusCode >= 400 {
		m.ErrorCount++
	}
}

// Snapshot returns a copy of the current metrics
func (m *Metrics) Snapshot() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgLatency := time.Duration(0)
	if m.RequestCount > 0 {
		avgLatency = m.TotalLatency / time.Duration(m.RequestCount)
	}

	return map[string]any{
		"request_count":    m.RequestCount,
		"error_count":      m.ErrorCount,
		"avg_latency_ms":   avgLatency.Milliseconds(),
		"active_requests":  m.ActiveRequests,
		"bytes_sent":       m.BytesSent,
		"bytes_received":   m.BytesReceived,
		"status_codes":     m.StatusCodes,
		"requests_by_path": m.RequestsByPath,
	}
}

// MetricsMiddleware returns a middleware that collects metrics
func MetricsMiddleware(metrics *Metrics) MiddlewareFunc {
	return func(c *Context) {
		start := time.Now()

		// Track active requests
		metrics.mu.Lock()
		metrics.ActiveRequests++
		metrics.mu.Unlock()

		defer func() {
			metrics.mu.Lock()
			metrics.ActiveRequests--
			metrics.mu.Unlock()
		}()

		// Process request
		c.Next()

		// Record metrics
		latency := time.Since(start)

		// Get status code and bytes from response writer
		statusCode := http.StatusOK
		bytesOut := int64(0)
		if rw, ok := c.Writer.(*ResponseWriter); ok {
			statusCode = rw.Status()
			bytesOut = int64(rw.Size())
		}

		metrics.Record(c.Path(), statusCode, latency, c.Request.ContentLength, bytesOut)
	}
}

// RegisterMetricsEndpoint registers a metrics endpoint
func (r *Router) RegisterMetricsEndpoint(path string, metrics *Metrics) {
	if path == "" {
		path = "/metrics"
	}

	r.GET(path, func(c *Context) {
		c.JSON(http.StatusOK, metrics.Snapshot())
	})
}
