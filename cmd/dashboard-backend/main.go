// Package main provides the entry point for the dashboard backend service.
// The dashboard backend aggregates metrics, health status, and events from all
// Kingdom services and serves them via REST API and WebSocket streaming.
//
// Endpoints:
//   - GET /health - Health check
//   - GET /ready - Readiness check
//   - GET /metrics - Prometheus metrics
//   - GET /api/v1/metrics - Aggregated metrics from all services
//   - POST /api/v1/metrics/query - Query specific metrics
//   - GET /api/v1/services - Service status list
//   - GET /api/v1/services/{name} - Service details
//   - GET /api/v1/events - Recent events
//   - GET /api/v1/events/summary - Event summary
//   - GET /api/v1/health - System health overview
//   - GET /api/v1/health/{name} - Service health details
//   - GET /api/v1/flows - Packet flow info
//   - GET /api/v1/stats - Dashboard statistics
//   - WS /ws - WebSocket for real-time updates
//   - WS /ws/metrics - WebSocket for metrics streaming
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"unheaded/cmd/dashboard-backend/internal/events"
	"unheaded/cmd/dashboard-backend/internal/health"
	"unheaded/cmd/dashboard-backend/internal/packetflow"
	"unheaded/cmd/dashboard-backend/internal/scraper"
	"unheaded/cmd/dashboard-backend/internal/server"
	"unheaded/cmd/dashboard-backend/internal/websocket"
	"unheaded/pkg/logger"
)

var (
	// Version information (set by build)
	Version   = "0.1.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

var (
	listenAddr = flag.String("listen", ":8080", "HTTP listen address")
	busboyAddr = flag.String("busboy", "localhost:9090", "Busboy server address")
	debug      = flag.Bool("debug", false, "Enable debug logging")
	jsonLogs   = flag.Bool("json", false, "Output logs in JSON format")

	// Scraper settings
	scrapeInterval = flag.Duration("scrape-interval", 15*time.Second, "Metrics scrape interval")
	scrapeTimeout  = flag.Duration("scrape-timeout", 10*time.Second, "Metrics scrape timeout")

	// Health settings
	healthInterval = flag.Duration("health-interval", 15*time.Second, "Health check interval")
	healthTimeout  = flag.Duration("health-timeout", 5*time.Second, "Health check timeout")

	// WebSocket settings
	maxConnections = flag.Int("max-connections", 100, "Maximum WebSocket connections")

	// Packet flow settings
	flowInterval = flag.Duration("flow-interval", 100*time.Millisecond, "Packet flow generation interval")
	maxFlows     = flag.Int("max-flows", 50, "Maximum concurrent flows")
)

func main() {
	flag.Parse()

	// Setup logging using Kingdom's native logger
	logConfig := logger.DefaultConfig()
	if *debug {
		logConfig.Level = logger.DebugLevel
	} else {
		logConfig.Level = logger.InfoLevel
	}
	if !*jsonLogs {
		logConfig.ConsoleMode = true
		logConfig.ColorEnabled = true
	}
	logConfig.CallerEnabled = *debug

	log := logger.NewWithConfig(logConfig)

	log.Info().
		Str("service", "dashboard-backend").
		Str("version", Version).
		Str("build_time", BuildTime).
		Str("git_commit", GitCommit).
		Msg("starting dashboard backend")

	// Create server config
	config := &server.Config{
		ListenAddr:   *listenAddr,
		BusboyAddr:   *busboyAddr,
		ServiceName:  "dashboard-backend",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,

		WebSocketConfig: &websocket.Config{
			MaxConnections: *maxConnections,
			ReadTimeout:    60 * time.Second,
			WriteTimeout:   10 * time.Second,
			PingInterval:   30 * time.Second,
			BufferSize:     256,
			MaxMessageSize: 65536,
		},

		ScraperConfig: &scraper.Config{
			ScrapeInterval:  *scrapeInterval,
			ScrapeTimeout:   *scrapeTimeout,
			RetentionPeriod: 1 * time.Hour,
			MaxSamples:      1000,
		},

		HealthConfig: &health.Config{
			PollInterval:      *healthInterval,
			PollTimeout:       *healthTimeout,
			RetentionPeriod:   24 * time.Hour,
			MaxHistory:        1000,
			FailureThreshold:  3,
			RecoveryThreshold: 2,
		},

		EventsConfig: &events.Config{
			BusboyAddr:        *busboyAddr,
			ServiceName:       "dashboard-backend",
			Topics:            getEventTopics(),
			BufferSize:        1000,
			RetentionPeriod:   1 * time.Hour,
			ReconnectInterval: 5 * time.Second,
		},

		PacketFlowConfig: &packetflow.Config{
			Interval:       *flowInterval,
			MaxFlows:       *maxFlows,
			TraceIDPattern: "trace-%d",
		},
	}

	// Create server
	srv, err := server.NewServer(config, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create server")
	}

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start server")
	}

	log.Info().
		Str("addr", *listenAddr).
		Str("busboy", *busboyAddr).
		Int("max_connections", *maxConnections).
		Dur("scrape_interval", *scrapeInterval).
		Dur("health_interval", *healthInterval).
		Msg("dashboard backend running")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Info().
		Str("signal", sig.String()).
		Msg("shutdown signal received")

	// Cancel context to signal all goroutines
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown error")
		os.Exit(1)
	}

	log.Info().Msg("dashboard backend stopped")
}

// getEventTopics returns the list of Busboy topics to subscribe to
func getEventTopics() []string {
	return []string{
		"metrics.*",
		"health.*",
		"alerts.*",
		"timeline.*",
		"tasks.*",
		"decisions.*",
		"state.*",
		"architecture.*",
	}
}
