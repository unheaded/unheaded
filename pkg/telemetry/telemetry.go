// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package telemetry provides application-level tracing and context propagation
// for Unheaded services. It wraps span creation, trace/span ID generation,
// context propagation, and OTLP-compatible export into a simple API that all
// services can use to instrument their operations.
//
// This package is the service-facing counterpart to the lower-level tracing
// collector: services use telemetry to create and finish spans, while the
// tracing collector aggregates and correlates them.
package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Errors returned by the telemetry package.
var (
	ErrInvalidTraceID  = errors.New("invalid trace ID")
	ErrInvalidSpanID   = errors.New("invalid span ID")
	ErrProviderClosed  = errors.New("tracer provider is shut down")
	ErrExportFailed    = errors.New("span export failed")
	ErrNoSpanInContext = errors.New("no span in context")
)

// ============================================================================
// TRACE & SPAN IDENTIFIERS
// ============================================================================

// TraceID is a 128-bit trace identifier compatible with W3C Trace Context.
type TraceID [16]byte

// String returns the lowercase hex encoding of the trace ID.
func (t TraceID) String() string {
	return hex.EncodeToString(t[:])
}

// IsValid returns true when the trace ID is not all zeros.
func (t TraceID) IsValid() bool {
	for _, b := range t {
		if b != 0 {
			return true
		}
	}
	return false
}

// MarshalJSON encodes the trace ID as a JSON string.
func (t TraceID) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON decodes a JSON string into a trace ID.
func (t *TraceID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return t.FromString(s)
}

// FromString parses a 32-character hex string into a TraceID.
func (t *TraceID) FromString(s string) error {
	if len(s) != 32 {
		return fmt.Errorf("%w: expected 32 hex chars, got %d", ErrInvalidTraceID, len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTraceID, err)
	}
	copy(t[:], b)
	return nil
}

// GenerateTraceID creates a new random 128-bit trace ID.
func GenerateTraceID() TraceID {
	var id TraceID
	_, _ = rand.Read(id[:])
	return id
}

// SpanID is a 64-bit span identifier.
type SpanID [8]byte

// String returns the lowercase hex encoding of the span ID.
func (s SpanID) String() string {
	return hex.EncodeToString(s[:])
}

// IsValid returns true when the span ID is not all zeros.
func (s SpanID) IsValid() bool {
	for _, b := range s {
		if b != 0 {
			return true
		}
	}
	return false
}

// MarshalJSON encodes the span ID as a JSON string.
func (s SpanID) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON decodes a JSON string into a span ID.
func (s *SpanID) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	return s.FromString(str)
}

// FromString parses a 16-character hex string into a SpanID.
func (s *SpanID) FromString(str string) error {
	if len(str) != 16 {
		return fmt.Errorf("%w: expected 16 hex chars, got %d", ErrInvalidSpanID, len(str))
	}
	b, err := hex.DecodeString(str)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSpanID, err)
	}
	copy(s[:], b)
	return nil
}

// GenerateSpanID creates a new random 64-bit span ID.
func GenerateSpanID() SpanID {
	var id SpanID
	_, _ = rand.Read(id[:])
	return id
}

// ============================================================================
// SPAN KIND & STATUS
// ============================================================================

// SpanKind describes the relationship between the span, its parents, and its children.
type SpanKind int

