// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package main - THE ORACLE'S ANTRE
// The Timeguru Service: Living timeline API for the Unheaded Kingdom
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
	"unheaded/pkg/discovery"
	"unheaded/pkg/logagg"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/timeguru/internal/api"
	"unheaded/services/timeguru/internal/parser"
	"unheaded/services/timeguru/internal/storage"
	tsync "unheaded/services/timeguru/internal/sync"
)

const (
	defaultPort         = "19000"
	defaultWotanAddr   = "localhost:18000"  // HTTP control plane
	defaultDBPath       = "./data/timeguru.db"
	defaultTimelinePath = "./references/timeline.md"
	shutdownTimeout     = 30 * time.Second
	fileWatchInterval   = 5 * time.Second
	wotanTopic         = "timeline.updates"
	wotanDisplayName   = "timeguru-service"
)

// traceIDCounter avoids collision risk of UnixNano()-only IDs under high throughput
var traceIDCounter int64

// Config holds service configuration
type Config struct {
	Port           string
	WotanAddr     string // HTTP control plane (subscribe, publish)
	WotanGRPCAddr string // gRPC data plane (streaming) — preferred for perf
	DBPath         string
	TimelinePath   string
	SyncDir        string // directory for synced timeline files (JSON/TOML/YAML/MD)
}

func main() {
	// Configure zerolog — structured JSON in prod, console for dev
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Str("service", "timeguru").Logger()

	log.Info().Msg("THE ORACLE'S ANTRE AWAKENS — Timeguru Service v1.0.0")

	// Load config from environment with defensive defaults
	config := loadConfig()

	log.Info().
		Str("port", config.Port).
		Str("wotan_http", config.WotanAddr).
		Str("wotan_grpc", config.WotanGRPCAddr).
		Str("db_path", config.DBPath).
		Str("timeline_path", config.TimelinePath).
		Str("sync_dir", config.SyncDir).
		Msg("configuration loaded")

	// Initialize storage
	store, err := storage.NewStore(config.DBPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize storage")
	}
	defer store.Close()
	log.Info().Msg("storage initialized (Crystal Grotto connected)")

	// Try to load timeline from markdown file on startup
	if config.TimelinePath != "" {
		if err := loadTimelineFromFile(config.TimelinePath, store); err != nil {
			log.Warn().Err(err).Str("path", config.TimelinePath).Msg("could not load timeline.md, starting with empty timeline")
		} else {
			log.Info().Msg("timeline loaded from markdown file")
		}
	}

	// Initialize transport config and health server
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	if config.WotanAddr != "" {
		transportCfg.WotanHTTPAddr = config.WotanAddr
	}
	if config.WotanGRPCAddr != "" {
		transportCfg.WotanGRPCAddr = config.WotanGRPCAddr
	}
	healthSrv := transport.NewHealthServer("timeguru")

	// Initialize wotan client using transport config addresses
	var wotan *wotanClient.Client
	wotan, err = initWotan(transportCfg.WotanHTTPAddr, transportCfg.WotanGRPCAddr)
	if err != nil {
		log.Warn().Err(err).Msg("Fae Chamber connection failed, continuing without Wotan integration")
		wotan = nil
		healthSrv.SetGRPCStatus(false)
	} else {
		defer wotan.Close()
		log.Info().Msg("Fae Chamber connected (Wotan online)")
	}

	// Log aggregation publisher — forwards structured logs to Wotan
	logPublisher := logagg.NewPublisher("timeguru", nil) // nil transport — publisher disabled until transport.Connect() is used
	log.Logger = log.Logger.Hook(logPublisher)

	// Initialize HTTP handler
	handler := api.NewHandler(store)

	// Initialize file syncer if sync directory is configured
	if config.SyncDir != "" {
		syncer, err := tsync.NewSyncer(config.SyncDir)
		if err != nil {
			log.Warn().Err(err).Msg("sync setup failed")
		} else {
			handler.SetSyncer(syncer)
			log.Info().Str("dir", config.SyncDir).Msg("file sync enabled (JSON/TOML/YAML/MD)")

			// Initial sync from current timeline state
			if tl, err := store.GetTimeline(context.Background()); err == nil {
				result := syncer.Sync(tl)
				tsync.LogSyncResult(result)
			}
		}
	}

	// Setup HTTP router with all endpoints
	mux := http.NewServeMux()

	// Health & metrics — transport readiness endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if healthSrv.Status() == transport.StatusDown {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"ready":false}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ready":true}`))
	})
	mux.HandleFunc("/health", handler.HandleHealth)

	// Timeline endpoints (multiple formats: JSON, YAML, TOML, Markdown)
	mux.HandleFunc("/timeline", handler.HandleGetTimelineWithFormat)
	mux.HandleFunc("/api/v1/timeline", handler.HandleGetTimelineWithFormat)

	// Sync & import endpoints
	mux.HandleFunc("/api/v1/timeline/sync", handler.HandleSync)
	mux.HandleFunc("/api/v1/timeline/import", handler.HandleImport)

	// Kanban tasks endpoint - transforms timeline data for kanban frontend
	mux.HandleFunc("/api/v1/timeline/tasks", handler.HandleGetTasks)

	// SSE stream endpoint - real-time timeline updates
	mux.HandleFunc("/api/v1/timeline/stream", handler.HandleTimelineStream)

	// Milestones endpoints
	mux.HandleFunc("/milestones", handler.HandleGetMilestones)
	mux.HandleFunc("/api/v1/milestones", handler.HandleGetMilestones)
	mux.HandleFunc("/milestones/", handleMilestoneRoutes(handler))
	mux.HandleFunc("/api/v1/milestones/", handleMilestoneRoutes(handler))

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("timeguru")
	var srvHandler http.Handler = mux
	srvHandler = auth.WrapHandler(srvHandler, auth.SetupMiddleware(authCfg))

	// HTTP server with defensive timeouts
	srv := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      srvHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Graceful shutdown setup
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Service discovery registration (best-effort, nil conn until transport.Connect)
	discovery.SetupServiceDiscovery(ctx, nil, "timeguru", 19000)

	// Start HTTP server in goroutine
	go func() {
		log.Info().Str("addr", ":"+config.Port).Msg("HTTP server listening")
		log.Info().Msg("endpoints: /health, /timeline, /milestones, /api/v1/timeline/{sync,import,tasks,stream}")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	// Start file watcher for timeline.md auto-reload + sync
	if config.TimelinePath != "" {
		go watchTimelineFile(ctx, config.TimelinePath, store, wotan, handler)
	}

	// Start wotan message listener if connected
	if wotan != nil {
		go listenForMessages(ctx, wotan)
		go listenForAlerts(ctx, wotan)
	}

	// Wait for shutdown signal
	<-ctx.Done()
	log.Info().Msg("shutdown signal received")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	log.Info().Msg("The Oracle's Antre sleeps")
}

