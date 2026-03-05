// SPDX-License-Identifier: MIT

package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewSuite(t *testing.T) {
	s := NewSuite()
	if s == nil {
		t.Fatal("NewSuite returned nil")
	}
	if s.iterations != 100 {
		t.Errorf("expected default iterations=100, got %d", s.iterations)
	}
	if s.warmup != 5 {
		t.Errorf("expected default warmup=5, got %d", s.warmup)
	}
}

func TestSetIterations(t *testing.T) {
	s := NewSuite()

	s.SetIterations(50)
	if s.iterations != 50 {
		t.Errorf("expected iterations=50, got %d", s.iterations)
	}

	// Minimum clamp
	s.SetIterations(0)
	if s.iterations != 1 {
		t.Errorf("expected iterations clamped to 1, got %d", s.iterations)
	}

	s.SetIterations(-10)
	if s.iterations != 1 {
		t.Errorf("expected iterations clamped to 1, got %d", s.iterations)
	}
}

func TestSetWarmup(t *testing.T) {
	s := NewSuite()

	s.SetWarmup(10)
	if s.warmup != 10 {
		t.Errorf("expected warmup=10, got %d", s.warmup)
	}

	s.SetWarmup(-1)
	if s.warmup != 0 {
		t.Errorf("expected warmup clamped to 0, got %d", s.warmup)
	}
}

func TestAddTest(t *testing.T) {
	s := NewSuite()

	s.AddTest("test-one", func(n int) error { return nil })
	s.AddTest("test-two", func(n int) error { return nil })

	if len(s.tests) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(s.tests))
	}
	if s.tests[0].name != "test-one" {
		t.Errorf("expected first test name 'test-one', got %q", s.tests[0].name)
	}
	if s.tests[1].name != "test-two" {
		t.Errorf("expected second test name 'test-two', got %q", s.tests[1].name)
	}
}

func TestRunBasic(t *testing.T) {
	s := NewSuite()
	s.SetIterations(10)
	s.SetWarmup(2)

	callCount := 0
	s.AddTest("counter", func(n int) error {
		callCount++
		time.Sleep(100 * time.Microsecond)
		return nil
	})

	results := s.Run()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Name != "counter" {
		t.Errorf("expected name 'counter', got %q", r.Name)
	}
	if r.Iterations != 10 {
		t.Errorf("expected iterations=10, got %d", r.Iterations)
	}
	if r.Error != "" {
		t.Errorf("unexpected error: %s", r.Error)
	}

	// Warmup (2) + measurement (10) = 12 calls
	expectedCalls := 12
	if callCount != expectedCalls {
		t.Errorf("expected %d calls (warmup+iterations), got %d", expectedCalls, callCount)
	}
}

func TestRunStatistics(t *testing.T) {
	s := NewSuite()
	s.SetIterations(50)
	s.SetWarmup(0)

	s.AddTest("sleep-bench", func(n int) error {
		time.Sleep(1 * time.Millisecond)
		return nil
	})

	results := s.Run()
	r := results[0]

	// Min must be <= Avg <= Max
	if r.Min > r.Avg {
		t.Errorf("Min (%s) > Avg (%s)", r.Min, r.Avg)
	}
	if r.Avg > r.Max {
		t.Errorf("Avg (%s) > Max (%s)", r.Avg, r.Max)
	}

	// Median should be between Min and Max
	if r.Median < r.Min || r.Median > r.Max {
		t.Errorf("Median (%s) outside [Min=%s, Max=%s]", r.Median, r.Min, r.Max)
	}

	// P95 >= Median
	if r.P95 < r.Median {
		t.Errorf("P95 (%s) < Median (%s)", r.P95, r.Median)
	}

	// P99 >= P95
	if r.P99 < r.P95 {
		t.Errorf("P99 (%s) < P95 (%s)", r.P99, r.P95)
	}

	// StdDev should be non-negative
	if r.StdDev < 0 {
		t.Errorf("StdDev is negative: %s", r.StdDev)
	}

	// Total time should be roughly iterations * sleep time
	expectedMin := 50 * time.Millisecond
	if r.TotalTime < expectedMin {
		t.Errorf("TotalTime (%s) less than expected minimum (%s)", r.TotalTime, expectedMin)
	}

	// Timestamp should be recent
	if time.Since(r.Timestamp) > 10*time.Second {
		t.Errorf("Timestamp too old: %s", r.Timestamp)
	}
}

