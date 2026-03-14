// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package observe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ==================== Metrics Tests ====================

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.statusCodes == nil {
		t.Fatal("expected non-nil statusCodes map")
	}
	if m.services == nil {
		t.Fatal("expected non-nil services map")
	}
	if len(m.bucketBounds) != 12 {
		t.Errorf("expected 12 bucket bounds, got %d", len(m.bucketBounds))
	}
	if len(m.latencyBuckets) != 13 {
		t.Errorf("expected 13 latency buckets (12+overflow), got %d", len(m.latencyBuckets))
	}
}

func TestRecordRequest_Success(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("svc-a", "default", 200, 50*time.Millisecond, nil)

	snap := m.Snapshot()
	if snap.TotalRequests != 1 {
		t.Errorf("expected 1 total request, got %d", snap.TotalRequests)
	}
	if snap.SuccessRequests != 1 {
		t.Errorf("expected 1 success request, got %d", snap.SuccessRequests)
	}
	if snap.FailedRequests != 0 {
		t.Errorf("expected 0 failed requests, got %d", snap.FailedRequests)
	}
	if snap.StatusCodes[200] != 1 {
		t.Errorf("expected status 200 count 1, got %d", snap.StatusCodes[200])
	}
}

func TestRecordRequest_ServerError(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("svc-a", "default", 500, 10*time.Millisecond, nil)

	snap := m.Snapshot()
	if snap.FailedRequests != 1 {
		t.Errorf("expected 1 failed request, got %d", snap.FailedRequests)
	}
	if snap.SuccessRequests != 0 {
		t.Errorf("expected 0 success requests, got %d", snap.SuccessRequests)
	}
}

func TestRecordRequest_WithError(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("svc-a", "default", 200, 10*time.Millisecond, errors.New("timeout"))

	snap := m.Snapshot()
	if snap.FailedRequests != 1 {
		t.Errorf("expected 1 failed request, got %d", snap.FailedRequests)
	}
}

func TestRecordRequest_ServiceMetrics(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("svc-a", "ns1", 200, 100*time.Millisecond, nil)
	m.RecordRequest("svc-a", "ns1", 500, 200*time.Millisecond, nil)
	m.RecordRequest("svc-b", "ns1", 200, 50*time.Millisecond, nil)

	snap := m.Snapshot()
	if len(snap.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(snap.Services))
	}

	svcA := snap.Services["ns1/svc-a"]
	if svcA == nil {
		t.Fatal("expected ns1/svc-a service snapshot")
	}
	if svcA.TotalRequests != 2 {
		t.Errorf("expected 2 total requests for svc-a, got %d", svcA.TotalRequests)
	}
	if svcA.SuccessRequests != 1 {
		t.Errorf("expected 1 success for svc-a, got %d", svcA.SuccessRequests)
	}
	if svcA.FailedRequests != 1 {
		t.Errorf("expected 1 failure for svc-a, got %d", svcA.FailedRequests)
	}
	if svcA.Name != "svc-a" {
		t.Errorf("expected name svc-a, got %s", svcA.Name)
	}
	if svcA.Namespace != "ns1" {
		t.Errorf("expected namespace ns1, got %s", svcA.Namespace)
	}
}

func TestRecordConnection(t *testing.T) {
	m := NewMetrics()

	m.RecordConnection(true, nil)
	m.RecordConnection(true, nil)
	m.RecordConnection(false, nil)

	snap := m.Snapshot()
	if snap.ActiveConnections != 1 {
		t.Errorf("expected 1 active connection, got %d", snap.ActiveConnections)
	}
	if snap.TotalConnections != 2 {
		t.Errorf("expected 2 total connections, got %d", snap.TotalConnections)
	}
	if snap.ConnectionErrors != 0 {
		t.Errorf("expected 0 connection errors, got %d", snap.ConnectionErrors)
	}
}

func TestRecordConnection_WithError(t *testing.T) {
	m := NewMetrics()

	m.RecordConnection(true, errors.New("refused"))

	snap := m.Snapshot()
	if snap.ConnectionErrors != 1 {
		t.Errorf("expected 1 connection error, got %d", snap.ConnectionErrors)
	}
	if snap.TotalConnections != 1 {
		t.Errorf("expected 1 total connection, got %d", snap.TotalConnections)
	}
}

func TestRecordBytes(t *testing.T) {
	m := NewMetrics()

	m.RecordBytes(100, 200)
	m.RecordBytes(50, 75)

	snap := m.Snapshot()
	if snap.BytesIn != 150 {
		t.Errorf("expected 150 bytes in, got %d", snap.BytesIn)
	}
	if snap.BytesOut != 275 {
		t.Errorf("expected 275 bytes out, got %d", snap.BytesOut)
	}
}

func TestRecordRetry(t *testing.T) {
	m := NewMetrics()

	// Must create service first via RecordRequest
	m.RecordRequest("svc-a", "ns1", 200, time.Millisecond, nil)
	m.RecordRetry("svc-a", "ns1")
	m.RecordRetry("svc-a", "ns1")

	snap := m.Snapshot()
	svc := snap.Services["ns1/svc-a"]
	if svc == nil {
		t.Fatal("expected service snapshot")
	}
	if svc.Retries != 2 {
		t.Errorf("expected 2 retries, got %d", svc.Retries)
	}
}

func TestRecordRetry_UnknownService(t *testing.T) {
	m := NewMetrics()
	// Should not panic on unknown service
	m.RecordRetry("unknown", "ns1")
}

