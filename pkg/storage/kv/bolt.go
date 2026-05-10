// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package kv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/storage"
)

const (
	defaultBoltBucket = "default"
	boltFileMode      = 0600
)

// BoltStore is a BoltDB-backed key-value store.
// This is a mock implementation that provides the interface.
// In production, this would use go.etcd.io/bbolt.
type BoltStore struct {
	cfg    Config
	log    *logger.Logger
	data   *MemoryStore // Fallback to memory for this mock
	path   string
	bucket []byte
	closed bool
	mu     sync.RWMutex
}

// BoltOptions configures a BoltDB store.
type BoltOptions struct {
	// Bucket is the bucket name to use.
	Bucket string

	// NoSync disables fsync after each write.
	NoSync bool

	// NoGrowSync disables file growth sync.
	NoGrowSync bool

	// ReadOnly opens the database in read-only mode.
	ReadOnly bool

	// MmapFlags sets the mmap flags.
	MmapFlags int

	// InitialMmapSize sets the initial mmap size.
	InitialMmapSize int

	// Timeout sets the open timeout.
	Timeout time.Duration
}

// DefaultBoltOptions returns default BoltDB options.
func DefaultBoltOptions() BoltOptions {
	return BoltOptions{
		Bucket:   defaultBoltBucket,
		NoSync:   false,
		ReadOnly: false,
		Timeout:  1 * time.Second,
	}
}

// NewBoltStore creates a new BoltDB-backed store.
func NewBoltStore(cfg Config) (*BoltStore, error) {
	log := cfg.Logger
	if log == nil {
		log = logger.New(os.Stdout)
	}

	// Ensure directory exists
	if cfg.Path != "" {
		dir := filepath.Dir(cfg.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("bolt: create directory: %w", err)
		}
	}

	// In production, this would initialize the actual BoltDB
	// opts := &bbolt.Options{
	//     Timeout:    1 * time.Second,
	//     ReadOnly:   cfg.ReadOnly,
	//     NoSync:     !cfg.SyncWrites,
	// }
	// db, err := bbolt.Open(cfg.Path, boltFileMode, opts)
	// if err != nil {
	//     return nil, fmt.Errorf("bolt: open database: %w", err)
	// }
	//
	// // Create default bucket
	// if !cfg.ReadOnly {
	//     err = db.Update(func(tx *bbolt.Tx) error {
	//         _, err := tx.CreateBucketIfNotExists([]byte(defaultBoltBucket))
	//         return err
	//     })
	//     if err != nil {
	//         db.Close()
	//         return nil, fmt.Errorf("bolt: create bucket: %w", err)
	//     }
	// }

	store := &BoltStore{
		cfg:    cfg,
		log:    log.With().Str("backend", "bolt").Logger(),
		data:   NewMemoryStore(cfg), // Mock: use memory store
		path:   cfg.Path,
		bucket: []byte(defaultBoltBucket),
	}

	store.log.Info().Str("path", cfg.Path).Msg("bolt store initialized")
	return store, nil
}

