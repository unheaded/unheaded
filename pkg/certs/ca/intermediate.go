// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ca

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"time"
)

// IntermediateCAConfig configures intermediate CA generation.
type IntermediateCAConfig struct {
	CommonName   string
	Organization string
	Country      string
	Duration     time.Duration
	KeyCurve     elliptic.Curve
}

// DefaultIntermediateCAConfig returns default intermediate CA configuration.
func DefaultIntermediateCAConfig() *IntermediateCAConfig {
	return &IntermediateCAConfig{
		CommonName:   "Unheaded Kingdom Intermediate CA",
		Organization: "Unheaded Kingdom",
		Country:      "XX",
		Duration:     5 * 365 * 24 * time.Hour,
		KeyCurve:     elliptic.P256(),
	}
}

// GenerateIntermediateCA generates a new intermediate CA signed by the parent.
func GenerateIntermediateCA(
	config *IntermediateCAConfig,
	parentCert *x509.Certificate,
	parentKey crypto.PrivateKey,
) (certPEM, keyPEM []byte, err error) {
	if config == nil {
		config = DefaultIntermediateCAConfig()
	}

	// Generate private key
	privateKey, err := ecdsa.GenerateKey(config.KeyCurve, rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Generate serial number
	serial, err := generateSerial()
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   config.CommonName,
			Organization: []string{config.Organization},
			Country:      []string{config.Country},
		},
		NotBefore:             now,
		NotAfter:              now.Add(config.Duration),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	// Sign with parent
	certDER, err := x509.CreateCertificate(rand.Reader, template, parentCert, &privateKey.PublicKey, parentKey)
	if err != nil {
		return nil, nil, err
	}

	// Encode certificate
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return certPEM, keyPEM, nil
}

// LoadIntermediateCA loads an intermediate CA from PEM-encoded data.
func LoadIntermediateCA(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	return LoadRootCA(certPEM, keyPEM) // Same loading logic
}

// VerifyIntermediateCA verifies an intermediate CA certificate against a root.
func VerifyIntermediateCA(intermCert, rootCert *x509.Certificate) error {
	if !intermCert.IsCA {
		return ErrInvalidCertificate
	}
	if intermCert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return ErrInvalidCertificate
	}

	// Build verification chain
	roots := x509.NewCertPool()
	roots.AddCert(rootCert)

	opts := x509.VerifyOptions{
		Roots:       roots,
		CurrentTime: time.Now(),
	}

	if _, err := intermCert.Verify(opts); err != nil {
		return ErrCertificateVerificationFailed
	}

	return nil
}
