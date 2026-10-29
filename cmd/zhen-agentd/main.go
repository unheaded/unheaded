// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

// Command zhen-agentd is the long-running HTTP daemon variant of
// zhen-agent. Same agent runtime (pkg/agent + pkg/champion + cs/vor +
// llama-server backends) but exposed over HTTP for multi-request use:
//
//	POST /api/v1/agent/ask {goal, project_root, session_id?, ...}
//	  → {answer, turns_used, budget_hit, trace[], session_id}
//
//	GET  /health    — liveness probe (always returns 200 once up)
//	GET  /ready     — readiness probe (verifies vor + llama-server reachable)
//
// Default port: 20105 (in the AI Services band 20101-20106 reserved
// per CLAUDE.md's Port Allocation table).
//
// Graceful shutdown on SIGINT/SIGTERM: stops accepting new requests,
// waits up to 10s for in-flight requests to finish, then exits.
//
// stdlib-only HTTP — same discipline as zhen-rag and zhen-agent.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"unheaded/pkg/agent"
	"unheaded/pkg/auth"
	"unheaded/pkg/champion"
	"unheaded/pkg/champion/pgstore"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/cmdline"
	"unheaded/pkg/localllm"
	"unheaded/pkg/metrics/prom"
	"unheaded/pkg/vorquery"
)

const (
	defaultPort      = 20105
	defaultVorURL    = "http://127.0.0.1:9876"
	defaultLlamaURL  = "http://127.0.0.1:8081"
	defaultModel     = "qwen2.5-coder-7b-instruct"
	defaultK         = 5
	defaultMaxTokens = 600
	defaultMaxTurns  = 8
	defaultMaxChars  = 10000
	httpTimeout      = 5 * time.Minute
	shutdownTimeout  = 10 * time.Second
)

