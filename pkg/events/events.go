// Package events provides shared event types for cross-service communication
// through the Fae Chamber (Busboy message bus).
//
// This package defines the canonical event structures that all services use
// to communicate. Using shared types ensures type safety and consistency.
package events

import (
	"encoding/json"
	"errors"
	"time"
)

// ============================================================================
// COMMON ERRORS
// ============================================================================

var (
	ErrNilEvent         = errors.New("event cannot be nil")
	ErrEmptyEventID     = errors.New("event ID cannot be empty")
	ErrEmptyTopic       = errors.New("topic cannot be empty")
	ErrEmptySender      = errors.New("sender cannot be empty")
	ErrInvalidPayload   = errors.New("invalid event payload")
	ErrMarshalFailed    = errors.New("failed to marshal event")
	ErrUnmarshalFailed  = errors.New("failed to unmarshal event")
)

// ============================================================================
// TOPICS - The Sacred Channels
// ============================================================================

// Task topics
const (
	TopicTasksCreated   = "tasks.created"
	TopicTasksUpdated   = "tasks.updated"
	TopicTasksCompleted = "tasks.completed"
	TopicTasksDeleted   = "tasks.deleted"
	TopicTasksAssigned  = "tasks.assigned"
	TopicTasksBlocked   = "tasks.blocked"
	TopicTasksAll       = "tasks.*"
)

// Timeline topics
const (
	TopicTimelineUpdates       = "timeline.updates"
	TopicTimelineMilestoneHit  = "timeline.milestone.hit"
	TopicTimelineMilestoneMiss = "timeline.milestone.missed"
	TopicTimelinePhaseStarted  = "timeline.phase.started"
	TopicTimelinePhaseComplete = "timeline.phase.completed"
	TopicTimelineSync          = "timeline.sync"
)

// Decision topics
const (
	TopicDecisionsCreated   = "decisions.created"
	TopicDecisionsApproved  = "decisions.approved"
	TopicDecisionsRejected  = "decisions.rejected"
	TopicDecisionsArchived  = "decisions.archived"
	TopicDecisionsEscalated = "decisions.escalated"
)

// Architecture topics
const (
	TopicArchitectureUpdates        = "architecture.updates"
	TopicArchitectureADRCreated     = "architecture.adr.created"
	TopicArchitectureADRUpdated     = "architecture.adr.updated"
	TopicArchitectureServiceAdded   = "architecture.service.added"
	TopicArchitectureServiceRemoved = "architecture.service.removed"
	TopicArchitectureReviewRequest  = "architecture.review.requested"
)

// Alert topics
const (
	TopicAlertsCritical  = "alerts.critical"
	TopicAlertsWarning   = "alerts.warning"
	TopicAlertsInfo      = "alerts.info"
	TopicAlertsResolved  = "alerts.resolved"
	TopicAlertsEscalated = "alerts.escalated"
)

// State topics (Cuirass)
const (
	TopicStateContainerCreated   = "state.container.created"
	TopicStateContainerDeleted   = "state.container.deleted"
	TopicStateContainerHealthy   = "state.container.healthy"
	TopicStateContainerUnhealthy = "state.container.unhealthy"
	TopicStateDriftDetected      = "state.drift.detected"
	TopicStateDriftResolved      = "state.drift.resolved"
	TopicStateReconcileStarted   = "state.reconcile.started"
	TopicStateReconcileComplete  = "state.reconcile.complete"
)

// Metrics topics
const (
	TopicMetricsCollected = "metrics.collected"
	TopicMetricsThreshold = "metrics.threshold"
	TopicMetricsAnomaly   = "metrics.anomaly"
)

// ============================================================================
// PRIORITY LEVELS
// ============================================================================

// Priority represents event priority levels
type Priority int

const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
	PriorityCritical
)

// String returns the string representation of priority
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityMedium:
		return "medium"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// ============================================================================
// BASE EVENT ENVELOPE
// ============================================================================

// Envelope wraps all events with common metadata
type Envelope struct {
	MessageID     string            `json:"message_id"`
	Topic         string            `json:"topic"`
	SenderID      string            `json:"sender_id"`
	Timestamp     time.Time         `json:"timestamp"`
	Seq           int64             `json:"seq,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	TraceID       string            `json:"trace_id,omitempty"`
	Version       string            `json:"version"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Payload       json.RawMessage   `json:"payload"`
}

// NewEnvelope creates a new event envelope
func NewEnvelope(topic, senderID string, payload interface{}) (*Envelope, error) {
	if topic == "" {
		return nil, ErrEmptyTopic
	}
	if senderID == "" {
		return nil, ErrEmptySender
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrMarshalFailed
	}

	return &Envelope{
		MessageID: generateID(),
		Topic:     topic,
		SenderID:  senderID,
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
		Payload:   payloadBytes,
	}, nil
}

// DecodePayload decodes the payload into the given type
func (e *Envelope) DecodePayload(v interface{}) error {
	if e == nil {
		return ErrNilEvent
	}
	if err := json.Unmarshal(e.Payload, v); err != nil {
		return ErrUnmarshalFailed
	}
	return nil
}

// ============================================================================
// TASK EVENTS
// ============================================================================

