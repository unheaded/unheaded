// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package loadbalancer

import (
	"io"
	"math"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

func TestMaglevTableBuild(t *testing.T) {
	t.Run("empty backends", func(t *testing.T) {
		table := BuildTable(nil, 1024)
		if len(table) != 0 {
			t.Errorf("expected empty table for nil backends, got len %d", len(table))
		}
	})

	t.Run("single backend fills table", func(t *testing.T) {
		backends := []Backend{
			{ID: "b1", Address: "10.0.0.1", Port: 8080, Weight: 1, Health: HealthHealthy},
		}
		table := BuildTable(backends, 1021) // 1021 is prime
		if len(table) != 1021 {
			t.Fatalf("expected table size 1021, got %d", len(table))
		}
		// All entries should point to backend 0.
		for i, idx := range table {
			if idx != 0 {
				t.Errorf("slot %d: expected backend index 0, got %d", i, idx)
				break
			}
		}
	})

	t.Run("all slots assigned", func(t *testing.T) {
		backends := []Backend{
			{ID: "b1", Address: "10.0.0.1", Port: 8080, Weight: 1, Health: HealthHealthy},
			{ID: "b2", Address: "10.0.0.2", Port: 8080, Weight: 1, Health: HealthHealthy},
			{ID: "b3", Address: "10.0.0.3", Port: 8080, Weight: 1, Health: HealthHealthy},
		}
		tableSize := 997 // prime
		table := BuildTable(backends, tableSize)
		if len(table) != tableSize {
			t.Fatalf("expected table size %d, got %d", tableSize, len(table))
		}
		for i, idx := range table {
			if idx == math.MaxUint32 {
				t.Errorf("slot %d is unassigned", i)
				break
			}
		}
	})

	t.Run("distribution is roughly even for equal weights", func(t *testing.T) {
		backends := []Backend{
			{ID: "backend-a", Address: "10.0.0.1", Port: 8080, Weight: 1, Health: HealthHealthy},
			{ID: "backend-b", Address: "10.0.0.2", Port: 8080, Weight: 1, Health: HealthHealthy},
		}
		tableSize := 4999 // prime
		table := BuildTable(backends, tableSize)

		counts := make(map[uint32]int)
		for _, idx := range table {
			counts[idx]++
		}

		for idx, count := range counts {
			pct := float64(count) / float64(tableSize) * 100
			// With equal weights and 2 backends, each should get roughly 50%.
			if pct < 30 || pct > 70 {
				t.Errorf("backend %d got %.1f%% of slots (expected ~50%%)", idx, pct)
			}
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		backends := []Backend{
			{ID: "b1", Address: "10.0.0.1", Port: 8080, Weight: 1, Health: HealthHealthy},
			{ID: "b2", Address: "10.0.0.2", Port: 8080, Weight: 2, Health: HealthHealthy},
		}
		table1 := BuildTable(backends, 997)
		table2 := BuildTable(backends, 997)

		if len(table1) != len(table2) {
			t.Fatal("tables have different sizes")
		}
		for i := range table1 {
			if table1[i] != table2[i] {
				t.Errorf("tables differ at slot %d: %d vs %d", i, table1[i], table2[i])
				break
			}
		}
	})

	t.Run("minimal disruption on backend removal", func(t *testing.T) {
		backends3 := []Backend{
			{ID: "a", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy},
			{ID: "b", Address: "10.0.0.2", Port: 80, Weight: 1, Health: HealthHealthy},
			{ID: "c", Address: "10.0.0.3", Port: 80, Weight: 1, Health: HealthHealthy},
		}
		backends2 := []Backend{
			{ID: "a", Address: "10.0.0.1", Port: 80, Weight: 1, Health: HealthHealthy},
			{ID: "b", Address: "10.0.0.2", Port: 80, Weight: 1, Health: HealthHealthy},
		}
		tableSize := 997
		table3 := BuildTable(backends3, tableSize)
		table2 := BuildTable(backends2, tableSize)

		// Count how many slots changed.
		changed := 0
		for i := range table3 {
			if table3[i] != table2[i] {
				changed++
			}
		}
		// Maglev guarantees that only slots belonging to the removed backend
		// should change. With 3 backends, roughly 1/3 of slots should change.
		changePct := float64(changed) / float64(tableSize) * 100
		// Allow generous tolerance; the key property is that the change is
		// bounded, not 100%.
		if changePct > 80 {
			t.Errorf("too many slots changed: %.1f%% (expected < 80%% for minimal disruption)", changePct)
		}
	})
}

func TestBackendAddRemove(t *testing.T) {
	log := newTestLogger()
	svc := NewXDPSync(log, nil)

	// Add backends.
	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 8080, Weight: 1, Health: HealthHealthy})
	svc.AddBackend(&Backend{ID: "b2", Address: "10.0.0.2", Port: 8080, Weight: 1, Health: HealthHealthy})

	// Table should be non-empty.
	table := svc.GetTable()
	if len(table) == 0 {
		t.Fatal("expected non-empty table after adding backends")
	}

	// Both backends should be in the table.
	b1, ok := svc.GetBackend("b1")
	if !ok || b1 == nil {
		t.Error("expected to find backend b1")
	}
	b2, ok := svc.GetBackend("b2")
	if !ok || b2 == nil {
		t.Error("expected to find backend b2")
	}

	// Remove b2.
	removed := svc.RemoveBackend("b2")
	if !removed {
		t.Error("expected RemoveBackend to return true for existing backend")
	}

	// b2 should be gone.
	_, ok = svc.GetBackend("b2")
	if ok {
		t.Error("expected b2 to be removed")
	}

	// Table should still be non-empty (b1 remains).
	table = svc.GetTable()
	if len(table) == 0 {
		t.Fatal("expected non-empty table after removing one of two backends")
	}

	// All slots should point to backend 0 (b1).
	for i, idx := range table {
		if idx != 0 {
			t.Errorf("slot %d: expected 0 (only backend), got %d", i, idx)
			break
		}
	}

	// Remove non-existent.
	removed = svc.RemoveBackend("nonexistent")
	if removed {
		t.Error("expected RemoveBackend to return false for nonexistent backend")
	}
}

