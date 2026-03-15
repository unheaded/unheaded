// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package store provides certificate storage.
package store

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrNotFound indicates the certificate was not found.
	ErrNotFound = errors.New("certificate not found")
	// ErrStoreClosed indicates the store has been closed.
	ErrStoreClosed = errors.New("store closed")
)

// Config configures the certificate store.
type Config struct {
	// Type is the store type (memory, file).
	Type string
	// Path is the storage path for file-based stores.
	Path string
	// CleanupInterval is how often to clean expired certs.
	CleanupInterval time.Duration
	// RetainExpired keeps expired certs for this duration.
	RetainExpired time.Duration
}

// DefaultConfig returns default store configuration.
func DefaultConfig() *Config {
	return &Config{
		Type:            "memory",
		CleanupInterval: 1 * time.Hour,
		RetainExpired:   7 * 24 * time.Hour,
	}
}

// Certificate is an interface for stored certificates.
// This allows the store package to not depend on the parent certs package.
type Certificate interface {
	GetSerial() string
	GetExpiresAt() time.Time
	IsRevoked() bool
}

// CertificateData is a concrete implementation for storage.
type CertificateData struct {
	Serial         string
	CertificatePEM []byte
	PrivateKeyPEM  []byte
	ChainPEM       []byte
	IssuedAt       time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
	SPIFFEID       string
	Type           int
	Metadata       map[string]string
}

// GetSerial returns the serial number.
func (c *CertificateData) GetSerial() string {
	return c.Serial
}

// GetExpiresAt returns the expiration time.
func (c *CertificateData) GetExpiresAt() time.Time {
	return c.ExpiresAt
}

// IsRevoked returns whether the certificate is revoked.
func (c *CertificateData) IsRevoked() bool {
	return c.RevokedAt != nil
}

// Store defines the certificate storage interface.
type Store interface {
	// Put stores a certificate.
	Put(ctx context.Context, serial string, cert interface{}) error
	// Get retrieves a certificate by serial.
	Get(ctx context.Context, serial string) (interface{}, error)
	// Delete removes a certificate.
	Delete(ctx context.Context, serial string) error
	// List returns all certificates.
	List(ctx context.Context) ([]interface{}, error)
	// ListByStatus returns certificates by status.
	ListByStatus(ctx context.Context, revoked bool) ([]interface{}, error)
	// Close closes the store.
	Close() error
}

// New creates a new store based on configuration.
func New(config *Config) (Store, error) {
	if config == nil {
		config = DefaultConfig()
	}

	switch config.Type {
	case "memory":
		return NewMemoryStore(config), nil
	case "file":
		return NewFileStore(config)
	default:
		return NewMemoryStore(config), nil
	}
}

// MemoryStore is an in-memory certificate store.
type MemoryStore struct {
	config  *Config
	certs   map[string]interface{}
	mu      sync.RWMutex
	closed  bool
	closeCh chan struct{}
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore(config *Config) *MemoryStore {
	s := &MemoryStore{
		config:  config,
		certs:   make(map[string]interface{}),
		closeCh: make(chan struct{}),
	}

	// Start cleanup goroutine
	if config.CleanupInterval > 0 {
		go s.cleanupLoop()
	}

	return s
}

// Put stores a certificate.
func (s *MemoryStore) Put(ctx context.Context, serial string, cert interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	s.certs[serial] = cert
	return nil
}

// Get retrieves a certificate by serial.
func (s *MemoryStore) Get(ctx context.Context, serial string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	cert, ok := s.certs[serial]
	if !ok {
		return nil, nil
	}
	return cert, nil
}

// Delete removes a certificate.
func (s *MemoryStore) Delete(ctx context.Context, serial string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	delete(s.certs, serial)
	return nil
}

// List returns all certificates.
func (s *MemoryStore) List(ctx context.Context) ([]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	result := make([]interface{}, 0, len(s.certs))
	for _, cert := range s.certs {
		result = append(result, cert)
	}
	return result, nil
}

// ListByStatus returns certificates by revocation status.
func (s *MemoryStore) ListByStatus(ctx context.Context, revoked bool) ([]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var result []interface{}
	for _, cert := range s.certs {
		if c, ok := cert.(Certificate); ok {
			if c.IsRevoked() == revoked {
				result = append(result, cert)
			}
		}
	}
	return result, nil
}

// Close closes the store.
func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.closeCh)
	return nil
}

// cleanupLoop periodically removes expired certificates.
func (s *MemoryStore) cleanupLoop() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.closeCh:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// cleanup removes expired certificates.
func (s *MemoryStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-s.config.RetainExpired)
	for serial, cert := range s.certs {
		if c, ok := cert.(Certificate); ok {
			if c.GetExpiresAt().Before(cutoff) {
				delete(s.certs, serial)
			}
		}
	}
}
