// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouteGroup_POST(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.POST("/items", func(c *Context) {
		c.String(http.StatusCreated, "created")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("POST status = %d, want 201", w.Code)
	}
}

func TestRouteGroup_PUT(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.PUT("/items/:id", func(c *Context) {
		c.String(http.StatusOK, "updated")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/items/1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("PUT status = %d, want 200", w.Code)
	}
}

func TestRouteGroup_DELETE(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.DELETE("/items/:id", func(c *Context) {
		c.String(http.StatusNoContent, "")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/items/1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("DELETE status = %d, want 204", w.Code)
	}
}

func TestRouteGroup_PATCH(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.PATCH("/items/:id", func(c *Context) {
		c.String(http.StatusOK, "patched")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/items/1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("PATCH status = %d, want 200", w.Code)
	}
}

func TestRouteGroup_HEAD(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.HEAD("/ping", func(c *Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/api/ping", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("HEAD status = %d, want 200", w.Code)
	}
}

func TestRouteGroup_OPTIONS(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.OPTIONS("/items", func(c *Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/items", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", w.Code)
	}
}

func TestRouteGroup_Any(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.Any("/universal", func(c *Context) {
		c.String(http.StatusOK, "%s", c.Method())
	})

	methods := []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodDelete, http.MethodPatch, http.MethodHead,
		http.MethodOptions,
	}
	for _, method := range methods {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/universal", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", method, w.Code)
		}
	}
}

func TestRouteGroup_Match(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	g.Match([]string{http.MethodGet, http.MethodPost}, "/multi", func(c *Context) {
		c.String(http.StatusOK, "matched")
	})

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/multi", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", method, w.Code)
		}
	}
}

func TestRouteGroup_Use(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")

	called := false
	g.Use(func(c *Context) {
		called = true
		c.Next()
	})
	g.GET("/test", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.ServeHTTP(w, req)
	if !called {
		t.Error("middleware not called")
	}
}

func TestRouteGroup_Route(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")

	g.Route(http.MethodGet, "/fluent").Handler(func(c *Context) {
		c.String(http.StatusOK, "fluent")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/fluent", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRouter_Route(t *testing.T) {
	r := NewRouter()
	r.Route(http.MethodGet, "/direct").Handler(func(c *Context) {
		c.String(http.StatusOK, "direct")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/direct", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRouteBuilder_Use(t *testing.T) {
	r := NewRouter()
	called := false
	r.Route(http.MethodGet, "/with-mw").
		Use(func(c *Context) {
			called = true
			c.Next()
		}).
		Handler(func(c *Context) {
			c.String(http.StatusOK, "ok")
		})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/with-mw", nil)
	r.ServeHTTP(w, req)
	if !called {
		t.Error("middleware not called via RouteBuilder.Use")
	}
}

// testResource implements the Resource interface for testing
type testResource struct {
	lastAction string
}

func (tr *testResource) Index(c *Context)  { tr.lastAction = "index"; c.String(200, "index") }
func (tr *testResource) Show(c *Context)   { tr.lastAction = "show"; c.String(200, "show") }
func (tr *testResource) Create(c *Context) { tr.lastAction = "create"; c.String(201, "create") }
func (tr *testResource) Update(c *Context) { tr.lastAction = "update"; c.String(200, "update") }
func (tr *testResource) Delete(c *Context) { tr.lastAction = "delete"; c.String(200, "delete") }

func TestRouter_Resource(t *testing.T) {
	r := NewRouter()
	res := &testResource{}
	r.Resource("/items", res)

	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/items", 200},
		{http.MethodGet, "/items/1", 200},
		{http.MethodPost, "/items", 201},
		{http.MethodPut, "/items/1", 200},
		{http.MethodPatch, "/items/1", 200},
		{http.MethodDelete, "/items/1", 200},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tt.method, tt.path, nil)
		r.ServeHTTP(w, req)
		if w.Code != tt.want {
			t.Errorf("%s %s: status = %d, want %d", tt.method, tt.path, w.Code, tt.want)
		}
	}
}

func TestRouteGroup_Resource(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")
	res := &testResource{}
	g.Resource("/items", res)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPartialResource_NilHandlers(t *testing.T) {
	pr := &PartialResource{}

	r := NewRouter()
	r.Resource("/partial", pr)

	// All methods should return 404 (NotFound) since handlers are nil
	methods := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/partial"},
		{http.MethodGet, "/partial/1"},
		{http.MethodPost, "/partial"},
		{http.MethodPut, "/partial/1"},
		{http.MethodDelete, "/partial/1"},
	}
	for _, tt := range methods {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tt.method, tt.path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", tt.method, tt.path, w.Code)
		}
	}
}

func TestPartialResource_WithHandlers(t *testing.T) {
	pr := &PartialResource{
		IndexHandler:  func(c *Context) { c.String(200, "index") },
		ShowHandler:   func(c *Context) { c.String(200, "show") },
		CreateHandler: func(c *Context) { c.String(201, "created") },
		UpdateHandler: func(c *Context) { c.String(200, "updated") },
		DeleteHandler: func(c *Context) { c.String(200, "deleted") },
	}

	r := NewRouter()
	r.Resource("/full", pr)

	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/full", 200},
		{http.MethodGet, "/full/1", 200},
		{http.MethodPost, "/full", 201},
		{http.MethodPut, "/full/1", 200},
		{http.MethodDelete, "/full/1", 200},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tt.method, tt.path, nil)
		r.ServeHTTP(w, req)
		if w.Code != tt.want {
			t.Errorf("%s %s: status = %d, want %d", tt.method, tt.path, w.Code, tt.want)
		}
	}
}

func TestHandlersChain_Last(t *testing.T) {
	handler1 := HandlerFunc(func(c *Context) {})
	handler2 := HandlerFunc(func(c *Context) {})

	chain := HandlersChain{handler1, handler2}
	if chain.Last() == nil {
		t.Error("Last should not be nil")
	}

	empty := HandlersChain{}
	if empty.Last() != nil {
		t.Error("Last on empty chain should be nil")
	}
}

func TestJoinPaths(t *testing.T) {
	tests := []struct {
		abs, rel, want string
	}{
		{"/api", "/users", "/api/users"},
		{"/api/", "/users", "/api/users"},
		{"/api", "users", "/api/users"},
		{"/api", "", "/api"},
		{"", "/users", "/users"},
	}
	for _, tt := range tests {
		got := joinPaths(tt.abs, tt.rel)
		if got != tt.want {
			t.Errorf("joinPaths(%q, %q) = %q, want %q", tt.abs, tt.rel, got, tt.want)
		}
	}
}

func TestContext_RemoteAddr(t *testing.T) {
	r := NewRouter()
	var addr string
	r.GET("/test", func(c *Context) {
		addr = c.RemoteAddr()
		c.Status(200)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	r.ServeHTTP(w, req)
	if addr != "192.168.1.1:12345" {
		t.Errorf("RemoteAddr = %q", addr)
	}
}

func TestContext_SetParam(t *testing.T) {
	ctx := &Context{
		params: make(map[string]string),
	}
	ctx.setParam("id", "42")
	if ctx.Param("id") != "42" {
		t.Errorf("Param(id) = %q, want 42", ctx.Param("id"))
	}
}

func TestContext_SetHandlers(t *testing.T) {
	ctx := &Context{}
	handlers := []HandlerFunc{
		func(c *Context) {},
		func(c *Context) {},
	}
	ctx.setHandlers(handlers)
	if len(ctx.handlers) != 2 {
		t.Errorf("handlers length = %d, want 2", len(ctx.handlers))
	}
}
