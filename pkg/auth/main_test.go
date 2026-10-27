// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"os"
	"testing"
)

// The HS256 tests in this package predate ADR-103. HS256 verification is
// test-only now, so the package opts in once here.
func TestMain(m *testing.M) {
	AllowHS256ForTests()
	os.Exit(m.Run())
}
