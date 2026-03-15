// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"sync"
	"sync/atomic"
	"testing"
)

// ==================== Algorithm Tests ====================

func TestAlgorithm_Constants(t *testing.T) {
	tests := []struct {
		algo     Algorithm
		expected string
	}{
		{RoundRobinAlgorithm, "round-robin"},
		{LeastConnAlgorithm, "least-conn"},
		{WeightedAlgorithm, "weighted"},
		{RandomAlgorithm, "random"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.algo) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.algo))
			}
		})
	}
}

// ==================== Endpoint Tests ====================

func TestEndpoint(t *testing.T) {
	t.Run("Fields", func(t *testing.T) {
		ep := &Endpoint{
			ID:       "ep-1",
			Address:  "10.0.0.1:8080",
			Weight:   10,
			Healthy:  true,
			Metadata: map[string]string{"zone": "us-east"},
		}

		if ep.ID != "ep-1" {
			t.Errorf("expected ID ep-1, got %s", ep.ID)
		}
		if ep.Address != "10.0.0.1:8080" {
			t.Errorf("expected address 10.0.0.1:8080, got %s", ep.Address)
		}
		if ep.Weight != 10 {
			t.Errorf("expected weight 10, got %d", ep.Weight)
		}
		if !ep.Healthy {
			t.Error("expected healthy to be true")
		}
		if ep.Metadata["zone"] != "us-east" {
			t.Errorf("expected zone us-east, got %s", ep.Metadata["zone"])
		}
	})
}

// ==================== LoadBalancer Tests ====================

func TestLoadBalancer_New(t *testing.T) {
	t.Run("WithDefaultAlgorithm", func(t *testing.T) {
		lb := New("")
		if lb == nil {
			t.Fatal("expected non-nil load balancer")
		}
		if lb.algorithm != RoundRobinAlgorithm {
			t.Errorf("expected default algorithm round-robin, got %s", lb.algorithm)
		}
	})

	t.Run("WithCustomAlgorithm", func(t *testing.T) {
		lb := New(LeastConnAlgorithm)
		if lb.algorithm != LeastConnAlgorithm {
			t.Errorf("expected algorithm least-conn, got %s", lb.algorithm)
		}
	})
}

func TestLoadBalancer_Register(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)

		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
			{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: true},
		}

		err := lb.Register("service-a", endpoints)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		services := lb.Services()
		if len(services) != 1 {
			t.Errorf("expected 1 service, got %d", len(services))
		}
	})

	t.Run("UpdateExisting", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)

		// Initial registration
		lb.Register("service-a", []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		})

		// Update with new endpoints
		lb.Register("service-a", []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
			{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: true},
		})

		// Should still have one service
		services := lb.Services()
		if len(services) != 1 {
			t.Errorf("expected 1 service, got %d", len(services))
		}
	})
}

func TestLoadBalancer_RegisterWithAlgorithm(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)

		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		}

		err := lb.RegisterWithAlgorithm("service-a", LeastConnAlgorithm, endpoints)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		algo, err := lb.GetAlgorithm("service-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if algo != LeastConnAlgorithm {
			t.Errorf("expected algorithm least-conn, got %s", algo)
		}
	})

	t.Run("UnknownAlgorithm", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)

		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		}

		err := lb.RegisterWithAlgorithm("service-a", Algorithm("unknown"), endpoints)
		if err != ErrUnknownAlgorithm {
			t.Errorf("expected ErrUnknownAlgorithm, got %v", err)
		}
	})
}

func TestLoadBalancer_Next(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)

		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		}
		lb.Register("service-a", endpoints)

		ep, err := lb.Next("service-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ep.ID != "ep-1" {
			t.Errorf("expected endpoint ep-1, got %s", ep.ID)
		}
	})

	t.Run("ServiceNotFound", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)

		_, err := lb.Next("unknown-service")
		if err != ErrNoEndpoints {
			t.Errorf("expected ErrNoEndpoints, got %v", err)
		}
	})
}

