// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package artifact provides artifact management for deployments.
package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RegistryConfig contains registry configuration.
type RegistryConfig struct {
	// URL is the registry URL
	URL string `json:"url"`

	// Username for authentication
	Username string `json:"username,omitempty"`

	// Password for authentication
	Password string `json:"password,omitempty"`

	// Insecure allows insecure connections
	Insecure bool `json:"insecure,omitempty"`

	// Timeout for registry operations
	Timeout time.Duration `json:"timeout,omitempty"`
}

// ContainerRegistry implements the Registry interface for OCI-compatible registries.
type ContainerRegistry struct {
	config     *RegistryConfig
	httpClient *http.Client
	mu         sync.RWMutex

	// Cache for image metadata
	metadataCache map[string]*ImageMetadata
}

// NewContainerRegistry creates a new container registry client.
func NewContainerRegistry(config *RegistryConfig) *ContainerRegistry {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &ContainerRegistry{
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		metadataCache: make(map[string]*ImageMetadata),
	}
}

// Push pushes an image to the registry.
func (r *ContainerRegistry) Push(ctx context.Context, name string, tags []string, reader io.Reader) (string, error) {
	// This is a simplified implementation
	// A real implementation would use OCI distribution spec

	// Read the content
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read image content: %w", err)
	}

	// For each tag, push to registry
	for _, tag := range tags {
		ref := fmt.Sprintf("%s/%s:%s", r.config.URL, name, tag)

		req, err := http.NewRequestWithContext(ctx, "PUT", ref, strings.NewReader(string(content)))
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		if r.config.Username != "" {
			req.SetBasicAuth(r.config.Username, r.config.Password)
		}
		req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")

		resp, err := r.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to push to registry: %w", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return "", fmt.Errorf("registry returned status %d", resp.StatusCode)
		}
	}

	// Return the first tag reference
	ref := fmt.Sprintf("%s/%s:%s", r.config.URL, name, tags[0])
	return ref, nil
}

