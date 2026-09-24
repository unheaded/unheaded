// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package prom presents pkg/metrics under the names prometheus/client_golang
// uses, so that moving a file off client_golang (ADR-094 step 4) is a change
// of import path, not a rewrite:
//
//	prometheus.CounterOpts  -> prom.CounterOpts
//	promhttp.Handler()      -> prom.Handler()
//	promauto.NewCounter(..) -> auto.NewCounter(..)   (package metrics/auto)
//
// It covers what this repository calls, found by surveying every importer,
// and nothing more. A client_golang name that is missing here is missing
// because no file uses it; add it when one does, with a parity test.
//
// One difference call sites can see: client_golang's Counter, Gauge and
// Histogram are interfaces, and here they are pointers to pkg/metrics types.
// A field declared as prometheus.Counter becomes prom.Counter and needs no
// other change, but code that type-asserts on the interface will not compile.
// None does today.
package prom

import (
	"net/http"
	"strings"

	"unheaded/pkg/metrics"
)

type (
	// Labels maps label names to values.
	Labels = metrics.Labels
	// Collector is anything a registry can gather.
	Collector = metrics.Collector

	// Counter is a single counter. client_golang's is an interface.
	Counter = *metrics.Counter
	// Gauge is a single gauge. client_golang's is an interface.
	Gauge = *metrics.Gauge
	// Histogram is a single histogram. client_golang's is an interface.
	Histogram = *metrics.Histogram

	// CounterVec is a family of counters partitioned by labels.
	CounterVec = metrics.CounterVec
	// GaugeVec is a family of gauges partitioned by labels.
	GaugeVec = metrics.GaugeVec
	// HistogramVec is a family of histograms partitioned by labels.
	HistogramVec = metrics.HistogramVec
)

// Opts names a metric the way client_golang does: the exposed name is
// Namespace_Subsystem_Name, with empty parts left out.
type Opts struct {
	Namespace   string
	Subsystem   string
	Name        string
	Help        string
	ConstLabels Labels
}

// CounterOpts configures a counter.
type CounterOpts Opts

// GaugeOpts configures a gauge.
type GaugeOpts Opts

// HistogramOpts configures a histogram. Nil Buckets means DefBuckets.
type HistogramOpts struct {
	Namespace   string
	Subsystem   string
	Name        string
	Help        string
	ConstLabels Labels
	Buckets     []float64
}

// DefBuckets are client_golang's default buckets. They are the same values
// as metrics.DefaultBuckets; the parity test pins that.
var DefBuckets = metrics.DefaultBuckets

// BuildFQName joins the non-empty parts with "_". An empty name gives "",
// exactly as client_golang does.
func BuildFQName(namespace, subsystem, name string) string {
	if name == "" {
		return ""
	}
	parts := make([]string, 0, 3)
	for _, p := range []string{namespace, subsystem, name} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "_")
}

// ExponentialBuckets returns count buckets, the first at start and each
// factor times the last.
func ExponentialBuckets(start, factor float64, count int) []float64 {
	return metrics.ExponentialBuckets(start, factor, count)
}

// LinearBuckets returns count buckets, the first at start, width apart.
func LinearBuckets(start, width float64, count int) []float64 {
	return metrics.LinearBuckets(start, width, count)
}

// NewCounter returns an unregistered counter.
func NewCounter(o CounterOpts) Counter {
	return metrics.NewCounter(BuildFQName(o.Namespace, o.Subsystem, o.Name), o.Help, o.ConstLabels)
}

// NewCounterVec returns an unregistered counter family.
func NewCounterVec(o CounterOpts, labelNames []string) *CounterVec {
	return metrics.NewCounterVec(BuildFQName(o.Namespace, o.Subsystem, o.Name), o.Help, o.ConstLabels, labelNames)
}

// NewGauge returns an unregistered gauge.
func NewGauge(o GaugeOpts) Gauge {
	return metrics.NewGauge(BuildFQName(o.Namespace, o.Subsystem, o.Name), o.Help, o.ConstLabels)
}

// NewGaugeVec returns an unregistered gauge family.
func NewGaugeVec(o GaugeOpts, labelNames []string) *GaugeVec {
	return metrics.NewGaugeVec(BuildFQName(o.Namespace, o.Subsystem, o.Name), o.Help, o.ConstLabels, labelNames)
}

// NewGaugeFunc returns an unregistered gauge whose value is read at scrape
// time.
func NewGaugeFunc(o GaugeOpts, read func() float64) Collector {
	return metrics.NewFuncGauge(BuildFQName(o.Namespace, o.Subsystem, o.Name), o.Help, o.ConstLabels, read)
}

func histogramOpts(o HistogramOpts) metrics.HistogramOpts {
	return metrics.HistogramOpts{
		Name:        BuildFQName(o.Namespace, o.Subsystem, o.Name),
		Help:        o.Help,
		ConstLabels: o.ConstLabels,
		Buckets:     o.Buckets,
	}
}

// NewHistogram returns an unregistered histogram.
func NewHistogram(o HistogramOpts) Histogram {
	return metrics.NewHistogram(histogramOpts(o))
}

// NewHistogramVec returns an unregistered histogram family.
func NewHistogramVec(o HistogramOpts, labelNames []string) *HistogramVec {
	return metrics.NewHistogramVec(histogramOpts(o), labelNames)
}

// Registerer is the part of a registry that metric constructors need.
type Registerer interface {
	Register(Collector) error
	MustRegister(...Collector)
}

// Registry is a metric registry with client_golang's variadic MustRegister.
type Registry struct {
	r *metrics.Registry
}

// NewRegistry returns an empty registry, without the go_* and process_*
// series the default one carries — the same as client_golang.
func NewRegistry() *Registry {
	return &Registry{r: metrics.NewRegistry()}
}

// Register adds c, failing if its name is already registered.
func (r *Registry) Register(c Collector) error {
	return r.r.Register(c)
}

// MustRegister adds every collector and panics on the first conflict.
func (r *Registry) MustRegister(cs ...Collector) {
	for _, c := range cs {
		r.r.MustRegister(c)
	}
}

// Unregister removes c, identified by its name and constant labels.
func (r *Registry) Unregister(c Collector) bool {
	return r.r.Unregister(c)
}

// Handler serves the registry in Prometheus text format — promhttp.HandlerFor.
func (r *Registry) Handler() http.Handler {
	return r.r.Handler()
}

// DefaultRegisterer is the default registry, which carries the go_* and
// process_* series, as client_golang's does.
var DefaultRegisterer Registerer = &Registry{r: metrics.DefaultRegistry}

// Register adds c to the default registry.
func Register(c Collector) error {
	return DefaultRegisterer.Register(c)
}

// MustRegister adds every collector to the default registry.
func MustRegister(cs ...Collector) {
	DefaultRegisterer.MustRegister(cs...)
}

// Handler serves the default registry — promhttp.Handler().
func Handler() http.Handler {
	return metrics.DefaultRegistry.Handler()
}
