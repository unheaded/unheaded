package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/unheaded/unheaded/pkg/busboy-client"
	"github.com/unheaded/unheaded/pkg/busboy-client/mock"
	"github.com/unheaded/unheaded/services/architect"
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

	busboyMessagesPublished = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unheaded_busboy_messages_published_total",
			Help: "Messages published to Busboy",
		},
		[]string{"service", "topic"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(busboyMessagesPublished)
}

func main() {
	// Flags
	addr := flag.String("addr", ":8001", "HTTP listen address")
	busboyAddr := flag.String("busboy", "localhost:9090", "Busboy address")
	logLevel := flag.String("log", "info", "Log level (debug, info, warn, error)")
	useMock := flag.Bool("mock-busboy", false, "Use mock Busboy client for testing")
	flag.Parse()

	// Setup logging
	zerolog.SetGlobalLevel(parseLogLevel(*logLevel))
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	log.Info().
		Str("addr", *addr).
		Str("busboy", *busboyAddr).
		Str("log_level", *logLevel).
		Bool("mock_busboy", *useMock).
		Msg("architect service starting")

	// Create service
	svc := architect.New()

	// Connect to Busboy
	var busboyClient interface {
		Subscribe(ctx context.Context, topic, displayName string) (*busboyClient.Subscriber, error)
		Publish(ctx context.Context, topic string, payload []byte) error
		Close() error
	}

	if *useMock {
		log.Info().Msg("using mock Busboy client")
		busboyClient = mock.NewMockClient(mock.WithAutoApprove())
	} else {
		client, err := busboyClient.NewClient(*busboyAddr)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to create Busboy client")
		}
		busboyClient = client

		// Subscribe to relevant topics
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if _, err := busboyClient.Subscribe(ctx, "architecture.updates", "architect"); err != nil {
			log.Warn().Err(err).Msg("failed to subscribe to architecture.updates")
		}
		cancel()
	}
	defer busboyClient.Close()

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

	// HTTP server
	server := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Info().Str("addr", *addr).Msg("starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Info().Msg("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
	}

	log.Info().Msg("architect service stopped")
}

// instrument wraps a handler with metrics and logging
func instrument(h http.HandlerFunc, operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Record metrics
		defer func() {
			duration := time.Since(start).Seconds()
			httpRequestDuration.WithLabelValues("architect", r.Method, r.URL.Path).Observe(duration)

			log.Debug().
				Str("operation", operation).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Dur("duration", time.Since(start)).
				Msg("request processed")
		}()

		// Call handler
		h(w, r)

		// Record request total
		httpRequestsTotal.WithLabelValues("architect", r.Method, r.URL.Path, "ok").Inc()
	}
}

// handleDesign handles both GET and POST for /design endpoint
func handleDesign(h *architect.HTTPHandler, operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		defer func() {
			duration := time.Since(start).Seconds()
			httpRequestDuration.WithLabelValues("architect", r.Method, r.URL.Path).Observe(duration)
			httpRequestsTotal.WithLabelValues("architect", r.Method, r.URL.Path, "ok").Inc()
		}()

		if r.Method == http.MethodPost {
			h.LogDesign(w, r)
		} else if r.Method == http.MethodGet {
			h.GetDesignDecisions(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
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
