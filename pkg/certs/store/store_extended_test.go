// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMemoryStore_Cleanup verifies that the cleanup method removes
// certificates that expired beyond the RetainExpired window.
func TestMemoryStore_Cleanup(t *testing.T) {
	cfg := &Config{
		CleanupInterval: 0, // no background goroutine
		RetainExpired:   1 * time.Hour,
	}
	s := NewMemoryStore(cfg)
	defer s.Close()
	ctx := context.Background()

	longExpired := time.Now().Add(-2 * time.Hour) // expired 2h ago, beyond 1h retain
	recentExpired := time.Now().Add(-30 * time.Minute) // expired 30m ago, within 1h retain
	notExpired := time.Now().Add(24 * time.Hour)

	s.Put(ctx, "old", &CertificateData{Serial: "old", ExpiresAt: longExpired})
	s.Put(ctx, "recent", &CertificateData{Serial: "recent", ExpiresAt: recentExpired})
	s.Put(ctx, "active", &CertificateData{Serial: "active", ExpiresAt: notExpired})

	// Also store a non-Certificate value to ensure it survives cleanup
	s.Put(ctx, "plain", "not-a-certificate")

	s.cleanup()

	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// "old" should be removed, "recent", "active", and "plain" should remain
	if len(list) != 3 {
		t.Errorf("after cleanup: got %d certs, want 3", len(list))
	}

	got, _ := s.Get(ctx, "old")
	if got != nil {
		t.Error("expected 'old' to be cleaned up")
	}
	got, _ = s.Get(ctx, "recent")
	if got == nil {
		t.Error("expected 'recent' to survive cleanup")
	}
	got, _ = s.Get(ctx, "active")
	if got == nil {
		t.Error("expected 'active' to survive cleanup")
	}
	got, _ = s.Get(ctx, "plain")
	if got == nil {
		t.Error("expected 'plain' to survive cleanup")
	}
}

// TestMemoryStore_CleanupLoop verifies the background cleanup goroutine
// fires and removes expired certs.
func TestMemoryStore_CleanupLoop(t *testing.T) {
	cfg := &Config{
		CleanupInterval: 50 * time.Millisecond,
		RetainExpired:   1 * time.Millisecond,
	}
	s := NewMemoryStore(cfg)
	ctx := context.Background()

	// Insert an already-expired cert
	expired := time.Now().Add(-1 * time.Hour)
	s.Put(ctx, "expire-me", &CertificateData{Serial: "expire-me", ExpiresAt: expired})

	// Wait for the cleanup loop to fire
	time.Sleep(200 * time.Millisecond)
	s.Close()

	// Re-check with a fresh store reference (store is closed, but data was cleaned before close)
	s.mu.RLock()
	_, exists := s.certs["expire-me"]
	s.mu.RUnlock()
	if exists {
		t.Error("expected 'expire-me' to be removed by cleanup loop")
	}
}

// TestMemoryStore_ListByStatus_NonCertificateValues confirms that
// non-Certificate values are skipped (not included for either status).
func TestMemoryStore_ListByStatus_NonCertificate(t *testing.T) {
	s := NewMemoryStore(&Config{CleanupInterval: 0})
	defer s.Close()
	ctx := context.Background()

	s.Put(ctx, "plain-string", "just a string")

	active, err := s.ListByStatus(ctx, false)
	if err != nil {
		t.Fatalf("ListByStatus(false): %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active, got %d", len(active))
	}

	revoked, err := s.ListByStatus(ctx, true)
	if err != nil {
		t.Fatalf("ListByStatus(true): %v", err)
	}
	if len(revoked) != 0 {
		t.Errorf("expected 0 revoked, got %d", len(revoked))
	}
}

// TestFileStore_List tests FileStore.List with multiple entries.
func TestFileStore_List(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	cert1 := &CertificateData{Serial: "list-001", ExpiresAt: time.Now().Add(24 * time.Hour)}
	cert2 := &CertificateData{Serial: "list-002", ExpiresAt: time.Now().Add(24 * time.Hour)}
	s.Put(ctx, "list-001", cert1)
	s.Put(ctx, "list-002", cert2)

	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2", len(list))
	}
}

// TestFileStore_List_Closed verifies List returns ErrStoreClosed on closed store.
func TestFileStore_List_Closed(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	s.Close()

	_, err = s.List(context.Background())
	if err != ErrStoreClosed {
		t.Errorf("List on closed store: got %v, want ErrStoreClosed", err)
	}
}

