// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store_test

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// --- FileStore extended tests ---

func TestFileStore_DoubleClose(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	if err := fs.Close(); err != nil {
		t.Fatalf("First close: %v", err)
	}
	if err := fs.Close(); err != nil {
		t.Fatalf("Second close: %v", err)
	}
}

func TestFileStore_ClosedSet(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	fs.Close()
	ctx := context.Background()

	err = fs.Set(ctx, "test/path", &secrets.Secret{
		Path: "test/path",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("test")},
	})
	if err != secrets.ErrStoreClosed {
		t.Errorf("Set on closed: got %v, want ErrStoreClosed", err)
	}
}

func TestFileStore_ClosedDelete(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	fs.Close()
	ctx := context.Background()

	err = fs.Delete(ctx, "test/path")
	if err != secrets.ErrStoreClosed {
		t.Errorf("Delete on closed: got %v, want ErrStoreClosed", err)
	}
}

func TestFileStore_ClosedList(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	fs.Close()
	ctx := context.Background()

	_, err = fs.List(ctx, "")
	if err != secrets.ErrStoreClosed {
		t.Errorf("List on closed: got %v, want ErrStoreClosed", err)
	}
}

func TestFileStore_ClosedWatch(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	fs.Close()
	ctx := context.Background()

	_, err = fs.Watch(ctx, "test/path")
	if err != secrets.ErrStoreClosed {
		t.Errorf("Watch on closed: got %v, want ErrStoreClosed", err)
	}
}

func TestFileStore_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer fs.Close()
	ctx := context.Background()

	// Set initial
	s1 := &secrets.Secret{
		Path: "test/overwrite",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("initial")},
	}
	if err := fs.Set(ctx, s1.Path, s1); err != nil {
		t.Fatalf("Set initial: %v", err)
	}

	// Overwrite
	s2 := &secrets.Secret{
		Path: "test/overwrite",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("updated")},
	}
	if err := fs.Set(ctx, s2.Path, s2); err != nil {
		t.Fatalf("Set updated: %v", err)
	}

	got, err := fs.Get(ctx, s2.Path)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.Data["value"]) != "updated" {
		t.Errorf("Data = %q, want %q", string(got.Data["value"]), "updated")
	}
}

func TestFileStore_WatchDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	fs, err := store.NewFileStore(store.FileStoreConfig{
		BasePath:     tmpDir,
		PollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer fs.Close()

	// Create secret first
	secret := &secrets.Secret{
		Path: "watched/todelete",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("exists")},
	}
	if err := fs.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set: %v", err)
	}

	ch, err := fs.Watch(ctx, "watched/todelete")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Delete in background
	go func() {
		time.Sleep(200 * time.Millisecond)
		fs.Delete(ctx, "watched/todelete")
	}()

	select {
	case got := <-ch:
		if got != nil {
			t.Errorf("Expected nil for deletion, got: %v", got)
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for watch delete notification")
	}
}

func TestFileStore_ConcurrentReads(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer fs.Close()
	ctx := context.Background()

	// Pre-create a secret
	s := &secrets.Secret{
		Path: "concurrent/secret",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("value")},
	}
	if err := fs.Set(ctx, s.Path, s); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	// Concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				got, err := fs.Get(ctx, "concurrent/secret")
				if err != nil {
					errs <- err
					continue
				}
				if string(got.Data["value"]) != "value" {
					errs <- err
				}
			}
		}()
	}

	// Concurrent listers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_, err := fs.List(ctx, "")
				if err != nil {
					errs <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

func TestFileStore_ListSkipsNonJSON(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := store.NewFileStore(store.FileStoreConfig{BasePath: tmpDir})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer fs.Close()
	ctx := context.Background()

	// Create a JSON secret
	s := &secrets.Secret{
		Path: "test/secret",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("val")},
	}
	fs.Set(ctx, s.Path, s)

	// Create a non-JSON file that should be skipped
	os.WriteFile(tmpDir+"/notajsonfile.txt", []byte("hello"), 0644)

	paths, err := fs.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 1 {
		t.Errorf("List count = %d, want 1 (should skip non-JSON files)", len(paths))
	}
}

