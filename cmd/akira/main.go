// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

// Akira — Kingdom Health Daemon
//
// Monitors all services via ping/pong health protocol.
// Every service pushes health to Wotan, Akira aggregates and
// triggers consensus-based auto-remediation when 66.67% report failure.
//
// Named after the film.
//
// Usage:
//   akira                         # Run daemon (foreground)
//   akira --once                  # Single health sweep, print results, exit
//   akira --config /etc/unheaded/akira.yaml
//
// ADR: ADR-029 Wotan Consensus Health + Auto-Remediation

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"unheaded/pkg/health"
	wotanClient "unheaded/pkg/wotan-client"
)

// AkiraConfig is the YAML config file format.
type AkiraConfig struct {
	Targets []struct {
		Name       string `yaml:"name"`
		Host       string `yaml:"host"`
		Port       int    `yaml:"port"`
		HealthPath string `yaml:"health_path"`
	} `yaml:"targets"`
}

func loadConfig(path string) ([]health.ServiceTarget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg AkiraConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	targets := make([]health.ServiceTarget, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		host := t.Host
		if host == "" {
			host = "localhost"
		}
		hp := t.HealthPath
		if hp == "" {
			hp = "/health"
		}
		targets = append(targets, health.ServiceTarget{
			Name: t.Name, Host: host, Port: t.Port, HealthPath: hp,
		})
	}
	return targets, nil
}

// Default service targets — The Doom Range
var defaultTargets = []health.ServiceTarget{
	{Name: "wotan", Host: "localhost", Port: 18000, HealthPath: "/health"},
	{Name: "timeguru", Host: "localhost", Port: 19000, HealthPath: "/health"},
	{Name: "architect", Host: "localhost", Port: 19001, HealthPath: "/health"},
	{Name: "captain", Host: "localhost", Port: 19002, HealthPath: "/health"},
	{Name: "micromanager", Host: "localhost", Port: 19003, HealthPath: "/health"},
	{Name: "monad", Host: "localhost", Port: 19004, HealthPath: "/health"},
	{Name: "sophia", Host: "localhost", Port: 19005, HealthPath: "/health"},
	{Name: "dashboard", Host: "localhost", Port: 20000, HealthPath: "/health"},
	{Name: "kanban", Host: "localhost", Port: 16668, HealthPath: "/health"},
	{Name: "zhenai", Host: "localhost", Port: 20103, HealthPath: "/health"},
	{Name: "inference", Host: "localhost", Port: 20100, HealthPath: "/health"},
	{Name: "daemon", Host: "localhost", Port: 17000, HealthPath: "/health"},
	{Name: "gateway", Host: "localhost", Port: 21000, HealthPath: "/health"},
}

