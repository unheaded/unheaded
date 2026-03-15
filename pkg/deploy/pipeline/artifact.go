// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// Artifact tracking errors.
var (
	ErrArtifactNotFound      = errors.New("artifact not found")
	ErrArtifactVersionExists = errors.New("artifact version already exists")
	ErrArtifactInvalid       = errors.New("invalid artifact")
	ErrArtifactNotReady      = errors.New("artifact not ready for deployment")
	ErrChecksumMismatch      = errors.New("artifact checksum mismatch")
)

// ArtifactType represents the type of deployment artifact.
type ArtifactType string

const (
	ArtifactTypeContainer ArtifactType = "container"
	ArtifactTypeBinary    ArtifactType = "binary"
	ArtifactTypePackage   ArtifactType = "package"
	ArtifactTypeConfig    ArtifactType = "config"
	ArtifactTypeNix       ArtifactType = "nix"
	ArtifactTypeHelm      ArtifactType = "helm"
)

// ArtifactState represents the state of an artifact.
type ArtifactState string

const (
	ArtifactStatePending   ArtifactState = "pending"
	ArtifactStateBuilding  ArtifactState = "building"
	ArtifactStateUploading ArtifactState = "uploading"
	ArtifactStateReady     ArtifactState = "ready"
	ArtifactStateDeploying ArtifactState = "deploying"
	ArtifactStateDeployed  ArtifactState = "deployed"
	ArtifactStateFailed    ArtifactState = "failed"
	ArtifactStateArchived  ArtifactState = "archived"
)

// PipelineArtifact represents an artifact tracked by the pipeline.
type PipelineArtifact struct {
	// ID is the unique artifact identifier
	ID string `json:"id"`

	// Name is the artifact name (e.g., service name)
	Name string `json:"name"`

	// Version is the artifact version
	Version string `json:"version"`

	// Type is the artifact type
	Type ArtifactType `json:"type"`

	// State is the current artifact state
	State ArtifactState `json:"state"`

	// Reference is the artifact reference (e.g., container image URI)
	Reference string `json:"reference"`

	// Checksum is the SHA256 checksum
	Checksum string `json:"checksum,omitempty"`

	// Size is the artifact size in bytes
	Size int64 `json:"size,omitempty"`

	// Build contains build information
	Build *ArtifactBuild `json:"build,omitempty"`

	// Deployment contains deployment information
	Deployment *ArtifactDeployment `json:"deployment,omitempty"`

	// Labels contain metadata labels
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations contain additional annotations
	Annotations map[string]string `json:"annotations,omitempty"`

	// CreatedAt is when the artifact was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the artifact was last updated
	UpdatedAt time.Time `json:"updated_at"`

	// DeployedAt is when the artifact was deployed
	DeployedAt time.Time `json:"deployed_at,omitempty"`
}

// ArtifactBuild contains build information for an artifact.
type ArtifactBuild struct {
	// BuildID is the build identifier
	BuildID string `json:"build_id,omitempty"`

	// BuildNumber is the CI build number
	BuildNumber int `json:"build_number,omitempty"`

	// GitCommit is the git commit SHA
	GitCommit string `json:"git_commit,omitempty"`

	// GitBranch is the git branch
	GitBranch string `json:"git_branch,omitempty"`

	// GitTag is the git tag (if any)
	GitTag string `json:"git_tag,omitempty"`

	// GitRepo is the git repository URL
	GitRepo string `json:"git_repo,omitempty"`

	// BuildHost is the build host
	BuildHost string `json:"build_host,omitempty"`

	// BuildTime is when the build occurred
	BuildTime time.Time `json:"build_time,omitempty"`

	// Builder is the build system (e.g., jenkins, github-actions)
	Builder string `json:"builder,omitempty"`

	// BuildURL is the build URL
	BuildURL string `json:"build_url,omitempty"`

	// BuildDuration is how long the build took
	BuildDuration time.Duration `json:"build_duration,omitempty"`

	// Dependencies lists artifact dependencies
	Dependencies []ArtifactDependency `json:"dependencies,omitempty"`
}

// ArtifactDependency represents a dependency of an artifact.
type ArtifactDependency struct {
	// Name is the dependency name
	Name string `json:"name"`

	// Version is the dependency version
	Version string `json:"version"`

	// Type is the dependency type
	Type string `json:"type"`

	// Reference is the dependency reference
	Reference string `json:"reference,omitempty"`
}

