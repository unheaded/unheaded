// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStore is a file-based certificate store.
type FileStore struct {
	config  *Config
	path    string
	index   map[string]*FileEntry
	mu      sync.RWMutex
	closed  bool
	closeCh chan struct{}
}

// FileEntry represents a stored certificate entry.
type FileEntry struct {
	Serial    string    `json:"serial"`
	Path      string    `json:"path"`
	ExpiresAt time.Time `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// NewFileStore creates a new file-based store.
func NewFileStore(config *Config) (*FileStore, error) {
	if config.Path == "" {
		return nil, fmt.Errorf("path required for file store")
	}

	// Ensure directory exists
	if err := os.MkdirAll(config.Path, 0700); err != nil {
		return nil, err
	}

	s := &FileStore{
		config:  config,
		path:    config.Path,
		index:   make(map[string]*FileEntry),
		closeCh: make(chan struct{}),
	}

	// Load existing index
	if err := s.loadIndex(); err != nil {
		// Index doesn't exist yet, which is fine
		_ = err
	}

	// Start cleanup goroutine
	if config.CleanupInterval > 0 {
		go s.cleanupLoop()
	}

	return s, nil
}

// Put stores a certificate.
func (s *FileStore) Put(ctx context.Context, serial string, cert interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// Serialize certificate
	data, err := json.MarshalIndent(cert, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	certPath := filepath.Join(s.path, serial+".json")
	if err := os.WriteFile(certPath, data, 0600); err != nil {
		return err
	}

	// Update index
	entry := &FileEntry{
		Serial: serial,
		Path:   certPath,
	}
	if c, ok := cert.(Certificate); ok {
		entry.ExpiresAt = c.GetExpiresAt()
		if c.IsRevoked() {
			now := time.Now()
			entry.RevokedAt = &now
		}
	}
	s.index[serial] = entry

	// Save index
	return s.saveIndex()
}

// Get retrieves a certificate by serial.
func (s *FileStore) Get(ctx context.Context, serial string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	entry, ok := s.index[serial]
	if !ok {
		return nil, nil
	}

	// Read file
	data, err := os.ReadFile(entry.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Deserialize into CertificateData
	var cert CertificateData
	if err := json.Unmarshal(data, &cert); err != nil {
		return nil, err
	}

	return &cert, nil
}

// Delete removes a certificate.
func (s *FileStore) Delete(ctx context.Context, serial string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	entry, ok := s.index[serial]
	if !ok {
		return nil
	}

	// Remove file
	if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Update index
	delete(s.index, serial)
	return s.saveIndex()
}

// List returns all certificates.
func (s *FileStore) List(ctx context.Context) ([]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var result []interface{}
	for serial := range s.index {
		s.mu.RUnlock()
		cert, err := s.Get(ctx, serial)
		s.mu.RLock()
		if err != nil {
			continue
		}
		if cert != nil {
			result = append(result, cert)
		}
	}
	return result, nil
}

// ListByStatus returns certificates by revocation status.
func (s *FileStore) ListByStatus(ctx context.Context, revoked bool) ([]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var result []interface{}
	for serial, entry := range s.index {
		isRevoked := entry.RevokedAt != nil
		if isRevoked == revoked {
			s.mu.RUnlock()
			cert, err := s.Get(ctx, serial)
			s.mu.RLock()
			if err != nil {
				continue
			}
			if cert != nil {
				result = append(result, cert)
			}
		}
	}
	return result, nil
}

// Close closes the store.
func (s *FileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.closeCh)
	return s.saveIndex()
}

// loadIndex loads the certificate index from disk.
func (s *FileStore) loadIndex() error {
	indexPath := filepath.Join(s.path, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &s.index)
}

// saveIndex saves the certificate index to disk.
func (s *FileStore) saveIndex() error {
	indexPath := filepath.Join(s.path, "index.json")
	data, err := json.MarshalIndent(s.index, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexPath, data, 0600)
}

// cleanupLoop periodically removes expired certificates.
func (s *FileStore) cleanupLoop() {
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
func (s *FileStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-s.config.RetainExpired)
	for serial, entry := range s.index {
		if entry.ExpiresAt.Before(cutoff) {
			// Remove file
			if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
				continue
			}
			delete(s.index, serial)
		}
	}

	// Save updated index
	_ = s.saveIndex()
}
