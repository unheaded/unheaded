// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package issue

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"time"
)

// TemplateType defines certificate template types.
type TemplateType int

const (
	// TemplateServer is for server certificates.
	TemplateServer TemplateType = iota
	// TemplateClient is for client certificates.
	TemplateClient
	// TemplateMutual is for mTLS certificates.
	TemplateMutual
	// TemplateWorkload is for SPIFFE workload identity.
	TemplateWorkload
)

// TemplateOptions contains options for template creation.
type TemplateOptions struct {
	Serial             *big.Int
	CommonName         string
	Organization       string
	OrganizationalUnit string
	Country            string
	DNSNames           []net.IP
	IPAddresses        []net.IP
	URIs               []*url.URL
	Duration           time.Duration
	SPIFFEID           string
}

// NewServerTemplate creates a server certificate template.
func NewServerTemplate(opts *TemplateOptions) *x509.Certificate {
	now := time.Now()
	return &x509.Certificate{
		SerialNumber: opts.Serial,
		Subject: pkix.Name{
			CommonName:         opts.CommonName,
			Organization:       []string{opts.Organization},
			OrganizationalUnit: []string{opts.OrganizationalUnit},
			Country:            []string{opts.Country},
		},
		NotBefore:             now,
		NotAfter:              now.Add(opts.Duration),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		IPAddresses:           opts.IPAddresses,
		URIs:                  opts.URIs,
	}
}

// NewClientTemplate creates a client certificate template.
func NewClientTemplate(opts *TemplateOptions) *x509.Certificate {
	now := time.Now()
	return &x509.Certificate{
		SerialNumber: opts.Serial,
		Subject: pkix.Name{
			CommonName:         opts.CommonName,
			Organization:       []string{opts.Organization},
			OrganizationalUnit: []string{opts.OrganizationalUnit},
			Country:            []string{opts.Country},
		},
		NotBefore:             now,
		NotAfter:              now.Add(opts.Duration),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		IPAddresses:           opts.IPAddresses,
		URIs:                  opts.URIs,
	}
}

// NewMutualTemplate creates a mutual TLS certificate template.
func NewMutualTemplate(opts *TemplateOptions) *x509.Certificate {
	now := time.Now()
	return &x509.Certificate{
		SerialNumber: opts.Serial,
		Subject: pkix.Name{
			CommonName:         opts.CommonName,
			Organization:       []string{opts.Organization},
			OrganizationalUnit: []string{opts.OrganizationalUnit},
			Country:            []string{opts.Country},
		},
		NotBefore:             now,
		NotAfter:              now.Add(opts.Duration),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		IPAddresses:           opts.IPAddresses,
		URIs:                  opts.URIs,
	}
}

// NewWorkloadTemplate creates a SPIFFE workload identity template.
func NewWorkloadTemplate(opts *TemplateOptions) *x509.Certificate {
	now := time.Now()

	// Parse SPIFFE ID as URI
	var uris []*url.URL
	if opts.SPIFFEID != "" {
		spiffeURL, err := url.Parse(opts.SPIFFEID)
		if err == nil {
			uris = append(uris, spiffeURL)
		}
	}
	uris = append(uris, opts.URIs...)

	return &x509.Certificate{
		SerialNumber: opts.Serial,
		Subject: pkix.Name{
			CommonName:         opts.CommonName,
			Organization:       []string{opts.Organization},
			OrganizationalUnit: []string{opts.OrganizationalUnit},
		},
		NotBefore:             now,
		NotAfter:              now.Add(opts.Duration),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		IPAddresses:           opts.IPAddresses,
		URIs:                  uris,
	}
}

// ShortLivedTemplate creates a template for short-lived certificates (hours).
func ShortLivedTemplate(templateType TemplateType, opts *TemplateOptions) *x509.Certificate {
	// Enforce short duration for moat certificates
	if opts.Duration > 24*time.Hour {
		opts.Duration = 24 * time.Hour
	}
	if opts.Duration == 0 {
		opts.Duration = 1 * time.Hour
	}

	switch templateType {
	case TemplateServer:
		return NewServerTemplate(opts)
	case TemplateClient:
		return NewClientTemplate(opts)
	case TemplateMutual:
		return NewMutualTemplate(opts)
	case TemplateWorkload:
		return NewWorkloadTemplate(opts)
	default:
		return NewMutualTemplate(opts)
	}
}
