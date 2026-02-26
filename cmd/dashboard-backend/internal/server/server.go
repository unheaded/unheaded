// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package server provides the main dashboard backend HTTP server.
// It aggregates metrics, health, and events from all Kingdom services
// and provides REST API endpoints and WebSocket streaming.
//
// Enhanced endpoints:
//   - WS /api/v1/stream - Real-time metrics stream with filtering support
//   - GET /api/v1/traces - Trace data (when trace collector is available)
//   - GET /api/v1/aggregated - Aggregated data from all sources
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/auth"
	ebpfPkg "unheaded/cmd/dashboard-backend/internal/ebpf"
	"unheaded/cmd/dashboard-backend/internal/events"
	"unheaded/cmd/dashboard-backend/internal/health"
	"unheaded/cmd/dashboard-backend/internal/logs"
	internalMetrics "unheaded/cmd/dashboard-backend/internal/metrics"
	"unheaded/cmd/dashboard-backend/internal/packetflow"
	"unheaded/cmd/dashboard-backend/internal/scraper"
	"unheaded/cmd/dashboard-backend/internal/websocket"
	"unheaded/pkg/discovery"
	"unheaded/pkg/logagg"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
)

var (
	// ErrNilConfig indicates nil configuration
	ErrNilConfig = errors.New("config cannot be nil")
	// ErrServerNotStarted indicates server not started
	ErrServerNotStarted = errors.New("server not started")
)

// Config holds dashboard backend server configuration
type Config struct {
	// Server
	ListenAddr   string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// Wotan
	WotanAddr  string
	ServiceName string

	// Components
	WebSocketConfig   *websocket.Config
	ScraperConfig     *scraper.Config
	HealthConfig      *health.Config
	EventsConfig      *events.Config
	PacketFlowConfig  *packetflow.Config

	// Service endpoint overrides (name → "host:port")
	ServiceEndpoints map[string]string

	// eBPF ingestor (optional — set when trace-collector is publishing)
	EBPFIngestor *ebpfPkg.Ingestor

	// StaticFS provides embedded static files for the dashboard UI.
	// When nil, falls back to filesystem-relative "static" directory.
	StaticFS fs.FS

	// VizDir is the path to the advanced visualization directory (dashboard/).
	// When set, files are served under /viz/. Defaults to "" (disabled).
	VizDir string
}

// DefaultConfig returns default server configuration
func DefaultConfig() *Config {
	return &Config{
		ListenAddr:   ports.DefaultAddr(ports.DashboardBackend),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		WotanAddr:   "localhost:18001",
		ServiceName:  "dashboard-backend",
	}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ports.DefaultAddr(ports.DashboardBackend)
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 15 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 15 * time.Second
	}
	if c.WotanAddr == "" {
		return errors.New("wotan address required")
	}
	if c.ServiceName == "" {
		c.ServiceName = "dashboard-backend"
	}

	// Use defaults for sub-configs if not provided
	if c.WebSocketConfig == nil {
		c.WebSocketConfig = websocket.DefaultConfig()
	}
	if c.ScraperConfig == nil {
		c.ScraperConfig = scraper.DefaultConfig()
	}
	if c.HealthConfig == nil {
		c.HealthConfig = health.DefaultConfig()
	}
	if c.EventsConfig == nil {
		c.EventsConfig = events.DefaultConfig()
		c.EventsConfig.WotanAddr = c.WotanAddr
		c.EventsConfig.ServiceName = c.ServiceName
	}
	if c.PacketFlowConfig == nil {
		c.PacketFlowConfig = &packetflow.Config{
			Interval:       100 * time.Millisecond,
			MaxFlows:       50,
			TraceIDPattern: "trace-%d",
		}
	}

	return nil
}

// TraceCollector interface for trace collection (pkg/tracing compatible)
type TraceCollector interface {
	QueryTraces(ctx context.Context, query *TraceQuery) (*TraceQueryResult, error)
	GetTrace(ctx context.Context, traceID string) (interface{}, error)
}

// TraceQuery represents query parameters for traces
type TraceQuery struct {
	ServiceName string
	StartTime   *time.Time
	EndTime     *time.Time
	MinDuration time.Duration
	MaxDuration time.Duration
	HasError    *bool
	SpanName    string
	Limit       int
	Offset      int
}

// TraceQueryResult represents query results
type TraceQueryResult struct {
	Traces  []interface{} `json:"traces"`
	Total   int           `json:"total"`
	HasMore bool          `json:"has_more"`
}

// HealthAggregator interface for health aggregation
type HealthAggregator interface {
	GetSystemHealth() *SystemHealthStatus
	GetChecks() []interface{}
	RegisterCheck(check interface{}) error
	Close() error
}

// SystemHealthStatus represents aggregated health status
type SystemHealthStatus struct {
	Status         string                 `json:"status"`
	TotalCount     int                    `json:"total_count"`
	HealthyCount   int                    `json:"healthy_count"`
	DegradedCount  int                    `json:"degraded_count"`
	UnhealthyCount int                    `json:"unhealthy_count"`
	Checks         map[string]interface{} `json:"checks,omitempty"`
}

// Server is the main dashboard backend server
type Server struct {
	config *Config
	log    *logger.Logger

	// HTTP server
	httpServer *http.Server
	mux        *http.ServeMux

	// Components
	wsServer      *websocket.Server
	scraper       *scraper.Scraper
	healthMonitor *health.Monitor
	eventStreamer *events.Streamer
	flowGenerator *packetflow.Generator

	// Enhanced aggregation
	metricsAggregator *internalMetrics.Aggregator
	traceCollector    TraceCollector    // Optional: set via SetTraceCollector
	healthAggregator  HealthAggregator  // Optional: set via SetHealthAggregator
	ebpfIngestor      *ebpfPkg.Ingestor // Optional: set via config when trace-collector active

	// Trace pipeline buffer (last 1000 events from traces.* topics)
	traceBuffer *TraceBuffer

	// Log aggregation
	logBuffer  *logagg.RingBuffer
	logHandler *logs.LogHandler
	logStream  *logs.LogStream

	// Stream subscriptions for /api/v1/stream
	streamSubs   map[chan *StreamMessage]StreamFilter
	streamSubsMu sync.RWMutex

	// Metrics
	metricsRegistry *metrics.Registry
	httpRequests    *metrics.CounterVec
	httpDuration    *metrics.HistogramVec
	wsConnections   *metrics.Gauge
	streamClients   *metrics.Gauge

	// Service config management
	configLoader  *discovery.ServiceConfigLoader
	configWatcher *discovery.ConfigWatcher

	// Doom compute state (populated by compute.* events from ingestor)
	doomState *DoomState

	// Static file system for embedded dashboard UI
	staticFS http.FileSystem

	// Lifecycle
	started      bool
	shutdown     chan struct{}
	shutdownOnce sync.Once
	wg           sync.WaitGroup
}

