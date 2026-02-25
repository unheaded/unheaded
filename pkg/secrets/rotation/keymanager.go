// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rotation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/sha3"

	"unheaded/pkg/logger"
	"unheaded/pkg/secrets"
)

// Common key management errors.
var (
	ErrKeyNotFound       = errors.New("key not found")
	ErrKeyRevoked        = errors.New("key has been revoked")
	ErrKeyExpired        = errors.New("key has expired")
	ErrInvalidKeyVersion = errors.New("invalid key version")
	ErrEscrowNotEnabled  = errors.New("key escrow not enabled")
	ErrRecoveryFailed    = errors.New("key recovery failed")
)

// KeyStatus represents the status of a key.
type KeyStatus string

const (
	// KeyStatusActive indicates the key is active and usable.
	KeyStatusActive KeyStatus = "active"

	// KeyStatusPending indicates the key is created but not yet activated.
	KeyStatusPending KeyStatus = "pending"

	// KeyStatusRotating indicates the key is being rotated.
	KeyStatusRotating KeyStatus = "rotating"

	// KeyStatusRevoked indicates the key has been revoked.
	KeyStatusRevoked KeyStatus = "revoked"

	// KeyStatusExpired indicates the key has expired.
	KeyStatusExpired KeyStatus = "expired"

	// KeyStatusEscrowed indicates the key is held in escrow.
	KeyStatusEscrowed KeyStatus = "escrowed"
)