func TestRecordCircuitOpen(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("svc-a", "ns1", 200, time.Millisecond, nil)
	m.RecordCircuitOpen("svc-a", "ns1")

	snap := m.Snapshot()
	svc := snap.Services["ns1/svc-a"]
	if svc == nil {
		t.Fatal("expected service snapshot")
	}
	if svc.CircuitOpens != 1 {
		t.Errorf("expected 1 circuit open, got %d", svc.CircuitOpens)
	}
	if svc.CircuitState != "open" {
		t.Errorf("expected circuit state open, got %s", svc.CircuitState)
	}
}

func TestRecordCircuitOpen_UnknownService(t *testing.T) {
	m := NewMetrics()
	// Should not panic
	m.RecordCircuitOpen("unknown", "ns1")
}

func TestUpdateCircuitState(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("svc-a", "ns1", 200, time.Millisecond, nil)
	m.UpdateCircuitState("svc-a", "ns1", "half-open")

	snap := m.Snapshot()
	svc := snap.Services["ns1/svc-a"]
	if svc.CircuitState != "half-open" {
		t.Errorf("expected circuit state half-open, got %s", svc.CircuitState)
	}
}

func TestUpdateCircuitState_UnknownService(t *testing.T) {
	m := NewMetrics()
	// Should not panic
	m.UpdateCircuitState("unknown", "ns1", "closed")
}

func TestGetOrCreateServiceMetrics(t *testing.T) {
	m := NewMetrics()

	svc1 := m.GetOrCreateServiceMetrics("svc-a", "ns1")
	if svc1 == nil {
		t.Fatal("expected non-nil service metrics")
	}
	if svc1.Name != "svc-a" {
		t.Errorf("expected name svc-a, got %s", svc1.Name)
	}
	if svc1.Namespace != "ns1" {
		t.Errorf("expected namespace ns1, got %s", svc1.Namespace)
	}

	// Get same service again
	svc2 := m.GetOrCreateServiceMetrics("svc-a", "ns1")
	if svc1 != svc2 {
		t.Error("expected same service metrics instance")
	}
}

func TestGetOrCreateServiceMetrics_Concurrent(t *testing.T) {
	m := NewMetrics()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc := m.GetOrCreateServiceMetrics("svc-a", "ns1")
			if svc == nil {
				t.Error("expected non-nil service metrics")
			}
		}()
	}
	wg.Wait()
}

func TestMetricsSnapshot(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("svc-a", "ns1", 200, 5*time.Millisecond, nil)
	m.RecordRequest("svc-a", "ns1", 404, 10*time.Millisecond, nil)
	m.RecordConnection(true, nil)
	m.RecordBytes(100, 200)

	snap := m.Snapshot()

	if snap.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if snap.TotalRequests != 2 {
		t.Errorf("expected 2 total requests, got %d", snap.TotalRequests)
	}
	if snap.StatusCodes[200] != 1 {
		t.Errorf("expected 1 for status 200, got %d", snap.StatusCodes[200])
	}
	if snap.StatusCodes[404] != 1 {
		t.Errorf("expected 1 for status 404, got %d", snap.StatusCodes[404])
	}
}

func TestMetricsReset(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("svc-a", "ns1", 200, time.Millisecond, nil)
	m.RecordConnection(true, nil)
	m.RecordBytes(100, 200)

	m.Reset()

	snap := m.Snapshot()
	if snap.TotalRequests != 0 {
		t.Errorf("expected 0 total requests after reset, got %d", snap.TotalRequests)
	}
	if snap.BytesIn != 0 {
		t.Errorf("expected 0 bytes in after reset, got %d", snap.BytesIn)
	}
	if snap.BytesOut != 0 {
		t.Errorf("expected 0 bytes out after reset, got %d", snap.BytesOut)
	}
	if len(snap.StatusCodes) != 0 {
		t.Errorf("expected empty status codes after reset, got %d", len(snap.StatusCodes))
	}
	if len(snap.Services) != 0 {
		t.Errorf("expected empty services after reset, got %d", len(snap.Services))
	}
}

func TestSuccessRate(t *testing.T) {
	tests := []struct {
		name     string
		total    uint64
		success  uint64
		expected float64
	}{
		{"no requests", 0, 0, 100.0},
		{"all success", 10, 10, 100.0},
		{"half success", 10, 5, 50.0},
		{"no success", 10, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := &MetricsSnapshot{
				TotalRequests:   tt.total,
				SuccessRequests: tt.success,
			}
			got := snap.SuccessRate()
			if got != tt.expected {
				t.Errorf("expected success rate %.1f, got %.1f", tt.expected, got)
			}
		})
	}
}

func TestLatencyPercentile(t *testing.T) {
	t.Run("empty histogram", func(t *testing.T) {
		snap := &MetricsSnapshot{
			LatencyBuckets:      make([]uint64, 13),
			LatencyBucketBounds: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
		}
		got := snap.LatencyPercentile(50)
		if got != 0 {
			t.Errorf("expected 0 for empty histogram, got %f", got)
		}
	})

	t.Run("all in first bucket", func(t *testing.T) {
		buckets := make([]uint64, 13)
		buckets[0] = 100
		snap := &MetricsSnapshot{
			LatencyBuckets:      buckets,
			LatencyBucketBounds: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
		}
		got := snap.LatencyPercentile(50)
		if got != 1 {
			t.Errorf("expected p50=1ms, got %f", got)
		}
	})

	t.Run("overflow bucket", func(t *testing.T) {
		buckets := make([]uint64, 13)
		buckets[12] = 100 // overflow bucket
		snap := &MetricsSnapshot{
			LatencyBuckets:      buckets,
			LatencyBucketBounds: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
		}
		got := snap.LatencyPercentile(50)
		if got != 20000 {
			t.Errorf("expected overflow p50=20000ms, got %f", got)
		}
	})

	t.Run("distributed across buckets", func(t *testing.T) {
		buckets := make([]uint64, 13)
		buckets[0] = 10  // <=1ms
		buckets[1] = 10  // <=5ms
		buckets[2] = 10  // <=10ms
		buckets[5] = 70  // <=100ms
		snap := &MetricsSnapshot{
			LatencyBuckets:      buckets,
			LatencyBucketBounds: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
		}
		got := snap.LatencyPercentile(95)
		if got != 100 {
			t.Errorf("expected p95=100ms, got %f", got)
		}
	})
}