func TestRunWithError(t *testing.T) {
	s := NewSuite()
	s.SetIterations(10)
	s.SetWarmup(0)

	s.AddTest("failing", func(n int) error {
		return fmt.Errorf("connection refused")
	})

	results := s.Run()
	r := results[0]

	if r.Error == "" {
		t.Fatal("expected error in result, got none")
	}
	if r.Error != "connection refused" {
		t.Errorf("expected 'connection refused', got %q", r.Error)
	}
}

func TestRunMultipleTests(t *testing.T) {
	s := NewSuite()
	s.SetIterations(5)
	s.SetWarmup(0)

	s.AddTest("fast", func(n int) error {
		return nil
	})
	s.AddTest("slow", func(n int) error {
		time.Sleep(1 * time.Millisecond)
		return nil
	})

	results := s.Run()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "fast" {
		t.Errorf("expected first result name 'fast', got %q", results[0].Name)
	}
	if results[1].Name != "slow" {
		t.Errorf("expected second result name 'slow', got %q", results[1].Name)
	}

	// "slow" should have higher avg than "fast"
	if results[1].Avg < results[0].Avg {
		t.Errorf("slow avg (%s) should be > fast avg (%s)", results[1].Avg, results[0].Avg)
	}
}

func TestResultsReturnsPreviousRun(t *testing.T) {
	s := NewSuite()
	s.SetIterations(3)
	s.SetWarmup(0)
	s.AddTest("x", func(n int) error { return nil })

	// Before Run, Results should be nil/empty
	if r := s.Results(); len(r) != 0 {
		t.Errorf("expected empty results before Run, got %d", len(r))
	}

	s.Run()
	r := s.Results()
	if len(r) != 1 {
		t.Fatalf("expected 1 result after Run, got %d", len(r))
	}
}

func TestSummaryReport(t *testing.T) {
	s := NewSuite()
	s.SetIterations(5)
	s.SetWarmup(0)

	// No results yet
	report := s.SummaryReport()
	if !strings.Contains(report, "No benchmark results") {
		t.Error("expected 'no results' message before Run")
	}

	s.AddTest("ping-test", func(n int) error {
		time.Sleep(100 * time.Microsecond)
		return nil
	})
	s.Run()

	report = s.SummaryReport()
	if !strings.Contains(report, "ping-test") {
		t.Error("report should contain test name 'ping-test'")
	}
	if !strings.Contains(report, "Iterations") {
		t.Error("report should contain 'Iterations'")
	}
	if !strings.Contains(report, "Median") {
		t.Error("report should contain 'Median'")
	}
	if !strings.Contains(report, "P95") {
		t.Error("report should contain 'P95'")
	}
}

func TestSummaryReportWithError(t *testing.T) {
	s := NewSuite()
	s.SetIterations(3)
	s.SetWarmup(0)
	s.AddTest("broken", func(n int) error {
		return fmt.Errorf("kaboom")
	})
	s.Run()

	report := s.SummaryReport()
	if !strings.Contains(report, "ERROR") {
		t.Error("report should contain ERROR for failed test")
	}
	if !strings.Contains(report, "kaboom") {
		t.Error("report should contain error message")
	}
}

func TestSaveJSON(t *testing.T) {
	s := NewSuite()
	s.SetIterations(5)
	s.SetWarmup(0)
	s.AddTest("json-test", func(n int) error {
		time.Sleep(50 * time.Microsecond)
		return nil
	})
	s.Run()

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "results.json")

	err := s.SaveJSON(outFile)
	if err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}

	var report struct {
		Version   string            `json:"version"`
		Timestamp time.Time         `json:"timestamp"`
		Results   []BenchmarkResult `json:"results"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}

	if report.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", report.Version)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result in JSON, got %d", len(report.Results))
	}
	if report.Results[0].Name != "json-test" {
		t.Errorf("expected name 'json-test', got %q", report.Results[0].Name)
	}
}

func TestSaveJSONNoResults(t *testing.T) {
	s := NewSuite()
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "empty.json")

	err := s.SaveJSON(outFile)
	if err == nil {
		t.Fatal("expected error when saving with no results")
	}
}

func TestPercentileEdgeCases(t *testing.T) {
	// Empty slice
	result := percentile(nil, 0.95)
	if result != 0 {
		t.Errorf("expected 0 for empty slice, got %s", result)
	}

	// Single element
	single := []time.Duration{5 * time.Millisecond}
	result = percentile(single, 0.99)
	if result != 5*time.Millisecond {
		t.Errorf("expected 5ms for single element, got %s", result)
	}
}

func TestStddevEdgeCases(t *testing.T) {
	// Single element should return 0
	single := []time.Duration{5 * time.Millisecond}
	result := stddev(single, 5*time.Millisecond)
	if result != 0 {
		t.Errorf("expected 0 stddev for single element, got %s", result)
	}
}
