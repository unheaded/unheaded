// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package compliance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestLogger returns a logger that discards output (safe for concurrent use).
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestService returns a ready-to-use *WireComplianceService with defaults.
func newTestService(t *testing.T) *WireComplianceService {
	t.Helper()
	return NewService(newTestLogger(), nil, nil)
}

// addTestPolicy inserts a policy directly into the service.
func addTestPolicy(t *testing.T, svc *WireComplianceService, src, dst string, action PolicyAction) *ServicePolicy {
	t.Helper()
	now := time.Now()
	policy := &ServicePolicy{
		SrcServiceID:       src,
		DstServiceID:       dst,
		Action:             action,
		AuditLevel:         "full",
		MinQoS:             1,
		MaxLatencyMS:       100.0,
		RequiresEncryption: true,
		TraceSampling:      1.0,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	svc.mu.Lock()
	svc.policies[policyKey(src, dst)] = policy
	svc.mu.Unlock()
	return policy
}

// addTestViolation inserts a violation directly.
func addTestViolation(t *testing.T, svc *WireComplianceService, src, dst string, vType ViolationType, traceID string) {
	t.Helper()
	svc.mu.Lock()
	v := Violation{
		Timestamp:      time.Now(),
		SrcServiceID:   src,
		DstServiceID:   dst,
		ViolationType:  vType,
		Classification: ClassInternal,
		ActionTaken:    ActionBlock,
		TraceID:        traceID,
		ZoneID:         "zone-us-east",
	}
	svc.violations = append(svc.violations, v)
	svc.score.TotalPackets++
	svc.score.Blocked++

	svc.auditLog = append(svc.auditLog, AuditEntry{
		Timestamp:      v.Timestamp,
		TraceID:        v.TraceID,
		SrcServiceID:   v.SrcServiceID,
		DstServiceID:   v.DstServiceID,
		Action:         v.ActionTaken,
		ViolationType:  v.ViolationType,
		Classification: v.Classification,
		ZoneID:         v.ZoneID,
	})
	svc.mu.Unlock()
}

// ---------------------------------------------------------------------------
// TestComplianceScore
// ---------------------------------------------------------------------------

func TestComplianceScore(t *testing.T) {
	t.Run("perfect score with no violations", func(t *testing.T) {
		svc := newTestService(t)

		// Record only allowed traffic
		for i := 0; i < 100; i++ {
			svc.RecordAllowed()
		}

		svc.aggregateViolations(context.Background())
		score := svc.GetScore()

		if score.TotalPackets != 100 {
			t.Errorf("TotalPackets = %d, want 100", score.TotalPackets)
		}
		if score.Allowed != 100 {
			t.Errorf("Allowed = %d, want 100", score.Allowed)
		}
		if score.ScorePct != 100.0 {
			t.Errorf("ScorePct = %.2f, want 100.0", score.ScorePct)
		}
		if score.ViolationRate != 0.0 {
			t.Errorf("ViolationRate = %.4f, want 0.0", score.ViolationRate)
		}
	})

	t.Run("degraded score with violations", func(t *testing.T) {
		svc := newTestService(t)

		for i := 0; i < 90; i++ {
			svc.RecordAllowed()
		}
		for i := 0; i < 10; i++ {
			svc.RecordViolation(Violation{
				Timestamp:     time.Now(),
				SrcServiceID:  "svc-a",
				DstServiceID:  "svc-b",
				ViolationType: ViolationPolicy,
				ActionTaken:   ActionBlock,
				TraceID:       "trace-test",
			})
		}

		svc.aggregateViolations(context.Background())
		score := svc.GetScore()

		if score.TotalPackets != 100 {
			t.Errorf("TotalPackets = %d, want 100", score.TotalPackets)
		}
		if score.Violations != 10 {
			t.Errorf("Violations = %d, want 10", score.Violations)
		}
		// 10% violation rate -> 90% score
		if score.ScorePct < 89.0 || score.ScorePct > 91.0 {
			t.Errorf("ScorePct = %.2f, want ~90.0", score.ScorePct)
		}
	})

	t.Run("empty service has 100% score", func(t *testing.T) {
		svc := newTestService(t)
		svc.aggregateViolations(context.Background())
		score := svc.GetScore()

		if score.ScorePct != 100.0 {
			t.Errorf("ScorePct = %.2f, want 100.0 for empty service", score.ScorePct)
		}
	})
}

// ---------------------------------------------------------------------------
// TestPolicyMatching
// ---------------------------------------------------------------------------

func TestPolicyMatching(t *testing.T) {
	svc := newTestService(t)

	t.Run("matching policy found", func(t *testing.T) {
		addTestPolicy(t, svc, "svc-a", "svc-b", ActionAllow)

		policy, ok := svc.MatchPolicy("svc-a", "svc-b")
		if !ok {
			t.Fatal("expected policy to be found")
		}
		if policy.Action != ActionAllow {
			t.Errorf("Action = %s, want ALLOW", policy.Action)
		}
		if policy.SrcServiceID != "svc-a" {
			t.Errorf("SrcServiceID = %q, want %q", policy.SrcServiceID, "svc-a")
		}
		if policy.DstServiceID != "svc-b" {
			t.Errorf("DstServiceID = %q, want %q", policy.DstServiceID, "svc-b")
		}
	})

	t.Run("no matching policy", func(t *testing.T) {
		_, ok := svc.MatchPolicy("svc-x", "svc-y")
		if ok {
			t.Error("expected no policy match for unknown services")
		}
	})

	t.Run("policy direction matters", func(t *testing.T) {
		addTestPolicy(t, svc, "svc-c", "svc-d", ActionBlock)

		// Forward direction exists
		_, ok := svc.MatchPolicy("svc-c", "svc-d")
		if !ok {
			t.Error("expected policy match for svc-c -> svc-d")
		}

		// Reverse direction does not exist
		_, ok = svc.MatchPolicy("svc-d", "svc-c")
		if ok {
			t.Error("expected no policy match for svc-d -> svc-c (direction matters)")
		}
	})

	t.Run("BLOCK policy found", func(t *testing.T) {
		addTestPolicy(t, svc, "svc-public", "svc-pii", ActionBlock)

		policy, ok := svc.MatchPolicy("svc-public", "svc-pii")
		if !ok {
			t.Fatal("expected policy to be found")
		}
		if policy.Action != ActionBlock {
			t.Errorf("Action = %s, want BLOCK", policy.Action)
		}
	})
}

// ---------------------------------------------------------------------------
// TestViolationAggregation
// ---------------------------------------------------------------------------

func TestViolationAggregation(t *testing.T) {
	t.Run("violations are trimmed to max", func(t *testing.T) {
		cfg := &Config{
			AggregationWindow: 10 * time.Second,
			MaxViolations:     5,
			MaxAuditEntries:   5,
		}
		svc := NewService(newTestLogger(), nil, cfg)

		// Add more violations than max
		for i := 0; i < 10; i++ {
			svc.RecordViolation(Violation{
				Timestamp:     time.Now(),
				SrcServiceID:  "svc-a",
				DstServiceID:  "svc-b",
				ViolationType: ViolationPolicy,
				ActionTaken:   ActionBlock,
				TraceID:       "trace-trim",
			})
		}

		svc.aggregateViolations(context.Background())

		violations := svc.ListViolations()
		if len(violations) > 5 {
			t.Errorf("len(violations) = %d, want <= 5 after trimming", len(violations))
		}
	})

	t.Run("aggregation recalculates score", func(t *testing.T) {
		svc := newTestService(t)

		for i := 0; i < 50; i++ {
			svc.RecordAllowed()
		}
		for i := 0; i < 50; i++ {
			svc.RecordViolation(Violation{
				Timestamp:     time.Now(),
				SrcServiceID:  "svc-a",
				DstServiceID:  "svc-b",
				ViolationType: ViolationPIIEgress,
				ActionTaken:   ActionBlock,
				TraceID:       "trace-agg",
			})
		}

		svc.aggregateViolations(context.Background())
		score := svc.GetScore()

		// 50% violation rate -> 50% score
		if score.ScorePct < 49.0 || score.ScorePct > 51.0 {
			t.Errorf("ScorePct = %.2f, want ~50.0", score.ScorePct)
		}
	})
}

// ---------------------------------------------------------------------------
// TestAuditSearch
// ---------------------------------------------------------------------------

func TestAuditSearch(t *testing.T) {
	svc := newTestService(t)

	addTestViolation(t, svc, "svc-a", "svc-b", ViolationPolicy, "trace-001")
	addTestViolation(t, svc, "svc-a", "svc-c", ViolationPIIEgress, "trace-002")
	addTestViolation(t, svc, "svc-b", "svc-c", ViolationCircuit, "trace-003")

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("unfiltered audit returns all", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/audit", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET audit status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		if count, ok := resp["count"].(float64); !ok || int(count) != 3 {
			t.Errorf("count = %v, want 3", resp["count"])
		}
	})

	t.Run("filter by trace_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/audit?trace_id=trace-002", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		if count, ok := resp["count"].(float64); !ok || int(count) != 1 {
			t.Errorf("count = %v, want 1 for trace_id filter", resp["count"])
		}
	})

	t.Run("filter by violation_type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/audit?violation_type=CIRCUIT", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		if count, ok := resp["count"].(float64); !ok || int(count) != 1 {
			t.Errorf("count = %v, want 1 for violation_type filter", resp["count"])
		}
	})
}

