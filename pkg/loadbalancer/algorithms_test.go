// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRoundRobinState tests the round-robin algorithm.
func TestRoundRobinState(t *testing.T) {
	tests := []struct {
		name          string
		backendCount  int
		selectCount   int
		wantDistribution bool // true if we expect even distribution
	}{
		{
			name:             "single backend",
			backendCount:     1,
			selectCount:      10,
			wantDistribution: true,
		},
		{
			name:             "two backends",
			backendCount:     2,
			selectCount:      100,
			wantDistribution: true,
		},
		{
			name:             "five backends",
			backendCount:     5,
			selectCount:      500,
			wantDistribution: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backends := createTestBackends(tt.backendCount)
			rr := NewRoundRobinState()

			counts := make(map[string]int)
			ctx := context.Background()

			for i := 0; i < tt.selectCount; i++ {
				selected, err := rr.Select(ctx, "key", backends)
				if err != nil {
					t.Fatalf("Select failed: %v", err)
				}
				counts[selected.Config.Name]++
			}

			if tt.wantDistribution {
				expectedPerBackend := tt.selectCount / tt.backendCount
				for name, count := range counts {
					// Allow 5% variance
					minExpected := int(float64(expectedPerBackend) * 0.95)
					maxExpected := int(float64(expectedPerBackend) * 1.05)
					if count < minExpected || count > maxExpected {
						t.Errorf("Backend %s got %d requests, expected ~%d", name, count, expectedPerBackend)
					}
				}
			}
		})
	}
}

// TestRoundRobinStateEmptyBackends tests round-robin with no backends.
func TestRoundRobinStateEmptyBackends(t *testing.T) {
	rr := NewRoundRobinState()
	ctx := context.Background()

	_, err := rr.Select(ctx, "key", []*Backend{})
	if err != ErrNoHealthyBackends {
		t.Errorf("Expected ErrNoHealthyBackends, got %v", err)
	}
}

// TestRoundRobinConcurrency tests thread safety of round-robin.
func TestRoundRobinConcurrency(t *testing.T) {
	backends := createTestBackends(5)
	rr := NewRoundRobinState()
	ctx := context.Background()

	var wg sync.WaitGroup
	goroutines := 100
	selectsPerGoroutine := 100

	counts := make(map[string]*int64)
	for _, b := range backends {
		var count int64
		counts[b.Config.Name] = &count
	}

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < selectsPerGoroutine; j++ {
				selected, err := rr.Select(ctx, "key", backends)
				if err != nil {
					t.Errorf("Select failed: %v", err)
					return
				}
				atomic.AddInt64(counts[selected.Config.Name], 1)
			}
		}()
	}

	wg.Wait()

	// Verify all selections happened
	var total int64
	for _, count := range counts {
		total += *count
	}
	expected := int64(goroutines * selectsPerGoroutine)
	if total != expected {
		t.Errorf("Total selections %d, expected %d", total, expected)
	}
}

// TestLeastConnectionsState tests least-connections algorithm.
func TestLeastConnectionsState(t *testing.T) {
	tests := []struct {
		name           string
		connections    []int64 // connections per backend
		expectedBackend int     // index of expected backend
	}{
		{
			name:           "first has least",
			connections:    []int64{1, 5, 10},
			expectedBackend: 0,
		},
		{
			name:           "middle has least",
			connections:    []int64{10, 1, 5},
			expectedBackend: 1,
		},
		{
			name:           "last has least",
			connections:    []int64{10, 5, 1},
			expectedBackend: 2,
		},
		{
			name:           "all equal",
			connections:    []int64{5, 5, 5},
			expectedBackend: 0, // First one when equal
		},
		{
			name:           "all zero",
			connections:    []int64{0, 0, 0},
			expectedBackend: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backends := createTestBackends(len(tt.connections))
			for i, conns := range tt.connections {
				for j := int64(0); j < conns; j++ {
					backends[i].IncrementConnections()
				}
			}

			lc := NewLeastConnectionsState()
			ctx := context.Background()

			selected, err := lc.Select(ctx, "key", backends)
			if err != nil {
				t.Fatalf("Select failed: %v", err)
			}

			if selected != backends[tt.expectedBackend] {
				t.Errorf("Expected backend %d, got %s", tt.expectedBackend, selected.Config.Name)
			}
		})
	}
}

