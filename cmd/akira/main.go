// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

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

	"unheaded/pkg/auth"
	"unheaded/pkg/cmdline"
	"unheaded/pkg/health"
	"unheaded/pkg/httputil"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
	utls "unheaded/pkg/tls"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotansig"
)

// AkiraConfig is the YAML config file format.
type AkiraConfig struct {
	Targets []struct {
		Name       string `yaml:"name"`
		Host       string `yaml:"host"`
		Port       int    `yaml:"port"`
		HealthPath string `yaml:"health_path"`
		// Scheme: http or https; empty follows UNHEADED_TLS_ENABLED.
		Scheme string `yaml:"scheme"`
		// Type: http (default) or exec (S-IRON-GATE Phase 10). An exec
		// target runs Argv (absolute argv[0], no shell) as Akira's uid every
		// Interval (Go duration; empty = every sweep), killed after Timeout
		// (default and ceiling 30s); exit 0 votes healthy. Env is added to a
		// minimal environment; Akira's own is not passed on.
		Type     string            `yaml:"type"`
		Argv     []string          `yaml:"argv"`
		Interval string            `yaml:"interval"`
		Timeout  string            `yaml:"timeout"`
		Env      map[string]string `yaml:"env"`
	} `yaml:"targets"`
}

// parseDuration reads an optional Go duration ("" is 0).
func parseDuration(field, v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", field, v, err)
	}
	return d, nil
}

