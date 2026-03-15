// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package artifact provides artifact management for deployments.
package artifact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemoryStorage implements in-memory artifact storage.
type MemoryStorage struct {
	mu       sync.RWMutex
	data     map[string][]byte
	metadata map[string]*StorageMetadata
}

// StorageMetadata contains storage metadata.
type StorageMetadata struct {
	Key       string    `json:"key"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewMemoryStorage creates a new in-memory storage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data:     make(map[string][]byte),
		metadata: make(map[string]*StorageMetadata),
	}
}

// Put stores data and returns the reference.
func (s *MemoryStorage) Put(ctx context.Context, key string, reader io.Reader) (string, int64, error) {
	select {
	case <-ctx.Done():
		return "", 0, ctx.Err()
	default:
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read data: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = data
	s.metadata[key] = &StorageMetadata{
		Key:       key,
		Size:      int64(len(data)),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return key, int64(len(data)), nil
}

// Get retrieves data by reference.
func (s *MemoryStorage) Get(ctx context.Context, ref string) (io.ReadCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	s.mu.RLock()
	data, exists := s.data[ref]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrArtifactNotFound
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

// Delete deletes data by reference.
func (s *MemoryStorage) Delete(ctx context.Context, ref string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[ref]; !exists {
		return ErrArtifactNotFound
	}

	delete(s.data, ref)
	delete(s.metadata, ref)
	return nil
}

// Exists checks if data exists.
func (s *MemoryStorage) Exists(ctx context.Context, ref string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	s.mu.RLock()
	_, exists := s.data[ref]
	s.mu.RUnlock()

	return exists, nil
}

// FileStorage implements file-based artifact storage.
type FileStorage struct {
	basePath string
	mu       sync.RWMutex
}

// NewFileStorage creates a new file storage.
func NewFileStorage(basePath string) (*FileStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &FileStorage{
		basePath: basePath,
	}, nil
}

// Put stores data and returns the reference.
func (s *FileStorage) Put(ctx context.Context, key string, reader io.Reader) (string, int64, error) {
	select {
	case <-ctx.Done():
		return "", 0, ctx.Err()
	default:
	}

	// Sanitize key for use as filename
	safeKey := sanitizeFilename(key)
	filePath := filepath.Join(s.basePath, safeKey)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create file
	file, err := os.Create(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy data to file
	size, err := io.Copy(file, reader)
	if err != nil {
		os.Remove(filePath)
		return "", 0, fmt.Errorf("failed to write data: %w", err)
	}

	return safeKey, size, nil
}

// Get retrieves data by reference.
func (s *FileStorage) Get(ctx context.Context, ref string) (io.ReadCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	filePath := filepath.Join(s.basePath, ref)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrArtifactNotFound
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// Delete deletes data by reference.
func (s *FileStorage) Delete(ctx context.Context, ref string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	filePath := filepath.Join(s.basePath, ref)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return ErrArtifactNotFound
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// Exists checks if data exists.
func (s *FileStorage) Exists(ctx context.Context, ref string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	filePath := filepath.Join(s.basePath, ref)
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat file: %w", err)
	}

	return true, nil
}

// List lists all stored artifacts.
func (s *FileStorage) List(ctx context.Context) ([]string, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var refs []string
	err := filepath.Walk(s.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(s.basePath, path)
			refs = append(refs, relPath)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return refs, nil
}

// sanitizeFilename converts a key to a safe filename.
func sanitizeFilename(key string) string {
	// Replace problematic characters
	replacer := func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}

	safe := []rune(key)
	for i, r := range safe {
		safe[i] = replacer(r)
	}

	return string(safe)
}

// CachingStorage wraps a storage with a cache layer.
type CachingStorage struct {
	backend Storage
	cache   *MemoryStorage
	maxSize int64
	mu      sync.RWMutex
	sizes   map[string]int64
	total   int64
}

// NewCachingStorage creates a new caching storage.
func NewCachingStorage(backend Storage, maxCacheSize int64) *CachingStorage {
	return &CachingStorage{
		backend: backend,
		cache:   NewMemoryStorage(),
		maxSize: maxCacheSize,
		sizes:   make(map[string]int64),
	}
}

// Put stores data in the backend and caches if small enough.
func (s *CachingStorage) Put(ctx context.Context, key string, reader io.Reader) (string, int64, error) {
	// Read data to buffer
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", 0, err
	}

	// Store in backend
	ref, size, err := s.backend.Put(ctx, key, bytes.NewReader(data))
	if err != nil {
		return "", 0, err
	}

	// Cache if small enough
	if size < s.maxSize/10 { // Cache items up to 10% of max cache size
		s.mu.Lock()
		// Evict if necessary
		for s.total+size > s.maxSize && len(s.sizes) > 0 {
			// Simple eviction: remove first item
			for evictKey := range s.sizes {
				s.cache.Delete(ctx, evictKey)
				s.total -= s.sizes[evictKey]
				delete(s.sizes, evictKey)
				break
			}
		}

		s.cache.Put(ctx, ref, bytes.NewReader(data))
		s.sizes[ref] = size
		s.total += size
		s.mu.Unlock()
	}

	return ref, size, nil
}

// Get retrieves data from cache or backend.
func (s *CachingStorage) Get(ctx context.Context, ref string) (io.ReadCloser, error) {
	// Try cache first
	s.mu.RLock()
	_, isCached := s.sizes[ref]
	s.mu.RUnlock()

	if isCached {
		reader, err := s.cache.Get(ctx, ref)
		if err == nil {
			return reader, nil
		}
	}

	// Fall back to backend
	return s.backend.Get(ctx, ref)
}

// Delete deletes from both cache and backend.
func (s *CachingStorage) Delete(ctx context.Context, ref string) error {
	// Remove from cache
	s.mu.Lock()
	if size, cached := s.sizes[ref]; cached {
		s.cache.Delete(ctx, ref)
		s.total -= size
		delete(s.sizes, ref)
	}
	s.mu.Unlock()

	// Delete from backend
	return s.backend.Delete(ctx, ref)
}

// Exists checks if data exists.
func (s *CachingStorage) Exists(ctx context.Context, ref string) (bool, error) {
	// Check cache first
	s.mu.RLock()
	_, isCached := s.sizes[ref]
	s.mu.RUnlock()

	if isCached {
		return true, nil
	}

	// Check backend
	return s.backend.Exists(ctx, ref)
}

// ReplicatedStorage replicates writes to multiple storage backends.
type ReplicatedStorage struct {
	primary   Storage
	replicas  []Storage
	mu        sync.RWMutex
}

// NewReplicatedStorage creates a new replicated storage.
func NewReplicatedStorage(primary Storage, replicas ...Storage) *ReplicatedStorage {
	return &ReplicatedStorage{
		primary:  primary,
		replicas: replicas,
	}
}

// Put stores data in all backends.
func (s *ReplicatedStorage) Put(ctx context.Context, key string, reader io.Reader) (string, int64, error) {
	// Read data once
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", 0, err
	}

	// Write to primary
	ref, size, err := s.primary.Put(ctx, key, bytes.NewReader(data))
	if err != nil {
		return "", 0, err
	}

	// Replicate to all replicas asynchronously
	for _, replica := range s.replicas {
		go func(r Storage) {
			r.Put(ctx, key, bytes.NewReader(data))
		}(replica)
	}

	return ref, size, nil
}

// Get retrieves data from primary or replicas.
func (s *ReplicatedStorage) Get(ctx context.Context, ref string) (io.ReadCloser, error) {
	// Try primary first
	reader, err := s.primary.Get(ctx, ref)
	if err == nil {
		return reader, nil
	}

	// Try replicas
	for _, replica := range s.replicas {
		reader, err := replica.Get(ctx, ref)
		if err == nil {
			return reader, nil
		}
	}

	return nil, ErrArtifactNotFound
}

// Delete deletes from all backends.
func (s *ReplicatedStorage) Delete(ctx context.Context, ref string) error {
	// Delete from primary
	err := s.primary.Delete(ctx, ref)

	// Delete from replicas
	for _, replica := range s.replicas {
		replica.Delete(ctx, ref)
	}

	return err
}

// Exists checks if data exists in primary.
func (s *ReplicatedStorage) Exists(ctx context.Context, ref string) (bool, error) {
	return s.primary.Exists(ctx, ref)
}
