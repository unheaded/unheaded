// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package audit provides comprehensive audit logging for the Unheaded Kingdom infrastructure.
// It implements tamper-evident logging with multiple storage backends, async processing,
// and SIEM integration capabilities.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// EventType represents the category of audit event.
type EventType string

const (
	// EventTypeAuthentication covers login, logout, token refresh events.
	EventTypeAuthentication EventType = "authentication"
	// EventTypeAuthorization covers permission checks and access decisions.
	EventTypeAuthorization EventType = "authorization"
	// EventTypeConfiguration covers system and service configuration changes.
	EventTypeConfiguration EventType = "configuration"
	// EventTypeDataAccess covers read/write operations on sensitive data.
	EventTypeDataAccess EventType = "data_access"
	// EventTypeSystem covers system-level events like startup, shutdown.
	EventTypeSystem EventType = "system"
)

// Severity indicates the importance level of an audit event.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// Outcome represents the result of an audited action.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomePending Outcome = "pending"
)

// AuditEvent represents a single audit log entry with cryptographic integrity.
type AuditEvent struct {
	// ID is the unique identifier for this event.
	ID string `json:"id"`
	// Timestamp when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Type categorizes the event.
	Type EventType `json:"type"`
	// Severity indicates importance level.
	Severity Severity `json:"severity"`
	// Action describes what was attempted.
	Action string `json:"action"`
	// Outcome indicates success or failure.
	Outcome Outcome `json:"outcome"`
	// Actor identifies who performed the action.
	Actor *Actor `json:"actor"`
	// Resource identifies what was acted upon.
	Resource *Resource `json:"resource,omitempty"`
	// Details contains action-specific data.
	Details map[string]interface{} `json:"details,omitempty"`
	// Source contains information about the request origin.
	Source *Source `json:"source,omitempty"`
	// PreviousHash links to the previous event for tamper detection.
	PreviousHash string `json:"previous_hash,omitempty"`
	// Hash is the cryptographic hash of this event.
	Hash string `json:"hash"`
	// Signature is an optional cryptographic signature.
	Signature string `json:"signature,omitempty"`
}

// Actor represents the entity that performed an action.
type Actor struct {
	// ID is the unique identifier of the actor.
	ID string `json:"id"`
	// Type indicates if this is a user, service, system, etc.
	Type string `json:"type"`
	// Name is the human-readable name.
	Name string `json:"name,omitempty"`
	// Roles are the roles assigned to this actor.
	Roles []string `json:"roles,omitempty"`
	// Session identifies the session if applicable.
	Session string `json:"session,omitempty"`
}

// Resource represents the target of an action.
type Resource struct {
	// ID is the unique identifier of the resource.
	ID string `json:"id"`
	// Type indicates the kind of resource.
	Type string `json:"type"`
	// Name is the human-readable name.
	Name string `json:"name,omitempty"`
	// Path is the resource path or location.
	Path string `json:"path,omitempty"`
}

// Source contains information about where the request originated.
type Source struct {
	// IP is the source IP address.
	IP string `json:"ip,omitempty"`
	// UserAgent is the client user agent string.
	UserAgent string `json:"user_agent,omitempty"`
	// Service is the originating service name.
	Service string `json:"service,omitempty"`
	// RequestID links to the original request.
	RequestID string `json:"request_id,omitempty"`
}

// ComputeHash calculates the cryptographic hash of the event.
func (e *AuditEvent) ComputeHash() string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
		e.ID,
		e.Timestamp.UTC().Format(time.RFC3339Nano),
		e.Type,
		e.Action,
		e.Outcome,
		e.PreviousHash,
		e.serializeDetails(),
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (e *AuditEvent) serializeDetails() string {
	if e.Details == nil {
		return ""
	}
	data, _ := json.Marshal(e.Details)
	return string(data)
}