// TestLeastConnectionsDynamic tests that least-connections updates with connection changes.
func TestLeastConnectionsDynamic(t *testing.T) {
	backends := createTestBackends(3)
	lc := NewLeastConnectionsState()
	ctx := context.Background()

	// Initially all have 0 connections, first should be selected
	selected, _ := lc.Select(ctx, "key", backends)
	if selected != backends[0] {
		t.Errorf("Expected first backend initially")
	}

	// Add connections to first backend
	backends[0].IncrementConnections()
	backends[0].IncrementConnections()
	backends[0].IncrementConnections()

	// Now second or third should be selected
	selected, _ = lc.Select(ctx, "key", backends)
	if selected == backends[0] {
		t.Errorf("Should not select backend with most connections")
	}
}

// TestIPHashState tests IP-hash algorithm.
func TestIPHashState(t *testing.T) {
	tests := []struct {
		name       string
		ip         string
		consistent bool
	}{
		{
			name:       "simple IP",
			ip:         "192.168.1.1",
			consistent: true,
		},
		{
			name:       "IP with port",
			ip:         "192.168.1.1:12345",
			consistent: true,
		},
		{
			name:       "IPv6",
			ip:         "::1",
			consistent: true,
		},
		{
			name:       "IPv6 with port",
			ip:         "[::1]:8080",
			consistent: true,
		},
	}

	backends := createTestBackends(5)
	iph := NewIPHashState()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Select multiple times with same IP
			var firstSelected *Backend
			for i := 0; i < 10; i++ {
				selected, err := iph.Select(ctx, tt.ip, backends)
				if err != nil {
					t.Fatalf("Select failed: %v", err)
				}
				if i == 0 {
					firstSelected = selected
				} else if tt.consistent && selected != firstSelected {
					t.Errorf("IP hash not consistent: first=%s, got=%s",
						firstSelected.Config.Name, selected.Config.Name)
				}
			}
		})
	}
}

// TestIPHashDistribution tests that IP hash distributes across backends.
func TestIPHashDistribution(t *testing.T) {
	backends := createTestBackends(5)
	iph := NewIPHashState()
	ctx := context.Background()

	counts := make(map[string]int)

	// Generate many different IPs
	for i := 0; i < 1000; i++ {
		ip := "192.168." + string(rune(i/256)) + "." + string(rune(i%256))
		selected, _ := iph.Select(ctx, ip, backends)
		counts[selected.Config.Name]++
	}

	// Each backend should get some traffic
	for name, count := range counts {
		if count == 0 {
			t.Errorf("Backend %s got no traffic", name)
		}
	}
}

// TestWeightedState tests weighted round-robin algorithm.
func TestWeightedState(t *testing.T) {
	tests := []struct {
		name     string
		weights  []int
		selects  int
		tolerance float64 // acceptable variance from expected ratio
	}{
		{
			name:      "equal weights",
			weights:   []int{1, 1, 1},
			selects:   300,
			tolerance: 0.15,
		},
		{
			name:      "different weights",
			weights:   []int{1, 2, 3},
			selects:   600,
			tolerance: 0.15,
		},
		{
			name:      "high ratio",
			weights:   []int{10, 1},
			selects:   1100,
			tolerance: 0.15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backends := createTestBackendsWithWeights(tt.weights)
			ws := NewWeightedState(backends)
			ctx := context.Background()

			counts := make(map[string]int)
			for i := 0; i < tt.selects; i++ {
				selected, err := ws.Select(ctx, "key", backends)
				if err != nil {
					t.Fatalf("Select failed: %v", err)
				}
				counts[selected.Config.Name]++
			}

			// Verify distribution matches weights
			totalWeight := 0
			for _, w := range tt.weights {
				totalWeight += w
			}

			for i, w := range tt.weights {
				name := backends[i].Config.Name
				expectedRatio := float64(w) / float64(totalWeight)
				actualRatio := float64(counts[name]) / float64(tt.selects)
				diff := actualRatio - expectedRatio
				if diff < 0 {
					diff = -diff
				}
				if diff > tt.tolerance {
					t.Errorf("Backend %s: expected ratio %.2f, got %.2f (diff %.2f)",
						name, expectedRatio, actualRatio, diff)
				}
			}
		})
	}
}

