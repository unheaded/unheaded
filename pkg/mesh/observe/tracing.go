// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package observe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// Trace header constants.
const (
	TraceIDHeader      = "X-Trace-ID"
	SpanIDHeader       = "X-Span-ID"
	ParentSpanIDHeader = "X-Parent-Span-ID"
	SampledHeader      = "X-Sampled"
)

// TraceID is a unique identifier for a trace.
type TraceID [16]byte

// SpanID is a unique identifier for a span.
type SpanID [8]byte

// String returns the hex representation of a TraceID.
func (t TraceID) String() string {
	return hex.EncodeToString(t[:])
}

// String returns the hex representation of a SpanID.
func (s SpanID) String() string {
	return hex.EncodeToString(s[:])
}

// ParseTraceID parses a trace ID from a hex string.
func ParseTraceID(s string) (TraceID, error) {
	var id TraceID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	if len(b) == 16 {
		copy(id[:], b)
	}
	return id, nil
}

// ParseSpanID parses a span ID from a hex string.
func ParseSpanID(s string) (SpanID, error) {
	var id SpanID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	if len(b) == 8 {
		copy(id[:], b)
	}
	return id, nil
}

// GenerateTraceID generates a new random trace ID.
func GenerateTraceID() TraceID {
	var id TraceID
	rand.Read(id[:])
	return id
}

// GenerateSpanID generates a new random span ID.
func GenerateSpanID() SpanID {
	var id SpanID
	rand.Read(id[:])
	return id
}

// Span represents a unit of work in a trace.
type Span struct {
	TraceID      TraceID
	SpanID       SpanID
	ParentSpanID SpanID
	Name         string
	Service      string
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	Status       SpanStatus
	Tags         map[string]string
	Logs         []SpanLog
	Sampled      bool

	mu sync.Mutex
}

// SpanStatus represents the status of a span.
type SpanStatus int

const (
	SpanStatusUnset SpanStatus = iota
	SpanStatusOK
	SpanStatusError
)

func (s SpanStatus) String() string {
	switch s {
	case SpanStatusOK:
		return "OK"
	case SpanStatusError:
		return "ERROR"
	default:
		return "UNSET"
	}
}

// SpanLog represents a log entry within a span.
type SpanLog struct {
	Timestamp time.Time
	Fields    map[string]string
}

// NewSpan creates a new span.
func NewSpan(name string, traceID TraceID, parentSpanID SpanID) *Span {
	return &Span{
		TraceID:      traceID,
		SpanID:       GenerateSpanID(),
		ParentSpanID: parentSpanID,
		Name:         name,
		StartTime:    time.Now(),
		Tags:         make(map[string]string),
		Logs:         make([]SpanLog, 0),
		Sampled:      true,
	}
}

// SetTag sets a tag on the span.
func (s *Span) SetTag(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tags[key] = value
}

// Log adds a log entry to the span.
func (s *Span) Log(fields map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Logs = append(s.Logs, SpanLog{
		Timestamp: time.Now(),
		Fields:    fields,
	})
}

// SetStatus sets the span status.
func (s *Span) SetStatus(status SpanStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
}

// Finish completes the span.
func (s *Span) Finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.EndTime = time.Now()
	s.Duration = s.EndTime.Sub(s.StartTime)
}

// Tracer provides distributed tracing functionality.
type Tracer struct {
	serviceName string
	sampleRate  float64
	exporter    SpanExporter

	mu    sync.RWMutex
	spans map[string]*Span // spanID -> span
}

// SpanExporter exports completed spans.
type SpanExporter interface {
	Export(span *Span) error
	Close() error
}

// NewTracer creates a new tracer.
func NewTracer(serviceName string, sampleRate float64, exporter SpanExporter) *Tracer {
	return &Tracer{
		serviceName: serviceName,
		sampleRate:  sampleRate,
		exporter:    exporter,
		spans:       make(map[string]*Span),
	}
}

// StartSpan starts a new span.
func (t *Tracer) StartSpan(ctx context.Context, name string) (*Span, context.Context) {
	// Get parent span from context if exists
	var traceID TraceID
	var parentSpanID SpanID

	if parent := SpanFromContext(ctx); parent != nil {
		traceID = parent.TraceID
		parentSpanID = parent.SpanID
	} else {
		traceID = GenerateTraceID()
	}

	span := NewSpan(name, traceID, parentSpanID)
	span.Service = t.serviceName
	span.Sampled = t.shouldSample()

	// Store span
	t.mu.Lock()
	t.spans[span.SpanID.String()] = span
	t.mu.Unlock()

	// Add to context
	ctx = ContextWithSpan(ctx, span)

	return span, ctx
}

