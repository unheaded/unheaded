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
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	ebpfPkg "unheaded/cmd/dashboard-backend/internal/ebpf"
	"unheaded/cmd/dashboard-backend/internal/events"
	"unheaded/cmd/dashboard-backend/internal/health"
	internalMetrics "unheaded/cmd/dashboard-backend/internal/metrics"
	"unheaded/cmd/dashboard-backend/internal/packetflow"
	"unheaded/cmd/dashboard-backend/internal/scraper"
	"unheaded/cmd/dashboard-backend/internal/websocket"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
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

	// Busboy
	BusboyAddr  string
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
}

// DefaultConfig returns default server configuration
func DefaultConfig() *Config {
	return &Config{
		ListenAddr:   ":8080",
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		BusboyAddr:   "localhost:9090",
		ServiceName:  "dashboard-backend",
	}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 15 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 15 * time.Second
	}
	if c.BusboyAddr == "" {
		return errors.New("busboy address required")
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
		c.EventsConfig.BusboyAddr = c.BusboyAddr
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

	// Stream subscriptions for /api/v1/stream
	streamSubs   map[chan *StreamMessage]StreamFilter
	streamSubsMu sync.RWMutex

	// Metrics
	metricsRegistry *metrics.Registry
	httpRequests    *metrics.CounterVec
	httpDuration    *metrics.HistogramVec
	wsConnections   *metrics.Gauge
	streamClients   *metrics.Gauge

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
		streamSubs:        make(map[chan *StreamMessage]StreamFilter),
		shutdown:          make(chan struct{}),
	}

	// Initialize metrics
	s.initMetrics()

	// Setup HTTP routes
	s.mux = http.NewServeMux()
	s.setupRoutes()

	s.httpServer = &http.Server{
		Addr:           config.ListenAddr,
		Handler:        s.mux,
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		IdleTimeout:    5 * config.ReadTimeout,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	return s, nil
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

	// eBPF endpoints (Campaign 2.2)
	s.mux.HandleFunc("/api/v1/latency", s.handleLatency)
	s.mux.HandleFunc("/api/v1/ebpf/stats", s.handleEBPFStats)
	s.mux.HandleFunc("/api/v1/ebpf/events", s.handleEBPFEvents)

	// Static file serving for dashboard UI
	// Serve static files from ./static directory
	staticHandler := http.FileServer(http.Dir("static"))
	s.mux.Handle("/static/", http.StripPrefix("/static/", staticHandler))

	// Serve index.html for root path
	s.mux.HandleFunc("/", s.handleStaticIndex)
}

// handleStaticIndex serves the dashboard index.html
func (s *Server) handleStaticIndex(w http.ResponseWriter, r *http.Request) {
	// Only serve index.html for exact root path or explicit request
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		// Check if it's a static file request
		http.ServeFile(w, r, "static/"+r.URL.Path)
		return
	}
	http.ServeFile(w, r, "static/index.html")
}

// Start starts the server and all components
func (s *Server) Start(ctx context.Context) error {
	if s.started {
		return errors.New("server already started")
	}
	s.started = true

	s.log.Info().
		Str("addr", s.config.ListenAddr).
		Str("busboy", s.config.BusboyAddr).
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
		s.log.Warn().Err(err).Msg("event streamer start failed, continuing without busboy")
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
	s.wg.Add(3)
	go s.broadcastFlows(ctx, flowCh)
	go s.broadcastHealthUpdates(ctx)
	go s.broadcastEvents(ctx)

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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	systemHealth := s.healthMonitor.GetSystemHealth()

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

		services = append(services, service)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"services":       services,
		"total":          systemHealth.TotalServices,
		"healthy":        systemHealth.HealthyCount,
		"degraded":       systemHealth.DegradedCount,
		"unhealthy":      systemHealth.UnhealthyCount,
		"overall_status": systemHealth.Status,
	})
}

// handleServiceByName handles GET /api/v1/services/{name}
func (s *Server) handleServiceByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Path[len("/api/v1/services/"):]
	if name == "" {
		http.Error(w, "service name required", http.StatusBadRequest)
		return
	}

	// Get health
	svcHealth, err := s.healthMonitor.GetServiceHealth(name)
	if err != nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	// Get metrics
	svcMetrics, _ := s.scraper.GetServiceMetrics(name)

	result := map[string]interface{}{
		"name":   name,
		"health": svcHealth,
	}

	if svcMetrics != nil {
		result["metrics"] = svcMetrics
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
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

	// Fallback: return empty result with info about trace collector status
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"traces":  []interface{}{},
		"total":   0,
		"message": "Trace collector not initialized. Connect to trace-collector service for distributed tracing.",
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
	})

	<-ctx.Done()
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
