// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// TEST HELPERS & MOCKS
// ============================================================================

// mockExporter records exported spans and allows error injection.
type mockExporter struct {
	mu          sync.Mutex
	spans       []*Span
	batchCalls  int
	shutdownErr error
	shutdownCnt int32
}

func newMockExporter() *mockExporter {
	return &mockExporter{}
}

func (m *mockExporter) ExportSpan(span *Span) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spans = append(m.spans, span)
}

func (m *mockExporter) ExportSpans(spans []*Span) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchCalls++
	m.spans = append(m.spans, spans...)
}

func (m *mockExporter) Shutdown(_ context.Context) error {
	atomic.AddInt32(&m.shutdownCnt, 1)
	return m.shutdownErr
}

func (m *mockExporter) getSpans() []*Span {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*Span, len(m.spans))
	copy(cp, m.spans)
	return cp
}

func (m *mockExporter) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.spans)
}

// failingExporter always returns an error on Shutdown.
type failingExporter struct{}

func (f *failingExporter) ExportSpan(_ *Span)    {}
func (f *failingExporter) ExportSpans(_ []*Span) {}
func (f *failingExporter) Shutdown(_ context.Context) error {
	return errors.New("shutdown failed")
}

// ============================================================================
// TRACE ID TESTS
// ============================================================================

func TestTraceID_Generate(t *testing.T) {
	t.Run("generates non-zero ID", func(t *testing.T) {
		id := GenerateTraceID()
		if !id.IsValid() {
			t.Fatal("generated trace ID should be valid (non-zero)")
		}
	})

	t.Run("uniqueness across 1000 IDs", func(t *testing.T) {
		seen := make(map[TraceID]struct{}, 1000)
		for i := 0; i < 1000; i++ {
			id := GenerateTraceID()
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate trace ID at iteration %d", i)
			}
			seen[id] = struct{}{}
		}
	})

	t.Run("string round-trip", func(t *testing.T) {
		original := GenerateTraceID()
		s := original.String()

		if len(s) != 32 {
			t.Fatalf("expected 32 hex chars, got %d", len(s))
		}

		var parsed TraceID
		if err := parsed.FromString(s); err != nil {
			t.Fatalf("FromString failed: %v", err)
		}
		if parsed != original {
			t.Fatalf("round-trip mismatch: %s vs %s", original, parsed)
		}
	})
}

func TestTraceID_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		id    TraceID
		valid bool
	}{
		{"zero is invalid", TraceID{}, false},
		{"non-zero is valid", TraceID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, true},
		{"all-ones is valid", TraceID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.id.IsValid() != tc.valid {
				t.Errorf("IsValid() = %v, want %v", tc.id.IsValid(), tc.valid)
			}
		})
	}
}

func TestTraceID_FromString_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"too short", "abcdef"},
		{"too long", "00112233445566778899aabbccddeeff00"},
		{"not hex", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{"empty", ""},
		{"31 chars", "0011223344556677889900aabbccddeef"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var id TraceID
			if err := id.FromString(tc.input); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestTraceID_JSON(t *testing.T) {
	original := GenerateTraceID()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed TraceID
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if original != parsed {
		t.Fatalf("JSON round-trip mismatch: %s vs %s", original, parsed)
	}
}

func TestTraceID_JSON_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not a string", `12345`},
		{"bad hex", `"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"`},
		{"wrong length", `"abcdef"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var id TraceID
			if err := json.Unmarshal([]byte(tc.input), &id); err == nil {
				t.Error("expected JSON unmarshal error, got nil")
			}
		})
	}
}

// ============================================================================
// SPAN ID TESTS
// ============================================================================

func TestSpanID_Generate(t *testing.T) {
	t.Run("generates non-zero ID", func(t *testing.T) {
		id := GenerateSpanID()
		if !id.IsValid() {
			t.Fatal("generated span ID should be valid (non-zero)")
		}
	})

	t.Run("uniqueness across 1000 IDs", func(t *testing.T) {
		seen := make(map[SpanID]struct{}, 1000)
		for i := 0; i < 1000; i++ {
			id := GenerateSpanID()
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate span ID at iteration %d", i)
			}
			seen[id] = struct{}{}
		}
	})

	t.Run("string round-trip", func(t *testing.T) {
		original := GenerateSpanID()
		s := original.String()

		if len(s) != 16 {
			t.Fatalf("expected 16 hex chars, got %d", len(s))
		}

		var parsed SpanID
		if err := parsed.FromString(s); err != nil {
			t.Fatalf("FromString failed: %v", err)
		}
		if parsed != original {
			t.Fatalf("round-trip mismatch: %s vs %s", original, parsed)
		}
	})
}

func TestSpanID_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		id    SpanID
		valid bool
	}{
		{"zero is invalid", SpanID{}, false},
		{"non-zero is valid", SpanID{0, 0, 0, 0, 0, 0, 0, 1}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.id.IsValid() != tc.valid {
				t.Errorf("IsValid() = %v, want %v", tc.id.IsValid(), tc.valid)
			}
		})
	}
}

func TestSpanID_FromString_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"too short", "abcd"},
		{"too long", "00112233445566778899"},
		{"not hex", "zzzzzzzzzzzzzzzz"},
		{"empty", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var id SpanID
			if err := id.FromString(tc.input); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestSpanID_JSON(t *testing.T) {
	original := GenerateSpanID()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed SpanID
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if original != parsed {
		t.Fatalf("JSON round-trip mismatch: %s vs %s", original, parsed)
	}
}

func TestSpanID_JSON_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not a string", `67890`},
		{"bad hex", `"zzzzzzzzzzzzzzzz"`},
		{"wrong length", `"abcd"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var id SpanID
			if err := json.Unmarshal([]byte(tc.input), &id); err == nil {
				t.Error("expected JSON unmarshal error, got nil")
			}
		})
	}
}