const (
	SpanKindInternal SpanKind = iota
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// String returns the human-readable name of the span kind.
func (k SpanKind) String() string {
	switch k {
	case SpanKindServer:
		return "server"
	case SpanKindClient:
		return "client"
	case SpanKindProducer:
		return "producer"
	case SpanKindConsumer:
		return "consumer"
	default:
		return "internal"
	}
}

// StatusCode represents the status of a finished span.
type StatusCode int

const (
	StatusUnset StatusCode = iota
	StatusOK
	StatusError
)

// String returns the human-readable name of the status code.
func (c StatusCode) String() string {
	switch c {
	case StatusOK:
		return "ok"
	case StatusError:
		return "error"
	default:
		return "unset"
	}
}

// ============================================================================
// SPAN
// ============================================================================

// SpanEvent records an event that occurred during a span's lifetime.
type SpanEvent struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Span represents an individual operation within a distributed trace.
// Spans are safe for concurrent use.
type Span struct {
	traceID      TraceID
	spanID       SpanID
	parentSpanID SpanID
	name         string
	kind         SpanKind
	startTime    time.Time
	endTime      time.Time
	statusCode   StatusCode
	statusMsg    string
	attributes   map[string]string
	events       []SpanEvent
	serviceName  string
	resource     map[string]string

	mu       sync.Mutex
	ended    int32
	exporter SpanExporter
}

// TraceID returns the trace ID of this span.
func (s *Span) TraceID() TraceID { return s.traceID }

// SpanID returns the span ID of this span.
func (s *Span) SpanID() SpanID { return s.spanID }

// ParentSpanID returns the parent span ID (zero if root).
func (s *Span) ParentSpanID() SpanID { return s.parentSpanID }

// Name returns the span name.
func (s *Span) Name() string { return s.name }

// Kind returns the span kind.
func (s *Span) Kind() SpanKind { return s.kind }

// StartTime returns when the span started.
func (s *Span) StartTime() time.Time { return s.startTime }

// EndTime returns when the span ended (zero if not yet ended).
func (s *Span) EndTime() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.endTime
}

// Duration returns the elapsed duration, or zero if the span has not ended.
func (s *Span) Duration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endTime.IsZero() {
		return 0
	}
	return s.endTime.Sub(s.startTime)
}

// StatusCode returns the span's status code.
func (s *Span) StatusCode() StatusCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusCode
}

// StatusMessage returns the span's status message.
func (s *Span) StatusMessage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusMsg
}

// IsEnded returns true if End has been called.
func (s *Span) IsEnded() bool {
	return atomic.LoadInt32(&s.ended) == 1
}

// SetAttributes sets key-value pairs on the span. Thread-safe.
func (s *Span) SetAttributes(attrs map[string]string) {
	if s.IsEnded() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = make(map[string]string)
	}
	for k, v := range attrs {
		s.attributes[k] = v
	}
}

// SetAttribute sets a single key-value attribute. Thread-safe.
func (s *Span) SetAttribute(key, value string) {
	s.SetAttributes(map[string]string{key: value})
}

// Attributes returns a copy of the span's attributes.
func (s *Span) Attributes() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[string]string, len(s.attributes))
	for k, v := range s.attributes {
		cp[k] = v
	}
	return cp
}

// AddEvent records a timestamped event on the span.
func (s *Span) AddEvent(name string, attrs map[string]string) {
	if s.IsEnded() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attrs,
	})
}

// Events returns a copy of the span's events.
func (s *Span) Events() []SpanEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]SpanEvent, len(s.events))
	copy(cp, s.events)
	return cp
}

// SetStatus sets the span status code and optional message.
func (s *Span) SetStatus(code StatusCode, msg string) {
	if s.IsEnded() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusCode = code
	s.statusMsg = msg
}

// RecordError records an error as an event and sets status to error.
func (s *Span) RecordError(err error) {
	if err == nil || s.IsEnded() {
		return
	}
	s.AddEvent("exception", map[string]string{
		"exception.message": err.Error(),
	})
	s.SetStatus(StatusError, err.Error())
}

// End finishes the span and exports it. Calling End more than once is a no-op.
func (s *Span) End() {
	if !atomic.CompareAndSwapInt32(&s.ended, 0, 1) {
		return
	}
	s.mu.Lock()
	s.endTime = time.Now()
	s.mu.Unlock()

	if s.exporter != nil {
		s.exporter.ExportSpan(s)
	}
}

