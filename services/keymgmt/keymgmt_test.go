// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package keymgmt

import (
	"testing"
	"time"

	"unheaded/pkg/logger"
)

func newTestService() *Service {
	log := logger.New(logger.Config{Level: logger.InfoLevel})
	cfg := DefaultConfig()
	cfg.GracePeriod = 100 * time.Millisecond // Fast for tests
	return NewService(log, nil, cfg)
}

func TestGenerateKey(t *testing.T) {
	svc := newTestService()

	record, err := svc.GenerateKey(0x02, 0x01, 1000)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	if record.AlgoID != 0x02 {
		t.Errorf("AlgoID = %d, want 0x02", record.AlgoID)
	}
	if record.Status != KeyActive {
		t.Errorf("Status = %v, want Active", record.Status)
	}
	if record.ServiceID != 1000 {
		t.Errorf("ServiceID = %d, want 1000", record.ServiceID)
	}
	if record.KeyRef == 0 {
		t.Error("KeyRef should not be 0")
	}
}

func TestGenerateKeyLimit(t *testing.T) {
	svc := newTestService()
	svc.config.MaxKeysPerService = 2

	if _, err := svc.GenerateKey(0x02, 0x01, 1000); err != nil {
		t.Fatalf("first key failed: %v", err)
	}
	if _, err := svc.GenerateKey(0x02, 0x01, 1000); err != nil {
		t.Fatalf("second key failed: %v", err)
	}
	if _, err := svc.GenerateKey(0x02, 0x01, 1000); err == nil {
		t.Fatal("third key should have failed (limit = 2)")
	}
}

func TestRotateKey(t *testing.T) {
	svc := newTestService()

	old, err := svc.GenerateKey(0x01, 0x01, 500)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	newRec, err := svc.RotateKey(old.ID)
	if err != nil {
		t.Fatalf("RotateKey failed: %v", err)
	}

	// Old key should be rotating
	oldRec, _ := svc.GetKey(old.ID)
	if oldRec.Status != KeyRotating {
		t.Errorf("old key status = %v, want Rotating", oldRec.Status)
	}
	if oldRec.ReplacedBy != newRec.ID {
		t.Errorf("old.ReplacedBy = %s, want %s", oldRec.ReplacedBy, newRec.ID)
	}

	// New key should be active
	if newRec.Status != KeyActive {
		t.Errorf("new key status = %v, want Active", newRec.Status)
	}

	// Wait for grace period
	time.Sleep(200 * time.Millisecond)

	oldRec, _ = svc.GetKey(old.ID)
	if oldRec.Status != KeyExpired {
		t.Errorf("old key after grace = %v, want Expired", oldRec.Status)
	}
}

func TestRotateNonActiveKey(t *testing.T) {
	svc := newTestService()

	rec, _ := svc.GenerateKey(0x02, 0x01, 100)
	_ = svc.RevokeKey(rec.ID)

	if _, err := svc.RotateKey(rec.ID); err == nil {
		t.Fatal("should not be able to rotate a revoked key")
	}
}

func TestRevokeKey(t *testing.T) {
	svc := newTestService()

	rec, _ := svc.GenerateKey(0x02, 0x01, 100)
	if err := svc.RevokeKey(rec.ID); err != nil {
		t.Fatalf("RevokeKey failed: %v", err)
	}

	revoked, _ := svc.GetKey(rec.ID)
	if revoked.Status != KeyRevoked {
		t.Errorf("status = %v, want Revoked", revoked.Status)
	}
	if revoked.RevokedAt == nil {
		t.Error("RevokedAt should be set")
	}
}

func TestRevokeAlreadyRevoked(t *testing.T) {
	svc := newTestService()

	rec, _ := svc.GenerateKey(0x02, 0x01, 100)
	_ = svc.RevokeKey(rec.ID)

	if err := svc.RevokeKey(rec.ID); err == nil {
		t.Fatal("should not be able to revoke an already revoked key")
	}
}

func TestRevokeNotFound(t *testing.T) {
	svc := newTestService()
	if err := svc.RevokeKey("nonexistent"); err == nil {
		t.Fatal("should fail for nonexistent key")
	}
}

func TestGetKeyByRef(t *testing.T) {
	svc := newTestService()

	rec, _ := svc.GenerateKey(0x02, 0x01, 100)
	found, ok := svc.GetKeyByRef(rec.KeyRef)
	if !ok {
		t.Fatal("key not found by ref")
	}
	if found.ID != rec.ID {
		t.Errorf("ID = %s, want %s", found.ID, rec.ID)
	}
}

func TestListKeys(t *testing.T) {
	svc := newTestService()

	svc.GenerateKey(0x01, 0x01, 200)
	svc.GenerateKey(0x02, 0x01, 200)
	svc.GenerateKey(0x02, 0x01, 300)

	list := svc.ListKeys(200)
	if len(list) != 2 {
		t.Errorf("len = %d, want 2", len(list))
	}
}

func TestActiveKeyCount(t *testing.T) {
	svc := newTestService()

	svc.GenerateKey(0x01, 0x01, 100)
	rec2, _ := svc.GenerateKey(0x02, 0x01, 100)
	svc.GenerateKey(0x02, 0x02, 100)

	if svc.ActiveKeyCount() != 3 {
		t.Errorf("active = %d, want 3", svc.ActiveKeyCount())
	}

	svc.RevokeKey(rec2.ID)
	if svc.ActiveKeyCount() != 2 {
		t.Errorf("active after revoke = %d, want 2", svc.ActiveKeyCount())
	}
}

func TestKeyStatusString(t *testing.T) {
	tests := []struct {
		status KeyStatus
		want   string
	}{
		{KeyActive, "active"},
		{KeyRotating, "rotating"},
		{KeyRevoked, "revoked"},
		{KeyExpired, "expired"},
		{KeyStatus(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("KeyStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestStats(t *testing.T) {
	svc := newTestService()

	svc.GenerateKey(0x01, 0x01, 100)
	svc.GenerateKey(0x02, 0x01, 100)

	stats := svc.Stats()
	if stats["total_keys"].(int) != 2 {
		t.Errorf("total_keys = %v, want 2", stats["total_keys"])
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.GracePeriod != 60*time.Second {
		t.Errorf("GracePeriod = %v, want 60s", cfg.GracePeriod)
	}
	if cfg.ListenAddr != ":19007" {
		t.Errorf("ListenAddr = %s, want :19007", cfg.ListenAddr)
	}
}
