// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rotation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/secrets/store"
)

func newKeyManager(t *testing.T) *KeyManager {
	t.Helper()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	km, err := NewKeyManager(KeyManagerConfig{Store: ms})
	if err != nil {
		t.Fatalf("new key manager: %v", err)
	}
	return km
}

func TestNewKeyManager_NilStore(t *testing.T) {
	_, err := NewKeyManager(KeyManagerConfig{})
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestNewKeyManager_OK(t *testing.T) {
	km := newKeyManager(t)
	if km == nil {
		t.Fatal("expected non-nil manager")
	}
}

// --- CreateKey ---

func TestCreateKey(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()

	mk, err := km.CreateKey(ctx, "master", "Master Key", "encryption", 32)
	if err != nil {
		t.Fatal(err)
	}
	if mk.ID != "master" {
		t.Fatalf("expected id master, got %s", mk.ID)
	}
	if mk.CurrentVersion != 1 {
		t.Fatalf("expected version 1, got %d", mk.CurrentVersion)
	}
	if mk.Versions[1].Status != KeyStatusActive {
		t.Fatal("expected active status")
	}
	if len(mk.Versions[1].KeyData) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(mk.Versions[1].KeyData))
	}
}

func TestCreateKey_Duplicate(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()

	_, err := km.CreateKey(ctx, "dup", "Key", "enc", 32)
	if err != nil {
		t.Fatal(err)
	}
	_, err = km.CreateKey(ctx, "dup", "Key2", "enc", 32)
	if err == nil {
		t.Fatal("expected error for duplicate key")
	}
}

// --- GetKey ---

func TestGetKey(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()

	km.CreateKey(ctx, "get-test", "Test", "enc", 32)

	mk, err := km.GetKey("get-test")
	if err != nil {
		t.Fatal(err)
	}
	if mk.ID != "get-test" {
		t.Fatalf("unexpected id: %s", mk.ID)
	}
}

func TestGetKey_NotFound(t *testing.T) {
	km := newKeyManager(t)
	_, err := km.GetKey("nope")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got: %v", err)
	}
}

// --- GetKeyVersion ---

func TestGetKeyVersion(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "ver", "V", "enc", 32)

	kv, err := km.GetKeyVersion("ver", 1)
	if err != nil {
		t.Fatal(err)
	}
	if kv.Version != 1 {
		t.Fatal("wrong version")
	}
}

func TestGetKeyVersion_NotFound(t *testing.T) {
	km := newKeyManager(t)
	_, err := km.GetKeyVersion("nope", 1)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatal("expected ErrKeyNotFound")
	}
}

func TestGetKeyVersion_InvalidVersion(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "inv", "I", "enc", 32)

	_, err := km.GetKeyVersion("inv", 99)
	if !errors.Is(err, ErrInvalidKeyVersion) {
		t.Fatal("expected ErrInvalidKeyVersion")
	}
}

func TestGetKeyVersion_Revoked(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "rev", "R", "enc", 32)
	km.RevokeKeyVersion(ctx, "rev", 1, "test")

	_, err := km.GetKeyVersion("rev", 1)
	if !errors.Is(err, ErrKeyRevoked) {
		t.Fatalf("expected ErrKeyRevoked, got: %v", err)
	}
}

func TestGetKeyVersion_Expired(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "exp", "E", "enc", 32)

	// Manually set expiry in the past
	km.mu.Lock()
	km.keys["exp"].Versions[1].ExpiresAt = time.Now().Add(-time.Minute)
	km.mu.Unlock()

	_, err := km.GetKeyVersion("exp", 1)
	if !errors.Is(err, ErrKeyExpired) {
		t.Fatalf("expected ErrKeyExpired, got: %v", err)
	}
}

// --- GetCurrentKeyMaterial ---

func TestGetCurrentKeyMaterial(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "mat", "M", "enc", 32)

	data, err := km.GetCurrentKeyMaterial("mat")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(data))
	}
}

func TestGetCurrentKeyMaterial_NotFound(t *testing.T) {
	km := newKeyManager(t)
	_, err := km.GetCurrentKeyMaterial("nope")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatal("expected ErrKeyNotFound")
	}
}

// --- RotateKey ---

func TestRotateKey(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "rot", "R", "enc", 32)

	kv, err := km.RotateKey(ctx, "rot", 32)
	if err != nil {
		t.Fatal(err)
	}
	if kv.Version != 2 {
		t.Fatalf("expected version 2, got %d", kv.Version)
	}
	if kv.Status != KeyStatusActive {
		t.Fatal("new version should be active")
	}

	// Old version should be rotating
	km.mu.RLock()
	oldV := km.keys["rot"].Versions[1]
	km.mu.RUnlock()
	if oldV.Status != KeyStatusRotating {
		t.Fatalf("old version should be rotating, got %s", oldV.Status)
	}
}

