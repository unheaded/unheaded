// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A broken config file must be named in the error, with yaml's line, so
// the operator knows which file to fix (S-LOW-FRUIT Phase 6).
func TestLoadConfigErrorNamesFileAndLine(t *testing.T) {
	cases := map[string]string{
		"unclosed flow sequence": "once: [\n",
		"tab indentation":        "a:\n\tb: 1\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "huginn.yaml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := loadConfig(path)
			if err == nil {
				t.Fatal("broken YAML loaded without error")
			}
			if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "line") {
				t.Errorf("error %q should name %s and a line", err, path)
			}
		})
	}
}

func TestLoadConfigMissingFileIsOptional(t *testing.T) {
	if _, err := loadConfig(filepath.Join(t.TempDir(), "absent.yaml")); err != nil {
		t.Errorf("a missing config file is optional, got %v", err)
	}
}
