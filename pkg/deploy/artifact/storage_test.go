// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package artifact

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- MemoryStorage tests ---

func TestMemoryStoragePutGetDelete(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	ref, size, err := s.Put(ctx, "key1", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if ref != "key1" {
		t.Fatalf("expected ref key1, got %s", ref)
	}
	if size != 5 {
		t.Fatalf("expected size 5, got %d", size)
	}

	rc, err := s.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "hello" {
		t.Fatalf("expected hello, got %s", string(data))
	}

	exists, err := s.Exists(ctx, "key1")
	if err != nil || !exists {
		t.Fatal("expected key1 to exist")
	}

	err = s.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	exists, _ = s.Exists(ctx, "key1")
	if exists {
		t.Fatal("expected key1 to be deleted")
	}
}

func TestMemoryStorageGetNotFound(t *testing.T) {
	s := NewMemoryStorage()
	_, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestMemoryStorageDeleteNotFound(t *testing.T) {
	s := NewMemoryStorage()
	err := s.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestMemoryStorageCanceledContext(t *testing.T) {
	s := NewMemoryStorage()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := s.Put(ctx, "k", strings.NewReader("v"))
	if err == nil {
		t.Fatal("expected error on canceled context")
	}

	_, err = s.Get(ctx, "k")
	if err == nil {
		t.Fatal("expected error on canceled context")
	}

	err = s.Delete(ctx, "k")
	if err == nil {
		t.Fatal("expected error on canceled context")
	}

	_, err = s.Exists(ctx, "k")
	if err == nil {
		t.Fatal("expected error on canceled context")
	}
}

// --- FileStorage tests ---

func TestFileStoragePutGetDeleteExists(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	ctx := context.Background()

	ref, size, err := s.Put(ctx, "artifact-1", strings.NewReader("file-content"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if size != int64(len("file-content")) {
		t.Fatalf("unexpected size: %d", size)
	}

	exists, err := s.Exists(ctx, ref)
	if err != nil || !exists {
		t.Fatal("expected artifact to exist")
	}

	rc, err := s.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "file-content" {
		t.Fatalf("content mismatch: %s", string(data))
	}

	err = s.Delete(ctx, ref)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	exists, _ = s.Exists(ctx, ref)
	if exists {
		t.Fatal("expected artifact to be deleted")
	}
}

func TestFileStorageGetNotFound(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStorage(dir)
	_, err := s.Get(context.Background(), "nonexistent")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestFileStorageDeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStorage(dir)
	err := s.Delete(context.Background(), "nonexistent")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestFileStorageSanitizesFilename(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStorage(dir)
	ctx := context.Background()

	// Key with special characters that get sanitized.
	ref, _, err := s.Put(ctx, "my:app:v1", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Colons should be replaced.
	if strings.Contains(ref, ":") {
		t.Fatalf("ref should not contain colons: %s", ref)
	}

	// Verify file exists on disk.
	fp := filepath.Join(dir, ref)
	if _, err := os.Stat(fp); err != nil {
		t.Fatalf("file not found on disk: %v", err)
	}
}

func TestFileStorageList(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStorage(dir)
	ctx := context.Background()

	s.Put(ctx, "a", strings.NewReader("1"))
	s.Put(ctx, "b", strings.NewReader("2"))

	refs, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
}

func TestFileStorageListEmpty(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStorage(dir)
	refs, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs, got %d", len(refs))
	}
}

func TestFileStorageCanceledContext(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStorage(dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := s.Put(ctx, "k", strings.NewReader("v"))
	if err == nil {
		t.Fatal("expected error on canceled context")
	}

	_, err = s.Get(ctx, "k")
	if err == nil {
		t.Fatal("expected error on canceled context")
	}

	err = s.Delete(ctx, "k")
	if err == nil {
		t.Fatal("expected error on canceled context")
	}

	_, err = s.Exists(ctx, "k")
	if err == nil {
		t.Fatal("expected error on canceled context")
	}

	_, err = s.List(ctx)
	if err == nil {
		t.Fatal("expected error on canceled context")
	}
}

// --- sanitizeFilename tests ---

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"my:file", "my_file"},
		{"path/to/file", "path_to_file"},
		{`a\b`, "a_b"},
		{"a*b?c", "a_b_c"},
		{`"quoted"`, "_quoted_"},
		{"a<b>c", "a_b_c"},
		{"pipe|char", "pipe_char"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Fatalf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- CachingStorage tests ---

func TestCachingStoragePutGet(t *testing.T) {
	backend := NewMemoryStorage()
	cs := NewCachingStorage(backend, 1024)
	ctx := context.Background()

	ref, size, err := cs.Put(ctx, "k1", strings.NewReader("cached-data"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if size != int64(len("cached-data")) {
		t.Fatalf("unexpected size: %d", size)
	}

	// Get should hit cache.
	rc, err := cs.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "cached-data" {
		t.Fatalf("unexpected data: %s", data)
	}

	// Exists should work.
	exists, err := cs.Exists(ctx, ref)
	if err != nil || !exists {
		t.Fatal("expected key to exist")
	}
}

func TestCachingStorageEviction(t *testing.T) {
	backend := NewMemoryStorage()
	// Max cache 100 bytes; items up to 10 bytes (10%) are cacheable.
	cs := NewCachingStorage(backend, 100)
	ctx := context.Background()

	// Fill cache with small items.
	for i := 0; i < 20; i++ {
		key := string(rune('a'+i)) + "k"
		cs.Put(ctx, key, strings.NewReader("12345")) // 5 bytes each
	}

	// All items should still be accessible from backend even if evicted from cache.
	for i := 0; i < 20; i++ {
		key := string(rune('a'+i)) + "k"
		rc, err := cs.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get(%s): %v", key, err)
		}
		rc.Close()
	}
}

func TestCachingStorageLargeItemSkipsCache(t *testing.T) {
	backend := NewMemoryStorage()
	// Max cache 100 bytes; items > 10 bytes skip cache.
	cs := NewCachingStorage(backend, 100)
	ctx := context.Background()

	bigData := strings.Repeat("x", 50) // 50 bytes > 10% of 100
	ref, _, err := cs.Put(ctx, "big", strings.NewReader(bigData))
	if err != nil {
		t.Fatal(err)
	}

	// Should still be retrievable from backend.
	rc, err := cs.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != bigData {
		t.Fatal("content mismatch for large item")
	}
}

func TestCachingStorageDelete(t *testing.T) {
	backend := NewMemoryStorage()
	cs := NewCachingStorage(backend, 1024)
	ctx := context.Background()

	ref, _, _ := cs.Put(ctx, "del", strings.NewReader("bye"))
	err := cs.Delete(ctx, ref)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	exists, _ := cs.Exists(ctx, ref)
	if exists {
		t.Fatal("expected key to be deleted")
	}
}

// --- ReplicatedStorage tests ---

func TestReplicatedStoragePutGet(t *testing.T) {
	primary := NewMemoryStorage()
	replica1 := NewMemoryStorage()
	replica2 := NewMemoryStorage()
	rs := NewReplicatedStorage(primary, replica1, replica2)
	ctx := context.Background()

	ref, size, err := rs.Put(ctx, "rep-key", strings.NewReader("replicated"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if size != int64(len("replicated")) {
		t.Fatalf("unexpected size: %d", size)
	}

	// Get from primary.
	rc, err := rs.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "replicated" {
		t.Fatal("content mismatch")
	}
}

func TestReplicatedStorageGetFallback(t *testing.T) {
	primary := NewMemoryStorage()
	replica := NewMemoryStorage()
	rs := NewReplicatedStorage(primary, replica)
	ctx := context.Background()

	// Put data directly in replica, not through ReplicatedStorage.
	replica.Put(ctx, "fallback-key", strings.NewReader("from-replica"))

	rc, err := rs.Get(ctx, "fallback-key")
	if err != nil {
		t.Fatalf("Get fallback: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "from-replica" {
		t.Fatal("expected data from replica")
	}
}

func TestReplicatedStorageGetNotFound(t *testing.T) {
	primary := NewMemoryStorage()
	rs := NewReplicatedStorage(primary)
	_, err := rs.Get(context.Background(), "missing")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestReplicatedStorageDelete(t *testing.T) {
	primary := NewMemoryStorage()
	rs := NewReplicatedStorage(primary)
	ctx := context.Background()

	rs.Put(ctx, "dk", strings.NewReader("data"))
	err := rs.Delete(ctx, "dk")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestReplicatedStorageExists(t *testing.T) {
	primary := NewMemoryStorage()
	rs := NewReplicatedStorage(primary)
	ctx := context.Background()

	exists, _ := rs.Exists(ctx, "nope")
	if exists {
		t.Fatal("should not exist")
	}

	rs.Put(ctx, "yes", strings.NewReader("data"))
	exists, _ = rs.Exists(ctx, "yes")
	if !exists {
		t.Fatal("should exist")
	}
}

// --- NewFileStorage error case ---

func TestNewFileStorageInvalidPath(t *testing.T) {
	// Use a path under /dev/null which is not a directory.
	_, err := NewFileStorage("/dev/null/impossible")
	if err == nil {
		t.Fatal("expected error creating storage in invalid path")
	}
}