func TestLatencyBucketAssignment(t *testing.T) {
	m := NewMetrics()

	// Record very fast request (<1ms)
	m.RecordRequest("svc", "ns", 200, 500*time.Microsecond, nil)
	// Record medium request (~50ms)
	m.RecordRequest("svc", "ns", 200, 50*time.Millisecond, nil)
	// Record slow request (~15s, overflow)
	m.RecordRequest("svc", "ns", 200, 15*time.Second, nil)

	snap := m.Snapshot()
	if snap.LatencyBuckets[0] != 1 {
		t.Errorf("expected 1 in bucket 0 (<=1ms), got %d", snap.LatencyBuckets[0])
	}
	// 50ms falls into bucket index 4 (<=50ms)
	if snap.LatencyBuckets[4] != 1 {
		t.Errorf("expected 1 in bucket 4 (<=50ms), got %d", snap.LatencyBuckets[4])
	}
	// 15s falls into overflow bucket (index 12)
	if snap.LatencyBuckets[12] != 1 {
		t.Errorf("expected 1 in overflow bucket, got %d", snap.LatencyBuckets[12])
	}
}

func TestServiceMetricsLatency(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("svc-a", "ns1", 200, 10*time.Millisecond, nil)
	m.RecordRequest("svc-a", "ns1", 200, 30*time.Millisecond, nil)
	m.RecordRequest("svc-a", "ns1", 200, 20*time.Millisecond, nil)

	snap := m.Snapshot()
	svc := snap.Services["ns1/svc-a"]
	if svc == nil {
		t.Fatal("expected service snapshot")
	}

	// Min should be ~10000us
	if svc.LatencyMinUs > 11000 {
		t.Errorf("expected min latency ~10000us, got %d", svc.LatencyMinUs)
	}
	// Max should be ~30000us
	if svc.LatencyMaxUs < 29000 {
		t.Errorf("expected max latency ~30000us, got %d", svc.LatencyMaxUs)
	}
	// Avg should be ~20000us
	if svc.LatencyAvgUs < 15000 || svc.LatencyAvgUs > 25000 {
		t.Errorf("expected avg latency ~20000us, got %d", svc.LatencyAvgUs)
	}
}

func TestMetrics_Concurrent(t *testing.T) {
	m := NewMetrics()
	var wg sync.WaitGroup

	// Phase 1: concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc := fmt.Sprintf("svc-%d", i%5)
			m.RecordRequest(svc, "ns", 200, time.Duration(i)*time.Millisecond, nil)
			m.RecordConnection(true, nil)
			m.RecordBytes(uint64(i), uint64(i*2))
			m.RecordRetry(svc, "ns")
		}(i)
	}
	wg.Wait()

	// Phase 2: snapshot after all writes complete (avoids race on latencyBuckets slice)
	snap := m.Snapshot()
	if snap.TotalRequests != 100 {
		t.Errorf("expected 100 total requests, got %d", snap.TotalRequests)
	}
}

// ==================== Tracing Tests ====================

func TestTraceID_String(t *testing.T) {
	var id TraceID
	for i := range id {
		id[i] = byte(i)
	}
	got := id.String()
	expected := "000102030405060708090a0b0c0d0e0f"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestSpanID_String(t *testing.T) {
	var id SpanID
	for i := range id {
		id[i] = byte(i + 0xa0)
	}
	got := id.String()
	expected := "a0a1a2a3a4a5a6a7"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestParseTraceID(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		hex := "000102030405060708090a0b0c0d0e0f"
		id, err := ParseTraceID(hex)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.String() != hex {
			t.Errorf("expected %s, got %s", hex, id.String())
		}
	})

	t.Run("invalid hex", func(t *testing.T) {
		_, err := ParseTraceID("not-hex")
		if err == nil {
			t.Error("expected error for invalid hex")
		}
	})

	t.Run("wrong length", func(t *testing.T) {
		id, err := ParseTraceID("0001")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should not copy partial data - id should be zeroed
		var zero TraceID
		if id != zero {
			t.Errorf("expected zero trace ID for wrong length, got %s", id.String())
		}
	})
}

func TestParseSpanID(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		hex := "a0a1a2a3a4a5a6a7"
		id, err := ParseSpanID(hex)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.String() != hex {
			t.Errorf("expected %s, got %s", hex, id.String())
		}
	})

	t.Run("invalid hex", func(t *testing.T) {
		_, err := ParseSpanID("zzz")
		if err == nil {
			t.Error("expected error for invalid hex")
		}
	})

	t.Run("wrong length", func(t *testing.T) {
		id, err := ParseSpanID("00")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var zero SpanID
		if id != zero {
			t.Errorf("expected zero span ID for wrong length, got %s", id.String())
		}
	})
}

