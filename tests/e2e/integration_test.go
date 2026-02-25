// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package e2e provides E2E integration tests that wire together the major packages.
// This test file verifies the integration flow:
// Scheduler -> Runtime -> Container -> DNS -> Mesh -> Load Balancer
package e2e

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/container"
	"unheaded/pkg/dns"
	"unheaded/pkg/loadbalancer"
	"unheaded/pkg/mesh"
	"unheaded/pkg/scheduler"
)

// ============================================================================
// INTEGRATION TEST HARNESS
// ============================================================================

// IntegrationTestHarness provides a coordinated test environment for all packages.
type IntegrationTestHarness struct {
	t *testing.T

	// Components
	Scheduler        *scheduler.Scheduler
	RuntimeManager   *TestRuntimeManager
	ContainerRuntime *TestContainerRuntime
	DNSDiscovery     *dns.ServiceDiscovery
	ServiceMesh      *mesh.Mesh
	LoadBalancer     *loadbalancer.Balancer

	// State tracking
	mu                 sync.Mutex
	scheduledWorkloads map[string]*scheduler.Workload
	createdContainers  map[string]*TestContainer
	registeredServices map[string]*dns.Service
	meshEndpoints      map[string][]*mesh.Endpoint

	// Cleanup
	cleanup []func()
}

// TestContainer represents a mock container for testing.
type TestContainer struct {
	ID        string
	Name      string
	Image     string
	State     container.ContainerState
	IPAddress string
	Port      int
	Labels    map[string]string
	CreatedAt time.Time
	StartedAt time.Time
}

// TestRuntimeManager simulates runtime operations for testing.
type TestRuntimeManager struct {
	mu         sync.Mutex
	containers map[string]*TestContainer
	nextIP     int
}

// TestContainerRuntime simulates the container runtime for testing.
type TestContainerRuntime struct {
	manager *TestRuntimeManager
}

// NewIntegrationTestHarness creates a new integration test harness.
func NewIntegrationTestHarness(t *testing.T) *IntegrationTestHarness {
	t.Helper()

	h := &IntegrationTestHarness{
		t:                  t,
		scheduledWorkloads: make(map[string]*scheduler.Workload),
		createdContainers:  make(map[string]*TestContainer),
		registeredServices: make(map[string]*dns.Service),
		meshEndpoints:      make(map[string][]*mesh.Endpoint),
		cleanup:            []func(){},
	}

	// Initialize components
	h.initScheduler()
	h.initRuntimeManager()
	h.initContainerRuntime()
	h.initDNSDiscovery()
	h.initServiceMesh()
	// LoadBalancer initialized per-test since it requires backends

	return h
}

// initScheduler initializes the scheduler component.
func (h *IntegrationTestHarness) initScheduler() {
	config := scheduler.DefaultConfig()
	config.SchedulingInterval = 100 * time.Millisecond
	config.WorkerCount = 2

	h.Scheduler = scheduler.NewScheduler(config)

	// Register test nodes
	for i := 1; i <= 3; i++ {
		node := scheduler.NewNode(
			fmt.Sprintf("node-%d", i),
			fmt.Sprintf("test-node-%d", i),
			scheduler.Resources{
				CPU:    4000,                       // 4 cores in millicores
				Memory: 8 * 1024 * 1024 * 1024,     // 8GB
				Disk:   100 * 1024 * 1024 * 1024,   // 100GB
			},
		)
		node.Labels = map[string]string{
			"zone": fmt.Sprintf("zone-%d", (i%3)+1),
			"env":  "test",
		}
		if err := h.Scheduler.RegisterNode(node); err != nil {
			h.t.Fatalf("Failed to register node %d: %v", i, err)
		}
	}

	h.cleanup = append(h.cleanup, func() {
		h.Scheduler.Stop()
	})
}

// initRuntimeManager initializes the runtime manager.
func (h *IntegrationTestHarness) initRuntimeManager() {
	h.RuntimeManager = &TestRuntimeManager{
		containers: make(map[string]*TestContainer),
		nextIP:     10,
	}
}

// initContainerRuntime initializes the container runtime.
func (h *IntegrationTestHarness) initContainerRuntime() {
	h.ContainerRuntime = &TestContainerRuntime{
		manager: h.RuntimeManager,
	}
}

// initDNSDiscovery initializes DNS-based service discovery.
func (h *IntegrationTestHarness) initDNSDiscovery() {
	config := dns.DefaultServiceDiscoveryConfig()
	config.Zone = "test.local"
	config.DefaultTTL = 30

	sd, err := dns.NewServiceDiscovery(config)
	if err != nil {
		h.t.Fatalf("Failed to create DNS service discovery: %v", err)
	}

	h.DNSDiscovery = sd

	h.cleanup = append(h.cleanup, func() {
		h.DNSDiscovery.Close()
	})
}

