// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import (
	"fmt"
	"os"
)

// LoadConfig reads the config file at path.
func LoadConfig(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %v", path, err)
	}
	return b, nil
}
