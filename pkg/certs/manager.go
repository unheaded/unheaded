// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package certs provides TLS certificate management for The Moat.
// It implements mTLS with short-lived certificates and automatic rotation.
package certs

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"sync"
	"time"

	"unheaded/pkg/certs/ca"
	"unheaded/pkg/certs/issue"
	"unheaded/pkg/certs/rotation"
	"unheaded/pkg/certs/store"
)

var (
	// ErrCertificateNotFound indicates the certificate doesn't exist.
	ErrCertificateNotFound = errors.New("certificate not found")
	// ErrCertificateRevoked indicates the certificate has been revoked.
	ErrCertificateRevoked = errors.New("certificate revoked")
	// ErrCertificateExpired indicates the certificate has expired.
	ErrCertificateExpired = errors.New("certificate expired")
	// ErrInvalidCSR indicates the CSR is malformed or invalid.
	ErrInvalidCSR = errors.New("invalid certificate signing request")
	// ErrCANotInitialized indicates the CA has not been initialized.
	ErrCANotInitialized = errors.New("certificate authority not initialized")
)

// CertType represents the type of certificate.
type CertType int

const (
	// CertTypeServer is for server authentication.
	CertTypeServer CertType = iota
	// CertTypeClient is for client authentication.
	CertTypeClient
	// CertTypeMutual is for both client and server authentication.
	CertTypeMutual
	// CertTypeCA is for certificate authority certificates.
	CertTypeCA
)

// CertRequest contains the parameters for certificate issuance.
type CertRequest struct {
	// CommonName is the CN field of the certificate.
	CommonName string
	// DNSNames are the SANs for DNS names.
	DNSNames []string
	// IPAddresses are the SANs for IP addresses.
	IPAddresses []string
	// URIs are the SANs for URI (including SPIFFE IDs).
	URIs []string
	// Type specifies the certificate type.
	Type CertType
	// Duration is how long the certificate is valid.
	Duration time.Duration
	// SPIFFEID is the SPIFFE identity for the certificate.
	SPIFFEID string
	// Organization is the O field.
	Organization string
	// OrganizationalUnit is the OU field.
	OrganizationalUnit string
	// CSR is an optional CSR to sign instead of generating a new key.
	CSR []byte
}

// Certificate represents an issued certificate with its metadata.
type Certificate struct {
	// Serial is the unique serial number.
	Serial string
	// Certificate is the X.509 certificate.
	Certificate *x509.Certificate
	// CertificatePEM is the PEM-encoded certificate.
	CertificatePEM []byte
	// PrivateKeyPEM is the PEM-encoded private key (if generated).
	PrivateKeyPEM []byte
	// ChainPEM is the full certificate chain.
	ChainPEM []byte
	// IssuedAt is when the certificate was issued.
	IssuedAt time.Time
	// ExpiresAt is when the certificate expires.
	ExpiresAt time.Time
	// RevokedAt is when the certificate was revoked (if applicable).
	RevokedAt *time.Time
	// SPIFFEID is the SPIFFE identity.
	SPIFFEID string
	// Type is the certificate type.
	Type CertType
}

// IsExpired returns true if the certificate has expired.
func (c *Certificate) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// IsRevoked returns true if the certificate has been revoked.
func (c *Certificate) IsRevoked() bool {
	return c.RevokedAt != nil
}

// IsValid returns true if the certificate is valid (not expired or revoked).
func (c *Certificate) IsValid() bool {
	return !c.IsExpired() && !c.IsRevoked()
}

// TimeToExpiry returns the duration until the certificate expires.
func (c *Certificate) TimeToExpiry() time.Duration {
	return time.Until(c.ExpiresAt)
}

// CertManager defines the interface for certificate management.
type CertManager interface {
	// IssueCertificate creates a new certificate.
	IssueCertificate(ctx context.Context, req *CertRequest) (*Certificate, error)
	// RevokeCertificate revokes a certificate by serial number.
	RevokeCertificate(ctx context.Context, serial string) error
	// GetCertificate retrieves a certificate by serial number.
	GetCertificate(ctx context.Context, serial string) (*Certificate, error)
	// ListCertificates returns all certificates.
	ListCertificates(ctx context.Context) ([]*Certificate, error)
	// RenewCertificate renews a certificate.
	RenewCertificate(ctx context.Context, serial string) (*Certificate, error)
	// GetCACertificate returns the CA certificate.
	GetCACertificate(ctx context.Context) (*Certificate, error)
	// VerifyCertificate verifies a certificate against the CA.
	VerifyCertificate(ctx context.Context, certPEM []byte) error
	// Close shuts down the certificate manager.
	Close() error
}

