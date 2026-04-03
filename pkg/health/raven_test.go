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

	raven := NewRaven("test-node", []ServiceTarget{
		{Name: "test-svc", Host: "127.0.0.1", Port: 0, HealthPath: "/health"},
	})

	// Parse the test server's port
	report := raven.CheckService(ServiceTarget{
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
	raven := NewRaven("test-node", nil)

	// Check a port that's definitely not listening
	report := raven.CheckService(ServiceTarget{
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
	raven := NewRaven("test-node", nil)

	// Simulate 5 health checks — 4 failing (80% > 66.67%)
	for i := 0; i < 5; i++ {
		report := HealthReport{
			Service:   "failing-svc",
			Reporter:  "test-node",
			Healthy:   i == 0, // Only first is healthy
			Timestamp: time.Now(),
		}
		raven.mu.Lock()
		state, ok := raven.states["failing-svc"]
		if !ok {
			state = &ConsensusState{Service: "failing-svc"}
			raven.states["failing-svc"] = state
		}
		state.Reports = append(state.Reports, report)
		raven.mu.Unlock()
	}

	alerts := raven.EvaluateConsensus()
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
	raven := NewRaven("test-node", nil)

	// All healthy — should NOT trigger
	for i := 0; i < 5; i++ {
		report := HealthReport{
			Service:   "healthy-svc",
			Reporter:  "test-node",
			Healthy:   true,
			Timestamp: time.Now(),
		}
		raven.mu.Lock()
		state, ok := raven.states["healthy-svc"]
		if !ok {
			state = &ConsensusState{Service: "healthy-svc"}
			raven.states["healthy-svc"] = state
		}
		state.Reports = append(state.Reports, report)
		raven.mu.Unlock()
	}

	alerts := raven.EvaluateConsensus()
	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts for healthy service, got %d", len(alerts))
	}
}

func TestRavenRunCancellation(t *testing.T) {
	raven := NewRaven("test-node", nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		raven.Run(ctx)
		close(done)
	}()

	// Cancel immediately
	cancel()

	select {
	case <-done:
		// Good — Run returned
	case <-time.After(5 * time.Second):
		t.Fatal("Raven.Run did not stop after context cancellation")
	}
}
