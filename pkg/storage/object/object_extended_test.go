// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package object

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/storage"
)

// --- S3Store tests ---

func TestS3Store_CRUD(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	// Create bucket
	if err := store.CreateBucket(ctx, "testbucket"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}

	// List buckets
	buckets, err := store.ListBuckets(ctx)
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if len(buckets) != 1 || buckets[0] != "testbucket" {
		t.Errorf("ListBuckets = %v, want [testbucket]", buckets)
	}

	// Put
	data := []byte("s3 hello world")
	info, err := store.Put(ctx, "testbucket", "file.txt", bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Size != int64(len(data)) {
		t.Errorf("Size = %d, want %d", info.Size, len(data))
	}

	// Get
	reader, getInfo, err := store.Get(ctx, "testbucket", "file.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(reader)
	reader.Close()
	if string(got) != "s3 hello world" {
		t.Errorf("Get data = %q, want %q", string(got), "s3 hello world")
	}
	if getInfo.Size != int64(len(data)) {
		t.Errorf("Get info size = %d, want %d", getInfo.Size, len(data))
	}

	// Stat
	statInfo, err := store.Stat(ctx, "testbucket", "file.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if statInfo.Size != int64(len(data)) {
		t.Errorf("Stat size = %d, want %d", statInfo.Size, len(data))
	}

	// List objects
	objects, err := store.List(ctx, "testbucket", "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objects) != 1 {
		t.Errorf("List count = %d, want 1", len(objects))
	}

	// Copy
	copyInfo, err := store.Copy(ctx, "testbucket", "file.txt", "testbucket", "copy.txt")
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if copyInfo.Size != int64(len(data)) {
		t.Errorf("Copy size = %d, want %d", copyInfo.Size, len(data))
	}

	// Delete
	if err := store.Delete(ctx, "testbucket", "file.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "testbucket", "copy.txt"); err != nil {
		t.Fatalf("Delete copy: %v", err)
	}

	// Delete bucket
	if err := store.DeleteBucket(ctx, "testbucket"); err != nil {
		t.Fatalf("DeleteBucket: %v", err)
	}
}

func TestS3Store_NotFound(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	// Get nonexistent
	_, _, err = store.Get(ctx, "testbucket", "nonexistent.txt")
	if err != storage.ErrObjectNotFound {
		t.Errorf("Get nonexistent: got %v, want ErrObjectNotFound", err)
	}

	// Stat nonexistent
	_, err = store.Stat(ctx, "testbucket", "nonexistent.txt")
	if err != storage.ErrObjectNotFound {
		t.Errorf("Stat nonexistent: got %v, want ErrObjectNotFound", err)
	}

	// Delete nonexistent
	err = store.Delete(ctx, "testbucket", "nonexistent.txt")
	if err != storage.ErrObjectNotFound {
		t.Errorf("Delete nonexistent: got %v, want ErrObjectNotFound", err)
	}
}

func TestS3Store_BucketNotFound(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	// Put to nonexistent bucket
	_, err = store.Put(ctx, "nope-bucket", "key", bytes.NewReader([]byte("data")), nil)
	if err != storage.ErrBucketNotFound {
		t.Errorf("Put to missing bucket: got %v, want ErrBucketNotFound", err)
	}

	// Get from nonexistent bucket
	_, _, err = store.Get(ctx, "nope-bucket", "key")
	if err != storage.ErrBucketNotFound {
		t.Errorf("Get from missing bucket: got %v, want ErrBucketNotFound", err)
	}

	// Delete from nonexistent bucket
	err = store.Delete(ctx, "nope-bucket", "key")
	if err != storage.ErrBucketNotFound {
		t.Errorf("Delete from missing bucket: got %v, want ErrBucketNotFound", err)
	}

	// List from nonexistent bucket
	_, err = store.List(ctx, "nope-bucket", "")
	if err != storage.ErrBucketNotFound {
		t.Errorf("List missing bucket: got %v, want ErrBucketNotFound", err)
	}

	// Stat from nonexistent bucket
	_, err = store.Stat(ctx, "nope-bucket", "key")
	if err != storage.ErrBucketNotFound {
		t.Errorf("Stat missing bucket: got %v, want ErrBucketNotFound", err)
	}

	// Delete nonexistent bucket
	err = store.DeleteBucket(ctx, "nope-bucket")
	if err != storage.ErrBucketNotFound {
		t.Errorf("DeleteBucket missing: got %v, want ErrBucketNotFound", err)
	}
}

func TestS3Store_Closed(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	ctx := context.Background()

	store.Close()

	if _, err := store.Put(ctx, "bucket", "key", nil, nil); err != storage.ErrClosed {
		t.Errorf("Put on closed: got %v, want ErrClosed", err)
	}
	if _, _, err := store.Get(ctx, "bucket", "key"); err != storage.ErrClosed {
		t.Errorf("Get on closed: got %v, want ErrClosed", err)
	}
	if err := store.Delete(ctx, "bucket", "key"); err != storage.ErrClosed {
		t.Errorf("Delete on closed: got %v, want ErrClosed", err)
	}
	if _, err := store.List(ctx, "bucket", ""); err != storage.ErrClosed {
		t.Errorf("List on closed: got %v, want ErrClosed", err)
	}
	if _, err := store.Stat(ctx, "bucket", "key"); err != storage.ErrClosed {
		t.Errorf("Stat on closed: got %v, want ErrClosed", err)
	}
	if err := store.CreateBucket(ctx, "bucket"); err != storage.ErrClosed {
		t.Errorf("CreateBucket on closed: got %v, want ErrClosed", err)
	}
	if err := store.DeleteBucket(ctx, "bucket"); err != storage.ErrClosed {
		t.Errorf("DeleteBucket on closed: got %v, want ErrClosed", err)
	}
	if _, err := store.ListBuckets(ctx); err != storage.ErrClosed {
		t.Errorf("ListBuckets on closed: got %v, want ErrClosed", err)
	}

	// Double close
	if err := store.Close(); err != nil {
		t.Errorf("double Close: %v", err)
	}
}

func TestS3Store_ValidationErrors(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	// Invalid bucket on Put
	if _, err := store.Put(ctx, "", "key", nil, nil); err == nil {
		t.Error("expected error for empty bucket on Put")
	}
	// Invalid key on Put
	if _, err := store.Put(ctx, "bucket", "", nil, nil); err == nil {
		t.Error("expected error for empty key on Put")
	}
	// Invalid bucket on Get
	if _, _, err := store.Get(ctx, "", "key"); err == nil {
		t.Error("expected error for empty bucket on Get")
	}
	// Invalid key on Get
	if _, _, err := store.Get(ctx, "bucket", ""); err == nil {
		t.Error("expected error for empty key on Get")
	}
	// Invalid bucket on Delete
	if err := store.Delete(ctx, "", "key"); err == nil {
		t.Error("expected error for empty bucket on Delete")
	}
	// Invalid key on Delete
	if err := store.Delete(ctx, "bucket", ""); err == nil {
		t.Error("expected error for empty key on Delete")
	}
	// Invalid bucket on List
	if _, err := store.List(ctx, "", ""); err == nil {
		t.Error("expected error for empty bucket on List")
	}
	// Invalid bucket on Stat
	if _, err := store.Stat(ctx, "", "key"); err == nil {
		t.Error("expected error for empty bucket on Stat")
	}
	// Invalid key on Stat
	if _, err := store.Stat(ctx, "bucket", ""); err == nil {
		t.Error("expected error for empty key on Stat")
	}
	// Invalid bucket on CreateBucket
	if err := store.CreateBucket(ctx, "ab"); err == nil {
		t.Error("expected error for short bucket on CreateBucket")
	}
	// Invalid bucket on DeleteBucket
	if err := store.DeleteBucket(ctx, "ab"); err == nil {
		t.Error("expected error for short bucket on DeleteBucket")
	}
}

func TestS3Store_PutWithOptions(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	opts := &storage.PutOptions{
		ContentType: "text/html",
		Metadata:    map[string]string{"author": "test"},
	}
	data := []byte("<html></html>")
	info, err := store.Put(ctx, "testbucket", "page.html", bytes.NewReader(data), opts)
	if err != nil {
		t.Fatalf("Put with opts: %v", err)
	}
	if info.ContentType != "text/html" {
		t.Errorf("ContentType = %q, want %q", info.ContentType, "text/html")
	}
	if info.Metadata["author"] != "test" {
		t.Errorf("Metadata[author] = %q, want %q", info.Metadata["author"], "test")
	}
}

func TestS3Store_MaxObjectSize(t *testing.T) {
	store, err := NewS3Store(Config{MaxObjectSize: 10})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	data := []byte("this is more than 10 bytes")
	_, err = store.Put(ctx, "testbucket", "big.bin", bytes.NewReader(data), nil)
	if err != storage.ErrValueTooLarge {
		t.Errorf("Put oversized: got %v, want ErrValueTooLarge", err)
	}
}

func TestS3Store_CreateBucketDuplicate(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	if err := store.CreateBucket(ctx, "testbucket"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if err := store.CreateBucket(ctx, "testbucket"); err == nil {
		t.Error("expected error for duplicate bucket")
	}
}

func TestS3Store_DeleteBucketNotEmpty(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")
	store.Put(ctx, "testbucket", "file.txt", bytes.NewReader([]byte("data")), nil)

	err = store.DeleteBucket(ctx, "testbucket")
	if err == nil {
		t.Error("expected error for non-empty bucket delete")
	}
}

func TestS3Store_ListWithPrefix(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	store.Put(ctx, "testbucket", "logs/2024/jan.log", bytes.NewReader([]byte("jan")), nil)
	store.Put(ctx, "testbucket", "logs/2024/feb.log", bytes.NewReader([]byte("feb")), nil)
	store.Put(ctx, "testbucket", "data/config.json", bytes.NewReader([]byte("{}")), nil)

	objects, err := store.List(ctx, "testbucket", "logs/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objects) != 2 {
		t.Errorf("List with prefix 'logs/' = %d, want 2", len(objects))
	}

	objects, err = store.List(ctx, "testbucket", "data/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objects) != 1 {
		t.Errorf("List with prefix 'data/' = %d, want 1", len(objects))
	}
}

func TestS3Store_ListContextCancelled(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()

	store.CreateBucket(context.Background(), "testbucket")
	// Put many objects
	for i := 0; i < 100; i++ {
		store.Put(context.Background(), "testbucket", "key-"+string(rune('a'+i%26)), bytes.NewReader([]byte("data")), nil)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = store.List(ctx, "testbucket", "")
	// Either nil or context error is acceptable
	if err != nil && err != context.Canceled {
		t.Errorf("List with cancelled context: got %v", err)
	}
}

func TestS3Store_PresignGet(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	url, err := store.PresignGet(ctx, "mybucket", "mykey", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if !strings.Contains(url, "mybucket") {
		t.Errorf("PresignGet URL missing bucket: %s", url)
	}
	if !strings.Contains(url, "mykey") {
		t.Errorf("PresignGet URL missing key: %s", url)
	}
	if !strings.Contains(url, "300") {
		t.Errorf("PresignGet URL missing expiry seconds: %s", url)
	}
}

func TestS3Store_PresignPut(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	url, err := store.PresignPut(ctx, "mybucket", "mykey", 10*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	if !strings.Contains(url, "mybucket") {
		t.Errorf("PresignPut URL missing bucket: %s", url)
	}
	if !strings.Contains(url, "mykey") {
		t.Errorf("PresignPut URL missing key: %s", url)
	}
}

func TestS3Store_CopyAcrossBuckets(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "src-bucket")
	store.CreateBucket(ctx, "dst-bucket")

	data := []byte("cross-bucket data")
	store.Put(ctx, "src-bucket", "original.txt", bytes.NewReader(data), &storage.PutOptions{
		ContentType: "text/plain",
	})

	copyInfo, err := store.Copy(ctx, "src-bucket", "original.txt", "dst-bucket", "copied.txt")
	if err != nil {
		t.Fatalf("Copy across buckets: %v", err)
	}
	if copyInfo.Size != int64(len(data)) {
		t.Errorf("Copy size = %d, want %d", copyInfo.Size, len(data))
	}

	// Verify copied data
	reader, _, err := store.Get(ctx, "dst-bucket", "copied.txt")
	if err != nil {
		t.Fatalf("Get copied: %v", err)
	}
	got, _ := io.ReadAll(reader)
	reader.Close()
	if string(got) != "cross-bucket data" {
		t.Errorf("Copied data = %q, want %q", string(got), "cross-bucket data")
	}
}

func TestS3Store_CopyNotFound(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	_, err = store.Copy(ctx, "testbucket", "nonexistent", "testbucket", "copy")
	if err != storage.ErrObjectNotFound {
		t.Errorf("Copy nonexistent: got %v, want ErrObjectNotFound", err)
	}
}

func TestS3Store_MultipleBuckets(t *testing.T) {
	store, err := NewS3Store(Config{})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "alpha-bucket")
	store.CreateBucket(ctx, "beta-bucket")
	store.CreateBucket(ctx, "gamma-bucket")

	buckets, err := store.ListBuckets(ctx)
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if len(buckets) != 3 {
		t.Errorf("ListBuckets count = %d, want 3", len(buckets))
	}
	// Should be sorted
	if buckets[0] != "alpha-bucket" || buckets[1] != "beta-bucket" || buckets[2] != "gamma-bucket" {
		t.Errorf("ListBuckets not sorted: %v", buckets)
	}
}

// --- Filesystem extended tests ---

func TestFilesystemStore_PutWithContentType(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	opts := &storage.PutOptions{
		ContentType: "application/custom",
		Metadata:    map[string]string{"key": "value"},
	}
	info, err := store.Put(ctx, "testbucket", "data.bin", bytes.NewReader([]byte("binary data")), opts)
	if err != nil {
		t.Fatalf("Put with opts: %v", err)
	}
	if info.ContentType != "application/custom" {
		t.Errorf("ContentType = %q, want %q", info.ContentType, "application/custom")
	}
	if info.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %q, want %q", info.Metadata["key"], "value")
	}
}

func TestFilesystemStore_MaxObjectSize(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir, MaxObjectSize: 10})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	data := []byte("this exceeds 10 bytes limit")
	_, err = store.Put(ctx, "testbucket", "big.bin", bytes.NewReader(data), nil)
	if err != storage.ErrValueTooLarge {
		t.Errorf("Put oversized: got %v, want ErrValueTooLarge", err)
	}
}

func TestFilesystemStore_NestedKeys(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	// Put with nested key
	data := []byte("nested data")
	_, err = store.Put(ctx, "testbucket", "path/to/nested/file.txt", bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Put nested: %v", err)
	}

	// Get nested
	reader, _, err := store.Get(ctx, "testbucket", "path/to/nested/file.txt")
	if err != nil {
		t.Fatalf("Get nested: %v", err)
	}
	got, _ := io.ReadAll(reader)
	reader.Close()
	if string(got) != "nested data" {
		t.Errorf("Get nested data = %q, want %q", string(got), "nested data")
	}

	// Stat nested
	info, err := store.Stat(ctx, "testbucket", "path/to/nested/file.txt")
	if err != nil {
		t.Fatalf("Stat nested: %v", err)
	}
	if info.Size != int64(len(data)) {
		t.Errorf("Stat nested size = %d, want %d", info.Size, len(data))
	}

	// Delete nested (should clean up empty parent dirs)
	if err := store.Delete(ctx, "testbucket", "path/to/nested/file.txt"); err != nil {
		t.Fatalf("Delete nested: %v", err)
	}
}

func TestFilesystemStore_ListWithPrefix(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	store.Put(ctx, "testbucket", "logs/jan.log", bytes.NewReader([]byte("jan")), nil)
	store.Put(ctx, "testbucket", "logs/feb.log", bytes.NewReader([]byte("feb")), nil)
	store.Put(ctx, "testbucket", "data/config.json", bytes.NewReader([]byte("{}")), nil)

	objects, err := store.List(ctx, "testbucket", "logs/")
	if err != nil {
		t.Fatalf("List with prefix: %v", err)
	}
	if len(objects) != 2 {
		t.Errorf("List with prefix 'logs/' = %d, want 2", len(objects))
	}
}

func TestFilesystemStore_DeleteBucketNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	err = store.DeleteBucket(ctx, "nonexistent")
	if err != storage.ErrBucketNotFound {
		t.Errorf("DeleteBucket nonexistent: got %v, want ErrBucketNotFound", err)
	}
}

