// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package architect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/httputil"
)

// ============================================================================
// WOTAN TEST HELPERS
// ============================================================================

// fakeWotanServer creates a test HTTP server that mimics the Wotan API.
// It returns the server and a cleanup function.
func fakeWotanServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// Handle subscribe requests
	mux.HandleFunc("/api/v1/topics/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Subscribe or Publish
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"subscriber": map[string]interface{}{
					"subscriber_id": "test-sub-1",
					"topic":         "test",
					"display_name":  "test",
					"status":        "approved",
				},
			})
			return
		}
		if r.Method == http.MethodGet {
			// GetMessages
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": []interface{}{},
			})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	return httptest.NewServer(mux)
}

// createWotanClient creates a real wotanClient.Client pointed at a test server.
func createWotanClient(t *testing.T, serverURL string) *wotanClient.Client {
	t.Helper()
	// Strip "http://" prefix and pass just the host:port
	addr := serverURL[len("http://"):]
	client, err := wotanClient.NewClient(addr)
	if err != nil {
		t.Fatalf("failed to create wotan client: %v", err)
	}
	return client
}

// ============================================================================
// NewWithWotan TESTS
// ============================================================================

func TestArchitectService_NewWithWotan(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()

	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc := NewWithWotan(client)
	if svc == nil {
		t.Fatal("NewWithWotan returned nil")
	}
	if svc.wotan != client {
		t.Error("wotan client not set correctly")
	}
	if svc.infra == nil {
		t.Error("infra state not initialized")
	}
	if svc.network == nil {
		t.Error("network state not initialized")
	}
	if svc.alertChannel == nil {
		t.Error("alertChannel not initialized")
	}
}

func TestArchitectService_NewWithWotan_NilClient(t *testing.T) {
	svc := NewWithWotan(nil)
	if svc == nil {
		t.Fatal("NewWithWotan(nil) returned nil")
	}
	if svc.wotan != nil {
		t.Error("wotan should be nil when created with nil client")
	}
}

// ============================================================================
// SetWotan TESTS
// ============================================================================

func TestArchitectService_SetWotan(t *testing.T) {
	svc := New()
	if svc.wotan != nil {
		t.Error("wotan should initially be nil")
	}

	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc.SetWotan(client)
	if svc.wotan != client {
		t.Error("SetWotan did not set client")
	}
}

func TestArchitectService_SetWotan_Nil(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc := NewWithWotan(client)
	svc.SetWotan(nil)
	if svc.wotan != nil {
		t.Error("SetWotan(nil) should clear the client")
	}
}

func TestArchitectService_SetWotan_ConcurrentAccess(t *testing.T) {
	svc := New()
	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	defer client.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.SetWotan(client)
		}()
	}
	wg.Wait()
}

// ============================================================================
// Start TESTS
// ============================================================================

func TestArchitectService_Start_NilWotan(t *testing.T) {
	svc := New()
	ctx := context.Background()

	err := svc.Start(ctx)
	if err != nil {
		t.Errorf("Start() with nil wotan should return nil, got %v", err)
	}
}

