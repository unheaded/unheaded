package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	busboyClient "unheaded/pkg/busboy-client"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

type Timeline struct {
	Title  string  `json:"title"`
	Phases []Phase `json:"phases"`
}

type Phase struct {
	Name  string `json:"name"`
	Tasks []Task `json:"tasks"`
}

type Task struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

var (
	timeline Timeline
)

func main() {
	listenAddress := os.Getenv("LISTEN_ADDRESS")
	if listenAddress == "" {
		listenAddress = ":8000"
	}

	busboyAddress := os.Getenv("BUSBOY_ADDRESS")
	if busboyAddress == "" {
		busboyAddress = "localhost:8080"
	}

	timelinePath := os.Getenv("TIMELINE_PATH")
	if timelinePath == "" {
		timelinePath = "/opt/unheaded/references/timeline.md"
	}

	// Load the timeline
	if err := loadTimeline(timelinePath); err != nil {
		log.Printf("Warning: Failed to load timeline from %s: %v (starting with empty timeline)", timelinePath, err)
	}

	// Connect to Busboy
	client, err := busboyClient.NewClient(busboyAddress)
	if err != nil {
		log.Printf("Warning: Failed to create busboy client: %v (running without pub/sub)", err)
	}

	// Start HTTP server (REST API)
	router := mux.NewRouter()
	router.HandleFunc("/health", healthHandler).Methods("GET")
	router.HandleFunc("/ready", readyHandler).Methods("GET")
	router.HandleFunc("/metrics", metricsHandler).Methods("GET")
	router.HandleFunc("/api/v1/timeline", timelineHandler).Methods("GET")
	router.HandleFunc("/timeline", timelineHandler).Methods("GET")

	httpServer := &http.Server{
		Addr:         listenAddress,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("Timeguru starting on %s", listenAddress)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Subscribe to timeline updates via Busboy (if connected)
	if client != nil {
		ctx := context.Background()
		_, subErr := client.Subscribe(ctx, "timeline.updates", "timeguru")
		if subErr != nil {
			log.Printf("Warning: Failed to subscribe to timeline.updates: %v", subErr)
		} else {
			go streamTimelineUpdates(ctx, client)
		}
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	if client != nil {
		client.Close()
	}

	log.Println("Timeguru stopped gracefully")
}

func loadTimeline(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &timeline)
}

func streamTimelineUpdates(ctx context.Context, client *busboyClient.Client) {
	ch, err := client.StreamMessages(ctx, "timeline.updates")
	if err != nil {
		log.Printf("Failed to stream timeline updates: %v", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			updateTimeline([]byte(msg.Payload))
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
}

func timelineHandler(w http.ResponseWriter, r *http.Request) {
	data, err := json.Marshal(map[string]interface{}{
		"timeline": timeline,
	})
	if err != nil {
		http.Error(w, "Failed to marshal timeline", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func updateTimeline(data []byte) {
	var newTimeline Timeline
	if err := json.Unmarshal(data, &newTimeline); err != nil {
		log.Printf("Failed to unmarshal timeline update: %v", err)
		return
	}
	timeline = newTimeline
	log.Println("Timeline updated")
}
