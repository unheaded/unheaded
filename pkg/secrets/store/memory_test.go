package store_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// TestMemoryStore_SetAndGet tests basic set and get operations
func TestMemoryStore_SetAndGet(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	secret := &secrets.Secret{
		Path: "test/secret",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{
			"value": []byte("secret-value"),
		},
		Metadata: map[string]string{
			"owner": "test",
		},
	}

	// Test Set
	if err := ms.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get
	got, err := ms.Get(ctx, secret.Path)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got.Data["value"]) != "secret-value" {
		t.Errorf("Data mismatch: got %s, want secret-value", string(got.Data["value"]))
	}

	if got.Metadata["owner"] != "test" {
		t.Errorf("Metadata mismatch: got %s, want test", got.Metadata["owner"])
	}
}

// TestMemoryStore_GetNotFound tests getting a non-existent secret
func TestMemoryStore_GetNotFound(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	_, err := ms.Get(ctx, "nonexistent/path")
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got: %v", err)
	}
}

// TestMemoryStore_Delete tests delete operation
func TestMemoryStore_Delete(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	secret := &secrets.Secret{
		Path: "test/delete",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("to-delete")},
	}

	if err := ms.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if err := ms.Delete(ctx, secret.Path); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := ms.Get(ctx, secret.Path)
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound after delete, got: %v", err)
	}
}

// TestMemoryStore_DeleteNotFound tests deleting a non-existent secret
func TestMemoryStore_DeleteNotFound(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	err := ms.Delete(ctx, "nonexistent/path")
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got: %v", err)
	}
}

// TestMemoryStore_List tests listing secrets by prefix
func TestMemoryStore_List(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	secretPaths := []string{
		"prod/db/password",
		"prod/db/username",
		"prod/api/key",
		"staging/db/password",
	}

	for _, path := range secretPaths {
		secret := &secrets.Secret{
			Path: path,
			Type: secrets.SecretTypeStatic,
			Data: map[string][]byte{"value": []byte("test")},
		}
		if err := ms.Set(ctx, path, secret); err != nil {
			t.Fatalf("Set failed for %s: %v", path, err)
		}
	}

	// List prod/db/* secrets
	paths, err := ms.List(ctx, "prod/db/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("Expected 2 paths, got %d: %v", len(paths), paths)
	}

	// List all prod/* secrets
	paths, err = ms.List(ctx, "prod/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(paths) != 3 {
		t.Errorf("Expected 3 paths, got %d: %v", len(paths), paths)
	}

	// List all secrets (empty prefix)
	paths, err = ms.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(paths) != 4 {
		t.Errorf("Expected 4 paths, got %d: %v", len(paths), paths)
	}
}

// TestMemoryStore_Watch tests watching for secret changes
func TestMemoryStore_Watch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	ch, err := ms.Watch(ctx, "watched/secret")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Set secret in background
	go func() {
		time.Sleep(100 * time.Millisecond)
		secret := &secrets.Secret{
			Path: "watched/secret",
			Type: secrets.SecretTypeStatic,
			Data: map[string][]byte{"value": []byte("new-value")},
		}
		ms.Set(ctx, secret.Path, secret)
	}()

	// Wait for update
	select {
	case got := <-ch:
		if got == nil {
			t.Error("Received nil secret")
		} else if string(got.Data["value"]) != "new-value" {
			t.Errorf("Unexpected value: %s", string(got.Data["value"]))
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for watch update")
	}
}

// TestMemoryStore_WatchDelete tests watching for secret deletion
func TestMemoryStore_WatchDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	// Create secret first
	secret := &secrets.Secret{
		Path: "watched/todelete",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("exists")},
	}
	if err := ms.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	ch, err := ms.Watch(ctx, "watched/todelete")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Delete secret in background
	go func() {
		time.Sleep(100 * time.Millisecond)
		ms.Delete(ctx, "watched/todelete")
	}()

	// Wait for deletion notification (nil)
	select {
	case got := <-ch:
		if got != nil {
			t.Errorf("Expected nil for deletion, got: %v", got)
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for watch delete notification")
	}
}

