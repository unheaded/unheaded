// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ==================== DNS Discovery Tests ====================

func TestDefaultDNSConfig(t *testing.T) {
	cfg := DefaultDNSConfig()
	if cfg.Domain != "svc.unheaded.local" {
		t.Errorf("expected domain svc.unheaded.local, got %s", cfg.Domain)
	}
	if cfg.DefaultPort != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.DefaultPort)
	}
	if cfg.RefreshRate != 30*time.Second {
		t.Errorf("expected refresh rate 30s, got %v", cfg.RefreshRate)
	}
}

func TestNewDNSDiscoverer_NilConfig(t *testing.T) {
	d := NewDNSDiscoverer(nil)
	defer d.Close()

	if d.domain != "svc.unheaded.local" {
		t.Errorf("expected default domain, got %s", d.domain)
	}
	if d.defaultPort != 8080 {
		t.Errorf("expected default port 8080, got %d", d.defaultPort)
	}
}

func TestNewDNSDiscoverer_CustomConfig(t *testing.T) {
	cfg := &DNSConfig{
		Domain:      "test.local",
		DefaultPort: 9090,
		RefreshRate: 5 * time.Second,
	}
	d := NewDNSDiscoverer(cfg)
	defer d.Close()

	if d.domain != "test.local" {
		t.Errorf("expected domain test.local, got %s", d.domain)
	}
	if d.defaultPort != 9090 {
		t.Errorf("expected port 9090, got %d", d.defaultPort)
	}
}

func TestNewDNSDiscoverer_CustomResolver(t *testing.T) {
	resolver := &net.Resolver{}
	cfg := &DNSConfig{
		Domain:      "test.local",
		DefaultPort: 8080,
		RefreshRate: 5 * time.Second,
		Resolver:    resolver,
	}
	d := NewDNSDiscoverer(cfg)
	defer d.Close()

	if d.resolver != resolver {
		t.Error("expected custom resolver to be used")
	}
}

func TestDNSDiscoverer_BuildDNSName(t *testing.T) {
	d := NewDNSDiscoverer(&DNSConfig{
		Domain:      "svc.local",
		DefaultPort: 8080,
		RefreshRate: 5 * time.Second,
	})
	defer d.Close()

	name := d.buildDNSName("api", "default")
	if name != "api.default.svc.local" {
		t.Errorf("expected api.default.svc.local, got %s", name)
	}
}

func TestDNSDiscoverer_Close(t *testing.T) {
	d := NewDNSDiscoverer(&DNSConfig{
		Domain:      "test.local",
		DefaultPort: 8080,
		RefreshRate: 5 * time.Second,
	})

	err := d.Close()
	if err != nil {
		t.Errorf("Close should not error, got: %v", err)
	}
}

func TestDNSDiscoverer_Discover_LookupFailure(t *testing.T) {
	// Use a resolver that will fail (non-existent domain)
	d := NewDNSDiscoverer(&DNSConfig{
		Domain:      "nonexistent.invalid.test",
		DefaultPort: 8080,
		RefreshRate: 30 * time.Second,
	})
	defer d.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := d.Discover(ctx, "api", "default")
	if err == nil {
		t.Error("expected error for nonexistent DNS lookup")
	}
}

