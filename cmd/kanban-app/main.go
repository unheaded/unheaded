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

	busboyClient "unheaded/pkg/busboy-client"
	"unheaded/pkg/logger"
)

// Package-level logger instance
var log = logger.New(os.Stderr)

//go:embed static/*
var staticFiles embed.FS

// Config holds server configuration
type Config struct {
	Port           string
	TimeGuruAddr   string
	BusboyAddr     string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	ShutdownTimeout time.Duration
}

// Task represents a kanban task
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Type        string    `json:"type,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Progress    int       `json:"progress,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Server holds the HTTP server and dependencies
type Server struct {
	config          Config
	httpServer      *http.Server
	tasks           []Task // DEPRECATED: Use taskManager instead
	tasksMu         sync.RWMutex
	sseClients      map[chan []byte]bool
	sseMu           sync.RWMutex
	taskManager     *TaskManager     // Busboy-integrated task management
	timelineManager *TimelineManager // Standalone Timeguru HTTP polling fallback
}

// NewServer creates a new kanban server with standalone timeline polling
func NewServer(cfg Config) *Server {
	s := &Server{
		config:     cfg,
		sseClients: make(map[chan []byte]bool),
		tasks:      getInitialTasks(), // Fallback if Timeguru unreachable
	}
	// Create standalone TimelineManager for direct Timeguru HTTP polling
	s.timelineManager = NewTimelineManager(func(eventType string, data interface{}) {
		s.broadcastUpdate(eventType, data)
	})
	return s
}

// NewServerWithTaskManager creates a server with Busboy integration
func NewServerWithTaskManager(cfg Config, tm *TaskManager) *Server {
	s := &Server{
		config:      cfg,
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
	}
	return s
}

