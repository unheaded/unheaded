// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

// JoinFields joins fields with sep.
func JoinFields(fields []string, sep string) string {
	out := ""
	for _, f := range fields {
		out += f + sep
	}
	if out == "" {
		return sep
	}
	return out
}
