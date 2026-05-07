// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package compliance

import (
	"bytes"
	"encoding/json"
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
// Tests for previously uncovered functions and branches
// ---------------------------------------------------------------------------

func TestGetZone(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	svc.AddZone(&GeoZone{
		ZoneID:           "eu-west",
		Name:             "EU West",
		AllowedExitZones: []string{"eu-central"},
		RequiresVPN:      true,
		PIIVault:         true,
	})

	t.Run("existing zone", func(t *testing.T) {
		zone, ok := svc.GetZone("eu-west")
		if !ok {
			t.Fatal("expected zone to be found")
		}
		if zone.Name != "EU West" {
			t.Errorf("Name = %q, want %q", zone.Name, "EU West")
		}
		if !zone.RequiresVPN {
			t.Error("expected RequiresVPN = true")
		}
		if !zone.PIIVault {
			t.Error("expected PIIVault = true")
		}
		if len(zone.AllowedExitZones) != 1 || zone.AllowedExitZones[0] != "eu-central" {
			t.Errorf("AllowedExitZones = %v, want [eu-central]", zone.AllowedExitZones)
		}
	})

	t.Run("non-existent zone", func(t *testing.T) {
		_, ok := svc.GetZone("ap-south")
		if ok {
			t.Error("expected zone not to be found")
		}
	})
}

func TestWotanDrops(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	// Initially zero
	if drops := svc.WotanDrops(); drops != 0 {
		t.Errorf("initial WotanDrops = %d, want 0", drops)
	}

	// RecordViolation with nil wotan should increment drops
	svc.RecordViolation(Violation{
		Timestamp:     time.Now(),
		SrcServiceID:  "svc-a",
		DstServiceID:  "svc-b",
		ViolationType: ViolationPolicy,
		ActionTaken:   ActionBlock,
		TraceID:       "trace-drop-1",
	})

	if drops := svc.WotanDrops(); drops != 1 {
		t.Errorf("WotanDrops after violation = %d, want 1", drops)
	}
}

func TestListPolicies(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	t.Run("empty policies", func(t *testing.T) {
		policies := svc.ListPolicies()
		if len(policies) != 0 {
			t.Errorf("len(policies) = %d, want 0", len(policies))
		}
	})

	t.Run("with policies", func(t *testing.T) {
		now := time.Now()
		svc.mu.Lock()
		svc.policies["a->b"] = &ServicePolicy{
			SrcServiceID: "a", DstServiceID: "b", Action: ActionAllow,
			CreatedAt: now, UpdatedAt: now,
		}
		svc.policies["c->d"] = &ServicePolicy{
			SrcServiceID: "c", DstServiceID: "d", Action: ActionBlock,
			CreatedAt: now, UpdatedAt: now,
		}
		svc.mu.Unlock()

		policies := svc.ListPolicies()
		if len(policies) != 2 {
			t.Errorf("len(policies) = %d, want 2", len(policies))
		}
	})
}

func TestStats(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	// Add some data
	svc.AddZone(&GeoZone{ZoneID: "z1", Name: "Zone 1"})
	svc.mu.Lock()
	svc.policies["x->y"] = &ServicePolicy{
		SrcServiceID: "x", DstServiceID: "y", Action: ActionAllow,
	}
	svc.mu.Unlock()
	svc.RecordAllowed()
	svc.RecordAllowed()
	svc.RecordViolation(Violation{
		Timestamp:     time.Now(),
		SrcServiceID:  "x",
		DstServiceID:  "y",
		ViolationType: ViolationPolicy,
		ActionTaken:   ActionBlock,
		TraceID:       "trace-stats",
	})

	stats := svc.Stats()

	if stats["total_policies"] != 1 {
		t.Errorf("total_policies = %v, want 1", stats["total_policies"])
	}
	if stats["total_violations"] != 1 {
		t.Errorf("total_violations = %v, want 1", stats["total_violations"])
	}
	if stats["total_audit_entries"] != 1 {
		t.Errorf("total_audit_entries = %v, want 1", stats["total_audit_entries"])
	}
	if stats["total_zones"] != 1 {
		t.Errorf("total_zones = %v, want 1", stats["total_zones"])
	}
	if stats["total_packets"] != int64(3) {
		t.Errorf("total_packets = %v, want 3", stats["total_packets"])
	}
	if stats["allowed_packets"] != int64(2) {
		t.Errorf("allowed_packets = %v, want 2", stats["allowed_packets"])
	}
	if stats["blocked_packets"] != int64(1) {
		t.Errorf("blocked_packets = %v, want 1", stats["blocked_packets"])
	}
	if stats["wotan_drops"].(int64) < 1 {
		t.Errorf("wotan_drops = %v, want >= 1", stats["wotan_drops"])
	}
}

func TestHandleCriticalAlert(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	// nil message should not panic
	svc.handleCriticalAlert(nil, nil)

	// Valid message should not panic
	svc.handleCriticalAlert(nil, &wotanClient.Message{
		MessageID: "msg-001",
		Topic:     "alerts.critical",
	})
}

func TestPublishEventNilWotan(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	initialDrops := svc.WotanDrops()
	svc.publishEvent(nil, "test.topic", map[string]interface{}{
		"key": "value",
	})

	if drops := svc.WotanDrops(); drops != initialDrops+1 {
		t.Errorf("WotanDrops = %d, want %d", drops, initialDrops+1)
	}
}

// ---------------------------------------------------------------------------
// HTTP handler method-not-allowed branches
// ---------------------------------------------------------------------------

func TestHTTPMethodNotAllowed(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"score POST", http.MethodPost, "/api/v1/compliance/score"},
		{"score DELETE", http.MethodDelete, "/api/v1/compliance/score"},
		{"violations POST", http.MethodPost, "/api/v1/compliance/violations"},
		{"violations DELETE", http.MethodDelete, "/api/v1/compliance/violations"},
		{"heatmap POST", http.MethodPost, "/api/v1/compliance/violations/heatmap"},
		{"heatmap PUT", http.MethodPut, "/api/v1/compliance/violations/heatmap"},
		{"audit POST", http.MethodPost, "/api/v1/compliance/audit"},
		{"audit DELETE", http.MethodDelete, "/api/v1/compliance/audit"},
		{"zones POST", http.MethodPost, "/api/v1/compliance/zones"},
		{"zones PUT", http.MethodPut, "/api/v1/compliance/zones"},
		{"policies DELETE", http.MethodDelete, "/api/v1/compliance/policies"},
		{"policies PUT", http.MethodPut, "/api/v1/compliance/policies"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s status = %d, want 405", tt.method, tt.path, rec.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Violations pagination edge cases
// ---------------------------------------------------------------------------

func TestHTTPViolationsPagination(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	// Add 5 violations
	for i := 0; i < 5; i++ {
		addTestViolation(t, svc, "svc-a", "svc-b", ViolationPolicy, "trace-page")
	}

	t.Run("with offset and limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/violations?offset=2&limit=2", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)

		violations := resp["violations"].([]interface{})
		if len(violations) != 2 {
			t.Errorf("violations count = %d, want 2", len(violations))
		}
		if int(resp["offset"].(float64)) != 2 {
			t.Errorf("offset = %v, want 2", resp["offset"])
		}
		if int(resp["limit"].(float64)) != 2 {
			t.Errorf("limit = %v, want 2", resp["limit"])
		}
	})

	t.Run("offset beyond total", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/violations?offset=100", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)

		violations := resp["violations"].([]interface{})
		if len(violations) != 0 {
			t.Errorf("violations count = %d, want 0 when offset > total", len(violations))
		}
	})

	t.Run("limit larger than remaining", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/violations?offset=3&limit=100", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)

		violations := resp["violations"].([]interface{})
		if len(violations) != 2 {
			t.Errorf("violations count = %d, want 2 (5 total, offset 3)", len(violations))
		}
	})
}