func TestDNSDiscoverer_Discover_CacheHit(t *testing.T) {
	d := NewDNSDiscoverer(&DNSConfig{
		Domain:      "test.local",
		DefaultPort: 8080,
		RefreshRate: 1 * time.Hour,
	})
	defer d.Close()

	// Manually populate the cache
	svc := &Service{
		Name:      "api",
		Namespace: "default",
		Endpoints: []*Endpoint{{Address: "10.0.0.1", Port: 8080}},
	}
	d.mu.Lock()
	d.cache["default/api"] = &dnsCache{
		service:   svc,
		expiresAt: time.Now().Add(1 * time.Hour),
	}
	d.mu.Unlock()

	ctx := context.Background()
	got, err := d.Discover(ctx, "api", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "api" {
		t.Errorf("expected api, got %s", got.Name)
	}
}

func TestDNSDiscoverer_Watch(t *testing.T) {
	d := NewDNSDiscoverer(&DNSConfig{
		Domain:      "test.local",
		DefaultPort: 8080,
		RefreshRate: 1 * time.Hour,
	})
	defer d.Close()

	// Populate cache so Discover inside Watch succeeds
	svc := &Service{
		Name:      "api",
		Namespace: "default",
		Endpoints: []*Endpoint{{Address: "10.0.0.1", Port: 8080}},
	}
	d.mu.Lock()
	d.cache["default/api"] = &dnsCache{
		service:   svc,
		expiresAt: time.Now().Add(1 * time.Hour),
	}
	d.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	ch, err := d.Watch(ctx, "api", "default")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case got := <-ch:
		if got.Name != "api" {
			t.Errorf("expected api, got %s", got.Name)
		}
	case <-time.After(150 * time.Millisecond):
		t.Error("timeout waiting for watch result")
	}

	// Wait for context cancellation cleanup
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// ==================== HeadlessServiceDiscoverer Tests ====================

func TestNewHeadlessServiceDiscoverer(t *testing.T) {
	cfg := &DNSConfig{
		Domain:      "svc.local",
		DefaultPort: 8080,
		RefreshRate: 5 * time.Second,
	}
	h := NewHeadlessServiceDiscoverer(cfg)
	defer h.Close()

	if h.DNSDiscoverer == nil {
		t.Fatal("expected non-nil inner discoverer")
	}
	if h.domain != "svc.local" {
		t.Errorf("expected domain svc.local, got %s", h.domain)
	}
}

func TestHeadlessServiceDiscoverer_DiscoverPod_Failure(t *testing.T) {
	cfg := &DNSConfig{
		Domain:      "nonexistent.invalid.test",
		DefaultPort: 8080,
		RefreshRate: 30 * time.Second,
	}
	h := NewHeadlessServiceDiscoverer(cfg)
	defer h.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := h.DiscoverPod(ctx, "pod-0", "api", "default")
	if err == nil {
		t.Error("expected error for nonexistent DNS lookup")
	}
}

// ==================== ExternalNameDiscoverer Tests ====================

func TestNewExternalNameDiscoverer(t *testing.T) {
	cfg := &DNSConfig{
		Domain:      "svc.local",
		DefaultPort: 8080,
		RefreshRate: 5 * time.Second,
	}
	e := NewExternalNameDiscoverer(cfg)
	defer e.Close()

	if e.externalNames == nil {
		t.Fatal("expected externalNames map to be initialized")
	}
}

func TestExternalNameDiscoverer_SetExternalName(t *testing.T) {
	cfg := &DNSConfig{
		Domain:      "svc.local",
		DefaultPort: 8080,
		RefreshRate: 5 * time.Second,
	}
	e := NewExternalNameDiscoverer(cfg)
	defer e.Close()

	e.SetExternalName("db", "default", "rds.amazonaws.com")

	e.mu.RLock()
	name, ok := e.externalNames["default/db"]
	e.mu.RUnlock()

	if !ok {
		t.Fatal("expected external name to be set")
	}
	if name != "rds.amazonaws.com" {
		t.Errorf("expected rds.amazonaws.com, got %s", name)
	}
}

func TestExternalNameDiscoverer_Discover_WithExternalName(t *testing.T) {
	cfg := &DNSConfig{
		Domain:      "svc.local",
		DefaultPort: 8080,
		RefreshRate: 5 * time.Second,
	}
	e := NewExternalNameDiscoverer(cfg)
	defer e.Close()

	// Set external name to a non-resolvable domain
	e.SetExternalName("db", "default", "nonexistent.invalid.test")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := e.Discover(ctx, "db", "default")
	if err == nil {
		t.Error("expected error for nonexistent external name")
	}
}

func TestExternalNameDiscoverer_Discover_FallbackToStandard(t *testing.T) {
	cfg := &DNSConfig{
		Domain:      "nonexistent.invalid.test",
		DefaultPort: 8080,
		RefreshRate: 5 * time.Second,
	}
	e := NewExternalNameDiscoverer(cfg)
	defer e.Close()

	// No external name set, should fall back to standard DNS
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := e.Discover(ctx, "api", "default")
	if err == nil {
		t.Error("expected error for nonexistent DNS fallback")
	}
}

// ==================== ParseSRVRecord Tests ====================

func TestParseSRVRecord(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		ep, err := ParseSRVRecord("10 20 8080 api.svc.local.")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ep.Priority != 10 {
			t.Errorf("expected priority 10, got %d", ep.Priority)
		}
		if ep.Weight != 20 {
			t.Errorf("expected weight 20, got %d", ep.Weight)
		}
		if ep.Port != 8080 {
			t.Errorf("expected port 8080, got %d", ep.Port)
		}
		if ep.Target != "api.svc.local" {
			t.Errorf("expected target api.svc.local, got %s", ep.Target)
		}
	})

	t.Run("No trailing dot", func(t *testing.T) {
		ep, err := ParseSRVRecord("5 10 443 secure.svc.local")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ep.Target != "secure.svc.local" {
			t.Errorf("expected target secure.svc.local, got %s", ep.Target)
		}
	})

	t.Run("TooFewParts", func(t *testing.T) {
		_, err := ParseSRVRecord("10 20 8080")
		if err == nil {
			t.Error("expected error for too few parts")
		}
	})

	t.Run("InvalidPriority", func(t *testing.T) {
		_, err := ParseSRVRecord("abc 20 8080 api.svc.local")
		if err == nil {
			t.Error("expected error for invalid priority")
		}
	})

	t.Run("InvalidWeight", func(t *testing.T) {
		_, err := ParseSRVRecord("10 xyz 8080 api.svc.local")
		if err == nil {
			t.Error("expected error for invalid weight")
		}
	})

	t.Run("InvalidPort", func(t *testing.T) {
		_, err := ParseSRVRecord("10 20 notaport api.svc.local")
		if err == nil {
			t.Error("expected error for invalid port")
		}
	})

	t.Run("EmptyString", func(t *testing.T) {
		_, err := ParseSRVRecord("")
		if err == nil {
			t.Error("expected error for empty string")
		}
	})
}

