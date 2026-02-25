// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package observability

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Grafana Adapter Tests ---

func TestGrafanaAdapter_Backend(t *testing.T) {
	a := NewGrafanaAdapter()
	if a.Backend() != BackendGrafana {
		t.Errorf("expected grafana, got %s", a.Backend())
	}
}

func TestGrafanaAdapter_Supports(t *testing.T) {
	a := NewGrafanaAdapter()
	sigs := a.Supports()
	if len(sigs) != 2 {
		t.Fatalf("expected 2 signal types, got %d", len(sigs))
	}
}

func TestGrafanaAdapter_EmitAlert_CreatesAnnotation(t *testing.T) {
	a := NewGrafanaAdapter()
	ctx := context.Background()

	now := time.Now()
	err := a.EmitAlert(ctx, Alert{
		Name:      "high_latency",
		Severity:  SeverityCritical,
		Message:   "P99 > 500ms",
		Service:   "timeguru",
		Timestamp: now,
	})
	if err != nil {
		t.Fatalf("EmitAlert: %v", err)
	}

	anns := a.Annotations()
	if len(anns) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(anns))
	}
	if !strings.Contains(anns[0].Text, "high_latency") {
		t.Error("annotation should contain alert name")
	}
	if !strings.Contains(anns[0].Text, "critical") {
		t.Error("annotation should contain severity")
	}
}

func TestGrafanaAdapter_GenerateServiceDashboard(t *testing.T) {
	a := NewGrafanaAdapter()
	dash := a.GenerateServiceDashboard("timeguru")

	if dash.UID != "unheaded-timeguru" {
		t.Errorf("expected uid unheaded-timeguru, got %s", dash.UID)
	}
	if len(dash.Panels) != 4 {
		t.Errorf("expected 4 panels, got %d", len(dash.Panels))
	}
	if a.DashboardCount() != 1 {
		t.Errorf("expected 1 dashboard, got %d", a.DashboardCount())
	}

	jsonStr, err := a.DashboardJSON("unheaded-timeguru")
	if err != nil {
		t.Fatalf("DashboardJSON: %v", err)
	}
	if !strings.Contains(jsonStr, "timeguru") {
		t.Error("JSON should contain service name")
	}
	if !strings.Contains(jsonStr, "Request Rate") {
		t.Error("JSON should contain panel title")
	}
}

func TestGrafanaAdapter_DashboardJSON_NotFound(t *testing.T) {
	a := NewGrafanaAdapter()
	_, err := a.DashboardJSON("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent dashboard")
	}
}

func TestGrafanaAdapter_Closed(t *testing.T) {
	a := NewGrafanaAdapter()
	a.Close()

	err := a.EmitAlert(context.Background(), Alert{Name: "test"})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed, got %v", err)
	}
}

func TestGrafanaAdapter_UnsupportedSignals(t *testing.T) {
	a := NewGrafanaAdapter()
	ctx := context.Background()

	if err := a.EmitLog(ctx, LogEntry{}); err != nil {
		t.Errorf("EmitLog should be no-op: %v", err)
	}
	if err := a.EmitTrace(ctx, Span{}); err != nil {
		t.Errorf("EmitTrace should be no-op: %v", err)
	}
}

// --- ELK Adapter Tests ---

func TestELKAdapter_Backend(t *testing.T) {
	a := NewELKAdapter("", 0)
	if a.Backend() != BackendELK {
		t.Errorf("expected elk, got %s", a.Backend())
	}
}

func TestELKAdapter_EmitLog(t *testing.T) {
	a := NewELKAdapter("test-logs", 100)
	ctx := context.Background()

	now := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	err := a.EmitLog(ctx, LogEntry{
		Level:     "info",
		Message:   "request processed",
		Service:   "timeguru",
		TraceID:   "trace-123",
		Fields:    map[string]string{"method": "GET"},
		Timestamp: now,
	})
	if err != nil {
		t.Fatalf("EmitLog: %v", err)
	}

	if a.DocumentCount() != 1 {
		t.Fatalf("expected 1 document, got %d", a.DocumentCount())
	}

	docs := a.Documents()
	if docs[0].Index != "test-logs-2026.02.24" {
		t.Errorf("expected date-based index, got %s", docs[0].Index)
	}
	if docs[0].Service != "timeguru" {
		t.Errorf("expected timeguru, got %s", docs[0].Service)
	}
	if docs[0].TraceID != "trace-123" {
		t.Errorf("expected trace-123, got %s", docs[0].TraceID)
	}
}

