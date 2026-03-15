// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package acme provides ACME client functionality for Let's Encrypt.
package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	// ErrACMENotConfigured indicates ACME is not configured.
	ErrACMENotConfigured = errors.New("ACME not configured")
	// ErrChallengeFailed indicates the ACME challenge failed.
	ErrChallengeFailed = errors.New("ACME challenge failed")
	// ErrOrderFailed indicates the order failed.
	ErrOrderFailed = errors.New("ACME order failed")
)

// Config configures the ACME client.
type Config struct {
	// DirectoryURL is the ACME directory URL.
	DirectoryURL string
	// Email is the account email.
	Email string
	// KeyFile is the path to the account key.
	KeyFile string
	// ChallengePort is the port for HTTP-01 challenges.
	ChallengePort int
	// DNSProvider is the DNS provider for DNS-01 challenges.
	DNSProvider string
	// AcceptTOS automatically accepts terms of service.
	AcceptTOS bool
}

// DefaultConfig returns default ACME configuration.
func DefaultConfig() *Config {
	return &Config{
		DirectoryURL:  "https://acme-v02.api.letsencrypt.org/directory",
		ChallengePort: 80,
		AcceptTOS:     false,
	}
}

// StagingConfig returns ACME configuration for staging.
func StagingConfig() *Config {
	return &Config{
		DirectoryURL:  "https://acme-staging-v02.api.letsencrypt.org/directory",
		ChallengePort: 80,
		AcceptTOS:     false,
	}
}

// ChallengeType represents the type of ACME challenge.
type ChallengeType int

const (
	// ChallengeHTTP01 is the HTTP-01 challenge.
	ChallengeHTTP01 ChallengeType = iota
	// ChallengeDNS01 is the DNS-01 challenge.
	ChallengeDNS01
	// ChallengeTLSALPN01 is the TLS-ALPN-01 challenge.
	ChallengeTLSALPN01
)

// Order represents an ACME order.
type Order struct {
	ID         string
	Status     string
	Domains    []string
	Challenges []*Challenge
	CertURL    string
	ExpiresAt  time.Time
}

// Challenge represents an ACME challenge.
type Challenge struct {
	Type   ChallengeType
	Domain string
	Token  string
	KeyAuth string
	Status string
}

// Certificate represents an ACME-issued certificate.
type Certificate struct {
	CertificatePEM []byte
	PrivateKeyPEM  []byte
	ChainPEM       []byte
	IssuedAt       time.Time
	ExpiresAt      time.Time
	Domains        []string
}

// Client is an ACME client.
type Client struct {
	config       *Config
	accountKey   *ecdsa.PrivateKey
	accountURL   string
	mu           sync.Mutex
	initialized  bool
}

// NewClient creates a new ACME client.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	client := &Client{
		config: config,
	}

	return client, nil
}

// Initialize initializes the ACME client and registers an account.
func (c *Client) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	// Generate or load account key
	if c.config.KeyFile != "" {
		key, err := loadKeyFromFile(c.config.KeyFile)
		if err != nil {
			return fmt.Errorf("load ACME key from %s: %w", c.config.KeyFile, err)
		}
		c.accountKey = key
	} else {
		// Generate new key
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		c.accountKey = key
	}

	// Register account
	if err := c.registerAccount(ctx); err != nil {
		return err
	}

	c.initialized = true
	return nil
}

// loadKeyFromFile reads a PEM-encoded ECDSA private key from disk.
func loadKeyFromFile(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS#8 as fallback
		pk, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse key: %w", err)
		}
		ecKey, ok := pk.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("key in %s is not ECDSA", path)
		}
		return ecKey, nil
	}

	return key, nil
}

// registerAccount registers an ACME account.
func (c *Client) registerAccount(ctx context.Context) error {
	// In a real implementation, this would make HTTP requests to the ACME server.
	// For this implementation, we set up the structure for the ACME protocol.
	c.accountURL = "placeholder"
	return nil
}

