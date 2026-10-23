// SPDX-License-Identifier: GPL-3.0-or-later
// Two-brains eval fixture (deliberately buggy until solved).

package fixture

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigMissingIsNotExist(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("errors.Is(err, fs.ErrNotExist) = false for %v", err)
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("context lost: %v", err)
	}
}
