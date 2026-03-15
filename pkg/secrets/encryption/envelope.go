// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package encryption

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/sha3"

	"unheaded/pkg/logger"
)

// EnvelopeEncryptor implements envelope encryption.
// In envelope encryption, data is encrypted with a Data Encryption Key (DEK),
// and the DEK is encrypted with a Key Encryption Key (KEK).
type EnvelopeEncryptor struct {
	kekProvider KEKProvider
	cache       *dekCache
	log         *logger.Logger
	mu          sync.RWMutex
}

// KEKProvider provides Key Encryption Keys.
type KEKProvider interface {
	// GetKEK retrieves a KEK by ID.
	GetKEK(ctx context.Context, keyID string) ([]byte, error)

	// CurrentKEKID returns the ID of the current KEK for encryption.
	CurrentKEKID() string

	// RotateKEK creates a new KEK and returns its ID.
	RotateKEK(ctx context.Context) (string, error)
}

// EnvelopeConfig configures the envelope encryptor.
type EnvelopeConfig struct {
	// KEKProvider provides key encryption keys.
	KEKProvider KEKProvider

	// DEKCacheSize is the maximum number of DEKs to cache.
	DEKCacheSize int

	// DEKCacheTTL is how long to cache DEKs.
	DEKCacheTTL time.Duration

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewEnvelopeEncryptor creates a new envelope encryptor.
func NewEnvelopeEncryptor(config EnvelopeConfig) (*EnvelopeEncryptor, error) {
	if config.KEKProvider == nil {
		return nil, errors.New("KEK provider is required")
	}

	cacheSize := config.DEKCacheSize
	if cacheSize <= 0 {
		cacheSize = 100
	}

	cacheTTL := config.DEKCacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	return &EnvelopeEncryptor{
		kekProvider: config.KEKProvider,
		cache:       newDEKCache(cacheSize, cacheTTL),
		log:         log.With().Str("component", "envelope-encryptor").Logger(),
	}, nil
}

// Encrypt encrypts data using envelope encryption.
func (e *EnvelopeEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	ctx := context.Background()

	// Generate a new DEK
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("generate DEK: %w", err)
	}

	// Encrypt data with DEK
	dataAEAD, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return nil, err
	}

	dataNonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(dataNonce); err != nil {
		return nil, err
	}

	encryptedData := dataAEAD.Seal(nil, dataNonce, plaintext, nil)

	// Get current KEK
	kekID := e.kekProvider.CurrentKEKID()
	kek, err := e.kekProvider.GetKEK(ctx, kekID)
	if err != nil {
		return nil, fmt.Errorf("get KEK: %w", err)
	}

	// Encrypt DEK with KEK
	kekAEAD, err := chacha20poly1305.NewX(kek)
	if err != nil {
		return nil, err
	}

	kekNonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(kekNonce); err != nil {
		return nil, err
	}

	encryptedDEK := kekAEAD.Seal(nil, kekNonce, dek, nil)

	// Build envelope
	envelope := &Envelope{
		Version:      1,
		KEKID:        kekID,
		EncryptedDEK: base64.StdEncoding.EncodeToString(append(kekNonce, encryptedDEK...)),
		DataNonce:    base64.StdEncoding.EncodeToString(dataNonce),
		Ciphertext:   base64.StdEncoding.EncodeToString(encryptedData),
	}

	return json.Marshal(envelope)
}

// Decrypt decrypts data using envelope encryption.
func (e *EnvelopeEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	ctx := context.Background()

	var envelope Envelope
	if err := json.Unmarshal(ciphertext, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}

	if envelope.Version != 1 {
		return nil, fmt.Errorf("unsupported envelope version: %d", envelope.Version)
	}

	// Decode encrypted DEK
	encryptedDEKWithNonce, err := base64.StdEncoding.DecodeString(envelope.EncryptedDEK)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted DEK: %w", err)
	}

	if len(encryptedDEKWithNonce) < chacha20poly1305.NonceSizeX {
		return nil, errors.New("encrypted DEK too short")
	}

	kekNonce := encryptedDEKWithNonce[:chacha20poly1305.NonceSizeX]
	encryptedDEK := encryptedDEKWithNonce[chacha20poly1305.NonceSizeX:]

	// Try to get DEK from cache
	cacheKey := base64.StdEncoding.EncodeToString(encryptedDEK)
	dek, ok := e.cache.Get(cacheKey)
	if !ok {
		// Get KEK and decrypt DEK
		kek, err := e.kekProvider.GetKEK(ctx, envelope.KEKID)
		if err != nil {
			return nil, fmt.Errorf("get KEK %s: %w", envelope.KEKID, err)
		}

		kekAEAD, err := chacha20poly1305.NewX(kek)
		if err != nil {
			return nil, err
		}

		dek, err = kekAEAD.Open(nil, kekNonce, encryptedDEK, nil)
		if err != nil {
			return nil, fmt.Errorf("decrypt DEK: %w", err)
		}

		// Cache the DEK
		e.cache.Set(cacheKey, dek)
	}

	// Decrypt data with DEK
	dataNonce, err := base64.StdEncoding.DecodeString(envelope.DataNonce)
	if err != nil {
		return nil, fmt.Errorf("decode data nonce: %w", err)
	}

	encryptedData, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	dataAEAD, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return nil, err
	}

	plaintext, err := dataAEAD.Open(nil, dataNonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt data: %w", err)
	}

	return plaintext, nil
}

