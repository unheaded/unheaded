// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// SameOriginOrAllowed returns a websocket.Upgrader CheckOrigin. A browser
// always sends Origin on a WebSocket handshake, and the page it names can
// read everything the socket sends: so a request passes only when Origin is
// absent (a non-browser client, which no web page can impersonate), when its
// host is the request's Host (same origin), or when it exactly matches an
// entry of allowed (scheme://host[:port]). "*" is never honoured.
func SameOriginOrAllowed(allowed []string) func(*http.Request) bool {
	ok := map[string]bool{}
	for _, o := range allowed {
		if o != "" && o != "*" {
			ok[o] = true
		}
	}
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		if ok[origin] {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	}
}

// OriginsFromEnv reads a comma-separated origin list from the environment
// variable name; blanks and "*" are dropped.
func OriginsFromEnv(name string) []string {
	var out []string
	for _, o := range strings.Split(os.Getenv(name), ",") {
		if o = strings.TrimSpace(o); o != "" && o != "*" {
			out = append(out, o)
		}
	}
	return out
}
