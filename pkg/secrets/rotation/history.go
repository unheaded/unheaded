// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package rotation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/secrets"
)

// Common history errors.
var (
	ErrHistoryNotFound = errors.New("rotation history not found")
)

// RotationStatus represents the status of a rotation operation.
type RotationStatus string

const (
	// RotationStatusPending indicates rotation is queued but not started.
	RotationStatusPending RotationStatus = "pending"

	// RotationStatusInProgress indicates rotation is currently running.
	RotationStatusInProgress RotationStatus = "in_progress"

	// RotationStatusSuccess indicates rotation completed successfully.
	RotationStatusSuccess RotationStatus = "success"

	// RotationStatusFailed indicates rotation failed.
	RotationStatusFailed RotationStatus = "failed"

	// RotationStatusRolledBack indicates rotation was rolled back.
	RotationStatusRolledBack RotationStatus = "rolled_back"
)

// RotationRecord represents a historical rotation operation.
type RotationRecord struct {
	// ID is the unique identifier for this rotation.
	ID string `json:"id"`

	// Path is the secret path that was rotated.
	Path string `json:"path"`

	// SecretType is the type of secret.
	SecretType secrets.SecretType `json:"secret_type"`

	// Status is the rotation status.
	Status RotationStatus `json:"status"`

	// Trigger is what triggered the rotation.
	Trigger string `json:"trigger"`

	// TriggerDetails contains additional trigger information.
	TriggerDetails map[string]interface{} `json:"trigger_details,omitempty"`

	// PolicyID is the policy that triggered rotation (if applicable).
	PolicyID string `json:"policy_id,omitempty"`

	// OldVersion is the version before rotation.
	OldVersion int64 `json:"old_version"`

	// NewVersion is the version after rotation.
	NewVersion int64 `json:"new_version"`

	// StartedAt is when rotation started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when rotation completed (or failed).
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration is how long the rotation took.
	Duration time.Duration `json:"duration,omitempty"`

	// Error contains the error message if rotation failed.
	Error string `json:"error,omitempty"`

	// HookResults contains results from rotation hooks.
	HookResults []*HookResult `json:"hook_results,omitempty"`

	// RollbackAttempted indicates if rollback was attempted.
	RollbackAttempted bool `json:"rollback_attempted,omitempty"`

	// RollbackSuccess indicates if rollback succeeded.
	RollbackSuccess bool `json:"rollback_success,omitempty"`

	// RollbackError contains the rollback error if it failed.
	RollbackError string `json:"rollback_error,omitempty"`

	// Metadata contains additional context.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Actor is who initiated the rotation.
	Actor string `json:"actor,omitempty"`
}

// HistoryFilter defines criteria for querying rotation history.
type HistoryFilter struct {
	// Path filters by secret path (supports prefix matching).
	Path string

	// Status filters by rotation status.
	Status RotationStatus

	// StartTime filters records after this time.
	StartTime time.Time

	// EndTime filters records before this time.
	EndTime time.Time

	// PolicyID filters by policy.
	PolicyID string

	// SuccessOnly filters for successful rotations only.
	SuccessOnly bool

	// FailuresOnly filters for failed rotations only.
	FailuresOnly bool

	// Limit limits the number of results.
	Limit int

	// Offset skips the first N results.
	Offset int
}

// HistoryStore is the interface for storing rotation history.
type HistoryStore interface {
	// Save saves a rotation record.
	Save(record *RotationRecord) error

	// Get retrieves a rotation record by ID.
	Get(id string) (*RotationRecord, error)

	// Query queries rotation history with filters.
	Query(ctx context.Context, filter HistoryFilter) ([]*RotationRecord, error)

	// GetLatest returns the most recent rotation for a path.
	GetLatest(path string) (*RotationRecord, error)

	// Delete removes a rotation record.
	Delete(id string) error

	// Prune removes records older than the specified duration.
	Prune(ctx context.Context, olderThan time.Duration) (int, error)
}

// MemoryHistoryStore stores rotation history in memory.
type MemoryHistoryStore struct {
	records map[string]*RotationRecord
	byPath  map[string][]*RotationRecord
	maxSize int
	mu      sync.RWMutex
}

// NewMemoryHistoryStore creates a new in-memory history store.
func NewMemoryHistoryStore(maxSize int) *MemoryHistoryStore {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &MemoryHistoryStore{
		records: make(map[string]*RotationRecord),
		byPath:  make(map[string][]*RotationRecord),
		maxSize: maxSize,
	}
}

