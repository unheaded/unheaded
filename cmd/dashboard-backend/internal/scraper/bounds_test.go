// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// L10: a metrics answer is bounded. A larger one is an error, not a partial
// read that scrapes as if it were whole.
func TestScrape_OversizedBodyIsAnError(t *testing.T) {
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		line := "metric_a 1\n"
		chunk := strings.Repeat(line, 1<<16)
		for sent := 0; sent <= maxScrapeBody; sent += len(chunk) {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer hs.Close()

	s := newTestScraper(t)
	target := makeTarget("big", hs.URL)
	if err := s.RegisterTarget(target); err != nil {
		t.Fatal(err)
	}
	s.scrapeTarget(context.Background(), target)
	got, ok := s.GetLatestResults()["big"]
	if !ok || got.Error == "" {
		t.Fatalf("result = %+v, %v; want an error", got, ok)
	}
	if len(got.Metrics) != 0 {
		t.Fatalf("%d samples kept from a truncated body", len(got.Metrics))
	}
}

// L10: a target answers for itself: a redirect is not followed elsewhere.
func TestScrape_RedirectIsNotFollowed(t *testing.T) {
	var reached bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		_, _ = w.Write([]byte("metric_a 1\n"))
	}))
	defer elsewhere.Close()
	hs := httptest.NewServer(http.RedirectHandler(elsewhere.URL, http.StatusFound))
	defer hs.Close()

	s := newTestScraper(t)
	target := makeTarget("moved", hs.URL)
	if err := s.RegisterTarget(target); err != nil {
		t.Fatal(err)
	}
	s.scrapeTarget(context.Background(), target)
	if reached {
		t.Fatal("the scraper followed a redirect")
	}
}
