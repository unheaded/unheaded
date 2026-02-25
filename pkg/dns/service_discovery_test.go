package dns

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestEnhancedServiceDiscovery_RegisterService(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false // Disable for testing
	config.EnableMetrics = true

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name:     "api",
		Protocol: "tcp",
		Port:     8080,
		TTL:      60,
		Endpoints: []*ServiceEndpoint{
			{
				ID:      "api-1",
				Address: net.ParseIP("10.0.0.1"),
				Port:    8080,
				Weight:  100,
				Health:  HealthStateHealthy,
			},
			{
				ID:      "api-2",
				Address: net.ParseIP("10.0.0.2"),
				Port:    8080,
				Weight:  100,
				Health:  HealthStateHealthy,
			},
		},
		Metadata: map[string]string{
			"version": "v1.0.0",
		},
	}

	if err := sd.RegisterService(svc); err != nil {
		t.Fatalf("RegisterService() error = %v", err)
	}

	// Verify service was registered
	services := sd.ListServices()
	if len(services) != 1 || services[0] != "api" {
		t.Errorf("ListServices() = %v, want [api]", services)
	}

	// Verify we can get the service
	retrieved, err := sd.GetService("api")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if retrieved.Name != "api" {
		t.Errorf("GetService().Name = %v, want api", retrieved.Name)
	}
	if len(retrieved.Endpoints) != 2 {
		t.Errorf("GetService().Endpoints len = %v, want 2", len(retrieved.Endpoints))
	}
}

func TestEnhancedServiceDiscovery_AddRemoveEndpoint(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name:     "db",
		Protocol: "tcp",
		Port:     5432,
	}
	sd.RegisterService(svc)

	// Add endpoint
	ep := &ServiceEndpoint{
		ID:      "db-1",
		Address: net.ParseIP("10.0.0.10"),
		Port:    5432,
		Weight:  100,
		Health:  HealthStateHealthy,
	}

	if err := sd.AddEndpoint("db", ep); err != nil {
		t.Fatalf("AddEndpoint() error = %v", err)
	}

	retrieved, _ := sd.GetService("db")
	if len(retrieved.Endpoints) != 1 {
		t.Errorf("After AddEndpoint, Endpoints len = %v, want 1", len(retrieved.Endpoints))
	}

	// Try adding duplicate
	if err := sd.AddEndpoint("db", ep); err == nil {
		t.Error("AddEndpoint() duplicate should return error")
	}

	// Remove endpoint
	if err := sd.RemoveEndpoint("db", "db-1"); err != nil {
		t.Fatalf("RemoveEndpoint() error = %v", err)
	}

	retrieved, _ = sd.GetService("db")
	if len(retrieved.Endpoints) != 0 {
		t.Errorf("After RemoveEndpoint, Endpoints len = %v, want 0", len(retrieved.Endpoints))
	}
}

func TestEnhancedServiceDiscovery_HealthAwareDNS(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name:     "web",
		Protocol: "http",
		Port:     80,
		Endpoints: []*ServiceEndpoint{
			{ID: "web-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100, Health: HealthStateHealthy},
			{ID: "web-2", Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100, Health: HealthStateUnhealthy},
			{ID: "web-3", Address: net.ParseIP("10.0.0.3"), Port: 80, Weight: 100, Health: HealthStateHealthy},
		},
	}
	sd.RegisterService(svc)

	// Resolve should only return healthy endpoints
	endpoints, err := sd.ResolveService("web")
	if err != nil {
		t.Fatalf("ResolveService() error = %v", err)
	}
	if len(endpoints) != 2 {
		t.Errorf("ResolveService() returned %d endpoints, want 2", len(endpoints))
	}

	// Check that only healthy endpoints are returned
	for _, ep := range endpoints {
		if ep.Health != HealthStateHealthy {
			t.Errorf("ResolveService() returned unhealthy endpoint: %v", ep.ID)
		}
	}

	// Mark all unhealthy
	sd.UpdateEndpointHealth("web", "web-1", HealthStateUnhealthy)
	sd.UpdateEndpointHealth("web", "web-3", HealthStateUnhealthy)

	// Should return error now
	_, err = sd.ResolveService("web")
	if err != ErrNoHealthyEndpoints {
		t.Errorf("ResolveService() with no healthy endpoints error = %v, want ErrNoHealthyEndpoints", err)
	}
}

