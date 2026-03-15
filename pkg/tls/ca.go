// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package tls provides mTLS configuration for Unheaded service mesh.
//
// It implements a lightweight internal CA for development and per-service
// certificate generation. Services use this package to establish zero-plaintext
// communication with TLS 1.3 and mutual certificate verification.
//
// TLS is OPTIONAL and controlled by the UNHEADED_TLS_ENABLED environment
// variable. When disabled, services operate over plain HTTP as before.
package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"time"
)

var (
	// ErrCANotInitialized is returned when a CA operation is attempted before Init.
	ErrCANotInitialized = errors.New("tls: CA not initialized")
	// ErrInvalidCertRequest is returned when a certificate request has invalid parameters.
	ErrInvalidCertRequest = errors.New("tls: invalid certificate request")
	// ErrCertExpired is returned when a certificate has expired.
	ErrCertExpired = errors.New("tls: certificate expired")
)

// CA is a lightweight internal certificate authority for development.
// It generates a self-signed root and issues per-service certificates.
type CA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte
	keyPEM  []byte
}

// CertRequest describes a service certificate to issue.
type CertRequest struct {
	// ServiceName is the CN and primary DNS SAN (e.g. "timeguru").
	ServiceName string
	// DNSNames are additional DNS SANs (localhost is always added).
	DNSNames []string
	// IPAddresses are SAN IP addresses (127.0.0.1 and ::1 are always added).
	IPAddresses []net.IP
	// TTL is the certificate validity duration (default 24h).
	TTL time.Duration
}

// ServiceCert holds the issued certificate, private key, and CA cert in PEM form.
type ServiceCert struct {
	CertPEM []byte
	KeyPEM  []byte
	CAPEM   []byte
}

// NewCA creates a new self-signed root CA for development use.
// The CA certificate is valid for 10 years with an ECDSA P-256 key.
func NewCA() (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Unheaded"},
			CommonName:   "Unheaded Internal CA",
		},
		NotBefore:             time.Now().Add(-5 * time.Minute), // clock skew tolerance
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &CA{
		cert:    cert,
		key:     key,
		certPEM: certPEM,
		keyPEM:  keyPEM,
	}, nil
}

// CertPEM returns the CA certificate in PEM format.
func (ca *CA) CertPEM() []byte {
	return ca.certPEM
}

// KeyPEM returns the CA private key in PEM format.
func (ca *CA) KeyPEM() []byte {
	return ca.keyPEM
}

// Certificate returns the parsed CA x509.Certificate.
func (ca *CA) Certificate() *x509.Certificate {
	return ca.cert
}

// CertPool returns a CertPool containing only this CA certificate.
func (ca *CA) CertPool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	return pool
}

// IssueCert generates a signed certificate for a service.
// The certificate includes the service name as CN, DNS SANs for the service name,
// localhost, and the .unheaded.internal domain, plus IP SANs for 127.0.0.1 and ::1.
func (ca *CA) IssueCert(req CertRequest) (*ServiceCert, error) {
	if req.ServiceName == "" {
		return nil, ErrInvalidCertRequest
	}

	ttl := req.TTL
	if ttl == 0 {
		ttl = 24 * time.Hour
	}

	// Build DNS SANs: always include service name, localhost, and internal domain.
	dnsNames := []string{
		req.ServiceName,
		req.ServiceName + ".unheaded.internal",
		"localhost",
	}
	for _, name := range req.DNSNames {
		if !containsString(dnsNames, name) {
			dnsNames = append(dnsNames, name)
		}
	}

	// Build IP SANs: always include loopback.
	ips := []net.IP{
		net.IPv4(127, 0, 0, 1),
		net.IPv6loopback,
	}
	for _, ip := range req.IPAddresses {
		if !containsIP(ips, ip) {
			ips = append(ips, ip)
		}
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization:       []string{"Unheaded"},
			OrganizationalUnit: []string{req.ServiceName},
			CommonName:         req.ServiceName + ".unheaded.internal",
		},
		DNSNames:    dnsNames,
		IPAddresses: ips,
		NotBefore:   time.Now().Add(-5 * time.Minute),
		NotAfter:    time.Now().Add(ttl),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &ServiceCert{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		CAPEM:   ca.certPEM,
	}, nil
}

// VerifyCert checks that a PEM certificate was signed by this CA and is not expired.
func (ca *CA) VerifyCert(certPEM []byte) error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return errors.New("tls: failed to decode PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	pool := ca.CertPool()
	opts := x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}
	if _, err := cert.Verify(opts); err != nil {
		return err
	}

	if time.Now().After(cert.NotAfter) {
		return ErrCertExpired
	}

	return nil
}

func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func containsIP(ips []net.IP, ip net.IP) bool {
	for _, v := range ips {
		if v.Equal(ip) {
			return true
		}
	}
	return false
}
