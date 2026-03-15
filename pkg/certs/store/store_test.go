// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Type != "memory" {
		t.Errorf("Type = %q, want %q", cfg.Type, "memory")
	}
	if cfg.CleanupInterval != 1*time.Hour {
		t.Errorf("CleanupInterval = %v, want %v", cfg.CleanupInterval, 1*time.Hour)
	}
	if cfg.RetainExpired != 7*24*time.Hour {
		t.Errorf("RetainExpired = %v, want %v", cfg.RetainExpired, 7*24*time.Hour)
	}
}

func TestCertificateData(t *testing.T) {
	now := time.Now()
	revokedAt := now.Add(-1 * time.Hour)

	t.Run("not revoked", func(t *testing.T) {
		c := &CertificateData{
			Serial:    "abc123",
			ExpiresAt: now.Add(24 * time.Hour),
		}
		if c.GetSerial() != "abc123" {
			t.Errorf("GetSerial() = %q, want %q", c.GetSerial(), "abc123")
		}
		if c.IsRevoked() {
			t.Error("expected not revoked")
		}
	})

	t.Run("revoked", func(t *testing.T) {
		c := &CertificateData{
			Serial:    "def456",
			ExpiresAt: now.Add(24 * time.Hour),
			RevokedAt: &revokedAt,
		}
		if !c.IsRevoked() {
			t.Error("expected revoked")
		}
	})

	t.Run("GetExpiresAt", func(t *testing.T) {
		exp := now.Add(48 * time.Hour)
		c := &CertificateData{ExpiresAt: exp}
		if !c.GetExpiresAt().Equal(exp) {
			t.Errorf("GetExpiresAt() = %v, want %v", c.GetExpiresAt(), exp)
		}
	})
}

func TestNew(t *testing.T) {
	t.Run("nil config defaults to memory", func(t *testing.T) {
		s, err := New(nil)
		if err != nil {
			t.Fatalf("New(nil) error: %v", err)
		}
		defer s.Close()
		if _, ok := s.(*MemoryStore); !ok {
			t.Errorf("expected *MemoryStore, got %T", s)
		}
	})

	t.Run("memory type", func(t *testing.T) {
		s, err := New(&Config{Type: "memory", CleanupInterval: 0})
		if err != nil {
			t.Fatalf("New error: %v", err)
		}
		defer s.Close()
		if _, ok := s.(*MemoryStore); !ok {
			t.Errorf("expected *MemoryStore, got %T", s)
		}
	})

	t.Run("file type", func(t *testing.T) {
		s, err := New(&Config{Type: "file", Path: t.TempDir(), CleanupInterval: 0})
		if err != nil {
			t.Fatalf("New error: %v", err)
		}
		defer s.Close()
		if _, ok := s.(*FileStore); !ok {
			t.Errorf("expected *FileStore, got %T", s)
		}
	})

	t.Run("unknown type defaults to memory", func(t *testing.T) {
		s, err := New(&Config{Type: "redis", CleanupInterval: 0})
		if err != nil {
			t.Fatalf("New error: %v", err)
		}
		defer s.Close()
		if _, ok := s.(*MemoryStore); !ok {
			t.Errorf("expected *MemoryStore, got %T", s)
		}
	})

	t.Run("file type no path", func(t *testing.T) {
		_, err := New(&Config{Type: "file"})
		if err == nil {
			t.Error("expected error for file store without path")
		}
	})
}

