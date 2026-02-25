// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package tls

import (
	"crypto/tls"

	"google.golang.org/grpc/credentials"
)

// NewServerTransportCredentials returns gRPC transport credentials for a TLS server.
// It wraps NewServerTLSConfig and converts to gRPC's credentials.TransportCredentials.
func NewServerTransportCredentials(cfg ServerConfig) (credentials.TransportCredentials, error) {
	tlsCfg, err := NewServerTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(tlsCfg), nil
}

// NewClientTransportCredentials returns gRPC transport credentials for a TLS client.
// It wraps NewClientTLSConfig and converts to gRPC's credentials.TransportCredentials.
func NewClientTransportCredentials(cfg ClientConfig) (credentials.TransportCredentials, error) {
	tlsCfg, err := NewClientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(tlsCfg), nil
}

// ServerTLSConfigToCredentials wraps an existing *tls.Config into gRPC server credentials.
func ServerTLSConfigToCredentials(cfg *tls.Config) credentials.TransportCredentials {
	return credentials.NewTLS(cfg)
}

// ClientTLSConfigToCredentials wraps an existing *tls.Config into gRPC client credentials.
func ClientTLSConfigToCredentials(cfg *tls.Config) credentials.TransportCredentials {
	return credentials.NewTLS(cfg)
}