func TestGenerateTraceID(t *testing.T) {
	id1 := GenerateTraceID()
	id2 := GenerateTraceID()

	var zero TraceID
	if id1 == zero {
		t.Error("generated trace ID should not be zero")
	}
	if id1 == id2 {
		t.Error("two generated trace IDs should differ")
	}
}

func TestGenerateSpanID(t *testing.T) {
	id1 := GenerateSpanID()
	id2 := GenerateSpanID()

	var zero SpanID
	if id1 == zero {
		t.Error("generated span ID should not be zero")
	}
	if id1 == id2 {
		t.Error("two generated span IDs should differ")
	}
}

func TestSpanStatus_String(t *testing.T) {
	tests := []struct {
		status   SpanStatus
		expected string
	}{
		{SpanStatusUnset, "UNSET"},
		{SpanStatusOK, "OK"},
		{SpanStatusError, "ERROR"},
		{SpanStatus(99), "UNSET"},
	}
	for _, tt := range tests {
		got := tt.status.String()
		if got != tt.expected {
			t.Errorf("SpanStatus(%d).String() = %s, want %s", tt.status, got, tt.expected)
		}
	}
}

func TestNewSpan(t *testing.T) {
	traceID := GenerateTraceID()
	parentID := GenerateSpanID()

	span := NewSpan("test-op", traceID, parentID)

	if span.Name != "test-op" {
		t.Errorf("expected name test-op, got %s", span.Name)
	}
	if span.TraceID != traceID {
		t.Error("trace ID mismatch")
	}
	if span.ParentSpanID != parentID {
		t.Error("parent span ID mismatch")
	}
	if span.StartTime.IsZero() {
		t.Error("expected non-zero start time")
	}
	if !span.Sampled {
		t.Error("expected sampled=true by default")
	}
	if span.Tags == nil {
		t.Error("expected non-nil tags")
	}
	if span.Logs == nil {
		t.Error("expected non-nil logs")
	}
}

func TestSpan_SetTag(t *testing.T) {
	span := NewSpan("test", GenerateTraceID(), SpanID{})
	span.SetTag("http.method", "GET")
	span.SetTag("http.url", "/api/v1/test")

	if span.Tags["http.method"] != "GET" {
		t.Errorf("expected tag GET, got %s", span.Tags["http.method"])
	}
	if span.Tags["http.url"] != "/api/v1/test" {
		t.Errorf("expected tag /api/v1/test, got %s", span.Tags["http.url"])
	}
}

func TestSpan_Log(t *testing.T) {
	span := NewSpan("test", GenerateTraceID(), SpanID{})
	span.Log(map[string]string{"event": "retry", "attempt": "1"})
	span.Log(map[string]string{"event": "success"})

	if len(span.Logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(span.Logs))
	}
	if span.Logs[0].Fields["event"] != "retry" {
		t.Errorf("expected event retry, got %s", span.Logs[0].Fields["event"])
	}
	if span.Logs[0].Timestamp.IsZero() {
		t.Error("expected non-zero log timestamp")
	}
}

func TestSpan_SetStatus(t *testing.T) {
	span := NewSpan("test", GenerateTraceID(), SpanID{})
	span.SetStatus(SpanStatusOK)
	if span.Status != SpanStatusOK {
		t.Errorf("expected status OK, got %v", span.Status)
	}
	span.SetStatus(SpanStatusError)
	if span.Status != SpanStatusError {
		t.Errorf("expected status Error, got %v", span.Status)
	}
}

func TestSpan_Finish(t *testing.T) {
	span := NewSpan("test", GenerateTraceID(), SpanID{})
	time.Sleep(time.Millisecond)
	span.Finish()

	if span.EndTime.IsZero() {
		t.Error("expected non-zero end time")
	}
	if span.Duration <= 0 {
		t.Error("expected positive duration")
	}
	if span.EndTime.Before(span.StartTime) {
		t.Error("end time should be after start time")
	}
}

func TestContextWithSpan_And_SpanFromContext(t *testing.T) {
	span := NewSpan("test", GenerateTraceID(), SpanID{})
	ctx := ContextWithSpan(context.Background(), span)

	got := SpanFromContext(ctx)
	if got != span {
		t.Error("expected same span from context")
	}
}

func TestSpanFromContext_Empty(t *testing.T) {
	got := SpanFromContext(context.Background())
	if got != nil {
		t.Error("expected nil span from empty context")
	}
}

func TestNewTracer(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := NewTracer("test-service", 1.0, exporter)
	if tracer == nil {
		t.Fatal("expected non-nil tracer")
	}
}

func TestTracer_StartSpan(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := NewTracer("test-service", 1.0, exporter)

	span, ctx := tracer.StartSpan(context.Background(), "root-op")

	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if span.Name != "root-op" {
		t.Errorf("expected name root-op, got %s", span.Name)
	}
	if span.Service != "test-service" {
		t.Errorf("expected service test-service, got %s", span.Service)
	}

	// Verify context has span
	ctxSpan := SpanFromContext(ctx)
	if ctxSpan != span {
		t.Error("expected span in context")
	}
}

func TestTracer_StartSpan_ChildSpan(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := NewTracer("test-service", 1.0, exporter)

	parent, ctx := tracer.StartSpan(context.Background(), "parent")
	child, _ := tracer.StartSpan(ctx, "child")

	if child.TraceID != parent.TraceID {
		t.Error("child should inherit parent trace ID")
	}
	if child.ParentSpanID != parent.SpanID {
		t.Error("child parent span ID should match parent span ID")
	}
}

