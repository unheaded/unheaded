// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

// Counter counts events.
type Counter struct {
	n int
}

// Inc adds one.
func (c *Counter) Inc() { c.n++ }

// Value returns the count.
func (c *Counter) Value() int { return c.n }