func TestEnhancedServiceDiscovery_RoundRobinLoadBalancing(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name:          "rr-svc",
		Protocol:      "tcp",
		Port:          8080,
		LoadBalancing: LoadBalanceRoundRobin,
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 8080, Weight: 100, Health: HealthStateHealthy},
			{ID: "ep-2", Address: net.ParseIP("10.0.0.2"), Port: 8080, Weight: 100, Health: HealthStateHealthy},
			{ID: "ep-3", Address: net.ParseIP("10.0.0.3"), Port: 8080, Weight: 100, Health: HealthStateHealthy},
		},
	}
	sd.RegisterService(svc)

	// Count selections over multiple calls
	counts := make(map[string]int)
	for i := 0; i < 9; i++ {
		ep, err := sd.ResolveServiceSingle("rr-svc")
		if err != nil {
			t.Fatalf("ResolveServiceSingle() error = %v", err)
		}
		counts[ep.ID]++
	}

	// Each should be selected 3 times
	for id, count := range counts {
		if count != 3 {
			t.Errorf("Round robin: endpoint %s selected %d times, want 3", id, count)
		}
	}
}

func TestEnhancedServiceDiscovery_WeightedLoadBalancing(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name:          "weighted-svc",
		Protocol:      "tcp",
		Port:          8080,
		LoadBalancing: LoadBalanceWeighted,
		Endpoints: []*ServiceEndpoint{
			{ID: "high", Address: net.ParseIP("10.0.0.1"), Port: 8080, Weight: 80, Health: HealthStateHealthy},
			{ID: "low", Address: net.ParseIP("10.0.0.2"), Port: 8080, Weight: 20, Health: HealthStateHealthy},
		},
	}
	sd.RegisterService(svc)

	// Count selections over many calls
	counts := make(map[string]int)
	iterations := 1000
	for i := 0; i < iterations; i++ {
		ep, err := sd.ResolveServiceSingle("weighted-svc")
		if err != nil {
			t.Fatalf("ResolveServiceSingle() error = %v", err)
		}
		counts[ep.ID]++
	}

	// High weight should be selected ~80% of the time
	highRatio := float64(counts["high"]) / float64(iterations)
	if highRatio < 0.7 || highRatio > 0.9 {
		t.Errorf("Weighted: high endpoint selected %.2f%%, want ~80%%", highRatio*100)
	}
}

func TestEnhancedServiceDiscovery_SRVLookup(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name:     "myapp",
		Protocol: "tcp",
		Port:     9000,
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 9000, Weight: 100, Priority: 10, Health: HealthStateHealthy},
			{ID: "ep-2", Address: net.ParseIP("10.0.0.2"), Port: 9001, Weight: 50, Priority: 20, Health: HealthStateHealthy},
		},
	}
	sd.RegisterService(svc)

	results, err := sd.LookupSRV(context.Background(), "myapp", "tcp")
	if err != nil {
		t.Fatalf("LookupSRV() error = %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("LookupSRV() returned %d results, want 2", len(results))
	}

	// Should be sorted by priority
	if results[0].Priority != 10 {
		t.Errorf("LookupSRV() first result priority = %d, want 10", results[0].Priority)
	}
	if results[1].Priority != 20 {
		t.Errorf("LookupSRV() second result priority = %d, want 20", results[1].Priority)
	}
}

