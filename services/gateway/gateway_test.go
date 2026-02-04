package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/logger"
	"unheaded/services/gateway/config"
	"unheaded/services/gateway/middleware"
	"unheaded/services/gateway/proxy"
	"unheaded/services/gateway/routes"
)

// mockLogger implements a simple logger for testing.
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard).SetLevel(logger.DebugLevel)
}

func TestTokenBucket(t *testing.T) {
	t.Run("allows requests within rate", func(t *testing.T) {
		bucket := middleware.NewTokenBucket(10, 10)

		for i := 0; i < 10; i++ {
			if !bucket.Allow() {
				t.Errorf("Request %d should be allowed", i)
			}
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		bucket := middleware.NewTokenBucket(1, 1)

		if !bucket.Allow() {
			t.Error("First request should be allowed")
		}

		if bucket.Allow() {
			t.Error("Second request should be blocked")
		}
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		bucket := middleware.NewTokenBucket(100, 1)

		if !bucket.Allow() {
			t.Error("First request should be allowed")
		}

		time.Sleep(20 * time.Millisecond)

		if !bucket.Allow() {
			t.Error("Request after refill should be allowed")
		}
	})
}

func TestCircuitBreaker(t *testing.T) {
	log := newTestLogger()
	cfg := &config.CircuitConfig{
		Enabled:          true,
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		HalfOpenRequests: 2,
	}

	t.Run("starts closed", func(t *testing.T) {
		cb := proxy.NewCircuitBreaker(cfg, log)
		if cb.State() != proxy.CircuitClosed {
			t.Error("Circuit breaker should start closed")
		}
	})

	t.Run("opens after failures", func(t *testing.T) {
		cb := proxy.NewCircuitBreaker(cfg, log)

		for i := 0; i < 3; i++ {
			cb.Allow()
			cb.RecordFailure()
		}

		if cb.State() != proxy.CircuitOpen {
			t.Error("Circuit breaker should be open after threshold failures")
		}
	})

	t.Run("blocks requests when open", func(t *testing.T) {
		cb := proxy.NewCircuitBreaker(cfg, log)

		// Open the circuit
		for i := 0; i < 3; i++ {
			cb.Allow()
			cb.RecordFailure()
		}

		if cb.Allow() {
			t.Error("Circuit breaker should block requests when open")
		}
	})

	t.Run("transitions to half-open after timeout", func(t *testing.T) {
		cb := proxy.NewCircuitBreaker(cfg, log)

		// Open the circuit
		for i := 0; i < 3; i++ {
			cb.Allow()
			cb.RecordFailure()
		}

		// Wait for timeout
		time.Sleep(150 * time.Millisecond)

		// Should transition to half-open
		if !cb.Allow() {
			t.Error("Circuit breaker should allow request in half-open state")
		}

		if cb.State() != proxy.CircuitHalfOpen {
			t.Error("Circuit breaker should be in half-open state")
		}
	})

	t.Run("closes after successful requests in half-open", func(t *testing.T) {
		cb := proxy.NewCircuitBreaker(cfg, log)

		// Open the circuit
		for i := 0; i < 3; i++ {
			cb.Allow()
			cb.RecordFailure()
		}

		// Wait for timeout
		time.Sleep(150 * time.Millisecond)

		// Process successful requests in half-open
		for i := 0; i < 2; i++ {
			cb.Allow()
			cb.RecordSuccess()
		}

		if cb.State() != proxy.CircuitClosed {
			t.Errorf("Circuit breaker should be closed, got %v", cb.State())
		}
	})
}

func TestLoadBalancer(t *testing.T) {
	log := newTestLogger()
	backends := []string{"http://backend1:8080", "http://backend2:8080", "http://backend3:8080"}

	t.Run("round robin distributes requests", func(t *testing.T) {
		lb := proxy.NewRoundRobinLoadBalancer(backends, log)

		counts := make(map[string]int)
		for i := 0; i < 9; i++ {
			backend, err := lb.Next()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			counts[backend]++
		}

		for _, backend := range backends {
			if counts[backend] != 3 {
				t.Errorf("Expected 3 requests to %s, got %d", backend, counts[backend])
			}
		}
	})

	t.Run("marks backends unhealthy", func(t *testing.T) {
		lb := proxy.NewRoundRobinLoadBalancer(backends, log)

		lb.MarkUnhealthy("http://backend1:8080")

		healthy := lb.GetHealthy()
		if len(healthy) != 2 {
			t.Errorf("Expected 2 healthy backends, got %d", len(healthy))
		}

		for _, h := range healthy {
			if h == "http://backend1:8080" {
				t.Error("Unhealthy backend should not be in healthy list")
			}
		}
	})

	t.Run("returns error when no backends available", func(t *testing.T) {
		lb := proxy.NewRoundRobinLoadBalancer(backends, log)

		for _, b := range backends {
			lb.MarkUnhealthy(b)
		}

		_, err := lb.Next()
		if err != proxy.ErrNoAvailableBackends {
			t.Errorf("Expected ErrNoAvailableBackends, got %v", err)
		}
	})
}

func TestCORSMiddleware(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"http://example.com", "http://test.com"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	cors := middleware.NewCORSMiddleware(cfg)

	t.Run("adds CORS headers for allowed origin", func(t *testing.T) {
		handler := cors.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Header().Get("Access-Control-Allow-Origin") != "http://example.com" {
			t.Error("Should set Access-Control-Allow-Origin header")
		}
	})

	t.Run("handles preflight requests", func(t *testing.T) {
		handler := cors.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("Expected 204, got %d", rec.Code)
		}

		if rec.Header().Get("Access-Control-Allow-Methods") == "" {
			t.Error("Should set Access-Control-Allow-Methods header")
		}
	})

	t.Run("does not add headers for disallowed origin", func(t *testing.T) {
		handler := cors.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "http://malicious.com")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("Should not set CORS headers for disallowed origin")
		}
	})
}

