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
	"syscall"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
	"unheaded/services/timeguru/internal/api"
	"unheaded/services/timeguru/internal/parser"
	"unheaded/services/timeguru/internal/storage"
)

const (
	defaultPort         = "8000"
	defaultBusboyAddr   = "localhost:9090"
	defaultDBPath       = "/opt/unheaded/data/timeguru.db"
	defaultTimelinePath = "/opt/unheaded/data/timeline.md"
	shutdownTimeout     = 30 * time.Second
	fileWatchInterval   = 5 * time.Second
	busboyTopic         = "timeline.updates"
	busboyDisplayName   = "timeguru-service"
)

// Config holds service configuration
type Config struct {
	Port         string
	BusboyAddr   string
	DBPath       string
	TimelinePath string
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
	log.Printf("[timeguru]   Busboy: %s", config.BusboyAddr)
	log.Printf("[timeguru]   Database: %s", config.DBPath)
	log.Printf("[timeguru]   Timeline: %s", config.TimelinePath)

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
	busboy, err = initBusboy(config.BusboyAddr)
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

	// Setup HTTP router with all endpoints
	mux := http.NewServeMux()

	// Health & metrics
	mux.HandleFunc("/health", handler.HandleHealth)

	// Timeline endpoints (multiple formats)
	mux.HandleFunc("/timeline", handler.HandleGetTimelineWithFormat)
	mux.HandleFunc("/api/v1/timeline", handler.HandleGetTimelineWithFormat)

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
		log.Println("[timeguru]   GET  /health              - Health check")
		log.Println("[timeguru]   GET  /timeline            - Timeline (JSON/YAML/MD)")
		log.Println("[timeguru]   GET  /milestones          - All milestones")
		log.Println("[timeguru]   POST /milestones/:id/update - Update milestone")
		log.Println("[timeguru] ═══════════════════════════════════════════════")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[timeguru] HTTP server failed: %v", err)
		}
	}()

	// Start file watcher for timeline.md auto-reload
	if config.TimelinePath != "" {
		go watchTimelineFile(ctx, config.TimelinePath, store, busboy)
	}

	// Start busboy message listener if connected
	if busboy != nil {
		go listenForMessages(ctx, busboy)
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
		Port:         getEnv("PORT", defaultPort),
		BusboyAddr:   getEnv("BUSBOY_ADDR", defaultBusboyAddr),
		DBPath:       getEnv("DB_PATH", defaultDBPath),
		TimelinePath: getEnv("TIMELINE_PATH", defaultTimelinePath),
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

// watchTimelineFile watches for changes to timeline.md and reloads
func watchTimelineFile(ctx context.Context, filePath string, store *storage.Store, busboy *busboyClient.Client) {
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
		path := r.URL.Path
		if len(path) < len("/milestones/") {
			http.Error(w, "milestone ID required", http.StatusBadRequest)
			return
		}

		// Parse ID from /milestones/:id/update
		pathParts := filepath.Base(path)
		if pathParts == "update" {
			milestoneID := filepath.Base(filepath.Dir(path))
			if milestoneID == "" || milestoneID == "milestones" || milestoneID == "v1" {
				http.Error(w, "milestone ID required", http.StatusBadRequest)
				return
			}
			handler.HandleUpdateMilestone(w, r, milestoneID)
			return
		}

		http.NotFound(w, r)
	}
}

// initBusboy initializes busboy client connection
func initBusboy(addr string) (*busboyClient.Client, error) {
	client, err := busboyClient.NewClient(addr)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	// Subscribe to timeline updates topic
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	subscriber, err := client.Subscribe(ctx, busboyTopic, busboyDisplayName)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("subscribe to %s: %w", busboyTopic, err)
	}

	log.Printf("[timeguru] Subscribed to topic %q (status: %s)", busboyTopic, subscriber.Status)

	return client, nil
}

// publishTimelineUpdate publishes an update event to Busboy
func publishTimelineUpdate(client *busboyClient.Client, eventType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload := fmt.Sprintf(`{"event":"%s","timestamp":"%s","source":"timeguru"}`,
		eventType, time.Now().Format(time.RFC3339))

	err := client.Publish(ctx, busboyTopic, []byte(payload))
	if err != nil {
		log.Printf("[timeguru] Failed to publish update event: %v", err)
		return
	}

	log.Printf("[timeguru] Published event: %s", eventType)
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
			log.Printf("[timeguru] Received message: %s (seq=%d)", msg.MessageID, msg.Seq)
			// TODO: Process timeline update events from other services
		}
	}
}
