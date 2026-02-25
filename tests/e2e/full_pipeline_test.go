// Package e2e provides comprehensive end-to-end tests for the Unheaded platform.
//
// This file verifies the full pipeline WITHOUT requiring live BPF programs or Docker:
//   - All services expose /health, /ready, /metrics endpoints
//   - Auth enforcement on protected endpoints
//   - Trace event propagation through Wotan to dashboard
//   - Service discovery registration and resolution
//
// All tests are self-contained using httptest.Server and in-process mocks.
package e2e

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/discovery"
	"unheaded/pkg/health"
	"unheaded/pkg/trace"
	"unheaded/pkg/wotan-client/mock"
)

// ============================================================================
// PIPELINE TEST HARNESS
// ============================================================================

// serviceDefinition describes a mock service in the pipeline.
type serviceDefinition struct {
	name    string
	version string
	port    int // logical port (for logging only, actual port is random)
}

// pipelineServices lists all Unheaded services that must be verified.
var pipelineServices = []serviceDefinition{
	{name: "timeguru", version: "0.1.0", port: 8000},
	{name: "captain", version: "0.1.0", port: 8001},
	{name: "architect", version: "0.1.0", port: 8002},
	{name: "micromanager", version: "0.1.0", port: 8003},
	{name: "monad", version: "0.1.0", port: 8004},
	{name: "sophia", version: "0.1.0", port: 8005},
	{name: "dashboard-backend", version: "0.1.0", port: 8080},
	{name: "kanban-app", version: "0.1.0", port: 8090},
}

// pipelineTestHarness wires up all services with auth, health, and Wotan.
type pipelineTestHarness struct {
	t *testing.T

	// Auth components
	signingKey []byte
	jwtAuth    *auth.JWTAuthenticator

	// Mock Wotan
	wotan *mock.MockClient

	// Service discovery
	discoveryRegistry *discovery.Registry
	discoveryWotan    *mockWotanDiscovery

	// Service servers keyed by name
	servers map[string]*httptest.Server

	// Cleanup
	cleanup []func()
}

// mockWotanDiscovery adapts the mock Wotan client to the discovery.WotanClient
// interface (Publish/Subscribe with different signatures).
type mockWotanDiscovery struct {
	mu       sync.Mutex
	handlers map[string]func([]byte)
	mock     *mock.MockClient
}

func newMockWotanDiscovery() *mockWotanDiscovery {
	return &mockWotanDiscovery{
		handlers: make(map[string]func([]byte)),
		mock:     mock.NewMockClient(mock.WithAutoApprove()),
	}
}

func (m *mockWotanDiscovery) Publish(topic string, data []byte) error {
	m.mu.Lock()
	handler, ok := m.handlers[topic]
	m.mu.Unlock()
	if ok {
		handler(data)
	}
	return nil
}

func (m *mockWotanDiscovery) Subscribe(topic string, handler func([]byte)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[topic] = handler
	return nil
}

// newPipelineTestHarness creates a harness with all mock services.
func newPipelineTestHarness(t *testing.T) *pipelineTestHarness {
	t.Helper()

	signingKey := []byte("test-signing-key-for-e2e-pipeline-32b!")
	jwtAuth := auth.NewJWTAuthenticator(auth.JWTConfig{
		SigningKey: signingKey,
		Issuer:    "unheaded-e2e-test",
	})

	wotanDiscovery := newMockWotanDiscovery()
	registry := discovery.New(wotanDiscovery)

	h := &pipelineTestHarness{
		t:                 t,
		signingKey:        signingKey,
		jwtAuth:           jwtAuth,
		wotan:             mock.NewMockClient(mock.WithAutoApprove()),
		discoveryRegistry: registry,
		discoveryWotan:    wotanDiscovery,
		servers:           make(map[string]*httptest.Server),
	}

	// Start discovery registry.
	ctx := context.Background()
	if err := registry.Start(ctx); err != nil {
		t.Fatalf("Failed to start discovery registry: %v", err)
	}
	h.cleanup = append(h.cleanup, func() { registry.Stop() })

	// Create a mock service for each definition.
	for _, svc := range pipelineServices {
		h.createMockService(svc)
	}

	return h
}

