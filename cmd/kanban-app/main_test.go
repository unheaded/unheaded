// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// TEST HELPERS
// ============================================================================

func createTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := Config{
		Port:            "8080",
		TimeGuruAddr:    "localhost:9091",
		WotanAddr:      "localhost:9090",
		DataDir:         t.TempDir(),
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
	s := NewServer(cfg)
	t.Cleanup(func() {
		if s.store != nil {
			s.store.Close()
		}
	})
	return s
}

// ============================================================================
// UNIT TESTS - GET /api/v1/timeline/tasks
// ============================================================================

func TestHandleGetTasks_HappyPath(t *testing.T) {
	server := createTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/tasks", nil)
	w := httptest.NewRecorder()

	server.handleGetTasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	tasks, ok := response["tasks"].([]interface{})
	if !ok {
		t.Fatal("expected tasks array in response")
	}

	if len(tasks) == 0 {
		t.Error("expected non-empty tasks array")
	}

	count, ok := response["count"].(float64)
	if !ok {
		t.Fatal("expected count in response")
	}

	if int(count) != len(tasks) {
		t.Errorf("count mismatch: got %d, want %d", int(count), len(tasks))
	}
}

func TestHandleGetTasks_MethodNotAllowed(t *testing.T) {
	server := createTestServer(t)

	// Note: POST, PUT, DELETE are valid methods for handleTasks router
	// Only PATCH should return 405 Method Not Allowed
	methods := []string{http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/timeline/tasks", nil)
			w := httptest.NewRecorder()

			// Use handleTasks (the router) instead of handleGetTasks
			server.handleTasks(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status 405 for %s, got %d", method, w.Code)
			}
		})
	}
}

func TestHandleGetTasks_NilServer_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with nil server")
		}
	}()

	var server *Server
	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/tasks", nil)
	w := httptest.NewRecorder()

	server.handleGetTasks(w, req)
}

// ============================================================================
// UNIT TESTS - GET /api/v1/health
// ============================================================================

func TestHandleHealth_HappyPath(t *testing.T) {
	server := createTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	status, ok := response["status"].(string)
	if !ok || status != "healthy" {
		t.Errorf("expected status 'healthy', got %v", status)
	}

	if _, ok := response["timestamp"].(string); !ok {
		t.Error("expected timestamp in response")
	}

	if _, ok := response["version"].(string); !ok {
		t.Error("expected version in response")
	}
}

// ============================================================================
// UNIT TESTS - Server Lifecycle
// ============================================================================

func TestNewServer_ValidConfig(t *testing.T) {
	cfg := Config{
		Port:            "8080",
		TimeGuruAddr:    "localhost:9091",
		WotanAddr:      "localhost:9090",
		DataDir:         t.TempDir(),
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	server := NewServer(cfg)
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	if server == nil {
		t.Fatal("expected non-nil server")
	}

	if server.config.Port != cfg.Port {
		t.Errorf("expected port %s, got %s", cfg.Port, server.config.Port)
	}

	if server.sseClients == nil {
		t.Error("expected initialized sseClients map")
	}

	// Tasks are now in SQLite Store (L1), not in-memory slice
	if server.store == nil {
		t.Error("expected store to be initialized")
	} else {
		count, err := server.store.TaskCount()
		if err != nil {
			t.Fatalf("failed to get task count: %v", err)
		}
		if count == 0 {
			t.Error("expected initial tasks to be seeded in store")
		}
	}
}

func TestServer_Shutdown_CleansUpSSEClients(t *testing.T) {
	server := createTestServer(t)

	// Add fake SSE clients
	server.sseMu.Lock()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)
	server.sseClients[ch1] = true
	server.sseClients[ch2] = true
	server.sseMu.Unlock()

	// Create a minimal HTTP server for shutdown test
	server.httpServer = &http.Server{
		Addr:    ":0", // Random port
		Handler: http.NewServeMux(),
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// Verify channels are closed
	server.sseMu.RLock()
	defer server.sseMu.RUnlock()

	if len(server.sseClients) != 0 {
		t.Errorf("expected 0 SSE clients after shutdown, got %d", len(server.sseClients))
	}
}

// ============================================================================
// UNIT TESTS - Broadcast Updates
// ============================================================================

func TestBroadcastUpdate_ValidData(t *testing.T) {
	server := createTestServer(t)

	// Create client channel
	clientCh := make(chan []byte, 10)
	server.sseMu.Lock()
	server.sseClients[clientCh] = true
	server.sseMu.Unlock()

	// Broadcast
	testData := map[string]string{"test": "data"}
	server.broadcastUpdate("test-event", testData)

	// Verify message received
	select {
	case msg := <-clientCh:
		var decoded map[string]interface{}
		if err := json.Unmarshal(msg, &decoded); err != nil {
			t.Fatalf("failed to unmarshal broadcast message: %v", err)
		}

		eventType, ok := decoded["type"].(string)
		if !ok || eventType != "test-event" {
			t.Errorf("expected event type 'test-event', got %v", eventType)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for broadcast message")
	}

	// Cleanup
	close(clientCh)
}

func TestBroadcastUpdate_FullChannel_SkipsMessage(t *testing.T) {
	server := createTestServer(t)

	// Create channel with capacity 1 and fill it
	clientCh := make(chan []byte, 1)
	clientCh <- []byte("blocked")

	server.sseMu.Lock()
	server.sseClients[clientCh] = true
	server.sseMu.Unlock()

	// Try to broadcast - should skip without blocking
	done := make(chan bool, 1)
	go func() {
		server.broadcastUpdate("test", map[string]string{"data": "value"})
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(100 * time.Millisecond):
		t.Error("broadcast blocked on full channel")
	}

	close(clientCh)
}

// ============================================================================
// UNIT TESTS - getEnv Helper
// ============================================================================

func TestGetEnv_ValueExists(t *testing.T) {
	key := "TEST_KANBAN_VAR"
	expected := "test-value"
	t.Setenv(key, expected)

	result := getEnv(key, "fallback")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestGetEnv_ValueMissing_ReturnsFallback(t *testing.T) {
	key := "NONEXISTENT_KANBAN_VAR"
	fallback := "fallback-value"

	result := getEnv(key, fallback)
	if result != fallback {
		t.Errorf("expected fallback %s, got %s", fallback, result)
	}
}

func TestGetEnv_EmptyValue_ReturnsEmpty(t *testing.T) {
	key := "EMPTY_KANBAN_VAR"
	t.Setenv(key, "")

	result := getEnv(key, "fallback")
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestServer_ConcurrentGetTasks(t *testing.T) {
	server := createTestServer(t)

	const numRequests = 100
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/tasks", nil)
			w := httptest.NewRecorder()
			server.handleGetTasks(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("concurrent request failed with status %d", w.Code)
			}
			done <- true
		}()
	}

	// Wait for all requests
	for i := 0; i < numRequests; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent requests")
		}
	}
}