// getInitialTasks returns initial task data (from timeline.md)
func getInitialTasks() []Task {
	now := time.Now()
	return []Task{
		{
			ID:          "ms-alpha",
			Title:       "Phase 1 Alpha Release",
			Description: "eBPF Foundation + Control Plane + Microservices",
			Status:      "in-progress",
			Type:        "milestone",
			Owner:       "Team",
			Progress:    75,
			CreatedAt:   now.Add(-30 * 24 * time.Hour),
			UpdatedAt:   now,
		},
		{
			ID:          "ebpf-probes",
			Title:       "eBPF Probes Implementation",
			Description: "packet_marker, flow_tracker, latency_probe",
			Status:      "in-progress",
			Type:        "feature",
			Owner:       "Architect",
			Progress:    40,
			CreatedAt:   now.Add(-14 * 24 * time.Hour),
			UpdatedAt:   now,
		},
		{
			ID:          "busboy-tests",
			Title:       "Busboy Unit Test Suite",
			Description: "Comprehensive unit tests for core components",
			Status:      "todo",
			Type:        "task",
			Owner:       "Developer",
			Progress:    0,
			CreatedAt:   now.Add(-7 * 24 * time.Hour),
			UpdatedAt:   now,
		},
		{
			ID:          "control-plane",
			Title:       "Control Plane REST API",
			Description: "Full CRUD + admin operations",
			Status:      "done",
			Type:        "feature",
			Owner:       "Architect",
			Progress:    100,
			CreatedAt:   now.Add(-21 * 24 * time.Hour),
			UpdatedAt:   now.Add(-3 * 24 * time.Hour),
		},
		{
			ID:          "grpc-streaming",
			Title:       "gRPC Bidirectional Streaming",
			Description: "Real-time message streaming with historical replay",
			Status:      "done",
			Type:        "feature",
			Owner:       "Architect",
			Progress:    100,
			CreatedAt:   now.Add(-20 * 24 * time.Hour),
			UpdatedAt:   now.Add(-5 * 24 * time.Hour),
		},
		{
			ID:          "rate-limiting",
			Title:       "Rate Limiting & Circuit Breakers",
			Description: "Token bucket per-client rate limiting",
			Status:      "done",
			Type:        "feature",
			Owner:       "Developer",
			Progress:    100,
			CreatedAt:   now.Add(-18 * 24 * time.Hour),
			UpdatedAt:   now.Add(-7 * 24 * time.Hour),
		},
		{
			ID:          "kanban-frontend",
			Title:       "Kanban Dashboard Frontend",
			Description: "Real-time board with Busboy integration",
			Status:      "in-progress",
			Type:        "feature",
			Owner:       "Developer",
			Progress:    60,
			CreatedAt:   now.Add(-2 * 24 * time.Hour),
			UpdatedAt:   now,
		},
		{
			ID:          "skill-updates",
			Title:       "Skill Cross-References",
			Description: "Update all skills with cross-references",
			Status:      "todo",
			Type:        "task",
			Owner:       "Captain",
			Progress:    0,
			CreatedAt:   now.Add(-1 * 24 * time.Hour),
			UpdatedAt:   now,
		},
		{
			ID:          "ci-cd",
			Title:       "CI/CD Pipeline Templates",
			Description: "GitHub Actions for build, test, deploy",
			Status:      "todo",
			Type:        "task",
			Owner:       "Developer",
			Progress:    0,
			CreatedAt:   now.Add(-1 * 24 * time.Hour),
			UpdatedAt:   now,
		},
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes - tasks at canonical /api/v1/tasks
	mux.HandleFunc("/api/v1/tasks", s.handleTasks)
	mux.HandleFunc("/api/v1/tasks/", s.handleTaskByID) // For /tasks/:id
	mux.HandleFunc("/api/v1/timeline/tasks", s.handleTasks) // Legacy compatibility
	mux.HandleFunc("/api/v1/stream", s.handleSSE)
	mux.HandleFunc("/ws", s.handleWebSocket) // WebSocket endpoint
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
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
	handler = s.loggingMiddleware(handler)                          // Logging (innermost)
	handler = requestSizeLimitMiddleware(1024 * 1024)(handler)      // 1MB max request
	handler = securityHeadersMiddleware(handler)                     // Security headers
	handler = corsMiddleware(handler)                                // CORS

	// Rate limiting (configurable)
	if getEnv("RATE_LIMIT_ENABLED", "true") == "true" {
		rateLimiter := NewRateLimiter(120, 30) // 120 req/min, burst 30 (page load is ~15 resources)
		handler = rateLimitMiddleware(rateLimiter)(handler)
		log.Info().Msg("Rate limiting enabled: 120 req/min, burst 30")
	}

	s.httpServer = &http.Server{
		Addr:         ":" + s.config.Port,
		Handler:      handler,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	log.Info().
		Str("port", s.config.Port).
		Str("timeguru", s.config.TimeGuruAddr).
		Str("busboy", s.config.BusboyAddr).
		Bool("busboy_enabled", s.taskManager != nil).
		Msg("Starting kanban-app server")

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
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
// Prefers TaskManager's timeline (Busboy-backed) when available,
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

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
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
// Used as fallback when Busboy is unavailable, or to seed initial data.
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
			Msg("HTTP request")
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

// handleTasks routes task operations (GET, POST, PUT, DELETE)
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
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

	if s.taskManager != nil {
		// Prefer Busboy-backed TaskManager
		taskList := s.taskManager.GetAllTasks()
		tasks = taskList
		count = len(taskList)
	} else if tm := s.getTimelineManager(); tm != nil {
		// Fallback: timeline tasks from direct Timeguru HTTP polling
		timelineTasks := tm.GetTimelineTasks()
		if len(timelineTasks) > 0 {
			tasks = timelineTasks
			count = len(timelineTasks)
		}
	}

	// Last resort: hardcoded initial tasks
	if tasks == nil {
		s.tasksMu.RLock()
		tasks = s.tasks
		count = len(s.tasks)
		s.tasksMu.RUnlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
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

			switch err {
			case ErrTaskAlreadyExists:
				http.Error(w, err.Error(), http.StatusConflict)
			case ErrInvalidTask:
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else {
		// Standalone mode: in-memory CRUD
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
	json.NewEncoder(w).Encode(map[string]interface{}{
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

			switch err {
			case ErrTaskNotFound:
				http.Error(w, err.Error(), http.StatusNotFound)
			case ErrInvalidTask:
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else {
		// Standalone mode: in-memory update
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
	json.NewEncoder(w).Encode(map[string]interface{}{
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
	} else {
		// Standalone mode: in-memory delete
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
	json.NewEncoder(w).Encode(map[string]interface{}{
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
		json.NewEncoder(w).Encode(task)
		return
	}

	// Standalone mode: search in-memory tasks
	s.tasksMu.RLock()
	defer s.tasksMu.RUnlock()
	for _, t := range s.tasks {
		if t.ID == taskID {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(t)
			return
		}
	}
	http.Error(w, "Task not found", http.StatusNotFound)
}

// handleUpdateTaskByID updates a single task
func (s *Server) handleUpdateTaskByID(w http.ResponseWriter, r *http.Request, taskID string) {
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	task.ID = taskID // Ensure ID from URL

	if s.taskManager != nil {
		if err := s.taskManager.UpdateTask(r.Context(), &task); err != nil {
			if err == ErrTaskNotFound {
				http.Error(w, "Task not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else {
		// Standalone mode: in-memory update
		s.tasksMu.Lock()
		found := false
		for i, t := range s.tasks {
			if t.ID == taskID {
				task.CreatedAt = t.CreatedAt
				task.UpdatedAt = time.Now()
				s.tasks[i] = task
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
	json.NewEncoder(w).Encode(map[string]interface{}{"task": task})
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
	} else {
		// Standalone mode: in-memory delete
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
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "task_id": taskID})
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
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
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
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client channel
	clientCh := make(chan []byte, 10)

	s.sseMu.Lock()
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
		// Busboy mode: send real kanban task cards
		allTasks := s.taskManager.GetAllTasks()
		if len(allTasks) > 0 {
			initialData, _ = json.Marshal(allTasks)
			log.Info().Int("count", len(allTasks)).Msg("SSE sending kanban tasks (Busboy)")
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

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":              "healthy",
		"timestamp":           time.Now().UTC().Format(time.RFC3339),
		"version":             "0.1.0",
		"busboy_enabled":      s.taskManager != nil,
		"timeline_subscribed": timelineSubscribed,
		"timeline_http":       timelineHTTP,
		"timeguru_addr":       s.config.TimeGuruAddr,
	})
}

// handleReady returns readiness status
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
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

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":   ready,
		"reason":  reason,
		"service": "kanban-app",
	})
}

// handleMetrics returns Prometheus-style metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	s.sseMu.RLock()
	sseClients := len(s.sseClients)
	s.sseMu.RUnlock()

	taskCount := 0
	if s.taskManager != nil {
		taskCount = len(s.taskManager.GetAllTasks())
	} else {
		s.tasksMu.RLock()
		taskCount = len(s.tasks)
		s.tasksMu.RUnlock()
	}

	fmt.Fprintf(w, "# HELP kanban_tasks_total Total tasks\n")
	fmt.Fprintf(w, "# TYPE kanban_tasks_total gauge\n")
	fmt.Fprintf(w, "kanban_tasks_total %d\n", taskCount)
	fmt.Fprintf(w, "# HELP kanban_sse_clients_active Active SSE client connections\n")
	fmt.Fprintf(w, "# TYPE kanban_sse_clients_active gauge\n")
	fmt.Fprintf(w, "kanban_sse_clients_active %d\n", sseClients)
	fmt.Fprintf(w, "# HELP kanban_busboy_enabled Whether Busboy integration is enabled\n")
	fmt.Fprintf(w, "# TYPE kanban_busboy_enabled gauge\n")
	if s.taskManager != nil {
		fmt.Fprintf(w, "kanban_busboy_enabled 1\n")
	} else {
		fmt.Fprintf(w, "kanban_busboy_enabled 0\n")
	}
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
		json.NewEncoder(w).Encode(map[string]interface{}{
			"timeline": nil,
			"message":  "No timeline data available yet. Waiting for Timeguru updates.",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
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

	// Try Busboy-backed TaskManager first
	if s.taskManager != nil {
		timelineTasks := s.taskManager.GetTimelineTasks()
		if len(timelineTasks) > 0 {
			tasks = timelineTasks
			count = len(timelineTasks)
			source = "timeline-busboy"
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

	// Last resort: regular tasks
	if tasks == nil {
		if s.taskManager != nil {
			taskList := s.taskManager.GetAllTasks()
			tasks = taskList
			count = len(taskList)
		} else {
			s.tasksMu.RLock()
			tasks = s.tasks
			count = len(s.tasks)
			s.tasksMu.RUnlock()
		}
		source = "fallback"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
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
		Port:            getEnv("PORT", "8081"),
		TimeGuruAddr:    getEnv("TIMEGURU_ADDR", "localhost:8000"),
		BusboyAddr:      getEnv("BUSBOY_ADDR", "localhost:8080"),
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	// Initialize Busboy client
	// BUSBOY_ADDR = HTTP control plane (subscribe, publish, admin)
	// BUSBOY_GRPC_ADDR = gRPC data plane (streaming) — preferred for perf
	var server *Server
	busboyEnabled := getEnv("BUSBOY_ENABLED", "true") == "true"
	busboyGRPCAddr := getEnv("BUSBOY_GRPC_ADDR", "localhost:9090")

	if busboyEnabled {
		log.Info().
			Str("busboy_http", cfg.BusboyAddr).
			Str("busboy_grpc", busboyGRPCAddr).
			Msg("Initializing Busboy client")

		var bc *busboyClient.Client
		var err error
		if busboyGRPCAddr != "" {
			bc, err = busboyClient.NewClientWithGRPC(cfg.BusboyAddr, busboyGRPCAddr)
			if err == nil {
				log.Info().
					Str("transport", bc.Transport().String()).
					Bool("grpc_healthy", bc.IsGRPCHealthy()).
					Msg("Busboy transport selected")
			}
		} else {
			bc, err = busboyClient.NewClient(cfg.BusboyAddr)
		}
		busboyClient := bc // shadows package name — variable used by NewTaskManager below
		if err != nil {
			log.Error().
				Err(err).
				Msg("Failed to create Busboy client, falling back to standalone mode")
			server = NewServer(cfg)
		} else {
			// Create TaskManager with broadcast function
			broadcast := func(eventType string, data interface{}) {
				if server != nil {
					server.broadcastUpdate(eventType, data)
				}
			}

			taskManager, err := NewTaskManager(busboyClient, broadcast)
			if err != nil {
				log.Error().
					Err(err).
					Msg("Failed to create TaskManager, falling back to standalone mode")
				server = NewServer(cfg)
			} else {
				// Initialize TaskManager (loads tasks + subscribes to Busboy)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := taskManager.Initialize(ctx); err != nil {
					log.Error().
						Err(err).
						Msg("Failed to initialize TaskManager, falling back to standalone mode")
					cancel()
					server = NewServer(cfg)
				} else {
					cancel()
					server = NewServerWithTaskManager(cfg, taskManager)
					log.Info().Msg("Busboy integration enabled")
				}
			}
		}
	} else {
		log.Warn().Msg("Busboy disabled, running in standalone mode")
		server = NewServer(cfg)
	}

	// Start Timeguru HTTP polling ONLY in standalone mode.
	// When Busboy integration is active, timeline updates arrive via
	// the timeline.updates subscription — no polling needed.
	pollCtx, pollCancel := context.WithCancel(context.Background())
	if server.taskManager == nil {
		go server.pollTimeguru(pollCtx, 30*time.Second)
		log.Info().Msg("Timeguru HTTP polling enabled (standalone mode)")
	} else {
		log.Info().Msg("Timeguru HTTP polling disabled (Busboy handles timeline events)")
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

	log.Info().Msg("Server exited")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