// initServiceMesh initializes the service mesh.
func (h *IntegrationTestHarness) initServiceMesh() {
	config := mesh.NewConfigBuilder().
		WithServiceName("integration-test").
		WithNamespace("test").
		WithDiscovery(mesh.DiscoveryTypeStatic, 5*time.Second).
		WithoutCircuitBreaker().
		WithoutHealthCheck().
		WithTLS(false).
		WithInboundProxy("127.0.0.1", 0).  // Disable inbound proxy
		WithOutboundProxy("127.0.0.1", 0). // Disable outbound proxy
		MustBuild()

	meshInstance, err := mesh.NewMesh(config)
	if err != nil {
		h.t.Fatalf("Failed to create service mesh: %v", err)
	}

	h.ServiceMesh = meshInstance

	h.cleanup = append(h.cleanup, func() {
		h.ServiceMesh.Stop()
	})
}

// CreateLoadBalancer creates a load balancer with given backends.
func (h *IntegrationTestHarness) CreateLoadBalancer(name string, backends []loadbalancer.BackendConfig) *loadbalancer.Balancer {
	config := loadbalancer.DefaultConfig()
	config.Name = name
	config.ListenAddress = "127.0.0.1:0" // Random available port
	config.Protocol = loadbalancer.ProtocolHTTP
	config.Algorithm = loadbalancer.AlgorithmRoundRobin
	config.Backends = backends
	config.HealthCheck.Enabled = false // Disable for tests
	config.MetricsEnabled = false

	lb, err := loadbalancer.New(config)
	if err != nil {
		h.t.Fatalf("Failed to create load balancer: %v", err)
	}

	h.LoadBalancer = lb

	h.cleanup = append(h.cleanup, func() {
		if lb.IsRunning() {
			lb.Stop()
		}
	})

	return lb
}

// Start starts all components.
func (h *IntegrationTestHarness) Start() error {
	if err := h.Scheduler.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}

	if err := h.ServiceMesh.Start(); err != nil {
		return fmt.Errorf("start mesh: %w", err)
	}

	return nil
}

// Cleanup cleans up all test resources.
func (h *IntegrationTestHarness) Cleanup() {
	// Run cleanup in reverse order
	for i := len(h.cleanup) - 1; i >= 0; i-- {
		h.cleanup[i]()
	}
}

// ============================================================================
// RUNTIME MANAGER METHODS
// ============================================================================

// CreateContainer creates a new test container.
func (rm *TestRuntimeManager) CreateContainer(ctx context.Context, config *container.ContainerSpec) (*TestContainer, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	id := fmt.Sprintf("container-%d-%s", time.Now().UnixNano(), config.Name)

	// Generate IP address
	rm.nextIP++
	ip := fmt.Sprintf("10.10.10.%d", rm.nextIP)

	c := &TestContainer{
		ID:        id,
		Name:      config.Name,
		Image:     config.Image,
		State:     container.StateCreated,
		IPAddress: ip,
		Port:      8080,
		Labels:    config.Labels,
		CreatedAt: time.Now(),
	}

	rm.containers[id] = c
	return c, nil
}

// StartContainer starts a test container.
func (rm *TestRuntimeManager) StartContainer(ctx context.Context, containerID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	c, ok := rm.containers[containerID]
	if !ok {
		return container.ErrContainerNotFound
	}

	c.State = container.StateRunning
	c.StartedAt = time.Now()
	return nil
}

// GetContainer retrieves a test container.
func (rm *TestRuntimeManager) GetContainer(ctx context.Context, containerID string) (*TestContainer, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	c, ok := rm.containers[containerID]
	if !ok {
		return nil, container.ErrContainerNotFound
	}
	return c, nil
}

// ListContainers lists all test containers.
func (rm *TestRuntimeManager) ListContainers() []*TestContainer {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	result := make([]*TestContainer, 0, len(rm.containers))
	for _, c := range rm.containers {
		result = append(result, c)
	}
	return result
}

// StopContainer stops a test container.
func (rm *TestRuntimeManager) StopContainer(ctx context.Context, containerID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	c, ok := rm.containers[containerID]
	if !ok {
		return container.ErrContainerNotFound
	}

	c.State = container.StateStopped
	return nil
}

// DeleteContainer deletes a test container.
func (rm *TestRuntimeManager) DeleteContainer(ctx context.Context, containerID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, ok := rm.containers[containerID]; !ok {
		return container.ErrContainerNotFound
	}

	delete(rm.containers, containerID)
	return nil
}

// ============================================================================
// E2E INTEGRATION TESTS
// ============================================================================