func TestArchitectService_Start_WithWotan(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()

	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc := NewWithWotan(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := svc.Start(ctx)
	if err != nil {
		t.Errorf("Start() with valid wotan should succeed, got %v", err)
	}
}

func TestArchitectService_Start_SubscribeError(t *testing.T) {
	// Create a server that rejects subscribe requests
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/topics/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "INTERNAL_ERROR",
				"message": "server error",
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc := NewWithWotan(client)
	ctx := context.Background()

	err := svc.Start(ctx)
	if err == nil {
		t.Error("Start() should return error when subscribe fails")
	}
}

// ============================================================================
// listenForAlerts TESTS
// ============================================================================

func TestArchitectService_listenForAlerts_NilWotan(t *testing.T) {
	svc := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	// Should return immediately without panic
	svc.listenForAlerts(ctx)
}

func TestArchitectService_listenForAlerts_ContextCancelled(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	// Do not defer client.Close() because StreamMessages' internal polling
	// goroutine also closes the channel on ctx cancel, and Close() would
	// double-close it. Instead, just let the client be garbage collected.

	svc := NewWithWotan(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	// Subscribe first so StreamMessages can work
	_, err := client.Subscribe(ctx, "alerts.critical", "test")
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	done := make(chan struct{})
	go func() {
		svc.listenForAlerts(ctx)
		close(done)
	}()

	// Cancel context to stop the listener
	cancel()

	select {
	case <-done:
		// Success
	case <-time.After(3 * time.Second):
		t.Error("listenForAlerts did not return after context cancellation")
	}
}

// ============================================================================
// handleCriticalAlert TESTS
// ============================================================================

func TestArchitectService_handleCriticalAlert_NilMessage(t *testing.T) {
	svc := New()
	ctx := context.Background()
	// Should not panic with nil message
	svc.handleCriticalAlert(ctx, nil)
}

func TestArchitectService_handleCriticalAlert_ValidMessage(t *testing.T) {
	svc := New()
	ctx := context.Background()
	msg := &wotanClient.Message{
		MessageID: "msg-1",
		Topic:     "alerts.critical",
		Payload:   `{"severity":"critical","message":"test alert"}`,
		CreatedAt: time.Now(),
	}
	// Should not panic
	svc.handleCriticalAlert(ctx, msg)
}

// ============================================================================
// publishStateChange TESTS
// ============================================================================

func TestArchitectService_publishStateChange_NilWotan(t *testing.T) {
	svc := New()
	ctx := context.Background()
	// Should return immediately without panic
	svc.publishStateChange(ctx, "test.event", map[string]string{"key": "value"})
}

func TestArchitectService_publishStateChange_WithTraceID(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc := NewWithWotan(client)
	ctx := context.Background()

	// Subscribe first so we're subscribed to architecture.updates
	_, err := client.Subscribe(ctx, "architecture.updates", "test")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Use a context with trace_id
	//nolint:staticcheck // Using string key for context value to match existing code
	ctx = context.WithValue(ctx, "trace_id", "test-trace-123")
	svc.publishStateChange(ctx, "test.event", map[string]string{"key": "value"})
	// No panic = success
}

func TestArchitectService_publishStateChange_WithoutTraceID(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc := NewWithWotan(client)
	ctx := context.Background()

	_, err := client.Subscribe(ctx, "architecture.updates", "test")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	svc.publishStateChange(ctx, "test.event", map[string]string{"key": "value"})
}

// ============================================================================
// GetNetworkNode ADDITIONAL TESTS (coverage gaps)
// ============================================================================

func TestArchitectService_GetNetworkNode_NilContext(t *testing.T) {
	svc := New()
	_, err := svc.GetNetworkNode(nil, "node-1")
	if err != ErrNilContext {
		t.Errorf("GetNetworkNode(nil ctx) = %v, want ErrNilContext", err)
	}
}

func TestArchitectService_GetNetworkNode_EmptyNodeID(t *testing.T) {
	svc := New()
	ctx := context.Background()
	_, err := svc.GetNetworkNode(ctx, "")
	if err == nil {
		t.Error("GetNetworkNode(empty ID) should return error")
	}
}

func TestArchitectService_GetNetworkNode_NotFound(t *testing.T) {
	svc := New()
	ctx := context.Background()
	_, err := svc.GetNetworkNode(ctx, "nonexistent")
	if err == nil {
		t.Error("GetNetworkNode(nonexistent) should return error")
	}
}

// ============================================================================
// AddNetworkNode ADDITIONAL TESTS
// ============================================================================

func TestArchitectService_AddNetworkNode_NilContext(t *testing.T) {
	svc := New()
	node := createValidNetworkNode()
	err := svc.AddNetworkNode(nil, node)
	if err != ErrNilContext {
		t.Errorf("AddNetworkNode(nil ctx) = %v, want ErrNilContext", err)
	}
}

func TestArchitectService_AddNetworkNode_TimestampsSet(t *testing.T) {
	svc := New()
	ctx := context.Background()
	node := createValidNetworkNode()
	node.CreatedAt = time.Time{}
	node.UpdatedAt = time.Time{}

	before := time.Now()
	err := svc.AddNetworkNode(ctx, node)
	after := time.Now()

	if err != nil {
		t.Fatalf("AddNetworkNode() = %v", err)
	}

	retrieved, _ := svc.GetNetworkNode(ctx, node.NodeID)
	if retrieved.CreatedAt.Before(before) || retrieved.CreatedAt.After(after) {
		t.Errorf("CreatedAt not set correctly: %v", retrieved.CreatedAt)
	}
	if retrieved.UpdatedAt.Before(before) || retrieved.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt not set correctly: %v", retrieved.UpdatedAt)
	}
}

// ============================================================================
// ListNetworkNodes ADDITIONAL TESTS
// ============================================================================

func TestArchitectService_ListNetworkNodes_NilContext(t *testing.T) {
	svc := New()
	_, err := svc.ListNetworkNodes(nil)
	if err != ErrNilContext {
		t.Errorf("ListNetworkNodes(nil ctx) = %v, want ErrNilContext", err)
	}
}

func TestArchitectService_ListNetworkNodes_Empty(t *testing.T) {
	svc := New()
	ctx := context.Background()
	nodes, err := svc.ListNetworkNodes(ctx)
	if err != nil {
		t.Errorf("ListNetworkNodes() = %v, want nil", err)
	}
	if len(nodes) != 0 {
		t.Errorf("ListNetworkNodes() returned %d nodes, want 0", len(nodes))
	}
}

// ============================================================================
// GetNetworkTopology ADDITIONAL TESTS
// ============================================================================

func TestArchitectService_GetNetworkTopology_NilContext(t *testing.T) {
	svc := New()
	_, err := svc.GetNetworkTopology(nil)
	if err != ErrNilContext {
		t.Errorf("GetNetworkTopology(nil ctx) = %v, want ErrNilContext", err)
	}
}

// ============================================================================
// LogDecision ADDITIONAL TESTS
// ============================================================================

func TestArchitectService_LogDecision_NilContext(t *testing.T) {
	svc := New()
	decision := createValidDecision()
	err := svc.LogDecision(nil, decision)
	if err != ErrNilContext {
		t.Errorf("LogDecision(nil ctx) = %v, want ErrNilContext", err)
	}
}

func TestArchitectService_LogDecision_TimestampsSet(t *testing.T) {
	svc := New()
	ctx := context.Background()
	decision := createValidDecision()
	decision.CreatedAt = time.Time{}
	decision.UpdatedAt = time.Time{}

	before := time.Now()
	err := svc.LogDecision(ctx, decision)
	after := time.Now()

	if err != nil {
		t.Fatalf("LogDecision() = %v", err)
	}

	decisions, _ := svc.ListDecisions(ctx)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].CreatedAt.Before(before) || decisions[0].CreatedAt.After(after) {
		t.Errorf("CreatedAt not set correctly")
	}
}