func main() {
	// Flags
	once := flag.Bool("once", false, "Run single health sweep and exit")
	configPath := flag.String("config", "", "Path to config YAML (optional)")
	nodeID := flag.String("node", "", "Node identifier (default: hostname)")
	listenPort := flag.Int("port", 16699, "HTTP API port for Akira status")
	wotanAddr := flag.String("wotan", "http://localhost:18000", "Wotan HTTP address for publishing health reports")
	flag.Parse()

	// Logger
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Str("service", "akira").Logger()

	// Node ID
	if *nodeID == "" {
		hostname, _ := os.Hostname()
		*nodeID = hostname
	}

	log.Info().
		Str("node", *nodeID).
		Int("targets", len(defaultTargets)).
		Msg("AKIRA — Kingdom Health Daemon starting")

	// Load targets (from config or defaults)
	targets := defaultTargets
	if *configPath != "" {
		loaded, err := loadConfig(*configPath)
		if err != nil {
			log.Fatal().Err(err).Str("config", *configPath).Msg("failed to load config")
		}
		targets = loaded
		log.Info().Str("config", *configPath).Int("targets", len(targets)).Msg("loaded targets from config")
	}

	// Connect to Wotan (best-effort — Akira works without it)
	var wotan *wotanClient.Client
	if *wotanAddr != "" {
		var err error
		wotan, err = wotanClient.NewClient(*wotanAddr)
		if err != nil {
			log.Warn().Err(err).Str("addr", *wotanAddr).Msg("Wotan connection failed — running without publish")
		} else {
			log.Info().Str("addr", *wotanAddr).Msg("connected to Wotan")
		}
	}

	// Create Akira instance
	akira := health.NewAkira(*nodeID, targets)

	// Set callbacks
	akira.OnReport(func(r health.HealthReport) {
		if !r.Healthy {
			log.Warn().
				Str("service", r.Service).
				Str("error", r.Error).
				Dur("latency", r.Latency).
				Msg("service unhealthy")
		}
		// Publish all health reports to Wotan
		if wotan != nil {
			payload, _ := json.Marshal(map[string]interface{}{
				"node":    *nodeID,
				"service": r.Service,
				"healthy": r.Healthy,
				"latency": r.Latency.Milliseconds(),
				"error":   r.Error,
				"time":    time.Now().UnixMilli(),
			})
			_ = wotan.Publish(context.Background(), "system.health.reports", payload)
		}
	})

	akira.OnAlert(func(s health.ConsensusState) {
		log.Error().
			Str("service", s.Service).
			Float64("failure_rate", s.FailureRate).
			Int("auto_restarts", s.AutoRestarts).
			Msg("CONSENSUS THRESHOLD — remediation needed")

		// Publish consensus alerts to Wotan
		if wotan != nil {
			payload, _ := json.Marshal(map[string]interface{}{
				"node":          *nodeID,
				"service":       s.Service,
				"failure_rate":  s.FailureRate,
				"auto_restarts": s.AutoRestarts,
				"time":          time.Now().UnixMilli(),
			})
			_ = wotan.Publish(context.Background(), "system.outage.reports", payload)
		}

		// Auto-restart if under limit
		if s.AutoRestarts < health.MaxAutoRestarts {
			svcUnit := fmt.Sprintf("unheaded-%s", s.Service)
			log.Warn().
				Str("service", s.Service).
				Str("unit", svcUnit).
				Int("attempt", s.AutoRestarts+1).
				Msg("attempting auto-restart via systemctl")

			cmd := exec.Command("systemctl", "restart", svcUnit) //nolint:gosec // svcUnit comes from kingdom service registry, not user input
			if out, err := cmd.CombinedOutput(); err != nil {
				log.Error().
					Err(err).
					Str("output", string(out)).
					Str("service", s.Service).
					Msg("auto-restart FAILED")
			} else {
				log.Info().
					Str("service", s.Service).
					Msg("auto-restart SUCCESS")
			}
		} else {
			log.Error().
				Str("service", s.Service).
				Int("restarts", s.AutoRestarts).
				Msg("MAX AUTO-RESTARTS EXCEEDED — escalating to human")
		}
	})

	// Single sweep mode
	if *once {
		reports := akira.CheckAll()
		healthy := 0
		for _, r := range reports {
			status := "FAIL"
			if r.Healthy {
				status = "OK"
				healthy++
			}
			fmt.Printf("  [%s] %s (:%d) %s %s\n",
				status, r.Service, findPort(r.Service, targets),
				r.Latency.Round(time.Millisecond), r.Error)
		}
		fmt.Printf("\n%d/%d healthy\n", healthy, len(reports))

		alerts := akira.EvaluateConsensus()
		if len(alerts) > 0 {
			fmt.Printf("\nALERTS (%d):\n", len(alerts))
			for _, a := range alerts {
				fmt.Printf("  %s: %.0f%% failure rate\n", a.Service, a.FailureRate*100)
			}
		}
		return
	}

	// Start HTTP API for status queries
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status":"ok","service":"akira","node":"%s"}`, *nodeID)
		})
		mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
			states := akira.GetStates()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(states)
		})
		addr := fmt.Sprintf(":%d", *listenPort)
		log.Info().Str("addr", addr).Msg("Akira HTTP API listening")
		srv := &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Akira HTTP API listen failed")
		}
	}()

	// Daemon mode — run until signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	akira.Run(ctx)
	log.Info().Msg("Akira stopped")
}

func findPort(name string, targets []health.ServiceTarget) int {
	for _, t := range targets {
		if t.Name == name {
			return t.Port
		}
	}
	return 0
}