// TestWeightedLeastConnState tests weighted least-connections algorithm.
func TestWeightedLeastConnState(t *testing.T) {
	tests := []struct {
		name           string
		weights        []int
		connections    []int64
		expectedIndex  int
	}{
		{
			name:          "weight beats connections",
			weights:       []int{10, 1},
			connections:   []int64{5, 1},
			expectedIndex: 0, // Higher weight compensates for more connections (5/10=0.5 < 1/1=1.0)
		},
		{
			name:          "connections beat weight",
			weights:       []int{1, 1},
			connections:   []int64{10, 0},
			expectedIndex: 1, // Same weight, fewer connections wins
		},
		{
			name:          "balanced",
			weights:       []int{2, 1},
			connections:   []int64{0, 0},
			expectedIndex: 0, // No connections, higher weight wins
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backends := createTestBackendsWithWeights(tt.weights)
			for i, conns := range tt.connections {
				for j := int64(0); j < conns; j++ {
					backends[i].IncrementConnections()
				}
			}

			wlc := NewWeightedLeastConnState(backends)
			ctx := context.Background()

			selected, err := wlc.Select(ctx, "key", backends)
			if err != nil {
				t.Fatalf("Select failed: %v", err)
			}

			if selected != backends[tt.expectedIndex] {
				t.Errorf("Expected backend %d, got %s", tt.expectedIndex, selected.Config.Name)
			}
		})
	}
}

// TestRandomState tests random algorithm.
func TestRandomState(t *testing.T) {
	backends := createTestBackends(5)
	rs := NewRandomState()
	ctx := context.Background()

	counts := make(map[string]int)
	selects := 1000

	for i := 0; i < selects; i++ {
		selected, err := rs.Select(ctx, "key", backends)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		counts[selected.Config.Name]++
	}

	// Each backend should get some traffic (random but distributed)
	for name, count := range counts {
		// With 5 backends and 1000 selects, each should get ~200
		// Allow wide variance since it's random
		if count < 50 || count > 400 {
			t.Errorf("Backend %s got %d selections, outside expected range", name, count)
		}
	}
}