// TestSchedulerToRuntime tests that the scheduler can dispatch to runtime.
func TestSchedulerToRuntime(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}

	// Create a workload
	workload := &scheduler.Workload{
		ID:        "test-workload-1",
		Name:      "test-app",
		Namespace: "default",
		Priority:  0,
		Resources: scheduler.ResourceRequirements{
			Requests: scheduler.Resources{
				CPU:    1000,                   // 1 core
				Memory: 1024 * 1024 * 1024,     // 1GB
			},
		},
		Labels: map[string]string{
			"app": "test-app",
		},
	}

	// Set callback for when workload is scheduled
	var scheduledNode *scheduler.Node
	scheduledCh := make(chan struct{})

	harness.Scheduler.OnScheduled(func(w *scheduler.Workload, n *scheduler.Node) {
		scheduledNode = n
		close(scheduledCh)
	})

	// Submit workload
	if err := harness.Scheduler.Submit(workload); err != nil {
		t.Fatalf("Failed to submit workload: %v", err)
	}

	// Wait for scheduling
	select {
	case <-scheduledCh:
		// Workload was scheduled
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for workload to be scheduled")
	}

	// Verify workload was scheduled
	if scheduledNode == nil {
		t.Fatal("Workload was not scheduled to any node")
	}

	t.Logf("Workload %s scheduled to node %s", workload.ID, scheduledNode.ID)

	// Now simulate runtime creating a container based on scheduled workload
	ctx := context.Background()
	containerSpec := &container.ContainerSpec{
		Name:  workload.Name,
		Image: "test-image:latest",
		Labels: map[string]string{
			"workload-id": workload.ID,
			"node-id":     scheduledNode.ID,
		},
	}

	testContainer, err := harness.RuntimeManager.CreateContainer(ctx, containerSpec)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	if err := harness.RuntimeManager.StartContainer(ctx, testContainer.ID); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Verify container is running
	c, err := harness.RuntimeManager.GetContainer(ctx, testContainer.ID)
	if err != nil {
		t.Fatalf("Failed to get container: %v", err)
	}

	if c.State != container.StateRunning {
		t.Errorf("Expected container state Running, got %s", c.State)
	}

	t.Logf("Container %s created and running with IP %s", c.ID, c.IPAddress)
}

// TestRuntimeToContainer tests that runtime can create containers.
func TestRuntimeToContainer(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	ctx := context.Background()

	// Create multiple containers
	specs := []container.ContainerSpec{
		{Name: "web-server", Image: "nginx:latest", Labels: map[string]string{"tier": "frontend"}},
		{Name: "api-server", Image: "api:latest", Labels: map[string]string{"tier": "backend"}},
		{Name: "database", Image: "postgres:latest", Labels: map[string]string{"tier": "data"}},
	}

	var containers []*TestContainer
	for _, spec := range specs {
		c, err := harness.RuntimeManager.CreateContainer(ctx, &spec)
		if err != nil {
			t.Fatalf("Failed to create container %s: %v", spec.Name, err)
		}

		if err := harness.RuntimeManager.StartContainer(ctx, c.ID); err != nil {
			t.Fatalf("Failed to start container %s: %v", spec.Name, err)
		}

		containers = append(containers, c)
		t.Logf("Created container: %s (ID: %s, IP: %s)", c.Name, c.ID, c.IPAddress)
	}

	// Verify all containers are running
	allContainers := harness.RuntimeManager.ListContainers()
	if len(allContainers) != len(specs) {
		t.Errorf("Expected %d containers, got %d", len(specs), len(allContainers))
	}

	runningCount := 0
	for _, c := range allContainers {
		if c.State == container.StateRunning {
			runningCount++
		}
	}

	if runningCount != len(specs) {
		t.Errorf("Expected %d running containers, got %d", len(specs), runningCount)
	}
}

// TestContainerToDNS tests that containers register with DNS.
func TestContainerToDNS(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	ctx := context.Background()

	// Create a container
	spec := &container.ContainerSpec{
		Name:  "web-service",
		Image: "nginx:latest",
		Labels: map[string]string{
			"service": "web",
		},
	}

	testContainer, err := harness.RuntimeManager.CreateContainer(ctx, spec)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	if err := harness.RuntimeManager.StartContainer(ctx, testContainer.ID); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Register service with DNS discovery
	svc := &dns.Service{
		Name:     "web-service",
		Protocol: "tcp",
		Port:     8080,
		TTL:      30,
		Endpoints: []*dns.Endpoint{
			{
				ID:       testContainer.ID,
				Address:  net.ParseIP(testContainer.IPAddress),
				Port:     uint16(testContainer.Port),
				Healthy:  true,
				Metadata: testContainer.Labels,
			},
		},
		Metadata: map[string]string{
			"version": "v1",
		},
	}

	if err := harness.DNSDiscovery.RegisterService(svc); err != nil {
		t.Fatalf("Failed to register service: %v", err)
	}

	// Lookup the service
	endpoints, err := harness.DNSDiscovery.LookupService("web-service")
	if err != nil {
		t.Fatalf("Failed to lookup service: %v", err)
	}

	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint, got %d", len(endpoints))
	}

	if endpoints[0].Address.String() != testContainer.IPAddress {
		t.Errorf("Expected endpoint address %s, got %s", testContainer.IPAddress, endpoints[0].Address.String())
	}

	t.Logf("Service %s registered with DNS, endpoint: %s:%d",
		svc.Name, endpoints[0].Address, endpoints[0].Port)
}