func main() {
	var (
		port         = flag.Int("port", defaultPort, "HTTP listen port")
		host         = flag.String("host", "127.0.0.1", "HTTP listen host (use 0.0.0.0 for non-loopback)")
		projectRoot  = flag.String("project-root", "", "default Champion sandbox root used when a request omits project_root (default: cwd)")
		allowedRoots = flag.String("allowed-roots", "", "comma-separated list of project roots requests may target; defaults to just -project-root")
		actionStore  = flag.String("action-store", "stderr", "audit-log backend: stderr (dev) | pg (production via The Well; requires WELL_DSN env)")
		kanbanStore  = flag.String("kanban-store", "memory", "kanban-task backend for kanban_create / kanban_update tool calls: memory (in-process, dev) | pg (production via The Well; DSN from WELL_KANBAN_DSN, else WELL_DSN). Without this, kanban_* tool calls pass the gate but fail at dispatch.")
		rateRPS      = flag.Float64("rate-limit", defaultRateLimitRPS, "per-IP requests/sec (0 disables)")
		rateBurst    = flag.Int("rate-burst", defaultRateBurst, "per-IP rate-limit burst capacity (only used when -rate-limit > 0)")
		noAuth       = flag.Bool("no-auth", false, "serve without authentication; only with a loopback -host and AUTH_ENABLED unset (dev). Without it, zhen-agentd exits 2 unless AUTH_ENABLED=true and AUTH_API_KEYS, AUTH_API_KEY_FILE or a trust dir (AUTH_TRUST_DIR) is set")
		auditWell    = flag.Bool("audit-well", false, "record one llm_requests row per llama-server call and one audit_events row per Champion gate decision in the Well (DSN from env WELL_OPS_DSN)")
		confirmQ     = flag.Bool("confirm-queue", false, "queue agent-loop writes the gate refuses pending confirmation in the Well (unheaded_app.zhen_pending_confirmations, DSN from env WELL_APP_ZHEN_DSN) instead of denying them; Stevie decides with `zhen-claude confirm`")
		auditWotan   = flag.Bool("audit-wotan", false, "also publish those records on Wotan (zhen.llm.requests, zhen.audit.toolcalls; HTTP host:port from env WOTAN_ADDR)")
		memoryStore  = flag.String("memory-store", "none", "memory backend for /api/v1/memory/recall and /remember (zhen-cli /recall, /remember): none (503) | pg (unheaded_app.zhen_memories, DSN from env WELL_APP_ZHEN_DSN)")
		askTokens    = flag.Bool("ask-tokens", false, "legacy: return pending-confirmation tokens in /api/v1/agent/ask traces. Only for a listener no model can reach; by default /ask queues (-confirm-queue) or denies such writes and returns no token")
		schedOn      = flag.Bool("scheduler", false, "run approved runbooks on the schedule in -schedules: one Kanban card and one zhen_agent_runs row (mode scheduled; DSN from env WELL_APP_ZHEN_DSN, in memory without it) per run, every call through the Champion gate; heartbeat gauge zhen_scheduler_heartbeat_timestamp_seconds (replaces raft/zhen_scheduler.py, ADR-028)")
		schedFile    = flag.String("schedules", envOr("ZHEN_SCHEDULES", "configs/zhen-schedules.yaml"), "with -scheduler: the schedule file (env ZHEN_SCHEDULES); read-only to the daemon")
		schedState   = flag.String("schedules-state", os.Getenv("ZHEN_SCHEDULES_STATE"), "with -scheduler: where run-time schedule changes (POST /api/v1/scheduler/add|remove|pause) persist (env ZHEN_SCHEDULES_STATE); empty keeps them until restart")
		metricsAddr  = flag.String("metrics-addr", os.Getenv("ZHEN_METRICS_ADDR"), "also serve /metrics alone on this host:port (env ZHEN_METRICS_ADDR), e.g. the WireGuard address Prometheus scrapes, '[fd00:dead:beef::1]:20105'; the API stays on -host")
	)
	var wf workerFlags
	registerWorkerFlags(flag.CommandLine, &wf)
	cmdline.Parse(cmdline.Spec{
		Name:     "zhen-agentd",
		Purpose:  "Serve the Zhenai agent runtime over HTTP for multi-request use.",
		Examples: []string{"zhen-agentd", "zhen-agentd --port 26300 --project-root ."},
	})
	if wf.overnight {
		set := map[string]bool{}
		flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
		if set["privacy"] && wf.privacyMode != "strict" {
			fmt.Fprintln(os.Stderr, "zhen-agentd: -overnight forces -privacy strict")
		}
		if err := applyOvernightDefaults(&wf, set, defaultStateDir()); err != nil {
			fatal("overnight: %v", err)
		}
	}

	if *schedOn && (wf.overnight || wf.once) {
		fatal("-scheduler runs with the listener, not with -overnight or -once")
	}

	// The listener needs auth (M-019). Checked before anything starts;
	// -overnight and -once runs open no listener.
	var authMW func(http.Handler) http.Handler
	if !wf.overnight && !wf.once {
		mw, err := listenerAuth(auth.LoadServiceAuthConfig("zhen-agentd"), *noAuth, *host)
		if err != nil {
			fmt.Fprintf(os.Stderr, "zhen-agentd: %v\n", err)
			os.Exit(2)
		}
		authMW = mw
	}

	vorURL := envOr("VOR_URL", defaultVorURL)
	llamaURL := envOr("LLAMA_URL", defaultLlamaURL)
	modelName := envOr("RAG_MODEL", defaultModel)

	root := *projectRoot
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fatal("getwd: %v", err)
		}
		root = cwd
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		fatal("resolve project-root: %v", err)
	}

	// Build the allow-list. Default-tenant root is always allowed; any
	// additional roots from -allowed-roots get normalized + validated +
	// added. A request's project_root must exactly match one of these
	// after normalization to be served.
	allowed := map[string]struct{}{rootAbs: {}}
	if *allowedRoots != "" {
		for _, r := range strings.Split(*allowedRoots, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			abs, err := filepath.Abs(r)
			if err != nil {
				fatal("resolve allowed-root %q: %v", r, err)
			}
			info, err := os.Stat(abs)
			if err != nil {
				fatal("allowed-root %q: %v", abs, err)
			}
			if !info.IsDir() {
				fatal("allowed-root %q: not a directory", abs)
			}
			allowed[abs] = struct{}{}
		}
	}

	aud, err := buildAgentdAudit(context.Background(), *auditWell, *auditWotan)
	if err != nil {
		fatal("audit init: %v", err)
	}
	defer aud.close()
	rawStore, err := buildActionStore(*actionStore)
	if err != nil {
		fatal("action-store init: %v", err)
	}
	rawStore = newAuditActionStore(rawStore, aud.auditor)
	// Wrap with the metrics decorator so every gate decision shows up
	// in /metrics under zhen_agentd_champion_actions_total.
	store := newMetricsActionStore(rawStore)
	kStore, err := buildKanbanStore(*kanbanStore)
	if err != nil {
		fatal("kanban-store init: %v", err)
	}
	pool := newChampionPool(store, kStore)
	mem, closeMem, err := buildMemoryStore(context.Background(), *memoryStore)
	if err != nil {
		fatal("memory-store init: %v", err)
	}
	defer closeMem()
	pool.memory = mem
	gcCtx, stopGC := context.WithCancel(context.Background())
	defer stopGC()
	pool.gcCtx = gcCtx // RT-04: confirmation tokens are swept on a ticker

	queue, closeQueue, err := buildConfirmQueue(context.Background(), *confirmQ)
	if err != nil {
		fatal("confirm-queue init: %v", err)
	}
	defer closeQueue()

	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	workerDone := make(chan struct{})
	if wf.enabled {
		wlog := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("service", "zhen-agentd")
		mode := "interactive"
		if wf.overnight {
			mode = "overnight"
		}
		w, closeWorker, err := buildWorker(workerCtx, wf, rootAbs, llamaURL, mode, aud, wlog)
		defer closeWorker()
		if err != nil {
			fatal("worker init: %v", err)
		}
		if wf.overnight {
			code := runOvernight(workerCtx, w, wf, aud, wlog)
			aud.close()
			closeWorker()
			os.Exit(code)
		}
		if wf.once {
			res, err := w.runOnce(workerCtx)
			fmt.Println(resultJSON(res))
			if err != nil {
				fmt.Fprintf(os.Stderr, "zhen-agentd: worker: %v\n", err)
				aud.close()
				os.Exit(1)
			}
			return
		}
		go func() { defer close(workerDone); workerLoop(workerCtx, w, wf.poll, wlog) }()
	} else {
		close(workerDone)
		if wf.once {
			fatal("-once needs -worker")
		}
	}

	httpClient := &http.Client{Timeout: httpTimeout}
	srv := &server{
		pool:        pool,
		defaultRoot: rootAbs,
		allowed:     allowed,
		retriever:   &vorRetriever{client: httpClient, baseURL: vorURL, maxChars: defaultMaxChars, index: vorquery.NewIndex(httpClient, vorURL)},
		llm:         &llamaLLM{client: httpClient, baseURL: llamaURL, model: modelName, audit: aud.auditor, gpuLock: wf.gpuLock},
		vorURL:      vorURL,
		llamaURL:    llamaURL,
		ready:       newReadyTracker(httpClient, vorURL, llamaURL),
		askTokens:   *askTokens,
		memory:      mem,
	}
	if queue != nil {
		srv.confirmQueue = queue
	}
	if aud.wotan != nil {
		srv.confirmPub = aud.wotan
	}
	if ca, ok := aud.auditor.(claudebridge.ConfirmAuditor); ok {
		srv.confirmAudit = ca
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", prom.Handler())
	mux.Handle("/health", instrument("/health", http.HandlerFunc(srv.handleHealth)))
	mux.Handle("/ready", instrument("/ready", http.HandlerFunc(srv.handleReady)))
	mux.Handle("/api/v1/agent/ask", instrument("/api/v1/agent/ask", http.HandlerFunc(srv.handleAsk)))
	mux.Handle("/api/v1/agent/ask/stream", instrument("/api/v1/agent/ask/stream", http.HandlerFunc(srv.handleAskStream)))
	mux.Handle("/api/v1/agent/confirm", instrument("/api/v1/agent/confirm", http.HandlerFunc(srv.handleConfirm)))
	mux.Handle("/api/v1/tool/exec", instrument("/api/v1/tool/exec", http.HandlerFunc(srv.handleToolExec)))
	mux.Handle("/api/v1/openapi.json", instrument("/api/v1/openapi.json", http.HandlerFunc(srv.handleOpenAPI)))
	mux.Handle("/api/v1/memory/recall", instrument("/api/v1/memory/recall", http.HandlerFunc(srv.handleMemoryRecall)))
	mux.Handle("/api/v1/memory/remember", instrument("/api/v1/memory/remember", http.HandlerFunc(srv.handleMemoryRemember)))
	mux.Handle("/api/v1/models", instrument("/api/v1/models", http.HandlerFunc(srv.handleModels)))
	srv.registerScheduler(mux)

	schedCtx, stopSched := context.WithCancel(context.Background())
	defer stopSched()
	schedDone := make(chan struct{})
	if *schedOn {
		schedLog := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("service", "zhen-agentd", "component", "scheduler")
		champ, err := pool.get(rootAbs)
		if err != nil {
			fatal("scheduler: champion init: %v", err)
		}
		runs, closeRuns := openScheduledStore(schedCtx, schedLog)
		defer closeRuns()
		var pub claudebridge.JSONPublisher
		if aud.wotan != nil {
			pub = aud.wotan
		}
		sch, err := newScheduler(schedCtx, schedulerConfig{Champion: champ, Path: *schedFile, StatePath: *schedState, Runs: runs,
			Events: agentdRunEvents{aud: aud, log: schedLog}, Pub: pub, Log: schedLog})
		if err != nil {
			fatal("scheduler: %v", err)
		}
		srv.attachScheduler(sch)
		fmt.Fprintf(os.Stderr, "zhen-agentd: scheduler on: %d job(s) from %s\n", len(sch.view().Jobs), sch.cfg.Path)
		go func() { defer close(schedDone); sch.loop(schedCtx, defaultSchedulerTick) }()
	} else {
		close(schedDone)
	}
	if *metricsAddr != "" {
		mm := http.NewServeMux()
		mm.Handle("/metrics", prom.Handler())
		msrv := &http.Server{Addr: *metricsAddr, Handler: mm, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			// Not fatal, and retried: the WireGuard address may come up
			// after the daemon. Meanwhile the API serves and the scrape
			// reads down (ZhenSchedulerStalled says so).
			for {
				fmt.Fprintf(os.Stderr, "zhen-agentd: /metrics also on http://%s\n", *metricsAddr)
				err := msrv.ListenAndServe()
				if err == nil || errors.Is(err, http.ErrServerClosed) {
					return
				}
				fmt.Fprintf(os.Stderr, "zhen-agentd: -metrics-addr %s: %v (retrying in 30s)\n", *metricsAddr, err)
				select {
				case <-schedCtx.Done():
					return
				case <-time.After(30 * time.Second):
				}
			}
		}()
		defer func() { _ = msrv.Close() }()
	}

	// Outer middleware chain (innermost-last):
	//   rate-limit  →  auth  →  mux
	// Auth wraps the mux so /metrics is exempted via SkipAuthPaths.
	// Rate-limit is outermost so a flood of unauthenticated requests
	// gets dropped before the auth path-skip check + verification.
	var handler http.Handler = mux
	if authMW != nil {
		fmt.Fprintf(os.Stderr, "zhen-agentd: auth middleware enabled\n")
		handler = authMW(mux)
	} else {
		fmt.Fprintf(os.Stderr, "zhen-agentd: -no-auth: serving WITHOUT authentication on loopback %s (dev only)\n", *host)
	}
	if rl := newRateLimiter(*rateRPS, *rateBurst); rl != nil {
		fmt.Fprintf(os.Stderr, "zhen-agentd: per-IP rate limit %.1f rps, burst %d\n", *rateRPS, *rateBurst)
		handler = rl.Middleware(handler)
	}

	listenAddr := fmt.Sprintf("%s:%d", *host, *port)
	httpSrv := &http.Server{
		Addr:              listenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Spin up the listener.
	errCh := make(chan error, 1)
	go func() {
		var allowedList []string
		for r := range allowed {
			allowedList = append(allowedList, r)
		}
		fmt.Fprintf(os.Stderr, "zhen-agentd listening on http://%s — default-root=%s allowed=%v vor=%s llama=%s\n",
			listenAddr, rootAbs, allowedList, vorURL, llamaURL)
		errCh <- httpSrv.ListenAndServe()
	}()

	// Wait for SIGINT/SIGTERM or listener failure.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		fmt.Fprintf(os.Stderr, "zhen-agentd: received %v, shutting down (timeout %s)...\n", sig, shutdownTimeout)
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			fatal("listen: %v", err)
		}
	}

	// Graceful shutdown: the worker first (a card in flight is cancelled,
	// nothing partial is committed), then the listener.
	stopWorker()
	stopSched() // a runbook in flight is cancelled; its run is recorded failed
	for name, done := range map[string]chan struct{}{"worker": workerDone, "scheduler": schedDone} {
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			fmt.Fprintf(os.Stderr, "zhen-agentd: %s did not stop in time\n", name)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "zhen-agentd: shutdown: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "zhen-agentd: clean shutdown")
}

