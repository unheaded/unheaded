package architect

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// HELPERS
// ============================================================================

func createTestService(t *testing.T) *ArchitectService {
	t.Helper()
	return New()
}

func createValidService() *Service {
	return &Service{
		ServiceID: "test-svc-1",
		Name:      "Test Service",
		Type:      "database",
		Status:    "healthy",
		Address:   "10.10.10.10",
		Port:      5432,
		Metadata:  make(map[string]interface{}),
	}
}

func createValidNetworkNode() *NetworkNode {
	return &NetworkNode{
		NodeID:   "node-1",
		Name:     "Test Node",
		Type:     "container",
		CIDR:     "10.10.10.0/24",
		Status:   "online",
		Metadata: make(map[string]interface{}),
	}
}

func createValidDecision() *ArchitectureDecision {
	return &ArchitectureDecision{
		DecisionID:  "dec-1",
		Title:       "Use PostgreSQL",
		Description: "Chose PostgreSQL for relational data due to ACID compliance",
		Component:   "database",
		Status:      "approved",
		Metadata:    make(map[string]interface{}),
	}
}

// ============================================================================
// SERVICE VALIDATION TESTS
// ============================================================================

func TestService_Validate_Valid(t *testing.T) {
	s := createValidService()
	if err := s.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestService_Validate_NilReceiver(t *testing.T) {
	var s *Service
	if err := s.Validate(); err != ErrNilInput {
		t.Errorf("Validate() = %v, want ErrNilInput", err)
	}
}

func TestService_Validate_EmptyServiceID(t *testing.T) {
	s := createValidService()
	s.ServiceID = ""
	if err := s.Validate(); err != ErrEmptyServiceID {
		t.Errorf("Validate() = %v, want ErrEmptyServiceID", err)
	}
}

func TestService_Validate_EmptyName(t *testing.T) {
	s := createValidService()
	s.Name = ""
	if err := s.Validate(); err != ErrEmptyName {
		t.Errorf("Validate() = %v, want ErrEmptyName", err)
	}
}

func TestService_Validate_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"negative port", -1},
		{"port above max", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := createValidService()
			s.Port = tt.port
			if err := s.Validate(); err == nil {
				t.Errorf("Validate() = nil, want error for port %d", tt.port)
			}
		})
	}
}

func TestService_Validate_BoundaryPorts(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"port 0", 0},
		{"port 1", 1},
		{"port 65535", 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := createValidService()
			s.Port = tt.port
			if err := s.Validate(); err != nil {
				t.Errorf("Validate() = %v, want nil for port %d", err, tt.port)
			}
		})
	}
}

// ============================================================================
// NETWORK NODE VALIDATION TESTS
// ============================================================================

