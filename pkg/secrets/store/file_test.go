// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/encryption"
	"unheaded/pkg/secrets/store"
)

// TestFileStore_SetAndGet tests basic file store operations
func TestFileStore_SetAndGet(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "filestore_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fs, err := store.NewFileStore(store.FileStoreConfig{
		BasePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}
	defer fs.Close()

	secret := &secrets.Secret{
		Path: "test/secret",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{
			"value": []byte("file-stored-secret"),
		},
		Metadata: map[string]string{
			"owner": "test",
		},
	}

	if err := fs.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := fs.Get(ctx, secret.Path)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got.Data["value"]) != "file-stored-secret" {
		t.Errorf("Data mismatch: got %s, want file-stored-secret", string(got.Data["value"]))
	}

	// Verify file exists
	files, _ := os.ReadDir(tmpDir)
	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}
}

// TestFileStore_WithEncryption tests file store with encryption
func TestFileStore_WithEncryption(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "filestore_enc_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	enc, err := encryption.NewSimpleEncryptor(key)
	if err != nil {
		t.Fatalf("NewSimpleEncryptor failed: %v", err)
	}

	fs, err := store.NewFileStore(store.FileStoreConfig{
		BasePath:  tmpDir,
		Encryptor: enc,
	})
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}
	defer fs.Close()

	secret := &secrets.Secret{
		Path: "encrypted/secret",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{
			"value": []byte("encrypted-value"),
		},
	}

	if err := fs.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Read raw file content - should be encrypted
	files, _ := os.ReadDir(tmpDir)
	if len(files) == 0 {
		t.Fatal("No files created")
	}

	rawContent, _ := os.ReadFile(filepath.Join(tmpDir, files[0].Name()))
	if string(rawContent) == "encrypted-value" {
		t.Error("File content should be encrypted")
	}

	// Get should decrypt
	got, err := fs.Get(ctx, secret.Path)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got.Data["value"]) != "encrypted-value" {
		t.Errorf("Data mismatch after decryption: got %s", string(got.Data["value"]))
	}
}

// TestFileStore_GetNotFound tests getting a non-existent secret
func TestFileStore_GetNotFound(t *testing.T) {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "filestore_test")
	defer os.RemoveAll(tmpDir)

	fs, _ := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	defer fs.Close()

	_, err := fs.Get(ctx, "nonexistent/path")
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got: %v", err)
	}
}

// TestFileStore_Delete tests delete operation
func TestFileStore_Delete(t *testing.T) {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "filestore_test")
	defer os.RemoveAll(tmpDir)

	fs, _ := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	defer fs.Close()

	secret := &secrets.Secret{
		Path: "test/delete",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("to-delete")},
	}

	fs.Set(ctx, secret.Path, secret)

	if err := fs.Delete(ctx, secret.Path); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := fs.Get(ctx, secret.Path)
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound after delete, got: %v", err)
	}

	// Verify file is removed
	files, _ := os.ReadDir(tmpDir)
	if len(files) != 0 {
		t.Errorf("Expected 0 files after delete, got %d", len(files))
	}
}

// TestFileStore_DeleteNotFound tests deleting a non-existent secret
func TestFileStore_DeleteNotFound(t *testing.T) {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "filestore_test")
	defer os.RemoveAll(tmpDir)

	fs, _ := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	defer fs.Close()

	err := fs.Delete(ctx, "nonexistent/path")
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got: %v", err)
	}
}

// TestFileStore_List tests listing secrets
func TestFileStore_List(t *testing.T) {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "filestore_test")
	defer os.RemoveAll(tmpDir)

	fs, _ := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	defer fs.Close()

	paths := []string{
		"prod/db/password",
		"prod/db/username",
		"prod/api/key",
		"staging/db/password",
	}

	for _, path := range paths {
		secret := &secrets.Secret{
			Path: path,
			Type: secrets.SecretTypeStatic,
			Data: map[string][]byte{"value": []byte("test")},
		}
		fs.Set(ctx, path, secret)
	}

	// List prod/db/* secrets
	got, err := fs.List(ctx, "prod/db/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("Expected 2 paths, got %d: %v", len(got), got)
	}

	// List all
	got, err = fs.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(got) != 4 {
		t.Errorf("Expected 4 paths, got %d: %v", len(got), got)
	}
}

// TestFileStore_PathEncoding tests that paths with special characters are handled
func TestFileStore_PathEncoding(t *testing.T) {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "filestore_test")
	defer os.RemoveAll(tmpDir)

	fs, _ := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	defer fs.Close()

	// Path with multiple levels
	secret := &secrets.Secret{
		Path: "app/env/prod/db/password",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("deep-path")},
	}

	if err := fs.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := fs.Get(ctx, secret.Path)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got.Data["value"]) != "deep-path" {
		t.Errorf("Data mismatch: got %s", string(got.Data["value"]))
	}
}

// TestFileStore_Watch tests watching for changes
func TestFileStore_Watch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpDir, _ := os.MkdirTemp("", "filestore_watch_test")
	defer os.RemoveAll(tmpDir)

	fs, _ := store.NewFileStore(store.FileStoreConfig{
		BasePath:     tmpDir,
		PollInterval: 100 * time.Millisecond, // Fast polling for test
	})
	defer fs.Close()

	ch, err := fs.Watch(ctx, "watched/secret")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Set secret after starting watch
	go func() {
		time.Sleep(200 * time.Millisecond)
		secret := &secrets.Secret{
			Path: "watched/secret",
			Type: secrets.SecretTypeStatic,
			Data: map[string][]byte{"value": []byte("watched-value")},
		}
		fs.Set(ctx, secret.Path, secret)
	}()

	select {
	case got := <-ch:
		if got == nil {
			t.Error("Received nil secret")
		} else if string(got.Data["value"]) != "watched-value" {
			t.Errorf("Unexpected value: %s", string(got.Data["value"]))
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for watch update")
	}
}

// TestFileStore_Close tests store closure
func TestFileStore_Close(t *testing.T) {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "filestore_test")
	defer os.RemoveAll(tmpDir)

	fs, _ := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})

	secret := &secrets.Secret{
		Path: "test/close",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("test")},
	}
	fs.Set(ctx, secret.Path, secret)

	if err := fs.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations after close should fail
	_, err := fs.Get(ctx, secret.Path)
	if err != secrets.ErrStoreClosed {
		t.Errorf("Expected ErrStoreClosed, got: %v", err)
	}
}

// TestFileStore_AtomicWrite tests that writes are atomic
func TestFileStore_AtomicWrite(t *testing.T) {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "filestore_atomic_test")
	defer os.RemoveAll(tmpDir)

	fs, _ := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	defer fs.Close()

	// Create initial secret
	secret := &secrets.Secret{
		Path: "test/atomic",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("initial")},
	}
	fs.Set(ctx, secret.Path, secret)

	// Update secret
	secret.Data["value"] = []byte("updated")
	if err := fs.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// No temp files should remain
	files, _ := os.ReadDir(tmpDir)
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".tmp" {
			t.Errorf("Temp file found after write: %s", f.Name())
		}
	}
}

// TestFileStore_RequiresBasePath tests that base path is required
func TestFileStore_RequiresBasePath(t *testing.T) {
	_, err := store.NewFileStore(store.FileStoreConfig{})
	if err == nil {
		t.Error("Expected error for missing base path")
	}
}