func TestTracingMiddleware(t *testing.T) {
	tracing := middleware.NewTracingMiddleware("test-service")

	t.Run("generates trace ID if not present", func(t *testing.T) {
		handler := tracing.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := middleware.GetTraceID(r.Context())
			if traceID == "" {
				t.Error("Trace ID should be set in context")
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Header().Get("X-Trace-ID") == "" {
			t.Error("Should set X-Trace-ID response header")
		}
	})

	t.Run("propagates existing trace ID", func(t *testing.T) {
		existingTraceID := "abc123def456"

		handler := tracing.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := middleware.GetTraceID(r.Context())
			if traceID != existingTraceID {
				t.Errorf("Expected trace ID %s, got %s", existingTraceID, traceID)
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Trace-ID", existingTraceID)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Header().Get("X-Trace-ID") != existingTraceID {
			t.Error("Should propagate existing trace ID")
		}
	})
}

func TestRouter(t *testing.T) {
	log := newTestLogger()

	t.Run("matches routes by path prefix", func(t *testing.T) {
		router := routes.NewRouter(log, nil)

		cfg := &config.RouteConfig{
			Name:       "api",
			PathPrefix: "/api/",
			Backends:   []string{"http://localhost:8081"},
		}
		router.AddRoute(cfg, &config.CircuitConfig{Enabled: false})

		route := router.Match("/api/users")
		if route == nil {
			t.Error("Should match /api/users")
		}

		route = router.Match("/other")
		if route != nil {
			t.Error("Should not match /other")
		}
	})

	t.Run("matches longer prefix first", func(t *testing.T) {
		router := routes.NewRouter(log, nil)

		router.AddRoute(&config.RouteConfig{
			Name:       "api",
			PathPrefix: "/api/",
			Backends:   []string{"http://localhost:8081"},
		}, &config.CircuitConfig{Enabled: false})

		router.AddRoute(&config.RouteConfig{
			Name:       "api-v2",
			PathPrefix: "/api/v2/",
			Backends:   []string{"http://localhost:8082"},
		}, &config.CircuitConfig{Enabled: false})

		route := router.Match("/api/v2/users")
		if route == nil || route.Config.Name != "api-v2" {
			t.Error("Should match longer prefix first")
		}
	})
}

func TestConfig(t *testing.T) {
	t.Run("default config has valid values", func(t *testing.T) {
		cfg := config.DefaultConfig()

		if cfg.Server.HTTPPort != 8080 {
			t.Errorf("Expected HTTP port 8080, got %d", cfg.Server.HTTPPort)
		}

		if cfg.RateLimit.RequestsPerSec != 100 {
			t.Errorf("Expected rate limit 100, got %f", cfg.RateLimit.RequestsPerSec)
		}

		if !cfg.CORS.Enabled {
			t.Error("CORS should be enabled by default")
		}
	})

	t.Run("validates config", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Server.HTTPPort = -1 // Port 0 is valid (OS assigns available port), but -1 is invalid

		err := cfg.Validate()
		if err == nil {
			t.Error("Should return error for invalid port")
		}
	})

	t.Run("validates auth config", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Auth.Enabled = true
		cfg.Auth.JWTSecret = ""
		cfg.Auth.APIKeys = nil

		err := cfg.Validate()
		if err == nil {
			t.Error("Should return error when auth enabled without credentials")
		}
	})
}

func TestGatewayIntegration(t *testing.T) {
	// Create a test backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"path":     r.URL.Path,
			"trace_id": r.Header.Get("X-Trace-ID"),
		})
	}))
	defer backend.Close()

	log := newTestLogger()

	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Busboy.Enabled = false
	cfg.Routes = []config.RouteConfig{
		{
			Name:       "test",
			PathPrefix: "/api/",
			Backends:   []string{backend.URL},
			Timeout:    5 * time.Second,
		},
	}

	gw, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	handler := gw.buildHandler()

	t.Run("proxies requests to backend", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/users", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}

		body, _ := io.ReadAll(rec.Body)
		var resp map[string]string
		json.Unmarshal(body, &resp)

		if resp["path"] != "/api/users" {
			t.Errorf("Expected path /api/users, got %s", resp["path"])
		}

		if resp["trace_id"] == "" {
			t.Error("Trace ID should be propagated to backend")
		}
	})

	t.Run("adds trace ID header to response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Header().Get("X-Trace-ID") == "" {
			t.Error("Response should have X-Trace-ID header")
		}
	})

	t.Run("returns 404 for unmatched routes", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/unknown", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", rec.Code)
		}
	})
}

func TestHealthEndpoints(t *testing.T) {
	log := newTestLogger()

	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Busboy.Enabled = false

	gw, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	t.Run("liveness endpoint returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/live", nil)
		rec := httptest.NewRecorder()

		gw.livenessHandler(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}
	})

	t.Run("readiness endpoint returns ready status", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/ready", nil)
		rec := httptest.NewRecorder()

		gw.readinessHandler(rec, req)

		// With no routes, it should be ready
		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}
	})
}

func TestGatewayStartStop(t *testing.T) {
	log := newTestLogger()

	cfg := config.DefaultConfig()
	cfg.Server.HTTPPort = 0 // Use random port
	cfg.Auth.Enabled = false
	cfg.Busboy.Enabled = false

	gw, err := New(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start should return after context is cancelled
	err = gw.Start(ctx)
	if err != nil {
		t.Errorf("Unexpected error during shutdown: %v", err)
	}

	if gw.IsRunning() {
		t.Error("Gateway should not be running after shutdown")
	}
}
