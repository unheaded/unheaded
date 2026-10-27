// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package middleware

import (
	"net"
	"strings"
	"testing"
)

// FuzzClientIP: the rate limiter's bucket key comes from a client-controlled
// RemoteAddr. Any string must be handled without a panic, and a valid
// host:port must key on the host alone, so a client cannot mint a fresh
// bucket per source port (S-CLEAN-SLATE Phase 2, Step 268).
func FuzzClientIP(f *testing.F) {
	for _, s := range []string{
		"192.0.2.1:5555", "[2001:db8::1]:443", "[::1]:0", "192.0.2.1", "2001:db8::1",
		"", ":", "[", "]:1", "host:port:extra", "[fe80::1%eth0]:80", "\x00:1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, addr string) {
		got := clientIP(addr)
		host, port, err := net.SplitHostPort(addr)
		if err == nil {
			if got != host {
				t.Fatalf("clientIP(%q) = %q, want host %q", addr, got, host)
			}
			if port != "" && strings.HasSuffix(got, ":"+port) && !strings.Contains(host, ":") {
				t.Fatalf("clientIP(%q) = %q kept the port", addr, got)
			}
			return
		}
		if got != addr {
			t.Fatalf("clientIP(%q) = %q, want the input unchanged when it has no port", addr, got)
		}
	})
}