// Save saves a rotation record.
func (s *MemoryHistoryStore) Save(record *RotationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check size limit
	if len(s.records) >= s.maxSize {
		// Remove oldest record
		var oldest *RotationRecord
		for _, r := range s.records {
			if oldest == nil || r.StartedAt.Before(oldest.StartedAt) {
				oldest = r
			}
		}
		if oldest != nil {
			delete(s.records, oldest.ID)
			// Remove from byPath
			pathRecords := s.byPath[oldest.Path]
			for i, r := range pathRecords {
				if r.ID == oldest.ID {
					s.byPath[oldest.Path] = append(pathRecords[:i], pathRecords[i+1:]...)
					break
				}
			}
		}
	}

	s.records[record.ID] = record
	s.byPath[record.Path] = append(s.byPath[record.Path], record)

	return nil
}

// Get retrieves a rotation record by ID.
func (s *MemoryHistoryStore) Get(id string) (*RotationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.records[id]
	if !ok {
		return nil, ErrHistoryNotFound
	}
	return record, nil
}

// Query queries rotation history with filters.
func (s *MemoryHistoryStore) Query(ctx context.Context, filter HistoryFilter) ([]*RotationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*RotationRecord

	// If path filter is set, start with path-specific records
	var candidates []*RotationRecord
	if filter.Path != "" {
		for path, records := range s.byPath {
			if matchPath(filter.Path, path) || matchPath(path, filter.Path) {
				candidates = append(candidates, records...)
			}
		}
	} else {
		for _, r := range s.records {
			candidates = append(candidates, r)
		}
	}

	for _, record := range candidates {
		// Apply filters
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		if filter.PolicyID != "" && record.PolicyID != filter.PolicyID {
			continue
		}
		if !filter.StartTime.IsZero() && record.StartedAt.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && record.StartedAt.After(filter.EndTime) {
			continue
		}
		if filter.SuccessOnly && record.Status != RotationStatusSuccess {
			continue
		}
		if filter.FailuresOnly && record.Status == RotationStatusSuccess {
			continue
		}

		results = append(results, record)
	}

	// Sort by time (most recent first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].StartedAt.After(results[j].StartedAt)
	})

	// Apply offset and limit
	if filter.Offset > 0 {
		if filter.Offset >= len(results) {
			return nil, nil
		}
		results = results[filter.Offset:]
	}
	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}

	return results, nil
}

// GetLatest returns the most recent rotation for a path.
func (s *MemoryHistoryStore) GetLatest(path string) (*RotationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records, ok := s.byPath[path]
	if !ok || len(records) == 0 {
		return nil, ErrHistoryNotFound
	}

	// Find most recent
	var latest *RotationRecord
	for _, r := range records {
		if latest == nil || r.StartedAt.After(latest.StartedAt) {
			latest = r
		}
	}

	return latest, nil
}

// Delete removes a rotation record.
func (s *MemoryHistoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.records[id]
	if !ok {
		return ErrHistoryNotFound
	}

	delete(s.records, id)

	// Remove from byPath
	pathRecords := s.byPath[record.Path]
	for i, r := range pathRecords {
		if r.ID == id {
			s.byPath[record.Path] = append(pathRecords[:i], pathRecords[i+1:]...)
			break
		}
	}

	return nil
}

// Prune removes records older than the specified duration.
func (s *MemoryHistoryStore) Prune(ctx context.Context, olderThan time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	pruned := 0

	for id, record := range s.records {
		if record.StartedAt.Before(cutoff) {
			delete(s.records, id)

			// Remove from byPath
			pathRecords := s.byPath[record.Path]
			for i, r := range pathRecords {
				if r.ID == id {
					s.byPath[record.Path] = append(pathRecords[:i], pathRecords[i+1:]...)
					break
				}
			}

			pruned++
		}
	}

	return pruned, nil
}

// FileHistoryStore stores rotation history in files.
type FileHistoryStore struct {
	basePath string
	log      *logger.Logger
	mu       sync.RWMutex
}

// FileHistoryStoreConfig configures the file history store.
type FileHistoryStoreConfig struct {
	// BasePath is the directory to store history files.
	BasePath string

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewFileHistoryStore creates a new file-based history store.
func NewFileHistoryStore(config FileHistoryStoreConfig) (*FileHistoryStore, error) {
	if config.BasePath == "" {
		return nil, errors.New("base path is required")
	}

	if err := os.MkdirAll(config.BasePath, 0700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	return &FileHistoryStore{
		basePath: config.BasePath,
		log:      log.With().Str("component", "file-history-store").Logger(),
	}, nil
}

// recordPath returns the file path for a record.
func (s *FileHistoryStore) recordPath(id string) string {
	return filepath.Join(s.basePath, id+".json")
}

// Save saves a rotation record.
func (s *FileHistoryStore) Save(record *RotationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	filePath := s.recordPath(record.ID)
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("write record: %w", err)
	}

	s.log.Debug().Str("id", record.ID).Str("path", record.Path).Msg("saved rotation record")
	return nil
}

// Get retrieves a rotation record by ID.
func (s *FileHistoryStore) Get(id string) (*RotationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.recordPath(id)
	data, err := os.ReadFile(filePath) // #nosec G304 -- internal secrets-store path derived from the configured store dir
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrHistoryNotFound
		}
		return nil, fmt.Errorf("read record: %w", err)
	}

	var record RotationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("unmarshal record: %w", err)
	}

	return &record, nil
}