// buildKanbanStore selects the kanban backend per the -kanban-store
// flag. "memory" (default, dev-mode) returns nil — the Champion will
// reject kanban_create/update with "no kanban store configured", which
// is honest about the lack of persistence. "pg" connects to The Well
// via WELL_DSN and adapts the database/KanbanStore to champion's
// interface via pgKanbanStore.
func buildKanbanStore(kind string) (champion.KanbanStore, error) {
	switch kind {
	case "memory", "":
		return nil, nil
	case "pg", "postgres", "postgresql":
		dsn := kanbanDSN()
		if dsn == "" {
			return nil, fmt.Errorf("kanban-store=pg requires WELL_KANBAN_DSN or WELL_DSN env var")
		}
		warnIfInsecureDSN(dsn)
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return nil, fmt.Errorf("open postgres for kanban: %w", err)
		}
		db.SetMaxOpenConns(5) // The Well's connection budget (THE_WELL_OPERATIONS.md)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(5 * time.Minute)

		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			db.Close() // #nosec G104 -- cleanup on an error path; the function is already returning the failure that matters
			return nil, fmt.Errorf("ping postgres for kanban: %w", err)
		}
		store, err := newPGKanbanStore(pingCtx, db)
		if err != nil {
			db.Close() // #nosec G104 -- cleanup on an error path; the function is already returning the failure that matters
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "zhen-agentd: connected to PostgreSQL kanban store; schema migrated\n")
		return store, nil
	}
	return nil, fmt.Errorf("unknown kanban-store %q (supported: memory, pg)", kind)
}