func TestELKAdapter_RingBuffer(t *testing.T) {
	a := NewELKAdapter("", 3)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a.EmitLog(ctx, LogEntry{Message: strings.Repeat("x", i+1)})
	}

	if a.DocumentCount() != 3 {
		t.Errorf("expected 3 docs (ring buffer), got %d", a.DocumentCount())
	}
}

func TestELKAdapter_BulkPayload(t *testing.T) {
	a := NewELKAdapter("test", 0)
	ctx := context.Background()

	a.EmitLog(ctx, LogEntry{Message: "hello", Service: "test"})
	a.EmitLog(ctx, LogEntry{Message: "world", Service: "test"})

	payload := a.BulkPayload()
	if !strings.Contains(payload, `{"index":`) {
		t.Error("bulk payload should contain index action")
	}
	if strings.Count(payload, `"index"`) < 2 {
		t.Error("bulk payload should have 2 index actions")
	}
}

func TestELKAdapter_Closed(t *testing.T) {
	a := NewELKAdapter("", 0)
	a.Close()

	err := a.EmitLog(context.Background(), LogEntry{Message: "test"})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed, got %v", err)
	}
}

func TestELKAdapter_DefaultIndex(t *testing.T) {
	a := NewELKAdapter("", 0)
	if a.index != "unheaded-logs" {
		t.Errorf("expected default index unheaded-logs, got %s", a.index)
	}
}

func TestLogstashConfig(t *testing.T) {
	cfg := LogstashConfig("timeguru", "es.internal:9200")
	if !strings.Contains(cfg, "timeguru") {
		t.Error("config should contain service name")
	}
	if !strings.Contains(cfg, "es.internal:9200") {
		t.Error("config should contain ES host")
	}
	if !strings.Contains(cfg, "elasticsearch") {
		t.Error("config should contain output section")
	}
}

func TestKibanaIndexPattern(t *testing.T) {
	pattern := KibanaIndexPattern("unheaded-logs")
	if !strings.Contains(pattern, "unheaded-logs-*") {
		t.Error("pattern should contain wildcard index")
	}
	if !strings.Contains(pattern, "@timestamp") {
		t.Error("pattern should contain time field")
	}
}

// --- Fluentd Adapter Tests ---

func TestFluentdAdapter_Backend(t *testing.T) {
	a := NewFluentdAdapter("", 0)
	if a.Backend() != BackendFluentd {
		t.Errorf("expected fluentd, got %s", a.Backend())
	}
}

func TestFluentdAdapter_EmitLog(t *testing.T) {
	a := NewFluentdAdapter("test.logs", 100)
	ctx := context.Background()

	err := a.EmitLog(ctx, LogEntry{
		Level:   "warn",
		Message: "slow query",
		Service: "wotan",
		TraceID: "trace-456",
		Fields:  map[string]string{"query_ms": "500"},
	})
	if err != nil {
		t.Fatalf("EmitLog: %v", err)
	}

	if a.EventCount() != 1 {
		t.Fatalf("expected 1 event, got %d", a.EventCount())
	}

	events := a.Events()
	if events[0].Tag != "test.logs.wotan.warn" {
		t.Errorf("expected tag test.logs.wotan.warn, got %s", events[0].Tag)
	}
	if events[0].Record["message"] != "slow query" {
		t.Errorf("expected slow query, got %s", events[0].Record["message"])
	}
	if events[0].Record["trace_id"] != "trace-456" {
		t.Errorf("expected trace_id in record")
	}
	if events[0].Record["query_ms"] != "500" {
		t.Errorf("expected custom field in record")
	}
}

func TestFluentdAdapter_RingBuffer(t *testing.T) {
	a := NewFluentdAdapter("", 2)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a.EmitLog(ctx, LogEntry{Message: "msg"})
	}

	if a.EventCount() != 2 {
		t.Errorf("expected 2 events (ring buffer), got %d", a.EventCount())
	}
}

func TestFluentdAdapter_DefaultTag(t *testing.T) {
	a := NewFluentdAdapter("", 0)
	if a.tag != "unheaded.logs" {
		t.Errorf("expected default tag, got %s", a.tag)
	}
}

