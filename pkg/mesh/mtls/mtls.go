// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mtls

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"sync"
	"time"
)

var (
	// ErrNoCertificate indicates no certificate is available.
	ErrNoCertificate = errors.New("no certificate available")
	// ErrPeerNotVerified indicates the peer certificate was not verified.
	ErrPeerNotVerified = errors.New("peer certificate not verified")
)

// Config holds mTLS configuration.
type Config struct {
	// Identity
	TrustDomain string
	ServiceName string
	Namespace   string

	// Certificate settings
	CertValidity   time.Duration
	RotationBuffer time.Duration

	// TLS settings
	MinVersion   uint16
	MaxVersion   uint16
	CipherSuites []uint16

	// CA
	RootCAs *x509.CertPool

	// Verification
	SkipVerify bool
}

// DefaultConfig returns a default mTLS configuration.
func DefaultConfig(trustDomain, serviceName string) *Config {
	return &Config{
		TrustDomain:    trustDomain,
		ServiceName:    serviceName,
		Namespace:      "default",
		CertValidity:   DefaultCertValidity,
		RotationBuffer: DefaultRotationBuffer,
		MinVersion:     tls.VersionTLS12,
		MaxVersion:     tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		},
	}
}

// Manager manages mTLS for a service.
type Manager struct {
	mu sync.RWMutex

	config   *Config
	certMgr  *CertificateManager
	verifier *SPIFFEVerifier
	bundle   *SPIFFEBundle

	// TLS configs (cached)
	serverConfig *tls.Config
	clientConfig *tls.Config

	// State
	initialized bool
}

// NewManager creates a new mTLS manager.
func NewManager(config *Config) *Manager {
	verifier := NewSPIFFEVerifier([]string{config.TrustDomain})

	m := &Manager{
		config:   config,
		verifier: verifier,
		bundle:   NewSPIFFEBundle(config.TrustDomain),
	}

	return m
}

// Initialize sets up the mTLS manager with CA credentials.
func (m *Manager) Initialize(caCert *x509.Certificate, caKey crypto.Signer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create certificate manager
	m.certMgr = NewCertificateManager(caCert, caKey)

	// Add CA to bundle
	m.bundle.AddRoot(caCert)

	// Generate initial certificate
	spiffeID := NewSPIFFEIDFromNamespace(m.config.TrustDomain, m.config.Namespace, m.config.ServiceName)
	certConfig := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: m.config.ServiceName,
		Validity:   m.config.CertValidity,
		KeyType:    "ecdsa-p256",
	}

	cert, key, err := m.certMgr.GenerateCertificate(certConfig)
	if err != nil {
		return err
	}

	if err := m.certMgr.SetCertificate(cert, key, certConfig); err != nil {
		return err
	}

	// Set up rotation callback
	m.certMgr.OnRotation(func(cert *x509.Certificate, key crypto.Signer) {
		m.refreshTLSConfigs()
	})

	// Build TLS configs
	m.buildTLSConfigs()

	m.initialized = true
	return nil
}

// InitializeWithBundle sets up mTLS using an existing certificate.
func (m *Manager) InitializeWithBundle(cert *x509.Certificate, key crypto.Signer, rootCAs *x509.CertPool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create certificate manager (without CA, won't auto-rotate)
	m.certMgr = NewCertificateManager(nil, nil)

	// Extract SPIFFE ID from cert
	spiffeID, err := m.verifier.ExtractSPIFFEID(cert)
	if err != nil {
		// Create one from config if not in cert
		spiffeID = NewSPIFFEIDFromNamespace(m.config.TrustDomain, m.config.Namespace, m.config.ServiceName)
	}

	certConfig := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: cert.Subject.CommonName,
		Validity:   cert.NotAfter.Sub(cert.NotBefore),
	}

	if err := m.certMgr.SetCertificate(cert, key, certConfig); err != nil {
		return err
	}

	// Set root CAs
	if rootCAs != nil {
		m.config.RootCAs = rootCAs
	}

	// Build TLS configs
	m.buildTLSConfigs()

	m.initialized = true
	return nil
}

// ServerTLSConfig returns the TLS config for servers.
func (m *Manager) ServerTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serverConfig.Clone()
}

// ClientTLSConfig returns the TLS config for clients.
func (m *Manager) ClientTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clientConfig.Clone()
}