func TestRotateKey_NotFound(t *testing.T) {
	km := newKeyManager(t)
	_, err := km.RotateKey(context.Background(), "nope", 32)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatal("expected ErrKeyNotFound")
	}
}

// --- RevokeKeyVersion ---

func TestRevokeKeyVersion(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "rk", "RK", "enc", 32)

	err := km.RevokeKeyVersion(ctx, "rk", 1, "compromised")
	if err != nil {
		t.Fatal(err)
	}

	// Verify key data is cleared
	km.mu.RLock()
	kv := km.keys["rk"].Versions[1]
	km.mu.RUnlock()
	if kv.KeyData != nil {
		t.Fatal("key data should be nil after revocation")
	}
	if kv.RevocationReason != "compromised" {
		t.Fatal("wrong revocation reason")
	}
}

func TestRevokeKeyVersion_AlreadyRevoked(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "rk2", "RK2", "enc", 32)
	km.RevokeKeyVersion(ctx, "rk2", 1, "first")

	// Revoking again should be a no-op
	err := km.RevokeKeyVersion(ctx, "rk2", 1, "second")
	if err != nil {
		t.Fatal(err)
	}
}

// --- RevokeAllVersions ---

func TestRevokeAllVersions(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "ra", "RA", "enc", 32)
	km.RotateKey(ctx, "ra", 32)
	km.RotateKey(ctx, "ra", 32)

	err := km.RevokeAllVersions(ctx, "ra", "incident")
	if err != nil {
		t.Fatal(err)
	}

	km.mu.RLock()
	for v, kv := range km.keys["ra"].Versions {
		if kv.Status != KeyStatusRevoked {
			t.Fatalf("version %d not revoked", v)
		}
	}
	km.mu.RUnlock()
}

// --- DeriveKey ---

func TestDeriveKey(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "dk", "DK", "enc", 32)

	key1, err := km.DeriveKey("dk", "service-a", 32)
	if err != nil {
		t.Fatal(err)
	}
	if len(key1) != 32 {
		t.Fatal("wrong key length")
	}

	// Same context should return same key (cached)
	key2, err := km.DeriveKey("dk", "service-a", 32)
	if err != nil {
		t.Fatal(err)
	}
	if string(key1) != string(key2) {
		t.Fatal("derived keys should match for same context")
	}

	// Different context should return different key
	key3, err := km.DeriveKey("dk", "service-b", 32)
	if err != nil {
		t.Fatal(err)
	}
	if string(key1) == string(key3) {
		t.Fatal("derived keys should differ for different contexts")
	}
}

func TestDeriveKeyForService(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "dks", "DKS", "enc", 32)

	key, err := km.DeriveKeyForService("dks", "timeguru", 32)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatal("wrong length")
	}
}

func TestDeriveKey_NotFound(t *testing.T) {
	km := newKeyManager(t)
	_, err := km.DeriveKey("nope", "ctx", 32)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ListKeys ---

func TestListKeys(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "l1", "L1", "enc", 32)
	km.CreateKey(ctx, "l2", "L2", "enc", 32)

	keys := km.ListKeys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestListKeys_Empty(t *testing.T) {
	km := newKeyManager(t)
	keys := km.ListKeys()
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}

// --- GetKeyHistory ---

func TestGetKeyHistory(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "hist", "H", "enc", 32)
	km.RotateKey(ctx, "hist", 32)

	versions, err := km.GetKeyHistory("hist")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	// Should be sorted by version
	if versions[0].Version > versions[1].Version {
		t.Fatal("versions not sorted")
	}
	// Key data should be stripped
	for _, v := range versions {
		if v.KeyData != nil {
			t.Fatal("key data should be nil in history")
		}
	}
}

func TestGetKeyHistory_NotFound(t *testing.T) {
	km := newKeyManager(t)
	_, err := km.GetKeyHistory("nope")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatal("expected ErrKeyNotFound")
	}
}

// --- hashKey ---

func TestHashKey(t *testing.T) {
	h1 := hashKey([]byte("test"))
	h2 := hashKey([]byte("test"))
	if h1 != h2 {
		t.Fatal("same input should produce same hash")
	}
	h3 := hashKey([]byte("other"))
	if h1 == h3 {
		t.Fatal("different input should produce different hash")
	}
}

// --- MemoryEscrowProvider ---