func TestArchitectService_LogDecision_MultipleDecisions(t *testing.T) {
	svc := New()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		d := createValidDecision()
		d.DecisionID = fmt.Sprintf("dec-%d", i)
		d.Title = fmt.Sprintf("Decision %d", i)
		d.Description = fmt.Sprintf("Description for decision %d", i)
		err := svc.LogDecision(ctx, d)
		if err != nil {
			t.Fatalf("LogDecision[%d] = %v", i, err)
		}
	}

	decisions, err := svc.ListDecisions(ctx)
	if err != nil {
		t.Fatalf("ListDecisions() = %v", err)
	}
	if len(decisions) != 5 {
		t.Errorf("expected 5 decisions, got %d", len(decisions))
	}
}

// ============================================================================
// ListDecisions ADDITIONAL TESTS
// ============================================================================

func TestArchitectService_ListDecisions_NilContext(t *testing.T) {
	svc := New()
	_, err := svc.ListDecisions(nil)
	if err != ErrNilContext {
		t.Errorf("ListDecisions(nil ctx) = %v, want ErrNilContext", err)
	}
}

func TestArchitectService_ListDecisions_Snapshot(t *testing.T) {
	svc := New()
	ctx := context.Background()

	d := createValidDecision()
	svc.LogDecision(ctx, d)

	decisions, _ := svc.ListDecisions(ctx)
	origLen := len(decisions)

	// Modifying the snapshot should not affect the source
	decisions = append(decisions, ArchitectureDecision{Title: "Extra"})

	decisions2, _ := svc.ListDecisions(ctx)
	if len(decisions2) != origLen {
		t.Errorf("ListDecisions snapshot isolation broken")
	}
}

// ============================================================================
// Health ADDITIONAL TESTS (unhealthy state)
// ============================================================================