// Envelope is the encrypted data structure.
type Envelope struct {
	// Version is the envelope format version.
	Version int `json:"version"`

	// KEKID is the ID of the KEK used to encrypt the DEK.
	KEKID string `json:"kek_id"`

	// EncryptedDEK is the encrypted data encryption key (base64).
	EncryptedDEK string `json:"encrypted_dek"`

	// DataNonce is the nonce used for data encryption (base64).
	DataNonce string `json:"data_nonce"`

	// Ciphertext is the encrypted data (base64).
	Ciphertext string `json:"ciphertext"`
}

// dekCache caches decrypted DEKs for performance.
type dekCache struct {
	entries map[string]*dekCacheEntry
	maxSize int
	ttl     time.Duration
	mu      sync.RWMutex
}

type dekCacheEntry struct {
	dek       []byte
	expiresAt time.Time
}

func newDEKCache(maxSize int, ttl time.Duration) *dekCache {
	c := &dekCache{
		entries: make(map[string]*dekCacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}

	// Start cleanup goroutine
	go c.cleanup()

	return c
}

func (c *dekCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}

	// Return a copy
	dek := make([]byte, len(entry.dek))
	copy(dek, entry.dek)
	return dek, true
}

func (c *dekCache) Set(key string, dek []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxSize {
		// Remove oldest entry
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldestKey == "" || v.expiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.expiresAt
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}

	// Store a copy
	dekCopy := make([]byte, len(dek))
	copy(dekCopy, dek)

	c.entries[key] = &dekCacheEntry{
		dek:       dekCopy,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *dekCache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.After(entry.expiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

// LocalKEKProvider provides KEKs from local storage.
// This is suitable for development and single-node deployments.
type LocalKEKProvider struct {
	keys      map[string][]byte
	currentID string
	mu        sync.RWMutex
}

// NewLocalKEKProvider creates a new local KEK provider.
func NewLocalKEKProvider() (*LocalKEKProvider, error) {
	p := &LocalKEKProvider{
		keys: make(map[string][]byte),
	}

	// Generate initial KEK
	if _, err := p.RotateKEK(context.Background()); err != nil {
		return nil, err
	}

	return p, nil
}

// NewLocalKEKProviderWithKey creates a provider with an existing key.
func NewLocalKEKProviderWithKey(keyID string, key []byte) (*LocalKEKProvider, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}

	keyCopy := make([]byte, 32)
	copy(keyCopy, key)

	return &LocalKEKProvider{
		keys:      map[string][]byte{keyID: keyCopy},
		currentID: keyID,
	}, nil
}

// GetKEK retrieves a KEK by ID.
func (p *LocalKEKProvider) GetKEK(ctx context.Context, keyID string) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key, ok := p.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("KEK not found: %s", keyID)
	}

	// Return a copy
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	return keyCopy, nil
}

// CurrentKEKID returns the ID of the current KEK.
func (p *LocalKEKProvider) CurrentKEKID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentID
}

// RotateKEK creates a new KEK and returns its ID.
func (p *LocalKEKProvider) RotateKEK(ctx context.Context) (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}

	keyID := fmt.Sprintf("kek-%d", time.Now().UnixNano())

	p.mu.Lock()
	p.keys[keyID] = key
	p.currentID = keyID
	p.mu.Unlock()

	return keyID, nil
}

// DerivedKEKProvider derives KEKs from a master key.
// Each key ID is used as context for the derivation.
type DerivedKEKProvider struct {
	masterKey []byte
	currentID string
	mu        sync.RWMutex
}

// NewDerivedKEKProvider creates a new derived KEK provider.
func NewDerivedKEKProvider(masterKey []byte) (*DerivedKEKProvider, error) {
	if len(masterKey) < 16 {
		return nil, errors.New("master key must be at least 16 bytes")
	}

	keyCopy := make([]byte, len(masterKey))
	copy(keyCopy, masterKey)

	return &DerivedKEKProvider{
		masterKey: keyCopy,
		currentID: fmt.Sprintf("v1-%d", time.Now().Unix()),
	}, nil
}

// GetKEK derives a KEK for the given ID.
func (p *DerivedKEKProvider) GetKEK(ctx context.Context, keyID string) ([]byte, error) {
	key := make([]byte, 32)
	h := hkdf.New(sha3.New256, p.masterKey, []byte(keyID), []byte("kek-derivation"))
	if _, err := io.ReadFull(h, key); err != nil {
		return nil, err
	}
	return key, nil
}

// CurrentKEKID returns the current key ID.
func (p *DerivedKEKProvider) CurrentKEKID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentID
}

// RotateKEK creates a new key ID.
func (p *DerivedKEKProvider) RotateKEK(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.currentID = fmt.Sprintf("v1-%d", time.Now().Unix())
	return p.currentID, nil
}