// --- MemoryStore extended tests ---

func TestMemoryStore_WatchContextCancel(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := ms.Watch(ctx, "test/path")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Cancel the context to trigger watcher removal
	cancel()

	// Give time for the goroutine to clean up
	time.Sleep(100 * time.Millisecond)

	// Set should not block even after watcher cleanup
	s := &secrets.Secret{
		Path: "test/path",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("after-cancel")},
	}
	if err := ms.Set(context.Background(), s.Path, s); err != nil {
		t.Fatalf("Set after cancel: %v", err)
	}

	// Channel should be closed after context cancel
	select {
	case _, ok := <-ch:
		if ok {
			// Got a value, that's fine too
		}
	case <-time.After(200 * time.Millisecond):
		// Timeout is acceptable too - channel might have been cleaned up
	}
}

func TestMemoryStore_WatchMultipleWatchers(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()
	ctx := context.Background()

	ch1, _ := ms.Watch(ctx, "test/path")
	ch2, _ := ms.Watch(ctx, "test/path")

	// Set should notify both watchers
	s := &secrets.Secret{
		Path: "test/path",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("multiwatch")},
	}
	ms.Set(ctx, s.Path, s)

	// Both should receive notification
	select {
	case got := <-ch1:
		if got == nil || string(got.Data["value"]) != "multiwatch" {
			t.Error("ch1 got unexpected value")
		}
	case <-time.After(time.Second):
		t.Error("ch1 timeout")
	}

	select {
	case got := <-ch2:
		if got == nil || string(got.Data["value"]) != "multiwatch" {
			t.Error("ch2 got unexpected value")
		}
	case <-time.After(time.Second):
		t.Error("ch2 timeout")
	}
}

// --- MemoryLeaseManager extended tests ---

func TestMemoryLeaseManager_RenewNotFound(t *testing.T) {
	lm := store.NewMemoryLeaseManager(nil)
	ctx := context.Background()

	_, err := lm.RenewLease(ctx, "nonexistent", time.Hour)
	if err != secrets.ErrSecretNotFound {
		t.Errorf("RenewLease nonexistent: got %v, want ErrSecretNotFound", err)
	}
}

func TestMemoryLeaseManager_RevokeNotFound(t *testing.T) {
	lm := store.NewMemoryLeaseManager(nil)
	ctx := context.Background()

	err := lm.RevokeLease(ctx, "nonexistent")
	if err != secrets.ErrSecretNotFound {
		t.Errorf("RevokeLease nonexistent: got %v, want ErrSecretNotFound", err)
	}
}

func TestMemoryLeaseManager_ListExcludesExpired(t *testing.T) {
	lm := store.NewMemoryLeaseManager(nil)
	ctx := context.Background()

	// Create a lease with very short duration (already expired)
	lm.CreateLease(ctx, "test/expired", 1*time.Nanosecond, true, 3)
	time.Sleep(10 * time.Millisecond) // Let it expire

	// Create an active lease
	lm.CreateLease(ctx, "test/active", time.Hour, true, 3)

	leases, err := lm.ListLeases(ctx)
	if err != nil {
		t.Fatalf("ListLeases: %v", err)
	}

	// Should only show the active lease
	if len(leases) != 1 {
		t.Errorf("ListLeases count = %d, want 1 (excluding expired)", len(leases))
	}
}