// FinishSpan finishes and exports a span.
func (t *Tracer) FinishSpan(span *Span) {
	span.Finish()

	// Remove from active spans
	t.mu.Lock()
	delete(t.spans, span.SpanID.String())
	t.mu.Unlock()

	// Export if sampled
	if span.Sampled && t.exporter != nil {
		t.exporter.Export(span)
	}
}

// ExtractSpan extracts span context from HTTP headers.
func (t *Tracer) ExtractSpan(r *http.Request) *Span {
	traceIDStr := r.Header.Get(TraceIDHeader)
	spanIDStr := r.Header.Get(SpanIDHeader)
	sampledStr := r.Header.Get(SampledHeader)

	if traceIDStr == "" {
		return nil
	}

	traceID, err := ParseTraceID(traceIDStr)
	if err != nil {
		return nil
	}

	var parentSpanID SpanID
	if spanIDStr != "" {
		parentSpanID, _ = ParseSpanID(spanIDStr)
	}

	span := NewSpan("", traceID, parentSpanID)
	span.Sampled = sampledStr != "0"

	return span
}

// InjectSpan injects span context into HTTP headers.
func (t *Tracer) InjectSpan(span *Span, r *http.Request) {
	if span == nil {
		return
	}

	r.Header.Set(TraceIDHeader, span.TraceID.String())
	r.Header.Set(SpanIDHeader, span.SpanID.String())
	if !isZeroSpanID(span.ParentSpanID) {
		r.Header.Set(ParentSpanIDHeader, span.ParentSpanID.String())
	}
	if span.Sampled {
		r.Header.Set(SampledHeader, "1")
	} else {
		r.Header.Set(SampledHeader, "0")
	}
}

func (t *Tracer) shouldSample() bool {
	if t.sampleRate >= 1.0 {
		return true
	}
	if t.sampleRate <= 0.0 {
		return false
	}

	var b [1]byte
	rand.Read(b[:])
	return float64(b[0])/256.0 < t.sampleRate
}

func isZeroSpanID(id SpanID) bool {
	for _, b := range id {
		if b != 0 {
			return false
		}
	}
	return true
}

// Context key for spans.
type spanContextKey struct{}

// ContextWithSpan returns a context with the span attached.
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanContextKey{}, span)
}

// SpanFromContext extracts a span from the context.
func SpanFromContext(ctx context.Context) *Span {
	if span, ok := ctx.Value(spanContextKey{}).(*Span); ok {
		return span
	}
	return nil
}

// InMemoryExporter stores spans in memory for testing.
type InMemoryExporter struct {
	mu    sync.Mutex
	spans []*Span
}

// NewInMemoryExporter creates a new in-memory exporter.
func NewInMemoryExporter() *InMemoryExporter {
	return &InMemoryExporter{
		spans: make([]*Span, 0),
	}
}

// Export stores the span.
func (e *InMemoryExporter) Export(span *Span) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, span)
	return nil
}

// Close is a no-op.
func (e *InMemoryExporter) Close() error {
	return nil
}

// Spans returns all exported spans.
func (e *InMemoryExporter) Spans() []*Span {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]*Span, len(e.spans))
	copy(result, e.spans)
	return result
}

// Clear removes all spans.
func (e *InMemoryExporter) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = make([]*Span, 0)
}

// ChannelExporter exports spans to a channel.
type ChannelExporter struct {
	ch   chan *Span
	done chan struct{}
}

// NewChannelExporter creates a new channel exporter.
func NewChannelExporter(bufferSize int) *ChannelExporter {
	return &ChannelExporter{
		ch:   make(chan *Span, bufferSize),
		done: make(chan struct{}),
	}
}

// Export sends the span to the channel.
func (e *ChannelExporter) Export(span *Span) error {
	select {
	case e.ch <- span:
		return nil
	case <-e.done:
		return nil
	default:
		// Channel full, drop span
		return nil
	}
}

// Close closes the exporter.
func (e *ChannelExporter) Close() error {
	close(e.done)
	close(e.ch)
	return nil
}

// Spans returns the channel for receiving spans.
func (e *ChannelExporter) Spans() <-chan *Span {
	return e.ch
}
