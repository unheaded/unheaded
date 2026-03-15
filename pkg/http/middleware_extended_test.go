// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddleware(t *testing.T) {
	router := NewRouter()
	router.Use(RequestID())
	router.GET("/test", func(c *Context) {
		reqID, _ := c.Get("request_id")
		c.String(http.StatusOK, "%s", reqID.(string))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Header().Get(HeaderXRequestID) == "" {
		t.Error("expected X-Request-ID header to be set")
	}
	if w.Body.Len() == 0 {
		t.Error("expected request ID in body")
	}
}

func TestRequestIDMiddleware_ExistingID(t *testing.T) {
	router := NewRouter()
	router.Use(RequestID())
	router.GET("/test", func(c *Context) {
		reqID, _ := c.Get("request_id")
		c.String(http.StatusOK, "%s", reqID.(string))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderXRequestID, "existing-id")
	router.ServeHTTP(w, req)

	if w.Body.String() != "existing-id" {
		t.Errorf("expected existing-id, got %s", w.Body.String())
	}
}

func TestRequestIDWithConfig_CustomHeader(t *testing.T) {
	router := NewRouter()
	router.Use(RequestIDWithConfig(RequestIDConfig{
		Header:    "X-Custom-ID",
		Generator: func() string { return "custom-123" },
	}))
	router.GET("/test", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Header().Get("X-Custom-ID") != "custom-123" {
		t.Errorf("expected custom-123, got %s", w.Header().Get("X-Custom-ID"))
	}
}

func TestSecureMiddleware(t *testing.T) {
	router := NewRouter()
	router.Use(Secure())
	router.GET("/test", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Header().Get("X-XSS-Protection") == "" {
		t.Error("expected X-XSS-Protection header")
	}
	if w.Header().Get("X-Content-Type-Options") == "" {
		t.Error("expected X-Content-Type-Options header")
	}
	if w.Header().Get("X-Frame-Options") == "" {
		t.Error("expected X-Frame-Options header")
	}
}

func TestSecureWithConfig(t *testing.T) {
	router := NewRouter()
	router.Use(SecureWithConfig(SecureConfig{
		HSTSMaxAge:            31536000,
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "no-referrer",
	}))
	router.GET("/test", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("expected HSTS header")
	}
	if w.Header().Get("Content-Security-Policy") != "default-src 'self'" {
		t.Error("expected CSP header")
	}
	if w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Error("expected Referrer-Policy header")
	}
}

func TestContextWritten(t *testing.T) {
	router := NewRouter()
	router.GET("/test", func(c *Context) {
		if c.Written() {
			t.Error("should not be written yet")
		}
		c.String(http.StatusOK, "ok")
		if !c.Written() {
			t.Error("should be written after response")
		}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)
}

func TestContextStatusCode(t *testing.T) {
	router := NewRouter()
	router.GET("/test", func(c *Context) {
		c.Status(http.StatusCreated)
		if c.StatusCode() != http.StatusCreated {
			t.Errorf("StatusCode = %d, want 201", c.StatusCode())
		}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)
}

func TestContextAddHeader(t *testing.T) {
	router := NewRouter()
	router.GET("/test", func(c *Context) {
		c.AddHeader("X-Custom", "val1")
		c.AddHeader("X-Custom", "val2")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	values := w.Header().Values("X-Custom")
	if len(values) != 2 {
		t.Errorf("expected 2 X-Custom values, got %d", len(values))
	}
}

func TestContextJSONPretty(t *testing.T) {
	router := NewRouter()
	router.GET("/test", func(c *Context) {
		c.JSONPretty(http.StatusOK, map[string]string{"hello": "world"}, "  ")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
	// Pretty-printed JSON should contain indentation
	if body[0] != '{' {
		t.Errorf("expected JSON object, got %q", body[:10])
	}
}

func TestContextJSONP(t *testing.T) {
	router := NewRouter()
	router.GET("/test", func(c *Context) {
		c.JSONP(http.StatusOK, "callback", map[string]string{"k": "v"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if len(body) < 10 {
		t.Error("expected JSONP response")
	}
}

func TestContextJSONP_InvalidCallback(t *testing.T) {
	router := NewRouter()
	router.GET("/test", func(c *Context) {
		c.JSONP(http.StatusOK, "<script>alert(1)</script>", map[string]string{"k": "v"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid callback", w.Code)
	}
}

func TestDefaultRequestIDGenerator(t *testing.T) {
	id := defaultRequestIDGenerator()
	if id == "" {
		t.Error("expected non-empty ID")
	}
}
