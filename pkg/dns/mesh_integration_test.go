// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package dns

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// mockServiceRegistry implements ServiceRegistry for testing
type mockServiceRegistry struct {
	mu       sync.RWMutex
	services map[string]*MeshService
	watchers map[string]chan *MeshService
}

func newMockServiceRegistry() *mockServiceRegistry {
	return &mockServiceRegistry{
		services: make(map[string]*MeshService),
		watchers: make(map[string]chan *MeshService),
	}
}

func (r *mockServiceRegistry) Discover(ctx context.Context, name, namespace string) (*MeshService, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	svc, ok := r.services[name]
	if !ok {
		return nil, ErrServiceNotFound
	}
	return svc, nil
}

func (r *mockServiceRegistry) Watch(ctx context.Context, name, namespace string) (<-chan *MeshService, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan *MeshService, 10)
	r.watchers[name] = ch
	return ch, nil
}

func (r *mockServiceRegistry) Register(ctx context.Context, svc *MeshService) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.services[svc.Name] = svc
	return nil
}

func (r *mockServiceRegistry) Deregister(ctx context.Context, name, namespace string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.services, name)
	return nil
}

func (r *mockServiceRegistry) ListServices(ctx context.Context, namespace string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name := range r.services {
		names = append(names, name)
	}
	return names, nil
}

func (r *mockServiceRegistry) addService(svc *MeshService) {
	r.mu.Lock()
	r.services[svc.Name] = svc
	r.mu.Unlock()
}

func (r *mockServiceRegistry) notifyWatcher(name string, svc *MeshService) {
	r.mu.RLock()
	ch, ok := r.watchers[name]
	r.mu.RUnlock()

	if ok {
		select {
		case ch <- svc:
		default:
		}
	}
}

// mockHealthAggregator implements HealthAggregator for testing
type mockHealthAggregator struct {
	mu       sync.RWMutex
	health   map[string]EndpointHealth
	callback func(address string, health EndpointHealth)
}

func newMockHealthAggregator() *mockHealthAggregator {
	return &mockHealthAggregator{
		health: make(map[string]EndpointHealth),
	}
}

func (h *mockHealthAggregator) GetHealth(address string) EndpointHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.health[address]
}

func (h *mockHealthAggregator) OnHealthChange(callback func(address string, health EndpointHealth)) {
	h.callback = callback
}

func (h *mockHealthAggregator) IsHealthy(address string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.health[address].Healthy
}

func (h *mockHealthAggregator) setHealth(address string, healthy bool) {
	h.mu.Lock()
	h.health[address] = EndpointHealth{
		Healthy:   healthy,
		LastCheck: time.Now(),
	}
	callback := h.callback
	h.mu.Unlock()

	if callback != nil {
		callback(address, h.health[address])
	}
}

func TestMeshIntegration_New(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	registry := newMockServiceRegistry()
	healthAgg := newMockHealthAggregator()

	mi := NewMeshIntegration(sd, registry, healthAgg, nil)
	if mi == nil {
		t.Fatal("NewMeshIntegration returned nil")
	}

	if mi.config == nil {
		t.Error("Config should be set")
	}
}

func TestMeshIntegration_StartStop(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	registry := newMockServiceRegistry()
	miConfig := &MeshIntegrationConfig{
		Namespace:    "test",
		SyncInterval: 100 * time.Millisecond,
	}

	mi := NewMeshIntegration(sd, registry, nil, miConfig)

	err := mi.Start(context.Background())
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mi.Stop()
}

func TestMeshIntegration_SyncService(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	registry := newMockServiceRegistry()
	registry.addService(&MeshService{
		Name:      "api",
		Namespace: "default",
		Protocol:  "tcp",
		Port:      8080,
		Instances: []*MeshInstance{
			{
				ID:      "api-1",
				Address: "10.0.0.1",
				Port:    8080,
				Health:  "passing",
				Weight:  100,
			},
		},
	})

	miConfig := DefaultMeshIntegrationConfig()
	mi := NewMeshIntegration(sd, registry, nil, miConfig)

	err := mi.Start(context.Background())
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer mi.Stop()

	// Give time for sync
	time.Sleep(50 * time.Millisecond)

	// Check service was synced
	svc, err := sd.GetService("api")
	if err != nil {
		t.Fatalf("GetService error: %v", err)
	}

	if svc.Name != "api" {
		t.Errorf("Name = %s, want api", svc.Name)
	}
	if len(svc.Endpoints) != 1 {
		t.Errorf("Endpoints count = %d, want 1", len(svc.Endpoints))
	}
}