// createMockService spins up an httptest.Server that exposes /health, /ready,
// /metrics, and /api/v1/* (protected by auth middleware).
func (h *pipelineTestHarness) createMockService(svc serviceDefinition) {
	mux := http.NewServeMux()

	// Unauthenticated endpoints.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		health.WriteHealthy(w, svc.name, svc.version)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ready", "service": svc.name})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP unheaded_http_requests_total Total HTTP requests\n")
		fmt.Fprintf(w, "# TYPE unheaded_http_requests_total counter\n")
		fmt.Fprintf(w, "unheaded_http_requests_total{service=\"%s\",method=\"GET\",path=\"/health\",status=\"200\"} 1\n", svc.name)
		fmt.Fprintf(w, "# HELP unheaded_http_request_duration_seconds HTTP request latency\n")
		fmt.Fprintf(w, "# TYPE unheaded_http_request_duration_seconds histogram\n")
	})

	// Protected API endpoints (require auth).
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"service": svc.name,
			"path":    r.URL.Path,
			"status":  "ok",
		})
	})

	authMiddleware := auth.Middleware(h.jwtAuth)
	protectedAPI := authMiddleware(apiHandler)

	mux.Handle("/api/", protectedAPI)

	server := httptest.NewServer(mux)
	h.servers[svc.name] = server
	h.cleanup = append(h.cleanup, server.Close)

	// Register with service discovery.
	if err := h.discoveryRegistry.Register(discovery.ServiceEntry{
		Name:    svc.name,
		Address: server.URL,
		Port:    svc.port,
		Health:  "healthy",
		Metadata: map[string]string{
			"version": svc.version,
		},
	}); err != nil {
		h.t.Fatalf("Failed to register %s with discovery: %v", svc.name, err)
	}
}

// generateToken creates a valid JWT for testing authenticated requests.
func (h *pipelineTestHarness) generateToken(subject string, roles []string) string {
	h.t.Helper()
	claims := &auth.JWTClaims{
		Sub:   subject,
		Iss:   "unheaded-e2e-test",
		Iat:   time.Now().Unix(),
		Exp:   time.Now().Add(5 * time.Minute).Unix(),
		Roles: roles,
	}
	token, err := auth.SignToken(h.signingKey, claims)
	if err != nil {
		h.t.Fatalf("Failed to generate test token: %v", err)
	}
	return token
}

// Cleanup tears down all test resources.
func (h *pipelineTestHarness) Cleanup() {
	for i := len(h.cleanup) - 1; i >= 0; i-- {
		h.cleanup[i]()
	}
}

// ============================================================================
// FULL PIPELINE TESTS
// ============================================================================

// TestFullPipeline_HealthEndpoints verifies all services expose /health
// returning 200 with the standard Unheaded health response shape.
func TestFullPipeline_HealthEndpoints(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	for _, svc := range pipelineServices {
		t.Run(svc.name, func(t *testing.T) {
			server := h.servers[svc.name]
			resp, err := http.Get(server.URL + "/health")
			if err != nil {
				t.Fatalf("GET /health failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200, got %d", resp.StatusCode)
			}

			var healthResp health.Response
			if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
				t.Fatalf("Failed to decode health response: %v", err)
			}

			if healthResp.Service != svc.name {
				t.Errorf("Expected service %q, got %q", svc.name, healthResp.Service)
			}
			if healthResp.Status != health.StatusHealthy {
				t.Errorf("Expected status healthy, got %s", healthResp.Status)
			}
			if healthResp.Version != svc.version {
				t.Errorf("Expected version %q, got %q", svc.version, healthResp.Version)
			}
			if healthResp.Timestamp.IsZero() {
				t.Error("Timestamp should not be zero")
			}
		})
	}
}

// TestFullPipeline_ReadyEndpoints verifies all services expose /ready returning 200.
func TestFullPipeline_ReadyEndpoints(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	for _, svc := range pipelineServices {
		t.Run(svc.name, func(t *testing.T) {
			server := h.servers[svc.name]
			resp, err := http.Get(server.URL + "/ready")
			if err != nil {
				t.Fatalf("GET /ready failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200, got %d", resp.StatusCode)
			}

			var readyResp map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&readyResp); err != nil {
				t.Fatalf("Failed to decode ready response: %v", err)
			}

			if readyResp["status"] != "ready" {
				t.Errorf("Expected status 'ready', got %q", readyResp["status"])
			}
		})
	}
}

