// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import "context"

// Retry calls f up to attempts times until it succeeds.
func Retry(ctx context.Context, attempts int, f func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = f(); err == nil {
			return nil
		}
	}
	return err
}
