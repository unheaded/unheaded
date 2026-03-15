// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package mtls provides mutual TLS handling for THE HAUBERK.
package mtls

import (
	"crypto/x509"
	"errors"
	"net/url"
	"strings"
)

const (
	// SPIFFEScheme is the SPIFFE URI scheme.
	SPIFFEScheme = "spiffe"
	// DefaultTrustDomain is the default SPIFFE trust domain.
	DefaultTrustDomain = "unheaded.local"
)

var (
	// ErrInvalidSPIFFEID indicates an invalid SPIFFE ID.
	ErrInvalidSPIFFEID = errors.New("invalid SPIFFE ID")
	// ErrTrustDomainMismatch indicates a trust domain mismatch.
	ErrTrustDomainMismatch = errors.New("trust domain mismatch")
	// ErrSPIFFENotFound indicates no SPIFFE ID in certificate.
	ErrSPIFFENotFound = errors.New("no SPIFFE ID found in certificate")
)

// SPIFFEID represents a SPIFFE identity.
type SPIFFEID struct {
	TrustDomain string
	Path        string
}

// NewSPIFFEID creates a new SPIFFE ID for a service.
func NewSPIFFEID(trustDomain, serviceName string) *SPIFFEID {
	return &SPIFFEID{
		TrustDomain: trustDomain,
		Path:        "/service/" + serviceName,
	}
}

// NewSPIFFEIDFromNamespace creates a SPIFFE ID with namespace.
func NewSPIFFEIDFromNamespace(trustDomain, namespace, serviceName string) *SPIFFEID {
	return &SPIFFEID{
		TrustDomain: trustDomain,
		Path:        "/ns/" + namespace + "/sa/" + serviceName,
	}
}

// ParseSPIFFEID parses a SPIFFE ID string.
func ParseSPIFFEID(s string) (*SPIFFEID, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, ErrInvalidSPIFFEID
	}

	if u.Scheme != SPIFFEScheme {
		return nil, ErrInvalidSPIFFEID
	}

	if u.Host == "" {
		return nil, ErrInvalidSPIFFEID
	}

	// SPIFFE ID must have a path
	if u.Path == "" {
		return nil, ErrInvalidSPIFFEID
	}

	// SPIFFE ID cannot have query or fragment
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, ErrInvalidSPIFFEID
	}

	// SPIFFE ID cannot have user info
	if u.User != nil {
		return nil, ErrInvalidSPIFFEID
	}

	// SPIFFE ID cannot have port
	if u.Port() != "" {
		return nil, ErrInvalidSPIFFEID
	}

	return &SPIFFEID{
		TrustDomain: u.Host,
		Path:        u.Path,
	}, nil
}

// String returns the SPIFFE ID as a URI string.
func (id *SPIFFEID) String() string {
	return SPIFFEScheme + "://" + id.TrustDomain + id.Path
}

// ServiceName extracts the service name from the SPIFFE ID path.
func (id *SPIFFEID) ServiceName() string {
	// Handle /service/{name} format
	if strings.HasPrefix(id.Path, "/service/") {
		return strings.TrimPrefix(id.Path, "/service/")
	}

	// Handle /ns/{namespace}/sa/{name} format
	parts := strings.Split(id.Path, "/")
	for i, part := range parts {
		if part == "sa" && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	// Return last path segment
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return ""
}

// Namespace extracts the namespace from the SPIFFE ID path.
func (id *SPIFFEID) Namespace() string {
	parts := strings.Split(id.Path, "/")
	for i, part := range parts {
		if part == "ns" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "default"
}

// Matches checks if the SPIFFE ID matches a pattern.
// Patterns can include wildcards:
// - spiffe://domain/* matches any path in the domain
// - spiffe://domain/service/* matches any service
func (id *SPIFFEID) Matches(pattern string) bool {
	patternID, err := ParseSPIFFEID(strings.TrimSuffix(pattern, "*"))
	if err != nil {
		// Try simple wildcard matching
		return matchWildcard(pattern, id.String())
	}

	// Trust domain must match
	if patternID.TrustDomain != id.TrustDomain {
		return false
	}

	// Check path matching
	if strings.HasSuffix(pattern, "*") {
		// Prefix match
		return strings.HasPrefix(id.Path, patternID.Path)
	}

	return patternID.Path == id.Path
}

// matchWildcard performs simple wildcard matching.
func matchWildcard(pattern, s string) bool {
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(s, pattern[1:])
	}

	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(s, pattern[:len(pattern)-1])
	}

	return pattern == s
}