// ArtifactDeployment contains deployment information for an artifact.
type ArtifactDeployment struct {
	// DeploymentID is the deployment identifier
	DeploymentID string `json:"deployment_id"`

	// PipelineID is the pipeline that deployed this artifact
	PipelineID string `json:"pipeline_id,omitempty"`

	// Environment is the deployment environment
	Environment string `json:"environment,omitempty"`

	// Cluster is the target cluster
	Cluster string `json:"cluster,omitempty"`

	// Namespace is the target namespace
	Namespace string `json:"namespace,omitempty"`

	// Replicas is the number of deployed replicas
	Replicas int `json:"replicas,omitempty"`

	// Strategy is the deployment strategy used
	Strategy string `json:"strategy,omitempty"`

	// DeployedBy is who/what deployed the artifact
	DeployedBy string `json:"deployed_by,omitempty"`

	// DeployedAt is when the artifact was deployed
	DeployedAt time.Time `json:"deployed_at,omitempty"`

	// PreviousVersion is the previous deployed version
	PreviousVersion string `json:"previous_version,omitempty"`

	// Healthy indicates if deployment is healthy
	Healthy bool `json:"healthy,omitempty"`
}

// ArtifactTracker tracks artifacts through the deployment pipeline.
type ArtifactTracker struct {
	mu sync.RWMutex

	// artifacts stores artifacts by ID
	artifacts map[string]*PipelineArtifact

	// byName indexes artifacts by name
	byName map[string][]*PipelineArtifact

	// byVersion indexes artifacts by name:version
	byVersion map[string]*PipelineArtifact

	// deployed tracks currently deployed artifacts by service
	deployed map[string]*PipelineArtifact

	// history tracks deployment history by service
	history map[string][]*DeploymentRecord

	// maxHistory is max history entries per service
	maxHistory int

	// eventCallback for artifact events
	eventCallback func(event *ArtifactEvent)
}

// DeploymentRecord records a deployment for history.
type DeploymentRecord struct {
	// ArtifactID is the artifact that was deployed
	ArtifactID string `json:"artifact_id"`

	// Name is the artifact name
	Name string `json:"name"`

	// Version is the deployed version
	Version string `json:"version"`

	// DeploymentID is the deployment ID
	DeploymentID string `json:"deployment_id"`

	// PipelineID is the pipeline ID
	PipelineID string `json:"pipeline_id,omitempty"`

	// Environment is the deployment environment
	Environment string `json:"environment,omitempty"`

	// Strategy is the deployment strategy
	Strategy string `json:"strategy,omitempty"`

	// Status is the deployment status
	Status string `json:"status"`

	// StartedAt is when deployment started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when deployment completed
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration is the deployment duration
	Duration time.Duration `json:"duration,omitempty"`

	// RollbackTo is set if this was a rollback deployment
	RollbackTo string `json:"rollback_to,omitempty"`

	// Error contains error if failed
	Error string `json:"error,omitempty"`
}

// ArtifactEvent represents an artifact tracking event.
type ArtifactEvent struct {
	// Type is the event type
	Type string `json:"type"`

	// ArtifactID is the artifact ID
	ArtifactID string `json:"artifact_id"`

	// Name is the artifact name
	Name string `json:"name"`

	// Version is the artifact version
	Version string `json:"version"`

	// State is the artifact state
	State ArtifactState `json:"state"`

	// Timestamp of the event
	Timestamp time.Time `json:"timestamp"`

	// Data contains additional event data
	Data map[string]interface{} `json:"data,omitempty"`
}

// NewArtifactTracker creates a new artifact tracker.
func NewArtifactTracker(maxHistory int) *ArtifactTracker {
	if maxHistory <= 0 {
		maxHistory = 100
	}

	return &ArtifactTracker{
		artifacts:  make(map[string]*PipelineArtifact),
		byName:     make(map[string][]*PipelineArtifact),
		byVersion:  make(map[string]*PipelineArtifact),
		deployed:   make(map[string]*PipelineArtifact),
		history:    make(map[string][]*DeploymentRecord),
		maxHistory: maxHistory,
	}
}

// SetEventCallback sets the event callback.
func (t *ArtifactTracker) SetEventCallback(callback func(event *ArtifactEvent)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.eventCallback = callback
}

