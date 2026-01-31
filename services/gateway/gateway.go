// Package gateway provides the core API Gateway service.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"unheaded/pkg/busboy-client"
	"unheaded/pkg/health"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/services/gateway/config"
	"unheaded/services/gateway/middleware"
	"unheaded/services/gateway/proxy"
	"unheaded/services/gateway/routes"
)

// Gateway is the main API Gateway service.
type Gateway struct {
	cfg           *config.Config
	log           *logger.Logger
	metrics       *metrics.Registry
	router        *routes.Router
	healthChecker *proxy.HealthChecker
	rateLimiter   *middleware.RateLimiter
	busboy        *busboy.Client
	httpServer    *http.Server
	http3Server   interface{} // http3.Server when enabled
	healthMgr     *health.Manager
	mu            sync.RWMutex
	running       bool
}

// New creates a new Gateway instance.
func New(cfg *config.Config, log *logger.Logger) (*Gateway, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create metrics registry
	metricsReg := metrics.NewRegistry("gateway")

	// Create router
	router := routes.NewRouter(log, metricsReg)

	// Create health checker
	healthChecker := proxy.NewHealthChecker(30*time.Second, 5*time.Second, log)

	// Create rate limiter
	rateLimiter := middleware.NewRateLimiter(&cfg.RateLimit, log, metricsReg)

	// Create Busboy client
	var busboyClient *busboy.Client
	if cfg.Busboy.Enabled {
		busboyClient = busboy.NewClient(cfg.Busboy.URL, cfg.Busboy.ServiceName)
	}

	// Create health manager
	healthMgr := health.NewManager()

	g := &Gateway{
		cfg:           cfg,
		log:           log,
		metrics:       metricsReg,
		router:        router,
		healthChecker: healthChecker,
		rateLimiter:   rateLimiter,
		busboy:        busboyClient,
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
	tracingMw := middleware.NewTracingMiddleware(g.cfg.Busboy.ServiceName)
	handler = tracingMw.Handler(handler)

	// Busboy event publishing
	if g.busboy != nil {
		handler = g.busboyMiddleware(handler)
	}

	return handler
}

// busboyMiddleware publishes request events to Busboy.
func (g *Gateway) busboyMiddleware(next http.Handler) http.Handler {
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

			if err := g.busboy.Publish(g.cfg.Busboy.Topic, event); err != nil {
				g.log.Warn("Failed to publish request event",
					"error", err.Error(),
				)
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

	// Build handler
	handler := g.buildHandler()

	// Create mux for system endpoints
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", g.healthHandler)
	mux.HandleFunc("/health/live", g.livenessHandler)
	mux.HandleFunc("/health/ready", g.readinessHandler)

	// Metrics endpoint
	if g.cfg.Metrics.Enabled {
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
	g.healthMgr.Register("gateway", health.CheckFunc(func(ctx context.Context) error {
		return nil // Gateway is healthy if running
	}))

	g.log.Info("Starting API Gateway",
		"http_port", g.cfg.Server.HTTPPort,
		"routes", len(g.cfg.Routes),
	)

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

	g.log.Info("Shutting down API Gateway")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), g.cfg.Server.ShutdownTimeout)
	defer cancel()

	// Stop health checker
	g.healthChecker.Stop()

	// Stop rate limiter
	g.rateLimiter.Stop()

	// Close router
	g.router.Close()

	// Close Busboy client
	if g.busboy != nil {
		g.busboy.Close()
	}

	// Shutdown HTTP server
	if err := g.httpServer.Shutdown(ctx); err != nil {
		g.log.Error("HTTP server shutdown error",
			"error", err.Error(),
		)
		return err
	}

	g.log.Info("API Gateway shutdown complete")
	return nil
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
	g.metrics.WritePrometheus(w)
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