// TestMemoryStore_Close tests store closure
func TestMemoryStore_Close(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})

	secret := &secrets.Secret{
		Path: "test/close",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("test")},
	}
	ms.Set(ctx, secret.Path, secret)

	if err := ms.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations after close should fail
	_, err := ms.Get(ctx, secret.Path)
	if err != secrets.ErrStoreClosed {
		t.Errorf("Expected ErrStoreClosed, got: %v", err)
	}

	err = ms.Set(ctx, "test/new", secret)
	if err != secrets.ErrStoreClosed {
		t.Errorf("Expected ErrStoreClosed for Set, got: %v", err)
	}

	err = ms.Delete(ctx, secret.Path)
	if err != secrets.ErrStoreClosed {
		t.Errorf("Expected ErrStoreClosed for Delete, got: %v", err)
	}

	_, err = ms.List(ctx, "")
	if err != secrets.ErrStoreClosed {
		t.Errorf("Expected ErrStoreClosed for List, got: %v", err)
	}

	_, err = ms.Watch(ctx, "test")
	if err != secrets.ErrStoreClosed {
		t.Errorf("Expected ErrStoreClosed for Watch, got: %v", err)
	}
}

// TestMemoryStore_DoubleClose tests closing the store twice
func TestMemoryStore_DoubleClose(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})

	if err := ms.Close(); err != nil {
		t.Fatalf("First close failed: %v", err)
	}

	// Second close should not panic
	if err := ms.Close(); err != nil {
		t.Fatalf("Second close failed: %v", err)
	}
}

// TestMemoryStore_ConcurrentAccess tests concurrent read/write operations
func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	// Pre-create some secrets
	for i := 0; i < 10; i++ {
		secret := &secrets.Secret{
			Path: "concurrent/secret" + string(rune('0'+i)),
			Type: secrets.SecretTypeStatic,
			Data: map[string][]byte{"value": []byte("initial")},
		}
		ms.Set(ctx, secret.Path, secret)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				secret := &secrets.Secret{
					Path: "concurrent/secret" + string(rune('0'+id%10)),
					Type: secrets.SecretTypeStatic,
					Data: map[string][]byte{"value": []byte("updated")},
				}
				if err := ms.Set(ctx, secret.Path, secret); err != nil {
					errors <- err
				}
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, err := ms.Get(ctx, "concurrent/secret"+string(rune('0'+id%10)))
				if err != nil && err != secrets.ErrSecretNotFound {
					errors <- err
				}
			}
		}(i)
	}

	// Concurrent listers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, err := ms.List(ctx, "concurrent/")
				if err != nil {
					errors <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

// TestMemoryStore_DataIsolation tests that modifications to returned secrets don't affect stored secrets
func TestMemoryStore_DataIsolation(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	original := &secrets.Secret{
		Path: "test/isolation",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("original")},
	}

	if err := ms.Set(ctx, original.Path, original); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get and modify
	got, _ := ms.Get(ctx, original.Path)
	got.Data["value"] = []byte("modified")

	// Original should be unchanged
	got2, _ := ms.Get(ctx, original.Path)
	if string(got2.Data["value"]) != "original" {
		t.Errorf("Store data was modified: got %s, want original", string(got2.Data["value"]))
	}
}

// TestMemoryLeaseManager tests lease management
func TestMemoryLeaseManager_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	lm := store.NewMemoryLeaseManager(nil)

	lease, err := lm.CreateLease(ctx, "test/secret", time.Hour, true, 3)
	if err != nil {
		t.Fatalf("CreateLease failed: %v", err)
	}

	if lease.SecretPath != "test/secret" {
		t.Errorf("Unexpected secret path: %s", lease.SecretPath)
	}
	if !lease.Renewable {
		t.Error("Lease should be renewable")
	}
	if lease.MaxRenewals != 3 {
		t.Errorf("MaxRenewals mismatch: got %d, want 3", lease.MaxRenewals)
	}

	// Get lease
	got, err := lm.GetLease(ctx, lease.ID)
	if err != nil {
		t.Fatalf("GetLease failed: %v", err)
	}
	if got.ID != lease.ID {
		t.Errorf("Lease ID mismatch: got %s, want %s", got.ID, lease.ID)
	}
}

// TestMemoryLeaseManager_GetNotFound tests getting a non-existent lease
func TestMemoryLeaseManager_GetNotFound(t *testing.T) {
	ctx := context.Background()
	lm := store.NewMemoryLeaseManager(nil)

	_, err := lm.GetLease(ctx, "nonexistent")
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got: %v", err)
	}
}

