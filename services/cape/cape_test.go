package cape

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestLogger returns a logger that discards output (safe for concurrent use).
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestWotan creates a minimal *wotanClient.Client for tests.
// We only need the struct pointer; the cape service never calls wotan
// methods during the paths we exercise.
func newTestWotan() *wotanClient.Client {
	c, _ := wotanClient.NewClient("localhost:19999")
	return c
}

// newTestService returns a ready-to-use *Service with defaults.
func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(newTestLogger(), newTestWotan(), nil)
}

// newStartedService returns a *Service that has been Start()'d.
func newStartedService(t *testing.T) *Service {
	t.Helper()
	svc := newTestService(t)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	return svc
}

// dummyHandler is a trivial HTTP handler for route tests.
func dummyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

// ---------------------------------------------------------------------------
// 1. DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"HTTPAddress", cfg.HTTPAddress, ":8080"},
		{"GRPCAddress", cfg.GRPCAddress, ":9090"},
		{"ReadTimeout", cfg.ReadTimeout, 30 * time.Second},
		{"WriteTimeout", cfg.WriteTimeout, 30 * time.Second},
		{"IdleTimeout", cfg.IdleTimeout, 120 * time.Second},
		{"MaxHeaderBytes", cfg.MaxHeaderBytes, 1 << 20},
		{"EnableHTTP3", cfg.EnableHTTP3, true},
		{"EnableGRPC", cfg.EnableGRPC, true},
		{"EnableWebSocket", cfg.EnableWebSocket, true},
		{"WotanTopic", cfg.WotanTopic, "cape.framework"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Errorf("DefaultConfig().%s = %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}

	if len(cfg.CORSOrigins) != 0 {
		t.Errorf("CORSOrigins should be empty by default, got %v", cfg.CORSOrigins)
	}
}

// ---------------------------------------------------------------------------
// 2. NewService
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	t.Parallel()

	t.Run("nil config uses defaults", func(t *testing.T) {
		t.Parallel()
		svc := NewService(newTestLogger(), newTestWotan(), nil)
		if svc.config.HTTPAddress != ":8080" {
			t.Errorf("expected default HTTPAddress, got %s", svc.config.HTTPAddress)
		}
	})

	t.Run("custom config is preserved", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{HTTPAddress: ":9999", WotanTopic: "test.topic"}
		svc := NewService(newTestLogger(), newTestWotan(), cfg)
		if svc.config.HTTPAddress != ":9999" {
			t.Errorf("expected :9999, got %s", svc.config.HTTPAddress)
		}
		if svc.config.WotanTopic != "test.topic" {
			t.Errorf("expected test.topic, got %s", svc.config.WotanTopic)
		}
	})

	t.Run("maps are initialised", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		if svc.routes == nil {
			t.Error("routes map is nil")
		}
		if svc.groups == nil {
			t.Error("groups map is nil")
		}
		if svc.websockets == nil {
			t.Error("websockets map is nil")
		}
		if svc.grpcServices == nil {
			t.Error("grpcServices map is nil")
		}
		if svc.middleware == nil {
			t.Error("middleware slice is nil")
		}
		if svc.connections == nil {
			t.Error("connections map is nil")
		}
	})
}

// ---------------------------------------------------------------------------
// 3. Start / Stop lifecycle
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	t.Parallel()

	t.Run("Start registers builtin routes", func(t *testing.T) {
		t.Parallel()
		svc := newStartedService(t)

		builtins := []struct{ method, path string }{
			{"GET", "/health"},
			{"GET", "/ready"},
			{"GET", "/routes"},
		}
		for _, b := range builtins {
			if _, ok := svc.GetRoute(b.method, b.path); !ok {
				t.Errorf("builtin route %s %s not registered after Start", b.method, b.path)
			}
		}
	})

	t.Run("Stop clears connections", func(t *testing.T) {
		t.Parallel()
		svc := newStartedService(t)
		svc.RegisterConnection(&Connection{ID: "c1", CreatedAt: time.Now()})
		svc.RegisterConnection(&Connection{ID: "c2", CreatedAt: time.Now()})

		if err := svc.Stop(); err != nil {
			t.Fatalf("Stop() returned error: %v", err)
		}

		svc.mu.RLock()
		n := len(svc.connections)
		svc.mu.RUnlock()
		if n != 0 {
			t.Errorf("expected 0 connections after Stop, got %d", n)
		}
	})

	t.Run("Stop is idempotent", func(t *testing.T) {
		t.Parallel()
		svc := newStartedService(t)
		if err := svc.Stop(); err != nil {
			t.Fatalf("first Stop() error: %v", err)
		}
		if err := svc.Stop(); err != nil {
			t.Fatalf("second Stop() error: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// 4. Route registration (Route, GET, POST, PUT, DELETE)
// ---------------------------------------------------------------------------

func TestRouteRegistration(t *testing.T) {
	t.Parallel()

	methods := []struct {
		name   string
		reg    func(s *Service, path string, h http.HandlerFunc, mw ...MiddlewareFunc) error
		method string
	}{
		{"GET", (*Service).GET, http.MethodGet},
		{"POST", (*Service).POST, http.MethodPost},
		{"PUT", (*Service).PUT, http.MethodPut},
		{"DELETE", (*Service).DELETE, http.MethodDelete},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestService(t)
			path := "/test/" + m.name
			if err := m.reg(svc, path, dummyHandler); err != nil {
				t.Fatalf("%s registration error: %v", m.name, err)
			}
			r, ok := svc.GetRoute(m.method, path)
			if !ok {
				t.Fatalf("route %s %s not found", m.method, path)
			}
			if r.Path != path {
				t.Errorf("route path = %s, want %s", r.Path, path)
			}
			if r.Method != m.method {
				t.Errorf("route method = %s, want %s", r.Method, m.method)
			}
		})
	}

	t.Run("Route with middleware stores middleware", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		mw := func(next http.Handler) http.Handler { return next }
		if err := svc.GET("/mw", dummyHandler, mw); err != nil {
			t.Fatal(err)
		}
		r, _ := svc.GetRoute("GET", "/mw")
		if len(r.Middleware) != 1 {
			t.Errorf("expected 1 middleware, got %d", len(r.Middleware))
		}
	})

	t.Run("Route overwrites same key", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		h1 := func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "v1") }
		h2 := func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "v2") }
		svc.GET("/dup", h1)
		svc.GET("/dup", h2)

		// Serve through handler
		req := httptest.NewRequest("GET", "/dup", nil)
		rec := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rec, req)
		if rec.Body.String() != "v2" {
			t.Errorf("expected v2 (latest handler), got %s", rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// 5. GetRoute / ListRoutes
// ---------------------------------------------------------------------------

func TestGetRoute(t *testing.T) {
	t.Parallel()

	t.Run("existing route found", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		svc.GET("/x", dummyHandler)
		r, ok := svc.GetRoute("GET", "/x")
		if !ok || r == nil {
			t.Fatal("route not found")
		}
	})

	t.Run("missing route returns false", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		_, ok := svc.GetRoute("GET", "/no-such-path")
		if ok {
			t.Error("expected false for missing route")
		}
	})

	t.Run("wrong method returns false", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		svc.GET("/only-get", dummyHandler)
		_, ok := svc.GetRoute("POST", "/only-get")
		if ok {
			t.Error("expected false for wrong method")
		}
	})
}

