// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the GNU General Public License v3.0 or later.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package main provides the kanban-app server
// Serves the Kanban dashboard and proxies to TimeGuru API
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/database"
	"unheaded/pkg/discovery"
	"unheaded/pkg/healthreport"
	"unheaded/pkg/httputil"
	"unheaded/pkg/logagg"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
)

// Package-level logger instance
var log = logger.New(os.Stderr)

//go:embed all:static
var staticFiles embed.FS

// Config holds server configuration
type Config struct {
	Port            string
	TimeGuruAddr    string
	WotanAddr       string
	DataDir         string // Directory for SQLite persistence (default: ./data)
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Task represents a kanban task
type Task struct {
	ID          string     `json:"id"`
	GUID        string     `json:"guid,omitempty"` // Unique UUID for cross-referencing and linking
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	Type        string     `json:"type,omitempty"`
	Owner       string     `json:"owner,omitempty"`
	Progress    int        `json:"progress,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`  // Soft delete timestamp (nil = active)
	ArchivedAt  *time.Time `json:"archived_at,omitempty"` // Archive timestamp (nil = not archived)
	Commits     string     `json:"commits,omitempty"`     // JSONB array of linked git commits
}

// Server holds the HTTP server and dependencies
type Server struct {
	config          Config
	httpServer      *http.Server
	store           TaskStore // SQLite or Postgres persistence
	tasks           []Task    // DEPRECATED: Use store or taskManager instead
	tasksMu         sync.RWMutex
	sseClients      map[chan []byte]bool
	sseMu           sync.RWMutex
	taskManager     *TaskManager            // Wotan-integrated task management
	timelineManager *TimelineManager        // Standalone Timeguru HTTP polling fallback
	healthSrv       *transport.HealthServer // Unified transport health
	ctx             context.Context         // server lifecycle context
	cancel          context.CancelFunc      // cancels ctx on shutdown

	// registry produces the /metrics body; this service used to assemble that
	// text with fmt.Fprintf calls.
	registry *metrics.Registry

	// logPublisher, when set, contributes log-forwarding counters.
	logPublisher *logagg.Publisher

	// minter, when set, mints kanban-app's service tokens for the calls it
	// makes (Timeguru's timeline, S-IRON-GATE Phase 6).
	minter *auth.ServiceTokenMinter
}

// initMetrics registers every series this service publishes. Values are read
// at scrape time from the state the server already maintains.
func (s *Server) initMetrics() {
	s.registry = metrics.NewRegistry()

	s.registry.MustRegister(metrics.NewFuncGauge(
		"kanban_tasks_total", "Total tasks", nil, func() float64 {
			if s.taskManager != nil {
				return float64(len(s.taskManager.GetAllTasks()))
			}
			s.tasksMu.RLock()
			defer s.tasksMu.RUnlock()
			return float64(len(s.tasks))
		}))

	s.registry.MustRegister(metrics.NewFuncGauge(
		"kanban_sse_clients_active", "Active SSE client connections", nil, func() float64 {
			s.sseMu.RLock()
			defer s.sseMu.RUnlock()
			return float64(len(s.sseClients))
		}))

	s.registry.MustRegister(metrics.NewFuncGauge(
		"kanban_wotan_enabled", "Whether Wotan integration is enabled", nil, func() float64 {
			if s.taskManager != nil {
				return 1
			}
			return 0
		}))
}

// SetLogPublisher attaches a logagg publisher and registers its counters in
// the registry /metrics serves, replacing a previous publisher's. Optional:
// nil removes them.
func (s *Server) SetLogPublisher(p *logagg.Publisher) error {
	s.tasksMu.Lock()
	defer s.tasksMu.Unlock()
	if s.logPublisher != nil {
		s.logPublisher.UnregisterCollectors(s.registry)
		s.logPublisher = nil
	}
	if p == nil {
		return nil
	}
	if err := p.RegisterCollectors(s.registry); err != nil {
		return err
	}
	s.logPublisher = p
	return nil
}

// NewServer creates a new kanban server with standalone timeline polling.
// Initializes task store: prefers Postgres, falls back to SQLite, then in-memory.
func NewServer(cfg Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		config:     cfg,
		sseClients: make(map[chan []byte]bool),
		ctx:        ctx,
		cancel:     cancel,
	}
	s.initMetrics()

	// Try Postgres first
	pgDB, pgErr := database.Connect(context.Background(), database.AppKanbanConfig())
	pgStore, pgStoreErr := wellStore(pgDB, pgErr)
	switch {
	case pgStoreErr != nil:
		log.Fatal().Err(pgStoreErr).Msg("The Well answers but is not migrated for kanban-app: refusing to start (run well-migrate; no SQLite fallback, it would split the board)")
	case pgStore != nil:
		// The Well is the source of cards; a fresh Well gets the
		// Genesis cards from migration 034, not from this binary.
		s.store = pgStore
		log.Info().Msg("Postgres connected — kanban tasks persisted to The Well (standalone mode)")
	default:
		log.Warn().Err(pgErr).Msg("Postgres unavailable — trying SQLite")
	}

	// SQLite fallback
	if s.store == nil {
		dbPath := cfg.DataDir + "/kanban.db"
		if cfg.DataDir == "" {
			dbPath = "./data/kanban.db"
		}
		store, err := openSQLiteStore(dbPath)
		if err != nil {
			// No database: an empty, read-only board.
			log.Error().Err(err).Str("path", dbPath).Msg("failed to initialize SQLite store — empty read-only board")
		} else {
			s.store = store
		}
	}

	// Create standalone TimelineManager for direct Timeguru HTTP polling
	s.timelineManager = NewTimelineManager(func(eventType string, data interface{}) {
		s.broadcastUpdate(eventType, data)
	})
	return s
}

