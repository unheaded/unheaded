// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package pleroma provides configuration truth and desired state management.
// In Gnostic philosophy, Pleroma is the fullness of the divine realm - perfect, complete, ideal.
// This service embodies that concept: the single source of truth for what infrastructure SHOULD be.
// It stores declarative configurations, validates them, and provides reconciliation targets.
package pleroma

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
)

// Configuration represents a desired state declaration.
type Configuration struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Namespace   string                 `json:"namespace"`
	Kind        ConfigKind             `json:"kind"`
	Version     int64                  `json:"version"`
	Spec        map[string]interface{} `json:"spec"`
	Hash        string                 `json:"hash"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Annotations map[string]string      `json:"annotations,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CreatedBy   string                 `json:"created_by,omitempty"`
	UpdatedBy   string                 `json:"updated_by,omitempty"`
	Status      ConfigStatus           `json:"status"`
	Parent      string                 `json:"parent,omitempty"`
	Children    []string               `json:"children,omitempty"`
}

// ConfigKind defines the type of configuration.
type ConfigKind string

const (
	KindService      ConfigKind = "service"      // Service configuration
	KindDeployment   ConfigKind = "deployment"   // Deployment spec
	KindNetwork      ConfigKind = "network"      // Network policy/config
	KindStorage      ConfigKind = "storage"      // Storage configuration
	KindSecret       ConfigKind = "secret"       // Secret reference (not value)
	KindPolicy       ConfigKind = "policy"       // Policy definition
	KindCustom       ConfigKind = "custom"       // User-defined kind
	KindInfrastructure ConfigKind = "infrastructure" // Infrastructure definition
)

// ConfigStatus tracks configuration lifecycle.
type ConfigStatus string

const (
	StatusDraft     ConfigStatus = "draft"     // Being edited, not active
	StatusPending   ConfigStatus = "pending"   // Submitted, awaiting validation
	StatusActive    ConfigStatus = "active"    // Active and being reconciled
	StatusSuperseded ConfigStatus = "superseded" // Replaced by newer version
	StatusDeleted   ConfigStatus = "deleted"   // Marked for deletion
)

