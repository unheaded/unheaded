// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Context Initialization Tests ---

func TestContextReset(t *testing.T) {
	c := &Context{
		params: make(map[string]string),
		keys:   make(map[string]any),
	}

	// Set some values
	c.params["id"] = "123"
	c.keys["user"] = "john"
	c.aborted = true
	c.written = true
	c.statusCode = http.StatusNotFound
	c.index = 5

	// Create new request
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	// Reset context
	c.reset(w, req)

	// Verify reset
	if len(c.params) != 0 {
		t.Error("params should be empty after reset")
	}
	if len(c.keys) != 0 {
		t.Error("keys should be empty after reset")
	}
	if c.aborted {
		t.Error("aborted should be false after reset")
	}
	if c.written {
		t.Error("written should be false after reset")
	}
	if c.statusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, c.statusCode)
	}
	if c.index != -1 {
		t.Errorf("expected index -1, got %d", c.index)
	}
}

func TestAcquireReleaseContext(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	c := acquireContext(w, req)

	if c.Writer != w {
		t.Error("Writer not set correctly")
	}
	if c.Request != req {
		t.Error("Request not set correctly")
	}
	if c.index != -1 {
		t.Errorf("expected index -1, got %d", c.index)
	}
	if c.aborted {
		t.Error("aborted should be false")
	}

	// Set some values
	c.params["id"] = "123"
	c.keys["user"] = "john"

	// Release context
	releaseContext(c)

	// Verify cleared
	if len(c.params) != 0 {
		t.Error("params should be empty after release")
	}
	if len(c.keys) != 0 {
		t.Error("keys should be empty after release")
	}
	if c.Request != nil {
		t.Error("Request should be nil after release")
	}
	if c.Writer != nil {
		t.Error("Writer should be nil after release")
	}
}

// --- Path Parameter Tests ---

func TestContextParam(t *testing.T) {
	c := &Context{
		params: map[string]string{
			"id":   "123",
			"name": "john",
		},
	}

	if c.Param("id") != "123" {
		t.Errorf("expected '123', got '%s'", c.Param("id"))
	}
	if c.Param("name") != "john" {
		t.Errorf("expected 'john', got '%s'", c.Param("name"))
	}
	if c.Param("nonexistent") != "" {
		t.Errorf("expected empty string for nonexistent param, got '%s'", c.Param("nonexistent"))
	}
}

func TestContextParamInt(t *testing.T) {
	c := &Context{
		params: map[string]string{
			"id":      "42",
			"invalid": "abc",
		},
	}

	id, err := c.ParamInt("id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Errorf("expected 42, got %d", id)
	}

	_, err = c.ParamInt("invalid")
	if err == nil {
		t.Error("expected error for invalid int")
	}
}

func TestContextParamInt64(t *testing.T) {
	c := &Context{
		params: map[string]string{
			"id": "9223372036854775807",
		},
	}

	id, err := c.ParamInt64("id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 9223372036854775807 {
		t.Errorf("expected 9223372036854775807, got %d", id)
	}
}

func TestContextParamDefault(t *testing.T) {
	c := &Context{
		params: map[string]string{
			"id": "123",
		},
	}

	if c.ParamDefault("id", "default") != "123" {
		t.Error("should return actual value when param exists")
	}
	if c.ParamDefault("nonexistent", "default") != "default" {
		t.Error("should return default value when param doesn't exist")
	}
}

func TestContextParams(t *testing.T) {
	c := &Context{
		params: map[string]string{
			"id":   "123",
			"name": "john",
		},
	}

	params := c.Params()

	// Verify it's a copy
	params["id"] = "modified"
	if c.params["id"] != "123" {
		t.Error("Params() should return a copy, not the original")
	}

	if len(params) != 2 {
		t.Errorf("expected 2 params, got %d", len(params))
	}
}

// --- Query Parameter Tests ---

func TestContextQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/search?q=test&page=5&active=true", nil)
	c := &Context{Request: req}

	if c.Query("q") != "test" {
		t.Errorf("expected 'test', got '%s'", c.Query("q"))
	}
	if c.Query("page") != "5" {
		t.Errorf("expected '5', got '%s'", c.Query("page"))
	}
	if c.Query("nonexistent") != "" {
		t.Errorf("expected empty string for nonexistent query, got '%s'", c.Query("nonexistent"))
	}
}

func TestContextQueryDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/search?q=test", nil)
	c := &Context{Request: req}

	if c.QueryDefault("q", "default") != "test" {
		t.Error("should return actual value when query exists")
	}
	if c.QueryDefault("missing", "default") != "default" {
		t.Error("should return default value when query doesn't exist")
	}
}

func TestContextQueryInt(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/search?page=5&invalid=abc", nil)
	c := &Context{Request: req}

	page, err := c.QueryInt("page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != 5 {
		t.Errorf("expected 5, got %d", page)
	}

	_, err = c.QueryInt("invalid")
	if err == nil {
		t.Error("expected error for invalid int")
	}
}

func TestContextQueryIntDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/search?page=5", nil)
	c := &Context{Request: req}

	if c.QueryIntDefault("page", 1) != 5 {
		t.Error("should return actual value when query exists")
	}
	if c.QueryIntDefault("missing", 1) != 1 {
		t.Error("should return default value when query doesn't exist")
	}
}

func TestContextQueryBool(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/search?active=true&disabled=false&invalid=maybe", nil)
	c := &Context{Request: req}

	active, err := c.QueryBool("active")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected true")
	}

	disabled, err := c.QueryBool("disabled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disabled {
		t.Error("expected false")
	}

	_, err = c.QueryBool("invalid")
	if err == nil {
		t.Error("expected error for invalid bool")
	}
}

func TestContextQueryBoolDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/search?active=true", nil)
	c := &Context{Request: req}

	if !c.QueryBoolDefault("active", false) {
		t.Error("should return actual value when query exists")
	}
	if !c.QueryBoolDefault("missing", true) {
		t.Error("should return default value when query doesn't exist")
	}
}

func TestContextQueryArray(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/search?tags=go&tags=rust&tags=python", nil)
	c := &Context{Request: req}

	tags := c.QueryArray("tags")
	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(tags))
	}
	if tags[0] != "go" || tags[1] != "rust" || tags[2] != "python" {
		t.Errorf("unexpected tags: %v", tags)
	}
}

func TestContextQueryValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/search?q=test&page=5", nil)
	c := &Context{Request: req}

	values := c.QueryValues()
	if values.Get("q") != "test" {
		t.Error("QueryValues should return parsed query values")
	}

	// Call again to test caching
	values2 := c.QueryValues()
	if values2.Get("page") != "5" {
		t.Error("cached QueryValues should work")
	}
}

// --- Request Body Tests ---

func TestContextBody(t *testing.T) {
	body := "test body content"
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	c := &Context{Request: req}

	data, err := c.Body()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != body {
		t.Errorf("expected '%s', got '%s'", body, string(data))
	}
}

