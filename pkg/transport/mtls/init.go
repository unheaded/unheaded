// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package mtls

import "time"

// DefaultServices is the list of all Unheaded services that need certificates.
var DefaultServices = []string{
	"gateway",
	"wotan",
	"dashboard-backend",
	"unheaded-daemon",
	"kanban-app",
	"wiki-server",
	"doom-bridge",
	"trace-collector",
	"monad",
	"sophia",
	"timeguru",
	"captain",
	"architect",
	"micromanager",
}

// DefaultCAValidity is 10 years for the root CA.
const DefaultCAValidity = 10 * 365 * 24 * time.Hour

// DefaultCertValidity is 1 year for service certificates.
const DefaultCertValidity = 365 * 24 * time.Hour

// InitPKI generates a complete PKI setup with root CA and all service certs.
func InitPKI(pkiDir string) error {
	caCert, caKey, err := GenerateRootCA(DefaultCAValidity)
	if err != nil {
		return err
	}
	return WritePKI(pkiDir, caCert, caKey, DefaultServices, DefaultCertValidity)
}
