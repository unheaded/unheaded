// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package health

import (
	"context"
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

func TestConsensusEvaluation(t *testing.T) {
	akira := NewAkira("test-node", nil)

	// Simulate 5 health checks — 4 failing (80% > 66.67%)
	for i := 0; i < 5; i++ {
		report := HealthReport{
			Service:   "failing-svc",
			Reporter:  "test-node",
			Healthy:   i == 0, // Only first is healthy
			Timestamp: time.Now(),
		}
		akira.mu.Lock()
		state, ok := akira.states["failing-svc"]
		if !ok {
			state = &ConsensusState{Service: "failing-svc"}
			akira.states["failing-svc"] = state
		}
		state.Reports = append(state.Reports, report)
		akira.mu.Unlock()
	}

	alerts := akira.EvaluateConsensus()
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Service != "failing-svc" {
		t.Errorf("Expected failing-svc, got %s", alerts[0].Service)
	}
	if alerts[0].FailureRate < ConsensusThreshold {
		t.Errorf("Expected failure rate >= %f, got %f", ConsensusThreshold, alerts[0].FailureRate)
	}
}

func TestNoConsensusWhenHealthy(t *testing.T) {
	akira := NewAkira("test-node", nil)

	// All healthy — should NOT trigger
	for i := 0; i < 5; i++ {
		report := HealthReport{
			Service:   "healthy-svc",
			Reporter:  "test-node",
			Healthy:   true,
			Timestamp: time.Now(),
		}
		akira.mu.Lock()
		state, ok := akira.states["healthy-svc"]
		if !ok {
			state = &ConsensusState{Service: "healthy-svc"}
			akira.states["healthy-svc"] = state
		}
		state.Reports = append(state.Reports, report)
		akira.mu.Unlock()
	}

	alerts := akira.EvaluateConsensus()
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts for healthy service, got %d", len(alerts))
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

func TestConsensusAtExactThreshold(t *testing.T) {
	akira := NewAkira("test-node", nil)

	// 3 reports: 2 failing = 66.67% = exactly at threshold
	for i := 0; i < 3; i++ {
		report := HealthReport{
			Service:   "edge-svc",
			Reporter:  "test-node",
			Healthy:   i == 0, // 1 healthy, 2 unhealthy = 66.67%
			Timestamp: time.Now(),
		}
		akira.mu.Lock()
		state, ok := akira.states["edge-svc"]
		if !ok {
			state = &ConsensusState{Service: "edge-svc"}
			akira.states["edge-svc"] = state
		}
		state.Reports = append(state.Reports, report)
		akira.mu.Unlock()
	}

	alerts := akira.EvaluateConsensus()
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert at exact threshold (66.67%%), got %d", len(alerts))
	}
	if alerts[0].FailureRate < ConsensusThreshold {
		t.Errorf("Failure rate %.4f should be >= threshold %.4f", alerts[0].FailureRate, ConsensusThreshold)
	}
}

func TestConsensusBelowThreshold(t *testing.T) {
	akira := NewAkira("test-node", nil)

	// 5 reports: 3 healthy, 2 failing = 40% < 66.67%
	for i := 0; i < 5; i++ {
		report := HealthReport{
			Service:   "ok-svc",
			Reporter:  "test-node",
			Healthy:   i < 3, // 3 healthy, 2 failing = 40%
			Timestamp: time.Now(),
		}
		akira.mu.Lock()
		state, ok := akira.states["ok-svc"]
		if !ok {
			state = &ConsensusState{Service: "ok-svc"}
			akira.states["ok-svc"] = state
		}
		state.Reports = append(state.Reports, report)
		akira.mu.Unlock()
	}

	alerts := akira.EvaluateConsensus()
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts at 40%% failure, got %d", len(alerts))
	}
}

func TestMultipleServicesConsensus(t *testing.T) {
	akira := NewAkira("test-node", nil)

	// 2 services: one failing, one healthy
	for i := 0; i < 5; i++ {
		akira.mu.Lock()
		// Failing service
		fs, ok := akira.states["bad-svc"]
		if !ok {
			fs = &ConsensusState{Service: "bad-svc"}
			akira.states["bad-svc"] = fs
		}
		fs.Reports = append(fs.Reports, HealthReport{Service: "bad-svc", Healthy: false, Timestamp: time.Now()})

		// Healthy service
		gs, ok := akira.states["good-svc"]
		if !ok {
			gs = &ConsensusState{Service: "good-svc"}
			akira.states["good-svc"] = gs
		}
		gs.Reports = append(gs.Reports, HealthReport{Service: "good-svc", Healthy: true, Timestamp: time.Now()})
		akira.mu.Unlock()
	}

	alerts := akira.EvaluateConsensus()
	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert (bad-svc only), got %d", len(alerts))
	}
	if alerts[0].Service != "bad-svc" {
		t.Errorf("Expected bad-svc alert, got %s", alerts[0].Service)
	}
}
