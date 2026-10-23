// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import "testing"

// The Logs page reads this buffer. NewServer used to fill it with six
// startup lines nobody logged ("wotan transport connected" whether or not
// it was) and then, every 15 s, a "health check OK" for dashboard-backend,
// wotan, kanban-app and wiki-server without checking anything. On east the
// whole 10,000-entry buffer was those lines. Nothing real feeds the buffer
// yet, so an honest one starts empty.
func TestNewServer_LogBufferHoldsNoInventedEntries(t *testing.T) {
	srv := newTestServer(t)
	if n := srv.logBuffer.Len(); n != 0 {
		entries, _ := srv.logBuffer.Since(0)
		t.Fatalf("log buffer holds %d entries nobody logged: %+v", n, entries)
	}
}
