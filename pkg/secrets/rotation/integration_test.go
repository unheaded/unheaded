// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package rotation

import (
	"context"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// --- generateSecurePassword ---

func TestGenerateSecurePassword_Default(t *testing.T) {
	pw, err := generateSecurePassword(0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) != 32 {
		t.Fatalf("expected default length 32, got %d", len(pw))
	}
}

func TestGenerateSecurePassword_CustomLength(t *testing.T) {
	pw, err := generateSecurePassword(64, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) != 64 {
		t.Fatalf("expected 64, got %d", len(pw))
	}
}

func TestGenerateSecurePassword_CustomCharset(t *testing.T) {
	pw, err := generateSecurePassword(100, "abc")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range pw {
		if c != 'a' && c != 'b' && c != 'c' {
			t.Fatalf("unexpected character %c", c)
		}
	}
}

// --- DefaultAPIKeyGenerator ---

func TestDefaultAPIKeyGenerator_Generate(t *testing.T) {
	g := &DefaultAPIKeyGenerator{}
	ctx := context.Background()

	key, err := g.Generate(ctx, APIKeyOptions{Length: 32})
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("expected 32 chars, got %d", len(key))
	}
}

func TestDefaultAPIKeyGenerator_GenerateDefaultLength(t *testing.T) {
	g := &DefaultAPIKeyGenerator{}
	key, err := g.Generate(context.Background(), APIKeyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("expected default 32, got %d", len(key))
	}
}

func TestDefaultAPIKeyGenerator_GenerateWithPrefix(t *testing.T) {
	g := &DefaultAPIKeyGenerator{}
	key, err := g.Generate(context.Background(), APIKeyOptions{
		Length: 32,
		Prefix: "sk_",
	})
	if err != nil {
		t.Fatal(err)
	}
	if key[:3] != "sk_" {
		t.Fatalf("expected sk_ prefix, got %s", key[:3])
	}
	if len(key) != 35 { // 3 prefix + 32 key
		t.Fatalf("expected 35, got %d", len(key))
	}
}

