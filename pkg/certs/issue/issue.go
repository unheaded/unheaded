// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package issue handles certificate issuance.
package issue

import (
	"context"
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
	"time"

	"unheaded/pkg/certs/ca"
)

var (
	// ErrInvalidRequest indicates the request is invalid.
	ErrInvalidRequest = errors.New("invalid certificate request")
	// ErrCSRParseFailed indicates CSR parsing failed.
	ErrCSRParseFailed = errors.New("failed to parse CSR")
)

// Request contains certificate issuance parameters.
type Request struct {
	CommonName         string
	DNSNames           []string
	IPAddresses        []string
	URIs               []string
	Duration           time.Duration
	SPIFFEID           string
	Organization       string
	OrganizationalUnit string
	CSR                []byte
	KeyUsage           x509.KeyUsage
	ExtKeyUsage        []x509.ExtKeyUsage
}

// Result contains the issued certificate and related material.
type Result struct {
	Serial         string
	Certificate    *x509.Certificate
	CertificatePEM []byte
	PrivateKeyPEM  []byte
	ChainPEM       []byte
}

// Issuer handles certificate issuance.
type Issuer struct {
	authority       *ca.Authority
	defaultDuration time.Duration
	maxDuration     time.Duration
}

// New creates a new certificate issuer.
func New(authority *ca.Authority, defaultDuration, maxDuration time.Duration) *Issuer {
	return &Issuer{
		authority:       authority,
		defaultDuration: defaultDuration,
		maxDuration:     maxDuration,
	}
}

// Issue creates a new certificate.
func (i *Issuer) Issue(ctx context.Context, req *Request) (*Result, error) {
	if err := i.validateRequest(req); err != nil {
		return nil, err
	}

	// Clamp duration
	duration := req.Duration
	if duration == 0 {
		duration = i.defaultDuration
	}
	if duration > i.maxDuration {
		duration = i.maxDuration
	}

	var privateKey *ecdsa.PrivateKey
	var publicKey interface{}

	// If CSR provided, use its public key
	if len(req.CSR) > 0 {
		csr, err := ParseCSR(req.CSR)
		if err != nil {
			return nil, err
		}
		publicKey = csr.PublicKey
	} else {
		// Generate new key pair
		var err error
		privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		publicKey = &privateKey.PublicKey
	}

	// Generate serial
	serial, err := generateSerial()
	if err != nil {
		return nil, err
	}

	// Parse IP addresses
	var ips []net.IP
	for _, ipStr := range req.IPAddresses {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			ips = append(ips, ip)
		}
	}

	// Parse URIs (including SPIFFE ID)
	var uris []*url.URL
	if req.SPIFFEID != "" {
		spiffeURL, err := url.Parse(req.SPIFFEID)
		if err == nil {
			uris = append(uris, spiffeURL)
		}
	}
	for _, uriStr := range req.URIs {
		uri, err := url.Parse(uriStr)
		if err == nil {
			uris = append(uris, uri)
		}
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         req.CommonName,
			Organization:       []string{req.Organization},
			OrganizationalUnit: []string{req.OrganizationalUnit},
		},
		NotBefore:             now,
		NotAfter:              now.Add(duration),
		KeyUsage:              req.KeyUsage,
		ExtKeyUsage:           req.ExtKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              req.DNSNames,
		IPAddresses:           ips,
		URIs:                  uris,
	}

	// Get signing cert and key
	signingCert, _ := i.authority.GetCertificate()
	signingKey := i.authority.GetSigningKey()

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, signingCert, publicKey, signingKey)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	var keyPEM []byte
	if privateKey != nil {
		keyDER, err := x509.MarshalECPrivateKey(privateKey)
		if err != nil {
			return nil, err
		}
		keyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: keyDER,
		})
	}

	// Build chain
	chainPEM := append(certPEM, i.authority.GetChain()...)

	return &Result{
		Serial:         serial.String(),
		Certificate:    cert,
		CertificatePEM: certPEM,
		PrivateKeyPEM:  keyPEM,
		ChainPEM:       chainPEM,
	}, nil
}

// validateRequest validates a certificate request.
func (i *Issuer) validateRequest(req *Request) error {
	if req == nil {
		return ErrInvalidRequest
	}
	if req.CommonName == "" && len(req.DNSNames) == 0 && len(req.IPAddresses) == 0 {
		return ErrInvalidRequest
	}
	return nil
}

// generateSerial generates a random serial number.
func generateSerial() (*big.Int, error) {
	serialMax := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialMax)
}

// KeyUsageFromType returns key usage for a certificate type.
func KeyUsageFromType(certType int) x509.KeyUsage {
	switch certType {
	case 0: // Server
		return x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	case 1: // Client
		return x509.KeyUsageDigitalSignature
	case 2: // Mutual
		return x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	default:
		return x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	}
}

// ExtKeyUsageFromType returns extended key usage for a certificate type.
func ExtKeyUsageFromType(certType int) []x509.ExtKeyUsage {
	switch certType {
	case 0: // Server
		return []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	case 1: // Client
		return []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	case 2: // Mutual
		return []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	default:
		return []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	}
}
