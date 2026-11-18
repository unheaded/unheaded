// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// callerKey is a request context key the tests use to stand in for the
// verified identity pkg/auth puts in the context (its key is unexported).
type callerKey struct{}

func callerLimiter(t *testing.T) *RateLimiter {
	t.Helper()
	rl := NewRateLimiter(rate.Limit(1), 1, time.Hour)
	rl.subject = func(r *http.Request) string {
		s, _ := r.Context().Value(callerKey{}).(string)
		return s
	}
	return rl
}

func hit(rl *RateLimiter, remote, caller string) int {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/", nil)
	req.RemoteAddr = remote
	if caller != "" {
		req = req.WithContext(context.WithValue(req.Context(), callerKey{}, caller))
	}
	rec := httptest.NewRecorder()
	rl.Middleware(ok).ServeHTTP(rec, req)
	return rec.Code
}

// Wotan's callers in compose share the bridge address: one bucket per
// address made every service pay for the busiest one (BlackMage F5). The
// bucket is per verified service identity.
func TestBucketPerVerifiedCaller(t *testing.T) {
	rl := callerLimiter(t)
	if c := hit(rl, "172.28.1.1:5000", "kanban-app"); c != http.StatusOK {
		t.Fatalf("kanban-app first: %d", c)
	}
	if c := hit(rl, "172.28.1.1:5001", "dashboard-backend"); c != http.StatusOK {
		t.Fatalf("dashboard-backend from the same address: %d, want its own bucket", c)
	}
	if c := hit(rl, "172.28.1.1:5002", "kanban-app"); c != http.StatusTooManyRequests {
		t.Fatalf("kanban-app second: %d, want 429", c)
	}
}

func TestNoIdentityFallsBackToIP(t *testing.T) {
	rl := callerLimiter(t)
	if c := hit(rl, "172.28.1.9:5000", ""); c != http.StatusOK {
		t.Fatalf("first: %d", c)
	}
	if c := hit(rl, "172.28.1.9:5001", ""); c != http.StatusTooManyRequests {
		t.Fatalf("second from the same address: %d, want 429", c)
	}
}

func TestKeysDoNotCollide(t *testing.T) {
	rl := callerLimiter(t)
	if c := hit(rl, "10.0.0.1:1", "172.28.1.1"); c != http.StatusOK {
		t.Fatalf("identity named like an address: %d", c)
	}
	if c := hit(rl, "172.28.1.1:2", ""); c != http.StatusOK {
		t.Fatalf("unauthenticated request from that address: %d, want its own bucket", c)
	}
}

// The default reads pkg/auth's verified service subject; with no identity
// in the context it is empty, so the address decides.
func TestDefaultSubjectWithoutIdentity(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 1, time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := rl.limitKey(req); got != "ip:192.0.2.1" {
		t.Fatalf("limitKey = %q, want ip:192.0.2.1", got)
	}
}