func TestListRoutes(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	svc.GET("/a", dummyHandler)
	svc.POST("/b", dummyHandler)
	svc.PUT("/c", dummyHandler)

	routes := svc.ListRoutes()
	if len(routes) != 3 {
		t.Errorf("expected 3 routes, got %d", len(routes))
	}
}

// ---------------------------------------------------------------------------
// 6. Route Groups
// ---------------------------------------------------------------------------

func TestGroup(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	mw := func(next http.Handler) http.Handler { return next }
	group := svc.Group("/api/v1", mw)
	if group == nil {
		t.Fatal("Group returned nil")
	}
	if group.Prefix != "/api/v1" {
		t.Errorf("prefix = %s, want /api/v1", group.Prefix)
	}
	if len(group.Middleware) != 1 {
		t.Errorf("expected 1 middleware, got %d", len(group.Middleware))
	}

	// Groups stored in service map
	svc.mu.RLock()
	_, ok := svc.groups["/api/v1"]
	svc.mu.RUnlock()
	if !ok {
		t.Error("group not stored in service map")
	}
}

func TestGroupOverwrite(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	g1 := svc.Group("/api")
	g2 := svc.Group("/api")
	if g1 == g2 {
		// They are different pointer allocations but both stored under the same key.
		// The second overwrites the first.
	}
	svc.mu.RLock()
	stored := svc.groups["/api"]
	svc.mu.RUnlock()
	if stored != g2 {
		t.Error("group should be overwritten by second call")
	}
}

// ---------------------------------------------------------------------------
// 7. Global middleware (Use)
// ---------------------------------------------------------------------------

func TestUseMiddleware(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	mw1 := func(next http.Handler) http.Handler { return next }
	mw2 := func(next http.Handler) http.Handler { return next }

	svc.Use(mw1)
	svc.Use(mw2)

	svc.mu.RLock()
	n := len(svc.middleware)
	svc.mu.RUnlock()
	if n != 2 {
		t.Errorf("expected 2 global middleware, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 8. Handler() and middleware chain execution order
// ---------------------------------------------------------------------------

func TestHandler_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	req := httptest.NewRequest("GET", "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandler_MatchesRoute(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	svc.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "world")
	})

	req := httptest.NewRequest("GET", "/hello", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "world" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "world")
	}
}

func TestHandler_MiddlewareChainOrder(t *testing.T) {
	t.Parallel()

	// Track execution order with a shared slice protected by mutex.
	var mu sync.Mutex
	var order []string
	record := func(label string) {
		mu.Lock()
		order = append(order, label)
		mu.Unlock()
	}

	svc := newTestService(t)

	// Global middleware
	svc.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("global1-before")
			next.ServeHTTP(w, r)
			record("global1-after")
		})
	})
	svc.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("global2-before")
			next.ServeHTTP(w, r)
			record("global2-after")
		})
	})

	// Route-specific middleware
	routeMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("route-before")
			next.ServeHTTP(w, r)
			record("route-after")
		})
	}

	svc.GET("/chain", func(w http.ResponseWriter, r *http.Request) {
		record("handler")
		w.WriteHeader(http.StatusOK)
	}, routeMW)

	req := httptest.NewRequest("GET", "/chain", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	// Global middleware applied in reverse order (outermost first in execution),
	// then route middleware, then handler.
	// Global[0] wraps Global[1] wraps RouteMW wraps Handler.
	// So execution is: global1-before, global2-before, route-before, handler,
	//                  route-after, global2-after, global1-after
	expected := []string{
		"global1-before", "global2-before", "route-before",
		"handler",
		"route-after", "global2-after", "global1-after",
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != len(expected) {
		t.Fatalf("order length = %d, want %d; got %v", len(order), len(expected), order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Errorf("order[%d] = %s, want %s (full: %v)", i, order[i], expected[i], order)
			break
		}
	}
}

func TestHandler_RouteMiddlewareOnly(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	called := false
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			next.ServeHTTP(w, r)
		})
	}
	svc.GET("/rmw", dummyHandler, mw)

	req := httptest.NewRequest("GET", "/rmw", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if !called {
		t.Error("route middleware was not invoked")
	}
}

func TestHandler_MultipleRouteMiddleware(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	var order []int
	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, 1)
			next.ServeHTTP(w, r)
		})
	}
	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, 2)
			next.ServeHTTP(w, r)
		})
	}

	svc.GET("/multi-mw", func(w http.ResponseWriter, r *http.Request) {
		order = append(order, 0)
	}, mw1, mw2)

	req := httptest.NewRequest("GET", "/multi-mw", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	// Route middleware applied in reverse: mw1 wraps mw2 wraps handler.
	// Execution: mw1 -> mw2 -> handler
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 0 {
		t.Errorf("unexpected middleware order: %v, want [1,2,0]", order)
	}
}

// ---------------------------------------------------------------------------
// 9. Builtin route responses
// ---------------------------------------------------------------------------