// Get retrieves a value by key.
func (b *BoltStore) Get(ctx context.Context, key []byte) ([]byte, error) {
	start := time.Now()

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		recordOp("bolt", "get", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	// In production:
	// var value []byte
	// err := b.db.View(func(tx *bbolt.Tx) error {
	//     bucket := tx.Bucket(b.bucket)
	//     if bucket == nil {
	//         return storage.ErrBucketNotFound
	//     }
	//     v := bucket.Get(key)
	//     if v == nil {
	//         return storage.ErrNotFound
	//     }
	//     value = CopyBytes(v)
	//     return nil
	// })

	value, err := b.data.Get(ctx, key)
	if err != nil {
		recordOp("bolt", "get", "error", time.Since(start).Seconds())
		return nil, err
	}

	recordOp("bolt", "get", "success", time.Since(start).Seconds())
	return value, nil
}

// Set stores a key-value pair.
func (b *BoltStore) Set(ctx context.Context, key, value []byte) error {
	start := time.Now()

	if err := Validate(key, value, b.cfg); err != nil {
		recordOp("bolt", "set", "error", time.Since(start).Seconds())
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		recordOp("bolt", "set", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		recordOp("bolt", "set", "error", time.Since(start).Seconds())
		return storage.ErrReadOnly
	}

	// In production:
	// err := b.db.Update(func(tx *bbolt.Tx) error {
	//     bucket := tx.Bucket(b.bucket)
	//     if bucket == nil {
	//         return storage.ErrBucketNotFound
	//     }
	//     return bucket.Put(key, value)
	// })

	if err := b.data.Set(ctx, key, value); err != nil {
		recordOp("bolt", "set", "error", time.Since(start).Seconds())
		return err
	}

	recordOp("bolt", "set", "success", time.Since(start).Seconds())
	return nil
}

// Delete removes a key.
func (b *BoltStore) Delete(ctx context.Context, key []byte) error {
	start := time.Now()

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		recordOp("bolt", "delete", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		recordOp("bolt", "delete", "error", time.Since(start).Seconds())
		return storage.ErrReadOnly
	}

	// In production:
	// err := b.db.Update(func(tx *bbolt.Tx) error {
	//     bucket := tx.Bucket(b.bucket)
	//     if bucket == nil {
	//         return storage.ErrBucketNotFound
	//     }
	//     return bucket.Delete(key)
	// })

	if err := b.data.Delete(ctx, key); err != nil {
		recordOp("bolt", "delete", "error", time.Since(start).Seconds())
		return err
	}

	recordOp("bolt", "delete", "success", time.Since(start).Seconds())
	return nil
}

// Scan iterates over all keys with the given prefix.
func (b *BoltStore) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	start := time.Now()

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		recordOp("bolt", "scan", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	// In production:
	// err := b.db.View(func(tx *bbolt.Tx) error {
	//     bucket := tx.Bucket(b.bucket)
	//     if bucket == nil {
	//         return storage.ErrBucketNotFound
	//     }
	//
	//     c := bucket.Cursor()
	//     for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
	//         select {
	//         case <-ctx.Done():
	//             return ctx.Err()
	//         default:
	//         }
	//
	//         if err := fn(CopyBytes(k), CopyBytes(v)); err != nil {
	//             return err
	//         }
	//     }
	//     return nil
	// })

	if err := b.data.Scan(ctx, prefix, fn); err != nil {
		recordOp("bolt", "scan", "error", time.Since(start).Seconds())
		return err
	}

	recordOp("bolt", "scan", "success", time.Since(start).Seconds())
	return nil
}

// Close releases resources.
func (b *BoltStore) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true
	b.log.Info().Msg("bolt store closed")

	// In production: return b.db.Close()
	return b.data.Close()
}

// Begin starts a new transaction.
func (b *BoltStore) Begin(ctx context.Context, update bool) (storage.Transaction, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, storage.ErrClosed
	}

	if update && b.cfg.ReadOnly {
		return nil, storage.ErrReadOnly
	}

	// In production:
	// tx, err := b.db.Begin(update)
	// if err != nil {
	//     return nil, err
	// }
	// return &boltTxn{tx: tx, bucket: b.bucket, update: update}, nil

	return b.data.Begin(ctx, update)
}

// Write applies a batch of operations atomically.
func (b *BoltStore) Write(ctx context.Context, ops []storage.BatchOp) error {
	start := time.Now()

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		recordOp("bolt", "batch", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		recordOp("bolt", "batch", "error", time.Since(start).Seconds())
		return storage.ErrReadOnly
	}

	// In production:
	// err := b.db.Update(func(tx *bbolt.Tx) error {
	//     bucket := tx.Bucket(b.bucket)
	//     if bucket == nil {
	//         return storage.ErrBucketNotFound
	//     }
	//
	//     for _, op := range ops {
	//         switch op.Type {
	//         case storage.OpSet:
	//             if err := bucket.Put(op.Key, op.Value); err != nil {
	//                 return err
	//             }
	//         case storage.OpDelete:
	//             if err := bucket.Delete(op.Key); err != nil {
	//                 return err
	//             }
	//         }
	//     }
	//     return nil
	// })

	if err := b.data.Write(ctx, ops); err != nil {
		recordOp("bolt", "batch", "error", time.Since(start).Seconds())
		return err
	}

	recordOp("bolt", "batch", "success", time.Since(start).Seconds())
	return nil
}

