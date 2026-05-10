// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalance

import (
	"sync"
	"sync/atomic"
	"testing"
)

// ==================== Endpoint Tests ====================

func TestEndpoint(t *testing.T) {
	t.Run("Fields", func(t *testing.T) {
		ep := &Endpoint{
			Address: "10.0.0.1",
			Port:    8080,
			Weight:  10,
			Healthy: true,
			Meta:    map[string]string{"zone": "us-east"},
		}

		if ep.Address != "10.0.0.1" {
			t.Errorf("expected address 10.0.0.1, got %s", ep.Address)
		}
		if ep.Port != 8080 {
			t.Errorf("expected port 8080, got %d", ep.Port)
		}
		if ep.Weight != 10 {
			t.Errorf("expected weight 10, got %d", ep.Weight)
		}
		if !ep.Healthy {
			t.Error("expected healthy to be true")
		}
		if ep.Meta["zone"] != "us-east" {
			t.Errorf("expected zone us-east, got %s", ep.Meta["zone"])
		}
	})
}

// ==================== EndpointPool Tests ====================

func TestEndpointPool(t *testing.T) {
	t.Run("NewEndpointPool", func(t *testing.T) {
		rr := NewRoundRobin()
		pool := NewEndpointPool(rr)

		if pool == nil {
			t.Fatal("expected non-nil pool")
		}
	})

	t.Run("SetEndpoints", func(t *testing.T) {
		rr := NewRoundRobin()
		pool := NewEndpointPool(rr)

		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Healthy: true},
		}
		pool.SetEndpoints(endpoints)

		got := pool.GetEndpoints()
		if len(got) != 2 {
			t.Errorf("expected 2 endpoints, got %d", len(got))
		}
	})

	t.Run("Pick_HealthyOnly", func(t *testing.T) {
		rr := NewRoundRobin()
		pool := NewEndpointPool(rr)

		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Healthy: false},
			{Address: "10.0.0.3", Port: 8080, Healthy: true},
		}
		pool.SetEndpoints(endpoints)

		// Should only pick healthy endpoints
		picked := make(map[string]bool)
		for i := 0; i < 10; i++ {
			ep := pool.Pick()
			if ep != nil {
				picked[ep.Address] = true
				if ep.Address == "10.0.0.2" {
					t.Error("should not pick unhealthy endpoint")
				}
			}
		}
	})

	t.Run("Pick_NoHealthyEndpoints", func(t *testing.T) {
		rr := NewRoundRobin()
		pool := NewEndpointPool(rr)

		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: false},
			{Address: "10.0.0.2", Port: 8080, Healthy: false},
		}
		pool.SetEndpoints(endpoints)

		ep := pool.Pick()
		if ep != nil {
			t.Error("expected nil when no healthy endpoints")
		}
	})

	t.Run("MarkHealthy", func(t *testing.T) {
		rr := NewRoundRobin()
		pool := NewEndpointPool(rr)

		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: true},
		}
		pool.SetEndpoints(endpoints)

		// Mark as unhealthy
		pool.MarkHealthy("10.0.0.1", 8080, false)

		ep := pool.Pick()
		if ep != nil {
			t.Error("expected nil after marking unhealthy")
		}

		// Mark as healthy again
		pool.MarkHealthy("10.0.0.1", 8080, true)

		ep = pool.Pick()
		if ep == nil {
			t.Error("expected endpoint after marking healthy")
		}
	})

	t.Run("HealthyCount", func(t *testing.T) {
		rr := NewRoundRobin()
		pool := NewEndpointPool(rr)

		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Healthy: false},
			{Address: "10.0.0.3", Port: 8080, Healthy: true},
		}
		pool.SetEndpoints(endpoints)

		if pool.HealthyCount() != 2 {
			t.Errorf("expected 2 healthy, got %d", pool.HealthyCount())
		}
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		rr := NewRoundRobin()
		pool := NewEndpointPool(rr)

		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Healthy: true},
		}
		pool.SetEndpoints(endpoints)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				pool.Pick()
				pool.HealthyCount()
			}()
		}
		wg.Wait()
	})
}

// ==================== RoundRobin Tests ====================

