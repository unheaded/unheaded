// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

const (
	// TraceIDHeader is the header name for trace ID.
	TraceIDHeader = "X-Trace-ID"
	// SpanIDHeader is the header name for span ID.
	SpanIDHeader = "X-Span-ID"
	// ParentSpanIDHeader is the header name for parent span ID.
	ParentSpanIDHeader = "X-Parent-Span-ID"
	// RequestIDHeader is the header name for request ID.
	RequestIDHeader = "X-Request-ID"
)

// TraceIDKey is the context key for trace ID.
const TraceIDKey ContextKey = "trace_id"

// SpanIDKey is the context key for span ID.
const SpanIDKey ContextKey = "span_id"

// RequestStartKey is the context key for request start time.
const RequestStartKey ContextKey = "request_start"

// TracingMiddleware adds distributed tracing to requests.
type TracingMiddleware struct {
	serviceName string
}

// NewTracingMiddleware creates a new tracing middleware.
func NewTracingMiddleware(serviceName string) *TracingMiddleware {
	return &TracingMiddleware{
		serviceName: serviceName,
	}
}

// Handler returns the middleware handler.
func (m *TracingMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get or generate trace ID
		traceID := r.Header.Get(TraceIDHeader)
		if traceID == "" {
			traceID = r.Header.Get(RequestIDHeader)
		}
		if traceID == "" {
			traceID = generateID()
		}

		// Get parent span ID
		parentSpanID := r.Header.Get(SpanIDHeader)

		// Generate new span ID for this request
		spanID := generateID()

		// Record start time
		startTime := time.Now()

		// Add to context
		ctx := r.Context()
		ctx = context.WithValue(ctx, TraceIDKey, traceID)
		ctx = context.WithValue(ctx, SpanIDKey, spanID)
		ctx = context.WithValue(ctx, RequestStartKey, startTime)

		// Set response headers
		w.Header().Set(TraceIDHeader, traceID)
		w.Header().Set(SpanIDHeader, spanID)
		if parentSpanID != "" {
			w.Header().Set(ParentSpanIDHeader, parentSpanID)
		}

		// Continue with the request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateID generates a random ID for tracing.
func generateID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(bytes)
}

// GetTraceID retrieves the trace ID from context.
func GetTraceID(ctx context.Context) string {
	if v := ctx.Value(TraceIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// GetSpanID retrieves the span ID from context.
func GetSpanID(ctx context.Context) string {
	if v := ctx.Value(SpanIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// GetRequestStart retrieves the request start time from context.
func GetRequestStart(ctx context.Context) time.Time {
	if v := ctx.Value(RequestStartKey); v != nil {
		return v.(time.Time)
	}
	return time.Time{}
}

// Span represents a trace span.
type Span struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	ServiceName  string            `json:"service_name"`
	OperationName string           `json:"operation_name"`
	StartTime    time.Time         `json:"start_time"`
	Duration     time.Duration     `json:"duration"`
	Tags         map[string]string `json:"tags,omitempty"`
	Status       string            `json:"status"`
}

// SpanBuilder helps build spans.
type SpanBuilder struct {
	span *Span
}

// NewSpan creates a new span builder.
func NewSpan(serviceName, operationName string) *SpanBuilder {
	return &SpanBuilder{
		span: &Span{
			SpanID:        generateID(),
			ServiceName:   serviceName,
			OperationName: operationName,
			StartTime:     time.Now(),
			Tags:          make(map[string]string),
		},
	}
}

// WithTraceID sets the trace ID.
func (b *SpanBuilder) WithTraceID(traceID string) *SpanBuilder {
	b.span.TraceID = traceID
	return b
}

// WithParentSpanID sets the parent span ID.
func (b *SpanBuilder) WithParentSpanID(parentSpanID string) *SpanBuilder {
	b.span.ParentSpanID = parentSpanID
	return b
}

// WithTag adds a tag.
func (b *SpanBuilder) WithTag(key, value string) *SpanBuilder {
	b.span.Tags[key] = value
	return b
}

// End completes the span.
func (b *SpanBuilder) End(status string) *Span {
	b.span.Duration = time.Since(b.span.StartTime)
	b.span.Status = status
	return b.span
}

// InjectHeaders injects tracing headers into an outgoing request.
func InjectHeaders(ctx context.Context, header http.Header) {
	if traceID := GetTraceID(ctx); traceID != "" {
		header.Set(TraceIDHeader, traceID)
	}
	if spanID := GetSpanID(ctx); spanID != "" {
		header.Set(ParentSpanIDHeader, spanID)
	}
	// Generate new span ID for the outgoing request
	header.Set(SpanIDHeader, generateID())
}