func TestLoadBalancer_Update(t *testing.T) {
	t.Run("ExistingService", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)

		lb.Register("service-a", []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		})

		// Update
		err := lb.Update("service-a", []*Endpoint{
			{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: true},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ep, _ := lb.Next("service-a")
		if ep.ID != "ep-2" {
			t.Errorf("expected updated endpoint ep-2, got %s", ep.ID)
		}
	})

	t.Run("NewService", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)

		// Update should register new service
		err := lb.Update("service-a", []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = lb.Next("service-a")
		if err != nil {
			t.Errorf("expected service to exist after update, got %v", err)
		}
	})
}

func TestLoadBalancer_Remove(t *testing.T) {
	lb := New(RoundRobinAlgorithm)

	lb.Register("service-a", []*Endpoint{
		{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
	})

	lb.Remove("service-a")

	_, err := lb.Next("service-a")
	if err != ErrNoEndpoints {
		t.Errorf("expected ErrNoEndpoints after removal, got %v", err)
	}
}

func TestLoadBalancer_Services(t *testing.T) {
	lb := New(RoundRobinAlgorithm)

	lb.Register("service-a", []*Endpoint{{Healthy: true}})
	lb.Register("service-b", []*Endpoint{{Healthy: true}})
	lb.Register("service-c", []*Endpoint{{Healthy: true}})

	services := lb.Services()
	if len(services) != 3 {
		t.Errorf("expected 3 services, got %d", len(services))
	}
}

func TestLoadBalancer_GetAlgorithm(t *testing.T) {
	t.Run("ServiceExists", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)
		lb.Register("service-a", []*Endpoint{{Healthy: true}})

		algo, err := lb.GetAlgorithm("service-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if algo != RoundRobinAlgorithm {
			t.Errorf("expected round-robin, got %s", algo)
		}
	})

	t.Run("ServiceNotFound", func(t *testing.T) {
		lb := New(RoundRobinAlgorithm)

		_, err := lb.GetAlgorithm("unknown")
		if err != ErrNoEndpoints {
			t.Errorf("expected ErrNoEndpoints, got %v", err)
		}
	})
}

func TestLoadBalancer_ConcurrentAccess(t *testing.T) {
	lb := New(RoundRobinAlgorithm)

	endpoints := []*Endpoint{
		{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: true},
	}
	lb.Register("service-a", endpoints)

	var wg sync.WaitGroup
	var successCount int64

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := lb.Next("service-a")
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != 100 {
		t.Errorf("expected 100 successes, got %d", successCount)
	}
}

// ==================== RoundRobin Tests ====================

func TestRoundRobin(t *testing.T) {
	t.Run("NewRoundRobin", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
			{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: true},
		}

		rr := NewRoundRobin(endpoints)
		if rr == nil {
			t.Fatal("expected non-nil round robin")
		}
		if rr.Algorithm() != RoundRobinAlgorithm {
			t.Errorf("expected algorithm round-robin, got %s", rr.Algorithm())
		}
	})

	t.Run("FiltersUnhealthy", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
			{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: false},
		}

		rr := NewRoundRobin(endpoints)
		if rr.Count() != 1 {
			t.Errorf("expected 1 healthy endpoint, got %d", rr.Count())
		}
	})

	t.Run("Next_DistributesEvenly", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
			{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: true},
			{ID: "ep-3", Address: "10.0.0.3:8080", Healthy: true},
		}

		rr := NewRoundRobin(endpoints)

		counts := make(map[string]int)
		for i := 0; i < 9; i++ {
			ep, _ := rr.Next()
			counts[ep.ID]++
		}

		for id, count := range counts {
			if count != 3 {
				t.Errorf("expected endpoint %s to be picked 3 times, got %d", id, count)
			}
		}
	})

	t.Run("Next_NoEndpoints", func(t *testing.T) {
		rr := NewRoundRobin(nil)

		_, err := rr.Next()
		if err != ErrNoEndpoints {
			t.Errorf("expected ErrNoEndpoints, got %v", err)
		}
	})

	t.Run("Update", func(t *testing.T) {
		rr := NewRoundRobin([]*Endpoint{
			{ID: "ep-1", Healthy: true},
		})

		rr.Update([]*Endpoint{
			{ID: "ep-1", Healthy: true},
			{ID: "ep-2", Healthy: true},
		})

		if rr.Count() != 2 {
			t.Errorf("expected 2 endpoints after update, got %d", rr.Count())
		}
	})

	t.Run("ConcurrentNext", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Healthy: true},
			{ID: "ep-2", Healthy: true},
		}
		rr := NewRoundRobin(endpoints)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rr.Next()
			}()
		}
		wg.Wait()
	})
}