// ============================================================================
// SPAN KIND & STATUS CODE TESTS
// ============================================================================

func TestSpanKind_String(t *testing.T) {
	tests := []struct {
		kind SpanKind
		want string
	}{
		{SpanKindInternal, "internal"},
		{SpanKindServer, "server"},
		{SpanKindClient, "client"},
		{SpanKindProducer, "producer"},
		{SpanKindConsumer, "consumer"},
		{SpanKind(99), "internal"}, // unknown defaults to internal
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.kind.String(); got != tc.want {
				t.Errorf("SpanKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

func TestStatusCode_String(t *testing.T) {
	tests := []struct {
		code StatusCode
		want string
	}{
		{StatusUnset, "unset"},
		{StatusOK, "ok"},
		{StatusError, "error"},
		{StatusCode(99), "unset"}, // unknown defaults to unset
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.code.String(); got != tc.want {
				t.Errorf("StatusCode(%d).String() = %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}

// ============================================================================
// SPAN CREATION TESTS
// ============================================================================

func TestSpan_BasicCreation(t *testing.T) {
	exporter := newMockExporter()
	tracer := &Tracer{
		serviceName: "test-service",
		resource:    map[string]string{"env": "test"},
		exporter:    exporter,
	}

	ctx := context.Background()
	ctx, span := tracer.StartSpan(ctx, "test-operation")

	if span == nil {
		t.Fatal("StartSpan returned nil span")
	}
	if span.Name() != "test-operation" {
		t.Errorf("Name() = %q, want %q", span.Name(), "test-operation")
	}
	if !span.TraceID().IsValid() {
		t.Error("span should have a valid trace ID")
	}
	if !span.SpanID().IsValid() {
		t.Error("span should have a valid span ID")
	}
	if span.ParentSpanID().IsValid() {
		t.Error("root span should have zero parent span ID")
	}
	if span.Kind() != SpanKindInternal {
		t.Errorf("Kind() = %v, want SpanKindInternal", span.Kind())
	}
	if span.StartTime().IsZero() {
		t.Error("StartTime should not be zero")
	}
	if span.IsEnded() {
		t.Error("span should not be ended before End() is called")
	}

	// Verify span is in context
	extracted := SpanFromContext(ctx)
	if extracted != span {
		t.Error("SpanFromContext should return the same span")
	}
}

func TestSpan_WithKind(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	kinds := []SpanKind{SpanKindServer, SpanKindClient, SpanKindProducer, SpanKindConsumer, SpanKindInternal}

	for _, kind := range kinds {
		t.Run(kind.String(), func(t *testing.T) {
			_, span := tracer.StartSpan(context.Background(), "op", WithSpanKind(kind))
			if span.Kind() != kind {
				t.Errorf("Kind() = %v, want %v", span.Kind(), kind)
			}
			span.End()
		})
	}
}

func TestSpan_WithTraceID(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	customID := GenerateTraceID()

	_, span := tracer.StartSpan(context.Background(), "op", WithTraceID(customID))
	if span.TraceID() != customID {
		t.Errorf("TraceID() = %s, want %s", span.TraceID(), customID)
	}
	span.End()
}

func TestSpan_WithAttributes(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	attrs := map[string]string{"http.method": "GET", "http.url": "/api/v1/users"}

	_, span := tracer.StartSpan(context.Background(), "op", WithAttributes(attrs))
	got := span.Attributes()
	for k, v := range attrs {
		if got[k] != v {
			t.Errorf("Attributes()[%q] = %q, want %q", k, got[k], v)
		}
	}
	span.End()
}

func TestSpan_SetAttribute(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")

	span.SetAttribute("key1", "value1")
	span.SetAttribute("key2", "value2")

	attrs := span.Attributes()
	if attrs["key1"] != "value1" || attrs["key2"] != "value2" {
		t.Errorf("unexpected attributes: %v", attrs)
	}
	span.End()
}

func TestSpan_SetAttributes_Merges(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")

	span.SetAttributes(map[string]string{"a": "1", "b": "2"})
	span.SetAttributes(map[string]string{"b": "updated", "c": "3"})

	attrs := span.Attributes()
	if attrs["a"] != "1" || attrs["b"] != "updated" || attrs["c"] != "3" {
		t.Errorf("merge failed: %v", attrs)
	}
	span.End()
}

func TestSpan_Attributes_ReturnsCopy(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")
	span.SetAttribute("key", "original")

	attrs := span.Attributes()
	attrs["key"] = "mutated" // mutate the copy

	// Original should be unchanged
	if span.Attributes()["key"] != "original" {
		t.Error("Attributes() should return a copy; mutation leaked")
	}
	span.End()
}

// ============================================================================
// SPAN END TESTS
// ============================================================================

func TestSpan_End(t *testing.T) {
	exporter := newMockExporter()
	tracer := &Tracer{serviceName: "svc", exporter: exporter}
	_, span := tracer.StartSpan(context.Background(), "op")

	span.End()

	if !span.IsEnded() {
		t.Error("span should be ended after End()")
	}
	if span.EndTime().IsZero() {
		t.Error("EndTime should be set after End()")
	}
	if span.Duration() <= 0 {
		t.Error("Duration should be positive after End()")
	}
	if exporter.count() != 1 {
		t.Errorf("expected 1 exported span, got %d", exporter.count())
	}
}

func TestSpan_End_Idempotent(t *testing.T) {
	exporter := newMockExporter()
	tracer := &Tracer{serviceName: "svc", exporter: exporter}
	_, span := tracer.StartSpan(context.Background(), "op")

	span.End()
	span.End() // second call should be no-op
	span.End() // third call should be no-op

	if exporter.count() != 1 {
		t.Errorf("End() called 3 times but expected 1 export, got %d", exporter.count())
	}
}

func TestSpan_NoMutationAfterEnd(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")
	span.SetAttribute("before", "end")
	span.End()

	// These should be no-ops
	span.SetAttribute("after", "end")
	span.AddEvent("should-not-appear", nil)
	span.SetStatus(StatusError, "late error")

	attrs := span.Attributes()
	if _, ok := attrs["after"]; ok {
		t.Error("SetAttribute after End() should be ignored")
	}
	if len(span.Events()) != 0 {
		t.Error("AddEvent after End() should be ignored")
	}
	if span.StatusCode() != StatusUnset {
		t.Error("SetStatus after End() should be ignored")
	}
}

func TestSpan_End_NilExporter(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: nil}
	_, span := tracer.StartSpan(context.Background(), "op")
	// Should not panic with nil exporter
	span.End()
	if !span.IsEnded() {
		t.Error("span should be ended")
	}
}

// ============================================================================
// SPAN STATUS TESTS
// ============================================================================

func TestSpan_SetStatus(t *testing.T) {
	tests := []struct {
		name string
		code StatusCode
		msg  string
	}{
		{"ok", StatusOK, ""},
		{"error", StatusError, "something broke"},
		{"unset", StatusUnset, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
			_, span := tracer.StartSpan(context.Background(), "op")

			span.SetStatus(tc.code, tc.msg)

			if span.StatusCode() != tc.code {
				t.Errorf("StatusCode() = %v, want %v", span.StatusCode(), tc.code)
			}
			if span.StatusMessage() != tc.msg {
				t.Errorf("StatusMessage() = %q, want %q", span.StatusMessage(), tc.msg)
			}
			span.End()
		})
	}
}

func TestSpan_RecordError(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")

	testErr := errors.New("test error")
	span.RecordError(testErr)

	if span.StatusCode() != StatusError {
		t.Errorf("StatusCode() = %v, want StatusError", span.StatusCode())
	}
	if span.StatusMessage() != "test error" {
		t.Errorf("StatusMessage() = %q, want %q", span.StatusMessage(), "test error")
	}

	events := span.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "exception" {
		t.Errorf("event name = %q, want %q", events[0].Name, "exception")
	}
	if events[0].Attributes["exception.message"] != "test error" {
		t.Errorf("exception.message = %q, want %q", events[0].Attributes["exception.message"], "test error")
	}
	span.End()
}

func TestSpan_RecordError_Nil(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")

	span.RecordError(nil) // should be no-op

	if span.StatusCode() != StatusUnset {
		t.Error("RecordError(nil) should not change status")
	}
	if len(span.Events()) != 0 {
		t.Error("RecordError(nil) should not add events")
	}
	span.End()
}

// ============================================================================
// SPAN EVENT TESTS
// ============================================================================

func TestSpan_AddEvent(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")

	span.AddEvent("checkpoint-1", nil)
	span.AddEvent("checkpoint-2", map[string]string{"detail": "value"})

	events := span.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Name != "checkpoint-1" {
		t.Errorf("event[0].Name = %q, want %q", events[0].Name, "checkpoint-1")
	}
	if events[1].Name != "checkpoint-2" {
		t.Errorf("event[1].Name = %q, want %q", events[1].Name, "checkpoint-2")
	}
	if events[1].Attributes["detail"] != "value" {
		t.Errorf("event[1] attribute 'detail' = %q, want %q", events[1].Attributes["detail"], "value")
	}
	if events[0].Timestamp.IsZero() {
		t.Error("event timestamp should not be zero")
	}
	span.End()
}

func TestSpan_Events_ReturnsCopy(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")
	span.AddEvent("original", nil)

	events := span.Events()
	events[0].Name = "mutated"

	// Original should be unchanged
	if span.Events()[0].Name != "original" {
		t.Error("Events() should return a copy; mutation leaked")
	}
	span.End()
}

// ============================================================================
// CONTEXT PROPAGATION TESTS
// ============================================================================

func TestContextWithSpan_And_SpanFromContext(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	ctx := context.Background()

	// No span in fresh context
	if got := SpanFromContext(ctx); got != nil {
		t.Error("SpanFromContext on fresh context should return nil")
	}

	// With span
	ctx, span := tracer.StartSpan(ctx, "parent")
	if got := SpanFromContext(ctx); got != span {
		t.Error("SpanFromContext should return the stored span")
	}
	span.End()
}

func TestContext_ChildSpanInheritsTraceID(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	ctx := context.Background()

	ctx, parent := tracer.StartSpan(ctx, "parent")
	_, child := tracer.StartSpan(ctx, "child")

	if child.TraceID() != parent.TraceID() {
		t.Errorf("child trace ID %s should match parent %s", child.TraceID(), parent.TraceID())
	}
	if child.ParentSpanID() != parent.SpanID() {
		t.Errorf("child parent span ID %s should match parent span ID %s",
			child.ParentSpanID(), parent.SpanID())
	}
	if child.SpanID() == parent.SpanID() {
		t.Error("child and parent should have different span IDs")
	}

	child.End()
	parent.End()
}

func TestContext_DeepNesting(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	ctx := context.Background()

	const depth = 10
	spans := make([]*Span, depth)
	var rootTraceID TraceID

	for i := 0; i < depth; i++ {
		var span *Span
		ctx, span = tracer.StartSpan(ctx, fmt.Sprintf("span-%d", i))
		spans[i] = span
		if i == 0 {
			rootTraceID = span.TraceID()
		}
	}

	// All spans share the same trace ID
	for i, span := range spans {
		if span.TraceID() != rootTraceID {
			t.Errorf("span[%d] trace ID %s differs from root %s", i, span.TraceID(), rootTraceID)
		}
	}

	// Each span (except root) has parent = previous span
	for i := 1; i < depth; i++ {
		if spans[i].ParentSpanID() != spans[i-1].SpanID() {
			t.Errorf("span[%d] parent should be span[%d]", i, i-1)
		}
	}

	// Clean up
	for i := depth - 1; i >= 0; i-- {
		spans[i].End()
	}
}

func TestContext_CancelledContext(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Should still work even with cancelled context
	_, span := tracer.StartSpan(ctx, "op")
	if span == nil {
		t.Fatal("StartSpan should succeed even with cancelled context")
	}
	span.End()
}

// ============================================================================
// W3C TRACE CONTEXT PROPAGATION TESTS
// ============================================================================

func TestInjectHTTP(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	ctx, span := tracer.StartSpan(context.Background(), "op")

	headers := make(http.Header)
	InjectHTTP(ctx, headers)

	tp := headers.Get(traceparentHeader)
	if tp == "" {
		t.Fatal("Traceparent header should be set")
	}

	expected := fmt.Sprintf("00-%s-%s-01", span.TraceID().String(), span.SpanID().String())
	if tp != expected {
		t.Errorf("Traceparent = %q, want %q", tp, expected)
	}
	span.End()
}

func TestInjectHTTP_NoSpanInContext(t *testing.T) {
	headers := make(http.Header)
	InjectHTTP(context.Background(), headers)

	if tp := headers.Get(traceparentHeader); tp != "" {
		t.Errorf("Traceparent should be empty when no span in context, got %q", tp)
	}
}

func TestExtractHTTP(t *testing.T) {
	traceID := GenerateTraceID()
	spanID := GenerateSpanID()
	tp := fmt.Sprintf("00-%s-%s-01", traceID.String(), spanID.String())

	headers := make(http.Header)
	headers.Set(traceparentHeader, tp)

	gotTraceID, gotSpanID, err := ExtractHTTP(headers)
	if err != nil {
		t.Fatalf("ExtractHTTP failed: %v", err)
	}
	if gotTraceID != traceID {
		t.Errorf("TraceID = %s, want %s", gotTraceID, traceID)
	}
	if gotSpanID != spanID {
		t.Errorf("SpanID = %s, want %s", gotSpanID, spanID)
	}
}

func TestExtractHTTP_NoHeader(t *testing.T) {
	headers := make(http.Header)
	traceID, spanID, err := ExtractHTTP(headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if traceID.IsValid() || spanID.IsValid() {
		t.Error("expected zero IDs when no header present")
	}
}

func TestParseTraceparent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", false},
		{"too few parts", "00-abc-01", true},
		{"bad trace ID", "00-xyz-b7ad6b7169203331-01", true},
		{"bad span ID", "00-0af7651916cd43dd8448eb211c80319c-xyz-01", true},
		{"empty string", "", true},
		{"five parts", "00-aa-bb-cc-dd", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ParseTraceparent(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseTraceparent(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestFormatTraceparent(t *testing.T) {
	traceID := GenerateTraceID()
	spanID := GenerateSpanID()

	tp := FormatTraceparent(traceID, spanID)

	gotTraceID, gotSpanID, err := ParseTraceparent(tp)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	if gotTraceID != traceID {
		t.Errorf("trace ID mismatch after round-trip")
	}
	if gotSpanID != spanID {
		t.Errorf("span ID mismatch after round-trip")
	}
}

func TestHTTPPropagation_RoundTrip(t *testing.T) {
	exporter := newMockExporter()
	serverTracer := &Tracer{serviceName: "server", exporter: exporter}
	clientTracer := &Tracer{serviceName: "client", exporter: exporter}

	// Client creates a span and injects into outgoing request
	clientCtx, clientSpan := clientTracer.StartSpan(context.Background(), "client-call",
		WithSpanKind(SpanKindClient))
	outHeaders := make(http.Header)
	InjectHTTP(clientCtx, outHeaders)

	// Server extracts from incoming request
	inTraceID, inSpanID, err := ExtractHTTP(outHeaders)
	if err != nil {
		t.Fatalf("ExtractHTTP failed: %v", err)
	}

	// Server creates a child span using extracted trace context
	serverCtx := context.Background()
	// First put a synthetic parent span in context for the trace to link
	parentSpan := &Span{
		traceID: inTraceID,
		spanID:  inSpanID,
	}
	serverCtx = ContextWithSpan(serverCtx, parentSpan)

	_, serverSpan := serverTracer.StartSpan(serverCtx, "server-handler",
		WithSpanKind(SpanKindServer))

	// Verify propagation
	if serverSpan.TraceID() != clientSpan.TraceID() {
		t.Error("server span should share client's trace ID")
	}
	if serverSpan.ParentSpanID() != clientSpan.SpanID() {
		t.Error("server span's parent should be client span")
	}

	serverSpan.End()
	clientSpan.End()
}

// ============================================================================
// TRACER PROVIDER TESTS
// ============================================================================

func TestTracerProvider_Basic(t *testing.T) {
	exporter := newMockExporter()
	provider := NewTracerProvider(TracerProviderConfig{
		ServiceName: "my-service",
		Resource:    map[string]string{"env": "test"},
		Exporter:    exporter,
	})

	tracer := provider.Tracer("")
	if tracer == nil {
		t.Fatal("Tracer() returned nil")
	}

	ctx, span := tracer.StartSpan(context.Background(), "op")
	if span == nil {
		t.Fatal("StartSpan returned nil")
	}
	span.End()

	if exporter.count() != 1 {
		t.Errorf("expected 1 export, got %d", exporter.count())
	}

	// Check that span is in context
	if SpanFromContext(ctx) != span {
		t.Error("span not found in context")
	}
}

func TestTracerProvider_CustomTracerName(t *testing.T) {
	exporter := newMockExporter()
	provider := NewTracerProvider(TracerProviderConfig{
		ServiceName: "default",
		Exporter:    exporter,
	})

	tracer := provider.Tracer("custom-component")
	_, span := tracer.StartSpan(context.Background(), "op")
	span.End()

	exported := exporter.getSpans()
	data := exported[0].ToExportData()
	if data.ServiceName != "custom-component" {
		t.Errorf("ServiceName = %q, want %q", data.ServiceName, "custom-component")
	}
}

func TestTracerProvider_EmptyTracerName_UsesDefault(t *testing.T) {
	exporter := newMockExporter()
	provider := NewTracerProvider(TracerProviderConfig{
		ServiceName: "my-service",
		Exporter:    exporter,
	})

	tracer := provider.Tracer("")
	_, span := tracer.StartSpan(context.Background(), "op")
	span.End()

	exported := exporter.getSpans()
	data := exported[0].ToExportData()
	if data.ServiceName != "my-service" {
		t.Errorf("ServiceName = %q, want %q", data.ServiceName, "my-service")
	}
}

func TestTracerProvider_Shutdown(t *testing.T) {
	exporter := newMockExporter()
	provider := NewTracerProvider(TracerProviderConfig{
		Exporter: exporter,
	})

	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Double shutdown should return error
	if err := provider.Shutdown(context.Background()); err != ErrProviderClosed {
		t.Errorf("double Shutdown should return ErrProviderClosed, got %v", err)
	}
}

func TestTracerProvider_Shutdown_WithError(t *testing.T) {
	provider := NewTracerProvider(TracerProviderConfig{
		Exporter: &failingExporter{},
	})

	if err := provider.Shutdown(context.Background()); err == nil {
		t.Error("expected shutdown error from failing exporter")
	}
}

func TestTracerProvider_Shutdown_NilExporter(t *testing.T) {
	provider := NewTracerProvider(TracerProviderConfig{
		Exporter: nil,
	})

	if err := provider.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown with nil exporter should succeed, got %v", err)
	}
}

// ============================================================================
// EXPORT DATA TESTS
// ============================================================================

func TestSpan_ToExportData(t *testing.T) {
	exporter := newMockExporter()
	tracer := &Tracer{
		serviceName: "export-test",
		resource:    map[string]string{"version": "1.0"},
		exporter:    exporter,
	}

	_, span := tracer.StartSpan(context.Background(), "export-op",
		WithSpanKind(SpanKindServer),
		WithAttributes(map[string]string{"http.method": "GET"}),
	)
	span.AddEvent("started", map[string]string{"step": "1"})
	span.SetStatus(StatusOK, "")
	span.End()

	data := span.ToExportData()

	if data.TraceID != span.TraceID() {
		t.Error("TraceID mismatch in export data")
	}
	if data.SpanID != span.SpanID() {
		t.Error("SpanID mismatch in export data")
	}
	if data.Name != "export-op" {
		t.Errorf("Name = %q, want %q", data.Name, "export-op")
	}
	if data.Kind != SpanKindServer {
		t.Errorf("Kind = %v, want SpanKindServer", data.Kind)
	}
	if data.StatusCode != StatusOK {
		t.Errorf("StatusCode = %v, want StatusOK", data.StatusCode)
	}
	if data.ServiceName != "export-test" {
		t.Errorf("ServiceName = %q, want %q", data.ServiceName, "export-test")
	}
	if data.Attributes["http.method"] != "GET" {
		t.Error("attribute http.method not exported")
	}
	if len(data.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(data.Events))
	}
	if data.Resource["version"] != "1.0" {
		t.Error("resource version not exported")
	}
}

func TestExportSpanData_JSON(t *testing.T) {
	tracer := &Tracer{serviceName: "json-test", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "json-op")
	span.SetAttribute("key", "value")
	span.AddEvent("evt", nil)
	span.End()

	data := span.ToExportData()

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ExportSpanData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Name != "json-op" {
		t.Errorf("decoded Name = %q, want %q", decoded.Name, "json-op")
	}
	if decoded.TraceID != data.TraceID {
		t.Error("decoded TraceID mismatch")
	}
	if decoded.SpanID != data.SpanID {
		t.Error("decoded SpanID mismatch")
	}
}

// ============================================================================
// IN-MEMORY EXPORTER TESTS
// ============================================================================

func TestInMemoryExporter_Basic(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := &Tracer{serviceName: "svc", exporter: exporter}

	_, s1 := tracer.StartSpan(context.Background(), "op1")
	s1.End()
	_, s2 := tracer.StartSpan(context.Background(), "op2")
	s2.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	if spans[0].Name != "op1" || spans[1].Name != "op2" {
		t.Error("span names mismatch")
	}
}

func TestInMemoryExporter_Reset(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := &Tracer{serviceName: "svc", exporter: exporter}

	_, span := tracer.StartSpan(context.Background(), "op")
	span.End()

	if len(exporter.GetSpans()) != 1 {
		t.Fatal("expected 1 span before reset")
	}

	exporter.Reset()

	if len(exporter.GetSpans()) != 0 {
		t.Error("expected 0 spans after reset")
	}
}

func TestInMemoryExporter_ExportSpans(t *testing.T) {
	exporter := NewInMemoryExporter()

	spans := make([]*Span, 5)
	for i := range spans {
		spans[i] = &Span{
			name:    fmt.Sprintf("span-%d", i),
			traceID: GenerateTraceID(),
			spanID:  GenerateSpanID(),
		}
	}

	exporter.ExportSpans(spans)

	got := exporter.GetSpans()
	if len(got) != 5 {
		t.Errorf("expected 5 spans, got %d", len(got))
	}
}

func TestInMemoryExporter_Shutdown_Error(t *testing.T) {
	exporter := NewInMemoryExporter()
	exporter.SetError(errors.New("forced error"))

	if err := exporter.Shutdown(context.Background()); err == nil {
		t.Error("expected error from Shutdown")
	}
}

func TestInMemoryExporter_GetSpans_ReturnsCopy(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := &Tracer{serviceName: "svc", exporter: exporter}
	_, span := tracer.StartSpan(context.Background(), "op")
	span.End()

	spans := exporter.GetSpans()
	spans[0] = ExportSpanData{Name: "mutated"} // mutate the copy

	// Original should be unchanged
	original := exporter.GetSpans()
	if original[0].Name != "op" {
		t.Error("GetSpans should return a copy")
	}
}

// ============================================================================
// OTLP EXPORTER MOCK TESTS
// ============================================================================

func TestOTLPExporter_SendsToEndpoint(t *testing.T) {
	var received []ExportSpanData
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-Custom") != "header-value" {
			t.Errorf("custom header not received")
		}

		var spans []ExportSpanData
		if err := json.NewDecoder(r.Body).Decode(&spans); err != nil {
			t.Errorf("failed to decode body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		received = append(received, spans...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	exporter := NewOTLPExporter(OTLPExporterConfig{
		Endpoint:  server.URL,
		Headers:   map[string]string{"X-Custom": "header-value"},
		Client:    server.Client(),
		BatchSize: 2,
	})

	tracer := &Tracer{serviceName: "otlp-test", exporter: exporter}

	// Create enough spans to trigger a flush (batchSize = 2)
	for i := 0; i < 2; i++ {
		_, span := tracer.StartSpan(context.Background(), fmt.Sprintf("op-%d", i))
		span.End()
	}

	// Wait a bit for the async flush
	time.Sleep(200 * time.Millisecond)

	// Shutdown to flush any remaining
	if err := exporter.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 spans received by server, got %d", count)
	}
}

func TestOTLPExporter_ShutdownFlushesRemaining(t *testing.T) {
	var received []ExportSpanData
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var spans []ExportSpanData
		json.NewDecoder(r.Body).Decode(&spans)
		mu.Lock()
		received = append(received, spans...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	exporter := NewOTLPExporter(OTLPExporterConfig{
		Endpoint:  server.URL,
		Client:    server.Client(),
		BatchSize: 100, // large batch so nothing auto-flushes
	})

	tracer := &Tracer{serviceName: "svc", exporter: exporter}
	_, span := tracer.StartSpan(context.Background(), "lingering")
	span.End()

	// Shutdown should flush the single span
	exporter.Shutdown(context.Background())

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 span flushed on shutdown, got %d", count)
	}
}

func TestOTLPExporter_DoubleShutdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	exporter := NewOTLPExporter(OTLPExporterConfig{
		Endpoint: server.URL,
		Client:   server.Client(),
	})

	exporter.Shutdown(context.Background())
	// Second shutdown should not panic
	exporter.Shutdown(context.Background())
}

func TestOTLPExporter_ExportAfterShutdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	exporter := NewOTLPExporter(OTLPExporterConfig{
		Endpoint: server.URL,
		Client:   server.Client(),
	})

	exporter.Shutdown(context.Background())

	// Should not panic
	span := &Span{
		name:    "after-shutdown",
		traceID: GenerateTraceID(),
		spanID:  GenerateSpanID(),
	}
	exporter.ExportSpan(span)
}

func TestOTLPExporter_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	exporter := NewOTLPExporter(OTLPExporterConfig{
		Endpoint:  server.URL,
		Client:    server.Client(),
		BatchSize: 1,
	})

	tracer := &Tracer{serviceName: "svc", exporter: exporter}
	_, span := tracer.StartSpan(context.Background(), "will-fail")
	span.End()

	// Wait for flush attempt
	time.Sleep(200 * time.Millisecond)

	// Should not panic; error is silently handled
	exporter.Shutdown(context.Background())
}

// ============================================================================
// CONCURRENT SAFETY TESTS
// ============================================================================

func TestSpan_ConcurrentAttributes(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "concurrent-attrs")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			span.SetAttribute(fmt.Sprintf("key-%d", n), fmt.Sprintf("val-%d", n))
		}(i)
	}
	wg.Wait()

	attrs := span.Attributes()
	if len(attrs) != 100 {
		t.Errorf("expected 100 attributes, got %d", len(attrs))
	}
	span.End()
}

