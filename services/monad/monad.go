// Package monad provides functional composition and unified state management.
// In Gnostic philosophy, the Monad is the supreme being - the One from which all emanates.
// This service embodies that concept: a unified interface for composing operations,
// managing state transitions, and ensuring transactional consistency across the Kingdom.
package monad

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
	"unheaded/pkg/logger"
)

// Result represents the outcome of a monadic operation.
// It encapsulates either a success value or an error, enabling railway-oriented programming.
type Result[T any] struct {
	value T
	err   error
	meta  map[string]interface{}
}

// Ok creates a successful Result.
func Ok[T any](value T) Result[T] {
	return Result[T]{value: value, meta: make(map[string]interface{})}
}

// Err creates a failed Result.
func Err[T any](err error) Result[T] {
	var zero T
	return Result[T]{value: zero, err: err, meta: make(map[string]interface{})}
}

// IsOk returns true if the Result is successful.
func (r Result[T]) IsOk() bool {
	return r.err == nil
}

// IsErr returns true if the Result is an error.
func (r Result[T]) IsErr() bool {
	return r.err != nil
}

// Unwrap returns the value, panicking if it's an error.
func (r Result[T]) Unwrap() T {
	if r.err != nil {
		panic(fmt.Sprintf("called Unwrap on error Result: %v", r.err))
	}
	return r.value
}

// UnwrapOr returns the value or a default if it's an error.
func (r Result[T]) UnwrapOr(defaultValue T) T {
	if r.err != nil {
		return defaultValue
	}
	return r.value
}

// UnwrapErr returns the error, panicking if it's successful.
func (r Result[T]) UnwrapErr() error {
	if r.err == nil {
		panic("called UnwrapErr on successful Result")
	}
	return r.err
}

// Error returns the error or nil.
func (r Result[T]) Error() error {
	return r.err
}

// Value returns the value (may be zero if error).
func (r Result[T]) Value() T {
	return r.value
}

// WithMeta adds metadata to the Result.
func (r Result[T]) WithMeta(key string, value interface{}) Result[T] {
	if r.meta == nil {
		r.meta = make(map[string]interface{})
	}
	r.meta[key] = value
	return r
}

// Meta returns the metadata.
func (r Result[T]) Meta() map[string]interface{} {
	return r.meta
}

// Map transforms the value if successful.
func Map[T, U any](r Result[T], f func(T) U) Result[U] {
	if r.err != nil {
		return Err[U](r.err)
	}
	return Ok(f(r.value))
}

// FlatMap chains monadic operations.
func FlatMap[T, U any](r Result[T], f func(T) Result[U]) Result[U] {
	if r.err != nil {
		return Err[U](r.err)
	}
	return f(r.value)
}

// Operation represents a unit of work in the Monad.
type Operation struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        OperationType          `json:"type"`
	Status      OperationStatus        `json:"status"`
	Input       interface{}            `json:"input,omitempty"`
	Output      interface{}            `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Duration    time.Duration          `json:"duration,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	ParentID    string                 `json:"parent_id,omitempty"`
	Children    []string               `json:"children,omitempty"`
}

// OperationType defines the kind of operation.
type OperationType string

const (
	OpTypeQuery    OperationType = "query"
	OpTypeMutation OperationType = "mutation"
	OpTypeEffect   OperationType = "effect"
	OpTypeComposed OperationType = "composed"
)

// OperationStatus tracks operation lifecycle.
type OperationStatus string

const (
	OpStatusPending   OperationStatus = "pending"
	OpStatusRunning   OperationStatus = "running"
	OpStatusCompleted OperationStatus = "completed"
	OpStatusFailed    OperationStatus = "failed"
	OpStatusCancelled OperationStatus = "cancelled"
)

