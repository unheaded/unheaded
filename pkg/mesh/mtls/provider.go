// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package mtls provides mTLS certificate management integrated with the centralized cert system.
package mtls

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/certs"
	"unheaded/pkg/certs/ca"
)

var (
	// ErrProviderNotInitialized indicates the provider is not initialized.
	ErrProviderNotInitialized = errors.New("mTLS provider not initialized")
	// ErrCertificateLoadFailed indicates certificate loading failed.
	ErrCertificateLoadFailed = errors.New("certificate loading failed")
	// ErrInvalidCertificate indicates the certificate is invalid.
	ErrInvalidCertificate = errors.New("invalid certificate")
	// ErrRotationFailed indicates certificate rotation failed.
	ErrRotationFailed = errors.New("certificate rotation failed")
)

// ProviderConfig configures the mTLS provider.
type ProviderConfig struct {
	// Identity
	TrustDomain string
	ServiceName string
	Namespace   string

	// Certificate paths (for file-based loading)
	CertFile string
	KeyFile  string
	CAFile   string
	CertDir  string

	// Certificate settings
	CertValidity   time.Duration
	RotationBuffer time.Duration
	AutoRotate     bool

	// TLS settings
	MinVersion   uint16
	MaxVersion   uint16
	CipherSuites []uint16
	VerifyClient bool
	SkipVerify   bool

	// Integration with centralized cert manager
	UseCertManager bool
	CertManager    certs.CertManager
}

// DefaultProviderConfig returns a default provider configuration.
func DefaultProviderConfig(trustDomain, serviceName, namespace string) *ProviderConfig {
	return &ProviderConfig{
		TrustDomain:    trustDomain,
		ServiceName:    serviceName,
		Namespace:      namespace,
		CertValidity:   DefaultCertValidity,
		RotationBuffer: DefaultRotationBuffer,
		AutoRotate:     true,
		MinVersion:     tls.VersionTLS12,
		MaxVersion:     tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		VerifyClient: true,
	}
}

// Provider provides mTLS configuration for the service mesh.
// It integrates with the centralized certificate management system.
type Provider struct {
	config *ProviderConfig

	// Certificate state
	cert      *tls.Certificate
	rootCAs   *x509.CertPool
	clientCAs *x509.CertPool
	spiffeID  *SPIFFEID
	verifier  *SPIFFEVerifier
	bundle    *SPIFFEBundle

	// Cached TLS configs
	serverConfig *tls.Config
	clientConfig *tls.Config

	// Integration with centralized cert manager
	certMgr certs.CertManager
	localCA *ca.Authority

	// Rotation state
	certSerial    string
	rotationTimer *time.Timer
	version       uint64

	// Callbacks
	onRotation []func(*tls.Certificate)
	onError    []func(error)

	// State
	mu          sync.RWMutex
	initialized bool
	closed      bool
	closeCh     chan struct{}
}

// NewProvider creates a new mTLS provider.
func NewProvider(config *ProviderConfig) *Provider {
	if config == nil {
		config = DefaultProviderConfig(DefaultTrustDomain, "unknown", "default")
	}

	verifier := NewSPIFFEVerifier([]string{config.TrustDomain})

	return &Provider{
		config:     config,
		verifier:   verifier,
		bundle:     NewSPIFFEBundle(config.TrustDomain),
		onRotation: make([]func(*tls.Certificate), 0),
		onError:    make([]func(error), 0),
		closeCh:    make(chan struct{}),
	}
}

// InitializeWithFiles initializes the provider from certificate files.
func (p *Provider) InitializeWithFiles(certFile, keyFile, caFile string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	// Load certificate and key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCertificateLoadFailed, err)
	}

	// Parse the certificate
	if len(cert.Certificate) > 0 {
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidCertificate, err)
		}
		cert.Leaf = x509Cert

		// Extract SPIFFE ID
		if id, err := p.verifier.ExtractSPIFFEID(x509Cert); err == nil {
			p.spiffeID = id
		}
	}

	p.cert = &cert

	// Load CA certificate
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCertificateLoadFailed, err)
	}

	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("%w: failed to parse CA certificate", ErrInvalidCertificate)
	}
	p.rootCAs = rootCAs
	p.clientCAs = rootCAs

	// Add CA to bundle
	block, _ := pem.Decode(caPEM)
	if block != nil {
		if caCert, err := x509.ParseCertificate(block.Bytes); err == nil {
			p.bundle.AddRoot(caCert)
		}
	}

	// Build TLS configs
	p.buildTLSConfigs()

	// Set up file watching for rotation if paths provided
	if p.config.AutoRotate {
		go p.watchCertificateFiles(certFile, keyFile, caFile)
	}

	p.initialized = true
	return nil
}

