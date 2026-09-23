// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"fmt"
	"io"
)

// FuncMetric is a metric whose value is read from a function at scrape time
// rather than held in the metric itself.
//
// It exists because services here already hold their counters as plain
// fields — an int64 behind a mutex, a value derived from a Stats() call — and
// exposing those should not require rewriting every increment site. A
// func-backed metric wraps what the service already has.
//
// This is what lets a service move off hand-written exposition. Before it,
// four services built their /metrics body with fmt.Sprintf, which is how you
// get a missing # TYPE, an unescaped label value, or the same # HELP twice —
// all of which the text format forbids and parsers reject. The registry
// writes HELP and TYPE from Describe(); this type writes only the sample.
type FuncMetric struct {
	desc *Desc
	read func() float64
}

// NewFuncCounter returns a counter whose value is read at scrape time.
//
// A nil read function reports 0 rather than panicking: an unavailable metric
// is better than a /metrics endpoint that dies mid-scrape and takes every
// other series on the page with it.
func NewFuncCounter(name, help string, constLabels Labels, read func() float64) *FuncMetric {
	return &FuncMetric{
		desc: NewDesc(name, help, TypeCounter, constLabels, nil),
		read: read,
	}
}

// NewFuncGauge returns a gauge whose value is read at scrape time.
func NewFuncGauge(name, help string, constLabels Labels, read func() float64) *FuncMetric {
	return &FuncMetric{
		desc: NewDesc(name, help, TypeGauge, constLabels, nil),
		read: read,
	}
}

// Describe returns the metric's descriptor.
func (f *FuncMetric) Describe() *Desc {
	return f.desc
}

// Write emits the sample line only. Registry.Gather has already written
// # HELP and # TYPE from Describe(); emitting them here too produces
// duplicates.
func (f *FuncMetric) Write(w io.Writer) error {
	var v float64
	if f.read != nil {
		v = f.read()
	}
	if _, err := fmt.Fprintf(w, "%s%s %s\n",
		f.desc.Name, formatLabels(f.desc.ConstLabels), formatFloat(v)); err != nil {
		return fmt.Errorf("write %s: %w", f.desc.Name, err)
	}
	return nil
}