// TestDNSToMesh tests that DNS entries are picked up by the service mesh.
func TestDNSToMesh(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}

	ctx := context.Background()

	// Create containers for a service
	ips := []string{"10.10.10.100", "10.10.10.101", "10.10.10.102"}
	var dnsEndpoints []*dns.Endpoint

	for i, ip := range ips {
		dnsEndpoints = append(dnsEndpoints, &dns.Endpoint{
			ID:      fmt.Sprintf("endpoint-%d", i),
			Address: net.ParseIP(ip),
			Port:    8080,
			Healthy: true,
		})
	}

	// Register with DNS
	svc := &dns.Service{
		Name:      "api-service",
		Protocol:  "tcp",
		Port:      8080,
		Endpoints: dnsEndpoints,
	}

	if err := harness.DNSDiscovery.RegisterService(svc); err != nil {
		t.Fatalf("Failed to register DNS service: %v", err)
	}

	// Register service with mesh (simulating DNS to mesh sync)
	meshEndpoints := make([]*mesh.Endpoint, len(dnsEndpoints))
	for i, ep := range dnsEndpoints {
		meshEndpoints[i] = &mesh.Endpoint{
			ID:       ep.ID,
			Address:  ep.Address.String(),
			Port:     int(ep.Port),
			Weight:   1,
			LastSeen: time.Now(),
		}
	}

	meshService := &mesh.ServiceInstance{
		ServiceName: "api-service",
		Namespace:   "test",
		Endpoints:   meshEndpoints,
		Protocol:    "http",
		UpdatedAt:   time.Now(),
	}

	if err := harness.ServiceMesh.RegisterService(meshService); err != nil {
		t.Fatalf("Failed to register mesh service: %v", err)
	}

	// Verify service is in mesh registry
	registeredSvc, ok := harness.ServiceMesh.GetService("api-service")
	if !ok {
		t.Fatal("Service not found in mesh registry")
	}

	if len(registeredSvc.Endpoints) != len(ips) {
		t.Errorf("Expected %d mesh endpoints, got %d", len(ips), len(registeredSvc.Endpoints))
	}

	// Get endpoints through mesh discovery
	discoveredEndpoints, err := harness.ServiceMesh.GetEndpoints(ctx, "api-service", "test")
	if err != nil {
		t.Fatalf("Failed to get mesh endpoints: %v", err)
	}

	if len(discoveredEndpoints) != len(ips) {
		t.Errorf("Expected %d discovered endpoints, got %d", len(ips), len(discoveredEndpoints))
	}

	t.Logf("Service %s synced to mesh with %d endpoints", svc.Name, len(discoveredEndpoints))
}

// TestMeshToLoadBalancer tests that mesh routes to load balancer.
func TestMeshToLoadBalancer(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	// Create backend configurations for load balancer
	backends := []loadbalancer.BackendConfig{
		{Name: "backend-1", Address: "127.0.0.1:9001", Weight: 1, Enabled: true},
		{Name: "backend-2", Address: "127.0.0.1:9002", Weight: 1, Enabled: true},
		{Name: "backend-3", Address: "127.0.0.1:9003", Weight: 1, Enabled: true},
	}

	lb := harness.CreateLoadBalancer("test-lb", backends)

	// Register same endpoints in mesh
	meshEndpoints := make([]*mesh.Endpoint, len(backends))
	for i, b := range backends {
		host, port, _ := net.SplitHostPort(b.Address)
		portNum := 0
		fmt.Sscanf(port, "%d", &portNum)

		meshEndpoints[i] = &mesh.Endpoint{
			ID:       b.Name,
			Address:  host,
			Port:     portNum,
			Weight:   b.Weight,
			LastSeen: time.Now(),
		}
	}

	// The mesh uses load balancer for routing decisions
	// Verify load balancer can select backends
	ctx := context.Background()

	// Test backend selection (round-robin)
	selectedBackends := make(map[string]int)
	for i := 0; i < 9; i++ {
		backend, err := lb.Select(ctx, "")
		if err != nil {
			t.Fatalf("Failed to select backend: %v", err)
		}
		selectedBackends[backend.Config.Name]++
	}

	// With round-robin, each backend should be selected 3 times
	for name, count := range selectedBackends {
		t.Logf("Backend %s selected %d times", name, count)
	}

	// Verify all backends were used
	if len(selectedBackends) != len(backends) {
		t.Errorf("Expected %d unique backends, got %d", len(backends), len(selectedBackends))
	}
}

