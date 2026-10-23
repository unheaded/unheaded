// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import tlsx "crypto/tls"

// (d) crypto/tls imported under another name.
var D = &tlsx.Config{
	MinVersion:       tlsx.VersionTLS13,
	CurvePreferences: []tlsx.CurveID{tlsx.CurveP384},
}
