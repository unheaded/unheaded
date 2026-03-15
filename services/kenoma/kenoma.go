// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package kenoma provides actual state observation and drift detection.
// In Gnostic philosophy, Kenoma is the void - the deficient material world that falls short of the divine.
// This service embodies that concept: observing what infrastructure ACTUALLY is, detecting drift from
// the desired state (Pleroma), and reporting differences that need reconciliation.
package kenoma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
)

// Observation represents a snapshot of actual state.
type Observation struct {
	ID           string                 `json:"id"`
	EntityID     string                 `json:"entity_id"`
	EntityType   EntityType             `json:"entity_type"`
	State        map[string]interface{} `json:"state"`
	Hash         string                 `json:"hash"`
	ObservedAt   time.Time              `json:"observed_at"`
	ObservedBy   string                 `json:"observed_by"`
	Source       string                 `json:"source"`
	Healthy      bool                   `json:"healthy"`
	HealthReason string                 `json:"health_reason,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// EntityType defines the kind of entity being observed.
type EntityType string

const (
	EntityService       EntityType = "service"
	EntityContainer     EntityType = "container"
	EntityNode          EntityType = "node"
	EntityNetwork       EntityType = "network"
	EntityStorage       EntityType = "storage"
	EntitySecret        EntityType = "secret"
	EntityLoadBalancer  EntityType = "loadbalancer"
	EntityDNS           EntityType = "dns"
	EntityCertificate   EntityType = "certificate"
)

// Drift represents a difference between desired and actual state.
type Drift struct {
	ID             string        `json:"id"`
	EntityID       string        `json:"entity_id"`
	EntityType     EntityType    `json:"entity_type"`
	DesiredStateID string        `json:"desired_state_id"` // Reference to Pleroma config
	DriftType      DriftType     `json:"drift_type"`
	Path           string        `json:"path"`
	DesiredValue   interface{}   `json:"desired_value"`
	ActualValue    interface{}   `json:"actual_value"`
	Severity       DriftSeverity `json:"severity"`
	DetectedAt     time.Time     `json:"detected_at"`
	ResolvedAt     *time.Time    `json:"resolved_at,omitempty"`
	AutoRemediable bool          `json:"auto_remediable"`
	Description    string        `json:"description,omitempty"`
}

// DriftType categorizes the kind of drift.
type DriftType string

const (
	DriftMissing    DriftType = "missing"    // Resource doesn't exist but should
	DriftExtra      DriftType = "extra"      // Resource exists but shouldn't
	DriftModified   DriftType = "modified"   // Resource exists but differs
	DriftOutdated   DriftType = "outdated"   // Resource version is old
	DriftUnhealthy  DriftType = "unhealthy"  // Resource is unhealthy
)

// DriftSeverity indicates the importance of the drift.
type DriftSeverity string

const (
	SeverityCritical DriftSeverity = "critical" // Immediate action required
	SeverityHigh     DriftSeverity = "high"     // Action required soon
	SeverityMedium   DriftSeverity = "medium"   // Should be addressed
	SeverityLow      DriftSeverity = "low"      // Minor drift, can wait
	SeverityInfo     DriftSeverity = "info"     // Informational only
)

// Observer is a function that collects actual state.
type Observer func(ctx context.Context, entityID string) (*Observation, error)

// HealthChecker validates the health of an observation.
type HealthChecker func(ctx context.Context, obs *Observation) (bool, string)

// Comparator compares desired and actual state.
type Comparator func(desired, actual map[string]interface{}) []Drift

// Service is the main Kenoma service for state observation and drift detection.
type Service struct {
	log    *logger.Logger
	wotan *wotanClient.Client
	config *Config

	mu            sync.RWMutex
	observations  map[string]*Observation // EntityID -> latest observation
	drifts        map[string]*Drift       // DriftID -> drift
	observers     map[EntityType]Observer
	healthCheckers map[EntityType]HealthChecker
	comparators   map[EntityType]Comparator

	// Indexes
	entityIndex map[string][]string // EntityID -> observation history IDs
	driftIndex  map[string][]string // EntityID -> drift IDs

	// Counters
	observationCounter int64
	driftCounter       int64

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64
}

// WotanDrops returns the total number of messages dropped because Wotan was nil.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// Config holds Kenoma service configuration.
type Config struct {
	ObservationInterval time.Duration `json:"observation_interval"`
	DriftCheckInterval  time.Duration `json:"drift_check_interval"`
	RetentionPeriod     time.Duration `json:"retention_period"`
	MaxObservations     int           `json:"max_observations"`
	AutoRemediate       bool          `json:"auto_remediate"`
	WotanTopic         string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ObservationInterval: 30 * time.Second,
		DriftCheckInterval:  60 * time.Second,
		RetentionPeriod:     24 * time.Hour,
		MaxObservations:     100000,
		AutoRemediate:       false,
		WotanTopic:         "kenoma.observations",
	}
}

// NewService creates a new Kenoma service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if log == nil {
		log = logger.New(io.Discard)
	}

	return &Service{
		log:            log,
		wotan:         wotan,
		config:         cfg,
		observations:   make(map[string]*Observation),
		drifts:         make(map[string]*Drift),
		observers:      make(map[EntityType]Observer),
		healthCheckers: make(map[EntityType]HealthChecker),
		comparators:    make(map[EntityType]Comparator),
		entityIndex:    make(map[string][]string),
		driftIndex:     make(map[string][]string),
	}
}

// Start initializes the Kenoma service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Kenoma awakens - the void observes the material world")

	// Register default comparators
	s.registerDefaultComparators()

	// Start observation loop
	go s.observationLoop(ctx)

	// Start drift detection loop
	go s.driftDetectionLoop(ctx)

	// Subscribe to state change events
	if s.wotan != nil {
		go s.subscribeToEvents(ctx)
	}

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Kenoma rests - observations preserved")
	return nil
}

// RegisterObserver registers an observer for an entity type.
func (s *Service) RegisterObserver(entityType EntityType, observer Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers[entityType] = observer
	s.log.Debug().Str("entity_type", string(entityType)).Msg("Observer registered")
}

// RegisterHealthChecker registers a health checker for an entity type.
func (s *Service) RegisterHealthChecker(entityType EntityType, checker HealthChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthCheckers[entityType] = checker
	s.log.Debug().Str("entity_type", string(entityType)).Msg("Health checker registered")
}

// RegisterComparator registers a comparator for an entity type.
func (s *Service) RegisterComparator(entityType EntityType, comparator Comparator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.comparators[entityType] = comparator
	s.log.Debug().Str("entity_type", string(entityType)).Msg("Comparator registered")
}

// Observe captures the current state of an entity.
func (s *Service) Observe(ctx context.Context, entityID string, entityType EntityType, state map[string]interface{}) (*Observation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.observationCounter++
	now := time.Now()

	obs := &Observation{
		ID:         fmt.Sprintf("obs-%d-%d", now.UnixNano(), s.observationCounter),
		EntityID:   entityID,
		EntityType: entityType,
		State:      state,
		Hash:       s.computeStateHash(state),
		ObservedAt: now,
		ObservedBy: "kenoma",
		Source:     "direct",
		Healthy:    true, // Will be updated by health check
	}

	// Run health check if available
	if checker, ok := s.healthCheckers[entityType]; ok {
		healthy, reason := checker(ctx, obs)
		obs.Healthy = healthy
		obs.HealthReason = reason
	}

	// Store observation
	s.observations[entityID] = obs

	// Update index
	s.entityIndex[entityID] = append(s.entityIndex[entityID], obs.ID)

	s.log.Debug().
		Str("id", obs.ID).
		Str("entity_id", entityID).
		Str("entity_type", string(entityType)).
		Bool("healthy", obs.Healthy).
		Msg("Observation recorded")

	// Publish event
	s.publishEvent(ctx, "kenoma.observed", map[string]interface{}{
		"observation_id": obs.ID,
		"entity_id":      entityID,
		"entity_type":    entityType,
		"healthy":        obs.Healthy,
	})

	return obs, nil
}

// GetObservation retrieves the latest observation for an entity.
func (s *Service) GetObservation(entityID string) (*Observation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	obs, ok := s.observations[entityID]
	return obs, ok
}

// GetActualState returns the current actual state for an entity.
func (s *Service) GetActualState(entityID string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obs, ok := s.observations[entityID]
	if !ok {
		return nil, fmt.Errorf("no observation found for entity: %s", entityID)
	}

	// Return copy of state
	state := make(map[string]interface{})
	for k, v := range obs.State {
		state[k] = v
	}

	return state, nil
}

// Compare compares desired state against actual state and detects drift.
func (s *Service) Compare(ctx context.Context, entityID string, entityType EntityType, desiredStateID string, desired, actual map[string]interface{}) ([]Drift, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get comparator for entity type
	comparator, ok := s.comparators[entityType]
	if !ok {
		comparator = s.defaultComparator
	}

	// Compare states
	drifts := comparator(desired, actual)

	// Record drifts
	now := time.Now()
	for i := range drifts {
		s.driftCounter++
		drifts[i].ID = fmt.Sprintf("drift-%d-%d", now.UnixNano(), s.driftCounter)
		drifts[i].EntityID = entityID
		drifts[i].EntityType = entityType
		drifts[i].DesiredStateID = desiredStateID
		drifts[i].DetectedAt = now
		drifts[i].Severity = s.assessSeverity(&drifts[i])
		drifts[i].AutoRemediable = s.isAutoRemediable(&drifts[i])

		// Store drift
		s.drifts[drifts[i].ID] = &drifts[i]

		// Update index
		s.driftIndex[entityID] = append(s.driftIndex[entityID], drifts[i].ID)

		s.log.Warn().
			Str("drift_id", drifts[i].ID).
			Str("entity_id", entityID).
			Str("drift_type", string(drifts[i].DriftType)).
			Str("path", drifts[i].Path).
			Str("severity", string(drifts[i].Severity)).
			Msg("Drift detected")
	}

	// Publish drift event if any drifts found
	if len(drifts) > 0 {
		s.publishEvent(ctx, "kenoma.drift.detected", map[string]interface{}{
			"entity_id":   entityID,
			"entity_type": entityType,
			"drift_count": len(drifts),
		})
	}

	return drifts, nil
}

// GetDrift retrieves a drift by ID.
func (s *Service) GetDrift(driftID string) (*Drift, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	drift, ok := s.drifts[driftID]
	return drift, ok
}

// ListDrifts returns all active drifts for an entity.
func (s *Service) ListDrifts(entityID string) []*Drift {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Drift
	driftIDs := s.driftIndex[entityID]
	for _, id := range driftIDs {
		if drift, ok := s.drifts[id]; ok {
			if drift.ResolvedAt == nil { // Only active drifts
				results = append(results, drift)
			}
		}
	}

	return results
}

// ListAllDrifts returns all active drifts.
func (s *Service) ListAllDrifts() []*Drift {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Drift
	for _, drift := range s.drifts {
		if drift.ResolvedAt == nil {
			results = append(results, drift)
		}
	}

	return results
}

// ResolveDrift marks a drift as resolved.
func (s *Service) ResolveDrift(ctx context.Context, driftID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	drift, ok := s.drifts[driftID]
	if !ok {
		return fmt.Errorf("drift not found: %s", driftID)
	}

	now := time.Now()
	drift.ResolvedAt = &now

	s.log.Info().
		Str("drift_id", driftID).
		Str("entity_id", drift.EntityID).
		Msg("Drift resolved")

	s.publishEvent(ctx, "kenoma.drift.resolved", map[string]interface{}{
		"drift_id":  driftID,
		"entity_id": drift.EntityID,
	})

	return nil
}

// defaultComparator provides basic state comparison.
func (s *Service) defaultComparator(desired, actual map[string]interface{}) []Drift {
	var drifts []Drift

	// Check for missing or modified values
	for key, desiredVal := range desired {
		actualVal, exists := actual[key]
		if !exists {
			drifts = append(drifts, Drift{
				DriftType:    DriftMissing,
				Path:         key,
				DesiredValue: desiredVal,
				Description:  fmt.Sprintf("Key '%s' missing from actual state", key),
			})
			continue
		}

		if !reflect.DeepEqual(desiredVal, actualVal) {
			drifts = append(drifts, Drift{
				DriftType:    DriftModified,
				Path:         key,
				DesiredValue: desiredVal,
				ActualValue:  actualVal,
				Description:  fmt.Sprintf("Key '%s' differs: desired=%v, actual=%v", key, desiredVal, actualVal),
			})
		}
	}

	// Check for extra values
	for key, actualVal := range actual {
		if _, exists := desired[key]; !exists {
			drifts = append(drifts, Drift{
				DriftType:   DriftExtra,
				Path:        key,
				ActualValue: actualVal,
				Description: fmt.Sprintf("Key '%s' exists in actual but not in desired state", key),
			})
		}
	}

	return drifts
}

// assessSeverity determines the severity of a drift.
func (s *Service) assessSeverity(drift *Drift) DriftSeverity {
	switch drift.DriftType {
	case DriftMissing:
		return SeverityHigh
	case DriftUnhealthy:
		return SeverityCritical
	case DriftModified:
		return SeverityMedium
	case DriftExtra:
		return SeverityLow
	case DriftOutdated:
		return SeverityMedium
	default:
		return SeverityInfo
	}
}

// isAutoRemediable determines if a drift can be auto-remediated.
func (s *Service) isAutoRemediable(drift *Drift) bool {
	// Simple policy: only modified values are auto-remediable
	return drift.DriftType == DriftModified && s.config.AutoRemediate
}

// computeStateHash generates a hash of the state.
func (s *Service) computeStateHash(state map[string]interface{}) string {
	data, err := json.Marshal(state)
	if err != nil {
		return ""
	}
	// Simple hash for demo
	hash := 0
	for _, b := range data {
		hash = hash*31 + int(b)
	}
	return fmt.Sprintf("%x", hash)
}

// registerDefaultComparators adds built-in comparators.
func (s *Service) registerDefaultComparators() {
	// Service comparator with port checking
	s.RegisterComparator(EntityService, func(desired, actual map[string]interface{}) []Drift {
		drifts := s.defaultComparator(desired, actual)

		// Additional service-specific checks
		if desiredPort, ok := desired["port"]; ok {
			if actualPort, ok := actual["port"]; ok {
				if desiredPort != actualPort {
					for i := range drifts {
						if drifts[i].Path == "port" {
							drifts[i].Severity = SeverityHigh // Port changes are high severity
						}
					}
				}
			}
		}

		return drifts
	})
}

// observationLoop runs periodic observations.
func (s *Service) observationLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.ObservationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runObservations(ctx)
		}
	}
}

// runObservations triggers observations for all registered observers.
func (s *Service) runObservations(ctx context.Context) {
	s.mu.RLock()
	observationCount := len(s.observations)
	s.mu.RUnlock()

	s.log.Debug().Int("tracked_entities", observationCount).Msg("Running observation cycle")
}

// driftDetectionLoop runs periodic drift checks.
func (s *Service) driftDetectionLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.DriftCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDriftDetection(ctx)
		}
	}
}

// runDriftDetection checks for drift across all entities.
func (s *Service) runDriftDetection(ctx context.Context) {
	s.mu.RLock()
	activeDrifts := 0
	for _, drift := range s.drifts {
		if drift.ResolvedAt == nil {
			activeDrifts++
		}
	}
	s.mu.RUnlock()

	s.log.Debug().Int("active_drifts", activeDrifts).Msg("Running drift detection cycle")
}

// subscribeToEvents listens for state change events.
func (s *Service) subscribeToEvents(ctx context.Context) {
	if s.wotan == nil {
		s.log.Warn().Msg("wotan unavailable — event subscription disabled (degraded mode)")
		return
	}

	_, err := s.wotan.Subscribe(ctx, "state.changed", "kenoma-service")
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to subscribe to state events")
		return
	}

	s.log.Info().Msg("Subscribed to state events")
}

// publishEvent sends an event to Wotan.
func (s *Service) publishEvent(ctx context.Context, eventType string, data map[string]interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		s.log.Warn().Str("event_type", eventType).Msg("wotan unavailable — event dropped (degraded mode)")
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

	// Count by severity
	severityCounts := make(map[string]int)
	activeDrifts := 0
	for _, drift := range s.drifts {
		if drift.ResolvedAt == nil {
			activeDrifts++
			severityCounts[string(drift.Severity)]++
		}
	}

	// Count by entity type
	typeCounts := make(map[string]int)
	for _, obs := range s.observations {
		typeCounts[string(obs.EntityType)]++
	}

	healthyCount := 0
	for _, obs := range s.observations {
		if obs.Healthy {
			healthyCount++
		}
	}

	return map[string]interface{}{
		"total_observations":     len(s.observations),
		"total_drifts":          len(s.drifts),
		"active_drifts":         activeDrifts,
		"drifts_by_severity":    severityCounts,
		"observations_by_type":  typeCounts,
		"healthy_entities":      healthyCount,
		"unhealthy_entities":    len(s.observations) - healthyCount,
		"registered_observers":  len(s.observers),
		"registered_comparators": len(s.comparators),
	}
}
