// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import "crypto/tls"

// (b) nil CurvePreferences + TLS 1.3 floor: Go's default, hybrid first.
var B = &tls.Config{MinVersion: tls.VersionTLS13}