// Validate checks if the event has all required fields.
func (e *AuditEvent) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("event ID is required")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	if e.Type == "" {
		return fmt.Errorf("event type is required")
	}
	if e.Action == "" {
		return fmt.Errorf("action is required")
	}
	if e.Actor == nil || e.Actor.ID == "" {
		return fmt.Errorf("actor with ID is required")
	}
	return nil
}

// VerifyHash checks if the stored hash matches the computed hash.
func (e *AuditEvent) VerifyHash() bool {
	return e.Hash == e.ComputeHash()
}

// AuditQuery defines parameters for querying audit events.
type AuditQuery struct {
	// StartTime is the beginning of the time range.
	StartTime time.Time `json:"start_time,omitempty"`
	// EndTime is the end of the time range.
	EndTime time.Time `json:"end_time,omitempty"`
	// Types filters by event types.
	Types []EventType `json:"types,omitempty"`
	// Severities filters by severity levels.
	Severities []Severity `json:"severities,omitempty"`
	// ActorIDs filters by actor identifiers.
	ActorIDs []string `json:"actor_ids,omitempty"`
	// ResourceTypes filters by resource types.
	ResourceTypes []string `json:"resource_types,omitempty"`
	// Actions filters by specific actions.
	Actions []string `json:"actions,omitempty"`
	// Outcomes filters by outcome.
	Outcomes []Outcome `json:"outcomes,omitempty"`
	// Limit maximum number of results.
	Limit int `json:"limit,omitempty"`
	// Offset for pagination.
	Offset int `json:"offset,omitempty"`
	// OrderBy field name to sort by.
	OrderBy string `json:"order_by,omitempty"`
	// Descending sort order.
	Descending bool `json:"descending,omitempty"`
}

// AuditFilter is used for filtering events during export.
type AuditFilter struct {
	Query      *AuditQuery `json:"query,omitempty"`
	IncludeRaw bool        `json:"include_raw,omitempty"`
}

// AuditLogger is the main interface for audit logging operations.
type AuditLogger interface {
	// Log records a single audit event.
	Log(ctx context.Context, event *AuditEvent) error
	// LogBatch records multiple audit events.
	LogBatch(ctx context.Context, events []*AuditEvent) error
	// Query retrieves audit events matching the query.
	Query(ctx context.Context, query *AuditQuery) ([]*AuditEvent, error)
	// Export exports audit events in the specified format.
	Export(ctx context.Context, format string, filter *AuditFilter) (io.Reader, error)
	// VerifyIntegrity checks the hash chain integrity.
	VerifyIntegrity(ctx context.Context, startID, endID string) (bool, error)
	// Close releases resources.
	Close() error
}

// Storage is the interface for audit event persistence.
type Storage interface {
	// Store persists an audit event.
	Store(ctx context.Context, event *AuditEvent) error
	// StoreBatch persists multiple events.
	StoreBatch(ctx context.Context, events []*AuditEvent) error
	// Get retrieves a single event by ID.
	Get(ctx context.Context, id string) (*AuditEvent, error)
	// Query retrieves events matching the query.
	Query(ctx context.Context, query *AuditQuery) ([]*AuditEvent, error)
	// GetLastHash returns the hash of the most recent event.
	GetLastHash(ctx context.Context) (string, error)
	// Count returns the number of events matching the query.
	Count(ctx context.Context, query *AuditQuery) (int64, error)
	// Close releases resources.
	Close() error
}

// Exporter handles exporting audit events to various formats.
type Exporter interface {
	// Export exports events to the specified format.
	Export(ctx context.Context, events []*AuditEvent, filter *AuditFilter) (io.Reader, error)
	// Format returns the export format name.
	Format() string
}

