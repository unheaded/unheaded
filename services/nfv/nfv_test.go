// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package nfv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
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

// ---------------------------------------------------------------------------
// HTTP handler tests
// ---------------------------------------------------------------------------

func TestHandleHealth(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", body["status"])
	}
	if body["service"] != "nfv" {
		t.Errorf("expected service 'nfv', got %v", body["service"])
	}
}

func TestHandleReady(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["ready"] != true {
		t.Errorf("expected ready true, got %v", body["ready"])
	}
}

func TestHandleMetrics(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	// Populate some data
	svc.CreateChain(ctx, makeChain(0, "m-chain", 2))
	svc.RegisterFunction("xdp_test", 42)
	svc.RecordStats(&ChainStats{ChainID: 0, PacketsProcessed: 500, Drops: 3})

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "nfv_chains_total 1") {
		t.Error("expected nfv_chains_total 1 in metrics")
	}
	if !strings.Contains(body, "nfv_functions_total 1") {
		t.Error("expected nfv_functions_total 1 in metrics")
	}
	if !strings.Contains(body, "nfv_packets_processed_total 500") {
		t.Error("expected nfv_packets_processed_total 500 in metrics")
	}
	if !strings.Contains(body, "nfv_drops_total 3") {
		t.Error("expected nfv_drops_total 3 in metrics")
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/plain; version=0.0.4" {
		t.Errorf("expected prometheus content-type, got %q", ct)
	}
}

func TestHandleChainsHTTP(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("GET /api/v1/chains empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chains", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["count"].(float64) != 0 {
			t.Errorf("expected 0 chains, got %v", body["count"])
		}
	})

	t.Run("POST /api/v1/chains create", func(t *testing.T) {
		chainJSON := `{"chain_id":1,"name":"http-chain","functions":[{"name":"f1","priority":0,"prog_fd":10,"required":true}]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chains", strings.NewReader(chainJSON))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /api/v1/chains invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chains", strings.NewReader("not json"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("POST /api/v1/chains invalid chain_id", func(t *testing.T) {
		chainJSON := `{"chain_id":100,"name":"bad-chain","functions":[]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chains", strings.NewReader(chainJSON))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("POST /api/v1/chains empty name", func(t *testing.T) {
		chainJSON := `{"chain_id":2,"name":"","functions":[]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chains", strings.NewReader(chainJSON))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("POST /api/v1/chains too many functions", func(t *testing.T) {
		funcs := make([]FunctionRef, MaxFunctionsPerChain+1)
		for i := range funcs {
			funcs[i] = FunctionRef{Name: fmt.Sprintf("f%d", i)}
		}
		chain := ChainDefinition{ChainID: 3, Name: "too-many", Functions: funcs}
		body, _ := json.Marshal(chain)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chains", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("PUT /api/v1/chains method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/chains", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("GET /api/v1/chains after create", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chains", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["count"].(float64) < 1 {
			t.Errorf("expected at least 1 chain after create")
		}
	})
}

func TestHandleChainByIDHTTP(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()
	svc.CreateChain(ctx, makeChain(5, "by-id-chain", 2))
	svc.RecordStats(&ChainStats{ChainID: 5, PacketsProcessed: 100, Drops: 1})

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("GET existing chain", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chains/5", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var chain ChainDefinition
		json.NewDecoder(rec.Body).Decode(&chain)
		if chain.Name != "by-id-chain" {
			t.Errorf("expected 'by-id-chain', got %q", chain.Name)
		}
	})

	t.Run("GET non-existent chain", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chains/99", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("GET invalid chain_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chains/abc", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("GET chain telemetry", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chains/5/telemetry", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var stats ChainStats
		json.NewDecoder(rec.Body).Decode(&stats)
		if stats.PacketsProcessed != 100 {
			t.Errorf("expected 100 packets, got %d", stats.PacketsProcessed)
		}
	})

	t.Run("GET telemetry for chain without stats", func(t *testing.T) {
		svc.CreateChain(ctx, makeChain(10, "no-stats-chain", 1))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chains/10/telemetry", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var stats ChainStats
		json.NewDecoder(rec.Body).Decode(&stats)
		if stats.PacketsProcessed != 0 {
			t.Errorf("expected 0 packets for missing stats, got %d", stats.PacketsProcessed)
		}
	})

	t.Run("GET telemetry for non-existent chain", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chains/99/telemetry", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("DELETE chain", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/chains/5", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("DELETE non-existent chain", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/chains/99", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("PATCH chain method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/chains/5", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleFunctionsHTTP(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	svc.RegisterFunction("xdp_filter", 42)

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("GET /api/v1/functions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/functions", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["count"].(float64) != 1 {
			t.Errorf("expected 1 function, got %v", body["count"])
		}
	})

	t.Run("POST /api/v1/functions method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/functions", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleFunctionReloadHTTP(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	// Create chain with a function to reload
	chain := makeChain(0, "reload-chain", 2)
	svc.CreateChain(ctx, chain)

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("POST reload new function", func(t *testing.T) {
		body := `{"name":"new_func","prog_fd":55}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/functions/reload", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp["status"] != "reloaded" {
			t.Errorf("expected status 'reloaded', got %v", resp["status"])
		}
	})

	t.Run("POST reload existing chain function", func(t *testing.T) {
		body := `{"name":"func_0","prog_fd":999}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/functions/reload", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		// Verify chain function was updated
		got, _ := svc.GetChain(0)
		for _, fn := range got.Functions {
			if fn.Name == "func_0" && fn.ProgFD != 999 {
				t.Errorf("expected prog_fd 999 after reload, got %d", fn.ProgFD)
			}
		}
	})

	t.Run("POST reload invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/functions/reload", strings.NewReader("bad"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("POST reload empty name", func(t *testing.T) {
		body := `{"name":"","prog_fd":10}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/functions/reload", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("GET reload method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/functions/reload", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestAggregateStats(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	svc.CreateChain(ctx, makeChain(0, "agg-chain", 1))
	svc.RecordStats(&ChainStats{ChainID: 0, PacketsProcessed: 500})

	// Should not panic or error
	svc.aggregateStats()
}

func TestHandleCriticalAlert(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	// nil message should not panic
	svc.handleCriticalAlert(nil)

	// valid message
	svc.handleCriticalAlert(&wotanClient.Message{
		MessageID: "alert-1",
		Topic:     "alerts.critical",
	})
}

func TestPublishEventMultipleDrops(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		svc.publishEvent(ctx, "test.topic", map[string]interface{}{"i": i})
	}
	if svc.WotanDrops() != 5 {
		t.Errorf("expected 5 wotan drops, got %d", svc.WotanDrops())
	}
}

func TestStatsAggregationLoopCancellation(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.statsAggregationLoop(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("statsAggregationLoop did not exit after context cancel")
	}
}

func TestListenForAlertsNilWotan(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	// Should return immediately when wotan is nil
	done := make(chan struct{})
	go func() {
		svc.listenForAlerts(ctx)
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("listenForAlerts did not return for nil wotan")
	}
}
