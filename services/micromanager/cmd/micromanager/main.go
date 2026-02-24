package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
	"unheaded/pkg/lifecycle"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/micromanager"
)

var (
	port            = flag.String("port", "19003", "HTTP port to listen on")
	wotanAddr      = flag.String("wotan", "", "Wotan address (host:port)")
	logLevel        = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	readTimeout     = flag.Duration("read-timeout", 15*time.Second, "HTTP read timeout")
	writeTimeout    = flag.Duration("write-timeout", 15*time.Second, "HTTP write timeout")
	idleTimeout     = flag.Duration("idle-timeout", 60*time.Second, "HTTP idle timeout")
	shutdownTimeout = flag.Duration("shutdown-timeout", 30*time.Second, "Graceful shutdown timeout")
)

// Metrics
var (
	httpRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "micromanager_http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "micromanager_http_duration_seconds",
			Help: "HTTP request duration in seconds",
		},
		[]string{"method", "path"},
	)

	tasksTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "micromanager_tasks_total",
			Help: "Total number of tasks",
		},
		[]string{"status"},
	)
)

func init() {
	// Configure zerolog
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
}

func main() {
	flag.Parse()

	// Set log level
	switch *logLevel {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	log.Info().
		Str("version", "1.0.0").
		Str("port", *port).
		Str("wotan", *wotanAddr).
		Msg("starting micromanager service")

	// Create store
	store := micromanager.NewStore()

	// Create Wotan client (if configured)
	var wotan *wotanClient.Client
	if *wotanAddr != "" {
		var err error
		wotan, err = wotanClient.NewClient(*wotanAddr)
		if err != nil {
			log.Error().Err(err).Str("addr", *wotanAddr).Msg("failed to create wotan client")
			// Continue anyway, just without wotan integration
		}
	}

	// Create service
	service := micromanager.NewService(store, wotan)

	// Start service
	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		log.Error().Err(err).Msg("failed to start service")
		os.Exit(1)
	}

	// Create API
	api := micromanager.NewAPI(store, service)

	// Set up HTTP routes with middleware
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/health", withMetrics("/health", api.Health))
	mux.HandleFunc("/ready", withMetrics("/ready", readinessProbe(service)))

	// API endpoints
	mux.HandleFunc("/api/v1/backlog", withMetrics("/api/v1/backlog", api.GetBacklog))
	mux.HandleFunc("/api/v1/tasks", withMetrics("/api/v1/tasks", api.CreateTask))
	mux.HandleFunc("/api/v1/sprint/status", withMetrics("/api/v1/sprint/status", api.GetSprintStatus))

	// Dynamic task endpoints
	mux.HandleFunc("/api/v1/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			withMetrics("/api/v1/tasks/:id", api.UpdateTask)(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("micromanager")
	var httpHandler http.Handler = mux
	httpHandler = auth.WrapHandler(httpHandler, auth.SetupMiddleware(authCfg))

	// Server configuration
	srv := &http.Server{
		Addr:         ":" + *port,
		Handler:      httpHandler,
		ReadTimeout:  *readTimeout,
		WriteTimeout: *writeTimeout,
		IdleTimeout:    *idleTimeout,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Start server in goroutine
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("server error")
		}
	}()

	// Graceful shutdown
	lifecycle.WaitForShutdown(srv, *shutdownTimeout, func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), *shutdownTimeout)
		defer stopCancel()
		if err := service.Stop(stopCtx); err != nil {
			log.Error().Err(err).Msg("service stop error")
		}
	})
}

// withMetrics wraps a handler with metrics collection
func withMetrics(path string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status
		wr := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call handler
		h(wr, r)

		// Record metrics
		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", wr.statusCode)

		httpRequests.WithLabelValues(r.Method, path, status).Inc()
		httpDuration.WithLabelValues(r.Method, path).Observe(duration)
	}
}

// responseWriter captures the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// readinessProbe returns a readiness check handler
func readinessProbe(service *micromanager.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simple readiness check - if service is running, we're ready
		status := service.HealthStatus()
		w.Header().Set("Content-Type", "application/json")

		if status["status"] == "healthy" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		fmt.Fprintf(w, `{"status":"ready","timestamp":%d}`, time.Now().Unix())
	}
}
