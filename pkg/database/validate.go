// SPDX-License-Identifier: GPL-3.0-or-later
package database

import (
	"fmt"
	"net/netip"
)

// ValidateConfig checks for common security misconfigurations.
func ValidateConfig(cfg Config) error {
	if cfg.Password == "" {
		return fmt.Errorf("POSTGRES_PASSWORD must not be empty")
	}
	if cfg.Password == "unheaded_dev" || cfg.Password == "postgres" || cfg.Password == "password" {
		// Only warn in dev, block in prod
		if cfg.SSLMode != "disable" {
			return fmt.Errorf("default password used with SSL enabled — set a real password")
		}
	}
	if cfg.SSLMode == "disable" && !isPrivateHost(cfg.Host) {
		return fmt.Errorf("SSL disabled for non-private host %q — set POSTGRES_SSLMODE=require", cfg.Host)
	}
	return nil
}

// isPrivateHost reports whether host is localhost or a literal loopback or
// private address (RFC 1918, RFC 4193). Names other than localhost are not
// private: "10.example.com" is a DNS name, not 10/8, and 172.32/12 is public.
func isPrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsPrivate()
}