// InitializeWithCA initializes the provider with a CA for self-signed certs.
func (p *Provider) InitializeWithCA(caCert *x509.Certificate, caKey crypto.Signer) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	// Create local CA authority
	caConfig := &ca.Config{
		CommonName:   p.config.TrustDomain + " CA",
		Organization: "Unheaded Kingdom",
		Duration:     10 * 365 * 24 * time.Hour,
	}

	if caCert != nil && caKey != nil {
		// Encode existing CA
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
		keyDER, err := x509.MarshalPKCS8PrivateKey(caKey)
		if err != nil {
			return fmt.Errorf("failed to marshal CA key: %w", err)
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		caConfig.RootCertPEM = certPEM
		caConfig.RootKeyPEM = keyPEM
	}

	authority, err := ca.New(caConfig)
	if err != nil {
		return fmt.Errorf("failed to create CA: %w", err)
	}
	p.localCA = authority

	// Set up root CAs
	rootCert, rootPEM := authority.GetRootCertificate()
	p.rootCAs = x509.NewCertPool()
	p.rootCAs.AddCert(rootCert)
	p.clientCAs = p.rootCAs

	// Add to bundle
	p.bundle.AddRoot(rootCert)

	// Also add intermediate
	intermCert, _ := authority.GetCertificate()
	if intermCert != nil {
		p.rootCAs.AddCert(intermCert)
		p.bundle.AddRoot(intermCert)
	}

	// Generate service certificate
	if err := p.generateServiceCertificate(); err != nil {
		return err
	}

	// Build TLS configs
	p.buildTLSConfigs()

	// Schedule rotation if enabled
	if p.config.AutoRotate {
		p.scheduleRotation()
	}

	p.initialized = true
	_ = rootPEM // suppress unused warning
	return nil
}

// InitializeWithCertManager initializes using the centralized cert manager.
func (p *Provider) InitializeWithCertManager(certMgr certs.CertManager) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	p.certMgr = certMgr
	p.config.UseCertManager = true

	ctx := context.Background()

	// Get CA certificate
	caCert, err := certMgr.GetCACertificate(ctx)
	if err != nil {
		return fmt.Errorf("failed to get CA certificate: %w", err)
	}

	// Set up root CAs
	p.rootCAs = x509.NewCertPool()
	p.rootCAs.AddCert(caCert.Certificate)
	p.clientCAs = p.rootCAs
	p.bundle.AddRoot(caCert.Certificate)

	// Issue service certificate
	spiffeID := NewSPIFFEIDFromNamespace(p.config.TrustDomain, p.config.Namespace, p.config.ServiceName)
	req := &certs.CertRequest{
		CommonName: p.config.ServiceName,
		Type:       certs.CertTypeMutual,
		Duration:   p.config.CertValidity,
		SPIFFEID:   spiffeID.String(),
	}

	cert, err := certMgr.IssueCertificate(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to issue service certificate: %w", err)
	}

	// Load the TLS certificate
	tlsCert, err := tls.X509KeyPair(cert.CertificatePEM, cert.PrivateKeyPEM)
	if err != nil {
		return fmt.Errorf("failed to load certificate: %w", err)
	}
	tlsCert.Leaf = cert.Certificate
	p.cert = &tlsCert
	p.certSerial = cert.Serial
	p.spiffeID = spiffeID

	// Build TLS configs
	p.buildTLSConfigs()

	// Start watching for cert expiration
	if p.config.AutoRotate {
		go p.watchCertManagerExpiration(ctx)
	}

	p.initialized = true
	return nil
}

// generateServiceCertificate generates a new service certificate.
func (p *Provider) generateServiceCertificate() error {
	if p.localCA == nil {
		return errors.New("no CA available")
	}

	spiffeID := NewSPIFFEIDFromNamespace(p.config.TrustDomain, p.config.Namespace, p.config.ServiceName)
	p.spiffeID = spiffeID

	certConfig := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: p.config.ServiceName,
		Validity:   p.config.CertValidity,
		KeyType:    "ecdsa-p256",
	}

	signingCert, _ := p.localCA.GetCertificate()
	signingKey := p.localCA.GetSigningKey()
	signer, ok := signingKey.(crypto.Signer)
	if !ok {
		return fmt.Errorf("local CA signing key does not implement crypto.Signer (got %T)", signingKey)
	}
	certMgr := NewCertificateManager(signingCert, signer)

	cert, key, err := certMgr.GenerateCertificate(certConfig)
	if err != nil {
		return fmt.Errorf("failed to generate certificate: %w", err)
	}

	// Create TLS certificate
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("failed to create TLS certificate: %w", err)
	}
	tlsCert.Leaf = cert
	p.cert = &tlsCert

	return nil
}

