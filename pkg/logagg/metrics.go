// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"fmt"
	"io"

	prom "github.com/prometheus/client_golang/prometheus"

	"unheaded/pkg/metrics"
)

// Collectors returns this publisher's counters as pkg/metrics collectors,
// ready to hand to whichever registry the service actually serves.
//
// They are NOT registered in a package-level registry on purpose. Services
// here do not share one: only wotan serves the Prometheus client registry,
// the other nine build their own *metrics.Registry. A counter registered
// globally by this package would be invisible in nine services out of ten —
// the same "gate nothing reaches" shape as a migration nothing runs against.
// So the publisher owns the numbers and the service owns the registry:
//
//	for _, c := range logPublisher.Collectors() {
//	    registry.MustRegister(c)
//	}
//
// Read live; each call reflects the counter at scrape time.
func (p *Publisher) Collectors() []metrics.Collector {
	return []metrics.Collector{
		&publisherCounter{
			name: "unheaded_logagg_entries_dropped_total",
			help: "Log entries discarded because the logagg publish queue was full",
			svc:  p.serviceName,
			read: p.Dropped,
		},
		&publisherCounter{
			name: "unheaded_logagg_entries_published_total",
			help: "Log entries successfully published to Wotan by logagg",
			svc:  p.serviceName,
			read: p.Published,
		},
		&publisherCounter{
			name: "unheaded_logagg_publish_errors_total",
			help: "logagg publish attempts the transport rejected",
			svc:  p.serviceName,
			read: p.Failed,
		},
	}
}

// publisherCounter exposes one atomic counter as a metrics.Collector.
type publisherCounter struct {
	name string
	help string
	svc  string
	read func() uint64
}

func (c *publisherCounter) Describe() *metrics.Desc {
	return metrics.NewDesc(c.name, c.help, metrics.TypeCounter, metrics.Labels{"service": c.svc}, nil)
}

// Write emits only the sample line. Registry.Gather already writes # HELP and
// # TYPE from Describe(); emitting them here too produces duplicate HELP lines,
// which the Prometheus text format forbids and parsers reject.
func (c *publisherCounter) Write(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "%s{service=%q} %d\n", c.name, c.svc, c.read()); err != nil {
		return fmt.Errorf("write %s: %w", c.name, err)
	}
	return nil
}

// ============================================================================
// ADAPTERS FOR THE OTHER TWO CONVENTIONS IN THIS TREE
// ============================================================================
//
// Services here expose metrics three different ways, so the publisher offers
// three thin views over the same atomics rather than picking a winner:
//
//	pkg/metrics.Registry   -> Collectors()          (dashboard-backend)
//	promhttp default reg.  -> PrometheusCollector() (architect, micromanager,
//	                                                 trace-collector-go, wotan)
//	hand-rolled exposition -> WriteMetrics()        (monad, sophia, kanban-app,
//	                                                 unheaded-daemon)
//
// Converging on one convention would delete two of these. That is a real
// cleanup and a bigger change than wiring a counter, so it is left as a
// decision rather than made silently here.

// PrometheusCollector returns the counters as a prometheus.Collector, for
// services that serve promhttp.Handler() off the default registry:
//
//	prometheus.MustRegister(logPublisher.PrometheusCollector())
func (p *Publisher) PrometheusCollector() prom.Collector {
	return &promPublisherCollector{p: p}
}

type promPublisherCollector struct{ p *Publisher }

func (c *promPublisherCollector) descs() []struct {
	desc *prom.Desc
	read func() uint64
} {
	labels := prom.Labels{"service": c.p.serviceName}
	return []struct {
		desc *prom.Desc
		read func() uint64
	}{
		{prom.NewDesc("unheaded_logagg_entries_dropped_total",
			"Log entries discarded because the logagg publish queue was full", nil, labels), c.p.Dropped},
		{prom.NewDesc("unheaded_logagg_entries_published_total",
			"Log entries successfully published to Wotan by logagg", nil, labels), c.p.Published},
		{prom.NewDesc("unheaded_logagg_publish_errors_total",
			"logagg publish attempts the transport rejected", nil, labels), c.p.Failed},
	}
}

func (c *promPublisherCollector) Describe(ch chan<- *prom.Desc) {
	for _, d := range c.descs() {
		ch <- d.desc
	}
}

func (c *promPublisherCollector) Collect(ch chan<- prom.Metric) {
	for _, d := range c.descs() {
		ch <- prom.MustNewConstMetric(d.desc, prom.CounterValue, float64(d.read()))
	}
}

// WriteMetrics appends the counters in Prometheus text format, for services
// that build their /metrics body by hand. Writes HELP and TYPE as well —
// unlike the Collector path, nothing else emits them here.
func (p *Publisher) WriteMetrics(w io.Writer) error {
	for _, m := range []struct {
		name, help string
		read       func() uint64
	}{
		{"unheaded_logagg_entries_dropped_total", "Log entries discarded because the logagg publish queue was full", p.Dropped},
		{"unheaded_logagg_entries_published_total", "Log entries successfully published to Wotan by logagg", p.Published},
		{"unheaded_logagg_publish_errors_total", "logagg publish attempts the transport rejected", p.Failed},
	} {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s{service=%q} %d\n",
			m.name, m.help, m.name, m.name, p.serviceName, m.read()); err != nil {
			return fmt.Errorf("write %s: %w", m.name, err)
		}
	}
	return nil
}