// Register registers a new artifact.
func (t *ArtifactTracker) Register(ctx context.Context, artifact *PipelineArtifact) error {
	if artifact == nil {
		return ErrArtifactInvalid
	}

	if artifact.Name == "" || artifact.Version == "" {
		return fmt.Errorf("%w: name and version required", ErrArtifactInvalid)
	}

	// Generate ID if not set
	if artifact.ID == "" {
		artifact.ID = generateArtifactID(artifact.Name, artifact.Version)
	}

	// Set timestamps
	now := time.Now()
	artifact.CreatedAt = now
	artifact.UpdatedAt = now

	// Default state
	if artifact.State == "" {
		artifact.State = ArtifactStatePending
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Check for duplicate
	versionKey := artifact.Name + ":" + artifact.Version
	if _, exists := t.byVersion[versionKey]; exists {
		return ErrArtifactVersionExists
	}

	// Store artifact
	t.artifacts[artifact.ID] = artifact
	t.byName[artifact.Name] = append(t.byName[artifact.Name], artifact)
	t.byVersion[versionKey] = artifact

	t.emitEvent(&ArtifactEvent{
		Type:       "artifact_registered",
		ArtifactID: artifact.ID,
		Name:       artifact.Name,
		Version:    artifact.Version,
		State:      artifact.State,
		Timestamp:  now,
	})

	return nil
}

// Update updates an artifact.
func (t *ArtifactTracker) Update(ctx context.Context, artifactID string, updates *ArtifactUpdate) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	artifact, exists := t.artifacts[artifactID]
	if !exists {
		return ErrArtifactNotFound
	}

	// Apply updates
	if updates.State != "" {
		artifact.State = updates.State
	}
	if updates.Reference != "" {
		artifact.Reference = updates.Reference
	}
	if updates.Checksum != "" {
		artifact.Checksum = updates.Checksum
	}
	if updates.Size > 0 {
		artifact.Size = updates.Size
	}
	if updates.Build != nil {
		artifact.Build = updates.Build
	}
	if updates.Labels != nil {
		if artifact.Labels == nil {
			artifact.Labels = make(map[string]string)
		}
		for k, v := range updates.Labels {
			artifact.Labels[k] = v
		}
	}
	if updates.Annotations != nil {
		if artifact.Annotations == nil {
			artifact.Annotations = make(map[string]string)
		}
		for k, v := range updates.Annotations {
			artifact.Annotations[k] = v
		}
	}

	artifact.UpdatedAt = time.Now()

	t.emitEvent(&ArtifactEvent{
		Type:       "artifact_updated",
		ArtifactID: artifact.ID,
		Name:       artifact.Name,
		Version:    artifact.Version,
		State:      artifact.State,
		Timestamp:  artifact.UpdatedAt,
	})

	return nil
}

