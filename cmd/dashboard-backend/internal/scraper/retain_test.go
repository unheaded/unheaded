// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package scraper

import (
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// Parsed names and label values are substrings of the scrape body, and the
// label map is new per scrape. Stored per sample, they pinned every scrape
// body inside the retention window: ~7,000 series x 240 samples/hour
// OOM-killed the backend at its 768 MB limit after an hour (2026-09-29).
// Samples in a series now share the series' own copies.
func TestStoreSample_DoesNotRetainScrapeBodies(t *testing.T) {
	s := newTestScraper(t)
	var bodies []string
	for i := 0; i < 3; i++ {
		body := strings.Repeat("x", 1<<16) + `wotan_up{job="w"} 1` // one scrape's text
		bodies = append(bodies, body)
		line := body[1<<16:]
		s.storeSample(MetricSample{
			Name:      line[:8],
			Labels:    map[string]string{"job": line[14:15]},
			Service:   "wotan",
			Value:     float64(i),
			Timestamp: time.Now(),
		})
	}
	got := s.QueryMetrics("wotan_up", map[string]string{"job": "w"}, time.Time{})
	if len(got) != 3 {
		t.Fatalf("stored %d samples, want 3", len(got))
	}
	m0 := reflect.ValueOf(got[0].Labels).Pointer()
	for i, smp := range got {
		if reflect.ValueOf(smp.Labels).Pointer() != m0 {
			t.Errorf("sample %d has its own label map", i)
		}
		for _, b := range bodies {
			lo := uintptr(unsafe.Pointer(unsafe.StringData(b)))
			hi := lo + uintptr(len(b))
			for _, str := range []string{smp.Name, smp.Labels["job"]} {
				p := uintptr(unsafe.Pointer(unsafe.StringData(str)))
				if p >= lo && p < hi {
					t.Fatalf("sample %d holds %q inside a scrape body", i, str)
				}
			}
		}
	}
}
