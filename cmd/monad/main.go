// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the MIT License.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package main provides the HTTP API server for the Monad service.
// The Monad is the supreme orchestrator of functional composition in the Kingdom,
// providing a unified interface for executing operations and transactions with
// railway-oriented error handling.
//
// Endpoints:
//   - GET /health - Health check
//   - GET /ready - Readiness check
//   - GET /metrics - Prometheus metrics
//   - POST /api/v1/operations - Execute an operation
//   - GET /api/v1/operations/:id - Get operation status
//   - POST /api/v1/transactions - Execute a transaction
//   - GET /api/v1/stats - Service statistics
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/discovery"
	"unheaded/pkg/logagg"
	"unheaded/pkg/logger"
	"unheaded/pkg/ports"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/monad"
)

var (
	// Version information (set by build)
	Version   = "0.1.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

var (
	listenAddr = flag.String("listen", ports.DefaultAddr(ports.Monad), "HTTP listen address")
	wotanAddr  = flag.String("wotan", "localhost:18001", "Wotan server address")
	debug      = flag.Bool("debug", false, "Enable debug logging")
	jsonLogs   = flag.Bool("json", false, "Output logs in JSON format")
)

func main() {
	flag.Parse()

	// Allow environment variable override for port
	if port := os.Getenv("MONAD_PORT"); port != "" {
		*listenAddr = ":" + port
	}
	if addr := os.Getenv("MONAD_LISTEN_ADDR"); addr != "" {
		*listenAddr = addr
	}
	if addr := os.Getenv("WOTAN_ADDR"); addr != "" {
		*wotanAddr = addr
	}

	// Transport config
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	transportCfg.WotanGRPCAddr = *wotanAddr

	// Service lifecycle context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Service discovery registration
	discovery.SetupServiceDiscovery(ctx, nil, "monad", 19004)

	// Health server
	healthSrv := transport.NewHealthServer("monad")

	// Setup logging using Kingdom's native logger
	logConfig := logger.DefaultConfig()
	if *debug {
		logConfig.Level = logger.DebugLevel
	} else {
		logConfig.Level = logger.InfoLevel
	}
	if !*jsonLogs {
		logConfig.ConsoleMode = true
		logConfig.ColorEnabled = true
	}
	logConfig.CallerEnabled = *debug

	log := logger.NewWithConfig(logConfig)

	log.Info().
		Str("service", "monad").
		Str("version", Version).
		Str("build_time", BuildTime).
		Str("git_commit", GitCommit).
		Msg("starting Monad service - The Supreme Orchestrator")

	// Connect to Wotan
	wotan, err := wotanClient.NewClient(*wotanAddr)
	if err != nil {
		log.Warn().
			Err(err).
			Str("wotan_addr", *wotanAddr).
			Msg("failed to create Wotan client, continuing without message bus")
		healthSrv.SetGRPCStatus(false)
		wotan = nil
	} else {
		log.Info().
			Str("wotan_addr", *wotanAddr).
			Msg("Wotan client created")

		// Subscribe to monad operations topic
		subCtx, subCancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, subscribeErr := wotan.Subscribe(subCtx, "monad.operations", "monad-service")
		subCancel()
		if subscribeErr != nil {
			log.Warn().
				Err(subscribeErr).
				Msg("failed to subscribe to Wotan topic, continuing without subscription")
		} else {
			log.Info().Msg("subscribed to monad.operations topic")
		}
	}

	// Log aggregation publisher — forwards structured logs to Wotan
	var logConn transport.Connection
	if wotan != nil {
		logConn, _ = transport.Connect(ctx, transportCfg) // best-effort; nil on failure
	}
	_ = logagg.NewPublisher("monad", logConn)

	// Create Monad service
	monadService := monad.NewService(log, wotan)

	// Register default handlers for demonstration
	registerDefaultHandlers(monadService, log)

	// Create HTTP server
	server, err := NewHTTPServer(monadService, log, *listenAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create HTTP server")
	}

	// Start server
	if err := server.Start(); err != nil {
		log.Fatal().Err(err).Msg("failed to start HTTP server")
	}

	log.Info().
		Str("addr", *listenAddr).
		Str("wotan", *wotanAddr).
		Msg("Monad service running - The One from which all emanates")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Info().
		Str("signal", sig.String()).
		Msg("shutdown signal received")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown error")
		os.Exit(1)
	}

	// Close Wotan client
	if wotan != nil {
		if err := wotan.Close(); err != nil {
			log.Warn().Err(err).Msg("error closing Wotan client")
		}
	}

	log.Info().Msg("Monad service stopped - The Circle closes")
}