// buildTLSConfigs builds the server and client TLS configurations.
func (p *Provider) buildTLSConfigs() {
	// Server config for accepting mTLS connections
	p.serverConfig = &tls.Config{
		MinVersion:   p.config.MinVersion,
		MaxVersion:   p.config.MaxVersion,
		CipherSuites: p.config.CipherSuites,
		ClientCAs:    p.clientCAs,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			p.mu.RLock()
			defer p.mu.RUnlock()
			return p.cert, nil
		},
		VerifyPeerCertificate: p.verifyPeerCertificate,
		// SessionTicketsDisabled prevents resumed sessions from bypassing
		// the VerifyPeerCertificate callback (gosec G123 / CWE-295).
		SessionTicketsDisabled: true,
	}

	if p.config.VerifyClient {
		p.serverConfig.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		p.serverConfig.ClientAuth = tls.NoClientCert
	}

	// Client config for outbound mTLS connections.
	p.clientConfig = &tls.Config{
		MinVersion:   p.config.MinVersion,
		MaxVersion:   p.config.MaxVersion,
		CipherSuites: p.config.CipherSuites,
		RootCAs:      p.rootCAs,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			p.mu.RLock()
			defer p.mu.RUnlock()
			return p.cert, nil
		},
		VerifyPeerCertificate:  p.verifyPeerCertificate,
		InsecureSkipVerify:     p.config.SkipVerify, //nolint:gosec // gated behind opt-in config field
		SessionTicketsDisabled: true,
	}
}

// verifyPeerCertificate implements custom peer verification.
func (p *Provider) verifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	return p.verifier.VerifyPeerCertificate(rawCerts, verifiedChains)
}

// ServerTLSConfig returns the TLS configuration for servers.
func (p *Provider) ServerTLSConfig() *tls.Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.serverConfig == nil {
		return nil
	}
	return p.serverConfig.Clone()
}

// ClientTLSConfig returns the TLS configuration for clients.
func (p *Provider) ClientTLSConfig() *tls.Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.clientConfig == nil {
		return nil
	}
	return p.clientConfig.Clone()
}

// SPIFFEID returns the provider's SPIFFE identity.
func (p *Provider) SPIFFEID() *SPIFFEID {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.spiffeID
}

// RootCAs returns the root CA certificate pool.
func (p *Provider) RootCAs() *x509.CertPool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.rootCAs
}

// Certificate returns the current certificate.
func (p *Provider) Certificate() *tls.Certificate {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cert
}

// CertificateVersion returns the current certificate version (incremented on rotation).
func (p *Provider) CertificateVersion() uint64 {
	return atomic.LoadUint64(&p.version)
}

// OnRotation registers a callback for certificate rotation.
func (p *Provider) OnRotation(fn func(*tls.Certificate)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onRotation = append(p.onRotation, fn)
}

// OnError registers a callback for errors.
func (p *Provider) OnError(fn func(error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onError = append(p.onError, fn)
}

// Rotate forces a certificate rotation.
func (p *Provider) Rotate() error {
	p.mu.Lock()
	if p.closed || !p.initialized {
		p.mu.Unlock()
		return ErrProviderNotInitialized
	}
	p.mu.Unlock()

	return p.rotateCertificate()
}

// rotateCertificate performs certificate rotation.
func (p *Provider) rotateCertificate() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var newCert *tls.Certificate

	if p.config.UseCertManager && p.certMgr != nil {
		// Use centralized cert manager
		ctx := context.Background()
		cert, err := p.certMgr.RenewCertificate(ctx, p.certSerial)
		if err != nil {
			p.notifyError(fmt.Errorf("%w: %v", ErrRotationFailed, err))
			return err
		}

		tlsCert, err := tls.X509KeyPair(cert.CertificatePEM, cert.PrivateKeyPEM)
		if err != nil {
			p.notifyError(err)
			return err
		}
		tlsCert.Leaf = cert.Certificate
		newCert = &tlsCert
		p.certSerial = cert.Serial
	} else if p.localCA != nil {
		// Generate new certificate using local CA
		if err := p.generateServiceCertificate(); err != nil {
			p.notifyError(err)
			return err
		}
		newCert = p.cert
	} else {
		return errors.New("no certificate source available for rotation")
	}

	p.cert = newCert
	atomic.AddUint64(&p.version, 1)

	// Rebuild TLS configs
	p.buildTLSConfigs()

	// Schedule next rotation
	p.scheduleRotation()

	// Notify callbacks
	p.notifyRotation(newCert)

	return nil
}

