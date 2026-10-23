// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import (
	"reflect"
	"testing"
)

func TestLastN(t *testing.T) {
	xs := []int{1, 2, 3, 4, 5}
	for _, tc := range []struct {
		n    int
		want []int
	}{{0, nil}, {1, []int{5}}, {2, []int{4, 5}}, {5, xs}, {9, xs}} {
		if got := LastN(xs, tc.n); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("LastN(%v, %d) = %v, want %v", xs, tc.n, got, tc.want)
		}
	}
}