// kanbanDSN is the kanban store's DSN: WELL_KANBAN_DSN (the board's own
// writer, app_kanban: the only role that may INSERT kanban_tasks, which
// the scheduler's per-run cards need) when set, else WELL_DSN.
func kanbanDSN() string {
	if v := strings.TrimSpace(os.Getenv("WELL_KANBAN_DSN")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("WELL_DSN"))
}

// warnIfInsecureDSN logs a stderr warning when WELL_DSN looks like it's
// transmitting credentials in cleartext to a non-loopback host. Heuristic,
// not authoritative — the operator can still get it wrong inside layered
// configs we can't see — but the common foot-gun (sslmode=disable to a
// remote host) is loud enough to catch.
func warnIfInsecureDSN(dsn string) {
	low := strings.ToLower(dsn)
	if !strings.Contains(low, "sslmode=disable") {
		return
	}
	// Loopback is fine — local Unix sockets and 127.0.0.1 don't need TLS.
	for _, ok := range []string{"host=/", "host=localhost", "host=127.0.0.1", "host=::1"} {
		if strings.Contains(low, ok) {
			return
		}
	}
	fmt.Fprintf(os.Stderr,
		"zhen-agentd: WARNING — WELL_DSN sets sslmode=disable for a non-loopback host; credentials and audit data are transmitted in cleartext. Set sslmode=require (or stronger) for production.\n")
}

// buildActionStore selects the audit-log backend per the -action-store
// flag. "stderr" (default, dev-mode) prints to stderr. "pg" connects
// to PostgreSQL via the WELL_DSN env var, runs the migration, and
// returns a *pgstore.Store. Unknown backends → error at startup.
func buildActionStore(kind string) (champion.ActionStore, error) {
	switch kind {
	case "stderr", "":
		return &stderrActionStore{}, nil
	case "pg", "postgres", "postgresql":
		dsn := strings.TrimSpace(os.Getenv("WELL_DSN"))
		if dsn == "" {
			return nil, fmt.Errorf("action-store=pg requires WELL_DSN env var")
		}
		warnIfInsecureDSN(dsn)
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		// The Well's connection budget (THE_WELL_OPERATIONS.md): one agent loop.
		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(5 * time.Minute)

		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			db.Close() // #nosec G104 -- cleanup on an error path; the function is already returning the failure that matters
			return nil, fmt.Errorf("ping postgres: %w", err)
		}
		store := pgstore.New(db)
		if err := store.CheckSchema(pingCtx); err != nil {
			db.Close() // #nosec G104 -- cleanup on an error path; the function is already returning the failure that matters
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "zhen-agentd: connected to PostgreSQL audit store; schema checked\n")
		return store, nil
	}
	return nil, fmt.Errorf("unknown action-store %q (supported: stderr, pg)", kind)
}

// --- server: holds the shared agent dependencies ---

type server struct {
	pool        *championPool
	defaultRoot string              // used when request omits project_root
	allowed     map[string]struct{} // absolute project_root → allowed
	retriever   agent.Retriever
	llm         agent.LLM

	confirmQueue claudebridge.ConfirmQueue   // nil: agent-loop writes are denied
	confirmPub   claudebridge.JSONPublisher  // zhen.confirm.pending notices
	confirmAudit claudebridge.ConfirmAuditor // confirm_issue events
	// askTokens restores the legacy /ask behaviour (tokens in the trace).
	askTokens bool
	vorURL    string
	llamaURL  string
	ready     *readyTracker
	memory    champion.MemoryStore // nil: /api/v1/memory/* answer 503
	sched     *scheduler           // nil: -scheduler off; /api/v1/scheduler/* answer 503
}

// championPool caches *champion.Champion instances keyed by absolute
// project_root, so subsequent requests for the same root reuse the
// same Champion (and therefore the same SnapshotStore-bound revert
// surface, ActionStore audit thread, etc.). The pool is unbounded —
// rely on the allow-list to bound the number of distinct roots.
type championPool struct {
	mu     sync.Mutex
	store  champion.ActionStore
	kanban champion.KanbanStore // nil → kanban_* tool calls fail at dispatch
	memory champion.MemoryStore // nil → memory_write fails at dispatch
	cache  map[string]*champion.Champion
	// gcCtx, when set, starts each new Champion's confirmation-token GC
	// (RT-04); it ends with the daemon.
	gcCtx   context.Context
	startGC func(context.Context, *champion.Champion) // tests replace it
	// sched, when set, is what schedule_update changes (-scheduler).
	sched champion.ScheduleUpdater
}

