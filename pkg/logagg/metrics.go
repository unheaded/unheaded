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