func TestFluentdAdapter_JSONPayload(t *testing.T) {
	a := NewFluentdAdapter("test", 0)
	a.EmitLog(context.Background(), LogEntry{Message: "hello", Service: "svc", Level: "info"})

	payload := a.JSONPayload()
	if !strings.Contains(payload, "test.svc.info") {
		t.Errorf("payload should contain tag, got: %s", payload)
	}
}

func TestFluentdAdapter_Closed(t *testing.T) {
	a := NewFluentdAdapter("", 0)
	a.Close()

	err := a.EmitLog(context.Background(), LogEntry{Message: "test"})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed, got %v", err)
	}
}

func TestFluentdConfig(t *testing.T) {
	cfg := FluentdConfig("timeguru", "aggregator.internal")
	if !strings.Contains(cfg, "timeguru") {
		t.Error("config should contain service name")
	}
	if !strings.Contains(cfg, "aggregator.internal") {
		t.Error("config should contain output host")
	}
	if !strings.Contains(cfg, "forward") {
		t.Error("config should use forward plugin")
	}
}

func TestFluentBitConfig(t *testing.T) {
	cfg := FluentBitConfig("wotan", "loki.internal")
	if !strings.Contains(cfg, "wotan") {
		t.Error("config should contain service name")
	}
	if !strings.Contains(cfg, "loki.internal") {
		t.Error("config should contain output host")
	}
	if !strings.Contains(cfg, "[INPUT]") {
		t.Error("config should have INPUT section")
	}
}

// --- Jaeger Adapter Tests ---

func TestJaegerAdapter_Backend(t *testing.T) {
	a := NewJaegerAdapter(0)
	if a.Backend() != BackendJaeger {
		t.Errorf("expected jaeger, got %s", a.Backend())
	}
}

func TestJaegerAdapter_Supports(t *testing.T) {
	a := NewJaegerAdapter(0)
	sigs := a.Supports()
	if len(sigs) != 1 || sigs[0] != SignalTrace {
		t.Error("Jaeger should support only trace signal")
	}
}

func TestJaegerAdapter_EmitTrace(t *testing.T) {
	a := NewJaegerAdapter(100)
	ctx := context.Background()

	now := time.Now()
	err := a.EmitTrace(ctx, Span{
		TraceID:   "trace-abc",
		SpanID:    "span-123",
		ParentID:  "span-000",
		Operation: "HTTP GET /api/v1/timeline",
		Service:   "timeguru",
		StartTime: now,
		Duration:  5 * time.Millisecond,
		Status:    SpanOK,
		Attributes: map[string]string{
			"http.method": "GET",
			"http.status": "200",
		},
	})
	if err != nil {
		t.Fatalf("EmitTrace: %v", err)
	}

	if a.SpanCount() != 1 {
		t.Fatalf("expected 1 span, got %d", a.SpanCount())
	}

	spans := a.Spans()
	if spans[0].TraceID != "trace-abc" {
		t.Errorf("expected trace-abc, got %s", spans[0].TraceID)
	}
	if spans[0].OperationName != "HTTP GET /api/v1/timeline" {
		t.Errorf("wrong operation: %s", spans[0].OperationName)
	}
	if spans[0].Duration != 5000 {
		t.Errorf("expected 5000 microseconds, got %d", spans[0].Duration)
	}
	if len(spans[0].References) != 1 {
		t.Error("expected 1 reference (CHILD_OF)")
	}
	// Should have status tag + 2 attribute tags
	if len(spans[0].Tags) < 3 {
		t.Errorf("expected at least 3 tags, got %d", len(spans[0].Tags))
	}
}

func TestJaegerAdapter_RingBuffer(t *testing.T) {
	a := NewJaegerAdapter(3)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a.EmitTrace(ctx, Span{SpanID: "span", Operation: "op"})
	}

	if a.SpanCount() != 3 {
		t.Errorf("expected 3 spans (ring buffer), got %d", a.SpanCount())
	}
}

func TestJaegerAdapter_CollectorPayload(t *testing.T) {
	a := NewJaegerAdapter(0)
	ctx := context.Background()

	a.EmitTrace(ctx, Span{Service: "timeguru", Operation: "op1"})
	a.EmitTrace(ctx, Span{Service: "wotan", Operation: "op2"})

	payload := a.CollectorPayload()
	if !strings.Contains(payload, "timeguru") {
		t.Error("payload should contain timeguru service")
	}
	if !strings.Contains(payload, "wotan") {
		t.Error("payload should contain wotan service")
	}
}