// ManagerConfig configures the certificate manager.
type ManagerConfig struct {
	// CAConfig is the CA configuration.
	CAConfig *ca.Config
	// StoreConfig is the store configuration.
	StoreConfig *store.Config
	// RotationConfig is the rotation configuration.
	RotationConfig *rotation.Config
	// DefaultDuration is the default certificate duration.
	DefaultDuration time.Duration
	// MaxDuration is the maximum allowed certificate duration.
	MaxDuration time.Duration
	// EnableRotation enables automatic certificate rotation.
	EnableRotation bool
}

// DefaultManagerConfig returns a default configuration.
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		CAConfig:        ca.DefaultConfig(),
		StoreConfig:     store.DefaultConfig(),
		RotationConfig:  rotation.DefaultConfig(),
		DefaultDuration: 24 * time.Hour,
		MaxDuration:     7 * 24 * time.Hour,
		EnableRotation:  true,
	}
}

// Manager implements the CertManager interface.
type Manager struct {
	config   *ManagerConfig
	ca       *ca.Authority
	issuer   *issue.Issuer
	store    store.Store
	rotator  *rotation.Rotator
	mu       sync.RWMutex
	closed   bool
	closeCh  chan struct{}
}

// NewManager creates a new certificate manager.
func NewManager(config *ManagerConfig) (*Manager, error) {
	if config == nil {
		config = DefaultManagerConfig()
	}

	// Initialize CA
	authority, err := ca.New(config.CAConfig)
	if err != nil {
		return nil, err
	}

	// Initialize store
	certStore, err := store.New(config.StoreConfig)
	if err != nil {
		return nil, err
	}

	// Initialize issuer
	issuer := issue.New(authority, config.DefaultDuration, config.MaxDuration)

	m := &Manager{
		config:  config,
		ca:      authority,
		issuer:  issuer,
		store:   certStore,
		closeCh: make(chan struct{}),
	}

	// Initialize rotator if enabled
	if config.EnableRotation {
		m.rotator = rotation.New(config.RotationConfig, m.renewCallback)
		go m.rotator.Start(m.closeCh)
	}

	return m, nil
}

// IssueCertificate creates a new certificate.
func (m *Manager) IssueCertificate(ctx context.Context, req *CertRequest) (*Certificate, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, errors.New("manager closed")
	}
	m.mu.RUnlock()

	if req.Duration == 0 {
		req.Duration = m.config.DefaultDuration
	}
	if req.Duration > m.config.MaxDuration {
		req.Duration = m.config.MaxDuration
	}

	// Convert to issuer request
	issueReq := &issue.Request{
		CommonName:         req.CommonName,
		DNSNames:           req.DNSNames,
		IPAddresses:        req.IPAddresses,
		URIs:               req.URIs,
		Duration:           req.Duration,
		SPIFFEID:           req.SPIFFEID,
		Organization:       req.Organization,
		OrganizationalUnit: req.OrganizationalUnit,
		CSR:                req.CSR,
		KeyUsage:           issue.KeyUsageFromType(int(req.Type)),
		ExtKeyUsage:        issue.ExtKeyUsageFromType(int(req.Type)),
	}

	// Issue certificate
	result, err := m.issuer.Issue(ctx, issueReq)
	if err != nil {
		return nil, err
	}

	cert := &Certificate{
		Serial:         result.Serial,
		Certificate:    result.Certificate,
		CertificatePEM: result.CertificatePEM,
		PrivateKeyPEM:  result.PrivateKeyPEM,
		ChainPEM:       result.ChainPEM,
		IssuedAt:       result.Certificate.NotBefore,
		ExpiresAt:      result.Certificate.NotAfter,
		SPIFFEID:       req.SPIFFEID,
		Type:           req.Type,
	}

	// Store certificate (convert to storable format)
	storeData := &store.CertificateData{
		Serial:         cert.Serial,
		CertificatePEM: cert.CertificatePEM,
		PrivateKeyPEM:  cert.PrivateKeyPEM,
		ChainPEM:       cert.ChainPEM,
		IssuedAt:       cert.IssuedAt,
		ExpiresAt:      cert.ExpiresAt,
		SPIFFEID:       cert.SPIFFEID,
		Type:           int(cert.Type),
	}
	if err := m.store.Put(ctx, cert.Serial, storeData); err != nil {
		return nil, err
	}

	// Register for rotation if enabled
	if m.rotator != nil {
		m.rotator.Register(cert.Serial, cert.ExpiresAt)
	}

	return cert, nil
}

// RevokeCertificate revokes a certificate by serial number.
func (m *Manager) RevokeCertificate(ctx context.Context, serial string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return errors.New("manager closed")
	}
	m.mu.RUnlock()

	data, err := m.store.Get(ctx, serial)
	if err != nil {
		return err
	}
	if data == nil {
		return ErrCertificateNotFound
	}

	storeData, ok := data.(*store.CertificateData)
	if !ok {
		return errors.New("invalid certificate data")
	}

	now := time.Now()
	storeData.RevokedAt = &now

	if err := m.store.Put(ctx, serial, storeData); err != nil {
		return err
	}

	// Add to CRL
	if err := m.ca.Revoke(serial); err != nil {
		return err
	}

	// Unregister from rotation
	if m.rotator != nil {
		m.rotator.Unregister(serial)
	}

	return nil
}

