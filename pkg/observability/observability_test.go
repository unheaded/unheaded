package observability

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAllBackends(t *testing.T) {
	backends := AllBackends()
	if len(backends) != 8 {
		t.Fatalf("expected 8 backends, got %d", len(backends))
	}
}

func TestPrometheusAdapter_EmitMetric(t *testing.T) {
	a := NewPrometheusAdapter()
	ctx := context.Background()

	err := a.EmitMetric(ctx, Metric{
		Name:   "http_requests_total",
		Value:  42,
		Labels: map[string]string{"service": "timeguru", "method": "GET"},
		Type:   MetricCounter,
	})
	if err != nil {
		t.Fatalf("EmitMetric: %v", err)
	}

	if a.MetricCount() != 1 {
		t.Errorf("expected 1 metric, got %d", a.MetricCount())
	}

	expo := a.Exposition()
	if !strings.Contains(expo, "http_requests_total") {
		t.Error("exposition should contain metric name")
	}
	if !strings.Contains(expo, "42") {
		t.Error("exposition should contain value")
	}
	if !strings.Contains(expo, `service="timeguru"`) {
		t.Error("exposition should contain labels")
	}
}

func TestPrometheusAdapter_Counter_Accumulates(t *testing.T) {
	a := NewPrometheusAdapter()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a.EmitMetric(ctx, Metric{
			Name:   "requests",
			Value:  1,
			Labels: map[string]string{"service": "test"},
			Type:   MetricCounter,
		})
	}

	expo := a.Exposition()
	if !strings.Contains(expo, "5") {
		t.Errorf("counter should accumulate to 5, got: %s", expo)
	}
}

func TestPrometheusAdapter_Gauge_LastValue(t *testing.T) {
	a := NewPrometheusAdapter()
	ctx := context.Background()

	a.EmitMetric(ctx, Metric{Name: "cpu", Value: 50, Type: MetricGauge})
	a.EmitMetric(ctx, Metric{Name: "cpu", Value: 75, Type: MetricGauge})

	expo := a.Exposition()
	if strings.Contains(expo, "50") {
		t.Error("gauge should only show last value, not 50")
	}
	if !strings.Contains(expo, "75") {
		t.Error("gauge should show last value 75")
	}
}

func TestPrometheusAdapter_Closed(t *testing.T) {
	a := NewPrometheusAdapter()
	a.Close()

	err := a.EmitMetric(context.Background(), Metric{Name: "test", Value: 1})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed, got %v", err)
	}
}

func TestPrometheusAdapter_UnsupportedSignals(t *testing.T) {
	a := NewPrometheusAdapter()
	ctx := context.Background()

	// These should be no-ops, not errors
	if err := a.EmitLog(ctx, LogEntry{}); err != nil {
		t.Errorf("EmitLog should be no-op: %v", err)
	}
	if err := a.EmitTrace(ctx, Span{}); err != nil {
		t.Errorf("EmitTrace should be no-op: %v", err)
	}
	if err := a.EmitAlert(ctx, Alert{}); err != nil {
		t.Errorf("EmitAlert should be no-op: %v", err)
	}
}

func TestZerologAdapter_EmitLog(t *testing.T) {
	a := NewZerologAdapter(100)
	ctx := context.Background()

	err := a.EmitLog(ctx, LogEntry{
		Level:   "info",
		Message: "request processed",
		Service: "timeguru",
		TraceID: "abc123",
		Fields:  map[string]string{"method": "GET", "path": "/api/v1/timeline"},
	})
	if err != nil {
		t.Fatalf("EmitLog: %v", err)
	}

	if a.EntryCount() != 1 {
		t.Errorf("expected 1 entry, got %d", a.EntryCount())
	}

	entries := a.Entries()
	if entries[0].Message != "request processed" {
		t.Errorf("wrong message: %s", entries[0].Message)
	}
	if entries[0].TraceID != "abc123" {
		t.Errorf("wrong trace_id: %s", entries[0].TraceID)
	}
}

func TestZerologAdapter_RingBuffer(t *testing.T) {
	a := NewZerologAdapter(3)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a.EmitLog(ctx, LogEntry{Message: strings.Repeat("x", i+1)})
	}

	if a.EntryCount() != 3 {
		t.Errorf("expected 3 entries (ring buffer), got %d", a.EntryCount())
	}

	entries := a.Entries()
	// Should have entries 3, 4, 5 (oldest dropped)
	if entries[0].Message != "xxx" {
		t.Errorf("expected 'xxx' as oldest retained, got %q", entries[0].Message)
	}
}