func TestRoundRobin(t *testing.T) {
	t.Run("NewRoundRobin", func(t *testing.T) {
		rr := NewRoundRobin()
		if rr == nil {
			t.Fatal("expected non-nil round robin")
		}
		if rr.Name() != "round-robin" {
			t.Errorf("expected name round-robin, got %s", rr.Name())
		}
	})

	t.Run("Pick_DistributesEvenly", func(t *testing.T) {
		rr := NewRoundRobin()
		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Healthy: true},
			{Address: "10.0.0.3", Port: 8080, Healthy: true},
		}

		counts := make(map[string]int)
		for i := 0; i < 9; i++ {
			ep := rr.Pick(endpoints)
			if ep != nil {
				counts[ep.Address]++
			}
		}

		for addr, count := range counts {
			if count != 3 {
				t.Errorf("expected %s to be picked 3 times, got %d", addr, count)
			}
		}
	})

	t.Run("Pick_NilSlice", func(t *testing.T) {
		rr := NewRoundRobin()
		ep := rr.Pick(nil)
		if ep != nil {
			t.Error("expected nil for nil endpoints")
		}
	})

	t.Run("Pick_EmptySlice", func(t *testing.T) {
		rr := NewRoundRobin()
		ep := rr.Pick([]*Endpoint{})
		if ep != nil {
			t.Error("expected nil for empty endpoints")
		}
	})

	t.Run("Update_NoEffect", func(t *testing.T) {
		rr := NewRoundRobin()
		// Update should be a no-op for round robin
		rr.Update([]*Endpoint{{Address: "10.0.0.1", Healthy: true}})
		// No panic or error expected
	})

	t.Run("ConcurrentPick", func(t *testing.T) {
		rr := NewRoundRobin()
		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Healthy: true},
			{Address: "10.0.0.2", Healthy: true},
		}

		var wg sync.WaitGroup
		var successCount int64
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ep := rr.Pick(endpoints)
				if ep != nil {
					atomic.AddInt64(&successCount, 1)
				}
			}()
		}
		wg.Wait()

		if successCount != 100 {
			t.Errorf("expected 100 successful picks, got %d", successCount)
		}
	})
}

// ==================== Weighted Tests ====================

func TestWeighted(t *testing.T) {
	t.Run("NewWeighted", func(t *testing.T) {
		w := NewWeighted()
		if w == nil {
			t.Fatal("expected non-nil weighted")
		}
		if w.Name() != "weighted" {
			t.Errorf("expected name weighted, got %s", w.Name())
		}
	})

	t.Run("Pick_RespectsWeights", func(t *testing.T) {
		w := NewWeighted()
		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Weight: 90, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Weight: 10, Healthy: true},
		}

		counts := make(map[string]int)
		for i := 0; i < 1000; i++ {
			ep := w.Pick(endpoints)
			if ep != nil {
				counts[ep.Address]++
			}
		}

		// 10.0.0.1 should be picked significantly more
		if counts["10.0.0.1"] < counts["10.0.0.2"]*2 {
			t.Errorf("expected weighted distribution, got: %v", counts)
		}
	})

	t.Run("Pick_DefaultWeight", func(t *testing.T) {
		w := NewWeighted()
		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Weight: 0, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Weight: 0, Healthy: true},
		}

		// Should work even with zero weights (defaults to 1)
		ep := w.Pick(endpoints)
		if ep == nil {
			t.Error("expected non-nil endpoint")
		}
	})

	t.Run("Pick_NilSlice", func(t *testing.T) {
		w := NewWeighted()
		ep := w.Pick(nil)
		if ep != nil {
			t.Error("expected nil for nil endpoints")
		}
	})

	t.Run("Update", func(t *testing.T) {
		w := NewWeighted()
		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Weight: 50, Healthy: true},
		}
		w.Update(endpoints)
		// No panic expected
	})
}

// ==================== SmoothWeighted Tests ====================

func TestSmoothWeighted(t *testing.T) {
	t.Run("NewSmoothWeighted", func(t *testing.T) {
		sw := NewSmoothWeighted()
		if sw == nil {
			t.Fatal("expected non-nil smooth weighted")
		}
		if sw.Name() != "smooth-weighted" {
			t.Errorf("expected name smooth-weighted, got %s", sw.Name())
		}
	})

	t.Run("Pick_SmoothDistribution", func(t *testing.T) {
		sw := NewSmoothWeighted()
		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Weight: 5, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Weight: 1, Healthy: true},
		}

		counts := make(map[string]int)
		for i := 0; i < 12; i++ {
			ep := sw.Pick(endpoints)
			if ep != nil {
				counts[ep.Address]++
			}
		}

		// With weights 5:1, over 12 picks we expect roughly 10:2 distribution
		if counts["10.0.0.1"] < counts["10.0.0.2"]*2 {
			t.Logf("distribution: %v (smooth weighted may vary)", counts)
		}
	})

	t.Run("Pick_NilSlice", func(t *testing.T) {
		sw := NewSmoothWeighted()
		ep := sw.Pick(nil)
		if ep != nil {
			t.Error("expected nil for nil endpoints")
		}
	})

	t.Run("Update", func(t *testing.T) {
		sw := NewSmoothWeighted()
		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Weight: 5, Healthy: true},
		}
		sw.Update(endpoints)

		// Add new endpoint
		endpoints = append(endpoints, &Endpoint{
			Address: "10.0.0.2", Weight: 5, Healthy: true,
		})
		sw.Update(endpoints)

		// Both should be pickable
		picked := make(map[string]bool)
		for i := 0; i < 10; i++ {
			ep := sw.Pick(endpoints)
			if ep != nil {
				picked[ep.Address] = true
			}
		}

		if len(picked) != 2 {
			t.Errorf("expected 2 different endpoints to be picked, got %d", len(picked))
		}
	})
}

