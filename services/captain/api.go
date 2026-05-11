// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package captain

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/httputil"
)

// HTTPServer provides REST API endpoints for the captain service
type HTTPServer struct {
	service *Service
	server  *http.Server
	mu      sync.RWMutex
	metrics *HTTPMetrics
}

// HTTPMetrics tracks HTTP metrics
type HTTPMetrics struct {
	RequestsTotal   int64
	RequestsSuccess int64
	RequestsError   int64
	AverageDuration time.Duration
	TotalDuration   time.Duration
	mu              sync.RWMutex
}

// HTTPRequest represents an HTTP request wrapper
type HTTPRequest struct {
	RequestID  string
	Method     string
	Path       string
	StartTime  time.Time
	ReceivedAt time.Time
	SizeBytes  int64
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(service *Service, addr string) (*HTTPServer, error) {
	if service == nil {
		return nil, errors.New("service cannot be nil")
	}
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	hs := &HTTPServer{
		service: service,
		metrics: &HTTPMetrics{},
	}

	mux := http.NewServeMux()

	// Health check endpoints
	mux.HandleFunc("/health", hs.healthHandler)
	mux.HandleFunc("/ready", hs.readyHandler)

	// Metrics endpoint
	mux.HandleFunc("/metrics", hs.metricsHandler)

	// Vision endpoint
	mux.HandleFunc("/api/v1/vision", hs.visionHandler)

	// Strategy endpoint
	mux.HandleFunc("/api/v1/strategy", hs.strategyHandler)

	// Decision endpoints
	mux.HandleFunc("/api/v1/decisions", hs.decisionsHandler)
	mux.HandleFunc("/api/v1/decisions/", hs.decisionDetailHandler)

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("captain")
	var httpHandler http.Handler = mux
	httpHandler = auth.WrapHandler(httpHandler, auth.SetupMiddleware(authCfg))

	hs.server = &http.Server{
		Addr:           addr,
		Handler:        http.MaxBytesHandler(httpHandler, 10*1024*1024), // 10MB max
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
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "captain api: ListenAndServe: %v\n", err)
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP server
func (hs *HTTPServer) Stop() error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if hs.server == nil {
		return nil
	}

	ctx, cancel := timeoutContext(5 * time.Second)
	defer cancel()

	return hs.server.Shutdown(ctx)
}

// ============================================================================
// HANDLERS
// ============================================================================

// healthHandler responds to health checks
func (hs *HTTPServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"service":   "captain",
		"status":    "healthy",
		"version":   "0.1.0",
		"timestamp": time.Now().UTC(),
	})
}

// readyHandler responds to readiness checks
func (hs *HTTPServer) readyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	// Check if service is closed
	if hs.service.IsClosed() {
		hs.writeError(w, http.StatusServiceUnavailable, "SERVICE_CLOSED", "Service is shutting down")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// metricsHandler returns Prometheus-style metrics
func (hs *HTTPServer) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	hs.metrics.mu.RLock()
	metrics := fmt.Sprintf(
		"# HELP captain_http_requests_total Total HTTP requests\n"+
			"# TYPE captain_http_requests_total counter\n"+
			"captain_http_requests_total %d\n"+
			"captain_http_requests_success %d\n"+
			"captain_http_requests_error %d\n",
		hs.metrics.RequestsTotal,
		hs.metrics.RequestsSuccess,
		hs.metrics.RequestsError,
	)
	hs.metrics.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(metrics))
}

// visionHandler returns the project vision
func (hs *HTTPServer) visionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	vision, err := hs.service.GetVision(r.Context())
	if err != nil {
		hs.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	hs.writeSuccess(w, http.StatusOK, vision)
}

// strategyHandler returns the strategic plan
func (hs *HTTPServer) strategyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	strategy, err := hs.service.GetStrategy(r.Context())
	if err != nil {
		hs.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	hs.writeSuccess(w, http.StatusOK, strategy)
}

// decisionsHandler handles decision listing and creation
func (hs *HTTPServer) decisionsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		hs.listDecisionsHandler(w, r)
	case http.MethodPost:
		hs.createDecisionHandler(w, r)
	default:
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET or POST only")
	}
}

// listDecisionsHandler lists decisions with pagination
func (hs *HTTPServer) listDecisionsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	limit := 10
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	decisions, err := hs.service.ListDecisions(r.Context(), limit, offset)
	if err != nil {
		hs.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if decisions == nil {
		decisions = []*Decision{}
	}

	hs.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"decisions": decisions,
		"limit":     limit,
		"offset":    offset,
		"total":     len(decisions),
	})
}

// createDecisionHandler creates a new decision
func (hs *HTTPServer) createDecisionHandler(w http.ResponseWriter, r *http.Request) {
	var decision Decision
	if err := httputil.DecodeJSONBody(r, 10*1024, &decision); err != nil {
		if reqErr, ok := httputil.IsRequestError(err); ok {
			hs.writeError(w, reqErr.Status, reqErr.Code, reqErr.Message)
			return
		}
		hs.writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	// Log decision
	if err := hs.service.LogDecision(r.Context(), &decision); err != nil {
		hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	hs.writeSuccess(w, http.StatusCreated, decision)
}

// decisionDetailHandler handles individual decision operations
func (hs *HTTPServer) decisionDetailHandler(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	id := r.URL.Path[len("/api/v1/decisions/"):]
	if id == "" {
		hs.writeError(w, http.StatusBadRequest, "MISSING_ID", "decision ID required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		hs.getDecisionHandler(w, r, id)
	case http.MethodPatch:
		hs.updateDecisionHandler(w, r, id)
	default:
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET or PATCH only")
	}
}

// getDecisionHandler retrieves a specific decision
func (hs *HTTPServer) getDecisionHandler(w http.ResponseWriter, r *http.Request, id string) {
	decision, err := hs.service.GetDecision(r.Context(), id)
	if err != nil {
		hs.writeError(w, http.StatusNotFound, "NOT_FOUND", "Decision not found")
		return
	}

	hs.writeSuccess(w, http.StatusOK, decision)
}

// updateDecisionHandler updates a decision's status
func (hs *HTTPServer) updateDecisionHandler(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Status string `json:"status"`
	}

	if err := httputil.DecodeJSONBody(r, httputil.DefaultMaxBodySize, &req); err != nil {
		if reqErr, ok := httputil.IsRequestError(err); ok {
			hs.writeError(w, reqErr.Status, reqErr.Code, reqErr.Message)
			return
		}
		hs.writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	if err := hs.service.UpdateDecisionStatus(r.Context(), id, req.Status); err != nil {
		hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	decision, _ := hs.service.GetDecision(r.Context(), id)
	hs.writeSuccess(w, http.StatusOK, decision)
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

	httputil.WriteSuccess(w, code, data)
}

// writeError writes an error response
func (hs *HTTPServer) writeError(w http.ResponseWriter, code int, errCode, msg string) {
	hs.metrics.mu.Lock()
	hs.metrics.RequestsTotal++
	hs.metrics.RequestsError++
	hs.metrics.mu.Unlock()

	httputil.WriteError(w, code, errCode, msg)
}

// timeoutContext creates a context with timeout from the given duration.
// Uses the parent request context so cancellation propagates correctly.
func timeoutContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