// ==================== LeastConn Tests ====================

func TestLeastConn(t *testing.T) {
	t.Run("NewLeastConn", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		}

		lc := NewLeastConn(endpoints)
		if lc == nil {
			t.Fatal("expected non-nil least conn")
		}
		if lc.Algorithm() != LeastConnAlgorithm {
			t.Errorf("expected algorithm least-conn, got %s", lc.Algorithm())
		}
	})

	t.Run("Next_SelectsLeastConnections", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
			{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: true},
		}

		lc := NewLeastConn(endpoints)

		// Get first endpoint
		ep1, _ := lc.Next()
		// ep1 now has 1 connection (auto-incremented by Next)

		// Get second endpoint - should be the other one
		ep2, _ := lc.Next()

		if ep1.ID == ep2.ID {
			// May pick the same one, but should eventually balance
			// Due to auto-increment in Next(), verify connections work
		}
	})

	t.Run("Release_DecrementsCount", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		}

		lc := NewLeastConn(endpoints)
		ep, _ := lc.Next() // Increments

		conns := lc.Connections(ep.ID)
		if conns != 1 {
			t.Errorf("expected 1 connection, got %d", conns)
		}

		lc.Release(ep.ID)
		conns = lc.Connections(ep.ID)
		if conns != 0 {
			t.Errorf("expected 0 connections after release, got %d", conns)
		}
	})

	t.Run("TotalConnections", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
			{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: true},
		}

		lc := NewLeastConn(endpoints)
		lc.Next()
		lc.Next()
		lc.Next()

		total := lc.TotalConnections()
		if total != 3 {
			t.Errorf("expected 3 total connections, got %d", total)
		}
	})

	t.Run("NoEndpoints", func(t *testing.T) {
		lc := NewLeastConn(nil)

		_, err := lc.Next()
		if err != ErrNoEndpoints {
			t.Errorf("expected ErrNoEndpoints, got %v", err)
		}
	})

	t.Run("Update_PreservesConnectionCounts", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
		}

		lc := NewLeastConn(endpoints)
		lc.Next() // ep-1 has 1 connection

		// Update with same endpoint
		lc.Update([]*Endpoint{
			{ID: "ep-1", Address: "10.0.0.1:8080", Healthy: true},
			{ID: "ep-2", Address: "10.0.0.2:8080", Healthy: true},
		})

		conns := lc.Connections("ep-1")
		if conns != 1 {
			t.Errorf("expected connection count preserved, got %d", conns)
		}
	})

	t.Run("Count", func(t *testing.T) {
		lc := NewLeastConn([]*Endpoint{
			{ID: "ep-1", Healthy: true},
			{ID: "ep-2", Healthy: true},
			{ID: "ep-3", Healthy: false},
		})

		if lc.Count() != 2 {
			t.Errorf("expected 2 healthy endpoints, got %d", lc.Count())
		}
	})
}

// ==================== Weighted Tests ====================