// Query queries rotation history with filters.
func (s *FileHistoryStore) Query(ctx context.Context, filter HistoryFilter) ([]*RotationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var results []*RotationRecord

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		filePath := filepath.Join(s.basePath, entry.Name())
		data, err := os.ReadFile(filePath) // #nosec G304 -- internal secrets-store path derived from the configured store dir
		if err != nil {
			continue
		}

		var record RotationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		// Apply filters
		if filter.Path != "" && !matchPath(filter.Path, record.Path) && !matchPath(record.Path, filter.Path) {
			continue
		}
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		if filter.PolicyID != "" && record.PolicyID != filter.PolicyID {
			continue
		}
		if !filter.StartTime.IsZero() && record.StartedAt.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && record.StartedAt.After(filter.EndTime) {
			continue
		}
		if filter.SuccessOnly && record.Status != RotationStatusSuccess {
			continue
		}
		if filter.FailuresOnly && record.Status == RotationStatusSuccess {
			continue
		}

		results = append(results, &record)
	}

	// Sort by time (most recent first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].StartedAt.After(results[j].StartedAt)
	})

	// Apply offset and limit
	if filter.Offset > 0 {
		if filter.Offset >= len(results) {
			return nil, nil
		}
		results = results[filter.Offset:]
	}
	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}

	return results, nil
}

// GetLatest returns the most recent rotation for a path.
func (s *FileHistoryStore) GetLatest(path string) (*RotationRecord, error) {
	records, err := s.Query(context.Background(), HistoryFilter{
		Path:  path,
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrHistoryNotFound
	}
	return records[0], nil
}

// Delete removes a rotation record.
func (s *FileHistoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.recordPath(id)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return ErrHistoryNotFound
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("remove record: %w", err)
	}

	s.log.Debug().Str("id", id).Msg("deleted rotation record")
	return nil
}

// Prune removes records older than the specified duration.
func (s *FileHistoryStore) Prune(ctx context.Context, olderThan time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	pruned := 0

	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return 0, fmt.Errorf("read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		select {
		case <-ctx.Done():
			return pruned, ctx.Err()
		default:
		}

		filePath := filepath.Join(s.basePath, entry.Name())
		data, err := os.ReadFile(filePath) // #nosec G304 -- internal secrets-store path derived from the configured store dir
		if err != nil {
			continue
		}

		var record RotationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		if record.StartedAt.Before(cutoff) {
			if err := os.Remove(filePath); err == nil {
				pruned++
			}
		}
	}

	s.log.Info().Int("pruned", pruned).Dur("older_than", olderThan).Msg("pruned rotation history")
	return pruned, nil
}

// HistoryTracker tracks rotation operations in real-time.
type HistoryTracker struct {
	store      HistoryStore
	log        *logger.Logger
	currentOps map[string]*RotationRecord
	mu         sync.RWMutex
}

// HistoryTrackerConfig configures the history tracker.
type HistoryTrackerConfig struct {
	// Store is the history store.
	Store HistoryStore

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewHistoryTracker creates a new history tracker.
func NewHistoryTracker(config HistoryTrackerConfig) *HistoryTracker {
	if config.Store == nil {
		config.Store = NewMemoryHistoryStore(10000)
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	return &HistoryTracker{
		store:      config.Store,
		log:        log.With().Str("component", "history-tracker").Logger(),
		currentOps: make(map[string]*RotationRecord),
	}
}

// StartRotation starts tracking a new rotation operation.
func (ht *HistoryTracker) StartRotation(record *RotationRecord) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	record.Status = RotationStatusInProgress
	record.StartedAt = time.Now()

	ht.currentOps[record.ID] = record

	if err := ht.store.Save(record); err != nil {
		return fmt.Errorf("save record: %w", err)
	}

	ht.log.Info().
		Str("id", record.ID).
		Str("path", record.Path).
		Str("trigger", record.Trigger).
		Msg("rotation started")

	return nil
}

// CompleteRotation marks a rotation as completed.
func (ht *HistoryTracker) CompleteRotation(id string, newVersion int64, hookResults []*HookResult) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	record, ok := ht.currentOps[id]
	if !ok {
		// Try to load from store
		var err error
		record, err = ht.store.Get(id)
		if err != nil {
			return fmt.Errorf("rotation not found: %s", id)
		}
	}

	record.Status = RotationStatusSuccess
	record.NewVersion = newVersion
	record.CompletedAt = time.Now()
	record.Duration = record.CompletedAt.Sub(record.StartedAt)
	record.HookResults = hookResults

	delete(ht.currentOps, id)

	if err := ht.store.Save(record); err != nil {
		return fmt.Errorf("save record: %w", err)
	}

	ht.log.Info().
		Str("id", record.ID).
		Str("path", record.Path).
		Int64("old_version", record.OldVersion).
		Int64("new_version", record.NewVersion).
		Dur("duration", record.Duration).
		Msg("rotation completed")

	return nil
}

