// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"bytes"
	"math"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCounter(t *testing.T) {
	c := NewCounter("test_counter", "A test counter", nil)

	if c.Value() != 0 {
		t.Errorf("expected initial value 0, got %f", c.Value())
	}

	c.Inc()
	if c.Value() != 1 {
		t.Errorf("expected value 1 after Inc, got %f", c.Value())
	}

	c.Add(5)
	if c.Value() != 6 {
		t.Errorf("expected value 6 after Add(5), got %f", c.Value())
	}
}

func TestCounterConcurrency(t *testing.T) {
	c := NewCounter("concurrent_counter", "A concurrent counter", nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()

	if c.Value() != 10000 {
		t.Errorf("expected value 10000, got %f", c.Value())
	}
}

func TestCounterVec(t *testing.T) {
	cv := NewCounterVec("http_requests", "HTTP request count", nil, []string{"method", "path"})

	cv.WithLabels(Labels{"method": "GET", "path": "/"}).Inc()
	cv.WithLabels(Labels{"method": "POST", "path": "/api"}).Add(5)
	cv.WithLabels(Labels{"method": "GET", "path": "/"}).Inc()

	if cv.WithLabels(Labels{"method": "GET", "path": "/"}).Value() != 2 {
		t.Errorf("expected GET / counter to be 2")
	}

	if cv.WithLabels(Labels{"method": "POST", "path": "/api"}).Value() != 5 {
		t.Errorf("expected POST /api counter to be 5")
	}
}

func TestGauge(t *testing.T) {
	g := NewGauge("test_gauge", "A test gauge", nil)

	if g.Value() != 0 {
		t.Errorf("expected initial value 0, got %f", g.Value())
	}

	g.Set(42)
	if g.Value() != 42 {
		t.Errorf("expected value 42 after Set, got %f", g.Value())
	}

	g.Inc()
	if g.Value() != 43 {
		t.Errorf("expected value 43 after Inc, got %f", g.Value())
	}

	g.Dec()
	if g.Value() != 42 {
		t.Errorf("expected value 42 after Dec, got %f", g.Value())
	}

	g.Add(8)
	if g.Value() != 50 {
		t.Errorf("expected value 50 after Add(8), got %f", g.Value())
	}

	g.Sub(10)
	if g.Value() != 40 {
		t.Errorf("expected value 40 after Sub(10), got %f", g.Value())
	}
}

func TestGaugeConcurrency(t *testing.T) {
	g := NewGauge("concurrent_gauge", "A concurrent gauge", nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				g.Inc()
			}
		}()
	}
	wg.Wait()

	if g.Value() != 10000 {
		t.Errorf("expected value 10000, got %f", g.Value())
	}
}

func TestGaugeVec(t *testing.T) {
	gv := NewGaugeVec("active_connections", "Active connections", nil, []string{"pool"})

	gv.WithLabels(Labels{"pool": "main"}).Set(10)
	gv.WithLabels(Labels{"pool": "backup"}).Set(5)

	if gv.WithLabels(Labels{"pool": "main"}).Value() != 10 {
		t.Errorf("expected main pool gauge to be 10")
	}

	if gv.WithLabels(Labels{"pool": "backup"}).Value() != 5 {
		t.Errorf("expected backup pool gauge to be 5")
	}
}

func TestHistogram(t *testing.T) {
	h := NewHistogram(HistogramOpts{
		Name:    "request_duration",
		Help:    "Request duration in seconds",
		Buckets: []float64{0.1, 0.5, 1.0, 2.5, 5.0},
	})

	// Observe some values
	h.Observe(0.05)  // bucket: 0.1
	h.Observe(0.3)   // bucket: 0.5
	h.Observe(0.8)   // bucket: 1.0
	h.Observe(2.0)   // bucket: 2.5
	h.Observe(10.0)  // bucket: +Inf only

	var buf bytes.Buffer
	if err := h.Write(&buf); err != nil {
		t.Fatalf("failed to write histogram: %v", err)
	}

	output := buf.String()

	// Check that buckets are present
	if !strings.Contains(output, "request_duration_bucket{le=\"0.1\"} 1") {
		t.Errorf("expected bucket 0.1 to have count 1")
	}
	if !strings.Contains(output, "request_duration_bucket{le=\"0.5\"} 2") {
		t.Errorf("expected bucket 0.5 to have count 2")
	}
	if !strings.Contains(output, "request_duration_bucket{le=\"1\"} 3") {
		t.Errorf("expected bucket 1 to have count 3")
	}
	if !strings.Contains(output, "request_duration_bucket{le=\"2.5\"} 4") {
		t.Errorf("expected bucket 2.5 to have count 4")
	}
	if !strings.Contains(output, "request_duration_bucket{le=\"5\"} 4") {
		t.Errorf("expected bucket 5 to have count 4")
	}
	if !strings.Contains(output, "request_duration_bucket{le=\"+Inf\"} 5") {
		t.Errorf("expected bucket +Inf to have count 5")
	}
	if !strings.Contains(output, "request_duration_count 5") {
		t.Errorf("expected count to be 5")
	}
}