func TestTracer_FinishSpan(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := NewTracer("test-service", 1.0, exporter)

	span, _ := tracer.StartSpan(context.Background(), "test-op")
	tracer.FinishSpan(span)

	spans := exporter.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 exported span, got %d", len(spans))
	}
	if spans[0].Duration <= 0 {
		t.Error("expected positive duration on exported span")
	}
}

func TestTracer_FinishSpan_NotSampled(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := NewTracer("test-service", 0.0, exporter) // 0% sample rate

	span, _ := tracer.StartSpan(context.Background(), "test-op")
	tracer.FinishSpan(span)

	spans := exporter.Spans()
	if len(spans) != 0 {
		t.Errorf("expected 0 exported spans for unsampled trace, got %d", len(spans))
	}
}

func TestTracer_FinishSpan_NilExporter(t *testing.T) {
	tracer := NewTracer("test-service", 1.0, nil)

	span, _ := tracer.StartSpan(context.Background(), "test-op")
	// Should not panic with nil exporter
	tracer.FinishSpan(span)
}

func TestTracer_ExtractSpan(t *testing.T) {
	tracer := NewTracer("test-service", 1.0, nil)

	t.Run("with valid headers", func(t *testing.T) {
		traceID := GenerateTraceID()
		spanID := GenerateSpanID()

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(TraceIDHeader, traceID.String())
		req.Header.Set(SpanIDHeader, spanID.String())
		req.Header.Set(SampledHeader, "1")

		span := tracer.ExtractSpan(req)
		if span == nil {
			t.Fatal("expected non-nil span")
		}
		if span.TraceID != traceID {
			t.Error("trace ID mismatch")
		}
		if span.ParentSpanID != spanID {
			t.Error("parent span ID mismatch")
		}
		if !span.Sampled {
			t.Error("expected sampled=true")
		}
	})

	t.Run("with sampled=0", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(TraceIDHeader, GenerateTraceID().String())
		req.Header.Set(SampledHeader, "0")

		span := tracer.ExtractSpan(req)
		if span == nil {
			t.Fatal("expected non-nil span")
		}
		if span.Sampled {
			t.Error("expected sampled=false")
		}
	})

	t.Run("no headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		span := tracer.ExtractSpan(req)
		if span != nil {
			t.Error("expected nil span for missing headers")
		}
	})

	t.Run("invalid trace ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(TraceIDHeader, "not-valid-hex")
		span := tracer.ExtractSpan(req)
		if span != nil {
			t.Error("expected nil span for invalid trace ID")
		}
	})
}

func TestTracer_InjectSpan(t *testing.T) {
	tracer := NewTracer("test-service", 1.0, nil)

	t.Run("with valid span", func(t *testing.T) {
		traceID := GenerateTraceID()
		parentID := GenerateSpanID()
		span := NewSpan("test", traceID, parentID)
		span.Sampled = true

		req := httptest.NewRequest("GET", "/test", nil)
		tracer.InjectSpan(span, req)

		if req.Header.Get(TraceIDHeader) != traceID.String() {
			t.Error("trace ID header mismatch")
		}
		if req.Header.Get(SpanIDHeader) != span.SpanID.String() {
			t.Error("span ID header mismatch")
		}
		if req.Header.Get(ParentSpanIDHeader) != parentID.String() {
			t.Error("parent span ID header mismatch")
		}
		if req.Header.Get(SampledHeader) != "1" {
			t.Error("expected sampled header = 1")
		}
	})

	t.Run("not sampled", func(t *testing.T) {
		span := NewSpan("test", GenerateTraceID(), SpanID{})
		span.Sampled = false

		req := httptest.NewRequest("GET", "/test", nil)
		tracer.InjectSpan(span, req)

		if req.Header.Get(SampledHeader) != "0" {
			t.Error("expected sampled header = 0")
		}
	})

	t.Run("zero parent span ID", func(t *testing.T) {
		span := NewSpan("test", GenerateTraceID(), SpanID{})

		req := httptest.NewRequest("GET", "/test", nil)
		tracer.InjectSpan(span, req)

		if req.Header.Get(ParentSpanIDHeader) != "" {
			t.Error("expected empty parent span ID header for zero parent")
		}
	})

	t.Run("nil span", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		// Should not panic
		tracer.InjectSpan(nil, req)
	})
}

func TestTracer_SampleRate(t *testing.T) {
	exporter := NewInMemoryExporter()
	// 100% sample rate
	tracer := NewTracer("test-service", 1.0, exporter)

	for i := 0; i < 10; i++ {
		span, _ := tracer.StartSpan(context.Background(), "op")
		tracer.FinishSpan(span)
	}

	spans := exporter.Spans()
	if len(spans) != 10 {
		t.Errorf("expected 10 spans with 100%% sample rate, got %d", len(spans))
	}
}

// ==================== InMemoryExporter Tests ====================

func TestInMemoryExporter(t *testing.T) {
	e := NewInMemoryExporter()

	span1 := NewSpan("op1", GenerateTraceID(), SpanID{})
	span2 := NewSpan("op2", GenerateTraceID(), SpanID{})

	if err := e.Export(span1); err != nil {
		t.Fatalf("unexpected export error: %v", err)
	}
	if err := e.Export(span2); err != nil {
		t.Fatalf("unexpected export error: %v", err)
	}

	spans := e.Spans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	e.Clear()
	spans = e.Spans()
	if len(spans) != 0 {
		t.Errorf("expected 0 spans after clear, got %d", len(spans))
	}

	if err := e.Close(); err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}