// FailRotation marks a rotation as failed.
func (ht *HistoryTracker) FailRotation(id string, err error, hookResults []*HookResult) error {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	record, ok := ht.currentOps[id]
	if !ok {
		// Try to load from store
		var loadErr error
		record, loadErr = ht.store.Get(id)
		if loadErr != nil {
			return fmt.Errorf("rotation not found: %s", id)
		}
	}

	record.Status = RotationStatusFailed
	record.Error = err.Error()
	record.CompletedAt = time.Now()
	record.Duration = record.CompletedAt.Sub(record.StartedAt)
	record.HookResults = hookResults

	delete(ht.currentOps, id)

	if saveErr := ht.store.Save(record); saveErr != nil {
		return fmt.Errorf("save record: %w", saveErr)
	}

	ht.log.Error().
		Str("id", record.ID).
		Str("path", record.Path).
		Str("error", record.Error).
		Dur("duration", record.Duration).
		Msg("rotation failed")

	return nil
}

// RecordRollback records a rollback attempt.
func (ht *HistoryTracker) RecordRollback(id string, success bool, rollbackErr error) error {
	record, err := ht.store.Get(id)
	if err != nil {
		return err
	}

	record.RollbackAttempted = true
	record.RollbackSuccess = success
	if rollbackErr != nil {
		record.RollbackError = rollbackErr.Error()
	}
	if success {
		record.Status = RotationStatusRolledBack
	}

	if err := ht.store.Save(record); err != nil {
		return fmt.Errorf("save record: %w", err)
	}

	ht.log.Info().
		Str("id", record.ID).
		Str("path", record.Path).
		Bool("success", success).
		Msg("rollback recorded")

	return nil
}

// GetHistory returns the history for a path.
func (ht *HistoryTracker) GetHistory(ctx context.Context, path string, limit int) ([]*RotationRecord, error) {
	return ht.store.Query(ctx, HistoryFilter{
		Path:  path,
		Limit: limit,
	})
}

// GetRecord retrieves a specific rotation record.
func (ht *HistoryTracker) GetRecord(id string) (*RotationRecord, error) {
	return ht.store.Get(id)
}

// GetStats returns rotation statistics.
func (ht *HistoryTracker) GetStats(ctx context.Context, path string, since time.Time) (*RotationStats, error) {
	records, err := ht.store.Query(ctx, HistoryFilter{
		Path:      path,
		StartTime: since,
	})
	if err != nil {
		return nil, err
	}

	stats := &RotationStats{
		Path:           path,
		Since:          since,
		TotalRotations: len(records),
	}

	for _, r := range records {
		switch r.Status {
		case RotationStatusSuccess:
			stats.SuccessfulRotations++
			stats.TotalDuration += r.Duration
		case RotationStatusFailed:
			stats.FailedRotations++
		case RotationStatusRolledBack:
			stats.RolledBackRotations++
		}

		if r.RollbackAttempted {
			stats.RollbackAttempts++
		}
	}

	if stats.SuccessfulRotations > 0 {
		stats.AverageDuration = stats.TotalDuration / time.Duration(stats.SuccessfulRotations)
	}

	if stats.TotalRotations > 0 {
		stats.SuccessRate = float64(stats.SuccessfulRotations) / float64(stats.TotalRotations) * 100
	}

	return stats, nil
}

// RotationStats contains rotation statistics.
type RotationStats struct {
	// Path is the secret path (empty for all paths).
	Path string `json:"path"`

	// Since is the start time for the statistics.
	Since time.Time `json:"since"`

	// TotalRotations is the total number of rotations.
	TotalRotations int `json:"total_rotations"`

	// SuccessfulRotations is the number of successful rotations.
	SuccessfulRotations int `json:"successful_rotations"`

	// FailedRotations is the number of failed rotations.
	FailedRotations int `json:"failed_rotations"`

	// RolledBackRotations is the number of rolled-back rotations.
	RolledBackRotations int `json:"rolled_back_rotations"`

	// RollbackAttempts is the number of rollback attempts.
	RollbackAttempts int `json:"rollback_attempts"`

	// TotalDuration is the total time spent on successful rotations.
	TotalDuration time.Duration `json:"total_duration"`

	// AverageDuration is the average rotation duration.
	AverageDuration time.Duration `json:"average_duration"`

	// SuccessRate is the percentage of successful rotations.
	SuccessRate float64 `json:"success_rate"`
}
