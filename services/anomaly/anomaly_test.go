// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package anomaly

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

// makeFlowScore creates a FlowScore with reasonable defaults.
func makeFlowScore(srcIP, dstIP string, score int, serviceID string) *FlowScore {
	return &FlowScore{
		FlowKey: FlowKey{
			SrcIP:   srcIP,
			DstIP:   dstIP,
			SrcPort: 12345,
			DstPort: 80,
			Proto:   6, // TCP
		},
		AnomalyScore: score,
		PacketCount:  100,
		LastSeen:     time.Now(),
		ServiceID:    serviceID,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestBaselineLearning(t *testing.T) {
	t.Parallel()

	t.Run("learn from samples", func(t *testing.T) {
		now := time.Now()
		samples := []FlowScore{
			{AnomalyScore: 10, PacketCount: 100, LastSeen: now},
			{AnomalyScore: 20, PacketCount: 200, LastSeen: now.Add(time.Second)},
			{AnomalyScore: 30, PacketCount: 300, LastSeen: now.Add(2 * time.Second)},
			{AnomalyScore: 40, PacketCount: 400, LastSeen: now.Add(3 * time.Second)},
		}

		baseline := learnBaseline("test-svc", samples)

		if baseline.ServiceID != "test-svc" {
			t.Errorf("expected service_id 'test-svc', got %q", baseline.ServiceID)
		}
		if baseline.SampleCount != 4 {
			t.Errorf("expected 4 samples, got %d", baseline.SampleCount)
		}

		// Mean anomaly score should be 25
		expectedMean := 25.0
		if math.Abs(baseline.EntropyMean-expectedMean) > 0.01 {
			t.Errorf("expected entropy mean ~%.1f, got %.1f", expectedMean, baseline.EntropyMean)
		}

		// Mean packet count should be 250
		expectedPacketMean := 250.0
		if math.Abs(baseline.PacketSizeMean-expectedPacketMean) > 0.01 {
			t.Errorf("expected packet size mean ~%.1f, got %.1f", expectedPacketMean, baseline.PacketSizeMean)
		}

		// StdDev should be positive
		if baseline.EntropyStdDev <= 0 {
			t.Error("expected positive entropy stddev")
		}
		if baseline.PacketSizeStdDev <= 0 {
			t.Error("expected positive packet size stddev")
		}

		// Inter-arrival should be positive
		if baseline.InterArrivalMeanMS <= 0 {
			t.Error("expected positive inter-arrival mean")
		}

		// Connection rate should be positive
		if baseline.ConnRatePerSec <= 0 {
			t.Error("expected positive connection rate")
		}

		if baseline.LastUpdated.IsZero() {
			t.Error("expected LastUpdated to be set")
		}
	})

	t.Run("learn from empty samples", func(t *testing.T) {
		baseline := learnBaseline("empty-svc", []FlowScore{})
		if baseline.SampleCount != 0 {
			t.Errorf("expected 0 samples, got %d", baseline.SampleCount)
		}
		if baseline.EntropyMean != 0 {
			t.Error("expected zero entropy mean for empty samples")
		}
	})

	t.Run("learn from single sample", func(t *testing.T) {
		samples := []FlowScore{
			{AnomalyScore: 50, PacketCount: 500, LastSeen: time.Now()},
		}
		baseline := learnBaseline("single-svc", samples)
		if baseline.SampleCount != 1 {
			t.Errorf("expected 1 sample, got %d", baseline.SampleCount)
		}
		if baseline.EntropyMean != 50.0 {
			t.Errorf("expected entropy mean 50, got %.1f", baseline.EntropyMean)
		}
		// StdDev should be 0 for single sample
		if baseline.EntropyStdDev != 0 {
			t.Errorf("expected stddev 0 for single sample, got %.1f", baseline.EntropyStdDev)
		}
	})
}

func TestAdaptiveThreshold(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("set and get threshold", func(t *testing.T) {
		threshold := &AdaptiveThreshold{
			ServiceID: "test-svc",
			Mean:      25.0,
			StdDev:    5.0,
			Threshold: 35.0, // mean + 2*stddev
		}
		svc.SetThreshold(threshold)

		got, ok := svc.GetThreshold("test-svc")
		if !ok {
			t.Fatal("expected threshold to exist")
		}
		if got.Threshold != 35.0 {
			t.Errorf("expected threshold 35.0, got %.1f", got.Threshold)
		}
		if got.LastUpdated.IsZero() {
			t.Error("expected LastUpdated to be set")
		}
	})

	t.Run("update thresholds from baselines", func(t *testing.T) {
		svc.SetBaseline(&ServiceBaseline{
			ServiceID:    "svc-a",
			EntropyMean:  20.0,
			EntropyStdDev: 3.0,
		})
		svc.SetBaseline(&ServiceBaseline{
			ServiceID:    "svc-b",
			EntropyMean:  50.0,
			EntropyStdDev: 10.0,
		})

		svc.updateThresholds()

		thA, ok := svc.GetThreshold("svc-a")
		if !ok {
			t.Fatal("expected threshold for svc-a")
		}
		expectedA := 20.0 + 2*3.0 // 26.0
		if math.Abs(thA.Threshold-expectedA) > 0.01 {
			t.Errorf("expected threshold %.1f for svc-a, got %.1f", expectedA, thA.Threshold)
		}

		thB, ok := svc.GetThreshold("svc-b")
		if !ok {
			t.Fatal("expected threshold for svc-b")
		}
		expectedB := 50.0 + 2*10.0 // 70.0
		if math.Abs(thB.Threshold-expectedB) > 0.01 {
			t.Errorf("expected threshold %.1f for svc-b, got %.1f", expectedB, thB.Threshold)
		}
	})

	t.Run("get non-existent threshold", func(t *testing.T) {
		_, ok := svc.GetThreshold("non-existent")
		if ok {
			t.Error("expected threshold not to exist")
		}
	})
}

func TestFlowScoring(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("record and retrieve flow scores", func(t *testing.T) {
		score1 := makeFlowScore("10.0.0.1", "10.0.0.2", 50, "timeguru")
		score2 := makeFlowScore("10.0.0.3", "10.0.0.4", 30, "captain")

		svc.RecordFlowScore(score1)
		svc.RecordFlowScore(score2)

		scores := svc.GetFlowScores("")
		if len(scores) != 2 {
			t.Fatalf("expected 2 scores, got %d", len(scores))
		}
	})

	t.Run("filter by service_id", func(t *testing.T) {
		scores := svc.GetFlowScores("timeguru")
		if len(scores) != 1 {
			t.Fatalf("expected 1 score for timeguru, got %d", len(scores))
		}
		if scores[0].ServiceID != "timeguru" {
			t.Errorf("expected service_id 'timeguru', got %q", scores[0].ServiceID)
		}
	})

	t.Run("update existing flow score", func(t *testing.T) {
		score := makeFlowScore("10.0.0.1", "10.0.0.2", 75, "timeguru")
		svc.RecordFlowScore(score)

		scores := svc.GetFlowScores("timeguru")
		if len(scores) != 1 {
			t.Fatalf("expected 1 score after update, got %d", len(scores))
		}
		if scores[0].AnomalyScore != 75 {
			t.Errorf("expected score 75, got %d", scores[0].AnomalyScore)
		}
	})
}

func TestAnomalyAlertGeneration(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("low score does not generate alert", func(t *testing.T) {
		score := makeFlowScore("10.0.0.1", "10.0.0.2", 50, "svc-a")
		svc.RecordFlowScore(score)

		alerts := svc.GetAlerts()
		if len(alerts) != 0 {
			t.Errorf("expected 0 alerts for score 50, got %d", len(alerts))
		}
	})

	t.Run("high score generates alert", func(t *testing.T) {
		score := makeFlowScore("10.0.0.5", "10.0.0.6", 85, "svc-b")
		svc.RecordFlowScore(score)

		alerts := svc.GetAlerts()
		if len(alerts) != 1 {
			t.Fatalf("expected 1 alert for score 85, got %d", len(alerts))
		}
		if alerts[0].Score != 85 {
			t.Errorf("expected alert score 85, got %d", alerts[0].Score)
		}
		if alerts[0].Severity != "high" {
			t.Errorf("expected severity 'high', got %q", alerts[0].Severity)
		}
	})

	t.Run("critical score generates critical alert", func(t *testing.T) {
		score := makeFlowScore("10.0.0.7", "10.0.0.8", 96, "svc-c")
		svc.RecordFlowScore(score)

		alerts := svc.GetAlerts()
		// Find the alert with score 96
		found := false
		for _, a := range alerts {
			if a.Score == 96 {
				if a.Severity != "critical" {
					t.Errorf("expected severity 'critical', got %q", a.Severity)
				}
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find alert with score 96")
		}
	})

	t.Run("severity classification", func(t *testing.T) {
		tests := []struct {
			score    int
			severity string
		}{
			{50, "low"},
			{70, "medium"},
			{85, "high"},
			{95, "critical"},
			{100, "critical"},
		}
		for _, tt := range tests {
			got := classifySeverity(tt.score)
			if got != tt.severity {
				t.Errorf("classifySeverity(%d) = %q, want %q", tt.score, got, tt.severity)
			}
		}
	})
}

func TestModelConfig(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("default model config", func(t *testing.T) {
		config := svc.GetModelConfig()
		if config.Type != "decision_tree" {
			t.Errorf("expected type 'decision_tree', got %q", config.Type)
		}
		if config.Depth != 5 {
			t.Errorf("expected depth 5, got %d", config.Depth)
		}
		if config.Version != "0.0.0" {
			t.Errorf("expected version '0.0.0', got %q", config.Version)
		}
	})

	t.Run("update model", func(t *testing.T) {
		newConfig := &ModelConfig{
			Type:             "decision_tree",
			Depth:            8,
			NodeCount:        127,
			TrainingAccuracy: 0.95,
			Version:          "1.0.0",
			Nodes: []TreeNode{
				{NodeID: 0, Threshold: 50, FeatureIdx: 0, LeftChild: 1, RightChild: 2, IsLeaf: false},
				{NodeID: 1, IsLeaf: true, LeafScore: 10},
				{NodeID: 2, IsLeaf: true, LeafScore: 90},
			},
		}
		svc.UpdateModel(newConfig)

		config := svc.GetModelConfig()
		if config.Version != "1.0.0" {
			t.Errorf("expected version '1.0.0', got %q", config.Version)
		}
		if config.Depth != 8 {
			t.Errorf("expected depth 8, got %d", config.Depth)
		}
		if config.NodeCount != 127 {
			t.Errorf("expected 127 nodes, got %d", config.NodeCount)
		}
		if config.TrainingAccuracy != 0.95 {
			t.Errorf("expected accuracy 0.95, got %.2f", config.TrainingAccuracy)
		}
		if len(config.Nodes) != 3 {
			t.Errorf("expected 3 tree nodes, got %d", len(config.Nodes))
		}
		if !config.CreatedAt.After(time.Now().Add(-time.Minute)) {
			t.Error("expected CreatedAt to be recent")
		}
	})
}

func TestHeatmapData(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("heatmap aggregation", func(t *testing.T) {
		// Add flows for different service pairs
		svc.RecordFlowScore(makeFlowScore("10.0.0.1", "10.0.0.2", 30, "svc-a"))
		svc.RecordFlowScore(makeFlowScore("10.0.0.1", "10.0.0.2", 40, "svc-a"))
		svc.RecordFlowScore(makeFlowScore("10.0.0.3", "10.0.0.4", 60, "svc-b"))

		// Get flow scores grouped
		scoresA := svc.GetFlowScores("svc-a")
		scoresB := svc.GetFlowScores("svc-b")

		if len(scoresA) == 0 {
			t.Error("expected flows for svc-a")
		}
		if len(scoresB) == 0 {
			t.Error("expected flows for svc-b")
		}
	})
}

func TestFlowKeyString(t *testing.T) {
	t.Parallel()

	fk := FlowKey{
		SrcIP:   "10.0.0.1",
		DstIP:   "10.0.0.2",
		SrcPort: 12345,
		DstPort: 80,
		Proto:   6,
	}

	got := fk.String()
	expected := "10.0.0.1:12345->10.0.0.2:80/6"
	if got != expected {
		t.Errorf("FlowKey.String() = %q, want %q", got, expected)
	}
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

func TestNewService(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	if svc.log == nil {
		t.Error("expected logger to be set")
	}
	if svc.flowScores == nil {
		t.Error("expected flowScores map to be initialized")
	}
	if svc.baselines == nil {
		t.Error("expected baselines map to be initialized")
	}
	if svc.thresholds == nil {
		t.Error("expected thresholds map to be initialized")
	}
	if svc.modelConfig == nil {
		t.Error("expected modelConfig to be initialized")
	}
	if svc.alerts == nil {
		t.Error("expected alerts slice to be initialized")
	}
}

func TestServiceStats(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	svc.RecordFlowScore(makeFlowScore("10.0.0.1", "10.0.0.2", 50, "svc-a"))
	svc.SetBaseline(&ServiceBaseline{ServiceID: "svc-a"})
	svc.SetThreshold(&AdaptiveThreshold{ServiceID: "svc-a", Threshold: 50.0})

	stats := svc.Stats()
	if stats["tracked_flows"].(int) != 1 {
		t.Errorf("expected 1 tracked flow")
	}
	if stats["baselines"].(int) != 1 {
		t.Errorf("expected 1 baseline")
	}
	if stats["thresholds"].(int) != 1 {
		t.Errorf("expected 1 threshold")
	}
	if stats["model_type"].(string) != "decision_tree" {
		t.Errorf("expected model_type 'decision_tree'")
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
	if body["service"] != "anomaly" {
		t.Errorf("expected service 'anomaly', got %v", body["service"])
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

	svc.RecordFlowScore(makeFlowScore("10.0.0.1", "10.0.0.2", 50, "svc-a"))
	svc.SetBaseline(&ServiceBaseline{ServiceID: "svc-a"})
	svc.SetThreshold(&AdaptiveThreshold{ServiceID: "svc-a", Threshold: 50.0})

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "anomaly_tracked_flows 1") {
		t.Error("expected anomaly_tracked_flows 1 in metrics")
	}
	if !strings.Contains(body, "anomaly_baselines_total 1") {
		t.Error("expected anomaly_baselines_total 1 in metrics")
	}
	if !strings.Contains(body, "anomaly_thresholds_total 1") {
		t.Error("expected anomaly_thresholds_total 1 in metrics")
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/plain; version=0.0.4" {
		t.Errorf("expected prometheus content-type, got %q", ct)
	}
}

func TestHandleTopAnomalies(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	svc.RecordFlowScore(makeFlowScore("10.0.0.1", "10.0.0.2", 90, "svc-a"))
	svc.RecordFlowScore(makeFlowScore("10.0.0.3", "10.0.0.4", 50, "svc-b"))
	svc.RecordFlowScore(makeFlowScore("10.0.0.5", "10.0.0.6", 70, "svc-a"))

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("GET all", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/top", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["count"].(float64) != 3 {
			t.Errorf("expected 3 flows, got %v", body["count"])
		}
	})

	t.Run("GET with limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/top?limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["count"].(float64) != 1 {
			t.Errorf("expected 1 flow with limit=1, got %v", body["count"])
		}
	})

	t.Run("GET filtered by service_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/top?service_id=svc-b", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["count"].(float64) != 1 {
			t.Errorf("expected 1 flow for svc-b, got %v", body["count"])
		}
	})

	t.Run("POST method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/top", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleHeatmap(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	svc.RecordFlowScore(makeFlowScore("10.0.0.1", "10.0.0.2", 30, "svc-a"))
	svc.RecordFlowScore(makeFlowScore("10.0.0.3", "10.0.0.4", 60, "svc-b"))

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("GET heatmap", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/heatmap", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["count"].(float64) < 1 {
			t.Errorf("expected at least 1 heatmap cell, got %v", body["count"])
		}
	})

	t.Run("POST method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/heatmap", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleHeatmapEmptyServiceID(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	// Flow with empty service_id should map to "unknown"
	score := makeFlowScore("10.0.0.1", "10.0.0.2", 40, "")
	svc.RecordFlowScore(score)

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/heatmap", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleBaseline(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	svc.SetBaseline(&ServiceBaseline{ServiceID: "svc-a", EntropyMean: 25.0})
	svc.SetThreshold(&AdaptiveThreshold{ServiceID: "svc-a", Threshold: 35.0})

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("GET existing baseline", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/baseline/svc-a", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["baseline"] == nil {
			t.Error("expected baseline in response")
		}
		if body["threshold"] == nil {
			t.Error("expected threshold in response")
		}
	})

	t.Run("GET baseline without threshold", func(t *testing.T) {
		svc.SetBaseline(&ServiceBaseline{ServiceID: "svc-nothresh", EntropyMean: 10.0})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/baseline/svc-nothresh", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["threshold"] != nil {
			t.Error("expected no threshold for svc-nothresh")
		}
	})

	t.Run("GET non-existent baseline", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/baseline/non-existent", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("GET baseline empty service_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/baseline/", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("POST method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/baseline/svc-a", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleModelConfig(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("GET model config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/model-config", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var config ModelConfig
		json.NewDecoder(rec.Body).Decode(&config)
		if config.Type != "decision_tree" {
			t.Errorf("expected type 'decision_tree', got %q", config.Type)
		}
	})

	t.Run("POST method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/model-config", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleModelUpdate(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("POST update model", func(t *testing.T) {
		body := `{"type":"decision_tree","depth":10,"node_count":200,"training_accuracy":0.98,"version":"2.0.0"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/model", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var config ModelConfig
		json.NewDecoder(rec.Body).Decode(&config)
		if config.Version != "2.0.0" {
			t.Errorf("expected version '2.0.0', got %q", config.Version)
		}
	})

	t.Run("POST with default type", func(t *testing.T) {
		body := `{"depth":5,"version":"2.1.0"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/model", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var config ModelConfig
		json.NewDecoder(rec.Body).Decode(&config)
		if config.Type != "decision_tree" {
			t.Errorf("expected default type 'decision_tree', got %q", config.Type)
		}
	})

	t.Run("POST missing version", func(t *testing.T) {
		body := `{"type":"decision_tree","depth":5}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/model", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("POST invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/model", strings.NewReader("bad"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("GET method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/model", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleThresholdOverride(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("POST override threshold", func(t *testing.T) {
		body := `{"threshold":75.0}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/threshold/svc-a", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		th, ok := svc.GetThreshold("svc-a")
		if !ok {
			t.Fatal("expected threshold to exist after override")
		}
		if th.Threshold != 75.0 {
			t.Errorf("expected threshold 75.0, got %f", th.Threshold)
		}
	})

	t.Run("POST empty service_id", func(t *testing.T) {
		body := `{"threshold":10.0}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/threshold/", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("POST invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/threshold/svc-a", strings.NewReader("bad"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("GET method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/threshold/svc-a", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestHandleAlerts(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	// Generate alerts by recording high-score flows
	for i := 0; i < 5; i++ {
		svc.RecordFlowScore(makeFlowScore(
			fmt.Sprintf("10.0.%d.1", i),
			fmt.Sprintf("10.0.%d.2", i),
			85+i, "svc-alert",
		))
	}

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	t.Run("GET alerts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/alerts", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["count"].(float64) < 1 {
			t.Errorf("expected at least 1 alert")
		}
	})

	t.Run("GET alerts with limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/alerts?limit=2", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		var body map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&body)
		if body["count"].(float64) != 2 {
			t.Errorf("expected 2 alerts with limit=2, got %v", body["count"])
		}
	})

	t.Run("POST method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/alerts", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestLearnBaselinesFromFlows(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	svc.RecordFlowScore(makeFlowScore("10.0.0.1", "10.0.0.2", 30, "svc-a"))
	svc.RecordFlowScore(makeFlowScore("10.0.0.3", "10.0.0.4", 50, "svc-a"))
	svc.RecordFlowScore(makeFlowScore("10.0.0.5", "10.0.0.6", 60, "svc-b"))

	svc.learnBaselinesFromFlows()

	baseA, ok := svc.GetBaseline("svc-a")
	if !ok {
		t.Fatal("expected baseline for svc-a")
	}
	if baseA.SampleCount != 2 {
		t.Errorf("expected 2 samples for svc-a, got %d", baseA.SampleCount)
	}

	baseB, ok := svc.GetBaseline("svc-b")
	if !ok {
		t.Fatal("expected baseline for svc-b")
	}
	if baseB.SampleCount != 1 {
		t.Errorf("expected 1 sample for svc-b, got %d", baseB.SampleCount)
	}
}

func TestLearnBaselinesEmptyServiceID(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	svc.RecordFlowScore(makeFlowScore("10.0.0.1", "10.0.0.2", 30, ""))

	svc.learnBaselinesFromFlows()

	_, ok := svc.GetBaseline("unknown")
	if !ok {
		t.Fatal("expected baseline for 'unknown' service (empty service_id)")
	}
}

func TestPushThresholdsToBPF(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

	// No thresholds - should not increment drops
	svc.pushThresholdsToBPF(ctx)
	if svc.WotanDrops() != 0 {
		t.Errorf("expected 0 drops for empty thresholds, got %d", svc.WotanDrops())
	}

	// With thresholds and nil wotan - should increment drops
	svc.SetThreshold(&AdaptiveThreshold{ServiceID: "svc-a", Threshold: 50.0})
	svc.pushThresholdsToBPF(ctx)
	if svc.WotanDrops() != 1 {
		t.Errorf("expected 1 drop after push, got %d", svc.WotanDrops())
	}
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

func TestListenForAlertsNilWotan(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx := context.Background()

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

func TestBaselineLearningLoopCancellation(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.baselineLearningLoop(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("baselineLearningLoop did not exit after context cancel")
	}
}

func TestThresholdUpdateLoopCancellation(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.thresholdUpdateLoop(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("thresholdUpdateLoop did not exit after context cancel")
	}
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

func TestGetBaseline(t *testing.T) {
	t.Parallel()
	svc := newTestService()

	t.Run("set and get", func(t *testing.T) {
		svc.SetBaseline(&ServiceBaseline{
			ServiceID:      "svc-x",
			PacketSizeMean: 100.0,
			EntropyMean:    20.0,
		})
		b, ok := svc.GetBaseline("svc-x")
		if !ok {
			t.Fatal("expected baseline to exist")
		}
		if b.PacketSizeMean != 100.0 {
			t.Errorf("expected packet_size_mean 100.0, got %f", b.PacketSizeMean)
		}
		if b.LastUpdated.IsZero() {
			t.Error("expected LastUpdated to be set")
		}
	})

	t.Run("get non-existent", func(t *testing.T) {
		_, ok := svc.GetBaseline("non-existent")
		if ok {
			t.Error("expected baseline not to exist")
		}
	})
}
