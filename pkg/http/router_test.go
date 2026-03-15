// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Router Tests ---

func TestNewRouter(t *testing.T) {
	r := NewRouter()
	if r == nil {
		t.Fatal("NewRouter returned nil")
	}
	if r.trees == nil {
		t.Fatal("Router trees not initialized")
	}
}

func TestRouterGET(t *testing.T) {
	r := NewRouter()

	called := false
	r.GET("/test", func(c *Context) {
		called = true
		c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if !called {
		t.Error("Handler was not called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRouterPOST(t *testing.T) {
	r := NewRouter()

	r.POST("/test", func(c *Context) {
		c.String(http.StatusCreated, "Created")
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}
}

func TestRouterPUT(t *testing.T) {
	r := NewRouter()

	r.PUT("/test", func(c *Context) {
		c.String(http.StatusOK, "Updated")
	})

	req := httptest.NewRequest(http.MethodPut, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRouterDELETE(t *testing.T) {
	r := NewRouter()

	r.DELETE("/test", func(c *Context) {
		c.NoContent()
	})

	req := httptest.NewRequest(http.MethodDelete, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status %d, got %d", http.StatusNoContent, w.Code)
	}
}

func TestRouterPathParams(t *testing.T) {
	r := NewRouter()

	r.GET("/users/:id", func(c *Context) {
		id := c.Param("id")
		c.String(http.StatusOK, "User: %s", id)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "User: 123" {
		t.Errorf("Expected 'User: 123', got '%s'", w.Body.String())
	}
}

func TestRouterMultiplePathParams(t *testing.T) {
	r := NewRouter()

	r.GET("/users/:userId/posts/:postId", func(c *Context) {
		userId := c.Param("userId")
		postId := c.Param("postId")
		c.String(http.StatusOK, "User: %s, Post: %s", userId, postId)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42/posts/100", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	expected := "User: 42, Post: 100"
	if w.Body.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, w.Body.String())
	}
}

func TestRouterCatchAll(t *testing.T) {
	r := NewRouter()

	r.GET("/files/*path", func(c *Context) {
		filePath := c.Param("path")
		c.String(http.StatusOK, "Path: %s", filePath)
	})

	req := httptest.NewRequest(http.MethodGet, "/files/documents/report.pdf", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	expected := "Path: /documents/report.pdf"
	if w.Body.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, w.Body.String())
	}
}

func TestRouterNotFound(t *testing.T) {
	r := NewRouter()

	r.GET("/test", func(c *Context) {
		c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestRouterMethodNotAllowed(t *testing.T) {
	r := NewRouter()

	r.GET("/test", func(c *Context) {
		c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// --- Middleware Tests ---

func TestRouterMiddleware(t *testing.T) {
	r := NewRouter()

	callOrder := []string{}

	r.Use(func(c *Context) {
		callOrder = append(callOrder, "middleware1")
		c.Next()
	})

	r.Use(func(c *Context) {
		callOrder = append(callOrder, "middleware2")
		c.Next()
	})

	r.GET("/test", func(c *Context) {
		callOrder = append(callOrder, "handler")
		c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	expected := []string{"middleware1", "middleware2", "handler"}
	if len(callOrder) != len(expected) {
		t.Errorf("Expected %d calls, got %d", len(expected), len(callOrder))
	}
	for i, v := range expected {
		if callOrder[i] != v {
			t.Errorf("Expected call order %v, got %v", expected, callOrder)
			break
		}
	}
}

func TestMiddlewareAbort(t *testing.T) {
	r := NewRouter()

	handlerCalled := false

	r.Use(func(c *Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})

	r.GET("/test", func(c *Context) {
		handlerCalled = true
		c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if handlerCalled {
		t.Error("Handler should not have been called after abort")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

// --- Group Tests ---

func TestRouteGroup(t *testing.T) {
	r := NewRouter()

	api := r.Group("/api")
	api.GET("/users", func(c *Context) {
		c.String(http.StatusOK, "Users")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "Users" {
		t.Errorf("Expected 'Users', got '%s'", w.Body.String())
	}
}

func TestRouteGroupNested(t *testing.T) {
	r := NewRouter()

	api := r.Group("/api")
	v1 := api.Group("/v1")
	v1.GET("/users", func(c *Context) {
		c.String(http.StatusOK, "V1 Users")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRouteGroupMiddleware(t *testing.T) {
	r := NewRouter()

	authCalled := false

	api := r.Group("/api")
	api.Use(func(c *Context) {
		authCalled = true
		c.Next()
	})
	api.GET("/users", func(c *Context) {
		c.String(http.StatusOK, "Users")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if !authCalled {
		t.Error("Group middleware was not called")
	}
}

// --- Context Tests ---

func TestContextQueryParams(t *testing.T) {
	r := NewRouter()

	r.GET("/search", func(c *Context) {
		q := c.Query("q")
		page := c.QueryIntDefault("page", 1)
		c.String(http.StatusOK, "Query: %s, Page: %d", q, page)
	})

	req := httptest.NewRequest(http.MethodGet, "/search?q=test&page=5", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	expected := "Query: test, Page: 5"
	if w.Body.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, w.Body.String())
	}
}

func TestContextJSON(t *testing.T) {
	r := NewRouter()

	r.GET("/json", func(c *Context) {
		c.JSON(http.StatusOK, map[string]string{"message": "hello"})
	})

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Expected JSON content type, got '%s'", contentType)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if result["message"] != "hello" {
		t.Errorf("Expected message 'hello', got '%s'", result["message"])
	}
}

func TestContextBindJSON_Router(t *testing.T) {
	r := NewRouter()

	type User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	r.POST("/users", func(c *Context) {
		var user User
		if err := c.BindJSON(&user); err != nil {
			c.BadRequest(err.Error())
			return
		}
		c.JSON(http.StatusCreated, user)
	})

	body := bytes.NewBufferString(`{"name":"John","email":"john@example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/users", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}
}

func TestContextSetGet_Router(t *testing.T) {
	r := NewRouter()

	r.Use(func(c *Context) {
		c.Set("user", "john")
		c.Next()
	})

	r.GET("/test", func(c *Context) {
		user := c.GetString("user")
		c.String(http.StatusOK, "User: %s", user)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	expected := "User: john"
	if w.Body.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, w.Body.String())
	}
}

func TestContextRedirect(t *testing.T) {
	r := NewRouter()

	r.GET("/old", func(c *Context) {
		c.Redirect(http.StatusMovedPermanently, "/new")
	})

	req := httptest.NewRequest(http.MethodGet, "/old", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("Expected status %d, got %d", http.StatusMovedPermanently, w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/new" {
		t.Errorf("Expected Location '/new', got '%s'", location)
	}
}

// --- Response Writer Tests ---

func TestResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewResponseWriter(w)

	rw.WriteHeader(http.StatusCreated)
	rw.Write([]byte("test"))

	if rw.Status() != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, rw.Status())
	}

	if rw.Size() != 4 {
		t.Errorf("Expected size 4, got %d", rw.Size())
	}

	if !rw.Written() {
		t.Error("Expected written to be true")
	}
}

// --- Built-in Middleware Tests ---

func TestRecoveryMiddleware(t *testing.T) {
	r := NewRouter()
	r.Use(Recovery())

	r.GET("/panic", func(c *Context) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()

	// Should not panic
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	r := NewRouter()
	r.Use(CORS())

	r.GET("/test", func(c *Context) {
		c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("Expected CORS origin '*', got '%s'", origin)
	}
}

func TestCORSPreflightRequest(t *testing.T) {
	r := NewRouter()
	r.Use(CORS())

	// Register both GET and OPTIONS
	r.GET("/test", func(c *Context) {
		c.String(http.StatusOK, "OK")
	})
	r.OPTIONS("/test", func(c *Context) {
		// CORS middleware will handle this
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status %d, got %d", http.StatusNoContent, w.Code)
	}

	methods := w.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("Expected Access-Control-Allow-Methods header")
	}
}

func TestBasicAuthMiddleware(t *testing.T) {
	r := NewRouter()
	r.Use(BasicAuth(map[string]string{
		"admin": "secret",
	}))

	r.GET("/protected", func(c *Context) {
		c.String(http.StatusOK, "Protected")
	})

	// Without auth
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d without auth, got %d", http.StatusUnauthorized, w.Code)
	}

	// With valid auth
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.SetBasicAuth("admin", "secret")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d with valid auth, got %d", http.StatusOK, w.Code)
	}

	// With invalid auth
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.SetBasicAuth("admin", "wrong")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d with invalid auth, got %d", http.StatusUnauthorized, w.Code)
	}
}

// --- Radix Tree Performance Tests ---

func TestRadixTreeManyRoutes(t *testing.T) {
	r := NewRouter()

	// Register many routes
	routes := []string{
		"/",
		"/users",
		"/users/:id",
		"/users/:id/posts",
		"/users/:id/posts/:postId",
		"/api/v1/users",
		"/api/v1/posts",
		"/api/v2/users",
		"/api/v2/posts",
		"/static/*path",
	}

	for _, route := range routes {
		r.GET(route, func(c *Context) {
			c.String(http.StatusOK, "OK")
		})
	}

	// Test each route
	testCases := []struct {
		path       string
		wantStatus int
	}{
		{"/", http.StatusOK},
		{"/users", http.StatusOK},
		{"/users/123", http.StatusOK},
		{"/users/123/posts", http.StatusOK},
		{"/users/123/posts/456", http.StatusOK},
		{"/api/v1/users", http.StatusOK},
		{"/api/v2/posts", http.StatusOK},
		{"/static/js/app.js", http.StatusOK},
		{"/notfound", http.StatusNotFound},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != tc.wantStatus {
			t.Errorf("Path %s: expected status %d, got %d", tc.path, tc.wantStatus, w.Code)
		}
	}
}

// --- Benchmark Tests ---

func BenchmarkRouterStaticPath(b *testing.B) {
	r := NewRouter()
	r.GET("/users/list", func(c *Context) {
		c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/users/list", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkRouterPathParam(b *testing.B) {
	r := NewRouter()
	r.GET("/users/:id", func(c *Context) {
		c.Param("id")
		c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkRouterManyRoutes(b *testing.B) {
	r := NewRouter()

	// Register 100 routes
	for i := 0; i < 100; i++ {
		path := "/route" + string(rune('A'+i%26)) + string(rune('0'+i%10))
		r.GET(path, func(c *Context) {
			c.String(http.StatusOK, "OK")
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/routeZ9", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

// --- Server Tests ---

func TestServerCreation(t *testing.T) {
	r := NewRouter()
	s := NewServer(r)

	if s.Router != r {
		t.Error("Server router not set correctly")
	}
}

func TestServerConfig(t *testing.T) {
	r := NewRouter()
	config := ServerConfig{
		Addr:            ":9090",
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    10 * time.Second,
		ShutdownTimeout: 5 * time.Second,
	}

	s := NewServerWithConfig(r, config)

	if s.Addr() != ":9090" {
		t.Errorf("Expected addr ':9090', got '%s'", s.Addr())
	}
}

// --- Health Check Tests ---

func TestHealthCheck(t *testing.T) {
	r := NewRouter()
	r.RegisterHealthChecks(HealthCheckConfig{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestLivenessProbe(t *testing.T) {
	r := NewRouter()
	r.RegisterHealthChecks(HealthCheckConfig{})

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// --- Metrics Tests ---

func TestMetrics(t *testing.T) {
	metrics := NewMetrics()
	r := NewRouter()
	r.Use(MetricsMiddleware(metrics))

	r.GET("/test", func(c *Context) {
		c.String(http.StatusOK, "OK")
	})

	// Make some requests
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	snapshot := metrics.Snapshot()
	if snapshot["request_count"].(int64) != 10 {
		t.Errorf("Expected 10 requests, got %d", snapshot["request_count"])
	}
}

// --- Integration Tests ---

func TestFullRequestLifecycle(t *testing.T) {
	r := NewRouter()

	// Add middleware
	r.Use(Logger())
	r.Use(Recovery())

	// Add routes
	api := r.Group("/api")
	api.Use(func(c *Context) {
		c.Set("api", true)
		c.Next()
	})

	api.GET("/users/:id", func(c *Context) {
		id := c.Param("id")
		isAPI := c.GetBool("api")

		c.JSON(http.StatusOK, map[string]any{
			"id":    id,
			"isAPI": isAPI,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users/42", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if result["id"] != "42" {
		t.Errorf("Expected id '42', got '%v'", result["id"])
	}

	if result["isAPI"] != true {
		t.Errorf("Expected isAPI true, got '%v'", result["isAPI"])
	}
}

func TestContextClientIP_Router(t *testing.T) {
	r := NewRouter()

	r.GET("/ip", func(c *Context) {
		c.String(http.StatusOK, "%s", c.ClientIP())
	})

	// Test 1: XFF from untrusted peer is IGNORED (secure default).
	// httptest.NewRequest sets RemoteAddr to "192.0.2.1:1234".
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.1")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Body.String() != "192.0.2.1" {
		t.Errorf("Expected RemoteAddr IP '192.0.2.1' (XFF ignored from untrusted peer), got '%s'", w.Body.String())
	}

	// Test 2: XFF from trusted proxy IS used.
	SetTrustedProxies([]string{"192.0.2.1"})
	defer SetTrustedProxies(nil) // cleanup

	req2 := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req2.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.1")
	w2 := httptest.NewRecorder()

	r.ServeHTTP(w2, req2)

	if w2.Body.String() != "192.168.1.1" {
		t.Errorf("Expected XFF IP '192.168.1.1' (trusted proxy), got '%s'", w2.Body.String())
	}
}

func TestContextFormValue_Router(t *testing.T) {
	r := NewRouter()

	r.POST("/form", func(c *Context) {
		name := c.FormValue("name")
		c.String(http.StatusOK, "Hello, %s", name)
	})

	body := strings.NewReader("name=World")
	req := httptest.NewRequest(http.MethodPost, "/form", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	expected := "Hello, World"
	if w.Body.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, w.Body.String())
	}
}

func TestContextHTML(t *testing.T) {
	r := NewRouter()

	r.GET("/html", func(c *Context) {
		c.HTML(http.StatusOK, "<h1>Hello</h1>")
	})

	req := httptest.NewRequest(http.MethodGet, "/html", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected HTML content type, got '%s'", contentType)
	}

	if w.Body.String() != "<h1>Hello</h1>" {
		t.Errorf("Expected '<h1>Hello</h1>', got '%s'", w.Body.String())
	}
}

func TestContextData(t *testing.T) {
	r := NewRouter()

	r.GET("/data", func(c *Context) {
		c.Data(http.StatusOK, "application/octet-stream", []byte{0x01, 0x02, 0x03})
	})

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	data, _ := io.ReadAll(w.Body)
	if len(data) != 3 || data[0] != 0x01 {
		t.Errorf("Expected binary data, got %v", data)
	}
}

func TestRouterAny(t *testing.T) {
	r := NewRouter()

	r.Any("/any", func(c *Context) {
		c.String(http.StatusOK, "%s", c.Method())
	})

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
	}

	for _, method := range methods {
		req := httptest.NewRequest(method, "/any", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Method %s: expected status %d, got %d", method, http.StatusOK, w.Code)
		}
	}
}

func TestRouteInfo(t *testing.T) {
	r := NewRouter()

	r.GET("/users", func(c *Context) {})
	r.POST("/users", func(c *Context) {})
	r.GET("/users/:id", func(c *Context) {})

	routes := r.Routes()

	if len(routes) < 3 {
		t.Errorf("Expected at least 3 routes, got %d", len(routes))
	}
}
