package kv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/storage"
)

// BadgerStore is a BadgerDB-backed key-value store.
// This is a mock implementation that provides the interface.
// In production, this would use github.com/dgraph-io/badger/v4.
type BadgerStore struct {
	cfg      Config
	log      *logger.Logger
	data     *MemoryStore // Fallback to memory for this mock
	closed   bool
	path     string
}

// NewBadgerStore creates a new BadgerDB-backed store.
func NewBadgerStore(cfg Config) (*BadgerStore, error) {
	log := cfg.Logger
	if log == nil {
		log = logger.New(os.Stdout)
	}

	// Ensure path exists
	if cfg.Path != "" {
		if err := os.MkdirAll(cfg.Path, 0755); err != nil {
			return nil, fmt.Errorf("badger: create directory: %w", err)
		}
	}

	// In production, this would initialize the actual BadgerDB
	// opts := badger.DefaultOptions(cfg.Path)
	// opts.SyncWrites = cfg.SyncWrites
	// opts.ReadOnly = cfg.ReadOnly
	// db, err := badger.Open(opts)

	store := &BadgerStore{
		cfg:  cfg,
		log:  log.With().Str("backend", "badger").Logger(),
		data: NewMemoryStore(cfg), // Mock: use memory store
		path: cfg.Path,
	}

	store.log.Info().Str("path", cfg.Path).Msg("badger store initialized")
	return store, nil
}