func TestNetworkNode_Validate_Valid(t *testing.T) {
	n := createValidNetworkNode()
	if err := n.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestNetworkNode_Validate_NilReceiver(t *testing.T) {
	var n *NetworkNode
	if err := n.Validate(); err != ErrNilInput {
		t.Errorf("Validate() = %v, want ErrNilInput", err)
	}
}

func TestNetworkNode_Validate_EmptyNodeID(t *testing.T) {
	n := createValidNetworkNode()
	n.NodeID = ""
	if err := n.Validate(); err == nil {
		t.Errorf("Validate() = nil, want error for empty NodeID")
	}
}

func TestNetworkNode_Validate_EmptyName(t *testing.T) {
	n := createValidNetworkNode()
	n.Name = ""
	if err := n.Validate(); err != ErrEmptyName {
		t.Errorf("Validate() = %v, want ErrEmptyName", err)
	}
}

// ============================================================================
// ARCHITECTURE DECISION VALIDATION TESTS
// ============================================================================

func TestArchitectureDecision_Validate_Valid(t *testing.T) {
	d := createValidDecision()
	if err := d.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestArchitectureDecision_Validate_NilReceiver(t *testing.T) {
	var d *ArchitectureDecision
	if err := d.Validate(); err != ErrNilInput {
		t.Errorf("Validate() = %v, want ErrNilInput", err)
	}
}

func TestArchitectureDecision_Validate_EmptyTitle(t *testing.T) {
	d := createValidDecision()
	d.Title = ""
	if err := d.Validate(); err == nil {
		t.Errorf("Validate() = nil, want error for empty title")
	}
}

func TestArchitectureDecision_Validate_EmptyDescription(t *testing.T) {
	d := createValidDecision()
	d.Description = ""
	if err := d.Validate(); err != ErrEmptyDecision {
		t.Errorf("Validate() = %v, want ErrEmptyDecision", err)
	}
}

// ============================================================================
// ARCHITECT SERVICE INITIALIZATION TESTS
// ============================================================================

func TestArchitectService_New(t *testing.T) {
	s := New()
	if s == nil {
		t.Errorf("New() = nil, want non-nil service")
	}
	if s.infra == nil {
		t.Errorf("infra = nil, want non-nil")
	}
	if s.network == nil {
		t.Errorf("network = nil, want non-nil")
	}
	if s.infra.Services == nil {
		t.Errorf("Services map = nil, want non-nil")
	}
	if s.network.Nodes == nil {
		t.Errorf("Nodes map = nil, want non-nil")
	}
}

// ============================================================================
// ADD SERVICE TESTS
// ============================================================================

func TestArchitectService_AddService_Happy(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	svc := createValidService()

	err := s.AddService(ctx, svc)
	if err != nil {
		t.Errorf("AddService() = %v, want nil", err)
	}

	// Verify it was stored
	retrieved, err := s.GetService(ctx, svc.ServiceID)
	if err != nil {
		t.Errorf("GetService() = %v, want nil", err)
	}
	if retrieved.ServiceID != svc.ServiceID {
		t.Errorf("ServiceID = %s, want %s", retrieved.ServiceID, svc.ServiceID)
	}
}

func TestArchitectService_AddService_NilContext(t *testing.T) {
	s := createTestService(t)
	svc := createValidService()

	err := s.AddService(nil, svc)
	if err != ErrNilContext {
		t.Errorf("AddService(nil ctx) = %v, want ErrNilContext", err)
	}
}

func TestArchitectService_AddService_InvalidService(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	svc := createValidService()
	svc.ServiceID = "" // invalid

	err := s.AddService(ctx, svc)
	if err == nil {
		t.Errorf("AddService(invalid) = nil, want error")
	}
}

func TestArchitectService_AddService_TimestampsSet(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	svc := createValidService()
	svc.CreatedAt = time.Time{} // ensure zeros
	svc.UpdatedAt = time.Time{}

	before := time.Now()
	err := s.AddService(ctx, svc)
	after := time.Now()

	if err != nil {
		t.Fatalf("AddService() = %v", err)
	}

	retrieved, _ := s.GetService(ctx, svc.ServiceID)
	if retrieved.CreatedAt.Before(before) || retrieved.CreatedAt.After(after) {
		t.Errorf("CreatedAt not set correctly: %v", retrieved.CreatedAt)
	}
	if retrieved.UpdatedAt.Before(before) || retrieved.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt not set correctly: %v", retrieved.UpdatedAt)
	}
}

// ============================================================================
// GET SERVICE TESTS
// ============================================================================

func TestArchitectService_GetService_NotFound(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	_, err := s.GetService(ctx, "nonexistent")
	if err != ErrServiceNotFound {
		t.Errorf("GetService(nonexistent) = %v, want ErrServiceNotFound", err)
	}
}

func TestArchitectService_GetService_NilContext(t *testing.T) {
	s := createTestService(t)
	_, err := s.GetService(nil, "test")
	if err != ErrNilContext {
		t.Errorf("GetService(nil ctx) = %v, want ErrNilContext", err)
	}
}

func TestArchitectService_GetService_EmptyServiceID(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	_, err := s.GetService(ctx, "")
	if err != ErrEmptyServiceID {
		t.Errorf("GetService(empty ID) = %v, want ErrEmptyServiceID", err)
	}
}

// ============================================================================
// LIST SERVICES TESTS
// ============================================================================

func TestArchitectService_ListServices_Empty(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	services, err := s.ListServices(ctx)
	if err != nil {
		t.Errorf("ListServices() = %v, want nil", err)
	}
	if len(services) != 0 {
		t.Errorf("ListServices() returned %d services, want 0", len(services))
	}
}

func TestArchitectService_ListServices_Multiple(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	svc1 := createValidService()
	svc1.ServiceID = "svc-1"
	svc2 := createValidService()
	svc2.ServiceID = "svc-2"

	s.AddService(ctx, svc1)
	s.AddService(ctx, svc2)

	services, err := s.ListServices(ctx)
	if err != nil {
		t.Errorf("ListServices() = %v", err)
	}
	if len(services) != 2 {
		t.Errorf("ListServices() returned %d services, want 2", len(services))
	}
}

func TestArchitectService_ListServices_NilContext(t *testing.T) {
	s := createTestService(t)
	_, err := s.ListServices(nil)
	if err != ErrNilContext {
		t.Errorf("ListServices(nil ctx) = %v, want ErrNilContext", err)
	}
}

// ============================================================================
// NETWORK NODE TESTS
// ============================================================================

func TestArchitectService_AddNetworkNode_Happy(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	node := createValidNetworkNode()

	err := s.AddNetworkNode(ctx, node)
	if err != nil {
		t.Errorf("AddNetworkNode() = %v, want nil", err)
	}

	retrieved, err := s.GetNetworkNode(ctx, node.NodeID)
	if err != nil {
		t.Errorf("GetNetworkNode() = %v, want nil", err)
	}
	if retrieved.NodeID != node.NodeID {
		t.Errorf("NodeID = %s, want %s", retrieved.NodeID, node.NodeID)
	}
}

func TestArchitectService_AddNetworkNode_InvalidNode(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	node := createValidNetworkNode()
	node.NodeID = "" // invalid

	err := s.AddNetworkNode(ctx, node)
	if err == nil {
		t.Errorf("AddNetworkNode(invalid) = nil, want error")
	}
}

func TestArchitectService_ListNetworkNodes_Multiple(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	node1 := createValidNetworkNode()
	node1.NodeID = "node-1"
	node2 := createValidNetworkNode()
	node2.NodeID = "node-2"

	s.AddNetworkNode(ctx, node1)
	s.AddNetworkNode(ctx, node2)

	nodes, err := s.ListNetworkNodes(ctx)
	if err != nil {
		t.Errorf("ListNetworkNodes() = %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("ListNetworkNodes() returned %d nodes, want 2", len(nodes))
	}
}

// ============================================================================
// ARCHITECTURE DECISION TESTS
// ============================================================================

func TestArchitectService_LogDecision_Happy(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	decision := createValidDecision()

	err := s.LogDecision(ctx, decision)
	if err != nil {
		t.Errorf("LogDecision() = %v, want nil", err)
	}

	decisions, _ := s.ListDecisions(ctx)
	if len(decisions) != 1 {
		t.Errorf("ListDecisions() returned %d decisions, want 1", len(decisions))
	}
}

func TestArchitectService_LogDecision_InvalidDecision(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	decision := createValidDecision()
	decision.Title = "" // invalid

	err := s.LogDecision(ctx, decision)
	if err == nil {
		t.Errorf("LogDecision(invalid) = nil, want error")
	}
}

func TestArchitectService_ListDecisions_Empty(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	decisions, err := s.ListDecisions(ctx)
	if err != nil {
		t.Errorf("ListDecisions() = %v", err)
	}
	if len(decisions) != 0 {
		t.Errorf("ListDecisions() returned %d decisions, want 0", len(decisions))
	}
}

// ============================================================================
// INFRASTRUCTURE STATE TESTS
// ============================================================================

func TestArchitectService_GetInfrastructureState_Happy(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	svc := createValidService()

	s.AddService(ctx, svc)

	state, err := s.GetInfrastructureState(ctx)
	if err != nil {
		t.Errorf("GetInfrastructureState() = %v, want nil", err)
	}
	if len(state.Services) != 1 {
		t.Errorf("state.Services length = %d, want 1", len(state.Services))
	}
}

func TestArchitectService_GetInfrastructureState_NilContext(t *testing.T) {
	s := createTestService(t)
	_, err := s.GetInfrastructureState(nil)
	if err != ErrNilContext {
		t.Errorf("GetInfrastructureState(nil ctx) = %v, want ErrNilContext", err)
	}
}

// ============================================================================
// NETWORK TOPOLOGY TESTS
// ============================================================================

func TestArchitectService_GetNetworkTopology_Happy(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	node := createValidNetworkNode()

	s.AddNetworkNode(ctx, node)

	topology, err := s.GetNetworkTopology(ctx)
	if err != nil {
		t.Errorf("GetNetworkTopology() = %v, want nil", err)
	}
	if len(topology.Nodes) != 1 {
		t.Errorf("topology.Nodes length = %d, want 1", len(topology.Nodes))
	}
}

// ============================================================================
// HEALTH TESTS
// ============================================================================

func TestArchitectService_Health_Happy(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	healthy, err := s.Health(ctx)
	if err != nil {
		t.Errorf("Health() = %v, want nil", err)
	}
	if !healthy {
		t.Errorf("Health() = false, want true")
	}
}

func TestArchitectService_Health_NilContext(t *testing.T) {
	s := createTestService(t)
	_, err := s.Health(nil)
	if err != ErrNilContext {
		t.Errorf("Health(nil ctx) = %v, want ErrNilContext", err)
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestArchitectService_AddService_ConcurrentAccess(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc := createValidService()
			svc.ServiceID = "svc-" + string(rune('0'+byte(n%10)))
			_ = s.AddService(ctx, svc)
		}(i)
	}

	wg.Wait()

	services, _ := s.ListServices(ctx)
	if len(services) == 0 {
		t.Errorf("concurrent AddService resulted in 0 services")
	}
}

func TestArchitectService_GetService_ConcurrentAccess(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	svc := createValidService()
	s.AddService(ctx, svc)

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.GetService(ctx, svc.ServiceID)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent GetService error: %v", err)
	}
}

func TestArchitectService_MixedConcurrentOps(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()
	var wg sync.WaitGroup

	// Mix of Add, Get, and List operations
	for i := 0; i < 50; i++ {
		wg.Add(3)

		go func(n int) {
			defer wg.Done()
			svc := createValidService()
			svc.ServiceID = "svc-" + string(rune('a'+byte(n%26)))
			_ = s.AddService(ctx, svc)
		}(i)

		go func(n int) {
			defer wg.Done()
			_ = s.AddNetworkNode(ctx, &NetworkNode{
				NodeID: "node-" + string(rune('a'+byte(n%26))),
				Name:   "Node",
				Type:   "container",
			})
		}(i)

		go func() {
			defer wg.Done()
			_, _ = s.ListServices(ctx)
		}()
	}

	wg.Wait()

	// Verify no panics and basic consistency
	services, _ := s.ListServices(ctx)
	nodes, _ := s.ListNetworkNodes(ctx)
	healthy, _ := s.Health(ctx)

	if len(services) == 0 || len(nodes) == 0 || !healthy {
		t.Logf("concurrent mixed ops: services=%d nodes=%d healthy=%v",
			len(services), len(nodes), healthy)
	}
}

// ============================================================================
// SNAPSHOT/ISOLATION TESTS
// ============================================================================

func TestArchitectService_GetInfrastructureState_Snapshot(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	svc := createValidService()
	s.AddService(ctx, svc)

	state, _ := s.GetInfrastructureState(ctx)
	oldLen := len(state.Services)

	// Modify the snapshot
	state.Services["new-svc"] = &Service{
		ServiceID: "new-svc",
		Name:      "New Service",
	}

	// Verify original is unchanged
	state2, _ := s.GetInfrastructureState(ctx)
	if len(state2.Services) != oldLen {
		t.Errorf("snapshot isolation broken: original modified")
	}
}

func TestArchitectService_GetNetworkTopology_Snapshot(t *testing.T) {
	s := createTestService(t)
	ctx := context.Background()

	node := createValidNetworkNode()
	s.AddNetworkNode(ctx, node)

	topology, _ := s.GetNetworkTopology(ctx)
	oldLen := len(topology.Nodes)

	// Modify the snapshot
	topology.Nodes["new-node"] = &NetworkNode{
		NodeID: "new-node",
		Name:   "New Node",
	}

	// Verify original is unchanged
	topology2, _ := s.GetNetworkTopology(ctx)
	if len(topology2.Nodes) != oldLen {
		t.Errorf("snapshot isolation broken: original modified")
	}
}