// ==================== SortSRVByPriority Tests ====================

func TestSortSRVByPriority(t *testing.T) {
	t.Run("SortByPriority", func(t *testing.T) {
		eps := []*SRVEndpoint{
			{Target: "c", Priority: 30, Weight: 10},
			{Target: "a", Priority: 10, Weight: 10},
			{Target: "b", Priority: 20, Weight: 10},
		}
		SortSRVByPriority(eps)

		if eps[0].Target != "a" || eps[1].Target != "b" || eps[2].Target != "c" {
			t.Errorf("expected order a,b,c got %s,%s,%s", eps[0].Target, eps[1].Target, eps[2].Target)
		}
	})

	t.Run("SamePriority_SortByWeight", func(t *testing.T) {
		eps := []*SRVEndpoint{
			{Target: "low", Priority: 10, Weight: 5},
			{Target: "high", Priority: 10, Weight: 100},
			{Target: "mid", Priority: 10, Weight: 50},
		}
		SortSRVByPriority(eps)

		// Same priority: higher weight should come first
		if eps[0].Target != "high" {
			t.Errorf("expected high weight first, got %s (weight %d)", eps[0].Target, eps[0].Weight)
		}
		if eps[1].Target != "mid" {
			t.Errorf("expected mid weight second, got %s (weight %d)", eps[1].Target, eps[1].Weight)
		}
		if eps[2].Target != "low" {
			t.Errorf("expected low weight last, got %s (weight %d)", eps[2].Target, eps[2].Weight)
		}
	})

	t.Run("Empty", func(t *testing.T) {
		eps := []*SRVEndpoint{}
		SortSRVByPriority(eps) // Should not panic
	})

	t.Run("Single", func(t *testing.T) {
		eps := []*SRVEndpoint{{Target: "only", Priority: 1, Weight: 1}}
		SortSRVByPriority(eps) // Should not panic
		if eps[0].Target != "only" {
			t.Errorf("expected only, got %s", eps[0].Target)
		}
	})
}

