package testing

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkResult holds the results of a benchmark run.
type BenchmarkResult struct {
	Name       string
	N          int
	Duration   time.Duration
	Bytes      int64
	Allocs     uint64
	AllocBytes uint64
}

// NsPerOp returns nanoseconds per operation.
func (r BenchmarkResult) NsPerOp() int64 {
	if r.N == 0 {
		return 0
	}
	return r.Duration.Nanoseconds() / int64(r.N)
}

// OpsPerSec returns operations per second.
func (r BenchmarkResult) OpsPerSec() float64 {
	if r.Duration == 0 {
		return 0
	}
	return float64(r.N) / r.Duration.Seconds()
}

// AllocsPerOp returns allocations per operation.
func (r BenchmarkResult) AllocsPerOp() uint64 {
	if r.N == 0 {
		return 0
	}
	return r.Allocs / uint64(r.N)
}

// BytesPerOp returns bytes allocated per operation.
func (r BenchmarkResult) BytesPerOp() uint64 {
	if r.N == 0 {
		return 0
	}
	return r.AllocBytes / uint64(r.N)
}

// String returns a formatted string representation.
func (r BenchmarkResult) String() string {
	return fmt.Sprintf("%s\t%d ops\t%d ns/op\t%.2f ops/s\t%d allocs/op\t%d B/op",
		r.Name, r.N, r.NsPerOp(), r.OpsPerSec(), r.AllocsPerOp(), r.BytesPerOp())
}

// Benchmarker provides enhanced benchmarking capabilities.
type Benchmarker struct {
	b           *testing.B
	setupFn     func()
	teardownFn  func()
	parallelism int
}

// NewBenchmarker creates a new benchmarker.
func NewBenchmarker(b *testing.B) *Benchmarker {
	return &Benchmarker{b: b, parallelism: runtime.GOMAXPROCS(0)}
}

// Setup sets a function to run before each benchmark iteration.
func (bm *Benchmarker) Setup(fn func()) *Benchmarker {
	bm.setupFn = fn
	return bm
}

// Teardown sets a function to run after each benchmark iteration.
func (bm *Benchmarker) Teardown(fn func()) *Benchmarker {
	bm.teardownFn = fn
	return bm
}

// Parallelism sets the parallelism level for parallel benchmarks.
func (bm *Benchmarker) Parallelism(p int) *Benchmarker {
	bm.parallelism = p
	return bm
}

// Run runs the benchmark function.
func (bm *Benchmarker) Run(fn func()) {
	bm.b.Helper()

	if bm.setupFn != nil {
		bm.setupFn()
	}

	bm.b.ResetTimer()

	for i := 0; i < bm.b.N; i++ {
		fn()
	}

	bm.b.StopTimer()

	if bm.teardownFn != nil {
		bm.teardownFn()
	}
}

// RunParallel runs the benchmark function in parallel.
func (bm *Benchmarker) RunParallel(fn func()) {
	bm.b.Helper()

	if bm.setupFn != nil {
		bm.setupFn()
	}

	bm.b.SetParallelism(bm.parallelism)
	bm.b.ResetTimer()

	bm.b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			fn()
		}
	})

	bm.b.StopTimer()

	if bm.teardownFn != nil {
		bm.teardownFn()
	}
}

// BenchmarkSuite allows running multiple benchmarks as a suite.
type BenchmarkSuite struct {
	b          *testing.B
	benchmarks []namedBenchmark
	setupFn    func()
	teardownFn func()
}

type namedBenchmark struct {
	name string
	fn   func(*testing.B)
}

// NewBenchmarkSuite creates a new benchmark suite.
func NewBenchmarkSuite(b *testing.B) *BenchmarkSuite {
	return &BenchmarkSuite{b: b}
}

// Setup sets a function to run once before all benchmarks.
func (s *BenchmarkSuite) Setup(fn func()) *BenchmarkSuite {
	s.setupFn = fn
	return s
}

// Teardown sets a function to run once after all benchmarks.
func (s *BenchmarkSuite) Teardown(fn func()) *BenchmarkSuite {
	s.teardownFn = fn
	return s
}

// Add adds a benchmark to the suite.
func (s *BenchmarkSuite) Add(name string, fn func(*testing.B)) *BenchmarkSuite {
	s.benchmarks = append(s.benchmarks, namedBenchmark{name: name, fn: fn})
	return s
}

// Run runs all benchmarks in the suite.
func (s *BenchmarkSuite) Run() {
	if s.setupFn != nil {
		s.setupFn()
	}

	defer func() {
		if s.teardownFn != nil {
			s.teardownFn()
		}
	}()

	for _, bm := range s.benchmarks {
		s.b.Run(bm.name, bm.fn)
	}
}

