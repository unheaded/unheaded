// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/enkrateia"
	"unheaded/pkg/metrics/metricstest"
)

// scrapeMetrics GETs /metrics from a running listener.
func scrapeMetrics(t *testing.T, addr string) string {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// With -metrics-addr 127.0.0.1:0, GET /metrics serves the three drift
// publish counters the sink keeps and a histogram around each scan, with the
// values the daemon actually reached (M-085: these counters existed with no
// listener, so nothing could ever read them).
func TestMetrics_ServesDriftCountersAndScanHistogram(t *testing.T) {
	m := newHeimdallMetrics()

	// Two slots, three records, no publisher running: one drop.
	sink := newDriftSink(&fakePublisher{}, nil, "drift.detected.east", 2, m)
	for i := 0; i < 3; i++ {
		sink.Enqueue(DriftRecord{NodeID: "east", Path: "/etc/hostname"})
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "mjolnir.yaml")
	body := "apiVersion: v1\nkind: BaselineManifest\nmetadata:\n  name: k\n  version: \"1\"\nspec:\n  files:\n    - path: " +
		filepath.Join(dir, "absent") + "\n      sha256: aa\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	man, err := LoadManifest(path, Trust{AllowUnsigned: true})
	if err != nil {
		t.Fatal(err)
	}
	h := &Heimdall{nodeID: "east", manifest: man, aggregator: enkrateia.NewAggregator(4), metrics: m}
	h.ScanFiles()

	addr, stop, err := startMetricsServer("127.0.0.1:0", m)
	if err != nil {
		t.Fatalf("startMetricsServer: %v", err)
	}
	defer func() { _ = stop(context.Background()) }()
	if addr == "" {
		t.Fatal("listener address is empty")
	}

	out := scrapeMetrics(t, addr)
	for _, want := range []string{
		"# TYPE heimdall_drift_published_total counter\nheimdall_drift_published_total 0\n",
		"# TYPE heimdall_drift_publish_failed_total counter\nheimdall_drift_publish_failed_total 0\n",
		"# TYPE heimdall_drift_publish_dropped_total counter\nheimdall_drift_publish_dropped_total 1\n",
		"# TYPE heimdall_scan_duration_seconds histogram\n",
		"heimdall_scan_duration_seconds_count 1\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("/metrics missing %q\ngot:\n%s", want, out)
		}
	}
	// The default registry rides along like every other service's scrape.
	if !strings.Contains(out, "go_goroutines ") {
		t.Errorf("/metrics missing go_goroutines (default registry not served)")
	}
	for _, p := range metricstest.Lint(out) {
		t.Error(p)
	}
}

// A daemon run without Wotan has no sink; the counters are still served, at
// zero, so a scrape tells "nothing published" from "nothing scraped".
func TestMetrics_CountersPresentWithoutSink(t *testing.T) {
	m := newHeimdallMetrics()
	addr, stop, err := startMetricsServer("127.0.0.1:0", m)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stop(context.Background()) }()
	out := scrapeMetrics(t, addr)
	for _, name := range []string{
		"heimdall_drift_published_total 0",
		"heimdall_drift_publish_failed_total 0",
		"heimdall_drift_publish_dropped_total 0",
		"heimdall_scan_duration_seconds_count 0",
	} {
		if !strings.Contains(out, name) {
			t.Errorf("/metrics missing %q", name)
		}
	}
}

// The default listener is loopback on the registered port: a host that
// wants Prometheus to scrape it passes its overlay address explicitly.
func TestMetrics_DefaultAddrIsLoopbackOnRegisteredPort(t *testing.T) {
	if defaultMetricsAddr != "127.0.0.1:19101" {
		t.Fatalf("defaultMetricsAddr = %q, want 127.0.0.1:19101", defaultMetricsAddr)
	}
}

// An empty -metrics-addr disables the listener (a unit on a host with no
// scraper): no error, no address, a stop that does nothing.
func TestMetrics_EmptyAddrDisablesListener(t *testing.T) {
	addr, stop, err := startMetricsServer("", newHeimdallMetrics())
	if err != nil {
		t.Fatalf("startMetricsServer(\"\"): %v", err)
	}
	if addr != "" {
		t.Fatalf("addr = %q, want empty", addr)
	}
	if err := stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

// A listener that cannot bind is an error at startup, not a silent gap.
func TestMetrics_BindFailureIsAnError(t *testing.T) {
	m := newHeimdallMetrics()
	addr, stop, err := startMetricsServer("127.0.0.1:0", m)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stop(context.Background()) }()
	if _, _, err := startMetricsServer(addr, newHeimdallMetrics()); err == nil {
		t.Fatalf("second bind on %s succeeded, want an error", addr)
	}
}
