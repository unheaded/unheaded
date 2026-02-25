// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package anamnesis provides event sourcing, history keeping, and state reconstruction.
// In Gnostic philosophy, Anamnesis is the soul's remembrance of its divine origin.
// This service embodies that concept: remembering every state change, reconstructing the past,
// providing audit trails, and enabling time travel through the Kingdom's history.
package anamnesis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
)

// Event represents a single state change in the Kingdom's history.
type Event struct {
	ID            string                 `json:"id"`
	SequenceNum   int64                  `json:"sequence_num"`
	Type          EventType              `json:"type"`
	AggregateID   string                 `json:"aggregate_id"`
	AggregateType string                 `json:"aggregate_type"`
	Payload       map[string]interface{} `json:"payload"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
	CausationID   string                 `json:"causation_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Actor         string                 `json:"actor,omitempty"`
	Version       int64                  `json:"version"`
	Hash          string                 `json:"hash"`
	PrevHash      string                 `json:"prev_hash,omitempty"`
}

// EventType categorizes events.
type EventType string

const (
	EventCreated   EventType = "created"
	EventUpdated   EventType = "updated"
	EventDeleted   EventType = "deleted"
	EventCommand   EventType = "command"
	EventQuery     EventType = "query"
	EventEffect    EventType = "effect"
	EventSnapshot  EventType = "snapshot"
	EventRollback  EventType = "rollback"
)

// Snapshot represents a point-in-time state capture.
type Snapshot struct {
	ID            string                 `json:"id"`
	AggregateID   string                 `json:"aggregate_id"`
	AggregateType string                 `json:"aggregate_type"`
	State         map[string]interface{} `json:"state"`
	Version       int64                  `json:"version"`
	EventID       string                 `json:"event_id"`
	CreatedAt     time.Time              `json:"created_at"`
	Hash          string                 `json:"hash"`
}

// EventStream represents a sequence of events for an aggregate.
type EventStream struct {
	AggregateID   string   `json:"aggregate_id"`
	AggregateType string   `json:"aggregate_type"`
	Events        []*Event `json:"events"`
	Version       int64    `json:"version"`
}

// Query represents a history query.
type Query struct {
	AggregateID   string    `json:"aggregate_id,omitempty"`
	AggregateType string    `json:"aggregate_type,omitempty"`
	EventTypes    []EventType `json:"event_types,omitempty"`
	FromTime      *time.Time `json:"from_time,omitempty"`
	ToTime        *time.Time `json:"to_time,omitempty"`
	FromVersion   int64      `json:"from_version,omitempty"`
	ToVersion     int64      `json:"to_version,omitempty"`
	Limit         int        `json:"limit,omitempty"`
	Offset        int        `json:"offset,omitempty"`
	Actor         string     `json:"actor,omitempty"`
	CorrelationID string     `json:"correlation_id,omitempty"`
}

