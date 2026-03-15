// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package waf

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestScoringEngineSetGetWeight(t *testing.T) {
	engine := NewScoringEngine(100, 50)
	defer engine.Close()

	engine.SetWeight("custom", 2.5)
	got := engine.GetWeight("custom")
	if got != 2.5 {
		t.Errorf("GetWeight = %f, want 2.5", got)
	}

	// Default weight
	if w := engine.GetWeight("sqli"); w != 1.5 {
		t.Errorf("default sqli weight = %f, want 1.5", w)
	}
}

func TestScoringEngineSetThresholds(t *testing.T) {
	engine := NewScoringEngine(100, 50)
	defer engine.Close()

	engine.SetThresholds(200, 80)

	stats := engine.GetStats()
	if stats["block_threshold"] != 200 {
		t.Errorf("block_threshold = %v, want 200", stats["block_threshold"])
	}
	if stats["log_threshold"] != 80 {
		t.Errorf("log_threshold = %v, want 80", stats["log_threshold"])
	}
}

func TestScoringEngineEnableCorrelation(t *testing.T) {
	engine := NewScoringEngine(100, 50)
	defer engine.Close()

	engine.EnableCorrelation(false)
	engine.EnableCorrelation(true)
}

func TestScoringEngineEnableBaseline(t *testing.T) {
	engine := NewScoringEngine(100, 50)
	defer engine.Close()

	engine.EnableBaseline(false)
	engine.EnableBaseline(true)
}

func TestScoringEngineGetStats(t *testing.T) {
	engine := NewScoringEngine(100, 50)
	defer engine.Close()

	stats := engine.GetStats()
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}
	if stats["block_threshold"] != 100 {
		t.Errorf("block_threshold = %v, want 100", stats["block_threshold"])
	}
	if _, ok := stats["tracked_endpoints"]; !ok {
		t.Error("expected tracked_endpoints in stats")
	}
	if _, ok := stats["active_attack_contexts"]; !ok {
		t.Error("expected active_attack_contexts in stats")
	}
}

func TestScoreFromDetectionResult(t *testing.T) {
	result := &mockDetectionResult{score: 75, matches: []string{"xss", "sqli"}}
	signal := ScoreFromDetectionResult(result)

	if signal.Score != 75 {
		t.Errorf("Score = %d, want 75", signal.Score)
	}
	if signal.Confidence != 0.8 {
		t.Errorf("Confidence = %f, want 0.8", signal.Confidence)
	}
	if len(signal.Matches) != 2 {
		t.Errorf("Matches len = %d, want 2", len(signal.Matches))
	}
}

type mockDetectionResult struct {
	score   int
	matches []string
}

func (m *mockDetectionResult) GetScore() int       { return m.score }
func (m *mockDetectionResult) GetMatches() []string { return m.matches }

func TestAggregateScores(t *testing.T) {
	signals := map[string]*SignalData{
		"sqli": {Score: 50},
		"xss":  {Score: 30},
	}
	weights := map[string]float64{
		"sqli": 2.0,
		"xss":  1.0,
	}

	total := AggregateScores(signals, weights)
	// 50*2 + 30*1 = 130
	if total != 130 {
		t.Errorf("AggregateScores = %d, want 130", total)
	}

	// Empty signals
	empty := AggregateScores(nil, weights)
	if empty != 0 {
		t.Errorf("empty AggregateScores = %d, want 0", empty)
	}
}

func TestNormalizeScore(t *testing.T) {
	// Normal case
	if n := NormalizeScore(50, 100); n != 50.0 {
		t.Errorf("NormalizeScore(50,100) = %f, want 50.0", n)
	}

	// Over max
	if n := NormalizeScore(200, 100); n != 100.0 {
		t.Errorf("NormalizeScore(200,100) = %f, want 100.0", n)
	}

	// Zero max
	if n := NormalizeScore(50, 0); n != 0 {
		t.Errorf("NormalizeScore(50,0) = %f, want 0", n)
	}

	// Negative max
	if n := NormalizeScore(50, -1); n != 0 {
		t.Errorf("NormalizeScore(50,-1) = %f, want 0", n)
	}
}

