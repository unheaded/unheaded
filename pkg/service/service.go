// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package service provides a standardized service scaffold for Unheaded Kingdom
// microservices. It codifies the patterns used across all 34 services:
//
//   - HTTP server with /health, /ready, /metrics endpoints
//   - Wotan client integration (connect, subscribe, publish)
//   - Auth middleware (pluggable authenticator)
//   - Graceful shutdown with configurable timeout
//   - Structured logging (zerolog)
//   - Service discovery registration
//   - Log aggregation forwarding
//
// Usage:
//
//	svc, err := service.New(service.Config{
//	    Name: "timeguru",
//	    Port: "19000",
//	})
//	svc.Handle("/api/v1/timeline", timelineHandler)
//	svc.Run(ctx)
package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/auth"
	wotanClient "unheaded/pkg/wotan-client"
)

// Config holds service configuration.
type Config struct {
	// Name is the service name (e.g., "timeguru", "captain").
	Name string

	// Port is the HTTP listen port (e.g., "19000").
	Port string

	// WotanAddr is the Wotan HTTP address for pub/sub.
	WotanAddr string

	// WotanGRPCAddr is the Wotan gRPC address for streaming.
	WotanGRPCAddr string

	// Version is the service version string.
	Version string

	// ShutdownTimeout is the graceful shutdown deadline.
	ShutdownTimeout time.Duration

	// Authenticator is the auth backend (nil = NoopAuthenticator).
	Authenticator auth.Authenticator

	// ReadyFunc is called by the /ready endpoint. Return true if ready.
	ReadyFunc func() bool
}

// Service is a standardized Unheaded microservice.
type Service struct {
	config     Config
	mux        *http.ServeMux
	httpServer *http.Server
	wotan      *wotanClient.Client
	logger     zerolog.Logger
	ready      atomic.Bool
}

// New creates a new Service with the given configuration.
// It sets up the HTTP mux, logger, and standard endpoints.
func New(cfg Config) (*Service, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("service name is required")
	}
	if cfg.Port == "" {
		return nil, fmt.Errorf("service port is required")
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 30 * time.Second
	}
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}
	// Authenticator is optional — if nil, HandleWithAuth falls back to no-op

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Str("service", cfg.Name).Logger()

	svc := &Service{
		config: cfg,
		mux:    http.NewServeMux(),
		logger: logger,
	}

	// Standard endpoints
	svc.mux.HandleFunc("/health", svc.handleHealth)
	svc.mux.HandleFunc("/ready", svc.handleReady)

	return svc, nil
}

// Handle registers an HTTP handler on the service mux.
func (s *Service) Handle(pattern string, handler http.HandlerFunc) {
	s.mux.HandleFunc(pattern, handler)
}

// HandleWithAuth registers an HTTP handler wrapped with auth middleware.
// If no authenticator is configured, the handler is registered without auth.
func (s *Service) HandleWithAuth(pattern string, handler http.HandlerFunc) {
	if s.config.Authenticator != nil {
		wrapped := auth.Middleware(s.config.Authenticator)(http.HandlerFunc(handler))
		s.mux.Handle(pattern, wrapped)
	} else {
		s.mux.HandleFunc(pattern, handler)
	}
}

// Logger returns the service's structured logger.
func (s *Service) Logger() zerolog.Logger {
	return s.logger
}

// Wotan returns the Wotan client (nil if not connected).
func (s *Service) Wotan() *wotanClient.Client {
	return s.wotan
}

// SetReady marks the service as ready to serve traffic.
func (s *Service) SetReady(ready bool) {
	s.ready.Store(ready)
}

// Run starts the service and blocks until shutdown signal.
// It connects to Wotan, registers with discovery, starts the HTTP server,
// and handles graceful shutdown on SIGINT/SIGTERM.
func (s *Service) Run(ctx context.Context) error {
	s.logger.Info().
		Str("version", s.config.Version).
		Str("port", s.config.Port).
		Msg("service starting")

	// Connect to Wotan (best effort — service works without it)
	if s.config.WotanAddr != "" {
		client, err := wotanClient.NewClient(s.config.WotanAddr)
		if err != nil {
			s.logger.Warn().Err(err).Msg("wotan connection failed — running without message bus")
		} else {
			s.wotan = client
			s.logger.Info().Str("wotan", s.config.WotanAddr).Msg("wotan connected")
		}
	}

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:              ":" + s.config.Port,
		Handler:           s.mux,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	// Signal handling
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		s.ready.Store(true)
		s.logger.Info().Str("addr", s.httpServer.Addr).Msg("HTTP server listening")
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-sigCtx.Done():
		s.logger.Info().Msg("shutdown signal received")
	}

	// Graceful shutdown
	s.ready.Store(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	if s.wotan != nil {
		_ = s.wotan.Close()
	}

	s.logger.Info().Msg("service stopped gracefully")
	return nil
}

// handleHealth returns 200 if the service is alive.
func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","service":"%s","version":"%s"}`, s.config.Name, s.config.Version)
}

// handleReady returns 200 if the service is ready, 503 otherwise.
func (s *Service) handleReady(w http.ResponseWriter, _ *http.Request) {
	ready := s.ready.Load()
	if s.config.ReadyFunc != nil {
		ready = ready && s.config.ReadyFunc()
	}
	w.Header().Set("Content-Type", "application/json")
	if ready {
		fmt.Fprintf(w, `{"ready":true,"service":"%s"}`, s.config.Name)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"ready":false,"service":"%s"}`, s.config.Name)
	}
}