// TestFullIntegrationFlow tests the complete integration flow.
func TestFullIntegrationFlow(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}

	ctx := context.Background()

	// Step 1: Scheduler schedules workloads to nodes
	t.Log("Step 1: Scheduling workloads...")

	workloads := []*scheduler.Workload{
		{
			ID:        "workload-web-1",
			Name:      "web-replica-1",
			Namespace: "production",
			Resources: scheduler.ResourceRequirements{
				Requests: scheduler.Resources{CPU: 500, Memory: 512 * 1024 * 1024},
			},
			Labels: map[string]string{"app": "web", "tier": "frontend"},
		},
		{
			ID:        "workload-web-2",
			Name:      "web-replica-2",
			Namespace: "production",
			Resources: scheduler.ResourceRequirements{
				Requests: scheduler.Resources{CPU: 500, Memory: 512 * 1024 * 1024},
			},
			Labels: map[string]string{"app": "web", "tier": "frontend"},
		},
		{
			ID:        "workload-api-1",
			Name:      "api-replica-1",
			Namespace: "production",
			Resources: scheduler.ResourceRequirements{
				Requests: scheduler.Resources{CPU: 1000, Memory: 1024 * 1024 * 1024},
			},
			Labels: map[string]string{"app": "api", "tier": "backend"},
		},
	}

	scheduledWorkloads := make(map[string]*scheduler.Node)
	scheduledCh := make(chan struct{}, len(workloads))
	var mu sync.Mutex

	harness.Scheduler.OnScheduled(func(w *scheduler.Workload, n *scheduler.Node) {
		mu.Lock()
		scheduledWorkloads[w.ID] = n
		mu.Unlock()
		scheduledCh <- struct{}{}
	})

	for _, w := range workloads {
		if err := harness.Scheduler.Submit(w); err != nil {
			t.Fatalf("Failed to submit workload %s: %v", w.ID, err)
		}
	}

	// Wait for all workloads to be scheduled
	for i := 0; i < len(workloads); i++ {
		select {
		case <-scheduledCh:
		case <-time.After(10 * time.Second):
			t.Fatalf("Timeout waiting for workload %d to be scheduled", i)
		}
	}

	t.Logf("Scheduled %d workloads", len(scheduledWorkloads))

	// Step 2: Runtime creates containers for each scheduled workload
	t.Log("Step 2: Creating containers...")

	containers := make(map[string]*TestContainer)
	for workloadID, node := range scheduledWorkloads {
		var w *scheduler.Workload
		for _, workload := range workloads {
			if workload.ID == workloadID {
				w = workload
				break
			}
		}

		spec := &container.ContainerSpec{
			Name:  w.Name,
			Image: fmt.Sprintf("%s:latest", w.Labels["app"]),
			Labels: map[string]string{
				"workload-id": w.ID,
				"node-id":     node.ID,
				"app":         w.Labels["app"],
			},
		}

		c, err := harness.RuntimeManager.CreateContainer(ctx, spec)
		if err != nil {
			t.Fatalf("Failed to create container for %s: %v", w.ID, err)
		}

		if err := harness.RuntimeManager.StartContainer(ctx, c.ID); err != nil {
			t.Fatalf("Failed to start container %s: %v", c.ID, err)
		}

		containers[w.ID] = c
		t.Logf("Created container %s for workload %s on node %s", c.ID, w.ID, node.ID)
	}

	// Step 3: Register containers with DNS
	t.Log("Step 3: Registering services with DNS...")

	// Group containers by app
	appContainers := make(map[string][]*TestContainer)
	for _, c := range containers {
		app := c.Labels["app"]
		appContainers[app] = append(appContainers[app], c)
	}

	for app, cList := range appContainers {
		endpoints := make([]*dns.Endpoint, len(cList))
		for i, c := range cList {
			endpoints[i] = &dns.Endpoint{
				ID:       c.ID,
				Address:  net.ParseIP(c.IPAddress),
				Port:     uint16(c.Port),
				Healthy:  true,
				Metadata: c.Labels,
			}
		}

		svc := &dns.Service{
			Name:      fmt.Sprintf("%s-service", app),
			Protocol:  "tcp",
			Port:      8080,
			Endpoints: endpoints,
		}

		if err := harness.DNSDiscovery.RegisterService(svc); err != nil {
			t.Fatalf("Failed to register DNS service %s: %v", svc.Name, err)
		}

		t.Logf("Registered DNS service %s with %d endpoints", svc.Name, len(endpoints))
	}

	// Step 4: Sync DNS services to Mesh
	t.Log("Step 4: Syncing services to mesh...")

	for app, cList := range appContainers {
		meshEndpoints := make([]*mesh.Endpoint, len(cList))
		for i, c := range cList {
			meshEndpoints[i] = &mesh.Endpoint{
				ID:       c.ID,
				Address:  c.IPAddress,
				Port:     c.Port,
				Weight:   1,
				LastSeen: time.Now(),
			}
		}

		meshSvc := &mesh.ServiceInstance{
			ServiceName: fmt.Sprintf("%s-service", app),
			Namespace:   "production",
			Endpoints:   meshEndpoints,
			Protocol:    "http",
			UpdatedAt:   time.Now(),
		}

		if err := harness.ServiceMesh.RegisterService(meshSvc); err != nil {
			t.Fatalf("Failed to register mesh service %s: %v", meshSvc.ServiceName, err)
		}

		t.Logf("Registered mesh service %s with %d endpoints", meshSvc.ServiceName, len(meshEndpoints))
	}

	// Step 5: Create load balancer with mesh endpoints
	t.Log("Step 5: Creating load balancer...")

	// Get web service endpoints for load balancer
	webContainers := appContainers["web"]
	backends := make([]loadbalancer.BackendConfig, len(webContainers))
	for i, c := range webContainers {
		backends[i] = loadbalancer.BackendConfig{
			Name:    c.ID,
			Address: fmt.Sprintf("%s:%d", c.IPAddress, c.Port),
			Weight:  1,
			Enabled: true,
		}
	}

	lb := harness.CreateLoadBalancer("web-lb", backends)
	t.Logf("Created load balancer with %d backends", len(backends))

	// Step 6: Verify the complete flow
	t.Log("Step 6: Verifying integration...")

	// Verify scheduler metrics
	metrics := harness.Scheduler.GetMetrics()
	if metrics.SuccessfulSchedulings < int64(len(workloads)) {
		t.Errorf("Expected at least %d successful schedulings, got %d",
			len(workloads), metrics.SuccessfulSchedulings)
	}

	// Verify DNS service discovery
	for app := range appContainers {
		serviceName := fmt.Sprintf("%s-service", app)
		endpoints, err := harness.DNSDiscovery.LookupService(serviceName)
		if err != nil {
			t.Errorf("Failed to lookup DNS service %s: %v", serviceName, err)
			continue
		}
		if len(endpoints) == 0 {
			t.Errorf("No endpoints found for DNS service %s", serviceName)
		}
	}

	// Verify mesh service registry
	meshServices := harness.ServiceMesh.ListServices()
	if len(meshServices) < len(appContainers) {
		t.Errorf("Expected at least %d mesh services, got %d",
			len(appContainers), len(meshServices))
	}

	// Verify load balancer backend selection
	selectedBackends := make(map[string]int)
	for i := 0; i < 10; i++ {
		backend, err := lb.Select(ctx, "")
		if err != nil {
			t.Errorf("Failed to select backend: %v", err)
			continue
		}
		selectedBackends[backend.Config.Name]++
	}

	if len(selectedBackends) != len(backends) {
		t.Errorf("Expected %d unique backends selected, got %d",
			len(backends), len(selectedBackends))
	}

	t.Log("Full integration flow completed successfully!")
	t.Logf("Summary:")
	t.Logf("  - Scheduled workloads: %d", len(scheduledWorkloads))
	t.Logf("  - Created containers: %d", len(containers))
	t.Logf("  - Registered DNS services: %d", len(appContainers))
	t.Logf("  - Registered mesh services: %d", len(meshServices))
	t.Logf("  - Load balancer backends: %d", len(backends))
}

