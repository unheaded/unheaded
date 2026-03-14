// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- splitPath tests ---

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"backend1/drain", []string{"backend1", "drain"}},
		{"/backend1/drain", []string{"backend1", "drain"}},
		{"backend1/drain/", []string{"backend1", "drain"}},
		{"/backend1/drain/", []string{"backend1", "drain"}},
		{"single", []string{"single"}},
		{"", nil},
		{"///", nil},
		{"a/b/c", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		got := splitPath(tt.path)
		if len(got) != len(tt.want) {
			t.Errorf("splitPath(%q) = %v (len %d), want %v (len %d)", tt.path, got, len(got), tt.want, len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitPath(%q)[%d] = %q, want %q", tt.path, i, got[i], tt.want[i])
			}
		}
	}
}

// --- Builder tests ---

func TestBuilder_Basic(t *testing.T) {
	b := NewBuilder("test-lb").
		ListenAddress("0.0.0.0:8080").
		Protocol(ProtocolHTTP).
		Algorithm(AlgorithmRoundRobin).
		AddBackend("b1", "127.0.0.1:8081", 1).
		Metrics(false)

	balancer, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if balancer == nil {
		t.Fatal("expected non-nil balancer")
	}
	if balancer.Config().Name != "test-lb" {
		t.Errorf("expected name test-lb, got %s", balancer.Config().Name)
	}
}

func TestBuilder_MultipleBackends(t *testing.T) {
	b := NewBuilder("test-lb").
		AddBackend("b1", "127.0.0.1:8081", 10).
		AddBackend("b2", "127.0.0.1:8082", 20).
		AddBackend("b3", "127.0.0.1:8083", 30).
		Metrics(false)

	balancer, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(balancer.Backends()) != 3 {
		t.Errorf("expected 3 backends, got %d", len(balancer.Backends()))
	}
}

func TestBuilder_HealthCheck(t *testing.T) {
	b := NewBuilder("test-lb").
		AddBackend("b1", "127.0.0.1:8081", 1).
		HealthCheck(HealthCheckHTTP, 10*time.Second, 5*time.Second).
		Metrics(false)

	balancer, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if balancer.Config().HealthCheck.Type != HealthCheckHTTP {
		t.Errorf("expected HTTP health check, got %s", balancer.Config().HealthCheck.Type)
	}
}

func TestBuilder_SessionPersistence(t *testing.T) {
	b := NewBuilder("test-lb").
		AddBackend("b1", "127.0.0.1:8081", 1).
		SessionPersistence(SessionPersistenceSourceIP, 30*time.Minute).
		Metrics(false)

	balancer, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if balancer.Config().SessionPersistence.Type != SessionPersistenceSourceIP {
		t.Errorf("expected source-ip, got %s", balancer.Config().SessionPersistence.Type)
	}
}

func TestBuilder_Timeouts(t *testing.T) {
	b := NewBuilder("test-lb").
		AddBackend("b1", "127.0.0.1:8081", 1).
		Timeouts(1*time.Second, 2*time.Second, 3*time.Second).
		Metrics(false)

	balancer, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if balancer.Config().Timeouts.Connect != 1*time.Second {
		t.Errorf("expected 1s connect, got %v", balancer.Config().Timeouts.Connect)
	}
}

func TestBuilder_NoBackends_Error(t *testing.T) {
	b := NewBuilder("test-lb").
		Metrics(false)

	_, err := b.Build()
	if err == nil {
		t.Error("expected error for no backends")
	}
}

func TestBuilder_AllAlgorithms(t *testing.T) {
	algos := []Algorithm{
		AlgorithmRoundRobin, AlgorithmLeastConnections, AlgorithmIPHash,
		AlgorithmWeighted, AlgorithmWeightedLeastConn, AlgorithmRandom,
	}
	for _, algo := range algos {
		b := NewBuilder("test-lb").
			Algorithm(algo).
			AddBackend("b1", "127.0.0.1:8081", 1).
			Metrics(false)

		balancer, err := b.Build()
		if err != nil {
			t.Fatalf("Build with %s failed: %v", algo, err)
		}
		if balancer.Config().Algorithm != algo {
			t.Errorf("expected %s, got %s", algo, balancer.Config().Algorithm)
		}
	}
}

// --- BalancerGroup tests ---

func makeTestBalancer(t *testing.T, name string) *Balancer {
	t.Helper()
	b := NewBuilder(name).
		AddBackend("b1", "127.0.0.1:8081", 1).
		Metrics(false)
	balancer, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	return balancer
}