// setScheduler hands schedule_update to u on every Champion, cached or new.
func (p *championPool) setScheduler(u champion.ScheduleUpdater) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sched = u
	for _, c := range p.cache {
		c.WithScheduleUpdater(u)
	}
}

func newChampionPool(store champion.ActionStore, kanban champion.KanbanStore) *championPool {
	return &championPool{
		store:  store,
		kanban: kanban,
		cache:  make(map[string]*champion.Champion),
		startGC: func(ctx context.Context, c *champion.Champion) {
			c.StartConfirmGC(ctx, champion.DefaultConfirmGCInterval)
		},
	}
}

// get returns the Champion for the given absolute project_root,
// constructing one on first call. The caller is responsible for
// having validated `root` against the allow-list before calling.
func (p *championPool) get(root string) (*champion.Champion, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.cache[root]; ok {
		return c, nil
	}
	c, err := champion.New(champion.Config{ProjectRoot: root}, p.store)
	if err != nil {
		return nil, err
	}
	if p.kanban != nil {
		c = c.WithKanbanStore(p.kanban)
	}
	if p.memory != nil {
		c = c.WithMemoryStore(p.memory)
	}
	c = c.WithModelSwapObserver(observeModelSwap)
	if p.sched != nil {
		c = c.WithScheduleUpdater(p.sched)
	}
	if p.gcCtx != nil && p.startGC != nil {
		p.startGC(p.gcCtx, c)
	}
	p.cache[root] = c
	return c, nil
}

// askRequest is the request body for POST /api/v1/agent/ask.
type askRequest struct {
	Goal        string  `json:"goal"`
	ProjectRoot string  `json:"project_root,omitempty"` // overrides default; must equal server's project_root for now
	SessionID   string  `json:"session_id,omitempty"`
	K           int     `json:"k,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	MaxTurns    int     `json:"max_turns,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	Seed        int     `json:"seed,omitempty"`
}

// askResponse mirrors agent.Result + the session id.
type askResponse struct {
	Answer    string       `json:"answer"`
	TurnsUsed int          `json:"turns_used"`
	BudgetHit bool         `json:"budget_hit"`
	Trace     []traceEntry `json:"trace"`
	SessionID string       `json:"session_id"`
}

type traceEntry struct {
	Thought      string `json:"thought,omitempty"`
	Tool         string `json:"tool,omitempty"`
	Args         any    `json:"args,omitempty"`
	Observation  string `json:"observation,omitempty"`
	Answer       string `json:"answer,omitempty"`
	Refused      bool   `json:"refused,omitempty"`
	Pending      bool   `json:"pending_confirmation,omitempty"`
	PendingToken string `json:"pending_token,omitempty"`
	QueueID      string `json:"queue_id,omitempty"`
}

func (s *server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16)) // 64 KiB cap
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read body: %v", err)
		return
	}
	defer r.Body.Close()

	var req askRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode body: %v", err)
		return
	}
	req.Goal = strings.TrimSpace(req.Goal)
	if req.Goal == "" {
		writeJSONError(w, http.StatusBadRequest, "missing %q", "goal")
		return
	}

	// Resolve + allow-check the requested project_root.
	rootReq := strings.TrimSpace(req.ProjectRoot)
	if rootReq == "" {
		rootReq = s.defaultRoot
	}
	rootAbs, err := filepath.Abs(rootReq)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "resolve project_root: %v", err)
		return
	}
	if _, ok := s.allowed[rootAbs]; !ok {
		writeJSONError(w, http.StatusForbidden,
			"project_root %q is not in the daemon's allow-list (use -allowed-roots to add it)",
			rootAbs)
		return
	}

	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("anon-%s", time.Now().UTC().Format("20060102T150405Z"))
	}

	champ, err := s.pool.get(rootAbs)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "champion init: %v", err)
		return
	}

	cfg := agent.Config{
		MaxTurns:      pickInt(req.MaxTurns, defaultMaxTurns),
		MaxTokens:     pickInt(req.MaxTokens, defaultMaxTokens),
		RetrieveK:     pickInt(req.K, defaultK),
		MaxTopicChars: defaultMaxChars,
		Temperature:   pickFloat(req.Temperature, 0.4),
		Seed:          req.Seed,
		Principal:     requestPrincipal(r), // RT-03: pending tokens bind to the caller
	}
	if !s.askTokens {
		cfg.OnPending = s.askPending(champ, req.SessionID)
	}
	a := agent.New(cfg, champ, s.retriever, s.llm)

	res, err := a.Run(withRunID(r.Context()), req.Goal)
	if err != nil {
		agentRunsTotal.WithLabelValues("error").Inc()
		writeJSONError(w, http.StatusInternalServerError, "agent.Run: %v", err)
		return
	}

	// Metrics: agent run outcome + confirm tokens issued in this run.
	switch {
	case res.BudgetHit:
		agentRunsTotal.WithLabelValues("budget_hit").Inc()
	default:
		agentRunsTotal.WithLabelValues("answer").Inc()
	}
	agentTurns.Observe(float64(res.TurnsUsed))
	for _, t := range res.Trace {
		if t.PendingToken != "" {
			confirmTokensIssued.Inc()
		}
	}

	resp := askResponse{
		Answer:    res.Answer,
		TurnsUsed: res.TurnsUsed,
		BudgetHit: res.BudgetHit,
		SessionID: req.SessionID,
		Trace:     traceFor(res.Trace),
	}
	writeJSON(w, http.StatusOK, resp)
}