// ---------------------------------------------------------------------------
// Policy update edge cases
// ---------------------------------------------------------------------------

func TestHTTPUpdatePolicyEdgeCases(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("invalid JSON body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/policies",
			bytes.NewBufferString("{invalid json"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/policies",
			bytes.NewBufferString(`{"action":"ALLOW"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("missing src_service_id only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/policies",
			bytes.NewBufferString(`{"dst_service_id":"svc-b","action":"ALLOW"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("update existing policy returns 200", func(t *testing.T) {
		body := `{"src_service_id":"svc-a","dst_service_id":"svc-b","action":"ALLOW"}`

		// Create first
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/policies",
			bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("first POST status = %d, want 201", rec.Code)
		}

		// Update same policy
		body2 := `{"src_service_id":"svc-a","dst_service_id":"svc-b","action":"BLOCK"}`
		req2 := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/policies",
			bytes.NewBufferString(body2))
		req2.Header.Set("Content-Type", "application/json")
		rec2 := httptest.NewRecorder()
		mux.ServeHTTP(rec2, req2)

		if rec2.Code != http.StatusOK {
			t.Errorf("second POST status = %d, want 200 (update)", rec2.Code)
		}

		var updated ServicePolicy
		json.NewDecoder(rec2.Body).Decode(&updated)
		if updated.Action != ActionBlock {
			t.Errorf("Action = %s, want BLOCK", updated.Action)
		}
	})
}

// ---------------------------------------------------------------------------
// Audit search with time range filters
// ---------------------------------------------------------------------------

func TestHTTPAuditTimeFilters(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	// Add audit entries with specific timestamps
	baseTime := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	svc.mu.Lock()
	svc.auditLog = append(svc.auditLog,
		AuditEntry{
			Timestamp:     baseTime,
			TraceID:       "trace-t1",
			SrcServiceID:  "svc-a",
			DstServiceID:  "svc-b",
			Action:        ActionBlock,
			ViolationType: ViolationPolicy,
			ZoneID:        "us-east",
		},
		AuditEntry{
			Timestamp:     baseTime.Add(2 * time.Hour),
			TraceID:       "trace-t2",
			SrcServiceID:  "svc-a",
			DstServiceID:  "svc-c",
			Action:        ActionBlock,
			ViolationType: ViolationPIIEgress,
			ZoneID:        "eu-west",
		},
		AuditEntry{
			Timestamp:     baseTime.Add(4 * time.Hour),
			TraceID:       "trace-t3",
			SrcServiceID:  "svc-b",
			DstServiceID:  "svc-c",
			Action:        ActionBlock,
			ViolationType: ViolationCircuit,
			ZoneID:        "us-east",
		},
	)
	svc.mu.Unlock()

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("filter by start time", func(t *testing.T) {
		start := baseTime.Add(1 * time.Hour).Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/audit?start="+start, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		count := int(resp["count"].(float64))
		if count != 2 {
			t.Errorf("count = %d, want 2 (entries after start)", count)
		}
	})

	t.Run("filter by end time", func(t *testing.T) {
		end := baseTime.Add(3 * time.Hour).Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/audit?end="+end, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		count := int(resp["count"].(float64))
		if count != 2 {
			t.Errorf("count = %d, want 2 (entries before end)", count)
		}
	})

	t.Run("filter by start and end time", func(t *testing.T) {
		start := baseTime.Add(1 * time.Hour).Format(time.RFC3339)
		end := baseTime.Add(3 * time.Hour).Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/compliance/audit?start="+start+"&end="+end, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		count := int(resp["count"].(float64))
		if count != 1 {
			t.Errorf("count = %d, want 1 (entries between start and end)", count)
		}
	})

	t.Run("invalid start time is ignored", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/audit?start=not-a-date", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		count := int(resp["count"].(float64))
		if count != 3 {
			t.Errorf("count = %d, want 3 (invalid start ignored)", count)
		}
	})

	t.Run("invalid end time is ignored", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/audit?end=not-a-date", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		count := int(resp["count"].(float64))
		if count != 3 {
			t.Errorf("count = %d, want 3 (invalid end ignored)", count)
		}
	})
}

