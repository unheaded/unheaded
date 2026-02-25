// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package secrets provides THE CRYSTAL GROTTO - the Unheaded Kingdom's secrets management.
// This is where precious infrastructure secrets are protected, encrypted, and rotated.
//
// The Sacred Law: ZERO customer data in secrets - only infrastructure credentials.
//
// Features:
//   - Pluggable secret storage backends (memory, file, Vault)
//   - Age encryption for file-based secrets
//   - Envelope encryption (secrets encrypted with KEK)
//   - Automatic secret rotation
//   - Audit logging of all access
//   - Lease-based access (secrets expire)
//   - Dynamic secret generation
package secrets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
)

// Common errors for secret operations.
var (
	ErrSecretNotFound     = errors.New("secret not found")
	ErrSecretExpired      = errors.New("secret has expired")
	ErrSecretRevoked      = errors.New("secret has been revoked")
	ErrInvalidSecretType  = errors.New("invalid secret type")
	ErrInvalidPath        = errors.New("invalid secret path")
	ErrAccessDenied       = errors.New("access denied")
	ErrRotationInProgress = errors.New("secret rotation already in progress")
	ErrLeaseExpired       = errors.New("lease has expired")
	ErrStoreClosed        = errors.New("secret store is closed")
)

// SecretType represents the type of secret.
type SecretType string

const (
	// SecretTypeStatic is a static secret (passwords, API keys).
	SecretTypeStatic SecretType = "static"
	// SecretTypeDynamic is a dynamically generated secret.
	SecretTypeDynamic SecretType = "dynamic"
	// SecretTypeCertificate is a TLS certificate.
	SecretTypeCertificate SecretType = "certificate"
	// SecretTypeSSHKey is an SSH key pair.
	SecretTypeSSHKey SecretType = "ssh_key"
	// SecretTypeKEK is a Key Encryption Key.
	SecretTypeKEK SecretType = "kek"
)

// Secret represents a secret with metadata.
type Secret struct {
	// Path is the unique path/identifier for the secret.
	Path string `json:"path"`

	// Type is the secret type.
	Type SecretType `json:"type"`

	// Data contains the secret data (key-value pairs).
	// For static secrets: {"value": "secret-value"}
	// For certificates: {"cert": "...", "key": "...", "ca": "..."}
	// For SSH keys: {"public": "...", "private": "..."}
	Data map[string][]byte `json:"data"`

	// Metadata contains additional information about the secret.
	Metadata map[string]string `json:"metadata"`

	// Version is the secret version (incremented on update).
	Version int64 `json:"version"`

	// CreatedAt is when the secret was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the secret was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// ExpiresAt is when the secret expires (zero means never).
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// RotatedAt is when the secret was last rotated.
	RotatedAt time.Time `json:"rotated_at,omitempty"`

	// LeaseID is the lease identifier if this secret has a lease.
	LeaseID string `json:"lease_id,omitempty"`

	// LeaseDuration is how long the lease is valid.
	LeaseDuration time.Duration `json:"lease_duration,omitempty"`

	// Revoked indicates if the secret has been revoked.
	Revoked bool `json:"revoked"`
}

// IsExpired returns true if the secret has expired.
func (s *Secret) IsExpired() bool {
	if s.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(s.ExpiresAt)
}

