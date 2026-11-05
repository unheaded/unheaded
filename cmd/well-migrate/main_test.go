// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The runner connects as the superuser: an sslmode that could fall back to
// plaintext is refused before any connection (port 1: nothing listens, so
// an attempt would fail differently).
func TestRun_RefusesWeakSSLMode(t *testing.T) {
	for _, mode := range []string{"prefer", "allow", ""} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("WELL_HOST", "127.0.0.1")
			t.Setenv("WELL_PORT", "1")
			t.Setenv("WELL_SSLMODE", mode)
			t.Setenv("WELL_MIGRATE_PASSWORD", "x")
			var out, errb bytes.Buffer
			code := run(context.Background(), []string{"-dir", "../../db/migrations", "status"}, &out, &errb)
			if code != 1 || !strings.Contains(errb.String(), "sslmode") {
				t.Fatalf("exit %d, stderr %q: want 1 and an sslmode refusal", code, errb.String())
			}
		})
	}
}

// M12: the CA path reaches the check. A named file that is missing stops the
// run before any connection, and the message names that file.
func TestRun_RefusesMissingCA(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone.crt")
	t.Setenv("WELL_HOST", "127.0.0.1")
	t.Setenv("WELL_PORT", "1")
	t.Setenv("WELL_SSLMODE", "verify-full")
	t.Setenv("WELL_SSLROOTCERT", gone)
	t.Setenv("WELL_MIGRATE_PASSWORD", "x")
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"-dir", "../../db/migrations", "status"}, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), gone) {
		t.Fatalf("exit %d, stderr %q: want 1 and a refusal naming %s", code, errb.String(), gone)
	}
}