// ArtifactUpdate contains fields to update.
type ArtifactUpdate struct {
	State       ArtifactState     `json:"state,omitempty"`
	Reference   string            `json:"reference,omitempty"`
	Checksum    string            `json:"checksum,omitempty"`
	Size        int64             `json:"size,omitempty"`
	Build       *ArtifactBuild    `json:"build,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Get retrieves an artifact by ID.
func (t *ArtifactTracker) Get(ctx context.Context, artifactID string) (*PipelineArtifact, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	artifact, exists := t.artifacts[artifactID]
	if !exists {
		return nil, ErrArtifactNotFound
	}

	return artifact, nil
}

// GetByVersion retrieves an artifact by name and version.
func (t *ArtifactTracker) GetByVersion(ctx context.Context, name, version string) (*PipelineArtifact, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	versionKey := name + ":" + version
	artifact, exists := t.byVersion[versionKey]
	if !exists {
		return nil, ErrArtifactNotFound
	}

	return artifact, nil
}

// GetVersions retrieves all versions of an artifact.
func (t *ArtifactTracker) GetVersions(ctx context.Context, name string, limit int) ([]*PipelineArtifact, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	artifacts, exists := t.byName[name]
	if !exists {
		return []*PipelineArtifact{}, nil
	}

	// Sort by creation time (newest first)
	sorted := make([]*PipelineArtifact, len(artifacts))
	copy(sorted, artifacts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	if limit > 0 && limit < len(sorted) {
		sorted = sorted[:limit]
	}

	return sorted, nil
}

// GetLatest retrieves the latest version of an artifact.
func (t *ArtifactTracker) GetLatest(ctx context.Context, name string) (*PipelineArtifact, error) {
	versions, err := t.GetVersions(ctx, name, 1)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, ErrArtifactNotFound
	}
	return versions[0], nil
}

// GetDeployed retrieves the currently deployed artifact for a service.
func (t *ArtifactTracker) GetDeployed(ctx context.Context, name string) (*PipelineArtifact, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	artifact, exists := t.deployed[name]
	if !exists {
		return nil, ErrArtifactNotFound
	}

	return artifact, nil
}

// MarkDeploying marks an artifact as deploying.
func (t *ArtifactTracker) MarkDeploying(ctx context.Context, artifactID string, deployment *ArtifactDeployment) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	artifact, exists := t.artifacts[artifactID]
	if !exists {
		return ErrArtifactNotFound
	}

	if artifact.State != ArtifactStateReady {
		return fmt.Errorf("%w: artifact is in state %s", ErrArtifactNotReady, artifact.State)
	}

	// Get previous version
	if prev, exists := t.deployed[artifact.Name]; exists {
		deployment.PreviousVersion = prev.Version
	}

	artifact.State = ArtifactStateDeploying
	artifact.Deployment = deployment
	artifact.UpdatedAt = time.Now()

	// Record deployment start
	record := &DeploymentRecord{
		ArtifactID:   artifact.ID,
		Name:         artifact.Name,
		Version:      artifact.Version,
		DeploymentID: deployment.DeploymentID,
		PipelineID:   deployment.PipelineID,
		Environment:  deployment.Environment,
		Strategy:     deployment.Strategy,
		Status:       "deploying",
		StartedAt:    time.Now(),
	}

	t.history[artifact.Name] = append(t.history[artifact.Name], record)

	// Trim history
	if len(t.history[artifact.Name]) > t.maxHistory {
		t.history[artifact.Name] = t.history[artifact.Name][len(t.history[artifact.Name])-t.maxHistory:]
	}

	t.emitEvent(&ArtifactEvent{
		Type:       "artifact_deploying",
		ArtifactID: artifact.ID,
		Name:       artifact.Name,
		Version:    artifact.Version,
		State:      artifact.State,
		Timestamp:  artifact.UpdatedAt,
		Data: map[string]interface{}{
			"deployment_id": deployment.DeploymentID,
			"environment":   deployment.Environment,
		},
	})

	return nil
}

// MarkDeployed marks an artifact as deployed.
func (t *ArtifactTracker) MarkDeployed(ctx context.Context, artifactID string, healthy bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	artifact, exists := t.artifacts[artifactID]
	if !exists {
		return ErrArtifactNotFound
	}

	now := time.Now()
	artifact.State = ArtifactStateDeployed
	artifact.UpdatedAt = now
	artifact.DeployedAt = now

	if artifact.Deployment != nil {
		artifact.Deployment.DeployedAt = now
		artifact.Deployment.Healthy = healthy
	}

	// Update deployed map
	t.deployed[artifact.Name] = artifact

	// Update deployment record
	for i := len(t.history[artifact.Name]) - 1; i >= 0; i-- {
		record := t.history[artifact.Name][i]
		if record.ArtifactID == artifactID && record.Status == "deploying" {
			record.Status = "deployed"
			record.CompletedAt = now
			record.Duration = now.Sub(record.StartedAt)
			break
		}
	}

	t.emitEvent(&ArtifactEvent{
		Type:       "artifact_deployed",
		ArtifactID: artifact.ID,
		Name:       artifact.Name,
		Version:    artifact.Version,
		State:      artifact.State,
		Timestamp:  now,
		Data: map[string]interface{}{
			"healthy": healthy,
		},
	})

	return nil
}

// MarkFailed marks an artifact deployment as failed.
func (t *ArtifactTracker) MarkFailed(ctx context.Context, artifactID string, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	artifact, exists := t.artifacts[artifactID]
	if !exists {
		return ErrArtifactNotFound
	}

	now := time.Now()
	artifact.State = ArtifactStateFailed
	artifact.UpdatedAt = now

	// Update deployment record
	for i := len(t.history[artifact.Name]) - 1; i >= 0; i-- {
		record := t.history[artifact.Name][i]
		if record.ArtifactID == artifactID && record.Status == "deploying" {
			record.Status = "failed"
			record.CompletedAt = now
			record.Duration = now.Sub(record.StartedAt)
			if err != nil {
				record.Error = err.Error()
			}
			break
		}
	}

	t.emitEvent(&ArtifactEvent{
		Type:       "artifact_failed",
		ArtifactID: artifact.ID,
		Name:       artifact.Name,
		Version:    artifact.Version,
		State:      artifact.State,
		Timestamp:  now,
		Data: map[string]interface{}{
			"error": err.Error(),
		},
	})

	return nil
}

// MarkRollback marks an artifact for rollback.
func (t *ArtifactTracker) MarkRollback(ctx context.Context, name, toVersion, deploymentID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Get the target artifact
	versionKey := name + ":" + toVersion
	artifact, exists := t.byVersion[versionKey]
	if !exists {
		return ErrArtifactNotFound
	}

	now := time.Now()

	// Record rollback in history
	record := &DeploymentRecord{
		ArtifactID:   artifact.ID,
		Name:         artifact.Name,
		Version:      artifact.Version,
		DeploymentID: deploymentID,
		Status:       "rollback",
		StartedAt:    now,
		RollbackTo:   toVersion,
	}

	t.history[name] = append(t.history[name], record)

	t.emitEvent(&ArtifactEvent{
		Type:       "artifact_rollback",
		ArtifactID: artifact.ID,
		Name:       artifact.Name,
		Version:    artifact.Version,
		State:      artifact.State,
		Timestamp:  now,
		Data: map[string]interface{}{
			"rollback_to":   toVersion,
			"deployment_id": deploymentID,
		},
	})

	return nil
}

// GetHistory retrieves deployment history for a service.
func (t *ArtifactTracker) GetHistory(ctx context.Context, name string, limit int) ([]*DeploymentRecord, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history, exists := t.history[name]
	if !exists {
		return []*DeploymentRecord{}, nil
	}

	// Return copy in reverse order (newest first)
	result := make([]*DeploymentRecord, len(history))
	for i, record := range history {
		result[len(history)-1-i] = record
	}

	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}

	return result, nil
}

// GetPreviousDeployed retrieves the previously deployed artifact for a service.
func (t *ArtifactTracker) GetPreviousDeployed(ctx context.Context, name string) (*PipelineArtifact, error) {
	history, err := t.GetHistory(ctx, name, 10)
	if err != nil {
		return nil, err
	}

	// Find the last successful deployment that's not the current one
	current, _ := t.GetDeployed(ctx, name)
	currentVersion := ""
	if current != nil {
		currentVersion = current.Version
	}

	for _, record := range history {
		if record.Status == "deployed" && record.Version != currentVersion {
			return t.GetByVersion(ctx, name, record.Version)
		}
	}

	return nil, ErrArtifactNotFound
}

// List lists all artifacts with optional filtering.
func (t *ArtifactTracker) List(ctx context.Context, filter *ArtifactFilter) ([]*PipelineArtifact, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*PipelineArtifact, 0)

	for _, artifact := range t.artifacts {
		if filter != nil && !filter.Match(artifact) {
			continue
		}
		result = append(result, artifact)
	}

	// Sort by creation time (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// ArtifactFilter defines filtering criteria.
type ArtifactFilter struct {
	Name      string            `json:"name,omitempty"`
	Type      ArtifactType      `json:"type,omitempty"`
	State     ArtifactState     `json:"state,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAfter  time.Time     `json:"created_after,omitempty"`
	CreatedBefore time.Time     `json:"created_before,omitempty"`
}