// BenchmarkComparison compares benchmark results.
type BenchmarkComparison struct {
	results map[string][]BenchmarkResult
	mu      sync.Mutex
}

// NewBenchmarkComparison creates a new benchmark comparison.
func NewBenchmarkComparison() *BenchmarkComparison {
	return &BenchmarkComparison{
		results: make(map[string][]BenchmarkResult),
	}
}

// Add adds a benchmark result.
func (c *BenchmarkComparison) Add(result BenchmarkResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results[result.Name] = append(c.results[result.Name], result)
}

// Compare compares two benchmark results and returns the speedup factor.
func (c *BenchmarkComparison) Compare(baseline, comparison string) (float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	baseResults, ok := c.results[baseline]
	if !ok || len(baseResults) == 0 {
		return 0, fmt.Errorf("no results for baseline %q", baseline)
	}

	compResults, ok := c.results[comparison]
	if !ok || len(compResults) == 0 {
		return 0, fmt.Errorf("no results for comparison %q", comparison)
	}

	// Use average ns/op
	baseAvg := averageNsPerOp(baseResults)
	compAvg := averageNsPerOp(compResults)

	if compAvg == 0 {
		return 0, fmt.Errorf("comparison average is zero")
	}

	return float64(baseAvg) / float64(compAvg), nil
}

func averageNsPerOp(results []BenchmarkResult) int64 {
	if len(results) == 0 {
		return 0
	}
	var total int64
	for _, r := range results {
		total += r.NsPerOp()
	}
	return total / int64(len(results))
}

// Summary returns a summary of all benchmark results.
func (c *BenchmarkComparison) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var sb strBuilder
	sb.WriteString("Benchmark Summary:\n")
	sb.WriteString("==================\n")

	names := make([]string, 0, len(c.results))
	for name := range c.results {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		results := c.results[name]
		avgNs := averageNsPerOp(results)
		sb.WriteString(fmt.Sprintf("%s: %d ns/op (avg of %d runs)\n", name, avgNs, len(results)))
	}

	return sb.String()
}

type strBuilder struct {
	data []byte
}

func (b *strBuilder) WriteString(s string) (int, error) {
	b.data = append(b.data, s...)
	return len(s), nil
}

func (b *strBuilder) String() string {
	return string(b.data)
}

// MemoryBenchmark provides memory allocation benchmarking.
type MemoryBenchmark struct {
	name   string
	before runtime.MemStats
	after  runtime.MemStats
}

// NewMemoryBenchmark creates a new memory benchmark.
func NewMemoryBenchmark(name string) *MemoryBenchmark {
	return &MemoryBenchmark{name: name}
}

// Start records the initial memory state.
func (m *MemoryBenchmark) Start() {
	runtime.GC()
	runtime.ReadMemStats(&m.before)
}

// Stop records the final memory state.
func (m *MemoryBenchmark) Stop() {
	runtime.ReadMemStats(&m.after)
}

// Allocs returns the number of allocations.
func (m *MemoryBenchmark) Allocs() uint64 {
	return m.after.Mallocs - m.before.Mallocs
}

// AllocBytes returns the number of bytes allocated.
func (m *MemoryBenchmark) AllocBytes() uint64 {
	return m.after.TotalAlloc - m.before.TotalAlloc
}

// HeapAlloc returns the heap allocation difference.
func (m *MemoryBenchmark) HeapAlloc() int64 {
	return int64(m.after.HeapAlloc) - int64(m.before.HeapAlloc)
}

// String returns a formatted string representation.
func (m *MemoryBenchmark) String() string {
	return fmt.Sprintf("%s: %d allocs, %d bytes allocated, %d heap delta",
		m.name, m.Allocs(), m.AllocBytes(), m.HeapAlloc())
}

// LatencyHistogram tracks latency distribution.
type LatencyHistogram struct {
	buckets []int64
	counts  []uint64
	min     int64
	max     int64
	sum     int64
	count   uint64
	mu      sync.Mutex
}

// NewLatencyHistogram creates a new latency histogram with buckets in microseconds.
func NewLatencyHistogram(buckets []int64) *LatencyHistogram {
	return &LatencyHistogram{
		buckets: buckets,
		counts:  make([]uint64, len(buckets)+1),
		min:     1<<63 - 1,
	}
}

// Record records a latency measurement in nanoseconds.
func (h *LatencyHistogram) Record(latencyNs int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	latencyUs := latencyNs / 1000 // Convert to microseconds

	h.count++
	h.sum += latencyUs

	if latencyUs < h.min {
		h.min = latencyUs
	}
	if latencyUs > h.max {
		h.max = latencyUs
	}

	// Find bucket
	for i, bucket := range h.buckets {
		if latencyUs <= bucket {
			h.counts[i]++
			return
		}
	}
	h.counts[len(h.buckets)]++ // Overflow bucket
}

