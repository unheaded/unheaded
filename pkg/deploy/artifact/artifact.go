// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package artifact provides artifact management for deployments.
package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Common errors returned by artifact operations.
var (
	ErrArtifactNotFound     = errors.New("artifact not found")
	ErrArtifactExists       = errors.New("artifact already exists")
	ErrArtifactInvalid      = errors.New("invalid artifact")
	ErrChecksumMismatch     = errors.New("checksum mismatch")
	ErrStorageUnavailable   = errors.New("storage unavailable")
	ErrRegistryUnavailable  = errors.New("registry unavailable")
)

// ArtifactType represents the type of artifact.
type ArtifactType string

const (
	ArtifactTypeContainer ArtifactType = "container"
	ArtifactTypeBinary    ArtifactType = "binary"
	ArtifactTypeArchive   ArtifactType = "archive"
	ArtifactTypeConfig    ArtifactType = "config"
	ArtifactTypeNix       ArtifactType = "nix"
)

// ArtifactState represents the state of an artifact.
type ArtifactState string

const (
	ArtifactStatePending   ArtifactState = "pending"
	ArtifactStateUploading ArtifactState = "uploading"
	ArtifactStateReady     ArtifactState = "ready"
	ArtifactStateFailed    ArtifactState = "failed"
	ArtifactStateDeleted   ArtifactState = "deleted"
)