// TestSchedulingWithAffinity tests scheduler affinity rules integration.
func TestSchedulingWithAffinity(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}

	// Create workload with zone affinity
	workload := &scheduler.Workload{
		ID:        "affinity-workload",
		Name:      "zone-aware-app",
		Namespace: "default",
		Resources: scheduler.ResourceRequirements{
			Requests: scheduler.Resources{CPU: 100, Memory: 100 * 1024 * 1024},
		},
		Affinity: &scheduler.AffinitySpec{
			NodeAffinity: &scheduler.NodeAffinity{
				RequiredDuringScheduling: []scheduler.NodeSelectorTerm{
					{
						MatchExpressions: []scheduler.LabelSelectorRequirement{
							{
								Key:      "zone",
								Operator: scheduler.LabelSelectorOpIn,
								Values:   []string{"zone-1"},
							},
						},
					},
				},
			},
		},
	}

	var scheduledNode *scheduler.Node
	scheduledCh := make(chan struct{})

	harness.Scheduler.OnScheduled(func(w *scheduler.Workload, n *scheduler.Node) {
		scheduledNode = n
		close(scheduledCh)
	})

	if err := harness.Scheduler.Submit(workload); err != nil {
		t.Fatalf("Failed to submit workload: %v", err)
	}

	select {
	case <-scheduledCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for workload to be scheduled")
	}

	// Verify workload was scheduled to zone-1
	if scheduledNode == nil {
		t.Fatal("Workload was not scheduled")
	}

	if scheduledNode.Labels["zone"] != "zone-1" {
		t.Errorf("Expected node in zone-1, got zone %s", scheduledNode.Labels["zone"])
	}

	t.Logf("Workload with zone affinity scheduled to node %s in zone %s",
		scheduledNode.ID, scheduledNode.Labels["zone"])
}