// ToExportData returns a serialisable snapshot of the span.
func (s *Span) ToExportData() ExportSpanData {
	s.mu.Lock()
	defer s.mu.Unlock()
	attrs := make(map[string]string, len(s.attributes))
	for k, v := range s.attributes {
		attrs[k] = v
	}
	events := make([]SpanEvent, len(s.events))
	copy(events, s.events)
	return ExportSpanData{
		TraceID:      s.traceID,
		SpanID:       s.spanID,
		ParentSpanID: s.parentSpanID,
		Name:         s.name,
		Kind:         s.kind,
		StartTime:    s.startTime,
		EndTime:      s.endTime,
		StatusCode:   s.statusCode,
		StatusMsg:    s.statusMsg,
		Attributes:   attrs,
		Events:       events,
		ServiceName:  s.serviceName,
		Resource:     s.resource,
	}
}

// ============================================================================
// EXPORT DATA
// ============================================================================

// ExportSpanData is an immutable snapshot of a finished span, suitable for
// serialisation or sending to an export pipeline.
type ExportSpanData struct {
	TraceID      TraceID           `json:"trace_id"`
	SpanID       SpanID            `json:"span_id"`
	ParentSpanID SpanID            `json:"parent_span_id,omitempty"`
	Name         string            `json:"name"`
	Kind         SpanKind          `json:"kind"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	StatusCode   StatusCode        `json:"status_code"`
	StatusMsg    string            `json:"status_message,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Events       []SpanEvent       `json:"events,omitempty"`
	ServiceName  string            `json:"service_name"`
	Resource     map[string]string `json:"resource,omitempty"`
}

// ============================================================================
// SPAN EXPORTER INTERFACE
// ============================================================================

// SpanExporter is the interface for exporting finished spans.
type SpanExporter interface {
	ExportSpan(span *Span)
	ExportSpans(spans []*Span)
	Shutdown(ctx context.Context) error
}

// ============================================================================
// CONTEXT PROPAGATION
// ============================================================================

type contextKey struct{}

// ContextWithSpan returns a copy of ctx carrying the given span.
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, contextKey{}, span)
}

// SpanFromContext retrieves the current span from a context.
// Returns nil if no span is present.
func SpanFromContext(ctx context.Context) *Span {
	v := ctx.Value(contextKey{})
	if v == nil {
		return nil
	}
	s, _ := v.(*Span)
	return s
}

// ============================================================================
// TRACER
// ============================================================================

// Tracer creates spans within a single service/component.
type Tracer struct {
	serviceName string
	resource    map[string]string
	exporter    SpanExporter
}

// StartSpan creates a new span. If the context already carries a span, the
// new span becomes its child and shares the same trace ID.
func (tr *Tracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	cfg := spanConfig{kind: SpanKindInternal}
	for _, o := range opts {
		o(&cfg)
	}

	span := &Span{
		spanID:      GenerateSpanID(),
		name:        name,
		kind:        cfg.kind,
		startTime:   time.Now(),
		serviceName: tr.serviceName,
		resource:    tr.resource,
		exporter:    tr.exporter,
		attributes:  make(map[string]string),
	}

	if parent := SpanFromContext(ctx); parent != nil {
		span.traceID = parent.traceID
		span.parentSpanID = parent.spanID
	} else {
		span.traceID = GenerateTraceID()
	}

	if cfg.traceID.IsValid() {
		span.traceID = cfg.traceID
	}

	for k, v := range cfg.attributes {
		span.attributes[k] = v
	}

	return ContextWithSpan(ctx, span), span
}

// spanConfig holds optional parameters for span creation.
type spanConfig struct {
	kind       SpanKind
	traceID    TraceID
	attributes map[string]string
}

// SpanOption configures a span at creation time.
type SpanOption func(*spanConfig)

// WithSpanKind sets the kind of the created span.
func WithSpanKind(kind SpanKind) SpanOption {
	return func(c *spanConfig) { c.kind = kind }
}

// WithTraceID overrides the auto-generated trace ID.
func WithTraceID(id TraceID) SpanOption {
	return func(c *spanConfig) { c.traceID = id }
}

