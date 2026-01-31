package testing

import (
	"testing"
	"time"
)

func TestBenchmarkResult(t *testing.T) {
	result := BenchmarkResult{
		Name:       "test",
		N:          1000,
		Duration:   time.Second,
		Bytes:      100000,
		Allocs:     5000,
		AllocBytes: 50000,
	}

	// NsPerOp: 1 second / 1000 ops = 1,000,000 ns/op
	if result.NsPerOp() != 1000000 {
		t.Errorf("Expected 1000000 ns/op, got %d", result.NsPerOp())
	}

	// OpsPerSec: 1000 ops / 1 second = 1000 ops/s
	if result.OpsPerSec() != 1000 {
		t.Errorf("Expected 1000 ops/s, got %f", result.OpsPerSec())
	}

	// AllocsPerOp: 5000 / 1000 = 5
	if result.AllocsPerOp() != 5 {
		t.Errorf("Expected 5 allocs/op, got %d", result.AllocsPerOp())
	}

	// BytesPerOp: 50000 / 1000 = 50
	if result.BytesPerOp() != 50 {
		t.Errorf("Expected 50 bytes/op, got %d", result.BytesPerOp())
	}
}

func TestBenchmarkResultZero(t *testing.T) {
	result := BenchmarkResult{N: 0}

	if result.NsPerOp() != 0 {
		t.Error("Expected 0 ns/op for N=0")
	}
	if result.OpsPerSec() != 0 {
		t.Error("Expected 0 ops/s for duration=0")
	}
	if result.AllocsPerOp() != 0 {
		t.Error("Expected 0 allocs/op for N=0")
	}
	if result.BytesPerOp() != 0 {
		t.Error("Expected 0 bytes/op for N=0")
	}
}

func TestBenchmarkResultString(t *testing.T) {
	result := BenchmarkResult{
		Name:       "test",
		N:          1000,
		Duration:   time.Second,
		Allocs:     5000,
		AllocBytes: 50000,
	}

	str := result.String()
	if str == "" {
		t.Error("Expected non-empty string")
	}
}

func TestBenchmarkComparison(t *testing.T) {
	comp := NewBenchmarkComparison()

	// Add baseline results
	comp.Add(BenchmarkResult{Name: "baseline", N: 1000, Duration: time.Second})
	comp.Add(BenchmarkResult{Name: "baseline", N: 1000, Duration: time.Second})

	// Add comparison results (2x faster)
	comp.Add(BenchmarkResult{Name: "optimized", N: 1000, Duration: 500 * time.Millisecond})
	comp.Add(BenchmarkResult{Name: "optimized", N: 1000, Duration: 500 * time.Millisecond})

	speedup, err := comp.Compare("baseline", "optimized")
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	// Baseline: 1000000 ns/op, Optimized: 500000 ns/op
	// Speedup should be 2x
	if speedup < 1.9 || speedup > 2.1 {
		t.Errorf("Expected speedup around 2x, got %f", speedup)
	}
}

func TestBenchmarkComparisonSummary(t *testing.T) {
	comp := NewBenchmarkComparison()
	comp.Add(BenchmarkResult{Name: "test1", N: 1000, Duration: time.Second})
	comp.Add(BenchmarkResult{Name: "test2", N: 1000, Duration: 2 * time.Second})

	summary := comp.Summary()
	if summary == "" {
		t.Error("Expected non-empty summary")
	}
}

func TestMemoryBenchmark(t *testing.T) {
	mb := NewMemoryBenchmark("test")
	mb.Start()

	// Allocate some memory
	data := make([]byte, 1024*1024) // 1MB
	_ = data

	mb.Stop()

	// Should have recorded some allocations
	if mb.Allocs() == 0 {
		t.Log("Warning: No allocations recorded (may be optimized away)")
	}

	str := mb.String()
	if str == "" {
		t.Error("Expected non-empty string")
	}
}

