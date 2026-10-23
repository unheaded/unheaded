// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import (
	"fmt"
	"io"
)

// WriteReport writes one line per entry to w.
func WriteReport(w io.Writer, lines []string) error {
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
	return nil
}