func TestArchitectService_Health_Unhealthy_NilInfra(t *testing.T) {
	svc := New()
	svc.infra = nil
	ctx := context.Background()

	healthy, err := svc.Health(ctx)
	if err != nil {
		t.Errorf("Health() error = %v", err)
	}
	if healthy {
		t.Error("Health() = true when infra is nil, want false")
	}
}

func TestArchitectService_Health_Unhealthy_NilNetwork(t *testing.T) {
	svc := New()
	svc.network = nil
	ctx := context.Background()

	healthy, err := svc.Health(ctx)
	if err != nil {
		t.Errorf("Health() error = %v", err)
	}
	if healthy {
		t.Error("Health() = true when network is nil, want false")
	}
}

// ============================================================================
// HANDLER: Ready ENDPOINT TESTS
// ============================================================================

func TestHTTPHandler_Ready_Success(t *testing.T) {
	svc := New()
	handler := NewHTTPHandler(svc)
	req, _ := http.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	handler.Ready(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Ready status = %d, want %d, body: %s", status, http.StatusOK, rr.Body.String())
	}

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	if ready, ok := data["ready"]; !ok || !ready.(bool) {
		t.Error("ready field missing or false")
	}
}

func TestHTTPHandler_Ready_WrongMethod(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("POST", "/ready", nil)
	rr := httptest.NewRecorder()

	handler.Ready(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}

func TestHTTPHandler_Ready_NilInfra(t *testing.T) {
	svc := New()
	svc.infra = nil
	handler := NewHTTPHandler(svc)

	req, _ := http.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	handler.Ready(rr, req)

	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}
}

func TestHTTPHandler_Ready_NilNetwork(t *testing.T) {
	svc := New()
	svc.network = nil
	handler := NewHTTPHandler(svc)

	req, _ := http.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	handler.Ready(rr, req)

	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}
}

// ============================================================================
// HANDLER: Health ENDPOINT ADDITIONAL TESTS (unhealthy service)
// ============================================================================

func TestHTTPHandler_Health_Unhealthy(t *testing.T) {
	svc := New()
	svc.infra = nil // Make it unhealthy
	handler := NewHTTPHandler(svc)

	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler.Health(rr, req)

	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}
}

// ============================================================================
// HANDLER: AddNetworkNode ADDITIONAL TESTS
// ============================================================================

