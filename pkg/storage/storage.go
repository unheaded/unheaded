// Package storage provides THE TASSETS - the thighs that hold the data.
// This is the Unheaded Kingdom's pluggable storage layer with support for
// key-value stores, object storage, caching, and write-ahead logging.
//
// Features:
//   - Pluggable backends (memory, BadgerDB, BoltDB)
//   - Key-value operations (Get, Set, Delete, Scan)
//   - Object storage for larger blobs
//   - Caching layer (LRU, TTL)
//   - Write-ahead log for durability
//   - Transaction support
//   - Compaction and cleanup
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// Common errors for storage operations.
var (
	// ErrNotFound is returned when a key does not exist.
	ErrNotFound = errors.New("storage: key not found")

	// ErrKeyTooLarge is returned when a key exceeds maximum size.
	ErrKeyTooLarge = errors.New("storage: key too large")

	// ErrValueTooLarge is returned when a value exceeds maximum size.
	ErrValueTooLarge = errors.New("storage: value too large")

	// ErrClosed is returned when operating on a closed store.
	ErrClosed = errors.New("storage: store closed")

	// ErrReadOnly is returned when writing to a read-only store.
	ErrReadOnly = errors.New("storage: read-only store")

	// ErrConflict is returned when a transaction conflicts.
	ErrConflict = errors.New("storage: transaction conflict")

	// ErrTxnClosed is returned when operating on a closed transaction.
	ErrTxnClosed = errors.New("storage: transaction closed")

	// ErrCorrupted is returned when data corruption is detected.
	ErrCorrupted = errors.New("storage: data corrupted")

	// ErrObjectNotFound is returned when an object does not exist.
	ErrObjectNotFound = errors.New("storage: object not found")

	// ErrBucketNotFound is returned when a bucket does not exist.
	ErrBucketNotFound = errors.New("storage: bucket not found")
)

// KVStore is the core key-value storage interface.
// All Kingdom storage backends must implement this interface.
type KVStore interface {
	// Get retrieves a value by key. Returns ErrNotFound if key doesn't exist.
	Get(ctx context.Context, key []byte) ([]byte, error)

	// Set stores a key-value pair. Creates or updates the key.
	Set(ctx context.Context, key, value []byte) error

	// Delete removes a key. Returns nil if key doesn't exist.
	Delete(ctx context.Context, key []byte) error

	// Scan iterates over all keys with the given prefix.
	// The callback receives a copy of key and value.
	// Return an error from the callback to stop iteration.
	Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error

	// Close releases all resources. Store cannot be used after Close.
	Close() error
}

// TransactionalStore extends KVStore with transaction support.
type TransactionalStore interface {
	KVStore

	// Begin starts a new transaction. If update is true, the transaction
	// can read and write. If false, it's read-only.
	Begin(ctx context.Context, update bool) (Transaction, error)
}

// Transaction represents an atomic batch of operations.
type Transaction interface {
	// Get retrieves a value within the transaction.
	Get(key []byte) ([]byte, error)

	// Set stores a key-value pair within the transaction.
	Set(key, value []byte) error

	// Delete removes a key within the transaction.
	Delete(key []byte) error

	// Commit applies all changes atomically.
	Commit() error

	// Rollback discards all changes.
	Rollback() error
}

// BatchWriter provides efficient batch write operations.
type BatchWriter interface {
	// Write applies a batch of operations atomically.
	Write(ctx context.Context, ops []BatchOp) error
}

// BatchOp represents a single operation in a batch.
type BatchOp struct {
	// Type is the operation type.
	Type BatchOpType
	// Key is the key to operate on.
	Key []byte
	// Value is the value for Set operations.
	Value []byte
}

// BatchOpType represents the type of batch operation.
type BatchOpType int

const (
	// OpSet sets a key-value pair.
	OpSet BatchOpType = iota
	// OpDelete removes a key.
	OpDelete
)

// Compactor provides compaction and cleanup capabilities.
type Compactor interface {
	// Compact triggers a compaction of the storage.
	Compact(ctx context.Context) error

	// GC runs garbage collection to reclaim space.
	GC(ctx context.Context) error
}

// Stats contains storage statistics.
type Stats struct {
	// Keys is the total number of keys.
	Keys int64
	// Size is the total size in bytes.
	Size int64
	// CompactionCount is the number of compactions performed.
	CompactionCount int64
	// LastCompaction is the time of the last compaction.
	LastCompaction time.Time
}

// StatsProvider provides storage statistics.
type StatsProvider interface {
	// Stats returns current storage statistics.
	Stats(ctx context.Context) (Stats, error)
}