func TestMemoryStore_CRUD(t *testing.T) {
	s := NewMemoryStore(&Config{CleanupInterval: 0})
	defer s.Close()
	ctx := context.Background()

	// Put
	cert := &CertificateData{
		Serial:    "serial-001",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := s.Put(ctx, "serial-001", cert); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get
	got, err := s.Get(ctx, "serial-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	gotCert := got.(*CertificateData)
	if gotCert.Serial != "serial-001" {
		t.Errorf("Serial = %q, want %q", gotCert.Serial, "serial-001")
	}

	// Get nonexistent
	got, err = s.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get nonexistent: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent, got %v", got)
	}

	// List
	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List len = %d, want 1", len(list))
	}

	// Delete
	if err := s.Delete(ctx, "serial-001"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = s.Get(ctx, "serial-001")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestMemoryStore_ListByStatus(t *testing.T) {
	s := NewMemoryStore(&Config{CleanupInterval: 0})
	defer s.Close()
	ctx := context.Background()

	revokedAt := time.Now()
	s.Put(ctx, "active", &CertificateData{
		Serial:    "active",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	s.Put(ctx, "revoked", &CertificateData{
		Serial:    "revoked",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		RevokedAt: &revokedAt,
	})

	// List active
	active, err := s.ListByStatus(ctx, false)
	if err != nil {
		t.Fatalf("ListByStatus(false): %v", err)
	}
	if len(active) != 1 {
		t.Errorf("active count = %d, want 1", len(active))
	}

	// List revoked
	revoked, err := s.ListByStatus(ctx, true)
	if err != nil {
		t.Fatalf("ListByStatus(true): %v", err)
	}
	if len(revoked) != 1 {
		t.Errorf("revoked count = %d, want 1", len(revoked))
	}
}

func TestMemoryStore_Closed(t *testing.T) {
	s := NewMemoryStore(&Config{CleanupInterval: 0})
	ctx := context.Background()

	s.Close()

	if err := s.Put(ctx, "x", "y"); err != ErrStoreClosed {
		t.Errorf("Put on closed: %v, want ErrStoreClosed", err)
	}
	if _, err := s.Get(ctx, "x"); err != ErrStoreClosed {
		t.Errorf("Get on closed: %v, want ErrStoreClosed", err)
	}
	if err := s.Delete(ctx, "x"); err != ErrStoreClosed {
		t.Errorf("Delete on closed: %v, want ErrStoreClosed", err)
	}
	if _, err := s.List(ctx); err != ErrStoreClosed {
		t.Errorf("List on closed: %v, want ErrStoreClosed", err)
	}
	if _, err := s.ListByStatus(ctx, false); err != ErrStoreClosed {
		t.Errorf("ListByStatus on closed: %v, want ErrStoreClosed", err)
	}

	// Double close should be safe
	if err := s.Close(); err != nil {
		t.Errorf("double Close: %v", err)
	}
}

func TestFileStore_CRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{
		Type:            "file",
		Path:            dir,
		CleanupInterval: 0,
	})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	// Put
	cert := &CertificateData{
		Serial:         "fs-001",
		CertificatePEM: []byte("cert-pem"),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}
	if err := s.Put(ctx, "fs-001", cert); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get
	got, err := s.Get(ctx, "fs-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	gotCert := got.(*CertificateData)
	if gotCert.Serial != "fs-001" {
		t.Errorf("Serial = %q, want %q", gotCert.Serial, "fs-001")
	}

	// Get nonexistent
	got, err = s.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get nonexistent: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}

	// Delete
	if err := s.Delete(ctx, "fs-001"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = s.Get(ctx, "fs-001")
	if got != nil {
		t.Error("expected nil after delete")
	}

	// Delete nonexistent (should be idempotent)
	if err := s.Delete(ctx, "nonexistent"); err != nil {
		t.Errorf("Delete nonexistent: %v", err)
	}
}

func TestFileStore_Closed(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(&Config{
		Type:            "file",
		Path:            dir,
		CleanupInterval: 0,
	})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ctx := context.Background()

	s.Close()

	if err := s.Put(ctx, "x", "y"); err != ErrStoreClosed {
		t.Errorf("Put on closed: %v, want ErrStoreClosed", err)
	}
	if _, err := s.Get(ctx, "x"); err != ErrStoreClosed {
		t.Errorf("Get on closed: %v, want ErrStoreClosed", err)
	}
	if err := s.Delete(ctx, "x"); err != ErrStoreClosed {
		t.Errorf("Delete on closed: %v, want ErrStoreClosed", err)
	}

	// Double close
	if err := s.Close(); err != nil {
		t.Errorf("double Close: %v", err)
	}
}

func TestFileStore_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Write cert
	s1, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ctx := context.Background()
	cert := &CertificateData{Serial: "persist-001", ExpiresAt: time.Now().Add(24 * time.Hour)}
	s1.Put(ctx, "persist-001", cert)
	s1.Close()

	// Reopen and verify
	s2, err := NewFileStore(&Config{Path: dir, CleanupInterval: 0})
	if err != nil {
		t.Fatalf("NewFileStore reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.Get(ctx, "persist-001")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got == nil {
		t.Fatal("expected cert after reopen, got nil")
	}
}