// NewServerWithTaskManager creates a server with Wotan integration and shared Store.
func NewServerWithTaskManager(cfg Config, tm *TaskManager, store TaskStore) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		config:      cfg,
		store:       store,
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         ctx,
		cancel:      cancel,
	}
	s.initMetrics()
	return s
}

// buildHandler is the mux behind the middleware stack, with auth from
// authCfg. With AUTH_ENABLED=true every read stays open (the board, its
// static UI, SSE: CLAUDE.md, The Meta Moment), and a write needs a token
// from an AUTH_ALLOW_CALLERS caller or an operator user token.
func (s *Server) buildHandler(authCfg auth.ServiceAuthConfig) (http.Handler, error) {
	mux := http.NewServeMux()
	// API routes - tasks at canonical /api/v1/tasks
	mux.HandleFunc("/api/v1/tasks", s.handleTasks)
	mux.HandleFunc("/api/v1/tasks/", s.handleTaskByID)      // For /tasks/:id
	mux.HandleFunc("/api/v1/timeline/tasks", s.handleTasks) // Legacy compatibility
	mux.HandleFunc("/api/v1/stream", s.handleSSE)
	mux.HandleFunc("/ws", s.handleWebSocket) // WebSocket endpoint
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/health", httputil.ProbeMethods(s.handleHealth))
	mux.HandleFunc("/ready", httputil.ProbeMethods(s.handleReady))
	mux.HandleFunc("/metrics", s.handleMetrics)

	// Timeline API endpoints - THE META MOMENT
	mux.HandleFunc("/api/v1/timeline", s.handleTimeline)
	mux.HandleFunc("/api/v1/timeline/cards", s.handleTimelineCards)

	// Static files - embedded in binary
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		// Fallback to serving from filesystem during development
		log.Warn().Msg("Using filesystem for static files (development mode)")
		mux.Handle("/", http.FileServer(http.Dir("static")))
	} else {
		log.Info().Msg("Serving embedded static files")
		mux.Handle("/", http.FileServer(http.FS(staticFS)))
	}

	// Apply middleware stack (order matters!)
	var handler http.Handler = mux
	handler = s.loggingMiddleware(handler)                     // Logging (innermost — sees request ID)
	handler = requestIDMiddleware(handler)                     // X-Request-ID injection
	handler = requestSizeLimitMiddleware(1024 * 1024)(handler) // 1MB max request
	handler = securityHeadersMiddleware(handler)               // Security headers

	// Auth (AUTH_ENABLED=true, ADR-103): reads open, writes need a token.
	handler, err = auth.NewServiceHandler(handler, authCfg, auth.ServiceRoutes{OpenReads: []string{"/"}})
	if err != nil {
		return nil, err
	}

	handler = corsMiddleware(handler) // CORS

	// Rate limiting (configurable)
	if getEnv("RATE_LIMIT_ENABLED", "true") == "true" {
		rateLimiter := NewRateLimiter(s.ctx, 120, 30) // 120 req/min, burst 30 (page load is ~15 resources)
		handler = rateLimitMiddleware(rateLimiter)(handler)
		log.Info().Msg("Rate limiting enabled: 120 req/min, burst 30")
	}

	// Outermost, so rate-limited and CORS-rejected requests are counted too.
	handler = httputil.SecurityHeaders(httputil.NewServiceMetrics("kanban").Instrument(mux, handler))
	return handler, nil
}

// Start starts the HTTP server. A configuration auth cannot use is an
// error here, so the service exits instead of serving writes open.
func (s *Server) Start() error {
	handler, err := s.buildHandler(auth.LoadServiceAuthConfig("kanban-app"))
	if err != nil {
		return fmt.Errorf("auth: %w (see ADR-103)", err)
	}

	s.httpServer = &http.Server{
		Addr:           ":" + s.config.Port,
		Handler:        handler,
		ReadTimeout:    s.config.ReadTimeout,
		WriteTimeout:   s.config.WriteTimeout,
		IdleTimeout:    5 * s.config.ReadTimeout,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	log.Info().
		Str("port", s.config.Port).
		Str("timeguru", s.config.TimeGuruAddr).
		Str("wotan", s.config.WotanAddr).
		Bool("wotan_enabled", s.taskManager != nil).
		Msg("Starting kanban-app server")

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	// Cancel server-scoped context (stops rate limiter cleanup, etc.)
	s.cancel()

	// Close all SSE connections
	s.sseMu.Lock()
	for ch := range s.sseClients {
		close(ch)
		delete(s.sseClients, ch)
	}
	s.sseMu.Unlock()

	return s.httpServer.Shutdown(ctx)
}

// getTimelineManager returns the active TimelineManager.
// Prefers TaskManager's timeline (Wotan-backed) when available,
// falls back to standalone HTTP-polling TimelineManager.
func (s *Server) getTimelineManager() *TimelineManager {
	if s.taskManager != nil {
		return s.taskManager.timelineManager
	}
	return s.timelineManager
}

// fetchTimelineFromTimeguru fetches the timeline via HTTP from Timeguru
func (s *Server) fetchTimelineFromTimeguru() error {
	url := fmt.Sprintf("http://%s/timeline", s.config.TimeGuruAddr)

	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("fetch timeline: %w", err)
	}
	if s.minter != nil {
		authz, err := s.minter.AuthorizationHeader("timeguru")
		if err != nil {
			return fmt.Errorf("fetch timeline: service token: %w", err)
		}
		req.Header.Set("Authorization", authz)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch timeline: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("timeguru returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Timeline *Timeline `json:"timeline"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode timeline: %w", err)
	}

	if result.Timeline == nil {
		return fmt.Errorf("timeguru returned nil timeline")
	}

	tm := s.getTimelineManager()
	if tm == nil {
		return fmt.Errorf("no timeline manager available")
	}
	return tm.UpdateTimeline(result.Timeline)
}

