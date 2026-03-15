// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package mtls

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/url"
	"sync"
	"time"
)

const (
	// DefaultCertValidity is the default certificate validity period.
	DefaultCertValidity = 1 * time.Hour
	// DefaultRotationBuffer is when to start rotation before expiry.
	DefaultRotationBuffer = 10 * time.Minute
)

var (
	// ErrCertExpired indicates the certificate has expired.
	ErrCertExpired = errors.New("certificate expired")
	// ErrCertNotYetValid indicates the certificate is not yet valid.
	ErrCertNotYetValid = errors.New("certificate not yet valid")
	// ErrNoPrivateKey indicates no private key is available.
	ErrNoPrivateKey = errors.New("no private key available")
)

// CertificateConfig holds certificate generation configuration.
type CertificateConfig struct {
	// Identity
	SPIFFEID    *SPIFFEID
	CommonName  string
	DNSNames    []string
	IPAddresses []net.IP

	// Validity
	NotBefore time.Time
	NotAfter  time.Time
	Validity  time.Duration

	// Key type
	KeyType string // "ecdsa-p256", "ecdsa-p384"

	// CA settings (for CA certs)
	IsCA       bool
	MaxPathLen int
}

// DefaultCertificateConfig returns a default certificate configuration.
func DefaultCertificateConfig(spiffeID *SPIFFEID) *CertificateConfig {
	return &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: spiffeID.ServiceName(),
		NotBefore:  time.Now().Add(-5 * time.Minute), // Clock skew buffer
		Validity:   DefaultCertValidity,
		KeyType:    "ecdsa-p256",
		IsCA:       false,
	}
}

// CertificateManager manages certificate lifecycle.
type CertificateManager struct {
	mu sync.RWMutex

	// Current certificate and key
	cert    *x509.Certificate
	key     crypto.Signer
	certPEM []byte
	keyPEM  []byte

	// CA for signing
	caCert *x509.Certificate
	caKey  crypto.Signer

	// Configuration
	config         *CertificateConfig
	rotationBuffer time.Duration

	// Rotation
	rotationTimer *time.Timer
	onRotation    func(cert *x509.Certificate, key crypto.Signer)

	// State
	closed bool
}

// NewCertificateManager creates a new certificate manager.
func NewCertificateManager(caCert *x509.Certificate, caKey crypto.Signer) *CertificateManager {
	return &CertificateManager{
		caCert:         caCert,
		caKey:          caKey,
		rotationBuffer: DefaultRotationBuffer,
	}
}

// GenerateCertificate generates a new certificate based on the config.
func (cm *CertificateManager) GenerateCertificate(config *CertificateConfig) (*x509.Certificate, crypto.Signer, error) {
	// Generate key pair
	key, err := cm.generateKey(config.KeyType)
	if err != nil {
		return nil, nil, err
	}

	// Build certificate template
	template := cm.buildTemplate(config)

	// Use CA or self-sign
	var parent *x509.Certificate
	var signingKey crypto.Signer
	if cm.caCert != nil && cm.caKey != nil {
		parent = cm.caCert
		signingKey = cm.caKey
	} else {
		parent = template
		signingKey = key
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, parent, key.Public(), signingKey)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

// SetCertificate sets the current certificate.
func (cm *CertificateManager) SetCertificate(cert *x509.Certificate, key crypto.Signer, config *CertificateConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.cert = cert
	cm.key = key
	cm.config = config

	// Encode PEM
	cm.certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})

	keyDER, err := x509.MarshalECPrivateKey(key.(*ecdsa.PrivateKey))
	if err != nil {
		return err
	}
	cm.keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	// Schedule rotation
	cm.scheduleRotation()

	return nil
}

// GetCertificate returns the current certificate.
func (cm *CertificateManager) GetCertificate() (*x509.Certificate, crypto.Signer, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.cert == nil {
		return nil, nil, ErrNoPrivateKey
	}

	// Check validity
	now := time.Now()
	if now.Before(cm.cert.NotBefore) {
		return nil, nil, ErrCertNotYetValid
	}
	if now.After(cm.cert.NotAfter) {
		return nil, nil, ErrCertExpired
	}

	return cm.cert, cm.key, nil
}

// GetCertificatePEM returns the certificate and key in PEM format.
func (cm *CertificateManager) GetCertificatePEM() (certPEM, keyPEM []byte, err error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.certPEM == nil || cm.keyPEM == nil {
		return nil, nil, ErrNoPrivateKey
	}

	return cm.certPEM, cm.keyPEM, nil
}