// TestRandomStateConcurrency tests thread safety of random selection.
func TestRandomStateConcurrency(t *testing.T) {
	backends := createTestBackends(5)
	rs := NewRandomState()
	ctx := context.Background()

	var wg sync.WaitGroup
	goroutines := 50
	selectsPerGoroutine := 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < selectsPerGoroutine; j++ {
				_, err := rs.Select(ctx, "key", backends)
				if err != nil {
					t.Errorf("Select failed: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}

// TestConsistentHashState tests consistent hashing algorithm.
func TestConsistentHashState(t *testing.T) {
	backends := createTestBackends(5)
	ch := NewConsistentHashState(backends, 150)
	ctx := context.Background()

	tests := []struct {
		name string
		key  string
	}{
		{"user123", "user123"},
		{"session456", "session456"},
		{"order789", "order789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Same key should always select same backend
			var firstSelected *Backend
			for i := 0; i < 10; i++ {
				selected, err := ch.Select(ctx, tt.key, backends)
				if err != nil {
					t.Fatalf("Select failed: %v", err)
				}
				if i == 0 {
					firstSelected = selected
				} else if selected != firstSelected {
					t.Errorf("Consistent hash not consistent for key %s", tt.key)
				}
			}
		})
	}
}

// TestConsistentHashMinimalRebalance tests that removing a backend minimizes rebalancing.
func TestConsistentHashMinimalRebalance(t *testing.T) {
	backends := createTestBackends(5)
	ch := NewConsistentHashState(backends, 150)
	ctx := context.Background()

	// Map keys to backends
	keyBackends := make(map[string]*Backend)
	keys := make([]string, 100)
	for i := 0; i < 100; i++ {
		key := "key" + string(rune(i))
		keys[i] = key
		selected, _ := ch.Select(ctx, key, backends)
		keyBackends[key] = selected
	}

	// Remove one backend
	newBackends := backends[:4]
	ch.rebuild(newBackends)

	// Check how many keys moved
	moved := 0
	for _, key := range keys {
		selected, _ := ch.Select(ctx, key, newBackends)
		if keyBackends[key] != selected && keyBackends[key] != backends[4] {
			// Key moved to different backend (excluding the removed one)
			moved++
		}
	}

	// With consistent hashing, only ~1/5 of keys should move
	maxMoved := len(keys) / 3 // Allow some slack
	if moved > maxMoved {
		t.Errorf("Too many keys moved: %d (max expected %d)", moved, maxMoved)
	}
}

// TestMaglevHashState tests Maglev consistent hashing.
func TestMaglevHashState(t *testing.T) {
	backends := createTestBackends(5)
	for _, b := range backends {
		b.SetState(BackendStateHealthy)
	}
	mh := NewMaglevHashState(backends, 65537)
	ctx := context.Background()

	// Test consistency
	t.Run("consistency", func(t *testing.T) {
		key := "test-key-123"
		var firstSelected *Backend
		for i := 0; i < 10; i++ {
			selected, err := mh.Select(ctx, key, backends)
			if err != nil {
				t.Fatalf("Select failed: %v", err)
			}
			if i == 0 {
				firstSelected = selected
			} else if selected != firstSelected {
				t.Errorf("Maglev hash not consistent")
			}
		}
	})

	// Test distribution
	t.Run("distribution", func(t *testing.T) {
		counts := make(map[string]int)
		for i := 0; i < 10000; i++ {
			key := "key" + string(rune(i))
			selected, _ := mh.Select(ctx, key, backends)
			counts[selected.Config.Name]++
		}

		// Each backend should get roughly equal share
		for _, count := range counts {
			ratio := float64(count) / 10000.0
			if ratio < 0.1 || ratio > 0.3 {
				t.Errorf("Uneven distribution: %d selections", count)
			}
		}
	})
}

// TestTwoRandomChoicesState tests power of two random choices algorithm.
func TestTwoRandomChoicesState(t *testing.T) {
	backends := createTestBackends(5)
	trc := NewTwoRandomChoicesState()
	ctx := context.Background()

	// Set different connection counts
	backends[0].IncrementConnections()
	backends[0].IncrementConnections()
	backends[0].IncrementConnections()
	backends[1].IncrementConnections()
	backends[1].IncrementConnections()

	// Backends with fewer connections should be selected more often
	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		selected, err := trc.Select(ctx, "key", backends)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		counts[selected.Config.Name]++
	}

	// Backend 0 (3 conns) should be selected less than backend 4 (0 conns)
	if counts[backends[0].Config.Name] > counts[backends[4].Config.Name] {
		t.Errorf("Backend with more connections selected more often")
	}
}

// TestTwoRandomChoicesSingleBackend tests with only one backend.
func TestTwoRandomChoicesSingleBackend(t *testing.T) {
	backends := createTestBackends(1)
	trc := NewTwoRandomChoicesState()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		selected, err := trc.Select(ctx, "key", backends)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if selected != backends[0] {
			t.Errorf("Expected only backend, got different")
		}
	}
}

// TestSourcePortHashState tests source port hash algorithm.
func TestSourcePortHashState(t *testing.T) {
	backends := createTestBackends(5)
	sph := NewSourcePortHashState()
	ctx := context.Background()

	tests := []struct {
		name string
		addr string
	}{
		{"with port", "192.168.1.1:12345"},
		{"without port", "12345"},
		{"different port", "192.168.1.1:54321"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Same address should always select same backend
			var firstSelected *Backend
			for i := 0; i < 5; i++ {
				selected, err := sph.Select(ctx, tt.addr, backends)
				if err != nil {
					t.Fatalf("Select failed: %v", err)
				}
				if i == 0 {
					firstSelected = selected
				} else if selected != firstSelected {
					t.Errorf("Source port hash not consistent")
				}
			}
		})
	}
}

// TestPriorityGroupState tests priority-based load balancing.
func TestPriorityGroupState(t *testing.T) {
	backends := createTestBackendsWithPriorities([]int{0, 1, 1, 2, 2})
	for _, b := range backends {
		b.SetState(BackendStateHealthy)
	}

	pg := NewPriorityGroupState(backends, AlgorithmRoundRobin)
	ctx := context.Background()

	// Should only select from priority 0
	for i := 0; i < 10; i++ {
		selected, err := pg.Select(ctx, "key", backends)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if selected.Config.Priority != 0 {
			t.Errorf("Expected priority 0, got %d", selected.Config.Priority)
		}
	}

	// Mark priority 0 unhealthy
	backends[0].SetState(BackendStateUnhealthy)

	// Should now select from priority 1
	selected, _ := pg.Select(ctx, "key", backends)
	if selected.Config.Priority != 1 {
		t.Errorf("Expected priority 1 after failover, got %d", selected.Config.Priority)
	}
}

