// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"fmt"
	"io"

	"unheaded/pkg/metrics"
)

// Collectors returns this publisher's counters as pkg/metrics collectors,
// ready to hand to whichever registry the service actually serves.
//
// They are NOT registered in a package-level registry on purpose: most
// services build and serve their own *metrics.Registry, and a counter
// registered globally here would be invisible in them. The publisher owns
// the numbers and the service owns the registry: RegisterCollectors(reg).
// This is the only exposition path; the hand-rolled text writer that
// monad, sophia, kanban-app, unheaded-daemon and timeguru appended after
// their registry went in S-WELL-SPRING Step 188 (byte-identical output,
// metrics_test.go).
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

// RegisterCollectors registers Collectors() in reg, the registry the service
// serves: all three or none, so a conflict never leaves a partial family
// set. A nil reg (a server built as a struct literal) is a no-op.
func (p *Publisher) RegisterCollectors(reg *metrics.Registry) error {
	if reg == nil {
		return nil
	}
	cs := p.Collectors()
	for i, c := range cs {
		if err := reg.Register(c); err != nil {
			for _, done := range cs[:i] {
				reg.Unregister(done)
			}
			return fmt.Errorf("logagg collectors for %q: %w", p.serviceName, err)
		}
	}
	return nil
}

// UnregisterCollectors removes this publisher's counters from reg (a
// service swapping publishers). A nil reg is a no-op.
func (p *Publisher) UnregisterCollectors(reg *metrics.Registry) {
	if reg == nil {
		return
	}
	for _, c := range p.Collectors() {
		reg.Unregister(c)
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
