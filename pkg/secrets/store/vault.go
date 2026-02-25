// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/secrets"
)

// VaultStore integrates with HashiCorp Vault for secret storage.
type VaultStore struct {
	client    *VaultClient
	mountPath string
	watchers  map[string][]chan *secrets.Secret
	mu        sync.RWMutex
	log       *logger.Logger
	closed    bool
	closeCh   chan struct{}
	pollInterval time.Duration
}

// VaultStoreConfig configures the Vault store.
type VaultStoreConfig struct {
	// Address is the Vault server address.
	Address string

	// Token is the Vault authentication token.
	Token string

	// MountPath is the KV secrets engine mount path (e.g., "secret").
	MountPath string

	// TLSConfig configures TLS for the connection.
	TLSConfig *VaultTLSConfig

	// Logger is the logger to use.
	Logger *logger.Logger

	// PollInterval is how often to poll for changes.
	PollInterval time.Duration

	// Timeout is the HTTP client timeout.
	Timeout time.Duration
}

// VaultTLSConfig configures TLS for Vault connections.
type VaultTLSConfig struct {
	// CACert is the path to the CA certificate.
	CACert string

	// ClientCert is the path to the client certificate.
	ClientCert string

	// ClientKey is the path to the client key.
	ClientKey string

	// Insecure skips TLS verification (not recommended for production).
	Insecure bool
}

// VaultClient is a simple HTTP client for Vault.
type VaultClient struct {
	address   string
	token     string
	client    *http.Client
	log       *logger.Logger
}

// NewVaultStore creates a new Vault-backed secret store.
func NewVaultStore(config VaultStoreConfig) (*VaultStore, error) {
	if config.Address == "" {
		return nil, errors.New("vault address is required")
	}
	if config.Token == "" {
		return nil, errors.New("vault token is required")
	}

	mountPath := config.MountPath
	if mountPath == "" {
		mountPath = "secret"
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	pollInterval := config.PollInterval
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}

	client := &VaultClient{
		address: strings.TrimSuffix(config.Address, "/"),
		token:   config.Token,
		client:  &http.Client{Timeout: timeout},
		log:     log,
	}

	vs := &VaultStore{
		client:       client,
		mountPath:    mountPath,
		watchers:     make(map[string][]chan *secrets.Secret),
		log:          log.With().Str("store", "vault").Logger(),
		closeCh:      make(chan struct{}),
		pollInterval: pollInterval,
	}

	// Verify connection
	if err := vs.verifyConnection(context.Background()); err != nil {
		return nil, fmt.Errorf("verify vault connection: %w", err)
	}

	// Start watcher
	go vs.watchSecrets()

	return vs, nil
}

// verifyConnection checks if we can connect to Vault.
func (s *VaultStore) verifyConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", s.client.address+"/v1/sys/health", nil)
	if err != nil {
		return err
	}

	resp, err := s.client.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault health check failed: %s", string(body))
	}

	s.log.Info().Str("address", s.client.address).Msg("connected to vault")
	return nil
}

// kvPath returns the KV v2 data path for a secret.
func (s *VaultStore) kvPath(path string) string {
	return fmt.Sprintf("/v1/%s/data/%s", s.mountPath, path)
}

// kvMetadataPath returns the KV v2 metadata path for a secret.
func (s *VaultStore) kvMetadataPath(path string) string {
	return fmt.Sprintf("/v1/%s/metadata/%s", s.mountPath, path)
}

// Get retrieves a secret by path from Vault.
func (s *VaultStore) Get(ctx context.Context, path string) (*secrets.Secret, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, secrets.ErrStoreClosed
	}
	s.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", s.client.address+s.kvPath(path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", s.client.token)

	resp, err := s.client.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, secrets.ErrSecretNotFound
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault error: %s", string(body))
	}

	var vaultResp vaultKVResponse
	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		return nil, fmt.Errorf("decode vault response: %w", err)
	}

	return s.vaultToSecret(path, &vaultResp)
}

// Set stores a secret at the given path in Vault.
func (s *VaultStore) Set(ctx context.Context, path string, secret *secrets.Secret) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return secrets.ErrStoreClosed
	}
	s.mu.RUnlock()

	// Convert secret to Vault format
	data := s.secretToVault(secret)

	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal secret: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.client.address+s.kvPath(path), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", s.client.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault error: %s", string(respBody))
	}

	// Notify watchers
	s.notifyWatchers(path, secret)

	s.log.Debug().Str("path", path).Msg("secret stored in vault")
	return nil
}

// Delete removes a secret by path from Vault.
func (s *VaultStore) Delete(ctx context.Context, path string) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return secrets.ErrStoreClosed
	}
	s.mu.RUnlock()

	// Delete metadata (which also deletes all versions)
	req, err := http.NewRequestWithContext(ctx, "DELETE", s.client.address+s.kvMetadataPath(path), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", s.client.token)

	resp, err := s.client.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return secrets.ErrSecretNotFound
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault error: %s", string(body))
	}

	// Notify watchers with nil
	s.notifyWatchers(path, nil)

	s.log.Debug().Str("path", path).Msg("secret deleted from vault")
	return nil
}

// List returns all secret paths matching the prefix from Vault.
func (s *VaultStore) List(ctx context.Context, prefix string) ([]string, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, secrets.ErrStoreClosed
	}
	s.mu.RUnlock()

	listPath := fmt.Sprintf("/v1/%s/metadata/%s", s.mountPath, prefix)
	req, err := http.NewRequestWithContext(ctx, "LIST", s.client.address+listPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", s.client.token)

	resp, err := s.client.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault error: %s", string(body))
	}

	var listResp vaultListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decode vault response: %w", err)
	}

	// Prepend prefix to keys
	paths := make([]string, len(listResp.Data.Keys))
	for i, key := range listResp.Data.Keys {
		paths[i] = prefix + key
	}

	return paths, nil
}

