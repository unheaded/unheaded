// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package cmdline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryBinaryUsesCmdline is the ratchet: every Go program under cmd/
// must go through this package for its help, version and usage errors, so
// a new binary cannot ship with the flag package's bare defaults. Add the
// call; do not add an exception.
func TestEveryBinaryUsesCmdline(t *testing.T) {
	root := filepath.Join("..", "..", "cmd")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("cmd/ not reachable from the test directory: %v", err)
	}
	var missing []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		hasMain, usesCmdline := false, false
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			src := string(b)
			if strings.Contains(src, "\nfunc main()") {
				hasMain = true
			}
			if strings.Contains(src, `"unheaded/pkg/cmdline"`) {
				usesCmdline = true
			}
		}
		// A binary whose dispatcher lives in a sub-package (unheaded-cli)
		// may import cmdline there instead.
		if hasMain && !usesCmdline {
			sub, _ := filepath.Glob(filepath.Join(dir, "*", "*.go"))
			for _, f := range sub {
				if b, err := os.ReadFile(f); err == nil && strings.Contains(string(b), `"unheaded/pkg/cmdline"`) {
					usesCmdline = true
					break
				}
			}
		}
		if hasMain && !usesCmdline {
			missing = append(missing, e.Name())
		}
	}
	if len(missing) > 0 {
		t.Errorf("binaries without pkg/cmdline (help/version/usage-error conventions): %v", missing)
	}
}