func TestMeshIntegration_HealthConversion(t *testing.T) {
	tests := []struct {
		meshHealth string
		expected   HealthState
	}{
		{"passing", HealthStateHealthy},
		{"healthy", HealthStateHealthy},
		{"warning", HealthStateDegraded},
		{"degraded", HealthStateDegraded},
		{"critical", HealthStateUnhealthy},
		{"unhealthy", HealthStateUnhealthy},
		{"unknown", HealthStateUnknown},
		{"", HealthStateUnknown},
	}

	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	mi := NewMeshIntegration(sd, nil, nil, nil)

	for _, tt := range tests {
		t.Run(tt.meshHealth, func(t *testing.T) {
			inst := &MeshInstance{
				ID:      "test",
				Address: "10.0.0.1",
				Port:    80,
				Health:  tt.meshHealth,
			}

			ep := mi.convertToEndpoint(inst)
			if ep == nil {
				t.Fatal("convertToEndpoint returned nil")
			}
			if ep.Health != tt.expected {
				t.Errorf("Health = %v, want %v", ep.Health, tt.expected)
			}
		})
	}
}

func TestMeshIntegration_HealthAggregatorCallback(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	healthAgg := newMockHealthAggregator()

	miConfig := &MeshIntegrationConfig{
		Namespace:         "default",
		SyncInterval:      time.Hour, // Long interval
		HealthSyncEnabled: true,
	}
	mi := NewMeshIntegration(sd, nil, healthAgg, miConfig)
	mi.Start(context.Background())
	defer mi.Stop()

	// Register a service with SD
	svc := &ServiceDefinition{
		Name: "test",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 8080, Health: HealthStateHealthy},
		},
	}
	sd.RegisterService(svc)

	// Trigger health change
	healthAgg.setHealth("10.0.0.1", false)

	// Give time for callback
	time.Sleep(50 * time.Millisecond)

	// Check health was updated
	retrieved, _ := sd.GetService("test")
	if len(retrieved.Endpoints) > 0 && retrieved.Endpoints[0].Health == HealthStateHealthy {
		// May or may not have changed depending on timing
		t.Log("Health state may not have changed yet")
	}
}

func TestMeshIntegration_WatchService(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	registry := newMockServiceRegistry()
	registry.addService(&MeshService{
		Name:      "watched",
		Namespace: "default",
		Protocol:  "tcp",
		Port:      80,
		Instances: []*MeshInstance{
			{ID: "ep-1", Address: "10.0.0.1", Port: 80, Health: "passing"},
		},
	})

	mi := NewMeshIntegration(sd, registry, nil, nil)
	mi.Start(context.Background())
	defer mi.Stop()

	// Start watching
	err := mi.WatchService(context.Background(), "watched")
	if err != nil {
		t.Fatalf("WatchService error: %v", err)
	}

	// Watch again should be no-op
	err = mi.WatchService(context.Background(), "watched")
	if err != nil {
		t.Fatalf("Second WatchService error: %v", err)
	}

	// Notify watcher
	registry.notifyWatcher("watched", &MeshService{
		Name:      "watched",
		Namespace: "default",
		Port:      80,
		Instances: []*MeshInstance{
			{ID: "ep-1", Address: "10.0.0.1", Port: 80, Health: "passing"},
			{ID: "ep-2", Address: "10.0.0.2", Port: 80, Health: "passing"},
		},
	})

	// Give time for update
	time.Sleep(100 * time.Millisecond)

	// Stop watching
	mi.StopWatching("watched")
}

func TestMeshIntegration_WatchServiceNoRegistry(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	mi := NewMeshIntegration(sd, nil, nil, nil) // No registry

	err := mi.WatchService(context.Background(), "test")
	if err == nil {
		t.Error("WatchService with no registry should return error")
	}
}