// SPIFFEVerifier verifies SPIFFE IDs in certificates.
type SPIFFEVerifier struct {
	trustDomains []string
	allowedIDs   []string // patterns of allowed SPIFFE IDs
}

// NewSPIFFEVerifier creates a new SPIFFE verifier.
func NewSPIFFEVerifier(trustDomains []string) *SPIFFEVerifier {
	return &SPIFFEVerifier{
		trustDomains: trustDomains,
		allowedIDs:   make([]string, 0),
	}
}

// AllowID adds an allowed SPIFFE ID pattern.
func (v *SPIFFEVerifier) AllowID(pattern string) {
	v.allowedIDs = append(v.allowedIDs, pattern)
}

// ExtractSPIFFEID extracts the SPIFFE ID from a certificate.
func (v *SPIFFEVerifier) ExtractSPIFFEID(cert *x509.Certificate) (*SPIFFEID, error) {
	for _, uri := range cert.URIs {
		if uri.Scheme == SPIFFEScheme {
			return ParseSPIFFEID(uri.String())
		}
	}
	return nil, ErrSPIFFENotFound
}

// VerifyCertificate verifies the SPIFFE ID in a certificate.
func (v *SPIFFEVerifier) VerifyCertificate(cert *x509.Certificate) (*SPIFFEID, error) {
	id, err := v.ExtractSPIFFEID(cert)
	if err != nil {
		return nil, err
	}

	// Verify trust domain
	if !v.isTrustedDomain(id.TrustDomain) {
		return nil, ErrTrustDomainMismatch
	}

	// Verify against allowed patterns
	if len(v.allowedIDs) > 0 {
		allowed := false
		for _, pattern := range v.allowedIDs {
			if id.Matches(pattern) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, ErrInvalidSPIFFEID
		}
	}

	return id, nil
}

// VerifyPeerCertificate is a callback for TLS config verification.
func (v *SPIFFEVerifier) VerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(verifiedChains) == 0 || len(verifiedChains[0]) == 0 {
		return ErrSPIFFENotFound
	}

	cert := verifiedChains[0][0]
	_, err := v.VerifyCertificate(cert)
	return err
}

func (v *SPIFFEVerifier) isTrustedDomain(domain string) bool {
	for _, td := range v.trustDomains {
		if td == domain || td == "*" {
			return true
		}
	}
	return false
}

// SPIFFEBundle represents a SPIFFE trust bundle.
type SPIFFEBundle struct {
	TrustDomain string
	Roots       []*x509.Certificate
}

// NewSPIFFEBundle creates a new trust bundle.
func NewSPIFFEBundle(trustDomain string) *SPIFFEBundle {
	return &SPIFFEBundle{
		TrustDomain: trustDomain,
		Roots:       make([]*x509.Certificate, 0),
	}
}

// AddRoot adds a root certificate to the bundle.
func (b *SPIFFEBundle) AddRoot(cert *x509.Certificate) {
	b.Roots = append(b.Roots, cert)
}

// CertPool returns an x509 certificate pool from the bundle.
func (b *SPIFFEBundle) CertPool() *x509.CertPool {
	pool := x509.NewCertPool()
	for _, cert := range b.Roots {
		pool.AddCert(cert)
	}
	return pool
}

// AuthorizationPolicy defines which SPIFFE IDs can access a service.
type AuthorizationPolicy struct {
	Service    string
	AllowedIDs []string // patterns
	DeniedIDs  []string // patterns
}

// IsAuthorized checks if a SPIFFE ID is authorized.
func (p *AuthorizationPolicy) IsAuthorized(id *SPIFFEID) bool {
	idStr := id.String()

	// Check denied list first
	for _, pattern := range p.DeniedIDs {
		if id.Matches(pattern) || matchWildcard(pattern, idStr) {
			return false
		}
	}

	// If no allowed list, allow all (that aren't denied)
	if len(p.AllowedIDs) == 0 {
		return true
	}

	// Check allowed list
	for _, pattern := range p.AllowedIDs {
		if id.Matches(pattern) || matchWildcard(pattern, idStr) {
			return true
		}
	}

	return false
}
