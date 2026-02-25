// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ==================== StaticDiscoverer Tests ====================

func TestStaticDiscoverer(t *testing.T) {
	t.Run("NewStaticDiscoverer", func(t *testing.T) {
		d := NewStaticDiscoverer()
		if d == nil {
			t.Fatal("expected non-nil discoverer")
		}
		if d.services == nil {
			t.Fatal("expected services map to be initialized")
		}
	})

	t.Run("AddService_and_Discover", func(t *testing.T) {
		d := NewStaticDiscoverer()

		svc := &Service{
			Name:      "api",
			Namespace: "default",
			Endpoints: []*Endpoint{
				{Address: "10.0.0.1", Port: 8080, Healthy: true},
				{Address: "10.0.0.2", Port: 8080, Healthy: true},
			},
		}
		d.AddService(svc)

		ctx := context.Background()
		discovered, err := d.Discover(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if discovered.Name != "api" {
			t.Errorf("expected name 'api', got '%s'", discovered.Name)
		}
		if len(discovered.Endpoints) != 2 {
			t.Errorf("expected 2 endpoints, got %d", len(discovered.Endpoints))
		}
	})

	t.Run("Discover_NotFound", func(t *testing.T) {
		d := NewStaticDiscoverer()

		ctx := context.Background()
		_, err := d.Discover(ctx, "unknown", "default")
		if err != ErrServiceNotFound {
			t.Errorf("expected ErrServiceNotFound, got %v", err)
		}
	})

	t.Run("RemoveService", func(t *testing.T) {
		d := NewStaticDiscoverer()

		svc := &Service{Name: "api", Namespace: "default"}
		d.AddService(svc)

		// Verify exists
		ctx := context.Background()
		_, err := d.Discover(ctx, "api", "default")
		if err != nil {
			t.Fatalf("service should exist: %v", err)
		}

		// Remove
		d.RemoveService("api", "default")

		// Verify removed
		_, err = d.Discover(ctx, "api", "default")
		if err != ErrServiceNotFound {
			t.Errorf("expected ErrServiceNotFound after removal, got %v", err)
		}
	})

	t.Run("Watch", func(t *testing.T) {
		d := NewStaticDiscoverer()

		svc := &Service{
			Name:      "api",
			Namespace: "default",
			Endpoints: []*Endpoint{
				{Address: "10.0.0.1", Port: 8080, Healthy: true},
			},
		}
		d.AddService(svc)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		ch, err := d.Watch(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should receive initial state
		select {
		case received := <-ch:
			if received.Name != "api" {
				t.Errorf("expected service name 'api', got '%s'", received.Name)
			}
		case <-time.After(50 * time.Millisecond):
			t.Error("timeout waiting for initial service state")
		}
	})

	t.Run("Watch_NonExistent", func(t *testing.T) {
		d := NewStaticDiscoverer()

		ctx := context.Background()
		_, err := d.Watch(ctx, "unknown", "default")
		if err != ErrServiceNotFound {
			t.Errorf("expected ErrServiceNotFound, got %v", err)
		}
	})

	t.Run("Close", func(t *testing.T) {
		d := NewStaticDiscoverer()
		err := d.Close()
		if err != nil {
			t.Errorf("expected nil error from Close, got %v", err)
		}
	})

	t.Run("Concurrent_Access", func(t *testing.T) {
		d := NewStaticDiscoverer()

		var wg sync.WaitGroup
		ctx := context.Background()

		// Add services concurrently
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				svc := &Service{
					Name:      "svc",
					Namespace: "default",
					Endpoints: []*Endpoint{{Address: "10.0.0.1", Port: 8080 + idx}},
				}
				d.AddService(svc)
			}(i)
		}

		// Discover concurrently
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				d.Discover(ctx, "svc", "default")
			}()
		}

		wg.Wait()
	})
}

// ==================== CachingDiscoverer Tests ====================