// ==================== ChannelExporter Tests ====================

func TestChannelExporter(t *testing.T) {
	e := NewChannelExporter(10)

	span := NewSpan("op", GenerateTraceID(), SpanID{})
	if err := e.Export(span); err != nil {
		t.Fatalf("unexpected export error: %v", err)
	}

	select {
	case got := <-e.Spans():
		if got.Name != "op" {
			t.Errorf("expected span name op, got %s", got.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for span")
	}

	if err := e.Close(); err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}

func TestChannelExporter_Full(t *testing.T) {
	e := NewChannelExporter(1)

	span1 := NewSpan("op1", GenerateTraceID(), SpanID{})
	span2 := NewSpan("op2", GenerateTraceID(), SpanID{})

	_ = e.Export(span1) // fills buffer
	err := e.Export(span2) // should drop (buffer full)
	if err != nil {
		t.Errorf("expected nil error on drop, got %v", err)
	}

	e.Close()
}

// ==================== AccessLogger Tests ====================

func TestDefaultAccessLogConfig(t *testing.T) {
	config := DefaultAccessLogConfig()
	if config.Format != FormatJSON {
		t.Errorf("expected FormatJSON, got %d", config.Format)
	}
	if config.BufferSize != 4096 {
		t.Errorf("expected buffer size 4096, got %d", config.BufferSize)
	}
	if !config.Async {
		t.Error("expected async=true")
	}
	if config.QueueSize != 10000 {
		t.Errorf("expected queue size 10000, got %d", config.QueueSize)
	}
}

func TestNewAccessLogger_NilConfig(t *testing.T) {
	logger := NewAccessLogger(nil)
	if logger == nil {
		t.Fatal("expected non-nil logger with nil config")
	}
	logger.Close()
}

func TestAccessLogger_SyncJSON(t *testing.T) {
	var buf bytes.Buffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatJSON,
		BufferSize: 1024,
		Async:      false,
	}
	logger := NewAccessLogger(config)

	entry := &AccessLogEntry{
		Timestamp:    time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		Method:       "GET",
		Path:         "/api/v1/test",
		Protocol:     "HTTP/1.1",
		Host:         "localhost",
		RemoteAddr:   "127.0.0.1",
		StatusCode:   200,
		RequestSize:  100,
		ResponseSize: 500,
		Latency:      50 * time.Millisecond,
	}

	logger.Log(entry)

	output := buf.String()
	if !strings.Contains(output, `"method":"GET"`) {
		t.Errorf("expected method in output, got: %s", output)
	}
	if !strings.Contains(output, `"path":"/api/v1/test"`) {
		t.Errorf("expected path in output, got: %s", output)
	}
	if !strings.Contains(output, `"status":200`) {
		t.Errorf("expected status in output, got: %s", output)
	}
	if !strings.Contains(output, `"latency_ms":50.000`) {
		t.Errorf("expected latency in output, got: %s", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Error("expected newline at end")
	}
}

func TestAccessLogger_JSONOptionalFields(t *testing.T) {
	var buf bytes.Buffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatJSON,
		BufferSize: 2048,
		Async:      false,
	}
	logger := NewAccessLogger(config)

	entry := &AccessLogEntry{
		Timestamp:         time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		Method:            "POST",
		Path:              "/api/v1/data",
		Protocol:          "HTTP/2",
		Host:              "example.com",
		RemoteAddr:        "10.0.0.1",
		StatusCode:        201,
		RequestSize:       1024,
		ResponseSize:      2048,
		Latency:           100 * time.Millisecond,
		UpstreamService:   "backend",
		UpstreamNamespace: "production",
		UpstreamAddress:   "10.10.10.20:8080",
		UpstreamLatency:   80 * time.Millisecond,
		PeerSPIFFEID:      "spiffe://domain/svc",
		TLSVersion:        "1.3",
		TraceID:           "abc123",
		SpanID:            "def456",
		Error:             "timeout",
		Retry:             2,
		UserAgent:         "test-agent/1.0",
	}

	logger.Log(entry)

	output := buf.String()
	checks := []string{
		`"upstream_service":"backend"`,
		`"upstream_namespace":"production"`,
		`"upstream_address":"10.10.10.20:8080"`,
		`"upstream_latency_ms":80.000`,
		`"peer_spiffe_id":"spiffe://domain/svc"`,
		`"tls_version":"1.3"`,
		`"trace_id":"abc123"`,
		`"span_id":"def456"`,
		`"error":"timeout"`,
		`"retry":2`,
		`"user_agent":"test-agent/1.0"`,
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("expected %s in output, got: %s", check, output)
		}
	}
}

func TestAccessLogger_CommonFormat(t *testing.T) {
	var buf bytes.Buffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatCommon,
		BufferSize: 1024,
		Async:      false,
	}
	logger := NewAccessLogger(config)

	entry := &AccessLogEntry{
		Timestamp:    time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC),
		Method:       "GET",
		Path:         "/index.html",
		Protocol:     "HTTP/1.1",
		RemoteAddr:   "192.168.1.1",
		StatusCode:   200,
		ResponseSize: 1234,
	}

	logger.Log(entry)

	output := buf.String()
	if !strings.Contains(output, "192.168.1.1") {
		t.Errorf("expected remote addr in common log, got: %s", output)
	}
	if !strings.Contains(output, `"GET /index.html HTTP/1.1"`) {
		t.Errorf("expected request line in common log, got: %s", output)
	}
	if !strings.Contains(output, "200") {
		t.Errorf("expected status in common log, got: %s", output)
	}
}