// Match checks if an artifact matches the filter.
func (f *ArtifactFilter) Match(artifact *PipelineArtifact) bool {
	if f.Name != "" && artifact.Name != f.Name {
		return false
	}
	if f.Type != "" && artifact.Type != f.Type {
		return false
	}
	if f.State != "" && artifact.State != f.State {
		return false
	}
	if !f.CreatedAfter.IsZero() && artifact.CreatedAt.Before(f.CreatedAfter) {
		return false
	}
	if !f.CreatedBefore.IsZero() && artifact.CreatedAt.After(f.CreatedBefore) {
		return false
	}
	if f.Labels != nil {
		for k, v := range f.Labels {
			if artifact.Labels[k] != v {
				return false
			}
		}
	}
	return true
}

// Verify verifies artifact integrity.
func (t *ArtifactTracker) Verify(ctx context.Context, artifactID string, reader io.Reader) error {
	t.mu.RLock()
	artifact, exists := t.artifacts[artifactID]
	t.mu.RUnlock()

	if !exists {
		return ErrArtifactNotFound
	}

	if artifact.Checksum == "" {
		return nil // No checksum to verify
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return fmt.Errorf("failed to read artifact: %w", err)
	}

	calculated := hex.EncodeToString(hash.Sum(nil))
	if calculated != artifact.Checksum {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, artifact.Checksum, calculated)
	}

	return nil
}

