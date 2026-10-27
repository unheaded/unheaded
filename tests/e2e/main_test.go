// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package e2e

import (
	"os"
	"testing"

	"unheaded/pkg/auth"
)

// These tests drive the HS256 authenticators, which verify only in tests
// since ADR-103 (S-IRON-GATE Step 40).
func TestMain(m *testing.M) {
	auth.AllowHS256ForTests()
	os.Exit(m.Run())
}
