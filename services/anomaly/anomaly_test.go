// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package anomaly

import (
	"context"
	"io"
	"math"
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