func traceFor(turns []agent.Turn) []traceEntry {
	out := make([]traceEntry, 0, len(turns))
	for _, t := range turns {
		e := traceEntry{
			Thought:      t.Thought,
			Observation:  t.Observation,
			Answer:       t.Answer,
			Refused:      t.Refused,
			Pending:      t.Pending,
			PendingToken: t.PendingToken,
			QueueID:      t.QueueID,
		}
		if t.ToolCall != nil {
			e.Tool = t.ToolCall.Name
			e.Args = t.ToolCall.Args
		}
		out = append(out, e)
	}
	return out
}

// handleAskStream is the SSE variant of handleAsk. Same request body
// (askRequest), same gate semantics — but per-turn output streams as
// Server-Sent Events while the agent loop is still running.
//
// Wire format:
//
//	event: turn
//	data: {<TraceEntry json>}
//
//	event: turn
//	data: {<TraceEntry json>}
//
//	event: done
//	data: {"answer":"...","turns_used":N,"budget_hit":false,"session_id":"..."}
//
// Or on error:
//
//	event: error
//	data: {"error":"..."}
//
// Clients can disconnect mid-stream; the agent loop notices the
// context cancel via the request context and stops at the next
// turn boundary. If the connection drops between turns, the
// callback's bool return is false and the loop bails.
//
// Method is POST (request body carries the goal). SSE convention is
// usually GET, but POST-with-streaming is well-supported by
// fetch+ReadableStream and EventSource alternatives like the SSE
// portion of @microsoft/fetch-event-source.
func (s *server) handleAskStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read body: %v", err)
		return
	}
	defer r.Body.Close()

	var req askRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode body: %v", err)
		return
	}
	req.Goal = strings.TrimSpace(req.Goal)
	if req.Goal == "" {
		writeJSONError(w, http.StatusBadRequest, "missing %q", "goal")
		return
	}

	rootReq := strings.TrimSpace(req.ProjectRoot)
	if rootReq == "" {
		rootReq = s.defaultRoot
	}
	rootAbs, err := filepath.Abs(rootReq)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "resolve project_root: %v", err)
		return
	}
	if _, ok := s.allowed[rootAbs]; !ok {
		writeJSONError(w, http.StatusForbidden,
			"project_root %q is not in the daemon's allow-list", rootAbs)
		return
	}

	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("anon-%s", time.Now().UTC().Format("20060102T150405Z"))
	}

	champ, err := s.pool.get(rootAbs)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "champion init: %v", err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming not supported by this connection")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)

	emit := func(event string, payload any) bool {
		buf, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		// SSE frame format:
		//   event: <name>\n
		//   data: <single-line json>\n
		//   \n
		_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, buf)
		if err != nil {
			return false
		}
		flusher.Flush()
		// Check whether the client disconnected.
		select {
		case <-r.Context().Done():
			return false
		default:
			return true
		}
	}

	cfg := agent.Config{
		MaxTurns:      pickInt(req.MaxTurns, defaultMaxTurns),
		MaxTokens:     pickInt(req.MaxTokens, defaultMaxTokens),
		RetrieveK:     pickInt(req.K, defaultK),
		MaxTopicChars: defaultMaxChars,
		Temperature:   pickFloat(req.Temperature, 0.4),
		Seed:          req.Seed,
		Principal:     requestPrincipal(r), // RT-03: pending tokens bind to the caller
	}
	if !s.askTokens {
		cfg.OnPending = s.askPending(champ, req.SessionID)
	}
	a := agent.New(cfg, champ, s.retriever, s.llm)

	res, err := a.Stream(withRunID(r.Context()), req.Goal, func(t agent.Turn) bool {
		entry := traceEntryFromAgent(t)
		return emit("turn", entry)
	})
	if err != nil {
		emit("error", map[string]string{"error": err.Error()})
		return
	}

	// Surface the final summary as a "done" event. Clients use this
	// as the loop terminator — equivalent to /ask's response body.
	switch {
	case res.BudgetHit:
		agentRunsTotal.WithLabelValues("budget_hit").Inc()
	default:
		agentRunsTotal.WithLabelValues("answer").Inc()
	}
	agentTurns.Observe(float64(res.TurnsUsed))
	for _, t := range res.Trace {
		if t.PendingToken != "" {
			confirmTokensIssued.Inc()
		}
	}

	emit("done", map[string]any{
		"answer":     res.Answer,
		"turns_used": res.TurnsUsed,
		"budget_hit": res.BudgetHit,
		"session_id": req.SessionID,
	})
}

// traceEntryFromAgent converts an agent.Turn into the JSON shape used
// by both /ask and /ask/stream.
func traceEntryFromAgent(t agent.Turn) traceEntry {
	e := traceEntry{
		Thought:      t.Thought,
		Observation:  t.Observation,
		Answer:       t.Answer,
		Refused:      t.Refused,
		Pending:      t.Pending,
		PendingToken: t.PendingToken,
		QueueID:      t.QueueID,
	}
	if t.ToolCall != nil {
		e.Tool = t.ToolCall.Name
		e.Args = t.ToolCall.Args
	}
	return e
}

// confirmRequest is the request body for POST /api/v1/agent/confirm.
type confirmRequest struct {
	// Token is the single-use confirmation token returned by an
	// earlier /api/v1/agent/ask response (Trace[i].PendingToken).
	Token string `json:"token"`
	// ProjectRoot is the same root the original tool call targeted.
	// Required: confirms which Champion in the pool redeems the token
	// (different roots have different ChampionPools instances; tokens
	// are scoped to the Champion that issued them).
	ProjectRoot string `json:"project_root"`
}

// confirmResponse mirrors the underlying tool call's typed return:
// some tool calls produce strings (read_file), some produce structured
// data (kanban_list), some produce nothing (write_file).
type confirmResponse struct {
	Result any    `json:"result,omitempty"`
	Status string `json:"status"` // "ok" | "expired" | "unknown" | "used" | "denied" | "error"
	Reason string `json:"reason,omitempty"`
}