func TestFilesystemStore_DeleteBucketNotEmpty(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")
	store.Put(ctx, "testbucket", "file.txt", bytes.NewReader([]byte("data")), nil)

	err = store.DeleteBucket(ctx, "testbucket")
	if err == nil {
		t.Error("expected error for non-empty bucket delete")
	}
}

func TestFilesystemStore_ClosedGet(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	ctx := context.Background()

	store.Close()

	_, _, err = store.Get(ctx, "bucket", "key")
	if err != storage.ErrClosed {
		t.Errorf("Get on closed: got %v, want ErrClosed", err)
	}

	err = store.Delete(ctx, "bucket", "key")
	if err != storage.ErrClosed {
		t.Errorf("Delete on closed: got %v, want ErrClosed", err)
	}

	_, err = store.List(ctx, "bucket", "")
	if err != storage.ErrClosed {
		t.Errorf("List on closed: got %v, want ErrClosed", err)
	}

	_, err = store.Stat(ctx, "bucket", "key")
	if err != storage.ErrClosed {
		t.Errorf("Stat on closed: got %v, want ErrClosed", err)
	}

	err = store.DeleteBucket(ctx, "bucket")
	if err != storage.ErrClosed {
		t.Errorf("DeleteBucket on closed: got %v, want ErrClosed", err)
	}
}

