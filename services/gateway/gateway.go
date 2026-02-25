// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package gateway provides the core API Gateway service.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/services/gateway/config"
	"unheaded/services/gateway/middleware"
	"unheaded/services/gateway/proxy"
	"unheaded/services/gateway/routes"
)

// CheckFunc is a function that performs a health check.
type CheckFunc func(ctx context.Context) error

// healthStatus represents the result of health checks.
type healthStatus struct {
	Healthy bool              `json:"healthy"`
	Checks  map[string]string `json:"checks"`
}

// healthManager manages simple health checks for the gateway.
type healthManager struct {
	checks map[string]CheckFunc
	mu     sync.RWMutex
}

// newHealthManager creates a new health manager.
func newHealthManager() *healthManager {
	return &healthManager{
		checks: make(map[string]CheckFunc),
	}
}

// Register adds a health check.
func (m *healthManager) Register(name string, check CheckFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checks[name] = check
}

// CheckAll runs all health checks and returns the aggregate status.
func (m *healthManager) CheckAll(ctx context.Context) *healthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := &healthStatus{
		Healthy: true,
		Checks:  make(map[string]string),
	}

	for name, check := range m.checks {
		if err := check(ctx); err != nil {
			status.Healthy = false
			status.Checks[name] = err.Error()
		} else {
			status.Checks[name] = "healthy"
		}
	}

	return status
}

// Gateway is the main API Gateway service.
type Gateway struct {
	cfg           *config.Config
	log           *logger.Logger
	metrics       *metrics.Registry
	router        *routes.Router
	healthChecker *proxy.HealthChecker
	rateLimiter   *middleware.RateLimiter
	wotan        *wotanClient.Client
	httpServer    *http.Server
	http3Server   interface{} // http3.Server when enabled
	healthMgr     *healthManager
	mu            sync.RWMutex
	running       bool
	alertCancel   context.CancelFunc // for cancelling alert listener
}

// New creates a new Gateway instance.
func New(cfg *config.Config, log *logger.Logger) (*Gateway, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create metrics registry
	metricsReg := metrics.NewRegistry()

	// Create router
	router := routes.NewRouter(log, metricsReg)

	// Create health checker
	healthChecker := proxy.NewHealthChecker(30*time.Second, 5*time.Second, log)

	// Create rate limiter
	rateLimiter := middleware.NewRateLimiter(&cfg.RateLimit, log, metricsReg)

	// Create Wotan client
	var client *wotanClient.Client
	if cfg.Wotan.Enabled {
		var err error
		client, err = wotanClient.NewClient(cfg.Wotan.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to create wotan client: %w", err)
		}
	}

	// Create health manager
	healthMgr := newHealthManager()

	g := &Gateway{
		cfg:           cfg,
		log:           log,
		metrics:       metricsReg,
		router:        router,
		healthChecker: healthChecker,
		rateLimiter:   rateLimiter,
		wotan:        client,
		healthMgr:     healthMgr,
	}

	// Configure routes
	for _, routeCfg := range cfg.Routes {
		if err := router.AddRoute(&routeCfg, &cfg.Circuit); err != nil {
			return nil, fmt.Errorf("failed to add route %s: %w", routeCfg.Name, err)
		}

		// Register with health checker
		if routeCfg.HealthCheckURL != "" {
			route := router.GetRoute(routeCfg.Name)
			if route != nil {
				healthChecker.RegisterRoute(routeCfg.Name, routeCfg.HealthCheckURL, route.LoadBalancer)
			}
		}
	}

	return g, nil
}

