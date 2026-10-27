// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"unheaded/pkg/auth"
)

// Per-IP rate limit defaults (M-019): on unless -rate-limit 0.
const (
	defaultRateLimitRPS = 5.0
	defaultRateBurst    = 10
)

// errNoAuthToken is why zhen-agentd will not open its listener.
var errNoAuthToken = errors.New("no auth token set; set AUTH_ENABLED=true with AUTH_API_KEYS " +
	"(or AUTH_API_KEY_FILE, or AUTH_TRUST_DIR + AUTH_ALLOW_CALLERS for service tokens), " +
	"or pass -no-auth for a loopback dev listener")

// listenerAuth decides how the HTTP listener authenticates. With auth
// configured it returns the middleware (probes and the OpenAPI spec stay
// open); with -no-auth on a loopback host it returns nil, nil; anything
// else is an error and the daemon exits 2 rather than serve the gate's
// API to whoever can reach it.
func listenerAuth(cfg auth.ServiceAuthConfig, noAuth bool, host string) (func(http.Handler) http.Handler, error) {
	if noAuth {
		if cfg.Enabled {
			return nil, errors.New("-no-auth conflicts with AUTH_ENABLED=true; pass one or the other")
		}
		if !isLoopbackHost(host) {
			return nil, fmt.Errorf("-no-auth is only allowed on a loopback -host (127.0.0.1, ::1, localhost), not %q", host)
		}
		return nil, nil
	}
	if !cfg.Enabled {
		return nil, errNoAuthToken
	}
	// NewServiceMiddleware, not SetupMiddleware: a configuration it refuses
	// (no credential, the retired AUTH_JWT_KEY, an unreadable key file or
	// trust dir) stops the daemon instead of serving 401 to everyone.
	mw, err := auth.NewServiceMiddleware(cfg)
	if errors.Is(err, auth.ErrNoCredentials) {
		return nil, errNoAuthToken
	}
	if err != nil {
		return nil, err
	}
	if mw == nil { // unreachable while Enabled; kept so a change there fails closed
		return nil, errNoAuthToken
	}
	// SetupMiddleware exempts /health, /ready, /metrics; the spec is
	// public too, so clients can learn how to authenticate.
	return auth.SkipAuthPaths(mw, "/api/v1/openapi.json"), nil
}

// isLoopbackHost reports whether a -host value binds loopback only.
func isLoopbackHost(host string) bool {
	h := strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