func TestFilesystemStore_ValidationErrorsExtended(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	// Get with invalid bucket
	if _, _, err := store.Get(ctx, "", "key"); err == nil {
		t.Error("expected error for empty bucket on Get")
	}
	// Get with invalid key
	if _, _, err := store.Get(ctx, "bucket", ""); err == nil {
		t.Error("expected error for empty key on Get")
	}
	// Delete with invalid bucket
	if err := store.Delete(ctx, "", "key"); err == nil {
		t.Error("expected error for empty bucket on Delete")
	}
	// Delete with invalid key
	if err := store.Delete(ctx, "bucket", ""); err == nil {
		t.Error("expected error for empty key on Delete")
	}
	// List with invalid bucket
	if _, err := store.List(ctx, "", ""); err == nil {
		t.Error("expected error for empty bucket on List")
	}
	// Stat with invalid bucket
	if _, err := store.Stat(ctx, "", "key"); err == nil {
		t.Error("expected error for empty bucket on Stat")
	}
	// Stat with invalid key
	if _, err := store.Stat(ctx, "bucket", ""); err == nil {
		t.Error("expected error for empty key on Stat")
	}
	// CreateBucket with invalid name
	if err := store.CreateBucket(ctx, "ab"); err == nil {
		t.Error("expected error for short bucket on CreateBucket")
	}
	// DeleteBucket with invalid name
	if err := store.DeleteBucket(ctx, ""); err == nil {
		t.Error("expected error for empty bucket on DeleteBucket")
	}
}