// ---------------------------------------------------------------------------
// TestDataClassification
// ---------------------------------------------------------------------------

func TestDataClassification(t *testing.T) {
	t.Run("string representation", func(t *testing.T) {
		tests := []struct {
			class DataClassification
			want  string
		}{
			{ClassPublic, "PUBLIC"},
			{ClassInternal, "INTERNAL"},
			{ClassSensitive, "SENSITIVE"},
			{ClassSovereignPII, "SOVEREIGN_PII"},
			{DataClassification(99), "UNKNOWN"},
		}

		for _, tt := range tests {
			got := tt.class.String()
			if got != tt.want {
				t.Errorf("DataClassification(%d).String() = %q, want %q", tt.class, got, tt.want)
			}
		}
	})

	t.Run("classification ordering", func(t *testing.T) {
		if ClassPublic >= ClassInternal {
			t.Error("PUBLIC should be less than INTERNAL")
		}
		if ClassInternal >= ClassSensitive {
			t.Error("INTERNAL should be less than SENSITIVE")
		}
		if ClassSensitive >= ClassSovereignPII {
			t.Error("SENSITIVE should be less than SOVEREIGN_PII")
		}
	})
}

// ---------------------------------------------------------------------------
// HTTP API tests
// ---------------------------------------------------------------------------

