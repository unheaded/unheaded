// Package architect provides infrastructure and architecture design tracking.
// It maintains state of infrastructure topology, network design, and architecture decisions.
package architect

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Common errors
var (
	ErrNilInput              = errors.New("input cannot be nil")
	ErrEmptyName             = errors.New("name cannot be empty")
	ErrEmptyServiceID        = errors.New("service ID cannot be empty")
	ErrServiceNotFound       = errors.New("service not found")
	ErrNilContext            = errors.New("context cannot be nil")
	ErrEmptyDecision         = errors.New("decision description cannot be empty")
	ErrInfraNotInitialized   = errors.New("infrastructure state not initialized")
	ErrNetworkNotInitialized = errors.New("network state not initialized")
)

// Service represents an infrastructure service
type Service struct {
	ServiceID string                 `json:"service_id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Status    string                 `json:"status"`
	Address   string                 `json:"address"`
	Port      int                    `json:"port"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Validate validates the Service
func (s *Service) Validate() error {
	if s == nil {
		return ErrNilInput
	}
	if s.ServiceID == "" {
		return ErrEmptyServiceID
	}
	if s.Name == "" {
		return ErrEmptyName
	}
	if s.Port < 0 || s.Port > 65535 {
		return fmt.Errorf("invalid port: %d", s.Port)
	}
	return nil
}

// NetworkNode represents a node in the network topology
type NetworkNode struct {
	NodeID    string                 `json:"node_id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"` // container, service, gateway, etc
	CIDR      string                 `json:"cidr"`
	Status    string                 `json:"status"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Validate validates the NetworkNode
func (n *NetworkNode) Validate() error {
	if n == nil {
		return ErrNilInput
	}
	if n.NodeID == "" {
		return errors.New("node ID cannot be empty")
	}
	if n.Name == "" {
		return ErrEmptyName
	}
	return nil
}

// ArchitectureDecision represents a recorded architecture decision
type ArchitectureDecision struct {
	DecisionID  string                 `json:"decision_id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Component   string                 `json:"component"`
	Status      string                 `json:"status"` // proposed, approved, implemented, deprecated
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Validate validates the ArchitectureDecision
func (a *ArchitectureDecision) Validate() error {
	if a == nil {
		return ErrNilInput
	}
	if a.Title == "" {
		return errors.New("title cannot be empty")
	}
	if a.Description == "" {
		return ErrEmptyDecision
	}
	return nil
}

// InfrastructureState represents the complete infrastructure state
type InfrastructureState struct {
	Services  map[string]*Service  `json:"services"`
	Decisions []ArchitectureDecision `json:"decisions"`
	mu        sync.RWMutex
}

// NetworkTopology represents the network topology
type NetworkTopology struct {
	Nodes map[string]*NetworkNode `json:"nodes"`
	mu    sync.RWMutex
}

// ArchitectService provides infrastructure and design tracking
type ArchitectService struct {
	infra   *InfrastructureState
	network *NetworkTopology
	mu      sync.RWMutex
}

// New creates a new ArchitectService
func New() *ArchitectService {
	return &ArchitectService{
		infra: &InfrastructureState{
			Services:  make(map[string]*Service),
			Decisions: make([]ArchitectureDecision, 0),
		},
		network: &NetworkTopology{
			Nodes: make(map[string]*NetworkNode),
		},
	}
}

// ============================================================================
// INFRASTRUCTURE METHODS
// ============================================================================

// AddService adds a service to infrastructure state
func (s *ArchitectService) AddService(ctx context.Context, service *Service) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := service.Validate(); err != nil {
		return fmt.Errorf("validate service: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.infra == nil {
		return ErrInfraNotInitialized
	}

	service.CreatedAt = time.Now()
	service.UpdatedAt = time.Now()
	s.infra.Services[service.ServiceID] = service

	return nil
}

// GetService retrieves a service by ID
func (s *ArchitectService) GetService(ctx context.Context, serviceID string) (*Service, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if serviceID == "" {
		return nil, ErrEmptyServiceID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.infra == nil {
		return nil, ErrInfraNotInitialized
	}

	service, ok := s.infra.Services[serviceID]
	if !ok {
		return nil, ErrServiceNotFound
	}

	return service, nil
}

// ListServices returns all services
func (s *ArchitectService) ListServices(ctx context.Context) ([]*Service, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.infra == nil {
		return nil, ErrInfraNotInitialized
	}

	services := make([]*Service, 0, len(s.infra.Services))
	for _, service := range s.infra.Services {
		services = append(services, service)
	}

	return services, nil
}

// GetInfrastructureState returns the complete infrastructure state
func (s *ArchitectService) GetInfrastructureState(ctx context.Context) (*InfrastructureState, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.infra == nil {
		return nil, ErrInfraNotInitialized
	}

	// Create a snapshot to prevent external mutation
	state := &InfrastructureState{
		Services:  make(map[string]*Service),
		Decisions: make([]ArchitectureDecision, len(s.infra.Decisions)),
	}

	for k, v := range s.infra.Services {
		state.Services[k] = v
	}
	copy(state.Decisions, s.infra.Decisions)

	return state, nil
}

// ============================================================================
// NETWORK METHODS
// ============================================================================

// AddNetworkNode adds a node to the network topology
func (s *ArchitectService) AddNetworkNode(ctx context.Context, node *NetworkNode) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := node.Validate(); err != nil {
		return fmt.Errorf("validate node: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.network == nil {
		return ErrNetworkNotInitialized
	}

	node.CreatedAt = time.Now()
	node.UpdatedAt = time.Now()
	s.network.Nodes[node.NodeID] = node

	return nil
}

// GetNetworkNode retrieves a network node by ID
func (s *ArchitectService) GetNetworkNode(ctx context.Context, nodeID string) (*NetworkNode, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if nodeID == "" {
		return nil, errors.New("node ID cannot be empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.network == nil {
		return nil, ErrNetworkNotInitialized
	}

	node, ok := s.network.Nodes[nodeID]
	if !ok {
		return nil, errors.New("network node not found")
	}

	return node, nil
}

// ListNetworkNodes returns all network nodes
func (s *ArchitectService) ListNetworkNodes(ctx context.Context) ([]*NetworkNode, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.network == nil {
		return nil, ErrNetworkNotInitialized
	}

	nodes := make([]*NetworkNode, 0, len(s.network.Nodes))
	for _, node := range s.network.Nodes {
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetNetworkTopology returns the complete network topology
func (s *ArchitectService) GetNetworkTopology(ctx context.Context) (*NetworkTopology, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.network == nil {
		return nil, ErrNetworkNotInitialized
	}

	// Create a snapshot
	topology := &NetworkTopology{
		Nodes: make(map[string]*NetworkNode),
	}

	for k, v := range s.network.Nodes {
		topology.Nodes[k] = v
	}

	return topology, nil
}

// ============================================================================
// ARCHITECTURE DECISION METHODS
// ============================================================================

// LogDecision records an architecture decision
func (s *ArchitectService) LogDecision(ctx context.Context, decision *ArchitectureDecision) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := decision.Validate(); err != nil {
		return fmt.Errorf("validate decision: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.infra == nil {
		return ErrInfraNotInitialized
	}

	decision.CreatedAt = time.Now()
	decision.UpdatedAt = time.Now()
	s.infra.Decisions = append(s.infra.Decisions, *decision)

	return nil
}

// ListDecisions returns all recorded decisions
func (s *ArchitectService) ListDecisions(ctx context.Context) ([]ArchitectureDecision, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.infra == nil {
		return nil, ErrInfraNotInitialized
	}

	decisions := make([]ArchitectureDecision, len(s.infra.Decisions))
	copy(decisions, s.infra.Decisions)

	return decisions, nil
}

// Health returns the health status
func (s *ArchitectService) Health(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, ErrNilContext
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.infra != nil && s.network != nil, nil
}