// ==================== CachingDiscoverer Close Tests ====================

func TestCachingDiscoverer_Close(t *testing.T) {
	closeCalled := false
	backend := &mockDiscoverer{
		closeFn: func() error {
			closeCalled = true
			return nil
		},
	}

	cd := NewCachingDiscoverer(backend, 100*time.Millisecond)
	err := cd.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !closeCalled {
		t.Error("expected backend Close to be called")
	}
}

func TestCachingDiscoverer_Close_Error(t *testing.T) {
	expectedErr := errors.New("close failed")
	backend := &mockDiscoverer{
		closeFn: func() error { return expectedErr },
	}

	cd := NewCachingDiscoverer(backend, 100*time.Millisecond)
	err := cd.Close()
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// ==================== RegistryClient Additional Tests ====================

func TestRegistryClient_DefaultTimeout(t *testing.T) {
	config := &RegistryConfig{
		BaseURL: "http://localhost:8080",
	}
	client := NewRegistryClient(config)
	defer client.Close()

	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("expected default timeout 10s, got %v", client.httpClient.Timeout)
	}
}

func TestRegistryClient_Register_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	ctx := context.Background()
	svc := &Service{Name: "api", Namespace: "default"}
	err := client.Register(ctx, svc)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestRegistryClient_Deregister_404IsOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	ctx := context.Background()
	err := client.Deregister(ctx, "api", "default")
	if err != nil {
		t.Errorf("Deregister should not error on 404, got %v", err)
	}
}

func TestRegistryClient_Deregister_500IsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	ctx := context.Background()
	err := client.Deregister(ctx, "api", "default")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestRegistryClient_Heartbeat_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	ctx := context.Background()
	err := client.Heartbeat(ctx, "api", "default")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestRegistryClient_FetchService_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	ctx := context.Background()
	_, err := client.Discover(ctx, "api", "default")
	if err == nil {
		t.Error("expected error for 502 response")
	}
}

func TestRegistryClient_SetHeaders_NoToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"api","namespace":"default"}`))
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	ctx := context.Background()
	client.Discover(ctx, "api", "default")

	if gotAuth != "" {
		t.Errorf("expected no Authorization header, got %s", gotAuth)
	}
}

func TestRegistryClient_Discover_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("X-Version", "v1")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"api","namespace":"default"}`))
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	ctx := context.Background()

	// First call
	_, err := client.Discover(ctx, "api", "default")
	if err != nil {
		t.Fatalf("first Discover failed: %v", err)
	}

	// Second call should use cache
	_, err = client.Discover(ctx, "api", "default")
	if err != nil {
		t.Fatalf("second Discover failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 server call (cached second), got %d", callCount)
	}
}

func TestRegistryClient_Watch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Version", "v1")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"api","namespace":"default","endpoints":[{"address":"10.0.0.1","port":8080}]}`))
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	ch, err := client.Watch(ctx, "api", "default")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	select {
	case svc := <-ch:
		if svc.Name != "api" {
			t.Errorf("expected api, got %s", svc.Name)
		}
	case <-time.After(150 * time.Millisecond):
		t.Error("timeout waiting for watch result")
	}
}

// ==================== InstanceRegistrar Error Cases ====================