// Archive archives an artifact.
func (t *ArtifactTracker) Archive(ctx context.Context, artifactID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	artifact, exists := t.artifacts[artifactID]
	if !exists {
		return ErrArtifactNotFound
	}

	artifact.State = ArtifactStateArchived
	artifact.UpdatedAt = time.Now()

	t.emitEvent(&ArtifactEvent{
		Type:       "artifact_archived",
		ArtifactID: artifact.ID,
		Name:       artifact.Name,
		Version:    artifact.Version,
		State:      artifact.State,
		Timestamp:  artifact.UpdatedAt,
	})

	return nil
}

// Delete removes an artifact.
func (t *ArtifactTracker) Delete(ctx context.Context, artifactID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	artifact, exists := t.artifacts[artifactID]
	if !exists {
		return ErrArtifactNotFound
	}

	// Check if currently deployed
	if deployed, ok := t.deployed[artifact.Name]; ok && deployed.ID == artifactID {
		return fmt.Errorf("cannot delete currently deployed artifact")
	}

	// Remove from maps
	delete(t.artifacts, artifactID)

	versionKey := artifact.Name + ":" + artifact.Version
	delete(t.byVersion, versionKey)

	// Remove from byName slice
	if versions, ok := t.byName[artifact.Name]; ok {
		for i, v := range versions {
			if v.ID == artifactID {
				t.byName[artifact.Name] = append(versions[:i], versions[i+1:]...)
				break
			}
		}
	}

	t.emitEvent(&ArtifactEvent{
		Type:       "artifact_deleted",
		ArtifactID: artifact.ID,
		Name:       artifact.Name,
		Version:    artifact.Version,
		Timestamp:  time.Now(),
	})

	return nil
}

// Cleanup removes old artifacts based on retention policy.
func (t *ArtifactTracker) Cleanup(ctx context.Context, policy *RetentionPolicy) (int, error) {
	if policy == nil {
		return 0, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	deleted := 0
	now := time.Now()

	for _, artifact := range t.artifacts {
		// Skip currently deployed
		if deployed, ok := t.deployed[artifact.Name]; ok && deployed.ID == artifact.ID {
			continue
		}

		shouldDelete := false

		// Check age
		if policy.MaxAge > 0 && now.Sub(artifact.CreatedAt) > policy.MaxAge {
			shouldDelete = true
		}

		// Check state
		if policy.DeleteArchived && artifact.State == ArtifactStateArchived {
			shouldDelete = true
		}
		if policy.DeleteFailed && artifact.State == ArtifactStateFailed {
			shouldDelete = true
		}

		if shouldDelete {
			delete(t.artifacts, artifact.ID)
			versionKey := artifact.Name + ":" + artifact.Version
			delete(t.byVersion, versionKey)
			deleted++
		}
	}

	// Enforce max versions per artifact
	if policy.MaxVersions > 0 {
		for name, versions := range t.byName {
			if len(versions) <= policy.MaxVersions {
				continue
			}

			// Sort by creation time (newest first)
			sort.Slice(versions, func(i, j int) bool {
				return versions[i].CreatedAt.After(versions[j].CreatedAt)
			})

			// Delete old versions
			for i := policy.MaxVersions; i < len(versions); i++ {
				artifact := versions[i]

				// Skip currently deployed
				if deployed, ok := t.deployed[name]; ok && deployed.ID == artifact.ID {
					continue
				}

				delete(t.artifacts, artifact.ID)
				versionKey := name + ":" + artifact.Version
				delete(t.byVersion, versionKey)
				deleted++
			}

			// Update byName slice
			if len(versions) > policy.MaxVersions {
				t.byName[name] = versions[:policy.MaxVersions]
			}
		}
	}

	return deleted, nil
}

// RetentionPolicy defines artifact retention rules.
type RetentionPolicy struct {
	// MaxAge is the maximum age for artifacts
	MaxAge time.Duration `json:"max_age,omitempty"`

	// MaxVersions is the maximum versions to keep per artifact
	MaxVersions int `json:"max_versions,omitempty"`

	// DeleteArchived deletes archived artifacts
	DeleteArchived bool `json:"delete_archived,omitempty"`

	// DeleteFailed deletes failed artifacts
	DeleteFailed bool `json:"delete_failed,omitempty"`
}

// GetStats returns artifact statistics.
func (t *ArtifactTracker) GetStats(ctx context.Context) *ArtifactStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := &ArtifactStats{
		Total:     len(t.artifacts),
		ByState:   make(map[ArtifactState]int),
		ByType:    make(map[ArtifactType]int),
		ByService: make(map[string]int),
		Deployed:  len(t.deployed),
	}

	var totalSize int64
	for _, artifact := range t.artifacts {
		stats.ByState[artifact.State]++
		stats.ByType[artifact.Type]++
		stats.ByService[artifact.Name]++
		totalSize += artifact.Size
	}

	stats.TotalSize = totalSize

	return stats
}