func TestBuiltinRoutes(t *testing.T) {
	t.Parallel()

	svc := newStartedService(t)
	handler := svc.Handler()

	t.Run("/health returns healthy JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("content-type = %s, want application/json", ct)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if body["status"] != "healthy" {
			t.Errorf("status = %v, want healthy", body["status"])
		}
		if _, ok := body["timestamp"]; !ok {
			t.Error("missing timestamp field")
		}
	})

	t.Run("/ready returns ready JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/ready", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if body["ready"] != true {
			t.Errorf("ready = %v, want true", body["ready"])
		}
	})

	t.Run("/routes returns list of routes", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/routes", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var routes []map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&routes); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if len(routes) < 3 {
			t.Errorf("expected at least 3 builtin routes, got %d", len(routes))
		}
	})
}

// ---------------------------------------------------------------------------
// 10. WebSocket handler registration
// ---------------------------------------------------------------------------

func TestWebSocketRegistration(t *testing.T) {
	t.Parallel()

	t.Run("registers handler", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		wsHandler := &WebSocketHandler{
			OnConnect: func(conn *Connection) {},
			OnMessage: func(conn *Connection, msg []byte) {},
			OnClose:   func(conn *Connection) {},
		}
		if err := svc.WebSocket("/ws", wsHandler); err != nil {
			t.Fatalf("WebSocket() error: %v", err)
		}

		svc.mu.RLock()
		stored, ok := svc.websockets["/ws"]
		svc.mu.RUnlock()
		if !ok {
			t.Fatal("websocket handler not found")
		}
		if stored.Path != "/ws" {
			t.Errorf("path = %s, want /ws", stored.Path)
		}
	})

	t.Run("overwrites same path", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		ws1 := &WebSocketHandler{}
		ws2 := &WebSocketHandler{}
		svc.WebSocket("/ws", ws1)
		svc.WebSocket("/ws", ws2)

		svc.mu.RLock()
		stored := svc.websockets["/ws"]
		svc.mu.RUnlock()
		if stored != ws2 {
			t.Error("second WebSocket handler should overwrite the first")
		}
	})

	t.Run("multiple paths", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		svc.WebSocket("/ws1", &WebSocketHandler{})
		svc.WebSocket("/ws2", &WebSocketHandler{})

		svc.mu.RLock()
		n := len(svc.websockets)
		svc.mu.RUnlock()
		if n != 2 {
			t.Errorf("expected 2 ws handlers, got %d", n)
		}
	})
}

// ---------------------------------------------------------------------------
// 11. gRPC service registration
// ---------------------------------------------------------------------------

func TestRegisterGRPC(t *testing.T) {
	t.Parallel()

	t.Run("registers service", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		grpcSvc := &GRPCService{
			Name:    "greeter",
			Package: "pb.greeter",
			Methods: []string{"SayHello", "SayBye"},
		}
		if err := svc.RegisterGRPC(grpcSvc); err != nil {
			t.Fatalf("RegisterGRPC error: %v", err)
		}

		svc.mu.RLock()
		stored, ok := svc.grpcServices["greeter"]
		svc.mu.RUnlock()
		if !ok {
			t.Fatal("gRPC service not found")
		}
		if len(stored.Methods) != 2 {
			t.Errorf("expected 2 methods, got %d", len(stored.Methods))
		}
	})

	t.Run("with interceptors", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		interceptor := func(ctx context.Context, req interface{}, info *GRPCMethodInfo, handler GRPCHandler) (interface{}, error) {
			return handler(ctx, req)
		}
		grpcSvc := &GRPCService{
			Name:         "auth",
			Methods:      []string{"Login"},
			Interceptors: []GRPCInterceptor{interceptor},
		}
		if err := svc.RegisterGRPC(grpcSvc); err != nil {
			t.Fatal(err)
		}

		svc.mu.RLock()
		stored := svc.grpcServices["auth"]
		svc.mu.RUnlock()
		if len(stored.Interceptors) != 1 {
			t.Errorf("expected 1 interceptor, got %d", len(stored.Interceptors))
		}
	})
}

// ---------------------------------------------------------------------------
// 12. Connection management
// ---------------------------------------------------------------------------

func TestConnectionManagement(t *testing.T) {
	t.Parallel()

	t.Run("register and get", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		conn := &Connection{ID: "conn-1", RemoteAddr: "127.0.0.1:1234", CreatedAt: time.Now()}
		svc.RegisterConnection(conn)

		got, ok := svc.GetConnection("conn-1")
		if !ok {
			t.Fatal("connection not found")
		}
		if got.ID != "conn-1" {
			t.Errorf("ID = %s, want conn-1", got.ID)
		}
		if got.RemoteAddr != "127.0.0.1:1234" {
			t.Errorf("RemoteAddr = %s, want 127.0.0.1:1234", got.RemoteAddr)
		}
	})

	t.Run("unregister", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		svc.RegisterConnection(&Connection{ID: "c1", CreatedAt: time.Now()})
		svc.UnregisterConnection("c1")
		_, ok := svc.GetConnection("c1")
		if ok {
			t.Error("connection should be removed")
		}
	})

	t.Run("unregister nonexistent is no-op", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		svc.UnregisterConnection("no-such-conn") // must not panic
	})

	t.Run("get missing connection returns false", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		_, ok := svc.GetConnection("missing")
		if ok {
			t.Error("expected false for missing connection")
		}
	})
}

// ---------------------------------------------------------------------------
// 13. Connection metadata
// ---------------------------------------------------------------------------

func TestConnectionMetadata(t *testing.T) {
	t.Parallel()

	t.Run("set and get", func(t *testing.T) {
		t.Parallel()
		conn := &Connection{ID: "m1", CreatedAt: time.Now()}
		conn.SetMetadata("user", "alice")
		val, ok := conn.GetMetadata("user")
		if !ok || val != "alice" {
			t.Errorf("metadata user = %v (ok=%v), want alice", val, ok)
		}
	})

	t.Run("get missing key returns false", func(t *testing.T) {
		t.Parallel()
		conn := &Connection{ID: "m2", CreatedAt: time.Now()}
		_, ok := conn.GetMetadata("nope")
		if ok {
			t.Error("expected false for missing key")
		}
	})

	t.Run("get from nil metadata returns false", func(t *testing.T) {
		t.Parallel()
		conn := &Connection{ID: "m3"}
		_, ok := conn.GetMetadata("anything")
		if ok {
			t.Error("expected false from nil metadata")
		}
	})

	t.Run("overwrite metadata key", func(t *testing.T) {
		t.Parallel()
		conn := &Connection{ID: "m4", CreatedAt: time.Now()}
		conn.SetMetadata("k", 1)
		conn.SetMetadata("k", 2)
		val, ok := conn.GetMetadata("k")
		if !ok || val != 2 {
			t.Errorf("expected 2, got %v", val)
		}
	})

	t.Run("multiple keys", func(t *testing.T) {
		t.Parallel()
		conn := &Connection{ID: "m5", CreatedAt: time.Now()}
		conn.SetMetadata("a", "x")
		conn.SetMetadata("b", 42)
		conn.SetMetadata("c", true)

		a, _ := conn.GetMetadata("a")
		b, _ := conn.GetMetadata("b")
		c, _ := conn.GetMetadata("c")
		if a != "x" || b != 42 || c != true {
			t.Errorf("metadata mismatch: a=%v b=%v c=%v", a, b, c)
		}
	})
}