func TestHealthStateTransitions(t *testing.T) {
	log := newTestLogger()
	svc := NewXDPSync(log, nil)

	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 8080, Weight: 1, Health: HealthHealthy})
	svc.AddBackend(&Backend{ID: "b2", Address: "10.0.0.2", Port: 8080, Weight: 1, Health: HealthHealthy})

	// Both backends healthy: table should have entries for both.
	table := svc.GetTable()
	counts := countBackends(table)
	if _, ok := counts[0]; !ok {
		t.Error("expected backend 0 in table")
	}
	if _, ok := counts[1]; !ok {
		t.Error("expected backend 1 in table")
	}

	// Mark b2 as unhealthy.
	ok := svc.SetBackendHealth("b2", HealthUnhealthy)
	if !ok {
		t.Error("expected SetBackendHealth to return true")
	}

	// Only b1 (healthy) should be in the table now.
	table = svc.GetTable()
	if len(table) == 0 {
		t.Fatal("expected non-empty table")
	}
	for i, idx := range table {
		if idx != 0 {
			t.Errorf("slot %d: expected only backend 0 (b1) after b2 unhealthy, got %d", i, idx)
			break
		}
	}

	// Transition b2 to DRAINING.
	svc.SetBackendHealth("b2", HealthDraining)
	b2, _ := svc.GetBackend("b2")
	if b2.Health != HealthDraining {
		t.Errorf("expected DRAINING, got %s", b2.Health)
	}

	// Restore b2 to HEALTHY.
	svc.SetBackendHealth("b2", HealthHealthy)
	table = svc.GetTable()
	counts = countBackends(table)
	if len(counts) != 2 {
		t.Errorf("expected 2 unique backends in table, got %d", len(counts))
	}

	// Verify LastCheck was updated.
	b2, _ = svc.GetBackend("b2")
	if b2.LastCheck.IsZero() {
		t.Error("expected LastCheck to be set")
	}
	if time.Since(b2.LastCheck) > 5*time.Second {
		t.Error("LastCheck seems too old")
	}

	// Non-existent backend.
	ok = svc.SetBackendHealth("nonexistent", HealthHealthy)
	if ok {
		t.Error("expected false for nonexistent backend")
	}
}

// countBackends counts how many slots each backend index occupies.
func countBackends(table []uint32) map[uint32]int {
	counts := make(map[uint32]int)
	for _, idx := range table {
		counts[idx]++
	}
	return counts
}
