// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package routes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"unheaded/pkg/logger"
	"unheaded/services/gateway/config"
)

func testLogger() *logger.Logger {
	return logger.New(io.Discard)
}

func TestRouter_AddRoute(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	err := router.AddRoute(&config.RouteConfig{
		Name:       "api",
		PathPrefix: "/api/",
		Backends:   []string{"http://localhost:8080"},
	}, &config.CircuitConfig{Enabled: false})
	if err != nil {
		t.Fatalf("AddRoute failed: %v", err)
	}

	routes := router.GetRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
}

func TestRouter_GetRoute(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	router.AddRoute(&config.RouteConfig{
		Name:       "api",
		PathPrefix: "/api/",
		Backends:   []string{"http://localhost:8080"},
	}, &config.CircuitConfig{Enabled: false})

	route := router.GetRoute("api")
	if route == nil {
		t.Fatal("expected route 'api' to exist")
	}

	route = router.GetRoute("nonexistent")
	if route != nil {
		t.Error("expected nil for nonexistent route")
	}
}

func TestRouter_Match(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	router.AddRoute(&config.RouteConfig{
		Name:       "api",
		PathPrefix: "/api/",
		Backends:   []string{"http://localhost:8080"},
	}, &config.CircuitConfig{Enabled: false})

	if route := router.Match("/api/users"); route == nil {
		t.Error("expected match for /api/users")
	}
	if route := router.Match("/other"); route != nil {
		t.Error("expected no match for /other")
	}
}

func TestRouter_MatchLongestPrefix(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	router.AddRoute(&config.RouteConfig{
		Name:       "api",
		PathPrefix: "/api/",
		Backends:   []string{"http://localhost:8080"},
	}, &config.CircuitConfig{Enabled: false})
	router.AddRoute(&config.RouteConfig{
		Name:       "api-v2",
		PathPrefix: "/api/v2/",
		Backends:   []string{"http://localhost:8081"},
	}, &config.CircuitConfig{Enabled: false})

	route := router.Match("/api/v2/users")
	if route == nil || route.Config.Name != "api-v2" {
		t.Error("expected api-v2 to match (longer prefix)")
	}

	route = router.Match("/api/v1/users")
	if route == nil || route.Config.Name != "api" {
		t.Error("expected api to match for /api/v1/users")
	}
}

func TestRouter_RemoveRoute(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	router.AddRoute(&config.RouteConfig{
		Name:       "api",
		PathPrefix: "/api/",
		Backends:   []string{"http://localhost:8080"},
	}, &config.CircuitConfig{Enabled: false})

	removed := router.RemoveRoute("api")
	if !removed {
		t.Error("expected RemoveRoute to return true")
	}

	if router.GetRoute("api") != nil {
		t.Error("expected route to be removed")
	}
	if router.Match("/api/users") != nil {
		t.Error("expected no match after removal")
	}
}

func TestRouter_RemoveRoute_NotFound(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	removed := router.RemoveRoute("nonexistent")
	if removed {
		t.Error("expected false for nonexistent route")
	}
}

func TestRouter_ServeHTTP_NotFound(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "not_found" {
		t.Errorf("expected not_found error, got %s", resp["error"])
	}
}

func TestRouter_ServeHTTP_Proxies(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("proxied"))
	}))
	defer backend.Close()

	router := NewRouter(testLogger(), nil)
	router.AddRoute(&config.RouteConfig{
		Name:       "test",
		PathPrefix: "/api/",
		Backends:   []string{backend.URL},
		Timeout:    5000000000, // 5s
	}, &config.CircuitConfig{Enabled: false})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRouter_Close(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	router.AddRoute(&config.RouteConfig{
		Name:       "api",
		PathPrefix: "/api/",
		Backends:   []string{"http://localhost:8080"},
	}, &config.CircuitConfig{Enabled: false})

	err := router.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestRouter_Info(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	router.AddRoute(&config.RouteConfig{
		Name:         "api",
		PathPrefix:   "/api/",
		Backends:     []string{"http://localhost:8080"},
		LoadBalancer: "round_robin",
	}, &config.CircuitConfig{Enabled: true, FailureThreshold: 5, SuccessThreshold: 2})

	infos := router.Info()
	if len(infos) != 1 {
		t.Fatalf("expected 1 info, got %d", len(infos))
	}

	info := infos[0]
	if info.Name != "api" {
		t.Errorf("expected name api, got %s", info.Name)
	}
	if info.PathPrefix != "/api/" {
		t.Errorf("expected path prefix /api/, got %s", info.PathPrefix)
	}
	if info.CircuitState != "closed" {
		t.Errorf("expected circuit state closed, got %s", info.CircuitState)
	}
	if len(info.HealthyBackends) != 1 {
		t.Errorf("expected 1 healthy backend, got %d", len(info.HealthyBackends))
	}
}

func TestRoutesHandler_GET(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	router.AddRoute(&config.RouteConfig{
		Name:       "api",
		PathPrefix: "/api/",
		Backends:   []string{"http://localhost:8080"},
	}, &config.CircuitConfig{Enabled: false})

	handler := NewRoutesHandler(router)
	req := httptest.NewRequest(http.MethodGet, "/_routes", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var infos []RouteInfo
	json.NewDecoder(rr.Body).Decode(&infos)
	if len(infos) != 1 {
		t.Fatalf("expected 1 route info, got %d", len(infos))
	}
}

func TestRoutesHandler_MethodNotAllowed(t *testing.T) {
	router := NewRouter(testLogger(), nil)
	handler := NewRoutesHandler(router)

	req := httptest.NewRequest(http.MethodPost, "/_routes", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestWriteNotFound(t *testing.T) {
	rr := httptest.NewRecorder()
	writeNotFound(rr)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}
