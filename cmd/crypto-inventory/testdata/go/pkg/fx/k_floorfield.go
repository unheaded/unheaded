// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import (
	"crypto/tls"
	"os"
)

// Options is not a tls.Config, but its MinVersion becomes one later.
type Options struct{ MinVersion uint16 }

// DefaultOptions sets a TLS 1.2 floor through a field of another struct.
func DefaultOptions() Options {
	return Options{MinVersion: tls.VersionTLS12}
}

// Kill switches in code count as well as in config.
func K() error {
	return os.Setenv("GODEBUG", "tlssecpmlkem=0")
}