// ---------------------------------------------------------------------------
// 14. BroadcastMessage
// ---------------------------------------------------------------------------

func TestBroadcastMessage(t *testing.T) {
	t.Parallel()

	t.Run("no connections is a no-op", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		if err := svc.BroadcastMessage([]byte("hello")); err != nil {
			t.Errorf("BroadcastMessage error: %v", err)
		}
	})

	t.Run("with connections does not error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		svc.RegisterConnection(&Connection{ID: "b1", CreatedAt: time.Now()})
		svc.RegisterConnection(&Connection{ID: "b2", CreatedAt: time.Now()})
		if err := svc.BroadcastMessage([]byte("msg")); err != nil {
			t.Errorf("BroadcastMessage error: %v", err)
		}
	})

	t.Run("empty payload", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t)
		if err := svc.BroadcastMessage(nil); err != nil {
			t.Errorf("BroadcastMessage with nil should not error: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// 15. Stats
// ---------------------------------------------------------------------------

func TestStats(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	svc.GET("/a", dummyHandler)
	svc.POST("/b", dummyHandler)
	svc.Group("/api")
	svc.WebSocket("/ws", &WebSocketHandler{})
	svc.RegisterGRPC(&GRPCService{Name: "svc1", Methods: []string{"M"}})
	svc.RegisterConnection(&Connection{ID: "c1", CreatedAt: time.Now()})
	svc.Use(func(next http.Handler) http.Handler { return next })

	stats := svc.Stats()
	checks := map[string]int{
		"total_routes":       2,
		"total_groups":       1,
		"websocket_handlers": 1,
		"grpc_services":      1,
		"active_connections": 1,
		"global_middleware":  1,
	}
	for k, want := range checks {
		got, ok := stats[k]
		if !ok {
			t.Errorf("stats missing key %s", k)
			continue
		}
		if got != want {
			t.Errorf("stats[%s] = %v, want %d", k, got, want)
		}
	}
}

func TestStats_EmptyService(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	stats := svc.Stats()
	for k, v := range stats {
		if v != 0 {
			t.Errorf("stats[%s] = %v on empty service, want 0", k, v)
		}
	}
}

// ---------------------------------------------------------------------------
// 16. ResponseWriter
// ---------------------------------------------------------------------------

func TestResponseWriter_WriteHeader(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	rw := &ResponseWriter{ResponseWriter: rec, StatusCode: http.StatusOK}

	rw.WriteHeader(http.StatusCreated)
	if rw.StatusCode != http.StatusCreated {
		t.Errorf("StatusCode = %d, want %d", rw.StatusCode, http.StatusCreated)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("underlying recorder code = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestResponseWriter_Write(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	rw := &ResponseWriter{ResponseWriter: rec, StatusCode: http.StatusOK}

	data := []byte("hello world")
	n, err := rw.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("wrote %d bytes, want %d", n, len(data))
	}
	if rw.BytesWritten != int64(len(data)) {
		t.Errorf("BytesWritten = %d, want %d", rw.BytesWritten, len(data))
	}

	// Write more
	n2, _ := rw.Write([]byte("!"))
	if rw.BytesWritten != int64(len(data)+n2) {
		t.Errorf("BytesWritten after second write = %d, want %d", rw.BytesWritten, len(data)+n2)
	}
}

func TestResponseWriter_DefaultStatus(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	rw := &ResponseWriter{ResponseWriter: rec, StatusCode: http.StatusOK}
	// If WriteHeader is never called, StatusCode stays at initialised value
	if rw.StatusCode != http.StatusOK {
		t.Errorf("default StatusCode = %d, want 200", rw.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// 17. LoggingMiddleware
// ---------------------------------------------------------------------------

func TestLoggingMiddleware(t *testing.T) {
	t.Parallel()
	log := newTestLogger()

	handler := LoggingMiddleware(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("logged"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rec.Code)
	}
	if rec.Body.String() != "logged" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "logged")
	}
}

func TestLoggingMiddleware_CapturesBytesAndStatus(t *testing.T) {
	t.Parallel()
	log := newTestLogger()

	// Use the middleware and verify the ResponseWriter wrapper captures data
	var capturedRW *ResponseWriter
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The w passed to the inner handler is our ResponseWriter
		if rw, ok := w.(*ResponseWriter); ok {
			capturedRW = rw
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	})

	handler := LoggingMiddleware(log)(inner)
	req := httptest.NewRequest("GET", "/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedRW == nil {
		t.Fatal("ResponseWriter wrapper not passed to inner handler")
	}
	if capturedRW.StatusCode != http.StatusNotFound {
		t.Errorf("captured status = %d, want 404", capturedRW.StatusCode)
	}
	if capturedRW.BytesWritten != int64(len("not found")) {
		t.Errorf("captured bytes = %d, want %d", capturedRW.BytesWritten, len("not found"))
	}
}

// ---------------------------------------------------------------------------
// 18. RecoveryMiddleware
// ---------------------------------------------------------------------------

func TestRecoveryMiddleware_NoPanic(t *testing.T) {
	t.Parallel()
	log := newTestLogger()

	handler := RecoveryMiddleware(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "fine")
	}))

	req := httptest.NewRequest("GET", "/ok", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestRecoveryMiddleware_RecoversPanic(t *testing.T) {
	t.Parallel()
	log := newTestLogger()

	handler := RecoveryMiddleware(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	}))

	req := httptest.NewRequest("GET", "/panic", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if body := rec.Body.String(); body == "" {
		t.Error("expected error body after panic recovery")
	}
}

func TestRecoveryMiddleware_RecoversNonStringPanic(t *testing.T) {
	t.Parallel()
	log := newTestLogger()

	handler := RecoveryMiddleware(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(42) // non-string panic value
	}))

	req := httptest.NewRequest("GET", "/int-panic", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// 19. CORSMiddleware
// ---------------------------------------------------------------------------

func TestCORSMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		origins        []string
		requestOrigin  string
		method         string
		wantACAO       string // Access-Control-Allow-Origin
		wantStatus     int
		expectBody     bool
	}{
		{
			name:          "allowed origin",
			origins:       []string{"https://example.com"},
			requestOrigin: "https://example.com",
			method:        "GET",
			wantACAO:      "https://example.com",
			wantStatus:    http.StatusOK,
			expectBody:    true,
		},
		{
			name:          "wildcard allows any",
			origins:       []string{"*"},
			requestOrigin: "https://anything.io",
			method:        "GET",
			wantACAO:      "https://anything.io",
			wantStatus:    http.StatusOK,
			expectBody:    true,
		},
		{
			name:          "disallowed origin",
			origins:       []string{"https://safe.com"},
			requestOrigin: "https://evil.com",
			method:        "GET",
			wantACAO:      "",
			wantStatus:    http.StatusOK,
			expectBody:    true,
		},
		{
			name:          "preflight OPTIONS allowed",
			origins:       []string{"https://app.com"},
			requestOrigin: "https://app.com",
			method:        "OPTIONS",
			wantACAO:      "https://app.com",
			wantStatus:    http.StatusNoContent,
			expectBody:    false,
		},
		{
			name:          "preflight OPTIONS disallowed",
			origins:       []string{"https://safe.com"},
			requestOrigin: "https://evil.com",
			method:        "OPTIONS",
			wantACAO:      "",
			wantStatus:    http.StatusNoContent,
			expectBody:    false,
		},
		{
			name:          "no origin header",
			origins:       []string{"*"},
			requestOrigin: "",
			method:        "GET",
			wantACAO:      "",
			wantStatus:    http.StatusOK,
			expectBody:    true,
		},
		{
			name:          "empty origins list",
			origins:       []string{},
			requestOrigin: "https://test.com",
			method:        "GET",
			wantACAO:      "",
			wantStatus:    http.StatusOK,
			expectBody:    true,
		},
		{
			name:          "multiple allowed origins - match second",
			origins:       []string{"https://a.com", "https://b.com"},
			requestOrigin: "https://b.com",
			method:        "GET",
			wantACAO:      "https://b.com",
			wantStatus:    http.StatusOK,
			expectBody:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, "body")
			})
			handler := CORSMiddleware(tc.origins)(inner)

			req := httptest.NewRequest(tc.method, "/cors", nil)
			if tc.requestOrigin != "" {
				req.Header.Set("Origin", tc.requestOrigin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}

			acao := rec.Header().Get("Access-Control-Allow-Origin")
			if acao != tc.wantACAO {
				t.Errorf("ACAO = %q, want %q", acao, tc.wantACAO)
			}

			if tc.wantACAO != "" {
				acam := rec.Header().Get("Access-Control-Allow-Methods")
				if acam == "" {
					t.Error("expected Access-Control-Allow-Methods header")
				}
				acah := rec.Header().Get("Access-Control-Allow-Headers")
				if acah == "" {
					t.Error("expected Access-Control-Allow-Headers header")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 20. Middleware composition (stacking multiple builtin middleware)
// ---------------------------------------------------------------------------

func TestMiddlewareComposition(t *testing.T) {
	t.Parallel()
	log := newTestLogger()

	svc := newTestService(t)
	svc.Use(RecoveryMiddleware(log))
	svc.Use(LoggingMiddleware(log))
	svc.Use(CORSMiddleware([]string{"*"}))

	svc.GET("/compose", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "composed")
	})

	req := httptest.NewRequest("GET", "/compose", nil)
	req.Header.Set("Origin", "https://test.com")
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "composed" {
		t.Errorf("body = %q, want composed", rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://test.com" {
		t.Error("CORS header missing in composed middleware")
	}
}

func TestMiddlewareComposition_RecoveryProtectsChain(t *testing.T) {
	t.Parallel()
	log := newTestLogger()

	svc := newTestService(t)
	svc.Use(RecoveryMiddleware(log))
	svc.Use(LoggingMiddleware(log))

	svc.GET("/panic-compose", func(w http.ResponseWriter, r *http.Request) {
		panic("boom in composed handler")
	})

	req := httptest.NewRequest("GET", "/panic-compose", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (panic recovered)", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// 21. RequestContext (struct coverage)
// ---------------------------------------------------------------------------

func TestRequestContext(t *testing.T) {
	t.Parallel()
	now := time.Now()
	rc := RequestContext{
		RequestID:   "req-123",
		UserID:      "usr-456",
		StartTime:   now,
		Path:        "/api/test",
		Method:      "POST",
		Headers:     map[string]string{"Authorization": "Bearer tok"},
		QueryParams: map[string]string{"page": "1"},
	}

	if rc.RequestID != "req-123" {
		t.Error("RequestID mismatch")
	}
	if rc.UserID != "usr-456" {
		t.Error("UserID mismatch")
	}
	if rc.Path != "/api/test" {
		t.Error("Path mismatch")
	}
	if rc.Method != "POST" {
		t.Error("Method mismatch")
	}
	if rc.Headers["Authorization"] != "Bearer tok" {
		t.Error("Headers mismatch")
	}
	if rc.QueryParams["page"] != "1" {
		t.Error("QueryParams mismatch")
	}
}

func TestRequestContextJSON(t *testing.T) {
	t.Parallel()
	rc := RequestContext{
		RequestID: "r1",
		Path:      "/test",
		Method:    "GET",
	}
	data, err := json.Marshal(rc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded RequestContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.RequestID != "r1" {
		t.Error("round-trip RequestID mismatch")
	}
}

// ---------------------------------------------------------------------------
// 22. GRPCInterceptor chain execution
// ---------------------------------------------------------------------------

func TestGRPCInterceptorExecution(t *testing.T) {
	t.Parallel()

	var order []string
	interceptor1 := func(ctx context.Context, req interface{}, info *GRPCMethodInfo, handler GRPCHandler) (interface{}, error) {
		order = append(order, "i1-before")
		resp, err := handler(ctx, req)
		order = append(order, "i1-after")
		return resp, err
	}

	finalHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		order = append(order, "handler")
		return "result", nil
	}

	info := &GRPCMethodInfo{FullMethod: "/test/Method", IsStream: false}
	resp, err := interceptor1(context.Background(), "input", info, finalHandler)
	if err != nil {
		t.Fatal(err)
	}
	if resp != "result" {
		t.Errorf("resp = %v, want result", resp)
	}
	expected := []string{"i1-before", "handler", "i1-after"}
	if len(order) != len(expected) {
		t.Fatalf("execution order mismatch: %v", order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Errorf("order[%d] = %s, want %s", i, order[i], expected[i])
		}
	}
}

// ---------------------------------------------------------------------------
// 23. Config JSON marshaling
// ---------------------------------------------------------------------------

func TestConfigJSON(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.HTTPAddress != cfg.HTTPAddress {
		t.Errorf("HTTPAddress = %s, want %s", decoded.HTTPAddress, cfg.HTTPAddress)
	}
	if decoded.WotanTopic != cfg.WotanTopic {
		t.Errorf("WotanTopic = %s, want %s", decoded.WotanTopic, cfg.WotanTopic)
	}
}

// ---------------------------------------------------------------------------
// 24. Route JSON marshaling (Handler is excluded)
// ---------------------------------------------------------------------------

func TestRouteJSON(t *testing.T) {
	t.Parallel()
	r := &Route{
		Path:        "/api/v1/data",
		Method:      "GET",
		Handler:     dummyHandler,
		Description: "Get data",
		Tags:        []string{"api", "v1"},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["path"] != "/api/v1/data" {
		t.Errorf("path = %v", decoded["path"])
	}
	// Handler is tagged json:"-" so should not appear
	if _, ok := decoded["handler"]; ok {
		t.Error("handler should not appear in JSON")
	}
}

// ---------------------------------------------------------------------------
// 25. Connection JSON marshaling
// ---------------------------------------------------------------------------

func TestConnectionJSON(t *testing.T) {
	t.Parallel()
	c := &Connection{
		ID:         "conn-42",
		RemoteAddr: "10.0.0.1:5555",
		Headers:    map[string]string{"X-Custom": "val"},
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)
	if decoded["id"] != "conn-42" {
		t.Error("id mismatch")
	}
	// metadata is unexported, should not appear
	if _, ok := decoded["metadata"]; ok {
		t.Error("metadata is unexported and should not be in JSON")
	}
}

// ---------------------------------------------------------------------------
// 26. WebSocketHandler JSON marshaling
// ---------------------------------------------------------------------------

func TestWebSocketHandlerJSON(t *testing.T) {
	t.Parallel()
	ws := &WebSocketHandler{
		Path:      "/ws/events",
		OnConnect: func(conn *Connection) {},
	}
	data, err := json.Marshal(ws)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)
	if decoded["path"] != "/ws/events" {
		t.Error("path mismatch")
	}
	// Function fields tagged json:"-"
	if _, ok := decoded["on_connect"]; ok {
		t.Error("OnConnect should not appear in JSON")
	}
}

// ---------------------------------------------------------------------------
// 27. GRPCService JSON marshaling
// ---------------------------------------------------------------------------

func TestGRPCServiceJSON(t *testing.T) {
	t.Parallel()
	svc := &GRPCService{
		Name:    "orders",
		Package: "pb.orders",
		Methods: []string{"Create", "List"},
	}
	data, err := json.Marshal(svc)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)
	if decoded["name"] != "orders" {
		t.Error("name mismatch")
	}
	if _, ok := decoded["interceptors"]; ok {
		t.Error("interceptors should not appear in JSON")
	}
}

// ---------------------------------------------------------------------------
// 28. Concurrent safety (race detector validation)
// ---------------------------------------------------------------------------

func TestConcurrentRouteRegistration(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := fmt.Sprintf("/concurrent/%d", i)
			svc.GET(path, dummyHandler)
		}(i)
	}
	wg.Wait()

	routes := svc.ListRoutes()
	if len(routes) != 50 {
		t.Errorf("expected 50 routes, got %d", len(routes))
	}
}

func TestConcurrentMiddlewareAdd(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Use(func(next http.Handler) http.Handler { return next })
		}()
	}
	wg.Wait()

	svc.mu.RLock()
	n := len(svc.middleware)
	svc.mu.RUnlock()
	if n != 20 {
		t.Errorf("expected 20 middleware, got %d", n)
	}
}

