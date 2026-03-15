// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package secrets_test

import (
	"context"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/encryption"
	"unheaded/pkg/secrets/rotation"
	"unheaded/pkg/secrets/store"
)

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	// Test Set and Get
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

	if err := ms.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

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

	// Test List
	paths, err := ms.List(ctx, "test/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(paths) != 1 || paths[0] != "test/secret" {
		t.Errorf("List returned unexpected paths: %v", paths)
	}

	// Test Delete
	if err := ms.Delete(ctx, secret.Path); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = ms.Get(ctx, secret.Path)
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Expected ErrSecretNotFound, got: %v", err)
	}
}

func TestMemoryStoreWatch(t *testing.T) {
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

func TestSecretClone(t *testing.T) {
	original := &secrets.Secret{
		Path: "original/path",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{
			"value": []byte("original-value"),
		},
		Metadata: map[string]string{
			"key": "value",
		},
		Version: 1,
	}

	clone := original.Clone()

	// Modify clone
	clone.Data["value"] = []byte("modified-value")
	clone.Metadata["key"] = "modified"
	clone.Version = 2

	// Original should be unchanged
	if string(original.Data["value"]) != "original-value" {
		t.Error("Original Data was modified")
	}
	if original.Metadata["key"] != "value" {
		t.Error("Original Metadata was modified")
	}
	if original.Version != 1 {
		t.Error("Original Version was modified")
	}
}

func TestSecretExpiration(t *testing.T) {
	// Non-expired secret
	secret := &secrets.Secret{
		Path:      "test/secret",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if secret.IsExpired() {
		t.Error("Secret should not be expired")
	}

	// Expired secret
	secret.ExpiresAt = time.Now().Add(-time.Hour)
	if !secret.IsExpired() {
		t.Error("Secret should be expired")
	}

	// No expiration
	secret.ExpiresAt = time.Time{}
	if secret.IsExpired() {
		t.Error("Secret with zero expiration should not be expired")
	}
}

func TestLeaseExpiration(t *testing.T) {
	// Non-expired lease
	lease := &secrets.Lease{
		ID:          "lease-1",
		ExpiresAt:   time.Now().Add(time.Hour),
		Renewable:   true,
		MaxRenewals: 3,
	}
	if lease.IsExpired() {
		t.Error("Lease should not be expired")
	}
	if !lease.CanRenew() {
		t.Error("Lease should be renewable")
	}

	// Expired lease
	lease.ExpiresAt = time.Now().Add(-time.Hour)
	if !lease.IsExpired() {
		t.Error("Lease should be expired")
	}

	// Revoked lease
	lease.ExpiresAt = time.Now().Add(time.Hour)
	lease.Revoked = true
	if lease.CanRenew() {
		t.Error("Revoked lease should not be renewable")
	}
}

func TestMemoryLeaseManager(t *testing.T) {
	ctx := context.Background()
	lm := store.NewMemoryLeaseManager(nil)

	// Create lease
	lease, err := lm.CreateLease(ctx, "test/secret", time.Hour, true, 3)
	if err != nil {
		t.Fatalf("CreateLease failed: %v", err)
	}

	if lease.SecretPath != "test/secret" {
		t.Errorf("Unexpected secret path: %s", lease.SecretPath)
	}

	// Get lease
	got, err := lm.GetLease(ctx, lease.ID)
	if err != nil {
		t.Fatalf("GetLease failed: %v", err)
	}
	if got.ID != lease.ID {
		t.Errorf("Lease ID mismatch: got %s, want %s", got.ID, lease.ID)
	}

	// Renew lease
	renewed, err := lm.RenewLease(ctx, lease.ID, 2*time.Hour)
	if err != nil {
		t.Fatalf("RenewLease failed: %v", err)
	}
	if renewed.RenewCount != 1 {
		t.Errorf("RenewCount mismatch: got %d, want 1", renewed.RenewCount)
	}

	// Revoke lease
	if err := lm.RevokeLease(ctx, lease.ID); err != nil {
		t.Fatalf("RevokeLease failed: %v", err)
	}

	got, err = lm.GetLease(ctx, lease.ID)
	if err != nil {
		t.Fatalf("GetLease after revoke failed: %v", err)
	}
	if !got.Revoked {
		t.Error("Lease should be revoked")
	}
}

func TestSimpleEncryptor(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	enc, err := encryption.NewSimpleEncryptor(key)
	if err != nil {
		t.Fatalf("NewSimpleEncryptor failed: %v", err)
	}

	plaintext := []byte("secret data to encrypt")

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Ciphertext should be different from plaintext
	if string(ciphertext) == string(plaintext) {
		t.Error("Ciphertext should be different from plaintext")
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted mismatch: got %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestSimpleEncryptorFromPassword(t *testing.T) {
	enc, err := encryption.NewSimpleEncryptorFromPassword("my-secret-password", nil)
	if err != nil {
		t.Fatalf("NewSimpleEncryptorFromPassword failed: %v", err)
	}

	plaintext := []byte("password-protected secret")

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted mismatch: got %s, want %s", string(decrypted), string(plaintext))
	}

	// Different password should fail
	enc2, _ := encryption.NewSimpleEncryptorFromPassword("wrong-password", nil)
	_, err = enc2.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decrypt with wrong password should fail")
	}
}

func TestEnvelopeEncryption(t *testing.T) {
	ctx := context.Background()

	// Create KEK provider
	kekProvider, err := encryption.NewLocalKEKProvider()
	if err != nil {
		t.Fatalf("NewLocalKEKProvider failed: %v", err)
	}

	// Create envelope encryptor
	enc, err := encryption.NewEnvelopeEncryptor(encryption.EnvelopeConfig{
		KEKProvider: kekProvider,
	})
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor failed: %v", err)
	}

	plaintext := []byte("envelope-encrypted secret data")

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted mismatch: got %s, want %s", string(decrypted), string(plaintext))
	}

	// Rotate KEK
	newKEKID, err := kekProvider.RotateKEK(ctx)
	if err != nil {
		t.Fatalf("RotateKEK failed: %v", err)
	}
	if newKEKID == "" {
		t.Error("New KEK ID should not be empty")
	}

	// Old ciphertext should still decrypt (old KEK is still available)
	decrypted, err = enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt after KEK rotation failed: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Error("Decryption after KEK rotation failed")
	}
}

