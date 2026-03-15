// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package mtls

import (
	"crypto/tls"
	"fmt"
	"os"
)

// SetupServiceTLS configures TLS for a service with mTLS.
// It loads the service certificate, key, and root CA from the PKI directory.
// Returns a TLS config suitable for gRPC/HTTP servers requiring client certificates.
func SetupServiceTLS(serviceName string, pkiDir string) (*tls.Config, error) {
	cfg := Config{
		ServiceName: serviceName,
		PKIDir:      pkiDir,
	}
	return ServerTLSConfig(cfg)
}

// SetupServiceClientTLS configures TLS for a service to connect to other services.
// It loads the service certificate, key, and root CA from the PKI directory.
// Returns a TLS config suitable for gRPC/HTTP clients.
func SetupServiceClientTLS(serviceName string, pkiDir string) (*tls.Config, error) {
	cfg := Config{
		ServiceName: serviceName,
		PKIDir:      pkiDir,
	}
	return ClientTLSConfig(cfg)
}

// ValidateCertificate checks if a certificate at the given path is valid.
func ValidateCertificate(certPath string) error {
	if certPath == "" {
		return fmt.Errorf("certificate path is empty")
	}

	// Check if file exists and is readable
	fileInfo, err := os.Stat(certPath)
	if err != nil {
		return fmt.Errorf("certificate file stat: %w", err)
	}

	if fileInfo.Size() == 0 {
		return fmt.Errorf("certificate file is empty")
	}

	return nil
}