func TestLatencyHistogram(t *testing.T) {
	// Buckets in microseconds: 100us, 500us, 1ms, 5ms, 10ms
	hist := NewLatencyHistogram([]int64{100, 500, 1000, 5000, 10000})

	// Record some latencies in nanoseconds
	hist.Record(50 * 1000)   // 50us -> bucket 0 (<=100us)
	hist.Record(200 * 1000)  // 200us -> bucket 1 (<=500us)
	hist.Record(800 * 1000)  // 800us -> bucket 2 (<=1000us)
	hist.Record(3000 * 1000) // 3ms -> bucket 3 (<=5000us)
	hist.Record(8000 * 1000) // 8ms -> bucket 4 (<=10000us)

	if hist.Count() != 5 {
		t.Errorf("Expected 5 samples, got %d", hist.Count())
	}

	if hist.Min() != 50 {
		t.Errorf("Expected min 50us, got %d", hist.Min())
	}

	if hist.Max() != 8000 {
		t.Errorf("Expected max 8000us, got %d", hist.Max())
	}

	// Mean should be (50 + 200 + 800 + 3000 + 8000) / 5 = 2410
	mean := hist.Mean()
	if mean < 2400 || mean > 2420 {
		t.Errorf("Expected mean around 2410us, got %f", mean)
	}

	str := hist.String()
	if str == "" {
		t.Error("Expected non-empty string")
	}
}

func TestLatencyHistogramPercentile(t *testing.T) {
	hist := NewLatencyHistogram([]int64{100, 200, 300, 400, 500})

	// Record 100 samples evenly distributed
	for i := 1; i <= 100; i++ {
		hist.Record(int64(i) * 5 * 1000) // 5us to 500us
	}

	// p50 should be around 250us
	p50 := hist.Percentile(50)
	if p50 < 200 || p50 > 300 {
		t.Errorf("Expected p50 around 250us, got %d", p50)
	}

	// p99 should be high
	p99 := hist.Percentile(99)
	if p99 < 400 {
		t.Errorf("Expected p99 > 400us, got %d", p99)
	}
}

func TestThroughputMeter(t *testing.T) {
	meter := NewThroughputMeter()
	meter.Start()

	// Simulate some work
	for i := 0; i < 100; i++ {
		meter.RecordOp()
		meter.RecordBytes(1024)
	}

	meter.Stop()

	if meter.OpsPerSecond() <= 0 {
		t.Error("Expected positive ops/s")
	}

	if meter.BytesPerSecond() <= 0 {
		t.Error("Expected positive bytes/s")
	}

	str := meter.String()
	if str == "" {
		t.Error("Expected non-empty string")
	}
}

func TestThroughputMeterBatch(t *testing.T) {
	meter := NewThroughputMeter()
	meter.Start()
	meter.RecordOps(100)
	meter.RecordBytes(1024 * 100)
	meter.Stop()

	// Just verify no panic and some output
	if meter.OpsPerSecond() <= 0 {
		t.Error("Expected positive ops/s")
	}
}

// This test verifies the concurrency benchmark structure
func TestConcurrencyBenchmarkStructure(t *testing.T) {
	// Just verify the structure works
	cb := NewConcurrencyBenchmark(nil)
	cb.Levels(1, 2, 4)

	if len(cb.levels) != 3 {
		t.Errorf("Expected 3 levels, got %d", len(cb.levels))
	}
}

// This test verifies the scalability benchmark structure
func TestScalabilityBenchmarkStructure(t *testing.T) {
	sb := NewScalabilityBenchmark(nil)
	sb.Sizes(10, 100, 1000)

	if len(sb.sizes) != 3 {
		t.Errorf("Expected 3 sizes, got %d", len(sb.sizes))
	}
}

func BenchmarkExample(b *testing.B) {
	bm := NewBenchmarker(b)
	bm.Setup(func() {
		// Setup code
	})
	bm.Teardown(func() {
		// Teardown code
	})
	bm.Run(func() {
		// Work to benchmark
		sum := 0
		for i := 0; i < 100; i++ {
			sum += i
		}
		_ = sum
	})
}

func BenchmarkParallelExample(b *testing.B) {
	bm := NewBenchmarker(b)
	bm.Parallelism(4)
	bm.RunParallel(func() {
		sum := 0
		for i := 0; i < 100; i++ {
			sum += i
		}
		_ = sum
	})
}
