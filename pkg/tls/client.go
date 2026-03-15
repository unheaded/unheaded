// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"time"
)

// ClientConfig holds the parameters for building a TLS client configuration.
type ClientConfig struct {
	// CertPEM is the client certificate in PEM format (for mTLS).
	CertPEM []byte
	// KeyPEM is the client private key in PEM format (for mTLS).
	KeyPEM []byte
	// CAPEM is the CA certificate used to verify the server's certificate.
	CAPEM []byte
	// Timeout is the HTTP client timeout (default 30s).
	Timeout time.Duration
}

// NewClientTLSConfig builds a *tls.Config suitable for an mTLS HTTP/gRPC client.
//
// Enforced settings:
//   - TLS 1.3 minimum
//   - Server cert verified against the provided CA
//   - Client certificate presented for mTLS
func NewClientTLSConfig(cfg ClientConfig) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}

	// Load client certificate for mTLS.
	if len(cfg.CertPEM) > 0 && len(cfg.KeyPEM) > 0 {
		cert, err := tls.X509KeyPair(cfg.CertPEM, cfg.KeyPEM)
		if err != nil {
			return nil, errors.New("tls: failed to load client certificate: " + err.Error())
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	// Load CA for server verification.
	if len(cfg.CAPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.CAPEM) {
			return nil, errors.New("tls: failed to parse CA certificate for client")
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg, nil
}

// NewTLSHTTPClient creates an *http.Client configured for mTLS connections.
func NewTLSHTTPClient(cfg ClientConfig) (*http.Client, error) {
	tlsCfg, err := NewClientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}, nil
}