// Clone creates a deep copy of the secret.
func (s *Secret) Clone() *Secret {
	if s == nil {
		return nil
	}

	clone := &Secret{
		Path:          s.Path,
		Type:          s.Type,
		Version:       s.Version,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
		ExpiresAt:     s.ExpiresAt,
		RotatedAt:     s.RotatedAt,
		LeaseID:       s.LeaseID,
		LeaseDuration: s.LeaseDuration,
		Revoked:       s.Revoked,
	}

	if s.Data != nil {
		clone.Data = make(map[string][]byte, len(s.Data))
		for k, v := range s.Data {
			data := make([]byte, len(v))
			copy(data, v)
			clone.Data[k] = data
		}
	}

	if s.Metadata != nil {
		clone.Metadata = make(map[string]string, len(s.Metadata))
		for k, v := range s.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// Lease represents a lease for secret access.
type Lease struct {
	// ID is the unique lease identifier.
	ID string `json:"id"`

	// SecretPath is the path to the leased secret.
	SecretPath string `json:"secret_path"`

	// ExpiresAt is when the lease expires.
	ExpiresAt time.Time `json:"expires_at"`

	// Renewable indicates if the lease can be renewed.
	Renewable bool `json:"renewable"`

	// MaxRenewals is the maximum number of times the lease can be renewed.
	MaxRenewals int `json:"max_renewals"`

	// RenewCount is the current renewal count.
	RenewCount int `json:"renew_count"`

	// Revoked indicates if the lease has been revoked.
	Revoked bool `json:"revoked"`

	// CreatedAt is when the lease was created.
	CreatedAt time.Time `json:"created_at"`
}

// IsExpired returns true if the lease has expired.
func (l *Lease) IsExpired() bool {
	return time.Now().After(l.ExpiresAt)
}

// CanRenew returns true if the lease can be renewed.
func (l *Lease) CanRenew() bool {
	return l.Renewable && !l.Revoked && l.RenewCount < l.MaxRenewals
}

// SecretStore is the interface for secret storage backends.
type SecretStore interface {
	// Get retrieves a secret by path.
	Get(ctx context.Context, path string) (*Secret, error)

	// Set stores a secret at the given path.
	Set(ctx context.Context, path string, secret *Secret) error

	// Delete removes a secret by path.
	Delete(ctx context.Context, path string) error

	// List returns all secret paths matching the prefix.
	List(ctx context.Context, prefix string) ([]string, error)

	// Watch watches for changes to a secret.
	Watch(ctx context.Context, path string) (<-chan *Secret, error)

	// Close closes the store and releases resources.
	Close() error
}

// LeaseManager manages secret leases.
type LeaseManager interface {
	// CreateLease creates a new lease for a secret.
	CreateLease(ctx context.Context, secretPath string, duration time.Duration, renewable bool, maxRenewals int) (*Lease, error)

	// GetLease retrieves a lease by ID.
	GetLease(ctx context.Context, leaseID string) (*Lease, error)

	// RenewLease extends a lease's expiration.
	RenewLease(ctx context.Context, leaseID string, duration time.Duration) (*Lease, error)

	// RevokeLease revokes a lease.
	RevokeLease(ctx context.Context, leaseID string) error

	// RevokeLeasesByPath revokes all leases for a secret path.
	RevokeLeasesByPath(ctx context.Context, secretPath string) error

	// ListLeases returns all active leases.
	ListLeases(ctx context.Context) ([]*Lease, error)
}

// AuditEvent represents a secret access event.
type AuditEvent struct {
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Operation is the type of operation (get, set, delete, etc.).
	Operation string `json:"operation"`

	// Path is the secret path.
	Path string `json:"path"`

	// Success indicates if the operation succeeded.
	Success bool `json:"success"`

	// Error is the error message if the operation failed.
	Error string `json:"error,omitempty"`

	// Actor is who performed the operation.
	Actor string `json:"actor,omitempty"`

	// Metadata contains additional event data.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AuditLogger logs secret access events.
type AuditLogger interface {
	// Log records an audit event.
	Log(event *AuditEvent) error

	// Query retrieves audit events matching the filter.
	Query(ctx context.Context, filter AuditFilter) ([]*AuditEvent, error)
}

// AuditFilter defines criteria for querying audit events.
type AuditFilter struct {
	// Path filters by secret path (supports prefix matching).
	Path string

	// Operation filters by operation type.
	Operation string

	// Actor filters by actor.
	Actor string

	// StartTime filters events after this time.
	StartTime time.Time

	// EndTime filters events before this time.
	EndTime time.Time

	// Limit limits the number of results.
	Limit int
}

// DynamicSecretGenerator generates dynamic secrets on demand.
type DynamicSecretGenerator interface {
	// Generate creates a new dynamic secret.
	Generate(ctx context.Context, path string, config map[string]interface{}) (*Secret, error)

	// Revoke revokes a dynamic secret.
	Revoke(ctx context.Context, path string, leaseID string) error

	// Type returns the type of secrets this generator creates.
	Type() SecretType
}

// Manager is the main secrets manager that orchestrates all operations.
type Manager struct {
	store       SecretStore
	leases      LeaseManager
	audit       AuditLogger
	generators  map[SecretType]DynamicSecretGenerator
	log         *logger.Logger
	mu          sync.RWMutex
	closed      bool
	stopCh      chan struct{}
}

// Package-level metrics (registered once)
var (
	managerMetricsOnce   sync.Once
	secretsTotal         *metrics.GaugeVec
	operationsTotal      *metrics.CounterVec
	operationErrors      *metrics.CounterVec
	operationLatency     *metrics.HistogramVec
)

// ManagerConfig configures the secrets manager.
type ManagerConfig struct {
	// Store is the secret storage backend.
	Store SecretStore

	// Leases is the lease manager (optional).
	Leases LeaseManager

	// Audit is the audit logger (optional).
	Audit AuditLogger

	// Logger is the logger to use.
	Logger *logger.Logger
}

// NewManager creates a new secrets manager.
func NewManager(config ManagerConfig) (*Manager, error) {
	if config.Store == nil {
		return nil, errors.New("store is required")
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	m := &Manager{
		store:      config.Store,
		leases:     config.Leases,
		audit:      config.Audit,
		generators: make(map[SecretType]DynamicSecretGenerator),
		log:        log.With().Str("component", "secrets-manager").Logger(),
		stopCh:     make(chan struct{}),
	}

	// Initialize metrics (only once per process)
	managerMetricsOnce.Do(func() {
		secretsTotal = metrics.NewGaugeVec(
			"unheaded_secrets_total",
			"Total number of secrets by type",
			nil,
			[]string{"type"},
		)
		_ = metrics.Register(secretsTotal)

		operationsTotal = metrics.NewCounterVec(
			"unheaded_secrets_operations_total",
			"Total secret operations",
			nil,
			[]string{"operation", "path_prefix"},
		)
		_ = metrics.Register(operationsTotal)

		operationErrors = metrics.NewCounterVec(
			"unheaded_secrets_operation_errors_total",
			"Total secret operation errors",
			nil,
			[]string{"operation", "error_type"},
		)
		_ = metrics.Register(operationErrors)

		operationLatency = metrics.NewHistogramVec(
			metrics.HistogramOpts{
				Name:    "unheaded_secrets_operation_duration_seconds",
				Help:    "Secret operation latency",
				Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
			},
			[]string{"operation"},
		)
		_ = metrics.Register(operationLatency)
	})

	return m, nil
}

// RegisterGenerator registers a dynamic secret generator.
func (m *Manager) RegisterGenerator(g DynamicSecretGenerator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generators[g.Type()] = g
}

// Get retrieves a secret by path.
func (m *Manager) Get(ctx context.Context, path string) (*Secret, error) {
	start := time.Now()
	defer func() {
		operationLatency.WithLabels(metrics.Labels{"operation": "get"}).Observe(time.Since(start).Seconds())
	}()

	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	operationsTotal.WithLabels(metrics.Labels{"operation": "get", "path_prefix": pathPrefix(path)}).Inc()

	secret, err := m.store.Get(ctx, path)
	if err != nil {
		m.logAudit("get", path, false, err.Error(), nil)
		operationErrors.WithLabels(metrics.Labels{"operation": "get", "error_type": errorType(err)}).Inc()
		return nil, err
	}

	if secret.Revoked {
		m.logAudit("get", path, false, ErrSecretRevoked.Error(), nil)
		operationErrors.WithLabels(metrics.Labels{"operation": "get", "error_type": "revoked"}).Inc()
		return nil, ErrSecretRevoked
	}

	if secret.IsExpired() {
		m.logAudit("get", path, false, ErrSecretExpired.Error(), nil)
		operationErrors.WithLabels(metrics.Labels{"operation": "get", "error_type": "expired"}).Inc()
		return nil, ErrSecretExpired
	}

	m.logAudit("get", path, true, "", nil)
	return secret.Clone(), nil
}

// Set stores a secret at the given path.
func (m *Manager) Set(ctx context.Context, path string, secret *Secret) error {
	start := time.Now()
	defer func() {
		operationLatency.WithLabels(metrics.Labels{"operation": "set"}).Observe(time.Since(start).Seconds())
	}()

	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	operationsTotal.WithLabels(metrics.Labels{"operation": "set", "path_prefix": pathPrefix(path)}).Inc()

	// Ensure path is set
	secret.Path = path

	// Set timestamps
	now := time.Now()
	if secret.CreatedAt.IsZero() {
		secret.CreatedAt = now
	}
	secret.UpdatedAt = now

	// Get existing secret to increment version
	existing, err := m.store.Get(ctx, path)
	if err == nil {
		secret.Version = existing.Version + 1
	} else if errors.Is(err, ErrSecretNotFound) {
		secret.Version = 1
	} else {
		m.logAudit("set", path, false, err.Error(), nil)
		operationErrors.WithLabels(metrics.Labels{"operation": "set", "error_type": errorType(err)}).Inc()
		return err
	}

	if err := m.store.Set(ctx, path, secret); err != nil {
		m.logAudit("set", path, false, err.Error(), nil)
		operationErrors.WithLabels(metrics.Labels{"operation": "set", "error_type": errorType(err)}).Inc()
		return err
	}

	secretsTotal.WithLabels(metrics.Labels{"type": string(secret.Type)}).Inc()
	m.logAudit("set", path, true, "", map[string]string{"version": fmt.Sprintf("%d", secret.Version)})

	m.log.Info().
		Str("path", path).
		Str("type", string(secret.Type)).
		Int64("version", secret.Version).
		Msg("secret stored")

	return nil
}

// Delete removes a secret by path.
func (m *Manager) Delete(ctx context.Context, path string) error {
	start := time.Now()
	defer func() {
		operationLatency.WithLabels(metrics.Labels{"operation": "delete"}).Observe(time.Since(start).Seconds())
	}()

	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	operationsTotal.WithLabels(metrics.Labels{"operation": "delete", "path_prefix": pathPrefix(path)}).Inc()

	// Revoke all leases for this secret
	if m.leases != nil {
		if err := m.leases.RevokeLeasesByPath(ctx, path); err != nil {
			m.log.Warn().Err(err).Str("path", path).Msg("failed to revoke leases for deleted secret")
		}
	}

	if err := m.store.Delete(ctx, path); err != nil {
		m.logAudit("delete", path, false, err.Error(), nil)
		operationErrors.WithLabels(metrics.Labels{"operation": "delete", "error_type": errorType(err)}).Inc()
		return err
	}

	m.logAudit("delete", path, true, "", nil)

	m.log.Info().
		Str("path", path).
		Msg("secret deleted")

	return nil
}

// List returns all secret paths matching the prefix.
func (m *Manager) List(ctx context.Context, prefix string) ([]string, error) {
	start := time.Now()
	defer func() {
		operationLatency.WithLabels(metrics.Labels{"operation": "list"}).Observe(time.Since(start).Seconds())
	}()

	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	operationsTotal.WithLabels(metrics.Labels{"operation": "list", "path_prefix": pathPrefix(prefix)}).Inc()

	paths, err := m.store.List(ctx, prefix)
	if err != nil {
		m.logAudit("list", prefix, false, err.Error(), nil)
		operationErrors.WithLabels(metrics.Labels{"operation": "list", "error_type": errorType(err)}).Inc()
		return nil, err
	}

	m.logAudit("list", prefix, true, "", map[string]string{"count": fmt.Sprintf("%d", len(paths))})
	return paths, nil
}

// Watch watches for changes to a secret.
func (m *Manager) Watch(ctx context.Context, path string) (<-chan *Secret, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	operationsTotal.WithLabels(metrics.Labels{"operation": "watch", "path_prefix": pathPrefix(path)}).Inc()

	ch, err := m.store.Watch(ctx, path)
	if err != nil {
		m.logAudit("watch", path, false, err.Error(), nil)
		operationErrors.WithLabels(metrics.Labels{"operation": "watch", "error_type": errorType(err)}).Inc()
		return nil, err
	}

	m.logAudit("watch", path, true, "", nil)
	return ch, nil
}

// GenerateDynamic generates a dynamic secret.
func (m *Manager) GenerateDynamic(ctx context.Context, path string, secretType SecretType, config map[string]interface{}) (*Secret, error) {
	start := time.Now()
	defer func() {
		operationLatency.WithLabels(metrics.Labels{"operation": "generate"}).Observe(time.Since(start).Seconds())
	}()

	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	generator, ok := m.generators[secretType]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no generator registered for secret type %s", secretType)
	}

	operationsTotal.WithLabels(metrics.Labels{"operation": "generate", "path_prefix": pathPrefix(path)}).Inc()

	secret, err := generator.Generate(ctx, path, config)
	if err != nil {
		m.logAudit("generate", path, false, err.Error(), nil)
		operationErrors.WithLabels(metrics.Labels{"operation": "generate", "error_type": errorType(err)}).Inc()
		return nil, err
	}

	// Store the generated secret
	if err := m.Set(ctx, path, secret); err != nil {
		// Revoke the generated secret if we can't store it
		_ = generator.Revoke(ctx, path, secret.LeaseID)
		return nil, err
	}

	m.logAudit("generate", path, true, "", map[string]string{"type": string(secretType)})
	return secret, nil
}

// CreateLease creates a lease for accessing a secret.
func (m *Manager) CreateLease(ctx context.Context, path string, duration time.Duration, renewable bool, maxRenewals int) (*Lease, error) {
	if m.leases == nil {
		return nil, errors.New("lease manager not configured")
	}

	// Verify secret exists
	if _, err := m.Get(ctx, path); err != nil {
		return nil, err
	}

	return m.leases.CreateLease(ctx, path, duration, renewable, maxRenewals)
}

// GetWithLease retrieves a secret with a lease.
func (m *Manager) GetWithLease(ctx context.Context, leaseID string) (*Secret, error) {
	if m.leases == nil {
		return nil, errors.New("lease manager not configured")
	}

	lease, err := m.leases.GetLease(ctx, leaseID)
	if err != nil {
		return nil, err
	}

	if lease.Revoked {
		return nil, ErrLeaseExpired
	}

	if lease.IsExpired() {
		return nil, ErrLeaseExpired
	}

	return m.Get(ctx, lease.SecretPath)
}

// Close closes the manager and releases resources.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	close(m.stopCh)
	m.mu.Unlock()

	return m.store.Close()
}

// logAudit logs an audit event.
func (m *Manager) logAudit(operation, path string, success bool, errMsg string, metadata map[string]string) {
	if m.audit == nil {
		return
	}

	event := &AuditEvent{
		Timestamp: time.Now(),
		Operation: operation,
		Path:      path,
		Success:   success,
		Error:     errMsg,
		Metadata:  metadata,
	}

	if err := m.audit.Log(event); err != nil {
		m.log.Error().Err(err).Msg("failed to log audit event")
	}
}

// GenerateRandomBytes generates cryptographically secure random bytes.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// GenerateRandomString generates a cryptographically secure random string.
func GenerateRandomString(n int) (string, error) {
	b, err := GenerateRandomBytes(n)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:n], nil
}

// GenerateLeaseID generates a unique lease ID.
func GenerateLeaseID() (string, error) {
	return GenerateRandomString(32)
}

// pathPrefix extracts the first component of a path for metrics.
func pathPrefix(path string) string {
	if len(path) == 0 {
		return "root"
	}
	for i, c := range path {
		if c == '/' && i > 0 {
			return path[:i]
		}
	}
	return path
}

// errorType categorizes an error for metrics.
func errorType(err error) string {
	switch {
	case errors.Is(err, ErrSecretNotFound):
		return "not_found"
	case errors.Is(err, ErrSecretExpired):
		return "expired"
	case errors.Is(err, ErrSecretRevoked):
		return "revoked"
	case errors.Is(err, ErrAccessDenied):
		return "access_denied"
	case errors.Is(err, ErrStoreClosed):
		return "store_closed"
	default:
		return "internal"
	}
}
