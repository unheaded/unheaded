// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "testing"

func TestUseColorHonoursNoColor(t *testing.T) {
	cases := []struct {
		flag bool
		env  string
		tty  bool
		want bool
	}{
		{false, "", true, true},
		{false, "1", true, false}, // NO_COLOR set: off (no-color.org)
		{false, "0", true, false}, // any non-empty value counts
		{true, "", true, false},   // --no-color
		{false, "", false, false}, // not a terminal
	}
	for _, c := range cases {
		if got := useColor(c.flag, c.env, c.tty); got != c.want {
			t.Errorf("useColor(%v, %q, %v) = %v, want %v", c.flag, c.env, c.tty, got, c.want)
		}
	}
}
