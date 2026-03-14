// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

func newTestSvc() *XDPSync {
	return NewXDPSync(logger.New(io.Discard), nil)
}

// ---------------------------------------------------------------------------
// HTTP handler tests
// ---------------------------------------------------------------------------

func TestHandleBackends_GET_Empty(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backends", nil)
	rr := httptest.NewRecorder()
	svc.handleBackends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected 0 total, got %v", resp["total"])
	}
}

func TestHandleBackends_POST_Success(t *testing.T) {
	svc := newTestSvc()
	body := `{"id":"b1","address":"10.0.0.1","port":8080,"weight":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backends", strings.NewReader(body))
	rr := httptest.NewRecorder()
	svc.handleBackends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify backend was added.
	b, ok := svc.GetBackend("b1")
	if !ok || b == nil {
		t.Fatal("expected backend b1 to exist")
	}
	if b.Weight != 2 {
		t.Errorf("expected weight 2, got %d", b.Weight)
	}
}

func TestHandleBackends_POST_Defaults(t *testing.T) {
	svc := newTestSvc()
	// Weight <= 0 should default to 1, Health empty should default to HEALTHY.
	body := `{"id":"b2","address":"10.0.0.2","port":8080,"weight":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backends", strings.NewReader(body))
	rr := httptest.NewRecorder()
	svc.handleBackends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	b, _ := svc.GetBackend("b2")
	if b.Weight != 1 {
		t.Errorf("expected default weight 1, got %d", b.Weight)
	}
	if b.Health != HealthHealthy {
		t.Errorf("expected default health HEALTHY, got %s", b.Health)
	}
}

func TestHandleBackends_POST_MissingID(t *testing.T) {
	svc := newTestSvc()
	body := `{"address":"10.0.0.1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backends", strings.NewReader(body))
	rr := httptest.NewRecorder()
	svc.handleBackends(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleBackends_POST_MissingAddress(t *testing.T) {
	svc := newTestSvc()
	body := `{"id":"b1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backends", strings.NewReader(body))
	rr := httptest.NewRecorder()
	svc.handleBackends(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleBackends_POST_InvalidJSON(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backends", strings.NewReader("{bad"))
	rr := httptest.NewRecorder()
	svc.handleBackends(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleBackends_MethodNotAllowed(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/backends", nil)
	rr := httptest.NewRecorder()
	svc.handleBackends(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleBackends_GET_SortedList(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(&Backend{ID: "c", Address: "10.0.0.3", Port: 80, Weight: 1, Health: HealthHealthy})
	svc.AddBackend(&Backend{ID: "a", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy})
	svc.AddBackend(&Backend{ID: "b", Address: "10.0.0.2", Port: 80, Weight: 1, Health: HealthHealthy})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backends", nil)
	rr := httptest.NewRecorder()
	svc.handleBackends(rr, req)

	var resp struct {
		Backends []Backend `json:"backends"`
		Total    int       `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Total != 3 {
		t.Fatalf("expected 3 backends, got %d", resp.Total)
	}
	if resp.Backends[0].ID != "a" || resp.Backends[1].ID != "b" || resp.Backends[2].ID != "c" {
		t.Error("expected backends sorted by ID")
	}
}

// ---------------------------------------------------------------------------
// handleBackendByID tests
// ---------------------------------------------------------------------------

func TestHandleBackendByID_GET(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backends/b1", nil)
	rr := httptest.NewRecorder()
	svc.handleBackendByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var b Backend
	json.NewDecoder(rr.Body).Decode(&b)
	if b.ID != "b1" {
		t.Errorf("expected b1, got %s", b.ID)
	}
}

func TestHandleBackendByID_GET_NotFound(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backends/nonexistent", nil)
	rr := httptest.NewRecorder()
	svc.handleBackendByID(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleBackendByID_DELETE(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/backends/b1", nil)
	rr := httptest.NewRecorder()
	svc.handleBackendByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	_, ok := svc.GetBackend("b1")
	if ok {
		t.Error("expected b1 to be deleted")
	}
}

func TestHandleBackendByID_DELETE_NotFound(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/backends/nope", nil)
	rr := httptest.NewRecorder()
	svc.handleBackendByID(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleBackendByID_EmptyID(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backends/", nil)
	rr := httptest.NewRecorder()
	svc.handleBackendByID(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty ID, got %d", rr.Code)
	}
}

func TestHandleBackendByID_MethodNotAllowed(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backends/b1", nil)
	rr := httptest.NewRecorder()
	svc.handleBackendByID(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// handleDistribution tests
// ---------------------------------------------------------------------------

func TestHandleDistribution_Empty(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution", nil)
	rr := httptest.NewRecorder()
	svc.handleDistribution(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if int(resp["table_size"].(float64)) != 0 {
		t.Errorf("expected table_size 0, got %v", resp["table_size"])
	}
}

func TestHandleDistribution_WithBackends(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(&Backend{ID: "a", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy})
	svc.AddBackend(&Backend{ID: "b", Address: "10.0.0.2", Port: 80, Weight: 1, Health: HealthHealthy})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/distribution", nil)
	rr := httptest.NewRecorder()
	svc.handleDistribution(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Distribution []Distribution `json:"distribution"`
		TableSize    int            `json:"table_size"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.TableSize == 0 {
		t.Error("expected non-zero table size")
	}
	if len(resp.Distribution) != 2 {
		t.Errorf("expected 2 entries in distribution, got %d", len(resp.Distribution))
	}
}

func TestHandleDistribution_MethodNotAllowed(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distribution", nil)
	rr := httptest.NewRecorder()
	svc.handleDistribution(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// handleRebalance tests
// ---------------------------------------------------------------------------

func TestHandleRebalance_Success(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rebalance", nil)
	rr := httptest.NewRecorder()
	svc.handleRebalance(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "rebalanced" {
		t.Errorf("expected rebalanced status, got %s", resp["status"])
	}
}

func TestHandleRebalance_MethodNotAllowed(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rebalance", nil)
	rr := httptest.NewRecorder()
	svc.handleRebalance(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// handleHealth, handleReady, handleMetrics tests
// ---------------------------------------------------------------------------

func TestHandleHealth(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	svc.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestHandleReady(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	svc.handleReady(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleMetrics_NoBackends(t *testing.T) {
	svc := newTestSvc()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	svc.handleMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "unheaded_lb_backends_total 0") {
		t.Error("expected backends_total 0 in metrics")
	}
	if !strings.Contains(body, "unheaded_lb_maglev_table_size 0") {
		t.Error("expected table_size 0 in metrics")
	}
	if !strings.Contains(body, "unheaded_lb_wotan_drops_total") {
		t.Error("expected wotan_drops_total in metrics")
	}
}

func TestHandleMetrics_WithBackends(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy})
	svc.AddBackend(&Backend{ID: "b2", Address: "10.0.0.2", Port: 80, Weight: 1, Health: HealthUnhealthy})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	svc.handleMetrics(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "unheaded_lb_backends_total 2") {
		t.Error("expected backends_total 2")
	}
	if !strings.Contains(body, "unheaded_lb_backends_healthy 1") {
		t.Error("expected backends_healthy 1")
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain content type, got %s", ct)
	}
}

// ---------------------------------------------------------------------------
// Start / Stop / Addr lifecycle tests
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	svc := newTestSvc()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	addr := svc.Addr()
	if addr == "" {
		t.Fatal("expected non-empty addr after Start")
	}

	// Hit the health endpoint via the live server.
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestAddr_BeforeStart(t *testing.T) {
	svc := newTestSvc()
	if addr := svc.Addr(); addr != "" {
		t.Errorf("expected empty addr before Start, got %s", addr)
	}
}

func TestWotanDrops(t *testing.T) {
	svc := newTestSvc()
	// With nil wotan, rebuilding table should increment drops.
	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy})
	drops := svc.WotanDrops()
	if drops < 1 {
		t.Errorf("expected at least 1 wotan drop (nil client), got %d", drops)
	}
}

// ---------------------------------------------------------------------------
// AddBackend edge cases
// ---------------------------------------------------------------------------

func TestAddBackend_Nil(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(nil) // should not panic
}

func TestAddBackend_EmptyID(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(&Backend{ID: "", Address: "10.0.0.1"}) // should be a no-op
	table := svc.GetTable()
	if len(table) != 0 {
		t.Error("expected empty table after adding backend with empty ID")
	}
}

// ---------------------------------------------------------------------------
// SetBackendHealth same-state
// ---------------------------------------------------------------------------

func TestSetBackendHealth_SameState(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy})

	// Set to same state should not trigger rebuild but should return true.
	ok := svc.SetBackendHealth("b1", HealthHealthy)
	if !ok {
		t.Error("expected true for existing backend")
	}
}

// ---------------------------------------------------------------------------
// pollHealth test
// ---------------------------------------------------------------------------

func TestPollHealth(t *testing.T) {
	svc := newTestSvc()
	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy})

	before, _ := svc.GetBackend("b1")
	firstCheck := before.LastCheck

	// Small delay so timestamps differ.
	time.Sleep(2 * time.Millisecond)

	svc.pollHealth()

	after, _ := svc.GetBackend("b1")
	if !after.LastCheck.After(firstCheck) {
		t.Error("expected LastCheck to be updated after pollHealth")
	}
}

// ---------------------------------------------------------------------------
// handleCriticalAlert test
// ---------------------------------------------------------------------------

func TestHandleCriticalAlert_Nil(t *testing.T) {
	svc := newTestSvc()
	// Should not panic with nil message.
	svc.handleCriticalAlert(nil)
}

// ---------------------------------------------------------------------------
// SyncToBPF test
// ---------------------------------------------------------------------------

func TestSyncToBPF(t *testing.T) {
	err := SyncToBPF([]uint32{0, 1, 2})
	if err != nil {
		t.Errorf("expected nil error from stub, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// writeJSON test
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"key": "val"})

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["key"] != "val" {
		t.Errorf("expected val, got %s", resp["key"])
	}
}

// ---------------------------------------------------------------------------
// isPrime / nextPrime tests
// ---------------------------------------------------------------------------

func TestIsPrime(t *testing.T) {
	tests := []struct {
		n    int
		want bool
	}{
		{0, false},
		{1, false},
		{2, true},
		{3, true},
		{4, false},
		{5, true},
		{9, false},
		{11, true},
		{25, false},
		{97, true},
		{100, false},
	}
	for _, tt := range tests {
		got := isPrime(tt.n)
		if got != tt.want {
			t.Errorf("isPrime(%d) = %v, want %v", tt.n, got, tt.want)
		}
	}
}

func TestNextPrime(t *testing.T) {
	tests := []struct {
		n    int
		want int
	}{
		{0, 2},
		{1, 2},
		{2, 2},
		{3, 3},
		{4, 5},
		{10, 11},
		{14, 17},
		{97, 97},
		{98, 101},
	}
	for _, tt := range tests {
		got := nextPrime(tt.n)
		if got != tt.want {
			t.Errorf("nextPrime(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// BuildTable edge cases
// ---------------------------------------------------------------------------

func TestBuildTable_ZeroSize(t *testing.T) {
	table := BuildTable([]Backend{{ID: "a", Weight: 1}}, 0)
	if len(table) != 0 {
		t.Errorf("expected empty table for size 0, got len %d", len(table))
	}
}

func TestBuildTable_NegativeSize(t *testing.T) {
	table := BuildTable([]Backend{{ID: "a", Weight: 1}}, -1)
	if len(table) != 0 {
		t.Errorf("expected empty table for negative size, got len %d", len(table))
	}
}

func TestBuildTable_WeightedBackends(t *testing.T) {
	backends := []Backend{
		{ID: "heavy", Address: "10.0.0.1", Port: 80, Weight: 5, Health: HealthHealthy},
		{ID: "light", Address: "10.0.0.2", Port: 80, Weight: 1, Health: HealthHealthy},
	}
	table := BuildTable(backends, 997)
	if len(table) == 0 {
		t.Fatal("expected non-empty table")
	}

	counts := countBackends(table)
	// heavy (index 0) should have more slots than light (index 1).
	if counts[0] <= counts[1] {
		t.Errorf("expected heavy backend (idx 0) to have more slots: heavy=%d, light=%d", counts[0], counts[1])
	}
}

// ---------------------------------------------------------------------------
// Full HTTP integration via live server
// ---------------------------------------------------------------------------

func TestHTTPIntegration(t *testing.T) {
	svc := newTestSvc()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer svc.Stop()

	base := "http://" + svc.Addr()

	// POST a backend.
	body := bytes.NewBufferString(`{"id":"web1","address":"10.0.0.1","port":8080,"weight":1}`)
	resp, err := http.Post(base+"/api/v1/backends", "application/json", body)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST backend: expected 200, got %d", resp.StatusCode)
	}

	// GET backends.
	resp, err = http.Get(base + "/api/v1/backends")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET backends: expected 200, got %d", resp.StatusCode)
	}

	// GET specific backend.
	resp, err = http.Get(base + "/api/v1/backends/web1")
	if err != nil {
		t.Fatalf("GET backend failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET backend/web1: expected 200, got %d", resp.StatusCode)
	}

	// GET distribution.
	resp, err = http.Get(base + "/api/v1/distribution")
	if err != nil {
		t.Fatalf("GET distribution failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET distribution: expected 200, got %d", resp.StatusCode)
	}

	// POST rebalance.
	resp, err = http.Post(base+"/api/v1/rebalance", "", nil)
	if err != nil {
		t.Fatalf("POST rebalance failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST rebalance: expected 200, got %d", resp.StatusCode)
	}

	// GET metrics.
	resp, err = http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET metrics failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET metrics: expected 200, got %d", resp.StatusCode)
	}

	// DELETE backend.
	delReq, _ := http.NewRequest(http.MethodDelete, base+"/api/v1/backends/web1", nil)
	resp, err = http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("DELETE backend: expected 200, got %d", resp.StatusCode)
	}
}
