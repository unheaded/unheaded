// Package main - THE ORACLE'S ANTRE
// The Timeguru Service: Living timeline API for the Unheaded Kingdom
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
	"unheaded/services/timeguru/internal/api"
	"unheaded/services/timeguru/internal/parser"
	"unheaded/services/timeguru/internal/storage"
	tsync "unheaded/services/timeguru/internal/sync"
)

const (
	defaultPort         = "8000"
	defaultBusboyAddr   = "localhost:8080"  // HTTP control plane (not gRPC 9090)
	defaultDBPath       = "./data/timeguru.db"
	defaultTimelinePath = "./references/timeline.md"
	shutdownTimeout     = 30 * time.Second
	fileWatchInterval   = 5 * time.Second
	busboyTopic         = "timeline.updates"
	busboyDisplayName   = "timeguru-service"
)

// Config holds service configuration
type Config struct {
	Port           string
	BusboyAddr     string // HTTP control plane (subscribe, publish)
	BusboyGRPCAddr string // gRPC data plane (streaming) — preferred for perf
	DBPath         string
	TimelinePath   string
	SyncDir        string // directory for synced timeline files (JSON/TOML/YAML/MD)
}

func main() {
	log.Println("[timeguru] ═══════════════════════════════════════════════")
	log.Println("[timeguru]     THE ORACLE'S ANTRE AWAKENS")
	log.Println("[timeguru]     Timeguru Service v1.0.0")
	log.Println("[timeguru] ═══════════════════════════════════════════════")

	// Load config from environment with defensive defaults
	config := loadConfig()

	log.Printf("[timeguru] Configuration:")
	log.Printf("[timeguru]   Port: %s", config.Port)
	log.Printf("[timeguru]   Busboy HTTP: %s", config.BusboyAddr)
	log.Printf("[timeguru]   Busboy gRPC: %s", config.BusboyGRPCAddr)
	log.Printf("[timeguru]   Database: %s", config.DBPath)
	log.Printf("[timeguru]   Timeline: %s", config.TimelinePath)
	log.Printf("[timeguru]   SyncDir: %s", config.SyncDir)

	// Initialize storage
	store, err := storage.NewStore(config.DBPath)
	if err != nil {
		log.Fatalf("[timeguru] failed to initialize storage: %v", err)
	}
	defer store.Close()
	log.Println("[timeguru] Storage initialized (Crystal Grotto connected)")

	// Try to load timeline from markdown file on startup
	if config.TimelinePath != "" {
		if err := loadTimelineFromFile(config.TimelinePath, store); err != nil {
			log.Printf("[timeguru] WARNING: could not load timeline.md: %v", err)
			log.Println("[timeguru] Service will start with empty timeline")
		} else {
			log.Println("[timeguru] Timeline loaded from markdown file")
		}
	}

	// Initialize busboy client
	var busboy *busboyClient.Client
	busboy, err = initBusboy(config.BusboyAddr, config.BusboyGRPCAddr)
	if err != nil {
		log.Printf("[timeguru] WARNING: Fae Chamber connection failed: %v", err)
		log.Println("[timeguru] Continuing without Busboy integration")
		busboy = nil
	} else {
		defer busboy.Close()
		log.Println("[timeguru] Fae Chamber connected (Busboy online)")
	}

	// Initialize HTTP handler
	handler := api.NewHandler(store)

	// Initialize file syncer if sync directory is configured
	if config.SyncDir != "" {
		syncer, err := tsync.NewSyncer(config.SyncDir)
		if err != nil {
			log.Printf("[timeguru] WARNING: sync setup failed: %v", err)
		} else {
			handler.SetSyncer(syncer)
			log.Printf("[timeguru] File sync enabled → %s (JSON/TOML/YAML/MD)", config.SyncDir)

			// Initial sync from current timeline state
			if tl, err := store.GetTimeline(context.Background()); err == nil {
				result := syncer.Sync(tl)
				tsync.LogSyncResult(result)
			}
		}
	}

	// Setup HTTP router with all endpoints
	mux := http.NewServeMux()

	// Health & metrics
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

	// HTTP server with defensive timeouts
	srv := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown setup
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start HTTP server in goroutine
	go func() {
		log.Printf("[timeguru] HTTP server listening on :%s", config.Port)
		log.Println("[timeguru] Endpoints available:")
		log.Println("[timeguru]   GET  /health                   - Health check")
		log.Println("[timeguru]   GET  /timeline?format=         - Timeline (json/yaml/toml/md)")
		log.Println("[timeguru]   GET  /milestones               - All milestones")
		log.Println("[timeguru]   POST /milestones/:id/update    - Update milestone")
		log.Println("[timeguru]   POST /api/v1/timeline/sync     - Sync to JSON/TOML/YAML/MD files")
		log.Println("[timeguru]   POST /api/v1/timeline/import   - Import from JSON/YAML/TOML")
		log.Println("[timeguru]   GET  /api/v1/timeline/tasks    - Kanban tasks")
		log.Println("[timeguru]   GET  /api/v1/timeline/stream   - SSE real-time updates")
		log.Println("[timeguru] ═══════════════════════════════════════════════")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[timeguru] HTTP server failed: %v", err)
		}
	}()

	// Start file watcher for timeline.md auto-reload + sync
	if config.TimelinePath != "" {
		go watchTimelineFile(ctx, config.TimelinePath, store, busboy, handler)
	}

	// Start busboy message listener if connected
	if busboy != nil {
		go listenForMessages(ctx, busboy)
		go listenForAlerts(ctx, busboy)
	}

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("[timeguru] ═══════════════════════════════════════════════")
	log.Println("[timeguru] Shutdown signal received")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[timeguru] HTTP server shutdown error: %v", err)
	}

	log.Println("[timeguru] The Oracle's Antre sleeps...")
	log.Println("[timeguru] ═══════════════════════════════════════════════")
}