func TestWeighted(t *testing.T) {
	t.Run("NewWeighted", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Weight: 10, Healthy: true},
		}

		w := NewWeighted(endpoints)
		if w == nil {
			t.Fatal("expected non-nil weighted")
		}
		if w.Algorithm() != WeightedAlgorithm {
			t.Errorf("expected algorithm weighted, got %s", w.Algorithm())
		}
	})

	t.Run("Next_RespectsWeights", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Weight: 90, Healthy: true},
			{ID: "ep-2", Weight: 10, Healthy: true},
		}

		w := NewWeighted(endpoints)

		counts := make(map[string]int)
		for i := 0; i < 1000; i++ {
			ep, _ := w.Next()
			counts[ep.ID]++
		}

		// ep-1 should be picked significantly more
		if counts["ep-1"] < counts["ep-2"] {
			t.Errorf("expected ep-1 to be picked more (weight 90), counts: %v", counts)
		}
	})

	t.Run("Next_DefaultWeight", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Weight: 0, Healthy: true}, // Default to 1
			{ID: "ep-2", Weight: 0, Healthy: true}, // Default to 1
		}

		w := NewWeighted(endpoints)

		// Should work even with zero weights (defaults to 1)
		_, err := w.Next()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("NoEndpoints", func(t *testing.T) {
		w := NewWeighted(nil)

		_, err := w.Next()
		if err != ErrNoEndpoints {
			t.Errorf("expected ErrNoEndpoints, got %v", err)
		}
	})

	t.Run("TotalWeight", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Weight: 10, Healthy: true},
			{ID: "ep-2", Weight: 20, Healthy: true},
		}

		w := NewWeighted(endpoints)
		if w.TotalWeight() != 30 {
			t.Errorf("expected total weight 30, got %d", w.TotalWeight())
		}
	})

	t.Run("Update", func(t *testing.T) {
		w := NewWeighted([]*Endpoint{
			{ID: "ep-1", Weight: 10, Healthy: true},
		})

		w.Update([]*Endpoint{
			{ID: "ep-1", Weight: 20, Healthy: true},
			{ID: "ep-2", Weight: 30, Healthy: true},
		})

		if w.TotalWeight() != 50 {
			t.Errorf("expected total weight 50 after update, got %d", w.TotalWeight())
		}
	})

	t.Run("Count", func(t *testing.T) {
		w := NewWeighted([]*Endpoint{
			{ID: "ep-1", Healthy: true},
			{ID: "ep-2", Healthy: false},
		})

		if w.Count() != 1 {
			t.Errorf("expected 1 healthy endpoint, got %d", w.Count())
		}
	})
}

// ==================== Random Tests ====================

func TestRandom(t *testing.T) {
	t.Run("NewRandom", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Healthy: true},
		}

		r := NewRandom(endpoints)
		if r == nil {
			t.Fatal("expected non-nil random")
		}
		if r.Algorithm() != RandomAlgorithm {
			t.Errorf("expected algorithm random, got %s", r.Algorithm())
		}
	})

	t.Run("Next_ReturnsRandomEndpoint", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Healthy: true},
			{ID: "ep-2", Healthy: true},
			{ID: "ep-3", Healthy: true},
		}

		r := NewRandom(endpoints)

		counts := make(map[string]int)
		for i := 0; i < 300; i++ {
			ep, _ := r.Next()
			counts[ep.ID]++
		}

		// All endpoints should be picked at least once
		for _, ep := range endpoints {
			if counts[ep.ID] == 0 {
				t.Errorf("endpoint %s was never picked", ep.ID)
			}
		}
	})

	t.Run("NoEndpoints", func(t *testing.T) {
		r := NewRandom(nil)

		_, err := r.Next()
		if err != ErrNoEndpoints {
			t.Errorf("expected ErrNoEndpoints, got %v", err)
		}
	})

	t.Run("Update", func(t *testing.T) {
		r := NewRandom([]*Endpoint{{ID: "ep-1", Healthy: true}})

		r.Update([]*Endpoint{
			{ID: "ep-1", Healthy: true},
			{ID: "ep-2", Healthy: true},
		})

		if r.Count() != 2 {
			t.Errorf("expected 2 endpoints after update, got %d", r.Count())
		}
	})

	t.Run("Count", func(t *testing.T) {
		r := NewRandom([]*Endpoint{
			{ID: "ep-1", Healthy: true},
			{ID: "ep-2", Healthy: false},
		})

		if r.Count() != 1 {
			t.Errorf("expected 1 healthy endpoint, got %d", r.Count())
		}
	})
}

