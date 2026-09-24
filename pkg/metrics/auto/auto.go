// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package auto is promauto for pkg/metrics: each constructor creates the
// metric and registers it in one call, panicking on a name conflict.
// auto.NewCounter registers with the default registry;
// auto.With(reg).NewCounter with reg; auto.With(nil) registers nowhere,
// as promauto.With(nil) does.
package auto

import "unheaded/pkg/metrics/prom"

// Factory creates metrics and registers them with one registry.
type Factory struct {
	r prom.Registerer
}

// With returns a Factory that registers with r, or nowhere if r is nil.
func With(r prom.Registerer) Factory {
	return Factory{r: r}
}

func (f Factory) register(c prom.Collector) {
	if f.r != nil {
		f.r.MustRegister(c)
	}
}

// NewCounter creates and registers a counter.
func (f Factory) NewCounter(o prom.CounterOpts) prom.Counter {
	c := prom.NewCounter(o)
	f.register(c)
	return c
}

// NewCounterVec creates and registers a counter family.
func (f Factory) NewCounterVec(o prom.CounterOpts, labelNames []string) *prom.CounterVec {
	c := prom.NewCounterVec(o, labelNames)
	f.register(c)
	return c
}

// NewGauge creates and registers a gauge.
func (f Factory) NewGauge(o prom.GaugeOpts) prom.Gauge {
	g := prom.NewGauge(o)
	f.register(g)
	return g
}

// NewGaugeVec creates and registers a gauge family.
func (f Factory) NewGaugeVec(o prom.GaugeOpts, labelNames []string) *prom.GaugeVec {
	g := prom.NewGaugeVec(o, labelNames)
	f.register(g)
	return g
}

// NewHistogram creates and registers a histogram.
func (f Factory) NewHistogram(o prom.HistogramOpts) prom.Histogram {
	h := prom.NewHistogram(o)
	f.register(h)
	return h
}

// NewHistogramVec creates and registers a histogram family.
func (f Factory) NewHistogramVec(o prom.HistogramOpts, labelNames []string) *prom.HistogramVec {
	h := prom.NewHistogramVec(o, labelNames)
	f.register(h)
	return h
}

var defaultFactory = With(prom.DefaultRegisterer)

// NewCounter creates a counter registered with the default registry.
func NewCounter(o prom.CounterOpts) prom.Counter { return defaultFactory.NewCounter(o) }

// NewCounterVec creates a counter family registered with the default registry.
func NewCounterVec(o prom.CounterOpts, labelNames []string) *prom.CounterVec {
	return defaultFactory.NewCounterVec(o, labelNames)
}

// NewGauge creates a gauge registered with the default registry.
func NewGauge(o prom.GaugeOpts) prom.Gauge { return defaultFactory.NewGauge(o) }

// NewGaugeVec creates a gauge family registered with the default registry.
func NewGaugeVec(o prom.GaugeOpts, labelNames []string) *prom.GaugeVec {
	return defaultFactory.NewGaugeVec(o, labelNames)
}

// NewHistogram creates a histogram registered with the default registry.
func NewHistogram(o prom.HistogramOpts) prom.Histogram { return defaultFactory.NewHistogram(o) }

// NewHistogramVec creates a histogram family registered with the default
// registry.
func NewHistogramVec(o prom.HistogramOpts, labelNames []string) *prom.HistogramVec {
	return defaultFactory.NewHistogramVec(o, labelNames)
}
