// Package store provides secret storage backends for the Crystal Grotto.
package store

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/unheaded/unheaded/pkg/logger"
	"github.com/unheaded/unheaded/pkg/secrets"
)

// MemoryStore is an in-memory secret store for testing and development.
// It is not suitable for production use as secrets are lost on restart.
type MemoryStore struct {
	secrets   map[string]*secrets.Secret
	watchers  map[string][]chan *secrets.Secret
	leases    map[string]*secrets.Lease
	mu        sync.RWMutex
	log       *logger.Logger
	closed    bool
	closeCh   chan struct{}
}

// MemoryStoreConfig configures the memory store.
type MemoryStoreConfig struct {
	Logger *logger.Logger
}

// NewMemoryStore creates a new in-memory secret store.
func NewMemoryStore(config MemoryStoreConfig) *MemoryStore {
	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	return &MemoryStore{
		secrets:  make(map[string]*secrets.Secret),
		watchers: make(map[string][]chan *secrets.Secret),
		leases:   make(map[string]*secrets.Lease),
		log:      log.With().Str("store", "memory").Logger(),
		closeCh:  make(chan struct{}),
	}
}

// Get retrieves a secret by path.
func (s *MemoryStore) Get(ctx context.Context, path string) (*secrets.Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, secrets.ErrStoreClosed
	}

	secret, ok := s.secrets[path]
	if !ok {
		return nil, secrets.ErrSecretNotFound
	}

	return secret.Clone(), nil
}

// Set stores a secret at the given path.
func (s *MemoryStore) Set(ctx context.Context, path string, secret *secrets.Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return secrets.ErrStoreClosed
	}

	// Store a clone to prevent external modifications
	s.secrets[path] = secret.Clone()

	// Notify watchers
	if watchers, ok := s.watchers[path]; ok {
		for _, ch := range watchers {
			select {
			case ch <- secret.Clone():
			default:
				// Channel full, skip
			}
		}
	}

	s.log.Debug().Str("path", path).Msg("secret stored")
	return nil
}

// Delete removes a secret by path.
func (s *MemoryStore) Delete(ctx context.Context, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return secrets.ErrStoreClosed
	}

	if _, ok := s.secrets[path]; !ok {
		return secrets.ErrSecretNotFound
	}

	delete(s.secrets, path)

	// Notify watchers with nil (indicates deletion)
	if watchers, ok := s.watchers[path]; ok {
		for _, ch := range watchers {
			select {
			case ch <- nil:
			default:
				// Channel full, skip
			}
		}
	}

	s.log.Debug().Str("path", path).Msg("secret deleted")
	return nil
}

// List returns all secret paths matching the prefix.
func (s *MemoryStore) List(ctx context.Context, prefix string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, secrets.ErrStoreClosed
	}

	var paths []string
	for path := range s.secrets {
		if strings.HasPrefix(path, prefix) {
			paths = append(paths, path)
		}
	}

	return paths, nil
}

// Watch watches for changes to a secret.
func (s *MemoryStore) Watch(ctx context.Context, path string) (<-chan *secrets.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, secrets.ErrStoreClosed
	}

	ch := make(chan *secrets.Secret, 10)
	s.watchers[path] = append(s.watchers[path], ch)

	// Clean up when context is done
	go func() {
		select {
		case <-ctx.Done():
			s.removeWatcher(path, ch)
		case <-s.closeCh:
			// Store closed
		}
	}()

	return ch, nil
}

// removeWatcher removes a watcher channel from the list.
func (s *MemoryStore) removeWatcher(path string, ch chan *secrets.Secret) {
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

// Close closes the store and releases resources.
func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.closeCh)

	// Close all watcher channels
	for _, watchers := range s.watchers {
		for _, ch := range watchers {
			close(ch)
		}
	}

	s.log.Info().Msg("memory store closed")
	return nil
}

// MemoryLeaseManager is an in-memory lease manager.
type MemoryLeaseManager struct {
	leases  map[string]*secrets.Lease
	mu      sync.RWMutex
	log     *logger.Logger
}

// NewMemoryLeaseManager creates a new in-memory lease manager.
func NewMemoryLeaseManager(log *logger.Logger) *MemoryLeaseManager {
	if log == nil {
		log = logger.New(io.Discard)
	}

	m := &MemoryLeaseManager{
		leases: make(map[string]*secrets.Lease),
		log:    log.With().Str("component", "lease-manager").Logger(),
	}

	// Start cleanup goroutine
	go m.cleanup()

	return m
}