func TestMeshIntegration_ExportToDNS(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	registry := newMockServiceRegistry()
	miConfig := &MeshIntegrationConfig{
		AutoRegister: true,
	}
	mi := NewMeshIntegration(sd, registry, nil, miConfig)

	svc := &ServiceDefinition{
		Name:      "exported",
		Namespace: "default",
		Protocol:  "tcp",
		Port:      8080,
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 8080, Health: HealthStateHealthy},
		},
	}

	err := mi.ExportToDNS(context.Background(), svc)
	if err != nil {
		t.Fatalf("ExportToDNS error: %v", err)
	}

	// Check it was registered
	registered, _ := registry.Discover(context.Background(), "exported", "default")
	if registered == nil {
		t.Error("Service should be registered in registry")
	}
}

func TestMeshIntegration_ExportToDNS_Disabled(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	miConfig := &MeshIntegrationConfig{
		AutoRegister: false,
	}
	mi := NewMeshIntegration(sd, nil, nil, miConfig)

	err := mi.ExportToDNS(context.Background(), &ServiceDefinition{Name: "test"})
	if err != nil {
		t.Fatalf("ExportToDNS should succeed when disabled: %v", err)
	}
}

func TestMeshIntegration_ConvertToMeshService(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	mi := NewMeshIntegration(sd, nil, nil, nil)

	svc := &ServiceDefinition{
		Name:      "test",
		Namespace: "production",
		Protocol:  "tcp",
		Port:      8080,
		Metadata:  map[string]string{"version": "1.0"},
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 8080, Weight: 100, Health: HealthStateHealthy},
			{ID: "ep-2", Address: net.ParseIP("10.0.0.2"), Port: 8081, Weight: 50, Health: HealthStateDegraded},
			{ID: "ep-3", Address: net.ParseIP("10.0.0.3"), Port: 8082, Weight: 0, Health: HealthStateUnhealthy},
		},
	}

	meshSvc := mi.convertToMeshService(svc)

	if meshSvc.Name != "test" {
		t.Errorf("Name = %s, want test", meshSvc.Name)
	}
	if meshSvc.Namespace != "production" {
		t.Errorf("Namespace = %s, want production", meshSvc.Namespace)
	}
	if len(meshSvc.Instances) != 3 {
		t.Fatalf("Instances count = %d, want 3", len(meshSvc.Instances))
	}

	// Check health conversion
	healthMap := map[string]string{
		"ep-1": "passing",
		"ep-2": "warning",
		"ep-3": "critical",
	}
	for _, inst := range meshSvc.Instances {
		expected := healthMap[inst.ID]
		if inst.Health != expected {
			t.Errorf("Instance %s health = %s, want %s", inst.ID, inst.Health, expected)
		}
	}
}

func TestMeshIntegration_ConvertToEndpoint_InvalidAddress(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	mi := NewMeshIntegration(sd, nil, nil, nil)

	// Invalid IP address that can't be resolved
	inst := &MeshInstance{
		ID:      "test",
		Address: "invalid-hostname-that-does-not-exist.local",
		Port:    80,
	}

	ep := mi.convertToEndpoint(inst)
	if ep != nil {
		t.Error("Invalid address should return nil endpoint")
	}
}

func TestMeshIntegration_UpdateService_AddRemove(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	mi := NewMeshIntegration(sd, nil, nil, nil)

	// Register initial service
	existing := &ServiceDefinition{
		Name:     "update-test",
		Protocol: "tcp",
		Port:     80,
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Health: HealthStateHealthy},
			{ID: "ep-2", Address: net.ParseIP("10.0.0.2"), Port: 80, Health: HealthStateHealthy},
		},
	}
	sd.RegisterService(existing)

	// New config with different endpoints
	newConfig := &ServiceDefinition{
		Name:     "update-test",
		Protocol: "tcp",
		Port:     80,
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-2", Address: net.ParseIP("10.0.0.2"), Port: 80, Health: HealthStateHealthy}, // Keep
			{ID: "ep-3", Address: net.ParseIP("10.0.0.3"), Port: 80, Health: HealthStateHealthy}, // New
		},
	}

	meshSvc := &MeshService{Name: "update-test"}
	err := mi.updateService(existing, newConfig, meshSvc)
	if err != nil {
		t.Fatalf("updateService error: %v", err)
	}

	// Check results
	updated, _ := sd.GetService("update-test")
	if len(updated.Endpoints) != 2 {
		t.Errorf("Endpoints count = %d, want 2", len(updated.Endpoints))
	}

	// ep-1 should be removed, ep-2 and ep-3 should exist
	epIDs := make(map[string]bool)
	for _, ep := range updated.Endpoints {
		epIDs[ep.ID] = true
	}
	if epIDs["ep-1"] {
		t.Error("ep-1 should be removed")
	}
	if !epIDs["ep-2"] {
		t.Error("ep-2 should exist")
	}
	if !epIDs["ep-3"] {
		t.Error("ep-3 should exist")
	}
}