func TestAccessLogger_CombinedFormat(t *testing.T) {
	var buf bytes.Buffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatCombined,
		BufferSize: 1024,
		Async:      false,
	}
	logger := NewAccessLogger(config)

	entry := &AccessLogEntry{
		Timestamp:    time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC),
		Method:       "GET",
		Path:         "/index.html",
		Protocol:     "HTTP/1.1",
		RemoteAddr:   "192.168.1.1",
		StatusCode:   200,
		ResponseSize: 1234,
		UserAgent:    "Mozilla/5.0",
	}

	logger.Log(entry)

	output := buf.String()
	if !strings.Contains(output, "Mozilla/5.0") {
		t.Errorf("expected user agent in combined log, got: %s", output)
	}
}

func TestAccessLogger_AsyncMode(t *testing.T) {
	var buf safeBuffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatJSON,
		BufferSize: 1024,
		Async:      true,
		QueueSize:  100,
	}
	logger := NewAccessLogger(config)

	entry := &AccessLogEntry{
		Timestamp:  time.Now(),
		Method:     "GET",
		Path:       "/async",
		Protocol:   "HTTP/1.1",
		RemoteAddr: "127.0.0.1",
		StatusCode: 200,
	}

	logger.Log(entry)

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)
	logger.Close()

	output := buf.String()
	if !strings.Contains(output, "/async") {
		t.Errorf("expected async entry in output, got: %s", output)
	}
}

func TestAccessLogger_Filter(t *testing.T) {
	var buf bytes.Buffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatJSON,
		BufferSize: 1024,
		Async:      false,
		Filters: []AccessLogFilter{
			FilterErrors(),
		},
	}
	logger := NewAccessLogger(config)

	// Should be filtered out (200 < 400)
	logger.Log(&AccessLogEntry{
		Timestamp:  time.Now(),
		Method:     "GET",
		Path:       "/ok",
		StatusCode: 200,
	})

	// Should pass filter
	logger.Log(&AccessLogEntry{
		Timestamp:  time.Now(),
		Method:     "GET",
		Path:       "/error",
		StatusCode: 500,
	})

	output := buf.String()
	if strings.Contains(output, "/ok") {
		t.Error("expected /ok to be filtered out")
	}
	if !strings.Contains(output, "/error") {
		t.Error("expected /error in output")
	}
}

// ==================== Filter Tests ====================

func TestFilterByStatus(t *testing.T) {
	f := FilterByStatus(200, 299)

	if !f(&AccessLogEntry{StatusCode: 200}) {
		t.Error("expected 200 to pass")
	}
	if !f(&AccessLogEntry{StatusCode: 299}) {
		t.Error("expected 299 to pass")
	}
	if f(&AccessLogEntry{StatusCode: 300}) {
		t.Error("expected 300 to fail")
	}
	if f(&AccessLogEntry{StatusCode: 199}) {
		t.Error("expected 199 to fail")
	}
}

func TestFilterErrors(t *testing.T) {
	f := FilterErrors()

	if f(&AccessLogEntry{StatusCode: 200}) {
		t.Error("expected 200 to fail")
	}
	if f(&AccessLogEntry{StatusCode: 399}) {
		t.Error("expected 399 to fail")
	}
	if !f(&AccessLogEntry{StatusCode: 400}) {
		t.Error("expected 400 to pass")
	}
	if !f(&AccessLogEntry{StatusCode: 500}) {
		t.Error("expected 500 to pass")
	}
}

func TestFilterByLatency(t *testing.T) {
	f := FilterByLatency(100 * time.Millisecond)

	if f(&AccessLogEntry{Latency: 50 * time.Millisecond}) {
		t.Error("expected 50ms to fail")
	}
	if !f(&AccessLogEntry{Latency: 100 * time.Millisecond}) {
		t.Error("expected 100ms to pass")
	}
	if !f(&AccessLogEntry{Latency: 200 * time.Millisecond}) {
		t.Error("expected 200ms to pass")
	}
}

func TestFilterByPath(t *testing.T) {
	f := FilterByPath("/api/")

	if !f(&AccessLogEntry{Path: "/api/v1/test"}) {
		t.Error("expected /api/v1/test to pass")
	}
	if f(&AccessLogEntry{Path: "/health"}) {
		t.Error("expected /health to fail")
	}
	if f(&AccessLogEntry{Path: "/ap"}) {
		t.Error("expected short path to fail")
	}
}

func TestFilterExcludePath(t *testing.T) {
	f := FilterExcludePath("/health")

	if !f(&AccessLogEntry{Path: "/api/v1/test"}) {
		t.Error("expected /api/v1/test to pass")
	}
	if f(&AccessLogEntry{Path: "/health"}) {
		t.Error("expected /health to fail")
	}
	if !f(&AccessLogEntry{Path: "/he"}) {
		t.Error("expected short path to pass")
	}
}

func TestSampledFilter(t *testing.T) {
	f := SampledFilter(0.5) // Sample 50%

	passed := 0
	total := 100
	for i := 0; i < total; i++ {
		if f(&AccessLogEntry{StatusCode: 200}) {
			passed++
		}
	}

	// With 50% rate, every 2nd entry passes
	if passed != 50 {
		t.Errorf("expected 50 sampled entries, got %d", passed)
	}
}

func TestSampledFilter_FullRate(t *testing.T) {
	// rate >= 1.0 means interval=1, every entry passes
	f := SampledFilter(1.0)

	for i := 0; i < 10; i++ {
		if !f(&AccessLogEntry{StatusCode: 200}) {
			t.Error("expected all entries to pass with 100% rate")
		}
	}
}

