// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package crosswalk

import "testing"

// The order worse() holds. No source in today's catalog is shared between a
// declared and a plain control, so nothing live exercises it: DECLARED sits
// just below PASS and above every status that says something is wrong.
func TestWorse_DeclaredSitsBelowPassAndAboveTheRest(t *testing.T) {
	order := []Status{StatusFail, StatusNotAssessed, StatusStale, StatusDeclared, StatusPass}
	for i, a := range order {
		for j, b := range order {
			if got, want := worse(a, b), i < j; got != want {
				t.Errorf("worse(%s, %s) = %v, want %v", a, b, got, want)
			}
		}
	}
}