// Artifact represents a deployable artifact.
type Artifact struct {
	// ID is the unique artifact identifier
	ID string `json:"id"`

	// Name is the artifact name
	Name string `json:"name"`

	// Version is the artifact version
	Version string `json:"version"`

	// Type is the artifact type
	Type ArtifactType `json:"type"`

	// State is the current artifact state
	State ArtifactState `json:"state"`

	// Checksum is the SHA256 checksum
	Checksum string `json:"checksum,omitempty"`

	// Size is the artifact size in bytes
	Size int64 `json:"size,omitempty"`

	// StorageRef references the storage location
	StorageRef string `json:"storage_ref,omitempty"`

	// RegistryRef references the registry location (for container images)
	RegistryRef string `json:"registry_ref,omitempty"`

	// Labels contain artifact metadata
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations contain additional metadata
	Annotations map[string]string `json:"annotations,omitempty"`

	// BuildInfo contains build information
	BuildInfo *BuildInfo `json:"build_info,omitempty"`

	// CreatedAt is when the artifact was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the artifact was last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// BuildInfo contains build metadata.
type BuildInfo struct {
	// GitCommit is the git commit hash
	GitCommit string `json:"git_commit,omitempty"`

	// GitBranch is the git branch
	GitBranch string `json:"git_branch,omitempty"`

	// GitTag is the git tag (if any)
	GitTag string `json:"git_tag,omitempty"`

	// BuildNumber is the CI build number
	BuildNumber string `json:"build_number,omitempty"`

	// BuildTime is when the build occurred
	BuildTime time.Time `json:"build_time,omitempty"`

	// BuildHost is the build host
	BuildHost string `json:"build_host,omitempty"`

	// Builder is the build system (jenkins, github-actions, etc.)
	Builder string `json:"builder,omitempty"`

	// Environment is the build environment
	Environment map[string]string `json:"environment,omitempty"`
}

// ArtifactSpec defines the specification for creating an artifact.
type ArtifactSpec struct {
	// Name is the artifact name
	Name string `json:"name"`

	// Version is the artifact version
	Version string `json:"version"`

	// Type is the artifact type
	Type ArtifactType `json:"type"`

	// Labels contain artifact metadata
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations contain additional metadata
	Annotations map[string]string `json:"annotations,omitempty"`

	// BuildInfo contains build information
	BuildInfo *BuildInfo `json:"build_info,omitempty"`
}

// Manager manages artifacts.
type Manager struct {
	mu sync.RWMutex

	// artifacts stores known artifacts
	artifacts map[string]*Artifact

	// storage for artifact storage
	storage Storage

	// registry for container images
	registry Registry
}

// NewManager creates a new artifact manager.
func NewManager(storage Storage, registry Registry) *Manager {
	return &Manager{
		artifacts: make(map[string]*Artifact),
		storage:   storage,
		registry:  registry,
	}
}

// Create creates a new artifact.
func (m *Manager) Create(ctx context.Context, spec *ArtifactSpec) (*Artifact, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}

	artifact := &Artifact{
		ID:          generateArtifactID(spec.Name, spec.Version),
		Name:        spec.Name,
		Version:     spec.Version,
		Type:        spec.Type,
		State:       ArtifactStatePending,
		Labels:      spec.Labels,
		Annotations: spec.Annotations,
		BuildInfo:   spec.BuildInfo,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	m.mu.Lock()
	if _, exists := m.artifacts[artifact.ID]; exists {
		m.mu.Unlock()
		return nil, ErrArtifactExists
	}
	m.artifacts[artifact.ID] = artifact
	m.mu.Unlock()

	return artifact, nil
}

// Upload uploads artifact content.
func (m *Manager) Upload(ctx context.Context, artifactID string, reader io.Reader) error {
	m.mu.Lock()
	artifact, exists := m.artifacts[artifactID]
	if !exists {
		m.mu.Unlock()
		return ErrArtifactNotFound
	}
	artifact.State = ArtifactStateUploading
	artifact.UpdatedAt = time.Now()
	m.mu.Unlock()

	// Calculate checksum while uploading
	hash := sha256.New()
	teeReader := io.TeeReader(reader, hash)

	// Upload to storage
	storageRef, size, err := m.storage.Put(ctx, artifactID, teeReader)
	if err != nil {
		m.mu.Lock()
		artifact.State = ArtifactStateFailed
		artifact.UpdatedAt = time.Now()
		m.mu.Unlock()
		return fmt.Errorf("storage upload failed: %w", err)
	}

	checksum := hex.EncodeToString(hash.Sum(nil))

	m.mu.Lock()
	artifact.StorageRef = storageRef
	artifact.Size = size
	artifact.Checksum = checksum
	artifact.State = ArtifactStateReady
	artifact.UpdatedAt = time.Now()
	m.mu.Unlock()

	return nil
}

// Get retrieves an artifact by ID.
func (m *Manager) Get(ctx context.Context, artifactID string) (*Artifact, error) {
	m.mu.RLock()
	artifact, exists := m.artifacts[artifactID]
	m.mu.RUnlock()

	if !exists {
		return nil, ErrArtifactNotFound
	}

	return artifact, nil
}

// GetByNameVersion retrieves an artifact by name and version.
func (m *Manager) GetByNameVersion(ctx context.Context, name, version string) (*Artifact, error) {
	artifactID := generateArtifactID(name, version)
	return m.Get(ctx, artifactID)
}

// Download downloads artifact content.
func (m *Manager) Download(ctx context.Context, artifactID string) (io.ReadCloser, error) {
	m.mu.RLock()
	artifact, exists := m.artifacts[artifactID]
	m.mu.RUnlock()

	if !exists {
		return nil, ErrArtifactNotFound
	}

	if artifact.State != ArtifactStateReady {
		return nil, fmt.Errorf("artifact not ready: %s", artifact.State)
	}

	return m.storage.Get(ctx, artifact.StorageRef)
}

// Delete deletes an artifact.
func (m *Manager) Delete(ctx context.Context, artifactID string) error {
	m.mu.Lock()
	artifact, exists := m.artifacts[artifactID]
	if !exists {
		m.mu.Unlock()
		return ErrArtifactNotFound
	}
	artifact.State = ArtifactStateDeleted
	artifact.UpdatedAt = time.Now()
	m.mu.Unlock()

	// Delete from storage
	if artifact.StorageRef != "" {
		if err := m.storage.Delete(ctx, artifact.StorageRef); err != nil {
			return fmt.Errorf("storage delete failed: %w", err)
		}
	}

	// Delete from registry if container
	if artifact.RegistryRef != "" && m.registry != nil {
		if err := m.registry.Delete(ctx, artifact.RegistryRef); err != nil {
			return fmt.Errorf("registry delete failed: %w", err)
		}
	}

	m.mu.Lock()
	delete(m.artifacts, artifactID)
	m.mu.Unlock()

	return nil
}

// List lists artifacts with optional filtering.
func (m *Manager) List(ctx context.Context, filter *ArtifactFilter) ([]*Artifact, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	artifacts := make([]*Artifact, 0)
	for _, artifact := range m.artifacts {
		if filter != nil && !filter.Match(artifact) {
			continue
		}
		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

// Verify verifies artifact integrity.
func (m *Manager) Verify(ctx context.Context, artifactID string) error {
	m.mu.RLock()
	artifact, exists := m.artifacts[artifactID]
	m.mu.RUnlock()

	if !exists {
		return ErrArtifactNotFound
	}

	if artifact.StorageRef == "" {
		return fmt.Errorf("artifact has no storage reference")
	}

	// Download and verify checksum
	reader, err := m.storage.Get(ctx, artifact.StorageRef)
	if err != nil {
		return fmt.Errorf("failed to get artifact: %w", err)
	}
	defer reader.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return fmt.Errorf("failed to read artifact: %w", err)
	}

	calculatedChecksum := hex.EncodeToString(hash.Sum(nil))
	if calculatedChecksum != artifact.Checksum {
		return ErrChecksumMismatch
	}

	return nil
}

// PushToRegistry pushes a container artifact to the registry.
func (m *Manager) PushToRegistry(ctx context.Context, artifactID string, tags []string) error {
	if m.registry == nil {
		return ErrRegistryUnavailable
	}

	m.mu.RLock()
	artifact, exists := m.artifacts[artifactID]
	m.mu.RUnlock()

	if !exists {
		return ErrArtifactNotFound
	}

	if artifact.Type != ArtifactTypeContainer {
		return fmt.Errorf("artifact is not a container image")
	}

	// Download artifact
	reader, err := m.storage.Get(ctx, artifact.StorageRef)
	if err != nil {
		return fmt.Errorf("failed to get artifact: %w", err)
	}
	defer reader.Close()

	// Push to registry
	registryRef, err := m.registry.Push(ctx, artifact.Name, tags, reader)
	if err != nil {
		return fmt.Errorf("failed to push to registry: %w", err)
	}

	m.mu.Lock()
	artifact.RegistryRef = registryRef
	artifact.UpdatedAt = time.Now()
	m.mu.Unlock()

	return nil
}

// PullFromRegistry pulls a container artifact from the registry.
func (m *Manager) PullFromRegistry(ctx context.Context, name, tag string) (*Artifact, error) {
	if m.registry == nil {
		return nil, ErrRegistryUnavailable
	}

	// Check if already exists
	artifactID := generateArtifactID(name, tag)
	m.mu.RLock()
	existing, exists := m.artifacts[artifactID]
	m.mu.RUnlock()

	if exists && existing.State == ArtifactStateReady {
		return existing, nil
	}

	// Pull from registry
	reader, metadata, err := m.registry.Pull(ctx, name, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to pull from registry: %w", err)
	}
	defer reader.Close()

	// Create artifact
	artifact := &Artifact{
		ID:          artifactID,
		Name:        name,
		Version:     tag,
		Type:        ArtifactTypeContainer,
		State:       ArtifactStateUploading,
		RegistryRef: metadata.Reference,
		Labels:      metadata.Labels,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	m.mu.Lock()
	m.artifacts[artifact.ID] = artifact
	m.mu.Unlock()

	// Store locally
	hash := sha256.New()
	teeReader := io.TeeReader(reader, hash)

	storageRef, size, err := m.storage.Put(ctx, artifactID, teeReader)
	if err != nil {
		m.mu.Lock()
		artifact.State = ArtifactStateFailed
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to store artifact: %w", err)
	}

	m.mu.Lock()
	artifact.StorageRef = storageRef
	artifact.Size = size
	artifact.Checksum = hex.EncodeToString(hash.Sum(nil))
	artifact.State = ArtifactStateReady
	artifact.UpdatedAt = time.Now()
	m.mu.Unlock()

	return artifact, nil
}

// ArtifactFilter defines filtering criteria for listing artifacts.
type ArtifactFilter struct {
	Name    string            `json:"name,omitempty"`
	Version string            `json:"version,omitempty"`
	Type    ArtifactType      `json:"type,omitempty"`
	State   ArtifactState     `json:"state,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// Match checks if an artifact matches the filter.
func (f *ArtifactFilter) Match(artifact *Artifact) bool {
	if f.Name != "" && artifact.Name != f.Name {
		return false
	}
	if f.Version != "" && artifact.Version != f.Version {
		return false
	}
	if f.Type != "" && artifact.Type != f.Type {
		return false
	}
	if f.State != "" && artifact.State != f.State {
		return false
	}
	if f.Labels != nil {
		for key, value := range f.Labels {
			if artifact.Labels[key] != value {
				return false
			}
		}
	}
	return true
}

// validateSpec validates an artifact specification.
func validateSpec(spec *ArtifactSpec) error {
	if spec == nil {
		return ErrArtifactInvalid
	}
	if spec.Name == "" {
		return fmt.Errorf("%w: name is required", ErrArtifactInvalid)
	}
	if spec.Version == "" {
		return fmt.Errorf("%w: version is required", ErrArtifactInvalid)
	}
	if spec.Type == "" {
		return fmt.Errorf("%w: type is required", ErrArtifactInvalid)
	}
	return nil
}

// generateArtifactID generates an artifact ID from name and version.
func generateArtifactID(name, version string) string {
	return fmt.Sprintf("%s:%s", name, version)
}

// Storage interface for artifact storage.
type Storage interface {
	// Put stores an artifact and returns the storage reference
	Put(ctx context.Context, key string, reader io.Reader) (ref string, size int64, err error)

	// Get retrieves an artifact by reference
	Get(ctx context.Context, ref string) (io.ReadCloser, error)

	// Delete deletes an artifact by reference
	Delete(ctx context.Context, ref string) error

	// Exists checks if an artifact exists
	Exists(ctx context.Context, ref string) (bool, error)
}

// Registry interface for container registry operations.
type Registry interface {
	// Push pushes an image to the registry
	Push(ctx context.Context, name string, tags []string, reader io.Reader) (ref string, err error)

	// Pull pulls an image from the registry
	Pull(ctx context.Context, name, tag string) (reader io.ReadCloser, metadata *ImageMetadata, err error)

	// Delete deletes an image from the registry
	Delete(ctx context.Context, ref string) error

	// Tags lists tags for an image
	Tags(ctx context.Context, name string) ([]string, error)
}

// ImageMetadata contains container image metadata.
type ImageMetadata struct {
	// Reference is the full image reference
	Reference string `json:"reference"`

	// Digest is the image digest
	Digest string `json:"digest"`

	// Size is the image size
	Size int64 `json:"size"`

	// Labels contain image labels
	Labels map[string]string `json:"labels,omitempty"`

	// Created is when the image was created
	Created time.Time `json:"created"`
}
