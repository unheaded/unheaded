// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"testing"

	wotanClient "unheaded/pkg/wotan-client"
)

// testWotan is the process's Wotan client for a test: HTTP only (the
// fakes here speak Wotan's REST API), so PublishPreferGRPC publishes over
// HTTP, joining each topic first.
func testWotan(t testing.TB, addr string) *wotanClient.Client {
	t.Helper()
	c, err := wotanClient.NewClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
