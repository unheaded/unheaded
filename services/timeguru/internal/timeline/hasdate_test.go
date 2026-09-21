// SPDX-License-Identifier: GPL-3.0-or-later
package timeline

import (
	"testing"
	"time"
)

// B9 review finding 3: a persisted "0001-01-01T00:00:00Z" decodes to a
// non-nil pointer at year 1. `!= nil` let it through as a due date.
func TestHasDate(t *testing.T) {
	zero := time.Time{}
	real := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	if HasDate(nil) {
		t.Error("nil must not count as a date")
	}
	if HasDate(&zero) {
		t.Error("a non-nil pointer to the zero time must not count as a date")
	}
	if !HasDate(&real) {
		t.Error("a real date must count")
	}
}