// registerDefaultHandlers registers some default operation handlers for demonstration
func registerDefaultHandlers(svc *monad.Service, log *logger.Logger) {
	// Echo query - returns the input as output
	svc.RegisterQuery("echo", func(ctx context.Context, input interface{}) (interface{}, error) {
		return map[string]interface{}{
			"echo":      input,
			"timestamp": time.Now().Unix(),
		}, nil
	})

	// Ping query - simple health ping
	svc.RegisterQuery("ping", func(ctx context.Context, input interface{}) (interface{}, error) {
		return map[string]interface{}{
			"pong":      true,
			"timestamp": time.Now().Unix(),
		}, nil
	})

	// Transform mutation - demonstrates state change tracking
	svc.RegisterMutation("transform", func(ctx context.Context, input interface{}) (interface{}, []monad.StateChange, error) {
		inputMap, ok := input.(map[string]interface{})
		if !ok {
			return nil, nil, errors.New("input must be an object")
		}

		// Record the transformation as a state change
		changes := []monad.StateChange{
			{
				Entity:    "transformation",
				Field:     "data",
				OldValue:  nil,
				NewValue:  inputMap,
				Timestamp: time.Now(),
			},
		}

		result := map[string]interface{}{
			"transformed": true,
			"input":       inputMap,
			"timestamp":   time.Now().Unix(),
		}

		return result, changes, nil
	})

	// Log effect - demonstrates side effect execution
	svc.RegisterEffect("log", func(ctx context.Context, input interface{}) error {
		log.Info().
			Interface("effect_input", input).
			Msg("effect executed: log")
		return nil
	})

	log.Info().Msg("registered default operation handlers (echo, ping, transform, log)")
}

// ============================================================================
// HTTP SERVER
// ============================================================================

// HTTPServer provides REST API endpoints for the Monad service
type HTTPServer struct {
	service   *monad.Service
	log       *logger.Logger
	server    *http.Server
	mu        sync.RWMutex
	metrics   *HTTPMetrics
	requestID int64
	ready     bool
}

// HTTPMetrics tracks HTTP metrics for Prometheus
type HTTPMetrics struct {
	RequestsTotal        int64
	RequestsSuccess      int64
	RequestsError        int64
	OperationsExecuted   int64
	TransactionsExecuted int64
	mu                   sync.RWMutex
}

// ResponseEnvelope wraps all API responses
type ResponseEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
	Meta    *MetaInfo   `json:"meta,omitempty"`
}

// ErrorInfo represents error details
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// MetaInfo represents response metadata
type MetaInfo struct {
	RequestID string        `json:"request_id"`
	Duration  time.Duration `json:"duration_ms"`
	Timestamp time.Time     `json:"timestamp"`
}

