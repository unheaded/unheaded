// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package mtls provides mutual TLS configuration for gRPC and HTTP services.
//
// It handles:
//   - Root CA generation and loading
//   - Service certificate generation (signed by root CA)
//   - TLS config for gRPC servers (requiring client certs)
//   - TLS config for gRPC clients (presenting client certs)
//   - Certificate rotation with zero downtime
package mtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config holds the mTLS configuration for a service.
type Config struct {
	// PKIDir is the directory containing certificates.
	// Default: /etc/unheaded/pki
	PKIDir string

	// ServiceName is the name of this service (used for cert CN and file names).
	ServiceName string

	// RootCAFile overrides the root CA cert path (default: <PKIDir>/root-ca.crt).
	RootCAFile string

	// CertFile overrides the service cert path (default: <PKIDir>/<service>.crt).
	CertFile string

	// KeyFile overrides the service key path (default: <PKIDir>/<service>.key).
	KeyFile string
}

func (c *Config) rootCAPath() string {
	if c.RootCAFile != "" {
		return c.RootCAFile
	}
	return filepath.Join(c.pkiDir(), "root-ca.crt")
}

func (c *Config) certPath() string {
	if c.CertFile != "" {
		return c.CertFile
	}
	return filepath.Join(c.pkiDir(), c.ServiceName+".crt")
}

func (c *Config) keyPath() string {
	if c.KeyFile != "" {
		return c.KeyFile
	}
	return filepath.Join(c.pkiDir(), c.ServiceName+".key")
}

func (c *Config) pkiDir() string {
	if c.PKIDir != "" {
		return c.PKIDir
	}
	return "/etc/unheaded/pki"
}

