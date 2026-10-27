// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// chaos-controller runs Yaldabaoth, the Kingdom's one chaos engine, behind
// its safety envelope (S-REAL-STEEL Phase 10, ADR-107): the blast-radius
// allowlist, the Kingdom SLO guard, the revert journal and three abort
// paths. Every fault goes through yaldabaoth.Service, the older fault-rule
// API and GameDay stages included.
//
//	chaos-controller              serve on :19012 (ports.Chaos)
//	chaos-controller revert-all   revert every journaled fault; exit 0 only
//	                              when the journal is empty afterwards (the
//	                              unit's ExecStopPost)
//	chaos-controller render-polkit  print the polkit rule for the allowlist's
//	                              systemd units (east install, D-23)
//	chaos-controller packet-chaos --iface steel0-h --flow-label 0x5EE1 ...
//	                              one packet_chaos experiment on a lab
//	                              interface from the command line, with
//	                              --dry-run to print the plan (packetcmd.go)
//
// Environment:
//
//	CHAOS_ALLOWLIST    allowlist file (default /etc/unheaded/chaos/allowlist.yaml); required
//	CHAOS_STATE_DIR    revert journal files (default /var/lib/unheaded-chaos)
//	WELL_DB            revert journals in The Well instead (unheaded_ops
//	                   chaos_journal, migration 039) as WELL_USER (default
//	                   ops_writer) with WELL_PASSWORD, WELL_HOST, WELL_PORT,
//	                   WELL_SSLMODE; open file entries are moved in at start.
//	                   Set and unreachable: the controller does not start and
//	                   revert-all exits 2 (journal.go)
//	KINGDOM_NODE       this host's name in the Well (default: the hostname)
//	CHAOS_ABORT_FILE   abort file (default /run/unheaded/chaos/ABORT)
//	CHAOS_DRY_RUN      "false" turns dry run off (a game-day window); anything else keeps it on
//	CUIRASS_URL        resolves LXD targets (default http://127.0.0.1:19006)
//	VM_URL, CHAOS_SLO_POLICY  the Kingdom SLO guard; without VM_URL nothing starts
//	WOTAN_ADDR, WOTAN_GRPC_ADDR  events, and the chaos.abort path
//	AUTH_ENABLED=true, AUTH_API_KEYS  pkg/auth; without AUTH_ENABLED every POST answers 403
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/discovery"
	"unheaded/pkg/healthreport"
	"unheaded/pkg/httputil"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
	"unheaded/pkg/sloguard"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/yaldabaoth"
)

const serviceName = "chaos-controller"

type config struct {
	addr       string
	allowlist  string
	stateDir   string
	abortFile  string
	dryRun     bool
	cuirassURL string
	cuirassKey string
	vmURL      string
	policy     string
	wotanHTTP  string
	wotanGRPC  string
	// getenv reads WELL_* for the journal store; nil reads nothing (files).
	getenv func(string) string
}

func (c config) env(k string) string {
	if c.getenv == nil {
		return ""
	}
	return c.getenv(k)
}

func envOr(getenv func(string) string, k, def string) string {
	if v := getenv(k); v != "" {
		return v
	}
	return def
}

func loadConfig(getenv func(string) string) config {
	return config{
		addr:       envOr(getenv, "HTTP_ADDR", ports.DefaultAddr(ports.Chaos)),
		allowlist:  envOr(getenv, "CHAOS_ALLOWLIST", "/etc/unheaded/chaos/allowlist.yaml"),
		stateDir:   envOr(getenv, "CHAOS_STATE_DIR", "/var/lib/unheaded-chaos"),
		abortFile:  envOr(getenv, "CHAOS_ABORT_FILE", "/run/unheaded/chaos/ABORT"),
		dryRun:     getenv("CHAOS_DRY_RUN") != "false",
		cuirassURL: envOr(getenv, "CUIRASS_URL", fmt.Sprintf("http://127.0.0.1:%d", ports.Cuirass)),
		cuirassKey: getenv("CUIRASS_API_KEY"),
		vmURL:      getenv("VM_URL"),
		policy:     envOr(getenv, "CHAOS_SLO_POLICY", "/etc/unheaded/chaos/policy.yaml"),
		wotanHTTP:  getenv("WOTAN_ADDR"),
		wotanGRPC:  getenv("WOTAN_GRPC_ADDR"),
		getenv:     getenv,
	}
}