// TestFullPipeline_MetricsEndpoints verifies all services expose /metrics
// with Prometheus-compatible output.
func TestFullPipeline_MetricsEndpoints(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	for _, svc := range pipelineServices {
		t.Run(svc.name, func(t *testing.T) {
			server := h.servers[svc.name]
			resp, err := http.Get(server.URL + "/metrics")
			if err != nil {
				t.Fatalf("GET /metrics failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200, got %d", resp.StatusCode)
			}

			contentType := resp.Header.Get("Content-Type")
			if !strings.Contains(contentType, "text/plain") {
				t.Errorf("Expected text/plain content type, got %q", contentType)
			}

			// Read body and verify it contains Prometheus-format metrics.
			buf := make([]byte, 4096)
			n, _ := resp.Body.Read(buf)
			body := string(buf[:n])

			if !strings.Contains(body, "# HELP") {
				t.Error("Metrics output should contain # HELP directives")
			}
			if !strings.Contains(body, "# TYPE") {
				t.Error("Metrics output should contain # TYPE directives")
			}
			if !strings.Contains(body, "unheaded_") {
				t.Error("Metrics output should contain unheaded_ prefixed metrics")
			}
		})
	}
}

// TestFullPipeline_AuthEnforcement verifies unauthenticated requests to
// protected endpoints return 401.
func TestFullPipeline_AuthEnforcement(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	for _, svc := range pipelineServices {
		t.Run(svc.name, func(t *testing.T) {
			server := h.servers[svc.name]

			// Request without auth token.
			resp, err := http.Get(server.URL + "/api/v1/status")
			if err != nil {
				t.Fatalf("GET /api/v1/status failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Expected 401 without auth, got %d", resp.StatusCode)
			}
		})
	}
}

// TestFullPipeline_AuthSuccess verifies authenticated requests to protected
// endpoints return 200.
func TestFullPipeline_AuthSuccess(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	token := h.generateToken("e2e-test-user", []string{auth.RoleAdmin})

	for _, svc := range pipelineServices {
		t.Run(svc.name, func(t *testing.T) {
			server := h.servers[svc.name]

			req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/status", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("GET /api/v1/status failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 with valid auth, got %d", resp.StatusCode)
			}

			var apiResp map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
				t.Fatalf("Failed to decode API response: %v", err)
			}

			if apiResp["service"] != svc.name {
				t.Errorf("Expected service %q, got %q", svc.name, apiResp["service"])
			}
		})
	}
}