// TaskEvent represents a task-related event
type TaskEvent struct {
	TaskID      string    `json:"task_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Priority    Priority  `json:"priority"`
	Owner       string    `json:"owner,omitempty"`
	Assignee    string    `json:"assignee,omitempty"`
	EpicID      string    `json:"epic_id,omitempty"`
	Progress    int       `json:"progress,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// Validate validates the task event
func (t *TaskEvent) Validate() error {
	if t == nil {
		return ErrNilEvent
	}
	if t.TaskID == "" {
		return errors.New("task_id cannot be empty")
	}
	if t.Title == "" {
		return errors.New("title cannot be empty")
	}
	if t.Status == "" {
		return errors.New("status cannot be empty")
	}
	return nil
}

// ============================================================================
// TIMELINE EVENTS
// ============================================================================

// TimelineEvent represents a timeline-related event
type TimelineEvent struct {
	Event          string    `json:"event"`
	MilestoneID    string    `json:"milestone_id,omitempty"`
	MilestoneName  string    `json:"milestone_name,omitempty"`
	Phase          string    `json:"phase,omitempty"`
	ProgressBefore int       `json:"progress_before,omitempty"`
	ProgressAfter  int       `json:"progress_after,omitempty"`
	Source         string    `json:"source"`
	Timestamp      time.Time `json:"timestamp"`
}

// MilestoneHitEvent represents a milestone achievement
type MilestoneHitEvent struct {
	MilestoneID   string    `json:"milestone_id"`
	MilestoneName string    `json:"milestone_name"`
	Phase         string    `json:"phase"`
	CompletedAt   time.Time `json:"completed_at"`
	Owner         string    `json:"owner,omitempty"`
}

// ============================================================================
// DECISION EVENTS
// ============================================================================

// DecisionEvent represents a strategic decision event
type DecisionEvent struct {
	DecisionID  string    `json:"decision_id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Owner       string    `json:"owner"`
	Priority    Priority  `json:"priority"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ApprovedBy  string    `json:"approved_by,omitempty"`
	RejectedBy  string    `json:"rejected_by,omitempty"`
	RejectionReason string `json:"rejection_reason,omitempty"`
}

// Validate validates the decision event
func (d *DecisionEvent) Validate() error {
	if d == nil {
		return ErrNilEvent
	}
	if d.DecisionID == "" {
		return errors.New("decision_id cannot be empty")
	}
	if d.Title == "" {
		return errors.New("title cannot be empty")
	}
	if d.Owner == "" {
		return errors.New("owner cannot be empty")
	}
	return nil
}

// ============================================================================
// ALERT EVENTS
// ============================================================================

// AlertSeverity represents alert severity levels
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

// AlertEvent represents an alert event
type AlertEvent struct {
	AlertID         string            `json:"alert_id"`
	Severity        AlertSeverity     `json:"severity"`
	Source          string            `json:"source"`
	Title           string            `json:"title"`
	Description     string            `json:"description,omitempty"`
	AffectedService string            `json:"affected_service,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	ResolvedAt      time.Time         `json:"resolved_at,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// Validate validates the alert event
func (a *AlertEvent) Validate() error {
	if a == nil {
		return ErrNilEvent
	}
	if a.AlertID == "" {
		return errors.New("alert_id cannot be empty")
	}
	if a.Title == "" {
		return errors.New("title cannot be empty")
	}
	if a.Source == "" {
		return errors.New("source cannot be empty")
	}
	return nil
}

// ============================================================================
// ARCHITECTURE EVENTS
// ============================================================================

// ADREvent represents an Architecture Decision Record event
type ADREvent struct {
	ADRID       string    `json:"adr_id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"` // proposed, accepted, deprecated, superseded
	Context     string    `json:"context,omitempty"`
	Decision    string    `json:"decision,omitempty"`
	Consequences string   `json:"consequences,omitempty"`
	Author      string    `json:"author"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ServiceEvent represents a service definition event
type ServiceEvent struct {
	ServiceID   string            `json:"service_id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Port        int               `json:"port,omitempty"`
	Protocol    string            `json:"protocol,omitempty"` // http, grpc, tcp
	Dependencies []string         `json:"dependencies,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// ============================================================================
// STATE EVENTS (Cuirass)
// ============================================================================

// ContainerStateEvent represents a container state change
type ContainerStateEvent struct {
	ContainerID   string    `json:"container_id"`
	ContainerName string    `json:"container_name"`
	State         string    `json:"state"` // running, stopped, paused, created, deleted
	PreviousState string    `json:"previous_state,omitempty"`
	HealthStatus  string    `json:"health_status,omitempty"` // healthy, unhealthy, unknown
	Timestamp     time.Time `json:"timestamp"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// DriftEvent represents a state drift detection
type DriftEvent struct {
	DriftID       string    `json:"drift_id"`
	ContainerID   string    `json:"container_id"`
	DriftType     string    `json:"drift_type"` // missing, orphaned, status, degraded
	Severity      string    `json:"severity"`
	DesiredState  string    `json:"desired_state"`
	ActualState   string    `json:"actual_state"`
	DetectedAt    time.Time `json:"detected_at"`
	ResolvedAt    time.Time `json:"resolved_at,omitempty"`
}

// ============================================================================
// HELPERS
// ============================================================================

// generateID creates a simple unique ID
func generateID() string {
	return time.Now().Format("20060102150405.000000000")
}

// MarshalEvent marshals an event to JSON bytes
func MarshalEvent(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, ErrMarshalFailed
	}
	return data, nil
}

// UnmarshalEvent unmarshals JSON bytes to the given type
func UnmarshalEvent(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return ErrUnmarshalFailed
	}
	return nil
}