func TestSpan_ConcurrentEvents(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "concurrent-events")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			span.AddEvent(fmt.Sprintf("event-%d", n), nil)
		}(i)
	}
	wg.Wait()

	events := span.Events()
	if len(events) != 100 {
		t.Errorf("expected 100 events, got %d", len(events))
	}
	span.End()
}

func TestSpan_ConcurrentEnd(t *testing.T) {
	exporter := newMockExporter()
	tracer := &Tracer{serviceName: "svc", exporter: exporter}
	_, span := tracer.StartSpan(context.Background(), "concurrent-end")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			span.End()
		}()
	}
	wg.Wait()

	// Only one export should occur
	if exporter.count() != 1 {
		t.Errorf("expected exactly 1 export from concurrent End(), got %d", exporter.count())
	}
}

func TestTracer_ConcurrentStartSpan(t *testing.T) {
	exporter := newMockExporter()
	tracer := &Tracer{serviceName: "svc", exporter: exporter}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, span := tracer.StartSpan(context.Background(), fmt.Sprintf("op-%d", n))
			span.SetAttribute("index", fmt.Sprintf("%d", n))
			span.End()
		}(i)
	}
	wg.Wait()

	if exporter.count() != 100 {
		t.Errorf("expected 100 exported spans, got %d", exporter.count())
	}
}