// loadConfig loads configuration from environment variables with defensive defaults
func loadConfig() Config {
	config := Config{
		Port:           getEnv("PORT", defaultPort),
		WotanAddr:     getEnv("WOTAN_ADDR", defaultWotanAddr),
		WotanGRPCAddr: getEnv("WOTAN_GRPC_ADDR", "localhost:18001"),
		DBPath:         getEnv("DB_PATH", defaultDBPath),
		TimelinePath:   getEnv("TIMELINE_PATH", defaultTimelinePath),
		SyncDir:        os.Getenv("SYNC_DIR"), // empty = disabled
	}

	// Defensive: validate all paths
	if config.Port == "" {
		config.Port = defaultPort
	}
	if config.WotanAddr == "" {
		config.WotanAddr = defaultWotanAddr
	}
	if config.DBPath == "" {
		config.DBPath = defaultDBPath
	}

	// Default sync dir to the directory containing timeline.md
	if config.SyncDir == "" && config.TimelinePath != "" {
		config.SyncDir = filepath.Dir(config.TimelinePath)
	}

	return config
}

// getEnv retrieves environment variable with fallback
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// loadTimelineFromFile parses timeline.md and stores it
func loadTimelineFromFile(filePath string, store *storage.Store) error {
	p, err := parser.NewMarkdownParser(filePath)
	if err != nil {
		return fmt.Errorf("create parser: %w", err)
	}

	tl, err := p.Parse()
	if err != nil {
		return fmt.Errorf("parse timeline: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.SaveTimeline(ctx, tl); err != nil {
		return fmt.Errorf("save timeline: %w", err)
	}

	log.Info().
		Int("phases", len(tl.Phases)).
		Int("milestones", len(tl.Milestones)).
		Str("file", filePath).
		Msg("loaded timeline from file")

	return nil
}

// watchTimelineFile watches for changes to timeline.md and reloads + syncs
func watchTimelineFile(ctx context.Context, filePath string, store *storage.Store, wotan *wotanClient.Client, handler *api.Handler) {
	watcher, err := parser.NewFileWatcher(filePath)
	if err != nil {
		log.Error().Err(err).Msg("file watcher setup failed")
		return
	}

	log.Info().Str("file", filePath).Dur("interval", fileWatchInterval).Msg("watching for changes")

	ticker := time.NewTicker(fileWatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("file watcher stopped")
			return
		case <-ticker.C:
			changed, err := watcher.HasChanged()
			if err != nil {
				log.Error().Err(err).Msg("file watch error")
				continue
			}

			if changed {
				log.Info().Msg("timeline file changed, reloading")

				tl, err := watcher.Parse()
				if err != nil {
					log.Error().Err(err).Msg("parse error on reload")
					continue
				}

				saveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				if err := store.SaveTimeline(saveCtx, tl); err != nil {
					log.Error().Err(err).Msg("save error on reload")
					cancel()
					continue
				}
				cancel()

				log.Info().
					Int("phases", len(tl.Phases)).
					Int("milestones", len(tl.Milestones)).
					Msg("timeline reloaded")

				// Auto-sync to all format mirrors (JSON/TOML/YAML/MD)
				handler.AutoSync(tl)

				// Publish update event to Wotan if connected
				if wotan != nil {
					go publishTimelineUpdate(wotan, "timeline_reloaded")
				}
			}
		}
	}
}

