// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package mtls provides workload identity management for zero-trust service mesh.
package mtls

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

var (
	// ErrIdentityNotFound indicates the identity was not found.
	ErrIdentityNotFound = errors.New("workload identity not found")
	// ErrIdentityDenied indicates the identity is not authorized.
	ErrIdentityDenied = errors.New("workload identity denied")
	// ErrIdentityExpired indicates the identity certificate has expired.
	ErrIdentityExpired = errors.New("workload identity certificate expired")
)

// WorkloadIdentity represents a service's cryptographic identity.
type WorkloadIdentity struct {
	// SPIFFE ID
	ID *SPIFFEID

	// Certificate and key
	Certificate *x509.Certificate
	PrivateKey  crypto.Signer

	// Trust bundle
	TrustBundle *x509.CertPool

	// Metadata
	ServiceName string
	Namespace   string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// IsValid returns true if the identity is valid (not expired).
func (w *WorkloadIdentity) IsValid() bool {
	now := time.Now()
	return now.After(w.Certificate.NotBefore) && now.Before(w.Certificate.NotAfter)
}

// TimeToExpiry returns the duration until expiry.
func (w *WorkloadIdentity) TimeToExpiry() time.Duration {
	return time.Until(w.ExpiresAt)
}

// TLSCertificate returns the identity as a tls.Certificate.
func (w *WorkloadIdentity) TLSCertificate() tls.Certificate {
	return tls.Certificate{
		Certificate: [][]byte{w.Certificate.Raw},
		PrivateKey:  w.PrivateKey,
		Leaf:        w.Certificate,
	}
}

// IdentitySource provides workload identities.
type IdentitySource interface {
	// GetIdentity returns the current workload identity.
	GetIdentity(ctx context.Context) (*WorkloadIdentity, error)
	// WatchIdentity watches for identity changes.
	WatchIdentity(ctx context.Context, ch chan<- *WorkloadIdentity) error
}

// IdentityManager manages workload identities for zero-trust networking.
type IdentityManager struct {
	config *IdentityConfig

	// Current identity
	identity *WorkloadIdentity

	// Identity source
	source IdentitySource

	// Authorization policies
	policies *PolicyStore

	// Watchers
	watchers   []chan<- *WorkloadIdentity
	watchersMu sync.RWMutex

	// State
	mu     sync.RWMutex
	closed bool
}

// IdentityConfig configures the identity manager.
type IdentityConfig struct {
	// Identity parameters
	TrustDomain string
	ServiceName string
	Namespace   string

	// Rotation settings
	AutoRotate     bool
	RotationBuffer time.Duration

	// Validation settings
	ValidatePeers      bool
	RequireSPIFFEID    bool
	AllowedTrustDomains []string
}

// DefaultIdentityConfig returns a default identity configuration.
func DefaultIdentityConfig(serviceName, namespace string) *IdentityConfig {
	return &IdentityConfig{
		TrustDomain:        DefaultTrustDomain,
		ServiceName:        serviceName,
		Namespace:          namespace,
		AutoRotate:         true,
		RotationBuffer:     10 * time.Minute,
		ValidatePeers:      true,
		RequireSPIFFEID:    true,
		AllowedTrustDomains: []string{DefaultTrustDomain},
	}
}

// NewIdentityManager creates a new identity manager.
func NewIdentityManager(config *IdentityConfig) *IdentityManager {
	if config == nil {
		config = DefaultIdentityConfig("unknown", "default")
	}

	return &IdentityManager{
		config:   config,
		policies: NewPolicyStore(),
		watchers: make([]chan<- *WorkloadIdentity, 0),
	}
}

// SetIdentity sets the current workload identity.
func (m *IdentityManager) SetIdentity(identity *WorkloadIdentity) {
	m.mu.Lock()
	m.identity = identity
	m.mu.Unlock()

	// Notify watchers
	m.notifyWatchers(identity)
}

// GetIdentity returns the current workload identity.
func (m *IdentityManager) GetIdentity() *WorkloadIdentity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.identity
}

// Watch registers a watcher for identity changes.
func (m *IdentityManager) Watch(ch chan<- *WorkloadIdentity) {
	m.watchersMu.Lock()
	defer m.watchersMu.Unlock()
	m.watchers = append(m.watchers, ch)
}

// notifyWatchers notifies all watchers of an identity change.
func (m *IdentityManager) notifyWatchers(identity *WorkloadIdentity) {
	m.watchersMu.RLock()
	defer m.watchersMu.RUnlock()

	for _, ch := range m.watchers {
		select {
		case ch <- identity:
		default:
			// Don't block if channel is full
		}
	}
}