func TestConcurrentConnectionOperations(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	var wg sync.WaitGroup

	// Concurrent register
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc.RegisterConnection(&Connection{
				ID:        fmt.Sprintf("conn-%d", i),
				CreatedAt: time.Now(),
			})
		}(i)
	}
	wg.Wait()

	// Concurrent get + unregister
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			svc.GetConnection(fmt.Sprintf("conn-%d", i))
		}(i)
		go func(i int) {
			defer wg.Done()
			svc.UnregisterConnection(fmt.Sprintf("conn-%d", i))
		}(i)
	}
	wg.Wait()
}

func TestConcurrentMetadataAccess(t *testing.T) {
	t.Parallel()
	conn := &Connection{ID: "race-test", CreatedAt: time.Now()}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			conn.SetMetadata(fmt.Sprintf("key-%d", i), i)
		}(i)
		go func(i int) {
			defer wg.Done()
			conn.GetMetadata(fmt.Sprintf("key-%d", i))
		}(i)
	}
	wg.Wait()
}

func TestConcurrentHandler(t *testing.T) {
	t.Parallel()
	svc := newStartedService(t)
	svc.GET("/concurrent-handler", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	handler := svc.Handler()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/concurrent-handler", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("unexpected status: %d", rec.Code)
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentStats(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	var wg sync.WaitGroup

	// Mix reads (Stats) with writes (route/connection registration)
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			svc.GET(fmt.Sprintf("/stat/%d", i), dummyHandler)
		}(i)
		go func(i int) {
			defer wg.Done()
			svc.RegisterConnection(&Connection{ID: fmt.Sprintf("s-%d", i), CreatedAt: time.Now()})
		}(i)
		go func() {
			defer wg.Done()
			svc.Stats()
		}()
	}
	wg.Wait()
}