func TestContextBindJSON(t *testing.T) {
	type User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}

	body := `{"name":"John","email":"john@example.com","age":30}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	c := &Context{Request: req}

	var user User
	err := c.BindJSON(&user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Name != "John" {
		t.Errorf("expected 'John', got '%s'", user.Name)
	}
	if user.Email != "john@example.com" {
		t.Errorf("expected 'john@example.com', got '%s'", user.Email)
	}
	if user.Age != 30 {
		t.Errorf("expected 30, got %d", user.Age)
	}
}

func TestContextBindJSONInvalid(t *testing.T) {
	body := `invalid json`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	c := &Context{Request: req}

	var data map[string]any
	err := c.BindJSON(&data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- Form Data Tests ---

func TestContextFormValue(t *testing.T) {
	body := "name=John&email=john@example.com"
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := &Context{Request: req}

	if c.FormValue("name") != "John" {
		t.Errorf("expected 'John', got '%s'", c.FormValue("name"))
	}
	if c.FormValue("email") != "john@example.com" {
		t.Errorf("expected 'john@example.com', got '%s'", c.FormValue("email"))
	}
}

func TestContextMultipartForm(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add form field
	_ = writer.WriteField("name", "John")

	// Add file
	part, _ := writer.CreateFormFile("file", "test.txt")
	part.Write([]byte("file content"))

	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/test", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c := &Context{Request: req}

	form, err := c.MultipartForm(32 << 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if form.Value["name"][0] != "John" {
		t.Error("form field not parsed correctly")
	}
	if len(form.File["file"]) != 1 {
		t.Error("form file not parsed correctly")
	}
}

func TestContextFormFile(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, _ := writer.CreateFormFile("document", "test.pdf")
	part.Write([]byte("pdf content"))

	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/test", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c := &Context{Request: req}

	fh, err := c.FormFile("document")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fh.Filename != "test.pdf" {
		t.Errorf("expected 'test.pdf', got '%s'", fh.Filename)
	}
}

// --- Request Info Tests ---

func TestContextMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	c := &Context{Request: req}

	if c.Method() != http.MethodPost {
		t.Errorf("expected '%s', got '%s'", http.MethodPost, c.Method())
	}
}

func TestContextPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	c := &Context{Request: req}

	if c.Path() != "/users/123" {
		t.Errorf("expected '/users/123', got '%s'", c.Path())
	}
}

func TestContextFullPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?page=1", nil)
	c := &Context{Request: req}

	if c.FullPath() != "/users?page=1" {
		t.Errorf("expected '/users?page=1', got '%s'", c.FullPath())
	}
}

func TestContextHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	c := &Context{Request: req}

	if c.Host() != "example.com" {
		t.Errorf("expected 'example.com', got '%s'", c.Host())
	}
}

func TestContextUserAgent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	c := &Context{Request: req}

	if c.UserAgent() != "TestAgent/1.0" {
		t.Errorf("expected 'TestAgent/1.0', got '%s'", c.UserAgent())
	}
}

func TestContextContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Content-Type", "application/json")
	c := &Context{Request: req}

	if c.ContentType() != "application/json" {
		t.Errorf("expected 'application/json', got '%s'", c.ContentType())
	}
}

func TestContextGetHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer token123")
	c := &Context{Request: req}

	if c.GetHeader("Authorization") != "Bearer token123" {
		t.Error("GetHeader should return header value")
	}
	if c.GetHeader("Nonexistent") != "" {
		t.Error("GetHeader should return empty string for nonexistent header")
	}
}

func TestContextClientIP(t *testing.T) {
	// Secure default: XFF/XRI headers are IGNORED unless the peer is a trusted proxy.
	// httptest.NewRequest sets RemoteAddr to "192.0.2.1:1234".

	t.Run("untrusted peer ignores XFF", func(t *testing.T) {
		SetTrustedProxies(nil) // ensure clean state
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")
		c := &Context{Request: req}
		got := c.ClientIP()
		if got != "192.0.2.1" {
			t.Errorf("expected RemoteAddr '192.0.2.1', got '%s'", got)
		}
	})

	t.Run("untrusted peer ignores XRI", func(t *testing.T) {
		SetTrustedProxies(nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Real-IP", "10.0.0.100")
		c := &Context{Request: req}
		got := c.ClientIP()
		if got != "192.0.2.1" {
			t.Errorf("expected RemoteAddr '192.0.2.1', got '%s'", got)
		}
	})

	t.Run("trusted proxy uses XFF single", func(t *testing.T) {
		SetTrustedProxies([]string{"192.0.2.1"})
		defer SetTrustedProxies(nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")
		c := &Context{Request: req}
		got := c.ClientIP()
		if got != "192.168.1.1" {
			t.Errorf("expected '192.168.1.1', got '%s'", got)
		}
	})

	t.Run("trusted proxy uses XFF multiple", func(t *testing.T) {
		SetTrustedProxies([]string{"192.0.2.1"})
		defer SetTrustedProxies(nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.1, 172.16.0.1")
		c := &Context{Request: req}
		got := c.ClientIP()
		if got != "192.168.1.1" {
			t.Errorf("expected '192.168.1.1', got '%s'", got)
		}
	})

	t.Run("trusted proxy uses XFF with spaces", func(t *testing.T) {
		SetTrustedProxies([]string{"192.0.2.1"})
		defer SetTrustedProxies(nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", "  192.168.1.1  ")
		c := &Context{Request: req}
		got := c.ClientIP()
		if got != "192.168.1.1" {
			t.Errorf("expected '192.168.1.1', got '%s'", got)
		}
	})

	t.Run("trusted proxy uses XRI", func(t *testing.T) {
		SetTrustedProxies([]string{"192.0.2.1"})
		defer SetTrustedProxies(nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Real-IP", "10.0.0.100")
		c := &Context{Request: req}
		got := c.ClientIP()
		if got != "10.0.0.100" {
			t.Errorf("expected '10.0.0.100', got '%s'", got)
		}
	})

	t.Run("RemoteAddr with port", func(t *testing.T) {
		SetTrustedProxies(nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		c := &Context{Request: req}
		got := c.ClientIP()
		if got != "192.168.1.1" {
			t.Errorf("expected '192.168.1.1', got '%s'", got)
		}
	})

	t.Run("RemoteAddr without port", func(t *testing.T) {
		SetTrustedProxies(nil)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1"
		c := &Context{Request: req}
		got := c.ClientIP()
		if got != "192.168.1.1" {
			t.Errorf("expected '192.168.1.1', got '%s'", got)
		}
	})
}

// --- Context Values (Request-Scoped Storage) Tests ---

func TestContextSetGet(t *testing.T) {
	c := &Context{
		keys: make(map[string]any),
	}

	c.Set("user", "john")
	c.Set("count", 42)
	c.Set("active", true)

	val, ok := c.Get("user")
	if !ok {
		t.Error("expected key 'user' to exist")
	}
	if val != "john" {
		t.Errorf("expected 'john', got '%v'", val)
	}

	_, ok = c.Get("nonexistent")
	if ok {
		t.Error("expected key 'nonexistent' to not exist")
	}
}

func TestContextMustGet(t *testing.T) {
	c := &Context{
		keys: map[string]any{
			"user": "john",
		},
	}

	val := c.MustGet("user")
	if val != "john" {
		t.Errorf("expected 'john', got '%v'", val)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nonexistent key")
		}
	}()
	c.MustGet("nonexistent")
}

func TestContextGetString(t *testing.T) {
	c := &Context{
		keys: map[string]any{
			"name":    "john",
			"count":   42,
			"missing": nil,
		},
	}

	if c.GetString("name") != "john" {
		t.Error("GetString should return string value")
	}
	if c.GetString("count") != "" {
		t.Error("GetString should return empty string for non-string value")
	}
	if c.GetString("nonexistent") != "" {
		t.Error("GetString should return empty string for nonexistent key")
	}
}

func TestContextGetInt(t *testing.T) {
	c := &Context{
		keys: map[string]any{
			"count":  42,
			"name":   "john",
			"amount": int64(100),
		},
	}

	if c.GetInt("count") != 42 {
		t.Error("GetInt should return int value")
	}
	if c.GetInt("name") != 0 {
		t.Error("GetInt should return 0 for non-int value")
	}
	if c.GetInt("nonexistent") != 0 {
		t.Error("GetInt should return 0 for nonexistent key")
	}
}

func TestContextGetInt64(t *testing.T) {
	c := &Context{
		keys: map[string]any{
			"bignum": int64(9223372036854775807),
			"count":  42,
		},
	}

	if c.GetInt64("bignum") != 9223372036854775807 {
		t.Error("GetInt64 should return int64 value")
	}
	if c.GetInt64("count") != 0 {
		t.Error("GetInt64 should return 0 for non-int64 value")
	}
}

func TestContextGetBool(t *testing.T) {
	c := &Context{
		keys: map[string]any{
			"active":   true,
			"disabled": false,
			"name":     "john",
		},
	}

	if !c.GetBool("active") {
		t.Error("GetBool should return true for active")
	}
	if c.GetBool("disabled") {
		t.Error("GetBool should return false for disabled")
	}
	if c.GetBool("name") {
		t.Error("GetBool should return false for non-bool value")
	}
	if c.GetBool("nonexistent") {
		t.Error("GetBool should return false for nonexistent key")
	}
}

// --- Context Values Concurrency Tests ---

func TestContextSetGetConcurrent(t *testing.T) {
	c := &Context{
		keys: make(map[string]any),
	}

	done := make(chan bool)

	// Multiple writers
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				c.Set("key", n)
			}
			done <- true
		}(i)
	}

	// Multiple readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.Get("key")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
}

// --- Standard Context Interface Tests ---

func TestContextContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), "key", "value")
	req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
	c := &Context{Request: req}

	stdCtx := c.Context()
	if stdCtx.Value("key") != "value" {
		t.Error("Context() should return request's context")
	}
}

func TestContextWithContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	c := &Context{Request: req}

	newCtx := context.WithValue(context.Background(), "key", "value")
	c2 := c.WithContext(newCtx)

	if c2.Request.Context().Value("key") != "value" {
		t.Error("WithContext should update request's context")
	}
}

func TestContextDeadline(t *testing.T) {
	deadline := time.Now().Add(1 * time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
	c := &Context{Request: req}

	d, ok := c.Deadline()
	if !ok {
		t.Error("expected deadline to be set")
	}
	if d != deadline {
		t.Error("deadline mismatch")
	}
}

func TestContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
	c := &Context{Request: req}

	// Should not be done yet
	select {
	case <-c.Done():
		t.Error("should not be done yet")
	default:
	}

	// Cancel context
	cancel()

	// Should be done now
	select {
	case <-c.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("should be done after cancel")
	}
}

func TestContextErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
	c := &Context{Request: req}

	if c.Err() != nil {
		t.Error("Err should be nil before cancel")
	}

	cancel()

	if c.Err() != context.Canceled {
		t.Error("Err should be context.Canceled after cancel")
	}
}

func TestContextValue(t *testing.T) {
	// Test context keys
	c := &Context{
		keys: map[string]any{
			"local": "localvalue",
		},
	}
	ctx := context.WithValue(context.Background(), "contextkey", "contextvalue")
	req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
	c.Request = req

	// Local key takes precedence
	if c.Value("local") != "localvalue" {
		t.Error("should return local value for string key")
	}

	// Falls back to request context
	if c.Value("contextkey") != "contextvalue" {
		t.Error("should return context value for non-local key")
	}

	// Non-string keys go directly to context
	type customKey struct{}
	ctx2 := context.WithValue(ctx, customKey{}, "customvalue")
	c.Request = c.Request.WithContext(ctx2)

	if c.Value(customKey{}) != "customvalue" {
		t.Error("should return context value for non-string key")
	}
}

// --- Handler Chain Tests ---

func TestContextNext(t *testing.T) {
	order := []string{}

	handlers := []HandlerFunc{
		func(c *Context) {
			order = append(order, "h1-before")
			c.Next()
			order = append(order, "h1-after")
		},
		func(c *Context) {
			order = append(order, "h2-before")
			c.Next()
			order = append(order, "h2-after")
		},
		func(c *Context) {
			order = append(order, "h3")
		},
	}

	c := &Context{
		handlers: handlers,
		index:    -1,
	}

	c.Next()

	expected := []string{"h1-before", "h2-before", "h3", "h2-after", "h1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d", len(expected), len(order))
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("expected %v, got %v", expected, order)
			break
		}
	}
}

func TestContextAbort(t *testing.T) {
	order := []string{}

	handlers := []HandlerFunc{
		func(c *Context) {
			order = append(order, "h1")
			c.Abort()
		},
		func(c *Context) {
			order = append(order, "h2")
		},
	}

	c := &Context{
		handlers: handlers,
		index:    -1,
	}

	c.Next()

	if len(order) != 1 || order[0] != "h1" {
		t.Errorf("expected only 'h1', got %v", order)
	}
	if !c.IsAborted() {
		t.Error("IsAborted should return true")
	}
}

func TestContextAbortWithStatus(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Context{
		Writer:     w,
		handlers:   []HandlerFunc{},
		index:      -1,
		statusCode: http.StatusOK,
	}

	c.AbortWithStatus(http.StatusForbidden)

	if !c.IsAborted() {
		t.Error("should be aborted")
	}
	if c.statusCode != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, c.statusCode)
	}
}

func TestContextAbortWithError(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Context{
		Writer:     w,
		handlers:   []HandlerFunc{},
		index:      -1,
		statusCode: http.StatusOK,
	}

	err := io.EOF
	c.AbortWithError(http.StatusBadRequest, err)

	if !c.IsAborted() {
		t.Error("should be aborted")
	}
	if c.statusCode != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, c.statusCode)
	}
}

// --- Integration Test with Router ---

func TestContextWithRouter(t *testing.T) {
	r := NewRouter()

	var capturedContext *Context

	r.GET("/users/:id", func(c *Context) {
		capturedContext = &Context{
			params:     c.Params(),
			statusCode: c.statusCode,
		}
		c.JSON(http.StatusOK, map[string]string{
			"id": c.Param("id"),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42?name=john", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if capturedContext.params["id"] != "42" {
		t.Error("path param not captured correctly")
	}

	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["id"] != "42" {
		t.Error("JSON response not correct")
	}
}