// ServerTLSConfig returns a tls.Config for a gRPC/HTTP server requiring mTLS.
// The server presents its own certificate and requires clients to present valid certs.
func ServerTLSConfig(cfg Config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.certPath(), cfg.keyPath())
	if err != nil {
		return nil, fmt.Errorf("mtls: load server cert: %w", err)
	}

	rootCA, err := loadCACertPool(cfg.rootCAPath())
	if err != nil {
		return nil, fmt.Errorf("mtls: load root CA: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    rootCA,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// ClientTLSConfig returns a tls.Config for a gRPC/HTTP client using mTLS.
// The client presents its own certificate and verifies the server's cert.
func ClientTLSConfig(cfg Config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.certPath(), cfg.keyPath())
	if err != nil {
		return nil, fmt.Errorf("mtls: load client cert: %w", err)
	}

	rootCA, err := loadCACertPool(cfg.rootCAPath())
	if err != nil {
		return nil, fmt.Errorf("mtls: load root CA: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      rootCA,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func loadCACertPool(caPath string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("mtls: failed to parse CA certificate")
	}
	return pool, nil
}

// GenerateRootCA generates a self-signed root CA certificate and key.
// The CA is valid for the specified duration and uses ECDSA P-256.
func GenerateRootCA(validity time.Duration) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Unheaded"},
			CommonName:   "Unheaded Root CA",
			Country:      []string{"US"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(validity),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: create CA cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: marshal CA key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

// GenerateServiceCert generates a service certificate signed by the given CA.
// The cert includes SANs for localhost, 127.0.0.1, and <service>.services.internal.
func GenerateServiceCert(caCertPEM, caKeyPEM []byte, serviceName string, validity time.Duration) (certPEM, keyPEM []byte, err error) {
	// Parse CA cert.
	caBlock, _ := pem.Decode(caCertPEM)
	if caBlock == nil {
		return nil, nil, errors.New("mtls: failed to decode CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: parse CA cert: %w", err)
	}

	// Parse CA key.
	keyBlock, _ := pem.Decode(caKeyPEM)
	if keyBlock == nil {
		return nil, nil, errors.New("mtls: failed to decode CA key PEM")
	}
	caKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: parse CA key: %w", err)
	}

	// Generate service key.
	svcKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: generate service key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Unheaded"},
			CommonName:   serviceName + ".services.internal",
			Country:      []string{"US"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(validity),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		DNSNames:    []string{serviceName + ".services.internal", "localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &svcKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: create service cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	svcKeyDER, err := x509.MarshalECPrivateKey(svcKey)
	if err != nil {
		return nil, nil, fmt.Errorf("mtls: marshal service key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: svcKeyDER})

	return certPEM, keyPEM, nil
}

// WritePKI writes CA and service certificates to the PKI directory.
func WritePKI(pkiDir string, caCertPEM, caKeyPEM []byte, services []string, validity time.Duration) error {
	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		return fmt.Errorf("mtls: create pki dir: %w", err)
	}

	// Write root CA.
	if err := os.WriteFile(filepath.Join(pkiDir, "root-ca.crt"), caCertPEM, 0644); err != nil {
		return fmt.Errorf("mtls: write CA cert: %w", err)
	}
	if err := os.WriteFile(filepath.Join(pkiDir, "root-ca.key"), caKeyPEM, 0600); err != nil {
		return fmt.Errorf("mtls: write CA key: %w", err)
	}

	// Generate and write per-service certs.
	for _, svc := range services {
		certPEM, keyPEM, err := GenerateServiceCert(caCertPEM, caKeyPEM, svc, validity)
		if err != nil {
			return fmt.Errorf("mtls: generate cert for %s: %w", svc, err)
		}
		if err := os.WriteFile(filepath.Join(pkiDir, svc+".crt"), certPEM, 0644); err != nil {
			return fmt.Errorf("mtls: write cert for %s: %w", svc, err)
		}
		if err := os.WriteFile(filepath.Join(pkiDir, svc+".key"), keyPEM, 0600); err != nil {
			return fmt.Errorf("mtls: write key for %s: %w", svc, err)
		}
	}

	return nil
}

// CertRotator monitors certificate expiry and triggers rotation.
type CertRotator struct {
	mu          sync.RWMutex
	cfg         Config
	currentCert *tls.Certificate
	caPool      *x509.CertPool
	stopCh      chan struct{}
}

// NewCertRotator creates a new certificate rotator that reloads certs periodically.
func NewCertRotator(cfg Config) (*CertRotator, error) {
	cr := &CertRotator{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
	if err := cr.reload(); err != nil {
		return nil, err
	}
	return cr, nil
}

// GetCertificate returns the current certificate (for tls.Config.GetCertificate).
func (cr *CertRotator) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.currentCert, nil
}

// GetClientCertificate returns the current cert for client connections.
func (cr *CertRotator) GetClientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.currentCert, nil
}

// CAPool returns the current CA cert pool.
func (cr *CertRotator) CAPool() *x509.CertPool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.caPool
}

// Start begins periodic certificate reloading.
func (cr *CertRotator) Start(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = cr.reload()
			case <-cr.stopCh:
				return
			}
		}
	}()
}

// Stop halts certificate rotation.
func (cr *CertRotator) Stop() {
	close(cr.stopCh)
}

func (cr *CertRotator) reload() error {
	cert, err := tls.LoadX509KeyPair(cr.cfg.certPath(), cr.cfg.keyPath())
	if err != nil {
		return err
	}

	pool, err := loadCACertPool(cr.cfg.rootCAPath())
	if err != nil {
		return err
	}

	cr.mu.Lock()
	cr.currentCert = &cert
	cr.caPool = pool
	cr.mu.Unlock()
	return nil
}

// RotatingServerTLSConfig returns a TLS config that auto-rotates certificates.
func RotatingServerTLSConfig(rotator *CertRotator) *tls.Config {
	return &tls.Config{
		GetCertificate: rotator.GetCertificate,
		ClientAuth:     tls.RequireAndVerifyClientCert,
		ClientCAs:      rotator.CAPool(),
		MinVersion:     tls.VersionTLS12,
	}
}

// RotatingClientTLSConfig returns a TLS config that auto-rotates certificates.
func RotatingClientTLSConfig(rotator *CertRotator) *tls.Config {
	return &tls.Config{
		GetClientCertificate: rotator.GetClientCertificate,
		RootCAs:              rotator.CAPool(),
		MinVersion:           tls.VersionTLS12,
	}
}