// ---------------------------------------------------------------------------
// Metrics with data populated
// ---------------------------------------------------------------------------

func TestHTTPMetricsWithData(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	// Populate some state
	svc.AddZone(&GeoZone{ZoneID: "z1", Name: "Zone 1"})
	svc.mu.Lock()
	svc.policies["a->b"] = &ServicePolicy{SrcServiceID: "a", DstServiceID: "b"}
	svc.mu.Unlock()
	svc.RecordAllowed()
	svc.RecordViolation(Violation{
		Timestamp:     time.Now(),
		SrcServiceID:  "a",
		DstServiceID:  "b",
		ViolationType: ViolationPolicy,
		ActionTaken:   ActionBlock,
	})
	svc.aggregateViolations(nil)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	expectedMetrics := []string{
		"compliance_score_pct",
		"compliance_total_packets 2",
		"compliance_violations_total 1",
		"compliance_policies_total 1",
		"compliance_audit_entries_total 1",
		"compliance_zones_total 1",
		"compliance_wotan_drops_total",
	}
	for _, m := range expectedMetrics {
		if !strings.Contains(body, m) {
			t.Errorf("metrics missing %q", m)
		}
	}
}

// ---------------------------------------------------------------------------
// Heatmap empty state
// ---------------------------------------------------------------------------