func TestTraceID_ConcurrentGeneration(t *testing.T) {
	const goroutines = 50
	const idsPerGoroutine = 100

	results := make(chan TraceID, goroutines*idsPerGoroutine)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				results <- GenerateTraceID()
			}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[TraceID]struct{})
	for id := range results {
		if !id.IsValid() {
			t.Error("generated invalid trace ID")
		}
		if _, dup := seen[id]; dup {
			t.Error("duplicate trace ID detected under concurrency")
		}
		seen[id] = struct{}{}
	}

	if len(seen) != goroutines*idsPerGoroutine {
		t.Errorf("expected %d unique IDs, got %d", goroutines*idsPerGoroutine, len(seen))
	}
}

func TestInMemoryExporter_ConcurrentExport(t *testing.T) {
	exporter := NewInMemoryExporter()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			span := &Span{
				name:    fmt.Sprintf("span-%d", n),
				traceID: GenerateTraceID(),
				spanID:  GenerateSpanID(),
			}
			exporter.ExportSpan(span)
		}(i)
	}
	wg.Wait()

	if len(exporter.GetSpans()) != 100 {
		t.Errorf("expected 100 spans, got %d", len(exporter.GetSpans()))
	}
}

// ============================================================================
// EDGE CASES & ERROR PATHS
// ============================================================================