// NewHTTPServer creates a new HTTP server for the Monad service
func NewHTTPServer(service *monad.Service, log *logger.Logger, addr string) (*HTTPServer, error) {
	if service == nil {
		return nil, errors.New("service cannot be nil")
	}
	if log == nil {
		return nil, errors.New("logger cannot be nil")
	}
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	hs := &HTTPServer{
		service: service,
		log:     log,
		metrics: &HTTPMetrics{},
		ready:   true,
	}

	mux := http.NewServeMux()

	// Health check endpoints
	mux.HandleFunc("/health", hs.healthHandler)
	mux.HandleFunc("/ready", hs.readyHandler)

	// Metrics endpoint
	mux.HandleFunc("/metrics", hs.metricsHandler)

	// API endpoints
	mux.HandleFunc("/api/v1/operations", hs.operationsHandler)
	mux.HandleFunc("/api/v1/operations/", hs.operationDetailHandler)
	mux.HandleFunc("/api/v1/transactions", hs.transactionsHandler)
	mux.HandleFunc("/api/v1/stats", hs.statsHandler)

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("monad")
	var httpHandler http.Handler = mux
	httpHandler = auth.WrapHandler(httpHandler, auth.SetupMiddleware(authCfg))
	httpHandler = http.MaxBytesHandler(hs.loggingMiddleware(httpHandler), 10*1024*1024)

	hs.server = &http.Server{
		Addr:           addr,
		Handler:        httpHandler,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	return hs, nil
}

// Start starts the HTTP server
func (hs *HTTPServer) Start() error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if hs.server == nil {
		return errors.New("server not initialized")
	}

	go func() {
		hs.log.Info().
			Str("addr", hs.server.Addr).
			Msg("HTTP server listening")
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hs.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP server
func (hs *HTTPServer) Stop(ctx context.Context) error {
	hs.mu.Lock()
	hs.ready = false
	hs.mu.Unlock()

	if hs.server == nil {
		return nil
	}

	return hs.server.Shutdown(ctx)
}

// loggingMiddleware logs all HTTP requests
func (hs *HTTPServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := hs.getNextRequestID()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Note: request ID is propagated via the JSON RequestID field in
		// MetaInfo (set at handler-time from atomic.LoadInt64(&hs.requestID))
		// and via the structured-log "request_id" field below — no
		// context-value plumbing needed here. The earlier
		// `context.WithValue(r.Context(), "request_id", ...)` was never
		// retrieved (SA1029 bare-string key + dead value).
		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		hs.log.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.statusCode).
			Dur("duration", duration).
			Str("request_id", fmt.Sprintf("req_%d", reqID)).
			Msg("request completed")
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ============================================================================
// HANDLERS
// ============================================================================

// healthHandler responds to health checks
func (hs *HTTPServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service":   "monad",
		"status":    "healthy",
		"version":   Version,
		"timestamp": time.Now().UTC(),
	})
}

// readyHandler responds to readiness checks
func (hs *HTTPServer) readyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only", nil)
		return
	}

	hs.mu.RLock()
	ready := hs.ready
	hs.mu.RUnlock()

	if !ready {
		hs.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service is shutting down", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ready",
		"service": "monad",
	})
}

// metricsHandler returns Prometheus-style metrics
func (hs *HTTPServer) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only", nil)
		return
	}

	hs.metrics.mu.RLock()
	stats := hs.service.Stats()
	metrics := fmt.Sprintf(
		"# HELP monad_http_requests_total Total HTTP requests\n"+
			"# TYPE monad_http_requests_total counter\n"+
			"monad_http_requests_total %d\n"+
			"# HELP monad_http_requests_success Successful HTTP requests\n"+
			"# TYPE monad_http_requests_success counter\n"+
			"monad_http_requests_success %d\n"+
			"# HELP monad_http_requests_error Failed HTTP requests\n"+
			"# TYPE monad_http_requests_error counter\n"+
			"monad_http_requests_error %d\n"+
			"# HELP monad_operations_executed_total Total operations executed\n"+
			"# TYPE monad_operations_executed_total counter\n"+
			"monad_operations_executed_total %d\n"+
			"# HELP monad_transactions_executed_total Total transactions executed\n"+
			"# TYPE monad_transactions_executed_total counter\n"+
			"monad_transactions_executed_total %d\n"+
			"# HELP monad_operations_total Total tracked operations\n"+
			"# TYPE monad_operations_total gauge\n"+
			"monad_operations_total %v\n"+
			"# HELP monad_operations_completed Completed operations\n"+
			"# TYPE monad_operations_completed gauge\n"+
			"monad_operations_completed %v\n"+
			"# HELP monad_operations_failed Failed operations\n"+
			"# TYPE monad_operations_failed gauge\n"+
			"monad_operations_failed %v\n"+
			"# HELP monad_operations_running Running operations\n"+
			"# TYPE monad_operations_running gauge\n"+
			"monad_operations_running %v\n"+
			"# HELP monad_transactions_total Total tracked transactions\n"+
			"# TYPE monad_transactions_total gauge\n"+
			"monad_transactions_total %v\n",
		hs.metrics.RequestsTotal,
		hs.metrics.RequestsSuccess,
		hs.metrics.RequestsError,
		hs.metrics.OperationsExecuted,
		hs.metrics.TransactionsExecuted,
		stats["total_operations"],
		stats["completed_operations"],
		stats["failed_operations"],
		stats["running_operations"],
		stats["total_transactions"],
	)
	hs.metrics.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(metrics))
}