func TestServer_ConcurrentBroadcast(t *testing.T) {
	server := createTestServer(t)

	// Create multiple client channels
	const numClients = 50
	clients := make([]chan []byte, numClients)
	for i := 0; i < numClients; i++ {
		ch := make(chan []byte, 10)
		clients[i] = ch
		server.sseMu.Lock()
		server.sseClients[ch] = true
		server.sseMu.Unlock()
	}

	// Concurrent broadcasts
	const numBroadcasts = 100
	done := make(chan bool, numBroadcasts)

	for i := 0; i < numBroadcasts; i++ {
		go func(n int) {
			server.broadcastUpdate("test", map[string]int{"iteration": n})
			done <- true
		}(i)
	}

	// Wait for broadcasts
	for i := 0; i < numBroadcasts; i++ {
		<-done
	}

	// Cleanup
	for _, ch := range clients {
		close(ch)
	}
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func TestHandleGetTasks_EmptyTasksList(t *testing.T) {
	// Create a server with NO store and NO tasks to test the empty fallback
	server := &Server{
		config:     Config{Port: "0"},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/tasks", nil)
	w := httptest.NewRecorder()

	server.handleGetTasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for empty tasks, got %d", w.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	count := int(response["count"].(float64))
	if count != 0 {
		t.Errorf("expected count 0 for empty tasks, got %d", count)
	}
}

func TestBroadcastUpdate_NoClients(t *testing.T) {
	server := createTestServer(t)

	// Broadcast with no clients - should not panic
	server.broadcastUpdate("test", map[string]string{"data": "value"})
}

// ============================================================================
// E2E SMOKE TEST — P1 #4
// ============================================================================
// Validates the full task lifecycle: create → list → verify → delete.
// Uses httptest.NewServer so the full middleware stack is exercised.
// ============================================================================

func TestE2E_TaskLifecycle(t *testing.T) {
	server := createTestServer(t)

	// Build the full handler stack (mirrors Server.Start)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tasks", server.handleTasks)
	mux.HandleFunc("/api/v1/tasks/", server.handleTaskByID)
	mux.HandleFunc("/api/v1/health", server.handleHealth)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := ts.Client()

	// ---- Step 1: Health check ----
	resp, err := client.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected health 200, got %d", resp.StatusCode)
	}

	// ---- Step 2: Create a task via POST /api/v1/tasks ----
	taskPayload := `{"id":"e2e-test-001","title":"E2E Smoke Test Task","status":"todo","description":"Created by E2E test"}`
	createResp, err := client.Post(
		ts.URL+"/api/v1/tasks",
		"application/json",
		strings.NewReader(taskPayload),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/tasks failed: %v", err)
	}
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", createResp.StatusCode)
	}

	// ---- Step 3: List tasks via GET /api/v1/tasks and verify ----
	listResp, err := client.Get(ts.URL + "/api/v1/tasks")
	if err != nil {
		t.Fatalf("GET /api/v1/tasks failed: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	var listResult struct {
		Tasks []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"tasks"`
		Count int `json:"count"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listResult); err != nil {
		t.Fatalf("failed to decode task list: %v", err)
	}

	found := false
	for _, task := range listResult.Tasks {
		if task.ID == "e2e-test-001" {
			found = true
			if task.Title != "E2E Smoke Test Task" {
				t.Errorf("task title mismatch: got %q", task.Title)
			}
			break
		}
	}
	if !found {
		t.Fatalf("created task e2e-test-001 not found in GET /api/v1/tasks (count=%d)", listResult.Count)
	}

	// ---- Step 4: Delete the task via DELETE /api/v1/tasks/{id} ----
	delReq, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/tasks/e2e-test-001", nil)
	if err != nil {
		t.Fatalf("failed to create DELETE request: %v", err)
	}
	delResp, err := client.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE /api/v1/tasks/e2e-test-001 failed: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on DELETE, got %d", delResp.StatusCode)
	}

	// ---- Step 5: Verify task is gone ----
	verifyResp, err := client.Get(ts.URL + "/api/v1/tasks")
	if err != nil {
		t.Fatalf("GET /api/v1/tasks (post-delete) failed: %v", err)
	}
	defer verifyResp.Body.Close()

	var verifyResult struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	if err := json.NewDecoder(verifyResp.Body).Decode(&verifyResult); err != nil {
		t.Fatalf("failed to decode post-delete task list: %v", err)
	}

	for _, task := range verifyResult.Tasks {
		if task.ID == "e2e-test-001" {
			t.Error("task e2e-test-001 still present after DELETE")
		}
	}
}