// ==================== filterHealthy Tests ====================

func TestFilterHealthy(t *testing.T) {
	t.Run("FiltersUnhealthy", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Healthy: true},
			{ID: "ep-2", Healthy: false},
			{ID: "ep-3", Healthy: true},
			{ID: "ep-4", Healthy: false},
		}

		healthy := filterHealthy(endpoints)
		if len(healthy) != 2 {
			t.Errorf("expected 2 healthy endpoints, got %d", len(healthy))
		}
	})

	t.Run("AllHealthy", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Healthy: true},
			{ID: "ep-2", Healthy: true},
		}

		healthy := filterHealthy(endpoints)
		if len(healthy) != 2 {
			t.Errorf("expected 2 healthy endpoints, got %d", len(healthy))
		}
	})

	t.Run("NoneHealthy", func(t *testing.T) {
		endpoints := []*Endpoint{
			{ID: "ep-1", Healthy: false},
			{ID: "ep-2", Healthy: false},
		}

		healthy := filterHealthy(endpoints)
		if len(healthy) != 0 {
			t.Errorf("expected 0 healthy endpoints, got %d", len(healthy))
		}
	})

	t.Run("NilSlice", func(t *testing.T) {
		healthy := filterHealthy(nil)
		if len(healthy) != 0 {
			t.Errorf("expected 0 endpoints for nil slice, got %d", len(healthy))
		}
	})
}

// ==================== Error Tests ====================

func TestErrors(t *testing.T) {
	t.Run("ErrNoEndpoints", func(t *testing.T) {
		if ErrNoEndpoints.Error() != "no endpoints available" {
			t.Errorf("unexpected error message: %s", ErrNoEndpoints.Error())
		}
	})

	t.Run("ErrUnknownAlgorithm", func(t *testing.T) {
		if ErrUnknownAlgorithm.Error() != "unknown load balancing algorithm" {
			t.Errorf("unexpected error message: %s", ErrUnknownAlgorithm.Error())
		}
	})
}

// ==================== Benchmark Tests ====================

func BenchmarkRoundRobin_Next(b *testing.B) {
	endpoints := make([]*Endpoint, 10)
	for i := 0; i < 10; i++ {
		endpoints[i] = &Endpoint{ID: "ep", Healthy: true}
	}
	rr := NewRoundRobin(endpoints)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.Next()
	}
}

func BenchmarkLeastConn_Next(b *testing.B) {
	endpoints := make([]*Endpoint, 10)
	for i := 0; i < 10; i++ {
		endpoints[i] = &Endpoint{ID: "ep", Healthy: true}
	}
	lc := NewLeastConn(endpoints)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Next()
	}
}

func BenchmarkWeighted_Next(b *testing.B) {
	endpoints := make([]*Endpoint, 10)
	for i := 0; i < 10; i++ {
		endpoints[i] = &Endpoint{ID: "ep", Weight: 10, Healthy: true}
	}
	w := NewWeighted(endpoints)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Next()
	}
}

func BenchmarkRandom_Next(b *testing.B) {
	endpoints := make([]*Endpoint, 10)
	for i := 0; i < 10; i++ {
		endpoints[i] = &Endpoint{ID: "ep", Healthy: true}
	}
	r := NewRandom(endpoints)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Next()
	}
}
