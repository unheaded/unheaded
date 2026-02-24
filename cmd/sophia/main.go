// Package main provides the HTTP API entry point for the Sophia service.
// Sophia is the wisdom and knowledge management service - providing intelligent
// insights, managing knowledge graphs, and offering decision support through
// pattern recognition and inference.
//
// Endpoints:
//   - GET /health - Health check
//   - GET /ready - Readiness check
//   - GET /metrics - Prometheus metrics
//   - POST /api/v1/knowledge - Add knowledge (Learn)
//   - GET /api/v1/knowledge - Query knowledge
//   - GET /api/v1/insights - Get insights
//   - POST /api/v1/decide - Request decision recommendation
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
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/logger"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/sophia"
)

var (
	// Version information (set by build)
	Version   = "0.1.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

var (
	listenAddr = flag.String("listen", ":19005", "HTTP listen address")
	wotanAddr = flag.String("wotan", "localhost:18001", "Wotan server address")
	debug      = flag.Bool("debug", false, "Enable debug logging")
	jsonLogs   = flag.Bool("json", false, "Output logs in JSON format")
)

// HTTPServer provides REST API endpoints for the Sophia service.
type HTTPServer struct {
	service   *sophia.Service
	wotan    *wotanClient.Client
	log       *logger.Logger
	server    *http.Server
	metrics   *HTTPMetrics
	requestID int64
	mu        sync.RWMutex
	ready     atomic.Bool
}

// HTTPMetrics tracks HTTP metrics for Prometheus.
type HTTPMetrics struct {
	RequestsTotal     int64
	RequestsSuccess   int64
	RequestsError     int64
	KnowledgeLearned  int64
	KnowledgeQueried  int64
	InsightsGenerated int64
	DecisionsMade     int64
	mu                sync.RWMutex
}

// ResponseEnvelope wraps API responses.
type ResponseEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
	Meta    *MetaInfo   `json:"meta,omitempty"`
}

// ErrorInfo represents error details in responses.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// MetaInfo represents request metadata in responses.
type MetaInfo struct {
	RequestID string `json:"request_id"`
	Duration  int64  `json:"duration_ms"`
	Timestamp string `json:"timestamp"`
}

