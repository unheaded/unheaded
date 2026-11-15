// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import "testing"

// zhen-agentd's API and metrics listeners obey the module bind rule
// (ADR-149, D5 (a)): loopback, or a private address on the host's allowlist.
func TestCheckListenAddrs(t *testing.T) {
	cases := []struct {
		name, host, metrics, allow string
		ok                         bool
	}{
		{"loopback, no metrics", "127.0.0.1", "", "", true},
		{"wildcard api", "0.0.0.0", "", "", false},
		{"v6 wildcard api", "::", "", "", false},
		{"metrics on wireguard, not listed", "127.0.0.1", "[fd00:dead:beef::1]:20111", "", false},
		{"metrics on wireguard, listed", "127.0.0.1", "[fd00:dead:beef::1]:20111", "fd00:dead:beef::1/128", true},
		{"global prefix listed", "127.0.0.1", "", "8.8.8.0/24", false},
		{"global api", "8.8.8.8", "", "", false},
		{"ipv6 loopback api", "::1", "", "", true},
		{"hostname", "localhost", "", "", false},
	}
	for _, c := range cases {
		err := checkListenAddrs(c.host, 20111, c.metrics, c.allow)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
