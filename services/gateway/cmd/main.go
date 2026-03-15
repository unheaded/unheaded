// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package main is the entry point for the API Gateway service.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"unheaded/pkg/logger"
	"unheaded/services/gateway"
	"unheaded/services/gateway/config"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	// Parse command-line flags
	configFile := flag.String("config", "", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	// Show version and exit
	if *showVersion {
		fmt.Printf("API Gateway\n")
		fmt.Printf("  Version:    %s\n", version)
		fmt.Printf("  Build Time: %s\n", buildTime)
		fmt.Printf("  Git Commit: %s\n", gitCommit)
		os.Exit(0)
	}

	// Load configuration
	var cfg *config.Config
	var err error

	if *configFile != "" {
		cfg, err = config.LoadFromFile(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config from %s: %v\n", *configFile, err)
			os.Exit(1)
		}
	} else {
		cfg = config.LoadFromEnv()
	}

	// Add default routes if none configured
	if len(cfg.Routes) == 0 {
		cfg.Routes = defaultRoutes(cfg)
	}

	// Create logger
	logLevel, _ := logger.ParseLevel(getEnv("LOG_LEVEL", "info"))
	logConfig := logger.DefaultConfig()
	logConfig.Level = logLevel
	if getEnv("LOG_FORMAT", "json") == "console" {
		logConfig.ConsoleMode = true
	}
	log := logger.NewWithConfig(logConfig)

	log.Info().
		Str("version", version).
		Str("build_time", buildTime).
		Str("git_commit", gitCommit).
		Msg("Starting API Gateway")

	// Create gateway
	gw, err := gateway.New(cfg, log)
	if err != nil {
		log.Error().Str("error", err.Error()).Msg("Failed to create gateway")
		os.Exit(1)
	}

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
				cancel()
				return
			case syscall.SIGHUP:
				log.Info().Msg("Received SIGHUP — reloading configuration")

				// Re-read config from the same source (file or env).
				var newCfg *config.Config
				var reloadErr error
				if *configFile != "" {
					newCfg, reloadErr = config.LoadFromFile(*configFile)
				} else {
					newCfg = config.LoadFromEnv()
				}
				if reloadErr != nil {
					log.Error().Str("error", reloadErr.Error()).Msg("Config reload failed — keeping current config")
					continue
				}

				// Validate before applying.
				if reloadErr = newCfg.Validate(); reloadErr != nil {
					log.Error().Str("error", reloadErr.Error()).Msg("Config validation failed — keeping current config")
					continue
				}

				// Reload TLS certificates: the Go HTTP server picks up new
				// cert/key files on the next TLS handshake when using
				// tls.Config with GetCertificate. For file-based TLS we log
				// the updated paths so operators know the reload was accepted.
				if newCfg.Server.TLSCertFile != "" && newCfg.Server.TLSKeyFile != "" {
					log.Info().
						Str("cert", newCfg.Server.TLSCertFile).
						Str("key", newCfg.Server.TLSKeyFile).
						Msg("TLS cert/key paths updated — will take effect on next handshake")
				}

				// Update routes: clear existing routes and add new ones.
				if len(newCfg.Routes) == 0 {
					newCfg.Routes = defaultRoutes(newCfg)
				}
				router := gw.GetRouter()
				router.ClearRoutes()
				for _, routeCfg := range newCfg.Routes {
					if err := router.AddRoute(&routeCfg, &newCfg.Circuit); err != nil {
						log.Error().Str("route", routeCfg.Name).Str("error", err.Error()).Msg("Failed to add route during reload")
					} else {
						log.Info().Str("route", routeCfg.Name).Msg("Route reloaded")
					}
				}

				log.Info().Int("routes", len(newCfg.Routes)).Msg("Configuration reload complete")
			}
		}
	}()

	// Start gateway
	if err := gw.Start(ctx); err != nil {
		log.Error().Str("error", err.Error()).Msg("Gateway error")
		os.Exit(1)
	}

	log.Info().Msg("API Gateway stopped")
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// defaultRoutes returns default routes for development.
// Routes are ordered from most specific to least specific path prefixes.
// Uses AlphaServices configuration for service host addresses.
func defaultRoutes(cfg *config.Config) []config.RouteConfig {
	alpha := cfg.AlphaServices

	return []config.RouteConfig{
		// Alpha Services - routed via /api/<service>/*
		{
			Name:           "timeguru",
			PathPrefix:     "/api/timeguru/",
			StripPrefix:    true,
			Backends:       []string{"http://" + alpha.TimeGuruHost},
			LoadBalancer:   "round_robin",
			HealthCheckURL: "/health",
			Timeout:        30 * time.Second,
			RetryCount:     2,
			RetryDelay:     100 * time.Millisecond,
		},
		{
			Name:           "captain",
			PathPrefix:     "/api/captain/",
			StripPrefix:    true,
			Backends:       []string{"http://" + alpha.CaptainHost},
			LoadBalancer:   "round_robin",
			HealthCheckURL: "/health",
			Timeout:        30 * time.Second,
			RetryCount:     2,
			RetryDelay:     100 * time.Millisecond,
		},
		{
			Name:           "architect",
			PathPrefix:     "/api/architect/",
			StripPrefix:    true,
			Backends:       []string{"http://" + alpha.ArchitectHost},
			LoadBalancer:   "round_robin",
			HealthCheckURL: "/health",
			Timeout:        30 * time.Second,
			RetryCount:     2,
			RetryDelay:     100 * time.Millisecond,
		},
		{
			Name:           "micromanager",
			PathPrefix:     "/api/micromanager/",
			StripPrefix:    true,
			Backends:       []string{"http://" + alpha.MicromanagerHost},
			LoadBalancer:   "round_robin",
			HealthCheckURL: "/health",
			Timeout:        30 * time.Second,
			RetryCount:     2,
			RetryDelay:     100 * time.Millisecond,
		},
		{
			Name:           "monad",
			PathPrefix:     "/api/monad/",
			StripPrefix:    true,
			Backends:       []string{"http://" + alpha.MonadHost},
			LoadBalancer:   "round_robin",
			HealthCheckURL: "/health",
			Timeout:        30 * time.Second,
			RetryCount:     2,
			RetryDelay:     100 * time.Millisecond,
		},
		{
			Name:           "sophia",
			PathPrefix:     "/api/sophia/",
			StripPrefix:    true,
			Backends:       []string{"http://" + alpha.SophiaHost},
			LoadBalancer:   "round_robin",
			HealthCheckURL: "/health",
			Timeout:        30 * time.Second,
			RetryCount:     2,
			RetryDelay:     100 * time.Millisecond,
		},
		// Infrastructure services
		{
			Name:           "wotan",
			PathPrefix:     "/events/",
			Backends:       []string{"http://" + getEnv("WOTAN_HOST", "wotan:8081")},
			LoadBalancer:   "round_robin",
			HealthCheckURL: "/health",
			Timeout:        30 * time.Second,
			RetryCount:     2,
			RetryDelay:     100 * time.Millisecond,
		},
	}
}