func TestEnhancedServiceDiscovery_BrowseServices(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	// Register multiple services
	services := []struct {
		name     string
		protocol string
	}{
		{"http", "tcp"},
		{"ssh", "tcp"},
		{"dns", "udp"},
	}

	for _, s := range services {
		svc := &ServiceDefinition{
			Name:     s.name,
			Protocol: s.protocol,
			Port:     80,
			Endpoints: []*ServiceEndpoint{
				{ID: s.name + "-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Health: HealthStateHealthy},
			},
		}
		sd.RegisterService(svc)
	}

	// Browse services
	result, err := sd.BrowseServices(context.Background())
	if err != nil {
		t.Fatalf("BrowseServices() error = %v", err)
	}

	if len(result) != 3 {
		t.Errorf("BrowseServices() returned %d services, want 3", len(result))
	}
}

func TestEnhancedServiceDiscovery_DNSRecords(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.Zone = "test.local"
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name:     "api",
		Protocol: "tcp",
		Port:     8080,
		TTL:      60,
		Endpoints: []*ServiceEndpoint{
			{ID: "api-1", Address: net.ParseIP("10.0.0.1"), Port: 8080, Weight: 100, Health: HealthStateHealthy},
		},
		Metadata: map[string]string{
			"version": "1.0.0",
		},
	}
	sd.RegisterService(svc)

	zone := sd.Zone()

	// Check A records
	aRecords := zone.GetRecords("api.test.local", TypeA)
	if len(aRecords) != 1 {
		t.Errorf("A records count = %d, want 1", len(aRecords))
	}

	// Check SRV records
	srvRecords := zone.GetRecords("_api._tcp.test.local", TypeSRV)
	if len(srvRecords) != 1 {
		t.Errorf("SRV records count = %d, want 1", len(srvRecords))
	}

	// Check TXT records
	txtRecords := zone.GetRecords("_api._tcp.test.local", TypeTXT)
	if len(txtRecords) != 1 {
		t.Errorf("TXT records count = %d, want 1", len(txtRecords))
	}

	// Check PTR for DNS-SD browsing
	ptrRecords := zone.GetRecords("_services._dns-sd._udp.test.local", TypePTR)
	if len(ptrRecords) == 0 {
		t.Error("DNS-SD browse PTR record not found")
	}
}

func TestEnhancedServiceDiscovery_HealthStateChange(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	// Track changes
	var mu sync.Mutex
	changes := []ServiceEvent{}
	sd.OnChange(func(name string, event ServiceEvent) {
		mu.Lock()
		changes = append(changes, event)
		mu.Unlock()
	})

	svc := &ServiceDefinition{
		Name: "health-test",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Health: HealthStateHealthy},
		},
	}
	sd.RegisterService(svc)

	// Wait for callback
	time.Sleep(10 * time.Millisecond)

	// Update health
	sd.UpdateEndpointHealth("health-test", "ep-1", HealthStateUnhealthy)

	// Wait for callback
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	hasHealthChange := false
	for _, e := range changes {
		if e == ServiceEventHealthChanged {
			hasHealthChange = true
		}
	}
	mu.Unlock()

	if !hasHealthChange {
		t.Error("Health change callback not triggered")
	}
}

func TestEnhancedServiceDiscovery_DeregisterService(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name: "temp",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Health: HealthStateHealthy},
		},
	}
	sd.RegisterService(svc)

	// Verify registered
	if len(sd.ListServices()) != 1 {
		t.Fatal("Service not registered")
	}

	// Deregister
	if err := sd.DeregisterService("temp"); err != nil {
		t.Fatalf("DeregisterService() error = %v", err)
	}

	// Verify removed
	if len(sd.ListServices()) != 0 {
		t.Error("Service not deregistered")
	}

	// Should error on second deregister
	if err := sd.DeregisterService("temp"); err != ErrServiceNotFound {
		t.Errorf("DeregisterService() error = %v, want ErrServiceNotFound", err)
	}
}