// TestSmoothWeightedRRState tests smooth weighted round-robin.
func TestSmoothWeightedRRState(t *testing.T) {
	backends := createTestBackendsWithWeights([]int{5, 3, 2})
	sw := NewSmoothWeightedRRState(backends)
	ctx := context.Background()

	counts := make(map[string]int)
	for i := 0; i < 100; i++ {
		selected, err := sw.Select(ctx, "key", backends)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		counts[selected.Config.Name]++
	}

	// Verify distribution roughly matches weights (5:3:2)
	expected := map[int]int{0: 50, 1: 30, 2: 20}
	for i, b := range backends {
		diff := counts[b.Config.Name] - expected[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > 15 { // 15% tolerance
			t.Errorf("Backend %s: expected ~%d, got %d", b.Config.Name, expected[i], counts[b.Config.Name])
		}
	}
}

// TestLeastResponseTimeState tests least response time algorithm.
func TestLeastResponseTimeState(t *testing.T) {
	backends := createTestBackends(3)

	// Simulate response times
	backends[0].RecordResponseTime(100 * time.Millisecond)
	backends[0].RecordResponseTime(100 * time.Millisecond)
	backends[1].RecordResponseTime(50 * time.Millisecond)
	backends[1].RecordResponseTime(50 * time.Millisecond)
	backends[2].RecordResponseTime(200 * time.Millisecond)
	backends[2].RecordResponseTime(200 * time.Millisecond)

	lrt := NewLeastResponseTimeState()
	ctx := context.Background()

	// Should select backend with fastest response time
	selected, err := lrt.Select(ctx, "key", backends)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if selected != backends[1] {
		t.Errorf("Expected backend with fastest response time")
	}
}

// TestLeastResponseTimeNoData tests behavior with no response time data.
func TestLeastResponseTimeNoData(t *testing.T) {
	backends := createTestBackends(3)
	lrt := NewLeastResponseTimeState()
	ctx := context.Background()

	// With no data, should return first backend
	selected, err := lrt.Select(ctx, "key", backends)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if selected != backends[0] {
		t.Errorf("Expected first backend when no response time data")
	}
}

// TestResourceBasedState tests resource-based load balancing.
func TestResourceBasedState(t *testing.T) {
	backends := createTestBackends(3)
	rbs := NewResourceBasedState()
	ctx := context.Background()

	// Set resource scores (lower is better)
	rbs.UpdateScore(backends[0], 0.8) // High utilization
	rbs.UpdateScore(backends[1], 0.2) // Low utilization
	rbs.UpdateScore(backends[2], 0.5) // Medium utilization

	// Should select backend with lowest score
	selected, err := rbs.Select(ctx, "key", backends)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if selected != backends[1] {
		t.Errorf("Expected backend with lowest resource score")
	}
}

// TestResourceBasedFallback tests fallback to connection count.
func TestResourceBasedFallback(t *testing.T) {
	backends := createTestBackends(3)
	backends[0].IncrementConnections()
	backends[0].IncrementConnections()

	rbs := NewResourceBasedState()
	ctx := context.Background()

	// No scores set, should fall back to connection count
	selected, err := rbs.Select(ctx, "key", backends)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if selected == backends[0] {
		t.Errorf("Should not select backend with most connections")
	}
}

// TestChainedAlgorithm tests chained algorithm fallback.
func TestChainedAlgorithm(t *testing.T) {
	backends := createTestBackends(3)
	for _, b := range backends {
		b.SetState(BackendStateHealthy)
	}

	// Chain: IPHash -> LeastConnections
	chain := NewChainedAlgorithm(
		NewIPHashState(),
		NewLeastConnectionsState(),
	)

	ctx := context.Background()

	// Should work normally
	selected, err := chain.Select(ctx, "192.168.1.1", backends)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if selected == nil {
		t.Error("Expected a backend to be selected")
	}
}

// TestChainedAlgorithmEmpty tests chained algorithm with no backends.
func TestChainedAlgorithmEmpty(t *testing.T) {
	chain := NewChainedAlgorithm(
		NewRoundRobinState(),
		NewLeastConnectionsState(),
	)

	ctx := context.Background()

	_, err := chain.Select(ctx, "key", []*Backend{})
	if err != ErrNoHealthyBackends {
		t.Errorf("Expected ErrNoHealthyBackends, got %v", err)
	}
}

// TestNewAlgorithmState tests the algorithm factory.
func TestNewAlgorithmState(t *testing.T) {
	backends := createTestBackends(3)

	tests := []struct {
		algorithm Algorithm
		wantType  string
	}{
		{AlgorithmRoundRobin, "*loadbalancer.RoundRobinState"},
		{AlgorithmLeastConnections, "*loadbalancer.LeastConnectionsState"},
		{AlgorithmIPHash, "*loadbalancer.IPHashState"},
		{AlgorithmWeighted, "*loadbalancer.WeightedState"},
		{AlgorithmWeightedLeastConn, "*loadbalancer.WeightedLeastConnState"},
		{AlgorithmRandom, "*loadbalancer.RandomState"},
		{Algorithm("unknown"), "*loadbalancer.RoundRobinState"}, // Default to round-robin
	}

	for _, tt := range tests {
		t.Run(string(tt.algorithm), func(t *testing.T) {
			state := NewAlgorithmState(tt.algorithm, backends)
			if state == nil {
				t.Error("Expected non-nil algorithm state")
			}
		})
	}
}

// Benchmark tests

// BenchmarkRoundRobin benchmarks round-robin selection.
func BenchmarkRoundRobin(b *testing.B) {
	backends := createTestBackends(10)
	rr := NewRoundRobinState()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.Select(ctx, "key", backends)
	}
}