// Get retrieves a value by key.
func (b *BadgerStore) Get(ctx context.Context, key []byte) ([]byte, error) {
	start := time.Now()

	if b.closed {
		recordOp("badger", "get", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	// In production:
	// var value []byte
	// err := b.db.View(func(txn *badger.Txn) error {
	//     item, err := txn.Get(key)
	//     if err == badger.ErrKeyNotFound {
	//         return storage.ErrNotFound
	//     }
	//     if err != nil {
	//         return err
	//     }
	//     value, err = item.ValueCopy(nil)
	//     return err
	// })

	value, err := b.data.Get(ctx, key)
	if err != nil {
		recordOp("badger", "get", "error", time.Since(start).Seconds())
		return nil, err
	}

	recordOp("badger", "get", "success", time.Since(start).Seconds())
	return value, nil
}

// Set stores a key-value pair.
func (b *BadgerStore) Set(ctx context.Context, key, value []byte) error {
	start := time.Now()

	if err := Validate(key, value, b.cfg); err != nil {
		recordOp("badger", "set", "error", time.Since(start).Seconds())
		return err
	}

	if b.closed {
		recordOp("badger", "set", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		recordOp("badger", "set", "error", time.Since(start).Seconds())
		return storage.ErrReadOnly
	}

	// In production:
	// err := b.db.Update(func(txn *badger.Txn) error {
	//     return txn.Set(key, value)
	// })

	if err := b.data.Set(ctx, key, value); err != nil {
		recordOp("badger", "set", "error", time.Since(start).Seconds())
		return err
	}

	recordOp("badger", "set", "success", time.Since(start).Seconds())
	return nil
}

// Delete removes a key.
func (b *BadgerStore) Delete(ctx context.Context, key []byte) error {
	start := time.Now()

	if b.closed {
		recordOp("badger", "delete", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		recordOp("badger", "delete", "error", time.Since(start).Seconds())
		return storage.ErrReadOnly
	}

	// In production:
	// err := b.db.Update(func(txn *badger.Txn) error {
	//     return txn.Delete(key)
	// })

	if err := b.data.Delete(ctx, key); err != nil {
		recordOp("badger", "delete", "error", time.Since(start).Seconds())
		return err
	}

	recordOp("badger", "delete", "success", time.Since(start).Seconds())
	return nil
}

// Scan iterates over all keys with the given prefix.
func (b *BadgerStore) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	start := time.Now()

	if b.closed {
		recordOp("badger", "scan", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	// In production:
	// err := b.db.View(func(txn *badger.Txn) error {
	//     opts := badger.DefaultIteratorOptions
	//     opts.Prefix = prefix
	//     it := txn.NewIterator(opts)
	//     defer it.Close()
	//
	//     for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
	//         select {
	//         case <-ctx.Done():
	//             return ctx.Err()
	//         default:
	//         }
	//
	//         item := it.Item()
	//         key := item.KeyCopy(nil)
	//         value, err := item.ValueCopy(nil)
	//         if err != nil {
	//             return err
	//         }
	//         if err := fn(key, value); err != nil {
	//             return err
	//         }
	//     }
	//     return nil
	// })

	if err := b.data.Scan(ctx, prefix, fn); err != nil {
		recordOp("badger", "scan", "error", time.Since(start).Seconds())
		return err
	}

	recordOp("badger", "scan", "success", time.Since(start).Seconds())
	return nil
}

// Close releases resources.
func (b *BadgerStore) Close() error {
	if b.closed {
		return nil
	}

	b.closed = true
	b.log.Info().Msg("badger store closed")

	// In production: return b.db.Close()
	return b.data.Close()
}

// Begin starts a new transaction.
func (b *BadgerStore) Begin(ctx context.Context, update bool) (storage.Transaction, error) {
	if b.closed {
		return nil, storage.ErrClosed
	}

	if update && b.cfg.ReadOnly {
		return nil, storage.ErrReadOnly
	}

	// In production:
	// txn := b.db.NewTransaction(update)
	// return &badgerTxn{txn: txn, update: update}, nil

	return b.data.Begin(ctx, update)
}

// Write applies a batch of operations atomically.
func (b *BadgerStore) Write(ctx context.Context, ops []storage.BatchOp) error {
	start := time.Now()

	if b.closed {
		recordOp("badger", "batch", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		recordOp("badger", "batch", "error", time.Since(start).Seconds())
		return storage.ErrReadOnly
	}

	// In production:
	// wb := b.db.NewWriteBatch()
	// defer wb.Cancel()
	//
	// for _, op := range ops {
	//     switch op.Type {
	//     case storage.OpSet:
	//         if err := wb.Set(op.Key, op.Value); err != nil {
	//             return err
	//         }
	//     case storage.OpDelete:
	//         if err := wb.Delete(op.Key); err != nil {
	//             return err
	//         }
	//     }
	// }
	// return wb.Flush()

	if err := b.data.Write(ctx, ops); err != nil {
		recordOp("badger", "batch", "error", time.Since(start).Seconds())
		return err
	}

	recordOp("badger", "batch", "success", time.Since(start).Seconds())
	return nil
}

// Compact triggers a compaction of the storage.
func (b *BadgerStore) Compact(ctx context.Context) error {
	if b.closed {
		return storage.ErrClosed
	}

	b.log.Info().Msg("running compaction")

	// In production:
	// for {
	//     err := b.db.RunValueLogGC(0.5)
	//     if err == badger.ErrNoRewrite {
	//         break
	//     }
	//     if err != nil {
	//         return err
	//     }
	// }

	return nil
}

// GC runs garbage collection to reclaim space.
func (b *BadgerStore) GC(ctx context.Context) error {
	if b.closed {
		return storage.ErrClosed
	}

	b.log.Info().Msg("running garbage collection")

	// In production:
	// return b.db.RunValueLogGC(0.5)

	return nil
}

// Stats returns store statistics.
func (b *BadgerStore) Stats(ctx context.Context) (storage.Stats, error) {
	if b.closed {
		return storage.Stats{}, storage.ErrClosed
	}

	// In production:
	// lsm, vlog := b.db.Size()
	// return storage.Stats{
	//     Size: lsm + vlog,
	// }, nil

	return b.data.Stats(ctx)
}

// badgerTxn wraps a BadgerDB transaction.
type badgerTxn struct {
	// In production: txn *badger.Txn
	memTxn storage.Transaction
	update bool
	closed bool
}

// Get retrieves a value within the transaction.
func (t *badgerTxn) Get(key []byte) ([]byte, error) {
	if t.closed {
		return nil, storage.ErrTxnClosed
	}

	// In production:
	// item, err := t.txn.Get(key)
	// if err == badger.ErrKeyNotFound {
	//     return nil, storage.ErrNotFound
	// }
	// if err != nil {
	//     return nil, err
	// }
	// return item.ValueCopy(nil)

	return t.memTxn.Get(key)
}

// Set stores a key-value pair within the transaction.
func (t *badgerTxn) Set(key, value []byte) error {
	if t.closed {
		return storage.ErrTxnClosed
	}
	if !t.update {
		return storage.ErrReadOnly
	}

	// In production: return t.txn.Set(key, value)
	return t.memTxn.Set(key, value)
}

// Delete removes a key within the transaction.
func (t *badgerTxn) Delete(key []byte) error {
	if t.closed {
		return storage.ErrTxnClosed
	}
	if !t.update {
		return storage.ErrReadOnly
	}

	// In production: return t.txn.Delete(key)
	return t.memTxn.Delete(key)
}

// Commit applies all changes atomically.
func (t *badgerTxn) Commit() error {
	if t.closed {
		return storage.ErrTxnClosed
	}
	t.closed = true

	// In production: return t.txn.Commit()
	return t.memTxn.Commit()
}

// Rollback discards all changes.
func (t *badgerTxn) Rollback() error {
	if t.closed {
		return storage.ErrTxnClosed
	}
	t.closed = true

	// In production: t.txn.Discard(); return nil
	return t.memTxn.Rollback()
}

// ScanReverse iterates over keys in reverse order.
func (b *BadgerStore) ScanReverse(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	if b.closed {
		return storage.ErrClosed
	}

	// Collect all matching keys
	var pairs []struct{ key, value []byte }
	err := b.Scan(ctx, prefix, func(key, value []byte) error {
		pairs = append(pairs, struct{ key, value []byte }{
			key:   CopyBytes(key),
			value: CopyBytes(value),
		})
		return nil
	})
	if err != nil {
		return err
	}

	// Iterate in reverse
	for i := len(pairs) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := fn(pairs[i].key, pairs[i].value); err != nil {
			return err
		}
	}

	return nil
}

// SetWithTTL stores a key-value pair with a time-to-live.
func (b *BadgerStore) SetWithTTL(ctx context.Context, key, value []byte, ttl time.Duration) error {
	if err := Validate(key, value, b.cfg); err != nil {
		return err
	}

	if b.closed {
		return storage.ErrClosed
	}

	if b.cfg.ReadOnly {
		return storage.ErrReadOnly
	}

	// In production:
	// return b.db.Update(func(txn *badger.Txn) error {
	//     e := badger.NewEntry(key, value).WithTTL(ttl)
	//     return txn.SetEntry(e)
	// })

	// For mock, just set without TTL
	return b.data.Set(ctx, key, value)
}

// KeyExists checks if a key exists without reading the value.
func (b *BadgerStore) KeyExists(ctx context.Context, key []byte) (bool, error) {
	if b.closed {
		return false, storage.ErrClosed
	}

	// In production:
	// var exists bool
	// err := b.db.View(func(txn *badger.Txn) error {
	//     _, err := txn.Get(key)
	//     if err == badger.ErrKeyNotFound {
	//         exists = false
	//         return nil
	//     }
	//     if err != nil {
	//         return err
	//     }
	//     exists = true
	//     return nil
	// })
	// return exists, err

	_, err := b.data.Get(ctx, key)
	if err == storage.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Keys returns all keys with the given prefix.
func (b *BadgerStore) Keys(ctx context.Context, prefix []byte) ([][]byte, error) {
	if b.closed {
		return nil, storage.ErrClosed
	}

	var keys [][]byte
	err := b.Scan(ctx, prefix, func(key, _ []byte) error {
		keys = append(keys, CopyBytes(key))
		return nil
	})
	if err != nil {
		return nil, err
	}

	return keys, nil
}

// ScanRange iterates over keys in a range [start, end).
func (b *BadgerStore) ScanRange(ctx context.Context, start, end []byte, fn func(key, value []byte) error) error {
	if b.closed {
		return storage.ErrClosed
	}

	return b.Scan(ctx, nil, func(key, value []byte) error {
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