func TestConcurrentBroadcast(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	for i := 0; i < 10; i++ {
		svc.RegisterConnection(&Connection{ID: fmt.Sprintf("bc-%d", i), CreatedAt: time.Now()})
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc.BroadcastMessage([]byte(fmt.Sprintf("msg-%d", i)))
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// 29. Edge cases
// ---------------------------------------------------------------------------

func TestHandler_MethodMismatch(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	svc.GET("/only-get", dummyHandler)

	// POST to a GET-only route
	req := httptest.NewRequest("POST", "/only-get", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for method mismatch, got %d", rec.Code)
	}
}

func TestHandler_EmptyPath(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	// Register a route with an empty path
	svc.GET("", dummyHandler)

	// Use "/" as request URL since httptest.NewRequest requires a non-empty URL.
	// This tests that the route key "GET:" (empty path) does not match "/".
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)
	// "/" does not equal the empty-string path, so expect 404
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for / when route registered with empty path, got %d", rec.Code)
	}
}

func TestGroupWithNoMiddleware(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	group := svc.Group("/empty")
	if group == nil {
		t.Fatal("group should not be nil")
	}
	if len(group.Middleware) != 0 {
		t.Errorf("expected 0 middleware, got %d", len(group.Middleware))
	}
	if len(group.Routes) != 0 {
		t.Errorf("expected 0 routes, got %d", len(group.Routes))
	}
}

