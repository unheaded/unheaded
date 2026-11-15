// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package server

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/cmd/dashboard-backend/internal/health"
	"unheaded/pkg/logger"
	"unheaded/pkg/modules"
)

// The module sync loop registers a module's health targets while its
// fragment exists and publishes unheaded_dashboard_health_targets{module}
// (ADR-149); with the fragment gone the module's count goes to 0.
func TestSyncModuleTargets_PublishesCounts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(modules.EnvDir, dir)
	frag := filepath.Join(dir, "zhen", "dashboard", "services.txt")
	if err := os.MkdirAll(filepath.Dir(frag), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(frag, []byte("zhenai=127.0.0.1:1\nllama-server=127.0.0.1:2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	log := logger.New(io.Discard)
	mon, err := health.NewMonitor(nil, log)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{log: log, healthMonitor: mon}
	s.initMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.syncModuleTargets(ctx, 20*time.Millisecond); close(done) }()

	waitFor(t, func() bool {
		return strings.Contains(scrape(t, s), `unheaded_dashboard_health_targets{module="zhen"} 2`)
	})
	if err := os.RemoveAll(filepath.Join(dir, "zhen")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		return strings.Contains(scrape(t, s), `unheaded_dashboard_health_targets{module="zhen"} 0`)
	})
	if !strings.Contains(scrape(t, s), `unheaded_dashboard_health_targets{module=""} 0`) {
		t.Fatalf("no platform line:\n%s", scrape(t, s))
	}
	cancel()
	<-done
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 5s")
}
