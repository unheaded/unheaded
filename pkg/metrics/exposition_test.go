// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"unheaded/pkg/metrics/metricstest"
)

func gatherString(t *testing.T, r *Registry) string {
	t.Helper()
	var buf bytes.Buffer
	if err := r.Gather(&buf); err != nil {
		t.Fatalf("Gather: %v", err)
	}
	return buf.String()
}

// A newline in help text ended the HELP line early; the rest of the help
// became a "sample" line, and a parser rejects the whole page. The text
// format requires \ and newline escaped in HELP, as client_golang does.
func TestHelpIsEscaped(t *testing.T) {
	r := NewRegistry()
	c := NewCounter("help_escape_total", "first line\nsecond line, a \\ backslash", nil)
	c.Inc()
	r.MustRegister(c)

	out := gatherString(t, r)
	want := `# HELP help_escape_total first line\nsecond line, a \\ backslash` + "\n"
	if !strings.Contains(out, want) {
		t.Fatalf("HELP not escaped; want line %q in:\n%s", want, out)
	}
	if problems := metricstest.Lint(out); problems != nil {
		t.Fatalf("exposition does not lint: %v\n%s", problems, out)
	}
}

// Prometheus rejects a scrape containing a label value that is not valid
// UTF-8 — every series on the page, not just the bad one.
func TestLabelValue_InvalidUTF8DoesNotPoisonThePage(t *testing.T) {
	r := NewRegistry()
	gv := NewGaugeVec("utf8_gauge", "h", nil, []string{"v"})
	r.MustRegister(gv)
	gv.WithLabelValues("ok\xff\xfeend").Set(1)

	out := gatherString(t, r)
	if !utf8.ValidString(out) {
		t.Fatalf("exposition is not valid UTF-8:\n%q", out)
	}
	if !strings.Contains(out, `utf8_gauge{v="ok`+"�"+`end"} 1`) &&
		!strings.Contains(out, `utf8_gauge{v="ok`+"��"+`end"} 1`) {
		t.Fatalf("invalid bytes not replaced:\n%q", out)
	}
}

// Labels.String is the series key. Unquoted, {a="1,b=2", b="3"} and
// {a="1", b="2,b=3"} both keyed as a=1,b=2,b=3: the second call got the
// first series back and its increments were published under the first's
// labels.
func TestVec_LabelValuesContainingSeparatorsAreDistinctSeries(t *testing.T) {
	r := NewRegistry()
	cv := NewCounterVec("sep_total", "h", nil, []string{"a", "b"})
	r.MustRegister(cv)

	cv.WithLabels(Labels{"a": "1,b=2", "b": "3"}).Inc()
	cv.WithLabels(Labels{"a": "1", "b": "2,b=3"}).Add(5)

	out := gatherString(t, r)
	for _, want := range []string{
		`sep_total{a="1,b=2",b="3"} 1`,
		`sep_total{a="1",b="2,b=3"} 5`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// parseHistogram returns the bucket counts in output order, the +Inf bucket
// and _count of the single histogram called name in out.
func parseHistogram(t *testing.T, out, name string) (buckets []uint64, inf, count uint64) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		v, err := strconv.ParseUint(f[1], 10, 64)
		switch {
		case strings.HasPrefix(f[0], name+`_bucket{le="+Inf"}`):
			if err != nil {
				t.Fatalf("bad line %q", line)
			}
			inf = v
		case strings.HasPrefix(f[0], name+"_bucket{"):
			if err != nil {
				t.Fatalf("bad line %q", line)
			}
			buckets = append(buckets, v)
		case f[0] == name+"_count":
			if err != nil {
				t.Fatalf("bad line %q", line)
			}
			count = v
		}
	}
	return buckets, inf, count
}

// A scrape taken while observations land must still be a valid histogram:
// buckets cumulative (never decreasing), none above +Inf, +Inf == _count.
// Buckets were read one by one from separately incremented cumulative
// counters, and the total from a third, so a scrape could see an
// observation in le="1" but not yet in le="2.5" or in the total.
func TestHistogram_ScrapeDuringObserveIsConsistent(t *testing.T) {
	h := NewHistogram(HistogramOpts{Name: "race_seconds", Help: "h"})
	r := NewRegistry()
	r.MustRegister(h)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					h.Observe(0.001)
				}
			}
		}()
	}

	bad := ""
	// Time-bounded rather than a fixed count: the race window is a few
	// instructions wide, and a second of scrapes catches it reliably.
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && bad == ""; {
		out := gatherString(t, r)
		buckets, inf, count := parseHistogram(t, out, "race_seconds")
		if inf != count {
			bad = "+Inf != _count\n" + out
		}
		var prev uint64
		for _, b := range buckets {
			if b < prev {
				bad = "buckets not cumulative\n" + out
			}
			if b > inf {
				bad = "bucket above +Inf\n" + out
			}
			prev = b
		}
	}
	close(stop)
	wg.Wait()
	if bad != "" {
		t.Fatal(bad)
	}
}

// The non-cumulative storage must still place a value equal to a bound in
// that bound's bucket (le is "less than or equal"), and NaN only in +Inf.
func TestHistogram_BoundaryAndNaNPlacement(t *testing.T) {
	h := NewHistogram(HistogramOpts{Name: "edge", Help: "h", Buckets: []float64{1, 2}})
	r := NewRegistry()
	r.MustRegister(h)
	for _, v := range []float64{1, 2, 2.5, 0.5} {
		h.Observe(v)
	}
	h.Observe(nan())

	out := gatherString(t, r)
	for _, want := range []string{
		`edge_bucket{le="1"} 2`,
		`edge_bucket{le="2"} 3`,
		`edge_bucket{le="+Inf"} 5`,
		`edge_count 5`,
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func nan() float64 {
	zero := 0.0
	return zero / zero
}
