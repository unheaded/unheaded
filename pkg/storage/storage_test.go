// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package storage_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/storage"
	"unheaded/pkg/storage/cache"
	"unheaded/pkg/storage/kv"
	"unheaded/pkg/storage/object"
)

// TestMemoryStore tests the in-memory key-value store.
func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemoryStore(kv.DefaultConfig())
	defer store.Close()

	// Test Set and Get
	t.Run("SetAndGet", func(t *testing.T) {
		key := []byte("test-key")
		value := []byte("test-value")

		err := store.Set(ctx, key, value)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		got, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if !bytes.Equal(got, value) {
			t.Errorf("Get returned %q, want %q", got, value)
		}
	})

	// Test Get non-existent key
	t.Run("GetNotFound", func(t *testing.T) {
		_, err := store.Get(ctx, []byte("non-existent"))
		if err != storage.ErrNotFound {
			t.Errorf("Get returned %v, want ErrNotFound", err)
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		key := []byte("delete-me")
		value := []byte("value")

		store.Set(ctx, key, value)
		err := store.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err = store.Get(ctx, key)
		if err != storage.ErrNotFound {
			t.Errorf("Get after delete returned %v, want ErrNotFound", err)
		}
	})

	// Test Scan
	t.Run("Scan", func(t *testing.T) {
		// Add some keys with a prefix
		prefix := []byte("scan-")
		for i := 0; i < 5; i++ {
			key := append(prefix, byte('a'+i))
			store.Set(ctx, key, []byte("value"))
		}

		var keys []string
		err := store.Scan(ctx, prefix, func(key, value []byte) error {
			keys = append(keys, string(key))
			return nil
		})
		if err != nil {
			t.Fatalf("Scan failed: %v", err)
		}

		if len(keys) != 5 {
			t.Errorf("Scan returned %d keys, want 5", len(keys))
		}
	})

	// Test concurrent access
	t.Run("ConcurrentAccess", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				key := []byte{byte(i)}
				value := []byte{byte(i * 2)}
				store.Set(ctx, key, value)
				store.Get(ctx, key)
			}(i)
		}
		wg.Wait()
	})
}

// TestMemoryStoreTransaction tests transaction support.
func TestMemoryStoreTransaction(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemoryStore(kv.DefaultConfig())
	defer store.Close()

	// Test commit
	t.Run("Commit", func(t *testing.T) {
		txn, err := store.Begin(ctx, true)
		if err != nil {
			t.Fatalf("Begin failed: %v", err)
		}

		key := []byte("txn-key")
		value := []byte("txn-value")

		err = txn.Set(key, value)
		if err != nil {
			t.Fatalf("Set in txn failed: %v", err)
		}

		err = txn.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		got, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get after commit failed: %v", err)
		}

		if !bytes.Equal(got, value) {
			t.Errorf("Got %q, want %q", got, value)
		}
	})

	// Test rollback
	t.Run("Rollback", func(t *testing.T) {
		txn, err := store.Begin(ctx, true)
		if err != nil {
			t.Fatalf("Begin failed: %v", err)
		}

		key := []byte("rollback-key")
		value := []byte("rollback-value")

		txn.Set(key, value)
		txn.Rollback()

		_, err = store.Get(ctx, key)
		if err != storage.ErrNotFound {
			t.Errorf("Get after rollback: got %v, want ErrNotFound", err)
		}
	})

	// Test read-only transaction
	t.Run("ReadOnly", func(t *testing.T) {
		// Set a value first
		key := []byte("readonly-key")
		value := []byte("readonly-value")
		store.Set(ctx, key, value)

		txn, err := store.Begin(ctx, false)
		if err != nil {
			t.Fatalf("Begin failed: %v", err)
		}
		defer txn.Rollback()

		got, err := txn.Get(key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if !bytes.Equal(got, value) {
			t.Errorf("Got %q, want %q", got, value)
		}

		err = txn.Set([]byte("new-key"), []byte("new-value"))
		if err != storage.ErrReadOnly {
			t.Errorf("Set in read-only txn: got %v, want ErrReadOnly", err)
		}
	})
}