// Validation represents a configuration validation result.
type Validation struct {
	ConfigID   string            `json:"config_id"`
	Valid      bool              `json:"valid"`
	Errors     []ValidationError `json:"errors,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
	ValidatedAt time.Time        `json:"validated_at"`
	ValidatedBy string           `json:"validated_by"`
}

// ValidationError describes a validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// Validator is a function that validates configurations.
type Validator func(ctx context.Context, config *Configuration) *Validation

// Reconciliation represents a desired vs actual state comparison.
type Reconciliation struct {
	ID            string                 `json:"id"`
	ConfigID      string                 `json:"config_id"`
	DesiredState  map[string]interface{} `json:"desired_state"`
	ActualState   map[string]interface{} `json:"actual_state,omitempty"`
	Differences   []Difference           `json:"differences,omitempty"`
	Status        ReconciliationStatus   `json:"status"`
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	Error         string                 `json:"error,omitempty"`
}

// Difference represents a single state difference.
type Difference struct {
	Path          string      `json:"path"`
	DesiredValue  interface{} `json:"desired_value"`
	ActualValue   interface{} `json:"actual_value,omitempty"`
	ChangeType    ChangeType  `json:"change_type"`
}

// ChangeType describes the kind of change needed.
type ChangeType string

const (
	ChangeCreate ChangeType = "create" // Resource needs to be created
	ChangeUpdate ChangeType = "update" // Resource needs to be updated
	ChangeDelete ChangeType = "delete" // Resource needs to be deleted
	ChangeNoop   ChangeType = "noop"   // No change needed
)

// ReconciliationStatus tracks reconciliation progress.
type ReconciliationStatus string

const (
	ReconcilePending    ReconciliationStatus = "pending"
	ReconcileInProgress ReconciliationStatus = "in_progress"
	ReconcileSucceeded  ReconciliationStatus = "succeeded"
	ReconcileFailed     ReconciliationStatus = "failed"
)

// Service is the main Pleroma service for configuration truth management.
type Service struct {
	log    *logger.Logger
	wotan *wotanClient.Client
	config *Config

	mu              sync.RWMutex
	configurations  map[string]*Configuration  // ID -> config
	reconciliations map[string]*Reconciliation // ID -> reconciliation
	validators      map[ConfigKind][]Validator // Kind -> validators

	// Indexes for efficient lookup
	namespaceIndex map[string][]string // namespace -> config IDs
	kindIndex      map[ConfigKind][]string
	labelIndex     map[string]map[string][]string // label key -> label value -> config IDs

	// Counters
	configCounter    int64
	reconcileCounter int64

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64
}

// WotanDrops returns the total number of messages dropped because Wotan was nil.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// Config holds Pleroma service configuration.
type Config struct {
	MaxConfigurations   int           `json:"max_configurations"`
	ValidationTimeout   time.Duration `json:"validation_timeout"`
	ReconcileInterval   time.Duration `json:"reconcile_interval"`
	RetentionVersions   int           `json:"retention_versions"`
	EnableValidation    bool          `json:"enable_validation"`
	WotanTopic         string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxConfigurations:  50000,
		ValidationTimeout:  10 * time.Second,
		ReconcileInterval:  30 * time.Second,
		RetentionVersions:  10,
		EnableValidation:   true,
		WotanTopic:        "pleroma.configurations",
	}
}

// NewService creates a new Pleroma service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if log == nil {
		log = logger.New(io.Discard)
	}

	return &Service{
		log:             log,
		wotan:          wotan,
		config:          cfg,
		configurations:  make(map[string]*Configuration),
		reconciliations: make(map[string]*Reconciliation),
		validators:      make(map[ConfigKind][]Validator),
		namespaceIndex:  make(map[string][]string),
		kindIndex:       make(map[ConfigKind][]string),
		labelIndex:      make(map[string]map[string][]string),
	}
}

// Start initializes the Pleroma service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Pleroma awakens - the fullness of configuration truth flows")

	// Register built-in validators
	s.registerBuiltinValidators()

	// Start reconciliation loop
	go s.reconciliationLoop(ctx)

	// Subscribe to configuration events
	if s.wotan != nil {
		go s.subscribeToEvents(ctx)
	}

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Pleroma rests - configuration truth preserved")
	return nil
}

// Apply creates or updates a configuration.
func (s *Service) Apply(ctx context.Context, cfg *Configuration) (*Configuration, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate ID if not provided
	if cfg.ID == "" {
		s.configCounter++
		cfg.ID = fmt.Sprintf("cfg-%s-%s-%d", cfg.Namespace, cfg.Name, s.configCounter)
	}

	// Set timestamps
	now := time.Now()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now

	// Increment version
	if existing, ok := s.configurations[cfg.ID]; ok {
		cfg.Version = existing.Version + 1
		// Mark old version as superseded (would store in history in production)
	} else {
		cfg.Version = 1
	}

	// Compute hash of spec
	cfg.Hash = s.computeHash(cfg.Spec)

	// Set status
	if cfg.Status == "" {
		cfg.Status = StatusPending
	}

	// Validate if enabled
	if s.config.EnableValidation {
		validation := s.validateConfig(ctx, cfg)
		if !validation.Valid {
			return nil, fmt.Errorf("validation failed: %v", validation.Errors)
		}
		cfg.Status = StatusActive
	}

	// Store configuration
	s.configurations[cfg.ID] = cfg

	// Update indexes
	s.updateIndexes(cfg)

	s.log.Info().
		Str("id", cfg.ID).
		Str("name", cfg.Name).
		Str("namespace", cfg.Namespace).
		Int64("version", cfg.Version).
		Msg("Configuration applied")

	// Publish event
	s.publishEvent(ctx, "pleroma.config.applied", map[string]interface{}{
		"config_id": cfg.ID,
		"name":      cfg.Name,
		"namespace": cfg.Namespace,
		"version":   cfg.Version,
		"kind":      cfg.Kind,
	})

	return cfg, nil
}

// Get retrieves a configuration by ID.
func (s *Service) Get(id string) (*Configuration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.configurations[id]
	return cfg, ok
}

// GetByName retrieves a configuration by namespace and name.
func (s *Service) GetByName(namespace, name string) (*Configuration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, cfg := range s.configurations {
		if cfg.Namespace == namespace && cfg.Name == name && cfg.Status == StatusActive {
			return cfg, true
		}
	}
	return nil, false
}

// List returns configurations matching optional filters.
func (s *Service) List(namespace string, kind ConfigKind, labels map[string]string) []*Configuration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Configuration

	for _, cfg := range s.configurations {
		// Filter by status (only active)
		if cfg.Status != StatusActive {
			continue
		}

		// Filter by namespace
		if namespace != "" && cfg.Namespace != namespace {
			continue
		}

		// Filter by kind
		if kind != "" && cfg.Kind != kind {
			continue
		}

		// Filter by labels
		if !s.matchLabels(cfg.Labels, labels) {
			continue
		}

		results = append(results, cfg)
	}

	// Sort by name for consistent ordering
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return results
}

// Delete marks a configuration for deletion.
func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, ok := s.configurations[id]
	if !ok {
		return fmt.Errorf("configuration not found: %s", id)
	}

	cfg.Status = StatusDeleted
	cfg.UpdatedAt = time.Now()

	s.log.Info().
		Str("id", id).
		Str("name", cfg.Name).
		Msg("Configuration marked for deletion")

	// Publish event
	s.publishEvent(ctx, "pleroma.config.deleted", map[string]interface{}{
		"config_id": id,
		"name":      cfg.Name,
		"namespace": cfg.Namespace,
	})

	return nil
}

// RegisterValidator registers a validation function for a configuration kind.
func (s *Service) RegisterValidator(kind ConfigKind, validator Validator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.validators[kind] = append(s.validators[kind], validator)
	s.log.Debug().Str("kind", string(kind)).Msg("Validator registered")
}

// GetDesiredState returns the desired state for a configuration.
func (s *Service) GetDesiredState(id string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, ok := s.configurations[id]
	if !ok {
		return nil, fmt.Errorf("configuration not found: %s", id)
	}

	// Deep copy spec to prevent modification
	state := make(map[string]interface{})
	for k, v := range cfg.Spec {
		state[k] = v
	}

	return state, nil
}

// RecordReconciliation records the result of a reconciliation attempt.
func (s *Service) RecordReconciliation(ctx context.Context, configID string, actualState map[string]interface{}, differences []Difference, success bool, errMsg string) (*Reconciliation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, ok := s.configurations[configID]
	if !ok {
		return nil, fmt.Errorf("configuration not found: %s", configID)
	}

	s.reconcileCounter++
	now := time.Now()

	reconcile := &Reconciliation{
		ID:           fmt.Sprintf("r-%d-%d", now.UnixNano(), s.reconcileCounter),
		ConfigID:     configID,
		DesiredState: cfg.Spec,
		ActualState:  actualState,
		Differences:  differences,
		StartedAt:    now,
		CompletedAt:  &now,
	}

	if success {
		reconcile.Status = ReconcileSucceeded
	} else {
		reconcile.Status = ReconcileFailed
		reconcile.Error = errMsg
	}

	s.reconciliations[reconcile.ID] = reconcile

	s.log.Debug().
		Str("reconcile_id", reconcile.ID).
		Str("config_id", configID).
		Str("status", string(reconcile.Status)).
		Int("differences", len(differences)).
		Msg("Reconciliation recorded")

	return reconcile, nil
}

// computeHash generates a hash of the configuration spec.
func (s *Service) computeHash(spec map[string]interface{}) string {
	data, err := json.Marshal(spec)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8]) // Use first 8 bytes for brevity
}

// validateConfig runs all validators for the configuration.
func (s *Service) validateConfig(ctx context.Context, cfg *Configuration) *Validation {
	validation := &Validation{
		ConfigID:    cfg.ID,
		Valid:       true,
		ValidatedAt: time.Now(),
		ValidatedBy: "pleroma",
	}

	// Run kind-specific validators
	validators := s.validators[cfg.Kind]
	for _, validator := range validators {
		result := validator(ctx, cfg)
		if !result.Valid {
			validation.Valid = false
			validation.Errors = append(validation.Errors, result.Errors...)
		}
		validation.Warnings = append(validation.Warnings, result.Warnings...)
	}

	// Built-in validation
	if cfg.Name == "" {
		validation.Valid = false
		validation.Errors = append(validation.Errors, ValidationError{
			Field:   "name",
			Message: "name is required",
			Code:    "required",
		})
	}

	if cfg.Namespace == "" {
		validation.Valid = false
		validation.Errors = append(validation.Errors, ValidationError{
			Field:   "namespace",
			Message: "namespace is required",
			Code:    "required",
		})
	}

	return validation
}

// updateIndexes updates all indexes for a configuration.
func (s *Service) updateIndexes(cfg *Configuration) {
	// Namespace index
	s.namespaceIndex[cfg.Namespace] = append(s.namespaceIndex[cfg.Namespace], cfg.ID)

	// Kind index
	s.kindIndex[cfg.Kind] = append(s.kindIndex[cfg.Kind], cfg.ID)

	// Label index
	for k, v := range cfg.Labels {
		if s.labelIndex[k] == nil {
			s.labelIndex[k] = make(map[string][]string)
		}
		s.labelIndex[k][v] = append(s.labelIndex[k][v], cfg.ID)
	}
}

// matchLabels checks if config labels match the filter labels.
func (s *Service) matchLabels(configLabels, filterLabels map[string]string) bool {
	for k, v := range filterLabels {
		if configLabels[k] != v {
			return false
		}
	}
	return true
}

// registerBuiltinValidators adds default validators.
func (s *Service) registerBuiltinValidators() {
	// Service validator
	s.RegisterValidator(KindService, func(ctx context.Context, cfg *Configuration) *Validation {
		v := &Validation{ConfigID: cfg.ID, Valid: true}

		// Check for port configuration
		if _, ok := cfg.Spec["port"]; !ok {
			v.Warnings = append(v.Warnings, "service has no port specified")
		}

		return v
	})

	// Network validator
	s.RegisterValidator(KindNetwork, func(ctx context.Context, cfg *Configuration) *Validation {
		v := &Validation{ConfigID: cfg.ID, Valid: true}

		// Check for CIDR
		if _, ok := cfg.Spec["cidr"]; !ok {
			v.Errors = append(v.Errors, ValidationError{
				Field:   "spec.cidr",
				Message: "network configuration requires CIDR",
				Code:    "required",
			})
			v.Valid = false
		}

		return v
	})
}

// reconciliationLoop runs periodic reconciliation checks.
func (s *Service) reconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkConfigurations(ctx)
		}
	}
}

// checkConfigurations checks all active configurations.
func (s *Service) checkConfigurations(ctx context.Context) {
	s.mu.RLock()
	activeCount := 0
	for _, cfg := range s.configurations {
		if cfg.Status == StatusActive {
			activeCount++
		}
	}
	s.mu.RUnlock()

	s.log.Debug().Int("active_configs", activeCount).Msg("Reconciliation check")
}

// subscribeToEvents listens for configuration events.
func (s *Service) subscribeToEvents(ctx context.Context) {
	if s.wotan == nil {
		s.log.Warn().Msg("wotan unavailable — event subscription disabled (degraded mode)")
		return
	}

	_, err := s.wotan.Subscribe(ctx, "config.apply", "pleroma-service")
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to subscribe to config events")
		return
	}

	s.log.Info().Msg("Subscribed to configuration events")
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

	// Count by status
	statusCounts := make(map[string]int)
	kindCounts := make(map[string]int)
	for _, cfg := range s.configurations {
		statusCounts[string(cfg.Status)]++
		kindCounts[string(cfg.Kind)]++
	}

	return map[string]interface{}{
		"total_configurations": len(s.configurations),
		"total_reconciliations": len(s.reconciliations),
		"by_status":            statusCounts,
		"by_kind":              kindCounts,
		"indexed_namespaces":   len(s.namespaceIndex),
		"registered_validators": len(s.validators),
	}
}
