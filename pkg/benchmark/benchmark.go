// SPDX-License-Identifier: GPL-3.0-or-later

// Package benchmark provides a benchmarking framework for Unheaded performance testing.
// It wraps Go's standard testing patterns with structured result collection,
// statistical analysis, and JSON/human-readable reporting.
package benchmark

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// BenchmarkFunc is the function signature for a benchmark test.
// It receives the iteration count and should perform the operation that many times.
// Return an error to signal the benchmark failed.
type BenchmarkFunc func(iterations int) error

// BenchmarkResult holds the statistical results of a single benchmark.
type BenchmarkResult struct {
	Name       string        `json:"name"`
	Iterations int           `json:"iterations"`
	Min        time.Duration `json:"min_ns"`
	Max        time.Duration `json:"max_ns"`
	Avg        time.Duration `json:"avg_ns"`
	Median     time.Duration `json:"median_ns"`
	P95        time.Duration `json:"p95_ns"`
	P99        time.Duration `json:"p99_ns"`
	StdDev     time.Duration `json:"stddev_ns"`
	TotalTime  time.Duration `json:"total_ns"`
	Error      string        `json:"error,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

// test is an internal representation of a registered benchmark.
type test struct {
	name string
	fn   BenchmarkFunc
}

// BenchmarkSuite holds a collection of benchmark tests and their configuration.
type BenchmarkSuite struct {
	mu         sync.Mutex
	tests      []test
	results    []BenchmarkResult
	iterations int
	warmup     int
}

// NewSuite creates a new BenchmarkSuite with sensible defaults.
func NewSuite() *BenchmarkSuite {
	return &BenchmarkSuite{
		iterations: 100,
		warmup:     5,
	}
}

// SetIterations sets the number of iterations per benchmark test.
func (s *BenchmarkSuite) SetIterations(n int) {
	if n < 1 {
		n = 1
	}
	s.iterations = n
}

// SetWarmup sets the number of warmup iterations run before measurement.
func (s *BenchmarkSuite) SetWarmup(n int) {
	if n < 0 {
		n = 0
	}
	s.warmup = n
}

// AddTest registers a named benchmark function with the suite.
func (s *BenchmarkSuite) AddTest(name string, fn BenchmarkFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tests = append(s.tests, test{name: name, fn: fn})
}

// Run executes all registered benchmarks and returns the results.
func (s *BenchmarkSuite) Run() []BenchmarkResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.results = make([]BenchmarkResult, 0, len(s.tests))

	for _, t := range s.tests {
		result := s.runSingle(t)
		s.results = append(s.results, result)
	}

	return s.results
}

// Results returns the results from the last Run() call.
func (s *BenchmarkSuite) Results() []BenchmarkResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.results
}

func (s *BenchmarkSuite) runSingle(t test) BenchmarkResult {
	result := BenchmarkResult{
		Name:       t.name,
		Iterations: s.iterations,
		Timestamp:  time.Now(),
	}

	// Warmup phase — run but discard timings
	for i := 0; i < s.warmup; i++ {
		_ = t.fn(1)
	}

	// Measurement phase — time each iteration individually
	durations := make([]time.Duration, 0, s.iterations)
	for i := 0; i < s.iterations; i++ {
		start := time.Now()
		err := t.fn(1)
		elapsed := time.Since(start)

		if err != nil {
			result.Error = err.Error()
			return result
		}
		durations = append(durations, elapsed)
	}

	// Compute statistics
	computeStats(&result, durations)
	return result
}

func computeStats(r *BenchmarkResult, durations []time.Duration) {
	n := len(durations)
	if n == 0 {
		return
	}

	// Sort for percentile computation
	sorted := make([]time.Duration, n)
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	// Min / Max
	r.Min = sorted[0]
	r.Max = sorted[n-1]

	// Sum for average
	var total time.Duration
	for _, d := range sorted {
		total += d
	}
	r.TotalTime = total
	r.Avg = total / time.Duration(n)

	// Median
	if n%2 == 0 {
		r.Median = (sorted[n/2-1] + sorted[n/2]) / 2
	} else {
		r.Median = sorted[n/2]
	}

	// Percentiles
	r.P95 = percentile(sorted, 0.95)
	r.P99 = percentile(sorted, 0.99)

	// Standard deviation
	r.StdDev = stddev(sorted, r.Avg)
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	idx := p * float64(n-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return time.Duration(float64(sorted[lower])*(1-frac) + float64(sorted[upper])*frac)
}

func stddev(sorted []time.Duration, avg time.Duration) time.Duration {
	n := len(sorted)
	if n < 2 {
		return 0
	}
	var sumSq float64
	avgF := float64(avg)
	for _, d := range sorted {
		diff := float64(d) - avgF
		sumSq += diff * diff
	}
	return time.Duration(math.Sqrt(sumSq / float64(n-1)))
}

// SummaryReport returns a human-readable summary of the benchmark results.
func (s *BenchmarkSuite) SummaryReport() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.results) == 0 {
		return "No benchmark results available. Run the suite first."
	}

	var b strings.Builder
	b.WriteString("==========================================================\n")
	b.WriteString(" Unheaded Performance Benchmark Report\n")
	fmt.Fprintf(&b, " Generated: %s\n", time.Now().Format(time.RFC3339))
	b.WriteString("==========================================================\n\n")

	for _, r := range s.results {
		fmt.Fprintf(&b, "--- %s ---\n", r.Name)
		if r.Error != "" {
			fmt.Fprintf(&b, "  ERROR: %s\n\n", r.Error)
			continue
		}
		fmt.Fprintf(&b, "  Iterations: %d\n", r.Iterations)
		fmt.Fprintf(&b, "  Min:        %s\n", r.Min)
		fmt.Fprintf(&b, "  Max:        %s\n", r.Max)
		fmt.Fprintf(&b, "  Avg:        %s\n", r.Avg)
		fmt.Fprintf(&b, "  Median:     %s\n", r.Median)
		fmt.Fprintf(&b, "  P95:        %s\n", r.P95)
		fmt.Fprintf(&b, "  P99:        %s\n", r.P99)
		fmt.Fprintf(&b, "  StdDev:     %s\n", r.StdDev)
		fmt.Fprintf(&b, "  Total:      %s\n", r.TotalTime)
		b.WriteString("\n")
	}

	b.WriteString("==========================================================\n")
	return b.String()
}

// SaveJSON writes the benchmark results to a JSON file.
func (s *BenchmarkSuite) SaveJSON(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.results) == 0 {
		return fmt.Errorf("no results to save — run the suite first")
	}

	report := struct {
		Version   string            `json:"version"`
		Timestamp time.Time         `json:"timestamp"`
		Results   []BenchmarkResult `json:"results"`
	}{
		Version:   "1.0.0",
		Timestamp: time.Now(),
		Results:   s.results,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal results: %w", err)
	}

	if err := os.WriteFile(filename, data, 0640); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}

	return nil
}