// TestFileStore_ListByStatus tests FileStore.ListByStatus with active and revoked certs.
func TestFileStore_ListByStatus(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	revokedAt := time.Now()
	activeCert := &CertificateData{
		Serial:    "active-001",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	revokedCert := &CertificateData{
		Serial:    "revoked-001",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		RevokedAt: &revokedAt,
	}

	s.Put(ctx, "active-001", activeCert)
	s.Put(ctx, "revoked-001", revokedCert)

	active, err := s.ListByStatus(ctx, false)
	if err != nil {
		t.Fatalf("ListByStatus(false): %v", err)
	}
	if len(active) != 1 {
		t.Errorf("active count = %d, want 1", len(active))
	}

	revoked, err := s.ListByStatus(ctx, true)
	if err != nil {
		t.Fatalf("ListByStatus(true): %v", err)
	}
	if len(revoked) != 1 {
		t.Errorf("revoked count = %d, want 1", len(revoked))
	}
}

// TestFileStore_ListByStatus_Closed verifies ListByStatus returns ErrStoreClosed.
func TestFileStore_ListByStatus_Closed(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	s.Close()

	_, err = s.ListByStatus(context.Background(), false)
	if err != ErrStoreClosed {
		t.Errorf("ListByStatus on closed store: got %v, want ErrStoreClosed", err)
	}
}

// TestFileStore_Cleanup verifies that the FileStore cleanup method removes
// expired certificate files and updates the index.
func TestFileStore_Cleanup(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Path:            dir,
		CleanupInterval: 0, // no background goroutine
		RetainExpired:   1 * time.Hour,
	}
	s, err := NewFileStore(cfg)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	longExpired := time.Now().Add(-2 * time.Hour)
	notExpired := time.Now().Add(24 * time.Hour)

	s.Put(ctx, "old-cert", &CertificateData{Serial: "old-cert", ExpiresAt: longExpired})
	s.Put(ctx, "good-cert", &CertificateData{Serial: "good-cert", ExpiresAt: notExpired})

	// Verify both files exist
	if _, err := os.Stat(filepath.Join(dir, "old-cert.json")); err != nil {
		t.Fatalf("old-cert.json should exist before cleanup: %v", err)
	}

	s.cleanup()

	// old-cert should be removed
	if _, err := os.Stat(filepath.Join(dir, "old-cert.json")); !os.IsNotExist(err) {
		t.Error("expected old-cert.json to be removed after cleanup")
	}

	// good-cert should remain
	got, err := s.Get(ctx, "good-cert")
	if err != nil {
		t.Fatalf("Get good-cert after cleanup: %v", err)
	}
	if got == nil {
		t.Error("expected good-cert to survive cleanup")
	}

	// old-cert should be gone from index
	got, err = s.Get(ctx, "old-cert")
	if err != nil {
		t.Fatalf("Get old-cert after cleanup: %v", err)
	}
	if got != nil {
		t.Error("expected old-cert to be removed from index after cleanup")
	}
}

// TestFileStore_CleanupLoop verifies the background cleanup goroutine fires.
func TestFileStore_CleanupLoop(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Path:            dir,
		CleanupInterval: 50 * time.Millisecond,
		RetainExpired:   1 * time.Millisecond,
	}
	s, err := NewFileStore(cfg)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ctx := context.Background()

	expired := time.Now().Add(-1 * time.Hour)
	s.Put(ctx, "loop-expire", &CertificateData{Serial: "loop-expire", ExpiresAt: expired})

	// Wait for cleanup loop to fire
	time.Sleep(200 * time.Millisecond)
	s.Close()

	// File should have been removed
	if _, err := os.Stat(filepath.Join(dir, "loop-expire.json")); !os.IsNotExist(err) {
		t.Error("expected loop-expire.json to be removed by cleanup loop")
	}
}

// TestFileStore_Put_RevokedCertificate tests that putting a revoked certificate
// correctly stores the RevokedAt timestamp in the index entry.
func TestFileStore_Put_RevokedCertificate(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	revokedAt := time.Now()
	cert := &CertificateData{
		Serial:    "revoked-put",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		RevokedAt: &revokedAt,
	}
	if err := s.Put(ctx, "revoked-put", cert); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Check the index entry has RevokedAt set
	s.mu.RLock()
	entry := s.index["revoked-put"]
	s.mu.RUnlock()

	if entry == nil {
		t.Fatal("index entry is nil")
	}
	if entry.RevokedAt == nil {
		t.Error("expected RevokedAt to be set in index entry for revoked cert")
	}
}