// WrapConn wraps a connection with mTLS as server.
func (m *Manager) WrapConn(conn net.Conn) (*tls.Conn, error) {
	config := m.ServerTLSConfig()
	tlsConn := tls.Server(conn, config)

	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// DialTLS establishes an mTLS connection.
func (m *Manager) DialTLS(network, addr string) (*tls.Conn, error) {
	config := m.ClientTLSConfig()

	conn, err := tls.Dial(network, addr, config)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// DialTLSWithTimeout establishes an mTLS connection with timeout.
func (m *Manager) DialTLSWithTimeout(network, addr string, timeout time.Duration) (*tls.Conn, error) {
	config := m.ClientTLSConfig()

	dialer := &net.Dialer{
		Timeout: timeout,
	}

	conn, err := tls.DialWithDialer(dialer, network, addr, config)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// GetPeerSPIFFEID extracts the SPIFFE ID from a TLS connection.
func (m *Manager) GetPeerSPIFFEID(conn *tls.Conn) (*SPIFFEID, error) {
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, ErrPeerNotVerified
	}

	return m.verifier.ExtractSPIFFEID(state.PeerCertificates[0])
}

// VerifyPeer verifies a peer's SPIFFE ID against allowed patterns.
func (m *Manager) VerifyPeer(conn *tls.Conn, allowedPatterns []string) error {
	id, err := m.GetPeerSPIFFEID(conn)
	if err != nil {
		return err
	}

	if len(allowedPatterns) == 0 {
		return nil // No restrictions
	}

	for _, pattern := range allowedPatterns {
		if id.Matches(pattern) {
			return nil
		}
	}

	return ErrInvalidSPIFFEID
}

// SPIFFEID returns the manager's SPIFFE ID.
func (m *Manager) SPIFFEID() *SPIFFEID {
	return NewSPIFFEIDFromNamespace(m.config.TrustDomain, m.config.Namespace, m.config.ServiceName)
}

// Certificate returns the current certificate.
func (m *Manager) Certificate() (*x509.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.certMgr == nil {
		return nil, ErrNoCertificate
	}

	cert, _, err := m.certMgr.GetCertificate()
	return cert, err
}

// Close shuts down the mTLS manager.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.certMgr != nil {
		m.certMgr.Close()
	}
}

func (m *Manager) buildTLSConfigs() {
	rootPool := m.config.RootCAs
	if rootPool == nil {
		rootPool = m.bundle.CertPool()
	}

	// Server config
	m.serverConfig = &tls.Config{
		MinVersion:   m.config.MinVersion,
		MaxVersion:   m.config.MaxVersion,
		CipherSuites: m.config.CipherSuites,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    rootPool,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return m.getTLSCertificate()
		},
		VerifyPeerCertificate: m.verifier.VerifyPeerCertificate,
	}

	// Client config
	m.clientConfig = &tls.Config{
		MinVersion:   m.config.MinVersion,
		MaxVersion:   m.config.MaxVersion,
		CipherSuites: m.config.CipherSuites,
		RootCAs:      rootPool,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return m.getTLSCertificate()
		},
		VerifyPeerCertificate: m.verifier.VerifyPeerCertificate,
		InsecureSkipVerify:    m.config.SkipVerify,
	}
}

func (m *Manager) getTLSCertificate() (*tls.Certificate, error) {
	certPEM, keyPEM, err := m.certMgr.GetCertificatePEM()
	if err != nil {
		return nil, err
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &cert, nil
}

func (m *Manager) refreshTLSConfigs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buildTLSConfigs()
}

// ConnectionInfo holds information about an mTLS connection.
type ConnectionInfo struct {
	PeerSPIFFEID    *SPIFFEID
	PeerCertificate *x509.Certificate
	TLSVersion      uint16
	CipherSuite     uint16
	ServerName      string
}

// GetConnectionInfo extracts connection info from a TLS connection.
func GetConnectionInfo(conn *tls.Conn) (*ConnectionInfo, error) {
	state := conn.ConnectionState()

	info := &ConnectionInfo{
		TLSVersion:  state.Version,
		CipherSuite: state.CipherSuite,
		ServerName:  state.ServerName,
	}

	if len(state.PeerCertificates) > 0 {
		info.PeerCertificate = state.PeerCertificates[0]

		// Extract SPIFFE ID
		verifier := NewSPIFFEVerifier([]string{"*"})
		if id, err := verifier.ExtractSPIFFEID(state.PeerCertificates[0]); err == nil {
			info.PeerSPIFFEID = id
		}
	}

	return info, nil
}

// TLSVersionString returns a human-readable TLS version string.
func TLSVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return "unknown"
	}
}
