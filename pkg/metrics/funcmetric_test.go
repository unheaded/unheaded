// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"
)

// Services hold their counters as plain fields and want to expose them
// without rewriting every increment site. A func-backed metric reads the
// value at scrape time.
func TestFuncCounter_ReadsAtScrapeTime(t *testing.T) {
	var n atomic.Int64
	reg := NewRegistry()
	if err := reg.Register(NewFuncCounter("svc_things_total", "Things done", nil, func() float64 {
		return float64(n.Load())
	})); err != nil {
		t.Fatalf("register: %v", err)
	}

	var first bytes.Buffer
	if err := reg.Gather(&first); err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !strings.Contains(first.String(), "svc_things_total 0") {
		t.Errorf("want 0 before increment, got:\n%s", first.String())
	}

	n.Store(7)

	var second bytes.Buffer
	if err := reg.Gather(&second); err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !strings.Contains(second.String(), "svc_things_total 7") {
		t.Errorf("want 7 after increment, got:\n%s", second.String())
	}
}

// The registry writes HELP and TYPE from Describe(); a collector that writes
// them too produces duplicates, which the text format forbids. This is the
// bug that shipped in pkg/logagg's first collector.
func TestFuncMetric_EmitsExactlyOneHelpAndType(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(NewFuncGauge("svc_temp", "Temperature", nil, func() float64 { return 1 }))

	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := buf.String()

	if got := strings.Count(out, "# HELP svc_temp "); got != 1 {
		t.Errorf("# HELP appears %d times, want 1:\n%s", got, out)
	}
	if got := strings.Count(out, "# TYPE svc_temp "); got != 1 {
		t.Errorf("# TYPE appears %d times, want 1:\n%s", got, out)
	}
	if !strings.Contains(out, "# TYPE svc_temp gauge") {
		t.Errorf("gauge not declared as a gauge:\n%s", out)
	}
}

func TestFuncCounter_DeclaredAsCounter(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(NewFuncCounter("svc_total", "Total", nil, func() float64 { return 3 }))

	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !strings.Contains(buf.String(), "# TYPE svc_total counter") {
		t.Errorf("counter not declared as a counter:\n%s", buf.String())
	}
}

// A nil read function must not panic a scrape — an unavailable metric is
// better than a dead /metrics endpoint.
func TestFuncMetric_NilReadIsSafe(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(NewFuncGauge("svc_nil", "Nil read", nil, nil))

	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatalf("gather with a nil read func: %v", err)
	}
	if !strings.Contains(buf.String(), "svc_nil 0") {
		t.Errorf("a nil read should report 0, got:\n%s", buf.String())
	}
}

// Const labels must appear on the sample line.
func TestFuncMetric_ConstLabels(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(NewFuncGauge("svc_labelled", "Labelled", Labels{"service": "monad"}, func() float64 { return 2 }))

	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatalf("gather: %v", err)
	}
	if !strings.Contains(buf.String(), `svc_labelled{service="monad"} 2`) {
		t.Errorf("const label missing:\n%s", buf.String())
	}
}
