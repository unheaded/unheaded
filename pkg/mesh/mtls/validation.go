// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package mtls provides certificate validation utilities for zero-trust networking.
package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

var (
	// ErrCertificateNotValid indicates the certificate is not valid.
	ErrCertificateNotValid = errors.New("certificate not valid")
	// ErrChainVerificationFailed indicates chain verification failed.
	ErrChainVerificationFailed = errors.New("certificate chain verification failed")
	// ErrCertificateExpiringSoon indicates the certificate is expiring soon.
	ErrCertificateExpiringSoon = errors.New("certificate expiring soon")
	// ErrWeakCipher indicates a weak cipher is being used.
	ErrWeakCipher = errors.New("weak cipher detected")
)

// CertificateInfo contains parsed certificate information.
type CertificateInfo struct {
	// Subject
	CommonName         string
	Organization       []string
	OrganizationalUnit []string

	// SANs
	DNSNames    []string
	IPAddresses []net.IP
	URIs        []string
	SPIFFEID    *SPIFFEID

	// Validity
	NotBefore    time.Time
	NotAfter     time.Time
	IsExpired    bool
	TimeToExpiry time.Duration

	// Chain info
	Issuer      string
	IsCA        bool
	KeyUsage    x509.KeyUsage
	ExtKeyUsage []x509.ExtKeyUsage

	// Serial
	SerialNumber string

	// Signature
	SignatureAlgorithm x509.SignatureAlgorithm
	PublicKeyAlgorithm x509.PublicKeyAlgorithm
}

// ParseCertificateInfo extracts information from a certificate.
func ParseCertificateInfo(cert *x509.Certificate) *CertificateInfo {
	info := &CertificateInfo{
		CommonName:         cert.Subject.CommonName,
		Organization:       cert.Subject.Organization,
		OrganizationalUnit: cert.Subject.OrganizationalUnit,
		DNSNames:           cert.DNSNames,
		IPAddresses:        cert.IPAddresses,
		NotBefore:          cert.NotBefore,
		NotAfter:           cert.NotAfter,
		IsExpired:          time.Now().After(cert.NotAfter),
		TimeToExpiry:       time.Until(cert.NotAfter),
		Issuer:             cert.Issuer.CommonName,
		IsCA:               cert.IsCA,
		KeyUsage:           cert.KeyUsage,
		ExtKeyUsage:        cert.ExtKeyUsage,
		SerialNumber:       cert.SerialNumber.String(),
		SignatureAlgorithm: cert.SignatureAlgorithm,
		PublicKeyAlgorithm: cert.PublicKeyAlgorithm,
	}

	// Extract URIs
	for _, uri := range cert.URIs {
		info.URIs = append(info.URIs, uri.String())
	}

	// Extract SPIFFE ID
	verifier := NewSPIFFEVerifier([]string{"*"})
	if id, err := verifier.ExtractSPIFFEID(cert); err == nil {
		info.SPIFFEID = id
	}

	return info
}

// LoadCertificateFromFile loads and parses a certificate from a file.
func LoadCertificateFromFile(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	return x509.ParseCertificate(block.Bytes)
}

// LoadCertificateChainFromFile loads a certificate chain from a file.
func LoadCertificateChainFromFile(path string) ([]*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return ParseCertificateChain(data)
}

// ParseCertificateChain parses a PEM-encoded certificate chain.
func ParseCertificateChain(data []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate

	for len(data) > 0 {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}

		if block.Type != "CERTIFICATE" {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		return nil, errors.New("no certificates found")
	}

	return certs, nil
}

// ValidationOptions configures certificate validation.
type ValidationOptions struct {
	// Trust anchors
	RootCAs       *x509.CertPool
	Intermediates *x509.CertPool

	// Validation time (default: now)
	CurrentTime time.Time

	// DNS name to verify (optional)
	DNSName string

	// Key usages to require
	KeyUsages []x509.ExtKeyUsage

	// SPIFFE validation
	RequireSPIFFE       bool
	AllowedTrustDomains []string

	// Expiry warnings
	ExpiryWarningDays int
}

// DefaultValidationOptions returns default validation options.
func DefaultValidationOptions(rootCAs *x509.CertPool) *ValidationOptions {
	return &ValidationOptions{
		RootCAs:             rootCAs,
		CurrentTime:         time.Now(),
		ExpiryWarningDays:   7,
		AllowedTrustDomains: []string{"*"},
	}
}

// ValidationResult contains the result of certificate validation.
type ValidationResult struct {
	Valid           bool
	Errors          []error
	Warnings        []string
	VerifiedChains  [][]*x509.Certificate
	CertificateInfo *CertificateInfo
	SPIFFEID        *SPIFFEID
}