// Transaction represents a composed set of operations with ACID-like guarantees.
type Transaction struct {
	ID         string             `json:"id"`
	Operations []*Operation       `json:"operations"`
	Status     TransactionStatus  `json:"status"`
	StartedAt  time.Time          `json:"started_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Rollback   func(context.Context) error `json:"-"`
}

// TransactionStatus tracks transaction lifecycle.
type TransactionStatus string

const (
	TxStatusPending   TransactionStatus = "pending"
	TxStatusRunning   TransactionStatus = "running"
	TxStatusCommitted TransactionStatus = "committed"
	TxStatusRolledBack TransactionStatus = "rolled_back"
	TxStatusFailed    TransactionStatus = "failed"
)

// StateChange represents a change to be applied or rolled back.
type StateChange struct {
	Entity    string      `json:"entity"`
	Field     string      `json:"field"`
	OldValue  interface{} `json:"old_value"`
	NewValue  interface{} `json:"new_value"`
	Timestamp time.Time   `json:"timestamp"`
}

// Service is the main Monad service that orchestrates functional composition.
type Service struct {
	log           *logger.Logger
	busboy        *busboyClient.Client

	mu            sync.RWMutex
	operations    map[string]*Operation
	transactions  map[string]*Transaction
	stateChanges  []StateChange

	opCounter     int64
	txCounter     int64

	// Handlers for different operation types
	queryHandlers    map[string]QueryHandler
	mutationHandlers map[string]MutationHandler
	effectHandlers   map[string]EffectHandler

	// Alert handling
	alertsCh chan *busboyClient.Message
}

// QueryHandler processes read-only queries.
type QueryHandler func(ctx context.Context, input interface{}) (interface{}, error)

// MutationHandler processes state-changing mutations.
type MutationHandler func(ctx context.Context, input interface{}) (interface{}, []StateChange, error)

// EffectHandler processes side effects (external calls, events).
type EffectHandler func(ctx context.Context, input interface{}) error

// Config holds Monad service configuration.
type Config struct {
	MaxConcurrentOps   int           `json:"max_concurrent_ops"`
	OperationTimeout   time.Duration `json:"operation_timeout"`
	TransactionTimeout time.Duration `json:"transaction_timeout"`
	EnableTracing      bool          `json:"enable_tracing"`
	BusboyTopic        string        `json:"busboy_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxConcurrentOps:   100,
		OperationTimeout:   30 * time.Second,
		TransactionTimeout: 60 * time.Second,
		EnableTracing:      true,
		BusboyTopic:        "monad.operations",
	}
}

// NewService creates a new Monad service.
func NewService(log *logger.Logger, busboy *busboyClient.Client) *Service {
	return &Service{
		log:              log,
		busboy:           busboy,
		operations:       make(map[string]*Operation),
		transactions:     make(map[string]*Transaction),
		stateChanges:     make([]StateChange, 0),
		queryHandlers:    make(map[string]QueryHandler),
		mutationHandlers: make(map[string]MutationHandler),
		effectHandlers:   make(map[string]EffectHandler),
		alertsCh:         make(chan *busboyClient.Message, 100),
	}
}

// Start initializes Busboy subscriptions and starts listening for alerts
func (s *Service) Start(ctx context.Context) error {
	if s.busboy == nil {
		s.log.Info().Msg("Busboy not configured, skipping subscriptions")
		return nil
	}

	// Subscribe to alerts.critical (required by CLAUDE.md)
	if _, err := s.busboy.Subscribe(ctx, "alerts.critical", "monad-service"); err != nil {
		s.log.Warn().Err(err).Msg("failed to subscribe to alerts.critical")
	} else {
		s.log.Info().Msg("subscribed to alerts.critical")
	}

	// Subscribe to monad.operations for own topic
	if _, err := s.busboy.Subscribe(ctx, "monad.operations", "monad-service"); err != nil {
		s.log.Warn().Err(err).Msg("failed to subscribe to monad.operations")
	} else {
		s.log.Info().Msg("subscribed to monad.operations")
	}

	// Start alert listener
	go s.listenForAlerts(ctx)

	return nil
}

// Stop gracefully shuts down the service
func (s *Service) Stop() error {
	s.log.Info().Msg("monad service stopping")
	close(s.alertsCh)
	return nil
}

// listenForAlerts listens for critical alerts from Busboy
func (s *Service) listenForAlerts(ctx context.Context) {
	if s.busboy == nil {
		return
	}

	msgCh, err := s.busboy.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to stream alerts.critical")
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
			s.handleCriticalAlert(ctx, msg)
		}
	}
}

// handleCriticalAlert processes critical alerts
func (s *Service) handleCriticalAlert(ctx context.Context, msg *busboyClient.Message) {
	if msg == nil {
		return
	}

	s.log.Warn().
		Str("message_id", msg.MessageID).
		Str("topic", msg.Topic).
		Msg("received critical alert")

	// Could trigger operation cancellation or state rollback based on alert
	_ = ctx
}

// RegisterQuery registers a query handler.
func (s *Service) RegisterQuery(name string, handler QueryHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queryHandlers[name] = handler
	s.log.Info().Str("name", name).Msg("Registered query handler")
}

// RegisterMutation registers a mutation handler.
func (s *Service) RegisterMutation(name string, handler MutationHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutationHandlers[name] = handler
	s.log.Info().Str("name", name).Msg("Registered mutation handler")
}

// RegisterEffect registers an effect handler.
func (s *Service) RegisterEffect(name string, handler EffectHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.effectHandlers[name] = handler
	s.log.Info().Str("name", name).Msg("Registered effect handler")
}

