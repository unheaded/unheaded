// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// TestMain gives every server a test builds its own empty store: the SQLite
// fixture (sqlite_store_test.go, seeded from KANBAN_GENESIS_FILE when a test
// sets it) in a directory of its own, in place of the
// Well. Production has only the Well (well_wait.go); before ADR-147 Phase
// 16 these tests reached the same SQLite store through the fallback that
// production no longer has.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "kanban-app-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var n atomic.Int64
	connectTaskStore = func(context.Context) (TaskStore, error) {
		return openSQLiteStore(filepath.Join(root, fmt.Sprintf("store-%d", n.Add(1)), "kanban.db"))
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