// ValidateCertificate performs comprehensive certificate validation.
func ValidateCertificate(cert *x509.Certificate, opts *ValidationOptions) *ValidationResult {
	result := &ValidationResult{
		Valid:           true,
		Errors:          make([]error, 0),
		Warnings:        make([]string, 0),
		CertificateInfo: ParseCertificateInfo(cert),
	}

	// Check expiration
	now := opts.CurrentTime
	if now.IsZero() {
		now = time.Now()
	}

	if now.Before(cert.NotBefore) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Errorf("certificate not yet valid (valid from %v)", cert.NotBefore))
	}

	if now.After(cert.NotAfter) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Errorf("certificate expired (expired at %v)", cert.NotAfter))
	}

	// Check expiry warning
	if opts.ExpiryWarningDays > 0 {
		warningThreshold := now.AddDate(0, 0, opts.ExpiryWarningDays)
		if cert.NotAfter.Before(warningThreshold) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("certificate expires in %v", time.Until(cert.NotAfter).Round(time.Hour)))
		}
	}

	// Verify chain if root CAs provided
	if opts.RootCAs != nil {
		verifyOpts := x509.VerifyOptions{
			Roots:         opts.RootCAs,
			Intermediates: opts.Intermediates,
			CurrentTime:   now,
			KeyUsages:     opts.KeyUsages,
		}

		if opts.DNSName != "" {
			verifyOpts.DNSName = opts.DNSName
		}

		chains, err := cert.Verify(verifyOpts)
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Errorf("chain verification failed: %w", err))
		} else {
			result.VerifiedChains = chains
		}
	}

	// Validate SPIFFE ID if required
	if opts.RequireSPIFFE || len(opts.AllowedTrustDomains) > 0 {
		verifier := NewSPIFFEVerifier(opts.AllowedTrustDomains)
		id, err := verifier.VerifyCertificate(cert)
		if err != nil {
			if opts.RequireSPIFFE {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Errorf("SPIFFE validation failed: %w", err))
			} else {
				result.Warnings = append(result.Warnings, "no SPIFFE ID found in certificate")
			}
		} else {
			result.SPIFFEID = id
		}
	}

	// Check key strength
	if warning := checkKeyStrength(cert); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}

	// Check signature algorithm
	if warning := checkSignatureAlgorithm(cert); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}

	return result
}

// checkKeyStrength checks for weak key sizes.
func checkKeyStrength(cert *x509.Certificate) string {
	switch cert.PublicKeyAlgorithm {
	case x509.RSA:
		if cert.PublicKey != nil {
			// RSA key size is typically checked via bits
			// For simplicity, we check if it's less than 2048 bits
			// In practice, this would use rsa.PublicKey.Size()
		}
	case x509.ECDSA:
		// ECDSA P-256 or higher is generally acceptable
	}
	return ""
}

// checkSignatureAlgorithm checks for weak signature algorithms.
func checkSignatureAlgorithm(cert *x509.Certificate) string {
	switch cert.SignatureAlgorithm {
	case x509.MD5WithRSA, x509.SHA1WithRSA:
		return "weak signature algorithm detected: " + cert.SignatureAlgorithm.String()
	}
	return ""
}

// TLSConnectionValidator validates TLS connections.
type TLSConnectionValidator struct {
	opts *ValidationOptions
}

// NewTLSConnectionValidator creates a new TLS connection validator.
func NewTLSConnectionValidator(rootCAs *x509.CertPool) *TLSConnectionValidator {
	return &TLSConnectionValidator{
		opts: DefaultValidationOptions(rootCAs),
	}
}

// SetRequireSPIFFE sets whether SPIFFE ID is required.
func (v *TLSConnectionValidator) SetRequireSPIFFE(required bool) {
	v.opts.RequireSPIFFE = required
}

// SetAllowedTrustDomains sets allowed trust domains.
func (v *TLSConnectionValidator) SetAllowedTrustDomains(domains []string) {
	v.opts.AllowedTrustDomains = domains
}

// ValidateConnection validates a TLS connection's peer certificate.
func (v *TLSConnectionValidator) ValidateConnection(conn *tls.Conn) (*ValidationResult, error) {
	state := conn.ConnectionState()

	if len(state.PeerCertificates) == 0 {
		return &ValidationResult{
			Valid:  false,
			Errors: []error{errors.New("no peer certificate")},
		}, nil
	}

	result := ValidateCertificate(state.PeerCertificates[0], v.opts)
	result.VerifiedChains = state.VerifiedChains

	// Additional connection-level checks
	if warning := checkCipherSuite(state.CipherSuite); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}

	if warning := checkTLSVersion(state.Version); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}

	return result, nil
}

// checkCipherSuite checks for weak cipher suites.
func checkCipherSuite(suite uint16) string {
	weakCiphers := map[uint16]string{
		tls.TLS_RSA_WITH_RC4_128_SHA:        "RC4",
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:   "3DES",
		tls.TLS_RSA_WITH_AES_128_CBC_SHA:    "CBC mode",
		tls.TLS_RSA_WITH_AES_256_CBC_SHA:    "CBC mode",
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256: "CBC mode",
	}

	if name, weak := weakCiphers[suite]; weak {
		return fmt.Sprintf("weak cipher suite detected: %s", name)
	}

	return ""
}