// registerInjectors adds the real injectors (S-REAL-STEEL Phases 11-12):
// netem on an allowlisted instance's host veth (resolved from Cuirass), the
// container-kill driver over Cuirass's chaos endpoint, and the east-only
// systemd stop/kill injector. Types with no injector (http_error and the
// rest) are refused with an explanation (D-22), never faked.
var registerInjectors = func(s *yaldabaoth.Service, c config, log *logger.Logger) {
	runner := yaldabaoth.ExecRunner{}

	// netem needs a resolver to turn an LXD instance into its host veth.
	res, err := yaldabaoth.NewCuirassResolver(c.cuirassURL)
	if err != nil {
		log.Warn().Err(err).Msg("netem: no Cuirass resolver; lxd netem targets will be refused")
	}
	for mode, types := range map[yaldabaoth.NetemMode][]yaldabaoth.ExperimentType{
		yaldabaoth.NetemDelay:     {yaldabaoth.ExpTypeNetworkDelay, yaldabaoth.ExpTypeLatencySpike},
		yaldabaoth.NetemLoss:      {yaldabaoth.ExpTypeNetworkLoss},
		yaldabaoth.NetemPartition: {yaldabaoth.ExpTypeNetworkPartition},
	} {
		inj, err := yaldabaoth.NewNetemInjector(mode, runner, res, log)
		if err != nil {
			log.Error().Err(err).Str("mode", string(mode)).Msg("netem injector not registered")
			continue
		}
		for _, t := range types {
			s.RegisterInjector(t, inj)
		}
	}

	// Container kill drives Cuirass's out-of-band chaos endpoint.
	if cc, err := yaldabaoth.NewContainerChaosClient(c.cuirassURL, c.cuirassKey); err != nil {
		log.Warn().Err(err).Msg("container-kill injector not registered")
	} else {
		s.RegisterInjector(yaldabaoth.ExpTypeContainerKill, yaldabaoth.NewContainerInjector(cc, "kill", 0, log))
	}

	// systemd stop/kill, east only (Phase 12, D-23). The injector refuses on
	// any other host and authorization is a polkit rule, so registering it on
	// west is inert.
	s.RegisterInjector(yaldabaoth.ExpTypeServiceStop, yaldabaoth.NewSystemdStop(runner, os.Hostname, log))
	s.RegisterInjector(yaldabaoth.ExpTypeProcessKill, yaldabaoth.NewSystemdKill(runner, os.Hostname, log))

	// packet_chaos (Phase 13) needs the kernel: injectors_linux.go sets the hook.
	registerPacketInjector(s)

	// D-22: http_error points at the workload's own error knob.
	s.RefuseType(yaldabaoth.ExpTypeHTTPError, "not supported: deploy a faulty build instead (steel-echo STEEL_ERROR_RATE)")
}

// registerPacketInjector adds the packet_chaos injector (yaldabaoth-ebpf on a
// lab veth, Phase 13); injectors_linux.go sets it, elsewhere it is a no-op
// and the type is refused for want of an injector.
var registerPacketInjector = func(*yaldabaoth.Service) {}

// wotanPublisher joins each topic as chaos-controller before publishing.
type wotanPublisher struct {
	c      *wotanClient.Client
	mu     sync.Mutex
	joined map[string]bool
}