// pollTimeguru periodically fetches timeline from Timeguru via HTTP.
// Used as fallback when Wotan is unavailable, or to seed initial data.
func (s *Server) pollTimeguru(ctx context.Context, interval time.Duration) {
	// Initial fetch
	if err := s.fetchTimelineFromTimeguru(); err != nil {
		log.Warn().Err(err).Str("addr", s.config.TimeGuruAddr).Msg("initial timeline fetch failed (will retry)")
	} else {
		tm := s.getTimelineManager()
		var count int
		if tm != nil {
			count = len(tm.GetTimelineTasks())
		}
		log.Info().Int("tasks", count).Msg("loaded timeline from Timeguru via HTTP")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.fetchTimelineFromTimeguru(); err != nil {
				log.Warn().Err(err).Msg("timeline poll failed")
			}
		}
	}
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status
		wrapped := &statusWriter{ResponseWriter: w, status: 200}

		next.ServeHTTP(wrapped, r)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.status).
			Dur("duration", time.Since(start)).
			Str("remote_addr", r.RemoteAddr).
			Str("request_id", getRequestID(r.Context())).
			Msg("request")
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Flush proxies to the underlying ResponseWriter if it supports http.Flusher.
// Required for SSE (Server-Sent Events) and streaming responses.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// writable: a write lands in a database (The Well or SQLite), directly or
// through the TaskManager. Without one the board is read-only: writes used
// to go to memory, answer 201/200, and vanish on restart.
//
// With a TaskManager every write goes through it, so its store decides: main
// hands the server the same store, and TaskManager drops its copy when the
// store fails at startup (seed or load), so the server's copy said writable
// for writes that only reached memory.
func (s *Server) writable() bool {
	if s.taskManager != nil {
		return s.taskManager.store != nil
	}
	return s.store != nil
}

// refuseIfReadOnly answers 503 to a write when there is no database.
func (s *Server) refuseIfReadOnly(w http.ResponseWriter, r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return false // reads, and methods the routers answer 405
	}
	if s.writable() {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{"error": "read-only: no database, so writes are refused"}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
	return true
}

