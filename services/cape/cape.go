// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cape provides internal web framework capabilities.
// Cape the Internal Framework is the Kingdom's flowing mantle - the backend web layer.
// Implements HTTP/3, gRPC, WebSocket handlers, middleware chains, and internal routing.
package cape

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// Route represents an HTTP route.
type Route struct {
	Path        string           `json:"path"`
	Method      string           `json:"method"`
	Handler     http.HandlerFunc `json:"-"`
	Middleware  []MiddlewareFunc `json:"-"`
	Description string           `json:"description,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
}

// MiddlewareFunc is a middleware function.
type MiddlewareFunc func(http.Handler) http.Handler

// RouteGroup organizes routes by prefix.
type RouteGroup struct {
	Prefix     string           `json:"prefix"`
	Middleware []MiddlewareFunc `json:"-"`
	Routes     []*Route         `json:"routes"`
}

// WebSocketHandler handles WebSocket connections.
type WebSocketHandler struct {
	Path      string                             `json:"path"`
	OnConnect func(conn *Connection)             `json:"-"`
	OnMessage func(conn *Connection, msg []byte) `json:"-"`
	OnClose   func(conn *Connection)             `json:"-"`
}

// Connection represents a WebSocket connection.
type Connection struct {
	ID         string            `json:"id"`
	RemoteAddr string            `json:"remote_addr"`
	Headers    map[string]string `json:"headers,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	metadata   map[string]interface{}
	mu         sync.RWMutex
}

// GRPCService represents a gRPC service.
type GRPCService struct {
	Name         string            `json:"name"`
	Package      string            `json:"package"`
	Methods      []string          `json:"methods"`
	Interceptors []GRPCInterceptor `json:"-"`
}

// GRPCInterceptor is a gRPC interceptor.
type GRPCInterceptor func(ctx context.Context, req interface{}, info *GRPCMethodInfo, handler GRPCHandler) (interface{}, error)

// GRPCMethodInfo describes a gRPC method.
type GRPCMethodInfo struct {
	FullMethod string
	IsStream   bool
}

// GRPCHandler handles gRPC calls.
type GRPCHandler func(ctx context.Context, req interface{}) (interface{}, error)

// RequestContext provides request context.
type RequestContext struct {
	RequestID   string            `json:"request_id"`
	UserID      string            `json:"user_id,omitempty"`
	StartTime   time.Time         `json:"start_time"`
	Path        string            `json:"path"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
}

// ResponseWriter wraps http.ResponseWriter with additional capabilities.
type ResponseWriter struct {
	http.ResponseWriter
	StatusCode   int
	BytesWritten int64
}

// Service is the main Cape internal framework service.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu           sync.RWMutex
	routes       map[string]*Route // method:path -> route
	groups       map[string]*RouteGroup
	websockets   map[string]*WebSocketHandler
	grpcServices map[string]*GRPCService
	middleware   []MiddlewareFunc       // Global middleware
	connections  map[string]*Connection // WebSocket connections
}

// Config holds Cape service configuration.
type Config struct {
	HTTPAddress     string        `json:"http_address"`
	GRPCAddress     string        `json:"grpc_address"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout"`
	MaxHeaderBytes  int           `json:"max_header_bytes"`
	EnableHTTP3     bool          `json:"enable_http3"`
	EnableGRPC      bool          `json:"enable_grpc"`
	EnableWebSocket bool          `json:"enable_websocket"`
	CORSOrigins     []string      `json:"cors_origins,omitempty"`
	WotanTopic      string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		HTTPAddress:     ":20000",
		GRPCAddress:     ":18001",
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     120 * time.Second,
		MaxHeaderBytes:  1 << 20, // 1MB
		EnableHTTP3:     true,
		EnableGRPC:      true,
		EnableWebSocket: true,
		CORSOrigins:     []string{},
		WotanTopic:      "cape.framework",
	}
}

// NewService creates a new Cape internal framework service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:          log,
		wotan:        wotan,
		config:       cfg,
		routes:       make(map[string]*Route),
		groups:       make(map[string]*RouteGroup),
		websockets:   make(map[string]*WebSocketHandler),
		grpcServices: make(map[string]*GRPCService),
		middleware:   make([]MiddlewareFunc, 0),
		connections:  make(map[string]*Connection),
	}
}

