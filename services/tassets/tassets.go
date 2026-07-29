// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package tassets provides data layer and storage management capabilities.
// Tassets the Data Layer is the Kingdom's foundation - storing all data securely.
// Implements data pipelines, storage backends, and backup/recovery.
package tassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// Object represents a stored object.
type Object struct {
	ID          string            `json:"id"`
	Bucket      string            `json:"bucket"`
	Key         string            `json:"key"`
	Size        int64             `json:"size"`
	ContentType string            `json:"content_type"`
	Checksum    string            `json:"checksum"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Version     int64             `json:"version"`
}

// Bucket represents a storage bucket.
type Bucket struct {
	Name         string            `json:"name"`
	Region       string            `json:"region,omitempty"`
	StorageClass StorageClass      `json:"storage_class"`
	Versioning   bool              `json:"versioning"`
	Encryption   bool              `json:"encryption"`
	Tags         map[string]string `json:"tags,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	ObjectCount  int64             `json:"object_count"`
	TotalSize    int64             `json:"total_size"`
}

// StorageClass defines storage tier.
type StorageClass string

const (
	StorageStandard   StorageClass = "standard"
	StorageInfrequent StorageClass = "infrequent"
	StorageArchive    StorageClass = "archive"
	StorageGlacier    StorageClass = "glacier"
)

// Backup represents a backup operation.
type Backup struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Source        string       `json:"source"`
	Destination   string       `json:"destination"`
	Type          BackupType   `json:"type"`
	Status        BackupStatus `json:"status"`
	Size          int64        `json:"size"`
	ObjectCount   int64        `json:"object_count"`
	StartedAt     *time.Time   `json:"started_at,omitempty"`
	CompletedAt   *time.Time   `json:"completed_at,omitempty"`
	Error         string       `json:"error,omitempty"`
	RetentionDays int          `json:"retention_days"`
}

// BackupType defines backup approach.
type BackupType string

const (
	BackupFull         BackupType = "full"
	BackupIncremental  BackupType = "incremental"
	BackupDifferential BackupType = "differential"
)

// BackupStatus tracks backup progress.
type BackupStatus string

const (
	BackupPending   BackupStatus = "pending"
	BackupRunning   BackupStatus = "running"
	BackupCompleted BackupStatus = "completed"
	BackupFailed    BackupStatus = "failed"
)

// Pipeline represents a data pipeline.
type Pipeline struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Source      DataSource     `json:"source"`
	Destination DataSource     `json:"destination"`
	Transforms  []Transform    `json:"transforms,omitempty"`
	Schedule    string         `json:"schedule,omitempty"`
	Status      PipelineStatus `json:"status"`
	LastRun     *time.Time     `json:"last_run,omitempty"`
}

// DataSource represents a data source/destination.
type DataSource struct {
	Type       string            `json:"type"` // s3, gcs, local, db
	Connection string            `json:"connection"`
	Config     map[string]string `json:"config,omitempty"`
}

// Transform represents a data transformation.
type Transform struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"` // filter, map, aggregate
	Config map[string]string `json:"config,omitempty"`
}

// PipelineStatus tracks pipeline state.
type PipelineStatus string

const (
	PipelineActive   PipelineStatus = "active"
	PipelinePaused   PipelineStatus = "paused"
	PipelineDisabled PipelineStatus = "disabled"
)

// Service is the main Tassets data layer service.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu        sync.RWMutex
	buckets   map[string]*Bucket
	objects   map[string]map[string]*Object // bucket -> key -> object
	backups   map[string]*Backup
	pipelines map[string]*Pipeline
	data      map[string][]byte // Simplified in-memory storage
}