// TestFullPipeline_HealthEndpointsSkipAuth verifies that /health, /ready, and
// /metrics are accessible WITHOUT authentication (Prometheus scraping, k8s probes).
func TestFullPipeline_HealthEndpointsSkipAuth(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	paths := []string{"/health", "/ready", "/metrics"}

	for _, svc := range pipelineServices {
		for _, path := range paths {
			name := fmt.Sprintf("%s%s", svc.name, path)
			t.Run(name, func(t *testing.T) {
				server := h.servers[svc.name]

				// No auth header.
				resp, err := http.Get(server.URL + path)
				if err != nil {
					t.Fatalf("GET %s failed: %v", path, err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					t.Errorf("Expected 200 for %s (no auth required), got %d", path, resp.StatusCode)
				}
			})
		}
	}
}

// TestFullPipeline_TraceEventPropagation verifies that a trace event flows
// through Wotan from producer to consumer.
func TestFullPipeline_TraceEventPropagation(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Generate a trace ID.
	traceID, err := trace.NewTraceID()
	if err != nil {
		t.Fatalf("Failed to generate trace ID: %v", err)
	}
	if traceID.IsZero() {
		t.Fatal("Trace ID should not be zero")
	}

	// Subscribe a consumer to the trace topic.
	traceTopic := "traces.events"
	_, err = h.wotan.Subscribe(ctx, traceTopic, "dashboard-backend")
	if err != nil {
		t.Fatalf("Failed to subscribe to trace topic: %v", err)
	}

	// Start streaming before publishing.
	msgCh, err := h.wotan.StreamMessages(ctx, traceTopic)
	if err != nil {
		t.Fatalf("Failed to stream messages: %v", err)
	}

	// Simulate trace-collector publishing a trace event.
	traceEvent := map[string]interface{}{
		"trace_id":   traceID.String(),
		"flow_label": traceID.FlowLabelHint(),
		"source":     "10.10.10.20",
		"dest":       "10.10.10.100",
		"protocol":   "tcp",
		"latency_ns": 1500000, // 1.5ms
		"timestamp":  time.Now().UnixMilli(),
	}

	payload, err := json.Marshal(traceEvent)
	if err != nil {
		t.Fatalf("Failed to marshal trace event: %v", err)
	}

	// Publisher must subscribe to the topic first (mock client requirement).
	_, err = h.wotan.Subscribe(ctx, traceTopic, "trace-collector")
	if err != nil {
		t.Fatalf("Publisher subscribe failed: %v", err)
	}

	if err := h.wotan.Publish(ctx, traceTopic, payload); err != nil {
		t.Fatalf("Failed to publish trace event: %v", err)
	}

	// Verify the consumer receives the event.
	select {
	case msg := <-msgCh:
		if msg.Topic != traceTopic {
			t.Errorf("Expected topic %q, got %q", traceTopic, msg.Topic)
		}

		var received map[string]interface{}
		if err := json.Unmarshal([]byte(msg.Payload), &received); err != nil {
			t.Fatalf("Failed to unmarshal received event: %v", err)
		}

		if received["trace_id"] != traceID.String() {
			t.Errorf("Trace ID mismatch: expected %s, got %v", traceID.String(), received["trace_id"])
		}

		t.Logf("Trace event propagated: trace_id=%s, latency_ns=%.0f",
			received["trace_id"], received["latency_ns"])

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for trace event propagation")
	}

	// Verify publish was recorded.
	if h.wotan.GetPublishCount() != 1 {
		t.Errorf("Expected 1 publish, got %d", h.wotan.GetPublishCount())
	}
}

// TestFullPipeline_ServiceDiscovery verifies that services register with the
// discovery registry and can be resolved by name.
func TestFullPipeline_ServiceDiscovery(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	for _, svc := range pipelineServices {
		t.Run("resolve_"+svc.name, func(t *testing.T) {
			entry, err := h.discoveryRegistry.Resolve(svc.name)
			if err != nil {
				t.Fatalf("Failed to resolve %s: %v", svc.name, err)
			}

			if entry.Name != svc.name {
				t.Errorf("Expected name %q, got %q", svc.name, entry.Name)
			}
			if entry.Health != "healthy" {
				t.Errorf("Expected health 'healthy', got %q", entry.Health)
			}
			if entry.Metadata["version"] != svc.version {
				t.Errorf("Expected version %q, got %q", svc.version, entry.Metadata["version"])
			}
		})
	}

	// Verify ResolveAll returns all services.
	t.Run("resolve_all", func(t *testing.T) {
		all := h.discoveryRegistry.ResolveAll()
		if len(all) < len(pipelineServices) {
			t.Errorf("Expected at least %d services, got %d", len(pipelineServices), len(all))
		}
	})
}

// TestFullPipeline_ServiceDiscoveryWatch verifies that the discovery registry
// notifies watchers when services register.
func TestFullPipeline_ServiceDiscoveryWatch(t *testing.T) {
	wotanDiscovery := newMockWotanDiscovery()
	registry := discovery.New(wotanDiscovery)

	ctx := context.Background()
	if err := registry.Start(ctx); err != nil {
		t.Fatalf("Failed to start registry: %v", err)
	}
	defer registry.Stop()

	var mu sync.Mutex
	received := make([]discovery.ServiceEntry, 0)

	registry.Watch(func(entry discovery.ServiceEntry) {
		mu.Lock()
		received = append(received, entry)
		mu.Unlock()
	})

	// Register a new service.
	if err := registry.Register(discovery.ServiceEntry{
		Name:    "test-service",
		Address: "http://localhost:9999",
		Port:    9999,
		Health:  "healthy",
	}); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Allow watcher goroutine to fire.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count == 0 {
		t.Error("Expected at least 1 watch notification")
	}
}

// TestFullPipeline_TraceIDGeneration verifies trace ID generation properties:
// uniqueness, non-zero, correct format, and flow label extraction.
func TestFullPipeline_TraceIDGeneration(t *testing.T) {
	const numIDs = 1000

	ids := make(map[string]struct{}, numIDs)
	for i := 0; i < numIDs; i++ {
		id, err := trace.NewTraceID()
		if err != nil {
			t.Fatalf("Failed to generate trace ID %d: %v", i, err)
		}

		if id.IsZero() {
			t.Fatalf("Trace ID %d is zero", i)
		}

		hexStr := id.String()
		if len(hexStr) != 32 {
			t.Fatalf("Expected 32 hex chars, got %d", len(hexStr))
		}

		if _, exists := ids[hexStr]; exists {
			t.Fatalf("Duplicate trace ID at iteration %d: %s", i, hexStr)
		}
		ids[hexStr] = struct{}{}

		// Flow label should be a valid 20-bit value.
		flowLabel := id.FlowLabelHint()
		if flowLabel > 0x000FFFFF {
			t.Errorf("Flow label exceeds 20 bits: %x", flowLabel)
		}

		// Timestamp should be recent.
		tsMS := id.TimestampMS()
		nowMS := time.Now().UnixMilli()
		if tsMS < nowMS-5000 || tsMS > nowMS+5000 {
			t.Errorf("Timestamp out of range: %d (now=%d)", tsMS, nowMS)
		}
	}

	t.Logf("Generated %d unique trace IDs", len(ids))
}

// TestFullPipeline_MultiServiceTraceCorrelation verifies that a single trace ID
// can be propagated across multiple services via Wotan.
func TestFullPipeline_MultiServiceTraceCorrelation(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	traceID, err := trace.NewTraceID()
	if err != nil {
		t.Fatalf("Failed to generate trace ID: %v", err)
	}

	// Simulate a request flowing through multiple services.
	// Each service publishes its span to the trace topic.
	traceTopic := "traces.spans"

	// Subscribe as the collector.
	_, err = h.wotan.Subscribe(ctx, traceTopic, "trace-collector")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	services := []string{"gateway", "timeguru", "wotan"}
	for _, svc := range services {
		span := map[string]interface{}{
			"trace_id":  traceID.String(),
			"service":   svc,
			"operation": "handle_request",
			"start_ns":  time.Now().UnixNano(),
			"duration":  1000000, // 1ms
		}
		payload, _ := json.Marshal(span)
		if err := h.wotan.Publish(ctx, traceTopic, payload); err != nil {
			t.Fatalf("Failed to publish span for %s: %v", svc, err)
		}
	}

	// Verify all spans were published.
	msgs, err := h.wotan.GetMessages(ctx, traceTopic, 0, 100)
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	if len(msgs) != len(services) {
		t.Errorf("Expected %d spans, got %d", len(services), len(msgs))
	}

	// Verify all spans share the same trace ID.
	for _, msg := range msgs {
		var span map[string]interface{}
		json.Unmarshal([]byte(msg.Payload), &span)

		if span["trace_id"] != traceID.String() {
			t.Errorf("Span trace_id mismatch: expected %s, got %v", traceID.String(), span["trace_id"])
		}
	}

	t.Logf("Correlated %d spans under trace %s", len(msgs), traceID.String())
}

// TestFullPipeline_ConcurrentHealthChecks verifies that all services can be
// health-checked concurrently without races.
func TestFullPipeline_ConcurrentHealthChecks(t *testing.T) {
	h := newPipelineTestHarness(t)
	defer h.Cleanup()

	var wg sync.WaitGroup
	errors := make(chan error, len(pipelineServices)*10)

	// Hit each service 10 times concurrently.
	for _, svc := range pipelineServices {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				server := h.servers[name]
				resp, err := http.Get(server.URL + "/health")
				if err != nil {
					errors <- fmt.Errorf("%s: %w", name, err)
					return
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					errors <- fmt.Errorf("%s: expected 200, got %d", name, resp.StatusCode)
				}
			}(svc.name)
		}
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}

	t.Logf("Completed %d concurrent health checks across %d services",
		len(pipelineServices)*10, len(pipelineServices))
}

// TestFullPipeline_mTLSEndToEnd verifies that a server configured with mTLS
// rejects connections without a valid client certificate.
func TestFullPipeline_mTLSEndToEnd(t *testing.T) {
	// This test uses the actual TLS package to set up a real mTLS server
	// and verifies that connections without client certs are rejected.
	// The test is self-contained and does not require any external services.

	// We test at the TLS connection level: a server requiring client certs
	// should reject a client that presents no certificate.
	// We use crypto/tls directly instead of the unheaded/pkg/tls package
	// to keep the test self-contained and avoid circular issues.

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/data", func(w http.ResponseWriter, r *http.Request) {
		// If we reach here, mTLS handshake passed.
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Create a basic TLS test server with mTLS required.
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{
		ClientAuth: tls.RequireAnyClientCert,
		MinVersion: tls.VersionTLS13,
	}
	server.StartTLS()
	defer server.Close()

	// Try connecting without a client cert -- should fail.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 5 * time.Second,
	}

	_, err := client.Get(server.URL + "/api/v1/data")
	if err == nil {
		t.Error("Expected connection to fail without client certificate, but it succeeded")
	} else {
		t.Logf("mTLS correctly rejected connection without client cert: %v", err)
	}
}
