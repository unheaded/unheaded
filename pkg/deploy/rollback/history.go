// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package rollback provides rollback management for deployments.
package rollback

import (
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
)

// HistoryEntry represents a historical deployment entry.
type HistoryEntry struct {
	// DeploymentID is the deployment ID
	DeploymentID string `json:"deployment_id"`

	// ServiceName is the service name
	ServiceName string `json:"service_name"`

	// Version is the deployed version
	Version string `json:"version"`

	// ArtifactRef is the artifact reference
	ArtifactRef string `json:"artifact_ref"`

	// State is the deployment state
	State string `json:"state"`

	// Replicas is the number of replicas
	Replicas int `json:"replicas"`

	// StartedAt is when the deployment started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the deployment completed
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration is the deployment duration
	Duration time.Duration `json:"duration,omitempty"`

	// RollbackID is set if this deployment was rolled back
	RollbackID string `json:"rollback_id,omitempty"`

	// RollbackFromID is set if this deployment is a rollback from another
	RollbackFromID string `json:"rollback_from_id,omitempty"`

	// Metadata contains additional deployment metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// History stores deployment and rollback history.
type History struct {
	mu sync.RWMutex

	// deployments stores deployment history by service
	deployments map[string][]*HistoryEntry

	// rollbacks stores rollback history by service
	rollbacks map[string][]*Rollback

	// rollbackByID maps rollback ID to rollback
	rollbackByID map[string]*Rollback

	// maxSize is the maximum entries per service
	maxSize int
}

// NewHistory creates a new history store.
func NewHistory(maxSize int) *History {
	if maxSize <= 0 {
		maxSize = 100
	}

	return &History{
		deployments:  make(map[string][]*HistoryEntry),
		rollbacks:    make(map[string][]*Rollback),
		rollbackByID: make(map[string]*Rollback),
		maxSize:      maxSize,
	}
}

// AddDeployment adds a deployment to history.
func (h *History) AddDeployment(entry *HistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()

	history := h.deployments[entry.ServiceName]
	history = append(history, entry)

	// Sort by start time (newest first)
	sort.Slice(history, func(i, j int) bool {
		return history[i].StartedAt.After(history[j].StartedAt)
	})

	// Trim to max size
	if len(history) > h.maxSize {
		history = history[:h.maxSize]
	}

	h.deployments[entry.ServiceName] = history
}

// Add adds a rollback to history.
func (h *History) Add(rollback *Rollback) {
	h.mu.Lock()
	defer h.mu.Unlock()

	history := h.rollbacks[rollback.ServiceName]
	history = append(history, rollback)

	// Sort by start time (newest first)
	sort.Slice(history, func(i, j int) bool {
		return history[i].StartedAt.After(history[j].StartedAt)
	})

	// Trim to max size
	if len(history) > h.maxSize {
		history = history[:h.maxSize]
	}

	h.rollbacks[rollback.ServiceName] = history
	h.rollbackByID[rollback.ID] = rollback
}

// Get returns a rollback by ID.
func (h *History) Get(rollbackID string) (*Rollback, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	rollback, exists := h.rollbackByID[rollbackID]
	if !exists {
		return nil, errors.New("rollback not found")
	}

	return rollback, nil
}

// GetByService returns rollbacks for a service.
func (h *History) GetByService(serviceName string) ([]*Rollback, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	history, exists := h.rollbacks[serviceName]
	if !exists {
		return []*Rollback{}, nil
	}

	// Return a copy
	result := make([]*Rollback, len(history))
	copy(result, history)
	return result, nil
}

// GetDeployments returns deployment history for a service.
func (h *History) GetDeployments(serviceName string, limit int) ([]*HistoryEntry, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	history, exists := h.deployments[serviceName]
	if !exists {
		return []*HistoryEntry{}, nil
	}

	if limit <= 0 || limit > len(history) {
		limit = len(history)
	}

	// Return a copy of the requested amount
	result := make([]*HistoryEntry, limit)
	copy(result, history[:limit])
	return result, nil
}

// GetPreviousSuccessful returns the previous successful deployment.
func (h *History) GetPreviousSuccessful(serviceName string, beforeDeploymentID string) (*HistoryEntry, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	history, exists := h.deployments[serviceName]
	if !exists {
		return nil, errors.New("no history for service")
	}

	foundCurrent := false
	for _, entry := range history {
		if entry.DeploymentID == beforeDeploymentID {
			foundCurrent = true
			continue
		}

		if foundCurrent && entry.State == "completed" {
			return entry, nil
		}
	}

	return nil, errors.New("no previous successful deployment found")
}

// GetRollbacksAfter returns rollbacks after a given time.
func (h *History) GetRollbacksAfter(serviceName string, after time.Time) ([]*Rollback, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	history, exists := h.rollbacks[serviceName]
	if !exists {
		return []*Rollback{}, nil
	}

	var result []*Rollback
	for _, rollback := range history {
		if rollback.StartedAt.After(after) {
			result = append(result, rollback)
		}
	}

	return result, nil
}

// GetDeploymentsAfter returns deployments after a given time.
func (h *History) GetDeploymentsAfter(serviceName string, after time.Time) ([]*HistoryEntry, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	history, exists := h.deployments[serviceName]
	if !exists {
		return []*HistoryEntry{}, nil
	}

	var result []*HistoryEntry
	for _, entry := range history {
		if entry.StartedAt.After(after) {
			result = append(result, entry)
		}
	}

	return result, nil
}

// GetStats returns history statistics for a service.
func (h *History) GetStats(serviceName string) *HistoryStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats := &HistoryStats{
		ServiceName: serviceName,
	}

	// Deployment stats
	if deployments, exists := h.deployments[serviceName]; exists {
		stats.TotalDeployments = len(deployments)
		for _, entry := range deployments {
			if entry.State == "completed" {
				stats.SuccessfulDeployments++
			} else if entry.State == "failed" {
				stats.FailedDeployments++
			}
		}
	}

	// Rollback stats
	if rollbacks, exists := h.rollbacks[serviceName]; exists {
		stats.TotalRollbacks = len(rollbacks)
		for _, rollback := range rollbacks {
			if rollback.State == RollbackStateCompleted {
				stats.SuccessfulRollbacks++
			} else if rollback.State == RollbackStateFailed {
				stats.FailedRollbacks++
			}

			// Track reasons
			switch rollback.Reason {
			case RollbackReasonManual:
				stats.ManualRollbacks++
			case RollbackReasonAutomatic, RollbackReasonHealthCheck, RollbackReasonCanaryFail:
				stats.AutomaticRollbacks++
			}
		}
	}

	return stats
}