// Config holds Tassets service configuration.
type Config struct {
	DefaultStorageClass StorageClass `json:"default_storage_class"`
	MaxObjectSize       int64        `json:"max_object_size"`
	DefaultRetention    int          `json:"default_retention_days"`
	EnableEncryption    bool         `json:"enable_encryption"`
	WotanTopic          string       `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DefaultStorageClass: StorageStandard,
		MaxObjectSize:       5 * 1024 * 1024 * 1024, // 5GB
		DefaultRetention:    30,
		EnableEncryption:    true,
		WotanTopic:          "tassets.storage",
	}
}

// NewService creates a new Tassets data layer service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:       log,
		wotan:     wotan,
		config:    cfg,
		buckets:   make(map[string]*Bucket),
		objects:   make(map[string]map[string]*Object),
		backups:   make(map[string]*Backup),
		pipelines: make(map[string]*Pipeline),
		data:      make(map[string][]byte),
	}
}

// Start initializes the Tassets service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Tassets equipped - data layer active")
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Tassets removed - data layer suspended")
	return nil
}

// CreateBucket creates a new storage bucket.
func (s *Service) CreateBucket(bucket *Bucket) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if bucket.Name == "" {
		return fmt.Errorf("bucket name is required")
	}

	if _, ok := s.buckets[bucket.Name]; ok {
		return fmt.Errorf("bucket already exists: %s", bucket.Name)
	}

	bucket.CreatedAt = time.Now()
	if bucket.StorageClass == "" {
		bucket.StorageClass = s.config.DefaultStorageClass
	}
	bucket.Encryption = s.config.EnableEncryption

	s.buckets[bucket.Name] = bucket
	s.objects[bucket.Name] = make(map[string]*Object)

	s.log.Info().Str("bucket", bucket.Name).Msg("Bucket created")
	return nil
}

// GetBucket retrieves a bucket by name.
func (s *Service) GetBucket(name string) (*Bucket, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket, ok := s.buckets[name]
	return bucket, ok
}

// ListBuckets returns all buckets.
func (s *Service) ListBuckets() []*Bucket {
	s.mu.RLock()
	defer s.mu.RUnlock()

	buckets := make([]*Bucket, 0, len(s.buckets))
	for _, b := range s.buckets {
		buckets = append(buckets, b)
	}
	return buckets
}

// PutObject stores an object.
func (s *Service) PutObject(bucket, key string, data []byte, metadata map[string]string) (*Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buckets[bucket]
	if !ok {
		return nil, fmt.Errorf("bucket not found: %s", bucket)
	}

	if int64(len(data)) > s.config.MaxObjectSize {
		return nil, fmt.Errorf("object size exceeds maximum: %d", s.config.MaxObjectSize)
	}

	// Compute checksum
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])

	now := time.Now()
	version := int64(1)

	// Check if object exists (versioning)
	if existing, ok := s.objects[bucket][key]; ok {
		if b.Versioning {
			version = existing.Version + 1
		}
	}

	obj := &Object{
		ID:        fmt.Sprintf("%s/%s", bucket, key),
		Bucket:    bucket,
		Key:       key,
		Size:      int64(len(data)),
		Checksum:  checksum,
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   version,
	}

	s.objects[bucket][key] = obj
	s.data[obj.ID] = data

	// Update bucket stats
	b.ObjectCount++
	b.TotalSize += obj.Size

	s.log.Debug().Str("bucket", bucket).Str("key", key).Int64("size", obj.Size).Msg("Object stored")
	return obj, nil
}

// GetObject retrieves an object.
func (s *Service) GetObject(bucket, key string) ([]byte, *Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.buckets[bucket]; !ok {
		return nil, nil, fmt.Errorf("bucket not found: %s", bucket)
	}

	obj, ok := s.objects[bucket][key]
	if !ok {
		return nil, nil, fmt.Errorf("object not found: %s/%s", bucket, key)
	}

	data := s.data[obj.ID]
	return data, obj, nil
}

// DeleteObject removes an object.
func (s *Service) DeleteObject(bucket, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buckets[bucket]
	if !ok {
		return fmt.Errorf("bucket not found: %s", bucket)
	}

	obj, ok := s.objects[bucket][key]
	if !ok {
		return fmt.Errorf("object not found: %s/%s", bucket, key)
	}

	delete(s.objects[bucket], key)
	delete(s.data, obj.ID)

	b.ObjectCount--
	b.TotalSize -= obj.Size

	s.log.Debug().Str("bucket", bucket).Str("key", key).Msg("Object deleted")
	return nil
}

// ListObjects returns objects in a bucket.
func (s *Service) ListObjects(bucket, prefix string, limit int) ([]*Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.buckets[bucket]; !ok {
		return nil, fmt.Errorf("bucket not found: %s", bucket)
	}

	var objects []*Object
	for key, obj := range s.objects[bucket] {
		if prefix == "" || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			objects = append(objects, obj)
			if limit > 0 && len(objects) >= limit {
				break
			}
		}
	}

	return objects, nil
}

// CopyObject copies an object.
func (s *Service) CopyObject(srcBucket, srcKey, dstBucket, dstKey string) (*Object, error) {
	data, srcObj, err := s.GetObject(srcBucket, srcKey)
	if err != nil {
		return nil, err
	}

	return s.PutObject(dstBucket, dstKey, data, srcObj.Metadata)
}

// CreateBackup creates a backup job.
func (s *Service) CreateBackup(backup *Backup) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if backup.ID == "" {
		backup.ID = fmt.Sprintf("backup-%d", time.Now().UnixNano())
	}
	backup.Status = BackupPending
	if backup.RetentionDays == 0 {
		backup.RetentionDays = s.config.DefaultRetention
	}

	s.backups[backup.ID] = backup

	s.log.Info().Str("id", backup.ID).Str("source", backup.Source).Msg("Backup created")
	return nil
}

// GetBackup retrieves a backup by ID.
func (s *Service) GetBackup(backupID string) (*Backup, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	backup, ok := s.backups[backupID]
	return backup, ok
}

// CreatePipeline creates a data pipeline.
func (s *Service) CreatePipeline(pipeline *Pipeline) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pipeline.ID == "" {
		pipeline.ID = fmt.Sprintf("pipeline-%d", time.Now().UnixNano())
	}
	pipeline.Status = PipelineActive

	s.pipelines[pipeline.ID] = pipeline

	s.log.Info().Str("id", pipeline.ID).Str("name", pipeline.Name).Msg("Pipeline created")
	return nil
}

// GetPipeline retrieves a pipeline by ID.
func (s *Service) GetPipeline(pipelineID string) (*Pipeline, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pipeline, ok := s.pipelines[pipelineID]
	return pipeline, ok
}

// StreamObject reads an object in chunks.
func (s *Service) StreamObject(bucket, key string, writer io.Writer) error {
	data, _, err := s.GetObject(bucket, key)
	if err != nil {
		return err
	}

	_, err = writer.Write(data)
	return err
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalObjects := 0
	totalSize := int64(0)
	for _, b := range s.buckets {
		totalObjects += int(b.ObjectCount)
		totalSize += b.TotalSize
	}

	return map[string]interface{}{
		"total_buckets":   len(s.buckets),
		"total_objects":   totalObjects,
		"total_size":      totalSize,
		"total_backups":   len(s.backups),
		"total_pipelines": len(s.pipelines),
	}
}

// RegisterRoutes registers /health, /ready, and /metrics endpoints on the provided mux.
// Required by CLAUDE.md: every component must expose these endpoints.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"status":  "healthy",
			"service": "tassets",
		})
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"ready":   true,
			"service": "tassets",
		})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := s.Stats()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP tassets_buckets_total Total storage buckets\n")
		fmt.Fprintf(w, "# TYPE tassets_buckets_total gauge\n")
		fmt.Fprintf(w, "tassets_buckets_total %v\n", stats["total_buckets"])
		fmt.Fprintf(w, "# HELP tassets_objects_total Total stored objects\n")
		fmt.Fprintf(w, "# TYPE tassets_objects_total gauge\n")
		fmt.Fprintf(w, "tassets_objects_total %v\n", stats["total_objects"])
		fmt.Fprintf(w, "# HELP tassets_storage_bytes Total storage size in bytes\n")
		fmt.Fprintf(w, "# TYPE tassets_storage_bytes gauge\n")
		fmt.Fprintf(w, "tassets_storage_bytes %v\n", stats["total_size"])
		fmt.Fprintf(w, "# HELP tassets_backups_total Total backups\n")
		fmt.Fprintf(w, "# TYPE tassets_backups_total gauge\n")
		fmt.Fprintf(w, "tassets_backups_total %v\n", stats["total_backups"])
	})
}
