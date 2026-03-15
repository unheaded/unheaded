// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package events - Event Router for Cross-Service Communication
//
// The Router provides a centralized way to register handlers for events
// and route incoming messages to the appropriate handlers.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/logger"
)

var log = logger.New(os.Stderr)

// ============================================================================
// ROUTER ERRORS
// ============================================================================

var (
	ErrRouterClosed     = errors.New("router is closed")
	ErrHandlerExists    = errors.New("handler already registered for topic")
	ErrNoHandler        = errors.New("no handler registered for topic")
	ErrNilHandler       = errors.New("handler cannot be nil")
	ErrInvalidMessage   = errors.New("invalid message format")
)

// ============================================================================
// HANDLER TYPES
// ============================================================================

// Handler processes an event message
type Handler func(ctx context.Context, topic string, payload []byte) error

// TypedHandler processes a specific event type
type TypedHandler[T any] func(ctx context.Context, event *T) error

// ============================================================================
// ROUTER
// ============================================================================

// Router routes events to registered handlers
type Router struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	closed   bool

	// Metrics
	messagesRouted   int64
	messagesDropped  int64
	handlerErrors    int64

	// Configuration
	defaultTimeout time.Duration
}

// RouterConfig configures the router
type RouterConfig struct {
	DefaultTimeout time.Duration // Default timeout for handlers (default: 30s)
}

// NewRouter creates a new event router
func NewRouter(cfg *RouterConfig) *Router {
	timeout := 30 * time.Second
	if cfg != nil && cfg.DefaultTimeout > 0 {
		timeout = cfg.DefaultTimeout
	}

	return &Router{
		handlers:       make(map[string][]Handler),
		defaultTimeout: timeout,
	}
}

// Register registers a handler for a topic pattern
// Pattern can include wildcards: "tasks.*" matches "tasks.created", "tasks.updated", etc.
func (r *Router) Register(pattern string, handler Handler) error {
	if pattern == "" {
		return ErrEmptyTopic
	}
	if handler == nil {
		return ErrNilHandler
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrRouterClosed
	}

	r.handlers[pattern] = append(r.handlers[pattern], handler)

	log.Debug().
		Str("pattern", pattern).
		Int("total_handlers", len(r.handlers[pattern])).
		Msg("registered event handler")

	return nil
}

// RegisterTyped registers a typed handler for a specific event type
// This provides type-safe event handling
func RegisterTyped[T any](r *Router, pattern string, handler TypedHandler[T]) error {
	return r.Register(pattern, func(ctx context.Context, topic string, payload []byte) error {
		var event T
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("unmarshal event: %w", err)
		}
		return handler(ctx, &event)
	})
}

// Route routes a message to all matching handlers
func (r *Router) Route(ctx context.Context, topic string, payload []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return ErrRouterClosed
	}

	// Find all matching handlers
	var matchingHandlers []Handler
	for pattern, handlers := range r.handlers {
		if matchesTopic(pattern, topic) {
			matchingHandlers = append(matchingHandlers, handlers...)
		}
	}
	r.mu.RUnlock()

	if len(matchingHandlers) == 0 {
		log.Debug().Msg("no handlers registered for topic")
		r.incrementDropped()
		return nil // Not an error - just no subscribers
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, r.defaultTimeout)
	defer cancel()

	// Execute all handlers
	var lastErr error
	for _, handler := range matchingHandlers {
		if err := handler(timeoutCtx, topic, payload); err != nil {
			log.Error().
				Err(err).
				Str("topic", topic).
				Msg("handler error")
			lastErr = err
			r.incrementErrors()
		}
	}

	r.incrementRouted()
	return lastErr
}

// RouteEnvelope routes an envelope to all matching handlers
func (r *Router) RouteEnvelope(ctx context.Context, env *Envelope) error {
	if env == nil {
		return ErrNilEvent
	}
	return r.Route(ctx, env.Topic, env.Payload)
}

// Close closes the router and prevents new registrations/routing
func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	r.handlers = nil

	log.Info().
		Int64("messages_routed", r.messagesRouted).
		Int64("messages_dropped", r.messagesDropped).
		Int64("handler_errors", r.handlerErrors).
		Msg("event router closed")

	return nil
}