// LearnRequest represents the request body for adding knowledge.
type LearnRequest struct {
	Type       string                 `json:"type"`
	Subject    string                 `json:"subject"`
	Predicate  string                 `json:"predicate"`
	Object     string                 `json:"object"`
	Confidence float64                `json:"confidence,omitempty"`
	Source     string                 `json:"source,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// QueryRequest represents the request body for querying knowledge.
type QueryRequest struct {
	Type          string                 `json:"type,omitempty"`
	Subject       string                 `json:"subject,omitempty"`
	Predicate     string                 `json:"predicate,omitempty"`
	Object        string                 `json:"object,omitempty"`
	Text          string                 `json:"text,omitempty"`
	Filters       map[string]interface{} `json:"filters,omitempty"`
	Limit         int                    `json:"limit,omitempty"`
	MinConfidence float64                `json:"min_confidence,omitempty"`
}

// DecideRequest represents the request body for decision recommendations.
type DecideRequest struct {
	Question string          `json:"question"`
	Options  []sophia.Option `json:"options"`
	Factors  []sophia.Factor `json:"factors,omitempty"`
	Context  map[string]interface{} `json:"context,omitempty"`
}

// NewHTTPServer creates a new HTTP server for Sophia.
func NewHTTPServer(service *sophia.Service, wotan *wotanClient.Client, log *logger.Logger, addr string) (*HTTPServer, error) {
	if service == nil {
		return nil, errors.New("service cannot be nil")
	}
	if log == nil {
		return nil, errors.New("logger cannot be nil")
	}
	if addr == "" {
		addr = ":19005"
	}

	hs := &HTTPServer{
		service: service,
		wotan:  wotan,
		log:     log,
		metrics: &HTTPMetrics{},
	}

	mux := http.NewServeMux()

	// Health check endpoints
	mux.HandleFunc("/health", hs.healthHandler)
	mux.HandleFunc("/ready", hs.readyHandler)

	// Metrics endpoint
	mux.HandleFunc("/metrics", hs.metricsHandler)

	// Knowledge API endpoints
	mux.HandleFunc("/api/v1/knowledge", hs.knowledgeHandler)

	// Insights endpoint
	mux.HandleFunc("/api/v1/insights", hs.insightsHandler)

	// Decision endpoint
	mux.HandleFunc("/api/v1/decide", hs.decideHandler)

	// Stats endpoint
	mux.HandleFunc("/api/v1/stats", hs.statsHandler)

	hs.server = &http.Server{
		Addr:         addr,
		Handler:      hs.middlewareChain(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	return hs, nil
}

// middlewareChain applies middleware to the handler.
func (hs *HTTPServer) middlewareChain(handler http.Handler) http.Handler {
	// Apply middleware in order: logging -> recovery -> auth -> max body -> cors
	handler = hs.loggingMiddleware(handler)
	handler = hs.recoveryMiddleware(handler)

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("sophia")
	handler = auth.WrapHandler(handler, auth.SetupMiddleware(authCfg))

	handler = http.MaxBytesHandler(handler, 10*1024*1024) // 10MB max
	handler = hs.corsMiddleware(handler)
	return handler
}

// loggingMiddleware logs HTTP requests.
func (hs *HTTPServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: 200}

		next.ServeHTTP(wrapped, r)

		hs.log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.status).
			Dur("duration", time.Since(start)).
			Str("remote_addr", r.RemoteAddr).
			Msg("HTTP request")
	})
}

// recoveryMiddleware recovers from panics.
func (hs *HTTPServer) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				hs.log.Error().
					Interface("panic", err).
					Str("path", r.URL.Path).
					Msg("recovered from panic")
				hs.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers.
func (hs *HTTPServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
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

// Start starts the HTTP server.
func (hs *HTTPServer) Start(ctx context.Context) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if hs.server == nil {
		return errors.New("server not initialized")
	}

	// Mark as ready
	hs.ready.Store(true)

	hs.log.Info().
		Str("addr", hs.server.Addr).
		Msg("Sophia HTTP server starting")

	go func() {
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hs.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the HTTP server.
func (hs *HTTPServer) Shutdown(ctx context.Context) error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.ready.Store(false)

	if hs.server == nil {
		return nil
	}

	hs.log.Info().Msg("Sophia HTTP server shutting down")
	return hs.server.Shutdown(ctx)
}

// ============================================================================
// HANDLERS
// ============================================================================

// healthHandler responds to health checks.
func (hs *HTTPServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "sophia",
		"version":   Version,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// readyHandler responds to readiness checks.
func (hs *HTTPServer) readyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	if !hs.ready.Load() {
		hs.writeError(w, http.StatusServiceUnavailable, "NOT_READY", "service is not ready")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ready",
		"service":   "sophia",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// metricsHandler returns Prometheus-style metrics.
func (hs *HTTPServer) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	hs.metrics.mu.RLock()
	stats := hs.service.Stats()
	metrics := fmt.Sprintf(
		"# HELP sophia_http_requests_total Total HTTP requests\n"+
			"# TYPE sophia_http_requests_total counter\n"+
			"sophia_http_requests_total %d\n"+
			"sophia_http_requests_success %d\n"+
			"sophia_http_requests_error %d\n"+
			"# HELP sophia_knowledge_total Total knowledge items\n"+
			"# TYPE sophia_knowledge_total gauge\n"+
			"sophia_knowledge_total %d\n"+
			"sophia_knowledge_learned %d\n"+
			"sophia_knowledge_queried %d\n"+
			"# HELP sophia_insights_total Total insights generated\n"+
			"# TYPE sophia_insights_total counter\n"+
			"sophia_insights_total %d\n"+
			"# HELP sophia_decisions_total Total decisions made\n"+
			"# TYPE sophia_decisions_total counter\n"+
			"sophia_decisions_total %d\n",
		hs.metrics.RequestsTotal,
		hs.metrics.RequestsSuccess,
		hs.metrics.RequestsError,
		stats["total_knowledge"],
		hs.metrics.KnowledgeLearned,
		hs.metrics.KnowledgeQueried,
		hs.metrics.InsightsGenerated,
		hs.metrics.DecisionsMade,
	)
	hs.metrics.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(metrics))
}

// knowledgeHandler routes knowledge operations.
func (hs *HTTPServer) knowledgeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		hs.queryKnowledgeHandler(w, r)
	case http.MethodPost:
		hs.learnKnowledgeHandler(w, r)
	default:
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET or POST only")
	}
}

// learnKnowledgeHandler handles POST /api/v1/knowledge.
func (hs *HTTPServer) learnKnowledgeHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Validate content type
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		hs.writeError(w, http.StatusUnsupportedMediaType, "INVALID_CONTENT_TYPE", "application/json required")
		return
	}

	// Read body
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024)) // 64KB max
	if err != nil {
		hs.writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	// Parse request
	var req LearnRequest
	if err := json.Unmarshal(body, &req); err != nil {
		hs.writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	// Validate request
	if req.Subject == "" {
		hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "subject is required")
		return
	}
	if req.Predicate == "" {
		hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "predicate is required")
		return
	}
	if req.Object == "" {
		hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "object is required")
		return
	}

	// Convert to knowledge type
	kType := sophia.KnowledgeFact
	switch req.Type {
	case "fact":
		kType = sophia.KnowledgeFact
	case "inference":
		kType = sophia.KnowledgeInference
	case "heuristic":
		kType = sophia.KnowledgeHeuristic
	case "pattern":
		kType = sophia.KnowledgePattern
	case "constraint":
		kType = sophia.KnowledgeConstraint
	case "goal":
		kType = sophia.KnowledgeGoal
	}

	// Create knowledge
	knowledge := &sophia.Knowledge{
		Type:       kType,
		Subject:    req.Subject,
		Predicate:  req.Predicate,
		Object:     req.Object,
		Confidence: req.Confidence,
		Source:     req.Source,
		Metadata:   req.Metadata,
	}

	// Learn
	ctx := r.Context()
	if err := hs.service.Learn(ctx, knowledge); err != nil {
		hs.log.Error().Err(err).Msg("failed to learn knowledge")
		hs.writeError(w, http.StatusInternalServerError, "LEARN_ERROR", err.Error())
		return
	}

	// Update metrics
	hs.metrics.mu.Lock()
	hs.metrics.KnowledgeLearned++
	hs.metrics.mu.Unlock()

	hs.log.Debug().
		Str("id", knowledge.ID).
		Str("subject", knowledge.Subject).
		Str("predicate", knowledge.Predicate).
		Msg("knowledge learned")

	hs.writeSuccess(w, http.StatusCreated, map[string]interface{}{
		"knowledge": knowledge,
	}, start)
}

// queryKnowledgeHandler handles GET /api/v1/knowledge.
func (hs *HTTPServer) queryKnowledgeHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Build query from URL parameters
	q := &sophia.Query{
		Subject:   r.URL.Query().Get("subject"),
		Predicate: r.URL.Query().Get("predicate"),
		Object:    r.URL.Query().Get("object"),
		Text:      r.URL.Query().Get("text"),
	}

	// Parse query type
	switch r.URL.Query().Get("type") {
	case "exact":
		q.Type = sophia.QueryExact
	case "semantic":
		q.Type = sophia.QuerySemantic
	case "graph":
		q.Type = sophia.QueryGraph
	case "infer":
		q.Type = sophia.QueryInfer
	default:
		q.Type = sophia.QueryExact
	}

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var limit int
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && limit > 0 {
			q.Limit = limit
		}
	}
	if q.Limit == 0 {
		q.Limit = 100 // default
	}

	// Parse min confidence
	if confStr := r.URL.Query().Get("min_confidence"); confStr != "" {
		var conf float64
		if _, err := fmt.Sscanf(confStr, "%f", &conf); err == nil {
			q.MinConfidence = conf
		}
	}

	// Execute query
	ctx := r.Context()
	results, err := hs.service.Query(ctx, q)
	if err != nil {
		hs.log.Error().Err(err).Msg("failed to query knowledge")
		hs.writeError(w, http.StatusInternalServerError, "QUERY_ERROR", err.Error())
		return
	}

	// Update metrics
	hs.metrics.mu.Lock()
	hs.metrics.KnowledgeQueried++
	hs.metrics.mu.Unlock()

	hs.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"knowledge": results,
		"count":     len(results),
		"query":     q,
	}, start)
}

// insightsHandler handles GET /api/v1/insights.
func (hs *HTTPServer) insightsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	start := time.Now()

	// Get insights from service stats
	stats := hs.service.Stats()

	// For now, return insights summary
	// In a full implementation, we would have a GetInsights() method
	hs.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"total_insights": stats["total_insights"],
		"service_stats":  stats,
	}, start)
}

// decideHandler handles POST /api/v1/decide.
func (hs *HTTPServer) decideHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "POST only")
		return
	}

	start := time.Now()

	// Validate content type
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		hs.writeError(w, http.StatusUnsupportedMediaType, "INVALID_CONTENT_TYPE", "application/json required")
		return
	}

	// Read body
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024)) // 64KB max
	if err != nil {
		hs.writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	// Parse request
	var req DecideRequest
	if err := json.Unmarshal(body, &req); err != nil {
		hs.writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	// Validate request
	if req.Question == "" {
		hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "question is required")
		return
	}
	if len(req.Options) == 0 {
		hs.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "at least one option is required")
		return
	}

	// Make decision
	ctx := r.Context()
	decision, err := hs.service.Decide(ctx, req.Question, req.Options, req.Factors)
	if err != nil {
		hs.log.Error().Err(err).Msg("failed to make decision")
		hs.writeError(w, http.StatusInternalServerError, "DECISION_ERROR", err.Error())
		return
	}

	// Add context if provided
	if req.Context != nil {
		decision.Context = req.Context
	}

	// Update metrics
	hs.metrics.mu.Lock()
	hs.metrics.DecisionsMade++
	hs.metrics.mu.Unlock()

	hs.log.Info().
		Str("decision_id", decision.ID).
		Str("question", decision.Question).
		Str("recommendation", decision.Recommendation).
		Float64("confidence", decision.Confidence).
		Msg("decision made")

	hs.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"decision": decision,
	}, start)
}

// statsHandler handles GET /api/v1/stats.
func (hs *HTTPServer) statsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		hs.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET only")
		return
	}

	start := time.Now()
	stats := hs.service.Stats()

	hs.metrics.mu.RLock()
	httpStats := map[string]interface{}{
		"requests_total":     hs.metrics.RequestsTotal,
		"requests_success":   hs.metrics.RequestsSuccess,
		"requests_error":     hs.metrics.RequestsError,
		"knowledge_learned":  hs.metrics.KnowledgeLearned,
		"knowledge_queried":  hs.metrics.KnowledgeQueried,
		"insights_generated": hs.metrics.InsightsGenerated,
		"decisions_made":     hs.metrics.DecisionsMade,
	}
	hs.metrics.mu.RUnlock()

	hs.writeSuccess(w, http.StatusOK, map[string]interface{}{
		"service":    stats,
		"http":       httpStats,
		"version":    Version,
		"build_time": BuildTime,
		"git_commit": GitCommit,
	}, start)
}

// ============================================================================
// RESPONSE WRITERS
// ============================================================================

// writeSuccess writes a successful response.
func (hs *HTTPServer) writeSuccess(w http.ResponseWriter, code int, data interface{}, start time.Time) {
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
			RequestID: fmt.Sprintf("sophia_%d", hs.getNextRequestID()),
			Duration:  time.Since(start).Milliseconds(),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// writeError writes an error response.
func (hs *HTTPServer) writeError(w http.ResponseWriter, code int, errCode, msg string) {
	hs.metrics.mu.Lock()
	hs.metrics.RequestsTotal++
	hs.metrics.RequestsError++
	hs.metrics.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	resp := ResponseEnvelope{
		Success: false,
		Error: &ErrorInfo{
			Code:    errCode,
			Message: msg,
		},
		Meta: &MetaInfo{
			RequestID: fmt.Sprintf("sophia_%d", hs.getNextRequestID()),
			Duration:  0,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// getNextRequestID returns the next request ID.
func (hs *HTTPServer) getNextRequestID() int64 {
	return atomic.AddInt64(&hs.requestID, 1)
}

// ============================================================================
// MAIN
// ============================================================================

func main() {
	flag.Parse()

	// Override from environment
	if addr := os.Getenv("SOPHIA_LISTEN_ADDR"); addr != "" {
		*listenAddr = addr
	}
	if addr := os.Getenv("WOTAN_ADDR"); addr != "" {
		*wotanAddr = addr
	}
	if os.Getenv("SOPHIA_DEBUG") == "true" {
		*debug = true
	}
	if os.Getenv("SOPHIA_JSON_LOGS") == "true" {
		*jsonLogs = true
	}

	// Transport config
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	if *wotanAddr != "" {
		transportCfg.WotanGRPCAddr = *wotanAddr
	}

	// Health server
	healthSrv := transport.NewHealthServer("sophia")
	_ = transportCfg // used for future transport.Connect()

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
		Str("service", "sophia").
		Str("version", Version).
		Str("build_time", BuildTime).
		Str("git_commit", GitCommit).
		Msg("Sophia awakens - wisdom flows through the Kingdom")

	// Connect to Wotan
	var wotanInstance *wotanClient.Client
	wotanEnabled := os.Getenv("WOTAN_ENABLED") != "false"

	if wotanEnabled && *wotanAddr != "" {
		log.Info().
			Str("wotan_addr", *wotanAddr).
			Msg("connecting to Wotan")

		client, err := wotanClient.NewClient(*wotanAddr)
		if err != nil {
			log.Warn().
				Err(err).
				Msg("failed to connect to Wotan, continuing without event bus")
			healthSrv.SetGRPCStatus(false)
		} else {
			wotanInstance = client
			log.Info().Msg("connected to Wotan")
		}
	} else {
		log.Warn().Msg("Wotan disabled, running standalone")
	}

	// Create Sophia service configuration
	sophiaConfig := sophia.DefaultConfig()
	if ttlStr := os.Getenv("SOPHIA_INSIGHT_TTL"); ttlStr != "" {
		if ttl, err := time.ParseDuration(ttlStr); err == nil {
			sophiaConfig.InsightTTL = ttl
		}
	}
	if confStr := os.Getenv("SOPHIA_MIN_CONFIDENCE"); confStr != "" {
		var conf float64
		if _, err := fmt.Sscanf(confStr, "%f", &conf); err == nil {
			sophiaConfig.MinConfidence = conf
		}
	}
	if os.Getenv("SOPHIA_ENABLE_INFERENCE") == "false" {
		sophiaConfig.EnableInference = false
	}

	// Create Sophia service
	sophiaService := sophia.NewService(log, wotanInstance, sophiaConfig)

	// Start the Sophia service
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sophiaService.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start Sophia service")
	}

	// Create HTTP server
	httpServer, err := NewHTTPServer(sophiaService, wotanInstance, log, *listenAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create HTTP server")
	}

	// Start HTTP server
	if err := httpServer.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start HTTP server")
	}

	log.Info().
		Str("addr", *listenAddr).
		Str("wotan", *wotanAddr).
		Bool("wotan_connected", wotanInstance != nil).
		Bool("inference_enabled", sophiaConfig.EnableInference).
		Msg("Sophia HTTP server running")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Info().
		Str("signal", sig.String()).
		Msg("shutdown signal received")

	// Cancel context to signal all goroutines
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown HTTP server
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	// Stop Sophia service
	if err := sophiaService.Stop(); err != nil {
		log.Error().Err(err).Msg("Sophia service stop error")
	}

	// Close Wotan connection
	if wotanInstance != nil {
		if err := wotanInstance.Close(); err != nil {
			log.Error().Err(err).Msg("Wotan client close error")
		}
	}

	log.Info().Msg("Sophia rests - wisdom preserved")
}