func TestFilesystemStore_WalkContextCancelled(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()

	store.CreateBucket(context.Background(), "testbucket")
	store.Put(context.Background(), "testbucket", "a.txt", bytes.NewReader([]byte("a")), nil)
	store.Put(context.Background(), "testbucket", "b.txt", bytes.NewReader([]byte("b")), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = store.Walk(ctx, "testbucket", "", func(info *ObjectInfo) error {
		return nil
	})
	// Walk should propagate context cancellation (may be wrapped)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Walk with cancelled context: got unexpected error %v", err)
	}
}

func TestFilesystemStore_WalkCallbackError(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")
	store.Put(ctx, "testbucket", "a.txt", bytes.NewReader([]byte("a")), nil)

	walkErr := io.ErrUnexpectedEOF
	err = store.Walk(ctx, "testbucket", "", func(info *ObjectInfo) error {
		return walkErr
	})
	if err != walkErr {
		t.Errorf("Walk callback error: got %v, want %v", err, walkErr)
	}
}

func TestNew_S3Backend(t *testing.T) {
	store, err := New(Config{Backend: BackendS3})
	if err != nil {
		t.Fatalf("New S3: %v", err)
	}
	defer store.Close()
}

// --- trackedReadCloser test ---

func TestTrackedReadCloser(t *testing.T) {
	data := []byte("tracked reader test data")
	underlying := io.NopCloser(bytes.NewReader(data))

	var closedWith int64
	trc := &trackedReadCloser{
		ReadCloser: underlying,
		onClose: func(bytesRead int64) {
			closedWith = bytesRead
		},
	}

	buf := make([]byte, 10)
	n, err := trc.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != 10 {
		t.Errorf("Read n = %d, want 10", n)
	}

	// Read remaining
	rest, err := io.ReadAll(trc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(rest) != len(data)-10 {
		t.Errorf("ReadAll len = %d, want %d", len(rest), len(data)-10)
	}

	trc.Close()
	if closedWith != int64(len(data)) {
		t.Errorf("onClose bytesRead = %d, want %d", closedWith, len(data))
	}
}

func TestTrackedReadCloser_NilOnClose(t *testing.T) {
	underlying := io.NopCloser(bytes.NewReader([]byte("test")))
	trc := &trackedReadCloser{ReadCloser: underlying}

	buf := make([]byte, 100)
	trc.Read(buf)
	// Should not panic
	trc.Close()
}

// --- hasExtension test ---

func TestHasExtension(t *testing.T) {
	tests := []struct {
		key  string
		exts []string
		want bool
	}{
		{"file.json", []string{".json"}, true},
		{"file.txt", []string{".json"}, false},
		{"file.tar.gz", []string{".gz", ".gzip"}, true},
		{".json", []string{".json"}, false}, // key must be longer than ext
		{"", []string{".json"}, false},
	}
	for _, tt := range tests {
		got := hasExtension(tt.key, tt.exts...)
		if got != tt.want {
			t.Errorf("hasExtension(%q, %v) = %v, want %v", tt.key, tt.exts, got, tt.want)
		}
	}
}

// --- recordOp / recordTransfer / recordSize coverage ---

func TestMetricsRecording(t *testing.T) {
	// These should not panic
	recordOp("test", "put", "success", 0.5)
	recordOp("test", "get", "error", 0.1)
	recordTransfer("test", "upload", 1024)
	recordTransfer("test", "download", 2048)
	recordSize("test", 4096)
}

// --- MultipartUpload struct test ---

func TestMultipartUploadStruct(t *testing.T) {
	upload := &MultipartUpload{
		ID:        "test-upload-id",
		Bucket:    "mybucket",
		Key:       "mykey",
		StartTime: time.Now(),
		Parts: []Part{
			{Number: 1, ETag: "etag1", Size: 1024},
			{Number: 2, ETag: "etag2", Size: 2048},
		},
	}

	if upload.ID != "test-upload-id" {
		t.Errorf("ID = %q, want %q", upload.ID, "test-upload-id")
	}
	if len(upload.Parts) != 2 {
		t.Errorf("Parts count = %d, want 2", len(upload.Parts))
	}
	if upload.Parts[0].Size != 1024 {
		t.Errorf("Part 1 Size = %d, want 1024", upload.Parts[0].Size)
	}
}