// GetCertificate retrieves a certificate by serial number.
func (m *Manager) GetCertificate(ctx context.Context, serial string) (*Certificate, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, errors.New("manager closed")
	}
	m.mu.RUnlock()

	data, err := m.store.Get(ctx, serial)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrCertificateNotFound
	}

	return m.storeDataToCert(data)
}

// storeDataToCert converts store data to Certificate.
func (m *Manager) storeDataToCert(data interface{}) (*Certificate, error) {
	storeData, ok := data.(*store.CertificateData)
	if !ok {
		return nil, errors.New("invalid certificate data")
	}

	var x509Cert *x509.Certificate
	if len(storeData.CertificatePEM) > 0 {
		block, _ := pem.Decode(storeData.CertificatePEM)
		if block != nil {
			x509Cert, _ = x509.ParseCertificate(block.Bytes)
		}
	}

	return &Certificate{
		Serial:         storeData.Serial,
		Certificate:    x509Cert,
		CertificatePEM: storeData.CertificatePEM,
		PrivateKeyPEM:  storeData.PrivateKeyPEM,
		ChainPEM:       storeData.ChainPEM,
		IssuedAt:       storeData.IssuedAt,
		ExpiresAt:      storeData.ExpiresAt,
		RevokedAt:      storeData.RevokedAt,
		SPIFFEID:       storeData.SPIFFEID,
		Type:           CertType(storeData.Type),
	}, nil
}

// ListCertificates returns all certificates.
func (m *Manager) ListCertificates(ctx context.Context) ([]*Certificate, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, errors.New("manager closed")
	}
	m.mu.RUnlock()

	dataList, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}

	certs := make([]*Certificate, 0, len(dataList))
	for _, data := range dataList {
		cert, err := m.storeDataToCert(data)
		if err != nil {
			continue
		}
		certs = append(certs, cert)
	}

	return certs, nil
}

// RenewCertificate renews a certificate.
func (m *Manager) RenewCertificate(ctx context.Context, serial string) (*Certificate, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, errors.New("manager closed")
	}
	m.mu.RUnlock()

	oldCert, err := m.GetCertificate(ctx, serial)
	if err != nil {
		return nil, err
	}
	if oldCert.IsRevoked() {
		return nil, ErrCertificateRevoked
	}

	// Create new certificate with same parameters
	req := &CertRequest{
		CommonName: oldCert.Certificate.Subject.CommonName,
		DNSNames:   oldCert.Certificate.DNSNames,
		Type:       oldCert.Type,
		SPIFFEID:   oldCert.SPIFFEID,
		Duration:   oldCert.ExpiresAt.Sub(oldCert.IssuedAt),
	}

	if len(oldCert.Certificate.Subject.Organization) > 0 {
		req.Organization = oldCert.Certificate.Subject.Organization[0]
	}
	if len(oldCert.Certificate.Subject.OrganizationalUnit) > 0 {
		req.OrganizationalUnit = oldCert.Certificate.Subject.OrganizationalUnit[0]
	}

	// Convert IP addresses
	for _, ip := range oldCert.Certificate.IPAddresses {
		req.IPAddresses = append(req.IPAddresses, ip.String())
	}

	// Issue new certificate
	newCert, err := m.IssueCertificate(ctx, req)
	if err != nil {
		return nil, err
	}

	// Revoke old certificate
	if err := m.RevokeCertificate(ctx, serial); err != nil {
		// Log but don't fail - new cert is already issued
		_ = err
	}

	return newCert, nil
}

// GetCACertificate returns the CA certificate.
func (m *Manager) GetCACertificate(ctx context.Context) (*Certificate, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, errors.New("manager closed")
	}
	m.mu.RUnlock()

	caCert, caPEM := m.ca.GetCertificate()
	if caCert == nil {
		return nil, ErrCANotInitialized
	}

	return &Certificate{
		Serial:         caCert.SerialNumber.String(),
		Certificate:    caCert,
		CertificatePEM: caPEM,
		IssuedAt:       caCert.NotBefore,
		ExpiresAt:      caCert.NotAfter,
		Type:           CertTypeCA,
	}, nil
}

// VerifyCertificate verifies a certificate against the CA.
func (m *Manager) VerifyCertificate(ctx context.Context, certPEM []byte) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return errors.New("manager closed")
	}
	m.mu.RUnlock()

	return m.ca.Verify(certPEM)
}

// Close shuts down the certificate manager.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	close(m.closeCh)

	if err := m.store.Close(); err != nil {
		return err
	}

	return nil
}

// renewCallback is called when a certificate needs renewal.
func (m *Manager) renewCallback(serial string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := m.RenewCertificate(ctx, serial)
	if err != nil {
		// Log renewal failure
		_ = err
	}
}