// WithAttributes sets initial attributes on the span.
func WithAttributes(attrs map[string]string) SpanOption {
	return func(c *spanConfig) { c.attributes = attrs }
}

// ============================================================================
// TRACER PROVIDER
// ============================================================================

// TracerProviderConfig configures a TracerProvider.
type TracerProviderConfig struct {
	ServiceName string
	Resource    map[string]string
	Exporter    SpanExporter
}

// TracerProvider manages tracers and the export pipeline.
type TracerProvider struct {
	serviceName string
	resource    map[string]string
	exporter    SpanExporter
	closed      int32
}

// NewTracerProvider creates a new TracerProvider.
func NewTracerProvider(cfg TracerProviderConfig) *TracerProvider {
	return &TracerProvider{
		serviceName: cfg.ServiceName,
		resource:    cfg.Resource,
		exporter:    cfg.Exporter,
	}
}

// Tracer returns a named Tracer. The service name from the provider is used
// unless overridden.
func (tp *TracerProvider) Tracer(name string) *Tracer {
	svcName := name
	if svcName == "" {
		svcName = tp.serviceName
	}
	return &Tracer{
		serviceName: svcName,
		resource:    tp.resource,
		exporter:    tp.exporter,
	}
}

// Shutdown gracefully stops the provider and flushes any pending exports.
func (tp *TracerProvider) Shutdown(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&tp.closed, 0, 1) {
		return ErrProviderClosed
	}
	if tp.exporter != nil {
		return tp.exporter.Shutdown(ctx)
	}
	return nil
}

// ============================================================================
// IN-MEMORY EXPORTER (useful for testing / buffering)
// ============================================================================

// InMemoryExporter collects exported spans in memory.
type InMemoryExporter struct {
	mu    sync.Mutex
	spans []ExportSpanData
	err   error // injected error for testing
}

// NewInMemoryExporter creates a new in-memory exporter.
func NewInMemoryExporter() *InMemoryExporter {
	return &InMemoryExporter{}
}

// ExportSpan records a single span.
func (e *InMemoryExporter) ExportSpan(span *Span) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, span.ToExportData())
}

// ExportSpans records multiple spans.
func (e *InMemoryExporter) ExportSpans(spans []*Span) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range spans {
		e.spans = append(e.spans, s.ToExportData())
	}
}

// Shutdown is a no-op for the in-memory exporter.
func (e *InMemoryExporter) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.err
}

// GetSpans returns a copy of all exported spans.
func (e *InMemoryExporter) GetSpans() []ExportSpanData {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := make([]ExportSpanData, len(e.spans))
	copy(cp, e.spans)
	return cp
}

// Reset clears all recorded spans.
func (e *InMemoryExporter) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = nil
}

// SetError injects an error that Shutdown will return.
func (e *InMemoryExporter) SetError(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.err = err
}

// ============================================================================
// OTLP JSON EXPORTER
// ============================================================================

// OTLPExporter sends spans as JSON to an OTLP-compatible HTTP endpoint.
type OTLPExporter struct {
	endpoint   string
	client     *http.Client
	headers    map[string]string
	mu         sync.Mutex
	batch      []*Span
	batchSize  int
	closed     int32
	flushCh    chan struct{}
	shutdownCh chan struct{}
	doneCh     chan struct{}
}

// OTLPExporterConfig configures the OTLP exporter.
type OTLPExporterConfig struct {
	Endpoint  string
	Headers   map[string]string
	Client    *http.Client
	BatchSize int
}

// NewOTLPExporter creates a new OTLP JSON exporter.
func NewOTLPExporter(cfg OTLPExporterConfig) *OTLPExporter {
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 64
	}
	e := &OTLPExporter{
		endpoint:   cfg.Endpoint,
		client:     cfg.Client,
		headers:    cfg.Headers,
		batchSize:  cfg.BatchSize,
		batch:      make([]*Span, 0, cfg.BatchSize),
		flushCh:    make(chan struct{}, 1),
		shutdownCh: make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
	go e.batchLoop()
	return e
}