// ArtifactStats contains artifact statistics.
type ArtifactStats struct {
	Total     int                      `json:"total"`
	TotalSize int64                    `json:"total_size"`
	Deployed  int                      `json:"deployed"`
	ByState   map[ArtifactState]int    `json:"by_state"`
	ByType    map[ArtifactType]int     `json:"by_type"`
	ByService map[string]int           `json:"by_service"`
}

// Export exports artifacts to JSON.
func (t *ArtifactTracker) Export(ctx context.Context) ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	export := struct {
		Artifacts []*PipelineArtifact            `json:"artifacts"`
		Deployed  map[string]string              `json:"deployed"`
		History   map[string][]*DeploymentRecord `json:"history"`
	}{
		Artifacts: make([]*PipelineArtifact, 0, len(t.artifacts)),
		Deployed:  make(map[string]string),
		History:   t.history,
	}

	for _, artifact := range t.artifacts {
		export.Artifacts = append(export.Artifacts, artifact)
	}

	for name, artifact := range t.deployed {
		export.Deployed[name] = artifact.ID
	}

	return json.Marshal(export)
}

// emitEvent emits an artifact event.
func (t *ArtifactTracker) emitEvent(event *ArtifactEvent) {
	callback := t.eventCallback
	if callback != nil {
		go callback(event)
	}
}

// generateArtifactID generates a unique artifact ID.
func generateArtifactID(name, version string) string {
	return fmt.Sprintf("%s-%s-%d", name, version, time.Now().UnixNano())
}

// ArtifactBuilder helps build artifact objects.
type ArtifactBuilder struct {
	artifact *PipelineArtifact
}

// NewArtifactBuilder creates a new artifact builder.
func NewArtifactBuilder(name, version string) *ArtifactBuilder {
	return &ArtifactBuilder{
		artifact: &PipelineArtifact{
			Name:    name,
			Version: version,
			State:   ArtifactStatePending,
			Labels:  make(map[string]string),
		},
	}
}

// WithType sets the artifact type.
func (b *ArtifactBuilder) WithType(t ArtifactType) *ArtifactBuilder {
	b.artifact.Type = t
	return b
}

// WithReference sets the artifact reference.
func (b *ArtifactBuilder) WithReference(ref string) *ArtifactBuilder {
	b.artifact.Reference = ref
	return b
}

// WithChecksum sets the artifact checksum.
func (b *ArtifactBuilder) WithChecksum(checksum string) *ArtifactBuilder {
	b.artifact.Checksum = checksum
	return b
}

// WithSize sets the artifact size.
func (b *ArtifactBuilder) WithSize(size int64) *ArtifactBuilder {
	b.artifact.Size = size
	return b
}

// WithBuild sets the build information.
func (b *ArtifactBuilder) WithBuild(build *ArtifactBuild) *ArtifactBuilder {
	b.artifact.Build = build
	return b
}

// WithLabel adds a label.
func (b *ArtifactBuilder) WithLabel(key, value string) *ArtifactBuilder {
	b.artifact.Labels[key] = value
	return b
}

// WithAnnotation adds an annotation.
func (b *ArtifactBuilder) WithAnnotation(key, value string) *ArtifactBuilder {
	if b.artifact.Annotations == nil {
		b.artifact.Annotations = make(map[string]string)
	}
	b.artifact.Annotations[key] = value
	return b
}

// Ready marks the artifact as ready.
func (b *ArtifactBuilder) Ready() *ArtifactBuilder {
	b.artifact.State = ArtifactStateReady
	return b
}

// Build returns the built artifact.
func (b *ArtifactBuilder) Build() *PipelineArtifact {
	return b.artifact
}