func (s *server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<14)) // 16 KiB cap (tokens are tiny)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read body: %v", err)
		return
	}
	defer r.Body.Close()

	var req confirmRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode body: %v", err)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeJSONError(w, http.StatusBadRequest, "missing %q", "token")
		return
	}
	rootReq := strings.TrimSpace(req.ProjectRoot)
	if rootReq == "" {
		rootReq = s.defaultRoot
	}
	rootAbs, err := filepath.Abs(rootReq)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "resolve project_root: %v", err)
		return
	}
	if _, ok := s.allowed[rootAbs]; !ok {
		writeJSONError(w, http.StatusForbidden,
			"project_root %q is not in the daemon's allow-list",
			rootAbs)
		return
	}

	champ, err := s.pool.get(rootAbs)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "champion init: %v", err)
		return
	}

	// RT-03: the token is bound to the principal that caused it; only an
	// authenticated caller can be that principal.
	principal := requestPrincipal(r)
	if principal == anonymousPrincipal {
		confirmTokensRedeemed.WithLabelValues("denied").Inc()
		writeJSON(w, http.StatusForbidden, confirmResponse{
			Status: "denied",
			Reason: "confirmation requires an authenticated principal",
		})
		return
	}

	out, err := champ.ConfirmPendingToolCall(r.Context(), req.Token, principal)
	if err != nil {
		// Classify the error for metrics + machine-readable status.
		status := classifyConfirmError(err)
		confirmTokensRedeemed.WithLabelValues(status).Inc()
		// Token failures (unknown/used/expired) are 400-ish — the
		// client gave a stale token. Destructive-blocked / path-denied
		// after confirm are 403 — the gate refused even after consent.
		code := http.StatusBadRequest
		if status == "denied" {
			code = http.StatusForbidden
		}
		writeJSON(w, code, confirmResponse{
			Status: status,
			Reason: err.Error(),
		})
		return
	}
	confirmTokensRedeemed.WithLabelValues("ok").Inc()
	writeJSON(w, http.StatusOK, confirmResponse{
		Result: out,
		Status: "ok",
	})
}

// classifyConfirmError maps an error from ConfirmPendingToolCall to a
// short status string used for the response and metrics labels.
func classifyConfirmError(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, champion.ErrConfirmerNotAllowed) {
		return "denied"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "expired"):
		return "expired"
	case strings.Contains(msg, "already used"):
		return "used"
	case strings.Contains(msg, "unknown"):
		return "unknown"
	// A nil confirmStore (no token has ever been issued for this Champion)
	// is indistinguishable from "unknown token" from the client's POV.
	case strings.Contains(msg, "no pending confirmations"):
		return "unknown"
	case strings.Contains(msg, "destructive"):
		return "denied"
	case strings.Contains(msg, "path-allowlist"):
		return "denied"
	}
	return "error"
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := s.ready.Status(r.Context())
	if st.Ready {
		writeJSON(w, http.StatusOK, st)
	} else {
		writeJSON(w, http.StatusServiceUnavailable, st)
	}
}

// --- readyTracker: fronts vor + llama-server health checks with a TTL cache ---

type readyTracker struct {
	httpClient *http.Client
	vorURL     string
	llamaURL   string

	mu     sync.Mutex
	last   readyStatus
	lastAt time.Time
	ttl    time.Duration
}

type readyStatus struct {
	Ready  bool   `json:"ready"`
	Vor    bool   `json:"vor_reachable"`
	Llama  bool   `json:"llama_reachable"`
	Detail string `json:"detail,omitempty"`
}

func newReadyTracker(c *http.Client, vorURL, llamaURL string) *readyTracker {
	return &readyTracker{
		httpClient: c,
		vorURL:     vorURL,
		llamaURL:   llamaURL,
		ttl:        5 * time.Second,
	}
}

func (rt *readyTracker) Status(ctx context.Context) readyStatus {
	rt.mu.Lock()
	if time.Since(rt.lastAt) < rt.ttl {
		st := rt.last
		rt.mu.Unlock()
		return st
	}
	rt.mu.Unlock()

	vorOk := pingHTTP(ctx, rt.httpClient, rt.vorURL+"/api/health")
	llamaOk := pingHTTP(ctx, rt.httpClient, rt.llamaURL+"/health")
	st := readyStatus{
		Ready: vorOk && llamaOk,
		Vor:   vorOk,
		Llama: llamaOk,
	}
	if !st.Ready {
		var missing []string
		if !vorOk {
			missing = append(missing, "vor")
		}
		if !llamaOk {
			missing = append(missing, "llama-server")
		}
		st.Detail = "unreachable: " + strings.Join(missing, ", ")
	}

	rt.mu.Lock()
	rt.last = st
	rt.lastAt = time.Now()
	rt.mu.Unlock()
	return st
}