// TestFileStore_Put_NonCertificate tests putting a value that does not
// implement the Certificate interface.
func TestFileStore_Put_NonCertificate(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	// A simple map should serialize to JSON fine but not implement Certificate
	data := map[string]string{"key": "value"}
	if err := s.Put(ctx, "non-cert", data); err != nil {
		t.Fatalf("Put non-certificate: %v", err)
	}

	// Index entry should have zero ExpiresAt and nil RevokedAt
	s.mu.RLock()
	entry := s.index["non-cert"]
	s.mu.RUnlock()

	if entry == nil {
		t.Fatal("index entry is nil")
	}
	if entry.RevokedAt != nil {
		t.Error("expected RevokedAt to be nil for non-certificate")
	}
}

// TestFileStore_Get_FileMissing tests Get when the index references a file
// that has been removed from disk (simulating corruption/manual deletion).
func TestFileStore_Get_FileMissing(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	cert := &CertificateData{Serial: "will-vanish", ExpiresAt: time.Now().Add(24 * time.Hour)}
	s.Put(ctx, "will-vanish", cert)

	// Manually remove the cert file
	os.Remove(filepath.Join(dir, "will-vanish.json"))

	got, err := s.Get(ctx, "will-vanish")
	if err != nil {
		t.Fatalf("Get should not error for missing file: %v", err)
	}
	if got != nil {
		t.Error("expected nil for missing file")
	}
}

// TestFileStore_List_WithMissingFile verifies List gracefully handles
// entries in the index whose files are missing from disk.
func TestFileStore_List_WithMissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	s.Put(ctx, "exists", &CertificateData{Serial: "exists", ExpiresAt: time.Now().Add(24 * time.Hour)})
	s.Put(ctx, "missing", &CertificateData{Serial: "missing", ExpiresAt: time.Now().Add(24 * time.Hour)})

	// Remove one file
	os.Remove(filepath.Join(dir, "missing.json"))

	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List len = %d, want 1 (missing file should be skipped)", len(list))
	}
}

// TestFileStore_ListByStatus_WithMissingFile verifies ListByStatus
// handles missing files gracefully.
func TestFileStore_ListByStatus_WithMissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	s.Put(ctx, "good", &CertificateData{Serial: "good", ExpiresAt: time.Now().Add(24 * time.Hour)})
	s.Put(ctx, "gone", &CertificateData{Serial: "gone", ExpiresAt: time.Now().Add(24 * time.Hour)})

	os.Remove(filepath.Join(dir, "gone.json"))

	active, err := s.ListByStatus(ctx, false)
	if err != nil {
		t.Fatalf("ListByStatus(false): %v", err)
	}
	if len(active) != 1 {
		t.Errorf("active count = %d, want 1", len(active))
	}
}

// TestFileStore_Cleanup_MissingFile verifies cleanup handles the case
// where an expired cert's file has already been removed from disk.
func TestFileStore_Cleanup_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Path:            dir,
		CleanupInterval: 0,
		RetainExpired:   1 * time.Hour,
	}
	s, err := NewFileStore(cfg)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	longExpired := time.Now().Add(-2 * time.Hour)
	s.Put(ctx, "already-gone", &CertificateData{Serial: "already-gone", ExpiresAt: longExpired})

	// Remove the file before cleanup runs
	os.Remove(filepath.Join(dir, "already-gone.json"))

	// cleanup should not panic or error
	s.cleanup()

	// Entry should be removed from index
	s.mu.RLock()
	_, exists := s.index["already-gone"]
	s.mu.RUnlock()
	if exists {
		t.Error("expected 'already-gone' to be removed from index after cleanup")
	}
}

// TestFileStore_Persistence_WithRevoked verifies that revocation status
// persists across store restarts via the index.
func TestFileStore_Persistence_WithRevoked(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	revokedAt := time.Now()

	// Write a revoked cert
	s1, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	s1.Put(ctx, "rev-persist", &CertificateData{
		Serial:    "rev-persist",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		RevokedAt: &revokedAt,
	})
	s1.Close()

	// Reopen
	s2, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore reopen: %v", err)
	}
	defer s2.Close()

	// Check that revoked status is in the index
	s2.mu.RLock()
	entry := s2.index["rev-persist"]
	s2.mu.RUnlock()

	if entry == nil {
		t.Fatal("entry not found after reopen")
	}
	if entry.RevokedAt == nil {
		t.Error("expected RevokedAt to persist after reopen")
	}

	// ListByStatus should find it as revoked
	revoked, err := s2.ListByStatus(ctx, true)
	if err != nil {
		t.Fatalf("ListByStatus(true): %v", err)
	}
	if len(revoked) != 1 {
		t.Errorf("revoked count = %d, want 1", len(revoked))
	}
}