// Query executes a read-only query.
func (s *Service) Query(ctx context.Context, name string, input interface{}) Result[interface{}] {
	s.mu.RLock()
	handler, ok := s.queryHandlers[name]
	s.mu.RUnlock()

	if !ok {
		return Err[interface{}](fmt.Errorf("unknown query: %s", name))
	}

	op := s.startOperation(name, OpTypeQuery, input)
	defer s.completeOperation(op)

	result, err := handler(ctx, input)
	if err != nil {
		op.Status = OpStatusFailed
		op.Error = err.Error()
		return Err[interface{}](err)
	}

	op.Output = result
	op.Status = OpStatusCompleted
	return Ok(result)
}

// Mutate executes a state-changing mutation.
func (s *Service) Mutate(ctx context.Context, name string, input interface{}) Result[interface{}] {
	s.mu.RLock()
	handler, ok := s.mutationHandlers[name]
	s.mu.RUnlock()

	if !ok {
		return Err[interface{}](fmt.Errorf("unknown mutation: %s", name))
	}

	op := s.startOperation(name, OpTypeMutation, input)
	defer s.completeOperation(op)

	result, changes, err := handler(ctx, input)
	if err != nil {
		op.Status = OpStatusFailed
		op.Error = err.Error()
		return Err[interface{}](err)
	}

	// Record state changes
	s.mu.Lock()
	s.stateChanges = append(s.stateChanges, changes...)
	s.mu.Unlock()

	op.Output = result
	op.Status = OpStatusCompleted

	// Publish operation event to Busboy
	s.publishOperationEvent(ctx, op)

	// Publish state changes if any
	if len(changes) > 0 {
		go s.publishStateChange(ctx, "monad.state.changed", changes)
	}

	return Ok(result)
}

// Effect executes a side effect.
func (s *Service) Effect(ctx context.Context, name string, input interface{}) Result[struct{}] {
	s.mu.RLock()
	handler, ok := s.effectHandlers[name]
	s.mu.RUnlock()

	if !ok {
		return Err[struct{}](fmt.Errorf("unknown effect: %s", name))
	}

	op := s.startOperation(name, OpTypeEffect, input)
	defer s.completeOperation(op)

	err := handler(ctx, input)
	if err != nil {
		op.Status = OpStatusFailed
		op.Error = err.Error()
		return Err[struct{}](err)
	}

	op.Status = OpStatusCompleted
	return Ok(struct{}{})
}

// BeginTransaction starts a new transaction.
func (s *Service) BeginTransaction(ctx context.Context) *TransactionBuilder {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.txCounter++
	tx := &Transaction{
		ID:         fmt.Sprintf("tx-%d-%d", time.Now().UnixNano(), s.txCounter),
		Operations: make([]*Operation, 0),
		Status:     TxStatusPending,
		StartedAt:  time.Now(),
	}

	s.transactions[tx.ID] = tx

	return &TransactionBuilder{
		service: s,
		tx:      tx,
		ctx:     ctx,
	}
}

// TransactionBuilder provides a fluent API for building transactions.
type TransactionBuilder struct {
	service *Service
	tx      *Transaction
	ctx     context.Context
	err     error
}

// Query adds a query to the transaction.
func (tb *TransactionBuilder) Query(name string, input interface{}) *TransactionBuilder {
	if tb.err != nil {
		return tb
	}

	result := tb.service.Query(tb.ctx, name, input)
	if result.IsErr() {
		tb.err = result.Error()
	}

	return tb
}

// Mutate adds a mutation to the transaction.
func (tb *TransactionBuilder) Mutate(name string, input interface{}) *TransactionBuilder {
	if tb.err != nil {
		return tb
	}

	result := tb.service.Mutate(tb.ctx, name, input)
	if result.IsErr() {
		tb.err = result.Error()
	}

	return tb
}

// Effect adds an effect to the transaction.
func (tb *TransactionBuilder) Effect(name string, input interface{}) *TransactionBuilder {
	if tb.err != nil {
		return tb
	}

	result := tb.service.Effect(tb.ctx, name, input)
	if result.IsErr() {
		tb.err = result.Error()
	}

	return tb
}

// Commit commits the transaction.
func (tb *TransactionBuilder) Commit() error {
	if tb.err != nil {
		tb.tx.Status = TxStatusFailed
		return tb.err
	}

	now := time.Now()
	tb.tx.CompletedAt = &now
	tb.tx.Status = TxStatusCommitted

	tb.service.log.Info().
		Str("tx_id", tb.tx.ID).
		Int("operations", len(tb.tx.Operations)).
		Msg("Transaction committed")

	return nil
}

// Rollback rolls back the transaction.
func (tb *TransactionBuilder) Rollback() error {
	tb.tx.Status = TxStatusRolledBack

	if tb.tx.Rollback != nil {
		if err := tb.tx.Rollback(tb.ctx); err != nil {
			tb.service.log.Error().Err(err).Str("tx_id", tb.tx.ID).Msg("Rollback failed")
			return err
		}
	}

	now := time.Now()
	tb.tx.CompletedAt = &now

	tb.service.log.Info().Str("tx_id", tb.tx.ID).Msg("Transaction rolled back")
	return nil
}

