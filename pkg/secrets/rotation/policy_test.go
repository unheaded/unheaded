// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rotation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// --- matchPath ---

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*", "anything", true},
		{"**", "anything/nested", true},
		{"secrets/*", "secrets/mykey", true},
		{"secrets/*", "other/mykey", false},
		{"*suffix", "some-suffix", true},
		{"*suffix", "no-match", false},
		{"exact", "exact", true},
		{"exact", "other", false},
		{"pre*", "prefix-match", true},
		{"pre*", "something", false},
	}
	for _, tt := range tests {
		got := matchPath(tt.pattern, tt.path)
		if got != tt.want {
			t.Errorf("matchPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

// --- UsageTracker ---

func TestUsageTracker_IncrementAndGet(t *testing.T) {
	ut := &UsageTracker{
		counts:     make(map[string]*usageCount),
		cleanupTTL: time.Hour,
	}

	c := ut.IncrementUsage("path/a", time.Minute)
	if c != 1 {
		t.Fatalf("expected 1, got %d", c)
	}
	c = ut.IncrementUsage("path/a", time.Minute)
	if c != 2 {
		t.Fatalf("expected 2, got %d", c)
	}

	g := ut.GetUsage("path/a")
	if g != 2 {
		t.Fatalf("expected 2, got %d", g)
	}
}

func TestUsageTracker_GetUsage_Unknown(t *testing.T) {
	ut := &UsageTracker{
		counts:     make(map[string]*usageCount),
		cleanupTTL: time.Hour,
	}
	if ut.GetUsage("unknown") != 0 {
		t.Fatal("expected 0 for unknown path")
	}
}

func TestUsageTracker_ResetUsage(t *testing.T) {
	ut := &UsageTracker{
		counts:     make(map[string]*usageCount),
		cleanupTTL: time.Hour,
	}
	ut.IncrementUsage("path/b", time.Minute)
	ut.IncrementUsage("path/b", time.Minute)
	ut.ResetUsage("path/b")

	if ut.GetUsage("path/b") != 0 {
		t.Fatal("expected 0 after reset")
	}
}

func TestUsageTracker_ResetUsage_Unknown(t *testing.T) {
	ut := &UsageTracker{
		counts:     make(map[string]*usageCount),
		cleanupTTL: time.Hour,
	}
	// Should not panic
	ut.ResetUsage("nonexistent")
}

func TestUsageTracker_WindowExpiry(t *testing.T) {
	ut := &UsageTracker{
		counts:     make(map[string]*usageCount),
		cleanupTTL: time.Hour,
	}

	// Use a very short window (already expired effectively)
	ut.IncrementUsage("path/c", time.Nanosecond)
	// After the window, next increment should reset
	time.Sleep(2 * time.Millisecond)
	c := ut.IncrementUsage("path/c", time.Minute)
	if c != 1 {
		t.Fatalf("expected 1 after window reset, got %d", c)
	}
}

// --- Policy validation ---

func TestValidatePolicy_EmptyID(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.AddPolicy(&Policy{SecretPath: "x", Type: PolicyTypeOnDemand})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestValidatePolicy_EmptyPath(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.AddPolicy(&Policy{ID: "p1", Type: PolicyTypeOnDemand})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestValidatePolicy_UnknownType(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.AddPolicy(&Policy{ID: "p1", SecretPath: "x", Type: "bogus"})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestValidatePolicy_TimeBasedMissingConfig(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.AddPolicy(&Policy{ID: "p1", SecretPath: "x", Type: PolicyTypeTimeBased})
	if err == nil {
		t.Fatal("expected error for missing time_config")
	}
}

func TestValidatePolicy_TimeBasedNoInterval(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.AddPolicy(&Policy{
		ID:         "p1",
		SecretPath: "x",
		Type:       PolicyTypeTimeBased,
		TimeConfig: &TimeBasedConfig{},
	})
	if err == nil {
		t.Fatal("expected error for zero interval/max_age")
	}
}

func TestValidatePolicy_UsageBased(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.AddPolicy(&Policy{ID: "p1", SecretPath: "x", Type: PolicyTypeUsageBased})
	if err == nil {
		t.Fatal("expected error for missing usage_config")
	}
	err = pm.AddPolicy(&Policy{
		ID: "p2", SecretPath: "x", Type: PolicyTypeUsageBased,
		UsageConfig: &UsageBasedConfig{MaxUsageCount: 0},
	})
	if err == nil {
		t.Fatal("expected error for zero max_usage_count")
	}
}

func TestValidatePolicy_Hybrid(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.AddPolicy(&Policy{ID: "p1", SecretPath: "x", Type: PolicyTypeHybrid})
	if err == nil {
		t.Fatal("expected error for hybrid without configs")
	}
}

func TestValidatePolicy_Expiry(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.AddPolicy(&Policy{ID: "p1", SecretPath: "x", Type: PolicyTypeExpiry})
	if err == nil {
		t.Fatal("expected error for missing expiry_config")
	}
	err = pm.AddPolicy(&Policy{
		ID: "p2", SecretPath: "x", Type: PolicyTypeExpiry,
		ExpiryConfig: &ExpiryBasedConfig{RotateBeforeExpiry: 0},
	})
	if err == nil {
		t.Fatal("expected error for zero rotate_before_expiry")
	}
}

func TestValidatePolicy_OnDemand(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.AddPolicy(&Policy{
		ID:         "p1",
		SecretPath: "x",
		Type:       PolicyTypeOnDemand,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// --- PolicyManager CRUD ---

func TestPolicyManager_AddGetRemoveList(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))

	p := &Policy{
		ID:         "time1",
		SecretPath: "secrets/api",
		Type:       PolicyTypeTimeBased,
		TimeConfig: &TimeBasedConfig{
			RotationInterval: 24 * time.Hour,
		},
		Enabled:  true,
		Priority: 10,
	}
	if err := pm.AddPolicy(p); err != nil {
		t.Fatal(err)
	}

	got, err := pm.GetPolicy("time1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "time1" {
		t.Fatal("wrong policy")
	}

	list := pm.ListPolicies()
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	if err := pm.RemovePolicy("time1"); err != nil {
		t.Fatal(err)
	}
	_, err = pm.GetPolicy("time1")
	if !errors.Is(err, ErrPolicyNotFound) {
		t.Fatal("expected not found after remove")
	}
}

func TestPolicyManager_RemoveNotFound(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.RemovePolicy("nonexistent")
	if !errors.Is(err, ErrPolicyNotFound) {
		t.Fatal("expected ErrPolicyNotFound")
	}
}

func TestPolicyManager_GetNotFound(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	_, err := pm.GetPolicy("nope")
	if !errors.Is(err, ErrPolicyNotFound) {
		t.Fatal("expected ErrPolicyNotFound")
	}
}

// --- GetPoliciesForPath ---

func TestGetPoliciesForPath_ExactAndWildcard(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))

	pm.AddPolicy(&Policy{
		ID: "exact", SecretPath: "secrets/db", Type: PolicyTypeOnDemand,
		Priority: 1,
	})
	pm.AddPolicy(&Policy{
		ID: "wild", SecretPath: "secrets/*", Type: PolicyTypeOnDemand,
		Priority: 5,
	})

	policies := pm.GetPoliciesForPath("secrets/db")
	if len(policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(policies))
	}
}

// --- ShouldRotate ---

func TestShouldRotate_TimeBased(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	path := "secrets/old"
	s := &secrets.Secret{
		Path:      path,
		Type:      secrets.SecretTypeStatic,
		Data:      map[string][]byte{"value": []byte("v")},
		Metadata:  map[string]string{},
		CreatedAt: time.Now().Add(-48 * time.Hour),
		UpdatedAt: time.Now().Add(-48 * time.Hour),
	}
	ms.Set(context.Background(), path, s)

	pm := NewPolicyManager(ms)
	pm.AddPolicy(&Policy{
		ID:         "tb1",
		SecretPath: path,
		Type:       PolicyTypeTimeBased,
		Enabled:    true,
		TimeConfig: &TimeBasedConfig{
			RotationInterval: 24 * time.Hour,
		},
	})

	should, policy, reason := pm.ShouldRotate(context.Background(), path)
	if !should {
		t.Fatal("expected should rotate")
	}
	if policy == nil {
		t.Fatal("expected policy")
	}
	if reason == "" {
		t.Fatal("expected reason")
	}
}

func TestShouldRotate_NoPolicies(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	pm := NewPolicyManager(ms)

	should, _, _ := pm.ShouldRotate(context.Background(), "secrets/x")
	if should {
		t.Fatal("should not rotate with no policies")
	}
}

func TestShouldRotate_Disabled(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	path := "secrets/disabled"
	s := &secrets.Secret{
		Path:      path,
		Type:      secrets.SecretTypeStatic,
		Data:      map[string][]byte{"value": []byte("v")},
		Metadata:  map[string]string{},
		CreatedAt: time.Now().Add(-48 * time.Hour),
		UpdatedAt: time.Now().Add(-48 * time.Hour),
	}
	ms.Set(context.Background(), path, s)

	pm := NewPolicyManager(ms)
	pm.AddPolicy(&Policy{
		ID:         "dis1",
		SecretPath: path,
		Type:       PolicyTypeTimeBased,
		Enabled:    false,
		TimeConfig: &TimeBasedConfig{RotationInterval: time.Hour},
	})

	should, _, _ := pm.ShouldRotate(context.Background(), path)
	if should {
		t.Fatal("disabled policy should not trigger rotation")
	}
}

func TestShouldRotate_OnDemand(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	path := "secrets/od"
	s := &secrets.Secret{
		Path: path, Type: secrets.SecretTypeStatic,
		Data: map[string][]byte{"value": []byte("v")}, Metadata: map[string]string{},
	}
	ms.Set(context.Background(), path, s)

	pm := NewPolicyManager(ms)
	pm.AddPolicy(&Policy{
		ID: "od1", SecretPath: path, Type: PolicyTypeOnDemand, Enabled: true,
	})

	should, _, _ := pm.ShouldRotate(context.Background(), path)
	if should {
		t.Fatal("on-demand should never auto-trigger")
	}
}

func TestShouldRotate_ExpiryBased(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	path := "certs/expiring"
	s := &secrets.Secret{
		Path:      path,
		Type:      secrets.SecretTypeCertificate,
		Data:      map[string][]byte{"cert": []byte("c")},
		Metadata:  map[string]string{},
		ExpiresAt: time.Now().Add(2 * time.Hour), // Expires in 2h
	}
	ms.Set(context.Background(), path, s)

	pm := NewPolicyManager(ms)
	pm.AddPolicy(&Policy{
		ID:         "exp1",
		SecretPath: path,
		Type:       PolicyTypeExpiry,
		Enabled:    true,
		ExpiryConfig: &ExpiryBasedConfig{
			RotateBeforeExpiry: 4 * time.Hour, // Threshold is 4h
		},
	})

	should, _, _ := pm.ShouldRotate(context.Background(), path)
	if !should {
		t.Fatal("should rotate before expiry")
	}
}

func TestShouldRotate_MinAge(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	path := "secrets/young"
	s := &secrets.Secret{
		Path:      path,
		Type:      secrets.SecretTypeStatic,
		Data:      map[string][]byte{"value": []byte("v")},
		Metadata:  map[string]string{},
		CreatedAt: time.Now().Add(-5 * time.Minute),
		UpdatedAt: time.Now().Add(-5 * time.Minute),
	}
	ms.Set(context.Background(), path, s)

	pm := NewPolicyManager(ms)
	pm.AddPolicy(&Policy{
		ID:         "min1",
		SecretPath: path,
		Type:       PolicyTypeTimeBased,
		Enabled:    true,
		TimeConfig: &TimeBasedConfig{
			RotationInterval: time.Minute,
			MinAge:           time.Hour,
		},
	})

	should, _, _ := pm.ShouldRotate(context.Background(), path)
	if should {
		t.Fatal("should not rotate when younger than MinAge")
	}
}

// --- RecordUsage ---

func TestRecordUsage(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	pm := NewPolicyManager(ms)
	pm.AddPolicy(&Policy{
		ID:         "usage1",
		SecretPath: "secrets/api",
		Type:       PolicyTypeUsageBased,
		Enabled:    true,
		UsageConfig: &UsageBasedConfig{
			MaxUsageCount: 3,
			UsageWindow:   time.Hour,
		},
	})

	needsRotation, count := pm.RecordUsage("secrets/api")
	if needsRotation || count != 1 {
		t.Fatalf("expected no rotation, count 1; got rotation=%v count=%d", needsRotation, count)
	}

	pm.RecordUsage("secrets/api")
	needsRotation, count = pm.RecordUsage("secrets/api")
	if !needsRotation || count != 3 {
		t.Fatalf("expected rotation at count 3; got rotation=%v count=%d", needsRotation, count)
	}
}

func TestRecordUsage_NoPolicies(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	pm := NewPolicyManager(ms)

	needsRotation, count := pm.RecordUsage("unknown")
	if needsRotation || count != 0 {
		t.Fatal("no policies should return false, 0")
	}
}

// --- OnRotationComplete ---

func TestOnRotationComplete(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	pm := NewPolicyManager(ms)
	pm.AddPolicy(&Policy{
		ID:         "orc1",
		SecretPath: "secrets/reset",
		Type:       PolicyTypeUsageBased,
		Enabled:    true,
		UsageConfig: &UsageBasedConfig{
			MaxUsageCount:   100,
			UsageWindow:     time.Hour,
			ResetOnRotation: true,
		},
	})

	pm.RecordUsage("secrets/reset")
	pm.RecordUsage("secrets/reset")
	pm.OnRotationComplete("secrets/reset", true)

	// Usage should be reset
	_, count := pm.RecordUsage("secrets/reset")
	if count != 1 {
		t.Fatalf("expected count 1 after reset, got %d", count)
	}
}

func TestOnRotationComplete_Failure(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	pm := NewPolicyManager(ms)
	pm.AddPolicy(&Policy{
		ID:         "orcf",
		SecretPath: "secrets/nreset",
		Type:       PolicyTypeUsageBased,
		Enabled:    true,
		UsageConfig: &UsageBasedConfig{
			MaxUsageCount:   100,
			UsageWindow:     time.Hour,
			ResetOnRotation: true,
		},
	})

	pm.RecordUsage("secrets/nreset")
	pm.OnRotationComplete("secrets/nreset", false)

	// Usage should NOT be reset on failure
	g := pm.usageTracker.GetUsage("secrets/nreset")
	if g != 1 {
		t.Fatalf("expected count 1 (not reset), got %d", g)
	}
}

// --- PolicySnapshot / RestorePolicies ---

func TestPolicySnapshot_RestorePolicies(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	pm := NewPolicyManager(ms)
	pm.AddPolicy(&Policy{
		ID: "snap1", SecretPath: "x", Type: PolicyTypeOnDemand, Priority: 5,
	})

	data, err := pm.PolicySnapshot()
	if err != nil {
		t.Fatal(err)
	}

	// Verify JSON is valid
	var policies []*Policy
	if err := json.Unmarshal(data, &policies); err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy in snapshot, got %d", len(policies))
	}

	// Restore into a new manager
	pm2 := NewPolicyManager(ms)
	if err := pm2.RestorePolicies(data); err != nil {
		t.Fatal(err)
	}
	list := pm2.ListPolicies()
	if len(list) != 1 {
		t.Fatalf("expected 1 restored policy, got %d", len(list))
	}
}

func TestRestorePolicies_InvalidJSON(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	err := pm.RestorePolicies([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- inRotationWindow ---

func TestInRotationWindow_NoRestrictions(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	now := time.Now()
	w := &RotationWindow{
		StartHour: 0,
		EndHour:   24,
	}
	if !pm.inRotationWindow(now, w) {
		t.Fatal("expected in window with 0-24 range")
	}
}

func TestInRotationWindow_DayOfWeek(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	// Pick a Monday
	monday := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC) // March 9, 2026 is Monday
	w := &RotationWindow{
		DaysOfWeek: []int{1}, // Monday only
		StartHour:  0,
		EndHour:    24,
	}
	if !pm.inRotationWindow(monday, w) {
		t.Fatal("expected in window on Monday")
	}
	w.DaysOfWeek = []int{0} // Sunday only
	if pm.inRotationWindow(monday, w) {
		t.Fatal("expected out of window on Monday with Sunday-only")
	}
}

func TestInRotationWindow_OvernightWindow(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	w := &RotationWindow{
		StartHour: 22,
		EndHour:   6,
	}
	at23 := time.Date(2026, 3, 9, 23, 0, 0, 0, time.UTC)
	at3 := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	at12 := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	if !pm.inRotationWindow(at23, w) {
		t.Fatal("23:00 should be in 22-06 window")
	}
	if !pm.inRotationWindow(at3, w) {
		t.Fatal("03:00 should be in 22-06 window")
	}
	if pm.inRotationWindow(at12, w) {
		t.Fatal("12:00 should not be in 22-06 window")
	}
}

func TestInRotationWindow_InvalidTimezone(t *testing.T) {
	pm := NewPolicyManager(store.NewMemoryStore(store.MemoryStoreConfig{}))
	w := &RotationWindow{
		StartHour: 0,
		EndHour:   24,
		Timezone:  "Invalid/Zone",
	}
	// Should fall back to UTC
	if !pm.inRotationWindow(time.Now(), w) {
		t.Fatal("should fall back to UTC")
	}
}