func TestCachingDiscoverer(t *testing.T) {
	t.Run("NewCachingDiscoverer", func(t *testing.T) {
		backend := NewStaticDiscoverer()
		cd := NewCachingDiscoverer(backend, 100*time.Millisecond)

		if cd == nil {
			t.Fatal("expected non-nil caching discoverer")
		}
		if cd.ttl != 100*time.Millisecond {
			t.Errorf("expected TTL 100ms, got %v", cd.ttl)
		}
		if cd.negative != 25*time.Millisecond {
			t.Errorf("expected negative TTL 25ms, got %v", cd.negative)
		}
	})

	t.Run("Cache_Hit", func(t *testing.T) {
		backend := NewStaticDiscoverer()
		svc := &Service{
			Name:      "api",
			Namespace: "default",
			Endpoints: []*Endpoint{{Address: "10.0.0.1", Port: 8080}},
		}
		backend.AddService(svc)

		cd := NewCachingDiscoverer(backend, 1*time.Second)
		ctx := context.Background()

		// First call - cache miss
		_, err := cd.Discover(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Remove from backend
		backend.RemoveService("api", "default")

		// Second call - should use cache
		discovered, err := cd.Discover(ctx, "api", "default")
		if err != nil {
			t.Fatalf("expected cache hit, got error: %v", err)
		}
		if discovered.Name != "api" {
			t.Errorf("expected cached service, got %s", discovered.Name)
		}
	})

	t.Run("Cache_Expiry", func(t *testing.T) {
		backend := NewStaticDiscoverer()
		svc := &Service{Name: "api", Namespace: "default"}
		backend.AddService(svc)

		cd := NewCachingDiscoverer(backend, 50*time.Millisecond)
		ctx := context.Background()

		// First call - populate cache
		cd.Discover(ctx, "api", "default")

		// Remove from backend
		backend.RemoveService("api", "default")

		// Wait for cache expiry
		time.Sleep(100 * time.Millisecond)

		// Should hit backend and fail
		_, err := cd.Discover(ctx, "api", "default")
		if err != ErrServiceNotFound {
			t.Errorf("expected ErrServiceNotFound after cache expiry, got %v", err)
		}
	})

	t.Run("Negative_Cache", func(t *testing.T) {
		backend := NewStaticDiscoverer()
		cd := NewCachingDiscoverer(backend, 200*time.Millisecond) // negative TTL = 50ms

		ctx := context.Background()

		// First call - cache negative result
		_, err := cd.Discover(ctx, "unknown", "default")
		if err != ErrServiceNotFound {
			t.Fatalf("expected ErrServiceNotFound, got %v", err)
		}

		// Add service to backend
		svc := &Service{Name: "unknown", Namespace: "default"}
		backend.AddService(svc)

		// Immediately - should still return cached error
		_, err = cd.Discover(ctx, "unknown", "default")
		if err != ErrServiceNotFound {
			t.Errorf("expected cached negative result, got %v", err)
		}

		// Wait for negative cache expiry
		time.Sleep(100 * time.Millisecond)

		// Now should find the service
		_, err = cd.Discover(ctx, "unknown", "default")
		if err != nil {
			t.Errorf("expected to find service after negative cache expiry, got %v", err)
		}
	})

	t.Run("Invalidate", func(t *testing.T) {
		backend := NewStaticDiscoverer()
		svc := &Service{Name: "api", Namespace: "default"}
		backend.AddService(svc)

		cd := NewCachingDiscoverer(backend, 1*time.Hour)
		ctx := context.Background()

		// Populate cache
		cd.Discover(ctx, "api", "default")

		// Remove from backend
		backend.RemoveService("api", "default")

		// Invalidate cache
		cd.Invalidate("api", "default")

		// Should hit backend and fail
		_, err := cd.Discover(ctx, "api", "default")
		if err != ErrServiceNotFound {
			t.Errorf("expected ErrServiceNotFound after invalidation, got %v", err)
		}
	})

	t.Run("Watch_Delegates", func(t *testing.T) {
		backend := NewStaticDiscoverer()
		svc := &Service{Name: "api", Namespace: "default"}
		backend.AddService(svc)

		cd := NewCachingDiscoverer(backend, 100*time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		ch, err := cd.Watch(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should receive from backend
		select {
		case <-ch:
			// OK
		case <-time.After(30 * time.Millisecond):
			t.Error("timeout waiting for watch result")
		}
	})
}

// ==================== CompositeDiscoverer Tests ====================

func TestCompositeDiscoverer(t *testing.T) {
	t.Run("NewCompositeDiscoverer", func(t *testing.T) {
		d1 := NewStaticDiscoverer()
		d2 := NewStaticDiscoverer()
		cd := NewCompositeDiscoverer(d1, d2)

		if cd == nil {
			t.Fatal("expected non-nil composite discoverer")
		}
		if len(cd.discoverers) != 2 {
			t.Errorf("expected 2 discoverers, got %d", len(cd.discoverers))
		}
	})

	t.Run("Discover_FirstSucceeds", func(t *testing.T) {
		d1 := NewStaticDiscoverer()
		d2 := NewStaticDiscoverer()

		svc := &Service{Name: "api", Namespace: "default", Version: "v1"}
		d1.AddService(svc)

		cd := NewCompositeDiscoverer(d1, d2)
		ctx := context.Background()

		discovered, err := cd.Discover(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if discovered.Version != "v1" {
			t.Errorf("expected version v1, got %s", discovered.Version)
		}
	})

	t.Run("Discover_Fallback", func(t *testing.T) {
		d1 := NewStaticDiscoverer()
		d2 := NewStaticDiscoverer()

		svc := &Service{Name: "api", Namespace: "default", Version: "v2"}
		d2.AddService(svc) // Only in second discoverer

		cd := NewCompositeDiscoverer(d1, d2)
		ctx := context.Background()

		discovered, err := cd.Discover(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if discovered.Version != "v2" {
			t.Errorf("expected version v2 from fallback, got %s", discovered.Version)
		}
	})

	t.Run("Discover_AllFail", func(t *testing.T) {
		d1 := NewStaticDiscoverer()
		d2 := NewStaticDiscoverer()

		cd := NewCompositeDiscoverer(d1, d2)
		ctx := context.Background()

		_, err := cd.Discover(ctx, "unknown", "default")
		if err != ErrServiceNotFound {
			t.Errorf("expected ErrServiceNotFound, got %v", err)
		}
	})

	t.Run("Discover_EmptyDiscoverers", func(t *testing.T) {
		cd := NewCompositeDiscoverer()
		ctx := context.Background()

		_, err := cd.Discover(ctx, "api", "default")
		if err != ErrServiceNotFound {
			t.Errorf("expected ErrServiceNotFound for empty composite, got %v", err)
		}
	})

	t.Run("Watch_FirstAvailable", func(t *testing.T) {
		d1 := NewStaticDiscoverer()
		d2 := NewStaticDiscoverer()

		svc := &Service{Name: "api", Namespace: "default"}
		d1.AddService(svc)

		cd := NewCompositeDiscoverer(d1, d2)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		ch, err := cd.Watch(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		select {
		case <-ch:
			// OK
		case <-time.After(30 * time.Millisecond):
			t.Error("timeout waiting for watch")
		}
	})

	t.Run("Watch_AllUnavailable", func(t *testing.T) {
		d1 := NewStaticDiscoverer()
		d2 := NewStaticDiscoverer()

		cd := NewCompositeDiscoverer(d1, d2)

		ctx := context.Background()
		_, err := cd.Watch(ctx, "unknown", "default")
		if err != ErrDiscoveryUnavailable {
			t.Errorf("expected ErrDiscoveryUnavailable, got %v", err)
		}
	})

	t.Run("Close_ClosesAll", func(t *testing.T) {
		d1 := NewStaticDiscoverer()
		d2 := NewStaticDiscoverer()

		cd := NewCompositeDiscoverer(d1, d2)
		err := cd.Close()
		if err != nil {
			t.Errorf("unexpected error from Close: %v", err)
		}
	})
}

// ==================== ServiceRegistry Tests ====================

func TestServiceRegistry(t *testing.T) {
	t.Run("NewServiceRegistry", func(t *testing.T) {
		r := NewServiceRegistry()
		if r == nil {
			t.Fatal("expected non-nil registry")
		}
	})

	t.Run("Register_and_Get", func(t *testing.T) {
		r := NewServiceRegistry()

		svc := &Service{
			Name:      "api",
			Namespace: "production",
			Endpoints: []*Endpoint{{Address: "10.0.0.1", Port: 8080}},
		}
		r.Register(svc)

		got, ok := r.Get("api", "production")
		if !ok {
			t.Fatal("expected to find registered service")
		}
		if got.Name != "api" {
			t.Errorf("expected name 'api', got '%s'", got.Name)
		}
		if got.UpdatedAt.IsZero() {
			t.Error("expected UpdatedAt to be set")
		}
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		r := NewServiceRegistry()

		_, ok := r.Get("unknown", "default")
		if ok {
			t.Error("expected not to find non-existent service")
		}
	})

	t.Run("Deregister", func(t *testing.T) {
		r := NewServiceRegistry()

		svc := &Service{Name: "api", Namespace: "default"}
		r.Register(svc)

		_, ok := r.Get("api", "default")
		if !ok {
			t.Fatal("service should exist before deregister")
		}

		r.Deregister("api", "default")

		_, ok = r.Get("api", "default")
		if ok {
			t.Error("service should not exist after deregister")
		}
	})

	t.Run("List", func(t *testing.T) {
		r := NewServiceRegistry()

		r.Register(&Service{Name: "api", Namespace: "default"})
		r.Register(&Service{Name: "web", Namespace: "default"})
		r.Register(&Service{Name: "db", Namespace: "production"})

		list := r.List()
		if len(list) != 3 {
			t.Errorf("expected 3 services, got %d", len(list))
		}
	})

	t.Run("Watch_Receives_Updates", func(t *testing.T) {
		r := NewServiceRegistry()

		ch := r.Watch("api", "default")

		// Register after watch
		go func() {
			time.Sleep(10 * time.Millisecond)
			r.Register(&Service{Name: "api", Namespace: "default", Version: "v1"})
		}()

		select {
		case svc := <-ch:
			if svc.Version != "v1" {
				t.Errorf("expected version v1, got %s", svc.Version)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for watch update")
		}
	})

	t.Run("Watch_Existing_Service", func(t *testing.T) {
		r := NewServiceRegistry()

		// Register before watch
		r.Register(&Service{Name: "api", Namespace: "default", Version: "v1"})

		ch := r.Watch("api", "default")

		// Should receive current state immediately
		select {
		case svc := <-ch:
			if svc.Version != "v1" {
				t.Errorf("expected version v1, got %s", svc.Version)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for initial state")
		}
	})

	t.Run("Watch_Multiple_Watchers", func(t *testing.T) {
		r := NewServiceRegistry()

		ch1 := r.Watch("api", "default")
		ch2 := r.Watch("api", "default")

		r.Register(&Service{Name: "api", Namespace: "default", Version: "v1"})

		received := 0
		timeout := time.After(100 * time.Millisecond)

	loop:
		for received < 2 {
			select {
			case <-ch1:
				received++
			case <-ch2:
				received++
			case <-timeout:
				break loop
			}
		}

		if received != 2 {
			t.Errorf("expected 2 watchers to receive update, got %d", received)
		}
	})

	t.Run("Unwatch", func(t *testing.T) {
		r := NewServiceRegistry()

		ch := r.Watch("api", "default")

		// Unwatch should close the channel
		r.Unwatch("api", "default")

		// Channel should be closed
		_, open := <-ch
		if open {
			t.Error("expected channel to be closed after unwatch")
		}
	})

	t.Run("Concurrent_Access", func(t *testing.T) {
		r := NewServiceRegistry()

		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				svc := &Service{Name: "svc", Namespace: "default", Version: "v" + string(rune('0'+idx%10))}
				r.Register(svc)
				r.Get("svc", "default")
			}(i)
		}
		wg.Wait()
	})
}

// ==================== Endpoint Tests ====================

func TestEndpoint(t *testing.T) {
	t.Run("Endpoint_Fields", func(t *testing.T) {
		ep := &Endpoint{
			Address:   "10.0.0.1",
			Port:      8080,
			Weight:    10,
			Healthy:   true,
			Zone:      "us-east-1a",
			Tags:      map[string]string{"version": "v1"},
			LastSeen:  time.Now(),
			ServiceID: "api-1",
		}

		if ep.Address != "10.0.0.1" {
			t.Errorf("expected address 10.0.0.1, got %s", ep.Address)
		}
		if ep.Port != 8080 {
			t.Errorf("expected port 8080, got %d", ep.Port)
		}
		if ep.Weight != 10 {
			t.Errorf("expected weight 10, got %d", ep.Weight)
		}
		if !ep.Healthy {
			t.Error("expected healthy to be true")
		}
		if ep.Tags["version"] != "v1" {
			t.Errorf("expected tag version=v1, got %s", ep.Tags["version"])
		}
	})
}

// ==================== Service Tests ====================

func TestService(t *testing.T) {
	t.Run("Service_Fields", func(t *testing.T) {
		svc := &Service{
			Name:      "api",
			Namespace: "production",
			Endpoints: []*Endpoint{
				{Address: "10.0.0.1", Port: 8080},
				{Address: "10.0.0.2", Port: 8080},
			},
			Protocol:  "http",
			Version:   "v1",
			Meta:      map[string]string{"region": "us-east"},
			UpdatedAt: time.Now(),
		}

		if svc.Name != "api" {
			t.Errorf("expected name api, got %s", svc.Name)
		}
		if svc.Namespace != "production" {
			t.Errorf("expected namespace production, got %s", svc.Namespace)
		}
		if len(svc.Endpoints) != 2 {
			t.Errorf("expected 2 endpoints, got %d", len(svc.Endpoints))
		}
		if svc.Protocol != "http" {
			t.Errorf("expected protocol http, got %s", svc.Protocol)
		}
		if svc.Meta["region"] != "us-east" {
			t.Errorf("expected meta region=us-east, got %s", svc.Meta["region"])
		}
	})
}

// ==================== Error Tests ====================

func TestErrors(t *testing.T) {
	t.Run("ErrServiceNotFound", func(t *testing.T) {
		if ErrServiceNotFound.Error() != "service not found" {
			t.Errorf("unexpected error message: %s", ErrServiceNotFound.Error())
		}
	})

	t.Run("ErrNoEndpoints", func(t *testing.T) {
		if ErrNoEndpoints.Error() != "no endpoints available" {
			t.Errorf("unexpected error message: %s", ErrNoEndpoints.Error())
		}
	})

	t.Run("ErrDiscoveryUnavailable", func(t *testing.T) {
		if ErrDiscoveryUnavailable.Error() != "discovery service unavailable" {
			t.Errorf("unexpected error message: %s", ErrDiscoveryUnavailable.Error())
		}
	})
}

// ==================== Mock Discoverer for Testing ====================

type mockDiscoverer struct {
	discoverFn func(ctx context.Context, service, namespace string) (*Service, error)
	watchFn    func(ctx context.Context, service, namespace string) (<-chan *Service, error)
	closeFn    func() error
}

func (m *mockDiscoverer) Discover(ctx context.Context, service, namespace string) (*Service, error) {
	if m.discoverFn != nil {
		return m.discoverFn(ctx, service, namespace)
	}
	return nil, ErrServiceNotFound
}

func (m *mockDiscoverer) Watch(ctx context.Context, service, namespace string) (<-chan *Service, error) {
	if m.watchFn != nil {
		return m.watchFn(ctx, service, namespace)
	}
	return nil, ErrDiscoveryUnavailable
}

func (m *mockDiscoverer) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func TestCompositeDiscoverer_WithMockFailures(t *testing.T) {
	t.Run("Close_ReturnsFirstError", func(t *testing.T) {
		expectedErr := errors.New("close error")
		d1 := &mockDiscoverer{
			closeFn: func() error { return expectedErr },
		}
		d2 := &mockDiscoverer{
			closeFn: func() error { return nil },
		}

		cd := NewCompositeDiscoverer(d1, d2)
		err := cd.Close()
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

// ==================== RegistryClient Tests (with Mock Server) ====================

func TestRegistryClient(t *testing.T) {
	t.Run("NewRegistryClient", func(t *testing.T) {
		config := &RegistryConfig{
			BaseURL:         "http://localhost:8080",
			Token:           "test-token",
			Timeout:         5 * time.Second,
			RefreshInterval: 10 * time.Second,
		}

		client := NewRegistryClient(config)
		defer client.Close()

		if client == nil {
			t.Fatal("expected non-nil client")
		}
		if client.baseURL != "http://localhost:8080" {
			t.Errorf("expected baseURL http://localhost:8080, got %s", client.baseURL)
		}
		if client.token != "test-token" {
			t.Errorf("expected token test-token, got %s", client.token)
		}
	})

	t.Run("Discover_Success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/services/default/api" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer test-token" {
				t.Errorf("missing or invalid auth header")
			}

			w.Header().Set("X-Version", "v1")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"name":"api","namespace":"default","endpoints":[{"address":"10.0.0.1","port":8080}]}`))
		}))
		defer server.Close()

		config := &RegistryConfig{
			BaseURL: server.URL,
			Token:   "test-token",
			Timeout: 5 * time.Second,
		}

		client := NewRegistryClient(config)
		defer client.Close()

		ctx := context.Background()
		svc, err := client.Discover(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if svc.Name != "api" {
			t.Errorf("expected name api, got %s", svc.Name)
		}
	})

	t.Run("Discover_NotFound", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		config := &RegistryConfig{
			BaseURL: server.URL,
			Timeout: 5 * time.Second,
		}

		client := NewRegistryClient(config)
		defer client.Close()

		ctx := context.Background()
		_, err := client.Discover(ctx, "unknown", "default")
		if err != ErrServiceNotFound {
			t.Errorf("expected ErrServiceNotFound, got %v", err)
		}
	})

	t.Run("Register_Success", func(t *testing.T) {
		var receivedMethod string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := &RegistryConfig{
			BaseURL: server.URL,
			Timeout: 5 * time.Second,
		}

		client := NewRegistryClient(config)
		defer client.Close()

		ctx := context.Background()
		svc := &Service{Name: "api", Namespace: "default"}
		err := client.Register(ctx, svc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedMethod != http.MethodPut {
			t.Errorf("expected PUT method, got %s", receivedMethod)
		}
	})

	t.Run("Deregister_Success", func(t *testing.T) {
		var receivedMethod string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := &RegistryConfig{
			BaseURL: server.URL,
			Timeout: 5 * time.Second,
		}

		client := NewRegistryClient(config)
		defer client.Close()

		ctx := context.Background()
		err := client.Deregister(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedMethod != http.MethodDelete {
			t.Errorf("expected DELETE method, got %s", receivedMethod)
		}
	})

	t.Run("Heartbeat_Success", func(t *testing.T) {
		var receivedPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := &RegistryConfig{
			BaseURL: server.URL,
			Timeout: 5 * time.Second,
		}

		client := NewRegistryClient(config)
		defer client.Close()

		ctx := context.Background()
		err := client.Heartbeat(ctx, "api", "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedPath != "/v1/services/default/api/heartbeat" {
			t.Errorf("unexpected path: %s", receivedPath)
		}
	})
}

// ==================== InstanceRegistrar Tests ====================

func TestInstanceRegistrar(t *testing.T) {
	t.Run("NewInstanceRegistrar", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := &RegistryConfig{BaseURL: server.URL, Timeout: 5 * time.Second}
		client := NewRegistryClient(config)
		defer client.Close()

		instance := &ServiceInstance{
			ID:        "api-1",
			Service:   "api",
			Namespace: "default",
			Address:   "10.0.0.1",
			Port:      8080,
		}

		registrar := NewInstanceRegistrar(client, instance, 10*time.Second)
		if registrar == nil {
			t.Fatal("expected non-nil registrar")
		}
		if registrar.interval != 10*time.Second {
			t.Errorf("expected interval 10s, got %v", registrar.interval)
		}
	})

	t.Run("Start_and_Stop", func(t *testing.T) {
		var registerCalled bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut {
				registerCalled = true
			}
			w.WriteHeader(http.StatusOK)
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

		registrar := NewInstanceRegistrar(client, instance, 100*time.Millisecond)

		ctx := context.Background()
		err := registrar.Start(ctx)
		if err != nil {
			t.Fatalf("unexpected error starting registrar: %v", err)
		}

		if !registerCalled {
			t.Error("expected register to be called")
		}

		err = registrar.Stop(ctx)
		if err != nil {
			t.Errorf("unexpected error stopping registrar: %v", err)
		}
	})

	t.Run("UpdateHealth", func(t *testing.T) {
		var receivedPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
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

		ctx := context.Background()
		err := registrar.UpdateHealth(ctx, HealthPassing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedPath != "/v1/instances/api-1/health" {
			t.Errorf("unexpected path: %s", receivedPath)
		}
	})
}

// ==================== HealthStatus Tests ====================

func TestHealthStatus(t *testing.T) {
	tests := []struct {
		status   HealthStatus
		expected string
	}{
		{HealthPassing, "passing"},
		{HealthWarning, "warning"},
		{HealthCritical, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.status))
			}
		})
	}
}

// ==================== splitKey Tests ====================

func TestSplitKey(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"default/api", []string{"default", "api"}},
		{"production/web-service", []string{"production", "web-service"}},
		{"nodelimiter", []string{"nodelimiter"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitKey(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d parts, got %d", len(tt.expected), len(result))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("expected part %d to be %s, got %s", i, tt.expected[i], result[i])
				}
			}
		})
	}
}
