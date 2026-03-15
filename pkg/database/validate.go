// SPDX-License-Identifier: GPL-3.0-or-later
package database

import (
	"fmt"
	"strings"
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

func isPrivateHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" ||
		strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.") ||
		strings.HasPrefix(host, "192.168.")
}
