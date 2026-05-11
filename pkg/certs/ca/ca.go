// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package ca provides certificate authority functionality.
package ca

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

var (
	// ErrInvalidCertificate indicates the certificate is invalid.
	ErrInvalidCertificate = errors.New("invalid certificate")
	// ErrCertificateVerificationFailed indicates verification failed.
	ErrCertificateVerificationFailed = errors.New("certificate verification failed")
)

// Config configures the certificate authority.
type Config struct {
	// CommonName is the CA common name.
	CommonName string
	// Organization is the CA organization.
	Organization string
	// Country is the CA country code.
	Country string
	// Duration is how long the CA certificate is valid.
	Duration time.Duration
	// KeySize is the key size in bits (for RSA) or curve (for ECDSA).
	KeySize int
	// RootCertPEM is an optional existing root certificate.
	RootCertPEM []byte
	// RootKeyPEM is an optional existing root private key.
	RootKeyPEM []byte
	// IntermediateCertPEM is an optional existing intermediate certificate.
	IntermediateCertPEM []byte
	// IntermediateKeyPEM is an optional existing intermediate private key.
	IntermediateKeyPEM []byte
}

// DefaultConfig returns a default CA configuration.
func DefaultConfig() *Config {
	return &Config{
		CommonName:   "Unheaded Kingdom Root CA",
		Organization: "Unheaded Kingdom",
		Country:      "XX",
		Duration:     10 * 365 * 24 * time.Hour, // 10 years
		KeySize:      256,                       // P-256 curve
	}
}

// Authority represents a certificate authority.
type Authority struct {
	config         *Config
	rootCert       *x509.Certificate
	rootKey        crypto.PrivateKey
	rootCertPEM    []byte
	intermCert     *x509.Certificate
	intermKey      crypto.PrivateKey
	intermCertPEM  []byte
	chainPEM       []byte
	revokedSerials map[string]time.Time
	mu             sync.RWMutex
}

// New creates a new certificate authority.
func New(config *Config) (*Authority, error) {
	if config == nil {
		config = DefaultConfig()
	}

	a := &Authority{
		config:         config,
		revokedSerials: make(map[string]time.Time),
	}

	// Load or generate root CA
	if config.RootCertPEM != nil && config.RootKeyPEM != nil {
		if err := a.loadRoot(config.RootCertPEM, config.RootKeyPEM); err != nil {
			return nil, err
		}
	} else {
		if err := a.generateRoot(); err != nil {
			return nil, err
		}
	}

	// Load or generate intermediate CA
	if config.IntermediateCertPEM != nil && config.IntermediateKeyPEM != nil {
		if err := a.loadIntermediate(config.IntermediateCertPEM, config.IntermediateKeyPEM); err != nil {
			return nil, err
		}
	} else {
		if err := a.generateIntermediate(); err != nil {
			return nil, err
		}
	}

	// Build chain
	a.chainPEM = append(a.intermCertPEM, a.rootCertPEM...)

	return a, nil
}

// generateRoot generates a new root CA certificate.
func (a *Authority) generateRoot() error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := generateSerial()
	if err != nil {
		return err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   a.config.CommonName,
			Organization: []string{a.config.Organization},
			Country:      []string{a.config.Country},
		},
		NotBefore:             now,
		NotAfter:              now.Add(a.config.Duration),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		MaxPathLenZero:        false,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return err
	}

	a.rootCert = cert
	a.rootKey = key
	a.rootCertPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return nil
}

// generateIntermediate generates a new intermediate CA certificate.
func (a *Authority) generateIntermediate() error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := generateSerial()
	if err != nil {
		return err
	}

	now := time.Now()
	// Intermediate CA lasts half as long as root
	duration := a.config.Duration / 2
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   a.config.CommonName + " Intermediate",
			Organization: []string{a.config.Organization},
			Country:      []string{a.config.Country},
		},
		NotBefore:             now,
		NotAfter:              now.Add(duration),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, a.rootCert, &key.PublicKey, a.rootKey)
	if err != nil {
		return err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return err
	}

	a.intermCert = cert
	a.intermKey = key
	a.intermCertPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return nil
}

// loadRoot loads an existing root CA.
func (a *Authority) loadRoot(certPEM, keyPEM []byte) error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return ErrInvalidCertificate
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return errors.New("invalid private key")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8
		pkcs8Key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
		ecKey, ok := pkcs8Key.(*ecdsa.PrivateKey)
		if !ok {
			return fmt.Errorf("ca: root private key is not ECDSA (got %T)", pkcs8Key)
		}
		key = ecKey
	}

	a.rootCert = cert
	a.rootKey = key
	a.rootCertPEM = certPEM

	return nil
}

// loadIntermediate loads an existing intermediate CA.
func (a *Authority) loadIntermediate(certPEM, keyPEM []byte) error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return ErrInvalidCertificate
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return errors.New("invalid private key")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		pkcs8Key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
		ecKey, ok := pkcs8Key.(*ecdsa.PrivateKey)
		if !ok {
			return fmt.Errorf("ca: intermediate private key is not ECDSA (got %T)", pkcs8Key)
		}
		key = ecKey
	}

	a.intermCert = cert
	a.intermKey = key
	a.intermCertPEM = certPEM

	return nil
}

// GetCertificate returns the signing certificate (intermediate CA).
func (a *Authority) GetCertificate() (*x509.Certificate, []byte) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.intermCert, a.intermCertPEM
}

// GetRootCertificate returns the root CA certificate.
func (a *Authority) GetRootCertificate() (*x509.Certificate, []byte) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.rootCert, a.rootCertPEM
}

// GetChain returns the full certificate chain.
func (a *Authority) GetChain() []byte {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.chainPEM
}

// GetSigningKey returns the signing key (intermediate CA key).
func (a *Authority) GetSigningKey() crypto.PrivateKey {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.intermKey
}

// Revoke adds a serial number to the revocation list.
func (a *Authority) Revoke(serial string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revokedSerials[serial] = time.Now()
	return nil
}

// IsRevoked checks if a serial number is revoked.
func (a *Authority) IsRevoked(serial string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.revokedSerials[serial]
	return ok
}

// Verify verifies a certificate against the CA chain.
func (a *Authority) Verify(certPEM []byte) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return ErrInvalidCertificate
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	// Check revocation
	if a.IsRevoked(cert.SerialNumber.String()) {
		return errors.New("certificate revoked")
	}

	// Build verification pool
	roots := x509.NewCertPool()
	roots.AddCert(a.rootCert)

	intermediates := x509.NewCertPool()
	intermediates.AddCert(a.intermCert)

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   time.Now(),
	}

	if _, err := cert.Verify(opts); err != nil {
		return ErrCertificateVerificationFailed
	}

	return nil
}

// generateSerial generates a random serial number.
func generateSerial() (*big.Int, error) {
	serialMax := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialMax)
}
