// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"net/http/httptest"
	"testing"
)

func TestSameOriginOrAllowed(t *testing.T) {
	check := SameOriginOrAllowed([]string{"http://console.lan:8080"})
	for _, c := range []struct {
		name, host, origin string
		want               bool
	}{
		{"no Origin (CLI client)", "west:26100", "", true},
		{"same origin", "west:26100", "http://west:26100", true},
		{"same origin, case differs", "WEST:26100", "http://west:26100", true},
		{"allowlisted", "west:26100", "http://console.lan:8080", true},
		{"foreign page", "west:26100", "https://evil.example", false},
		{"same host, other port", "west:26100", "http://west:8080", false},
		{"allowlist is exact", "west:26100", "http://console.lan:8081", false},
		{"opaque origin", "west:26100", "null", false},
		{"garbage", "west:26100", "://", false},
	} {
		r := httptest.NewRequest("GET", "http://"+c.host+"/ws", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := check(r); got != c.want {
			t.Errorf("%s: Origin %q to %s = %v, want %v", c.name, c.origin, c.host, got, c.want)
		}
	}
}

func TestOriginsFromEnv(t *testing.T) {
	t.Setenv("WS_ALLOWED_ORIGINS", " http://a:1 ,,*, http://b ")
	got := OriginsFromEnv("WS_ALLOWED_ORIGINS")
	if len(got) != 2 || got[0] != "http://a:1" || got[1] != "http://b" {
		t.Fatalf("OriginsFromEnv = %q, want [http://a:1 http://b] (blanks and * dropped)", got)
	}
}
