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
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"unheaded/pkg/health"
	"unheaded/pkg/httputil"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
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
	data, err := os.ReadFile(path) // #nosec G304 -- operator-configured path; no G304 site in this tree derives from an HTTP request (verified)
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
	{Name: "kanban", Host: "localhost", Port: 20001, HealthPath: "/health"},
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
	listenPort := flag.Int("port", ports.Akira, "HTTP API port for Akira status")
	remediate := flag.Bool("remediate", false, "restart a service (systemctl restart unheaded-<name>) when two-thirds of reporters see it failing; off by default")
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

	// Connect to Wotan (best-effort — Akira works without it). A client must
	// subscribe to a topic before it can publish there; Akira used to call
	// Publish without subscribing and discard the error, so it never
	// published anything.
	var wotan *wotanClient.Client
	if *wotanAddr != "" {
		c, err := wotanClient.NewClient(strings.TrimPrefix(*wotanAddr, "http://"))
		if err != nil {
			log.Warn().Err(err).Str("addr", *wotanAddr).Msg("Wotan connection failed — running without publish")
		} else {
			ok := true
			for _, topic := range []string{topicReports, topicConsensus} {
				if _, err := c.Subscribe(context.Background(), topic, "akira"); err != nil {
					log.Warn().Err(err).Str("topic", topic).Msg("Wotan subscribe failed — running without publish")
					ok = false
				}
			}
			if ok {
				wotan = c
				log.Info().Str("addr", *wotanAddr).Msg("connected to Wotan")
			}
		}
	}
	publish := func(topic string, v interface{}) {
		if wotan == nil {
			return
		}
		payload, err := json.Marshal(v)
		if err != nil {
			return
		}
		if err := wotan.Publish(context.Background(), topic, payload); err != nil {
			log.Warn().Err(err).Str("topic", topic).Msg("publish failed")
		}
	}

	// Create Akira instance
	akira := health.NewAkira(*nodeID, targets)

	// Every other reporter's reports are votes too (ADR-029: every node a
	// watchdog). This node's own are recorded locally by CheckAll.
	if wotan != nil && !*once {
		go consumeReports(wotan, *nodeID, akira)
	}

	akira.OnReport(func(r health.HealthReport) {
		if !r.Healthy {
			log.Warn().
				Str("service", r.Service).
				Str("error", r.Error).
				Dur("latency", r.Latency).
				Msg("service unhealthy")
		}
		publish(topicReports, r)
	})

	// restarts counts remediation attempts per outage; it resets when the
	// service drops below the threshold. (AutoRestarts used to be read but
	// never incremented, so escalation to a human could never happen.)
	restarts := map[string]int{}
	akira.OnSeverityChange(func(v health.Verdict) {
		log.Info().
			Str("service", v.Service).
			Str("severity", string(v.Severity)).
			Int("failing", v.Failing).
			Int("reporters", v.Reporters).
			Msg("consensus severity changed")
		publish(topicConsensus, struct {
			health.Verdict
			Node string `json:"node"`
			Time int64  `json:"time"`
		}{v, *nodeID, time.Now().UnixMilli()})
		if !v.Remediate {
			delete(restarts, v.Service)
		}
	})

	akira.OnAlert(func(v health.Verdict) {
		if !*remediate {
			if restarts[v.Service] == 0 {
				log.Error().Str("service", v.Service).Float64("failure_rate", v.FailureRate).
					Msg("CONSENSUS THRESHOLD — remediation off (--remediate=false): needs a human")
				restarts[v.Service] = -1 // logged for this outage
			}
			return
		}
		n := restarts[v.Service]
		if n >= health.MaxAutoRestarts {
			if n == health.MaxAutoRestarts {
				log.Error().Str("service", v.Service).Int("restarts", n).
					Msg("MAX AUTO-RESTARTS EXCEEDED — escalating to human")
				restarts[v.Service] = n + 1 // escalate once
			}
			return
		}
		restarts[v.Service] = n + 1
		svcUnit := fmt.Sprintf("unheaded-%s", v.Service)
		log.Warn().Str("service", v.Service).Str("unit", svcUnit).Int("attempt", n+1).
			Msg("attempting auto-restart via systemctl")
		cmd := exec.Command("systemctl", "restart", svcUnit) // #nosec G204 -- unit name from the kingdom service registry, not user input
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Error().Err(err).Str("output", string(out)).Str("service", v.Service).Msg("auto-restart FAILED")
		} else {
			log.Info().Str("service", v.Service).Msg("auto-restart SUCCESS")
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
				fmt.Printf("  %s: %d/%d reporters failing (%s)\n", a.Service, a.Failing, a.Reporters, a.Severity)
			}
		}
		return
	}

	// Start HTTP API for status queries
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", httputil.ProbeMethods(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status":"ok","service":"akira","node":"%s"}`, *nodeID)
		}))
		mux.Handle("/ready", httputil.ProbeMethods(readyHandler(akira).ServeHTTP))
		// /metrics did not exist here, though CLAUDE.md requires it of every
		// component (ADR-094 step 5). Akira keeps no registry of its own, so
		// this serves the default one: go_*, process_*, and what linked
		// libraries register at load.
		mux.Handle("/metrics", metrics.HandlerFor(metrics.DefaultRegistry))
		mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
			states := akira.GetStates()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(states) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
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

// readyHandler serves /ready. Akira is ready once one full sweep has
// completed: before that /api/v1/status is empty, and the first sweep waits
// a whole HealthCheckInterval. A constant 200 would publish a readiness
// nothing measured.
func readyHandler(a interface{ Sweeps() uint64 }) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := a.Sweeps()
		if n == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"not_ready","reason":"no completed sweep"}`)
			return
		}
		fmt.Fprintf(w, `{"status":"ready","sweeps":%d}`, n)
	})
}

// Wotan topics (ADR-029).
const (
	topicReports   = "system.health.reports"   // every reporter's checks
	topicConsensus = "system.health.consensus" // severity changes
)

// consumeReports feeds other reporters' reports from Wotan into the ballot.
func consumeReports(c *wotanClient.Client, self string, a *health.Akira) {
	ch, err := c.StreamMessages(context.Background(), topicReports)
	if err != nil {
		log.Warn().Err(err).Msg("cannot read system.health.reports; tallying this node's checks only")
		return
	}
	for m := range ch {
		var r health.HealthReport
		if err := json.Unmarshal([]byte(m.Payload), &r); err != nil || r.Reporter == self {
			continue
		}
		a.Record(r)
	}
}

func findPort(name string, targets []health.ServiceTarget) int {
	for _, t := range targets {
		if t.Name == name {
			return t.Port
		}
	}
	return 0
}