// operationsHandler handles POST /api/v1/operations - Execute an operation
func (hs *HTTPServer) operationsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		hs.executeOperationHandler(w, r)
	case http.MethodGet:
		hs.listOperationsHandler(w, r)
	default:
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "POST or GET only", nil)
	}
}

// ExecuteOperationRequest represents an operation execution request
type ExecuteOperationRequest struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"` // query, mutation, effect
	Input interface{} `json:"input,omitempty"`
}

// executeOperationHandler executes a single operation
func (hs *HTTPServer) executeOperationHandler(w http.ResponseWriter, r *http.Request) {
	// Validate content type
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		hs.writeError(w, http.StatusUnsupportedMediaType, "INVALID_CONTENT_TYPE", "application/json required", nil)
		return
	}

	// Read body with limit
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024)) // 1MB max for operations
	if err != nil {
		hs.writeError(w, http.StatusBadRequest, "INVALID_BODY", "Failed to read request body", err)
		return
	}

	// Parse request
	var req ExecuteOperationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		hs.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err)
		return
	}

	// Validate request
	if req.Name == "" {
		hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Operation name is required", nil)
		return
	}
	if req.Type == "" {
		req.Type = "query" // Default to query
	}

	// Execute based on type
	ctx := r.Context()
	var result interface{}
	var opErr error

	switch strings.ToLower(req.Type) {
	case "query":
		res := hs.service.Query(ctx, req.Name, req.Input)
		if res.IsErr() {
			opErr = res.Error()
		} else {
			result = res.Value()
		}
	case "mutation":
		res := hs.service.Mutate(ctx, req.Name, req.Input)
		if res.IsErr() {
			opErr = res.Error()
		} else {
			result = res.Value()
		}
	case "effect":
		res := hs.service.Effect(ctx, req.Name, req.Input)
		if res.IsErr() {
			opErr = res.Error()
		} else {
			result = map[string]interface{}{"success": true}
		}
	default:
		hs.writeError(w, http.StatusBadRequest, "INVALID_TYPE", "Type must be query, mutation, or effect", nil)
		return
	}

	// Update metrics
	hs.metrics.mu.Lock()
	hs.metrics.OperationsExecuted++
	hs.metrics.mu.Unlock()

	if opErr != nil {
		hs.writeError(w, http.StatusBadRequest, "OPERATION_FAILED", opErr.Error(), opErr)
		return
	}

	hs.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"result": result,
		"operation": map[string]interface{}{
			"name": req.Name,
			"type": req.Type,
		},
	})
}

// listOperationsHandler lists recent operations
func (hs *HTTPServer) listOperationsHandler(w http.ResponseWriter, r *http.Request) {
	limit := 50 // Default limit
	if l := r.URL.Query().Get("limit"); l != "" {
		_, _ = fmt.Sscanf(l, "%d", &limit)
		if limit <= 0 || limit > 1000 {
			limit = 50
		}
	}

	ops := hs.service.ListOperations(limit)

	hs.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"operations": ops,
		"count":      len(ops),
	})
}

// operationDetailHandler handles GET /api/v1/operations/:id
func (hs *HTTPServer) operationDetailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only", nil)
		return
	}

	// Extract ID from path
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/operations/")
	if id == "" {
		hs.writeError(w, http.StatusBadRequest, "MISSING_ID", "Operation ID required", nil)
		return
	}

	op, found := hs.service.GetOperation(id)
	if !found {
		hs.writeError(w, http.StatusNotFound, "NOT_FOUND", "Operation not found", nil)
		return
	}

	hs.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"operation": op,
	})
}

// ExecuteTransactionRequest represents a transaction execution request
type ExecuteTransactionRequest struct {
	Operations []ExecuteOperationRequest `json:"operations"`
}