func TestSpan_ZeroDuration_WhenNotEnded(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")

	if span.Duration() != 0 {
		t.Errorf("Duration() before End() should be 0, got %v", span.Duration())
	}
	if !span.EndTime().IsZero() {
		t.Error("EndTime() before End() should be zero")
	}
	span.End()
}

func TestSpanFromContext_NilContext(t *testing.T) {
	// While context.Background() != nil, a context with no span value
	// should return nil.
	ctx := context.Background()
	if span := SpanFromContext(ctx); span != nil {
		t.Error("expected nil span from empty context")
	}
}

func TestContextWithSpan_Overwrite(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}

	ctx, span1 := tracer.StartSpan(context.Background(), "first")
	ctx, span2 := tracer.StartSpan(ctx, "second")

	// Context should have the most recent span
	got := SpanFromContext(ctx)
	if got != span2 {
		t.Error("context should contain the most recently added span")
	}
	_ = span1
	span2.End()
	span1.End()
}

func TestTracer_EmptySpanName(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "")
	if span.Name() != "" {
		t.Errorf("Name() = %q, want empty string", span.Name())
	}
	span.End()
}

func TestTracer_NoExporter(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: nil}
	_, span := tracer.StartSpan(context.Background(), "op")
	// Should not panic
	span.End()
}