// Compact is a no-op for BoltDB.
// BoltDB auto-compacts during normal operations.
func (b *BoltStore) Compact(ctx context.Context) error {
	b.log.Debug().Msg("compact called (no-op for bolt)")
	return nil
}

// GC is a no-op for BoltDB.
func (b *BoltStore) GC(ctx context.Context) error {
	b.log.Debug().Msg("gc called (no-op for bolt)")
	return nil
}

// Stats returns store statistics.
func (b *BoltStore) Stats(ctx context.Context) (storage.Stats, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return storage.Stats{}, storage.ErrClosed
	}

	// In production:
	// stats := b.db.Stats()
	// return storage.Stats{
	//     Keys: int64(stats.TxStats.PageCount),
	//     Size: int64(stats.TxStats.PageAlloc),
	// }, nil

	return b.data.Stats(ctx)
}

// CreateBucket creates a new bucket.
func (b *BoltStore) CreateBucket(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		return storage.ErrReadOnly
	}

	// In production:
	// return b.db.Update(func(tx *bbolt.Tx) error {
	//     _, err := tx.CreateBucketIfNotExists([]byte(name))
	//     return err
	// })

	b.log.Info().Str("bucket", name).Msg("bucket created")
	return nil
}

// DeleteBucket deletes a bucket.
func (b *BoltStore) DeleteBucket(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		return storage.ErrReadOnly
	}

	// In production:
	// return b.db.Update(func(tx *bbolt.Tx) error {
	//     return tx.DeleteBucket([]byte(name))
	// })

	b.log.Info().Str("bucket", name).Msg("bucket deleted")
	return nil
}

// UseBucket returns a store that uses the specified bucket.
func (b *BoltStore) UseBucket(name string) *BoltStore {
	return &BoltStore{
		cfg:    b.cfg,
		log:    b.log,
		data:   b.data,
		path:   b.path,
		bucket: []byte(name),
	}
}

// boltTxn wraps a BoltDB transaction.
//
// In the future bbolt-backed implementation this will hold a *bbolt.Tx and
// the bucket name. Today the path goes through the in-memory store, so we
// don't carry the bucket on the txn.
type boltTxn struct {
	memTxn storage.Transaction
	update bool
	closed bool
}

// Get retrieves a value within the transaction.
func (t *boltTxn) Get(key []byte) ([]byte, error) {
	if t.closed {
		return nil, storage.ErrTxnClosed
	}

	// In production:
	// bucket := t.tx.Bucket(t.bucket)
	// if bucket == nil {
	//     return nil, storage.ErrBucketNotFound
	// }
	// v := bucket.Get(key)
	// if v == nil {
	//     return nil, storage.ErrNotFound
	// }
	// return CopyBytes(v), nil

	return t.memTxn.Get(key)
}

// Set stores a key-value pair within the transaction.
func (t *boltTxn) Set(key, value []byte) error {
	if t.closed {
		return storage.ErrTxnClosed
	}
	if !t.update {
		return storage.ErrReadOnly
	}

	// In production:
	// bucket := t.tx.Bucket(t.bucket)
	// if bucket == nil {
	//     return storage.ErrBucketNotFound
	// }
	// return bucket.Put(key, value)

	return t.memTxn.Set(key, value)
}