// HistoryStats contains history statistics.
type HistoryStats struct {
	// ServiceName is the service name
	ServiceName string `json:"service_name"`

	// Deployment stats
	TotalDeployments      int `json:"total_deployments"`
	SuccessfulDeployments int `json:"successful_deployments"`
	FailedDeployments     int `json:"failed_deployments"`

	// Rollback stats
	TotalRollbacks      int `json:"total_rollbacks"`
	SuccessfulRollbacks int `json:"successful_rollbacks"`
	FailedRollbacks     int `json:"failed_rollbacks"`
	ManualRollbacks     int `json:"manual_rollbacks"`
	AutomaticRollbacks  int `json:"automatic_rollbacks"`
}

// Clear clears history for a service.
func (h *History) Clear(serviceName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Remove rollbacks from ID map
	if rollbacks, exists := h.rollbacks[serviceName]; exists {
		for _, rollback := range rollbacks {
			delete(h.rollbackByID, rollback.ID)
		}
	}

	delete(h.deployments, serviceName)
	delete(h.rollbacks, serviceName)
}

// ClearAll clears all history.
func (h *History) ClearAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.deployments = make(map[string][]*HistoryEntry)
	h.rollbacks = make(map[string][]*Rollback)
	h.rollbackByID = make(map[string]*Rollback)
}

// Export exports history as JSON.
func (h *History) Export() ([]byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	export := struct {
		Deployments map[string][]*HistoryEntry `json:"deployments"`
		Rollbacks   map[string][]*Rollback     `json:"rollbacks"`
	}{
		Deployments: h.deployments,
		Rollbacks:   h.rollbacks,
	}

	return json.Marshal(export)
}