// Watch watches for changes to a secret.
func (s *VaultStore) Watch(ctx context.Context, path string) (<-chan *secrets.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, secrets.ErrStoreClosed
	}

	ch := make(chan *secrets.Secret, 10)
	s.watchers[path] = append(s.watchers[path], ch)

	go func() {
		select {
		case <-ctx.Done():
			s.removeWatcher(path, ch)
		case <-s.closeCh:
		}
	}()

	return ch, nil
}

// notifyWatchers sends a secret update to all watchers.
func (s *VaultStore) notifyWatchers(path string, secret *secrets.Secret) {
	s.mu.RLock()
	watchers := s.watchers[path]
	s.mu.RUnlock()

	for _, ch := range watchers {
		select {
		case ch <- secret:
		default:
		}
	}
}

// removeWatcher removes a watcher channel.
func (s *VaultStore) removeWatcher(path string, ch chan *secrets.Secret) {
	s.mu.Lock()
	defer s.mu.Unlock()

	watchers := s.watchers[path]
	for i, w := range watchers {
		if w == ch {
			s.watchers[path] = append(watchers[:i], watchers[i+1:]...)
			close(ch)
			break
		}
	}
}

// watchSecrets polls Vault for changes.
func (s *VaultStore) watchSecrets() {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	versions := make(map[string]int64)

	for {
		select {
		case <-ticker.C:
			s.checkVersions(versions)
		case <-s.closeCh:
			return
		}
	}
}

// checkVersions checks for version changes on watched secrets.
func (s *VaultStore) checkVersions(versions map[string]int64) {
	s.mu.RLock()
	watchedPaths := make([]string, 0, len(s.watchers))
	for path := range s.watchers {
		watchedPaths = append(watchedPaths, path)
	}
	s.mu.RUnlock()

	ctx := context.Background()
	for _, path := range watchedPaths {
		secret, err := s.Get(ctx, path)
		if err != nil {
			if errors.Is(err, secrets.ErrSecretNotFound) {
				if _, ok := versions[path]; ok {
					delete(versions, path)
					s.notifyWatchers(path, nil)
				}
			}
			continue
		}

		if prev, ok := versions[path]; !ok || secret.Version > prev {
			versions[path] = secret.Version
			if ok { // Only notify if we've seen this before
				s.notifyWatchers(path, secret)
			}
		}
	}
}

// Close closes the store.
func (s *VaultStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.closeCh)

	for _, watchers := range s.watchers {
		for _, ch := range watchers {
			close(ch)
		}
	}

	s.log.Info().Msg("vault store closed")
	return nil
}

// secretToVault converts a Secret to Vault KV format.
func (s *VaultStore) secretToVault(secret *secrets.Secret) map[string]interface{} {
	// Convert byte values to base64 strings
	data := make(map[string]interface{})
	for k, v := range secret.Data {
		data[k] = string(v)
	}

	// Add metadata as custom_metadata
	customMeta := map[string]interface{}{
		"type":       string(secret.Type),
		"created_at": secret.CreatedAt.Format(time.RFC3339),
		"updated_at": secret.UpdatedAt.Format(time.RFC3339),
	}
	if !secret.ExpiresAt.IsZero() {
		customMeta["expires_at"] = secret.ExpiresAt.Format(time.RFC3339)
	}
	for k, v := range secret.Metadata {
		customMeta["meta_"+k] = v
	}

	return map[string]interface{}{
		"data":    data,
		"options": map[string]interface{}{"cas": secret.Version - 1},
	}
}

// vaultToSecret converts a Vault KV response to a Secret.
func (s *VaultStore) vaultToSecret(path string, resp *vaultKVResponse) (*secrets.Secret, error) {
	secret := &secrets.Secret{
		Path:     path,
		Type:     secrets.SecretTypeStatic,
		Data:     make(map[string][]byte),
		Metadata: make(map[string]string),
		Version:  int64(resp.Data.Metadata.Version),
	}

	// Parse timestamps
	if resp.Data.Metadata.CreatedTime != "" {
		if t, err := time.Parse(time.RFC3339Nano, resp.Data.Metadata.CreatedTime); err == nil {
			secret.CreatedAt = t
		}
	}

	// Convert data
	for k, v := range resp.Data.Data {
		if str, ok := v.(string); ok {
			secret.Data[k] = []byte(str)
		}
	}

	// Check custom metadata for type
	if customMeta := resp.Data.Metadata.CustomMetadata; customMeta != nil {
		if typeStr, ok := customMeta["type"].(string); ok {
			secret.Type = secrets.SecretType(typeStr)
		}
		if expiresAt, ok := customMeta["expires_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, expiresAt); err == nil {
				secret.ExpiresAt = t
			}
		}
	}

	return secret, nil
}

// Vault API response types

type vaultKVResponse struct {
	Data struct {
		Data     map[string]interface{} `json:"data"`
		Metadata struct {
			CreatedTime    string                 `json:"created_time"`
			CustomMetadata map[string]interface{} `json:"custom_metadata"`
			DeletionTime   string                 `json:"deletion_time"`
			Destroyed      bool                   `json:"destroyed"`
			Version        int                    `json:"version"`
		} `json:"metadata"`
	} `json:"data"`
}

type vaultListResponse struct {
	Data struct {
		Keys []string `json:"keys"`
	} `json:"data"`
}