// buildHandler builds the HTTP handler with middleware chain.
func (g *Gateway) buildHandler() http.Handler {
	// Start with the router
	var handler http.Handler = g.router

	// Add middleware (applied in reverse order)
	// Logging should be outermost to capture all requests
	loggingMw := middleware.NewLoggingMiddleware(g.log, g.metrics)
	handler = loggingMw.Handler(handler)

	// Rate limiting
	handler = g.rateLimiter.Handler(handler)

	// Authentication
	authMw := middleware.NewAuthMiddleware(&g.cfg.Auth, g.log)
	handler = authMw.Handler(handler)

	// CORS
	corsMw := middleware.NewCORSMiddleware(&g.cfg.CORS)
	handler = corsMw.Handler(handler)

	// Tracing (should be one of the first to execute)
	tracingMw := middleware.NewTracingMiddleware(g.cfg.Wotan.ServiceName)
	handler = tracingMw.Handler(handler)

	// Wotan event publishing
	if g.wotan != nil {
		handler = g.wotanMiddleware(handler)
	}

	return handler
}

// wotanMiddleware publishes request events to Wotan.
func (g *Gateway) wotanMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Publish request event asynchronously
		go func() {
			event := map[string]interface{}{
				"type":       "request",
				"method":     r.Method,
				"path":       r.URL.Path,
				"trace_id":   middleware.GetTraceID(r.Context()),
				"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
				"user_agent": r.UserAgent(),
				"remote_ip":  r.RemoteAddr,
			}

			payload, err := json.Marshal(event)
			if err != nil {
				g.log.Warn().Str("error", err.Error()).Msg("Failed to marshal request event")
				return
			}

			if err := g.wotan.Publish(r.Context(), g.cfg.Wotan.Topic, payload); err != nil {
				g.log.Warn().Str("error", err.Error()).Msg("Failed to publish request event")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// Start starts the gateway.
func (g *Gateway) Start(ctx context.Context) error {
	g.mu.Lock()
	if g.running {
		g.mu.Unlock()
		return fmt.Errorf("gateway already running")
	}
	g.running = true
	g.mu.Unlock()

	// Subscribe to alerts.critical (required by CLAUDE.md)
	if g.wotan != nil {
		subCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if _, err := g.wotan.Subscribe(subCtx, "alerts.critical", "gateway-service"); err != nil {
			g.log.Warn().Str("error", err.Error()).Msg("failed to subscribe to alerts.critical")
		} else {
			g.log.Info().Msg("subscribed to alerts.critical")
		}
		cancel()

		// Subscribe to gateway.events for own topic
		subCtx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		if _, err := g.wotan.Subscribe(subCtx2, g.cfg.Wotan.Topic, "gateway-service"); err != nil {
			g.log.Warn().Str("error", err.Error()).Msg("failed to subscribe to gateway topic")
		} else {
			g.log.Info().Str("topic", g.cfg.Wotan.Topic).Msg("subscribed to gateway topic")
		}
		cancel2()

		// Start alert listener
		alertCtx, alertCancel := context.WithCancel(context.Background())
		g.alertCancel = alertCancel
		go g.listenForAlerts(alertCtx)
	}

	// Build handler
	handler := g.buildHandler()

	// Create mux for system endpoints
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", g.healthHandler)
	mux.HandleFunc("/health/live", g.livenessHandler)
	mux.HandleFunc("/health/ready", g.readinessHandler)
	mux.HandleFunc("/ready", g.readinessHandler) // Standard /ready endpoint (required by CLAUDE.md)

	// Metrics endpoint - always register /metrics (required by CLAUDE.md)
	mux.HandleFunc("/metrics", g.metricsHandler)
	if g.cfg.Metrics.Enabled && g.cfg.Metrics.Path != "" && g.cfg.Metrics.Path != "/metrics" {
		mux.HandleFunc(g.cfg.Metrics.Path, g.metricsHandler)
	}

	// Routes info endpoint
	mux.Handle("/_routes", routes.NewRoutesHandler(g.router))

	// All other requests go to the gateway
	mux.Handle("/", handler)

	// Create HTTP server
	g.httpServer = &http.Server{
		Addr:           fmt.Sprintf(":%d", g.cfg.Server.HTTPPort),
		Handler:        mux,
		ReadTimeout:    g.cfg.Server.ReadTimeout,
		WriteTimeout:   g.cfg.Server.WriteTimeout,
		IdleTimeout:    g.cfg.Server.IdleTimeout,
		MaxHeaderBytes: g.cfg.Server.MaxHeaderBytes,
	}

	// Start health checker
	g.healthChecker.Start()

	// Register health checks
	g.healthMgr.Register("gateway", func(ctx context.Context) error {
		return nil // Gateway is healthy if running
	})

	g.log.Info().
		Int("http_port", g.cfg.Server.HTTPPort).
		Int("routes", len(g.cfg.Routes)).
		Msg("Starting API Gateway")

	// Start HTTP server
	errCh := make(chan error, 1)
	go func() {
		var err error
		if g.cfg.Server.TLSCertFile != "" && g.cfg.Server.TLSKeyFile != "" {
			err = g.httpServer.ListenAndServeTLS(g.cfg.Server.TLSCertFile, g.cfg.Server.TLSKeyFile)
		} else {
			err = g.httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		return g.Shutdown()
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully shuts down the gateway.
func (g *Gateway) Shutdown() error {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return nil
	}
	g.running = false
	g.mu.Unlock()

	g.log.Info().Msg("Shutting down API Gateway")

	// Cancel alert listener
	if g.alertCancel != nil {
		g.alertCancel()
	}

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), g.cfg.Server.ShutdownTimeout)
	defer cancel()

	// Stop health checker
	g.healthChecker.Stop()

	// Stop rate limiter
	g.rateLimiter.Stop()

	// Close router
	g.router.Close()

	// Close Wotan client
	if g.wotan != nil {
		g.wotan.Close()
	}

	// Shutdown HTTP server
	if err := g.httpServer.Shutdown(ctx); err != nil {
		g.log.Error().Str("error", err.Error()).Msg("HTTP server shutdown error")
		return err
	}

	g.log.Info().Msg("API Gateway shutdown complete")
	return nil
}

// listenForAlerts listens for critical alerts from Wotan
func (g *Gateway) listenForAlerts(ctx context.Context) {
	if g.wotan == nil {
		return
	}

	msgCh, err := g.wotan.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		g.log.Warn().Str("error", err.Error()).Msg("failed to stream alerts.critical")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			g.handleCriticalAlert(ctx, msg)
		}
	}
}

