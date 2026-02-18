package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/services/wotan/internal/logger"
	"unheaded/services/wotan/internal/metrics"
)

func init() {
	// Initialize logger for tests
	logger.Initialize(logger.Config{Level: "debug"})
	// Initialize metrics for tests
	metrics.Initialize("middleware_test")
}

// ============================================================================
// responseWriter Tests
// ============================================================================

func TestResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)

	if rw.statusCode != http.StatusNotFound {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusNotFound)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("underlying recorder code = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestResponseWriter_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	data := []byte("hello world")
	n, err := rw.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}
	if rw.size != len(data) {
		t.Errorf("size = %d, want %d", rw.size, len(data))
	}

	// Write more data
	n2, _ := rw.Write([]byte("!"))
	if rw.size != len(data)+n2 {
		t.Errorf("size after second write = %d, want %d", rw.size, len(data)+n2)
	}
}

// ============================================================================
// Logging Middleware Tests
// ============================================================================

func TestLogging(t *testing.T) {
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify request ID header was set
	requestID := rec.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("X-Request-ID header not set")
	}
}

func TestLogging_PreservesStatusCode(t *testing.T) {
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest("GET", "/notfound", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ============================================================================
// Metrics Middleware Tests
// ============================================================================

func TestMetrics(t *testing.T) {
	handler := Metrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("metrics test"))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMetrics_NilMetrics(t *testing.T) {
	// Save and restore the default metrics
	savedMetrics := metrics.Get()
	defer func() {
		if savedMetrics != nil {
			metrics.Initialize("middleware_test_restore")
		}
	}()

	handler := Metrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic even with nil metrics
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ============================================================================
// SecurityHeaders Middleware Tests
// ============================================================================

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":          "DENY",
		"X-XSS-Protection":         "1; mode=block",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'self'",
		"Referrer-Policy":          "strict-origin-when-cross-origin",
	}

	for header, expected := range expectedHeaders {
		got := rec.Header().Get(header)
		if got != expected {
			t.Errorf("%s = %q, want %q", header, got, expected)
		}
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ============================================================================
// CORS Middleware Tests
// ============================================================================

func TestCORS_AllowedOrigin(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Error("CORS Allow-Origin header not set for allowed origin")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestCORS_WildcardOrigin(t *testing.T) {
	handler := CORS([]string{"*"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://any-origin.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "http://any-origin.com" {
		t.Error("CORS Allow-Origin header not set for wildcard")
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	handler := CORS([]string{"http://allowed.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS Allow-Origin header should not be set for disallowed origin")
	}
}

func TestCORS_Preflight(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for preflight")
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Status = %d, want %d for preflight", rec.Code, http.StatusNoContent)
	}
}

// ============================================================================
// Timeout Middleware Tests
// ============================================================================

func TestTimeout_Completes(t *testing.T) {
	handler := Timeout(1 * time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestTimeout_Exceeds(t *testing.T) {
	handler := Timeout(50 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/slow", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("Status = %d, want %d for timeout", rec.Code, http.StatusGatewayTimeout)
	}
}

// ============================================================================
// Recovery Middleware Tests
// ============================================================================

func TestRecovery_NoPanic(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRecovery_WithPanic(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest("GET", "/panic", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d after panic", rec.Code, http.StatusInternalServerError)
	}
}

// ============================================================================
// Chain Middleware Tests
// ============================================================================

func TestChain(t *testing.T) {
	var order []string

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw1-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw1-after")
		})
	}

	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw2-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw2-after")
		})
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	})

	chained := Chain(handler, mw1, mw2)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	chained.ServeHTTP(rec, req)

	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("order length = %d, want %d", len(order), len(expected))
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %s, want %s", i, order[i], v)
		}
	}
}

func TestChain_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	chained := Chain(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	chained.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ============================================================================
// AdminAuthHandler Tests (additional cases not in admin_auth_test.go)
// ============================================================================

func TestAdminAuthHandler_MissingKey(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "test-key-123",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	handler := AdminAuthHandler(config, func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called without key")
	})

	req := httptest.NewRequest("POST", "/admin/action", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminAuthHandler_EmptyAPIKey(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "", // Not configured
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	handler := AdminAuthHandler(config, func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when API key not configured")
	})

	req := httptest.NewRequest("POST", "/admin/action", nil)
	req.Header.Set("X-Admin-API-Key", "some-key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestAdminAuthHandler_BearerToken(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "test-key-123",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	called := false
	handler := AdminAuthHandler(config, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/admin/action", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("Handler was not called with valid Bearer token")
	}
}

func TestAdminAuthHandler_DefaultHeaderName(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "test-key-123",
		HeaderName: "", // Empty, should default to X-Admin-API-Key
		Realm:      "Test",
	}

	called := false
	handler := AdminAuthHandler(config, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/admin/action", nil)
	req.Header.Set("X-Admin-API-Key", "test-key-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("Handler was not called with default header name")
	}
}

// ============================================================================
// IsAdminAuthenticated Tests (additional cases not in admin_auth_test.go)
// ============================================================================

func TestIsAdminAuthenticated_NotSet(t *testing.T) {
	ctx := httptest.NewRequest("GET", "/", nil).Context()
	if IsAdminAuthenticated(ctx) {
		t.Error("IsAdminAuthenticated should return false when not set")
	}
}

// ============================================================================
// statusCodeString Tests
// ============================================================================

func TestStatusCodeString(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{401, "401"},
		{403, "403"},
		{500, "500"},
		{404, "500"}, // Default case
	}

	for _, tt := range tests {
		got := statusCodeString(tt.code)
		if got != tt.want {
			t.Errorf("statusCodeString(%d) = %s, want %s", tt.code, got, tt.want)
		}
	}
}

// ============================================================================
// writeAuthError Tests
// ============================================================================

func TestWriteAuthError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAuthError(rec, http.StatusUnauthorized, "test_error", "test message")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	body := rec.Body.String()
	if body == "" {
		t.Error("Response body is empty")
	}
}