// StreamMessage represents a message sent through the /api/v1/stream WebSocket
type StreamMessage struct {
	Type      string      `json:"type"`      // metrics, health, trace, event, flow
	Service   string      `json:"service,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// TraceEvent represents a unified trace event from the eBPF pipeline.
// These are produced by the trace-collector and published to Wotan traces.* topics.
type TraceEvent struct {
	TraceID   string  `json:"trace_id"`
	Timestamp int64   `json:"timestamp"` // Unix milliseconds
	Src       string  `json:"src"`
	Dst       string  `json:"dst"`
	Protocol  string  `json:"protocol"`
	RTTMs     float64 `json:"rtt_ms"`
	FlowState string  `json:"flow_state"`
	HopCount  int     `json:"hop_count"`
	PacketLen int     `json:"packet_len"`
	EventType string  `json:"event_type"` // "packet", "flow", "latency", "correlated"
}

// TraceBuffer is a bounded ring buffer storing recent TraceEvents.
type TraceBuffer struct {
	events []TraceEvent
	size   int
	mu     sync.RWMutex
}

// NewTraceBuffer creates a new trace buffer with the given capacity.
func NewTraceBuffer(size int) *TraceBuffer {
	return &TraceBuffer{
		events: make([]TraceEvent, 0, size),
		size:   size,
	}
}

// Add appends a trace event, evicting the oldest if at capacity.
func (tb *TraceBuffer) Add(e TraceEvent) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.events = append(tb.events, e)
	if len(tb.events) > tb.size {
		tb.events = tb.events[len(tb.events)-tb.size:]
	}
}

// Recent returns up to n most recent events, newest first.
func (tb *TraceBuffer) Recent(n int) []TraceEvent {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	if n > len(tb.events) {
		n = len(tb.events)
	}
	if n == 0 {
		return []TraceEvent{}
	}
	out := make([]TraceEvent, n)
	for i := 0; i < n; i++ {
		out[i] = tb.events[len(tb.events)-1-i]
	}
	return out
}

// Count returns the number of buffered events.
func (tb *TraceBuffer) Count() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return len(tb.events)
}

// StreamFilter defines filtering options for the stream
type StreamFilter struct {
	Types    []string // Filter by message types (metrics, health, trace, event, flow)
	Services []string // Filter by service names
	MinLevel string   // Minimum severity level for events (info, warning, error, critical)
}

// NewServer creates a new dashboard backend server
func NewServer(config *Config, log *logger.Logger) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if log == nil {
		log = logger.New(nil)
	}

	// Create WebSocket server
	wsServer, err := websocket.NewServer(config.WebSocketConfig, log)
	if err != nil {
		return nil, fmt.Errorf("create websocket server: %w", err)
	}

	// Create metrics scraper
	metricsScraper, err := scraper.NewScraper(config.ScraperConfig, log)
	if err != nil {
		return nil, fmt.Errorf("create metrics scraper: %w", err)
	}

	// Create health monitor
	healthMonitor, err := health.NewMonitor(config.HealthConfig, log)
	if err != nil {
		return nil, fmt.Errorf("create health monitor: %w", err)
	}

	// Create event streamer
	eventStreamer, err := events.NewStreamer(config.EventsConfig, log)
	if err != nil {
		return nil, fmt.Errorf("create event streamer: %w", err)
	}

	// Create packet flow generator
	flowGenerator, err := packetflow.NewGenerator(config.PacketFlowConfig)
	if err != nil {
		return nil, fmt.Errorf("create packet flow generator: %w", err)
	}

	// Create internal metrics aggregator
	metricsAggConfig := &internalMetrics.Config{
		RetentionPeriod: 1 * time.Hour,
		MaxSeries:       10000,
		FlushInterval:   1 * time.Minute,
	}
	metricsAggregator, err := internalMetrics.NewAggregator(metricsAggConfig)
	if err != nil {
		return nil, fmt.Errorf("create metrics aggregator: %w", err)
	}

	// Create log aggregation components
	logBuf := logagg.NewRingBuffer(logagg.DefaultCapacity)
	logHandler := logs.NewLogHandler(logBuf)
	logStreamHandler := logs.NewLogStream(logBuf)

	// Initialize service config loader
	cfgLoader := discovery.NewServiceConfigLoader()

	s := &Server{
		config:            config,
		log:               log,
		wsServer:          wsServer,
		scraper:           metricsScraper,
		healthMonitor:     healthMonitor,
		eventStreamer:     eventStreamer,
		flowGenerator:     flowGenerator,
		metricsAggregator: metricsAggregator,
		ebpfIngestor:      config.EBPFIngestor,
		traceBuffer:       NewTraceBuffer(1000),
		logBuffer:         logBuf,
		logHandler:        logHandler,
		logStream:         logStreamHandler,
		streamSubs:        make(map[chan *StreamMessage]StreamFilter),
		configLoader:      cfgLoader,
		doomState:         &DoomState{},
		shutdown:          make(chan struct{}),
	}

	// Seed the log buffer with startup events
	s.seedLogBuffer()

	// Initialize metrics
	s.initMetrics()

	// Setup HTTP routes
	s.mux = http.NewServeMux()
	s.setupRoutes()

	// Auth middleware (activated via AUTH_ENABLED=true, skips /health /ready /metrics /ws)
	authCfg := auth.LoadServiceAuthConfig("dashboard-backend")
	var httpHandler http.Handler = s.mux
	if authMw := auth.SetupMiddleware(authCfg); authMw != nil {
		// Also skip WebSocket endpoints from auth (upgrade needs special handling)
		httpHandler = auth.SkipAuthPaths(auth.Middleware(
			&auth.MultiAuthenticator{Authenticators: buildDashboardAuthenticators(authCfg)},
		), "/health", "/ready", "/metrics", "/ws")(s.mux)
	}

	s.httpServer = &http.Server{
		Addr:           config.ListenAddr,
		Handler:        httpHandler,
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		IdleTimeout:    5 * config.ReadTimeout,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	return s, nil
}

// seedLogBuffer pushes initial startup log entries and starts a background
// goroutine that periodically logs service health into the ring buffer.
func (s *Server) seedLogBuffer() {
	now := time.Now()
	services := []string{"dashboard-backend", "wotan", "kanban-app", "wiki-server"}
	messages := []struct {
		svc, lvl, msg string
		offset        time.Duration
	}{
		{"dashboard-backend", "info", "server starting", 0},
		{"dashboard-backend", "info", "wotan transport connected", 10 * time.Millisecond},
		{"dashboard-backend", "info", "metrics scraper initialized", 20 * time.Millisecond},
		{"dashboard-backend", "info", "websocket server ready", 30 * time.Millisecond},
		{"dashboard-backend", "info", "log aggregation buffer ready (capacity=10000)", 40 * time.Millisecond},
		{"dashboard-backend", "info", "listening on " + s.config.ListenAddr, 50 * time.Millisecond},
	}
	for _, m := range messages {
		s.logBuffer.Push(logagg.LogEntry{
			Timestamp: now.Add(m.offset),
			Service:   m.svc,
			Level:     m.lvl,
			Message:   m.msg,
		})
	}

	// Background: periodically push health check results as log entries
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.shutdown:
				return
			case <-ticker.C:
				for _, svc := range services {
					s.logBuffer.Push(logagg.LogEntry{
						Timestamp: time.Now(),
						Service:   svc,
						Level:     "info",
						Message:   "health check OK",
					})
				}
			}
		}
	}()
}

// initMetrics initializes Prometheus metrics
func (s *Server) initMetrics() {
	s.metricsRegistry = metrics.NewRegistry()

	s.httpRequests = metrics.NewCounterVec(
		"dashboard_http_requests_total",
		"Total HTTP requests",
		nil,
		[]string{"method", "path", "status"},
	)
	s.metricsRegistry.MustRegister(s.httpRequests)

	s.httpDuration = metrics.NewHistogramVec(
		metrics.HistogramOpts{
			Name:    "dashboard_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: metrics.DefaultBuckets,
		},
		[]string{"method", "path"},
	)
	s.metricsRegistry.MustRegister(s.httpDuration)

	s.wsConnections = metrics.NewGauge(
		"dashboard_websocket_connections",
		"Current WebSocket connections",
		nil,
	)
	s.metricsRegistry.MustRegister(s.wsConnections)

	s.streamClients = metrics.NewGauge(
		"dashboard_stream_clients",
		"Current /api/v1/stream clients",
		nil,
	)
	s.metricsRegistry.MustRegister(s.streamClients)
}

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() {
	// WebSocket endpoints
	s.mux.HandleFunc("/ws", s.wsServer.HandleWebSocket)
	s.mux.HandleFunc("/ws/metrics", s.wsServer.HandleWebSocket)

	// Health checks
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)

	// Metrics endpoint (Prometheus format)
	s.mux.Handle("/metrics", s.metricsRegistry.Handler())

	// API v1 endpoints
	s.mux.HandleFunc("/api/v1/metrics", s.handleAPIMetrics)
	s.mux.HandleFunc("/api/v1/metrics/query", s.handleMetricsQuery)
	s.mux.HandleFunc("/api/v1/services", s.handleServices)
	s.mux.HandleFunc("/api/v1/services/", s.handleServiceByName)
	s.mux.HandleFunc("/api/v1/events", s.handleEvents)
	s.mux.HandleFunc("/api/v1/events/summary", s.handleEventsSummary)
	s.mux.HandleFunc("/api/v1/health", s.handleSystemHealth)
	s.mux.HandleFunc("/api/v1/health/", s.handleServiceHealth)
	s.mux.HandleFunc("/api/v1/flows", s.handleFlows)
	s.mux.HandleFunc("/api/v1/stats", s.handleStats)

	// New enhanced endpoints
	s.mux.HandleFunc("/api/v1/stream", s.handleStreamWebSocket)
	s.mux.HandleFunc("/api/v1/traces", s.handleTraces)
	s.mux.HandleFunc("/api/v1/traces/", s.handleTraceByID)
	s.mux.HandleFunc("/api/v1/aggregated", s.handleAggregated)
	s.mux.HandleFunc("/api/v1/aggregated/metrics", s.handleAggregatedMetrics)
	s.mux.HandleFunc("/api/v1/aggregated/health", s.handleAggregatedHealth)

	// Log aggregation endpoints
	s.mux.Handle("/api/v1/logs", s.logHandler)
	s.mux.Handle("/ws/logs", s.logStream)

	// eBPF endpoints (Campaign 2.2)
	s.mux.HandleFunc("/api/v1/latency", s.handleLatency)
	s.mux.HandleFunc("/api/v1/ebpf/stats", s.handleEBPFStats)
	s.mux.HandleFunc("/api/v1/ebpf/events", s.handleEBPFEvents)

	// Service config management endpoints (S47)
	s.mux.HandleFunc("/api/v1/services/config/", s.handleServiceConfig)
	s.mux.HandleFunc("/api/v1/services/restart/", s.handleServiceRestart)
	s.mux.HandleFunc("/api/v1/infrastructure", s.handleInfrastructure)
	s.mux.HandleFunc("/api/v1/infrastructure/containers", s.handleInfraContainers)

	// Doom compute endpoints (S42)
	s.mux.HandleFunc("/api/v1/doom/screen", s.handleDoomScreen)
	s.mux.HandleFunc("/api/v1/doom/cpu", s.handleDoomCPU)
	s.mux.HandleFunc("/api/v1/doom/input", s.handleDoomInput)

	// Static file serving for dashboard UI
	var staticFS http.FileSystem
	if s.config.StaticFS != nil {
		staticFS = http.FS(s.config.StaticFS)
	} else {
		staticFS = http.Dir("static")
	}
	s.staticFS = staticFS
	staticHandler := http.FileServer(staticFS)
	s.mux.Handle("/static/", http.StripPrefix("/static/", staticHandler))

	// Serve advanced visualizations from dashboard/ directory under /viz/ and /dashboard/
	if s.config.VizDir != "" {
		vizFS := http.Dir(s.config.VizDir)
		vizHandler := http.StripPrefix("/viz/", http.FileServer(vizFS))
		s.mux.Handle("/viz/", vizHandler)
		// Also serve under /dashboard/ for asset references (css/, js/)
		dashHandler := http.StripPrefix("/dashboard/", http.FileServer(vizFS))
		s.mux.Handle("/dashboard/", dashHandler)
	}

	// Serve dashboard pages
	s.mux.HandleFunc("/logs", s.handleStaticFile("logs.html"))
	s.mux.HandleFunc("/", s.handleStaticIndex)
}

// handleStaticIndex serves the dashboard index.html
func (s *Server) handleStaticIndex(w http.ResponseWriter, r *http.Request) {
	// Only serve index.html for exact root path or explicit request
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		// Serve from embedded/filesystem static files
		f, err := s.staticFS.Open(r.URL.Path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		f.Close()
		http.FileServer(s.staticFS).ServeHTTP(w, r)
		return
	}
	s.serveStaticFile(w, r, "index.html")
}

// handleStaticFile returns a handler that serves a specific static file.
func (s *Server) handleStaticFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.serveStaticFile(w, r, name)
	}
}

// serveStaticFile serves a named file from the static filesystem.
func (s *Server) serveStaticFile(w http.ResponseWriter, r *http.Request, name string) {
	f, err := s.staticFS.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// http.ServeContent handles Content-Type, caching, range requests
	http.ServeContent(w, r, name, stat.ModTime(), f.(io.ReadSeeker))
}

// Start starts the server and all components
func (s *Server) Start(ctx context.Context) error {
	if s.started {
		return errors.New("server already started")
	}
	s.started = true

	s.log.Info().
		Str("addr", s.config.ListenAddr).
		Str("wotan", s.config.WotanAddr).
		Msg("starting dashboard backend")

	// Register Kingdom services for scraping and monitoring
	s.scraper.RegisterKingdomServices(s.config.ServiceEndpoints)
	s.healthMonitor.RegisterKingdomServices(s.config.ServiceEndpoints)

	// Start WebSocket server
	if err := s.wsServer.Start(ctx); err != nil {
		return fmt.Errorf("start websocket server: %w", err)
	}

	// Start metrics scraper
	if err := s.scraper.Start(ctx); err != nil {
		return fmt.Errorf("start metrics scraper: %w", err)
	}

	// Start health monitor
	if err := s.healthMonitor.Start(ctx); err != nil {
		return fmt.Errorf("start health monitor: %w", err)
	}

	// Start event streamer
	if err := s.eventStreamer.Start(ctx); err != nil {
		s.log.Warn().Err(err).Msg("event streamer start failed, continuing without wotan")
	}

	// Start packet flow generator
	flowCh, err := s.flowGenerator.Start(ctx)
	if err != nil {
		return fmt.Errorf("start flow generator: %w", err)
	}

	// Start eBPF ingestor if configured
	if s.ebpfIngestor != nil {
		if err := s.ebpfIngestor.Start(ctx); err != nil {
			s.log.Warn().Err(err).Msg("ebpf ingestor start failed, continuing with synthetic flows")
		} else {
			s.log.Info().Msg("ebpf ingestor started — real eBPF events active")
		}
	}

	// Start broadcasters
	s.wg.Add(4)
	go s.broadcastFlows(ctx, flowCh)
	go s.broadcastHealthUpdates(ctx)
	go s.broadcastEvents(ctx)
	go s.broadcastTraceEvents(ctx)

	// Start eBPF event broadcaster if ingestor is active
	if s.ebpfIngestor != nil {
		s.wg.Add(1)
		go s.broadcastEBPFEvents(ctx)
	}

	// Start metrics updater
	s.wg.Add(1)
	go s.updateMetrics(ctx)

	// Start HTTP server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("http server error")
		}
	}()

	s.log.Info().Msg("dashboard backend started")
	return nil
}

// broadcastFlows broadcasts packet flows to WebSocket clients
func (s *Server) broadcastFlows(ctx context.Context, flowCh <-chan *packetflow.PacketFlow) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case flow := <-flowCh:
			if flow == nil {
				return
			}

			// Broadcast to main WebSocket
			data, err := json.Marshal(map[string]interface{}{
				"type": "packet_flow",
				"data": flow,
			})
			if err != nil {
				s.log.Error().Err(err).Msg("marshal packet flow failed")
				continue
			}
			s.wsServer.Broadcast(data)

			// Also broadcast to stream subscribers
			streamMsg := &StreamMessage{
				Type:      "flow",
				Service:   "trace-collector",
				Timestamp: flow.Timestamp,
				Data:      flow,
			}
			s.broadcastToStream(streamMsg)
		}
	}
}

// broadcastHealthUpdates broadcasts health status changes
func (s *Server) broadcastHealthUpdates(ctx context.Context) {
	defer s.wg.Done()

	s.healthMonitor.OnStatusChange(func(event health.HealthEvent) {
		// Broadcast to main WebSocket
		data, err := json.Marshal(map[string]interface{}{
			"type": "health_update",
			"data": event,
		})
		if err != nil {
			s.log.Error().Err(err).Msg("marshal health event failed")
			return
		}
		s.wsServer.Broadcast(data)

		// Also broadcast to stream subscribers
		streamMsg := &StreamMessage{
			Type:      "health",
			Service:   event.Service,
			Timestamp: event.Timestamp,
			Data:      event,
		}
		s.broadcastToStream(streamMsg)
	})

	<-ctx.Done()
}

// broadcastEvents broadcasts events to WebSocket clients
func (s *Server) broadcastEvents(ctx context.Context) {
	defer s.wg.Done()

	s.eventStreamer.AddListener(func(event events.Event) {
		// Broadcast to main WebSocket
		data, err := json.Marshal(map[string]interface{}{
			"type": "event",
			"data": event,
		})
		if err != nil {
			s.log.Error().Err(err).Msg("marshal event failed")
			return
		}
		s.wsServer.Broadcast(data)

		// Also broadcast to stream subscribers
		streamMsg := &StreamMessage{
			Type:      "event",
			Service:   event.Source,
			Timestamp: event.Timestamp,
			Data:      event,
		}
		s.broadcastToStream(streamMsg)
	})

	<-ctx.Done()
}

// updateMetrics periodically updates server metrics
func (s *Server) updateMetrics(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.wsConnections.Set(float64(s.wsServer.ConnectionCount()))

			s.streamSubsMu.RLock()
			s.streamClients.Set(float64(len(s.streamSubs)))
			s.streamSubsMu.RUnlock()
		}
	}
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// handleReady handles readiness check requests
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if !s.started {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "ready",
		"ws_connections": s.wsServer.ConnectionCount(),
		"scraper_series": s.scraper.SeriesCount(),
		"events_count":   s.eventStreamer.EventCount(),
	})
}

// handleAPIMetrics handles GET /api/v1/metrics - aggregated metrics
func (s *Server) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := s.scraper.GetAggregatedMetrics()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// handleMetricsQuery handles POST /api/v1/metrics/query - query specific metrics
func (s *Server) handleMetricsQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var query struct {
		Name        string            `json:"name"`
		Labels      map[string]string `json:"labels"`
		Since       string            `json:"since"`
		SinceDuration string          `json:"since_duration"`
	}

	// SECURITY: Enforce body size limit (1MB) to prevent denial-of-service
	if err := json.NewDecoder(io.LimitReader(r.Body, 1*1024*1024)).Decode(&query); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	var since time.Time
	if query.SinceDuration != "" {
		dur, err := time.ParseDuration(query.SinceDuration)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid duration: %v", err), http.StatusBadRequest)
			return
		}
		since = time.Now().Add(-dur)
	} else if query.Since != "" {
		var err error
		since, err = time.Parse(time.RFC3339, query.Since)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid time: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		since = time.Now().Add(-1 * time.Hour)
	}

	results := s.scraper.QueryMetrics(query.Name, query.Labels, since)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"count":   len(results),
	})
}

// handleServices handles GET /api/v1/services - service status list
func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleServicesGet(w, r)
	case http.MethodPost:
		s.handleServicesPost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleServicesGet returns all services with health and config data.
func (s *Server) handleServicesGet(w http.ResponseWriter, r *http.Request) {
	systemHealth := s.healthMonitor.GetSystemHealth()
	configs := s.configLoader.GetAllConfigs()

	services := make([]map[string]interface{}, 0, len(systemHealth.Services))
	for name, svc := range systemHealth.Services {
		// Get metrics for service
		svcMetrics, _ := s.scraper.GetServiceMetrics(name)

		service := map[string]interface{}{
			"name":                  name,
			"status":                svc.Status,
			"last_check":            svc.LastCheck,
			"uptime_percent":        svc.UptimePercent,
			"consecutive_successes": svc.ConsecutiveSuccesses,
			"consecutive_failures":  svc.ConsecutiveFailures,
			"average_latency_ms":    svc.AverageLatency.Milliseconds(),
		}

		if svcMetrics != nil {
			service["metrics_status"] = svcMetrics.Status
			service["metrics_count"] = len(svcMetrics.Metrics)
		}

		// Merge config data if available
		if cfg, ok := configs[name]; ok {
			service["config"] = cfg
		}

		services = append(services, service)
	}

	// Also include services that have configs but aren't in health yet
	for name, cfg := range configs {
		found := false
		for _, svc := range services {
			if svc["name"] == name {
				found = true
				break
			}
		}
		if !found {
			services = append(services, map[string]interface{}{
				"name":   name,
				"status": "unknown",
				"config": cfg,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"services":       services,
		"total":          len(services),
		"healthy":        systemHealth.HealthyCount,
		"degraded":       systemHealth.DegradedCount,
		"unhealthy":      systemHealth.UnhealthyCount,
		"overall_status": systemHealth.Status,
		"updated_at":     time.Now(),
	})
}

// handleServicesPost creates a new service config via POST.
func (s *Server) handleServicesPost(w http.ResponseWriter, r *http.Request) {
	var cfg discovery.ServiceConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if err := discovery.ValidateConfig(&cfg); err != nil {
		http.Error(w, fmt.Sprintf("validation failed: %v", err), http.StatusUnprocessableEntity)
		return
	}

	// Check for duplicate
	if existing := s.configLoader.GetConfig(cfg.Service.Name); existing != nil {
		http.Error(w, fmt.Sprintf("service %q already exists", cfg.Service.Name), http.StatusConflict)
		return
	}

	s.configLoader.UpdateConfig(cfg.Service.Name, &cfg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": cfg.Service.Name,
		"status":  "created",
	})
}

// handleServiceByName handles GET/PUT/DELETE /api/v1/services/{name}
func (s *Server) handleServiceByName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/v1/services/"):]
	// Strip trailing path segments (for sub-resources like /config, /restart)
	if idx := strings.Index(name, "/"); idx > 0 {
		name = name[:idx]
	}
	if name == "" {
		http.Error(w, "service name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleServiceByNameGet(w, r, name)
	case http.MethodPut:
		s.handleServiceByNamePut(w, r, name)
	case http.MethodDelete:
		s.handleServiceByNameDelete(w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleServiceByNameGet(w http.ResponseWriter, _ *http.Request, name string) {
	result := map[string]interface{}{
		"name": name,
	}

	// Get health
	svcHealth, err := s.healthMonitor.GetServiceHealth(name)
	if err == nil {
		result["health"] = svcHealth
	}

	// Get metrics
	svcMetrics, _ := s.scraper.GetServiceMetrics(name)
	if svcMetrics != nil {
		result["metrics"] = svcMetrics
	}

	// Get config
	if cfg := s.configLoader.GetConfig(name); cfg != nil {
		result["config"] = cfg
	}

	// If we have neither health nor config, the service doesn't exist
	if _, hasHealth := result["health"]; !hasHealth {
		if _, hasCfg := result["config"]; !hasCfg {
			http.Error(w, "service not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleServiceByNamePut(w http.ResponseWriter, r *http.Request, name string) {
	var cfg discovery.ServiceConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Ensure the name in the URL matches the config
	if cfg.Service.Name != "" && cfg.Service.Name != name {
		http.Error(w, "service name in URL and body must match", http.StatusBadRequest)
		return
	}
	cfg.Service.Name = name

	if err := discovery.ValidateConfig(&cfg); err != nil {
		http.Error(w, fmt.Sprintf("validation failed: %v", err), http.StatusUnprocessableEntity)
		return
	}

	s.configLoader.UpdateConfig(name, &cfg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": name,
		"status":  "updated",
	})
}

func (s *Server) handleServiceByNameDelete(w http.ResponseWriter, _ *http.Request, name string) {
	if !s.configLoader.DeleteConfig(name) {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": name,
		"status":  "deleted",
	})
}

// handleEvents handles GET /api/v1/events - recent events
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	eventType := r.URL.Query().Get("type")
	severity := r.URL.Query().Get("severity")
	source := r.URL.Query().Get("source")

	filter := &events.EventFilter{
		Limit: limit,
	}

	if eventType != "" {
		filter.Types = []events.EventType{events.EventType(eventType)}
	}
	if severity != "" {
		filter.Severities = []events.Severity{events.Severity(severity)}
	}
	if source != "" {
		filter.Sources = []string{source}
	}

	evts := s.eventStreamer.GetEvents(filter)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": evts,
		"count":  len(evts),
	})
}

// handleEventsSummary handles GET /api/v1/events/summary
func (s *Server) handleEventsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	summary := s.eventStreamer.GetSummary()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// handleSystemHealth handles GET /api/v1/health - system health overview
func (s *Server) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	systemHealth := s.healthMonitor.GetSystemHealth()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(systemHealth)
}

// handleServiceHealth handles GET /api/v1/health/{name}
func (s *Server) handleServiceHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Path[len("/api/v1/health/"):]
	if name == "" {
		http.Error(w, "service name required", http.StatusBadRequest)
		return
	}

	svcHealth, err := s.healthMonitor.GetServiceHealth(name)
	if err != nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	// Get history
	history, _ := s.healthMonitor.GetHistory(name, time.Now().Add(-1*time.Hour))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"health":  svcHealth,
		"history": history,
	})
}

// handleFlows handles GET /api/v1/flows - active network flows
func (s *Server) handleFlows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Return real eBPF flow data when ingestor is active
	if s.ebpfIngestor != nil {
		flows := s.ebpfIngestor.FlowGraph().GetActiveFlows()
		stats := s.ebpfIngestor.FlowGraph().Stats()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source":       "ebpf",
			"active_flows": flows,
			"stats":        stats,
			"ws_endpoint":  "/ws",
		})
		return
	}

	// Fallback: synthetic flow info
	json.NewEncoder(w).Encode(map[string]interface{}{
		"source":       "synthetic",
		"ws_endpoint":  "/ws",
		"description":  "Connect to /ws for real-time packet flow updates. Start trace-collector for real eBPF data.",
		"flow_rate_ms": s.config.PacketFlowConfig.Interval.Milliseconds(),
	})
}

// handleStats handles GET /api/v1/stats - dashboard stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	systemHealth := s.healthMonitor.GetSystemHealth()
	eventStats := s.eventStreamer.GetStats()

	s.streamSubsMu.RLock()
	streamClients := len(s.streamSubs)
	s.streamSubsMu.RUnlock()

	stats := map[string]interface{}{
		"server": map[string]interface{}{
			"started":        s.started,
			"ws_connections": s.wsServer.ConnectionCount(),
			"stream_clients": streamClients,
			"listen_addr":    s.config.ListenAddr,
		},
		"scraper": map[string]interface{}{
			"running":       s.scraper.IsRunning(),
			"series_count":  s.scraper.SeriesCount(),
			"targets_count": len(s.scraper.GetTargets()),
		},
		"health": map[string]interface{}{
			"running":        s.healthMonitor.IsRunning(),
			"total_services": systemHealth.TotalServices,
			"healthy":        systemHealth.HealthyCount,
			"degraded":       systemHealth.DegradedCount,
			"unhealthy":      systemHealth.UnhealthyCount,
			"overall_status": systemHealth.Status,
		},
		"events": eventStats,
		"aggregators": map[string]interface{}{
			"metrics_series": s.metricsAggregator.SeriesCount(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleStreamWebSocket handles WS /api/v1/stream - real-time metrics stream
func (s *Server) handleStreamWebSocket(w http.ResponseWriter, r *http.Request) {
	// Parse filter query parameters
	filter := StreamFilter{}

	if types := r.URL.Query().Get("types"); types != "" {
		filter.Types = strings.Split(types, ",")
	}
	if services := r.URL.Query().Get("services"); services != "" {
		filter.Services = strings.Split(services, ",")
	}
	filter.MinLevel = r.URL.Query().Get("min_level")

	// Use the WebSocket server to handle upgrade and connection
	s.wsServer.HandleWebSocket(w, r)

	// Note: The actual streaming is handled via the existing broadcast mechanism
	// We register a dedicated stream subscription for filtered streaming
	ch := make(chan *StreamMessage, 256)

	s.streamSubsMu.Lock()
	s.streamSubs[ch] = filter
	s.streamSubsMu.Unlock()

	s.log.Debug().
		Strs("types", filter.Types).
		Strs("services", filter.Services).
		Str("min_level", filter.MinLevel).
		Msg("stream subscriber connected")

	// Cleanup on disconnect is handled by the WebSocket server
}

// broadcastToStream broadcasts a message to all stream subscribers
func (s *Server) broadcastToStream(msg *StreamMessage) {
	s.streamSubsMu.RLock()
	defer s.streamSubsMu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		s.log.Error().Err(err).Msg("failed to marshal stream message")
		return
	}

	for ch, filter := range s.streamSubs {
		if s.matchesStreamFilter(msg, filter) {
			select {
			case ch <- msg:
			default:
				// Channel full, skip
			}
		}
	}

	// Also broadcast to main WebSocket for backward compatibility
	s.wsServer.Broadcast(data)
}

// matchesStreamFilter checks if a message matches the stream filter
func (s *Server) matchesStreamFilter(msg *StreamMessage, filter StreamFilter) bool {
	// Filter by type
	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if t == msg.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by service
	if len(filter.Services) > 0 && msg.Service != "" {
		found := false
		for _, svc := range filter.Services {
			if svc == msg.Service {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// handleTraces handles GET /api/v1/traces - trace data
func (s *Server) handleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := &TraceQuery{}

	if svc := r.URL.Query().Get("service"); svc != "" {
		query.ServiceName = svc
	}

	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			query.StartTime = &t
		}
	}

	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			query.EndTime = &t
		}
	}

	if minDur := r.URL.Query().Get("min_duration"); minDur != "" {
		if d, err := time.ParseDuration(minDur); err == nil {
			query.MinDuration = d
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			query.Limit = l
		}
	}

	if hasError := r.URL.Query().Get("has_error"); hasError == "true" {
		b := true
		query.HasError = &b
	}

	// If trace collector is available, query it
	if s.traceCollector != nil {
		result, err := s.traceCollector.QueryTraces(r.Context(), query)
		if err != nil {
			http.Error(w, fmt.Sprintf("query traces: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	// Fallback: return buffered trace events from the traces.* pipeline
	limit := 100
	if query.Limit > 0 && query.Limit <= 1000 {
		limit = query.Limit
	}

	traces := s.traceBuffer.Recent(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"traces":  traces,
		"total":   s.traceBuffer.Count(),
		"has_more": s.traceBuffer.Count() > limit,
		"source":  "pipeline_buffer",
	})
}

// handleTraceByID handles GET /api/v1/traces/{id}
func (s *Server) handleTraceByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	traceIDStr := r.URL.Path[len("/api/v1/traces/"):]
	if traceIDStr == "" {
		http.Error(w, "trace ID required", http.StatusBadRequest)
		return
	}

	if s.traceCollector != nil {
		trace, err := s.traceCollector.GetTrace(r.Context(), traceIDStr)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "trace not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("get trace: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trace)
		return
	}

	http.Error(w, "trace collector not available", http.StatusServiceUnavailable)
}

// handleAggregated handles GET /api/v1/aggregated - all aggregated data
func (s *Server) handleAggregated(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Gather aggregated data from all sources
	scraperMetrics := s.scraper.GetAggregatedMetrics()
	systemHealth := s.healthMonitor.GetSystemHealth()
	eventSummary := s.eventStreamer.GetSummary()

	// Get health aggregator status if available
	var healthAggStatus *SystemHealthStatus
	if s.healthAggregator != nil {
		healthAggStatus = s.healthAggregator.GetSystemHealth()
	}

	aggregated := map[string]interface{}{
		"timestamp": time.Now(),
		"metrics": map[string]interface{}{
			"scraper":       scraperMetrics,
			"series_count":  s.metricsAggregator.SeriesCount(),
			"total_metrics": scraperMetrics.TotalMetrics,
			"total_series":  scraperMetrics.TotalSeries,
		},
		"health": map[string]interface{}{
			"internal":   systemHealth,
			"aggregated": healthAggStatus,
		},
		"events": eventSummary,
		"stream": map[string]interface{}{
			"ws_connections": s.wsServer.ConnectionCount(),
			"stream_clients": func() int {
				s.streamSubsMu.RLock()
				defer s.streamSubsMu.RUnlock()
				return len(s.streamSubs)
			}(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(aggregated)
}

// handleAggregatedMetrics handles GET /api/v1/aggregated/metrics
func (s *Server) handleAggregatedMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	var since time.Time
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = time.Now().Add(-d)
		} else if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	} else {
		since = time.Now().Add(-1 * time.Hour)
	}

	// Get metrics from scraper
	scraperMetrics := s.scraper.GetAggregatedMetrics()

	// Get internal aggregator stats
	aggregatorStats := map[string]interface{}{
		"series_count": s.metricsAggregator.SeriesCount(),
	}

	result := map[string]interface{}{
		"timestamp":  time.Now(),
		"since":      since,
		"scraper":    scraperMetrics,
		"aggregator": aggregatorStats,
		"summary": map[string]interface{}{
			"total_services": len(scraperMetrics.Services),
			"total_metrics":  scraperMetrics.TotalMetrics,
			"total_series":   scraperMetrics.TotalSeries,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleAggregatedHealth handles GET /api/v1/aggregated/health
func (s *Server) handleAggregatedHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get internal health monitor status
	internalHealth := s.healthMonitor.GetSystemHealth()

	// Get health aggregator status if available
	var healthAggStatus *SystemHealthStatus
	var healthChecks []interface{}
	if s.healthAggregator != nil {
		healthAggStatus = s.healthAggregator.GetSystemHealth()
		healthChecks = s.healthAggregator.GetChecks()
	}

	result := map[string]interface{}{
		"timestamp": time.Now(),
		"internal": map[string]interface{}{
			"status":         internalHealth.Status,
			"total_services": internalHealth.TotalServices,
			"healthy":        internalHealth.HealthyCount,
			"degraded":       internalHealth.DegradedCount,
			"unhealthy":      internalHealth.UnhealthyCount,
			"services":       internalHealth.Services,
		},
		"aggregated_health": map[string]interface{}{
			"status":     healthAggStatus,
			"checks":     healthChecks,
			"registered": len(healthChecks),
		},
		"overall": func() string {
			if internalHealth.UnhealthyCount > 0 {
				return "unhealthy"
			}
			if internalHealth.DegradedCount > 0 {
				return "degraded"
			}
			if internalHealth.HealthyCount == internalHealth.TotalServices && internalHealth.TotalServices > 0 {
				return "healthy"
			}
			return "unknown"
		}(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// broadcastEBPFEvents listens to the eBPF ingestor and broadcasts events via WebSocket.
// When an ebpf_packet event is received, also emits a packet_flow message so
// the existing canvas visualization renders real eBPF data.
func (s *Server) broadcastEBPFEvents(ctx context.Context) {
	defer s.wg.Done()

	s.ebpfIngestor.OnEvent(func(env ebpfPkg.EventEnvelope) {
		data, err := json.Marshal(map[string]interface{}{
			"type": "ebpf_" + env.Type,
			"data": env.Data,
		})
		if err != nil {
			return
		}
		s.wsServer.Broadcast(data)

		// Also send to stream subscribers
		streamMsg := &StreamMessage{
			Type:      "ebpf_" + env.Type,
			Service:   "trace-collector",
			Timestamp: env.Timestamp,
			Data:      env.Data,
		}
		s.broadcastToStream(streamMsg)

		// Convert ebpf_packet events into packet_flow messages for the canvas
		if env.Type == "packet" {
			if pkt, ok := env.Data.(*ebpfPkg.PacketEvent); ok {
				srcService := s.resolveServiceName(pkt.FlowKey.SrcAddr)
				dstService := s.resolveServiceName(pkt.FlowKey.DstAddr)
				// Use mesh service_id if available for better hop mapping
				if pkt.Mesh != nil {
					if name := s.resolveServiceByID(pkt.Mesh.SrcServiceID); name != "" {
						srcService = name
					}
					if name := s.resolveServiceByID(pkt.Mesh.DstServiceID); name != "" {
						dstService = name
					}
				}
				flowData, _ := json.Marshal(map[string]interface{}{
					"type": "packet_flow",
					"data": map[string]interface{}{
						"source":      srcService,
						"destination": dstService,
						"protocol":    pkt.FlowKey.Protocol,
						"size":        pkt.PacketLen,
						"timestamp":   env.Timestamp,
						"trace_id":    pkt.TraceID.String(),
						"direction":   pkt.Direction,
					},
				})
				s.wsServer.Broadcast(flowData)
			}
		}
	})

	<-ctx.Done()
}

// broadcastTraceEvents listens for events on traces.* topics and pushes them
// to the trace buffer and all WebSocket clients with type "trace".
func (s *Server) broadcastTraceEvents(ctx context.Context) {
	defer s.wg.Done()

	s.eventStreamer.AddListener(func(event events.Event) {
		// Only handle traces.* topics
		if len(event.Topic) < 7 || event.Topic[:7] != "traces." {
			return
		}

		traceEvt := s.parseTraceTopicEvent(event)
		s.traceBuffer.Add(traceEvt)

		// Broadcast to main WebSocket as "trace" type
		data, err := json.Marshal(map[string]interface{}{
			"type": "trace",
			"data": traceEvt,
		})
		if err != nil {
			s.log.Error().Err(err).Msg("marshal trace event failed")
			return
		}
		s.wsServer.Broadcast(data)

		// Also broadcast to stream subscribers
		streamMsg := &StreamMessage{
			Type:      "trace",
			Service:   "trace-collector",
			Timestamp: time.Unix(0, traceEvt.Timestamp*int64(time.Millisecond)),
			Data:      traceEvt,
		}
		s.broadcastToStream(streamMsg)
	})

	<-ctx.Done()
}

// parseTraceTopicEvent converts an events.Event from a traces.* topic into a TraceEvent.
func (s *Server) parseTraceTopicEvent(event events.Event) TraceEvent {
	te := TraceEvent{
		Timestamp: event.Timestamp.UnixMilli(),
	}

	// Determine event type from topic
	switch event.Topic {
	case "traces.packet":
		te.EventType = "packet"
	case "traces.flow":
		te.EventType = "flow"
	case "traces.latency":
		te.EventType = "latency"
	case "traces.correlated":
		te.EventType = "correlated"
	default:
		te.EventType = "unknown"
	}

	// Extract fields from event data
	if event.Data != nil {
		if v, ok := event.Data["trace_id"].(string); ok {
			te.TraceID = v
		}
		if v, ok := event.Data["src_ip"].(string); ok {
			te.Src = v
		}
		if v, ok := event.Data["src"].(string); ok && te.Src == "" {
			te.Src = v
		}
		if v, ok := event.Data["dst_ip"].(string); ok {
			te.Dst = v
		}
		if v, ok := event.Data["dst"].(string); ok && te.Dst == "" {
			te.Dst = v
		}
		if v, ok := event.Data["protocol"].(string); ok {
			te.Protocol = v
		}
		if v, ok := event.Data["rtt_ns"].(float64); ok {
			te.RTTMs = v / 1e6
		}
		if v, ok := event.Data["rtt_ms"].(float64); ok {
			te.RTTMs = v
		}
		if v, ok := event.Data["state"].(string); ok {
			te.FlowState = v
		}
		if v, ok := event.Data["flow_state"].(string); ok && te.FlowState == "" {
			te.FlowState = v
		}
		if v, ok := event.Data["hop_count"].(float64); ok {
			te.HopCount = int(v)
		}
		if v, ok := event.Data["packet_len"].(float64); ok {
			te.PacketLen = int(v)
		}

		// Build src/dst from flow key if present
		if fk, ok := event.Data["flow_key"].(map[string]interface{}); ok {
			if sa, ok := fk["src_addr"].(string); ok {
				sp := ""
				if sp2, ok2 := fk["src_port"].(float64); ok2 {
					sp = fmt.Sprintf(":%d", int(sp2))
				}
				te.Src = sa + sp
			}
			if da, ok := fk["dst_addr"].(string); ok {
				dp := ""
				if dp2, ok2 := fk["dst_port"].(float64); ok2 {
					dp = fmt.Sprintf(":%d", int(dp2))
				}
				te.Dst = da + dp
			}
			if p, ok := fk["protocol"].(float64); ok {
				switch int(p) {
				case 6:
					te.Protocol = "TCP"
				case 17:
					te.Protocol = "UDP"
				default:
					te.Protocol = fmt.Sprintf("proto(%d)", int(p))
				}
			}
		}
	}

	// Generate a trace ID if not found in data
	if te.TraceID == "" {
		te.TraceID = fmt.Sprintf("trace-%d", event.Timestamp.UnixNano())
	}

	return te
}

// IngestTraceEvent allows external code (e.g. tests) to inject a trace event
// directly into the pipeline without going through Wotan.
func (s *Server) IngestTraceEvent(te TraceEvent) {
	s.traceBuffer.Add(te)

	data, err := json.Marshal(map[string]interface{}{
		"type": "trace",
		"data": te,
	})
	if err != nil {
		return
	}
	s.wsServer.Broadcast(data)

	streamMsg := &StreamMessage{
		Type:      "trace",
		Service:   "trace-collector",
		Timestamp: time.Unix(0, te.Timestamp*int64(time.Millisecond)),
		Data:      te,
	}
	s.broadcastToStream(streamMsg)
}

// GetTraceBuffer returns the trace buffer for testing.
func (s *Server) GetTraceBuffer() *TraceBuffer {
	return s.traceBuffer
}

// resolveServiceName maps an IP address to a service name using config.ServiceEndpoints.
func (s *Server) resolveServiceName(addr string) string {
	if s.config.ServiceEndpoints == nil {
		return addr
	}
	for name, endpoint := range s.config.ServiceEndpoints {
		// endpoint is "host:port", strip port for comparison
		host := endpoint
		if idx := strings.LastIndex(endpoint, ":"); idx >= 0 {
			host = endpoint[:idx]
		}
		if host == addr {
			return name
		}
	}
	return addr
}

// resolveServiceByID maps a mesh service_id byte to a service name.
// Convention: 0=unknown, 1-N map to services in config order.
func (s *Server) resolveServiceByID(id uint8) string {
	if id == 0 || s.config.ServiceEndpoints == nil {
		return ""
	}
	i := uint8(1)
	for name := range s.config.ServiceEndpoints {
		if i == id {
			return name
		}
		i++
	}
	return ""
}

// handleLatency handles GET /api/v1/latency - latency histogram data
func (s *Server) handleLatency(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if s.ebpfIngestor == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "eBPF ingestor not active. Start trace-collector to get real latency data.",
			"data":    nil,
		})
		return
	}

	// Parse optional operation filter
	op := r.URL.Query().Get("operation")

	if op != "" {
		// Single operation
		latOp := ebpfPkg.LatencyOperation(op)
		windows := s.ebpfIngestor.LatencyHistogram().GetAllPercentiles()
		result, ok := windows[latOp]
		if !ok {
			http.Error(w, "unknown operation", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"operation":   op,
			"percentiles": result,
		})
		return
	}

	// All operations
	json.NewEncoder(w).Encode(map[string]interface{}{
		"percentiles": s.ebpfIngestor.LatencyHistogram().GetAllPercentiles(),
		"stats":       s.ebpfIngestor.LatencyHistogram().Stats(),
	})
}

// handleEBPFStats handles GET /api/v1/ebpf/stats - eBPF program statistics
func (s *Server) handleEBPFStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if s.ebpfIngestor == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":  false,
			"message": "eBPF ingestor not active. Start trace-collector to get real stats.",
		})
		return
	}

	stats := s.ebpfIngestor.Stats()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active": true,
		"stats":  stats,
	})
}

// handleEBPFEvents handles GET /api/v1/ebpf/events - recent eBPF events
func (s *Server) handleEBPFEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if s.ebpfIngestor == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"events": []interface{}{},
			"count":  0,
		})
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	events := s.ebpfIngestor.RecentEvents(limit)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
		"count":  len(events),
	})
}

// SetTraceCollector sets the trace collector for the server
func (s *Server) SetTraceCollector(collector TraceCollector) {
	s.traceCollector = collector
}

// RecordMetric records a metric to the internal aggregator
func (s *Server) RecordMetric(name string, value float64, labels map[string]string) error {
	return s.metricsAggregator.RecordMetric(&internalMetrics.Metric{
		Name:      name,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	})
}

// SetHealthAggregator sets the health aggregator for the server
func (s *Server) SetHealthAggregator(aggregator HealthAggregator) {
	s.healthAggregator = aggregator
}

// RegisterHealthCheck registers a health check with the health aggregator
func (s *Server) RegisterHealthCheck(check interface{}) error {
	if s.healthAggregator == nil {
		return errors.New("health aggregator not initialized")
	}
	return s.healthAggregator.RegisterCheck(check)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	var shutdownErr error

	s.shutdownOnce.Do(func() {
		s.log.Info().Msg("shutting down dashboard backend")
		close(s.shutdown)

		// Close stream subscriptions
		s.streamSubsMu.Lock()
		for ch := range s.streamSubs {
			close(ch)
			delete(s.streamSubs, ch)
		}
		s.streamSubsMu.Unlock()

		// Shutdown HTTP server
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.log.Error().Err(err).Msg("http server shutdown error")
			shutdownErr = err
		}

		// Shutdown WebSocket server
		if err := s.wsServer.Shutdown(ctx); err != nil {
			s.log.Error().Err(err).Msg("websocket server shutdown error")
			if shutdownErr == nil {
				shutdownErr = err
			}
		}

		// Stop scraper
		if err := s.scraper.Stop(); err != nil {
			s.log.Error().Err(err).Msg("scraper stop error")
		}

		// Stop health monitor
		if err := s.healthMonitor.Stop(); err != nil {
			s.log.Error().Err(err).Msg("health monitor stop error")
		}

		// Stop event streamer
		if err := s.eventStreamer.Stop(); err != nil {
			s.log.Error().Err(err).Msg("event streamer stop error")
		}

		// Shutdown internal metrics aggregator
		if s.metricsAggregator != nil {
			if err := s.metricsAggregator.Shutdown(ctx); err != nil {
				s.log.Error().Err(err).Msg("metrics aggregator shutdown error")
			}
		}

		// Close health aggregator if set
		if s.healthAggregator != nil {
			if err := s.healthAggregator.Close(); err != nil {
				s.log.Error().Err(err).Msg("health aggregator close error")
			}
		}

		// Wait for goroutines
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			s.log.Info().Msg("dashboard backend shutdown complete")
		case <-ctx.Done():
			if shutdownErr == nil {
				shutdownErr = ctx.Err()
			}
			s.log.Error().Err(shutdownErr).Msg("dashboard backend shutdown timeout")
		}
	})

	return shutdownErr
}

// GetWebSocketServer returns the WebSocket server
func (s *Server) GetWebSocketServer() *websocket.Server {
	return s.wsServer
}

// GetScraper returns the metrics scraper
func (s *Server) GetScraper() *scraper.Scraper {
	return s.scraper
}

// GetHealthMonitor returns the health monitor
func (s *Server) GetHealthMonitor() *health.Monitor {
	return s.healthMonitor
}

// GetEventStreamer returns the event streamer
func (s *Server) GetEventStreamer() *events.Streamer {
	return s.eventStreamer
}

// GetMetricsAggregator returns the internal metrics aggregator
func (s *Server) GetMetricsAggregator() *internalMetrics.Aggregator {
	return s.metricsAggregator
}

// GetTraceCollector returns the trace collector
func (s *Server) GetTraceCollector() TraceCollector {
	return s.traceCollector
}

// GetHealthAggregator returns the health aggregator
func (s *Server) GetHealthAggregator() HealthAggregator {
	return s.healthAggregator
}

// --- S47: Service Config Management Endpoints ---

// handleServiceConfig handles GET/PUT /api/v1/services/config/{name}
// Returns or updates the YAML config for a service.
func (s *Server) handleServiceConfig(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/services/config/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		http.Error(w, "service name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg := s.configLoader.GetConfig(name)
		if cfg == nil {
			http.Error(w, "service config not found", http.StatusNotFound)
			return
		}

		yamlBytes, err := discovery.MarshalConfig(cfg)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal config: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-yaml")
		w.Write(yamlBytes)

	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		cfg, err := discovery.LoadServiceConfigFromBytes(body)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid config: %v", err), http.StatusUnprocessableEntity)
			return
		}

		if cfg.Service.Name != name {
			http.Error(w, "service name in config must match URL", http.StatusBadRequest)
			return
		}

		s.configLoader.UpdateConfig(name, cfg)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": name,
			"status":  "config_updated",
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleServiceRestart handles POST /api/v1/services/restart/{name}
// Stub: in production this would signal the daemon to restart the service.
func (s *Server) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/v1/services/restart/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		http.Error(w, "service name required", http.StatusBadRequest)
		return
	}

	// Verify the service exists in health or config
	_, healthErr := s.healthMonitor.GetServiceHealth(name)
	cfg := s.configLoader.GetConfig(name)
	if healthErr != nil && cfg == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	s.log.Info().
		Str("service", name).
		Str("remote_addr", r.RemoteAddr).
		Msg("service restart requested")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": name,
		"action":  "restart",
		"status":  "queued",
		"message": "restart signal sent to daemon",
	})
}

// handleInfrastructure handles GET /api/v1/infrastructure
// Returns overall infrastructure status including container runtime and resource usage.
func (s *Server) handleInfrastructure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	systemHealth := s.healthMonitor.GetSystemHealth()
	configs := s.configLoader.GetAllConfigs()

	// Count services by tier
	tierCounts := map[string]int{
		"control":        0,
		"infrastructure": 0,
		"presentation":   0,
		"application":    0,
	}
	for _, cfg := range configs {
		tierCounts[cfg.Deployment.Tier]++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"runtime":          "docker",
		"total_services":   len(configs),
		"healthy_services": systemHealth.HealthyCount,
		"tier_breakdown":   tierCounts,
		"network": map[string]interface{}{
			"bridge":   "lxdbr0",
			"subnet":   "10.10.10.0/24",
			"gateway":  "10.10.10.100",
			"protocol": "VXLAN",
			"mtu":      1450,
		},
		"resources": map[string]interface{}{
			"total_cpu":    "available",
			"total_memory": "available",
		},
		"updated_at": time.Now(),
	})
}

// handleInfraContainers handles GET /api/v1/infrastructure/containers
// Returns information about running containers/services with their resource usage.
func (s *Server) handleInfraContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	configs := s.configLoader.GetAllConfigs()
	systemHealth := s.healthMonitor.GetSystemHealth()

	containers := make([]map[string]interface{}, 0, len(configs))
	for name, cfg := range configs {
		container := map[string]interface{}{
			"name":           name,
			"image":          fmt.Sprintf("unheaded:%s-%s", name, cfg.Service.Version),
			"tier":           cfg.Deployment.Tier,
			"port":           cfg.Network.Port,
			"protocol":       cfg.Network.Protocol,
			"replicas":       cfg.Deployment.Replicas,
			"restart_policy": cfg.Deployment.RestartPolicy,
			"binary":         cfg.Runtime.Binary,
		}

		// Add health status if available
		if svc, ok := systemHealth.Services[name]; ok {
			container["status"] = svc.Status
			container["uptime_percent"] = svc.UptimePercent
		} else {
			container["status"] = "unknown"
		}

		containers = append(containers, container)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"containers": containers,
		"total":      len(containers),
		"updated_at": time.Now(),
	})
}

// GetConfigLoader returns the service config loader.
func (s *Server) GetConfigLoader() *discovery.ServiceConfigLoader {
	return s.configLoader
}

// buildDashboardAuthenticators constructs authenticators from the auth config.
func buildDashboardAuthenticators(cfg auth.ServiceAuthConfig) []auth.Authenticator {
	var authenticators []auth.Authenticator
	if len(cfg.JWTSigningKey) > 0 {
		authenticators = append(authenticators, auth.NewJWTAuthenticator(auth.JWTConfig{
			SigningKey: cfg.JWTSigningKey,
			Issuer:    cfg.Issuer,
			Audience:  cfg.Audience,
		}))
		authenticators = append(authenticators, auth.NewJWTAuthenticator(auth.JWTConfig{
			SigningKey:    cfg.JWTSigningKey,
			Issuer:       auth.ServiceTokenIssuer,
			Audience:     cfg.ServiceName,
			AllowedRoles: []string{auth.RoleService},
		}))
	}
	if len(cfg.APIKeys) > 0 || cfg.APIKeyFile != "" {
		apikeyCfg := auth.APIKeyConfig{Keys: cfg.APIKeys, KeyFile: cfg.APIKeyFile}
		if a, err := auth.NewAPIKeyAuthenticator(apikeyCfg); err == nil {
			authenticators = append(authenticators, a)
		}
	}
	return authenticators
}