// NeedsRotation checks if the certificate needs rotation.
func (cm *CertificateManager) NeedsRotation() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.cert == nil {
		return true
	}

	rotateAt := cm.cert.NotAfter.Add(-cm.rotationBuffer)
	return time.Now().After(rotateAt)
}

// Rotate rotates the certificate.
func (cm *CertificateManager) Rotate() error {
	cm.mu.Lock()
	config := cm.config
	callback := cm.onRotation
	cm.mu.Unlock()

	if config == nil {
		return errors.New("no certificate configuration")
	}

	// Update validity window
	config.NotBefore = time.Now().Add(-5 * time.Minute)
	config.NotAfter = time.Time{} // Will be set based on Validity

	cert, key, err := cm.GenerateCertificate(config)
	if err != nil {
		return err
	}

	if err := cm.SetCertificate(cert, key, config); err != nil {
		return err
	}

	if callback != nil {
		callback(cert, key)
	}

	return nil
}

// OnRotation sets the callback for certificate rotation.
func (cm *CertificateManager) OnRotation(fn func(cert *x509.Certificate, key crypto.Signer)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.onRotation = fn
}

// Close stops the certificate manager.
func (cm *CertificateManager) Close() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.closed = true
	if cm.rotationTimer != nil {
		cm.rotationTimer.Stop()
	}
}

func (cm *CertificateManager) generateKey(keyType string) (crypto.Signer, error) {
	var curve elliptic.Curve

	switch keyType {
	case "ecdsa-p384":
		curve = elliptic.P384()
	case "ecdsa-p256", "":
		curve = elliptic.P256()
	default:
		curve = elliptic.P256()
	}

	return ecdsa.GenerateKey(curve, rand.Reader)
}

func (cm *CertificateManager) buildTemplate(config *CertificateConfig) *x509.Certificate {
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	notBefore := config.NotBefore
	if notBefore.IsZero() {
		notBefore = time.Now().Add(-5 * time.Minute)
	}

	notAfter := config.NotAfter
	if notAfter.IsZero() {
		validity := config.Validity
		if validity == 0 {
			validity = DefaultCertValidity
		}
		notAfter = notBefore.Add(validity)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: config.CommonName,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  config.IsCA,
		DNSNames:              config.DNSNames,
		IPAddresses:           config.IPAddresses,
	}

	if config.IsCA {
		template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		template.MaxPathLen = config.MaxPathLen
		template.MaxPathLenZero = config.MaxPathLen == 0
	}

	// Add SPIFFE ID as URI SAN
	if config.SPIFFEID != nil {
		u, _ := url.Parse(config.SPIFFEID.String())
		template.URIs = append(template.URIs, u)
	}

	return template
}

func (cm *CertificateManager) scheduleRotation() {
	if cm.rotationTimer != nil {
		cm.rotationTimer.Stop()
	}

	if cm.cert == nil || cm.closed {
		return
	}

	rotateAt := cm.cert.NotAfter.Add(-cm.rotationBuffer)
	duration := time.Until(rotateAt)
	if duration < 0 {
		duration = time.Minute // Rotate soon
	}

	cm.rotationTimer = time.AfterFunc(duration, func() {
		if err := cm.Rotate(); err != nil {
			// Log error (in production, use proper logging)
		}
	})
}

// GenerateSelfSignedCA generates a self-signed CA certificate.
func GenerateSelfSignedCA(trustDomain string, validity time.Duration) (*x509.Certificate, crypto.Signer, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Unheaded Kingdom"},
			CommonName:   trustDomain + " CA",
		},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(validity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

// CertificateChain represents a certificate chain.
type CertificateChain struct {
	Leaf          *x509.Certificate
	Intermediates []*x509.Certificate
	Key           crypto.Signer
}

// PEM returns the chain in PEM format.
func (c *CertificateChain) PEM() []byte {
	var result []byte

	// Leaf first
	result = append(result, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: c.Leaf.Raw,
	})...)

	// Then intermediates
	for _, cert := range c.Intermediates {
		result = append(result, pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		})...)
	}

	return result
}

// Verify verifies the certificate chain against the given roots.
func (c *CertificateChain) Verify(roots *x509.CertPool) error {
	intermediatePool := x509.NewCertPool()
	for _, cert := range c.Intermediates {
		intermediatePool.AddCert(cert)
	}

	_, err := c.Leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediatePool,
	})
	return err
}
