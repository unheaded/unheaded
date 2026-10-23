// SPDX-License-Identifier: GPL-3.0-or-later

//go:build go1.27

package fx

import "crypto/tls"

// A pure ML-KEM group is post-quantum but not hybrid (tls.MLKEM1024 needs
// go1.27; the module says go 1.26, so this file carries its own version).
var GPQ = &tls.Config{
	MinVersion:       tls.VersionTLS13,
	CurvePreferences: []tls.CurveID{tls.MLKEM1024},
}