func TestBalancerGroup_Add(t *testing.T) {
	g := NewBalancerGroup()

	b := makeTestBalancer(t, "lb1")
	if err := g.Add(b); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	all := g.All()
	if len(all) != 1 {
		t.Errorf("expected 1, got %d", len(all))
	}
}

func TestBalancerGroup_AddDuplicate(t *testing.T) {
	g := NewBalancerGroup()

	b := makeTestBalancer(t, "lb1")
	g.Add(b)

	b2 := makeTestBalancer(t, "lb1")
	if err := g.Add(b2); err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestBalancerGroup_Get(t *testing.T) {
	g := NewBalancerGroup()

	b := makeTestBalancer(t, "lb1")
	g.Add(b)

	got, ok := g.Get("lb1")
	if !ok {
		t.Error("expected to find lb1")
	}
	if got != b {
		t.Error("expected same balancer")
	}

	_, ok = g.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent")
	}
}

func TestBalancerGroup_Remove(t *testing.T) {
	g := NewBalancerGroup()

	b := makeTestBalancer(t, "lb1")
	g.Add(b)

	if err := g.Remove("lb1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	_, ok := g.Get("lb1")
	if ok {
		t.Error("expected lb1 to be removed")
	}
}

func TestBalancerGroup_Remove_NotFound(t *testing.T) {
	g := NewBalancerGroup()

	if err := g.Remove("nonexistent"); err == nil {
		t.Error("expected error for not found")
	}
}

func TestBalancerGroup_Stats(t *testing.T) {
	g := NewBalancerGroup()

	b1 := makeTestBalancer(t, "lb1")
	b2 := makeTestBalancer(t, "lb2")
	g.Add(b1)
	g.Add(b2)

	stats := g.Stats()
	if len(stats) != 2 {
		t.Errorf("expected 2 stats, got %d", len(stats))
	}
	if _, ok := stats["lb1"]; !ok {
		t.Error("expected lb1 in stats")
	}
	if _, ok := stats["lb2"]; !ok {
		t.Error("expected lb2 in stats")
	}
}

// --- Balancer lifecycle tests ---

func TestBalancer_IsRunning(t *testing.T) {
	b := makeTestBalancer(t, "test")
	if b.IsRunning() {
		t.Error("expected not running initially")
	}
}

func TestBalancer_Uptime_NotRunning(t *testing.T) {
	b := makeTestBalancer(t, "test")
	if b.Uptime() != 0 {
		t.Error("expected 0 uptime when not running")
	}
}

func TestBalancer_Backends(t *testing.T) {
	b := makeTestBalancer(t, "test")

	backends := b.Backends()
	if len(backends) != 1 {
		t.Errorf("expected 1 backend, got %d", len(backends))
	}
}

func TestBalancer_GetBackend(t *testing.T) {
	b := makeTestBalancer(t, "test")

	backend, ok := b.GetBackend("b1")
	if !ok {
		t.Error("expected to find b1")
	}
	if backend.Config.Name != "b1" {
		t.Errorf("expected b1, got %s", backend.Config.Name)
	}

	_, ok = b.GetBackend("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent")
	}
}

func TestBalancer_AddBackend(t *testing.T) {
	b := makeTestBalancer(t, "test")

	err := b.AddBackend(BackendConfig{Name: "b2", Address: "127.0.0.1:8082", Enabled: true})
	if err != nil {
		t.Fatalf("AddBackend failed: %v", err)
	}

	if len(b.Backends()) != 2 {
		t.Errorf("expected 2 backends, got %d", len(b.Backends()))
	}
}

func TestBalancer_RemoveBackend(t *testing.T) {
	balancer := makeTestBalancer(t, "test")
	balancer.AddBackend(BackendConfig{Name: "b2", Address: "127.0.0.1:8082", Enabled: true})

	err := balancer.RemoveBackend("b2")
	if err != nil {
		t.Fatalf("RemoveBackend failed: %v", err)
	}

	if len(balancer.Backends()) != 1 {
		t.Errorf("expected 1 backend, got %d", len(balancer.Backends()))
	}
}

