// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the Business Source License 1.1 (BSL 1.1).
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

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
	"bufio"
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	ebpfPkg "unheaded/cmd/dashboard-backend/internal/ebpf"
	"unheaded/cmd/dashboard-backend/internal/events"
	"unheaded/cmd/dashboard-backend/internal/health"
	"unheaded/cmd/dashboard-backend/internal/packetflow"
	"unheaded/cmd/dashboard-backend/internal/scraper"
	"unheaded/cmd/dashboard-backend/internal/server"
	"unheaded/cmd/dashboard-backend/internal/websocket"
	"unheaded/pkg/discovery"
	"unheaded/pkg/logagg"
	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
)

// Embed the dashboard static files into the binary so it works
// regardless of the working directory.
//
//go:embed static/*
var staticFiles embed.FS

var (
	// Version information (set by build)
	Version   = "0.1.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

var (
	listenAddr     = flag.String("listen", ports.DefaultAddr(ports.DashboardBackend), "HTTP listen address")
	wotanAddr     = flag.String("wotan", "localhost:18001", "Wotan server address")
	wotanGRPCAddr = flag.String("wotan-grpc-addr", "", "Wotan gRPC address for TopicStream (enables real eBPF events)")
	debug          = flag.Bool("debug", false, "Enable debug logging")
	jsonLogs       = flag.Bool("json", false, "Output logs in JSON format")

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

	// WebSocket allowed origins for LAN access
	wsAllowedOrigins = flag.String("ws-allowed-origins", "", "Comma-separated WebSocket allowed origins (empty=localhost only)")

	// Service endpoint overrides
	// TODO: Default to 127.0.0.1 endpoints instead of LXD IPs. --services-file
	// should remain as an override option, but defaults should be localhost.
	// Eventually move to a main config file and/or UI-based configuration.
	servicesFile = flag.String("services-file", "", "Path to services endpoint file (name=host:port per line)")
	vizDir       = flag.String("viz-dir", "", "Path to advanced visualizations directory (dashboard/), served under /viz/")
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

	// Initialize transport configuration
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	if *wotanAddr != "" {
		transportCfg.WotanHTTPAddr = "http://" + *wotanAddr
	}
	if *wotanGRPCAddr != "" {
		transportCfg.WotanGRPCAddr = *wotanGRPCAddr
	}

	// Create transport health server
	healthSrv := transport.NewHealthServer("dashboard-backend")

	log.Info().
		Str("grpc_addr", transportCfg.WotanGRPCAddr).
		Str("http_addr", transportCfg.WotanHTTPAddr).
		Str("transport", string(transportCfg.Type)).
		Msg("transport config initialized")

	// Log aggregation publisher — forwards structured logs to Wotan
	_ = logagg.NewPublisher("dashboard-backend", nil) // nil transport — publisher disabled until zerolog is adopted

	// Load service endpoint overrides
	serviceEndpoints, err := loadServiceEndpoints(*servicesFile)
	if err != nil {
		log.Fatal().Err(err).Str("file", *servicesFile).Msg("failed to load services file")
	}
	if len(serviceEndpoints) > 0 {
		log.Info().Int("count", len(serviceEndpoints)).Msg("loaded service endpoint overrides")
		for name, addr := range serviceEndpoints {
			log.Info().Str("service", name).Str("addr", addr).Msg("service endpoint")
		}
	}

	// Create embedded static filesystem (strips the "static/" prefix)
	dashboardFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create static filesystem")
	}

	// Create server config
	config := &server.Config{
		ListenAddr:   *listenAddr,
		WotanAddr:   *wotanAddr,
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
			AllowedOrigins: parseAllowedOrigins(*wsAllowedOrigins),
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
			WotanAddr:        *wotanAddr,
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

		ServiceEndpoints: serviceEndpoints,
		StaticFS:         dashboardFS,
		VizDir:          *vizDir,
	}

	// Create eBPF ingestor if gRPC address is provided (or from WOTAN_GRPC_ADDR env)
	grpcAddr := *wotanGRPCAddr
	if grpcAddr == "" {
		grpcAddr = os.Getenv("WOTAN_GRPC_ADDR")
	}
	if grpcAddr != "" {
		log.Info().Str("grpc_addr", grpcAddr).Msg("creating TopicStreamClient for eBPF event ingestion")

		httpFallback, _ := wotanClient.NewClient(*wotanAddr)
		tsc, err := wotanClient.NewTopicStreamClient(grpcAddr,
			wotanClient.WithHTTPFallback(httpFallback),
		)
		if err != nil {
			log.Warn().Err(err).Msg("failed to create TopicStreamClient, eBPF events disabled")
			healthSrv.SetGRPCStatus(false)
		} else {
			ingestorConfig := ebpfPkg.DefaultIngestorConfig()
			config.EBPFIngestor = ebpfPkg.NewIngestor(ingestorConfig, tsc, log)
			log.Info().Msg("eBPF event ingestor configured")
		}
	}

	// Create server
	srv, err := server.NewServer(config, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create server")
	}

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Service discovery registration
	discovery.SetupServiceDiscovery(ctx, nil, "dashboard-backend", 20000)

	if err := srv.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start server")
	}

	log.Info().
		Str("addr", *listenAddr).
		Str("wotan", *wotanAddr).
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

// parseAllowedOrigins splits a comma-separated origins string.
// Returns nil (not empty slice) when input is empty — this preserves
// the deny-by-default behavior in websocket.Server.isOriginAllowed().
func parseAllowedOrigins(origins string) []string {
	if origins == "" {
		return nil
	}
	parts := strings.Split(origins, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// getEventTopics returns the list of Wotan topics to subscribe to
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
		"ebpf.packet.events",
		"ebpf.flow.events",
		"ebpf.latency.events",
		"ebpf.syscall.events",
		"compute.hop",
		"compute.miss",
		"compute.halt",
		"compute.syscall",
		"traces.packet",
		"traces.flow",
		"traces.latency",
		"traces.correlated",
	}
}

// loadServiceEndpoints loads service endpoint overrides from a file.
// File format: one "name=host:port" per line. Lines starting with # are comments.
// Also checks SERVICES_FILE env var if no flag provided.
// Returns nil map (no overrides) if no file specified.
func loadServiceEndpoints(path string) (map[string]string, error) {
	if path == "" {
		path = os.Getenv("SERVICES_FILE")
	}
	if path == "" {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open services file: %w", err)
	}
	defer f.Close()

	endpoints := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line (expected name=host:port): %q", line)
		}
		name := strings.TrimSpace(parts[0])
		addr := strings.TrimSpace(parts[1])
		if name == "" || addr == "" {
			return nil, fmt.Errorf("empty name or address in: %q", line)
		}
		endpoints[name] = addr
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read services file: %w", err)
	}

	return endpoints, nil
}