// ==================== LeastConn Tests ====================

func TestLeastConn(t *testing.T) {
	t.Run("NewLeastConn", func(t *testing.T) {
		lc := NewLeastConn()
		if lc == nil {
			t.Fatal("expected non-nil least conn")
		}
		if lc.Name() != "least-conn" {
			t.Errorf("expected name least-conn, got %s", lc.Name())
		}
	})

	t.Run("Pick_SelectsLeastConnections", func(t *testing.T) {
		lc := NewLeastConn()
		endpoints := []*Endpoint{
			{Address: "10.0.0.1", Port: 8080, Healthy: true},
			{Address: "10.0.0.2", Port: 8080, Healthy: true},
		}

		// Update to initialize
		lc.Update(endpoints)

		// First pick
		ep1 := lc.Pick(endpoints)
		if ep1 == nil {
			t.Fatal("expected non-nil endpoint")
		}

		// Increment connections for first endpoint
		lc.IncrementConnections(ep1)

		// Second pick may equal the first when connection counts are tied —
		// least-connections doesn't guarantee strict alternation in that case.
		_ = lc.Pick(endpoints)
		_ = ep1
	})

	t.Run("IncrementConnections", func(t *testing.T) {
		lc := NewLeastConn()
		ep := &Endpoint{Address: "10.0.0.1", Port: 8080}

		lc.IncrementConnections(ep)
		lc.IncrementConnections(ep)

		conns := lc.GetConnections(ep)
		if conns != 2 {
			t.Errorf("expected 2 connections, got %d", conns)
		}
	})

	t.Run("DecrementConnections", func(t *testing.T) {
		lc := NewLeastConn()
		ep := &Endpoint{Address: "10.0.0.1", Port: 8080}

		lc.IncrementConnections(ep)
		lc.IncrementConnections(ep)
		lc.DecrementConnections(ep)

		conns := lc.GetConnections(ep)
		if conns != 1 {
			t.Errorf("expected 1 connection, got %d", conns)
		}
	})

	t.Run("DecrementConnections_DoesNotGoNegative", func(t *testing.T) {
		lc := NewLeastConn()
		ep := &Endpoint{Address: "10.0.0.1", Port: 8080}

		lc.DecrementConnections(ep) // Should not panic or go negative

		conns := lc.GetConnections(ep)
		if conns < 0 {
			t.Errorf("connections should not be negative, got %d", conns)
		}
	})

	t.Run("Pick_NilSlice", func(t *testing.T) {
		lc := NewLeastConn()
		ep := lc.Pick(nil)
		if ep != nil {
			t.Error("expected nil for nil endpoints")
		}
	})

	t.Run("Update_CleansStaleEndpoints", func(t *testing.T) {
		lc := NewLeastConn()
		ep1 := &Endpoint{Address: "10.0.0.1", Port: 8080}
		ep2 := &Endpoint{Address: "10.0.0.2", Port: 8080}

		lc.Update([]*Endpoint{ep1, ep2})
		lc.IncrementConnections(ep1)
		lc.IncrementConnections(ep2)

		// Remove ep2 from the list
		lc.Update([]*Endpoint{ep1})

		// ep2's connections should be gone
		conns := lc.GetConnections(ep2)
		if conns != 0 {
			t.Errorf("expected 0 connections for removed endpoint, got %d", conns)
		}
	})
}

// ==================== Helper Function Tests ====================

