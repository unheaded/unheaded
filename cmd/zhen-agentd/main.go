// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

// Command zhen-agentd is the long-running HTTP daemon variant of
// zhen-agent. Same agent runtime (pkg/agent + pkg/champion + cs/vor +
// llama-server backends) but exposed over HTTP for multi-request use:
//
//   POST /api/v1/agent/ask {goal, project_root, session_id?, ...}
//     → {answer, turns_used, budget_hit, trace[], session_id}
//
//   GET  /health    — liveness probe (always returns 200 once up)
//   GET  /ready     — readiness probe (verifies vor + llama-server reachable)
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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"unheaded/pkg/agent"
	"unheaded/pkg/auth"
	"unheaded/pkg/champion"
	"unheaded/pkg/champion/pgstore"
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
		port        = flag.Int("port", defaultPort, "HTTP listen port")
		host        = flag.String("host", "127.0.0.1", "HTTP listen host (use 0.0.0.0 for non-loopback)")
		projectRoot  = flag.String("project-root", "", "default Champion sandbox root used when a request omits project_root (default: cwd)")
		allowedRoots = flag.String("allowed-roots", "", "comma-separated list of project roots requests may target; defaults to just -project-root")
		actionStore  = flag.String("action-store", "stderr", "audit-log backend: stderr (dev) | pg (production via The Well; requires WELL_DSN env)")
		rateRPS      = flag.Float64("rate-limit", 0, "per-IP requests/sec (0 disables; recommended ~5 for unauthenticated, higher behind auth)")
		rateBurst    = flag.Int("rate-burst", 10, "per-IP rate-limit burst capacity (only used when -rate-limit > 0)")
	)
	flag.Parse()

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

	rawStore, err := buildActionStore(*actionStore)
	if err != nil {
		fatal("action-store init: %v", err)
	}
	// Wrap with the metrics decorator so every gate decision shows up
	// in /metrics under zhen_agentd_champion_actions_total.
	store := newMetricsActionStore(rawStore)
	pool := newChampionPool(store)

	httpClient := &http.Client{Timeout: httpTimeout}
	srv := &server{
		pool:        pool,
		defaultRoot: rootAbs,
		allowed:     allowed,
		retriever:   &vorRetriever{client: httpClient, baseURL: vorURL, maxChars: defaultMaxChars},
		llm:         &llamaLLM{client: httpClient, baseURL: llamaURL, model: modelName},
		vorURL:      vorURL,
		llamaURL:    llamaURL,
		ready:       newReadyTracker(httpClient, vorURL, llamaURL),
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/health", instrument("/health", http.HandlerFunc(srv.handleHealth)))
	mux.Handle("/ready", instrument("/ready", http.HandlerFunc(srv.handleReady)))
	mux.Handle("/api/v1/agent/ask", instrument("/api/v1/agent/ask", http.HandlerFunc(srv.handleAsk)))
	mux.Handle("/api/v1/agent/ask/stream", instrument("/api/v1/agent/ask/stream", http.HandlerFunc(srv.handleAskStream)))
	mux.Handle("/api/v1/agent/confirm", instrument("/api/v1/agent/confirm", http.HandlerFunc(srv.handleConfirm)))
	mux.Handle("/api/v1/openapi.json", instrument("/api/v1/openapi.json", http.HandlerFunc(srv.handleOpenAPI)))

	// Outer middleware chain (innermost-last):
	//   rate-limit  →  auth  →  mux
	// Auth wraps the mux so /metrics is exempted via SkipAuthPaths.
	// Rate-limit is outermost so a flood of unauthenticated requests
	// gets dropped before the auth path-skip check + verification.
	authCfg := auth.LoadServiceAuthConfig("zhen-agentd")
	authMW := auth.SetupMiddleware(authCfg)
	var handler http.Handler = mux
	if authMW != nil {
		fmt.Fprintf(os.Stderr, "zhen-agentd: auth middleware enabled\n")
		// SetupMiddleware already exempts /health, /ready, /metrics.
		// Layer an additional skip for /api/v1/openapi.json so clients
		// can discover the spec (and how to authenticate) without
		// auth — same posture as /metrics.
		extendedAuth := auth.SkipAuthPaths(authMW, "/api/v1/openapi.json")
		handler = extendedAuth(mux)
	} else {
		fmt.Fprintf(os.Stderr, "zhen-agentd: auth middleware DISABLED (set AUTH_ENABLED=true to enable)\n")
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

	// Graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "zhen-agentd: shutdown: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "zhen-agentd: clean shutdown")
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
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		// Sensible pool defaults — same as pkg/database/connect.go.
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)

		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			db.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		}
		store := pgstore.New(db)
		if err := store.Migrate(pingCtx); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply schema: %w", err)
		}
		fmt.Fprintf(os.Stderr, "zhen-agentd: connected to PostgreSQL audit store; schema migrated\n")
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
	vorURL      string
	llamaURL    string
	ready       *readyTracker
}

