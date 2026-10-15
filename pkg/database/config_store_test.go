// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package database

import (
	"context"
	"errors"
	"testing"
)

// Set refuses a missing reason before touching the database (a nil *sql.DB
// would panic if it did); The Well refuses one too (db/migrations/028,
// checked by scripts/well/test-migrate.sh).
func TestConfigStoreSetNeedsReason(t *testing.T) {
	s := NewConfigStore(nil)
	for _, reason := range []string{"", "  "} {
		if err := s.Set(context.Background(), "k", []byte(`1`), "t", reason); !errors.Is(err, ErrNoReason) {
			t.Errorf("reason %q: %v", reason, err)
		}
	}
}