func pingHTTP(ctx context.Context, c *http.Client, endpoint string) bool {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(pingCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == 200
}

// --- vorRetriever / llamaLLM / stderrActionStore — same as zhen-agent.
// Duplicated here rather than refactored into a shared package; the query
// shaping and topic ordering they share live in pkg/vorquery.

type vorRetriever struct {
	client   *http.Client
	baseURL  string
	maxChars int
	index    *vorquery.Index // topic names, for vorquery's name matching
}

func (r *vorRetriever) Retrieve(ctx context.Context, query string, k int) ([]champion.Reference, []agent.TopicContent, error) {
	terms := vorquery.SearchTerms(query)
	q := strings.Join(terms, " ")
	endpoint := fmt.Sprintf("%s/api/search?q=%s",
		strings.TrimRight(r.baseURL, "/"), url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	var hits []vorSearchHit
	if err := json.NewDecoder(resp.Body).Decode(&hits); err != nil {
		return nil, nil, fmt.Errorf("decode: %w", err)
	}

	searched := make([]vorquery.Topic, len(hits))
	for i, h := range hits {
		searched[i] = vorquery.Topic{Name: h.Topic, Category: h.Category}
	}
	order, err := r.index.Order(ctx, terms, searched)
	if err != nil {
		fmt.Fprintf(os.Stderr, "zhen-agentd: vor topic listing: %v\n", err)
	}

	seen := make(map[string]struct{})
	var refs []champion.Reference
	var contents []agent.TopicContent
	for _, name := range order {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		t, err := r.topic(ctx, name)
		if err != nil {
			continue
		}
		ref := champion.Reference{
			Topic: t.Name, Category: t.Category,
			SourceKind: t.SourceKind, SourceTrust: t.SourceTrust,
			SourcePath: t.SourcePath, SourceLabel: t.SourceLabel,
		}
		refs = append(refs, ref)
		contents = append(contents, agent.TopicContent{Ref: ref, Content: clipContent(t.Content, r.maxChars)})
		if len(refs) >= k {
			break
		}
	}
	return refs, contents, nil
}

func (r *vorRetriever) topic(ctx context.Context, name string) (*vorTopic, error) {
	endpoint := fmt.Sprintf("%s/api/topics/%s",
		strings.TrimRight(r.baseURL, "/"), url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	var t vorTopic
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &t, nil
}

type vorSearchHit struct {
	Topic       string `json:"topic"`
	Category    string `json:"category"`
	SourceKind  string `json:"source_kind"`
	SourceTrust string `json:"source_trust"`
	SourcePath  string `json:"source_path,omitempty"`
	SourceLabel string `json:"source_label,omitempty"`
}

type vorTopic struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	SourceKind  string `json:"source_kind"`
	SourceTrust string `json:"source_trust"`
	SourcePath  string `json:"source_path,omitempty"`
	SourceLabel string `json:"source_label,omitempty"`
}

type llamaLLM struct {
	client  *http.Client
	baseURL string
	model   string
	audit   claudebridge.Auditor // nil: not audited
	gpuLock string               // shared GPU lease per call ("" = none)
}

// Complete asks llama-server once and, when auditing is on, records the
// request (tokens, duration, outcome; never the text).
func (l *llamaLLM) Complete(ctx context.Context, msgs []agent.Message, maxTokens int, temp float64, seed int) (string, error) {
	t0 := time.Now()
	text, u, err := l.complete(ctx, msgs, maxTokens, temp, seed)
	if l.audit != nil {
		rec := claudebridge.RequestRecord{
			Conversation: runIDFrom(ctx), Model: l.model, ServedBy: u.Model, StopReason: u.FinishReason,
			InputTokens: u.PromptTokens, OutputTokens: u.CompletionTokens,
			Duration: time.Since(t0), Sent: true, Backend: claudebridge.BackendLocal,
		}
		if err != nil {
			rec.Err = err.Error()
		}
		l.audit.Request(ctx, rec)
	}
	return text, err
}

// llamaUsage is what an OpenAI-compatible completion reports besides text.
type llamaUsage struct {
	Model            string
	FinishReason     string
	PromptTokens     int64
	CompletionTokens int64
}

func (l *llamaLLM) complete(ctx context.Context, msgs []agent.Message, maxTokens int, temp float64, seed int) (string, llamaUsage, error) {
	lm := make([]localllm.Message, len(msgs))
	for i, m := range msgs {
		lm[i] = localllm.Message{Role: m.Role, Content: m.Content}
	}
	c := &localllm.Client{BaseURL: l.baseURL, Model: l.model, HTTP: l.client, Timeout: httpTimeout, GPULock: l.gpuLock}
	a, err := c.Chat(ctx, localllm.Request{Messages: lm, MaxTokens: maxTokens, Temperature: temp, Seed: &seed})
	u := llamaUsage{Model: a.Model, FinishReason: a.FinishReason, PromptTokens: a.PromptTokens, CompletionTokens: a.CompletionTokens}
	if err != nil {
		return "", u, err
	}
	return a.Text, u, nil
}

type stderrActionStore struct {
	mu     sync.Mutex
	nextID int64
}

func (s *stderrActionStore) LogAction(_ context.Context, a *champion.Action) (int64, error) {
	s.mu.Lock()
	s.nextID++
	a.ID = s.nextID
	id := s.nextID
	s.mu.Unlock()
	fmt.Fprintf(os.Stderr, "[champion] log #%d: %s — %s\n", id, a.ActionType, a.Intent)
	return id, nil
}

func (s *stderrActionStore) UpdateAction(_ context.Context, id int64, status, _, errMsg string, _ int) error {
	if errMsg != "" {
		fmt.Fprintf(os.Stderr, "[champion] log #%d: %s (%s)\n", id, status, errMsg)
	} else {
		fmt.Fprintf(os.Stderr, "[champion] log #%d: %s\n", id, status)
	}
	return nil
}

func (s *stderrActionStore) GetActions(_ context.Context, _ string, _ int) ([]champion.Action, error) {
	return nil, nil
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		fmt.Fprintf(os.Stderr, "zhen-agentd: encode response: %v\n", err)
	}
}

func writeJSONError(w http.ResponseWriter, code int, format string, args ...any) {
	writeJSON(w, code, map[string]string{"error": fmt.Sprintf(format, args...)})
}

func pickInt(have, fallback int) int {
	if have > 0 {
		return have
	}
	return fallback
}

func pickFloat(have, fallback float64) float64 {
	if have > 0 {
		return have
	}
	return fallback
}

func clipContent(s string, maxChars int) string {
	if maxChars <= 0 || len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "\n\n…[truncated]"
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "zhen-agentd: "+format+"\n", args...)
	os.Exit(1)
}
