package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
	"unheaded/pkg/lifecycle"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/architect"
)

// Metrics
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unheaded_http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"service", "method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "unheaded_http_request_duration_seconds",
			Help: "HTTP request latency",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		},
		[]string{"service", "method", "path"},
	)

	wotanMessagesPublished = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unheaded_wotan_messages_published_total",
			Help: "Messages published to Wotan",
		},
		[]string{"service", "topic"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(wotanMessagesPublished)
}

func main() {
	// Flags
	addr := flag.String("addr", ":8001", "HTTP listen address")
	wotanAddr := flag.String("wotan", "localhost:8081", "Wotan HTTP address")
	logLevel := flag.String("log", "info", "Log level (debug, info, warn, error)")
	useMock := flag.Bool("mock-wotan", false, "Use mock Wotan client for testing")
	flag.Parse()

	// Setup logging
	zerolog.SetGlobalLevel(parseLogLevel(*logLevel))
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	log.Info().
		Str("addr", *addr).
		Str("wotan", *wotanAddr).
		Str("log_level", *logLevel).
		Bool("mock_wotan", *useMock).
		Msg("architect service starting")

	// Connect to Wotan
	var wotanConn *wotanClient.Client

	if *useMock {
		log.Info().Msg("using mock Wotan client - no actual Wotan integration")
		// Mock client doesn't support full interface, create nil client
		wotanConn = nil
	} else {
		var err error
		wotanConn, err = wotanClient.NewClient(*wotanAddr)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to create Wotan client")
		}
		defer wotanConn.Close()
	}

	// Create service with Wotan integration
	var svc *architect.ArchitectService
	if wotanConn != nil {
		svc = architect.NewWithWotan(wotanConn)
	} else {
		svc = architect.New()
	}

	// Start service (subscribes to alerts.critical and architecture.updates)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := svc.Start(ctx); err != nil {
		log.Warn().Err(err).Msg("failed to start Wotan integration")
	}
	cancel()

	// Create a long-running context for alert listening
	_, alertCancel := context.WithCancel(context.Background())
	defer alertCancel()

	// HTTP handler
	handler := architect.NewHTTPHandler(svc)

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/health", handler.Health)

	// Infrastructure endpoints
	mux.HandleFunc("/infrastructure", instrument(handler.GetInfrastructure, "GET_INFRASTRUCTURE"))
	mux.HandleFunc("/infrastructure/services", instrument(handler.AddService, "ADD_SERVICE"))

	// Network endpoints
	mux.HandleFunc("/network", instrument(handler.GetNetwork, "GET_NETWORK"))
	mux.HandleFunc("/network/nodes", instrument(handler.AddNetworkNode, "ADD_NETWORK_NODE"))

	// Design endpoints
	mux.HandleFunc("/design", handleDesign(handler, "GET_DESIGN_DECISIONS"))
	mux.Handle("/metrics", promhttp.Handler())

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("architect")
	var httpHandler http.Handler = mux
	httpHandler = auth.WrapHandler(httpHandler, auth.SetupMiddleware(authCfg))

	// HTTP server
	server := &http.Server{
		Addr:         *addr,
		Handler:      httpHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Start server in goroutine
	go func() {
		log.Info().Str("addr", *addr).Msg("starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	// Graceful shutdown
	lifecycle.WaitForShutdown(server, 10*time.Second, alertCancel)
}

// statusRecorder wraps http.ResponseWriter to capture the HTTP status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// instrument wraps a handler with metrics and logging
func instrument(h http.HandlerFunc, operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		// Call handler
		h(rec, r)

		// Record metrics after handler completes
		duration := time.Since(start).Seconds()
		httpRequestDuration.WithLabelValues("architect", r.Method, r.URL.Path).Observe(duration)
		httpRequestsTotal.WithLabelValues("architect", r.Method, r.URL.Path, fmt.Sprintf("%d", rec.status)).Inc()

		log.Debug().
			Str("operation", operation).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rec.status).
			Dur("duration", time.Since(start)).
			Msg("request processed")
	}
}

// handleDesign handles both GET and POST for /design endpoint
func handleDesign(h *architect.HTTPHandler, operation string) http.HandlerFunc {
	return instrument(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			h.LogDesign(w, r)
		} else if r.Method == http.MethodGet {
			h.GetDesignDecisions(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}, operation)
}

// parseLogLevel parses the log level string
func parseLogLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