// TestMemoryLeaseManager_Renew tests lease renewal
func TestMemoryLeaseManager_Renew(t *testing.T) {
	ctx := context.Background()
	lm := store.NewMemoryLeaseManager(nil)

	lease, _ := lm.CreateLease(ctx, "test/secret", time.Hour, true, 3)

	// Renew lease
	renewed, err := lm.RenewLease(ctx, lease.ID, 2*time.Hour)
	if err != nil {
		t.Fatalf("RenewLease failed: %v", err)
	}

	if renewed.RenewCount != 1 {
		t.Errorf("RenewCount mismatch: got %d, want 1", renewed.RenewCount)
	}

	// Renew again
	renewed, err = lm.RenewLease(ctx, lease.ID, 2*time.Hour)
	if err != nil {
		t.Fatalf("Second RenewLease failed: %v", err)
	}

	if renewed.RenewCount != 2 {
		t.Errorf("RenewCount mismatch: got %d, want 2", renewed.RenewCount)
	}
}

// TestMemoryLeaseManager_RenewMaxReached tests renewal when max renewals is reached
func TestMemoryLeaseManager_RenewMaxReached(t *testing.T) {
	ctx := context.Background()
	lm := store.NewMemoryLeaseManager(nil)

	lease, _ := lm.CreateLease(ctx, "test/secret", time.Hour, true, 1)

	// First renewal should succeed
	_, err := lm.RenewLease(ctx, lease.ID, 2*time.Hour)
	if err != nil {
		t.Fatalf("First RenewLease failed: %v", err)
	}

	// Second renewal should fail (max reached)
	_, err = lm.RenewLease(ctx, lease.ID, 2*time.Hour)
	if err != secrets.ErrLeaseExpired {
		t.Errorf("Expected ErrLeaseExpired, got: %v", err)
	}
}

// TestMemoryLeaseManager_RenewNotRenewable tests renewal of non-renewable lease
func TestMemoryLeaseManager_RenewNotRenewable(t *testing.T) {
	ctx := context.Background()
	lm := store.NewMemoryLeaseManager(nil)

	lease, _ := lm.CreateLease(ctx, "test/secret", time.Hour, false, 0)

	// Renewal should fail
	_, err := lm.RenewLease(ctx, lease.ID, 2*time.Hour)
	if err != secrets.ErrLeaseExpired {
		t.Errorf("Expected ErrLeaseExpired for non-renewable lease, got: %v", err)
	}
}

// TestMemoryLeaseManager_Revoke tests lease revocation
func TestMemoryLeaseManager_Revoke(t *testing.T) {
	ctx := context.Background()
	lm := store.NewMemoryLeaseManager(nil)

	lease, _ := lm.CreateLease(ctx, "test/secret", time.Hour, true, 3)

	if err := lm.RevokeLease(ctx, lease.ID); err != nil {
		t.Fatalf("RevokeLease failed: %v", err)
	}

	got, err := lm.GetLease(ctx, lease.ID)
	if err != nil {
		t.Fatalf("GetLease after revoke failed: %v", err)
	}
	if !got.Revoked {
		t.Error("Lease should be revoked")
	}

	// Renewing revoked lease should fail
	_, err = lm.RenewLease(ctx, lease.ID, time.Hour)
	if err != secrets.ErrLeaseExpired {
		t.Errorf("Expected ErrLeaseExpired for revoked lease renewal, got: %v", err)
	}
}

// TestMemoryLeaseManager_RevokeByPath tests revoking all leases for a path
func TestMemoryLeaseManager_RevokeByPath(t *testing.T) {
	ctx := context.Background()
	lm := store.NewMemoryLeaseManager(nil)

	// Create multiple leases for same path
	lease1, _ := lm.CreateLease(ctx, "test/secret", time.Hour, true, 3)
	lease2, _ := lm.CreateLease(ctx, "test/secret", time.Hour, true, 3)
	lease3, _ := lm.CreateLease(ctx, "other/secret", time.Hour, true, 3)

	if err := lm.RevokeLeasesByPath(ctx, "test/secret"); err != nil {
		t.Fatalf("RevokeLeasesByPath failed: %v", err)
	}

	// Check that leases for test/secret are revoked
	got1, _ := lm.GetLease(ctx, lease1.ID)
	if !got1.Revoked {
		t.Error("Lease1 should be revoked")
	}

	got2, _ := lm.GetLease(ctx, lease2.ID)
	if !got2.Revoked {
		t.Error("Lease2 should be revoked")
	}

	// Lease for other path should not be affected
	got3, _ := lm.GetLease(ctx, lease3.ID)
	if got3.Revoked {
		t.Error("Lease3 should not be revoked")
	}
}

