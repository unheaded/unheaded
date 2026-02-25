// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package tls

import (
	"net/http"
	"os"
)

// ServiceTLSConfig is loaded from environment variables. All fields are optional.
// When Enabled is false, the TLS layer is a no-op and services run over plain HTTP.
type ServiceTLSConfig struct {
	// Enabled is true when UNHEADED_TLS_ENABLED=true.
	Enabled bool
	// CertFile is the path to the PEM-encoded service certificate (UNHEADED_TLS_CERT).
	CertFile string
	// KeyFile is the path to the PEM-encoded service private key (UNHEADED_TLS_KEY_FILE).
	KeyFile string
	// CAFile is the path to the PEM-encoded CA certificate (UNHEADED_TLS_CA_CERT).
	CAFile string
	// RequireClientCert enables mTLS when true (UNHEADED_TLS_MTLS_ENABLED, default true).
	RequireClientCert bool
}

// LoadServiceTLSConfig reads TLS configuration from environment variables.
// Returns a config with Enabled=false when UNHEADED_TLS_ENABLED is not "true",
// allowing non-TLS operation with zero code changes.
func LoadServiceTLSConfig() ServiceTLSConfig {
	cfg := ServiceTLSConfig{
		Enabled:           os.Getenv("UNHEADED_TLS_ENABLED") == "true",
		CertFile:          os.Getenv("UNHEADED_TLS_CERT"),
		KeyFile:           os.Getenv("UNHEADED_TLS_KEY_FILE"),
		CAFile:            os.Getenv("UNHEADED_TLS_CA_CERT"),
		RequireClientCert: os.Getenv("UNHEADED_TLS_MTLS_ENABLED") != "false",
	}
	return cfg
}

// MaybeWrapServer creates an mTLS-configured *http.Server when TLS is enabled.
// When disabled, it returns a plain *http.Server. Callers can use ListenAndServe
// for plain HTTP or ListenAndServeTLS (with empty cert/key args) for TLS.
//
// Usage:
//
//	srv, tlsEnabled, err := tls.MaybeWrapServer(":8443", handler)
//	if tlsEnabled {
//	    srv.ListenAndServeTLS("", "")  // certs already loaded in TLSConfig
//	} else {
//	    srv.ListenAndServe()
//	}
func MaybeWrapServer(addr string, handler http.Handler) (srv *http.Server, tlsEnabled bool, err error) {
	cfg := LoadServiceTLSConfig()
	if !cfg.Enabled {
		return &http.Server{
			Addr:    addr,
			Handler: handler,
		}, false, nil
	}

	certPEM, err := os.ReadFile(cfg.CertFile)
	if err != nil {
		return nil, false, err
	}
	keyPEM, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		return nil, false, err
	}
	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, false, err
	}

	tlsSrv, err := NewTLSServer(addr, handler, ServerConfig{
		CertPEM:           certPEM,
		KeyPEM:            keyPEM,
		CAPEM:             caPEM,
		RequireClientCert: cfg.RequireClientCert,
	})
	if err != nil {
		return nil, false, err
	}

	return tlsSrv, true, nil
}

// MaybeCreateClient creates an mTLS-configured *http.Client when TLS is enabled.
// When disabled, it returns http.DefaultClient.
func MaybeCreateClient() (*http.Client, error) {
	cfg := LoadServiceTLSConfig()
	if !cfg.Enabled {
		return http.DefaultClient, nil
	}

	certPEM, err := os.ReadFile(cfg.CertFile)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		return nil, err
	}
	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, err
	}

	return NewTLSHTTPClient(ClientConfig{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		CAPEM:   caPEM,
	})
}