func TestConnectionWithHeaders(t *testing.T) {
	t.Parallel()
	conn := &Connection{
		ID:      "headers-test",
		Headers: map[string]string{"Upgrade": "websocket", "Connection": "Upgrade"},
	}
	if conn.Headers["Upgrade"] != "websocket" {
		t.Error("Upgrade header mismatch")
	}
}

func TestRouteWithTags(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	svc.mu.Lock()
	svc.routes["GET:/tagged"] = &Route{
		Path:        "/tagged",
		Method:      "GET",
		Handler:     dummyHandler,
		Description: "A tagged route",
		Tags:        []string{"admin", "internal"},
	}
	svc.mu.Unlock()

	r, ok := svc.GetRoute("GET", "/tagged")
	if !ok {
		t.Fatal("route not found")
	}
	if r.Description != "A tagged route" {
		t.Error("description mismatch")
	}
	if len(r.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(r.Tags))
	}
}

func TestGRPCMethodInfo(t *testing.T) {
	t.Parallel()
	info := &GRPCMethodInfo{FullMethod: "/pkg.Svc/Method", IsStream: true}
	if info.FullMethod != "/pkg.Svc/Method" {
		t.Error("FullMethod mismatch")
	}
	if !info.IsStream {
		t.Error("IsStream should be true")
	}
}

// ---------------------------------------------------------------------------
// 30. Integration-style: full request lifecycle
// ---------------------------------------------------------------------------

func TestFullRequestLifecycle(t *testing.T) {
	t.Parallel()
	log := newTestLogger()
	svc := NewService(log, newTestWotan(), nil)

	// Add middleware
	svc.Use(RecoveryMiddleware(log))
	svc.Use(CORSMiddleware([]string{"https://app.example.com"}))

	// Register routes
	svc.GET("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": "value"})
	})

	// Register WebSocket
	svc.WebSocket("/ws/live", &WebSocketHandler{
		OnConnect: func(conn *Connection) { conn.SetMetadata("status", "connected") },
	})

	// Register gRPC
	svc.RegisterGRPC(&GRPCService{Name: "data", Methods: []string{"Get", "Set"}})

	// Register connections
	svc.RegisterConnection(&Connection{ID: "live-1", CreatedAt: time.Now()})

	// Start
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	handler := svc.Handler()

	// Test custom route with CORS
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("data endpoint status = %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Error("CORS header missing")
	}

	var respBody map[string]string
	json.NewDecoder(rec.Body).Decode(&respBody)
	if respBody["key"] != "value" {
		t.Error("response body mismatch")
	}

	// Verify stats
	stats := svc.Stats()
	if stats["active_connections"] != 1 {
		t.Errorf("active_connections = %v, want 1", stats["active_connections"])
	}

	// Stop
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
	stats = svc.Stats()
	if stats["active_connections"] != 0 {
		t.Errorf("connections after stop = %v, want 0", stats["active_connections"])
	}
}

// ---------------------------------------------------------------------------
// 31. httptest.Server integration
// ---------------------------------------------------------------------------

func TestWithHTTPTestServer(t *testing.T) {
	t.Parallel()
	svc := newStartedService(t)
	svc.GET("/srv/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "pong")
	})

	ts := httptest.NewServer(svc.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/srv/ping")
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("body = %q, want pong", body)
	}
}