// CreateLease creates a new lease for a secret.
func (m *MemoryLeaseManager) CreateLease(ctx context.Context, secretPath string, duration time.Duration, renewable bool, maxRenewals int) (*secrets.Lease, error) {
	leaseID, err := secrets.GenerateLeaseID()
	if err != nil {
		return nil, err
	}

	lease := &secrets.Lease{
		ID:          leaseID,
		SecretPath:  secretPath,
		ExpiresAt:   time.Now().Add(duration),
		Renewable:   renewable,
		MaxRenewals: maxRenewals,
		CreatedAt:   time.Now(),
	}

	m.mu.Lock()
	m.leases[leaseID] = lease
	m.mu.Unlock()

	m.log.Info().
		Str("lease_id", leaseID).
		Str("secret_path", secretPath).
		Time("expires_at", lease.ExpiresAt).
		Msg("lease created")

	return lease, nil
}

// GetLease retrieves a lease by ID.
func (m *MemoryLeaseManager) GetLease(ctx context.Context, leaseID string) (*secrets.Lease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	lease, ok := m.leases[leaseID]
	if !ok {
		return nil, secrets.ErrSecretNotFound
	}

	return lease, nil
}

// RenewLease extends a lease's expiration.
func (m *MemoryLeaseManager) RenewLease(ctx context.Context, leaseID string, duration time.Duration) (*secrets.Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[leaseID]
	if !ok {
		return nil, secrets.ErrSecretNotFound
	}

	if !lease.CanRenew() {
		return nil, secrets.ErrLeaseExpired
	}

	lease.ExpiresAt = time.Now().Add(duration)
	lease.RenewCount++

	m.log.Info().
		Str("lease_id", leaseID).
		Int("renew_count", lease.RenewCount).
		Time("expires_at", lease.ExpiresAt).
		Msg("lease renewed")

	return lease, nil
}

// RevokeLease revokes a lease.
func (m *MemoryLeaseManager) RevokeLease(ctx context.Context, leaseID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[leaseID]
	if !ok {
		return secrets.ErrSecretNotFound
	}

	lease.Revoked = true

	m.log.Info().
		Str("lease_id", leaseID).
		Msg("lease revoked")

	return nil
}

// RevokeLeasesByPath revokes all leases for a secret path.
func (m *MemoryLeaseManager) RevokeLeasesByPath(ctx context.Context, secretPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, lease := range m.leases {
		if lease.SecretPath == secretPath && !lease.Revoked {
			lease.Revoked = true
			count++
		}
	}

	m.log.Info().
		Str("secret_path", secretPath).
		Int("count", count).
		Msg("leases revoked by path")

	return nil
}

// ListLeases returns all active leases.
func (m *MemoryLeaseManager) ListLeases(ctx context.Context) ([]*secrets.Lease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var leases []*secrets.Lease
	for _, lease := range m.leases {
		if !lease.Revoked && !lease.IsExpired() {
			leases = append(leases, lease)
		}
	}

	return leases, nil
}

// cleanup periodically removes expired leases.
func (m *MemoryLeaseManager) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		for id, lease := range m.leases {
			if lease.IsExpired() || lease.Revoked {
				delete(m.leases, id)
			}
		}
		m.mu.Unlock()
	}
}

// MemoryAuditLogger is an in-memory audit logger for testing.
type MemoryAuditLogger struct {
	events []*secrets.AuditEvent
	mu     sync.RWMutex
	maxSize int
}

// NewMemoryAuditLogger creates a new in-memory audit logger.
func NewMemoryAuditLogger(maxSize int) *MemoryAuditLogger {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &MemoryAuditLogger{
		events:  make([]*secrets.AuditEvent, 0, maxSize),
		maxSize: maxSize,
	}
}

// Log records an audit event.
func (l *MemoryAuditLogger) Log(event *secrets.AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Trim if at capacity
	if len(l.events) >= l.maxSize {
		l.events = l.events[1:]
	}

	l.events = append(l.events, event)
	return nil
}

// Query retrieves audit events matching the filter.
func (l *MemoryAuditLogger) Query(ctx context.Context, filter secrets.AuditFilter) ([]*secrets.AuditEvent, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []*secrets.AuditEvent

	for _, event := range l.events {
		// Apply filters
		if filter.Path != "" && !strings.HasPrefix(event.Path, filter.Path) {
			continue
		}
		if filter.Operation != "" && event.Operation != filter.Operation {
			continue
		}
		if filter.Actor != "" && event.Actor != filter.Actor {
			continue
		}
		if !filter.StartTime.IsZero() && event.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && event.Timestamp.After(filter.EndTime) {
			continue
		}

		results = append(results, event)

		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}

	return results, nil
}

// Events returns all recorded events (for testing).
func (l *MemoryAuditLogger) Events() []*secrets.AuditEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	events := make([]*secrets.AuditEvent, len(l.events))
	copy(events, l.events)
	return events
}

// Clear removes all events (for testing).
func (l *MemoryAuditLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = l.events[:0]
}