// TestLRUCache tests the LRU cache.
func TestLRUCache(t *testing.T) {
	ctx := context.Background()
	cfg := cache.Config{
		MaxSize: 3,
		Type:    cache.TypeLRU,
	}
	c := cache.NewLRUCache(cfg)
	defer c.Close()

	// Test Set and Get
	t.Run("SetAndGet", func(t *testing.T) {
		key := []byte("cache-key")
		value := []byte("cache-value")

		err := c.Set(ctx, key, value, 0)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		got, ok := c.Get(ctx, key)
		if !ok {
			t.Fatal("Get returned not found")
		}

		if !bytes.Equal(got, value) {
			t.Errorf("Got %q, want %q", got, value)
		}
	})

	// Test eviction
	t.Run("Eviction", func(t *testing.T) {
		// Fill cache beyond capacity
		for i := 0; i < 5; i++ {
			key := []byte{byte('a' + i)}
			c.Set(ctx, key, []byte("value"), 0)
		}

		stats := c.Stats()
		if stats.Size > 3 {
			t.Errorf("Cache size %d > max size 3", stats.Size)
		}
	})

	// Test TTL expiration
	t.Run("TTLExpiration", func(t *testing.T) {
		c2 := cache.NewLRUCache(cache.Config{
			MaxSize:    100,
			DefaultTTL: 50 * time.Millisecond,
		})
		defer c2.Close()

		key := []byte("ttl-key")
		value := []byte("ttl-value")

		c2.Set(ctx, key, value, 50*time.Millisecond)

		// Should be found immediately
		if _, ok := c2.Get(ctx, key); !ok {
			t.Error("Get immediately after set returned not found")
		}

		// Wait for expiration
		time.Sleep(100 * time.Millisecond)

		// Should be expired
		if _, ok := c2.Get(ctx, key); ok {
			t.Error("Get after TTL expiration returned found")
		}
	})

	// Test Clear
	t.Run("Clear", func(t *testing.T) {
		c3 := cache.NewLRUCache(cache.DefaultConfig())
		defer c3.Close()

		for i := 0; i < 10; i++ {
			c3.Set(ctx, []byte{byte(i)}, []byte("value"), 0)
		}

		c3.Clear(ctx)

		stats := c3.Stats()
		if stats.Size != 0 {
			t.Errorf("After clear, size = %d, want 0", stats.Size)
		}
	})
}

// TestTTLCache tests the TTL cache.
func TestTTLCache(t *testing.T) {
	ctx := context.Background()
	cfg := cache.Config{
		DefaultTTL:      100 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	}
	c := cache.NewTTLCache(cfg)
	defer c.Close()

	// Test basic operations
	t.Run("BasicOperations", func(t *testing.T) {
		key := []byte("ttl-cache-key")
		value := []byte("ttl-cache-value")

		c.Set(ctx, key, value, 0)

		got, ok := c.Get(ctx, key)
		if !ok {
			t.Fatal("Get returned not found")
		}

		if !bytes.Equal(got, value) {
			t.Errorf("Got %q, want %q", got, value)
		}
	})

	// Test custom TTL
	t.Run("CustomTTL", func(t *testing.T) {
		key := []byte("custom-ttl-key")
		value := []byte("custom-ttl-value")

		c.Set(ctx, key, value, 30*time.Millisecond)

		// Should exist
		if _, ok := c.Get(ctx, key); !ok {
			t.Error("Get immediately after set returned not found")
		}

		// Wait for expiration
		time.Sleep(60 * time.Millisecond)

		// Should be expired
		if _, ok := c.Get(ctx, key); ok {
			t.Error("Get after TTL expiration returned found")
		}
	})
}