// ExportSpan adds a span to the batch; flushes when batch is full.
func (e *OTLPExporter) ExportSpan(span *Span) {
	if atomic.LoadInt32(&e.closed) == 1 {
		return
	}
	e.mu.Lock()
	e.batch = append(e.batch, span)
	shouldFlush := len(e.batch) >= e.batchSize
	e.mu.Unlock()
	if shouldFlush {
		select {
		case e.flushCh <- struct{}{}:
		default:
		}
	}
}

// ExportSpans adds multiple spans, flushing if needed.
func (e *OTLPExporter) ExportSpans(spans []*Span) {
	for _, s := range spans {
		e.ExportSpan(s)
	}
}

// Shutdown flushes remaining spans and stops the background loop.
func (e *OTLPExporter) Shutdown(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&e.closed, 0, 1) {
		return nil
	}
	close(e.shutdownCh)
	<-e.doneCh
	return e.flush()
}

func (e *OTLPExporter) batchLoop() {
	defer close(e.doneCh)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.shutdownCh:
			return
		case <-e.flushCh:
			_ = e.flush()
		case <-ticker.C:
			_ = e.flush()
		}
	}
}

func (e *OTLPExporter) flush() error {
	e.mu.Lock()
	if len(e.batch) == 0 {
		e.mu.Unlock()
		return nil
	}
	toSend := e.batch
	e.batch = make([]*Span, 0, e.batchSize)
	e.mu.Unlock()

	data := make([]ExportSpanData, 0, len(toSend))
	for _, s := range toSend {
		data = append(data, s.ToExportData())
	}

	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("%w: marshal: %v", ErrExportFailed, err)
	}

	req, err := http.NewRequest(http.MethodPost, e.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("%w: create request: %v", ErrExportFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrExportFailed, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%w: HTTP %d", ErrExportFailed, resp.StatusCode)
	}
	return nil
}

// ============================================================================
// W3C TRACE CONTEXT PROPAGATION
// ============================================================================

const (
	traceparentHeader = "Traceparent"
	tracestateHeader  = "Tracestate"
)

// InjectHTTP writes the current span's trace context into HTTP headers using
// the W3C Trace Context format.
func InjectHTTP(ctx context.Context, headers http.Header) {
	span := SpanFromContext(ctx)
	if span == nil {
		return
	}
	tp := fmt.Sprintf("00-%s-%s-01", span.traceID.String(), span.spanID.String())
	headers.Set(traceparentHeader, tp)
}

// ExtractHTTP reads W3C Trace Context headers and returns a TraceID and
// parent SpanID. Returns zero values if headers are missing or malformed.
func ExtractHTTP(headers http.Header) (TraceID, SpanID, error) {
	tp := headers.Get(traceparentHeader)
	if tp == "" {
		return TraceID{}, SpanID{}, nil
	}
	return ParseTraceparent(tp)
}

// ParseTraceparent parses a W3C traceparent header value.
// Format: version-traceID-parentID-flags  (e.g. "00-<32hex>-<16hex>-01")
func ParseTraceparent(value string) (TraceID, SpanID, error) {
	parts := strings.Split(value, "-")
	if len(parts) != 4 {
		return TraceID{}, SpanID{}, fmt.Errorf("%w: expected 4 parts, got %d", ErrInvalidTraceID, len(parts))
	}
	var traceID TraceID
	if err := traceID.FromString(parts[1]); err != nil {
		return TraceID{}, SpanID{}, err
	}
	var spanID SpanID
	if err := spanID.FromString(parts[2]); err != nil {
		return TraceID{}, SpanID{}, err
	}
	return traceID, spanID, nil
}

// FormatTraceparent creates a W3C traceparent header value.
func FormatTraceparent(traceID TraceID, spanID SpanID) string {
	return fmt.Sprintf("00-%s-%s-01", traceID.String(), spanID.String())
}
