// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package secrets_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// mockGenerator implements DynamicSecretGenerator for testing.
type mockGenerator struct {
	secretType  secrets.SecretType
	generateErr error
	revokeErr   error
	revokeCalls int
}

func (g *mockGenerator) Generate(_ context.Context, path string, config map[string]interface{}) (*secrets.Secret, error) {
	if g.generateErr != nil {
		return nil, g.generateErr
	}
	return &secrets.Secret{
		Path: path,
		Type: g.secretType,
		Data: map[string][]byte{"value": []byte("generated-secret")},
	}, nil
}

func (g *mockGenerator) Revoke(_ context.Context, _ string, _ string) error {
	g.revokeCalls++
	return g.revokeErr
}

func (g *mockGenerator) Type() secrets.SecretType {
	return g.secretType
}

func TestManagerRegisterGenerator(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	gen := &mockGenerator{secretType: secrets.SecretTypeDynamic}
	mgr.RegisterGenerator(gen)

	// GenerateDynamic should work now
	secret, err := mgr.GenerateDynamic(ctx, "dynamic/test", secrets.SecretTypeDynamic, nil)
	if err != nil {
		t.Fatalf("GenerateDynamic: %v", err)
	}
	if string(secret.Data["value"]) != "generated-secret" {
		t.Errorf("unexpected data: %s", string(secret.Data["value"]))
	}

	// Verify it was stored
	got, err := mgr.Get(ctx, "dynamic/test")
	if err != nil {
		t.Fatalf("Get after generate: %v", err)
	}
	if string(got.Data["value"]) != "generated-secret" {
		t.Errorf("stored data mismatch: %s", string(got.Data["value"]))
	}
}

func TestManagerGenerateDynamic_NoGenerator(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	_, err = mgr.GenerateDynamic(ctx, "dynamic/test", secrets.SecretTypeDynamic, nil)
	if err == nil {
		t.Fatal("expected error for unregistered generator")
	}
}

func TestManagerGenerateDynamic_GeneratorFails(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	gen := &mockGenerator{
		secretType:  secrets.SecretTypeDynamic,
		generateErr: errors.New("generation failed"),
	}
	mgr.RegisterGenerator(gen)

	_, err = mgr.GenerateDynamic(ctx, "dynamic/fail", secrets.SecretTypeDynamic, nil)
	if err == nil {
		t.Fatal("expected error from failed generator")
	}
}

func TestManagerGenerateDynamic_Closed(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	gen := &mockGenerator{secretType: secrets.SecretTypeDynamic}
	mgr.RegisterGenerator(gen)

	mgr.Close()

	_, err = mgr.GenerateDynamic(context.Background(), "dynamic/closed", secrets.SecretTypeDynamic, nil)
	if !errors.Is(err, secrets.ErrStoreClosed) {
		t.Errorf("expected ErrStoreClosed, got: %v", err)
	}
}

func TestManagerWatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	al := store.NewMemoryAuditLogger(100)
	mgr, err := secrets.NewManager(secrets.ManagerConfig{
		Store: ms,
		Audit: al,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	ch, err := mgr.Watch(ctx, "watched/secret")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Set secret in background to trigger watch
	go func() {
		time.Sleep(100 * time.Millisecond)
		s := &secrets.Secret{
			Path: "watched/secret",
			Type: secrets.SecretTypeStatic,
			Data: map[string][]byte{"value": []byte("watched-value")},
		}
		mgr.Set(ctx, s.Path, s)
	}()

	select {
	case got := <-ch:
		if got == nil {
			t.Error("received nil from watch channel")
		}
	case <-ctx.Done():
		t.Error("timeout waiting for watch update")
	}

	// Verify audit logged the watch operation
	events := al.Events()
	foundWatch := false
	for _, e := range events {
		if e.Operation == "watch" {
			foundWatch = true
			break
		}
	}
	if !foundWatch {
		t.Error("expected watch audit event")
	}
}

func TestManagerWatch_Closed(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	mgr.Close()

	_, err = mgr.Watch(context.Background(), "any/path")
	if !errors.Is(err, secrets.ErrStoreClosed) {
		t.Errorf("expected ErrStoreClosed, got: %v", err)
	}
}

func TestManagerCloseIdempotent(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// First close should succeed
	if err := mgr.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Second close should be a no-op
	if err := mgr.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestManagerOperationsAfterClose(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	mgr.Close()

	// All operations should return ErrStoreClosed
	_, err = mgr.Get(ctx, "test")
	if !errors.Is(err, secrets.ErrStoreClosed) {
		t.Errorf("Get after close: expected ErrStoreClosed, got %v", err)
	}

	err = mgr.Set(ctx, "test", &secrets.Secret{Path: "test", Data: map[string][]byte{"v": []byte("x")}})
	if !errors.Is(err, secrets.ErrStoreClosed) {
		t.Errorf("Set after close: expected ErrStoreClosed, got %v", err)
	}

	err = mgr.Delete(ctx, "test")
	if !errors.Is(err, secrets.ErrStoreClosed) {
		t.Errorf("Delete after close: expected ErrStoreClosed, got %v", err)
	}

	_, err = mgr.List(ctx, "")
	if !errors.Is(err, secrets.ErrStoreClosed) {
		t.Errorf("List after close: expected ErrStoreClosed, got %v", err)
	}
}

func TestManagerGetWithLease_NoLeaseManager(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	_, err = mgr.GetWithLease(ctx, "some-lease-id")
	if err == nil {
		t.Error("expected error when lease manager not configured")
	}
}

func TestManagerCreateLease_NoLeaseManager(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	_, err = mgr.CreateLease(ctx, "test/path", time.Hour, true, 3)
	if err == nil {
		t.Error("expected error when lease manager not configured")
	}
}

func TestManagerGetWithLease_RevokedLease(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	lm := store.NewMemoryLeaseManager(nil)
	mgr, err := secrets.NewManager(secrets.ManagerConfig{
		Store:  ms,
		Leases: lm,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	// Store a secret
	s := &secrets.Secret{
		Path: "test/lease",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("leased")},
	}
	if err := mgr.Set(ctx, s.Path, s); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Create and revoke a lease
	lease, err := mgr.CreateLease(ctx, s.Path, time.Hour, true, 3)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}

	if err := lm.RevokeLease(ctx, lease.ID); err != nil {
		t.Fatalf("RevokeLease: %v", err)
	}

	_, err = mgr.GetWithLease(ctx, lease.ID)
	if !errors.Is(err, secrets.ErrLeaseExpired) {
		t.Errorf("expected ErrLeaseExpired for revoked lease, got: %v", err)
	}
}

func TestManagerNewManager_NilStore(t *testing.T) {
	_, err := secrets.NewManager(secrets.ManagerConfig{})
	if err == nil {
		t.Error("expected error with nil store")
	}
}

func TestGenerateRandomBytes(t *testing.T) {
	b, err := secrets.GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes: %v", err)
	}
	if len(b) != 32 {
		t.Errorf("length = %d, want 32", len(b))
	}

	// Two calls should produce different results
	b2, _ := secrets.GenerateRandomBytes(32)
	same := true
	for i := range b {
		if b[i] != b2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("two random byte slices should differ")
	}
}

func TestGenerateLeaseID(t *testing.T) {
	id, err := secrets.GenerateLeaseID()
	if err != nil {
		t.Fatalf("GenerateLeaseID: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("length = %d, want 32", len(id))
	}
}

func TestManagerDeleteWithLeases(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	lm := store.NewMemoryLeaseManager(nil)
	mgr, err := secrets.NewManager(secrets.ManagerConfig{
		Store:  ms,
		Leases: lm,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	// Store a secret and create a lease
	s := &secrets.Secret{
		Path: "test/delete-lease",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("to-delete")},
	}
	if err := mgr.Set(ctx, s.Path, s); err != nil {
		t.Fatalf("Set: %v", err)
	}

	lease, err := mgr.CreateLease(ctx, s.Path, time.Hour, true, 3)
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}

	// Delete should also revoke leases
	if err := mgr.Delete(ctx, s.Path); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Lease should be revoked
	got, err := lm.GetLease(ctx, lease.ID)
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if !got.Revoked {
		t.Error("lease should be revoked after secret deletion")
	}
}

func TestSecretClone_Nil(t *testing.T) {
	var s *secrets.Secret
	clone := s.Clone()
	if clone != nil {
		t.Error("Clone of nil should return nil")
	}
}

func TestManagerCreateLease_SecretNotFound(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	lm := store.NewMemoryLeaseManager(nil)
	mgr, err := secrets.NewManager(secrets.ManagerConfig{
		Store:  ms,
		Leases: lm,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	_, err = mgr.CreateLease(ctx, "nonexistent/path", time.Hour, true, 3)
	if err == nil {
		t.Error("expected error for non-existent secret")
	}
}