// ==================== BufferedAccessLogger Tests ====================

func TestBufferedAccessLogger(t *testing.T) {
	var buf safeBuffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatJSON,
		BufferSize: 1024,
		Async:      false,
	}

	bl := NewBufferedAccessLogger(config, 5, 100*time.Millisecond)

	for i := 0; i < 3; i++ {
		bl.Log(&AccessLogEntry{
			Timestamp:  time.Now(),
			Method:     "GET",
			Path:       fmt.Sprintf("/test/%d", i),
			Protocol:   "HTTP/1.1",
			RemoteAddr: "127.0.0.1",
			StatusCode: 200,
		})
	}

	// Not yet flushed (batch size is 5)
	if buf.Len() != 0 {
		t.Errorf("expected no output before flush, got %d bytes", buf.Len())
	}

	bl.Flush()

	output := buf.String()
	if !strings.Contains(output, "/test/0") {
		t.Errorf("expected /test/0 in output after flush, got: %s", output)
	}

	bl.Close()
}

func TestBufferedAccessLogger_AutoFlushOnSize(t *testing.T) {
	var buf safeBuffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatJSON,
		BufferSize: 1024,
		Async:      false,
	}

	bl := NewBufferedAccessLogger(config, 2, 10*time.Second)

	bl.Log(&AccessLogEntry{
		Timestamp:  time.Now(),
		Method:     "GET",
		Path:       "/a",
		StatusCode: 200,
	})
	bl.Log(&AccessLogEntry{
		Timestamp:  time.Now(),
		Method:     "GET",
		Path:       "/b",
		StatusCode: 200,
	})

	// Give the flush a moment
	time.Sleep(10 * time.Millisecond)

	output := buf.String()
	if !strings.Contains(output, "/a") || !strings.Contains(output, "/b") {
		t.Errorf("expected both entries after auto flush, got: %s", output)
	}

	bl.Close()
}

func TestBufferedAccessLogger_TimerFlush(t *testing.T) {
	var buf safeBuffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatJSON,
		BufferSize: 1024,
		Async:      false,
	}

	bl := NewBufferedAccessLogger(config, 100, 50*time.Millisecond)

	bl.Log(&AccessLogEntry{
		Timestamp:  time.Now(),
		Method:     "GET",
		Path:       "/timer",
		StatusCode: 200,
	})

	// Wait for timer flush
	time.Sleep(100 * time.Millisecond)

	output := buf.String()
	if !strings.Contains(output, "/timer") {
		t.Errorf("expected /timer in output after timer flush, got: %s", output)
	}

	bl.Close()
}

// ==================== JSON Escaping Tests ====================

func TestAccessLogger_JSONEscaping(t *testing.T) {
	var buf bytes.Buffer
	config := &AccessLogConfig{
		Writer:     &buf,
		Format:     FormatJSON,
		BufferSize: 1024,
		Async:      false,
	}
	logger := NewAccessLogger(config)

	entry := &AccessLogEntry{
		Timestamp:  time.Now(),
		Method:     "GET",
		Path:       "/test?q=hello\"world",
		Protocol:   "HTTP/1.1",
		RemoteAddr: "127.0.0.1",
		StatusCode: 200,
		UserAgent:  "agent\\with\\backslash",
		Error:      "line1\nline2",
	}

	logger.Log(entry)

	output := buf.String()
	if !strings.Contains(output, `hello\"world`) {
		t.Errorf("expected escaped quotes in path, got: %s", output)
	}
	if !strings.Contains(output, `agent\\with\\backslash`) {
		t.Errorf("expected escaped backslashes, got: %s", output)
	}
	if !strings.Contains(output, `line1\nline2`) {
		t.Errorf("expected escaped newline, got: %s", output)
	}
}

// ==================== Header Constants Test ====================

func TestTraceHeaderConstants(t *testing.T) {
	if TraceIDHeader != "X-Trace-ID" {
		t.Errorf("unexpected TraceIDHeader: %s", TraceIDHeader)
	}
	if SpanIDHeader != "X-Span-ID" {
		t.Errorf("unexpected SpanIDHeader: %s", SpanIDHeader)
	}
	if ParentSpanIDHeader != "X-Parent-Span-ID" {
		t.Errorf("unexpected ParentSpanIDHeader: %s", ParentSpanIDHeader)
	}
	if SampledHeader != "X-Sampled" {
		t.Errorf("unexpected SampledHeader: %s", SampledHeader)
	}
}

// ==================== Inject/Extract Round Trip ====================

func TestTracer_InjectExtractRoundTrip(t *testing.T) {
	tracer := NewTracer("test-service", 1.0, nil)

	origSpan, _ := tracer.StartSpan(context.Background(), "round-trip")

	req := httptest.NewRequest("GET", "/test", nil)
	tracer.InjectSpan(origSpan, req)

	extractedSpan := tracer.ExtractSpan(req)
	if extractedSpan == nil {
		t.Fatal("expected non-nil extracted span")
	}
	if extractedSpan.TraceID != origSpan.TraceID {
		t.Error("trace ID should match after round-trip")
	}
	if extractedSpan.ParentSpanID != origSpan.SpanID {
		t.Error("parent span ID should be original span ID")
	}
}

// ==================== Helpers ====================

// safeBuffer is a thread-safe bytes.Buffer for async logger tests.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *safeBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

func (sb *safeBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Len()
}

// Ensure httptest is used (import validation).
var _ *http.Request = httptest.NewRequest("GET", "/", nil)