func TestSpan_RecordError_AfterEnd(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")
	span.End()

	// Should be no-op
	span.RecordError(errors.New("late error"))

	if span.StatusCode() != StatusUnset {
		t.Error("RecordError after End should be ignored")
	}
}

func TestWithTraceID_ZeroID_IsIgnored(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	zeroID := TraceID{}

	_, span := tracer.StartSpan(context.Background(), "op", WithTraceID(zeroID))

	// A zero TraceID is not valid, so the span should get an auto-generated one
	if !span.TraceID().IsValid() {
		t.Error("span should have a valid (auto-generated) trace ID when given zero")
	}
	span.End()
}

func TestWithAttributes_Nil(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op", WithAttributes(nil))
	// Should not panic and attributes should be empty
	attrs := span.Attributes()
	if len(attrs) != 0 {
		t.Errorf("expected 0 attributes, got %d", len(attrs))
	}
	span.End()
}

func TestSpan_SetAttributes_Nil(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "op")
	span.SetAttributes(nil) // should not panic
	span.End()
}

func TestParseTraceparent_ValidFormats(t *testing.T) {
	// Construct known-good trace and span IDs
	traceID := GenerateTraceID()
	spanID := GenerateSpanID()
	tp := fmt.Sprintf("00-%s-%s-01", traceID.String(), spanID.String())

	gotTrace, gotSpan, err := ParseTraceparent(tp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTrace != traceID || gotSpan != spanID {
		t.Error("parsed IDs do not match original")
	}
}

func TestExportSpanData_EmptyEvents(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "no-events")
	span.End()

	data := span.ToExportData()
	if len(data.Events) != 0 {
		t.Error("expected nil or empty events slice")
	}
}