// TestMemoryLeaseManager_ListLeases tests listing active leases
func TestMemoryLeaseManager_ListLeases(t *testing.T) {
	ctx := context.Background()
	lm := store.NewMemoryLeaseManager(nil)

	// Create leases
	lm.CreateLease(ctx, "test/secret1", time.Hour, true, 3)
	lease2, _ := lm.CreateLease(ctx, "test/secret2", time.Hour, true, 3)
	lm.CreateLease(ctx, "test/secret3", time.Hour, true, 3)

	// Revoke one
	lm.RevokeLease(ctx, lease2.ID)

	leases, err := lm.ListLeases(ctx)
	if err != nil {
		t.Fatalf("ListLeases failed: %v", err)
	}

	// Should only return 2 active leases
	if len(leases) != 2 {
		t.Errorf("Expected 2 active leases, got %d", len(leases))
	}
}

// TestMemoryAuditLogger tests audit logging
func TestMemoryAuditLogger_Log(t *testing.T) {
	al := store.NewMemoryAuditLogger(100)

	event := &secrets.AuditEvent{
		Timestamp: time.Now(),
		Operation: "get",
		Path:      "test/secret",
		Success:   true,
		Actor:     "test-user",
	}

	if err := al.Log(event); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	events := al.Events()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].Operation != "get" {
		t.Errorf("Operation mismatch: got %s, want get", events[0].Operation)
	}
}

// TestMemoryAuditLogger_Query tests querying audit events
func TestMemoryAuditLogger_Query(t *testing.T) {
	ctx := context.Background()
	al := store.NewMemoryAuditLogger(100)

	// Log various events
	now := time.Now()
	events := []*secrets.AuditEvent{
		{Timestamp: now.Add(-2 * time.Hour), Operation: "get", Path: "prod/db/password", Success: true, Actor: "admin"},
		{Timestamp: now.Add(-1 * time.Hour), Operation: "set", Path: "prod/db/password", Success: true, Actor: "admin"},
		{Timestamp: now, Operation: "get", Path: "staging/api/key", Success: false, Actor: "user"},
		{Timestamp: now, Operation: "delete", Path: "prod/db/old", Success: true, Actor: "admin"},
	}

	for _, e := range events {
		al.Log(e)
	}

	// Query by operation
	results, err := al.Query(ctx, secrets.AuditFilter{Operation: "get"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 get events, got %d", len(results))
	}

	// Query by path prefix
	results, err = al.Query(ctx, secrets.AuditFilter{Path: "prod/"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 prod events, got %d", len(results))
	}

	// Query by actor
	results, err = al.Query(ctx, secrets.AuditFilter{Actor: "admin"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 admin events, got %d", len(results))
	}

	// Query with time range
	results, err = al.Query(ctx, secrets.AuditFilter{StartTime: now.Add(-90 * time.Minute)})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 events in time range, got %d", len(results))
	}

	// Query with limit
	results, err = al.Query(ctx, secrets.AuditFilter{Limit: 2})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 events with limit, got %d", len(results))
	}
}

// TestMemoryAuditLogger_MaxSize tests that audit logger respects max size
func TestMemoryAuditLogger_MaxSize(t *testing.T) {
	al := store.NewMemoryAuditLogger(5)

	// Log more events than max size
	for i := 0; i < 10; i++ {
		al.Log(&secrets.AuditEvent{
			Timestamp: time.Now(),
			Operation: "test",
			Path:      "test/path",
		})
	}

	events := al.Events()
	if len(events) != 5 {
		t.Errorf("Expected 5 events (max size), got %d", len(events))
	}
}

// TestMemoryAuditLogger_Clear tests clearing audit events
func TestMemoryAuditLogger_Clear(t *testing.T) {
	al := store.NewMemoryAuditLogger(100)

	al.Log(&secrets.AuditEvent{Timestamp: time.Now(), Operation: "test", Path: "test"})
	al.Log(&secrets.AuditEvent{Timestamp: time.Now(), Operation: "test", Path: "test"})

	al.Clear()

	events := al.Events()
	if len(events) != 0 {
		t.Errorf("Expected 0 events after clear, got %d", len(events))
	}
}
