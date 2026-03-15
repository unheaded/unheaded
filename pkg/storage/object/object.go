// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package object provides object storage implementations for the Kingdom.
// This package supports filesystem and S3-compatible backends for storing
// large binary objects (blobs).
package object

import (
	"context"
	"fmt"
	"io"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/storage"
)

// Backend represents an object storage backend type.
type Backend string

const (
	// BackendFilesystem is the filesystem storage backend.
	BackendFilesystem Backend = "filesystem"
	// BackendS3 is the S3-compatible storage backend.
	BackendS3 Backend = "s3"
)

// Config holds object store configuration.
type Config struct {
	// Backend is the storage backend to use.
	Backend Backend `json:"backend" yaml:"backend"`

	// Path is the root directory for filesystem backend.
	Path string `json:"path" yaml:"path"`

	// S3Config holds S3-specific configuration.
	S3Config S3Config `json:"s3" yaml:"s3"`

	// MaxObjectSize is the maximum object size in bytes.
	MaxObjectSize int64 `json:"max_object_size" yaml:"max_object_size"`

	// Logger is the logger instance.
	Logger *logger.Logger

	// Metrics is the metrics registry.
	Metrics *metrics.Registry
}

// S3Config holds S3-specific configuration.
type S3Config struct {
	// Endpoint is the S3 endpoint URL.
	Endpoint string `json:"endpoint" yaml:"endpoint"`

	// Region is the AWS region.
	Region string `json:"region" yaml:"region"`

	// Bucket is the default bucket name.
	Bucket string `json:"bucket" yaml:"bucket"`

	// AccessKey is the access key ID.
	AccessKey string `json:"access_key" yaml:"access_key"`

	// SecretKey is the secret access key.
	SecretKey string `json:"secret_key" yaml:"secret_key"`

	// UsePathStyle enables path-style bucket addressing.
	UsePathStyle bool `json:"use_path_style" yaml:"use_path_style"`

	// DisableSSL disables SSL/TLS.
	DisableSSL bool `json:"disable_ssl" yaml:"disable_ssl"`
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Backend:       BackendFilesystem,
		Path:          "/var/lib/unheaded/objects",
		MaxObjectSize: 5 * 1024 * 1024 * 1024, // 5GB
	}
}

// New creates a new object store based on configuration.
func New(cfg Config) (storage.ObjectStore, error) {
	switch cfg.Backend {
	case BackendFilesystem:
		return NewFilesystemStore(cfg)
	case BackendS3:
		return NewS3Store(cfg)
	default:
		return nil, fmt.Errorf("object: unknown backend: %s", cfg.Backend)
	}
}

// Metrics for object operations.
var (
	objectOpsTotal = metrics.NewCounterVec(
		"unheaded_object_ops_total",
		"Total object operations",
		nil,
		[]string{"backend", "operation", "status"},
	)

	objectOpsDuration = metrics.NewHistogramVec(
		metrics.HistogramOpts{
			Name:    "unheaded_object_ops_duration_seconds",
			Help:    "Object operation duration",
			Buckets: metrics.ExponentialBuckets(0.001, 2, 14),
		},
		[]string{"backend", "operation"},
	)

	objectBytesTransferred = metrics.NewCounterVec(
		"unheaded_object_bytes_total",
		"Total bytes transferred",
		nil,
		[]string{"backend", "direction"},
	)

	objectSizeBytes = metrics.NewHistogramVec(
		metrics.HistogramOpts{
			Name:    "unheaded_object_size_bytes",
			Help:    "Object size distribution",
			Buckets: metrics.ExponentialBuckets(1024, 4, 10),
		},
		[]string{"backend"},
	)
)

// recordOp records an object operation for metrics.
func recordOp(backend, operation, status string, duration float64) {
	objectOpsTotal.WithLabels(metrics.Labels{
		"backend":   backend,
		"operation": operation,
		"status":    status,
	}).Inc()

	objectOpsDuration.WithLabels(metrics.Labels{
		"backend":   backend,
		"operation": operation,
	}).Observe(duration)
}

// recordTransfer records bytes transferred.
func recordTransfer(backend, direction string, bytes int64) {
	objectBytesTransferred.WithLabels(metrics.Labels{
		"backend":   backend,
		"direction": direction,
	}).Add(float64(bytes))
}

// recordSize records object size.
func recordSize(backend string, size int64) {
	objectSizeBytes.WithLabels(metrics.Labels{
		"backend": backend,
	}).Observe(float64(size))
}

// ObjectInfo contains object metadata.
type ObjectInfo = storage.ObjectInfo

// PutOptions configures object put operations.
type PutOptions = storage.PutOptions

// LimitedReader wraps a reader with a size limit.
type LimitedReader struct {
	R io.Reader
	N int64
}

// Read reads up to the limit.
func (l *LimitedReader) Read(p []byte) (n int, err error) {
	if l.N <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > l.N {
		p = p[0:l.N]
	}
	n, err = l.R.Read(p)
	l.N -= int64(n)
	return
}

