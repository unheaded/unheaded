// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/sloguard"
)

// probeTo points a fixture's probes at a test server instead of the
// instance address (a loopback httptest server would be refused).
func probeTo(fx *fixture, srv *httptest.Server) {
	fx.s.setProbeURL(func(info *InstanceInfo, p Probe) (string, error) {
		return srv.URL + p.Path, nil
	})
}

func probeExp(target string, probes ...Probe) *Experiment {
	e := delayExp(target, time.Minute)
	e.SteadyState = &SteadyState{Probes: probes}
	return e
}

func TestRunProbe_RealHTTPStatus(t *testing.T) {
	fx := newFixture(t)
	exp := probeExp("steel-echo-v2-0")
	for name, c := range map[string]struct {
		status int
		delay  time.Duration
		want   bool
	}{
		"200 passes":     {200, 0, true},
		"500 fails":      {500, 0, false},
		"timeout fails":  {200, 400 * time.Millisecond, false},
		"404 is not 200": {404, 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			probeTo(fx, probeServer(t, c.status, c.delay))
			start := time.Now()
			r := fx.s.runProbe(context.Background(), exp, Probe{Name: "h", Port: 8080, Path: "/health", TimeoutMS: 200})
			if r.Success != c.want {
				t.Fatalf("probe = %+v, want success %v", r, c.want)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("the probe ignored its timeout")
			}
		})
	}
}

func TestRunProbe_RefusesNonAllowlistedTarget(t *testing.T) {
	fx := newFixture(t)
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	probeTo(fx, srv)
	exp := probeExp("steel-echo-v2-0")
	for _, inst := range []string{"pihole", "plain-0", "unmanaged-0", "other-project-0", "nope-0", "10.0.0.1"} {
		r := fx.s.runProbe(context.Background(), exp, Probe{Name: "x", Instance: inst, Port: 8080, Path: "/health"})
		if r.Success || r.Error == "" {
			t.Errorf("probe of %s = %+v", inst, r)
		}
	}
	for _, path := range []string{"health", "//evil.example/x", "/a/../../b", "/x?y=1", "http://h/"} {
		r := fx.s.runProbe(context.Background(), exp, Probe{Name: "x", Port: 8080, Path: path})
		if r.Success {
			t.Errorf("path %q probed", path)
		}
	}
	if hit {
		t.Fatal("a refused probe made a request")
	}
	// The default address check refuses what an instance could claim.
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "0.0.0.0", "", "::1"} {
		if _, err := defaultProbeURL(&InstanceInfo{Name: "i", IP: ip}, Probe{Port: 80, Path: "/health"}); err == nil {
			t.Errorf("instance IP %q accepted", ip)
		}
	}
}

func TestRunProbe_NoRedirects(t *testing.T) {
	fx := newFixture(t)
	followed := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true }))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/health", http.StatusFound)
	}))
	defer redir.Close()
	probeTo(fx, redir)
	r := fx.s.runProbe(context.Background(), probeExp("steel-echo-v2-0"), Probe{Name: "h", Port: 8080, Path: "/health"})
	if r.Success || followed {
		t.Fatalf("redirect followed (%v) or counted as success: %+v", followed, r)
	}
}

// --- the abort decision (D-20) ---

func TestShouldAbort_KingdomFastBurn(t *testing.T) {
	fx := newFixture(t)
	exp := fx.start(t, delayExp("steel-echo-v2-0", 5*time.Second))
	fx.guard.set(sloguard.Verdict{OK: false, Burning: []string{"wotan"}, Reason: "wotan burning"}, nil)
	e := fx.wait(t, exp.ID)
	if e.Status != StatusAborted || !strings.Contains(e.Results.AbortReason, "wotan") {
		t.Fatalf("experiment %s: %+v", e.Status, e.Results)
	}
	if fx.inj.activeCount() != 0 || len(fx.s.JournalEntries()) != 0 {
		t.Fatal("aborted but the fault is still in place")
	}
}