func TestCombineConfidences(t *testing.T) {
	// Empty
	if c := CombineConfidences(nil); c != 0 {
		t.Errorf("empty = %f, want 0", c)
	}

	// Single value
	c := CombineConfidences([]float64{0.8})
	if c < 0.7 || c > 0.9 {
		t.Errorf("single = %f, expected near 0.8", c)
	}

	// Multiple values
	c = CombineConfidences([]float64{0.9, 0.8, 0.7})
	if c <= 0 || c > 1 {
		t.Errorf("multi = %f, expected in (0,1]", c)
	}
}

func TestSqrt(t *testing.T) {
	tests := []struct {
		input, expected float64
		tolerance       float64
	}{
		{4, 2, 0.001},
		{9, 3, 0.001},
		{0, 0, 0},
		{-1, 0, 0},
		{1, 1, 0.001},
	}
	for _, tt := range tests {
		got := sqrt(tt.input)
		diff := got - tt.expected
		if diff < 0 {
			diff = -diff
		}
		if diff > tt.tolerance {
			t.Errorf("sqrt(%f) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestPow(t *testing.T) {
	if p := pow(2, 0); p != 1 {
		t.Errorf("pow(2,0) = %f, want 1", p)
	}
	if p := pow(2, 3); p != 8 {
		t.Errorf("pow(2,3) = %f, want 8", p)
	}
	if p := pow(3, 2); p != 9 {
		t.Errorf("pow(3,2) = %f, want 9", p)
	}
}

func TestNthRoot(t *testing.T) {
	r := nthRoot(8, 3) // cube root of 8 = 2
	if r < 1.9 || r > 2.1 {
		t.Errorf("nthRoot(8,3) = %f, want ~2.0", r)
	}

	// Edge cases
	if r := nthRoot(0, 3); r != 0 {
		t.Errorf("nthRoot(0,3) = %f, want 0", r)
	}
	if r := nthRoot(-1, 3); r != 0 {
		t.Errorf("nthRoot(-1,3) = %f, want 0", r)
	}
}

func TestIntToStr(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-5, "-5"},
		{1000, "1000"},
	}
	for _, tt := range tests {
		if got := intToStr(tt.n); got != tt.want {
			t.Errorf("intToStr(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestExtractClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	ip := extractClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("extractClientIP = %q, want 10.0.0.1", ip)
	}

	// No port
	req.RemoteAddr = "10.0.0.1"
	ip = extractClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("extractClientIP (no port) = %q, want 10.0.0.1", ip)
	}
}

func TestActionString(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{ActionAllow, "allow"},
		{ActionBlock, "block"},
		{ActionLog, "log"},
		{ActionRedirect, "redirect"},
		{ActionChallenge, "challenge"},
		{Action(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.action.String(); got != tt.want {
			t.Errorf("Action(%d).String() = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestShieldAccessors(t *testing.T) {
	shield := New()
	defer shield.Close()

	if shield.IPManager() == nil {
		t.Error("IPManager() is nil")
	}
	if shield.BotDetector() == nil {
		t.Error("BotDetector() is nil")
	}
	if shield.ScoringEngine() == nil {
		t.Error("ScoringEngine() is nil")
	}
	if shield.GetMetrics() == nil {
		t.Error("GetMetrics() is nil")
	}
}

func TestShieldSetLogger(t *testing.T) {
	shield := New()
	defer shield.Close()

	shield.SetLogger(nil)
}

func TestShieldSetStrictMode(t *testing.T) {
	shield := New()
	defer shield.Close()

	shield.SetStrictMode(true)
	shield.SetStrictMode(false)
}

func TestShieldRemoveRule(t *testing.T) {
	shield := New()
	defer shield.Close()

	err := shield.RemoveRule("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent rule")
	}
}

func TestNewWithConfig(t *testing.T) {
	config := DefaultConfig()
	shield := NewWithConfig(config)
	defer shield.Close()

	if shield == nil {
		t.Fatal("NewWithConfig returned nil")
	}
}

func TestBaselineTrackerCompareToBaseline(t *testing.T) {
	bt := NewBaselineTracker(time.Hour)
	defer bt.Close()

	// Track some requests to build baseline
	for i := 0; i < 10; i++ {
		bt.Track("/api/test", 5)
	}

	// Compare with a high score — should show deviation
	deviation := bt.CompareToBaseline("/api/test", 50)
	if deviation < 0 {
		t.Errorf("expected non-negative deviation, got %f", deviation)
	}

	// Compare with unknown endpoint
	unknown := bt.CompareToBaseline("/unknown", 50)
	_ = unknown // no baseline for unknown
}