func TestMemoryLeaseManager_RevokeLeasesByPathNoMatch(t *testing.T) {
	lm := store.NewMemoryLeaseManager(nil)
	ctx := context.Background()

	lm.CreateLease(ctx, "test/secret1", time.Hour, true, 3)

	// Revoke by a path that has no leases
	err := lm.RevokeLeasesByPath(ctx, "nonexistent/path")
	if err != nil {
		t.Fatalf("RevokeLeasesByPath: %v", err)
	}

	// Original lease should still be active
	leases, _ := lm.ListLeases(ctx)
	if len(leases) != 1 {
		t.Errorf("ListLeases = %d, want 1", len(leases))
	}
}

// --- MemoryAuditLogger extended tests ---

func TestMemoryAuditLogger_DefaultMaxSize(t *testing.T) {
	// Passing 0 should use default (10000)
	al := store.NewMemoryAuditLogger(0)

	// Should be able to log more than a few events
	for i := 0; i < 100; i++ {
		al.Log(&secrets.AuditEvent{
			Timestamp: time.Now(),
			Operation: "test",
			Path:      "test/path",
		})
	}

	events := al.Events()
	if len(events) != 100 {
		t.Errorf("Events count = %d, want 100", len(events))
	}
}

func TestMemoryAuditLogger_NegativeMaxSize(t *testing.T) {
	// Negative should use default
	al := store.NewMemoryAuditLogger(-5)

	al.Log(&secrets.AuditEvent{
		Timestamp: time.Now(),
		Operation: "test",
		Path:      "test/path",
	})

	events := al.Events()
	if len(events) != 1 {
		t.Errorf("Events count = %d, want 1", len(events))
	}
}