func TestExportSpanData_EmptyAttributes(t *testing.T) {
	tracer := &Tracer{serviceName: "svc", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "no-attrs")
	span.End()

	data := span.ToExportData()
	// Attributes map is initialized but should be empty
	if len(data.Attributes) != 0 {
		t.Errorf("expected empty attributes, got %d", len(data.Attributes))
	}
}

// ============================================================================
// INTEGRATION-STYLE TESTS
// ============================================================================

func TestFullTracing_Workflow(t *testing.T) {
	exporter := NewInMemoryExporter()
	provider := NewTracerProvider(TracerProviderConfig{
		ServiceName: "integration-test",
		Resource:    map[string]string{"env": "test", "version": "0.1.0"},
		Exporter:    exporter,
	})

	tracer := provider.Tracer("")
	ctx := context.Background()

	// Simulate: HTTP request -> DB query -> cache check
	ctx, httpSpan := tracer.StartSpan(ctx, "HTTP GET /users",
		WithSpanKind(SpanKindServer),
		WithAttributes(map[string]string{
			"http.method": "GET",
			"http.url":    "/users",
		}),
	)

	ctx, dbSpan := tracer.StartSpan(ctx, "SELECT * FROM users",
		WithSpanKind(SpanKindClient),
		WithAttributes(map[string]string{
			"db.system": "sqlite",
			"db.name":   "main",
		}),
	)
	dbSpan.AddEvent("query.compiled", nil)
	dbSpan.SetStatus(StatusOK, "")
	dbSpan.End()

	_, cacheSpan := tracer.StartSpan(ctx, "cache.set",
		WithSpanKind(SpanKindInternal),
	)
	cacheSpan.SetAttribute("cache.key", "users:all")
	cacheSpan.SetStatus(StatusOK, "")
	cacheSpan.End()

	httpSpan.SetStatus(StatusOK, "")
	httpSpan.End()

	// Verify all spans were exported
	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	// All spans should share the same trace ID
	rootTraceID := spans[0].TraceID
	for i, s := range spans {
		if s.TraceID != rootTraceID {
			t.Errorf("span[%d] has different trace ID", i)
		}
	}

	// Verify parent-child relationships
	// dbSpan (exported first) should be child of httpSpan
	if spans[0].ParentSpanID != httpSpan.SpanID() {
		t.Error("DB span should be child of HTTP span")
	}
	// cacheSpan should be child of dbSpan (it was started in dbSpan's context,
	// but actually it was started after dbSpan ended -- let's verify parent)
	if spans[1].ParentSpanID != dbSpan.SpanID() {
		t.Error("cache span should be child of DB span (context at time of creation)")
	}

	// httpSpan should be root
	if httpSpan.ParentSpanID().IsValid() {
		t.Error("HTTP span should be root (no parent)")
	}

	// Shutdown
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestFullTracing_ErrorPropagation(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := &Tracer{serviceName: "err-test", exporter: exporter}

	ctx := context.Background()
	ctx, parent := tracer.StartSpan(ctx, "parent-op")
	_, child := tracer.StartSpan(ctx, "child-op")

	child.RecordError(errors.New("database connection failed"))
	child.End()

	parent.SetStatus(StatusError, "child operation failed")
	parent.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Both spans should indicate errors
	for _, s := range spans {
		if s.StatusCode != StatusError {
			t.Errorf("span %q should have error status", s.Name)
		}
	}

	// Child span should have an exception event
	childData := spans[0] // child ends first
	if len(childData.Events) == 0 {
		t.Error("child span should have exception event")
	}
}

func TestHTTPMiddleware_Simulation(t *testing.T) {
	exporter := NewInMemoryExporter()
	tracer := &Tracer{serviceName: "http-middleware", exporter: exporter}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace context from incoming request
		inTraceID, inSpanID, _ := ExtractHTTP(r.Header)

		ctx := r.Context()
		if inTraceID.IsValid() {
			// Create synthetic parent to carry the trace
			parentSpan := &Span{traceID: inTraceID, spanID: inSpanID}
			ctx = ContextWithSpan(ctx, parentSpan)
		}

		ctx, span := tracer.StartSpan(ctx, "handle-request",
			WithSpanKind(SpanKindServer),
		)
		defer span.End()

		span.SetAttribute("http.method", r.Method)
		span.SetAttribute("http.path", r.URL.Path)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Simulate request with traceparent header
	traceID := GenerateTraceID()
	spanID := GenerateSpanID()
	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	req.Header.Set("Traceparent", FormatTraceparent(traceID, spanID))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Span should inherit the incoming trace ID
	if spans[0].TraceID != traceID {
		t.Errorf("span trace ID %s should match incoming %s", spans[0].TraceID, traceID)
	}
	if spans[0].ParentSpanID != spanID {
		t.Errorf("span parent %s should match incoming span %s", spans[0].ParentSpanID, spanID)
	}
}

// ============================================================================
// BENCHMARKS
// ============================================================================

func BenchmarkGenerateTraceID(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			GenerateTraceID()
		}
	})
}

