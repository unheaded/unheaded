// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/metrics"
)

func TestDefaultConfig_DryRunByDefault(t *testing.T) {
	c := DefaultConfig()
	if !c.DryRunMode {
		t.Fatal("DryRunMode must default to true: a game-day window turns it off by env")
	}
	if c.AbortFile != "/run/unheaded/chaos/ABORT" || c.SafetyCheckInterval <= 0 || c.DefaultDuration <= 0 {
		t.Fatalf("defaults %+v", c)
	}
}

// No experiment type has a built-in injector that only logs and reports
// success (the old network_delay, process_kill, http_error and
// latency_spike "injectors" did exactly that). Unregistered types refuse.
func TestNoInjectorRefusesInsteadOfSucceeding(t *testing.T) {
	for _, dry := range []bool{false, true} {
		cfg := DefaultConfig()
		cfg.DryRunMode = dry
		cfg.StateDir = t.TempDir()
		cfg.AbortFile = filepath.Join(t.TempDir(), "ABORT")
		s, err := NewService(cfg, Deps{Allowlist: testAllowlist(t), Guard: okGuard(), Resolver: newResolver()})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, typ := range []ExperimentType{ExpTypeNetworkDelay, ExpTypeProcessKill, ExpTypeHTTPError, ExpTypeLatencySpike,
			ExpTypeContainerKill, ExpTypeCPUStress, ExpTypeDNSFailure, ExpTypeCustom} {
			e := delayExp("steel-echo-v2-0", time.Second)
			e.Type = typ
			if _, err := s.StartExperiment(context.Background(), e); !isRefusal(err, RefuseNoInjector) {
				t.Errorf("dry=%v %s: %v, want a no_injector refusal", dry, typ, err)
			}
		}
		if n := len(s.List()); n != 0 {
			t.Errorf("refused experiments were recorded as runs: %d", n)
		}
		s.Stop()
	}
}

func TestNoFakeInjectorSourcesLeft(t *testing.T) {
	banned := regexp.MustCompile(`(?i)simulat|in (a )?real implementation|would (be|actually|evaluate|undo|check)|Success: true, // `)
	files, _ := filepath.Glob("*.go")
	files2, _ := filepath.Glob("../../cmd/chaos-controller/*.go")
	n := 0
	for _, f := range append(files, files2...) {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f) // #nosec G304 -- fixed package directories
		if err != nil {
			t.Fatal(err)
		}
		n++
		for i, line := range strings.Split(string(b), "\n") {
			if banned.MatchString(line) {
				t.Errorf("%s:%d: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
	if n < 6 {
		t.Fatalf("scanned %d files", n)
	}
}

func TestDryRun_InjectsNothing(t *testing.T) {
	fx := newFixture(t, func(c *Config, _ *Deps) { c.DryRunMode = true })
	exp := fx.start(t, delayExp("steel-echo-v2-0", time.Second))
	e := fx.wait(t, exp.ID)
	if e.Status != StatusCompleted || !e.Results.DryRun || e.Results.Success {
		t.Fatalf("dry run ended %s %+v", e.Status, e.Results)
	}
	if fx.inj.injectedCount() != 0 || len(fx.s.JournalEntries()) != 0 {
		t.Fatal("a dry run injected or journaled")
	}
}

func TestStartExperiment_Validation(t *testing.T) {
	fx := newFixture(t)
	bad := map[string]*Experiment{
		"nil":          nil,
		"no type":      {Target: Target{Kind: KindLXD, ID: "steel-echo-v2-0"}},
		"unknown type": {Type: "melt", Target: Target{Kind: KindLXD, ID: "steel-echo-v2-0"}},
		"no target":    {Type: ExpTypeNetworkDelay},
		"long name":    {Name: strings.Repeat("n", 300), Type: ExpTypeNetworkDelay, Target: Target{Kind: KindLXD, ID: "steel-echo-v2-0"}},
	}
	for name, e := range bad {
		if _, err := fx.s.StartExperiment(context.Background(), e); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if fx.inj.injectedCount() != 0 {
		t.Fatal("an invalid experiment injected")
	}
}

func TestExperiment_FullCycle(t *testing.T) {
	fx := newFixture(t)
	exp := fx.start(t, delayExp("steel-echo-v2-0", 100*time.Millisecond))
	if got, ok := fx.s.Get(exp.ID); !ok || got.Target.ID != "steel-echo-v2-0" {
		t.Fatalf("get: %+v %v", got, ok)
	}
	e := fx.wait(t, exp.ID)
	if e.Status != StatusCompleted || !e.Results.Success || !e.Results.Reverted || e.Results.Duration <= 0 {
		t.Fatalf("cycle: %s %+v", e.Status, e.Results)
	}
	if fx.inj.injectedCount() != 1 || fx.inj.activeCount() != 0 {
		t.Fatalf("injected %d, active %d", fx.inj.injectedCount(), fx.inj.activeCount())
	}
	if len(fx.s.List()) != 1 {
		t.Fatal("list")
	}
	if fx.pub.count(TopicStarted) != 1 || fx.pub.count(TopicCompleted) != 1 {
		t.Fatalf("events started %d completed %d", fx.pub.count(TopicStarted), fx.pub.count(TopicCompleted))
	}
}

func TestStop_RevertsRunning(t *testing.T) {
	fx := newFixture(t)
	fx.start(t, delayExp("steel-echo-v2-0", 10*time.Second))
	waitFor(t, "injection", func() bool { return fx.inj.activeCount() == 1 })
	fx.s.Stop()
	if fx.inj.activeCount() != 0 || len(fx.s.JournalEntries()) != 0 {
		t.Fatal("stop left a fault in place")
	}
}

func TestMetrics(t *testing.T) {
	fx := newFixture(t)
	_, _ = fx.s.StartExperiment(context.Background(), delayExp("pihole", time.Second))
	exp := fx.start(t, delayExp("steel-echo-v2-0", 50*time.Millisecond))
	fx.wait(t, exp.ID)
	fx.s.Abort("http", "x")
	var b bytes.Buffer
	if err := metrics.GatherAll(&b, fx.s.Registry()); err != nil {
		t.Fatal(err)
	}
	body := b.String()
	for _, want := range []string{
		`chaos_refusals_total{reason="allowlist"} 1`,
		`chaos_experiments_total{result="completed",type="network_delay"} 1`,
		`chaos_active_injections 0`,
		`chaos_aborts_total{path="http"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics lack %s\n%s", want, body)
		}
	}
}