func (p *wotanPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	p.mu.Lock()
	joined := p.joined[topic]
	p.mu.Unlock()
	if !joined {
		jctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := p.c.SubscribeWithRetry(jctx, topic, yaldabaoth.WotanName, nil)
		cancel()
		if err != nil {
			return err
		}
		p.mu.Lock()
		p.joined[topic] = true
		p.mu.Unlock()
	}
	return p.c.Publish(ctx, topic, payload)
}

// build makes the engine. A missing allowlist or a damaged journal is
// fatal: the controller must not run without its fence or its memory.
func build(c config, log *logger.Logger) (*yaldabaoth.Service, error) {
	allow, err := yaldabaoth.LoadAllowlist(c.allowlist)
	if err != nil {
		return nil, err
	}
	res, err := yaldabaoth.NewCuirassResolver(c.cuirassURL)
	if err != nil {
		return nil, err
	}
	deps := yaldabaoth.Deps{Allowlist: allow, Resolver: res, Log: log}
	if c.vmURL != "" {
		g, err := sloguard.New(c.vmURL, c.policy)
		if err != nil {
			return nil, fmt.Errorf("SLO guard: %w", err)
		}
		deps.Guard = g
	} else {
		log.Warn().Msg("VM_URL is not set: no SLO guard, so every experiment is refused")
	}
	if c.wotanHTTP != "" {
		var wc *wotanClient.Client
		if c.wotanGRPC != "" {
			wc, err = wotanClient.NewClientWithGRPC(c.wotanHTTP, c.wotanGRPC)
		} else {
			wc, err = wotanClient.NewClient(c.wotanHTTP)
		}
		if err != nil {
			log.Warn().Err(err).Msg("Wotan client: events and the chaos.abort path are off")
		} else {
			deps.Publisher = &wotanPublisher{c: wc, joined: map[string]bool{}}
			deps.Subscriber = wc
		}
	}
	cfg := yaldabaoth.DefaultConfig()
	cfg.StateDir = c.stateDir
	cfg.AbortFile = c.abortFile
	cfg.DryRunMode = c.dryRun
	cfg.Hostname = os.Hostname
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// The connection lives as long as the process.
	journal, _, err := experimentJournal(ctx, c.env)
	if err != nil {
		return nil, err
	}
	cfg.Journal = journal
	mode := "file"
	if journal != nil {
		mode = journal.Mode()
	}
	log.Info().Str("journal", mode).Msg("chaos: revert journal")
	s, err := yaldabaoth.NewService(cfg, deps)
	if err != nil {
		return nil, err
	}
	registerInjectors(s, c, log)
	return s, nil
}

