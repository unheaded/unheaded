// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import "testing"

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		code int
		want string
	}{
		{200, "ok"}, {204, "ok"}, {404, "not_found"}, {400, "client_error"},
		{500, "server_error"}, {302, "unknown"},
		{429, "rate_limited"}, {503, "unavailable"},
	} {
		if got := Classify(tc.code); got != tc.want {
			t.Errorf("Classify(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}
