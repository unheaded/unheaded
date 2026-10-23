// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import "testing"

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Add("wotan", 18000)
	if p, ok := r.Port("wotan"); !ok || p != 18000 {
		t.Fatalf("Port = %d %v", p, ok)
	}
	if _, ok := r.Port("nope"); ok {
		t.Fatal("unknown service found")
	}
}