func TestShouldAbort_MissingSeriesRefusesStart(t *testing.T) {
	fx := newFixture(t)
	fx.guard.set(sloguard.Verdict{OK: false, Missing: []string{"dashboard"}, Reason: "dashboard missing"}, nil)
	if _, err := fx.s.StartExperiment(context.Background(), delayExp("steel-echo-v2-0", time.Second)); !isRefusal(err, RefuseSteadyState) {
		t.Fatalf("missing series: %v", err)
	}
	fx.guard.set(sloguard.Verdict{}, errors.New("connection refused"))
	if _, err := fx.s.StartExperiment(context.Background(), delayExp("steel-echo-v2-0", time.Second)); !isRefusal(err, RefuseSteadyState) {
		t.Fatalf("Victoria down: %v", err)
	}
	if fx.inj.injectedCount() != 0 {
		t.Fatal("injected without a steady state")
	}
	// No guard at all: nothing starts (D-20 "not skip the guard").
	fx2 := newFixture(t, func(_ *Config, d *Deps) { d.Guard = nil })
	if _, err := fx2.s.StartExperiment(context.Background(), delayExp("steel-echo-v2-0", time.Second)); !isRefusal(err, RefuseNoGuard) {
		t.Fatalf("no guard: %v", err)
	}
}

func TestShouldAbort_AbortFile(t *testing.T) {
	fx := newFixture(t)
	exp := fx.start(t, delayExp("steel-echo-v2-0", 5*time.Second))
	waitFor(t, "injection", func() bool { return fx.inj.activeCount() == 1 })
	if err := os.WriteFile(fx.abort, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	e := fx.wait(t, exp.ID)
	if e.Status != StatusAborted || !strings.Contains(e.Results.AbortReason, "file") {
		t.Fatalf("experiment %s %+v", e.Status, e.Results)
	}
	if fx.inj.activeCount() != 0 {
		t.Fatal("the fault survived the abort file")
	}
	// While the file is there nothing new starts.
	if _, err := fx.s.StartExperiment(context.Background(), delayExp("steel-echo-v2-0", time.Second)); !isRefusal(err, RefuseAbortFile) {
		t.Fatalf("with the abort file present: %v", err)
	}
	if err := os.Remove(fx.abort); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "abort file noticed gone", func() bool {
		_, err := fx.s.StartExperiment(context.Background(), &Experiment{Type: "no-such-type"})
		return !isRefusal(err, RefuseAbortFile)
	})
}

func TestShouldAbort_HealthyGuardContinues(t *testing.T) {
	fx := newFixture(t)
	exp := fx.start(t, delayExp("steel-echo-v2-0", 150*time.Millisecond))
	e := fx.wait(t, exp.ID)
	if e.Status != StatusCompleted || !e.Results.Success || !e.Results.HypothesisValid || !e.Results.Reverted {
		t.Fatalf("a healthy run ended %s %+v", e.Status, e.Results)
	}
	fx.guard.mu.Lock()
	n := fx.guard.n
	fx.guard.mu.Unlock()
	if n < 3 { // before, during (several), after
		t.Fatalf("the guard was asked %d times", n)
	}
}

func TestSteadyState_ProbesGateStartAndAbortBelow(t *testing.T) {
	fx := newFixture(t)
	probeTo(fx, probeServer(t, 500, 0))
	exp := probeExp("steel-echo-v2-0", Probe{Name: "h", Port: 8080, Path: "/health"})
	if _, err := fx.s.StartExperiment(context.Background(), exp); !isRefusal(err, RefuseSteadyState) {
		t.Fatalf("a failing probe before start: %v", err)
	}
	// Probes that pass first and then fail: impact by default, an abort
	// only with abort_below.
	healthy := probeServer(t, 200, 0)
	probeTo(fx, healthy)
	exp = probeExp("steel-echo-v2-0", Probe{Name: "h", Port: 8080, Path: "/health"})
	exp.SteadyState.AbortBelow = 1
	exp.Schedule.Duration = 5 * time.Second
	got := fx.start(t, exp)
	waitFor(t, "injection", func() bool { return fx.inj.activeCount() == 1 })
	probeTo(fx, probeServer(t, 500, 0))
	e := fx.wait(t, got.ID)
	if e.Status != StatusAborted || !strings.Contains(e.Results.AbortReason, "abort_below") {
		t.Fatalf("probes under abort_below: %s %+v", e.Status, e.Results)
	}
}
