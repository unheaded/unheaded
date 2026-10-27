// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	pkgmetrics "unheaded/pkg/metrics"
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

// TestLogging_Levels: success is debug only; failures are warn (4xx) or
// error (5xx), one line per status per failureLogEvery, the next one
// carrying how many were held back.
func TestLogging_Levels(t *testing.T) {
	var buf bytes.Buffer
	lg := zerolog.New(&buf).Level(zerolog.InfoLevel)
	failures = &failureLog{last: map[int]time.Time{}, held: map[int]int{}}
	status := http.StatusOK
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	serve := func(n int) {
		for i := 0; i < n; i++ {
			req := httptest.NewRequest("POST", "/api/v1/topics/t/publish", nil)
			req = req.WithContext(logger.WithContext(req.Context(), &lg))
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
	}

	serve(50)
	if buf.Len() != 0 {
		t.Fatalf("successful requests logged at info: %s", buf.String())
	}

	status = http.StatusBadRequest
	serve(150)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"level":"warn"`) ||
		!strings.Contains(lines[0], `"path":"/api/v1/topics/t/publish"`) || !strings.Contains(lines[0], `"status_code":400`) {
		t.Fatalf("150 identical 400s should give one warn line with method and path, got %d: %s", len(lines), buf.String())
	}

	// After the interval the next one is logged with the count held back.
	failures.last[http.StatusBadRequest] = time.Now().Add(-failureLogEvery)
	buf.Reset()
	serve(1)
	if !strings.Contains(buf.String(), `"same_status_not_logged":149`) {
		t.Fatalf("held-back count missing: %s", buf.String())
	}

	status = http.StatusInternalServerError
	buf.Reset()
	serve(3)
	if strings.Count(buf.String(), "\n") != 1 || !strings.Contains(buf.String(), `"level":"error"`) {
		t.Fatalf("5xx should be one error line: %s", buf.String())
	}
}

// ============================================================================
// Metrics Middleware Tests
// ============================================================================

func TestMetrics(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("metrics test"))
	})
	handler := MetricsFor(MuxRoute(inner))(inner)

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

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := MetricsFor(MuxRoute(inner))(inner)

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
		"X-Frame-Options":           "DENY",
		"X-XSS-Protection":          "1; mode=block",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'self'",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
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

	// A wildcard allow-list answers "*", never the reflected origin: a
	// reflected origin plus Allow-Credentials let any site make credentialed
	// requests (TestCORS_WildcardNeverAllowsCredentials).
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin for wildcard = %q, want *", got)
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

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want %d for timeout", rec.Code, http.StatusServiceUnavailable)
	}
}

// A handler that outlives the timeout cannot write into the response that
// was already sent. It used to: on a real connection that write hit the
// recycled writer and panicked Wotan.
func TestTimeout_LateWriteDropped(t *testing.T) {
	wrote := make(chan error, 1)
	handler := Timeout(20 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		time.Sleep(20 * time.Millisecond)
		_, err := w.Write([]byte("late"))
		wrote <- err
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	select {
	case err := <-wrote:
		if err == nil {
			t.Error("late Write succeeded, want http.ErrHandlerTimeout")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler never wrote")
	}
	if strings.Contains(rec.Body.String(), "late") {
		t.Errorf("late write reached the response: %q", rec.Body.String())
	}
}

// A panic in a timed handler reaches Recovery instead of killing the process.
func TestTimeout_PanicReachesRecovery(t *testing.T) {
	handler := Recovery(Timeout(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want 500 from Recovery", rec.Code)
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

// The legacy unheaded_chat_http_* family labelled by r.URL.Path, topic name
// included: ~27 series per topic across its histograms, growing with every
// topic ever used (2,871 Wotan series live on 2026-09-29, enough to push the
// dashboard's scrape store past its memory limit). It now labels by the
// ServeMux pattern, like the shared unheaded_http_* middleware.
func TestMetricsFor_LabelsByPatternNotRawPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/topics/", func(w http.ResponseWriter, r *http.Request) {})
	h := MetricsFor(MuxRoute(mux))(mux)

	for _, p := range []string{"/api/v1/topics/zz-card-a/publish", "/api/v1/topics/zz-card-b/publish", "/zz-card-nowhere"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, p, nil))
	}
	rec := httptest.NewRecorder()
	pkgmetrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	out := rec.Body.String()
	if strings.Contains(out, "zz-card") {
		t.Errorf("raw path leaked into a label:\n%s", grep(out, "zz-card"))
	}
	for _, want := range []string{`path="/api/v1/topics/"`, `path="unmatched"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestMuxRoute_NotAMux(t *testing.T) {
	route := MuxRoute(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if got := route(httptest.NewRequest(http.MethodGet, "/x/y", nil)); got != "unmatched" {
		t.Errorf("route = %q, want unmatched", got)
	}
}

func grep(s, sub string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}