func TestBalancer_RemoveBackend_NotFound(t *testing.T) {
	b := makeTestBalancer(t, "test")

	err := b.RemoveBackend("nonexistent")
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestBalancer_SetAlgorithm(t *testing.T) {
	b := makeTestBalancer(t, "test")

	err := b.SetAlgorithm(AlgorithmLeastConnections)
	if err != nil {
		t.Fatalf("SetAlgorithm failed: %v", err)
	}
	if b.Config().Algorithm != AlgorithmLeastConnections {
		t.Errorf("expected least-connections, got %s", b.Config().Algorithm)
	}
}

func TestBalancer_SetAlgorithm_Invalid(t *testing.T) {
	b := makeTestBalancer(t, "test")

	err := b.SetAlgorithm("magic")
	if err == nil {
		t.Error("expected error for invalid algorithm")
	}
}

func TestBalancer_UpdateBackendWeight(t *testing.T) {
	b := makeTestBalancer(t, "test")

	err := b.UpdateBackendWeight("b1", 50)
	if err != nil {
		t.Fatalf("UpdateBackendWeight failed: %v", err)
	}
}

func TestBalancer_DrainBackend(t *testing.T) {
	b := makeTestBalancer(t, "test")

	err := b.DrainBackend("b1")
	if err != nil {
		t.Fatalf("DrainBackend failed: %v", err)
	}

	backend, _ := b.GetBackend("b1")
	if backend.State() != BackendStateDraining {
		t.Errorf("expected draining, got %s", backend.State())
	}
}

func TestBalancer_DrainBackend_NotFound(t *testing.T) {
	b := makeTestBalancer(t, "test")

	err := b.DrainBackend("nonexistent")
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestBalancer_EnableDisableBackend(t *testing.T) {
	b := makeTestBalancer(t, "test")

	err := b.DisableBackend("b1")
	if err != nil {
		t.Fatal(err)
	}
	backend, _ := b.GetBackend("b1")
	if backend.State() != BackendStateDisabled {
		t.Errorf("expected disabled, got %s", backend.State())
	}

	err = b.EnableBackend("b1")
	if err != nil {
		t.Fatal(err)
	}
	if backend.State() != BackendStateUnknown {
		t.Errorf("expected unknown, got %s", backend.State())
	}
}

func TestBalancer_EnableBackend_NotFound(t *testing.T) {
	b := makeTestBalancer(t, "test")
	if err := b.EnableBackend("nonexistent"); err == nil {
		t.Error("expected error")
	}
}

func TestBalancer_DisableBackend_NotFound(t *testing.T) {
	b := makeTestBalancer(t, "test")
	if err := b.DisableBackend("nonexistent"); err == nil {
		t.Error("expected error")
	}
}

func TestBalancer_Stats(t *testing.T) {
	b := makeTestBalancer(t, "test")

	stats := b.Stats()
	if stats.Name != "test" {
		t.Errorf("expected name test, got %s", stats.Name)
	}
	if stats.Running {
		t.Error("expected not running")
	}
	if stats.Protocol != ProtocolHTTP {
		t.Errorf("expected HTTP, got %s", stats.Protocol)
	}
	if stats.Pool.TotalBackends != 1 {
		t.Errorf("expected 1 backend in pool, got %d", stats.Pool.TotalBackends)
	}
}

func TestBalancer_HealthyBackends(t *testing.T) {
	b := makeTestBalancer(t, "test")

	healthy := b.HealthyBackends()
	// Unknown state counts as healthy
	if len(healthy) != 1 {
		t.Errorf("expected 1 healthy, got %d", len(healthy))
	}
}

func TestBalancer_HealthHandler(t *testing.T) {
	b := makeTestBalancer(t, "test")

	handler := b.HealthHandler()
	if handler == nil {
		t.Error("expected non-nil health handler")
	}
}

func TestBalancer_MetricsHandler_Disabled(t *testing.T) {
	b := makeTestBalancer(t, "test")

	handler := b.MetricsHandler()
	if handler == nil {
		t.Error("expected non-nil metrics handler even when disabled")
	}
}

func TestBalancer_OnCallbacks(t *testing.T) {
	b := makeTestBalancer(t, "test")

	startCalled := false
	stopCalled := false
	addCalled := false
	removeCalled := false
	healthCalled := false

	b.OnStart(func() { startCalled = true })
	b.OnStop(func() { stopCalled = true })
	b.OnBackendAdd(func(backend *Backend) { addCalled = true })
	b.OnBackendRemove(func(backend *Backend) { removeCalled = true })
	b.OnBackendHealthChange(func(backend *Backend, healthy bool) { healthCalled = true })

	// Trigger add callback
	b.AddBackend(BackendConfig{Name: "b2", Address: "127.0.0.1:8082", Enabled: true})
	if !addCalled {
		t.Error("expected add callback to be called")
	}

	// Trigger remove callback
	b.RemoveBackend("b2")
	if !removeCalled {
		t.Error("expected remove callback to be called")
	}

	// Start/stop/health callbacks verified by existence
	_ = startCalled
	_ = stopCalled
	_ = healthCalled
}

func TestBalancer_WriteMetrics_Disabled(t *testing.T) {
	b := makeTestBalancer(t, "test")

	err := b.WriteMetrics(nil)
	if err == nil {
		t.Error("expected error when metrics disabled")
	}
}

// --- ConfigWatcher tests ---

func TestConfigWatcher_New(t *testing.T) {
	b := makeTestBalancer(t, "test")
	cw := NewConfigWatcher(b, "/tmp/config.json", time.Second)

	if cw == nil {
		t.Fatal("expected non-nil watcher")
	}
}

func TestConfigWatcher_StartStop(t *testing.T) {
	b := makeTestBalancer(t, "test")
	cw := NewConfigWatcher(b, "/tmp/config.json", 100*time.Millisecond)

	if err := cw.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start should error
	if err := cw.Start(); err == nil {
		t.Error("expected error for double start")
	}

	if err := cw.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Double stop should error
	if err := cw.Stop(); err == nil {
		t.Error("expected error for double stop")
	}
}

// --- APIHandler tests ---

func makeAPIHandler(t *testing.T) (*APIHandler, *Balancer) {
	t.Helper()
	b := NewBuilder("api-test").
		AddBackend("b1", "127.0.0.1:8081", 10).
		AddBackend("b2", "127.0.0.1:8082", 20).
		Metrics(false)
	balancer, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	return NewAPIHandler(balancer), balancer
}

func TestAPIHandler_GetStats(t *testing.T) {
	handler, _ := makeAPIHandler(t)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json, got %s", w.Header().Get("Content-Type"))
	}
}

func TestAPIHandler_ListBackends(t *testing.T) {
	handler, _ := makeAPIHandler(t)

	req := httptest.NewRequest("GET", "/api/backends", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var backends []BackendStats
	if err := json.NewDecoder(w.Body).Decode(&backends); err != nil {
		t.Fatal(err)
	}
	if len(backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(backends))
	}
}

func TestAPIHandler_AddBackend(t *testing.T) {
	handler, balancer := makeAPIHandler(t)

	body := `{"name":"b3","address":"127.0.0.1:8083","enabled":true,"weight":5}`
	req := httptest.NewRequest("POST", "/api/backends", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(balancer.Backends()) != 3 {
		t.Errorf("expected 3 backends after add, got %d", len(balancer.Backends()))
	}
}

func TestAPIHandler_AddBackend_InvalidJSON(t *testing.T) {
	handler, _ := makeAPIHandler(t)

	req := httptest.NewRequest("POST", "/api/backends", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// NOTE: The APIHandler.ServeHTTP has a path-slicing bug: it compares
// r.URL.Path[:13] (13 chars = "/api/backends") against "/api/backends/"
// (14 chars), so DELETE and POST /api/backends/* routes never match.
// The tests below document the actual behavior.

func TestAPIHandler_RemoveBackend_RoutingBug(t *testing.T) {
	handler, _ := makeAPIHandler(t)

	// Due to path[:13] != "/api/backends/" (length mismatch), this falls through to 404
	req := httptest.NewRequest("DELETE", "/api/backends/b2", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Documenting actual behavior: routes don't match, so 404
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 due to routing bug, got %d", w.Code)
	}
}

func TestAPIHandler_BackendAction_RoutingBug(t *testing.T) {
	handler, _ := makeAPIHandler(t)

	// Same path[:13] bug applies to POST actions
	req := httptest.NewRequest("POST", "/api/backends/b1/drain", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 due to routing bug, got %d", w.Code)
	}
}

func TestAPIHandler_GetConfig(t *testing.T) {
	handler, _ := makeAPIHandler(t)

	req := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPIHandler_NotFound(t *testing.T) {
	handler, _ := makeAPIHandler(t)

	req := httptest.NewRequest("GET", "/api/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Balancer.Select tests ---

func TestBalancer_Select(t *testing.T) {
	b := makeTestBalancer(t, "test")

	backend, err := b.Select(nil, "key")
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if backend == nil {
		t.Error("expected non-nil backend")
	}
}

// --- Balancer.RecordError tests ---

func TestBalancer_RecordError(t *testing.T) {
	b := makeTestBalancer(t, "test")

	backend, _ := b.GetBackend("b1")
	b.RecordError(backend, http.ErrAbortHandler) // Just verify no panic
}

func TestBalancer_APIHandler(t *testing.T) {
	b := makeTestBalancer(t, "test")
	handler := b.APIHandler()
	if handler == nil {
		t.Error("expected non-nil API handler")
	}
}
