// Package server provides the main dashboard backend HTTP server.
// It aggregates metrics, health, and events from all Kingdom services
// and provides REST API endpoints and WebSocket streaming.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"unheaded/cmd/dashboard-backend/internal/events"
	"unheaded/cmd/dashboard-backend/internal/health"
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

	// Metrics
	metricsRegistry *metrics.Registry
	httpRequests    *metrics.CounterVec
	httpDuration    *metrics.HistogramVec
	wsConnections   *metrics.Gauge

	// Lifecycle
	started      bool
	shutdown     chan struct{}
	shutdownOnce sync.Once
	wg           sync.WaitGroup
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

	s := &Server{
		config:        config,
		log:           log,
		wsServer:      wsServer,
		scraper:       metricsScraper,
		healthMonitor: healthMonitor,
		eventStreamer: eventStreamer,
		flowGenerator: flowGenerator,
		shutdown:      make(chan struct{}),
	}

	// Initialize metrics
	s.initMetrics()

	// Setup HTTP routes
	s.mux = http.NewServeMux()
	s.setupRoutes()

	s.httpServer = &http.Server{
		Addr:         config.ListenAddr,
		Handler:      s.mux,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
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
	s.scraper.RegisterKingdomServices()
	s.healthMonitor.RegisterKingdomServices()

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

	// Start broadcasters
	s.wg.Add(3)
	go s.broadcastFlows(ctx, flowCh)
	go s.broadcastHealthUpdates(ctx)
	go s.broadcastEvents(ctx)

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

			data, err := json.Marshal(map[string]interface{}{
				"type": "packet_flow",
				"data": flow,
			})
			if err != nil {
				s.log.Error().Err(err).Msg("marshal packet flow failed")
				continue
			}

			s.wsServer.Broadcast(data)
		}
	}
}

// broadcastHealthUpdates broadcasts health status changes
func (s *Server) broadcastHealthUpdates(ctx context.Context) {
	defer s.wg.Done()

	s.healthMonitor.OnStatusChange(func(event health.HealthEvent) {
		data, err := json.Marshal(map[string]interface{}{
			"type": "health_update",
			"data": event,
		})
		if err != nil {
			s.log.Error().Err(err).Msg("marshal health event failed")
			return
		}

		s.wsServer.Broadcast(data)
	})

	<-ctx.Done()
}

// broadcastEvents broadcasts events to WebSocket clients
func (s *Server) broadcastEvents(ctx context.Context) {
	defer s.wg.Done()

	s.eventStreamer.AddListener(func(event events.Event) {
		data, err := json.Marshal(map[string]interface{}{
			"type": "event",
			"data": event,
		})
		if err != nil {
			s.log.Error().Err(err).Msg("marshal event failed")
			return
		}

		s.wsServer.Broadcast(data)
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

	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
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

// handleFlows handles GET /api/v1/flows - packet flow info
func (s *Server) handleFlows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "active",
		"ws_endpoint":  "/ws",
		"description":  "Connect to /ws for real-time packet flow updates",
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

	stats := map[string]interface{}{
		"server": map[string]interface{}{
			"started":        s.started,
			"ws_connections": s.wsServer.ConnectionCount(),
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
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	var shutdownErr error

	s.shutdownOnce.Do(func() {
		s.log.Info().Msg("shutting down dashboard backend")
		close(s.shutdown)

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