func TestHistogramConcurrency(t *testing.T) {
	h := NewHistogram(HistogramOpts{
		Name:    "concurrent_histogram",
		Help:    "A concurrent histogram",
		Buckets: DefaultBuckets,
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h.Observe(float64(n) * 0.001)
			}
		}(i)
	}
	wg.Wait()

	var buf bytes.Buffer
	if err := h.Write(&buf); err != nil {
		t.Fatalf("failed to write histogram: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "concurrent_histogram_count 10000") {
		t.Errorf("expected count to be 10000, output: %s", output)
	}
}

func TestLinearBuckets(t *testing.T) {
	buckets := LinearBuckets(0.1, 0.1, 5)
	expected := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	for i, v := range expected {
		if math.Abs(buckets[i]-v) > 0.0001 {
			t.Errorf("bucket %d: expected %f, got %f", i, v, buckets[i])
		}
	}
}

func TestExponentialBuckets(t *testing.T) {
	buckets := ExponentialBuckets(1, 2, 5)
	expected := []float64{1, 2, 4, 8, 16}

	for i, v := range expected {
		if math.Abs(buckets[i]-v) > 0.0001 {
			t.Errorf("bucket %d: expected %f, got %f", i, v, buckets[i])
		}
	}
}

func TestSummary(t *testing.T) {
	s := NewSummary(SummaryOpts{
		Name:       "response_size",
		Help:       "Response size in bytes",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		MaxAge:     time.Minute,
	})

	// Observe some values
	for i := 1; i <= 100; i++ {
		s.Observe(float64(i))
	}

	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatalf("failed to write summary: %v", err)
	}

	output := buf.String()

	// Check that quantiles are present
	if !strings.Contains(output, "response_size{quantile=\"0.5\"}") {
		t.Errorf("expected 0.5 quantile")
	}
	if !strings.Contains(output, "response_size{quantile=\"0.9\"}") {
		t.Errorf("expected 0.9 quantile")
	}
	if !strings.Contains(output, "response_size{quantile=\"0.99\"}") {
		t.Errorf("expected 0.99 quantile")
	}
	if !strings.Contains(output, "response_size_count 100") {
		t.Errorf("expected count to be 100")
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	counter := NewCounter("test_counter", "A test counter", nil)
	gauge := NewGauge("test_gauge", "A test gauge", nil)

	if err := r.Register(counter); err != nil {
		t.Fatalf("failed to register counter: %v", err)
	}

	if err := r.Register(gauge); err != nil {
		t.Fatalf("failed to register gauge: %v", err)
	}

	// Try to register duplicate
	if err := r.Register(counter); err == nil {
		t.Error("expected error when registering duplicate metric")
	}

	counter.Add(42)
	gauge.Set(100)

	var buf bytes.Buffer
	if err := r.Gather(&buf); err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "# HELP test_counter A test counter") {
		t.Errorf("expected counter help")
	}
	if !strings.Contains(output, "# TYPE test_counter counter") {
		t.Errorf("expected counter type")
	}
	if !strings.Contains(output, "test_counter 42") {
		t.Errorf("expected counter value")
	}

	if !strings.Contains(output, "# HELP test_gauge A test gauge") {
		t.Errorf("expected gauge help")
	}
	if !strings.Contains(output, "# TYPE test_gauge gauge") {
		t.Errorf("expected gauge type")
	}
	if !strings.Contains(output, "test_gauge 100") {
		t.Errorf("expected gauge value")
	}
}

func TestLabels(t *testing.T) {
	l1 := Labels{"method": "GET", "path": "/"}
	l2 := Labels{"status": "200"}

	merged := l1.Merge(l2)

	if len(merged) != 3 {
		t.Errorf("expected 3 labels, got %d", len(merged))
	}

	if merged["method"] != "GET" {
		t.Errorf("expected method=GET")
	}
	if merged["status"] != "200" {
		t.Errorf("expected status=200")
	}
}

func TestLabelsString(t *testing.T) {
	l := Labels{"b": "2", "a": "1", "c": "3"}
	s := l.String()

	// Should be sorted alphabetically
	if s != "a=1,b=2,c=3" {
		t.Errorf("expected 'a=1,b=2,c=3', got '%s'", s)
	}
}

func TestLabelEscaping(t *testing.T) {
	l := Labels{"msg": "hello\nworld", "quote": "say \"hi\""}

	formatted := formatLabels(l)

	if !strings.Contains(formatted, "\\n") {
		t.Errorf("expected newline to be escaped")
	}
	if !strings.Contains(formatted, "\\\"") {
		t.Errorf("expected quote to be escaped")
	}
}