// ValidatePeer validates a peer's identity from a TLS connection.
func (m *IdentityManager) ValidatePeer(conn *tls.Conn) (*SPIFFEID, error) {
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		if m.config.RequireSPIFFEID {
			return nil, ErrIdentityNotFound
		}
		return nil, nil
	}

	peerCert := state.PeerCertificates[0]

	// Extract SPIFFE ID
	verifier := NewSPIFFEVerifier(m.config.AllowedTrustDomains)
	peerID, err := verifier.VerifyCertificate(peerCert)
	if err != nil {
		if m.config.RequireSPIFFEID {
			return nil, err
		}
		return nil, nil
	}

	return peerID, nil
}

// Authorize checks if a peer identity is authorized to access a service.
func (m *IdentityManager) Authorize(peerID *SPIFFEID, targetService string) error {
	// Get policy for target service
	policy := m.policies.GetPolicy(targetService)
	if policy == nil {
		// No policy = allow all
		return nil
	}

	if !policy.IsAuthorized(peerID) {
		return ErrIdentityDenied
	}

	return nil
}

// SetPolicy sets an authorization policy for a service.
func (m *IdentityManager) SetPolicy(service string, policy *AuthorizationPolicy) {
	m.policies.SetPolicy(service, policy)
}

// GetPolicy returns the authorization policy for a service.
func (m *IdentityManager) GetPolicy(service string) *AuthorizationPolicy {
	return m.policies.GetPolicy(service)
}

// Close shuts down the identity manager.
func (m *IdentityManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	// Close watcher channels
	m.watchersMu.Lock()
	for _, ch := range m.watchers {
		close(ch)
	}
	m.watchers = nil
	m.watchersMu.Unlock()

	return nil
}

// PolicyStore manages authorization policies.
type PolicyStore struct {
	policies map[string]*AuthorizationPolicy
	mu       sync.RWMutex
}

// NewPolicyStore creates a new policy store.
func NewPolicyStore() *PolicyStore {
	return &PolicyStore{
		policies: make(map[string]*AuthorizationPolicy),
	}
}

// SetPolicy sets a policy for a service.
func (s *PolicyStore) SetPolicy(service string, policy *AuthorizationPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[service] = policy
}

// GetPolicy returns the policy for a service.
func (s *PolicyStore) GetPolicy(service string) *AuthorizationPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policies[service]
}

// DeletePolicy removes a policy for a service.
func (s *PolicyStore) DeletePolicy(service string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.policies, service)
}

// ListPolicies returns all policies.
func (s *PolicyStore) ListPolicies() map[string]*AuthorizationPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*AuthorizationPolicy)
	for k, v := range s.policies {
		result[k] = v
	}
	return result
}

// PeerInfo contains information extracted from a peer connection.
type PeerInfo struct {
	// Identity
	SPIFFEID *SPIFFEID
	CommonName string
	ServiceName string
	Namespace string

	// Connection info
	LocalAddr  net.Addr
	RemoteAddr net.Addr

	// TLS info
	TLSVersion     uint16
	CipherSuite    uint16
	Certificate    *x509.Certificate
	VerifiedChains [][]*x509.Certificate
}

// ExtractPeerInfo extracts peer information from a TLS connection.
func ExtractPeerInfo(conn *tls.Conn) (*PeerInfo, error) {
	state := conn.ConnectionState()

	info := &PeerInfo{
		LocalAddr:      conn.LocalAddr(),
		RemoteAddr:     conn.RemoteAddr(),
		TLSVersion:     state.Version,
		CipherSuite:    state.CipherSuite,
		VerifiedChains: state.VerifiedChains,
	}

	if len(state.PeerCertificates) > 0 {
		info.Certificate = state.PeerCertificates[0]
		info.CommonName = state.PeerCertificates[0].Subject.CommonName

		// Extract SPIFFE ID
		verifier := NewSPIFFEVerifier([]string{"*"})
		if id, err := verifier.ExtractSPIFFEID(state.PeerCertificates[0]); err == nil {
			info.SPIFFEID = id
			info.ServiceName = id.ServiceName()
			info.Namespace = id.Namespace()
		}
	}

	return info, nil
}

// ServiceMatcher helps match service identities.
type ServiceMatcher struct {
	patterns []string
}

// NewServiceMatcher creates a new service matcher.
func NewServiceMatcher(patterns ...string) *ServiceMatcher {
	return &ServiceMatcher{
		patterns: patterns,
	}
}