// TestFilesystemObjectStore tests the filesystem object store.
func TestFilesystemObjectStore(t *testing.T) {
	ctx := context.Background()

	// Create temporary directory
	tmpDir := t.TempDir()

	cfg := object.Config{
		Backend: object.BackendFilesystem,
		Path:    tmpDir,
	}

	store, err := object.NewFilesystemStore(cfg)
	if err != nil {
		t.Fatalf("NewFilesystemStore failed: %v", err)
	}
	defer store.Close()

	bucket := "test-bucket"

	// Create bucket
	t.Run("CreateBucket", func(t *testing.T) {
		err := store.CreateBucket(ctx, bucket)
		if err != nil {
			t.Fatalf("CreateBucket failed: %v", err)
		}

		buckets, err := store.ListBuckets(ctx)
		if err != nil {
			t.Fatalf("ListBuckets failed: %v", err)
		}

		found := false
		for _, b := range buckets {
			if b == bucket {
				found = true
				break
			}
		}
		if !found {
			t.Error("Created bucket not found in list")
		}
	})

	// Test Put and Get
	t.Run("PutAndGet", func(t *testing.T) {
		key := "test-object"
		content := "Hello, World!"

		info, err := store.Put(ctx, bucket, key, strings.NewReader(content), nil)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		if info.Size != int64(len(content)) {
			t.Errorf("Put returned size %d, want %d", info.Size, len(content))
		}

		reader, info, err := store.Get(ctx, bucket, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if string(data) != content {
			t.Errorf("Got %q, want %q", data, content)
		}
	})

	// Test Stat
	t.Run("Stat", func(t *testing.T) {
		key := "stat-object"
		content := "stat content"

		store.Put(ctx, bucket, key, strings.NewReader(content), nil)

		info, err := store.Stat(ctx, bucket, key)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		if info.Size != int64(len(content)) {
			t.Errorf("Stat returned size %d, want %d", info.Size, len(content))
		}
	})

	// Test List
	t.Run("List", func(t *testing.T) {
		// Add some objects
		for i := 0; i < 3; i++ {
			key := "list-object-" + string(rune('a'+i))
			store.Put(ctx, bucket, key, strings.NewReader("content"), nil)
		}

		objects, err := store.List(ctx, bucket, "list-")
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		if len(objects) < 3 {
			t.Errorf("List returned %d objects, want at least 3", len(objects))
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		key := "delete-object"
		store.Put(ctx, bucket, key, strings.NewReader("content"), nil)

		err := store.Delete(ctx, bucket, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err = store.Stat(ctx, bucket, key)
		if err != storage.ErrObjectNotFound {
			t.Errorf("Stat after delete: got %v, want ErrObjectNotFound", err)
		}
	})
}

// TestS3ObjectStore tests the S3 object store.
func TestS3ObjectStore(t *testing.T) {
	ctx := context.Background()

	cfg := object.Config{
		Backend: object.BackendS3,
		S3Config: object.S3Config{
			Endpoint: "http://localhost:9000",
			Region:   "us-east-1",
		},
	}

	store, err := object.NewS3Store(cfg)
	if err != nil {
		t.Fatalf("NewS3Store failed: %v", err)
	}
	defer store.Close()

	bucket := "s3-test-bucket"

	// Create bucket
	err = store.CreateBucket(ctx, bucket)
	if err != nil {
		t.Fatalf("CreateBucket failed: %v", err)
	}

	// Test Put and Get
	t.Run("PutAndGet", func(t *testing.T) {
		key := "s3-test-object"
		content := "S3 Hello, World!"

		_, err := store.Put(ctx, bucket, key, strings.NewReader(content), nil)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		reader, _, err := store.Get(ctx, bucket, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if string(data) != content {
			t.Errorf("Got %q, want %q", data, content)
		}
	})
}

// TestCacheWrapper tests the cache wrapper for KV stores.
func TestCacheWrapper(t *testing.T) {
	ctx := context.Background()

	// Create store and cache
	store := kv.NewMemoryStore(kv.DefaultConfig())
	defer store.Close()

	c := cache.NewLRUCache(cache.Config{
		MaxSize:    100,
		DefaultTTL: time.Minute,
	})
	defer c.Close()

	wrapper := cache.NewCacheWrapper(store, c, time.Minute)
	defer wrapper.Close()

	// Test that reads go through cache
	t.Run("CacheHit", func(t *testing.T) {
		key := []byte("cached-key")
		value := []byte("cached-value")

		// Set directly in store
		store.Set(ctx, key, value)

		// First read - should cache
		got, err := wrapper.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if !bytes.Equal(got, value) {
			t.Errorf("Got %q, want %q", got, value)
		}

		// Delete from store
		store.Delete(ctx, key)

		// Second read - should hit cache
		got, err = wrapper.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get from cache failed: %v", err)
		}
		if !bytes.Equal(got, value) {
			t.Errorf("Got from cache %q, want %q", got, value)
		}
	})

	// Test cache invalidation on write
	t.Run("WriteInvalidatesCache", func(t *testing.T) {
		key := []byte("invalidate-key")
		value1 := []byte("value1")
		value2 := []byte("value2")

		wrapper.Set(ctx, key, value1)

		got, _ := wrapper.Get(ctx, key)
		if !bytes.Equal(got, value1) {
			t.Errorf("Got %q, want %q", got, value1)
		}

		wrapper.Set(ctx, key, value2)

		got, _ = wrapper.Get(ctx, key)
		if !bytes.Equal(got, value2) {
			t.Errorf("Got %q, want %q", got, value2)
		}
	})
}

// TestPrefixedStore tests the prefix wrapper.
func TestPrefixedStore(t *testing.T) {
	ctx := context.Background()

	store := kv.NewMemoryStore(kv.DefaultConfig())
	defer store.Close()

	prefix := []byte("prefix:")
	prefixed := kv.NewPrefix(store, prefix)

	// Test that keys are prefixed
	t.Run("KeysArePrefixed", func(t *testing.T) {
		key := []byte("my-key")
		value := []byte("my-value")

		prefixed.Set(ctx, key, value)

		// Check with prefix
		fullKey := append(prefix, key...)
		got, err := store.Get(ctx, fullKey)
		if err != nil {
			t.Fatalf("Get with full key failed: %v", err)
		}
		if !bytes.Equal(got, value) {
			t.Errorf("Got %q, want %q", got, value)
		}

		// Check through prefix wrapper
		got, err = prefixed.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get through prefix failed: %v", err)
		}
		if !bytes.Equal(got, value) {
			t.Errorf("Got through prefix %q, want %q", got, value)
		}
	})
}

// TestLoadingCache tests the loading cache.
func TestLoadingCache(t *testing.T) {
	ctx := context.Background()

	c := cache.NewLRUCache(cache.Config{MaxSize: 100})
	defer c.Close()

	loadCount := 0
	loader := cache.LoaderFunc(func(ctx context.Context, key []byte) ([]byte, error) {
		loadCount++
		return append([]byte("loaded-"), key...), nil
	})

	lc := cache.NewLoadingCache(c, loader, time.Minute)

	// First get - should load
	got, err := lc.Get(ctx, []byte("key1"))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(got) != "loaded-key1" {
		t.Errorf("Got %q, want %q", got, "loaded-key1")
	}
	if loadCount != 1 {
		t.Errorf("Load count = %d, want 1", loadCount)
	}

	// Second get - should hit cache
	got, err = lc.Get(ctx, []byte("key1"))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(got) != "loaded-key1" {
		t.Errorf("Got %q, want %q", got, "loaded-key1")
	}
	if loadCount != 1 {
		t.Errorf("Load count = %d, want 1 (cache hit)", loadCount)
	}
}

// TestBatchWrite tests batch write operations.
func TestBatchWrite(t *testing.T) {
	ctx := context.Background()

	store := kv.NewMemoryStore(kv.DefaultConfig())
	defer store.Close()

	ops := []storage.BatchOp{
		{Type: storage.OpSet, Key: []byte("batch-1"), Value: []byte("value-1")},
		{Type: storage.OpSet, Key: []byte("batch-2"), Value: []byte("value-2")},
		{Type: storage.OpSet, Key: []byte("batch-3"), Value: []byte("value-3")},
	}

	err := store.Write(ctx, ops)
	if err != nil {
		t.Fatalf("Batch write failed: %v", err)
	}

	// Verify all keys exist
	for _, op := range ops {
		got, err := store.Get(ctx, op.Key)
		if err != nil {
			t.Errorf("Get %s failed: %v", op.Key, err)
			continue
		}
		if !bytes.Equal(got, op.Value) {
			t.Errorf("Got %q, want %q", got, op.Value)
		}
	}

	// Test delete in batch
	deleteOps := []storage.BatchOp{
		{Type: storage.OpDelete, Key: []byte("batch-1")},
		{Type: storage.OpDelete, Key: []byte("batch-2")},
	}

	err = store.Write(ctx, deleteOps)
	if err != nil {
		t.Fatalf("Batch delete failed: %v", err)
	}

	// Verify deleted
	_, err = store.Get(ctx, []byte("batch-1"))
	if err != storage.ErrNotFound {
		t.Errorf("Get deleted key: got %v, want ErrNotFound", err)
	}

	// Verify remaining key
	got, err := store.Get(ctx, []byte("batch-3"))
	if err != nil {
		t.Fatalf("Get remaining key failed: %v", err)
	}
	if string(got) != "value-3" {
		t.Errorf("Got %q, want %q", got, "value-3")
	}
}

// BenchmarkMemoryStore benchmarks the memory store.
func BenchmarkMemoryStore(b *testing.B) {
	ctx := context.Background()
	store := kv.NewMemoryStore(kv.DefaultConfig())
	defer store.Close()

	key := []byte("benchmark-key")
	value := []byte("benchmark-value-with-some-longer-content-to-be-more-realistic")

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			store.Set(ctx, key, value)
		}
	})

	b.Run("Get", func(b *testing.B) {
		store.Set(ctx, key, value)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			store.Get(ctx, key)
		}
	})

	b.Run("SetGet", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			k := append(key, byte(i%256))
			store.Set(ctx, k, value)
			store.Get(ctx, k)
		}
	})
}

// BenchmarkLRUCache benchmarks the LRU cache.
func BenchmarkLRUCache(b *testing.B) {
	ctx := context.Background()
	c := cache.NewLRUCache(cache.Config{MaxSize: 10000})
	defer c.Close()

	key := []byte("benchmark-key")
	value := []byte("benchmark-value")

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c.Set(ctx, key, value, 0)
		}
	})

	b.Run("Get", func(b *testing.B) {
		c.Set(ctx, key, value, 0)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Get(ctx, key)
		}
	})

	b.Run("SetWithEviction", func(b *testing.B) {
		smallCache := cache.NewLRUCache(cache.Config{MaxSize: 100})
		defer smallCache.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			k := append(key, byte(i%256))
			smallCache.Set(ctx, k, value, 0)
		}
	})
}