func TestZerologAdapter_Closed(t *testing.T) {
	a := NewZerologAdapter(10)
	a.Close()

	err := a.EmitLog(context.Background(), LogEntry{Message: "test"})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed, got %v", err)
	}
}

func TestPipeline_FanOut(t *testing.T) {
	prom := NewPrometheusAdapter()
	logs := NewZerologAdapter(100)

	p := NewPipeline(prom, logs)
	ctx := context.Background()

	// Metric goes to prometheus only
	p.EmitMetric(ctx, Metric{Name: "test", Value: 1, Type: MetricCounter})
	if prom.MetricCount() != 1 {
		t.Error("prometheus should have 1 metric")
	}
	if logs.EntryCount() != 0 {
		t.Error("zerolog should have 0 entries (metrics not supported)")
	}

	// Log goes to zerolog only
	p.EmitLog(ctx, LogEntry{Message: "hello"})
	if logs.EntryCount() != 1 {
		t.Error("zerolog should have 1 entry")
	}
	if prom.MetricCount() != 1 {
		t.Error("prometheus should still have 1 metric")
	}
}

func TestPipeline_Close(t *testing.T) {
	prom := NewPrometheusAdapter()
	logs := NewZerologAdapter(10)

	p := NewPipeline(prom, logs)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err := p.EmitMetric(context.Background(), Metric{Name: "test", Value: 1})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed after Close, got %v", err)
	}

	err = p.EmitLog(context.Background(), LogEntry{Message: "test"})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed after Close, got %v", err)
	}
}

func TestPipeline_AddAdapter(t *testing.T) {
	p := NewPipeline()
	if len(p.Adapters()) != 0 {
		t.Error("expected 0 adapters initially")
	}

	prom := NewPrometheusAdapter()
	p.AddAdapter(prom)
	if len(p.Adapters()) != 1 {
		t.Error("expected 1 adapter after add")
	}

	ctx := context.Background()
	p.EmitMetric(ctx, Metric{Name: "test", Value: 1, Type: MetricGauge})
	if prom.MetricCount() != 1 {
		t.Error("added adapter should receive metrics")
	}
}

func TestPipeline_TimestampDefaults(t *testing.T) {
	logs := NewZerologAdapter(10)
	p := NewPipeline(logs)
	ctx := context.Background()

	before := time.Now()
	p.EmitLog(ctx, LogEntry{Message: "test"})
	after := time.Now()

	entries := logs.Entries()
	if entries[0].Timestamp.Before(before) || entries[0].Timestamp.After(after) {
		t.Error("timestamp should be set to now when zero")
	}
}

func TestPipeline_ConcurrentSafety(t *testing.T) {
	prom := NewPrometheusAdapter()
	logs := NewZerologAdapter(0)
	p := NewPipeline(prom, logs)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.EmitMetric(ctx, Metric{Name: "concurrent", Value: 1, Type: MetricCounter})
			p.EmitLog(ctx, LogEntry{Message: "concurrent"})
		}()
	}
	wg.Wait()

	if logs.EntryCount() != 100 {
		t.Errorf("expected 100 log entries, got %d", logs.EntryCount())
	}
}

func TestPipeline_EmitTrace(t *testing.T) {
	// No adapter supports traces yet, but pipeline should not error
	p := NewPipeline(NewPrometheusAdapter())
	ctx := context.Background()

	err := p.EmitTrace(ctx, Span{
		TraceID:   "trace-123",
		SpanID:    "span-456",
		Operation: "HTTP GET /api/v1/timeline",
		Service:   "timeguru",
		StartTime: time.Now(),
		Duration:  5 * time.Millisecond,
		Status:    SpanOK,
	})
	if err != nil {
		t.Errorf("EmitTrace should succeed (no adapter handles it): %v", err)
	}
}

func TestPipeline_EmitAlert(t *testing.T) {
	p := NewPipeline(NewPrometheusAdapter())
	ctx := context.Background()

	err := p.EmitAlert(ctx, Alert{
		Name:     "high_latency",
		Severity: SeverityWarning,
		Message:  "P99 latency > 100ms",
		Service:  "gateway",
	})
	if err != nil {
		t.Errorf("EmitAlert should succeed: %v", err)
	}
}
