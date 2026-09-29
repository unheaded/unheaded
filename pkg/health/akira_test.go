// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestConsensusThreshold(t *testing.T) {
	// Two-thirds consensus — hardcoded, not configurable
	if ConsensusThreshold < 0.666 || ConsensusThreshold > 0.667 {
		t.Errorf("ConsensusThreshold should be 2/3, got %f", ConsensusThreshold)
	}
}

func TestCheckService_Healthy(t *testing.T) {
	// Create a mock healthy service
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	akira := NewAkira("test-node", []ServiceTarget{
		{Name: "test-svc", Host: "127.0.0.1", Port: 0, HealthPath: "/health"},
	})

	// Parse the test server's port
	report := akira.CheckService(ServiceTarget{
		Name: "test-svc", Host: srv.Listener.Addr().(*net.TCPAddr).IP.String(),
		Port: srv.Listener.Addr().(*net.TCPAddr).Port, HealthPath: "/",
	})

	if !report.Healthy {
		t.Errorf("Expected healthy, got unhealthy: %s", report.Error)
	}
	if report.Service != "test-svc" {
		t.Errorf("Expected service=test-svc, got %s", report.Service)
	}
	if report.Reporter != "test-node" {
		t.Errorf("Expected reporter=test-node, got %s", report.Reporter)
	}
}

func TestCheckService_Unhealthy(t *testing.T) {
	akira := NewAkira("test-node", nil)

	// Check a port that's definitely not listening
	report := akira.CheckService(ServiceTarget{
		Name: "dead-svc", Host: "127.0.0.1", Port: 1, HealthPath: "/health",
	})

	if report.Healthy {
		t.Error("Expected unhealthy for unreachable service")
	}
	if report.Error == "" {
		t.Error("Expected error message for unhealthy service")
	}
}

func TestAkiraRunCancellation(t *testing.T) {
	akira := NewAkira("test-node", nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		akira.Run(ctx)
		close(done)
	}()

	// Cancel immediately
	cancel()

	select {
	case <-done:
		// Good — Run returned
	case <-time.After(5 * time.Second):
		t.Fatal("Akira.Run did not stop after context cancellation")
	}
}

// Sweeps counts COMPLETED sweeps only: /ready depends on it, and a count that
// moved before the last target was checked would report ready with a
// partially filled state map.
func TestSweeps_CountsCompletedSweeps(t *testing.T) {
	var seenDuring uint64
	var a *Akira
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		seenDuring = a.Sweeps()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a = NewAkira("node-1", []ServiceTarget{{Name: "svc", Host: "127.0.0.1", Port: srv.Listener.Addr().(*net.TCPAddr).Port, HealthPath: "/"}})
	if got := a.Sweeps(); got != 0 {
		t.Fatalf("Sweeps before any sweep = %d, want 0", got)
	}
	a.CheckAll()
	if seenDuring != 0 {
		t.Fatalf("Sweeps during the first sweep = %d, want 0", seenDuring)
	}
	if got := a.Sweeps(); got != 1 {
		t.Fatalf("Sweeps after one sweep = %d, want 1", got)
	}
	a.CheckAll()
	if got := a.Sweeps(); got != 2 {
		t.Fatalf("Sweeps after two sweeps = %d, want 2", got)
	}
}

// Akira tallies its own checks and other reporters' reports together, calls
// OnSeverityChange once per change of band, and OnAlert while a service is
// at the two-thirds threshold.
func TestAkira_ConsensusAcrossReporters(t *testing.T) {
	a := NewAkira("west", nil)
	var changes []string
	var alerts []string
	a.OnSeverityChange(func(v Verdict) { changes = append(changes, v.Service+":"+string(v.Severity)) })
	a.OnAlert(func(v Verdict) { alerts = append(alerts, v.Service) })

	now := time.Now()
	a.Record(HealthReport{Service: "wotan", Reporter: "west", Healthy: false, Timestamp: now})
	a.Record(HealthReport{Service: "wotan", Reporter: "east", Healthy: true, Timestamp: now})
	a.EvaluateConsensus() // 1 of 2: ERROR, below threshold
	a.EvaluateConsensus() // unchanged: no second change callback
	if fmt.Sprint(changes) != "[wotan:ERROR]" || len(alerts) != 0 {
		t.Fatalf("changes %v alerts %v, want [wotan:ERROR] and none", changes, alerts)
	}

	a.Record(HealthReport{Service: "wotan", Reporter: "east", Healthy: false, Timestamp: now.Add(time.Second)})
	got := a.EvaluateConsensus() // 2 of 2: PANIC, remediate
	if len(got) != 1 || got[0].Reporters != 2 || !got[0].Remediate {
		t.Fatalf("alerts %+v, want wotan with 2 reporters", got)
	}
	if fmt.Sprint(changes) != "[wotan:ERROR wotan:PANIC]" || fmt.Sprint(alerts) != "[wotan]" {
		t.Errorf("changes %v alerts %v", changes, alerts)
	}
	if st := a.GetStates()["wotan"]; st.Severity != SeverityPanic || st.Failing != 2 {
		t.Errorf("GetStates wotan = %+v", st)
	}
}

// One reporter failing a check five times is one vote, not 5/5 = 100 %.
func TestAkira_OwnChecksAreOneVote(t *testing.T) {
	a := NewAkira("west", []ServiceTarget{{Name: "dead", Host: "127.0.0.1", Port: 1}})
	for i := 0; i < 5; i++ {
		a.CheckAll()
	}
	a.Record(HealthReport{Service: "dead", Reporter: "east", Healthy: true, Timestamp: time.Now()})
	a.Record(HealthReport{Service: "dead", Reporter: "north", Healthy: true, Timestamp: time.Now()})
	if got := a.EvaluateConsensus(); len(got) != 0 {
		t.Errorf("alerts %+v, want none: 1 of 3 reporters failing", got)
	}
	if st := a.GetStates()["dead"]; st.Reporters != 3 || st.Failing != 1 || st.Severity != SeverityWarn {
		t.Errorf("dead = %+v, want 1 of 3, WARN", st)
	}
}