func TestJaegerAdapter_Closed(t *testing.T) {
	a := NewJaegerAdapter(0)
	a.Close()

	err := a.EmitTrace(context.Background(), Span{SpanID: "test"})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed, got %v", err)
	}
}

func TestJaegerAdapter_UnsupportedSignals(t *testing.T) {
	a := NewJaegerAdapter(0)
	ctx := context.Background()

	if err := a.EmitMetric(ctx, Metric{}); err != nil {
		t.Errorf("EmitMetric should be no-op: %v", err)
	}
	if err := a.EmitLog(ctx, LogEntry{}); err != nil {
		t.Errorf("EmitLog should be no-op: %v", err)
	}
	if err := a.EmitAlert(ctx, Alert{}); err != nil {
		t.Errorf("EmitAlert should be no-op: %v", err)
	}
}

func TestJaegerCollectorConfig(t *testing.T) {
	cfg := JaegerCollectorConfig("es.internal:9200")
	if !strings.Contains(cfg, "es.internal:9200") {
		t.Error("config should contain ES host")
	}
	if !strings.Contains(cfg, "elasticsearch") {
		t.Error("config should reference elasticsearch storage")
	}
}

// --- Nagios Adapter Tests ---

func TestNagiosAdapter_Backend(t *testing.T) {
	a := NewNagiosAdapter()
	if a.Backend() != BackendNagios {
		t.Errorf("expected nagios, got %s", a.Backend())
	}
}

func TestNagiosAdapter_Supports(t *testing.T) {
	a := NewNagiosAdapter()
	sigs := a.Supports()
	if len(sigs) != 2 {
		t.Fatalf("expected 2 signal types, got %d", len(sigs))
	}
}

func TestNagiosAdapter_EmitAlert(t *testing.T) {
	a := NewNagiosAdapter()
	ctx := context.Background()

	err := a.EmitAlert(ctx, Alert{
		Name:     "disk_full",
		Severity: SeverityCritical,
		Message:  "Disk usage > 95%",
		Service:  "wotan",
	})
	if err != nil {
		t.Fatalf("EmitAlert: %v", err)
	}

	if a.CheckCount() != 1 {
		t.Fatalf("expected 1 check, got %d", a.CheckCount())
	}

	checks := a.CheckResults()
	if checks[0].ReturnCode != 2 { // CRITICAL
		t.Errorf("expected return code 2 (CRITICAL), got %d", checks[0].ReturnCode)
	}
	if checks[0].Service != "wotan_disk_full" {
		t.Errorf("expected wotan_disk_full, got %s", checks[0].Service)
	}
}

func TestNagiosAdapter_EmitMetric(t *testing.T) {
	a := NewNagiosAdapter()
	ctx := context.Background()

	err := a.EmitMetric(ctx, Metric{
		Name:   "cpu_usage",
		Value:  75.5,
		Labels: map[string]string{"service": "timeguru"},
	})
	if err != nil {
		t.Fatalf("EmitMetric: %v", err)
	}

	checks := a.CheckResults()
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if !strings.Contains(checks[0].Output, "cpu_usage=75.5") {
		t.Errorf("expected performance data, got %s", checks[0].Output)
	}
}

func TestNagiosAdapter_SeverityMapping(t *testing.T) {
	tests := []struct {
		severity AlertSeverity
		expected int
	}{
		{SeverityInfo, 0},
		{SeverityWarning, 1},
		{SeverityError, 2},
		{SeverityCritical, 2},
	}

	for _, tt := range tests {
		got := severityToNagios(tt.severity)
		if got != tt.expected {
			t.Errorf("severity %s: expected %d, got %d", tt.severity, tt.expected, got)
		}
	}
}

func TestNagiosAdapter_NSCAPayload(t *testing.T) {
	a := NewNagiosAdapter()
	ctx := context.Background()

	a.EmitAlert(ctx, Alert{Name: "test", Severity: SeverityWarning, Message: "warn", Service: "svc"})

	payload := a.NSCAPayload()
	if !strings.Contains(payload, "unheaded") {
		t.Error("NSCA payload should contain host")
	}
	if !strings.Contains(payload, "\t1\t") {
		t.Error("NSCA payload should contain WARNING return code")
	}
}