// revertAll is `chaos-controller revert-all`. It needs only the journal
// and the injectors: a controller whose allowlist, Cuirass or Victoria is
// gone must still take its faults back (it runs as ExecStopPost).
func revertAll(c config, out io.Writer) int {
	cfg := yaldabaoth.DefaultConfig()
	cfg.StateDir = c.stateDir
	cfg.AbortFile = ""
	log := logger.New(io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	journal, closeJournal, err := experimentJournal(ctx, c.env)
	if err != nil {
		fmt.Fprintf(out, "revert-all: %v\n", err)
		return 2
	}
	defer closeJournal()
	cfg.Journal = journal
	s, err := yaldabaoth.NewService(cfg, yaldabaoth.Deps{Log: log})
	if err == nil {
		registerInjectors(s, c, log)
	}
	if err != nil {
		fmt.Fprintf(out, "revert-all: %v\n", err)
		return 2
	}
	left, err := s.RevertAll(context.Background())
	if err != nil {
		fmt.Fprintf(out, "revert-all: %v\n", err)
	}
	if left > 0 {
		fmt.Fprintf(out, "revert-all: %d fault(s) still in place, see %s\n", left, c.stateDir)
		return 1
	}
	fmt.Fprintln(out, "revert-all: journal empty")
	return 0
}

// --- HTTP ---

type api struct {
	s           *yaldabaoth.Service
	gameday     *yaldabaoth.GameDayScheduler
	authEnabled bool
}

const maxBody = 64 << 10

func refusalStatus(err error) int {
	var r *yaldabaoth.Refusal
	if !errors.As(err, &r) {
		return http.StatusBadRequest
	}
	switch r.Reason {
	case yaldabaoth.RefuseInvalid:
		return http.StatusBadRequest
	case yaldabaoth.RefuseAllowlist, yaldabaoth.RefuseLimits:
		return http.StatusForbidden
	case yaldabaoth.RefuseConcurrency, yaldabaoth.RefuseAbortFile:
		return http.StatusConflict
	case yaldabaoth.RefuseNoInjector:
		return http.StatusNotImplemented
	default:
		return http.StatusServiceUnavailable
	}
}

func writeErr(w http.ResponseWriter, code int, err error) {
	httputil.WriteJSON(w, code, map[string]string{"error": err.Error()})
}

func (a *api) mustAuth(w http.ResponseWriter) bool {
	if !a.authEnabled {
		writeErr(w, http.StatusForbidden, errors.New("chaos writes need AUTH_ENABLED=true"))
		return false
	}
	return true
}

func (a *api) routes(reg *metrics.Registry) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/chaos/experiments", a.handleExperiments)
	mux.HandleFunc("/api/v1/chaos/experiments/", a.handleExperiment)
	mux.HandleFunc("/api/v1/chaos/abort", a.handleAbort)
	mux.HandleFunc("/api/v1/chaos/inject", a.handleInject)
	mux.HandleFunc("/api/v1/chaos/gameday", a.handleGameDay)
	mux.HandleFunc("/api/v1/chaos/gameday/status", func(w http.ResponseWriter, r *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, a.gameday.Status())
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": serviceName, "stats": a.s.Stats()})
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"ready": true})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_ = metrics.GatherAll(w, reg, metrics.DefaultRegistry)
	})
	return mux
}

func (a *api) start(w http.ResponseWriter, r *http.Request, exp *yaldabaoth.Experiment) {
	got, err := a.s.StartExperiment(r.Context(), exp)
	if err != nil {
		writeErr(w, refusalStatus(err), err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, got)
}

func (a *api) handleExperiments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		httputil.WriteJSON(w, http.StatusOK, a.s.List())
	case http.MethodPost:
		if !a.mustAuth(w) {
			return
		}
		var exp yaldabaoth.Experiment
		if err := httputil.DecodeJSONBody(r, maxBody, &exp); err != nil {
			httputil.HandleRequestError(w, err)
			return
		}
		a.start(w, r, &exp)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeErr(w, http.StatusMethodNotAllowed, errors.New("GET or POST"))
	}
}

func (a *api) handleExperiment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeErr(w, http.StatusMethodNotAllowed, errors.New("GET only"))
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/chaos/experiments/")
	e, ok := a.s.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no experiment %q", id))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, e)
}

func (a *api) handleAbort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeErr(w, http.StatusMethodNotAllowed, errors.New("POST only"))
		return
	}
	if !a.mustAuth(w) {
		return
	}
	n := a.s.Abort("http", "POST /api/v1/chaos/abort")
	a.gameday.Stop()
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"aborted": n})
}