// Matches returns true if the SPIFFE ID matches any pattern.
func (m *ServiceMatcher) Matches(id *SPIFFEID) bool {
	if len(m.patterns) == 0 {
		return true // No patterns = match all
	}

	for _, pattern := range m.patterns {
		if id.Matches(pattern) {
			return true
		}
	}

	return false
}

// AddPattern adds a pattern to the matcher.
func (m *ServiceMatcher) AddPattern(pattern string) {
	m.patterns = append(m.patterns, pattern)
}

// IdentityValidator validates workload identities.
type IdentityValidator struct {
	allowedDomains []string
	allowedServices []string
	deniedServices  []string
	requireSPIFFE   bool
}

// NewIdentityValidator creates a new identity validator.
func NewIdentityValidator(allowedDomains []string) *IdentityValidator {
	return &IdentityValidator{
		allowedDomains:  allowedDomains,
		allowedServices: make([]string, 0),
		deniedServices:  make([]string, 0),
		requireSPIFFE:   true,
	}
}

// AllowService adds a service to the allowed list.
func (v *IdentityValidator) AllowService(pattern string) {
	v.allowedServices = append(v.allowedServices, pattern)
}

// DenyService adds a service to the denied list.
func (v *IdentityValidator) DenyService(pattern string) {
	v.deniedServices = append(v.deniedServices, pattern)
}

// SetRequireSPIFFE sets whether SPIFFE ID is required.
func (v *IdentityValidator) SetRequireSPIFFE(required bool) {
	v.requireSPIFFE = required
}

// ValidateConnection validates a TLS connection.
func (v *IdentityValidator) ValidateConnection(conn *tls.Conn) (*SPIFFEID, error) {
	state := conn.ConnectionState()

	if len(state.PeerCertificates) == 0 {
		if v.requireSPIFFE {
			return nil, ErrIdentityNotFound
		}
		return nil, nil
	}

	// Extract and verify SPIFFE ID
	verifier := NewSPIFFEVerifier(v.allowedDomains)
	for _, pattern := range v.allowedServices {
		verifier.AllowID(pattern)
	}

	id, err := verifier.VerifyCertificate(state.PeerCertificates[0])
	if err != nil {
		return nil, err
	}

	// Check deny list
	idStr := id.String()
	for _, pattern := range v.deniedServices {
		if id.Matches(pattern) || strings.Contains(idStr, pattern) {
			return nil, ErrIdentityDenied
		}
	}

	// Check allow list if specified
	if len(v.allowedServices) > 0 {
		allowed := false
		for _, pattern := range v.allowedServices {
			if id.Matches(pattern) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, ErrIdentityDenied
		}
	}

	return id, nil
}

// ValidateCertificate validates a certificate directly.
func (v *IdentityValidator) ValidateCertificate(cert *x509.Certificate) (*SPIFFEID, error) {
	verifier := NewSPIFFEVerifier(v.allowedDomains)
	return verifier.VerifyCertificate(cert)
}

// ZeroTrustEnforcer enforces zero-trust policies on connections.
type ZeroTrustEnforcer struct {
	validator *IdentityValidator
	policies  *PolicyStore

	// Metrics
	allowedCount uint64
	deniedCount  uint64
}

// NewZeroTrustEnforcer creates a new zero-trust enforcer.
func NewZeroTrustEnforcer(trustDomains []string) *ZeroTrustEnforcer {
	return &ZeroTrustEnforcer{
		validator: NewIdentityValidator(trustDomains),
		policies:  NewPolicyStore(),
	}
}

// Enforce validates and authorizes a connection to a target service.
func (e *ZeroTrustEnforcer) Enforce(conn *tls.Conn, targetService string) error {
	// Validate the peer identity
	peerID, err := e.validator.ValidateConnection(conn)
	if err != nil {
		e.deniedCount++
		return fmt.Errorf("identity validation failed: %w", err)
	}

	// Check authorization policy
	policy := e.policies.GetPolicy(targetService)
	if policy != nil && !policy.IsAuthorized(peerID) {
		e.deniedCount++
		return ErrIdentityDenied
	}

	e.allowedCount++
	return nil
}

// SetPolicy sets a policy for a target service.
func (e *ZeroTrustEnforcer) SetPolicy(service string, policy *AuthorizationPolicy) {
	e.policies.SetPolicy(service, policy)
}

// Stats returns enforcer statistics.
func (e *ZeroTrustEnforcer) Stats() (allowed, denied uint64) {
	return e.allowedCount, e.deniedCount
}
