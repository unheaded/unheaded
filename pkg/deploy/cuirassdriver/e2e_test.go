// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cuirassdriver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/deploy/strategy"
)

// End-to-end strategy runs: the real strategies, the real driver, an
// in-memory Cuirass.

func seedVersion(f *fakeCuirass, ver string, n int) {
	for i := 0; i < n; i++ {
		spec := containerSpec{ID: fmt.Sprintf("steel-echo-%s-%d", ver, i), Image: "steel-echo-" + ver,
			Labels: map[string]string{"svc": "steel-echo", "ver": ver}, Health: &HealthSpec{Port: 8080, Path: "/health"}}
		if i == 0 {
			spec.Proxy = testProxy
		}
		f.seed(spec)
	}
}

func run(t *testing.T, d *Driver, typ strategy.Type, cfg *strategy.Config, cb *strategy.Callbacks, from, to string) (*strategy.Result, error) {
	t.Helper()
	insts, err := d.Instances(context.Background(), "steel-echo")
	if err != nil {
		t.Fatal(err)
	}
	s, err := strategy.GetStrategy(typ)
	if err != nil {
		t.Fatal(err)
	}
	cfg.HealthCheck.Interval = 2 * time.Millisecond
	cfg.ProgressDeadline = 20 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.Execute(ctx, &strategy.ExecuteParams{
		DeploymentID: "dep-" + to, ServiceName: "steel-echo", CurrentVersion: from, TargetVersion: to,
		Replicas: 3, Config: cfg, Instances: insts, Callbacks: cb,
	})
}

func TestE2E_RollingKeepsThreeHealthy(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.healthyAfter = 2
	seedVersion(f, "v1", 3)
	minHealthy, ticks := 99, 0
	f.onTick = func(f *fakeCuirass) {
		n := 0
		for _, a := range f.actual {
			if a.Health == "healthy" && a.Status == "running" {
				n++
			}
		}
		ticks++
		minHealthy = min(minHealthy, n)
	}
	d := newTestDriver(t, srv.URL)
	cb, err := d.Callbacks(testSpec(), "dep-v2", "v2")
	if err != nil {
		t.Fatal(err)
	}
	cfg := strategy.DefaultConfig(strategy.TypeRolling)
	cfg.MaxSurge, cfg.MaxUnavailable = 1, 0
	res, err := run(t, d, strategy.TypeRolling, cfg, cb, "v1", "v2")
	if err != nil || !res.Success {
		t.Fatalf("rolling: %v %+v", err, res)
	}
	f.mu.Lock()
	gotMin, gotTicks := minHealthy, ticks
	f.mu.Unlock()
	if gotTicks < 6 || gotMin < 3 {
		t.Fatalf("healthy instances dropped to %d during the rollout (%d ticks), want >= 3", gotMin, gotTicks)
	}
	onlyVersion(t, f, "v2", 3)
}

func TestE2E_BlueGreenMovesTheProxyOnceAfterGreenIsHealthy(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.healthyAfter = 2
	seedVersion(f, "v2", 3)
	d := newTestDriver(t, srv.URL)
	cb, err := d.Callbacks(testSpec(), "dep-v3", "v3")
	if err != nil {
		t.Fatal(err)
	}
	cfg := strategy.DefaultConfig(strategy.TypeBlueGreen)
	cfg.TestDuration = 0
	res, err := run(t, d, strategy.TypeBlueGreen, cfg, cb, "v2", "v3")
	if err != nil || !res.Success {
		t.Fatalf("blue-green: %v %+v", err, res)
	}
	log := f.logSnapshot()
	moves, moveAt, firstBlueDelete := 0, -1, -1
	for i, l := range log {
		if l == "PUT steel-echo-v2-0 proxy=false" {
			moves++
			moveAt = i
		}
		if strings.HasPrefix(l, "DELETE steel-echo-v2-") && firstBlueDelete < 0 {
			firstBlueDelete = i
		}
	}
	if moves != 1 {
		t.Fatalf("proxy moved %d times, want once; log %v", moves, log)
	}
	if firstBlueDelete < moveAt {
		t.Fatalf("a blue instance was deleted before the switch; log %v", log)
	}
	onlyVersion(t, f, "v3", 3)
}

func TestE2E_CanaryFailedAnalysisRollsBack(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.healthyAfter = 2
	seedVersion(f, "v3", 3)
	d := newTestDriver(t, srv.URL)
	cb, err := d.Callbacks(testSpec(), "dep-v4", "v4")
	if err != nil {
		t.Fatal(err)
	}
	var seen *strategy.AnalysisRequest
	cb.OnAnalysis = func(_ context.Context, req *strategy.AnalysisRequest) (*strategy.AnalysisResult, error) {
		seen = req
		return &strategy.AnalysisResult{Pass: false, ShouldRollback: true, Message: "error ratio 0.30",
			Metrics: &strategy.AnalysisMetrics{ErrorRate: 0.30, RequestCount: 300}}, nil
	}
	cfg := &strategy.Config{
		Type:        strategy.TypeCanary,
		Steps:       []strategy.CanaryStep{{Weight: 33, Analysis: true}, {Weight: 100}},
		Analysis:    &strategy.AnalysisConfig{ErrorRateThreshold: 0.01, MinSampleSize: 200, Interval: time.Millisecond},
		HealthCheck: &strategy.HealthCheckConfig{},
	}
	res, err := run(t, d, strategy.TypeCanary, cfg, cb, "v3", "v4")
	if !errors.Is(err, strategy.ErrRollbackRequired) || res == nil || res.Success || !res.RolledBack {
		t.Fatalf("canary: err %v res %+v, want rolled back", err, res)
	}
	if seen == nil || len(seen.Canary) != 1 || seen.Canary[0].ID != "steel-echo-v4-0" || len(seen.Stable) != 3 {
		t.Fatalf("analysis saw %+v", seen)
	}
	onlyVersion(t, f, "v3", 3)
}