func TestMemoryEscrowProvider(t *testing.T) {
	ep := NewMemoryEscrowProvider()
	ctx := context.Background()

	err := ep.Store(ctx, "key1", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	data, err := ep.Retrieve(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "secret" {
		t.Fatal("wrong data")
	}

	err = ep.Delete(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}

	_, err = ep.Retrieve(ctx, "key1")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatal("expected ErrKeyNotFound after delete")
	}
}

func TestMemoryEscrowProvider_NotFound(t *testing.T) {
	ep := NewMemoryEscrowProvider()
	_, err := ep.Retrieve(context.Background(), "missing")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatal("expected ErrKeyNotFound")
	}
}

func TestMemoryEscrowProvider_DeleteNonexistent(t *testing.T) {
	ep := NewMemoryEscrowProvider()
	err := ep.Delete(context.Background(), "nope")
	if err != nil {
		t.Fatal("delete of nonexistent key should not error")
	}
}

func TestMemoryEscrowProvider_Concurrent(t *testing.T) {
	ep := NewMemoryEscrowProvider()
	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := string(rune('a' + idx%26))
			_ = ep.Store(ctx, key, []byte("data"))
			_, _ = ep.Retrieve(ctx, key)
		}(i)
	}
	wg.Wait()
}

// --- Escrow integration with KeyManager ---

func TestKeyManager_WithEscrow(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	escrow := NewMemoryEscrowProvider()

	km, err := NewKeyManager(KeyManagerConfig{
		Store:  ms,
		Escrow: escrow,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Create key with escrow enabled later
	mk, err := km.CreateKey(ctx, "escrow-key", "Escrow Test", "enc", 32)
	if err != nil {
		t.Fatal(err)
	}

	// Enable escrow
	mk.EscrowEnabled = true
	err = km.EnableEscrow(ctx, "escrow-key")
	if err != nil {
		t.Fatal(err)
	}

	// Rotate should also escrow
	_, err = km.RotateKey(ctx, "escrow-key", 32)
	if err != nil {
		t.Fatal(err)
	}
}

func TestKeyManager_EnableEscrow_NoProvider(t *testing.T) {
	km := newKeyManager(t)
	ctx := context.Background()
	km.CreateKey(ctx, "no-esc", "NE", "enc", 32)

	err := km.EnableEscrow(ctx, "no-esc")
	if !errors.Is(err, ErrEscrowNotEnabled) {
		t.Fatal("expected ErrEscrowNotEnabled")
	}
}

func TestKeyManager_RecoverKey_NoProvider(t *testing.T) {
	km := newKeyManager(t)
	_, err := km.RecoverKey(context.Background(), "any")
	if !errors.Is(err, ErrEscrowNotEnabled) {
		t.Fatal("expected ErrEscrowNotEnabled")
	}
}

func TestKeyManager_RecoverKey(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	escrow := NewMemoryEscrowProvider()
	km, _ := NewKeyManager(KeyManagerConfig{Store: ms, Escrow: escrow})
	ctx := context.Background()

	// Store something in escrow directly
	escrow.Store(ctx, "recover-id", []byte("recovered-secret"))

	data, err := km.RecoverKey(ctx, "recover-id")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "recovered-secret" {
		t.Fatal("wrong recovered data")
	}
}

// --- FileEscrowProvider ---

func TestNewFileEscrowProvider_Validation(t *testing.T) {
	_, err := NewFileEscrowProvider(FileEscrowConfig{})
	if err == nil {
		t.Fatal("expected error for empty base path")
	}

	_, err = NewFileEscrowProvider(FileEscrowConfig{
		BasePath:   t.TempDir(),
		EncryptKey: []byte("short"),
	})
	if err == nil {
		t.Fatal("expected error for short encrypt key")
	}
}

func TestFileEscrowProvider_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	ep, err := NewFileEscrowProvider(FileEscrowConfig{
		BasePath:   dir,
		EncryptKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	err = ep.Store(ctx, "fk1", []byte("file-secret-data"))
	if err != nil {
		t.Fatal(err)
	}

	data, err := ep.Retrieve(ctx, "fk1")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "file-secret-data" {
		t.Fatalf("expected file-secret-data, got %s", string(data))
	}

	err = ep.Delete(ctx, "fk1")
	if err != nil {
		t.Fatal(err)
	}

	_, err = ep.Retrieve(ctx, "fk1")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatal("expected ErrKeyNotFound after delete")
	}
}

func TestFileEscrowProvider_DeleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	ep, _ := NewFileEscrowProvider(FileEscrowConfig{BasePath: dir, EncryptKey: key})

	err := ep.Delete(context.Background(), "nope")
	if err != nil {
		t.Fatal("delete of nonexistent should not error")
	}
}
