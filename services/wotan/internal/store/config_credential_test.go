// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoHardcodedCredentialInSource guards gosec G101's old finding: the
// Postgres and hybrid stores (removed 2026-11-09) once fell back to a
// hardcoded "postgres://unheaded:unheaded_dev@...?sslmode=disable". This
// fails if that credential reappears in the package's non-test source.
func TestNoHardcodedCredentialInSource(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		// Split the needle so this test does not itself trip a secret scanner.
		needle := "unheaded:" + "unheaded_dev@"
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, needle) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				t.Errorf("%s: hardcoded credential reintroduced in non-comment code", name)
			}
		}
	}
}
