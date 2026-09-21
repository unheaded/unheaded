// SPDX-License-Identifier: GPL-3.0-or-later
package scraper

import (
	"fmt"
	"testing"
	"time"
)

// Pins the two allocation properties behind the 2026-09-21 OOM: a new series
// must not preallocate MaxSamples up front, and a full series must not let
// its backing array grow past a small multiple of MaxSamples.
func TestSeriesMemoryIsBounded(t *testing.T) {
	const max = 100
	s := &Scraper{config: &Config{MaxSamples: max}, series: map[string]*MetricSeries{}}

	s.storeSample(MetricSample{Name: "m", Service: "svc", Timestamp: time.Now()})
	series := s.series[seriesKey("m", nil)]
	if c := cap(series.Samples); c >= max {
		t.Fatalf("new series preallocated cap %d (MaxSamples=%d); must grow on demand", c, max)
	}

	for i := 0; i < 50*max; i++ {
		series.AddSample(MetricSample{Name: "m", Value: float64(i), Timestamp: time.Now()}, max)
	}
	if l := len(series.Samples); l != max {
		t.Fatalf("len after overflow: want %d, got %d", max, l)
	}
	if c := cap(series.Samples); c > 2*max {
		t.Fatalf("backing array cap %d exceeds 2*MaxSamples (%d): tail re-slice leak", c, 2*max)
	}
	// Order and content survive the in-place shift.
	want := fmt.Sprintf("%v", float64(50*max-1))
	if got := fmt.Sprintf("%v", series.Samples[max-1].Value); got != want {
		t.Fatalf("newest sample: want %s, got %s", want, got)
	}
	if got := series.Samples[0].Value; got != float64(50*max-max) {
		t.Fatalf("oldest retained sample: want %v, got %v", float64(50*max-max), got)
	}
}
