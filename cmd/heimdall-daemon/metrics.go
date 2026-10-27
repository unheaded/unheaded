// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// /metrics for heimdall-daemon (M-085). The drift sink always counted its
// publishes, failures and drops, but nothing served them. They now live in
// one registry, with a histogram around each baseline scan, served on
// -metrics-addr (default loopback 127.0.0.1:19101; empty disables it).

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"unheaded/pkg/metrics"
	"unheaded/pkg/ports"
)

// defaultMetricsAddr is loopback on the registered port. A host whose
// Prometheus scrapes it over the overlay passes that address instead.
var defaultMetricsAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(ports.HeimdallMetrics))

// heimdallMetrics is the daemon's registry and the series in it.
type heimdallMetrics struct {
	reg          *metrics.Registry
	published    *metrics.Counter
	failed       *metrics.Counter
	dropped      *metrics.Counter
	scanDuration *metrics.Histogram
}

func newHeimdallMetrics() *heimdallMetrics {
	m := &heimdallMetrics{
		reg:       metrics.NewRegistry(),
		published: metrics.NewCounter("heimdall_drift_published_total", "Drift events published to Wotan, signed.", nil),
		failed:    metrics.NewCounter("heimdall_drift_publish_failed_total", "Drift events Wotan refused or that failed to reach it.", nil),
		dropped:   metrics.NewCounter("heimdall_drift_publish_dropped_total", "Drift events dropped (oldest first) while the publish queue was full.", nil),
		scanDuration: metrics.NewHistogram(metrics.HistogramOpts{
			Name:    "heimdall_scan_duration_seconds",
			Help:    "Time one baseline scan took (hash every manifest file, compare).",
			Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5, 10, 30},
		}),
	}
	m.reg.MustRegister(m.published)
	m.reg.MustRegister(m.failed)
	m.reg.MustRegister(m.dropped)
	m.reg.MustRegister(m.scanDuration)
	return m
}

// observeScan records one scan's duration; a nil receiver records nothing.
func (m *heimdallMetrics) observeScan(d time.Duration) {
	if m == nil {
		return
	}
	m.scanDuration.Observe(d.Seconds())
}

// startMetricsServer serves GET /metrics (this registry plus the default
// one: go_*, process_*) on addr. An empty addr serves nothing. It returns
// the bound address (useful with port 0) and a stop function.
func startMetricsServer(addr string, m *heimdallMetrics) (string, func(context.Context) error, error) {
	if addr == "" {
		return "", func(context.Context) error { return nil }, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.HandlerFor(m.reg, metrics.DefaultRegistry))
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// The listener died under a running daemon; drift detection
			// itself is unaffected, so say so and keep scanning.
			fmt.Fprintf(os.Stderr, "[METRICS] listener stopped: %v\n", err)
		}
	}()
	return ln.Addr().String(), srv.Shutdown, nil
}