// ObjectStore is the interface for object/blob storage.
type ObjectStore interface {
	// Put stores an object in the bucket.
	Put(ctx context.Context, bucket, key string, reader io.Reader, opts *PutOptions) (*ObjectInfo, error)

	// Get retrieves an object from the bucket.
	Get(ctx context.Context, bucket, key string) (io.ReadCloser, *ObjectInfo, error)

	// Delete removes an object from the bucket.
	Delete(ctx context.Context, bucket, key string) error

	// List lists objects in the bucket with the given prefix.
	List(ctx context.Context, bucket, prefix string) ([]*ObjectInfo, error)

	// Stat returns object metadata without downloading.
	Stat(ctx context.Context, bucket, key string) (*ObjectInfo, error)

	// CreateBucket creates a new bucket.
	CreateBucket(ctx context.Context, bucket string) error

	// DeleteBucket deletes a bucket. Must be empty.
	DeleteBucket(ctx context.Context, bucket string) error

	// ListBuckets returns all buckets.
	ListBuckets(ctx context.Context) ([]string, error)

	// Close releases resources.
	Close() error
}

// PutOptions configures object put operations.
type PutOptions struct {
	// ContentType is the MIME type of the object.
	ContentType string
	// Metadata is custom key-value metadata.
	Metadata map[string]string
	// Size is the expected size. -1 if unknown.
	Size int64
}

// ObjectInfo contains object metadata.
type ObjectInfo struct {
	// Key is the object key.
	Key string
	// Size is the object size in bytes.
	Size int64
	// ContentType is the MIME type.
	ContentType string
	// ETag is the entity tag for caching.
	ETag string
	// LastModified is when the object was last modified.
	LastModified time.Time
	// Metadata is custom key-value metadata.
	Metadata map[string]string
}

// Cache is the interface for caching layers.
type Cache interface {
	// Get retrieves a cached value. Returns nil if not found or expired.
	Get(ctx context.Context, key []byte) ([]byte, bool)

	// Set stores a value with optional TTL. TTL of 0 means no expiration.
	Set(ctx context.Context, key, value []byte, ttl time.Duration) error

	// Delete removes a cached value.
	Delete(ctx context.Context, key []byte)

	// Clear removes all cached values.
	Clear(ctx context.Context)

	// Stats returns cache statistics.
	Stats() CacheStats

	// Close releases resources.
	Close() error
}

// CacheStats contains cache statistics.
type CacheStats struct {
	// Hits is the number of cache hits.
	Hits int64
	// Misses is the number of cache misses.
	Misses int64
	// Size is the current number of cached items.
	Size int64
	// Bytes is the current size in bytes.
	Bytes int64
	// Evictions is the number of evicted items.
	Evictions int64
}

// WAL is the write-ahead log interface.
type WAL interface {
	// Write appends an entry to the log.
	Write(ctx context.Context, data []byte) (uint64, error)

	// Read reads an entry by sequence number.
	Read(ctx context.Context, seq uint64) ([]byte, error)

	// Scan iterates over entries starting from a sequence number.
	Scan(ctx context.Context, startSeq uint64, fn func(seq uint64, data []byte) error) error

	// Truncate removes entries before the given sequence number.
	Truncate(ctx context.Context, beforeSeq uint64) error

	// Sync ensures all entries are flushed to disk.
	Sync() error

	// LastSequence returns the last written sequence number.
	LastSequence() uint64

	// Close releases resources.
	Close() error
}

// Config holds storage configuration.
type Config struct {
	// Type is the storage backend type.
	Type string `json:"type" yaml:"type"`

	// Path is the data directory path.
	Path string `json:"path" yaml:"path"`

	// MaxKeySize is the maximum key size in bytes.
	MaxKeySize int `json:"max_key_size" yaml:"max_key_size"`

	// MaxValueSize is the maximum value size in bytes.
	MaxValueSize int `json:"max_value_size" yaml:"max_value_size"`

	// SyncWrites enables synchronous writes for durability.
	SyncWrites bool `json:"sync_writes" yaml:"sync_writes"`

	// ReadOnly opens the store in read-only mode.
	ReadOnly bool `json:"read_only" yaml:"read_only"`

	// CacheSize is the in-memory cache size in bytes.
	CacheSize int64 `json:"cache_size" yaml:"cache_size"`

	// CompactionInterval is how often to run compaction.
	CompactionInterval time.Duration `json:"compaction_interval" yaml:"compaction_interval"`
}

// DefaultConfig returns a default storage configuration.
func DefaultConfig() Config {
	return Config{
		Type:               "memory",
		Path:               "/var/lib/unheaded/storage",
		MaxKeySize:         1024,
		MaxValueSize:       64 * 1024 * 1024, // 64MB
		SyncWrites:         true,
		ReadOnly:           false,
		CacheSize:          64 * 1024 * 1024, // 64MB
		CompactionInterval: 1 * time.Hour,
	}
}