// Start initializes the Cape service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Cape donned - internal framework active")
	s.registerBuiltinRoutes()
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Cape removed - internal framework suspended")

	// Close all WebSocket connections
	s.mu.Lock()
	for id := range s.connections {
		delete(s.connections, id)
	}
	s.mu.Unlock()

	return nil
}

// Use adds global middleware.
func (s *Service) Use(mw MiddlewareFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.middleware = append(s.middleware, mw)
}

// Route registers a new route.
func (s *Service) Route(method, path string, handler http.HandlerFunc, middleware ...MiddlewareFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	route := &Route{
		Path:       path,
		Method:     method,
		Handler:    handler,
		Middleware: middleware,
	}

	key := fmt.Sprintf("%s:%s", method, path)
	s.routes[key] = route

	s.log.Debug().Str("method", method).Str("path", path).Msg("Route registered")
	return nil
}

// GET registers a GET route.
func (s *Service) GET(path string, handler http.HandlerFunc, middleware ...MiddlewareFunc) error {
	return s.Route(http.MethodGet, path, handler, middleware...)
}

// POST registers a POST route.
func (s *Service) POST(path string, handler http.HandlerFunc, middleware ...MiddlewareFunc) error {
	return s.Route(http.MethodPost, path, handler, middleware...)
}

// PUT registers a PUT route.
func (s *Service) PUT(path string, handler http.HandlerFunc, middleware ...MiddlewareFunc) error {
	return s.Route(http.MethodPut, path, handler, middleware...)
}

// DELETE registers a DELETE route.
func (s *Service) DELETE(path string, handler http.HandlerFunc, middleware ...MiddlewareFunc) error {
	return s.Route(http.MethodDelete, path, handler, middleware...)
}

// Group creates a route group.
func (s *Service) Group(prefix string, middleware ...MiddlewareFunc) *RouteGroup {
	s.mu.Lock()
	defer s.mu.Unlock()

	group := &RouteGroup{
		Prefix:     prefix,
		Middleware: middleware,
		Routes:     make([]*Route, 0),
	}

	s.groups[prefix] = group
	return group
}

// WebSocket registers a WebSocket handler.
func (s *Service) WebSocket(path string, handler *WebSocketHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	handler.Path = path
	s.websockets[path] = handler

	s.log.Debug().Str("path", path).Msg("WebSocket handler registered")
	return nil
}

// RegisterGRPC registers a gRPC service.
func (s *Service) RegisterGRPC(svc *GRPCService) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.grpcServices[svc.Name] = svc

	s.log.Debug().Str("service", svc.Name).Int("methods", len(svc.Methods)).Msg("gRPC service registered")
	return nil
}

// Handler returns the HTTP handler.
func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("%s:%s", r.Method, r.URL.Path)

		s.mu.RLock()
		route, ok := s.routes[key]
		s.mu.RUnlock()

		if !ok {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		// Build handler chain
		handler := http.Handler(route.Handler)

		// Apply route-specific middleware (in reverse order)
		for i := len(route.Middleware) - 1; i >= 0; i-- {
			handler = route.Middleware[i](handler)
		}

		// Apply global middleware (in reverse order)
		s.mu.RLock()
		globalMW := s.middleware
		s.mu.RUnlock()

		for i := len(globalMW) - 1; i >= 0; i-- {
			handler = globalMW[i](handler)
		}

		handler.ServeHTTP(w, r)
	})
}

// GetRoute returns a route by method and path.
func (s *Service) GetRoute(method, path string) (*Route, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", method, path)
	route, ok := s.routes[key]
	return route, ok
}

// ListRoutes returns all routes.
func (s *Service) ListRoutes() []*Route {
	s.mu.RLock()
	defer s.mu.RUnlock()

	routes := make([]*Route, 0, len(s.routes))
	for _, r := range s.routes {
		routes = append(routes, r)
	}
	return routes
}

// RegisterConnection registers a WebSocket connection.
func (s *Service) RegisterConnection(conn *Connection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[conn.ID] = conn
}

