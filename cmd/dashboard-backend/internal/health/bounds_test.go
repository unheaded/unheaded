// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// pollOnce is the status of one check. ServiceHealth moves only after a
// run of checks, so it would say nothing here.
func pollOnce(t *testing.T, url string) Status {
	t.Helper()
	m := newTestMonitor(t)
	target := &ServiceTarget{Name: "svc", HealthURL: url, Enabled: true}
	if err := m.RegisterTarget(target); err != nil {
		t.Fatal(err)
	}
	m.pollTarget(context.Background(), target)
	got, err := m.GetHistory("svc", time.Time{})
	if err != nil || len(got) != 1 {
		t.Fatalf("history = %+v, %v", got, err)
	}
	return got[0].Status
}

// L10: a health answer is bounded, and one past the bound is not healthy.
func TestPoll_OversizedBodyIsNotHealthy(t *testing.T) {
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","pad":"` + strings.Repeat("a", maxHealthBody) + `"}`))
	}))
	defer hs.Close()
	if got := pollOnce(t, hs.URL); got == StatusHealthy {
		t.Fatalf("oversized answer scored %q", got)
	}
}

// L10: a target answers for itself: a redirect is not followed elsewhere.
func TestPoll_RedirectIsNotFollowed(t *testing.T) {
	elsewhere := healthyServer()
	defer elsewhere.Close()
	hs := httptest.NewServer(http.RedirectHandler(elsewhere.URL, http.StatusFound))
	defer hs.Close()
	if got := pollOnce(t, hs.URL); got == StatusHealthy {
		t.Fatalf("a redirect to a healthy host scored %q", got)
	}
}