// handleMilestoneRoutes creates a handler for /milestones/:id routes
func handleMilestoneRoutes(handler *api.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse URL path segments — never use filepath on URL paths (traversal risk)
		// Expected: /milestones/{id}/update
		trimmed := strings.TrimPrefix(r.URL.Path, "/milestones/")
		if trimmed == "" || trimmed == r.URL.Path {
			http.Error(w, "milestone ID required", http.StatusBadRequest)
			return
		}

		parts := strings.SplitN(trimmed, "/", 2)
		milestoneID := parts[0]
		if milestoneID == "" {
			http.Error(w, "milestone ID required", http.StatusBadRequest)
			return
		}

		// /milestones/{id}/update
		if len(parts) == 2 && parts[1] == "update" {
			handler.HandleUpdateMilestone(w, r, milestoneID)
			return
		}

		http.NotFound(w, r)
	}
}

// initWotan initializes wotan client connection with dual transport
// HTTP for control plane (subscribe/publish), gRPC for data plane (streaming)
func initWotan(httpAddr, grpcAddr string) (*wotanClient.Client, error) {
	var client *wotanClient.Client
	var err error
	if grpcAddr != "" {
		client, err = wotanClient.NewClientWithGRPC(httpAddr, grpcAddr)
		if err != nil {
			return nil, fmt.Errorf("create dual-transport client: %w", err)
		}
		log.Info().Str("http", httpAddr).Str("grpc", grpcAddr).Msg("Wotan client created (dual transport)")
	} else {
		client, err = wotanClient.NewClient(httpAddr)
		if err != nil {
			return nil, fmt.Errorf("create HTTP client: %w", err)
		}
		log.Info().Str("http", httpAddr).Msg("Wotan client created (HTTP-only)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Subscribe to alerts.critical (required by CLAUDE.md)
	alertSub, err := client.Subscribe(ctx, "alerts.critical", wotanDisplayName)
	if err != nil {
		log.Warn().Err(err).Msg("failed to subscribe to alerts.critical")
	} else {
		log.Info().Str("status", alertSub.Status).Msg("subscribed to alerts.critical")
	}

	// Subscribe to timeline updates topic
	subscriber, err := client.Subscribe(ctx, wotanTopic, wotanDisplayName)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("subscribe to %s: %w", wotanTopic, err)
	}

	log.Info().Str("topic", wotanTopic).Str("status", subscriber.Status).Msg("subscribed to topic")

	return client, nil
}

// publishTimelineUpdate publishes an update event to Wotan with trace_id
func publishTimelineUpdate(client *wotanClient.Client, eventType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Generate trace_id using atomic counter + timestamp to avoid collisions
	counter := atomic.AddInt64(&traceIDCounter, 1)
	traceID := fmt.Sprintf("timeguru-%d-%d", time.Now().Unix(), counter)

	payload := fmt.Sprintf(`{"event":"%s","timestamp":"%s","timestamp_ms":%d,"source":"timeguru","service":"timeguru","trace_id":"%s"}`,
		eventType, time.Now().Format(time.RFC3339), time.Now().UnixMilli(), traceID)

	err := client.Publish(ctx, wotanTopic, []byte(payload))
	if err != nil {
		log.Error().Err(err).Str("event", eventType).Msg("failed to publish update event")
		return
	}

	log.Info().Str("event", eventType).Str("trace_id", traceID).Msg("published event")
}

// listenForMessages listens for wotan messages
func listenForMessages(ctx context.Context, client *wotanClient.Client) {
	log.Info().Str("topic", wotanTopic).Msg("listening for messages")

	// Stream messages
	msgCh, err := client.StreamMessages(ctx, wotanTopic)
	if err != nil {
		log.Error().Err(err).Msg("failed to stream messages")
		return
	}

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("message listener stopped")
			return
		case msg, ok := <-msgCh:
			if !ok {
				log.Info().Msg("message channel closed")
				return
			}
			log.Info().
				Str("message_id", msg.MessageID).
				Int64("seq", msg.Seq).
				Str("topic", msg.Topic).
				Str("sender", msg.SenderID).
				Msg("received message (processing deferred)")
		}
	}
}

// listenForAlerts listens for critical alerts from Wotan
func listenForAlerts(ctx context.Context, client *wotanClient.Client) {
	log.Info().Msg("listening for critical alerts")

	msgCh, err := client.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		log.Error().Err(err).Msg("failed to stream alerts")
		return
	}

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("alert listener stopped")
			return
		case msg, ok := <-msgCh:
			if !ok {
				log.Info().Msg("alert channel closed")
				return
			}
			log.Warn().
				Str("message_id", msg.MessageID).
				Int64("seq", msg.Seq).
				Str("payload", msg.Payload).
				Msg("CRITICAL ALERT received")
		}
	}
}
