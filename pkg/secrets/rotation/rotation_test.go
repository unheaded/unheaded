// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package rotation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// helper to create a MemoryStore with a pre-populated secret.
func newTestStore(t *testing.T, path string, secret *secrets.Secret) secrets.SecretStore {
	t.Helper()
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	if secret != nil {
		if err := ms.Set(context.Background(), path, secret); err != nil {
			t.Fatalf("seed store: %v", err)
		}
	}
	return ms
}

func staticSecret(path string) *secrets.Secret {
	return &secrets.Secret{
		Path:    path,
		Type:    secrets.SecretTypeStatic,
		Version: 1,
		Data: map[string][]byte{
			"value": []byte("old-secret"),
		},
		Metadata:  map[string]string{},
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
	}
}

func certSecret(path string) *secrets.Secret {
	return &secrets.Secret{
		Path:    path,
		Type:    secrets.SecretTypeCertificate,
		Version: 1,
		Data: map[string][]byte{
			"cert": []byte("placeholder"),
			"key":  []byte("placeholder"),
		},
		Metadata:  map[string]string{},
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
	}
}

func sshSecret(path string) *secrets.Secret {
	return &secrets.Secret{
		Path:    path,
		Type:    secrets.SecretTypeSSHKey,
		Version: 1,
		Data: map[string][]byte{
			"public":  []byte("placeholder"),
			"private": []byte("placeholder"),
		},
		Metadata:  map[string]string{},
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
	}
}

// --- NewRotationManager ---

func TestNewRotationManager_NilStore(t *testing.T) {
	_, err := NewRotationManager(RotationManagerConfig{})
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestNewRotationManager_OK(t *testing.T) {
	s := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, err := NewRotationManager(RotationManagerConfig{Store: s})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rm == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestNewRotationManager_NilLogger(t *testing.T) {
	s := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, err := NewRotationManager(RotationManagerConfig{Store: s})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rm.log == nil {
		t.Fatal("expected non-nil logger even when config.Logger is nil")
	}
}

// --- RegisterRotator ---

func TestRegisterRotator(t *testing.T) {
	s := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: s})
	// Default rotators are already registered; re-register with custom.
	custom := &StaticSecretRotator{}
	rm.RegisterRotator(custom)
	rm.mu.RLock()
	r, ok := rm.rotators[secrets.SecretTypeStatic]
	rm.mu.RUnlock()
	if !ok || r != custom {
		t.Fatal("custom rotator not registered")
	}
}

// --- Rotate (static) ---

func TestRotate_Static_Success(t *testing.T) {
	path := "secrets/mypassword"
	ms := newTestStore(t, path, staticSecret(path))

	rm, err := NewRotationManager(RotationManagerConfig{Store: ms})
	if err != nil {
		t.Fatal(err)
	}

	cfg := RotationConfig{
		TTL:          24 * time.Hour,
		Parameters:   map[string]interface{}{"length": 16},
		KeepPrevious: true,
	}

	result, err := rm.Rotate(context.Background(), path, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("rotation not successful: %v", result.Error)
	}
	if result.NewSecret == nil {
		t.Fatal("new secret is nil")
	}
	if result.PreviousSecret == nil {
		t.Fatal("previous secret should be kept")
	}
	val := result.NewSecret.Data["value"]
	if len(val) != 16 {
		t.Fatalf("expected length 16, got %d", len(val))
	}
	if result.Duration == 0 {
		t.Fatal("expected non-zero duration")
	}
}