// Percentile returns the approximate percentile value.
func (h *LatencyHistogram) Percentile(p float64) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.count == 0 {
		return 0
	}

	target := uint64(float64(h.count) * p / 100.0)
	var cumulative uint64

	for i, count := range h.counts {
		cumulative += count
		if cumulative >= target {
			if i < len(h.buckets) {
				return h.buckets[i]
			}
			return h.max
		}
	}

	return h.max
}

// Min returns the minimum latency in microseconds.
func (h *LatencyHistogram) Min() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.count == 0 {
		return 0
	}
	return h.min
}

// Max returns the maximum latency in microseconds.
func (h *LatencyHistogram) Max() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.max
}

// Mean returns the mean latency in microseconds.
func (h *LatencyHistogram) Mean() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.count == 0 {
		return 0
	}
	return float64(h.sum) / float64(h.count)
}

// Count returns the number of samples.
func (h *LatencyHistogram) Count() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

// String returns a formatted string representation.
func (h *LatencyHistogram) String() string {
	return fmt.Sprintf("count=%d min=%dus max=%dus mean=%.2fus p50=%dus p95=%dus p99=%dus",
		h.Count(), h.Min(), h.Max(), h.Mean(),
		h.Percentile(50), h.Percentile(95), h.Percentile(99))
}

// ThroughputMeter measures throughput over time.
type ThroughputMeter struct {
	ops      int64
	bytes    int64
	start    time.Time
	duration time.Duration
}

// NewThroughputMeter creates a new throughput meter.
func NewThroughputMeter() *ThroughputMeter {
	return &ThroughputMeter{}
}

// Start starts the measurement.
func (m *ThroughputMeter) Start() {
	m.start = time.Now()
}

// Stop stops the measurement.
func (m *ThroughputMeter) Stop() {
	m.duration = time.Since(m.start)
}

// RecordOp records an operation.
func (m *ThroughputMeter) RecordOp() {
	atomic.AddInt64(&m.ops, 1)
}

// RecordOps records multiple operations.
func (m *ThroughputMeter) RecordOps(n int64) {
	atomic.AddInt64(&m.ops, n)
}

// RecordBytes records bytes processed.
func (m *ThroughputMeter) RecordBytes(n int64) {
	atomic.AddInt64(&m.bytes, n)
}

// OpsPerSecond returns operations per second.
func (m *ThroughputMeter) OpsPerSecond() float64 {
	if m.duration == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&m.ops)) / m.duration.Seconds()
}

// BytesPerSecond returns bytes per second.
func (m *ThroughputMeter) BytesPerSecond() float64 {
	if m.duration == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&m.bytes)) / m.duration.Seconds()
}

// String returns a formatted string representation.
func (m *ThroughputMeter) String() string {
	return fmt.Sprintf("%.2f ops/s, %.2f MB/s",
		m.OpsPerSecond(), m.BytesPerSecond()/1024/1024)
}

// ConcurrencyBenchmark runs benchmarks at different concurrency levels.
type ConcurrencyBenchmark struct {
	b      *testing.B
	levels []int
}

// NewConcurrencyBenchmark creates a new concurrency benchmark.
func NewConcurrencyBenchmark(b *testing.B) *ConcurrencyBenchmark {
	return &ConcurrencyBenchmark{
		b:      b,
		levels: []int{1, 2, 4, 8, 16, 32},
	}
}

// Levels sets the concurrency levels to test.
func (cb *ConcurrencyBenchmark) Levels(levels ...int) *ConcurrencyBenchmark {
	cb.levels = levels
	return cb
}

// Run runs the benchmark at each concurrency level.
func (cb *ConcurrencyBenchmark) Run(fn func()) {
	for _, level := range cb.levels {
		cb.b.Run(fmt.Sprintf("concurrency-%d", level), func(b *testing.B) {
			b.SetParallelism(level)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					fn()
				}
			})
		})
	}
}

// ScalabilityBenchmark measures scalability with increasing data sizes.
type ScalabilityBenchmark struct {
	b     *testing.B
	sizes []int
}

// NewScalabilityBenchmark creates a new scalability benchmark.
func NewScalabilityBenchmark(b *testing.B) *ScalabilityBenchmark {
	return &ScalabilityBenchmark{
		b:     b,
		sizes: []int{100, 1000, 10000, 100000},
	}
}

// Sizes sets the data sizes to test.
func (sb *ScalabilityBenchmark) Sizes(sizes ...int) *ScalabilityBenchmark {
	sb.sizes = sizes
	return sb
}

// Run runs the benchmark at each data size.
func (sb *ScalabilityBenchmark) Run(fn func(size int)) {
	for _, size := range sb.sizes {
		sb.b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				fn(size)
			}
		})
	}
}