func TestHTTPHeatmapEmpty(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/violations/heatmap", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if int(resp["count"].(float64)) != 0 {
		t.Errorf("count = %v, want 0 for empty heatmap", resp["count"])
	}
}

// ---------------------------------------------------------------------------
// Zones empty state
// ---------------------------------------------------------------------------

func TestHTTPZonesEmpty(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/zones", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if int(resp["count"].(float64)) != 0 {
		t.Errorf("count = %v, want 0", resp["count"])
	}
}

// ---------------------------------------------------------------------------
// Ready endpoint reflects started state
// ---------------------------------------------------------------------------

func TestHTTPReadyState(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("not started", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp["ready"] != false {
			t.Errorf("ready = %v, want false before Start()", resp["ready"])
		}
	})
}

// ---------------------------------------------------------------------------
// Aggregation edge cases
// ---------------------------------------------------------------------------

func TestAggregateViolationsAuditCompleteness(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	// Record packets without corresponding audit entries
	for i := 0; i < 10; i++ {
		svc.RecordAllowed()
	}
	// Add a violation which also adds an audit entry
	svc.RecordViolation(Violation{
		Timestamp:     time.Now(),
		SrcServiceID:  "svc-a",
		DstServiceID:  "svc-b",
		ViolationType: ViolationPolicy,
		ActionTaken:   ActionBlock,
		TraceID:       "trace-audit",
	})

	svc.aggregateViolations(nil)
	score := svc.GetScore()

	// 1 audit entry out of 11 total packets
	expectedCompleteness := 1.0 / 11.0
	if score.AuditCompleteness < expectedCompleteness-0.01 || score.AuditCompleteness > expectedCompleteness+0.01 {
		t.Errorf("AuditCompleteness = %.4f, want ~%.4f", score.AuditCompleteness, expectedCompleteness)
	}
}

func TestAggregateViolationsAuditCompletenessCap(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	// Create a scenario where audit entries exceed total packets
	// by manually adding audit entries
	svc.RecordAllowed()
	svc.mu.Lock()
	for i := 0; i < 5; i++ {
		svc.auditLog = append(svc.auditLog, AuditEntry{
			Timestamp:    time.Now(),
			TraceID:      "extra",
			SrcServiceID: "svc-a",
			DstServiceID: "svc-b",
			Action:       ActionAllow,
		})
	}
	svc.mu.Unlock()

	svc.aggregateViolations(nil)
	score := svc.GetScore()

	if score.AuditCompleteness > 1.0 {
		t.Errorf("AuditCompleteness = %.4f, should be capped at 1.0", score.AuditCompleteness)
	}
}