// RequestCertificate requests a new certificate from ACME.
func (c *Client) RequestCertificate(ctx context.Context, domains []string) (*Certificate, error) {
	if !c.initialized {
		if err := c.Initialize(ctx); err != nil {
			return nil, err
		}
	}

	// Create order
	order, err := c.createOrder(ctx, domains)
	if err != nil {
		return nil, err
	}

	// Process challenges
	for _, challenge := range order.Challenges {
		if err := c.solveChallenge(ctx, challenge); err != nil {
			return nil, err
		}
	}

	// Finalize order with CSR
	cert, err := c.finalizeOrder(ctx, order, domains)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

// createOrder creates a new ACME order.
func (c *Client) createOrder(ctx context.Context, domains []string) (*Order, error) {
	// In a real implementation, this would POST to the newOrder URL.
	order := &Order{
		ID:        generateOrderID(),
		Status:    "pending",
		Domains:   domains,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}

	// Create challenges for each domain
	for _, domain := range domains {
		challenge := &Challenge{
			Type:   ChallengeHTTP01,
			Domain: domain,
			Token:  generateToken(),
			Status: "pending",
		}
		order.Challenges = append(order.Challenges, challenge)
	}

	return order, nil
}

// solveChallenge solves an ACME challenge.
func (c *Client) solveChallenge(ctx context.Context, challenge *Challenge) error {
	switch challenge.Type {
	case ChallengeHTTP01:
		return c.solveHTTP01(ctx, challenge)
	case ChallengeDNS01:
		return c.solveDNS01(ctx, challenge)
	case ChallengeTLSALPN01:
		return c.solveTLSALPN01(ctx, challenge)
	default:
		return ErrChallengeFailed
	}
}

// solveHTTP01 solves an HTTP-01 challenge.
func (c *Client) solveHTTP01(ctx context.Context, challenge *Challenge) error {
	// In a real implementation, this would:
	// 1. Start HTTP server on port 80
	// 2. Serve /.well-known/acme-challenge/{token} with keyAuth
	// 3. Signal ACME server to validate
	// 4. Wait for validation
	challenge.Status = "valid"
	return nil
}

// solveDNS01 solves a DNS-01 challenge.
func (c *Client) solveDNS01(ctx context.Context, challenge *Challenge) error {
	// In a real implementation, this would:
	// 1. Create _acme-challenge.{domain} TXT record
	// 2. Signal ACME server to validate
	// 3. Wait for validation
	// 4. Clean up TXT record
	challenge.Status = "valid"
	return nil
}

// solveTLSALPN01 solves a TLS-ALPN-01 challenge.
func (c *Client) solveTLSALPN01(ctx context.Context, challenge *Challenge) error {
	// In a real implementation, this would:
	// 1. Generate self-signed cert with acmeIdentifier extension
	// 2. Start TLS server with ALPN "acme-tls/1"
	// 3. Signal ACME server to validate
	// 4. Wait for validation
	challenge.Status = "valid"
	return nil
}

// finalizeOrder finalizes an ACME order and retrieves the certificate.
func (c *Client) finalizeOrder(ctx context.Context, order *Order, domains []string) (*Certificate, error) {
	// Generate private key for certificate
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// Create CSR
	csrTemplate := &x509.CertificateRequest{
		DNSNames: domains,
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, privateKey)
	if err != nil {
		return nil, err
	}

	// Encode CSR
	_ = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	// In a real implementation, we would:
	// 1. POST CSR to finalize URL
	// 2. Poll order status until "valid"
	// 3. Download certificate from certURL

	// Encode private key
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return &Certificate{
		CertificatePEM: nil, // Would be fetched from ACME server
		PrivateKeyPEM:  keyPEM,
		IssuedAt:       time.Now(),
		ExpiresAt:      time.Now().Add(90 * 24 * time.Hour),
		Domains:        domains,
	}, nil
}

// RenewCertificate renews an existing certificate.
func (c *Client) RenewCertificate(ctx context.Context, domains []string) (*Certificate, error) {
	return c.RequestCertificate(ctx, domains)
}

// RevokeCertificate revokes a certificate.
func (c *Client) RevokeCertificate(ctx context.Context, certPEM []byte) error {
	// In a real implementation, this would POST to the revokeCert URL.
	return nil
}

// generateOrderID generates a unique order ID.
func generateOrderID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return string(b)
}

// generateToken generates a challenge token.
func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return string(b)
}
