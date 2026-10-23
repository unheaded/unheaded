// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

// Classify names an HTTP status for the retry logic.
func Classify(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "ok"
	case code == 404:
		return "not_found"
	case code >= 500:
		return "server_error"
	case code >= 400:
		return "client_error"
	}
	return "unknown"
}