func TestHTTPComplianceScore(t *testing.T) {
	svc := newTestService(t)
	for i := 0; i < 100; i++ {
		svc.RecordAllowed()
	}
	svc.aggregateViolations(context.Background())

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/score", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET score status = %d, want 200", rec.Code)
	}

	var score ComplianceScore
	if err := json.NewDecoder(rec.Body).Decode(&score); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if score.ScorePct != 100.0 {
		t.Errorf("ScorePct = %.2f, want 100.0", score.ScorePct)
	}
}

func TestHTTPPolicies(t *testing.T) {
	svc := newTestService(t)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("create policy", func(t *testing.T) {
		body := `{"src_service_id":"svc-a","dst_service_id":"svc-b","action":"ALLOW","requires_encryption":true}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/policies", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("POST policies status = %d, want 201", rec.Code)
		}
	})

	t.Run("list policies", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/policies", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET policies status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		if count, ok := resp["count"].(float64); !ok || int(count) != 1 {
			t.Errorf("count = %v, want 1", resp["count"])
		}
	})
}

func TestHTTPViolations(t *testing.T) {
	svc := newTestService(t)
	addTestViolation(t, svc, "svc-x", "svc-y", ViolationPolicy, "trace-v1")

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/violations", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET violations status = %d, want 200", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if total, ok := resp["total"].(float64); !ok || int(total) != 1 {
		t.Errorf("total = %v, want 1", resp["total"])
	}
}

func TestHTTPViolationHeatmap(t *testing.T) {
	svc := newTestService(t)
	addTestViolation(t, svc, "svc-a", "svc-b", ViolationPolicy, "trace-h1")
	addTestViolation(t, svc, "svc-a", "svc-b", ViolationPIIEgress, "trace-h2")
	addTestViolation(t, svc, "svc-c", "svc-d", ViolationCircuit, "trace-h3")

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/violations/heatmap", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET heatmap status = %d, want 200", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if count, ok := resp["count"].(float64); !ok || int(count) != 2 {
		t.Errorf("count = %v, want 2 (two unique src->dst pairs)", resp["count"])
	}
}

func TestHTTPZones(t *testing.T) {
	svc := newTestService(t)
	svc.AddZone(&GeoZone{
		ZoneID:           "us-east",
		Name:             "US East",
		AllowedExitZones: []string{"us-west"},
		RequiresVPN:      false,
		PIIVault:         true,
	})

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/zones", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET zones status = %d, want 200", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if count, ok := resp["count"].(float64); !ok || int(count) != 1 {
		t.Errorf("count = %v, want 1", resp["count"])
	}
}

func TestHTTPHealthAndReady(t *testing.T) {
	svc := newTestService(t)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /health status = %d, want 200", rec.Code)
		}
	})

	t.Run("ready", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /ready status = %d, want 200", rec.Code)
		}
	})

	t.Run("metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /metrics status = %d, want 200", rec.Code)
		}
	})
}