// KeyVersion represents a versioned key.
type KeyVersion struct {
	// Version is the key version number.
	Version int64 `json:"version"`

	// KeyID is the unique identifier for this key version.
	KeyID string `json:"key_id"`

	// Status is the key status.
	Status KeyStatus `json:"status"`

	// Algorithm is the algorithm used for this key.
	Algorithm string `json:"algorithm"`

	// KeyData contains the encrypted key data (if stored).
	KeyData []byte `json:"key_data,omitempty"`

	// KeyHash is a hash of the key for verification.
	KeyHash string `json:"key_hash"`

	// CreatedAt is when the key version was created.
	CreatedAt time.Time `json:"created_at"`

	// ActivatedAt is when the key became active.
	ActivatedAt time.Time `json:"activated_at,omitempty"`

	// ExpiresAt is when the key expires.
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// RevokedAt is when the key was revoked.
	RevokedAt time.Time `json:"revoked_at,omitempty"`

	// RevocationReason is why the key was revoked.
	RevocationReason string `json:"revocation_reason,omitempty"`

	// DerivedFrom is the parent key ID this key was derived from.
	DerivedFrom string `json:"derived_from,omitempty"`

	// DerivationContext is the context used for derivation.
	DerivationContext string `json:"derivation_context,omitempty"`

	// Metadata contains additional key metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ManagedKey represents a key with version history.
type ManagedKey struct {
	// ID is the unique identifier for this managed key.
	ID string `json:"id"`

	// Name is a human-readable name.
	Name string `json:"name"`

	// Purpose describes what the key is used for.
	Purpose string `json:"purpose"`

	// CurrentVersion is the current active version.
	CurrentVersion int64 `json:"current_version"`

	// Versions contains all key versions.
	Versions map[int64]*KeyVersion `json:"versions"`

	// RotationPolicy is the rotation policy for this key.
	RotationPolicy *Policy `json:"rotation_policy,omitempty"`

	// EscrowEnabled indicates if key escrow is enabled.
	EscrowEnabled bool `json:"escrow_enabled"`

	// CreatedAt is when the managed key was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the managed key was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// KeyManager manages cryptographic keys with versioning.
type KeyManager struct {
	keys         map[string]*ManagedKey
	store        secrets.SecretStore
	escrow       EscrowProvider
	log          *logger.Logger
	mu           sync.RWMutex
	derivedKeys  map[string][]byte // Cache for derived keys
	derivedMu    sync.RWMutex
}

// KeyManagerConfig configures the key manager.
type KeyManagerConfig struct {
	// Store is the secret store for persisting keys.
	Store secrets.SecretStore

	// Escrow is the optional escrow provider.
	Escrow EscrowProvider

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewKeyManager creates a new key manager.
func NewKeyManager(config KeyManagerConfig) (*KeyManager, error) {
	if config.Store == nil {
		return nil, errors.New("store is required")
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	return &KeyManager{
		keys:        make(map[string]*ManagedKey),
		store:       config.Store,
		escrow:      config.Escrow,
		log:         log.With().Str("component", "key-manager").Logger(),
		derivedKeys: make(map[string][]byte),
	}, nil
}

// CreateKey creates a new managed key.
func (km *KeyManager) CreateKey(ctx context.Context, id, name, purpose string, keySize int) (*ManagedKey, error) {
	km.mu.Lock()
	defer km.mu.Unlock()

	if _, exists := km.keys[id]; exists {
		return nil, fmt.Errorf("key already exists: %s", id)
	}

	// Generate key material
	keyData := make([]byte, keySize)
	if _, err := rand.Read(keyData); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	now := time.Now()
	keyID := fmt.Sprintf("%s-v1-%d", id, now.UnixNano())
	keyHash := hashKey(keyData)

	version := &KeyVersion{
		Version:     1,
		KeyID:       keyID,
		Status:      KeyStatusActive,
		Algorithm:   "AES-256-GCM",
		KeyData:     keyData,
		KeyHash:     keyHash,
		CreatedAt:   now,
		ActivatedAt: now,
	}

	key := &ManagedKey{
		ID:             id,
		Name:           name,
		Purpose:        purpose,
		CurrentVersion: 1,
		Versions:       map[int64]*KeyVersion{1: version},
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	km.keys[id] = key

	// Persist to store
	if err := km.persistKey(ctx, key); err != nil {
		delete(km.keys, id)
		return nil, fmt.Errorf("persist key: %w", err)
	}

	// Escrow if enabled
	if km.escrow != nil && key.EscrowEnabled {
		if err := km.escrow.Store(ctx, keyID, keyData); err != nil {
			km.log.Warn().Err(err).Str("key_id", id).Msg("failed to escrow key")
		}
	}

	km.log.Info().
		Str("id", id).
		Str("name", name).
		Int64("version", version.Version).
		Msg("created managed key")

	return key, nil
}

// GetKey retrieves a managed key.
func (km *KeyManager) GetKey(id string) (*ManagedKey, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	key, ok := km.keys[id]
	if !ok {
		return nil, ErrKeyNotFound
	}

	return key, nil
}

// GetKeyVersion retrieves a specific version of a key.
func (km *KeyManager) GetKeyVersion(id string, version int64) (*KeyVersion, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	key, ok := km.keys[id]
	if !ok {
		return nil, ErrKeyNotFound
	}

	kv, ok := key.Versions[version]
	if !ok {
		return nil, ErrInvalidKeyVersion
	}

	if kv.Status == KeyStatusRevoked {
		return nil, ErrKeyRevoked
	}

	if kv.Status == KeyStatusExpired || (!kv.ExpiresAt.IsZero() && time.Now().After(kv.ExpiresAt)) {
		return nil, ErrKeyExpired
	}

	return kv, nil
}

// GetCurrentKeyMaterial retrieves the current key material.
func (km *KeyManager) GetCurrentKeyMaterial(id string) ([]byte, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	key, ok := km.keys[id]
	if !ok {
		return nil, ErrKeyNotFound
	}

	kv, ok := key.Versions[key.CurrentVersion]
	if !ok {
		return nil, ErrInvalidKeyVersion
	}

	if kv.Status == KeyStatusRevoked {
		return nil, ErrKeyRevoked
	}

	if kv.Status == KeyStatusExpired || (!kv.ExpiresAt.IsZero() && time.Now().After(kv.ExpiresAt)) {
		return nil, ErrKeyExpired
	}

	// Return a copy
	result := make([]byte, len(kv.KeyData))
	copy(result, kv.KeyData)
	return result, nil
}

// RotateKey rotates a managed key to a new version.
func (km *KeyManager) RotateKey(ctx context.Context, id string, keySize int) (*KeyVersion, error) {
	km.mu.Lock()
	defer km.mu.Unlock()

	key, ok := km.keys[id]
	if !ok {
		return nil, ErrKeyNotFound
	}

	// Generate new key material
	keyData := make([]byte, keySize)
	if _, err := rand.Read(keyData); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	now := time.Now()
	newVersion := key.CurrentVersion + 1
	keyID := fmt.Sprintf("%s-v%d-%d", id, newVersion, now.UnixNano())
	keyHash := hashKey(keyData)

	version := &KeyVersion{
		Version:     newVersion,
		KeyID:       keyID,
		Status:      KeyStatusActive,
		Algorithm:   "AES-256-GCM",
		KeyData:     keyData,
		KeyHash:     keyHash,
		CreatedAt:   now,
		ActivatedAt: now,
	}

	// Mark previous version as pending rotation
	if prevVersion, ok := key.Versions[key.CurrentVersion]; ok {
		prevVersion.Status = KeyStatusRotating
	}

	key.Versions[newVersion] = version
	key.CurrentVersion = newVersion
	key.UpdatedAt = now

	// Persist
	if err := km.persistKey(ctx, key); err != nil {
		delete(key.Versions, newVersion)
		key.CurrentVersion = newVersion - 1
		return nil, fmt.Errorf("persist key: %w", err)
	}

	// Escrow if enabled
	if km.escrow != nil && key.EscrowEnabled {
		if err := km.escrow.Store(ctx, keyID, keyData); err != nil {
			km.log.Warn().Err(err).Str("key_id", id).Msg("failed to escrow rotated key")
		}
	}

	km.log.Info().
		Str("id", id).
		Int64("old_version", newVersion-1).
		Int64("new_version", newVersion).
		Msg("rotated key")

	return version, nil
}

// RevokeKeyVersion revokes a specific key version.
func (km *KeyManager) RevokeKeyVersion(ctx context.Context, id string, version int64, reason string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	key, ok := km.keys[id]
	if !ok {
		return ErrKeyNotFound
	}

	kv, ok := key.Versions[version]
	if !ok {
		return ErrInvalidKeyVersion
	}

	if kv.Status == KeyStatusRevoked {
		return nil // Already revoked
	}

	now := time.Now()
	kv.Status = KeyStatusRevoked
	kv.RevokedAt = now
	kv.RevocationReason = reason

	// Clear key data
	for i := range kv.KeyData {
		kv.KeyData[i] = 0
	}
	kv.KeyData = nil

	key.UpdatedAt = now

	// Persist
	if err := km.persistKey(ctx, key); err != nil {
		return fmt.Errorf("persist key: %w", err)
	}

	km.log.Info().
		Str("id", id).
		Int64("version", version).
		Str("reason", reason).
		Msg("revoked key version")

	return nil
}

// RevokeAllVersions revokes all versions of a key.
func (km *KeyManager) RevokeAllVersions(ctx context.Context, id string, reason string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	key, ok := km.keys[id]
	if !ok {
		return ErrKeyNotFound
	}

	now := time.Now()
	for _, kv := range key.Versions {
		if kv.Status != KeyStatusRevoked {
			kv.Status = KeyStatusRevoked
			kv.RevokedAt = now
			kv.RevocationReason = reason

			// Clear key data
			for i := range kv.KeyData {
				kv.KeyData[i] = 0
			}
			kv.KeyData = nil
		}
	}

	key.UpdatedAt = now

	// Persist
	if err := km.persistKey(ctx, key); err != nil {
		return fmt.Errorf("persist key: %w", err)
	}

	km.log.Info().
		Str("id", id).
		Str("reason", reason).
		Msg("revoked all key versions")

	return nil
}

// DeriveKey derives a key for a specific service/purpose.
func (km *KeyManager) DeriveKey(id, context string, keySize int) ([]byte, error) {
	// Check cache
	cacheKey := fmt.Sprintf("%s:%s:%d", id, context, keySize)
	km.derivedMu.RLock()
	if derived, ok := km.derivedKeys[cacheKey]; ok {
		km.derivedMu.RUnlock()
		result := make([]byte, len(derived))
		copy(result, derived)
		return result, nil
	}
	km.derivedMu.RUnlock()

	// Get master key
	masterKey, err := km.GetCurrentKeyMaterial(id)
	if err != nil {
		return nil, fmt.Errorf("get master key: %w", err)
	}

	// Derive key using HKDF
	derivedKey := make([]byte, keySize)
	h := hkdf.New(sha3.New256, masterKey, []byte(context), []byte("key-derivation"))
	if _, err := io.ReadFull(h, derivedKey); err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	// Cache
	km.derivedMu.Lock()
	derivedCopy := make([]byte, len(derivedKey))
	copy(derivedCopy, derivedKey)
	km.derivedKeys[cacheKey] = derivedCopy
	km.derivedMu.Unlock()

	return derivedKey, nil
}

// DeriveKeyForService derives a key for a specific service.
func (km *KeyManager) DeriveKeyForService(id, service string, keySize int) ([]byte, error) {
	context := fmt.Sprintf("service:%s", service)
	return km.DeriveKey(id, context, keySize)
}

// EnableEscrow enables escrow for a key.
func (km *KeyManager) EnableEscrow(ctx context.Context, id string) error {
	if km.escrow == nil {
		return ErrEscrowNotEnabled
	}

	km.mu.Lock()
	defer km.mu.Unlock()

	key, ok := km.keys[id]
	if !ok {
		return ErrKeyNotFound
	}

	key.EscrowEnabled = true

	// Escrow all active versions
	for _, kv := range key.Versions {
		if kv.Status == KeyStatusActive && len(kv.KeyData) > 0 {
			if err := km.escrow.Store(ctx, kv.KeyID, kv.KeyData); err != nil {
				km.log.Warn().Err(err).Str("key_id", kv.KeyID).Msg("failed to escrow key version")
			}
		}
	}

	key.UpdatedAt = time.Now()
	return km.persistKey(ctx, key)
}

// RecoverKey recovers a key from escrow.
func (km *KeyManager) RecoverKey(ctx context.Context, keyID string) ([]byte, error) {
	if km.escrow == nil {
		return nil, ErrEscrowNotEnabled
	}

	keyData, err := km.escrow.Retrieve(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecoveryFailed, err)
	}

	km.log.Info().Str("key_id", keyID).Msg("recovered key from escrow")
	return keyData, nil
}

// ListKeys returns all managed keys.
func (km *KeyManager) ListKeys() []*ManagedKey {
	km.mu.RLock()
	defer km.mu.RUnlock()

	keys := make([]*ManagedKey, 0, len(km.keys))
	for _, k := range km.keys {
		keys = append(keys, k)
	}
	return keys
}

// GetKeyHistory returns the version history for a key.
func (km *KeyManager) GetKeyHistory(id string) ([]*KeyVersion, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	key, ok := km.keys[id]
	if !ok {
		return nil, ErrKeyNotFound
	}

	versions := make([]*KeyVersion, 0, len(key.Versions))
	for _, v := range key.Versions {
		// Don't include key data in history
		vCopy := *v
		vCopy.KeyData = nil
		versions = append(versions, &vCopy)
	}

	// Sort by version
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})

	return versions, nil
}

// persistKey persists a key to the store.
func (km *KeyManager) persistKey(ctx context.Context, key *ManagedKey) error {
	// Convert to secret
	data, err := json.Marshal(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	secret := &secrets.Secret{
		Path: fmt.Sprintf("keys/%s", key.ID),
		Type: secrets.SecretTypeKEK,
		Data: map[string][]byte{
			"key": data,
		},
		Metadata: map[string]string{
			"name":    key.Name,
			"purpose": key.Purpose,
		},
		Version:   key.CurrentVersion,
		UpdatedAt: key.UpdatedAt,
	}

	return km.store.Set(ctx, secret.Path, secret)
}

// LoadKeys loads keys from the store.
func (km *KeyManager) LoadKeys(ctx context.Context) error {
	paths, err := km.store.List(ctx, "keys/")
	if err != nil {
		return fmt.Errorf("list keys: %w", err)
	}

	km.mu.Lock()
	defer km.mu.Unlock()

	for _, path := range paths {
		secret, err := km.store.Get(ctx, path)
		if err != nil {
			km.log.Warn().Err(err).Str("path", path).Msg("failed to load key")
			continue
		}

		data, ok := secret.Data["key"]
		if !ok {
			continue
		}

		var key ManagedKey
		if err := json.Unmarshal(data, &key); err != nil {
			km.log.Warn().Err(err).Str("path", path).Msg("failed to unmarshal key")
			continue
		}

		km.keys[key.ID] = &key
	}

	km.log.Info().Int("count", len(km.keys)).Msg("loaded keys")
	return nil
}

// hashKey creates a hash of key material for verification.
func hashKey(data []byte) string {
	h := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(h[:])
}

// EscrowProvider is the interface for key escrow storage.
type EscrowProvider interface {
	// Store stores a key in escrow.
	Store(ctx context.Context, keyID string, keyData []byte) error

	// Retrieve retrieves a key from escrow.
	Retrieve(ctx context.Context, keyID string) ([]byte, error)

	// Delete removes a key from escrow.
	Delete(ctx context.Context, keyID string) error
}

// FileEscrowProvider stores escrowed keys in encrypted files.
type FileEscrowProvider struct {
	basePath    string
	encryptKey  []byte
	log         *logger.Logger
	mu          sync.RWMutex
}

// FileEscrowConfig configures the file escrow provider.
type FileEscrowConfig struct {
	// BasePath is the directory for escrow files.
	BasePath string

	// EncryptKey is the key used to encrypt escrowed keys.
	EncryptKey []byte

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewFileEscrowProvider creates a new file escrow provider.
func NewFileEscrowProvider(config FileEscrowConfig) (*FileEscrowProvider, error) {
	if config.BasePath == "" {
		return nil, errors.New("base path is required")
	}
	if len(config.EncryptKey) < 32 {
		return nil, errors.New("encrypt key must be at least 32 bytes")
	}

	if err := os.MkdirAll(config.BasePath, 0700); err != nil {
		return nil, fmt.Errorf("create escrow directory: %w", err)
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	encryptKey := make([]byte, 32)
	copy(encryptKey, config.EncryptKey[:32])

	return &FileEscrowProvider{
		basePath:   config.BasePath,
		encryptKey: encryptKey,
		log:        log.With().Str("component", "file-escrow").Logger(),
	}, nil
}

// escrowPath returns the file path for an escrowed key.
func (e *FileEscrowProvider) escrowPath(keyID string) string {
	// Hash the key ID for filename
	h := sha256.Sum256([]byte(keyID))
	filename := base64.RawURLEncoding.EncodeToString(h[:16]) + ".escrow"
	return filepath.Join(e.basePath, filename)
}

// Store stores a key in escrow.
func (e *FileEscrowProvider) Store(ctx context.Context, keyID string, keyData []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Encrypt the key data
	encrypted, err := e.encrypt(keyData, keyID)
	if err != nil {
		return fmt.Errorf("encrypt key: %w", err)
	}

	filePath := e.escrowPath(keyID)
	if err := os.WriteFile(filePath, encrypted, 0600); err != nil {
		return fmt.Errorf("write escrow file: %w", err)
	}

	e.log.Info().Str("key_id", keyID).Msg("stored key in escrow")
	return nil
}

// Retrieve retrieves a key from escrow.
func (e *FileEscrowProvider) Retrieve(ctx context.Context, keyID string) ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	filePath := e.escrowPath(keyID)
	encrypted, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("read escrow file: %w", err)
	}

	keyData, err := e.decrypt(encrypted, keyID)
	if err != nil {
		return nil, fmt.Errorf("decrypt key: %w", err)
	}

	e.log.Info().Str("key_id", keyID).Msg("retrieved key from escrow")
	return keyData, nil
}

// Delete removes a key from escrow.
func (e *FileEscrowProvider) Delete(ctx context.Context, keyID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	filePath := e.escrowPath(keyID)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("remove escrow file: %w", err)
	}

	e.log.Info().Str("key_id", keyID).Msg("deleted key from escrow")
	return nil
}

// encrypt encrypts data using the escrow key.
func (e *FileEscrowProvider) encrypt(data []byte, context string) ([]byte, error) {
	// Derive a unique key for this context
	derivedKey := make([]byte, 32)
	h := hkdf.New(sha256.New, e.encryptKey, []byte(context), []byte("escrow-encryption"))
	if _, err := io.ReadFull(h, derivedKey); err != nil {
		return nil, err
	}

	// Simple XOR encryption for escrow (in production, use proper AEAD)
	// This is simplified - real implementation should use ChaCha20-Poly1305
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	encrypted := make([]byte, len(nonce)+len(data))
	copy(encrypted, nonce)

	for i, b := range data {
		encrypted[len(nonce)+i] = b ^ derivedKey[i%len(derivedKey)] ^ nonce[i%len(nonce)]
	}

	return encrypted, nil
}

// decrypt decrypts data using the escrow key.
func (e *FileEscrowProvider) decrypt(encrypted []byte, context string) ([]byte, error) {
	if len(encrypted) < 24 {
		return nil, errors.New("encrypted data too short")
	}

	// Derive a unique key for this context
	derivedKey := make([]byte, 32)
	h := hkdf.New(sha256.New, e.encryptKey, []byte(context), []byte("escrow-encryption"))
	if _, err := io.ReadFull(h, derivedKey); err != nil {
		return nil, err
	}

	nonce := encrypted[:24]
	ciphertext := encrypted[24:]

	data := make([]byte, len(ciphertext))
	for i, b := range ciphertext {
		data[i] = b ^ derivedKey[i%len(derivedKey)] ^ nonce[i%len(nonce)]
	}

	return data, nil
}

// MemoryEscrowProvider stores escrowed keys in memory (for testing).
type MemoryEscrowProvider struct {
	keys map[string][]byte
	mu   sync.RWMutex
}

// NewMemoryEscrowProvider creates a new in-memory escrow provider.
func NewMemoryEscrowProvider() *MemoryEscrowProvider {
	return &MemoryEscrowProvider{
		keys: make(map[string][]byte),
	}
}

// Store stores a key in escrow.
func (e *MemoryEscrowProvider) Store(ctx context.Context, keyID string, keyData []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	dataCopy := make([]byte, len(keyData))
	copy(dataCopy, keyData)
	e.keys[keyID] = dataCopy

	return nil
}

// Retrieve retrieves a key from escrow.
func (e *MemoryEscrowProvider) Retrieve(ctx context.Context, keyID string) ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	data, ok := e.keys[keyID]
	if !ok {
		return nil, ErrKeyNotFound
	}

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy, nil
}

// Delete removes a key from escrow.
func (e *MemoryEscrowProvider) Delete(ctx context.Context, keyID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.keys, keyID)
	return nil
}