func TestInstanceRegistrar_Start_RegistrationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	instance := &ServiceInstance{
		ID:        "api-1",
		Service:   "api",
		Namespace: "default",
	}

	registrar := NewInstanceRegistrar(client, instance, 10*time.Second)
	err := registrar.Start(context.Background())
	if err == nil {
		t.Error("expected error when registration fails")
	}
}

func TestInstanceRegistrar_UpdateHealth_AllStatuses(t *testing.T) {
	var lastBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/instances/api-1/health" {
			body := make([]byte, 1024)
			n, _ := r.Body.Read(body)
			lastBody = body[:n]
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
	client := NewRegistryClient(config)
	defer client.Close()

	instance := &ServiceInstance{ID: "api-1"}
	registrar := NewInstanceRegistrar(client, instance, 10*time.Second)

	for _, status := range []HealthStatus{HealthPassing, HealthWarning, HealthCritical} {
		err := registrar.UpdateHealth(context.Background(), status)
		if err != nil {
			t.Errorf("UpdateHealth(%s) failed: %v", status, err)
		}

		var parsed map[string]string
		if err := json.Unmarshal(lastBody, &parsed); err != nil {
			t.Errorf("failed to parse body: %v", err)
			continue
		}
		if parsed["status"] != string(status) {
			t.Errorf("expected status %s, got %s", status, parsed["status"])
		}
	}
}

// ==================== CompositeDiscoverer Close Error Tests ====================

func TestCompositeDiscoverer_Close_MultipleErrors(t *testing.T) {
	err1 := errors.New("error 1")
	d1 := &mockDiscoverer{closeFn: func() error { return err1 }}
	d2 := &mockDiscoverer{closeFn: func() error { return errors.New("error 2") }}

	cd := NewCompositeDiscoverer(d1, d2)
	err := cd.Close()
	// Should return the first error
	if err != err1 {
		t.Errorf("expected first error, got %v", err)
	}
}

// ==================== ServiceRegistry Watch Edge Cases ====================

func TestServiceRegistry_Unwatch_NonExistent(t *testing.T) {
	r := NewServiceRegistry()
	// Should not panic
	r.Unwatch("nonexistent", "default")
}

func TestServiceRegistry_Register_NotifiesFullChannel(t *testing.T) {
	r := NewServiceRegistry()

	ch := r.Watch("api", "default")

	// Fill the channel buffer (capacity 10)
	for i := 0; i < 10; i++ {
		r.Register(&Service{Name: "api", Namespace: "default", Version: "v" + string(rune('0'+i))})
	}

	// One more should not block (select default path)
	done := make(chan struct{})
	go func() {
		r.Register(&Service{Name: "api", Namespace: "default", Version: "overflow"})
		close(done)
	}()

	select {
	case <-done:
		// OK - did not block
	case <-time.After(200 * time.Millisecond):
		t.Error("Register blocked on full watcher channel")
	}

	// Drain the channel
	drained := 0
	for {
		select {
		case <-ch:
			drained++
		default:
			goto drained_done
		}
	}
drained_done:
	if drained < 10 {
		t.Errorf("expected at least 10 drained events, got %d", drained)
	}
}

// ==================== SplitKey Additional Tests ====================

func TestSplitKey_WithMultipleSlashes(t *testing.T) {
	// splitKey should split on the first /
	result := splitKey("ns/svc/extra")
	if len(result) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(result))
	}
	if result[0] != "ns" {
		t.Errorf("expected first part 'ns', got '%s'", result[0])
	}
	if result[1] != "svc/extra" {
		t.Errorf("expected second part 'svc/extra', got '%s'", result[1])
	}
}

func TestSplitKey_Empty(t *testing.T) {
	result := splitKey("")
	if len(result) != 1 || result[0] != "" {
		t.Errorf("expected [''], got %v", result)
	}
}

func TestSplitKey_SlashOnly(t *testing.T) {
	result := splitKey("/")
	if len(result) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(result))
	}
	if result[0] != "" || result[1] != "" {
		t.Errorf("expected ['', ''], got %v", result)
	}
}
