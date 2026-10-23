// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

// LastN returns the last n elements of xs.
func LastN(xs []int, n int) []int {
	if n <= 0 {
		return nil
	}
	if n >= len(xs) {
		return xs
	}
	return xs[len(xs)-n-1:]
}