// execTargetNames is the set of exec targets: they have no unit, so
// --remediate never restarts them.
func execTargetNames(targets []health.ServiceTarget) map[string]bool {
	m := map[string]bool{}
	for _, t := range targets {
		if t.Exec != nil {
			m[t.Name] = true
		}
	}
	return m
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
	seen := make(map[string]bool, len(cfg.Targets))
	for i, t := range cfg.Targets {
		// A target's name is its service on the ballot. Without one, every
		// check of it was made and then dropped as anonymous, so it never
		// appeared in /api/v1/status; two targets with one name were a single
		// vote, the last check overwriting the other. Neither said so.
		if t.Name == "" {
			return nil, fmt.Errorf("target %d: name is required", i+1)
		}
		if seen[t.Name] {
			return nil, fmt.Errorf("target %d: duplicate name %q", i+1, t.Name)
		}
		seen[t.Name] = true
		switch t.Type {
		case "", "http":
			if len(t.Argv) > 0 || t.Interval != "" || t.Timeout != "" || len(t.Env) > 0 {
				return nil, fmt.Errorf("target %q: argv, interval, timeout and env belong to type: exec", t.Name)
			}
		case "exec":
			if t.Port != 0 || t.Host != "" || t.HealthPath != "" || t.Scheme != "" {
				return nil, fmt.Errorf("target %q: an exec target has no host, port, health_path or scheme", t.Name)
			}
			interval, err := parseDuration("interval", t.Interval)
			if err != nil {
				return nil, fmt.Errorf("target %q: %w", t.Name, err)
			}
			timeout, err := parseDuration("timeout", t.Timeout)
			if err != nil {
				return nil, fmt.Errorf("target %q: %w", t.Name, err)
			}
			if timeout == 0 {
				timeout = health.DefaultExecTimeout
			}
			ec := &health.ExecCheck{Argv: t.Argv, Interval: interval, Timeout: timeout, Env: t.Env}
			if err := ec.Validate(); err != nil {
				return nil, fmt.Errorf("target %q: %w", t.Name, err)
			}
			targets = append(targets, health.ServiceTarget{Name: t.Name, Exec: ec})
			continue
		default:
			return nil, fmt.Errorf("target %q: type %q is not http or exec", t.Name, t.Type)
		}
		if t.Port < 1 || t.Port > 65535 {
			return nil, fmt.Errorf("target %q: port %d is not 1-65535", t.Name, t.Port)
		}
		host := t.Host
		if host == "" {
			host = "localhost"
		}
		hp := t.HealthPath
		if hp == "" {
			hp = "/health"
		}
		switch t.Scheme {
		case "", "http", "https":
		default:
			return nil, fmt.Errorf("target %q: scheme %q is not http or https", t.Name, t.Scheme)
		}
		targets = append(targets, health.ServiceTarget{
			Name: t.Name, Host: host, Port: t.Port, HealthPath: hp, Scheme: t.Scheme,
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
	// Not first-party Go listeners: they never serve pkg/tls, so they stay
	// plain http when the host turns UNHEADED_TLS_ENABLED on.
	{Name: "zhenai", Host: "localhost", Port: 20103, HealthPath: "/health", Scheme: "http"},
	{Name: "inference", Host: "localhost", Port: 20100, HealthPath: "/health", Scheme: "http"},
	{Name: "daemon", Host: "localhost", Port: 17000, HealthPath: "/health"},
	{Name: "gateway", Host: "localhost", Port: 21000, HealthPath: "/health", Scheme: "http"},
}

func main() {
	// Flags
	once := flag.Bool("once", false, "Run single health sweep and exit")
	configPath := flag.String("config", "", "Path to config YAML (optional)")
	nodeID := flag.String("node", "", "Node identifier (default: hostname)")
	listenPort := flag.Int("port", ports.Akira, "HTTP API port for Akira status")
	remediate := flag.Bool("remediate", false, "restart a service (systemctl restart unheaded-<name>) when two-thirds of reporters see it failing; off by default")
	wotanAddr := flag.String("wotan", "http://localhost:18000", "Wotan HTTP address for publishing health reports")
	wotanGRPC := flag.String("wotan-grpc", "", "Wotan gRPC address for the signed topics (consensus, remediate); default: the --wotan host on port 18001")
	cmdline.Parse(cmdline.Spec{
		Name:     "akira",
		Purpose:  "Kingdom health daemon: polls every service, publishes votes to Wotan and decides the health band by consensus (ADR-029).",
		Examples: []string{"akira", "akira --once", "akira --config /etc/unheaded/akira.yaml"},
	})
	wotanClient.InitServiceIdentity("akira") // ADR-103 §6: every Wotan call carries akira's service token

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
	//
	// system.health.consensus and system.health.remediate are signed topics
	// (S-IRON-GATE Phase 9): with HEALTH_SIGNING_KEY_FILE they go out signed
	// over gRPC, without it they are not published at all.
	healthSigner, err := loadHealthSigner()
	if err != nil {
		log.Fatal().Err(err).Msg("health signing key: refusing to start")
	}
	if healthSigner == nil {
		log.Warn().Str("env", wotansig.HealthKeyEnv).
			Msg("no health signing key: consensus and remediation suggestions are NOT published to Wotan (reports still are)")
	}
	var wotan *wotanClient.Client
	if *wotanAddr != "" {
		httpAddr := strings.TrimPrefix(*wotanAddr, "http://")
		var c *wotanClient.Client
		var err error
		if healthSigner != nil {
			c, err = wotanClient.NewClientWithGRPC(httpAddr, wotanGRPCAddr(*wotanAddr, *wotanGRPC))
		} else {
			c, err = wotanClient.NewClient(httpAddr)
		}
		if err != nil {
			log.Warn().Err(err).Str("addr", *wotanAddr).Msg("Wotan connection failed — running without publish")
		} else {
			ok := true
			for _, topic := range []string{topicReports, topicConsensus, topicRemediate} {
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
	var wp *wotanPublisher
	if wotan != nil {
		wp = newWotanPublisher(wotan, healthSigner, metrics.DefaultRegistry)
	}
	publish := func(topic string, v interface{}) {
		if wp == nil {
			return
		}
		wp.publish(topic, v)
	}

	// Create Akira instance
	akira := health.NewAkira(*nodeID, targets)
	// Checks of first-party targets go over TLS with the deployment CA when
	// UNHEADED_TLS_ENABLED=true (S-IRON-GATE Phase 8).
	checkClient, err := utls.ClientFromEnv(5 * time.Second)
	if err != nil {
		log.Fatal().Err(err).Msg("TLS: refusing to start")
	}
	akira.SetHTTPClient(checkClient)

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
	// Entering CRITICAL or PANIC publishes one suggestion to
	// system.health.remediate, with or without --remediate.
	suggest := newRemediator(*nodeID, publish, metrics.DefaultRegistry)
	akira.OnSeverityChange(func(v health.Verdict) {
		suggest.onChange(v)
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

	noUnit := execTargetNames(targets)
	akira.OnAlert(func(v health.Verdict) {
		if noUnit[v.Service] {
			// An exec check (cert-expiry) has no unit to restart: its own
			// output, in the report's error, says what a human must do.
			if restarts[v.Service] == 0 {
				log.Error().Str("service", v.Service).Float64("failure_rate", v.FailureRate).
					Msg("CONSENSUS THRESHOLD on an exec check — no unit to restart: needs a human")
				restarts[v.Service] = -1 // logged for this outage
			}
			return
		}
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
			where := fmt.Sprintf(":%d", findPort(r.Service, targets))
			if noUnit[r.Service] {
				where = "exec"
			}
			fmt.Printf("  [%s] %s (%s) %s %s\n",
				status, r.Service, where,
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

	// HTTP API for status queries. A bad auth configuration refuses to
	// start rather than serve open (S-IRON-GATE Phase 6 recipe).
	api, err := apiHandler(akira, *nodeID, auth.LoadServiceAuthConfig("akira"))
	if err != nil {
		log.Fatal().Err(err).Msg("auth configuration unusable (ADR-103)")
	}
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", *listenPort),
		Handler:           api,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	// TLS from UNHEADED_TLS_* (S-IRON-GATE Phase 8): never a silent
	// plaintext fallback.
	tlsOn, err := utls.ConfigureServer(srv)
	if err != nil {
		log.Fatal().Err(err).Msg("TLS: refusing to start")
	}
	go func() {
		log.Info().Str("addr", srv.Addr).Bool("tls", tlsOn).Msg("Akira HTTP API listening")
		if err := utls.ListenAndServe(srv); err != nil && err != http.ErrServerClosed {
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
// akiraAPI is what the HTTP API reads from the daemon (*health.Akira).
type akiraAPI interface {
	Sweeps() uint64
	GetStates() map[string]health.Verdict
}

// apiHandler is Akira's HTTP API behind service auth: with AUTH_ENABLED=true
// every route but /health, /ready, /metrics and the GET consensus read
// /api/v1/status needs a service token from AUTH_ALLOW_CALLERS (no service
// calls Akira today, so none is listed: every token is 403). The status read
// stays open like the Kanban board's reads: health bands, no user data, and
// runbooks and game days curl it. Akira has no write route; any write is
// refused before a handler sees it.
func apiHandler(a akiraAPI, node string, cfg auth.ServiceAuthConfig) (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", httputil.ProbeMethods(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"akira","node":"%s"}`, node)
	}))
	mux.Handle("/ready", httputil.ProbeMethods(readyHandler(a).ServeHTTP))
	// /metrics did not exist here, though CLAUDE.md requires it of every
	// component (ADR-094 step 5). Akira keeps no registry of its own, so
	// this serves the default one: go_*, process_*, and what linked
	// libraries register at load.
	mux.Handle("/metrics", metrics.HandlerFor(metrics.DefaultRegistry))
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		states := a.GetStates()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(states) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
	})
	return auth.NewServiceHandler(mux, cfg, auth.ServiceRoutes{OpenReads: []string{"/api/v1/status"}})
}

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
