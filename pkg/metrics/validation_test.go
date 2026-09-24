// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Fatalf("want panic containing %q, got none", want)
		}
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %q, want it to contain %q", r, want)
		}
	}()
	f()
}

// The reproduction recorded in ADR-094: a misspelled key started a second
// series and an empty map published the bare name.
func TestVec_RejectsMismatchedLabels(t *testing.T) {
	cv := NewCounterVec("c_total", "c", nil, []string{"code"})
	mustPanic(t, `label "code" missing`, func() { cv.WithLabels(Labels{"cdoe": "200"}) })
	mustPanic(t, "got 0 labels", func() { cv.WithLabels(Labels{}) })
	mustPanic(t, "got 2 labels", func() { cv.WithLabels(Labels{"code": "200", "extra": "x"}) })

	gv := NewGaugeVec("g", "g", nil, []string{"host", "mount"})
	mustPanic(t, `label "mount" missing`, func() { gv.WithLabels(Labels{"host": "a", "mnt": "/"}) })

	hv := NewHistogramVec(HistogramOpts{Name: "h", Help: "h"}, []string{"method"})
	mustPanic(t, "got 0 labels", func() { hv.WithLabels(nil) })
}

func TestVec_AcceptsExactLabelsAndReusesSeries(t *testing.T) {
	cv := NewCounterVec("c_total", "c", nil, []string{"method", "code"})
	cv.WithLabels(Labels{"method": "GET", "code": "200"}).Inc()
	cv.WithLabels(Labels{"code": "200", "method": "GET"}).Inc() // same set, other order
	reg := NewRegistry()
	reg.MustRegister(cv)
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `c_total{code="200",method="GET"} 2`) {
		t.Errorf("want one series at 2, got:\n%s", buf.String())
	}
}

// Positional values map onto declared names in declaration order, not
// alphabetical order: "status" is declared before "method" on purpose.
func TestWithLabelValues_MapsInDeclaredOrder(t *testing.T) {
	cv := NewCounterVec("c_total", "c", nil, []string{"status", "method"})
	cv.WithLabelValues("500", "POST").Inc()
	if got := cv.WithLabels(Labels{"status": "500", "method": "POST"}).Value(); got != 1 {
		t.Errorf("positional and named lookups disagree: %g", got)
	}
	gv := NewGaugeVec("g", "g", nil, []string{"host"})
	gv.WithLabelValues("east").Set(3)
	if got := gv.WithLabels(Labels{"host": "east"}).Value(); got != 3 {
		t.Errorf("gauge: %g", got)
	}
	hv := NewHistogramVec(HistogramOpts{Name: "h", Help: "h", Buckets: []float64{1}}, []string{"path"})
	hv.WithLabelValues("/x").Observe(0.5)

	mustPanic(t, "got 1 label values, declared 2", func() { cv.WithLabelValues("500") })
	mustPanic(t, "got 3 label values, declared 2", func() { cv.WithLabelValues("a", "b", "c") })
}

// The reproduction recorded in ADR-094: {1, 1, +Inf} emitted le="1" twice
// and le="+Inf" twice, and Prometheus rejects the whole scrape.
func TestHistogram_BucketsNormalised(t *testing.T) {
	mustPanic(t, "duplicate histogram bucket 1", func() {
		NewHistogram(HistogramOpts{Name: "h", Buckets: []float64{1, 1, math.Inf(1)}})
	})
	mustPanic(t, "duplicate histogram bucket 2", func() {
		NewHistogramVec(HistogramOpts{Name: "h", Buckets: []float64{2, 1, 2}}, []string{"x"})
	})
	mustPanic(t, "NaN histogram bucket", func() {
		NewHistogram(HistogramOpts{Name: "h", Buckets: []float64{1, math.NaN()}})
	})

	for _, h := range []*Histogram{
		NewHistogram(HistogramOpts{Name: "h", Help: "h", Buckets: []float64{5, 1, math.Inf(1)}}),
		NewHistogramVec(HistogramOpts{Name: "h", Help: "h", Buckets: []float64{5, 1, math.Inf(1)}}, []string{"x"}).WithLabelValues("a"),
	} {
		h.Observe(3)
		var buf bytes.Buffer
		if err := h.Write(&buf); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if n := strings.Count(out, `le="+Inf"`); n != 1 {
			t.Errorf("le=\"+Inf\" appears %d times, want 1:\n%s", n, out)
		}
		if strings.Index(out, `le="1"`) > strings.Index(out, `le="5"`) {
			t.Errorf("buckets not sorted:\n%s", out)
		}
		if bucketCount(out, "1") != "0" || bucketCount(out, "5") != "1" {
			t.Errorf("wrong counts:\n%s", out)
		}
	}
}

// bucketCount returns the value on the line carrying le="<le>", with or
// without other labels after it.
func bucketCount(out, le string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, `le="`+le+`"`) {
			return line[strings.LastIndexByte(line, ' ')+1:]
		}
	}
	return ""
}

// normalizeBuckets copies; it must never reorder the caller's slice or the
// shared DefaultBuckets.
func TestNormalizeBuckets_DoesNotMutateInput(t *testing.T) {
	in := []float64{3, 1, 2}
	normalizeBuckets("h", in)
	if in[0] != 3 || in[1] != 1 || in[2] != 2 {
		t.Errorf("input reordered: %v", in)
	}
	before := append([]float64(nil), DefaultBuckets...)
	out := normalizeBuckets("h", nil)
	out[0] = -1
	for i := range before {
		if DefaultBuckets[i] != before[i] {
			t.Fatalf("DefaultBuckets modified through the returned slice")
		}
	}
}

// le and quantile are written by the metric itself; a user label of the
// same name would publish two values under one label.
func TestReservedLabels(t *testing.T) {
	mustPanic(t, `label "le" is reserved`, func() {
		NewHistogramVec(HistogramOpts{Name: "h"}, []string{"le"})
	})
	mustPanic(t, `label "le" is reserved`, func() {
		NewHistogram(HistogramOpts{Name: "h", ConstLabels: Labels{"le": "x"}})
	})
	mustPanic(t, `label "quantile" is reserved`, func() {
		NewSummary(SummaryOpts{Name: "s", ConstLabels: Labels{"quantile": "x"}})
	})
}

// A family with no samples is omitted, HELP and TYPE included, as
// client_golang does. Pinned here, not only in the prom parity test, so it
// outlives client_golang (ADR-094 step 6).
func TestGather_OmitsFamiliesWithNoSamples(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(NewCounterVec("untouched_total", "never incremented", nil, []string{"x"}))
	used := NewCounterVec("used_total", "incremented", nil, []string{"x"})
	reg.MustRegister(used)
	used.WithLabelValues("a").Inc()

	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "untouched_total") {
		t.Errorf("empty family published:\n%s", out)
	}
	if !strings.Contains(out, "# TYPE used_total counter") || !strings.Contains(out, `used_total{x="a"} 1`) {
		t.Errorf("used family missing:\n%s", out)
	}
}
