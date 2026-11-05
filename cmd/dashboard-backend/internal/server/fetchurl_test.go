// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// fetchURL answers for the address it was given, as the scrape and health
// paths do: a redirect is a failed fetch and its target is never asked.
func TestFetchURL_DoesNotFollowRedirects(t *testing.T) {
	var reached atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		_, _ = w.Write([]byte(`{"hostname":"elsewhere"}`))
	}))
	defer target.Close()
	for _, code := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, code)
		}))
		body, err := fetchURL(context.Background(), origin.URL)
		origin.Close()
		if err == nil {
			t.Fatalf("redirect %d: fetch succeeded with %q", code, body)
		}
	}
	if n := reached.Load(); n != 0 {
		t.Fatalf("redirect target asked %d times", n)
	}
}

func TestFetchURL_ReturnsTheBody(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("summary"))
	}))
	defer origin.Close()
	body, err := fetchURL(context.Background(), origin.URL)
	if err != nil || body != "summary" {
		t.Fatalf("fetchURL = %q, %v", body, err)
	}
}
