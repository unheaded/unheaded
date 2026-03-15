// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package object

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/storage"
)

// FilesystemStore is a filesystem-backed object store.
type FilesystemStore struct {
	cfg    Config
	log    *logger.Logger
	root   string
	mu     sync.RWMutex
	closed bool
}

// NewFilesystemStore creates a new filesystem-backed object store.
func NewFilesystemStore(cfg Config) (*FilesystemStore, error) {
	log := cfg.Logger
	if log == nil {
		log = logger.New(os.Stdout)
	}

	root := cfg.Path
	if root == "" {
		root = "/var/lib/unheaded/objects"
	}

	// Create root directory
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("filesystem: create root directory: %w", err)
	}

	store := &FilesystemStore{
		cfg:  cfg,
		log:  log.With().Str("backend", "filesystem").Logger(),
		root: root,
	}

	store.log.Info().Str("path", root).Msg("filesystem object store initialized")
	return store, nil
}

// Put stores an object in the bucket.
func (f *FilesystemStore) Put(ctx context.Context, bucket, key string, reader io.Reader, opts *storage.PutOptions) (*storage.ObjectInfo, error) {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, err
	}
	if err := ValidateKey(key); err != nil {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	// Ensure bucket directory exists
	bucketPath := filepath.Join(f.root, bucket)
	if err := os.MkdirAll(bucketPath, 0755); err != nil {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("filesystem: create bucket directory: %w", err)
	}

	// Create object path (handle nested keys)
	objectPath := filepath.Join(bucketPath, key)
	objectDir := filepath.Dir(objectPath)
	if err := os.MkdirAll(objectDir, 0755); err != nil {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("filesystem: create object directory: %w", err)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp(objectDir, ".tmp-*")
	if err != nil {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("filesystem: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}()

	// Copy data and calculate MD5
	hash := md5.New()
	writer := io.MultiWriter(tmpFile, hash)

	size, err := io.Copy(writer, reader)
	if err != nil {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("filesystem: write object: %w", err)
	}

	// Check size limit
	if f.cfg.MaxObjectSize > 0 && size > f.cfg.MaxObjectSize {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, storage.ErrValueTooLarge
	}

	// Close temp file before rename
	if err := tmpFile.Close(); err != nil {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("filesystem: close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, objectPath); err != nil {
		recordOp("filesystem", "put", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("filesystem: rename object: %w", err)
	}

	// Store metadata
	etag := hex.EncodeToString(hash.Sum(nil))
	contentType := "application/octet-stream"
	if opts != nil && opts.ContentType != "" {
		contentType = opts.ContentType
	} else {
		contentType = ContentTypeFromExtension(key)
	}

	info := &storage.ObjectInfo{
		Key:          key,
		Size:         size,
		ContentType:  contentType,
		ETag:         etag,
		LastModified: time.Now(),
	}

	if opts != nil && opts.Metadata != nil {
		info.Metadata = opts.Metadata
	}

	// Store metadata in a sidecar file
	if err := f.writeMetadata(objectPath, info); err != nil {
		f.log.Warn().Err(err).Str("key", key).Msg("failed to write metadata")
	}

	recordOp("filesystem", "put", "success", time.Since(start).Seconds())
	recordTransfer("filesystem", "upload", size)
	recordSize("filesystem", size)

	f.log.Debug().
		Str("bucket", bucket).
		Str("key", key).
		Int64("size", size).
		Msg("object stored")

	return info, nil
}

// Get retrieves an object from the bucket.
func (f *FilesystemStore) Get(ctx context.Context, bucket, key string) (io.ReadCloser, *storage.ObjectInfo, error) {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("filesystem", "get", "error", time.Since(start).Seconds())
		return nil, nil, err
	}
	if err := ValidateKey(key); err != nil {
		recordOp("filesystem", "get", "error", time.Since(start).Seconds())
		return nil, nil, err
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.closed {
		recordOp("filesystem", "get", "error", time.Since(start).Seconds())
		return nil, nil, storage.ErrClosed
	}

	objectPath := filepath.Join(f.root, bucket, key)

	file, err := os.Open(objectPath)
	if err != nil {
		if os.IsNotExist(err) {
			recordOp("filesystem", "get", "not_found", time.Since(start).Seconds())
			return nil, nil, storage.ErrObjectNotFound
		}
		recordOp("filesystem", "get", "error", time.Since(start).Seconds())
		return nil, nil, fmt.Errorf("filesystem: open object: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		recordOp("filesystem", "get", "error", time.Since(start).Seconds())
		return nil, nil, fmt.Errorf("filesystem: stat object: %w", err)
	}

	// Read metadata
	info, err := f.readMetadata(objectPath)
	if err != nil {
		// Generate basic info if metadata unavailable
		info = &storage.ObjectInfo{
			Key:          key,
			Size:         stat.Size(),
			ContentType:  ContentTypeFromExtension(key),
			LastModified: stat.ModTime(),
		}
	}

	recordOp("filesystem", "get", "success", time.Since(start).Seconds())
	recordTransfer("filesystem", "download", info.Size)

	return &trackedReadCloser{
		ReadCloser: file,
		onClose: func(bytesRead int64) {
			f.log.Debug().
				Str("bucket", bucket).
				Str("key", key).
				Int64("bytes_read", bytesRead).
				Msg("object read")
		},
	}, info, nil
}

// Delete removes an object from the bucket.
func (f *FilesystemStore) Delete(ctx context.Context, bucket, key string) error {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("filesystem", "delete", "error", time.Since(start).Seconds())
		return err
	}
	if err := ValidateKey(key); err != nil {
		recordOp("filesystem", "delete", "error", time.Since(start).Seconds())
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		recordOp("filesystem", "delete", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	objectPath := filepath.Join(f.root, bucket, key)
	metadataPath := objectPath + ".meta"

	// Remove object
	if err := os.Remove(objectPath); err != nil {
		if os.IsNotExist(err) {
			recordOp("filesystem", "delete", "not_found", time.Since(start).Seconds())
			return storage.ErrObjectNotFound
		}
		recordOp("filesystem", "delete", "error", time.Since(start).Seconds())
		return fmt.Errorf("filesystem: delete object: %w", err)
	}

	// Remove metadata (ignore errors)
	os.Remove(metadataPath)

	// Try to remove empty parent directories
	f.cleanupEmptyDirs(filepath.Dir(objectPath), filepath.Join(f.root, bucket))

	recordOp("filesystem", "delete", "success", time.Since(start).Seconds())
	f.log.Debug().Str("bucket", bucket).Str("key", key).Msg("object deleted")
	return nil
}

// List lists objects in the bucket with the given prefix.
func (f *FilesystemStore) List(ctx context.Context, bucket, prefix string) ([]*storage.ObjectInfo, error) {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("filesystem", "list", "error", time.Since(start).Seconds())
		return nil, err
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.closed {
		recordOp("filesystem", "list", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	bucketPath := filepath.Join(f.root, bucket)
	var objects []*storage.ObjectInfo

	err := filepath.Walk(bucketPath, func(path string, info os.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return nil
		}

		// Skip metadata files
		if strings.HasSuffix(path, ".meta") {
			return nil
		}

		// Get relative key
		relPath, err := filepath.Rel(bucketPath, path)
		if err != nil {
			return nil
		}

		// Apply prefix filter
		if prefix != "" && !strings.HasPrefix(relPath, prefix) {
			return nil
		}

		// Read or generate object info
		objInfo, _ := f.readMetadata(path)
		if objInfo == nil {
			objInfo = &storage.ObjectInfo{
				Key:          relPath,
				Size:         info.Size(),
				ContentType:  ContentTypeFromExtension(relPath),
				LastModified: info.ModTime(),
			}
		}

		objects = append(objects, objInfo)
		return nil
	})

	if err != nil && err != filepath.SkipDir {
		recordOp("filesystem", "list", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("filesystem: list objects: %w", err)
	}

	// Sort by key
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})

	recordOp("filesystem", "list", "success", time.Since(start).Seconds())
	return objects, nil
}

// Stat returns object metadata without downloading.
func (f *FilesystemStore) Stat(ctx context.Context, bucket, key string) (*storage.ObjectInfo, error) {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("filesystem", "stat", "error", time.Since(start).Seconds())
		return nil, err
	}
	if err := ValidateKey(key); err != nil {
		recordOp("filesystem", "stat", "error", time.Since(start).Seconds())
		return nil, err
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.closed {
		recordOp("filesystem", "stat", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	objectPath := filepath.Join(f.root, bucket, key)

	stat, err := os.Stat(objectPath)
	if err != nil {
		if os.IsNotExist(err) {
			recordOp("filesystem", "stat", "not_found", time.Since(start).Seconds())
			return nil, storage.ErrObjectNotFound
		}
		recordOp("filesystem", "stat", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("filesystem: stat object: %w", err)
	}

	// Read metadata
	info, err := f.readMetadata(objectPath)
	if err != nil {
		info = &storage.ObjectInfo{
			Key:          key,
			Size:         stat.Size(),
			ContentType:  ContentTypeFromExtension(key),
			LastModified: stat.ModTime(),
		}
	}

	recordOp("filesystem", "stat", "success", time.Since(start).Seconds())
	return info, nil
}

// CreateBucket creates a new bucket.
func (f *FilesystemStore) CreateBucket(ctx context.Context, bucket string) error {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("filesystem", "create_bucket", "error", time.Since(start).Seconds())
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		recordOp("filesystem", "create_bucket", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	bucketPath := filepath.Join(f.root, bucket)
	if err := os.MkdirAll(bucketPath, 0755); err != nil {
		recordOp("filesystem", "create_bucket", "error", time.Since(start).Seconds())
		return fmt.Errorf("filesystem: create bucket: %w", err)
	}

	recordOp("filesystem", "create_bucket", "success", time.Since(start).Seconds())
	f.log.Info().Str("bucket", bucket).Msg("bucket created")
	return nil
}

// DeleteBucket deletes a bucket. Must be empty.
func (f *FilesystemStore) DeleteBucket(ctx context.Context, bucket string) error {
	start := time.Now()

	if err := ValidateBucket(bucket); err != nil {
		recordOp("filesystem", "delete_bucket", "error", time.Since(start).Seconds())
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		recordOp("filesystem", "delete_bucket", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	bucketPath := filepath.Join(f.root, bucket)

	// Check if bucket exists
	if _, err := os.Stat(bucketPath); os.IsNotExist(err) {
		recordOp("filesystem", "delete_bucket", "not_found", time.Since(start).Seconds())
		return storage.ErrBucketNotFound
	}

	// Check if bucket is empty
	entries, err := os.ReadDir(bucketPath)
	if err != nil {
		recordOp("filesystem", "delete_bucket", "error", time.Since(start).Seconds())
		return fmt.Errorf("filesystem: read bucket: %w", err)
	}
	if len(entries) > 0 {
		recordOp("filesystem", "delete_bucket", "error", time.Since(start).Seconds())
		return fmt.Errorf("filesystem: bucket not empty")
	}

	if err := os.Remove(bucketPath); err != nil {
		recordOp("filesystem", "delete_bucket", "error", time.Since(start).Seconds())
		return fmt.Errorf("filesystem: delete bucket: %w", err)
	}

	recordOp("filesystem", "delete_bucket", "success", time.Since(start).Seconds())
	f.log.Info().Str("bucket", bucket).Msg("bucket deleted")
	return nil
}

// ListBuckets returns all buckets.
func (f *FilesystemStore) ListBuckets(ctx context.Context) ([]string, error) {
	start := time.Now()

	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.closed {
		recordOp("filesystem", "list_buckets", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	entries, err := os.ReadDir(f.root)
	if err != nil {
		recordOp("filesystem", "list_buckets", "error", time.Since(start).Seconds())
		return nil, fmt.Errorf("filesystem: list buckets: %w", err)
	}

	var buckets []string
	for _, entry := range entries {
		if entry.IsDir() {
			buckets = append(buckets, entry.Name())
		}
	}

	sort.Strings(buckets)
	recordOp("filesystem", "list_buckets", "success", time.Since(start).Seconds())
	return buckets, nil
}

// Close releases resources.
func (f *FilesystemStore) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil
	}

	f.closed = true
	f.log.Info().Msg("filesystem object store closed")
	return nil
}

// Copy copies an object within the store.
func (f *FilesystemStore) Copy(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) (*storage.ObjectInfo, error) {
	start := time.Now()

	// Get source
	reader, info, err := f.Get(ctx, srcBucket, srcKey)
	if err != nil {
		recordOp("filesystem", "copy", "error", time.Since(start).Seconds())
		return nil, err
	}
	defer reader.Close()

	// Put to destination
	opts := &storage.PutOptions{
		ContentType: info.ContentType,
		Metadata:    info.Metadata,
	}

	result, err := f.Put(ctx, dstBucket, dstKey, reader, opts)
	if err != nil {
		recordOp("filesystem", "copy", "error", time.Since(start).Seconds())
		return nil, err
	}

	recordOp("filesystem", "copy", "success", time.Since(start).Seconds())
	return result, nil
}

// Walk walks objects with a callback.
func (f *FilesystemStore) Walk(ctx context.Context, bucket, prefix string, fn func(info *storage.ObjectInfo) error) error {
	objects, err := f.List(ctx, bucket, prefix)
	if err != nil {
		return err
	}

	for _, obj := range objects {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := fn(obj); err != nil {
			return err
		}
	}

	return nil
}

// Helper methods

func (f *FilesystemStore) writeMetadata(objectPath string, info *storage.ObjectInfo) error {
	metadataPath := objectPath + ".meta"
	data := fmt.Sprintf("%s\n%d\n%s\n%s",
		info.ContentType,
		info.Size,
		info.ETag,
		info.LastModified.Format(time.RFC3339),
	)
	return os.WriteFile(metadataPath, []byte(data), 0644)
}

func (f *FilesystemStore) readMetadata(objectPath string) (*storage.ObjectInfo, error) {
	metadataPath := objectPath + ".meta"
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 4 {
		return nil, fmt.Errorf("invalid metadata")
	}

	var size int64
	fmt.Sscanf(string(lines[1]), "%d", &size)

	lastMod, _ := time.Parse(time.RFC3339, string(lines[3]))

	// Get key from path
	key, _ := filepath.Rel(f.root, objectPath)
	parts := strings.SplitN(key, string(filepath.Separator), 2)
	if len(parts) > 1 {
		key = parts[1]
	}

	return &storage.ObjectInfo{
		Key:          key,
		ContentType:  string(lines[0]),
		Size:         size,
		ETag:         string(lines[2]),
		LastModified: lastMod,
	}, nil
}

func (f *FilesystemStore) cleanupEmptyDirs(path, stopAt string) {
	for path != stopAt && path != f.root {
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) > 0 {
			return
		}
		os.Remove(path)
		path = filepath.Dir(path)
	}
}

// trackedReadCloser tracks bytes read before closing.
type trackedReadCloser struct {
	io.ReadCloser
	bytesRead int64
	onClose   func(int64)
}

func (t *trackedReadCloser) Read(p []byte) (n int, err error) {
	n, err = t.ReadCloser.Read(p)
	t.bytesRead += int64(n)
	return
}

func (t *trackedReadCloser) Close() error {
	if t.onClose != nil {
		t.onClose(t.bytesRead)
	}
	return t.ReadCloser.Close()
}