// Stats returns router statistics
func (r *Router) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handlerCount := 0
	for _, h := range r.handlers {
		handlerCount += len(h)
	}

	return map[string]interface{}{
		"closed":           r.closed,
		"topic_patterns":   len(r.handlers),
		"total_handlers":   handlerCount,
		"messages_routed":  r.messagesRouted,
		"messages_dropped": r.messagesDropped,
		"handler_errors":   r.handlerErrors,
	}
}

// ============================================================================
// METRICS HELPERS
// ============================================================================

func (r *Router) incrementRouted() {
	r.mu.Lock()
	r.messagesRouted++
	r.mu.Unlock()
}

func (r *Router) incrementDropped() {
	r.mu.Lock()
	r.messagesDropped++
	r.mu.Unlock()
}

func (r *Router) incrementErrors() {
	r.mu.Lock()
	r.handlerErrors++
	r.mu.Unlock()
}

// ============================================================================
// TOPIC MATCHING
// ============================================================================

// matchesTopic checks if a pattern matches a topic
// Supports wildcards:
//   - "tasks.*" matches "tasks.created", "tasks.updated"
//   - "*.critical" matches "alerts.critical", "state.critical"
//   - "*" matches everything
func matchesTopic(pattern, topic string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == topic {
		return true
	}

	// Handle wildcard patterns
	if strings.Contains(pattern, "*") {
		// Split by wildcard
		parts := strings.Split(pattern, "*")
		if len(parts) != 2 {
			// Only support single wildcard for now
			return pattern == topic
		}

		prefix := parts[0]
		suffix := parts[1]

		hasPrefix := prefix == "" || strings.HasPrefix(topic, prefix)
		hasSuffix := suffix == "" || strings.HasSuffix(topic, suffix)

		return hasPrefix && hasSuffix
	}

	return false
}

// ============================================================================
// CROSS-SERVICE ROUTING PATTERNS
// ============================================================================

// StandardRoutes sets up standard cross-service routing patterns
// This is a convenience function for common Kingdom routing
func (r *Router) StandardRoutes() error {
	// All services should handle critical alerts
	if err := r.Register(TopicAlertsCritical, func(ctx context.Context, topic string, payload []byte) error {
		var alert AlertEvent
		if err := json.Unmarshal(payload, &alert); err != nil {
			return err
		}
		log.Warn().
			Str("alert_id", alert.AlertID).
			Str("severity", string(alert.Severity)).
			Str("source", alert.Source).
			Str("title", alert.Title).
			Msg("CRITICAL ALERT received")
		return nil
	}); err != nil {
		return fmt.Errorf("register critical alerts: %w", err)
	}

	// Log all timeline milestone events
	if err := r.Register("timeline.milestone.*", func(ctx context.Context, topic string, payload []byte) error {
		var milestone MilestoneHitEvent
		if err := json.Unmarshal(payload, &milestone); err != nil {
			return err
		}
		log.Info().
			Str("milestone_id", milestone.MilestoneID).
			Str("milestone", milestone.MilestoneName).
			Str("phase", milestone.Phase).
			Msg("🎉 MILESTONE achieved!")
		return nil
	}); err != nil {
		return fmt.Errorf("register milestone events: %w", err)
	}

	return nil
}

// ============================================================================
// SERVICE-SPECIFIC ROUTING HELPERS
// ============================================================================

// TimeguruRoutes sets up routes for Timeguru service
func TimeguruRoutes(r *Router, onTaskCompleted func(ctx context.Context, event *TaskEvent) error) error {
	// Listen for task completions to update timeline
	return RegisterTyped(r, TopicTasksCompleted, onTaskCompleted)
}

// CaptainRoutes sets up routes for Captain service
func CaptainRoutes(r *Router, onMilestone func(ctx context.Context, event *MilestoneHitEvent) error) error {
	// Listen for milestone achievements
	return RegisterTyped(r, TopicTimelineMilestoneHit, onMilestone)
}

// MicromanagerRoutes sets up routes for Micromanager service
func MicromanagerRoutes(r *Router, onDecisionApproved func(ctx context.Context, event *DecisionEvent) error) error {
	// Listen for approved decisions to create implementation tasks
	return RegisterTyped(r, TopicDecisionsApproved, onDecisionApproved)
}

// ArchitectRoutes sets up routes for Architect service
func ArchitectRoutes(r *Router, onDecisionApproved func(ctx context.Context, event *DecisionEvent) error) error {
	// Listen for approved decisions to create ADRs
	return RegisterTyped(r, TopicDecisionsApproved, onDecisionApproved)
}