// BenchmarkLeastConnections benchmarks least-connections selection.
func BenchmarkLeastConnections(b *testing.B) {
	backends := createTestBackends(10)
	lc := NewLeastConnectionsState()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Select(ctx, "key", backends)
	}
}

// BenchmarkIPHash benchmarks IP hash selection.
func BenchmarkIPHash(b *testing.B) {
	backends := createTestBackends(10)
	iph := NewIPHashState()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iph.Select(ctx, "192.168.1.1", backends)
	}
}

// BenchmarkWeighted benchmarks weighted round-robin selection.
func BenchmarkWeighted(b *testing.B) {
	backends := createTestBackendsWithWeights([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	ws := NewWeightedState(backends)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ws.Select(ctx, "key", backends)
	}
}

// BenchmarkMaglev benchmarks Maglev hash selection.
func BenchmarkMaglev(b *testing.B) {
	backends := createTestBackends(10)
	for _, be := range backends {
		be.SetState(BackendStateHealthy)
	}
	mh := NewMaglevHashState(backends, 65537)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mh.Select(ctx, "test-key", backends)
	}
}

// BenchmarkConsistentHash benchmarks consistent hash selection.
func BenchmarkConsistentHash(b *testing.B) {
	backends := createTestBackends(10)
	for _, be := range backends {
		be.SetState(BackendStateHealthy)
	}
	ch := NewConsistentHashState(backends, 150)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.Select(ctx, "test-key", backends)
	}
}

// BenchmarkRoundRobinParallel benchmarks concurrent round-robin.
func BenchmarkRoundRobinParallel(b *testing.B) {
	backends := createTestBackends(10)
	rr := NewRoundRobinState()
	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rr.Select(ctx, "key", backends)
		}
	})
}

// Helper functions

func createTestBackends(count int) []*Backend {
	backends := make([]*Backend, count)
	for i := 0; i < count; i++ {
		config := BackendConfig{
			Name:    "backend" + string(rune('0'+i)),
			Address: "127.0.0.1:" + string(rune('0'+i)) + "080",
			Weight:  1,
			Enabled: true,
		}
		backends[i], _ = NewBackend(config)
		backends[i].SetState(BackendStateHealthy)
	}
	return backends
}

func createTestBackendsWithWeights(weights []int) []*Backend {
	backends := make([]*Backend, len(weights))
	for i, w := range weights {
		config := BackendConfig{
			Name:    "backend" + string(rune('0'+i)),
			Address: "127.0.0.1:" + string(rune('0'+i)) + "080",
			Weight:  w,
			Enabled: true,
		}
		backends[i], _ = NewBackend(config)
		backends[i].SetState(BackendStateHealthy)
	}
	return backends
}

func createTestBackendsWithPriorities(priorities []int) []*Backend {
	backends := make([]*Backend, len(priorities))
	for i, p := range priorities {
		config := BackendConfig{
			Name:     "backend" + string(rune('0'+i)),
			Address:  "127.0.0.1:" + string(rune('0'+i)) + "080",
			Weight:   1,
			Priority: p,
			Enabled:  true,
		}
		backends[i], _ = NewBackend(config)
		backends[i].SetState(BackendStateHealthy)
	}
	return backends
}