func TestAggregateViolationsTrimsAuditLog(t *testing.T) {
	cfg := &Config{
		AggregationWindow: 10 * time.Second,
		MaxViolations:     100,
		MaxAuditEntries:   3,
	}
	svc := NewService(logger.New(io.Discard), nil, cfg)

	// Add more audit entries than max (via violations)
	for i := 0; i < 6; i++ {
		svc.RecordViolation(Violation{
			Timestamp:     time.Now(),
			SrcServiceID:  "svc-a",
			DstServiceID:  "svc-b",
			ViolationType: ViolationPolicy,
			ActionTaken:   ActionBlock,
			TraceID:       "trace-trim-audit",
		})
	}

	svc.aggregateViolations(nil)

	svc.mu.RLock()
	auditCount := len(svc.auditLog)
	svc.mu.RUnlock()

	if auditCount > 3 {
		t.Errorf("audit log count = %d, want <= 3 after trimming", auditCount)
	}
}

// ---------------------------------------------------------------------------
// Health endpoint response body
// ---------------------------------------------------------------------------

func TestHTTPHealthBody(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", resp["status"])
	}
	if resp["service"] != "wire-compliance" {
		t.Errorf("service = %v, want wire-compliance", resp["service"])
	}
}

// ---------------------------------------------------------------------------
// Score with negative scenario (more violations than packets is impossible
// via normal API but test the score floor at 0)
// ---------------------------------------------------------------------------

func TestAggregateViolationsScoreFloor(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	// Manually set a state where violations exceed total packets
	svc.mu.Lock()
	svc.score.TotalPackets = 1
	for i := 0; i < 5; i++ {
		svc.violations = append(svc.violations, Violation{
			Timestamp:     time.Now(),
			SrcServiceID:  "svc-a",
			DstServiceID:  "svc-b",
			ViolationType: ViolationPolicy,
			ActionTaken:   ActionBlock,
		})
	}
	svc.mu.Unlock()

	svc.aggregateViolations(nil)
	score := svc.GetScore()

	if score.ScorePct < 0 {
		t.Errorf("ScorePct = %.2f, should not be negative", score.ScorePct)
	}
}

// ---------------------------------------------------------------------------
// listenForAlerts with nil wotan
// ---------------------------------------------------------------------------

func TestListenForAlertsNilWotan(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	// Should return immediately without panicking
	svc.listenForAlerts(nil)
}

// ---------------------------------------------------------------------------
// DefaultConfig values
// ---------------------------------------------------------------------------

func TestDefaultConfigValues(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.AggregationWindow != 10*time.Second {
		t.Errorf("AggregationWindow = %v, want 10s", cfg.AggregationWindow)
	}
	if cfg.MaxViolations != 10000 {
		t.Errorf("MaxViolations = %d, want 10000", cfg.MaxViolations)
	}
	if cfg.MaxAuditEntries != 50000 {
		t.Errorf("MaxAuditEntries = %d, want 50000", cfg.MaxAuditEntries)
	}
	if cfg.WotanTopic != "compliance.violations" {
		t.Errorf("WotanTopic = %q, want compliance.violations", cfg.WotanTopic)
	}
}

// ---------------------------------------------------------------------------
// NewService with nil config uses defaults
// ---------------------------------------------------------------------------

func TestNewServiceNilConfig(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	if svc.config.MaxViolations != 10000 {
		t.Errorf("MaxViolations = %d, want 10000 (default)", svc.config.MaxViolations)
	}
}

// ---------------------------------------------------------------------------
// Compliance score JSON response structure
// ---------------------------------------------------------------------------

func TestHTTPComplianceScoreResponse(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	svc.RecordAllowed()
	svc.RecordViolation(Violation{
		Timestamp:     time.Now(),
		SrcServiceID:  "a",
		DstServiceID:  "b",
		ViolationType: ViolationPolicy,
		ActionTaken:   ActionBlock,
	})
	svc.aggregateViolations(nil)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/score", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var score ComplianceScore
	json.NewDecoder(rec.Body).Decode(&score)

	if score.TotalPackets != 2 {
		t.Errorf("TotalPackets = %d, want 2", score.TotalPackets)
	}
	if score.Violations != 1 {
		t.Errorf("Violations = %d, want 1", score.Violations)
	}
	if score.ScorePct != 50.0 {
		t.Errorf("ScorePct = %.2f, want 50.0", score.ScorePct)
	}
}
