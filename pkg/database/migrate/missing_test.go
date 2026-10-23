// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package migrate

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lib/pq"
)

// Only "database does not exist" makes a ledger empty; any other refusal
// (a wrong password, say) stays an error.
func TestIsMissingDatabase(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{&pq.Error{Code: "3D000"}, true},
		{fmt.Errorf("x: %w", &pq.Error{Code: "3D000"}), true},
		{&pq.Error{Code: "28P01"}, false},
		{errors.New(`database "x" does not exist (3D000)`), false},
		{nil, false},
	} {
		if got := isMissingDatabase(c.err); got != c.want {
			t.Errorf("isMissingDatabase(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