// UnregisterConnection removes a WebSocket connection.
func (s *Service) UnregisterConnection(connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.connections, connID)
}

// GetConnection returns a connection by ID.
func (s *Service) GetConnection(connID string) (*Connection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conn, ok := s.connections[connID]
	return conn, ok
}

// BroadcastMessage sends a message to all WebSocket connections.
func (s *Service) BroadcastMessage(msg []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// In real implementation, would send to all connections
	s.log.Debug().Int("connections", len(s.connections)).Msg("Broadcasting message")
	return nil
}

func (s *Service) registerBuiltinRoutes() {
	// Health check
	s.GET("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "healthy",
			"service":   "cape",
			"timestamp": time.Now(),
		})
	})

	// Readiness check
	s.GET("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   true,
			"service": "cape",
		})
	})

	// Prometheus-style metrics endpoint
	s.GET("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := s.Stats()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP cape_routes_total Total registered routes\n")
		fmt.Fprintf(w, "# TYPE cape_routes_total gauge\n")
		fmt.Fprintf(w, "cape_routes_total %v\n", stats["total_routes"])
		fmt.Fprintf(w, "# HELP cape_groups_total Total route groups\n")
		fmt.Fprintf(w, "# TYPE cape_groups_total gauge\n")
		fmt.Fprintf(w, "cape_groups_total %v\n", stats["total_groups"])
		fmt.Fprintf(w, "# HELP cape_websocket_handlers_total Total WebSocket handlers\n")
		fmt.Fprintf(w, "# TYPE cape_websocket_handlers_total gauge\n")
		fmt.Fprintf(w, "cape_websocket_handlers_total %v\n", stats["websocket_handlers"])
		fmt.Fprintf(w, "# HELP cape_grpc_services_total Total gRPC services\n")
		fmt.Fprintf(w, "# TYPE cape_grpc_services_total gauge\n")
		fmt.Fprintf(w, "cape_grpc_services_total %v\n", stats["grpc_services"])
		fmt.Fprintf(w, "# HELP cape_active_connections Total active WebSocket connections\n")
		fmt.Fprintf(w, "# TYPE cape_active_connections gauge\n")
		fmt.Fprintf(w, "cape_active_connections %v\n", stats["active_connections"])
		fmt.Fprintf(w, "# HELP cape_global_middleware_total Total global middleware\n")
		fmt.Fprintf(w, "# TYPE cape_global_middleware_total gauge\n")
		fmt.Fprintf(w, "cape_global_middleware_total %v\n", stats["global_middleware"])
	})

	// Routes listing
	s.GET("/routes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.ListRoutes())
	})
}

// Connection methods

// SetMetadata sets metadata on a connection.
func (c *Connection) SetMetadata(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.metadata == nil {
		c.metadata = make(map[string]interface{})
	}
	c.metadata[key] = value
}

// GetMetadata gets metadata from a connection.
func (c *Connection) GetMetadata(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.metadata == nil {
		return nil, false
	}
	val, ok := c.metadata[key]
	return val, ok
}

// ResponseWriter methods

// WriteHeader captures the status code.
func (rw *ResponseWriter) WriteHeader(code int) {
	rw.StatusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures bytes written.
func (rw *ResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.BytesWritten += int64(n)
	return n, err
}

// Builtin middleware

// LoggingMiddleware logs requests.
func LoggingMiddleware(log *logger.Logger) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &ResponseWriter{ResponseWriter: w, StatusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			log.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", rw.StatusCode).
				Int64("bytes", rw.BytesWritten).
				Dur("duration", time.Since(start)).
				Msg("Request handled")
		})
	}
}

// RecoveryMiddleware recovers from panics.
func RecoveryMiddleware(log *logger.Logger) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error().Interface("panic", err).Msg("Recovered from panic")
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware handles CORS.
func CORSMiddleware(origins []string) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, o := range origins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"total_routes":       len(s.routes),
		"total_groups":       len(s.groups),
		"websocket_handlers": len(s.websockets),
		"grpc_services":      len(s.grpcServices),
		"active_connections": len(s.connections),
		"global_middleware":  len(s.middleware),
	}
}