func TestDefaultAPIKeyGenerator_Validate(t *testing.T) {
	g := &DefaultAPIKeyGenerator{}
	err := g.Validate(context.Background(), "a-valid-key-with-enough-length")
	if err != nil {
		t.Fatal(err)
	}

	err = g.Validate(context.Background(), "short")
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestDefaultAPIKeyGenerator_Revoke(t *testing.T) {
	g := &DefaultAPIKeyGenerator{}
	err := g.Revoke(context.Background(), "any-key")
	if err != nil {
		t.Fatal("revoke should be no-op")
	}
}

// --- ExternalAPIKeyGenerator ---

func TestExternalAPIKeyGenerator_Generate(t *testing.T) {
	g := NewExternalAPIKeyGenerator("http://example.com", "token")
	key, err := g.Generate(context.Background(), APIKeyOptions{Length: 24})
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 24 {
		t.Fatalf("expected 24, got %d", len(key))
	}
}

func TestExternalAPIKeyGenerator_RevokeAndValidate(t *testing.T) {
	g := NewExternalAPIKeyGenerator("http://example.com", "token")
	if err := g.Revoke(context.Background(), "key"); err != nil {
		t.Fatal(err)
	}
	if err := g.Validate(context.Background(), "key"); err != nil {
		t.Fatal(err)
	}
}

// --- CertificateIntegration ---

func TestNewCertificateIntegration_Validation(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	_, err := NewCertificateIntegration(CertificateIntegrationConfig{})
	if err == nil {
		t.Fatal("expected error for nil manager")
	}

	_, err = NewCertificateIntegration(CertificateIntegrationConfig{Manager: rm})
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestNewCertificateIntegration_OK(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	ci, err := NewCertificateIntegration(CertificateIntegrationConfig{
		Manager: rm,
		Store:   ms,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ci == nil {
		t.Fatal("expected non-nil")
	}
}

func TestCertificateIntegration_GetCertificateInfo(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	// First generate a real cert by rotating
	path := "certs/info"
	ms.Set(context.Background(), path, certSecret(path))
	rm.Rotate(context.Background(), path, RotationConfig{
		TTL:        24 * time.Hour,
		Parameters: map[string]interface{}{"common_name": "info.test", "key_size": 2048},
	})

	ci, _ := NewCertificateIntegration(CertificateIntegrationConfig{
		Manager: rm,
		Store:   ms,
	})

	info, err := ci.GetCertificateInfo(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if info.CommonName != "info.test" {
		t.Fatalf("expected info.test, got %s", info.CommonName)
	}
}

func TestCertificateIntegration_GetCertificateInfo_NotFound(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	ci, _ := NewCertificateIntegration(CertificateIntegrationConfig{Manager: rm, Store: ms})

	_, err := ci.GetCertificateInfo(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- DatabaseCredentialIntegration ---

func TestNewDatabaseCredentialIntegration_Validation(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	_, err := NewDatabaseCredentialIntegration(DatabaseCredentialIntegrationConfig{})
	if err == nil {
		t.Fatal("expected error for nil manager")
	}

	_, err = NewDatabaseCredentialIntegration(DatabaseCredentialIntegrationConfig{Manager: rm})
	if err == nil {
		t.Fatal("expected error for nil store")
	}

	dci, err := NewDatabaseCredentialIntegration(DatabaseCredentialIntegrationConfig{
		Manager: rm, Store: ms,
	})
	if err != nil || dci == nil {
		t.Fatal("expected success")
	}
}

func TestDatabaseCredentialIntegration_RegisterDriver(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	dci, _ := NewDatabaseCredentialIntegration(DatabaseCredentialIntegrationConfig{
		Manager: rm, Store: ms,
	})

	// Should not panic with nil driver (just stores it)
	dci.RegisterDriver("test", nil)
}

// --- APIKeyIntegration ---

func TestNewAPIKeyIntegration_Validation(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	_, err := NewAPIKeyIntegration(APIKeyIntegrationConfig{})
	if err == nil {
		t.Fatal("expected error for nil manager")
	}

	_, err = NewAPIKeyIntegration(APIKeyIntegrationConfig{Manager: rm})
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestNewAPIKeyIntegration_OK(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	aki, err := NewAPIKeyIntegration(APIKeyIntegrationConfig{
		Manager: rm, Store: ms,
	})
	if err != nil {
		t.Fatal(err)
	}
	if aki == nil {
		t.Fatal("expected non-nil")
	}
}

func TestAPIKeyIntegration_RotateAPIKey(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	aki, _ := NewAPIKeyIntegration(APIKeyIntegrationConfig{
		Manager: rm, Store: ms,
	})

	result, err := aki.RotateAPIKey(context.Background(), "apikeys/test", "default", APIKeyOptions{
		Length: 32,
		Prefix: "uk_",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if result.NewSecret == nil {
		t.Fatal("expected new secret")
	}
}

func TestAPIKeyIntegration_RotateAPIKey_WithExistingKey(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	aki, _ := NewAPIKeyIntegration(APIKeyIntegrationConfig{
		Manager: rm, Store: ms,
	})

	// Create initial key
	existingSecret := &secrets.Secret{
		Path:    "apikeys/existing",
		Type:    secrets.SecretTypeStatic,
		Version: 1,
		Data: map[string][]byte{
			"key": []byte("old-api-key-that-is-long-enough-for-validation"),
		},
		Metadata: map[string]string{},
	}
	ms.Set(context.Background(), "apikeys/existing", existingSecret)

	result, err := aki.RotateAPIKey(context.Background(), "apikeys/existing", "default", APIKeyOptions{
		Length: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.NewSecret.Version != 2 {
		t.Fatalf("expected version 2, got %d", result.NewSecret.Version)
	}
}

func TestAPIKeyIntegration_RotateAPIKey_GeneratorNotFound(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	aki, _ := NewAPIKeyIntegration(APIKeyIntegrationConfig{
		Manager: rm, Store: ms,
	})

	_, err := aki.RotateAPIKey(context.Background(), "apikeys/test", "nonexistent", APIKeyOptions{})
	if err == nil {
		t.Fatal("expected error for nonexistent generator")
	}
}

func TestAPIKeyIntegration_RotateAPIKey_EmptyGeneratorUsesDefault(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	aki, _ := NewAPIKeyIntegration(APIKeyIntegrationConfig{
		Manager: rm, Store: ms,
	})
	// Remove the default generator to trigger fallback
	aki.mu.Lock()
	delete(aki.generators, "")
	aki.mu.Unlock()

	result, err := aki.RotateAPIKey(context.Background(), "apikeys/empty", "", APIKeyOptions{Length: 32})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatal("expected success with empty generator name fallback")
	}
}

func TestAPIKeyIntegration_RotateAPIKey_WithScopes(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	aki, _ := NewAPIKeyIntegration(APIKeyIntegrationConfig{
		Manager: rm, Store: ms,
	})

	result, err := aki.RotateAPIKey(context.Background(), "apikeys/scoped", "default", APIKeyOptions{
		Length:    32,
		Scopes:    []string{"read", "write"},
		Metadata:  map[string]string{"env": "prod"},
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.NewSecret.Metadata["scopes"] != "read,write" {
		t.Fatal("scopes not set")
	}
	if result.NewSecret.Metadata["env"] != "prod" {
		t.Fatal("metadata not set")
	}
	if result.NewSecret.ExpiresAt.IsZero() {
		t.Fatal("expiry not set")
	}
}

// --- CertificateRotatorWithCA ---

func TestCertificateRotatorWithCA_Type(t *testing.T) {
	r := &CertificateRotatorWithCA{}
	if r.Type() != secrets.SecretTypeCertificate {
		t.Fatal("wrong type")
	}
}

func TestCertificateRotatorWithCA_SelfSigned(t *testing.T) {
	r := &CertificateRotatorWithCA{}
	s := certSecret("certs/ca")
	cfg := RotationConfig{
		TTL:        24 * time.Hour,
		Parameters: map[string]interface{}{"common_name": "ca-test", "key_size": 2048},
	}
	newS, err := r.Rotate(context.Background(), s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Verify(context.Background(), newS); err != nil {
		t.Fatal(err)
	}
}

func TestCertificateRotatorWithCA_Rollback(t *testing.T) {
	r := &CertificateRotatorWithCA{}
	if err := r.Rollback(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
}

// --- RotationMetricsCollector ---

func TestNewRotationMetricsCollector_Defaults(t *testing.T) {
	c := NewRotationMetricsCollector(RotationMetricsCollectorConfig{})
	if c.interval != time.Minute {
		t.Fatalf("expected 1m default, got %v", c.interval)
	}
}

func TestRotationMetricsCollector_StartStop(t *testing.T) {
	c := NewRotationMetricsCollector(RotationMetricsCollectorConfig{
		Interval: 100 * time.Millisecond,
	})
	c.Start()
	time.Sleep(10 * time.Millisecond)
	c.Stop()
}

func TestRotationMetricsCollector_CollectMetrics_NilTracker(t *testing.T) {
	c := NewRotationMetricsCollector(RotationMetricsCollectorConfig{})
	// Should not panic
	c.collectMetrics()
}

// --- Scheduler integration tests ---

func TestNewScheduler_Validation(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	_, err := NewScheduler(SchedulerConfig{})
	if err == nil {
		t.Fatal("expected error for nil manager")
	}

	_, err = NewScheduler(SchedulerConfig{Manager: rm})
	if err == nil {
		t.Fatal("expected error for nil store")
	}

	s, err := NewScheduler(SchedulerConfig{Manager: rm, Store: ms})
	if err != nil || s == nil {
		t.Fatal("expected success")
	}
}

func TestScheduler_AddSchedule_Validation(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	s, _ := NewScheduler(SchedulerConfig{Manager: rm, Store: ms})

	err := s.AddSchedule(&Schedule{})
	if err == nil {
		t.Fatal("expected error for empty path")
	}

	err = s.AddSchedule(&Schedule{Path: "x"})
	if err == nil {
		t.Fatal("expected error for zero interval")
	}
}

func TestScheduler_AddGetListRemove(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	s, _ := NewScheduler(SchedulerConfig{Manager: rm, Store: ms})

	err := s.AddSchedule(&Schedule{
		Path:     "secrets/sched",
		Interval: time.Hour,
		Enabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	sched, ok := s.GetSchedule("secrets/sched")
	if !ok || sched == nil {
		t.Fatal("expected schedule")
	}

	list := s.ListSchedules()
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}

	s.RemoveSchedule("secrets/sched")
	_, ok = s.GetSchedule("secrets/sched")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestScheduler_EnableDisable(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	s, _ := NewScheduler(SchedulerConfig{Manager: rm, Store: ms})

	s.AddSchedule(&Schedule{Path: "secrets/ed", Interval: time.Hour, Enabled: true})

	s.DisableSchedule("secrets/ed")
	sched, _ := s.GetSchedule("secrets/ed")
	if sched.Enabled {
		t.Fatal("expected disabled")
	}

	s.EnableSchedule("secrets/ed")
	sched, _ = s.GetSchedule("secrets/ed")
	if !sched.Enabled {
		t.Fatal("expected enabled")
	}
	if sched.FailedAttempts != 0 {
		t.Fatal("failed attempts should reset on enable")
	}
}

func TestScheduler_EnableDisable_NotFound(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	s, _ := NewScheduler(SchedulerConfig{Manager: rm, Store: ms})

	if err := s.EnableSchedule("nope"); err == nil {
		t.Fatal("expected error")
	}
	if err := s.DisableSchedule("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestScheduler_NextDueRotation(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	s, _ := NewScheduler(SchedulerConfig{Manager: rm, Store: ms})

	// No schedules
	next, _ := s.NextDueRotation()
	if next != nil {
		t.Fatal("expected nil")
	}

	s.AddSchedule(&Schedule{Path: "a", Interval: 2 * time.Hour, Enabled: true})
	s.AddSchedule(&Schedule{Path: "b", Interval: time.Hour, Enabled: true})

	next, _ = s.NextDueRotation()
	if next == nil {
		t.Fatal("expected a schedule")
	}
}

func TestScheduler_TriggerRotation_NotFound(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	s, _ := NewScheduler(SchedulerConfig{Manager: rm, Store: ms})

	_, err := s.TriggerRotation("nope")
	if err == nil {
		t.Fatal("expected error for nonexistent schedule")
	}
}

func TestScheduler_StartStop(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})
	s, _ := NewScheduler(SchedulerConfig{Manager: rm, Store: ms})

	s.Start(50 * time.Millisecond)
	// Starting again should be no-op
	s.Start(50 * time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	s.Stop()
	// Stopping again should be no-op
	s.Stop()
}
