package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		BusboyAddr:      "localhost:9090",
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
	return NewServer(cfg)
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
		BusboyAddr:      "localhost:9090",
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	server := NewServer(cfg)

	if server == nil {
		t.Fatal("expected non-nil server")
	}

	if server.config.Port != cfg.Port {
		t.Errorf("expected port %s, got %s", cfg.Port, server.config.Port)
	}

	if server.sseClients == nil {
		t.Error("expected initialized sseClients map")
	}

	if len(server.tasks) == 0 {
		t.Error("expected initial tasks to be loaded")
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
	server := createTestServer(t)

	// Clear tasks
	server.tasksMu.Lock()
	server.tasks = []Task{}
	server.tasksMu.Unlock()

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