func TestTimer(t *testing.T) {
	var observed float64
	timer := NewTimer(func(d float64) {
		observed = d
	})

	time.Sleep(10 * time.Millisecond)
	duration := timer.ObserveDuration()

	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}

	if observed < 0.01 {
		t.Errorf("expected observed >= 0.01, got %f", observed)
	}
}

func TestFormatFloat(t *testing.T) {
	cases := []struct {
		input    float64
		expected string
	}{
		{0, "0"},
		{42, "42"},
		{3.14159, "3.14159"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
	}

	for _, tc := range cases {
		result := formatFloat(tc.input)
		if result != tc.expected {
			t.Errorf("formatFloat(%v): expected %s, got %s", tc.input, tc.expected, result)
		}
	}
}

func TestBuildInfoGauge(t *testing.T) {
	info := BuildInfo{
		Version:   "1.0.0",
		Revision:  "abc123",
		Branch:    "main",
		BuildDate: "2024-01-15",
		GoVersion: "go1.21",
	}

	gauge := NewBuildInfoGauge("kingdom", info)

	if gauge.Value() != 1 {
		t.Errorf("expected build info gauge value to be 1")
	}

	var buf bytes.Buffer
	if err := gauge.Write(&buf); err != nil {
		t.Fatalf("failed to write gauge: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "kingdom_build_info") {
		t.Errorf("expected metric name to contain kingdom_build_info")
	}
	if !strings.Contains(output, "version=\"1.0.0\"") {
		t.Errorf("expected version label")
	}
}

func TestMetricFamily(t *testing.T) {
	mf := NewMetricFamily("http_requests", "HTTP requests", TypeCounter, nil)

	c1 := NewCounter("http_requests", "HTTP requests", Labels{"method": "GET"})
	c2 := NewCounter("http_requests", "HTTP requests", Labels{"method": "POST"})

	c1.Add(100)
	c2.Add(50)

	mf.Add(c1)
	mf.Add(c2)

	var buf bytes.Buffer
	if err := mf.Write(&buf); err != nil {
		t.Fatalf("failed to write metric family: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "http_requests") {
		t.Errorf("expected metric name in output")
	}
}

func TestStatusRecorder(t *testing.T) {
	// Create a mock ResponseWriter
	mock := &mockResponseWriter{
		headers: make(map[string][]string),
	}

	recorder := NewStatusRecorder(mock)

	if recorder.Status != 200 {
		t.Errorf("expected default status 200, got %d", recorder.Status)
	}

	recorder.WriteHeader(404)
	if recorder.Status != 404 {
		t.Errorf("expected status 404, got %d", recorder.Status)
	}
}

type mockResponseWriter struct {
	headers map[string][]string
	body    bytes.Buffer
	status  int
}

func (m *mockResponseWriter) Header() http.Header {
	return m.headers
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.status = statusCode
}

func TestUnregister(t *testing.T) {
	r := NewRegistry()

	counter := NewCounter("test_counter", "A test counter", nil)
	r.MustRegister(counter)

	if !r.Unregister(counter) {
		t.Error("expected unregister to return true")
	}

	if r.Unregister(counter) {
		t.Error("expected second unregister to return false")
	}

	// Should be able to register again
	if err := r.Register(counter); err != nil {
		t.Errorf("expected to be able to register after unregister: %v", err)
	}
}

func BenchmarkCounterInc(b *testing.B) {
	c := NewCounter("bench_counter", "Benchmark counter", nil)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Inc()
		}
	})
}

func BenchmarkGaugeSet(b *testing.B) {
	g := NewGauge("bench_gauge", "Benchmark gauge", nil)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			g.Set(42.0)
		}
	})
}

func BenchmarkHistogramObserve(b *testing.B) {
	h := NewHistogram(HistogramOpts{
		Name:    "bench_histogram",
		Help:    "Benchmark histogram",
		Buckets: DefaultBuckets,
	})
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0.0
		for pb.Next() {
			h.Observe(i)
			i += 0.001
		}
	})
}

func BenchmarkCounterVecWithLabels(b *testing.B) {
	cv := NewCounterVec("bench_counter_vec", "Benchmark counter vec", nil, []string{"method", "path"})
	labels := Labels{"method": "GET", "path": "/"}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cv.WithLabels(labels).Inc()
		}
	})
}

func BenchmarkRegistryGather(b *testing.B) {
	r := NewRegistry()
	for i := 0; i < 100; i++ {
		c := NewCounter(
			"counter_"+string(rune('a'+i%26))+string(rune('0'+i/26)),
			"A counter",
			nil,
		)
		c.Add(float64(i))
		r.MustRegister(c)
	}

	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		r.Gather(&buf)
	}
}