func TestStaticSecretRotation(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	// Create initial secret
	secret := &secrets.Secret{
		Path: "test/password",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{
			"value": []byte("original-password"),
		},
	}
	if err := ms.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Create rotation manager
	rm, err := rotation.NewRotationManager(rotation.RotationManagerConfig{
		Store: ms,
	})
	if err != nil {
		t.Fatalf("NewRotationManager failed: %v", err)
	}

	// Rotate
	result, err := rm.Rotate(ctx, secret.Path, rotation.RotationConfig{
		TTL: 24 * time.Hour,
		Parameters: map[string]interface{}{
			"length":  16,
			"charset": "abcdefghijklmnopqrstuvwxyz",
		},
	})
	if err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("Rotation not successful: %v", result.Error)
	}

	// Verify new secret
	newSecret, err := ms.Get(ctx, secret.Path)
	if err != nil {
		t.Fatalf("Get after rotation failed: %v", err)
	}

	newValue := string(newSecret.Data["value"])
	if len(newValue) != 16 {
		t.Errorf("New value length: got %d, want 16", len(newValue))
	}

	if newValue == "original-password" {
		t.Error("Value should have changed after rotation")
	}
}

func TestSSHKeyRotation(t *testing.T) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	// Create initial SSH key secret
	secret := &secrets.Secret{
		Path: "test/ssh-key",
		Type: secrets.SecretTypeSSHKey,
		Data: map[string][]byte{
			"private": []byte("dummy-private"),
			"public":  []byte("dummy-public"),
		},
	}
	if err := ms.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Create rotation manager
	rm, err := rotation.NewRotationManager(rotation.RotationManagerConfig{
		Store: ms,
	})
	if err != nil {
		t.Fatalf("NewRotationManager failed: %v", err)
	}

	// Rotate
	result, err := rm.Rotate(ctx, secret.Path, rotation.RotationConfig{
		Parameters: map[string]interface{}{
			"key_type": "ed25519",
			"comment":  "test@example.com",
		},
	})
	if err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("Rotation not successful: %v", result.Error)
	}

	// Verify new key
	newSecret, err := ms.Get(ctx, secret.Path)
	if err != nil {
		t.Fatalf("Get after rotation failed: %v", err)
	}

	privateKey := string(newSecret.Data["private"])
	publicKey := string(newSecret.Data["public"])

	if len(privateKey) == 0 {
		t.Error("Private key should not be empty")
	}
	if len(publicKey) == 0 {
		t.Error("Public key should not be empty")
	}
}