// Pull pulls an image from the registry.
func (r *ContainerRegistry) Pull(ctx context.Context, name, tag string) (io.ReadCloser, *ImageMetadata, error) {
	ref := fmt.Sprintf("%s/%s:%s", r.config.URL, name, tag)

	// Check cache
	r.mu.RLock()
	metadata, cached := r.metadataCache[ref]
	r.mu.RUnlock()

	if !cached {
		// Fetch metadata
		var err error
		metadata, err = r.fetchMetadata(ctx, name, tag)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch metadata: %w", err)
		}

		r.mu.Lock()
		r.metadataCache[ref] = metadata
		r.mu.Unlock()
	}

	// Pull the image content
	req, err := http.NewRequestWithContext(ctx, "GET", ref, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	if r.config.Username != "" {
		req.SetBasicAuth(r.config.Username, r.config.Password)
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to pull from registry: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	return resp.Body, metadata, nil
}

// Delete deletes an image from the registry.
func (r *ContainerRegistry) Delete(ctx context.Context, ref string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", ref, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if r.config.Username != "" {
		req.SetBasicAuth(r.config.Username, r.config.Password)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete from registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	// Remove from cache
	r.mu.Lock()
	delete(r.metadataCache, ref)
	r.mu.Unlock()

	return nil
}

// Tags lists tags for an image.
func (r *ContainerRegistry) Tags(ctx context.Context, name string) ([]string, error) {
	url := fmt.Sprintf("%s/v2/%s/tags/list", r.config.URL, name)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if r.config.Username != "" {
		req.SetBasicAuth(r.config.Username, r.config.Password)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var result struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Tags, nil
}

// fetchMetadata fetches image metadata from the registry.
func (r *ContainerRegistry) fetchMetadata(ctx context.Context, name, tag string) (*ImageMetadata, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", r.config.URL, name, tag)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if r.config.Username != "" {
		req.SetBasicAuth(r.config.Username, r.config.Password)
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	// Parse manifest
	var manifest struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		Config        struct {
			MediaType string `json:"mediaType"`
			Size      int64  `json:"size"`
			Digest    string `json:"digest"`
		} `json:"config"`
		Layers []struct {
			MediaType string `json:"mediaType"`
			Size      int64  `json:"size"`
			Digest    string `json:"digest"`
		} `json:"layers"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	// Calculate total size
	var totalSize int64
	totalSize += manifest.Config.Size
	for _, layer := range manifest.Layers {
		totalSize += layer.Size
	}

	metadata := &ImageMetadata{
		Reference: fmt.Sprintf("%s/%s:%s", r.config.URL, name, tag),
		Digest:    resp.Header.Get("Docker-Content-Digest"),
		Size:      totalSize,
		Created:   time.Now(), // Would parse from config in real implementation
	}

	return metadata, nil
}

// LocalRegistry implements a local in-memory registry for testing.
type LocalRegistry struct {
	mu     sync.RWMutex
	images map[string]*localImage
}

type localImage struct {
	content  []byte
	metadata *ImageMetadata
	tags     []string
}

// NewLocalRegistry creates a new local registry.
func NewLocalRegistry() *LocalRegistry {
	return &LocalRegistry{
		images: make(map[string]*localImage),
	}
}

// Push pushes an image to the local registry.
func (r *LocalRegistry) Push(ctx context.Context, name string, tags []string, reader io.Reader) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read content: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, tag := range tags {
		ref := fmt.Sprintf("%s:%s", name, tag)
		r.images[ref] = &localImage{
			content: content,
			metadata: &ImageMetadata{
				Reference: ref,
				Size:      int64(len(content)),
				Created:   time.Now(),
			},
			tags: tags,
		}
	}

	return fmt.Sprintf("%s:%s", name, tags[0]), nil
}

// Pull pulls an image from the local registry.
func (r *LocalRegistry) Pull(ctx context.Context, name, tag string) (io.ReadCloser, *ImageMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ref := fmt.Sprintf("%s:%s", name, tag)
	image, exists := r.images[ref]
	if !exists {
		return nil, nil, ErrArtifactNotFound
	}

	return io.NopCloser(strings.NewReader(string(image.content))), image.metadata, nil
}

// Delete deletes an image from the local registry.
func (r *LocalRegistry) Delete(ctx context.Context, ref string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.images[ref]; !exists {
		return ErrArtifactNotFound
	}

	delete(r.images, ref)
	return nil
}

// Tags lists tags for an image in the local registry.
func (r *LocalRegistry) Tags(ctx context.Context, name string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tags := make([]string, 0)
	prefix := name + ":"
	for ref := range r.images {
		if strings.HasPrefix(ref, prefix) {
			tag := strings.TrimPrefix(ref, prefix)
			tags = append(tags, tag)
		}
	}

	return tags, nil
}

// RegistryMirror provides a caching layer for registry pulls.
type RegistryMirror struct {
	upstream Registry
	cache    Storage
	mu       sync.RWMutex
	cached   map[string]bool
}

// NewRegistryMirror creates a new registry mirror.
func NewRegistryMirror(upstream Registry, cache Storage) *RegistryMirror {
	return &RegistryMirror{
		upstream: upstream,
		cache:    cache,
		cached:   make(map[string]bool),
	}
}

// Push pushes to the upstream registry.
func (m *RegistryMirror) Push(ctx context.Context, name string, tags []string, reader io.Reader) (string, error) {
	return m.upstream.Push(ctx, name, tags, reader)
}

// Pull pulls from cache or upstream.
func (m *RegistryMirror) Pull(ctx context.Context, name, tag string) (io.ReadCloser, *ImageMetadata, error) {
	ref := fmt.Sprintf("%s:%s", name, tag)

	// Check cache
	m.mu.RLock()
	isCached := m.cached[ref]
	m.mu.RUnlock()

	if isCached {
		reader, err := m.cache.Get(ctx, ref)
		if err == nil {
			// Return cached content with minimal metadata
			return reader, &ImageMetadata{Reference: ref}, nil
		}
		// Cache miss, fall through to upstream
	}

	// Pull from upstream
	reader, metadata, err := m.upstream.Pull(ctx, name, tag)
	if err != nil {
		return nil, nil, err
	}

	// Cache the content
	content, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read content: %w", err)
	}

	// Store in cache
	if _, _, err := m.cache.Put(ctx, ref, strings.NewReader(string(content))); err == nil {
		m.mu.Lock()
		m.cached[ref] = true
		m.mu.Unlock()
	}

	return io.NopCloser(strings.NewReader(string(content))), metadata, nil
}

// Delete deletes from both cache and upstream.
func (m *RegistryMirror) Delete(ctx context.Context, ref string) error {
	// Delete from cache
	m.cache.Delete(ctx, ref)
	m.mu.Lock()
	delete(m.cached, ref)
	m.mu.Unlock()

	// Delete from upstream
	return m.upstream.Delete(ctx, ref)
}

// Tags lists tags from the upstream.
func (m *RegistryMirror) Tags(ctx context.Context, name string) ([]string, error) {
	return m.upstream.Tags(ctx, name)
}