// handleInject is the older fault-rule API: the rule runs as an
// experiment (and, naming no allowlisted target, is refused).
func (a *api) handleInject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeErr(w, http.StatusMethodNotAllowed, errors.New("POST only"))
		return
	}
	if !a.mustAuth(w) {
		return
	}
	var req struct {
		SrcServiceID uint8  `json:"src_svc"`
		DstServiceID uint8  `json:"dst_svc"`
		FaultType    string `json:"fault_type"`
		DropRatePPM  uint32 `json:"drop_rate"`
		DelayUS      uint64 `json:"delay_us"`
		DurationSec  int64  `json:"duration"`
	}
	if err := httputil.DecodeJSONBody(r, maxBody, &req); err != nil {
		httputil.HandleRequestError(w, err)
		return
	}
	if req.DurationSec < 0 || req.DurationSec > 86400 {
		writeErr(w, http.StatusBadRequest, errors.New("duration out of range"))
		return
	}
	a.start(w, r, yaldabaoth.RuleExperiment(yaldabaoth.FaultRule{
		SrcServiceID: req.SrcServiceID, DstServiceID: req.DstServiceID, FaultType: req.FaultType,
		DropRatePPM: req.DropRatePPM, DelayUS: req.DelayUS, Duration: time.Duration(req.DurationSec) * time.Second,
	}))
}

func (a *api) handleGameDay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeErr(w, http.StatusMethodNotAllowed, errors.New("POST only"))
		return
	}
	if !a.mustAuth(w) {
		return
	}
	var sc yaldabaoth.GameDayScenario
	if err := httputil.DecodeJSONBody(r, maxBody, &sc); err != nil {
		httputil.HandleRequestError(w, err)
		return
	}
	if err := a.gameday.Schedule(sc); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "scheduled", "scenario": sc.Name})
}

func newAPI(s *yaldabaoth.Service, pub yaldabaoth.Publisher, authEnabled bool) *api {
	return &api{s: s, gameday: yaldabaoth.NewGameDayScheduler(nil, s, pub), authEnabled: authEnabled}
}

func handler(a *api) http.Handler {
	mux := a.routes(a.s.Registry())
	h := auth.WrapHandler(mux, auth.SetupMiddleware(auth.LoadServiceAuthConfig(serviceName)))
	return httputil.SecurityHeaders(httputil.NewServiceMetrics(serviceName).Instrument(mux, h))
}

// renderPolkit prints the polkit rule for the configured allowlist's systemd
// units (D-23). The east install redirects it to
// /etc/polkit-1/rules.d/60-unheaded-chaos.rules.
func renderPolkit(c config, out io.Writer) int {
	allow, err := yaldabaoth.LoadAllowlist(c.allowlist)
	if err != nil {
		fmt.Fprintf(out, "render-polkit: %v\n", err)
		return 2
	}
	rule, err := yaldabaoth.RenderPolkitRule(allow)
	if err != nil {
		fmt.Fprintf(out, "render-polkit: %v\n", err)
		return 2
	}
	fmt.Fprint(out, rule)
	return 0
}

func main() {
	c := loadConfig(os.Getenv)
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "revert-all":
			os.Exit(revertAll(c, os.Stdout))
		case "render-polkit":
			os.Exit(renderPolkit(c, os.Stdout))
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "packet-chaos" {
		os.Exit(packetChaosMain(os.Args[2:]))
	}
	log := logger.New(os.Stderr)
	s, err := build(c, log)
	if err != nil {
		log.Fatal().Err(err).Msg("chaos-controller cannot start")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := s.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("chaos-controller cannot start")
	}
	authEnabled := auth.LoadServiceAuthConfig(serviceName).Enabled
	if !authEnabled {
		log.Warn().Msg("AUTH_ENABLED is not true: every chaos write answers 403")
	}
	a := newAPI(s, nil, authEnabled)

	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	discovery.Start(ctx, transportCfg, serviceName, ports.PortOf(c.addr, ports.Chaos))
	healthreport.StartService(ctx, transportCfg, serviceName, healthreport.HealthURL(c.addr, ports.PortOf(c.addr, ports.Chaos))) // ADR-029: a vote on Akira's ballot

	srv := &http.Server{
		Addr:              c.addr,
		Handler:           handler(a),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info().Str("addr", c.addr).Bool("dry_run", c.dryRun).Msg("chaos-controller listening")
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("HTTP server")
		}
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	a.gameday.Stop()
	s.Stop() // aborts and reverts whatever runs
	log.Info().Msg("chaos-controller stopped")
}