func TestEnhancedServiceDiscovery_Heartbeat(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name: "heartbeat-test",
		Endpoints: []*ServiceEndpoint{
			{ID: "ep-1", Address: net.ParseIP("10.0.0.1"), Port: 80, Health: HealthStateHealthy},
		},
	}
	sd.RegisterService(svc)

	before, _ := sd.GetService("heartbeat-test")
	initialLastSeen := before.Endpoints[0].LastSeen

	time.Sleep(10 * time.Millisecond)

	// Heartbeat
	if err := sd.Heartbeat("heartbeat-test", "ep-1"); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}

	after, _ := sd.GetService("heartbeat-test")
	if !after.Endpoints[0].LastSeen.After(initialLastSeen) {
		t.Error("Heartbeat did not update LastSeen")
	}
}

func TestEnhancedServiceDiscovery_DegradedHealth(t *testing.T) {
	config := DefaultServiceDiscoveryConfig()
	config.EnableHealthChecks = false

	sd, err := NewEnhancedServiceDiscovery(config)
	if err != nil {
		t.Fatalf("NewEnhancedServiceDiscovery() error = %v", err)
	}
	defer sd.Close()

	svc := &ServiceDefinition{
		Name:          "degraded-test",
		LoadBalancing: LoadBalanceWeighted,
		Endpoints: []*ServiceEndpoint{
			{ID: "healthy", Address: net.ParseIP("10.0.0.1"), Port: 80, Weight: 100, Health: HealthStateHealthy},
			{ID: "degraded", Address: net.ParseIP("10.0.0.2"), Port: 80, Weight: 100, Health: HealthStateDegraded},
		},
	}
	sd.RegisterService(svc)

	// Both should be returned (degraded is still considered healthy for resolution)
	endpoints, err := sd.ResolveService("degraded-test")
	if err != nil {
		t.Fatalf("ResolveService() error = %v", err)
	}
	if len(endpoints) != 2 {
		t.Errorf("ResolveService() returned %d endpoints, want 2", len(endpoints))
	}

	// But degraded should have reduced effective weight
	for _, ep := range endpoints {
		if ep.ID == "degraded" && ep.EffectiveWeight() != 50 {
			t.Errorf("Degraded endpoint weight = %d, want 50", ep.EffectiveWeight())
		}
	}
}

func TestServiceEndpoint_EffectiveWeight(t *testing.T) {
	tests := []struct {
		name   string
		ep     *ServiceEndpoint
		want   uint16
	}{
		{
			name: "healthy",
			ep:   &ServiceEndpoint{Weight: 100, Health: HealthStateHealthy},
			want: 100,
		},
		{
			name: "degraded",
			ep:   &ServiceEndpoint{Weight: 100, Health: HealthStateDegraded},
			want: 50,
		},
		{
			name: "unhealthy",
			ep:   &ServiceEndpoint{Weight: 100, Health: HealthStateUnhealthy},
			want: 0,
		},
		{
			name: "unknown",
			ep:   &ServiceEndpoint{Weight: 100, Health: HealthStateUnknown},
			want: 100, // Unknown treated as healthy for weight
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ep.EffectiveWeight(); got != tt.want {
				t.Errorf("EffectiveWeight() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadBalancingPolicy_String(t *testing.T) {
	tests := []struct {
		policy LoadBalancingPolicy
		want   string
	}{
		{LoadBalanceRoundRobin, "round-robin"},
		{LoadBalanceWeighted, "weighted"},
		{LoadBalanceRandom, "random"},
		{LoadBalanceFirstHealthy, "first-healthy"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.policy.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHealthState_String(t *testing.T) {
	tests := []struct {
		state HealthState
		want  string
	}{
		{HealthStateHealthy, "healthy"},
		{HealthStateUnhealthy, "unhealthy"},
		{HealthStateDegraded, "degraded"},
		{HealthStateUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