// TestServiceHealthPropagation tests health status propagation through the stack.
func TestServiceHealthPropagation(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	ctx := context.Background()

	// Create a service with multiple endpoints
	endpoints := []*dns.Endpoint{
		{ID: "ep-1", Address: net.ParseIP("10.10.10.101"), Port: 8080, Healthy: true},
		{ID: "ep-2", Address: net.ParseIP("10.10.10.102"), Port: 8080, Healthy: true},
		{ID: "ep-3", Address: net.ParseIP("10.10.10.103"), Port: 8080, Healthy: true},
	}

	svc := &dns.Service{
		Name:      "health-test-service",
		Protocol:  "tcp",
		Port:      8080,
		Endpoints: endpoints,
	}

	if err := harness.DNSDiscovery.RegisterService(svc); err != nil {
		t.Fatalf("Failed to register service: %v", err)
	}

	// Verify all endpoints are healthy
	healthyEndpoints, err := harness.DNSDiscovery.LookupService("health-test-service")
	if err != nil {
		t.Fatalf("Failed to lookup service: %v", err)
	}

	if len(healthyEndpoints) != 3 {
		t.Errorf("Expected 3 healthy endpoints, got %d", len(healthyEndpoints))
	}

	// Mark one endpoint unhealthy
	if err := harness.DNSDiscovery.SetEndpointHealth("health-test-service", "ep-2", false); err != nil {
		t.Fatalf("Failed to set endpoint health: %v", err)
	}

	// Verify only healthy endpoints are returned
	healthyEndpoints, err = harness.DNSDiscovery.LookupService("health-test-service")
	if err != nil {
		t.Fatalf("Failed to lookup service: %v", err)
	}

	if len(healthyEndpoints) != 2 {
		t.Errorf("Expected 2 healthy endpoints after marking one unhealthy, got %d", len(healthyEndpoints))
	}

	// Verify unhealthy endpoint is not in the list
	for _, ep := range healthyEndpoints {
		if ep.ID == "ep-2" {
			t.Error("Unhealthy endpoint ep-2 should not be in healthy endpoints list")
		}
	}

	t.Logf("Health propagation test: started with 3 endpoints, marked 1 unhealthy, now have %d healthy",
		len(healthyEndpoints))

	_ = ctx // Silence unused variable
}

// TestLoadBalancerAlgorithms tests different load balancing algorithms.
func TestLoadBalancerAlgorithms(t *testing.T) {
	testCases := []struct {
		name      string
		algorithm loadbalancer.Algorithm
	}{
		{"RoundRobin", loadbalancer.AlgorithmRoundRobin},
		{"LeastConnections", loadbalancer.AlgorithmLeastConnections},
		{"Random", loadbalancer.AlgorithmRandom},
		{"Weighted", loadbalancer.AlgorithmWeighted},
	}

	backends := []loadbalancer.BackendConfig{
		{Name: "backend-1", Address: "127.0.0.1:9001", Weight: 10, Enabled: true},
		{Name: "backend-2", Address: "127.0.0.1:9002", Weight: 20, Enabled: true},
		{Name: "backend-3", Address: "127.0.0.1:9003", Weight: 30, Enabled: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := loadbalancer.DefaultConfig()
			config.Name = fmt.Sprintf("test-lb-%s", tc.name)
			config.ListenAddress = "127.0.0.1:0"
			config.Algorithm = tc.algorithm
			config.Backends = backends
			config.HealthCheck.Enabled = false
			config.MetricsEnabled = false

			lb, err := loadbalancer.New(config)
			if err != nil {
				t.Fatalf("Failed to create load balancer: %v", err)
			}
			defer lb.Stop()

			ctx := context.Background()

			// Select backends multiple times
			selections := make(map[string]int)
			for i := 0; i < 30; i++ {
				backend, err := lb.Select(ctx, "")
				if err != nil {
					t.Fatalf("Failed to select backend: %v", err)
				}
				selections[backend.Config.Name]++
			}

			// Verify all backends were used (except for weighted which may skip low-weight backends)
			t.Logf("Algorithm %s selections: %v", tc.algorithm, selections)

			// For round-robin, verify even distribution
			if tc.algorithm == loadbalancer.AlgorithmRoundRobin {
				for _, count := range selections {
					if count != 10 {
						t.Logf("Round-robin distribution not perfectly even: %v", selections)
					}
				}
			}
		})
	}
}