// scheduleRotation schedules the next certificate rotation.
func (p *Provider) scheduleRotation() {
	if p.rotationTimer != nil {
		p.rotationTimer.Stop()
	}

	if p.cert == nil || p.cert.Leaf == nil {
		return
	}

	rotateAt := p.cert.Leaf.NotAfter.Add(-p.config.RotationBuffer)
	duration := time.Until(rotateAt)
	if duration < 0 {
		duration = time.Minute // Rotate soon
	}

	p.rotationTimer = time.AfterFunc(duration, func() {
		if err := p.rotateCertificate(); err != nil {
			p.notifyError(err)
		}
	})
}

// watchCertificateFiles watches certificate files for changes.
func (p *Provider) watchCertificateFiles(certFile, keyFile, caFile string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var lastMod time.Time

	for {
		select {
		case <-p.closeCh:
			return
		case <-ticker.C:
			info, err := os.Stat(certFile)
			if err != nil {
				continue
			}

			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()

				// Reload certificates
				cert, err := tls.LoadX509KeyPair(certFile, keyFile)
				if err != nil {
					p.notifyError(fmt.Errorf("failed to reload certificate: %w", err))
					continue
				}

				if len(cert.Certificate) > 0 {
					x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
					if err == nil {
						cert.Leaf = x509Cert
					}
				}

				p.mu.Lock()
				p.cert = &cert
				atomic.AddUint64(&p.version, 1)
				p.buildTLSConfigs()
				p.mu.Unlock()

				p.notifyRotation(&cert)
			}
		}
	}
}

// watchCertManagerExpiration watches for certificate expiration when using cert manager.
func (p *Provider) watchCertManagerExpiration(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.closeCh:
			return
		case <-ticker.C:
			p.mu.RLock()
			cert := p.cert
			p.mu.RUnlock()

			if cert == nil || cert.Leaf == nil {
				continue
			}

			// Check if we need to rotate
			rotateAt := cert.Leaf.NotAfter.Add(-p.config.RotationBuffer)
			if time.Now().After(rotateAt) {
				if err := p.Rotate(); err != nil {
					p.notifyError(err)
				}
			}
		}
	}
}

// notifyRotation notifies rotation callbacks.
func (p *Provider) notifyRotation(cert *tls.Certificate) {
	for _, fn := range p.onRotation {
		go fn(cert)
	}
}

// notifyError notifies error callbacks.
func (p *Provider) notifyError(err error) {
	for _, fn := range p.onError {
		go fn(err)
	}
}

// SaveToFiles saves the current certificates to files.
func (p *Provider) SaveToFiles(certDir string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.cert == nil {
		return ErrProviderNotInitialized
	}

	// Create directory if needed
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return err
	}

	// Save certificate
	certPath := filepath.Join(certDir, "cert.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: p.cert.Certificate[0]})
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return err
	}

	// Save key (if available)
	if p.cert.PrivateKey != nil {
		keyPath := filepath.Join(certDir, "key.pem")
		keyDER, err := x509.MarshalPKCS8PrivateKey(p.cert.PrivateKey)
		if err != nil {
			return fmt.Errorf("failed to marshal private key: %w", err)
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
			return err
		}
	}

	// Save CA bundle
	if len(p.bundle.Roots) > 0 {
		caPath := filepath.Join(certDir, "ca.pem")
		var caPEM []byte
		for _, cert := range p.bundle.Roots {
			caPEM = append(caPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})...)
		}
		if err := os.WriteFile(caPath, caPEM, 0600); err != nil {
			return err
		}
	}

	return nil
}

// VerifyPeerIdentity verifies a peer's SPIFFE ID from a connection.
func (p *Provider) VerifyPeerIdentity(conn *tls.Conn, allowedPatterns []string) error {
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return ErrPeerNotVerified
	}

	peerID, err := p.verifier.ExtractSPIFFEID(state.PeerCertificates[0])
	if err != nil {
		return err
	}

	if len(allowedPatterns) == 0 {
		return nil // No restrictions
	}

	for _, pattern := range allowedPatterns {
		if peerID.Matches(pattern) {
			return nil
		}
	}

	return ErrInvalidSPIFFEID
}

// Close shuts down the provider.
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	close(p.closeCh)

	if p.rotationTimer != nil {
		p.rotationTimer.Stop()
	}

	return nil
}

// Initialized returns whether the provider is initialized.
func (p *Provider) Initialized() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.initialized
}
