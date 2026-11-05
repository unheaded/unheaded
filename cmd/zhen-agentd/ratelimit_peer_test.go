// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"net/http/httptest"
	"testing"
)

// The limiter runs before auth, so what names a client must not be something
// the client wrote. Two requests from one peer are one client.
func TestClientIP_IgnoresForwardedHeaders(t *testing.T) {
	first := httptest.NewRequest("GET", "/", nil)
	first.RemoteAddr = "192.0.2.10:4000"
	first.Header.Set("X-Forwarded-For", "198.51.100.1")
	second := httptest.NewRequest("GET", "/", nil)
	second.RemoteAddr = "192.0.2.10:4001"
	second.Header.Set("X-Forwarded-For", "198.51.100.2")

	if clientIP(first) != "192.0.2.10" || clientIP(second) != "192.0.2.10" {
		t.Fatalf("got %q and %q, want the peer for both", clientIP(first), clientIP(second))
	}
}
