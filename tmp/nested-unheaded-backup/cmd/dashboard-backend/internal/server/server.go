// Package server provides the main dashboard backend HTTP server.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	busboyClient "github.com/unheaded/unheaded/pkg/busboy-client"

	"github.com/unheaded/unheaded/cmd/dashboard-backend/internal/metrics"
	"github.com/unheaded/unheaded/cmd/dashboard-backend/internal/packetflow"
	"github.com/unheaded/unheaded/cmd/dashboard-backend/internal/websocket"
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
	ListenAddr string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// Busboy
	BusboyAddr   string
	ServiceName  string

	// Components
	WebSocketConfig   *websocket.Config
	MetricsConfig     *metrics.Config
	PacketFlowConfig  *packetflow.Config
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

	// Validate sub-configs
	if err := c.WebSocketConfig.Validate(); err != nil {
		return fmt.Errorf("websocket config: %w", err)
	}
	if err := c.MetricsConfig.Validate(); err != nil {
		return fmt.Errorf("metrics config: %w", err)
	}
	if err := c.PacketFlowConfig.Validate(); err != nil {
		return fmt.Errorf("packet flow config: %w", err)
	}

	return nil
}

// Server is the main dashboard backend server
type Server struct {
	config *Config

	// HTTP server
	httpServer *http.Server
	mux        *http.ServeMux

	// Components
	wsServer       *websocket.Server
	metricsAgg     *metrics.Aggregator
	flowGenerator  *packetflow.Generator
	busboyClient   *busboyClient.Client

	// Lifecycle
	started      bool
	shutdown     chan struct{}
	shutdownOnce sync.Once
	wg           sync.WaitGroup
}

// NewServer creates a new dashboard backend server
func NewServer(config *Config) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create WebSocket server
	wsServer, err := websocket.NewServer(config.WebSocketConfig)
	if err != nil {
		return nil, fmt.Errorf("create websocket server: %w", err)
	}

	// Create metrics aggregator
	metricsAgg, err := metrics.NewAggregator(config.MetricsConfig)
	if err != nil {
		return nil, fmt.Errorf("create metrics aggregator: %w", err)
	}

	// Create packet flow generator
	flowGenerator, err := packetflow.NewGenerator(config.PacketFlowConfig)
	if err != nil {
		return nil, fmt.Errorf("create packet flow generator: %w", err)
	}

	s := &Server{
		config:        config,
		wsServer:      wsServer,
		metricsAgg:    metricsAgg,
		flowGenerator: flowGenerator,
		shutdown:      make(chan struct{}),
	}

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

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() {
	// WebSocket
	s.mux.HandleFunc("/ws", s.wsServer.HandleWebSocket)

	// Health checks
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)

	// Metrics
	s.mux.Handle("/metrics", promhttp.Handler())

	// API
	s.mux.HandleFunc("/api/v1/metrics/query", s.handleMetricsQuery)
	s.mux.HandleFunc("/api/v1/flows", s.handleFlows)
}

// Start starts the server and all components
func (s *Server) Start(ctx context.Context) error {
	if s.started {
		return errors.New("server already started")
	}
	s.started = true

	log.Info().
		Str("addr", s.config.ListenAddr).
		Str("busboy", s.config.BusboyAddr).
		Msg("starting dashboard backend")

	// Connect to Busboy
	client, err := busboyClient.NewClient(s.config.BusboyAddr)
	if err != nil {
		return fmt.Errorf("connect to busboy: %w", err)
	}
	s.busboyClient = client

	// Subscribe to metrics topics
	subscriber, err := s.busboyClient.Subscribe(ctx, "metrics.*", s.config.ServiceName)
	if err != nil {
		return fmt.Errorf("subscribe to metrics: %w", err)
	}
	log.Info().
		Str("subscriber_id", subscriber.SubscriberID).
		Str("status", subscriber.Status).
		Msg("subscribed to busboy metrics")

	// Start packet flow generator
	flowCh, err := s.flowGenerator.Start(ctx)
	if err != nil {
		return fmt.Errorf("start flow generator: %w", err)
	}

	// Start flow broadcaster
	s.wg.Add(1)
	go s.broadcastFlows(ctx, flowCh)

	// Start metrics collector (if Busboy subscription approved)
	if subscriber.Status == "approved" {
		s.wg.Add(1)
		go s.collectMetrics(ctx)
	} else {
		log.Warn().Msg("busboy subscription pending approval, metrics collection disabled")
	}

	// Start HTTP server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("http server error")
		}
	}()

	log.Info().Msg("dashboard backend started")
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

			// Serialize to JSON
			data, err := json.Marshal(map[string]interface{}{
				"type": "packet_flow",
				"data": flow,
			})
			if err != nil {
				log.Error().Err(err).Msg("marshal packet flow failed")
				continue
			}

			// Broadcast to WebSocket clients
			s.wsServer.Broadcast(data)
		}
	}
}

// collectMetrics collects metrics from Busboy and aggregates
func (s *Server) collectMetrics(ctx context.Context) {
	defer s.wg.Done()

	// TODO: Stream messages from Busboy when subscription approved
	// For now, this is a placeholder for future implementation

	msgCh, err := s.busboyClient.StreamMessages(ctx, "metrics.*")
	if err != nil {
		log.Error().Err(err).Msg("stream metrics failed")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case msg := <-msgCh:
			if msg == nil {
				return
			}

			// Parse metric from message
			var metric metrics.Metric
			if err := json.Unmarshal([]byte(msg.Payload), &metric); err != nil {
				log.Warn().Err(err).Str("payload", msg.Payload).Msg("invalid metric format")
				continue
			}

			// Record metric
			if err := s.metricsAgg.RecordMetric(&metric); err != nil {
				log.Error().Err(err).Msg("record metric failed")
			}
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
		"status": "ready",
		"connections": s.wsServer.ConnectionCount(),
		"series": s.metricsAgg.SeriesCount(),
	})
}

// handleMetricsQuery handles metrics query requests
func (s *Server) handleMetricsQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var query metrics.Query
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	results, err := s.metricsAgg.QueryMetrics(&query)
	if err != nil {
		http.Error(w, fmt.Sprintf("query failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"count":   len(results),
	})
}

// handleFlows handles packet flow listing (recent flows)
func (s *Server) handleFlows(w http.ResponseWriter, r *http.Request) {
	// Return information about packet flow generation
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "active",
		"ws_endpoint": "/ws",
		"description": "Connect to /ws for real-time packet flow updates",
	})
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	var shutdownErr error

	s.shutdownOnce.Do(func() {
		log.Info().Msg("shutting down dashboard backend")
		close(s.shutdown)

		// Shutdown HTTP server
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("http server shutdown error")
			shutdownErr = err
		}

		// Shutdown WebSocket server
		if err := s.wsServer.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("websocket server shutdown error")
			if shutdownErr == nil {
				shutdownErr = err
			}
		}

		// Shutdown metrics aggregator
		if err := s.metricsAgg.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("metrics aggregator shutdown error")
			if shutdownErr == nil {
				shutdownErr = err
			}
		}

		// Close Busboy client
		if s.busboyClient != nil {
			if err := s.busboyClient.Close(); err != nil {
				log.Error().Err(err).Msg("busboy client close error")
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
			log.Info().Msg("dashboard backend shutdown complete")
		case <-ctx.Done():
			if shutdownErr == nil {
				shutdownErr = ctx.Err()
			}
			log.Error().Err(shutdownErr).Msg("dashboard backend shutdown timeout")
		}
	})

	return shutdownErr
}