// loadConfig loads configuration from environment variables with defensive defaults
func loadConfig() Config {
	config := Config{
		Port:           getEnv("PORT", defaultPort),
		BusboyAddr:     getEnv("BUSBOY_ADDR", defaultBusboyAddr),
		BusboyGRPCAddr: getEnv("BUSBOY_GRPC_ADDR", "localhost:9090"),
		DBPath:         getEnv("DB_PATH", defaultDBPath),
		TimelinePath:   getEnv("TIMELINE_PATH", defaultTimelinePath),
		SyncDir:        os.Getenv("SYNC_DIR"), // empty = disabled
	}

	// Defensive: validate all paths
	if config.Port == "" {
		config.Port = defaultPort
	}
	if config.BusboyAddr == "" {
		config.BusboyAddr = defaultBusboyAddr
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

	log.Printf("[timeguru] Loaded %d phases, %d milestones from %s",
		len(tl.Phases), len(tl.Milestones), filePath)

	return nil
}

// watchTimelineFile watches for changes to timeline.md and reloads + syncs
func watchTimelineFile(ctx context.Context, filePath string, store *storage.Store, busboy *busboyClient.Client, handler *api.Handler) {
	watcher, err := parser.NewFileWatcher(filePath)
	if err != nil {
		log.Printf("[timeguru] File watcher setup failed: %v", err)
		return
	}

	log.Printf("[timeguru] Watching %s for changes (interval: %v)", filePath, fileWatchInterval)

	ticker := time.NewTicker(fileWatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[timeguru] File watcher stopped")
			return
		case <-ticker.C:
			changed, err := watcher.HasChanged()
			if err != nil {
				log.Printf("[timeguru] File watch error: %v", err)
				continue
			}

			if changed {
				log.Println("[timeguru] Timeline file changed, reloading...")

				tl, err := watcher.Parse()
				if err != nil {
					log.Printf("[timeguru] Parse error on reload: %v", err)
					continue
				}

				saveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				if err := store.SaveTimeline(saveCtx, tl); err != nil {
					log.Printf("[timeguru] Save error on reload: %v", err)
					cancel()
					continue
				}
				cancel()

				log.Printf("[timeguru] Timeline reloaded: %d phases, %d milestones",
					len(tl.Phases), len(tl.Milestones))

				// Auto-sync to all format mirrors (JSON/TOML/YAML/MD)
				handler.AutoSync(tl)

				// Publish update event to Busboy if connected
				if busboy != nil {
					go publishTimelineUpdate(busboy, "timeline_reloaded")
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

// initBusboy initializes busboy client connection with dual transport
// HTTP for control plane (subscribe/publish), gRPC for data plane (streaming)
func initBusboy(httpAddr, grpcAddr string) (*busboyClient.Client, error) {
	var client *busboyClient.Client
	var err error
	if grpcAddr != "" {
		client, err = busboyClient.NewClientWithGRPC(httpAddr, grpcAddr)
		if err != nil {
			return nil, fmt.Errorf("create dual-transport client: %w", err)
		}
		log.Printf("[timeguru] Busboy client: HTTP=%s gRPC=%s (dual transport)", httpAddr, grpcAddr)
	} else {
		client, err = busboyClient.NewClient(httpAddr)
		if err != nil {
			return nil, fmt.Errorf("create HTTP client: %w", err)
		}
		log.Printf("[timeguru] Busboy client: HTTP=%s (HTTP-only)", httpAddr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Subscribe to alerts.critical (required by CLAUDE.md)
	alertSub, err := client.Subscribe(ctx, "alerts.critical", busboyDisplayName)
	if err != nil {
		log.Printf("[timeguru] WARNING: failed to subscribe to alerts.critical: %v", err)
	} else {
		log.Printf("[timeguru] Subscribed to alerts.critical (status: %s)", alertSub.Status)
	}

	// Subscribe to timeline updates topic
	subscriber, err := client.Subscribe(ctx, busboyTopic, busboyDisplayName)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("subscribe to %s: %w", busboyTopic, err)
	}

	log.Printf("[timeguru] Subscribed to topic %q (status: %s)", busboyTopic, subscriber.Status)

	return client, nil
}

// publishTimelineUpdate publishes an update event to Busboy with trace_id
func publishTimelineUpdate(client *busboyClient.Client, eventType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Generate trace_id for this event
	traceID := fmt.Sprintf("timeguru-%d", time.Now().UnixNano())

	payload := fmt.Sprintf(`{"event":"%s","timestamp":"%s","timestamp_ms":%d,"source":"timeguru","service":"timeguru","trace_id":"%s"}`,
		eventType, time.Now().Format(time.RFC3339), time.Now().UnixMilli(), traceID)

	err := client.Publish(ctx, busboyTopic, []byte(payload))
	if err != nil {
		log.Printf("[timeguru] Failed to publish update event: %v", err)
		return
	}

	log.Printf("[timeguru] Published event: %s (trace_id: %s)", eventType, traceID)
}

// listenForMessages listens for busboy messages
func listenForMessages(ctx context.Context, client *busboyClient.Client) {
	log.Printf("[timeguru] Listening for messages on topic %q", busboyTopic)

	// Stream messages
	msgCh, err := client.StreamMessages(ctx, busboyTopic)
	if err != nil {
		log.Printf("[timeguru] Failed to stream messages: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("[timeguru] Message listener stopped")
			return
		case msg, ok := <-msgCh:
			if !ok {
				log.Println("[timeguru] Message channel closed")
				return
			}
			log.Printf("[timeguru] Received message: id=%s seq=%d topic=%s sender=%s - processing deferred",
				msg.MessageID, msg.Seq, msg.Topic, msg.SenderID)
		}
	}
}

// listenForAlerts listens for critical alerts from Busboy
func listenForAlerts(ctx context.Context, client *busboyClient.Client) {
	log.Println("[timeguru] Listening for critical alerts")

	msgCh, err := client.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		log.Printf("[timeguru] Failed to stream alerts: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("[timeguru] Alert listener stopped")
			return
		case msg, ok := <-msgCh:
			if !ok {
				log.Println("[timeguru] Alert channel closed")
				return
			}
			log.Printf("[timeguru] CRITICAL ALERT received: %s (seq=%d) - payload: %s",
				msg.MessageID, msg.Seq, msg.Payload)
			// Could update timeline status or create milestone markers for critical events
		}
	}
}