// transactionsHandler handles POST /api/v1/transactions
func (hs *HTTPServer) transactionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "POST only", nil)
		return
	}

	// Validate content type
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		hs.writeError(w, http.StatusUnsupportedMediaType, "INVALID_CONTENT_TYPE", "application/json required", nil)
		return
	}

	// Read body with limit
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024)) // 5MB max for transactions
	if err != nil {
		hs.writeError(w, http.StatusBadRequest, "INVALID_BODY", "Failed to read request body", err)
		return
	}

	// Parse request
	var req ExecuteTransactionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		hs.writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err)
		return
	}

	// Validate request
	if len(req.Operations) == 0 {
		hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "At least one operation is required", nil)
		return
	}

	// Begin transaction
	ctx := r.Context()
	tx := hs.service.BeginTransaction(ctx)

	// Execute each operation in the transaction
	for i, op := range req.Operations {
		if op.Name == "" {
			_ = tx.Rollback()
			hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR",
				fmt.Sprintf("Operation %d: name is required", i), nil)
			return
		}

		opType := strings.ToLower(op.Type)
		if opType == "" {
			opType = "query"
		}

		switch opType {
		case "query":
			tx = tx.Query(op.Name, op.Input)
		case "mutation":
			tx = tx.Mutate(op.Name, op.Input)
		case "effect":
			tx = tx.Effect(op.Name, op.Input)
		default:
			_ = tx.Rollback()
			hs.writeError(w, http.StatusBadRequest, "INVALID_TYPE",
				fmt.Sprintf("Operation %d: type must be query, mutation, or effect", i), nil)
			return
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		hs.writeError(w, http.StatusBadRequest, "TRANSACTION_FAILED", err.Error(), err)
		return
	}

	// Update metrics
	hs.metrics.mu.Lock()
	hs.metrics.TransactionsExecuted++
	hs.metrics.OperationsExecuted += int64(len(req.Operations))
	hs.metrics.mu.Unlock()

	hs.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"success":          true,
		"operations_count": len(req.Operations),
		"message":          "Transaction committed successfully",
	})
}

// statsHandler returns service statistics
func (hs *HTTPServer) statsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only", nil)
		return
	}

	stats := hs.service.Stats()

	// Add HTTP metrics
	hs.metrics.mu.RLock()
	stats["http_requests_total"] = hs.metrics.RequestsTotal
	stats["http_requests_success"] = hs.metrics.RequestsSuccess
	stats["http_requests_error"] = hs.metrics.RequestsError
	stats["operations_executed"] = hs.metrics.OperationsExecuted
	stats["transactions_executed"] = hs.metrics.TransactionsExecuted
	hs.metrics.mu.RUnlock()

	hs.writeSuccess(w, http.StatusOK, stats)
}

// ============================================================================
// RESPONSE WRITERS
// ============================================================================

// writeSuccess writes a successful response
func (hs *HTTPServer) writeSuccess(w http.ResponseWriter, code int, data interface{}) {
	hs.metrics.mu.Lock()
	hs.metrics.RequestsTotal++
	hs.metrics.RequestsSuccess++
	hs.metrics.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	resp := ResponseEnvelope{
		Success: true,
		Data:    data,
		Meta: &MetaInfo{
			RequestID: fmt.Sprintf("req_%d", atomic.LoadInt64(&hs.requestID)),
			Duration:  0,
			Timestamp: time.Now(),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// writeError writes an error response
func (hs *HTTPServer) writeError(w http.ResponseWriter, code int, errCode, msg string, err error) {
	hs.metrics.mu.Lock()
	hs.metrics.RequestsTotal++
	hs.metrics.RequestsError++
	hs.metrics.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	errInfo := &ErrorInfo{
		Code:    errCode,
		Message: msg,
	}
	if err != nil {
		errInfo.Details = err.Error()
	}

	resp := ResponseEnvelope{
		Success: false,
		Error:   errInfo,
		Meta: &MetaInfo{
			RequestID: fmt.Sprintf("req_%d", atomic.LoadInt64(&hs.requestID)),
			Duration:  0,
			Timestamp: time.Now(),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// getNextRequestID returns the next request ID
func (hs *HTTPServer) getNextRequestID() int64 {
	return atomic.AddInt64(&hs.requestID, 1)
}