func TestRotate_Static_KeepPreviousFalse(t *testing.T) {
	path := "secrets/nokeep"
	ms := newTestStore(t, path, staticSecret(path))
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	result, err := rm.Rotate(context.Background(), path, RotationConfig{
		Parameters: map[string]interface{}{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PreviousSecret != nil {
		t.Fatal("previous secret should not be returned")
	}
}

func TestRotate_SecretNotFound(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	result, err := rm.Rotate(context.Background(), "nonexistent", RotationConfig{})
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on error")
	}
}

func TestRotate_NoRotator(t *testing.T) {
	path := "secrets/dynamic"
	s := &secrets.Secret{
		Path:     path,
		Type:     secrets.SecretTypeDynamic, // no rotator for dynamic
		Version:  1,
		Data:     map[string][]byte{"value": []byte("x")},
		Metadata: map[string]string{},
	}
	ms := newTestStore(t, path, s)
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	_, err := rm.Rotate(context.Background(), path, RotationConfig{})
	if err == nil {
		t.Fatal("expected error for missing rotator")
	}
	if !errors.Is(err, ErrNoRotator) {
		t.Fatalf("expected ErrNoRotator, got: %v", err)
	}
}

func TestRotate_AlreadyInProgress(t *testing.T) {
	path := "secrets/concurrent"
	ms := newTestStore(t, path, staticSecret(path))
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	// Manually mark as in-progress
	rm.mu.Lock()
	rm.inProgress[path] = true
	rm.mu.Unlock()

	_, err := rm.Rotate(context.Background(), path, RotationConfig{})
	if !errors.Is(err, ErrRotationInProgress) {
		t.Fatalf("expected ErrRotationInProgress, got: %v", err)
	}
}

func TestRotate_ConcurrentDifferentPaths(t *testing.T) {
	ms := store.NewMemoryStore(store.MemoryStoreConfig{})
	for i := 0; i < 5; i++ {
		p := "secrets/concurrent/" + string(rune('a'+i))
		_ = ms.Set(context.Background(), p, staticSecret(p))
	}
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := "secrets/concurrent/" + string(rune('a'+idx))
			_, err := rm.Rotate(context.Background(), p, RotationConfig{
				Parameters: map[string]interface{}{},
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- StaticSecretRotator ---

func TestStaticSecretRotator_Type(t *testing.T) {
	r := &StaticSecretRotator{}
	if r.Type() != secrets.SecretTypeStatic {
		t.Fatalf("expected %s, got %s", secrets.SecretTypeStatic, r.Type())
	}
}

func TestStaticSecretRotator_Rotate_DefaultLength(t *testing.T) {
	r := &StaticSecretRotator{}
	s := staticSecret("test")
	cfg := RotationConfig{Parameters: map[string]interface{}{}}
	newS, err := r.Rotate(context.Background(), s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	val := newS.Data["value"]
	if len(val) != 32 {
		t.Fatalf("expected 32, got %d", len(val))
	}
}

func TestStaticSecretRotator_Rotate_CustomCharset(t *testing.T) {
	r := &StaticSecretRotator{}
	s := staticSecret("test")
	cfg := RotationConfig{
		Parameters: map[string]interface{}{
			"length":  8,
			"charset": "abc",
		},
	}
	newS, err := r.Rotate(context.Background(), s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	val := newS.Data["value"]
	for _, b := range val {
		if b != 'a' && b != 'b' && b != 'c' {
			t.Fatalf("unexpected character %c in value", b)
		}
	}
}

func TestStaticSecretRotator_Rotate_TTL(t *testing.T) {
	r := &StaticSecretRotator{}
	s := staticSecret("test")
	cfg := RotationConfig{
		TTL:        time.Hour,
		Parameters: map[string]interface{}{},
	}
	newS, err := r.Rotate(context.Background(), s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if newS.ExpiresAt.IsZero() {
		t.Fatal("expected non-zero ExpiresAt")
	}
	if time.Until(newS.ExpiresAt) < 59*time.Minute {
		t.Fatal("expiry too soon")
	}
}

func TestStaticSecretRotator_Verify(t *testing.T) {
	r := &StaticSecretRotator{}
	s := staticSecret("test")
	if err := r.Verify(context.Background(), s); err != nil {
		t.Fatal(err)
	}

	noValue := &secrets.Secret{Data: map[string][]byte{}}
	if err := r.Verify(context.Background(), noValue); err == nil {
		t.Fatal("expected error for missing value")
	}
}

func TestStaticSecretRotator_Rollback(t *testing.T) {
	r := &StaticSecretRotator{}
	if err := r.Rollback(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
}

// --- CertificateRotator ---

func TestCertificateRotator_Type(t *testing.T) {
	r := &CertificateRotator{}
	if r.Type() != secrets.SecretTypeCertificate {
		t.Fatalf("expected certificate, got %s", r.Type())
	}
}

func TestCertificateRotator_RotateAndVerify(t *testing.T) {
	r := &CertificateRotator{}
	s := certSecret("certs/test")
	cfg := RotationConfig{
		TTL: 24 * time.Hour,
		Parameters: map[string]interface{}{
			"common_name": "test.example.com",
			"key_size":    2048,
		},
	}
	newS, err := r.Rotate(context.Background(), s, cfg)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if err := r.Verify(context.Background(), newS); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestCertificateRotator_Verify_Missing(t *testing.T) {
	r := &CertificateRotator{}
	noCert := &secrets.Secret{Data: map[string][]byte{"key": []byte("x")}}
	if err := r.Verify(context.Background(), noCert); err == nil {
		t.Fatal("expected error for missing cert")
	}
	noKey := &secrets.Secret{Data: map[string][]byte{"cert": []byte("x")}}
	if err := r.Verify(context.Background(), noKey); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestCertificateRotator_Rollback(t *testing.T) {
	r := &CertificateRotator{}
	if err := r.Rollback(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
}

// --- SSHKeyRotator ---

func TestSSHKeyRotator_Type(t *testing.T) {
	r := &SSHKeyRotator{}
	if r.Type() != secrets.SecretTypeSSHKey {
		t.Fatalf("expected ssh_key, got %s", r.Type())
	}
}

func TestSSHKeyRotator_Ed25519_RotateAndVerify(t *testing.T) {
	r := &SSHKeyRotator{}
	s := sshSecret("ssh/ed25519")
	cfg := RotationConfig{
		Parameters: map[string]interface{}{
			"key_type": "ed25519",
			"comment":  "test@unheaded",
		},
	}
	newS, err := r.Rotate(context.Background(), s, cfg)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if err := r.Verify(context.Background(), newS); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestSSHKeyRotator_RSA_RotateAndVerify(t *testing.T) {
	r := &SSHKeyRotator{}
	s := sshSecret("ssh/rsa")
	cfg := RotationConfig{
		Parameters: map[string]interface{}{
			"key_type": "rsa",
			"key_size": 2048,
		},
	}
	newS, err := r.Rotate(context.Background(), s, cfg)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if err := r.Verify(context.Background(), newS); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestSSHKeyRotator_UnsupportedType(t *testing.T) {
	r := &SSHKeyRotator{}
	s := sshSecret("ssh/unsupported")
	cfg := RotationConfig{
		Parameters: map[string]interface{}{
			"key_type": "ecdsa384",
		},
	}
	_, err := r.Rotate(context.Background(), s, cfg)
	if err == nil {
		t.Fatal("expected error for unsupported key type")
	}
}

func TestSSHKeyRotator_Verify_MissingKeys(t *testing.T) {
	r := &SSHKeyRotator{}
	noPriv := &secrets.Secret{Data: map[string][]byte{"public": []byte("x")}}
	if err := r.Verify(context.Background(), noPriv); err == nil {
		t.Fatal("expected error for missing private key")
	}
	noPub := &secrets.Secret{Data: map[string][]byte{"private": []byte("x")}}
	if err := r.Verify(context.Background(), noPub); err == nil {
		t.Fatal("expected error for missing public key")
	}
}

func TestSSHKeyRotator_TTL(t *testing.T) {
	r := &SSHKeyRotator{}
	s := sshSecret("ssh/ttl")
	cfg := RotationConfig{
		TTL:        2 * time.Hour,
		Parameters: map[string]interface{}{},
	}
	newS, err := r.Rotate(context.Background(), s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if newS.ExpiresAt.IsZero() {
		t.Fatal("expected non-zero ExpiresAt with TTL")
	}
}

func TestSSHKeyRotator_Rollback(t *testing.T) {
	r := &SSHKeyRotator{}
	if err := r.Rollback(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
}

// --- Rotate end-to-end with certificate ---

func TestRotate_Certificate_EndToEnd(t *testing.T) {
	path := "certs/tls"
	ms := newTestStore(t, path, certSecret(path))
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	result, err := rm.Rotate(context.Background(), path, RotationConfig{
		TTL: 48 * time.Hour,
		Parameters: map[string]interface{}{
			"common_name": "e2e.test",
			"key_size":    2048,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("expected success, error: %v", result.Error)
	}
}

// --- Rotate end-to-end with SSH ---

func TestRotate_SSH_EndToEnd(t *testing.T) {
	path := "ssh/deploy"
	ms := newTestStore(t, path, sshSecret(path))
	rm, _ := NewRotationManager(RotationManagerConfig{Store: ms})

	result, err := rm.Rotate(context.Background(), path, RotationConfig{
		Parameters: map[string]interface{}{
			"key_type": "ed25519",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("expected success, error: %v", result.Error)
	}
}

// --- RotationConfig zero-value ---

func TestRotationConfig_ZeroValue(t *testing.T) {
	cfg := RotationConfig{}
	if cfg.TTL != 0 || cfg.KeepPrevious || cfg.GracePeriod != 0 {
		t.Fatal("zero value RotationConfig should have zero defaults")
	}
}

// --- RotationResult ---

func TestRotationResult_Fields(t *testing.T) {
	r := &RotationResult{
		Success:   true,
		Timestamp: time.Now(),
	}
	if !r.Success {
		t.Fatal("expected success")
	}
}
