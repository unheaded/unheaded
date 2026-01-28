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

	busboyClient "github.com/unheaded/unheaded/pkg/busboy-client"
	"github.com/unheaded/unheaded/services/timeguru/internal/api"
	"github.com/unheaded/unheaded/services/timeguru/internal/storage"
)

const (
	defaultPort        = "8000"
	defaultBusboyAddr  = "localhost:9090"
	defaultDBPath      = "/opt/unheaded/data/timeguru.db"
	shutdownTimeout    = 30 * time.Second
	busboyTopic        = "timeline.updates"
	busboyDisplayName  = "timeguru-service"
)

// Config holds service configuration
type Config struct {
	Port       string
	BusboyAddr string
	DBPath     string
}

func main() {
	// Load config from environment with defensive defaults
	config := loadConfig()

	log.Printf("[timeguru] starting service on port %s", config.Port)
	log.Printf("[timeguru] busboy address: %s", config.BusboyAddr)
	log.Printf("[timeguru] database path: %s", config.DBPath)

	// Initialize storage
	store, err := storage.NewStore(config.DBPath)
	if err != nil {
		log.Fatalf("[timeguru] failed to initialize storage: %v", err)
	}
	defer store.Close()

	log.Println("[timeguru] storage initialized")

	// Initialize busboy client
	busboyClient, err := initBusboy(config.BusboyAddr)
	if err != nil {
		log.Printf("[timeguru] WARNING: busboy connection failed: %v", err)
		log.Println("[timeguru] continuing without busboy integration")
		busboyClient = nil
	} else {
		defer busboyClient.Close()
		log.Println("[timeguru] busboy connected")
	}

	// Initialize HTTP handler
	handler := api.NewHandler(store)

	// Setup HTTP router
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.HandleHealth)
	mux.HandleFunc("/timeline", handler.HandleGetTimeline)
	mux.HandleFunc("/milestones", handler.HandleGetMilestones)
	mux.HandleFunc("/milestones/", func(w http.ResponseWriter, r *http.Request) {
		// Extract milestone ID from path: /milestones/:id/update
		path := r.URL.Path
		if len(path) < len("/milestones/") {
			http.Error(w, "milestone ID required", http.StatusBadRequest)
			return
		}

		// Parse ID from /milestones/:id/update
		pathParts := filepath.Base(path)
		if pathParts == "update" {
			// Get parent directory which is the ID
			milestoneID := filepath.Base(filepath.Dir(path))
			if milestoneID == "" || milestoneID == "milestones" {
				http.Error(w, "milestone ID required", http.StatusBadRequest)
				return
			}
			handler.HandleUpdateMilestone(w, r, milestoneID)
			return
		}

		http.NotFound(w, r)
	})

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
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[timeguru] HTTP server failed: %v", err)
		}
	}()

	// Start busboy message listener if connected
	if busboyClient != nil {
		go listenForMessages(ctx, busboyClient)
	}

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("[timeguru] shutdown signal received")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[timeguru] HTTP server shutdown error: %v", err)
	}

	log.Println("[timeguru] service stopped")
}

// loadConfig loads configuration from environment variables with defensive defaults
func loadConfig() Config {
	config := Config{
		Port:       getEnv("PORT", defaultPort),
		BusboyAddr: getEnv("BUSBOY_ADDR", defaultBusboyAddr),
		DBPath:     getEnv("DB_PATH", defaultDBPath),
	}

	// Defensive: validate port
	if config.Port == "" {
		config.Port = defaultPort
	}

	// Defensive: validate busboy address
	if config.BusboyAddr == "" {
		config.BusboyAddr = defaultBusboyAddr
	}

	// Defensive: validate DB path
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

	log.Printf("[timeguru] subscribed to topic %q (status: %s)", busboyTopic, subscriber.Status)

	return client, nil
}

// listenForMessages listens for busboy messages (currently logs only)
func listenForMessages(ctx context.Context, client *busboyClient.Client) {
	// In production, this would process timeline update events
	// For now, we just log that we're listening

	log.Printf("[timeguru] listening for messages on topic %q", busboyTopic)

	// Stream messages
	msgCh, err := client.StreamMessages(ctx, busboyTopic)
	if err != nil {
		log.Printf("[timeguru] failed to stream messages: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("[timeguru] message listener stopped")
			return
		case msg, ok := <-msgCh:
			if !ok {
				log.Println("[timeguru] message channel closed")
				return
			}
			log.Printf("[timeguru] received message: %s (seq=%d)", msg.MessageID, msg.Seq)
			// TODO: Process timeline update events
		}
	}
}