// handleCriticalAlert processes critical alerts
func (g *Gateway) handleCriticalAlert(ctx context.Context, msg *wotanClient.Message) {
	if msg == nil {
		return
	}

	g.log.Warn().
		Str("message_id", msg.MessageID).
		Str("topic", msg.Topic).
		Msg("received critical alert - gateway monitoring")

	// Gateway can respond to alerts by potentially updating route health status
	// or triggering circuit breakers
	_ = ctx
}

// healthHandler handles health check requests.
func (g *Gateway) healthHandler(w http.ResponseWriter, r *http.Request) {
	status := g.healthMgr.CheckAll(r.Context())

	w.Header().Set("Content-Type", "application/json")
	if !status.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// livenessHandler handles Kubernetes liveness probe.
func (g *Gateway) livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"alive"}`))
}

// readinessHandler handles Kubernetes readiness probe.
func (g *Gateway) readinessHandler(w http.ResponseWriter, r *http.Request) {
	// Check if we have at least one healthy backend
	routeInfos := g.router.Info()
	hasHealthyBackend := false

	for _, info := range routeInfos {
		if len(info.HealthyBackends) > 0 {
			hasHealthyBackend = true
			break
		}
	}

	if !hasHealthyBackend && len(routeInfos) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"not_ready","reason":"no_healthy_backends"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ready"}`))
}

// metricsHandler handles Prometheus metrics endpoint.
func (g *Gateway) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	g.metrics.Gather(w)
}

// GetRouter returns the gateway router.
func (g *Gateway) GetRouter() *routes.Router {
	return g.router
}

// GetMetrics returns the metrics registry.
func (g *Gateway) GetMetrics() *metrics.Registry {
	return g.metrics
}

// IsRunning returns whether the gateway is running.
func (g *Gateway) IsRunning() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.running
}
