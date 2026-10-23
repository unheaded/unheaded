// SPDX-License-Identifier: GPL-3.0-or-later

package tls

import "crypto/tls"

// (n) inside unheaded/pkg/tls the profile functions are called bare.
func ApplyStrict(cfg *tls.Config) *tls.Config { return cfg }

func bare() *tls.Config {
	cfg := &tls.Config{}
	ApplyStrict(cfg)
	return cfg
}