func TestNagiosAdapter_Closed(t *testing.T) {
	a := NewNagiosAdapter()
	a.Close()

	err := a.EmitAlert(context.Background(), Alert{Name: "test"})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed, got %v", err)
	}

	err = a.EmitMetric(context.Background(), Metric{Name: "test"})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed for metric, got %v", err)
	}
}

func TestNagiosServiceDefinition(t *testing.T) {
	def := NagiosServiceDefinition("timeguru", 19000)
	if !strings.Contains(def, "timeguru") {
		t.Error("definition should contain service name")
	}
	if !strings.Contains(def, "19000") {
		t.Error("definition should contain port")
	}
	if !strings.Contains(def, "check_http") {
		t.Error("definition should contain check_http command")
	}
	if !strings.Contains(def, "/health") {
		t.Error("definition should check /health endpoint")
	}
}

// --- Loki Adapter Tests ---

func TestLokiAdapter_Backend(t *testing.T) {
	a := NewLokiAdapter(0)
	if a.Backend() != BackendLoki {
		t.Errorf("expected loki, got %s", a.Backend())
	}
}

func TestLokiAdapter_Supports(t *testing.T) {
	a := NewLokiAdapter(0)
	sigs := a.Supports()
	if len(sigs) != 1 || sigs[0] != SignalLog {
		t.Error("Loki should support only log signal")
	}
}

func TestLokiAdapter_EmitLog(t *testing.T) {
	a := NewLokiAdapter(100)
	ctx := context.Background()

	err := a.EmitLog(ctx, LogEntry{
		Level:   "info",
		Message: "request processed",
		Service: "timeguru",
		TraceID: "trace-789",
	})
	if err != nil {
		t.Fatalf("EmitLog: %v", err)
	}

	if a.EntryCount() != 1 {
		t.Fatalf("expected 1 entry, got %d", a.EntryCount())
	}
	if a.StreamCount() != 1 {
		t.Fatalf("expected 1 stream, got %d", a.StreamCount())
	}

	streams := a.Streams()
	for _, s := range streams {
		if s.Labels["service"] != "timeguru" {
			t.Errorf("expected service=timeguru label")
		}
		if s.Labels["level"] != "info" {
			t.Errorf("expected level=info label")
		}
		if !strings.Contains(s.Values[0].Line, "request processed") {
			t.Errorf("entry should contain message")
		}
		if !strings.Contains(s.Values[0].Line, "trace_id=trace-789") {
			t.Errorf("entry should contain trace_id")
		}
	}
}

func TestLokiAdapter_MultipleStreams(t *testing.T) {
	a := NewLokiAdapter(0)
	ctx := context.Background()

	a.EmitLog(ctx, LogEntry{Service: "timeguru", Level: "info", Message: "a"})
	a.EmitLog(ctx, LogEntry{Service: "timeguru", Level: "error", Message: "b"})
	a.EmitLog(ctx, LogEntry{Service: "wotan", Level: "info", Message: "c"})

	// Different service+level combos = different streams
	if a.StreamCount() < 2 {
		t.Errorf("expected at least 2 streams, got %d", a.StreamCount())
	}
	if a.EntryCount() != 3 {
		t.Errorf("expected 3 total entries, got %d", a.EntryCount())
	}
}

func TestLokiAdapter_RingBuffer(t *testing.T) {
	a := NewLokiAdapter(3)
	ctx := context.Background()

	// Same stream (same service + level = same labels = same stream)
	for i := 0; i < 5; i++ {
		a.EmitLog(ctx, LogEntry{Service: "test", Level: "info", Message: "msg"})
	}

	if a.StreamCount() != 1 {
		t.Errorf("expected 1 stream, got %d", a.StreamCount())
	}
	if a.EntryCount() != 3 {
		t.Errorf("expected 3 entries (ring buffer per stream), got %d", a.EntryCount())
	}
}

func TestLokiAdapter_PushPayload(t *testing.T) {
	a := NewLokiAdapter(0)
	a.EmitLog(context.Background(), LogEntry{Service: "test", Message: "hello"})

	payload := a.PushPayload()
	if !strings.Contains(payload, "streams") {
		t.Error("push payload should contain streams key")
	}
	if !strings.Contains(payload, "hello") {
		t.Error("push payload should contain log line")
	}
}