// Import imports history from JSON.
func (h *History) Import(data []byte) error {
	var imported struct {
		Deployments map[string][]*HistoryEntry `json:"deployments"`
		Rollbacks   map[string][]*Rollback     `json:"rollbacks"`
	}

	if err := json.Unmarshal(data, &imported); err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Merge deployments
	for service, entries := range imported.Deployments {
		h.deployments[service] = append(h.deployments[service], entries...)
		// Sort and trim
		sort.Slice(h.deployments[service], func(i, j int) bool {
			return h.deployments[service][i].StartedAt.After(h.deployments[service][j].StartedAt)
		})
		if len(h.deployments[service]) > h.maxSize {
			h.deployments[service] = h.deployments[service][:h.maxSize]
		}
	}

	// Merge rollbacks
	for service, rollbacks := range imported.Rollbacks {
		for _, rollback := range rollbacks {
			h.rollbackByID[rollback.ID] = rollback
		}
		h.rollbacks[service] = append(h.rollbacks[service], rollbacks...)
		// Sort and trim
		sort.Slice(h.rollbacks[service], func(i, j int) bool {
			return h.rollbacks[service][i].StartedAt.After(h.rollbacks[service][j].StartedAt)
		})
		if len(h.rollbacks[service]) > h.maxSize {
			h.rollbacks[service] = h.rollbacks[service][:h.maxSize]
		}
	}

	return nil
}

// HistoryQuery defines a query for history entries.
type HistoryQuery struct {
	// ServiceName filters by service
	ServiceName string `json:"service_name,omitempty"`

	// StartTime filters entries after this time
	StartTime time.Time `json:"start_time,omitempty"`

	// EndTime filters entries before this time
	EndTime time.Time `json:"end_time,omitempty"`

	// State filters by state
	State string `json:"state,omitempty"`

	// IncludeRollbacks includes rollbacks in results
	IncludeRollbacks bool `json:"include_rollbacks,omitempty"`

	// Limit limits results
	Limit int `json:"limit,omitempty"`

	// Offset for pagination
	Offset int `json:"offset,omitempty"`
}

// Query queries history.
func (h *History) Query(query *HistoryQuery) (*HistoryQueryResult, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := &HistoryQueryResult{
		Entries:   []*HistoryEntry{},
		Rollbacks: []*Rollback{},
	}

	// Get deployments
	var services []string
	if query.ServiceName != "" {
		services = []string{query.ServiceName}
	} else {
		for service := range h.deployments {
			services = append(services, service)
		}
	}

	for _, service := range services {
		entries := h.deployments[service]
		for _, entry := range entries {
			if !matchesQuery(entry, query) {
				continue
			}
			result.Entries = append(result.Entries, entry)
		}

		if query.IncludeRollbacks {
			rollbacks := h.rollbacks[service]
			for _, rollback := range rollbacks {
				if !matchesRollbackQuery(rollback, query) {
					continue
				}
				result.Rollbacks = append(result.Rollbacks, rollback)
			}
		}
	}

	// Sort all entries by time
	sort.Slice(result.Entries, func(i, j int) bool {
		return result.Entries[i].StartedAt.After(result.Entries[j].StartedAt)
	})

	// Apply pagination
	if query.Offset > 0 && query.Offset < len(result.Entries) {
		result.Entries = result.Entries[query.Offset:]
	}
	if query.Limit > 0 && query.Limit < len(result.Entries) {
		result.Entries = result.Entries[:query.Limit]
	}

	result.Total = len(result.Entries)
	return result, nil
}

// HistoryQueryResult contains query results.
type HistoryQueryResult struct {
	Entries   []*HistoryEntry `json:"entries"`
	Rollbacks []*Rollback     `json:"rollbacks,omitempty"`
	Total     int             `json:"total"`
}

// matchesQuery checks if an entry matches a query.
func matchesQuery(entry *HistoryEntry, query *HistoryQuery) bool {
	if query.ServiceName != "" && entry.ServiceName != query.ServiceName {
		return false
	}
	if !query.StartTime.IsZero() && entry.StartedAt.Before(query.StartTime) {
		return false
	}
	if !query.EndTime.IsZero() && entry.StartedAt.After(query.EndTime) {
		return false
	}
	if query.State != "" && entry.State != query.State {
		return false
	}
	return true
}

// matchesRollbackQuery checks if a rollback matches a query.
func matchesRollbackQuery(rollback *Rollback, query *HistoryQuery) bool {
	if query.ServiceName != "" && rollback.ServiceName != query.ServiceName {
		return false
	}
	if !query.StartTime.IsZero() && rollback.StartedAt.Before(query.StartTime) {
		return false
	}
	if !query.EndTime.IsZero() && rollback.StartedAt.After(query.EndTime) {
		return false
	}
	if query.State != "" && string(rollback.State) != query.State {
		return false
	}
	return true
}