func BenchmarkGenerateSpanID(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			GenerateSpanID()
		}
	})
}

func BenchmarkStartSpan(b *testing.B) {
	exporter := newMockExporter()
	tracer := &Tracer{serviceName: "bench", exporter: exporter}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := tracer.StartSpan(ctx, "bench-op")
		span.End()
	}
}

func BenchmarkStartSpan_Parallel(b *testing.B) {
	exporter := newMockExporter()
	tracer := &Tracer{serviceName: "bench", exporter: exporter}
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, span := tracer.StartSpan(ctx, "bench-op")
			span.End()
		}
	})
}

func BenchmarkSpanSetAttribute(b *testing.B) {
	tracer := &Tracer{serviceName: "bench", exporter: newMockExporter()}
	_, span := tracer.StartSpan(context.Background(), "bench-op")
	defer span.End()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span.SetAttribute("key", "value")
	}
}

func BenchmarkInjectHTTP(b *testing.B) {
	tracer := &Tracer{serviceName: "bench", exporter: newMockExporter()}
	ctx, span := tracer.StartSpan(context.Background(), "bench-op")
	defer span.End()

	headers := make(http.Header)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		InjectHTTP(ctx, headers)
	}
}

func BenchmarkParseTraceparent(b *testing.B) {
	tp := FormatTraceparent(GenerateTraceID(), GenerateSpanID())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseTraceparent(tp)
	}
}
