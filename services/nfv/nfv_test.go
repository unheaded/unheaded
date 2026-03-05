// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package nfv

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestLogger returns a logger that discards output, suitable for tests.
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestService creates a Service with sensible test defaults.
// The wotan client is nil because no network operations are performed.
func newTestService() *Service {
	return NewService(newTestLogger(), nil)
}

// makeChain creates a ChainDefinition with reasonable defaults.
func makeChain(id int, name string, funcCount int) *ChainDefinition {
	functions := make([]FunctionRef, funcCount)
	for i := 0; i < funcCount; i++ {
		functions[i] = FunctionRef{
			Name:     fmt.Sprintf("func_%d", i),
			Priority: i,
			ProgFD:   100 + i,
			Required: true,
		}
	}
	return &ChainDefinition{
		ChainID:          id,
		Name:             name,
		Description:      "test chain",
		Functions:        functions,
		TelemetryEnabled: true,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestChainCreation(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	t.Run("create valid chain", func(t *testing.T) {
		chain := makeChain(0, "test-chain", 3)
		if err := svc.CreateChain(ctx, chain); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		got, ok := svc.GetChain(0)
		if !ok {
			t.Fatal("expected chain to exist")
		}
		if got.Name != "test-chain" {
			t.Errorf("expected name 'test-chain', got %q", got.Name)
		}
		if len(got.Functions) != 3 {
			t.Errorf("expected 3 functions, got %d", len(got.Functions))
		}
		if got.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be set")
		}
	})

	t.Run("create chain with invalid id", func(t *testing.T) {
		chain := makeChain(64, "invalid-chain", 1)
		if err := svc.CreateChain(ctx, chain); err == nil {
			t.Fatal("expected error for invalid chain_id")
		}
	})

	t.Run("create chain with negative id", func(t *testing.T) {
		chain := makeChain(-1, "negative-chain", 1)
		if err := svc.CreateChain(ctx, chain); err == nil {
			t.Fatal("expected error for negative chain_id")
		}
	})

	t.Run("create chain with empty name", func(t *testing.T) {
		chain := makeChain(1, "", 1)
		if err := svc.CreateChain(ctx, chain); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("list chains", func(t *testing.T) {
		chains := svc.ListChains()
		if len(chains) < 1 {
			t.Error("expected at least 1 chain")
		}
	})
}

func TestChainDeletion(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	chain := makeChain(5, "to-delete", 2)
	if err := svc.CreateChain(ctx, chain); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Run("delete existing chain", func(t *testing.T) {
		if err := svc.DeleteChain(ctx, 5); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if _, ok := svc.GetChain(5); ok {
			t.Error("expected chain to be deleted")
		}
	})

	t.Run("delete non-existent chain", func(t *testing.T) {
		if err := svc.DeleteChain(ctx, 99); err == nil {
			t.Fatal("expected error for non-existent chain")
		}
	})
}

func TestFunctionRegistry(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("register and list functions", func(t *testing.T) {
		svc.RegisterFunction("xdp_filter", 42)
		svc.RegisterFunction("tc_classify", 43)

		functions := svc.ListFunctions()
		if len(functions) != 2 {
			t.Fatalf("expected 2 functions, got %d", len(functions))
		}

		found := false
		for _, fn := range functions {
			if fn.Name == "xdp_filter" && fn.ProgFD == 42 {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find xdp_filter function")
		}
	})

	t.Run("overwrite function", func(t *testing.T) {
		svc.RegisterFunction("xdp_filter", 99)
		functions := svc.ListFunctions()
		for _, fn := range functions {
			if fn.Name == "xdp_filter" {
				if fn.ProgFD != 99 {
					t.Errorf("expected prog_fd 99, got %d", fn.ProgFD)
				}
				break
			}
		}
	})
}

func TestMaxChainLimit(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	// Fill up all 64 chain slots
	for i := 0; i < MaxActiveChains; i++ {
		chain := makeChain(i, fmt.Sprintf("chain-%d", i), 1)
		if err := svc.CreateChain(ctx, chain); err != nil {
			t.Fatalf("failed to create chain %d: %v", i, err)
		}
	}

	chains := svc.ListChains()
	if len(chains) != MaxActiveChains {
		t.Fatalf("expected %d chains, got %d", MaxActiveChains, len(chains))
	}

	// Verify stats reflect all chains
	stats := svc.Stats()
	if stats["active_chains"].(int) != MaxActiveChains {
		t.Errorf("expected %d active chains in stats", MaxActiveChains)
	}
}

func TestMaxFunctionLimit(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	t.Run("chain at max function limit", func(t *testing.T) {
		chain := makeChain(0, "max-funcs", MaxFunctionsPerChain)
		if err := svc.CreateChain(ctx, chain); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("chain exceeding function limit", func(t *testing.T) {
		chain := makeChain(1, "too-many-funcs", MaxFunctionsPerChain+1)
		if err := svc.CreateChain(ctx, chain); err == nil {
			t.Fatal("expected error for exceeding function limit")
		}
	})
}

func TestStatsAggregation(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	chain := makeChain(0, "stats-chain", 2)
	if err := svc.CreateChain(ctx, chain); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Record stats
	stats := &ChainStats{
		ChainID:          0,
		PacketsProcessed: 1000,
		Drops:            5,
		TotalLatencyUS:   50000,
		FunctionStats: []FunctionStat{
			{Name: "func_0", Invocations: 1000, AvgLatencyUS: 25.0, DropCount: 3},
			{Name: "func_1", Invocations: 997, AvgLatencyUS: 25.0, DropCount: 2},
		},
	}
	svc.RecordStats(stats)

	t.Run("get chain stats", func(t *testing.T) {
		got, ok := svc.GetChainStats(0)
		if !ok {
			t.Fatal("expected stats to exist")
		}
		if got.PacketsProcessed != 1000 {
			t.Errorf("expected 1000 packets, got %d", got.PacketsProcessed)
		}
		if got.Drops != 5 {
			t.Errorf("expected 5 drops, got %d", got.Drops)
		}
		if got.LastUpdated.IsZero() {
			t.Error("expected LastUpdated to be set")
		}
	})

	t.Run("stats for non-existent chain", func(t *testing.T) {
		_, ok := svc.GetChainStats(99)
		if ok {
			t.Error("expected no stats for non-existent chain")
		}
	})

	t.Run("service-level stats", func(t *testing.T) {
		svcStats := svc.Stats()
		if svcStats["total_packets"].(uint64) != 1000 {
			t.Errorf("expected 1000 total packets in service stats")
		}
		if svcStats["total_drops"].(uint64) != 5 {
			t.Errorf("expected 5 total drops in service stats")
		}
	})
}

func TestWotanDrops(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	// With nil Wotan, publish should increment drop counter
	svc.publishEvent(ctx, "test.topic", map[string]interface{}{"test": true})

	if svc.WotanDrops() != 1 {
		t.Errorf("expected 1 wotan drop, got %d", svc.WotanDrops())
	}
}

func TestAuthLevelString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level AuthLevel
		want  string
	}{
		{AuthNormal, "NORMAL"},
		{AuthPriority, "PRIORITY"},
		{AuthReserved, "RESERVED"},
		{AuthExperimental, "EXPERIMENTAL"},
		{AuthLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("AuthLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestNewService(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	if svc.log == nil {
		t.Error("expected logger to be set")
	}
	if svc.chains == nil {
		t.Error("expected chains map to be initialized")
	}
	if svc.functionRegistry == nil {
		t.Error("expected functionRegistry map to be initialized")
	}
	if svc.chainStats == nil {
		t.Error("expected chainStats map to be initialized")
	}
}

func TestChainTimestamps(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	before := time.Now()
	chain := makeChain(0, "ts-chain", 1)
	if err := svc.CreateChain(ctx, chain); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now()

	got, _ := svc.GetChain(0)
	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Error("CreatedAt not within expected range")
	}
	if got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) {
		t.Error("UpdatedAt not within expected range")
	}
}