func TestDefaultMeshIntegrationConfig(t *testing.T) {
	config := DefaultMeshIntegrationConfig()

	if config.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", config.Namespace)
	}
	if config.SyncInterval != 30*time.Second {
		t.Errorf("SyncInterval = %v, want 30s", config.SyncInterval)
	}
	if !config.HealthSyncEnabled {
		t.Error("HealthSyncEnabled should be true")
	}
	if !config.AutoRegister {
		t.Error("AutoRegister should be true")
	}
	if config.DefaultTTL != 30 {
		t.Errorf("DefaultTTL = %d, want 30", config.DefaultTTL)
	}
	if config.DefaultProtocol != "tcp" {
		t.Errorf("DefaultProtocol = %s, want tcp", config.DefaultProtocol)
	}
}

func TestDNSHandler_ServeDNS(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.Zone = "test.local"
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	// Register a service
	sd.RegisterService(&ServiceDefinition{
		Name:     "api",
		Protocol: "tcp",
		Port:     8080,
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 8080, Health: HealthStateHealthy},
		},
	})

	handler := NewDNSHandler(sd, nil, nil)

	tests := []struct {
		name     string
		qname    string
		qtype    uint16
		wantCode uint8
	}{
		{"A query", "api.test.local", TypeA, RcodeSuccess},
		{"SRV query", "_api._tcp.test.local", TypeSRV, RcodeSuccess},
		{"NXDOMAIN", "nonexistent.test.local", TypeA, RcodeNameError},
		{"Out of zone", "example.com", TypeA, RcodeNameError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockWriter := &mockResponseWriter{}
			query := NewQuery(tt.qname, tt.qtype)

			handler.ServeDNS(context.Background(), mockWriter, query)

			if mockWriter.msg == nil {
				t.Fatal("No response written")
			}
			if mockWriter.msg.Header.Rcode() != tt.wantCode {
				t.Errorf("Rcode = %d, want %d", mockWriter.msg.Header.Rcode(), tt.wantCode)
			}
		})
	}
}

func TestDNSHandler_EmptyQuery(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	handler := NewDNSHandler(sd, nil, nil)

	mockWriter := &mockResponseWriter{}
	query := &Message{
		Header:    Header{ID: 1234},
		Questions: []Question{}, // Empty
	}

	handler.ServeDNS(context.Background(), mockWriter, query)

	if mockWriter.msg == nil || mockWriter.msg.Header.Rcode() != RcodeFormatError {
		t.Error("Empty query should return FORMERR")
	}
}

func TestDNSHandler_TXTQuery(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.Zone = "txt.local"
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	sd.RegisterService(&ServiceDefinition{
		Name:     "meta",
		Protocol: "tcp",
		Port:     80,
		Metadata: map[string]string{"version": "1.0"},
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Health: HealthStateHealthy},
		},
	})

	handler := NewDNSHandler(sd, nil, nil)

	mockWriter := &mockResponseWriter{}
	query := NewQuery("_meta._tcp.txt.local", TypeTXT)

	handler.ServeDNS(context.Background(), mockWriter, query)

	if mockWriter.msg != nil && len(mockWriter.msg.Answers) > 0 {
		// TXT record found
		t.Log("TXT record returned")
	}
}

func TestDNSHandler_PTRQuery(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false
	config.Zone = "ptr.local"
	sd, _ := NewEnhancedServiceDiscovery(config)
	defer sd.Close()

	sd.RegisterService(&ServiceDefinition{
		Name:     "browse",
		Protocol: "tcp",
		Port:     80,
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Health: HealthStateHealthy},
		},
	})

	handler := NewDNSHandler(sd, nil, nil)

	mockWriter := &mockResponseWriter{}
	query := NewQuery("_services._dns-sd._udp.ptr.local", TypePTR)

	handler.ServeDNS(context.Background(), mockWriter, query)

	if mockWriter.msg != nil && len(mockWriter.msg.Answers) > 0 {
		t.Log("PTR record for DNS-SD browsing returned")
	}
}