// ProgressReader wraps a reader to track progress.
type ProgressReader struct {
	R         io.Reader
	Size      int64
	BytesRead int64
	OnUpdate  func(read, total int64)
}

// Read reads and reports progress.
func (p *ProgressReader) Read(buf []byte) (n int, err error) {
	n, err = p.R.Read(buf)
	p.BytesRead += int64(n)
	if p.OnUpdate != nil {
		p.OnUpdate(p.BytesRead, p.Size)
	}
	return
}

// MultipartUpload represents an in-progress multipart upload.
type MultipartUpload struct {
	// ID is the upload identifier.
	ID string
	// Bucket is the target bucket.
	Bucket string
	// Key is the object key.
	Key string
	// Parts is the list of uploaded parts.
	Parts []Part
	// StartTime is when the upload started.
	StartTime time.Time
}

// Part represents a single part of a multipart upload.
type Part struct {
	// Number is the part number (1-based).
	Number int
	// ETag is the part's entity tag.
	ETag string
	// Size is the part size in bytes.
	Size int64
}

// MultipartUploader provides multipart upload capabilities.
type MultipartUploader interface {
	// CreateMultipartUpload initiates a multipart upload.
	CreateMultipartUpload(ctx context.Context, bucket, key string, opts *PutOptions) (*MultipartUpload, error)

	// UploadPart uploads a single part.
	UploadPart(ctx context.Context, upload *MultipartUpload, partNumber int, data io.Reader) (*Part, error)

	// CompleteMultipartUpload finishes the upload.
	CompleteMultipartUpload(ctx context.Context, upload *MultipartUpload) (*ObjectInfo, error)

	// AbortMultipartUpload cancels the upload.
	AbortMultipartUpload(ctx context.Context, upload *MultipartUpload) error
}

// Copier provides copy capabilities.
type Copier interface {
	// Copy copies an object within the store.
	Copy(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) (*ObjectInfo, error)
}

// Versioning provides object versioning capabilities.
type Versioning interface {
	// ListVersions lists all versions of an object.
	ListVersions(ctx context.Context, bucket, key string) ([]*ObjectInfo, error)

	// GetVersion retrieves a specific version.
	GetVersion(ctx context.Context, bucket, key, versionID string) (io.ReadCloser, *ObjectInfo, error)

	// DeleteVersion deletes a specific version.
	DeleteVersion(ctx context.Context, bucket, key, versionID string) error
}

// Presigner provides presigned URL capabilities.
type Presigner interface {
	// PresignGet generates a presigned URL for downloading.
	PresignGet(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)

	// PresignPut generates a presigned URL for uploading.
	PresignPut(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// ObjectWalker walks objects in a bucket.
type ObjectWalker interface {
	// Walk walks objects with a callback.
	Walk(ctx context.Context, bucket, prefix string, fn func(info *ObjectInfo) error) error
}

// ValidateKey validates an object key.
func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("object: empty key")
	}
	if len(key) > 1024 {
		return fmt.Errorf("object: key too long (max 1024)")
	}
	return nil
}

// ValidateBucket validates a bucket name.
func ValidateBucket(bucket string) error {
	if bucket == "" {
		return fmt.Errorf("object: empty bucket name")
	}
	if len(bucket) < 3 || len(bucket) > 63 {
		return fmt.Errorf("object: bucket name must be 3-63 characters")
	}
	return nil
}

// GenerateETag generates an ETag from content.
func GenerateETag(data []byte) string {
	// In production, use MD5 or similar
	return fmt.Sprintf("%x", len(data))
}

// ContentTypeFromExtension returns a content type based on file extension.
func ContentTypeFromExtension(key string) string {
	// Basic content type detection
	switch {
	case hasExtension(key, ".json"):
		return "application/json"
	case hasExtension(key, ".xml"):
		return "application/xml"
	case hasExtension(key, ".html", ".htm"):
		return "text/html"
	case hasExtension(key, ".css"):
		return "text/css"
	case hasExtension(key, ".js"):
		return "application/javascript"
	case hasExtension(key, ".txt"):
		return "text/plain"
	case hasExtension(key, ".png"):
		return "image/png"
	case hasExtension(key, ".jpg", ".jpeg"):
		return "image/jpeg"
	case hasExtension(key, ".gif"):
		return "image/gif"
	case hasExtension(key, ".svg"):
		return "image/svg+xml"
	case hasExtension(key, ".pdf"):
		return "application/pdf"
	case hasExtension(key, ".zip"):
		return "application/zip"
	case hasExtension(key, ".gz", ".gzip"):
		return "application/gzip"
	case hasExtension(key, ".tar"):
		return "application/x-tar"
	default:
		return "application/octet-stream"
	}
}

func hasExtension(key string, exts ...string) bool {
	for _, ext := range exts {
		if len(key) > len(ext) && key[len(key)-len(ext):] == ext {
			return true
		}
	}
	return false
}
