// Package captain provides leadership and strategy for Unheaded.
// It tracks executive decisions and publishes strategy updates.
package captain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
)

// Common errors
var (
	ErrNilService      = errors.New("service cannot be nil")
	ErrNilDecision     = errors.New("decision cannot be nil")
	ErrEmptyDecision   = errors.New("decision content cannot be empty")
	ErrEmptyOwner      = errors.New("decision owner cannot be empty")
	ErrInvalidPriority = errors.New("invalid priority level")
	ErrServiceClosed   = errors.New("service is closed")
	ErrNilBusboy       = errors.New("busboy client cannot be nil")
	ErrNilStorage      = errors.New("storage cannot be nil")
)

// Priority levels for decisions
type Priority int

const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
	PriorityCritical
)

// Decision represents a strategic executive decision
type Decision struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Owner     string    `json:"owner"`
	Priority  Priority  `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Status    string    `json:"status"` // pending, approved, rejected, archived
}

// Vision represents the project vision
type Vision struct {
	Title       string `json:"title"`
	Statement   string `json:"statement"`
	Goals       []string `json:"goals"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Strategy represents the strategic plan
type Strategy struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Phases      []Phase `json:"phases"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Phase represents a strategy phase
type Phase struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Timeline    string `json:"timeline"`
}

// Validate validates the decision
func (d *Decision) Validate() error {
	if d == nil {
		return errors.New("decision cannot be nil")
	}
	if d.Title == "" {
		return errors.New("decision title cannot be empty")
	}
	if d.Content == "" {
		return errors.New("decision content cannot be empty")
	}
	if d.Owner == "" {
		return errors.New("decision owner cannot be empty")
	}
	if d.Priority < PriorityLow || d.Priority > PriorityCritical {
		return errors.New("invalid priority level")
	}
	if len(d.Title) > 500 {
		return errors.New("decision title too long (max 500 chars)")
	}
	if len(d.Content) > 10000 {
		return errors.New("decision content too long (max 10000 chars)")
	}
	if len(d.Owner) > 200 {
		return errors.New("decision owner too long (max 200 chars)")
	}
	return nil
}

// Storage interface for persistence
type Storage interface {
	SaveDecision(ctx context.Context, d *Decision) error
	GetDecision(ctx context.Context, id string) (*Decision, error)
	ListDecisions(ctx context.Context, limit, offset int) ([]*Decision, error)
	DeleteDecision(ctx context.Context, id string) error
}

// BusboyCommunicator interface for messaging
type BusboyCommunicator interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Subscribe(ctx context.Context, topic, displayName string) (*busboyClient.Subscriber, error)
	StreamMessages(ctx context.Context, topic string) (<-chan *busboyClient.Message, error)
}

// Service provides captain leadership functions
type Service struct {
	mu              sync.RWMutex
	closed          bool
	storage         Storage
	busboy          BusboyCommunicator
	vision          *Vision
	strategy        *Strategy
	metricsCallback func(metric string, value interface{})
}

// Config holds service configuration
type Config struct {
	Storage         Storage
	Busboy          BusboyCommunicator
	MetricsCallback func(string, interface{}) // Optional metrics callback
}

// NewService creates a new captain service
func NewService(cfg Config) (*Service, error) {
	if cfg.Storage == nil {
		return nil, ErrNilStorage
	}
	if cfg.Busboy == nil {
		return nil, ErrNilBusboy
	}

	s := &Service{
		storage:         cfg.Storage,
		busboy:          cfg.Busboy,
		metricsCallback: cfg.MetricsCallback,
		vision: &Vision{
			Title:       "Production-ready infrastructure in hours, not months",
			Statement:   "Unheaded delivers the complete suit of armor for modern SaaS applications",
			Goals:       []string{"eBPF observability", "Immutable infrastructure", "Zero customer data access", "Self-hosting proof"},
			UpdatedAt:   time.Now(),
		},
		strategy: &Strategy{
			Title:       "Alpha Execution Plan",
			Description: "Aggressive parallel execution with 7-8 agents",
			Phases: []Phase{
				{Name: "eBPF Foundation", Description: "Kernel tracing programs", Timeline: "Feb 3-4, 2026"},
				{Name: "Microservices", Description: "Core service implementations", Timeline: "Feb 4-5, 2026"},
				{Name: "Integration", Description: "Full system wiring", Timeline: "Feb 6-8, 2026"},
			},
			UpdatedAt: time.Now(),
		},
	}

	return s, nil
}

// GetVision returns the project vision
func (s *Service) GetVision(ctx context.Context) (*Vision, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrServiceClosed
	}

	if s.vision == nil {
		return nil, errors.New("vision not initialized")
	}

	// Return a copy to prevent mutation
	vision := *s.vision
	return &vision, nil
}

// GetStrategy returns the strategic plan
func (s *Service) GetStrategy(ctx context.Context) (*Strategy, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrServiceClosed
	}

	if s.strategy == nil {
		return nil, errors.New("strategy not initialized")
	}

	// Return a copy to prevent mutation
	strategy := *s.strategy
	return &strategy, nil
}

// LogDecision records a strategic decision
func (s *Service) LogDecision(ctx context.Context, decision *Decision) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrServiceClosed
	}
	s.mu.RUnlock()

	// Validate input
	if decision == nil {
		return ErrNilDecision
	}
	if decision.Title == "" {
		return errors.New("decision title cannot be empty")
	}
	if decision.Content == "" {
		return ErrEmptyDecision
	}
	if decision.Owner == "" {
		return ErrEmptyOwner
	}
	if decision.Priority < PriorityLow || decision.Priority > PriorityCritical {
		return ErrInvalidPriority
	}
	if len(decision.Title) > 500 {
		return errors.New("decision title too long (max 500 chars)")
	}
	if len(decision.Content) > 10000 {
		return errors.New("decision content too long (max 10000 chars)")
	}
	if len(decision.Owner) > 200 {
		return errors.New("decision owner too long (max 200 chars)")
	}

	// Set timestamps
	now := time.Now()
	decision.CreatedAt = now
	decision.UpdatedAt = now
	if decision.Status == "" {
		decision.Status = "pending"
	}

	// Generate ID if not set
	if decision.ID == "" {
		decision.ID = fmt.Sprintf("decision_%d", now.UnixNano())
	}

	// Persist decision
	if err := s.storage.SaveDecision(ctx, decision); err != nil {
		return fmt.Errorf("save decision: %w", err)
	}

	// Publish to Busboy
	payload, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("marshal decision: %w", err)
	}

	if err := s.busboy.Publish(ctx, "decisions.created", payload); err != nil {
		// Log error but don't fail the operation
		if s.metricsCallback != nil {
			s.metricsCallback("decisions.publish_error", 1)
		}
	}

	// Track metrics
	if s.metricsCallback != nil {
		s.metricsCallback("decisions.logged", 1)
		s.metricsCallback("decisions.priority", int(decision.Priority))
	}

	return nil
}

// GetDecision retrieves a specific decision by ID
func (s *Service) GetDecision(ctx context.Context, id string) (*Decision, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrServiceClosed
	}
	s.mu.RUnlock()

	if id == "" {
		return nil, errors.New("decision ID cannot be empty")
	}
	if len(id) > 200 {
		return nil, errors.New("decision ID too long")
	}

	decision, err := s.storage.GetDecision(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get decision: %w", err)
	}

	return decision, nil
}

// ListDecisions retrieves decisions with pagination
func (s *Service) ListDecisions(ctx context.Context, limit, offset int) ([]*Decision, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrServiceClosed
	}
	s.mu.RUnlock()

	// Validate bounds
	if limit <= 0 {
		limit = 10
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 1000000 {
		return nil, errors.New("offset too large")
	}

	decisions, err := s.storage.ListDecisions(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}

	return decisions, nil
}

// UpdateDecisionStatus updates the status of a decision
func (s *Service) UpdateDecisionStatus(ctx context.Context, id, newStatus string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrServiceClosed
	}
	s.mu.RUnlock()

	if id == "" {
		return errors.New("decision ID cannot be empty")
	}

	// Validate status
	validStatuses := map[string]bool{
		"pending":   true,
		"approved":  true,
		"rejected":  true,
		"archived":  true,
	}
	if !validStatuses[newStatus] {
		return errors.New("invalid status")
	}

	decision, err := s.storage.GetDecision(ctx, id)
	if err != nil {
		return fmt.Errorf("get decision: %w", err)
	}

	decision.Status = newStatus
	decision.UpdatedAt = time.Now()

	if err := s.storage.SaveDecision(ctx, decision); err != nil {
		return fmt.Errorf("update decision: %w", err)
	}

	return nil
}

// Close gracefully shuts down the service
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return nil
}

// IsClosed checks if service is closed
func (s *Service) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}