func TestLokiAdapter_Closed(t *testing.T) {
	a := NewLokiAdapter(0)
	a.Close()

	err := a.EmitLog(context.Background(), LogEntry{Message: "test"})
	if err != ErrAdapterClosed {
		t.Errorf("expected ErrAdapterClosed, got %v", err)
	}
}

func TestLokiAdapter_UnsupportedSignals(t *testing.T) {
	a := NewLokiAdapter(0)
	ctx := context.Background()

	if err := a.EmitMetric(ctx, Metric{}); err != nil {
		t.Errorf("EmitMetric should be no-op: %v", err)
	}
	if err := a.EmitTrace(ctx, Span{}); err != nil {
		t.Errorf("EmitTrace should be no-op: %v", err)
	}
	if err := a.EmitAlert(ctx, Alert{}); err != nil {
		t.Errorf("EmitAlert should be no-op: %v", err)
	}
}

func TestPromtailConfig(t *testing.T) {
	cfg := PromtailConfig("timeguru", "http://loki.internal:3100")
	if !strings.Contains(cfg, "timeguru") {
		t.Error("config should contain service name")
	}
	if !strings.Contains(cfg, "loki.internal:3100") {
		t.Error("config should contain Loki URL")
	}
	if !strings.Contains(cfg, "pipeline_stages") {
		t.Error("config should contain pipeline stages")
	}
}

// --- Pipeline Integration with New Adapters ---

func TestPipeline_FullStack(t *testing.T) {
	prom := NewPrometheusAdapter()
	logs := NewZerologAdapter(100)
	elk := NewELKAdapter("test", 100)
	fluentd := NewFluentdAdapter("test", 100)
	jaeger := NewJaegerAdapter(100)
	nagios := NewNagiosAdapter()
	loki := NewLokiAdapter(100)
	grafana := NewGrafanaAdapter()

	p := NewPipeline(prom, logs, elk, fluentd, jaeger, nagios, loki, grafana)
	ctx := context.Background()

	// Emit all signal types
	p.EmitMetric(ctx, Metric{
		Name: "test", Value: 1, Type: MetricCounter,
		Labels: map[string]string{"service": "test"},
	})
	p.EmitLog(ctx, LogEntry{
		Level: "info", Message: "test log", Service: "test",
	})
	p.EmitTrace(ctx, Span{
		TraceID: "t1", SpanID: "s1", Operation: "op", Service: "test",
		Duration: time.Millisecond,
	})
	p.EmitAlert(ctx, Alert{
		Name: "test_alert", Severity: SeverityWarning, Service: "test",
		Message: "test alert",
	})

	// Verify each adapter received the signals it supports
	if prom.MetricCount() != 1 {
		t.Error("prometheus should have 1 metric")
	}
	if logs.EntryCount() != 1 {
		t.Error("zerolog should have 1 log")
	}
	if elk.DocumentCount() != 1 {
		t.Error("elk should have 1 document")
	}
	if fluentd.EventCount() != 1 {
		t.Error("fluentd should have 1 event")
	}
	if jaeger.SpanCount() != 1 {
		t.Error("jaeger should have 1 span")
	}
	if nagios.CheckCount() != 2 { // 1 metric + 1 alert
		t.Errorf("nagios should have 2 checks, got %d", nagios.CheckCount())
	}
	if loki.EntryCount() != 1 {
		t.Error("loki should have 1 entry")
	}
	if len(grafana.Annotations()) != 1 {
		t.Error("grafana should have 1 annotation")
	}
}

func TestPipeline_FullStack_ConcurrentSafety(t *testing.T) {
	jaeger := NewJaegerAdapter(0)
	loki := NewLokiAdapter(0)
	nagios := NewNagiosAdapter()

	p := NewPipeline(jaeger, loki, nagios)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.EmitTrace(ctx, Span{SpanID: "s", Operation: "op", Service: "test"})
			p.EmitLog(ctx, LogEntry{Service: "test", Level: "info", Message: "msg"})
			p.EmitAlert(ctx, Alert{Name: "a", Severity: SeverityInfo, Service: "test"})
		}()
	}
	wg.Wait()

	if jaeger.SpanCount() != 50 {
		t.Errorf("expected 50 spans, got %d", jaeger.SpanCount())
	}
	if loki.EntryCount() != 50 {
		t.Errorf("expected 50 loki entries, got %d", loki.EntryCount())
	}
	if nagios.CheckCount() != 50 {
		t.Errorf("expected 50 nagios checks, got %d", nagios.CheckCount())
	}
}
