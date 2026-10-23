// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import "testing"

func TestJoinFields(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{{nil, ""}, {[]string{"a"}, "a"}, {[]string{"a", "b"}, "a,b"}, {[]string{"", ""}, ","}} {
		if got := JoinFields(tc.in, ","); got != tc.want {
			t.Errorf("JoinFields(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