// Projection represents a read model built from events.
type Projection struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	State       map[string]interface{} `json:"state"`
	Version     int64                  `json:"version"`
	LastEventID string                 `json:"last_event_id"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// ProjectionHandler processes events to update a projection.
type ProjectionHandler func(projection *Projection, event *Event) error

// EventHandler is called when an event is appended.
type EventHandler func(ctx context.Context, event *Event) error

// Service is the main Anamnesis service for event sourcing and history.
type Service struct {
	log    *logger.Logger
	wotan *wotanClient.Client
	config *Config

	mu           sync.RWMutex
	events       []*Event                // WAL - Write-Ahead Log
	eventIndex   map[string]*Event       // EventID -> Event
	aggregateIndex map[string][]int      // AggregateID -> event indices
	snapshots    map[string][]*Snapshot  // AggregateID -> snapshots
	projections  map[string]*Projection  // ProjectionID -> projection

	// Handlers
	eventHandlers     []EventHandler
	projectionHandlers map[string]ProjectionHandler

	// Counters
	sequenceNum   int64
	snapshotCounter int64
}

// Config holds Anamnesis service configuration.
type Config struct {
	MaxEvents          int           `json:"max_events"`
	SnapshotInterval   int           `json:"snapshot_interval"` // Events between snapshots
	RetentionPeriod    time.Duration `json:"retention_period"`
	CompactionInterval time.Duration `json:"compaction_interval"`
	EnableSnapshots    bool          `json:"enable_snapshots"`
	WotanTopic        string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxEvents:          1000000,
		SnapshotInterval:   100,
		RetentionPeriod:    30 * 24 * time.Hour, // 30 days
		CompactionInterval: 1 * time.Hour,
		EnableSnapshots:    true,
		WotanTopic:        "anamnesis.events",
	}
}

// NewService creates a new Anamnesis service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if log == nil {
		log = logger.New(io.Discard)
	}

	return &Service{
		log:                log,
		wotan:             wotan,
		config:             cfg,
		events:             make([]*Event, 0),
		eventIndex:         make(map[string]*Event),
		aggregateIndex:     make(map[string][]int),
		snapshots:          make(map[string][]*Snapshot),
		projections:        make(map[string]*Projection),
		eventHandlers:      make([]EventHandler, 0),
		projectionHandlers: make(map[string]ProjectionHandler),
	}
}

// Start initializes the Anamnesis service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Anamnesis awakens - the soul remembers all that has passed")

	// Start compaction loop
	go s.compactionLoop(ctx)

	// Subscribe to events
	if s.wotan != nil {
		go s.subscribeToEvents(ctx)
	}

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Anamnesis rests - history preserved for eternity")
	return nil
}

// Append records a new event.
func (s *Service) Append(ctx context.Context, aggregateID, aggregateType string, eventType EventType, payload map[string]interface{}) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sequenceNum++
	now := time.Now()

	// Get previous hash for chain integrity
	prevHash := ""
	if len(s.events) > 0 {
		prevHash = s.events[len(s.events)-1].Hash
	}

	// Calculate aggregate version
	version := int64(1)
	if indices, ok := s.aggregateIndex[aggregateID]; ok {
		if len(indices) > 0 {
			lastEvent := s.events[indices[len(indices)-1]]
			version = lastEvent.Version + 1
		}
	}

	event := &Event{
		ID:            fmt.Sprintf("evt-%d-%d", now.UnixNano(), s.sequenceNum),
		SequenceNum:   s.sequenceNum,
		Type:          eventType,
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		Payload:       payload,
		Metadata:      make(map[string]interface{}),
		Timestamp:     now,
		Version:       version,
		PrevHash:      prevHash,
	}

	// Compute hash for integrity
	event.Hash = s.computeEventHash(event)

	// Store event
	eventIndex := len(s.events)
	s.events = append(s.events, event)
	s.eventIndex[event.ID] = event
	s.aggregateIndex[aggregateID] = append(s.aggregateIndex[aggregateID], eventIndex)

	s.log.Debug().
		Str("event_id", event.ID).
		Str("aggregate_id", aggregateID).
		Str("type", string(eventType)).
		Int64("version", version).
		Int64("sequence", s.sequenceNum).
		Msg("Event appended")

	// Snapshot handlers under the write lock so goroutines don't need to
	// re-acquire the lock just to read the handler slices.
	handlersCopy := make([]EventHandler, len(s.eventHandlers))
	copy(handlersCopy, s.eventHandlers)
	projHandlersCopy := make(map[string]ProjectionHandler, len(s.projectionHandlers))
	for k, v := range s.projectionHandlers {
		projHandlersCopy[k] = v
	}

	// Notify handlers with the snapshot (no lock needed inside the goroutine
	// for reading the handler list).
	go s.notifyHandlersSnapshot(ctx, event, handlersCopy, projHandlersCopy)

	// Check if snapshot is needed
	if s.config.EnableSnapshots && version%int64(s.config.SnapshotInterval) == 0 {
		go s.createSnapshot(ctx, aggregateID, aggregateType, version)
	}

	// Publish to Wotan
	s.publishEvent(ctx, "anamnesis.event.appended", map[string]interface{}{
		"event_id":       event.ID,
		"aggregate_id":   aggregateID,
		"aggregate_type": aggregateType,
		"event_type":     eventType,
		"version":        version,
	})

	return event, nil
}

// GetEvent retrieves an event by ID.
func (s *Service) GetEvent(eventID string) (*Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	event, ok := s.eventIndex[eventID]
	return event, ok
}

// GetEventStream retrieves all events for an aggregate.
func (s *Service) GetEventStream(aggregateID string) (*EventStream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	indices, ok := s.aggregateIndex[aggregateID]
	if !ok || len(indices) == 0 {
		return nil, fmt.Errorf("no events found for aggregate: %s", aggregateID)
	}

	events := make([]*Event, len(indices))
	var aggregateType string
	var version int64

	for i, idx := range indices {
		events[i] = s.events[idx]
		aggregateType = events[i].AggregateType
		version = events[i].Version
	}

	return &EventStream{
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		Events:        events,
		Version:       version,
	}, nil
}

// Query searches events based on criteria.
func (s *Service) Query(ctx context.Context, q *Query) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Event

	// Start with all events or aggregate-specific events
	var candidates []*Event
	if q.AggregateID != "" {
		indices := s.aggregateIndex[q.AggregateID]
		candidates = make([]*Event, len(indices))
		for i, idx := range indices {
			candidates[i] = s.events[idx]
		}
	} else {
		candidates = s.events
	}

	for _, event := range candidates {
		// Filter by aggregate type
		if q.AggregateType != "" && event.AggregateType != q.AggregateType {
			continue
		}

		// Filter by event types
		if len(q.EventTypes) > 0 {
			found := false
			for _, t := range q.EventTypes {
				if event.Type == t {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Filter by time range
		if q.FromTime != nil && event.Timestamp.Before(*q.FromTime) {
			continue
		}
		if q.ToTime != nil && event.Timestamp.After(*q.ToTime) {
			continue
		}

		// Filter by version range
		if q.FromVersion > 0 && event.Version < q.FromVersion {
			continue
		}
		if q.ToVersion > 0 && event.Version > q.ToVersion {
			continue
		}

		// Filter by actor
		if q.Actor != "" && event.Actor != q.Actor {
			continue
		}

		// Filter by correlation ID
		if q.CorrelationID != "" && event.CorrelationID != q.CorrelationID {
			continue
		}

		results = append(results, event)
	}

	// Apply offset
	if q.Offset > 0 && q.Offset < len(results) {
		results = results[q.Offset:]
	}

	// Apply limit
	if q.Limit > 0 && len(results) > q.Limit {
		results = results[:q.Limit]
	}

	return results, nil
}

// ReconstructState rebuilds state from events.
func (s *Service) ReconstructState(ctx context.Context, aggregateID string, atVersion int64) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := make(map[string]interface{})

	// Start from latest snapshot before target version
	snapshots := s.snapshots[aggregateID]
	startVersion := int64(0)

	for _, snap := range snapshots {
		if snap.Version <= atVersion && snap.Version > startVersion {
			// Copy snapshot state
			for k, v := range snap.State {
				state[k] = v
			}
			startVersion = snap.Version
		}
	}

	// Apply events from snapshot to target version
	indices := s.aggregateIndex[aggregateID]
	for _, idx := range indices {
		event := s.events[idx]
		if event.Version <= startVersion {
			continue
		}
		if event.Version > atVersion {
			break
		}

		// Apply event to state
		s.applyEventToState(state, event)
	}

	return state, nil
}

// applyEventToState applies an event's changes to state.
func (s *Service) applyEventToState(state map[string]interface{}, event *Event) {
	switch event.Type {
	case EventCreated, EventUpdated:
		for k, v := range event.Payload {
			state[k] = v
		}
	case EventDeleted:
		for k := range event.Payload {
			delete(state, k)
		}
	}
}

// GetLatestSnapshot retrieves the most recent snapshot for an aggregate.
func (s *Service) GetLatestSnapshot(aggregateID string) (*Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshots := s.snapshots[aggregateID]
	if len(snapshots) == 0 {
		return nil, false
	}

	return snapshots[len(snapshots)-1], true
}

// RegisterEventHandler registers a handler called on each event.
func (s *Service) RegisterEventHandler(handler EventHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventHandlers = append(s.eventHandlers, handler)
}

// RegisterProjection registers a projection with its handler.
func (s *Service) RegisterProjection(name string, handler ProjectionHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.projectionHandlers[name] = handler
	s.projections[name] = &Projection{
		ID:      fmt.Sprintf("proj-%s", name),
		Name:    name,
		State:   make(map[string]interface{}),
		Version: 0,
	}

	s.log.Debug().Str("name", name).Msg("Projection registered")
}

// GetProjection retrieves a projection's current state.
func (s *Service) GetProjection(name string) (*Projection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	proj, ok := s.projections[name]
	return proj, ok
}

// RebuildProjection replays all events through a projection.
func (s *Service) RebuildProjection(ctx context.Context, name string) error {
	s.mu.Lock()
	handler, ok := s.projectionHandlers[name]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("projection not found: %s", name)
	}

	proj := s.projections[name]
	proj.State = make(map[string]interface{}) // Reset state
	proj.Version = 0
	s.mu.Unlock()

	// Replay all events
	s.mu.RLock()
	events := make([]*Event, len(s.events))
	copy(events, s.events)
	s.mu.RUnlock()

	for _, event := range events {
		if err := handler(proj, event); err != nil {
			return fmt.Errorf("projection handler failed: %w", err)
		}
	}

	s.mu.Lock()
	proj.UpdatedAt = time.Now()
	s.mu.Unlock()

	s.log.Info().Str("name", name).Int("events", len(events)).Msg("Projection rebuilt")
	return nil
}

// createSnapshot creates a snapshot of current state.
func (s *Service) createSnapshot(ctx context.Context, aggregateID, aggregateType string, version int64) {
	state, err := s.ReconstructState(ctx, aggregateID, version)
	if err != nil {
		s.log.Warn().Err(err).Str("aggregate_id", aggregateID).Msg("Failed to create snapshot")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshotCounter++
	now := time.Now()

	// Find the event ID at this version
	var eventID string
	if indices, ok := s.aggregateIndex[aggregateID]; ok {
		for _, idx := range indices {
			if s.events[idx].Version == version {
				eventID = s.events[idx].ID
				break
			}
		}
	}

	snapshot := &Snapshot{
		ID:            fmt.Sprintf("snap-%d-%d", now.UnixNano(), s.snapshotCounter),
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		State:         state,
		Version:       version,
		EventID:       eventID,
		CreatedAt:     now,
		Hash:          s.computeStateHash(state),
	}

	s.snapshots[aggregateID] = append(s.snapshots[aggregateID], snapshot)

	s.log.Debug().
		Str("snapshot_id", snapshot.ID).
		Str("aggregate_id", aggregateID).
		Int64("version", version).
		Msg("Snapshot created")
}

// notifyHandlersSnapshot calls pre-snapshotted event and projection handlers.
// The handler slices are captured under the caller's lock so this method does
// not need to acquire the lock to iterate over them, eliminating the race
// between the spawned goroutine and concurrent Append / RegisterEventHandler
// calls that mutate the handler slices.
func (s *Service) notifyHandlersSnapshot(ctx context.Context, event *Event, handlers []EventHandler, projHandlers map[string]ProjectionHandler) {
	// Call event handlers without holding any lock -- handlers execute
	// concurrently with the rest of the system.
	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			s.log.Warn().Err(err).Str("event_id", event.ID).Msg("Event handler failed")
		}
	}

	// Update projections under the write lock because projection state is
	// shared.
	s.mu.Lock()
	for name, handler := range projHandlers {
		proj := s.projections[name]
		if proj == nil {
			continue
		}
		if err := handler(proj, event); err != nil {
			s.log.Warn().Err(err).Str("projection", name).Msg("Projection handler failed")
			continue
		}
		proj.Version++
		proj.LastEventID = event.ID
		proj.UpdatedAt = time.Now()
	}
	s.mu.Unlock()
}

// computeEventHash generates a hash for event integrity.
func (s *Service) computeEventHash(event *Event) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%d|%s",
		event.AggregateID,
		event.AggregateType,
		event.Type,
		event.Timestamp.Format(time.RFC3339Nano),
		event.Version,
		event.PrevHash,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// computeStateHash generates a hash for state.
func (s *Service) computeStateHash(state map[string]interface{}) string {
	data, _ := json.Marshal(state)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8])
}

// compactionLoop runs periodic log compaction.
func (s *Service) compactionLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.CompactionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runCompaction(ctx)
		}
	}
}

// runCompaction removes old events beyond retention period.
func (s *Service) runCompaction(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-s.config.RetentionPeriod)
	originalCount := len(s.events)

	// Find events to keep
	var newEvents []*Event
	for _, event := range s.events {
		if event.Timestamp.After(cutoff) {
			newEvents = append(newEvents, event)
		}
	}

	if len(newEvents) < originalCount {
		s.events = newEvents

		// Rebuild indexes
		s.eventIndex = make(map[string]*Event)
		s.aggregateIndex = make(map[string][]int)

		for i, event := range s.events {
			s.eventIndex[event.ID] = event
			s.aggregateIndex[event.AggregateID] = append(s.aggregateIndex[event.AggregateID], i)
		}

		s.log.Info().
			Int("removed", originalCount-len(newEvents)).
			Int("remaining", len(newEvents)).
			Msg("Event log compacted")
	}
}

// subscribeToEvents listens for external events.
func (s *Service) subscribeToEvents(ctx context.Context) {
	if s.wotan == nil {
		return
	}

	_, err := s.wotan.Subscribe(ctx, "state.changed", "anamnesis-service")
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to subscribe to state events")
		return
	}

	s.log.Info().Msg("Subscribed to state events")
}

// publishEvent sends an event to Wotan.
func (s *Service) publishEvent(ctx context.Context, eventType string, data map[string]interface{}) {
	if s.wotan == nil {
		return
	}

	event := map[string]interface{}{
		"event_type": eventType,
		"timestamp":  time.Now().Unix(),
		"data":       data,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to marshal event")
		return
	}

	if err := s.wotan.Publish(ctx, s.config.WotanTopic, payload); err != nil {
		s.log.Warn().Err(err).Msg("Failed to publish event")
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Count by event type
	typeCounts := make(map[string]int)
	for _, event := range s.events {
		typeCounts[string(event.Type)]++
	}

	// Count snapshots
	totalSnapshots := 0
	for _, snaps := range s.snapshots {
		totalSnapshots += len(snaps)
	}

	// Count aggregate types
	aggregateTypes := make(map[string]int)
	for _, event := range s.events {
		aggregateTypes[event.AggregateType]++
	}

	return map[string]interface{}{
		"total_events":       len(s.events),
		"total_aggregates":   len(s.aggregateIndex),
		"total_snapshots":    totalSnapshots,
		"total_projections":  len(s.projections),
		"events_by_type":     typeCounts,
		"by_aggregate_type":  aggregateTypes,
		"sequence_number":    s.sequenceNum,
		"registered_handlers": len(s.eventHandlers),
	}
}

// Timeline represents a chronological view of events.
type Timeline struct {
	Events     []*Event  `json:"events"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	TotalCount int       `json:"total_count"`
}

// GetTimeline returns events in a time range, sorted chronologically.
func (s *Service) GetTimeline(ctx context.Context, from, to time.Time, limit int) (*Timeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var events []*Event
	for _, event := range s.events {
		if event.Timestamp.After(from) && event.Timestamp.Before(to) {
			events = append(events, event)
		}
	}

	// Sort by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	totalCount := len(events)
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	return &Timeline{
		Events:     events,
		StartTime:  from,
		EndTime:    to,
		TotalCount: totalCount,
	}, nil
}