// Delete removes a key within the transaction.
func (t *boltTxn) Delete(key []byte) error {
	if t.closed {
		return storage.ErrTxnClosed
	}
	if !t.update {
		return storage.ErrReadOnly
	}

	// In production:
	// bucket := t.tx.Bucket(t.bucket)
	// if bucket == nil {
	//     return storage.ErrBucketNotFound
	// }
	// return bucket.Delete(key)

	return t.memTxn.Delete(key)
}

// Commit applies all changes atomically.
func (t *boltTxn) Commit() error {
	if t.closed {
		return storage.ErrTxnClosed
	}
	t.closed = true

	// In production: return t.tx.Commit()
	return t.memTxn.Commit()
}

// Rollback discards all changes.
func (t *boltTxn) Rollback() error {
	if t.closed {
		return storage.ErrTxnClosed
	}
	t.closed = true

	// In production: return t.tx.Rollback()
	return t.memTxn.Rollback()
}

// ScanBucket iterates over all key-value pairs in a bucket.
func (b *BoltStore) ScanBucket(ctx context.Context, bucketName string, fn func(key, value []byte) error) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return storage.ErrClosed
	}

	// In production:
	// return b.db.View(func(tx *bbolt.Tx) error {
	//     bucket := tx.Bucket([]byte(bucketName))
	//     if bucket == nil {
	//         return storage.ErrBucketNotFound
	//     }
	//     return bucket.ForEach(func(k, v []byte) error {
	//         return fn(CopyBytes(k), CopyBytes(v))
	//     })
	// })

	return b.data.Scan(ctx, nil, fn)
}

// NextSequence returns the next sequence number for a bucket.
func (b *BoltStore) NextSequence(bucketName string) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return 0, storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		return 0, storage.ErrReadOnly
	}

	// In production:
	// var seq uint64
	// err := b.db.Update(func(tx *bbolt.Tx) error {
	//     bucket := tx.Bucket([]byte(bucketName))
	//     if bucket == nil {
	//         return storage.ErrBucketNotFound
	//     }
	//     var err error
	//     seq, err = bucket.NextSequence()
	//     return err
	// })
	// return seq, err

	// Mock: just return a timestamp-based sequence
	return uint64(time.Now().UnixNano()), nil
}

// Backup writes a consistent snapshot of the database to the writer.
func (b *BoltStore) Backup(ctx context.Context, fn func(key, value []byte) error) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return storage.ErrClosed
	}

	// In production:
	// return b.db.View(func(tx *bbolt.Tx) error {
	//     _, err := tx.WriteTo(w)
	//     return err
	// })

	return b.data.Scan(ctx, nil, fn)
}

// First returns the first key-value pair.
func (b *BoltStore) First(ctx context.Context) ([]byte, []byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, nil, storage.ErrClosed
	}

	var firstKey, firstValue []byte
	err := b.data.Scan(ctx, nil, func(key, value []byte) error {
		firstKey = CopyBytes(key)
		firstValue = CopyBytes(value)
		return fmt.Errorf("stop")
	})

	if firstKey == nil {
		return nil, nil, storage.ErrNotFound
	}

	if err != nil && err.Error() != "stop" {
		return nil, nil, err
	}

	return firstKey, firstValue, nil
}

// Last returns the last key-value pair.
func (b *BoltStore) Last(ctx context.Context) ([]byte, []byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, nil, storage.ErrClosed
	}

	var lastKey, lastValue []byte
	err := b.data.Scan(ctx, nil, func(key, value []byte) error {
		lastKey = CopyBytes(key)
		lastValue = CopyBytes(value)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	if lastKey == nil {
		return nil, nil, storage.ErrNotFound
	}

	return lastKey, lastValue, nil
}

// ScanRange iterates over keys in a range [start, end).
func (b *BoltStore) ScanRange(ctx context.Context, start, end []byte, fn func(key, value []byte) error) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return storage.ErrClosed
	}

	return b.data.Scan(ctx, nil, func(key, value []byte) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if bytes.Compare(key, start) >= 0 && bytes.Compare(key, end) < 0 {
			return fn(key, value)
		}
		return nil
	})
}