func TestManager(t *testing.T) {
	ctx := context.Background()

	// Create stores
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	lm := store.NewMemoryLeaseManager(nil)
	al := store.NewMemoryAuditLogger(100)

	// Create manager
	mgr, err := secrets.NewManager(secrets.ManagerConfig{
		Store:  ms,
		Leases: lm,
		Audit:  al,
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.Close()

	// Set secret
	secret := &secrets.Secret{
		Path: "manager/test",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{
			"api_key": []byte("test-api-key"),
		},
	}

	if err := mgr.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get secret
	got, err := mgr.Get(ctx, secret.Path)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got.Data["api_key"]) != "test-api-key" {
		t.Errorf("Data mismatch: got %s", string(got.Data["api_key"]))
	}

	// Check version
	if got.Version != 1 {
		t.Errorf("Version mismatch: got %d, want 1", got.Version)
	}

	// Update secret
	secret.Data["api_key"] = []byte("updated-key")
	if err := mgr.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ = mgr.Get(ctx, secret.Path)
	if got.Version != 2 {
		t.Errorf("Version after update: got %d, want 2", got.Version)
	}

	// List secrets
	paths, err := mgr.List(ctx, "manager/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(paths) != 1 {
		t.Errorf("List count: got %d, want 1", len(paths))
	}

	// Create lease
	lease, err := mgr.CreateLease(ctx, secret.Path, time.Hour, true, 3)
	if err != nil {
		t.Fatalf("CreateLease failed: %v", err)
	}

	// Get with lease
	got, err = mgr.GetWithLease(ctx, lease.ID)
	if err != nil {
		t.Fatalf("GetWithLease failed: %v", err)
	}
	if string(got.Data["api_key"]) != "updated-key" {
		t.Error("GetWithLease returned wrong data")
	}

	// Delete secret
	if err := mgr.Delete(ctx, secret.Path); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = mgr.Get(ctx, secret.Path)
	if err != secrets.ErrSecretNotFound {
		t.Errorf("Get after delete: expected ErrSecretNotFound, got %v", err)
	}

	// Check audit log
	events := al.Events()
	if len(events) == 0 {
		t.Error("Expected audit events")
	}

	// Should have: set, get, set, list, get (via lease), delete, get (failed)
	operations := make(map[string]int)
	for _, e := range events {
		operations[e.Operation]++
	}

	if operations["set"] < 2 {
		t.Error("Expected at least 2 set operations in audit log")
	}
	if operations["get"] < 3 {
		t.Error("Expected at least 3 get operations in audit log")
	}
}

func TestGenerateRandomString(t *testing.T) {
	s1, err := secrets.GenerateRandomString(32)
	if err != nil {
		t.Fatalf("GenerateRandomString failed: %v", err)
	}

	if len(s1) != 32 {
		t.Errorf("Length mismatch: got %d, want 32", len(s1))
	}

	s2, err := secrets.GenerateRandomString(32)
	if err != nil {
		t.Fatalf("GenerateRandomString failed: %v", err)
	}

	if s1 == s2 {
		t.Error("Two random strings should not be equal")
	}
}

func TestSecretRevokedAndExpired(t *testing.T) {
	ctx := context.Background()

	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	mgr, err := secrets.NewManager(secrets.ManagerConfig{
		Store: ms,
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.Close()

	// Test expired secret
	expiredSecret := &secrets.Secret{
		Path:      "test/expired",
		Type:      secrets.SecretTypeStatic,
		Data:      map[string][]byte{"value": []byte("expired-value")},
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if err := ms.Set(ctx, expiredSecret.Path, expiredSecret); err != nil {
		t.Fatalf("Set expired secret failed: %v", err)
	}

	_, err = mgr.Get(ctx, expiredSecret.Path)
	if err != secrets.ErrSecretExpired {
		t.Errorf("Expected ErrSecretExpired, got: %v", err)
	}

	// Test revoked secret
	revokedSecret := &secrets.Secret{
		Path:    "test/revoked",
		Type:    secrets.SecretTypeStatic,
		Data:    map[string][]byte{"value": []byte("revoked-value")},
		Revoked: true,
	}
	if err := ms.Set(ctx, revokedSecret.Path, revokedSecret); err != nil {
		t.Fatalf("Set revoked secret failed: %v", err)
	}

	_, err = mgr.Get(ctx, revokedSecret.Path)
	if err != secrets.ErrSecretRevoked {
		t.Errorf("Expected ErrSecretRevoked, got: %v", err)
	}
}

func TestRotationScheduler(t *testing.T) {
	ctx := context.Background()

	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	// Create initial secret
	secret := &secrets.Secret{
		Path: "scheduled/secret",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("scheduled-value")},
	}
	if err := ms.Set(ctx, secret.Path, secret); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Create rotation manager
	rm, err := rotation.NewRotationManager(rotation.RotationManagerConfig{
		Store: ms,
	})
	if err != nil {
		t.Fatalf("NewRotationManager failed: %v", err)
	}

	// Create scheduler
	scheduler, err := rotation.NewScheduler(rotation.SchedulerConfig{
		Manager: rm,
		Store:   ms,
	})
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	// Add schedule
	schedule := &rotation.Schedule{
		Path:     secret.Path,
		Interval: 24 * time.Hour,
		Config: rotation.RotationConfig{
			Parameters: map[string]interface{}{
				"length": 32,
			},
		},
		Enabled: true,
	}

	if err := scheduler.AddSchedule(schedule); err != nil {
		t.Fatalf("AddSchedule failed: %v", err)
	}

	// Get schedule
	got, ok := scheduler.GetSchedule(secret.Path)
	if !ok {
		t.Fatal("GetSchedule returned false")
	}
	if got.Path != secret.Path {
		t.Errorf("Schedule path mismatch: got %s", got.Path)
	}

	// List schedules
	schedules := scheduler.ListSchedules()
	if len(schedules) != 1 {
		t.Errorf("ListSchedules count: got %d, want 1", len(schedules))
	}

	// Trigger rotation
	result, err := scheduler.TriggerRotation(secret.Path)
	if err != nil {
		t.Fatalf("TriggerRotation failed: %v", err)
	}
	if !result.Success {
		t.Errorf("TriggerRotation not successful: %v", result.Error)
	}

	// Disable schedule
	if err := scheduler.DisableSchedule(secret.Path); err != nil {
		t.Fatalf("DisableSchedule failed: %v", err)
	}

	got, _ = scheduler.GetSchedule(secret.Path)
	if got.Enabled {
		t.Error("Schedule should be disabled")
	}

	// Enable schedule
	if err := scheduler.EnableSchedule(secret.Path); err != nil {
		t.Fatalf("EnableSchedule failed: %v", err)
	}

	got, _ = scheduler.GetSchedule(secret.Path)
	if !got.Enabled {
		t.Error("Schedule should be enabled")
	}

	// Remove schedule
	scheduler.RemoveSchedule(secret.Path)
	_, ok = scheduler.GetSchedule(secret.Path)
	if ok {
		t.Error("Schedule should be removed")
	}
}

func BenchmarkMemoryStoreGet(b *testing.B) {
	ctx := context.Background()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	defer ms.Close()

	secret := &secrets.Secret{
		Path: "bench/secret",
		Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("benchmark-value")},
	}
	ms.Set(ctx, secret.Path, secret)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ms.Get(ctx, secret.Path)
	}
}

func BenchmarkSimpleEncryption(b *testing.B) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	enc, _ := encryption.NewSimpleEncryptor(key)
	plaintext := []byte("benchmark encryption data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ciphertext, _ := enc.Encrypt(plaintext)
		enc.Decrypt(ciphertext)
	}
}

func BenchmarkEnvelopeEncryption(b *testing.B) {
	kekProvider, _ := encryption.NewLocalKEKProvider()
	enc, _ := encryption.NewEnvelopeEncryptor(encryption.EnvelopeConfig{
		KEKProvider: kekProvider,
	})

	plaintext := []byte("benchmark envelope encryption data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ciphertext, _ := enc.Encrypt(plaintext)
		enc.Decrypt(ciphertext)
	}
}