// Config holds configuration for the audit system.
type Config struct {
	// BufferSize is the number of events to buffer before flushing.
	BufferSize int `json:"buffer_size"`
	// FlushInterval is how often to flush the buffer.
	FlushInterval time.Duration `json:"flush_interval"`
	// AsyncWorkers is the number of async workers.
	AsyncWorkers int `json:"async_workers"`
	// RetentionDays is how long to retain audit logs.
	RetentionDays int `json:"retention_days"`
	// EnableTamperDetection enables hash chain verification.
	EnableTamperDetection bool `json:"enable_tamper_detection"`
	// StorageConfig is storage-specific configuration.
	StorageConfig map[string]interface{} `json:"storage_config"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		BufferSize:            1000,
		FlushInterval:         5 * time.Second,
		AsyncWorkers:          4,
		RetentionDays:         90,
		EnableTamperDetection: true,
	}
}

// EventBuilder provides a fluent interface for building audit events.
type EventBuilder struct {
	event *AuditEvent
}

// NewEventBuilder creates a new EventBuilder.
func NewEventBuilder() *EventBuilder {
	return &EventBuilder{
		event: &AuditEvent{
			Timestamp: time.Now().UTC(),
			Details:   make(map[string]interface{}),
		},
	}
}

// WithID sets the event ID.
func (b *EventBuilder) WithID(id string) *EventBuilder {
	b.event.ID = id
	return b
}

// WithType sets the event type.
func (b *EventBuilder) WithType(t EventType) *EventBuilder {
	b.event.Type = t
	return b
}

// WithSeverity sets the severity level.
func (b *EventBuilder) WithSeverity(s Severity) *EventBuilder {
	b.event.Severity = s
	return b
}

// WithAction sets the action.
func (b *EventBuilder) WithAction(action string) *EventBuilder {
	b.event.Action = action
	return b
}

// WithOutcome sets the outcome.
func (b *EventBuilder) WithOutcome(outcome Outcome) *EventBuilder {
	b.event.Outcome = outcome
	return b
}

// WithActor sets the actor.
func (b *EventBuilder) WithActor(actor *Actor) *EventBuilder {
	b.event.Actor = actor
	return b
}

// WithResource sets the resource.
func (b *EventBuilder) WithResource(resource *Resource) *EventBuilder {
	b.event.Resource = resource
	return b
}

// WithSource sets the source information.
func (b *EventBuilder) WithSource(source *Source) *EventBuilder {
	b.event.Source = source
	return b
}

// WithDetail adds a detail key-value pair.
func (b *EventBuilder) WithDetail(key string, value interface{}) *EventBuilder {
	b.event.Details[key] = value
	return b
}

// WithDetails sets all details.
func (b *EventBuilder) WithDetails(details map[string]interface{}) *EventBuilder {
	b.event.Details = details
	return b
}

// Build returns the constructed event.
func (b *EventBuilder) Build() *AuditEvent {
	return b.event
}

// Registry manages available storage backends and exporters.
type Registry struct {
	mu        sync.RWMutex
	storages  map[string]func(config map[string]interface{}) (Storage, error)
	exporters map[string]Exporter
}

// NewRegistry creates a new Registry.
func NewRegistry() *Registry {
	return &Registry{
		storages:  make(map[string]func(config map[string]interface{}) (Storage, error)),
		exporters: make(map[string]Exporter),
	}
}

// RegisterStorage registers a storage backend factory.
func (r *Registry) RegisterStorage(name string, factory func(config map[string]interface{}) (Storage, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storages[name] = factory
}

// GetStorage creates a storage backend by name.
func (r *Registry) GetStorage(name string, config map[string]interface{}) (Storage, error) {
	r.mu.RLock()
	factory, ok := r.storages[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown storage backend: %s", name)
	}
	return factory(config)
}

// RegisterExporter registers an exporter.
func (r *Registry) RegisterExporter(exporter Exporter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exporters[exporter.Format()] = exporter
}

// GetExporter returns an exporter by format.
func (r *Registry) GetExporter(format string) (Exporter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	exporter, ok := r.exporters[format]
	return exporter, ok
}

// DefaultRegistry is the global registry instance.
var DefaultRegistry = NewRegistry()