func TestHTTPHandler_AddNetworkNode_InvalidJSON(t *testing.T) {
	handler := NewHTTPHandler(nil)

	body := bytes.NewReader([]byte("{not valid json"))
	req, _ := http.NewRequest("POST", "/network/nodes", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AddNetworkNode(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

func TestHTTPHandler_AddNetworkNode_WrongMethod(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("GET", "/network/nodes", nil)
	rr := httptest.NewRecorder()

	handler.AddNetworkNode(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}

// ============================================================================
// HANDLER: LogDesign ADDITIONAL TESTS
// ============================================================================

func TestHTTPHandler_LogDesign_InvalidJSON(t *testing.T) {
	handler := NewHTTPHandler(nil)

	body := bytes.NewReader([]byte("not json"))
	req, _ := http.NewRequest("POST", "/design", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LogDesign(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

func TestHTTPHandler_LogDesign_WrongMethod(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("GET", "/design", nil)
	rr := httptest.NewRecorder()

	handler.LogDesign(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}

// ============================================================================
// HANDLER: GetDesignDecisions EMPTY STATE
// ============================================================================

func TestHTTPHandler_GetDesignDecisions_Empty(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("GET", "/design", nil)
	rr := httptest.NewRecorder()

	handler.GetDesignDecisions(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
}

// ============================================================================
// ADR MANAGEMENT WORKFLOW (Design Review)
// ============================================================================

func TestADRManagement_FullWorkflow(t *testing.T) {
	handler := NewHTTPHandler(nil)

	// Step 1: Log a proposed ADR
	adr1 := ArchitectureDecision{
		DecisionID:  "ADR-001",
		Title:       "Use gRPC for service communication",
		Description: "gRPC provides efficient binary protocol for inter-service communication",
		Component:   "infrastructure",
		Status:      "proposed",
	}
	body, _ := json.Marshal(adr1)
	req, _ := http.NewRequest("POST", "/design", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.LogDesign(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("LogDesign ADR-001 status = %d, want %d", rr.Code, http.StatusCreated)
	}

	// Step 2: Log an approved ADR
	adr2 := ArchitectureDecision{
		DecisionID:  "ADR-002",
		Title:       "Use NixOS for containers",
		Description: "NixOS provides declarative, reproducible container definitions",
		Component:   "containers",
		Status:      "approved",
	}
	body, _ = json.Marshal(adr2)
	req, _ = http.NewRequest("POST", "/design", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	handler.LogDesign(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("LogDesign ADR-002 status = %d, want %d", rr.Code, http.StatusCreated)
	}

	// Step 3: Retrieve all decisions
	req, _ = http.NewRequest("GET", "/design", nil)
	rr = httptest.NewRecorder()
	handler.GetDesignDecisions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GetDesignDecisions status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)
	decisions := resp.Data.([]interface{})
	if len(decisions) != 2 {
		t.Errorf("expected 2 decisions, got %d", len(decisions))
	}
}

// ============================================================================
// DESIGN REVIEW WORKFLOW (end-to-end via HTTP)
// ============================================================================

func TestDesignReviewWorkflow(t *testing.T) {
	svc := New()
	handler := NewHTTPHandler(svc)

	// 1. Add infrastructure services
	services := []Service{
		{ServiceID: "svc-wotan", Name: "Wotan", Type: "message-bus", Status: "running", Port: 9090},
		{ServiceID: "svc-gateway", Name: "Gateway", Type: "proxy", Status: "running", Port: 443},
		{ServiceID: "svc-timeguru", Name: "Timeguru", Type: "service", Status: "running", Port: 8000},
	}

	for _, s := range services {
		body, _ := json.Marshal(s)
		req, _ := http.NewRequest("POST", "/infrastructure/services", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.AddService(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("AddService(%s) status = %d", s.ServiceID, rr.Code)
		}
	}

	// 2. Add network nodes
	nodes := []NetworkNode{
		{NodeID: "net-bridge", Name: "lxdbr0", Type: "bridge", CIDR: "10.10.10.0/24", Status: "active"},
		{NodeID: "net-gateway", Name: "Gateway Node", Type: "gateway", CIDR: "10.10.10.100/32", Status: "active"},
	}

	for _, n := range nodes {
		body, _ := json.Marshal(n)
		req, _ := http.NewRequest("POST", "/network/nodes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.AddNetworkNode(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("AddNetworkNode(%s) status = %d", n.NodeID, rr.Code)
		}
	}

	// 3. Log design decision about this architecture
	decision := ArchitectureDecision{
		DecisionID:  "ADR-ARCH-1",
		Title:       "Hub-and-spoke via Wotan",
		Description: "All inter-service communication goes through Wotan message bus",
		Component:   "architecture",
		Status:      "implemented",
	}
	body, _ := json.Marshal(decision)
	req, _ := http.NewRequest("POST", "/design", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.LogDesign(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("LogDesign status = %d", rr.Code)
	}

	// 4. Verify infrastructure state
	req, _ = http.NewRequest("GET", "/infrastructure", nil)
	rr = httptest.NewRecorder()
	handler.GetInfrastructure(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GetInfrastructure status = %d", rr.Code)
	}

	var infraResp httputil.Response
	json.NewDecoder(rr.Body).Decode(&infraResp)
	infraData := infraResp.Data.(map[string]interface{})
	svcMap := infraData["services"].(map[string]interface{})
	if len(svcMap) != 3 {
		t.Errorf("expected 3 services, got %d", len(svcMap))
	}

	// 5. Verify network topology
	req, _ = http.NewRequest("GET", "/network", nil)
	rr = httptest.NewRecorder()
	handler.GetNetwork(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GetNetwork status = %d", rr.Code)
	}

	// 6. Verify health
	req, _ = http.NewRequest("GET", "/health", nil)
	rr = httptest.NewRecorder()
	handler.Health(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("Health status = %d", rr.Code)
	}

	// 7. Verify readiness
	req, _ = http.NewRequest("GET", "/ready", nil)
	rr = httptest.NewRecorder()
	handler.Ready(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("Ready status = %d", rr.Code)
	}
}

// ============================================================================
// WOTAN INTEGRATION TESTS
// ============================================================================

func TestWotanIntegration_AddServicePublishes(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc := NewWithWotan(client)
	ctx := context.Background()

	// Subscribe to architecture.updates so publish can work
	_, err := client.Subscribe(ctx, "architecture.updates", "test")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// AddService triggers publishStateChange in a goroutine
	s := createValidService()
	err = svc.AddService(ctx, s)
	if err != nil {
		t.Fatalf("AddService() = %v", err)
	}

	// Give goroutine time to execute
	time.Sleep(100 * time.Millisecond)
}

func TestWotanIntegration_AddNetworkNodePublishes(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc := NewWithWotan(client)
	ctx := context.Background()

	_, err := client.Subscribe(ctx, "architecture.updates", "test")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	node := createValidNetworkNode()
	err = svc.AddNetworkNode(ctx, node)
	if err != nil {
		t.Fatalf("AddNetworkNode() = %v", err)
	}

	time.Sleep(100 * time.Millisecond)
}

func TestWotanIntegration_LogDecisionPublishes(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	defer client.Close()

	svc := NewWithWotan(client)
	ctx := context.Background()

	_, err := client.Subscribe(ctx, "architecture.updates", "test")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	decision := createValidDecision()
	err = svc.LogDecision(ctx, decision)
	if err != nil {
		t.Fatalf("LogDecision() = %v", err)
	}

	time.Sleep(100 * time.Millisecond)
}

func TestWotanIntegration_StartAndListen(t *testing.T) {
	server := fakeWotanServer(t)
	defer server.Close()
	client := createWotanClient(t, server.URL)
	// Do not defer client.Close() -- Start -> listenForAlerts -> StreamMessages
	// starts an internal goroutine that closes the channel on ctx cancel.
	// Calling client.Close() afterward would double-close.

	svc := NewWithWotan(client)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := svc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}

	// Let it run briefly then cancel
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
}

// ============================================================================
// CONCURRENT ACCESS TO ALL OPERATIONS (race detector test)
// ============================================================================

func TestArchitectService_ConcurrentAllOps(t *testing.T) {
	svc := New()
	ctx := context.Background()
	var wg sync.WaitGroup

	// Mix of all operations concurrently
	for i := 0; i < 20; i++ {
		wg.Add(7)

		go func(n int) {
			defer wg.Done()
			s := createValidService()
			s.ServiceID = fmt.Sprintf("svc-%d", n)
			_ = svc.AddService(ctx, s)
		}(i)

		go func(n int) {
			defer wg.Done()
			_, _ = svc.GetService(ctx, fmt.Sprintf("svc-%d", n))
		}(i)

		go func() {
			defer wg.Done()
			_, _ = svc.ListServices(ctx)
		}()

		go func(n int) {
			defer wg.Done()
			node := createValidNetworkNode()
			node.NodeID = fmt.Sprintf("node-%d", n)
			_ = svc.AddNetworkNode(ctx, node)
		}(i)

		go func(n int) {
			defer wg.Done()
			_, _ = svc.GetNetworkNode(ctx, fmt.Sprintf("node-%d", n))
		}(i)

		go func(n int) {
			defer wg.Done()
			d := createValidDecision()
			d.DecisionID = fmt.Sprintf("dec-%d", n)
			_ = svc.LogDecision(ctx, d)
		}(i)

		go func() {
			defer wg.Done()
			_, _ = svc.Health(ctx)
		}()
	}

	wg.Wait()

	// Verify basic consistency
	services, _ := svc.ListServices(ctx)
	nodes, _ := svc.ListNetworkNodes(ctx)
	decisions, _ := svc.ListDecisions(ctx)
	healthy, _ := svc.Health(ctx)

	if len(services) == 0 {
		t.Error("expected services after concurrent adds")
	}
	if len(nodes) == 0 {
		t.Error("expected nodes after concurrent adds")
	}
	if len(decisions) == 0 {
		t.Error("expected decisions after concurrent adds")
	}
	if !healthy {
		t.Error("expected healthy service")
	}
}

// ============================================================================
// HANDLER CONTENT-TYPE VERIFICATION
// ============================================================================

func TestHTTPHandler_ResponseContentType(t *testing.T) {
	handler := NewHTTPHandler(nil)

	tests := []struct {
		name    string
		method  string
		path    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"health", "GET", "/health", handler.Health},
		{"ready", "GET", "/ready", handler.Ready},
		{"infrastructure", "GET", "/infrastructure", handler.GetInfrastructure},
		{"network", "GET", "/network", handler.GetNetwork},
		{"design", "GET", "/design", handler.GetDesignDecisions},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			tt.handler(rr, req)

			ct := rr.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
		})
	}
}

// ============================================================================
// NIL INFRA/NETWORK STATE TESTS (for methods with those checks)
// ============================================================================

func TestArchitectService_AddService_NilInfra(t *testing.T) {
	svc := New()
	svc.infra = nil
	ctx := context.Background()
	s := createValidService()

	err := svc.AddService(ctx, s)
	if err != ErrInfraNotInitialized {
		t.Errorf("AddService(nil infra) = %v, want ErrInfraNotInitialized", err)
	}
}

func TestArchitectService_GetService_NilInfra(t *testing.T) {
	svc := New()
	svc.infra = nil
	ctx := context.Background()

	_, err := svc.GetService(ctx, "test")
	if err != ErrInfraNotInitialized {
		t.Errorf("GetService(nil infra) = %v, want ErrInfraNotInitialized", err)
	}
}

func TestArchitectService_ListServices_NilInfra(t *testing.T) {
	svc := New()
	svc.infra = nil
	ctx := context.Background()

	_, err := svc.ListServices(ctx)
	if err != ErrInfraNotInitialized {
		t.Errorf("ListServices(nil infra) = %v, want ErrInfraNotInitialized", err)
	}
}

func TestArchitectService_GetInfrastructureState_NilInfra(t *testing.T) {
	svc := New()
	svc.infra = nil
	ctx := context.Background()

	_, err := svc.GetInfrastructureState(ctx)
	if err != ErrInfraNotInitialized {
		t.Errorf("GetInfrastructureState(nil infra) = %v, want ErrInfraNotInitialized", err)
	}
}

func TestArchitectService_AddNetworkNode_NilNetwork(t *testing.T) {
	svc := New()
	svc.network = nil
	ctx := context.Background()
	node := createValidNetworkNode()

	err := svc.AddNetworkNode(ctx, node)
	if err != ErrNetworkNotInitialized {
		t.Errorf("AddNetworkNode(nil network) = %v, want ErrNetworkNotInitialized", err)
	}
}

func TestArchitectService_GetNetworkNode_NilNetwork(t *testing.T) {
	svc := New()
	svc.network = nil
	ctx := context.Background()

	_, err := svc.GetNetworkNode(ctx, "test")
	if err != ErrNetworkNotInitialized {
		t.Errorf("GetNetworkNode(nil network) = %v, want ErrNetworkNotInitialized", err)
	}
}

func TestArchitectService_ListNetworkNodes_NilNetwork(t *testing.T) {
	svc := New()
	svc.network = nil
	ctx := context.Background()

	_, err := svc.ListNetworkNodes(ctx)
	if err != ErrNetworkNotInitialized {
		t.Errorf("ListNetworkNodes(nil network) = %v, want ErrNetworkNotInitialized", err)
	}
}

func TestArchitectService_GetNetworkTopology_NilNetwork(t *testing.T) {
	svc := New()
	svc.network = nil
	ctx := context.Background()

	_, err := svc.GetNetworkTopology(ctx)
	if err != ErrNetworkNotInitialized {
		t.Errorf("GetNetworkTopology(nil network) = %v, want ErrNetworkNotInitialized", err)
	}
}

func TestArchitectService_LogDecision_NilInfra(t *testing.T) {
	svc := New()
	svc.infra = nil
	ctx := context.Background()
	decision := createValidDecision()

	err := svc.LogDecision(ctx, decision)
	if err != ErrInfraNotInitialized {
		t.Errorf("LogDecision(nil infra) = %v, want ErrInfraNotInitialized", err)
	}
}

func TestArchitectService_ListDecisions_NilInfra(t *testing.T) {
	svc := New()
	svc.infra = nil
	ctx := context.Background()

	_, err := svc.ListDecisions(ctx)
	if err != ErrInfraNotInitialized {
		t.Errorf("ListDecisions(nil infra) = %v, want ErrInfraNotInitialized", err)
	}
}