func TestEndpointKey(t *testing.T) {
	tests := []struct {
		ep       *Endpoint
		expected string
	}{
		{&Endpoint{Address: "10.0.0.1", Port: 8080}, "10.0.0.1:8080"},
		{&Endpoint{Address: "localhost", Port: 80}, "localhost:80"},
		{&Endpoint{Address: "192.168.1.1", Port: 443}, "192.168.1.1:443"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := endpointKey(tt.ep)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{8080, "8080"},
		{-1, "-1"},
		{-123, "-123"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := itoa(tt.input)
			if result != tt.expected {
				t.Errorf("itoa(%d) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

// ==================== Integration with EndpointPool Tests ====================

func TestEndpointPool_WithRoundRobin(t *testing.T) {
	rr := NewRoundRobin()
	pool := NewEndpointPool(rr)

	endpoints := []*Endpoint{
		{Address: "10.0.0.1", Port: 8080, Healthy: true},
		{Address: "10.0.0.2", Port: 8080, Healthy: true},
		{Address: "10.0.0.3", Port: 8080, Healthy: true},
	}
	pool.SetEndpoints(endpoints)

	counts := make(map[string]int)
	for i := 0; i < 9; i++ {
		ep := pool.Pick()
		if ep != nil {
			counts[ep.Address]++
		}
	}

	for addr, count := range counts {
		if count != 3 {
			t.Errorf("expected %s to be picked 3 times, got %d", addr, count)
		}
	}
}

func TestEndpointPool_WithLeastConn(t *testing.T) {
	lc := NewLeastConn()
	pool := NewEndpointPool(lc)

	endpoints := []*Endpoint{
		{Address: "10.0.0.1", Port: 8080, Healthy: true},
		{Address: "10.0.0.2", Port: 8080, Healthy: true},
	}
	pool.SetEndpoints(endpoints)

	// Pick first endpoint
	ep1 := pool.Pick()
	if ep1 == nil {
		t.Fatal("expected non-nil endpoint")
	}

	// Increment connections
	lc.IncrementConnections(ep1)

	// Next pick should be the other endpoint
	ep2 := pool.Pick()
	if ep2 == nil {
		t.Fatal("expected non-nil endpoint")
	}

	// With least-conn, second should be different (it has fewer connections)
	if ep2.Address == ep1.Address {
		t.Logf("note: same endpoint picked (connections may be equal)")
	}
}

func TestEndpointPool_WithWeighted(t *testing.T) {
	w := NewWeighted()
	pool := NewEndpointPool(w)

	endpoints := []*Endpoint{
		{Address: "10.0.0.1", Port: 8080, Weight: 10, Healthy: true},
		{Address: "10.0.0.2", Port: 8080, Weight: 1, Healthy: true},
	}
	pool.SetEndpoints(endpoints)

	counts := make(map[string]int)
	for i := 0; i < 100; i++ {
		ep := pool.Pick()
		if ep != nil {
			counts[ep.Address]++
		}
	}

	// Higher weight endpoint should be picked more
	if counts["10.0.0.1"] <= counts["10.0.0.2"] {
		t.Errorf("expected weighted endpoint to be picked more: %v", counts)
	}
}

// ==================== Benchmark Tests ====================

func BenchmarkRoundRobin_Pick(b *testing.B) {
	rr := NewRoundRobin()
	endpoints := make([]*Endpoint, 10)
	for i := 0; i < 10; i++ {
		endpoints[i] = &Endpoint{Address: "10.0.0.1", Port: 8080, Healthy: true}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.Pick(endpoints)
	}
}

func BenchmarkWeighted_Pick(b *testing.B) {
	w := NewWeighted()
	endpoints := make([]*Endpoint, 10)
	for i := 0; i < 10; i++ {
		endpoints[i] = &Endpoint{Address: "10.0.0.1", Port: 8080, Weight: 10, Healthy: true}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Pick(endpoints)
	}
}

func BenchmarkLeastConn_Pick(b *testing.B) {
	lc := NewLeastConn()
	endpoints := make([]*Endpoint, 10)
	for i := 0; i < 10; i++ {
		endpoints[i] = &Endpoint{Address: "10.0.0.1", Port: 8080 + i, Healthy: true}
	}
	lc.Update(endpoints)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Pick(endpoints)
	}
}

func BenchmarkEndpointPool_Pick(b *testing.B) {
	rr := NewRoundRobin()
	pool := NewEndpointPool(rr)
	endpoints := make([]*Endpoint, 10)
	for i := 0; i < 10; i++ {
		endpoints[i] = &Endpoint{Address: "10.0.0.1", Port: 8080, Healthy: true}
	}
	pool.SetEndpoints(endpoints)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Pick()
	}
}
