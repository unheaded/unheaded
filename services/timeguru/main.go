// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/ports"
	wotanClient "unheaded/pkg/wotan-client"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
		listenAddress = ports.DefaultAddr(ports.Timeguru)
	}

	wotanAddress := os.Getenv("WOTAN_ADDRESS")
	if wotanAddress == "" {
		wotanAddress = "localhost:18000"
	}

	timelinePath := os.Getenv("TIMELINE_PATH")
	if timelinePath == "" {
		timelinePath = "/opt/unheaded/references/timeline.md"
	}

	// Configure zerolog
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// Load the timeline
	if err := loadTimeline(timelinePath); err != nil {
		log.Warn().Err(err).Str("path", timelinePath).Msg("failed to load timeline, starting with empty timeline")
	}

	// Connect to Wotan
	client, err := wotanClient.NewClient(wotanAddress)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create wotan client, running without pub/sub")
	}

	// Start HTTP server (REST API)
	router := mux.NewRouter()
	router.HandleFunc("/health", healthHandler).Methods("GET")
	router.HandleFunc("/ready", readyHandler).Methods("GET")
	router.HandleFunc("/metrics", metricsHandler).Methods("GET")
	router.HandleFunc("/api/v1/timeline", timelineHandler).Methods("GET")
	router.HandleFunc("/timeline", timelineHandler).Methods("GET")

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("timeguru")
	var srvHandler http.Handler = router
	srvHandler = auth.WrapHandler(srvHandler, auth.SetupMiddleware(authCfg))

	httpServer := &http.Server{
		Addr:           listenAddress,
		Handler:        srvHandler,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	go func() {
		log.Info().Str("addr", listenAddress).Msg("timeguru starting")
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	// Subscribe to timeline updates via Wotan (if connected)
	if client != nil {
		ctx := context.Background()
		_, subErr := client.Subscribe(ctx, "timeline.updates", "timeguru")
		if subErr != nil {
			log.Warn().Err(subErr).Msg("failed to subscribe to timeline.updates")
		} else {
			go streamTimelineUpdates(ctx, client)
		}
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Info().Msg("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	if client != nil {
		_ = client.Close()
	}

	log.Info().Msg("timeguru stopped gracefully")
}

func loadTimeline(path string) error {
	// path is operator-controlled (config-supplied or default
	// references/timeline.md), not user-supplied per request.
	data, err := os.ReadFile(path) // #nosec G703,G304 -- operator-controlled config path
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &timeline)
}

func streamTimelineUpdates(ctx context.Context, client *wotanClient.Client) {
	ch, err := client.StreamMessages(ctx, "timeline.updates")
	if err != nil {
		log.Error().Err(err).Msg("failed to stream timeline updates")
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
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
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
	_, _ = w.Write(data)
}

func updateTimeline(data []byte) {
	var newTimeline Timeline
	if err := json.Unmarshal(data, &newTimeline); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal timeline update")
		return
	}
	timeline = newTimeline
	log.Info().Msg("timeline updated")
}
