// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// L8: a proxy may add its own header line rather than join the client's, and
// a client's line comes first. All lines are one list, read from the nearest
// hop, so the client's own line cannot decide the answer.
func TestClientIP_ReadsEveryForwardedLine(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:443"
	r.Header.Add("X-Forwarded-For", "203.0.113.66")
	r.Header.Add("X-Forwarded-For", "198.51.100.7")
	if got := ClientIP(r, TrustedProxySet([]string{"192.0.2.1"})); got != "198.51.100.7" {
		t.Fatalf("ClientIP = %q, want the address the proxy saw", got)
	}
}