// championPool caches *champion.Champion instances keyed by absolute
// project_root, so subsequent requests for the same root reuse the
// same Champion (and therefore the same SnapshotStore-bound revert
// surface, ActionStore audit thread, etc.). The pool is unbounded —
// rely on the allow-list to bound the number of distinct roots.
type championPool struct {
	mu    sync.Mutex
	store champion.ActionStore
	cache map[string]*champion.Champion
}

func newChampionPool(store champion.ActionStore) *championPool {
	return &championPool{
		store: store,
		cache: make(map[string]*champion.Champion),
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
	}
	a := agent.New(cfg, champ, s.retriever, s.llm)

	res, err := a.Run(r.Context(), req.Goal)
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
//   event: turn
//   data: {<TraceEntry json>}
//
//   event: turn
//   data: {<TraceEntry json>}
//
//   event: done
//   data: {"answer":"...","turns_used":N,"budget_hit":false,"session_id":"..."}
//
// Or on error:
//
//   event: error
//   data: {"error":"..."}
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
	}
	a := agent.New(cfg, champ, s.retriever, s.llm)

	res, err := a.Stream(r.Context(), req.Goal, func(t agent.Turn) bool {
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

	out, err := champ.ConfirmPendingToolCall(r.Context(), req.Token)
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
	msg := err.Error()
	switch {
	case strings.Contains(msg, "expired"):
		return "expired"
	case strings.Contains(msg, "already used"):
		return "used"
	case strings.Contains(msg, "unknown"):
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

	mu      sync.Mutex
	last    readyStatus
	lastAt  time.Time
	ttl     time.Duration
}

type readyStatus struct {
	Ready    bool   `json:"ready"`
	Vor      bool   `json:"vor_reachable"`
	Llama    bool   `json:"llama_reachable"`
	Detail   string `json:"detail,omitempty"`
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
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == 200
}

// --- vorRetriever / llamaLLM / stderrActionStore — same as zhen-agent.
// Duplicated here rather than refactored into a shared package; if a
// third caller appears, factor out then.

type vorRetriever struct {
	client   *http.Client
	baseURL  string
	maxChars int
}

func (r *vorRetriever) Retrieve(ctx context.Context, query string, k int) ([]champion.Reference, []agent.TopicContent, error) {
	q := strings.TrimRight(query, "?!.")
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

	seen := make(map[string]struct{})
	var refs []champion.Reference
	var contents []agent.TopicContent
	for _, h := range hits {
		if _, dup := seen[h.Topic]; dup {
			continue
		}
		seen[h.Topic] = struct{}{}
		t, err := r.topic(ctx, h.Topic)
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
}

func (l *llamaLLM) Complete(ctx context.Context, msgs []agent.Message, maxTokens int, temp float64, seed int) (string, error) {
	cm := make([]map[string]string, len(msgs))
	for i, m := range msgs {
		cm[i] = map[string]string{"role": m.Role, "content": m.Content}
	}
	body, err := json.Marshal(map[string]any{
		"model":       l.model,
		"messages":    cm,
		"max_tokens":  maxTokens,
		"temperature": temp,
		"seed":        seed,
		"stream":      false,
	})
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(l.baseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return cr.Choices[0].Message.Content, nil
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