// storeReadFailed answers a failed store lookup: 404 when the task is not
// there, 500 when the store could not say. Every lookup error used to be a
// 404, so a database that was down reported each task as missing.
func storeReadFailed(w http.ResponseWriter, taskID string, err error) {
	if errors.Is(err, ErrStoreNotFound) {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	log.Error().Err(err).Str("task_id", taskID).Msg("task store read failed")
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

// handleTasks routes task operations (GET, POST, PUT, DELETE)
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if s.refuseIfReadOnly(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetTasks(w, r)
	case http.MethodPost:
		s.handleCreateTask(w, r)
	case http.MethodPut:
		s.handleUpdateTask(w, r)
	case http.MethodDelete:
		s.handleDeleteTask(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetTasks returns all tasks
func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	var tasks interface{}
	var count int

	// Each source is accepted only when it actually has rows, and `count > 0`
	// is what decides that — NOT `tasks == nil`. GetAllTasks returns
	// make([]*Task, 0, n), so assigning it into an interface{} yields a
	// non-nil interface holding an empty slice: the old nil test never fired
	// and an empty source permanently shadowed every fallback below it. That
	// is the whole window between startup and Wotan's first delivery, and
	// again after a wotan restart drops subscriptions — the board reads zero
	// while the store still holds the tasks.
	if s.taskManager != nil {
		// Prefer Wotan-backed TaskManager
		if taskList := s.taskManager.GetAllTasks(); len(taskList) > 0 {
			tasks = taskList
			count = len(taskList)
		}
	}

	// Next: timeline tasks from direct Timeguru HTTP polling
	if count == 0 {
		if tm := s.getTimelineManager(); tm != nil {
			if timelineTasks := tm.GetTimelineTasks(); len(timelineTasks) > 0 {
				tasks = timelineTasks
				count = len(timelineTasks)
			}
		}
	}

	// Next: SQLite/Postgres store (L1 persistence). A read error used to be
	// skipped like an empty store, and the board answered 200 with no tasks.
	if count == 0 && s.store != nil {
		storeTasks, err := s.store.GetAllTasks()
		if err != nil {
			log.Error().Err(err).Msg("task store read failed")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if len(storeTasks) > 0 {
			tasks = storeTasks
			count = len(storeTasks)
		}
	}

	// Last resort: in-memory tasks
	if count == 0 {
		s.tasksMu.RLock()
		tasks = s.tasks
		count = len(s.tasks)
		s.tasksMu.RUnlock()
	}

	// Never encode a nil into "tasks": the board expects a JSON array, and
	// `null` is not one. Keyed on count, not `tasks == nil` — s.tasks above
	// may itself be a nil []Task, which lands in the interface as a non-nil
	// typed nil and slips straight past a nil check. Same trap as the
	// fallback chain; it is easy to fall into twice.
	if count == 0 {
		tasks = []Task{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"tasks": tasks,
		"count": count,
	})
}

// handleCreateTask creates a new task (POST)
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate input
	if err := validateTaskInput(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.taskManager != nil {
		// Create task via TaskManager
		ctx := r.Context()
		if err := s.taskManager.CreateTask(ctx, &task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to create task")

			// errors.Is: TaskManager wraps ErrInvalidTask with the reason, so
			// `switch err` never matched it and a bad status was a 500.
			switch {
			case errors.Is(err, ErrTaskAlreadyExists):
				http.Error(w, err.Error(), http.StatusConflict)
			case errors.Is(err, ErrInvalidTask):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		// Standalone + SQLite persistence
		now := time.Now()
		task.CreatedAt = now
		task.UpdatedAt = now
		// SaveTask upserts: a lookup error taken for "absent" let a create
		// overwrite an existing task.
		existing, err := s.store.GetTask(task.ID)
		if err != nil && !errors.Is(err, ErrStoreNotFound) {
			storeReadFailed(w, task.ID, err)
			return
		}
		if existing != nil {
			http.Error(w, "task already exists", http.StatusConflict)
			return
		}
		if err := s.store.SaveTask(&task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to save task to store")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		// Fallback: in-memory only (no persistence)
		now := time.Now()
		task.CreatedAt = now
		task.UpdatedAt = now
		s.tasksMu.Lock()
		for _, t := range s.tasks {
			if t.ID == task.ID {
				s.tasksMu.Unlock()
				http.Error(w, "task already exists", http.StatusConflict)
				return
			}
		}
		s.tasks = append(s.tasks, task)
		s.tasksMu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"task": task,
	})
}

// handleUpdateTask updates an existing task (PUT)
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate input
	if err := validateTaskInput(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.taskManager != nil {
		// Update task via TaskManager
		ctx := r.Context()
		if err := s.taskManager.UpdateTask(ctx, &task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to update task")

			switch {
			case errors.Is(err, ErrTaskNotFound):
				http.Error(w, err.Error(), http.StatusNotFound)
			case errors.Is(err, ErrInvalidTask):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		// Standalone + SQLite persistence
		existing, err := s.store.GetTask(task.ID)
		if err != nil {
			storeReadFailed(w, task.ID, err)
			return
		}
		task.CreatedAt = existing.CreatedAt
		task.UpdatedAt = time.Now()
		if err := s.store.SaveTask(&task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to update task in store")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		// Fallback: in-memory only
		s.tasksMu.Lock()
		found := false
		for i, t := range s.tasks {
			if t.ID == task.ID {
				task.CreatedAt = t.CreatedAt
				task.UpdatedAt = time.Now()
				s.tasks[i] = task
				found = true
				break
			}
		}
		s.tasksMu.Unlock()
		if !found {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"task": task,
	})
}

// handleDeleteTask deletes a task (DELETE)
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	// Parse task ID from query parameter
	taskID := r.URL.Query().Get("id")
	if taskID == "" {
		http.Error(w, "Missing 'id' query parameter", http.StatusBadRequest)
		return
	}

	if s.taskManager != nil {
		// Delete task via TaskManager
		ctx := r.Context()
		if err := s.taskManager.DeleteTask(ctx, taskID); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("failed to delete task")

			switch err {
			case ErrTaskNotFound:
				http.Error(w, err.Error(), http.StatusNotFound)
			case ErrEmptyTaskID:
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		// Standalone + SQLite persistence
		if err := s.store.DeleteTask(taskID); err != nil {
			if errors.Is(err, ErrStoreNotFound) {
				http.Error(w, "task not found", http.StatusNotFound)
			} else {
				log.Error().Err(err).Str("task_id", taskID).Msg("failed to delete task from store")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else {
		// Fallback: in-memory only
		s.tasksMu.Lock()
		found := false
		for i, t := range s.tasks {
			if t.ID == taskID {
				s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
				found = true
				break
			}
		}
		s.tasksMu.Unlock()
		if !found {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"deleted": true,
		"task_id": taskID,
	})
}

// handleTaskByID handles requests to /api/v1/tasks/:id
func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	// Extract task ID from path
	path := r.URL.Path
	prefix := "/api/v1/tasks/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	taskID := strings.TrimPrefix(path, prefix)
	if taskID == "" {
		http.Error(w, "Task ID required", http.StatusBadRequest)
		return
	}

	// Special routes that look like task IDs
	switch taskID {
	case "deleted":
		s.handleDeletedTasks(w, r)
		return
	case "archived":
		s.handleArchivedTasks(w, r)
		return
	case "completed":
		s.handleCompletedTasks(w, r)
		return
	}

	if s.refuseIfReadOnly(w, r) {
		return
	}

	// Handle /tasks/:id/restore
	if strings.HasSuffix(taskID, "/restore") {
		realID := strings.TrimSuffix(taskID, "/restore")
		s.handleRestoreTask(w, r, realID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetTaskByID(w, r, taskID)
	case http.MethodPut:
		s.handleUpdateTaskByID(w, r, taskID)
	case http.MethodDelete:
		s.handleDeleteTaskByID(w, r, taskID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetTaskByID returns a single task
func (s *Server) handleGetTaskByID(w http.ResponseWriter, r *http.Request, taskID string) {
	if s.taskManager != nil {
		task, err := s.taskManager.GetTask(taskID)
		if err != nil {
			if err == ErrTaskNotFound {
				http.Error(w, "Task not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(task) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		return
	}

	// Standalone + SQLite
	if s.store != nil {
		task, err := s.store.GetTask(taskID)
		if err != nil {
			storeReadFailed(w, taskID, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(task) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		return
	}

	// Fallback: in-memory
	s.tasksMu.RLock()
	defer s.tasksMu.RUnlock()
	for _, t := range s.tasks {
		if t.ID == taskID {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(t) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			return
		}
	}
	http.Error(w, "Task not found", http.StatusNotFound)
}

// handleUpdateTaskByID updates a single task
func (s *Server) handleUpdateTaskByID(w http.ResponseWriter, r *http.Request, taskID string) {
	// Decode partial update — only fields present in JSON will be non-zero
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Get existing task to merge updates into
	var task *Task
	if s.taskManager != nil {
		t, err := s.taskManager.GetTask(taskID)
		if err != nil {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
		task = t
	} else if s.store != nil {
		t, err := s.store.GetTask(taskID)
		if err != nil {
			storeReadFailed(w, taskID, err)
			return
		}
		task = t
	} else {
		s.tasksMu.RLock()
		for i := range s.tasks {
			if s.tasks[i].ID == taskID {
				t := s.tasks[i]
				task = &t
				break
			}
		}
		s.tasksMu.RUnlock()
		if task == nil {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
	}

	// Merge updates into existing task (only overwrite fields that are present)
	if v, ok := updates["title"].(string); ok && v != "" {
		task.Title = v
	}
	if v, ok := updates["description"].(string); ok {
		task.Description = v
	}
	if v, ok := updates["status"].(string); ok && v != "" {
		task.Status = v
	}
	if v, ok := updates["type"].(string); ok && v != "" {
		task.Type = v
	}
	if v, ok := updates["owner"].(string); ok {
		task.Owner = v
	}
	if v, ok := updates["progress"].(float64); ok {
		task.Progress = int(v)
	}
	task.UpdatedAt = time.Now()

	// Save merged task
	if s.taskManager != nil {
		if err := s.taskManager.UpdateTask(r.Context(), task); err != nil {
			if errors.Is(err, ErrInvalidTask) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		if err := s.store.SaveTask(task); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("failed to update task in store")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		s.tasksMu.Lock()
		for i, t := range s.tasks {
			if t.ID == taskID {
				s.tasks[i] = *task
				break
			}
		}
		s.tasksMu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"task": task}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

// handleDeleteTaskByID deletes a single task
func (s *Server) handleDeleteTaskByID(w http.ResponseWriter, r *http.Request, taskID string) {
	if s.taskManager != nil {
		if err := s.taskManager.DeleteTask(r.Context(), taskID); err != nil {
			if err == ErrTaskNotFound {
				http.Error(w, "Task not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		// Standalone + SQLite persistence
		if err := s.store.DeleteTask(taskID); err != nil {
			if errors.Is(err, ErrStoreNotFound) {
				http.Error(w, "Task not found", http.StatusNotFound)
			} else {
				log.Error().Err(err).Str("task_id", taskID).Msg("failed to delete task from store")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else {
		// Fallback: in-memory only
		s.tasksMu.Lock()
		found := false
		for i, t := range s.tasks {
			if t.ID == taskID {
				s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
				found = true
				break
			}
		}
		s.tasksMu.Unlock()
		if !found {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "task_id": taskID}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

// handleDeletedTasks returns soft-deleted tasks.
func (s *Server) handleDeletedTasks(w http.ResponseWriter, r *http.Request) {
	if pgStore, ok := s.store.(*PgStore); ok {
		tasks, err := pgStore.inner.GetDeletedTasks(context.Background())
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		out := make([]Task, len(tasks))
		for i, t := range tasks {
			out[i] = fromPgTask(t)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"tasks": out, "count": len(out)}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"tasks": []Task{}, "count": 0}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

// handleArchivedTasks returns archived tasks.
func (s *Server) handleArchivedTasks(w http.ResponseWriter, r *http.Request) {
	if pgStore, ok := s.store.(*PgStore); ok {
		tasks, err := pgStore.inner.GetArchivedTasks(context.Background())
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		out := make([]Task, len(tasks))
		for i, t := range tasks {
			out[i] = fromPgTask(t)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"tasks": out, "count": len(out)}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"tasks": []Task{}, "count": 0}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

// handleCompletedTasks returns done tasks (still on board but accessible here too).
//
// Done is a status every store has, so this reads whichever holds the board.
// It used to read Postgres only and answer an empty list otherwise: on east
// (SQLite) it said 0 completed while the board had 9 done cards.
func (s *Server) handleCompletedTasks(w http.ResponseWriter, r *http.Request) {
	var all []Task
	switch {
	case s.store != nil:
		tasks, err := s.store.GetAllTasks()
		if err != nil {
			log.Error().Err(err).Msg("completed tasks: store read failed")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		all = tasks
	case s.taskManager != nil:
		for _, t := range s.taskManager.GetAllTasks() {
			all = append(all, *t)
		}
	default:
		s.tasksMu.RLock()
		all = append(all, s.tasks...)
		s.tasksMu.RUnlock()
	}
	out := []Task{}
	for _, t := range all {
		if t.Status == "done" {
			out = append(out, t)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"tasks": out, "count": len(out)}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

// handleRestoreTask un-deletes or un-archives a task.
func (s *Server) handleRestoreTask(w http.ResponseWriter, r *http.Request, taskID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if pgStore, ok := s.store.(*PgStore); ok {
		if err := pgStore.inner.RestoreTask(context.Background(), taskID); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"restored": true, "task_id": taskID}) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		return
	}
	http.Error(w, "Restore not available without PostgreSQL", http.StatusNotImplemented)
}

// handleWebSocket handles WebSocket connections for real-time updates
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// WebSocket upgrade handled by SSE for now (simpler, works everywhere)
	// Full WebSocket implementation can be added later
	s.handleSSE(w, r)
}

// validateTaskInput validates HTTP request task input
func validateTaskInput(task *Task) error {
	if task == nil {
		return errors.New("task cannot be nil")
	}
	if task.ID == "" {
		return errors.New("task.id is required")
	}
	if task.Title == "" {
		return errors.New("task.title is required")
	}
	if task.Status == "" {
		return errors.New("task.status is required")
	}

	// Validate ID format (alphanumeric + hyphens only)
	for _, r := range task.ID {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') { //nolint:staticcheck // QF1001: De Morgan would invert allow-list to forbid-list — less readable for char validation
			return errors.New("task.id contains invalid characters (use alphanumeric, -, _)")
		}
	}

	// Validate title length
	if len(task.Title) > 200 {
		return errors.New("task.title too long (max 200 characters)")
	}

	// Validate description length
	if len(task.Description) > 1000 {
		return errors.New("task.description too long (max 1000 characters)")
	}

	return nil
}

// handleSSE handles Server-Sent Events for real-time updates
// maxSSEClients bounds open /api/v1/stream connections. Each holds a
// goroutine and a channel until the client hangs up, and the rate limit
// (120/min per address) does not bound how many stay open.
const maxSSEClients = 256

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Check if client supports SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// CORS origin set by corsMiddleware — no duplicate wildcard here

	// Create client channel
	clientCh := make(chan []byte, 10)

	s.sseMu.Lock()
	if len(s.sseClients) >= maxSSEClients {
		s.sseMu.Unlock()
		http.Error(w, "too many open streams", http.StatusServiceUnavailable)
		return
	}
	s.sseClients[clientCh] = true
	s.sseMu.Unlock()

	// Cleanup on disconnect
	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients, clientCh)
		s.sseMu.Unlock()
	}()

	// Send initial tasks — prefer kanban cards (GetAllTasks) over timeline milestones
	var initialData []byte
	if s.taskManager != nil {
		// Wotan mode: send real kanban task cards
		allTasks := s.taskManager.GetAllTasks()
		if len(allTasks) > 0 {
			initialData, _ = json.Marshal(allTasks)
			log.Info().Int("count", len(allTasks)).Msg("SSE sending kanban tasks (Wotan)")
		}
	} else if tm := s.getTimelineManager(); tm != nil {
		// Standalone mode: send timeline milestones as tasks
		timelineTasks := tm.GetTimelineTasks()
		if len(timelineTasks) > 0 {
			initialData, _ = json.Marshal(timelineTasks)
			log.Info().Int("count", len(timelineTasks)).Msg("SSE sending timeline tasks (Timeguru HTTP)")
		}
	}
	if initialData == nil {
		// Last resort: hardcoded fallback tasks
		s.tasksMu.RLock()
		initialData, _ = json.Marshal(s.tasks)
		s.tasksMu.RUnlock()
	}

	fmt.Fprintf(w, "event: tasks\ndata: %s\n\n", initialData)
	flusher.Flush()

	// Keep-alive ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Wait for events or disconnect
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-clientCh:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// handleHealth returns health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	timelineSubscribed := false
	if s.taskManager != nil {
		timelineSubscribed = s.taskManager.IsTimelineSubscribed()
	}

	timelineHTTP := false
	if tm := s.getTimelineManager(); tm != nil {
		timelineHTTP = tm.GetTimeline() != nil
	}

	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"status":              "healthy",
		"timestamp":           time.Now().UTC().Format(time.RFC3339),
		"version":             "0.1.0",
		"wotan_enabled":       s.taskManager != nil,
		"timeline_subscribed": timelineSubscribed,
		"timeline_http":       timelineHTTP,
		"timeguru_addr":       s.config.TimeGuruAddr,
	})
}

// handleReady returns readiness status.
// Delegates to the transport HealthServer when available.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.healthSrv != nil {
		status := s.healthSrv.Status()
		w.Header().Set("Content-Type", "application/json")
		if status == transport.StatusDown {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
				"ready":   false,
				"reason":  string(status),
				"service": "kanban-app",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"ready":   true,
			"reason":  string(status),
			"service": "kanban-app",
		})
		return
	}

	// Legacy fallback when healthSrv is not wired
	w.Header().Set("Content-Type", "application/json")

	ready := true
	reason := "ready"

	// In standalone mode (no TaskManager), we're still ready to serve
	if s.taskManager == nil {
		reason = "standalone mode"
	}

	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"ready":   ready,
		"reason":  reason,
		"service": "kanban-app",
	})
}

// handleMetrics returns Prometheus-style metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	// A Server built as a struct literal (tests, embedding callers) has no
	// registry. Guard rather than dereference: a nil-pointer panic here does
	// not lose one series, it takes the whole /metrics endpoint down.
	// #nosec G104 -- response already committed; a write failure here means
	// the client went away and nothing further can be sent.
	//
	// The default registry rides along: go_*, process_* and what linked
	// libraries register at load (pkg/wotan-client) never reached this
	// scrape before ADR-094 step 5. GatherAll skips a nil registry.
	// The log publisher's counters are in s.registry (SetLogPublisher).
	_ = metrics.GatherAll(w, s.registry, metrics.DefaultRegistry)
}

// handleTimeline returns the current timeline data
func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tm := s.getTimelineManager()
	if tm == nil {
		http.Error(w, "Timeline not available", http.StatusServiceUnavailable)
		return
	}

	timeline := tm.GetTimeline()

	w.Header().Set("Content-Type", "application/json")

	if timeline == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"timeline": nil,
			"message":  "No timeline data available yet. Waiting for Timeguru updates.",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"timeline": timeline,
	})
}

// handleTimelineCards returns timeline data converted to Kanban cards
func (s *Server) handleTimelineCards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var tasks interface{}
	var count int
	source := "timeline"

	// Try Wotan-backed TaskManager first
	if s.taskManager != nil {
		timelineTasks := s.taskManager.GetTimelineTasks()
		if len(timelineTasks) > 0 {
			tasks = timelineTasks
			count = len(timelineTasks)
			source = "timeline-wotan"
		}
	}

	// Fallback: standalone TimelineManager (direct Timeguru HTTP)
	if tasks == nil {
		if tm := s.getTimelineManager(); tm != nil {
			timelineTasks := tm.GetTimelineTasks()
			if len(timelineTasks) > 0 {
				tasks = timelineTasks
				count = len(timelineTasks)
				source = "timeline-http"
			}
		}
	}

	// No timeline: no timeline cards. This used to answer with the board's
	// own tasks (source "fallback"), which the board merges with
	// /api/v1/tasks and then announces "Timeline Loaded: Kanban is tracking
	// its own timeline". On east, where timeguru has no timeline, every page
	// load said so over the same 64 tasks.
	if tasks == nil {
		tasks = []*Task{}
		source = "none"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"tasks":  tasks,
		"count":  count,
		"source": source,
	})
}

// broadcastUpdate sends an update to all SSE clients
func (s *Server) broadcastUpdate(eventType string, data interface{}) {
	msg, err := json.Marshal(map[string]interface{}{
		"type": eventType,
		"data": data,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal broadcast message")
		return
	}

	s.sseMu.RLock()
	defer s.sseMu.RUnlock()

	for ch := range s.sseClients {
		select {
		case ch <- msg:
		default:
			// Channel full, skip
		}
	}
}

func main() {
	// Setup logging - use Kingdom's native logger
	log = logger.NewWithConfig(logger.Config{
		Level:         logger.InfoLevel,
		TimeFormat:    time.RFC3339,
		CallerEnabled: true,
		ConsoleMode:   true,
		Output:        os.Stderr,
	})

	// Load config
	cfg := Config{
		Port:            getEnv("PORT", "20001"),
		TimeGuruAddr:    getEnv("TIMEGURU_ADDR", "localhost:19000"),
		WotanAddr:       getEnv("WOTAN_ADDR", "localhost:18000"),
		DataDir:         getEnv("DATA_DIR", "./data"),
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	// Transport config — unified env-based configuration
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)

	// Override transport config with service-level WotanAddr if set
	if cfg.WotanAddr != "" {
		transportCfg.WotanHTTPAddr = "http://" + cfg.WotanAddr
	}

	// Health server — dual-protocol health tracking
	healthSrv := transport.NewHealthServer("kanban-app")

	// Log aggregation publisher — forwards structured logs to Wotan
	var logConn transport.Connection
	if cfg.WotanAddr != "" {
		var err error
		if logConn, err = logagg.Connect(context.Background(), transportCfg, "kanban-app"); err != nil {
			// best-effort: logs stay local, but say why so a nil here is not silent
			log.Warn().Err(err).Msg("log aggregation: no transport connection; logs not forwarded")
			logConn = nil
		}
	}
	logPublisher := logagg.NewPublisher("kanban-app", logConn)
	if logPublisher.Enabled() {
		// LoggerHook forwards the entry's structured fields too; going
		// through Publisher.Run passed the message alone.
		log.AddHook(logPublisher.LoggerHook())
		log.Info().Msg("Log forwarding to Wotan enabled via logagg publisher")
	}

	// Service discovery: announced on Wotan's system.discovery topic (best-effort)
	discoveryCtx, discoveryCancel := context.WithCancel(context.Background())
	defer discoveryCancel()
	discovery.Start(discoveryCtx, transportCfg, "kanban-app", ports.PortOf(":"+cfg.Port, ports.KanbanApp))
	healthreport.StartService(discoveryCtx, transportCfg, "kanban-app", healthreport.HealthURL(":"+cfg.Port, ports.PortOf(":"+cfg.Port, ports.KanbanApp))) // ADR-029: a vote on Akira's ballot

	// Initialize Wotan client
	// WOTAN_ADDR = HTTP control plane (subscribe, publish, admin, circuit-breaker fallback)
	// WOTAN_GRPC_ADDR = gRPC data plane (TopicStream) — primary transport for streaming
	//
	// Transport strategy (Campaign 1 — TopicStream gRPC Sprint):
	//   1. If WOTAN_GRPC_ADDR is set → create TopicStreamClient (gRPC streaming primary)
	//      with HTTP Client as circuit-breaker fallback
	//   2. If gRPC unavailable → create plain HTTP Client (polling mode)
	//   3. If WOTAN_ENABLED=false → standalone mode (no Wotan)
	var server *Server
	wotanEnabled := getEnv("WOTAN_ENABLED", "true") == "true"
	wotanGRPCAddr := getEnv("WOTAN_GRPC_ADDR", "localhost:18001")
	if wotanGRPCAddr != "" {
		transportCfg.WotanGRPCAddr = wotanGRPCAddr
	}

	if wotanEnabled {
		log.Info().
			Str("wotan_http", cfg.WotanAddr).
			Str("wotan_grpc", wotanGRPCAddr).
			Msg("Initializing Wotan client")

		var busClient WotanClient
		var err error
		var transportName string

		if wotanGRPCAddr != "" {
			// Primary path: TopicStreamClient with gRPC streaming + HTTP fallback
			// Create HTTP client first as circuit-breaker fallback
			httpFallback, httpErr := wotanClient.NewClient(cfg.WotanAddr)
			if httpErr != nil {
				log.Warn().Err(httpErr).Msg("HTTP fallback client creation failed — TopicStream will run without fallback")
			}

			var tsOpts []wotanClient.TopicStreamOption
			tsOpts = append(tsOpts, wotanClient.WithRetryPolicy(
				100*time.Millisecond, // initial backoff
				30*time.Second,       // max backoff cap
				10,                   // max consecutive failures before circuit break
			))
			tsOpts = append(tsOpts, wotanClient.WithBufferSize(256))
			if httpFallback != nil {
				tsOpts = append(tsOpts, wotanClient.WithHTTPFallback(httpFallback))
			}

			tsc, tsErr := wotanClient.NewTopicStreamClient(wotanGRPCAddr, tsOpts...)
			if tsErr != nil {
				log.Warn().Err(tsErr).Msg("TopicStreamClient creation failed — falling back to HTTP polling client")
				busClient, err = wotanClient.NewClient(cfg.WotanAddr)
				transportName = "http-poll"
			} else {
				// Probe gRPC health to provide an informative startup log message.
				// The client will handle reconnection automatically regardless of the probe's outcome.
				probeCtx, probeCancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer probeCancel()
				if pingErr := tsc.Ping(probeCtx); pingErr != nil {
					log.Warn().Err(pingErr).Msg("gRPC health probe failed. Client will attempt to connect in the background")
					transportName = "grpc-stream (degraded)"
				} else {
					transportName = "grpc-stream"
				}
				busClient = tsc
			}
		} else {
			// No gRPC address — HTTP-only mode
			bc, bcErr := wotanClient.NewClient(cfg.WotanAddr)
			if bcErr != nil {
				err = bcErr
			} else {
				busClient = bc
				transportName = "http-poll"
			}
		}

		if transportName != "" {
			log.Info().Str("transport", transportName).Msg("Wotan transport selected")
		}

		if err != nil {
			log.Error().
				Err(err).
				Msg("Failed to create Wotan client, falling back to standalone mode")
			healthSrv.SetGRPCStatus(false)
			server = NewServer(cfg)
		} else {
			// Initialize task store: prefer Postgres, fall back to SQLite.
			var kanbanStore TaskStore

			pgDB, pgErr := database.Connect(context.Background(), database.AppKanbanConfig())
			pgStore, pgStoreErr := wellStore(pgDB, pgErr)
			switch {
			case pgStoreErr != nil:
				log.Fatal().Err(pgStoreErr).Msg("The Well answers but is not migrated for kanban-app: refusing to start (run well-migrate; no SQLite fallback, it would split the board)")
			case pgStore != nil:
				kanbanStore = pgStore
				log.Info().Msg("Postgres connected — kanban tasks persisted to The Well")
			default:
				log.Warn().Err(pgErr).Msg("Postgres unavailable — trying SQLite fallback")
			}

			if kanbanStore == nil {
				// SQLite fallback
				dbPath := cfg.DataDir + "/kanban.db"
				if cfg.DataDir == "" {
					dbPath = "./data/kanban.db"
				}
				sqliteStore, sqliteErr := openSQLiteStore(dbPath)
				if sqliteErr != nil {
					log.Warn().Err(sqliteErr).Str("path", dbPath).Msg("SQLite store init failed — TaskManager will run in-memory only")
				} else {
					kanbanStore = sqliteStore
				}
			}

			// Create TaskManager with broadcast function and Store
			broadcast := func(eventType string, data interface{}) {
				if server != nil {
					server.broadcastUpdate(eventType, data)
				}
			}

			taskManager, err := NewTaskManager(busClient, broadcast, kanbanStore)
			if err != nil {
				log.Error().
					Err(err).
					Msg("Failed to create TaskManager, falling back to standalone mode")
				healthSrv.SetGRPCStatus(false)
				server = NewServer(cfg)
			} else {
				// Initialize TaskManager (loads tasks from Store + subscribes to Wotan)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := taskManager.Initialize(ctx); err != nil {
					log.Error().
						Err(err).
						Msg("Failed to initialize TaskManager, falling back to standalone mode")
					healthSrv.SetGRPCStatus(false)
					cancel()
					server = NewServer(cfg)
				} else {
					cancel()
					server = NewServerWithTaskManager(cfg, taskManager, kanbanStore)
					log.Info().
						Str("transport", transportName).
						Msg("Wotan integration enabled with persistent store")
				}
			}
		}
	} else {
		log.Warn().Msg("Wotan disabled, running in standalone mode")
		server = NewServer(cfg)
	}

	// Wire transport health server into the kanban server
	server.healthSrv = healthSrv

	// kanban-app's own key mints the tokens its Timeguru calls carry
	// (ADR-103), set before the pollers below start. Without a key the
	// calls go anonymous, and a Timeguru with auth on answers 401.
	if m, err := auth.LoadServiceAuthConfig("kanban-app").ServiceTokenMinter(); err != nil {
		log.Warn().Err(err).Msg("no service key: calls to timeguru go without a token")
	} else {
		server.minter = m
	}

	// Start Timeguru HTTP polling ONLY in standalone mode.
	// When Wotan integration is active, timeline updates arrive via
	// the timeline.updates subscription — no polling needed.
	pollCtx, pollCancel := context.WithCancel(context.Background())
	if server.taskManager == nil {
		go server.pollTimeguru(pollCtx, 30*time.Second)
		log.Info().Msg("Timeguru HTTP polling enabled (standalone mode)")
	} else {
		// Wotan carries the notification; the content still comes over HTTP.
		// Wire the refetch, and fetch once now so a freshly started board does
		// not wait for timeguru's next file change to show anything.
		server.taskManager.timelineManager.SetRefetch(server.fetchTimelineFromTimeguru)
		go func() {
			for attempt := 1; attempt <= 5; attempt++ {
				if err := server.fetchTimelineFromTimeguru(); err == nil {
					log.Info().Msg("initial timeline fetched from Timeguru")
					return
				} else if attempt == 5 {
					log.Warn().Err(err).Msg("initial timeline fetch failed; waiting for a timeline.updates notification")
				}
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
			}
		}()
		log.Info().Msg("Timeguru HTTP polling disabled (Wotan notifies; content fetched on notification)")
	}

	// Every construction branch above converges here, so attach once.
	if err := server.SetLogPublisher(logPublisher); err != nil {
		log.Warn().Err(err).Msg("log forwarding counters not served on /metrics")
	}

	// Start server in goroutine
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	// Stop timeline polling
	pollCancel()

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	// Close TaskManager if present
	if server.taskManager != nil {
		if err := server.taskManager.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close TaskManager")
		}
	}

	// Close SQLite store if present
	if server.store != nil {
		if err := server.store.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close SQLite store")
		} else {
			log.Info().Msg("SQLite store closed cleanly")
		}
	}

	log.Info().Msg("Server exited")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