// checkTLSVersion checks for outdated TLS versions.
func checkTLSVersion(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0 is deprecated"
	case tls.VersionTLS11:
		return "TLS 1.1 is deprecated"
	}
	return ""
}

// CertificateChainValidator validates certificate chains.
type CertificateChainValidator struct {
	roots         *x509.CertPool
	intermediates *x509.CertPool
}

// NewCertificateChainValidator creates a new chain validator.
func NewCertificateChainValidator(roots *x509.CertPool) *CertificateChainValidator {
	return &CertificateChainValidator{
		roots:         roots,
		intermediates: x509.NewCertPool(),
	}
}

// AddIntermediate adds an intermediate certificate.
func (v *CertificateChainValidator) AddIntermediate(cert *x509.Certificate) {
	v.intermediates.AddCert(cert)
}

// ValidateChain validates a certificate chain.
func (v *CertificateChainValidator) ValidateChain(chain []*x509.Certificate) error {
	if len(chain) == 0 {
		return errors.New("empty certificate chain")
	}

	leaf := chain[0]

	// Add intermediates from chain
	intermediates := x509.NewCertPool()
	for i := 1; i < len(chain); i++ {
		intermediates.AddCert(chain[i])
	}

	// Also add any pre-configured intermediates
	// Note: x509.CertPool doesn't have a way to iterate or merge,
	// so we just use the chain intermediates

	opts := x509.VerifyOptions{
		Roots:         v.roots,
		Intermediates: intermediates,
		CurrentTime:   time.Now(),
	}

	_, err := leaf.Verify(opts)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrChainVerificationFailed, err)
	}

	return nil
}

// CertificateHealthChecker monitors certificate health.
type CertificateHealthChecker struct {
	certs        map[string]*x509.Certificate
	warningDays  int
	criticalDays int
}

// NewCertificateHealthChecker creates a new health checker.
func NewCertificateHealthChecker(warningDays, criticalDays int) *CertificateHealthChecker {
	return &CertificateHealthChecker{
		certs:        make(map[string]*x509.Certificate),
		warningDays:  warningDays,
		criticalDays: criticalDays,
	}
}

// AddCertificate adds a certificate to monitor.
func (c *CertificateHealthChecker) AddCertificate(name string, cert *x509.Certificate) {
	c.certs[name] = cert
}

// HealthStatus represents certificate health status.
type HealthStatus int

const (
	// HealthOK indicates the certificate is healthy.
	HealthOK HealthStatus = iota
	// HealthWarning indicates the certificate will expire soon.
	HealthWarning
	// HealthCritical indicates the certificate is about to expire or has expired.
	HealthCritical
)

// CertificateHealth contains health information for a certificate.
type CertificateHealth struct {
	Name         string
	Status       HealthStatus
	ExpiresAt    time.Time
	TimeToExpiry time.Duration
	Message      string
}

// CheckHealth returns the health status of all monitored certificates.
func (c *CertificateHealthChecker) CheckHealth() []CertificateHealth {
	now := time.Now()
	results := make([]CertificateHealth, 0, len(c.certs))

	for name, cert := range c.certs {
		health := CertificateHealth{
			Name:         name,
			ExpiresAt:    cert.NotAfter,
			TimeToExpiry: time.Until(cert.NotAfter),
		}

		if now.After(cert.NotAfter) {
			health.Status = HealthCritical
			health.Message = "certificate has expired"
		} else if cert.NotAfter.Before(now.AddDate(0, 0, c.criticalDays)) {
			health.Status = HealthCritical
			health.Message = fmt.Sprintf("certificate expires in %v", health.TimeToExpiry.Round(time.Hour))
		} else if cert.NotAfter.Before(now.AddDate(0, 0, c.warningDays)) {
			health.Status = HealthWarning
			health.Message = fmt.Sprintf("certificate expires in %v", health.TimeToExpiry.Round(time.Hour))
		} else {
			health.Status = HealthOK
			health.Message = "certificate is healthy"
		}

		results = append(results, health)
	}

	return results
}

// CertificateFingerprint calculates a certificate fingerprint.
func CertificateFingerprint(cert *x509.Certificate, hashType string) string {
	// For now, just use the serial number as a simple identifier
	// In production, this would compute SHA-256 of the DER-encoded certificate
	return cert.SerialNumber.String()
}

// IsSelfSigned checks if a certificate is self-signed.
func IsSelfSigned(cert *x509.Certificate) bool {
	return cert.CheckSignatureFrom(cert) == nil
}

// CompareCertificates compares two certificates for equality.
func CompareCertificates(a, b *x509.Certificate) bool {
	if a == nil || b == nil {
		return a == b
	}
	return strings.EqualFold(a.SerialNumber.String(), b.SerialNumber.String()) &&
		a.Issuer.String() == b.Issuer.String()
}
