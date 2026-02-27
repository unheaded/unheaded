// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package architect provides infrastructure and architecture design tracking.
// It maintains state of infrastructure topology, network design, and architecture decisions.
package architect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
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
	infra        *InfrastructureState
	network      *NetworkTopology
	wotan       *wotanClient.Client
	alertChannel chan *wotanClient.Message
	mu           sync.RWMutex

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64
}

// WotanDrops returns the total number of messages dropped because Wotan was nil.
func (s *ArchitectService) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
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
		alertChannel: make(chan *wotanClient.Message, 100),
	}
}

// NewWithWotan creates a new ArchitectService with Wotan integration
func NewWithWotan(wotan *wotanClient.Client) *ArchitectService {
	return &ArchitectService{
		infra: &InfrastructureState{
			Services:  make(map[string]*Service),
			Decisions: make([]ArchitectureDecision, 0),
		},
		network: &NetworkTopology{
			Nodes: make(map[string]*NetworkNode),
		},
		wotan:       wotan,
		alertChannel: make(chan *wotanClient.Message, 100),
	}
}

// SetWotan sets the Wotan client for the service
func (s *ArchitectService) SetWotan(wotan *wotanClient.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wotan = wotan
}

// Start initializes Wotan subscriptions and starts listening for alerts
func (s *ArchitectService) Start(ctx context.Context) error {
	if s.wotan == nil {
		log.Println("[WARN] architect: wotan unavailable — subscriptions disabled (degraded mode)")
		return nil
	}

	// Subscribe to alerts.critical (required by CLAUDE.md)
	if _, err := s.wotan.Subscribe(ctx, "alerts.critical", "architect-service"); err != nil {
		return fmt.Errorf("subscribe to alerts.critical: %w", err)
	}

	// Subscribe to architecture.updates for own topic
	if _, err := s.wotan.Subscribe(ctx, "architecture.updates", "architect-service"); err != nil {
		return fmt.Errorf("subscribe to architecture.updates: %w", err)
	}

	// Start alert listener
	go s.listenForAlerts(ctx)

	return nil
}

// listenForAlerts listens for critical alerts from Wotan
func (s *ArchitectService) listenForAlerts(ctx context.Context) {
	if s.wotan == nil {
		log.Println("[WARN] architect: wotan unavailable — alert listener disabled (degraded mode)")
		return
	}

	msgCh, err := s.wotan.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			// Handle critical alerts - could trigger infrastructure state changes
			s.handleCriticalAlert(ctx, msg)
		}
	}
}

// handleCriticalAlert processes critical alerts
func (s *ArchitectService) handleCriticalAlert(ctx context.Context, msg *wotanClient.Message) {
	if msg == nil {
		return
	}
	// Log and potentially update infrastructure state based on alert
	// For now, just acknowledge receipt
	_ = ctx
}

// publishStateChange publishes a state change event to Wotan
func (s *ArchitectService) publishStateChange(ctx context.Context, eventType string, data interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		log.Printf("[WARN] architect: wotan unavailable — %s event dropped (degraded mode)", eventType)
		return
	}

	// Extract trace_id from context if available
	traceID := ""
	if tid := ctx.Value("trace_id"); tid != nil {
		traceID = tid.(string)
	}
	if traceID == "" {
		traceID = fmt.Sprintf("arch-%d", time.Now().UnixNano())
	}

	event := map[string]interface{}{
		"event_type": eventType,
		"service":    "architect",
		"data":       data,
		"timestamp":  time.Now().UnixMilli(),
		"trace_id":   traceID,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	// Use a short timeout for publishing
	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.wotan.Publish(pubCtx, "architecture.updates", payload); err != nil {
		log.Printf("architect: wotan publish to architecture.updates failed: %v", err)
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

	// Publish state change to Wotan
	go s.publishStateChange(ctx, "service.added", map[string]interface{}{
		"service_id": service.ServiceID,
		"name":       service.Name,
		"type":       service.Type,
		"status":     service.Status,
	})

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

	// Publish state change to Wotan
	go s.publishStateChange(ctx, "network.node_added", map[string]interface{}{
		"node_id": node.NodeID,
		"name":    node.Name,
		"type":    node.Type,
		"cidr":    node.CIDR,
	})

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

	// Publish state change to Wotan
	go s.publishStateChange(ctx, "decision.logged", map[string]interface{}{
		"decision_id": decision.DecisionID,
		"title":       decision.Title,
		"component":   decision.Component,
		"status":      decision.Status,
	})

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
