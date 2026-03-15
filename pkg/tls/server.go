// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"time"
)

// ServerConfig holds the parameters for building a TLS server configuration.
type ServerConfig struct {
	// CertPEM is the server certificate in PEM format.
	CertPEM []byte
	// KeyPEM is the server private key in PEM format.
	KeyPEM []byte
	// CAPEM is the CA certificate used to verify client certs (mTLS).
	CAPEM []byte
	// RequireClientCert enables mTLS (RequireAndVerifyClientCert).
	// When false, the server still uses TLS but does not demand a client cert.
	RequireClientCert bool
}

// NewServerTLSConfig builds a *tls.Config suitable for an mTLS HTTP/gRPC server.
//
// Enforced settings:
//   - TLS 1.3 minimum
//   - Cipher suites: TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256
//   - Client cert verification when RequireClientCert is true
func NewServerTLSConfig(cfg ServerConfig) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(cfg.CertPEM, cfg.KeyPEM)
	if err != nil {
		return nil, errors.New("tls: failed to load server certificate: " + err.Error())
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}

	if cfg.RequireClientCert && len(cfg.CAPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.CAPEM) {
			return nil, errors.New("tls: failed to parse CA certificate")
		}
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
		tlsCfg.ClientCAs = pool
	}

	return tlsCfg, nil
}

// NewTLSServer creates an *http.Server with TLS configured from the given ServerConfig.
// addr is the listen address (e.g. ":8443").
func NewTLSServer(addr string, handler http.Handler, cfg ServerConfig) (*http.Server, error) {
	tlsCfg, err := NewServerTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}, nil
}