func TestWithHTTPTestServer_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	ts := httptest.NewServer(svc.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestWithHTTPTestServer_BuiltinHealth(t *testing.T) {
	t.Parallel()
	svc := newStartedService(t)

	ts := httptest.NewServer(svc.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "healthy" {
		t.Error("health check failed")
	}
}

// ---------------------------------------------------------------------------
// 32. WebSocket upgrade simulation via HTTP
// ---------------------------------------------------------------------------

func TestWebSocketUpgradeHeaderDetection(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// Register a handler that checks for WebSocket upgrade headers
	svc.GET("/ws-check", func(w http.ResponseWriter, r *http.Request) {
		upgrade := r.Header.Get("Upgrade")
		connection := r.Header.Get("Connection")
		if upgrade == "websocket" && connection == "Upgrade" {
			w.WriteHeader(http.StatusSwitchingProtocols)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "not a websocket request")
	})

	handler := svc.Handler()

	// Without upgrade headers
	req := httptest.NewRequest("GET", "/ws-check", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-ws request: status = %d, want 400", rec.Code)
	}

	// With upgrade headers
	req = httptest.NewRequest("GET", "/ws-check", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSwitchingProtocols {
		t.Errorf("ws request: status = %d, want 101", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// 33. Static asset serving simulation
// ---------------------------------------------------------------------------

func TestStaticAssetServing(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// Simulate a static asset handler
	svc.GET("/static/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		fmt.Fprint(w, "body { margin: 0; }")
	})
	svc.GET("/static/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, "console.log('hello');")
	})

	handler := svc.Handler()

	tests := []struct {
		path        string
		contentType string
		body        string
	}{
		{"/static/style.css", "text/css", "body { margin: 0; }"},
		{"/static/app.js", "application/javascript", "console.log('hello');"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("GET", tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != tc.contentType {
				t.Errorf("content-type = %s, want %s", ct, tc.contentType)
			}
			if rec.Body.String() != tc.body {
				t.Errorf("body mismatch")
			}
		})
	}
}

func TestStaticAssetCacheHeaders(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	svc.GET("/static/img.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/static/img.png", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=86400" {
		t.Errorf("Cache-Control = %s", cc)
	}
	if etag := rec.Header().Get("ETag"); etag != `"abc123"` {
		t.Errorf("ETag = %s", etag)
	}
}

// ---------------------------------------------------------------------------
// 34. RouteGroup routes slice
// ---------------------------------------------------------------------------

func TestRouteGroupRoutesSlice(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	group := svc.Group("/v2")

	// Manually add routes to the group (the group Routes slice is public)
	group.Routes = append(group.Routes, &Route{
		Path:   "/v2/users",
		Method: "GET",
	})
	group.Routes = append(group.Routes, &Route{
		Path:   "/v2/users",
		Method: "POST",
	})

	if len(group.Routes) != 2 {
		t.Errorf("expected 2 group routes, got %d", len(group.Routes))
	}
}

// ---------------------------------------------------------------------------
// 35. GRPCService with empty methods
// ---------------------------------------------------------------------------

func TestGRPCServiceEmptyMethods(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	grpcSvc := &GRPCService{Name: "empty", Methods: []string{}}
	if err := svc.RegisterGRPC(grpcSvc); err != nil {
		t.Fatal(err)
	}
	svc.mu.RLock()
	stored := svc.grpcServices["empty"]
	svc.mu.RUnlock()
	if len(stored.Methods) != 0 {
		t.Error("expected empty methods")
	}
}

// ---------------------------------------------------------------------------
// 36. Concurrent WebSocket and gRPC registration
// ---------------------------------------------------------------------------

func TestConcurrentWebSocketGRPCRegistration(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	var wg sync.WaitGroup

	for i := 0; i < 25; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			svc.WebSocket(fmt.Sprintf("/ws/%d", i), &WebSocketHandler{})
		}(i)
		go func(i int) {
			defer wg.Done()
			svc.RegisterGRPC(&GRPCService{
				Name:    fmt.Sprintf("svc-%d", i),
				Methods: []string{"Do"},
			})
		}(i)
	}
	wg.Wait()

	svc.mu.RLock()
	wsCount := len(svc.websockets)
	grpcCount := len(svc.grpcServices)
	svc.mu.RUnlock()

	if wsCount != 25 {
		t.Errorf("expected 25 ws handlers, got %d", wsCount)
	}
	if grpcCount != 25 {
		t.Errorf("expected 25 grpc services, got %d", grpcCount)
	}
}

// ---------------------------------------------------------------------------
// 37. Concurrent group creation
// ---------------------------------------------------------------------------

func TestConcurrentGroupCreation(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	var wg sync.WaitGroup

	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc.Group(fmt.Sprintf("/group/%d", i))
		}(i)
	}
	wg.Wait()

	svc.mu.RLock()
	n := len(svc.groups)
	svc.mu.RUnlock()
	if n != 30 {
		t.Errorf("expected 30 groups, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 38. Handler with no global middleware
// ---------------------------------------------------------------------------

func TestHandler_NoGlobalMiddleware(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	called := false
	svc.GET("/no-global-mw", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/no-global-mw", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// 39. Handler with only global middleware, no route middleware
// ---------------------------------------------------------------------------

func TestHandler_GlobalMiddlewareOnly(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	headerSet := false
	svc.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Global", "true")
			headerSet = true
			next.ServeHTTP(w, r)
		})
	})

	svc.GET("/global-only", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/global-only", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if !headerSet {
		t.Error("global middleware was not invoked")
	}
	if rec.Header().Get("X-Global") != "true" {
		t.Error("X-Global header not set by middleware")
	}
}

// ---------------------------------------------------------------------------
// 40. Middleware that short-circuits
// ---------------------------------------------------------------------------

func TestMiddleware_ShortCircuit(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	handlerCalled := false
	svc.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Short-circuit: don't call next
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "blocked")
		})
	})

	svc.GET("/blocked", func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	req := httptest.NewRequest("GET", "/blocked", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if handlerCalled {
		t.Error("handler should not be called when middleware short-circuits")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// 41. Middleware that modifies the request
// ---------------------------------------------------------------------------

func TestMiddleware_ModifiesRequest(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	svc.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Injected", "yes")
			next.ServeHTTP(w, r)
		})
	})

	var gotHeader string
	svc.GET("/inject", func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Injected")
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/inject", nil)
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, req)

	if gotHeader != "yes" {
		t.Errorf("injected header = %q, want yes", gotHeader)
	}
}

// ---------------------------------------------------------------------------
// 42. Start is idempotent (calling Start multiple times)
// ---------------------------------------------------------------------------

func TestStartIdempotent(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	ctx := context.Background()

	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	// Second start overwrites builtin routes but should not fail
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Builtins still work
	_, ok := svc.GetRoute("GET", "/health")
	if !ok {
		t.Error("/health route missing after double Start")
	}
}

// ---------------------------------------------------------------------------
// 43. Benchmark: Handler dispatch
// ---------------------------------------------------------------------------

func BenchmarkHandlerDispatch(b *testing.B) {
	svc := NewService(logger.New(io.Discard), newTestWotan(), nil)
	svc.GET("/bench", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := svc.Handler()
	req := httptest.NewRequest("GET", "/bench", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkHandlerDispatchWithMiddleware(b *testing.B) {
	log := logger.New(io.Discard)
	svc := NewService(log, newTestWotan(), nil)
	svc.Use(RecoveryMiddleware(log))
	svc.Use(LoggingMiddleware(log))
	svc.Use(CORSMiddleware([]string{"*"}))
	svc.GET("/bench-mw", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := svc.Handler()
	req := httptest.NewRequest("GET", "/bench-mw", nil)
	req.Header.Set("Origin", "https://test.com")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkRouteRegistration(b *testing.B) {
	svc := NewService(logger.New(io.Discard), newTestWotan(), nil)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		svc.GET(fmt.Sprintf("/bench/%d", i), dummyHandler)
	}
}
