// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"time"
)

// RootCAConfig configures root CA generation.
type RootCAConfig struct {
	CommonName   string
	Organization string
	Country      string
	Province     string
	Locality     string
	Duration     time.Duration
	KeyCurve     elliptic.Curve
}

// DefaultRootCAConfig returns default root CA configuration.
func DefaultRootCAConfig() *RootCAConfig {
	return &RootCAConfig{
		CommonName:   "Unheaded Kingdom Root CA",
		Organization: "Unheaded Kingdom",
		Country:      "XX",
		Duration:     10 * 365 * 24 * time.Hour,
		KeyCurve:     elliptic.P384(),
	}
}

// GenerateRootCA generates a new root CA certificate and key.
func GenerateRootCA(config *RootCAConfig) (certPEM, keyPEM []byte, err error) {
	if config == nil {
		config = DefaultRootCAConfig()
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
			Province:     []string{config.Province},
			Locality:     []string{config.Locality},
		},
		NotBefore:             now,
		NotAfter:              now.Add(config.Duration),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
		MaxPathLenZero:        false,
	}

	// Self-sign
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
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

// LoadRootCA loads a root CA from PEM-encoded data.
func LoadRootCA(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, nil, ErrInvalidCertificate
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return nil, nil, ErrInvalidCertificate
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8
		pkcs8Key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, nil, err
		}
		var ok bool
		key, ok = pkcs8Key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, nil, ErrInvalidCertificate
		}
	}

	return cert, key, nil
}

// VerifyRootCA verifies a root CA certificate.
func VerifyRootCA(cert *x509.Certificate) error {
	if !cert.IsCA {
		return ErrInvalidCertificate
	}
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return ErrInvalidCertificate
	}
	if cert.MaxPathLen < 1 && !cert.MaxPathLenZero {
		// Root should allow at least one intermediate
	}
	return nil
}