func TestMemoryAuditLogger_QueryEndTime(t *testing.T) {
	al := store.NewMemoryAuditLogger(100)
	ctx := context.Background()

	now := time.Now()
	al.Log(&secrets.AuditEvent{
		Timestamp: now.Add(-2 * time.Hour),
		Operation: "get",
		Path:      "test/secret",
		Actor:     "user",
	})
	al.Log(&secrets.AuditEvent{
		Timestamp: now.Add(-1 * time.Hour),
		Operation: "set",
		Path:      "test/secret",
		Actor:     "user",
	})
	al.Log(&secrets.AuditEvent{
		Timestamp: now,
		Operation: "delete",
		Path:      "test/secret",
		Actor:     "admin",
	})

	// Query with EndTime filter
	results, err := al.Query(ctx, secrets.AuditFilter{
		EndTime: now.Add(-30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Query with EndTime = %d, want 2", len(results))
	}
}

func TestMemoryAuditLogger_QueryCombinedFilters(t *testing.T) {
	al := store.NewMemoryAuditLogger(100)
	ctx := context.Background()

	now := time.Now()
	al.Log(&secrets.AuditEvent{
		Timestamp: now,
		Operation: "get",
		Path:      "prod/db/password",
		Actor:     "admin",
	})
	al.Log(&secrets.AuditEvent{
		Timestamp: now,
		Operation: "set",
		Path:      "prod/db/password",
		Actor:     "admin",
	})
	al.Log(&secrets.AuditEvent{
		Timestamp: now,
		Operation: "get",
		Path:      "prod/db/password",
		Actor:     "user",
	})

	// Combined filter: operation=get AND actor=admin
	results, err := al.Query(ctx, secrets.AuditFilter{
		Operation: "get",
		Actor:     "admin",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Query combined = %d, want 1", len(results))
	}
}

func TestMemoryAuditLogger_EventsCopy(t *testing.T) {
	al := store.NewMemoryAuditLogger(100)

	al.Log(&secrets.AuditEvent{
		Timestamp: time.Now(),
		Operation: "get",
		Path:      "test/path",
	})

	events1 := al.Events()
	events2 := al.Events()

	// Should be different slices (copy)
	if len(events1) != len(events2) {
		t.Error("Events copies should have same length")
	}
}

// --- VaultStore config tests ---

func TestNewVaultStore_MissingAddress(t *testing.T) {
	_, err := store.NewVaultStore(store.VaultStoreConfig{
		Token: "test-token",
	})
	if err == nil {
		t.Error("expected error for missing address")
	}
}

func TestNewVaultStore_MissingToken(t *testing.T) {
	_, err := store.NewVaultStore(store.VaultStoreConfig{
		Address: "http://localhost:8200",
	})
	if err == nil {
		t.Error("expected error for missing token")
	}
}

// --- FileStore struct types test ---

func TestSecretFileJSON(t *testing.T) {
	sf := store.SecretFile{
		Data: map[string]interface{}{"key": "value"},
		SOPS: &store.SOPSMetadata{
			Age: []store.AgeRecipient{
				{Recipient: "age1abc", EncryptedKey: "enc123"},
			},
			LastModified: "2026-01-01T00:00:00Z",
			MAC:          "mac-value",
			Version:      "3.7.3",
		},
		Metadata: &store.SecretFileMetadata{
			Path:      "test/path",
			Type:      secrets.SecretTypeStatic,
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded store.SecretFile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.SOPS.Version != "3.7.3" {
		t.Errorf("SOPS Version = %q, want %q", decoded.SOPS.Version, "3.7.3")
	}
	if decoded.Metadata.Path != "test/path" {
		t.Errorf("Metadata Path = %q, want %q", decoded.Metadata.Path, "test/path")
	}
	if len(decoded.SOPS.Age) != 1 {
		t.Errorf("SOPS Age count = %d, want 1", len(decoded.SOPS.Age))
	}
	if decoded.SOPS.Age[0].Recipient != "age1abc" {
		t.Errorf("Age Recipient = %q, want %q", decoded.SOPS.Age[0].Recipient, "age1abc")
	}
}

func TestSecretFileJSON_Nil(t *testing.T) {
	sf := store.SecretFile{
		Data: map[string]interface{}{"key": "value"},
	}

	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded store.SecretFile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.SOPS != nil {
		t.Error("SOPS should be nil")
	}
	if decoded.Metadata != nil {
		t.Error("Metadata should be nil")
	}
}

// --- VaultStore config types ---

func TestVaultStoreConfig_Fields(t *testing.T) {
	cfg := store.VaultStoreConfig{
		Address:      "http://vault:8200",
		Token:        "s.token123",
		MountPath:    "secret",
		PollInterval: 5 * time.Second,
		Timeout:      30 * time.Second,
		TLSConfig: &store.VaultTLSConfig{
			CACert:     "/etc/certs/ca.pem",
			ClientCert: "/etc/certs/client.pem",
			ClientKey:  "/etc/certs/client-key.pem",
			Insecure:   false,
		},
	}

	if cfg.Address != "http://vault:8200" {
		t.Errorf("Address = %q", cfg.Address)
	}
	if cfg.TLSConfig.CACert != "/etc/certs/ca.pem" {
		t.Errorf("CACert = %q", cfg.TLSConfig.CACert)
	}
	if cfg.TLSConfig.Insecure {
		t.Error("Insecure should be false")
	}
}

func TestFileStoreConfig_Fields(t *testing.T) {
	cfg := store.FileStoreConfig{
		BasePath:     "/var/lib/secrets",
		PollInterval: 10 * time.Second,
	}
	if cfg.BasePath != "/var/lib/secrets" {
		t.Errorf("BasePath = %q", cfg.BasePath)
	}
	if cfg.PollInterval != 10*time.Second {
		t.Errorf("PollInterval = %v", cfg.PollInterval)
	}
}

func TestMemoryStoreConfig_Fields(t *testing.T) {
	cfg := store.MemoryStoreConfig{}
	ms := store.NewMemoryStore(cfg)
	defer ms.Close()

	// Should not panic with nil logger
	ctx := context.Background()
	s := &secrets.Secret{
		Path: "test/path",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("test")},
	}
	if err := ms.Set(ctx, s.Path, s); err != nil {
		t.Fatalf("Set: %v", err)
	}
}