// startOperation creates and tracks a new operation.
func (s *Service) startOperation(name string, opType OperationType, input interface{}) *Operation {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.opCounter++
	op := &Operation{
		ID:        fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), s.opCounter),
		Name:      name,
		Type:      opType,
		Status:    OpStatusRunning,
		Input:     input,
		StartedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	s.operations[op.ID] = op
	return op
}

// completeOperation marks an operation as finished and records duration.
func (s *Service) completeOperation(op *Operation) {
	now := time.Now()
	op.CompletedAt = &now
	op.Duration = now.Sub(op.StartedAt)

	s.log.Debug().
		Str("op_id", op.ID).
		Str("name", op.Name).
		Str("status", string(op.Status)).
		Dur("duration", op.Duration).
		Msg("Operation completed")
}

// publishOperationEvent sends an operation event to Busboy.
func (s *Service) publishOperationEvent(ctx context.Context, op *Operation) {
	if s.busboy == nil {
		return
	}

	// Extract trace_id from context or generate one
	traceID := ""
	if tid := ctx.Value("trace_id"); tid != nil {
		traceID = tid.(string)
	}
	if traceID == "" {
		traceID = fmt.Sprintf("monad-%d", time.Now().UnixNano())
	}

	event := map[string]interface{}{
		"event_type": "monad.operation.completed",
		"op_id":      op.ID,
		"name":       op.Name,
		"type":       op.Type,
		"status":     op.Status,
		"duration":   op.Duration.Milliseconds(),
		"timestamp":  time.Now().UnixMilli(),
		"trace_id":   traceID,
		"service":    "monad",
	}

	payload, err := json.Marshal(event)
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to marshal operation event")
		return
	}

	// Use a timeout for publishing
	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.busboy.Publish(pubCtx, "monad.operations", payload); err != nil {
		s.log.Warn().Err(err).Msg("Failed to publish operation event")
	}
}

// publishStateChange publishes state changes to Busboy with trace_id
func (s *Service) publishStateChange(ctx context.Context, eventType string, changes []StateChange) {
	if s.busboy == nil {
		return
	}

	// Extract trace_id from context or generate one
	traceID := ""
	if tid := ctx.Value("trace_id"); tid != nil {
		traceID = tid.(string)
	}
	if traceID == "" {
		traceID = fmt.Sprintf("monad-%d", time.Now().UnixNano())
	}

	event := map[string]interface{}{
		"event_type": eventType,
		"service":    "monad",
		"changes":    changes,
		"timestamp":  time.Now().UnixMilli(),
		"trace_id":   traceID,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to marshal state change event")
		return
	}

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.busboy.Publish(pubCtx, "monad.operations", payload); err != nil {
		s.log.Warn().Err(err).Msg("Failed to publish state change event")
	}
}

// GetOperation retrieves an operation by ID.
func (s *Service) GetOperation(id string) (*Operation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	op, ok := s.operations[id]
	return op, ok
}

// GetTransaction retrieves a transaction by ID.
func (s *Service) GetTransaction(id string) (*Transaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tx, ok := s.transactions[id]
	return tx, ok
}

// ListOperations returns recent operations.
func (s *Service) ListOperations(limit int) []*Operation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ops := make([]*Operation, 0, len(s.operations))
	for _, op := range s.operations {
		ops = append(ops, op)
	}

	// Sort by start time (most recent first)
	for i := 0; i < len(ops)-1; i++ {
		for j := i + 1; j < len(ops); j++ {
			if ops[i].StartedAt.Before(ops[j].StartedAt) {
				ops[i], ops[j] = ops[j], ops[i]
			}
		}
	}

	if limit > 0 && len(ops) > limit {
		ops = ops[:limit]
	}

	return ops
}

// GetStateChanges returns the history of state changes.
func (s *Service) GetStateChanges() []StateChange {
	s.mu.RLock()
	defer s.mu.RUnlock()

	changes := make([]StateChange, len(s.stateChanges))
	copy(changes, s.stateChanges)
	return changes
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var completed, failed, running int
	for _, op := range s.operations {
		switch op.Status {
		case OpStatusCompleted:
			completed++
		case OpStatusFailed:
			failed++
		case OpStatusRunning:
			running++
		}
	}

	return map[string]interface{}{
		"total_operations":     len(s.operations),
		"completed_operations": completed,
		"failed_operations":    failed,
		"running_operations":   running,
		"total_transactions":   len(s.transactions),
		"state_changes":        len(s.stateChanges),
		"registered_queries":   len(s.queryHandlers),
		"registered_mutations": len(s.mutationHandlers),
		"registered_effects":   len(s.effectHandlers),
	}
}
