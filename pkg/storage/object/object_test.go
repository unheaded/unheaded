// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package object

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Backend != BackendFilesystem {
		t.Errorf("Backend = %q, want %q", cfg.Backend, BackendFilesystem)
	}
	if cfg.MaxObjectSize != 5*1024*1024*1024 {
		t.Errorf("MaxObjectSize = %d, want 5GB", cfg.MaxObjectSize)
	}
}

func TestValidateKey(t *testing.T) {
	tests := []struct {
		key     string
		wantErr bool
	}{
		{"file.txt", false},
		{"path/to/file.json", false},
		{"a", false},
		{"", true},
		{strings.Repeat("x", 1025), true},
		{strings.Repeat("x", 1024), false},
	}
	for _, tt := range tests {
		err := ValidateKey(tt.key)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateKey(%q) err = %v, wantErr = %v", tt.key, err, tt.wantErr)
		}
	}
}

func TestValidateBucket(t *testing.T) {
	tests := []struct {
		bucket  string
		wantErr bool
	}{
		{"mybucket", false},
		{"abc", false},
		{strings.Repeat("x", 63), false},
		{"", true},
		{"ab", true},  // too short
		{strings.Repeat("x", 64), true}, // too long
	}
	for _, tt := range tests {
		err := ValidateBucket(tt.bucket)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateBucket(%q) err = %v, wantErr = %v", tt.bucket, err, tt.wantErr)
		}
	}
}

func TestContentTypeFromExtension(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"file.json", "application/json"},
		{"file.xml", "application/xml"},
		{"file.html", "text/html"},
		{"file.htm", "text/html"},
		{"file.css", "text/css"},
		{"file.js", "application/javascript"},
		{"file.txt", "text/plain"},
		{"file.png", "image/png"},
		{"file.jpg", "image/jpeg"},
		{"file.jpeg", "image/jpeg"},
		{"file.gif", "image/gif"},
		{"file.svg", "image/svg+xml"},
		{"file.pdf", "application/pdf"},
		{"file.zip", "application/zip"},
		{"file.gz", "application/gzip"},
		{"file.gzip", "application/gzip"},
		{"file.tar", "application/x-tar"},
		{"file.bin", "application/octet-stream"},
		{"noext", "application/octet-stream"},
	}
	for _, tt := range tests {
		got := ContentTypeFromExtension(tt.key)
		if got != tt.expected {
			t.Errorf("ContentTypeFromExtension(%q) = %q, want %q", tt.key, got, tt.expected)
		}
	}
}

func TestGenerateETag(t *testing.T) {
	etag := GenerateETag([]byte("hello"))
	if etag == "" {
		t.Error("expected non-empty ETag")
	}
	// Different lengths should give different etags
	etag2 := GenerateETag([]byte("hello world"))
	if etag == etag2 {
		t.Error("expected different ETags for different-length data")
	}
}

func TestLimitedReader(t *testing.T) {
	data := []byte("hello world, this is a test")
	lr := &LimitedReader{R: bytes.NewReader(data), N: 5}

	buf := make([]byte, 100)
	n, err := lr.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != 5 {
		t.Errorf("Read n = %d, want 5", n)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("Read = %q, want %q", string(buf[:n]), "hello")
	}

	// Next read should return EOF
	n, err = lr.Read(buf)
	if err != io.EOF {
		t.Errorf("expected EOF, got n=%d, err=%v", n, err)
	}
}

func TestProgressReader(t *testing.T) {
	data := []byte("hello world")
	var lastRead, lastTotal int64
	pr := &ProgressReader{
		R:    bytes.NewReader(data),
		Size: int64(len(data)),
		OnUpdate: func(read, total int64) {
			lastRead = read
			lastTotal = total
		},
	}

	buf := make([]byte, 100)
	n, _ := pr.Read(buf)
	if n != len(data) {
		t.Errorf("Read n = %d, want %d", n, len(data))
	}
	if lastRead != int64(len(data)) {
		t.Errorf("lastRead = %d, want %d", lastRead, len(data))
	}
	if lastTotal != int64(len(data)) {
		t.Errorf("lastTotal = %d, want %d", lastTotal, len(data))
	}
	if pr.BytesRead != int64(len(data)) {
		t.Errorf("BytesRead = %d, want %d", pr.BytesRead, len(data))
	}
}

func TestProgressReader_NilCallback(t *testing.T) {
	data := []byte("hello")
	pr := &ProgressReader{R: bytes.NewReader(data), Size: int64(len(data))}
	buf := make([]byte, 100)
	n, _ := pr.Read(buf)
	if n != len(data) {
		t.Errorf("Read n = %d, want %d", n, len(data))
	}
}

func TestNew_UnknownBackend(t *testing.T) {
	_, err := New(Config{Backend: "redis"})
	if err == nil {
		t.Error("expected error for unknown backend")
	}
}

func TestNew_Filesystem(t *testing.T) {
	dir := t.TempDir()
	store, err := New(Config{Backend: BackendFilesystem, Path: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()
}

func TestFilesystemStore_CRUD(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
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
	data := []byte("hello world")
	info, err := store.Put(ctx, "testbucket", "file.txt", bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Size != int64(len(data)) {
		t.Errorf("Size = %d, want %d", info.Size, len(data))
	}
	if info.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want %q", info.ContentType, "text/plain")
	}

	// Get
	reader, getInfo, err := store.Get(ctx, "testbucket", "file.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(reader)
	reader.Close()
	if string(got) != "hello world" {
		t.Errorf("Get data = %q, want %q", string(got), "hello world")
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

	// Walk
	var walked int
	err = store.Walk(ctx, "testbucket", "", func(info *ObjectInfo) error {
		walked++
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if walked != 2 {
		t.Errorf("walked = %d, want 2", walked)
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

func TestFilesystemStore_NotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.CreateBucket(ctx, "testbucket")

	// Get nonexistent
	_, _, err = store.Get(ctx, "testbucket", "nonexistent.txt")
	if err == nil {
		t.Error("expected error for nonexistent object")
	}

	// Stat nonexistent
	_, err = store.Stat(ctx, "testbucket", "nonexistent.txt")
	if err == nil {
		t.Error("expected error for nonexistent stat")
	}

	// Delete nonexistent
	err = store.Delete(ctx, "testbucket", "nonexistent.txt")
	if err == nil {
		t.Error("expected error for nonexistent delete")
	}
}

func TestFilesystemStore_Closed(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	ctx := context.Background()

	store.Close()

	if err := store.CreateBucket(ctx, "bucket"); err == nil {
		t.Error("expected error on closed store")
	}
	if _, err := store.ListBuckets(ctx); err == nil {
		t.Error("expected error on closed store")
	}
	if _, err := store.Put(ctx, "bucket", "key", nil, nil); err == nil {
		t.Error("expected error on closed store")
	}

	// Double close
	if err := store.Close(); err != nil {
		t.Errorf("double Close: %v", err)
	}
}

func TestFilesystemStore_ValidationErrors(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystemStore(Config{Path: dir})
	if err != nil {
		t.Fatalf("NewFilesystemStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	// Invalid bucket
	if _, err := store.Put(ctx, "", "key", nil, nil); err == nil {
		t.Error("expected error for empty bucket")
	}
	// Invalid key
	if _, err := store.Put(ctx, "bucket", "", nil, nil); err == nil {
		t.Error("expected error for empty key")
	}
}