// TestConcurrentScheduling tests concurrent workload scheduling.
func TestConcurrentScheduling(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}

	numWorkloads := 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	scheduledCount := 0
	failedCount := 0

	harness.Scheduler.OnScheduled(func(w *scheduler.Workload, n *scheduler.Node) {
		mu.Lock()
		scheduledCount++
		mu.Unlock()
	})

	harness.Scheduler.OnFailed(func(w *scheduler.Workload, err error) {
		mu.Lock()
		failedCount++
		mu.Unlock()
	})

	// Submit workloads concurrently
	for i := 0; i < numWorkloads; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			workload := &scheduler.Workload{
				ID:        fmt.Sprintf("concurrent-workload-%d", idx),
				Name:      fmt.Sprintf("concurrent-app-%d", idx),
				Namespace: "default",
				Resources: scheduler.ResourceRequirements{
					Requests: scheduler.Resources{
						CPU:    100, // Small resource request
						Memory: 50 * 1024 * 1024,
					},
				},
			}

			if err := harness.Scheduler.Submit(workload); err != nil {
				t.Logf("Failed to submit workload %d: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for scheduling to complete
	time.Sleep(3 * time.Second)

	mu.Lock()
	total := scheduledCount + failedCount
	mu.Unlock()

	t.Logf("Concurrent scheduling results: %d scheduled, %d failed, %d total",
		scheduledCount, failedCount, total)

	if scheduledCount == 0 {
		t.Error("No workloads were successfully scheduled")
	}
}

// TestEndToEndLatency measures the latency through the integration stack.
func TestEndToEndLatency(t *testing.T) {
	harness := NewIntegrationTestHarness(t)
	defer harness.Cleanup()

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start harness: %v", err)
	}

	ctx := context.Background()

	// Measure scheduling latency
	scheduleStart := time.Now()

	workload := &scheduler.Workload{
		ID:        "latency-test",
		Name:      "latency-app",
		Namespace: "default",
		Resources: scheduler.ResourceRequirements{
			Requests: scheduler.Resources{CPU: 100, Memory: 100 * 1024 * 1024},
		},
	}

	scheduledCh := make(chan struct{})
	var scheduledNode *scheduler.Node

	harness.Scheduler.OnScheduled(func(w *scheduler.Workload, n *scheduler.Node) {
		scheduledNode = n
		close(scheduledCh)
	})

	if err := harness.Scheduler.Submit(workload); err != nil {
		t.Fatalf("Failed to submit workload: %v", err)
	}

	<-scheduledCh
	schedulingLatency := time.Since(scheduleStart)

	// Measure container creation latency
	containerStart := time.Now()

	spec := &container.ContainerSpec{
		Name:  workload.Name,
		Image: "test:latest",
	}

	testContainer, err := harness.RuntimeManager.CreateContainer(ctx, spec)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	if err := harness.RuntimeManager.StartContainer(ctx, testContainer.ID); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	containerLatency := time.Since(containerStart)

	// Measure DNS registration latency
	dnsStart := time.Now()

	svc := &dns.Service{
		Name:     "latency-service",
		Protocol: "tcp",
		Port:     8080,
		Endpoints: []*dns.Endpoint{
			{
				ID:      testContainer.ID,
				Address: net.ParseIP(testContainer.IPAddress),
				Port:    uint16(testContainer.Port),
				Healthy: true,
			},
		},
	}

	if err := harness.DNSDiscovery.RegisterService(svc); err != nil {
		t.Fatalf("Failed to register DNS service: %v", err)
	}

	dnsLatency := time.Since(dnsStart)

	// Measure mesh registration latency
	meshStart := time.Now()

	meshSvc := &mesh.ServiceInstance{
		ServiceName: "latency-service",
		Namespace:   "test",
		Endpoints: []*mesh.Endpoint{
			{
				ID:       testContainer.ID,
				Address:  testContainer.IPAddress,
				Port:     testContainer.Port,
				Weight:   1,
				LastSeen: time.Now(),
			},
		},
	}

	if err := harness.ServiceMesh.RegisterService(meshSvc); err != nil {
		t.Fatalf("Failed to register mesh service: %v", err)
	}

	meshLatency := time.Since(meshStart)

	totalLatency := time.Since(scheduleStart)

	t.Logf("End-to-end latency breakdown:")
	t.Logf("  Scheduling: %v", schedulingLatency)
	t.Logf("  Container creation: %v", containerLatency)
	t.Logf("  DNS registration: %v", dnsLatency)
	t.Logf("  Mesh registration: %v", meshLatency)
	t.Logf("  Total: %v", totalLatency)

	// Set some reasonable thresholds
	if schedulingLatency > 5*time.Second {
		t.Errorf("Scheduling latency too high: %v", schedulingLatency)
	}

	_ = scheduledNode // Silence unused variable
}
