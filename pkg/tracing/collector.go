// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package tracing provides distributed tracing collection and correlation.
// THE TRACE WEAVER - Where the threads of causality are gathered and woven.
//
// This package implements a trace collector that:
// - Receives spans via gRPC (OpenTelemetry protocol compatible)
// - Receives kernel traces from eBPF ring buffers
// - Correlates traces by trace ID across kernel and application layers
// - Stores traces in memory with configurable retention
// - Supports querying traces by ID, service, time range, duration, and error status
// - Exports traces to Wotan for real-time streaming
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"unheaded/pkg/ports"
)

// ============================================================================
// ERRORS - WHEN THE THREADS ARE SEVERED
// ============================================================================

var (
	ErrTraceNotFound     = errors.New("trace not found")
	ErrSpanNotFound      = errors.New("span not found")
	ErrInvalidTraceID    = errors.New("invalid trace ID")
	ErrInvalidSpanID     = errors.New("invalid span ID")
	ErrCollectorClosed   = errors.New("collector is closed")
	ErrStorageFull       = errors.New("trace storage full")
	ErrInvalidQuery      = errors.New("invalid query parameters")
	ErrSamplingRejected  = errors.New("span rejected by sampler")
	ErrWotanUnavailable = errors.New("wotan connection unavailable")
)

// ============================================================================
// IDENTIFIERS - THE UNIQUE THREADS
// ============================================================================

// TraceID is a 128-bit trace identifier (OpenTelemetry compatible)
type TraceID [16]byte

// String returns the hex representation of the trace ID
func (t TraceID) String() string {
	return hex.EncodeToString(t[:])
}

// IsZero returns true if the trace ID is all zeros
func (t TraceID) IsZero() bool {
	for _, b := range t {
		if b != 0 {
			return false
		}
	}
	return true
}

// MarshalJSON implements json.Marshaler
func (t TraceID) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON implements json.Unmarshaler
func (t *TraceID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return t.FromString(s)
}

// FromString parses a hex string into a TraceID
func (t *TraceID) FromString(s string) error {
	if len(s) != 32 {
		return ErrInvalidTraceID
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return ErrInvalidTraceID
	}
	copy(t[:], b)
	return nil
}

// NewTraceID generates a new random trace ID
func NewTraceID() TraceID {
	var id TraceID
	rand.Read(id[:])
	return id
}

// TraceIDFromBytes creates a TraceID from a byte slice
func TraceIDFromBytes(b []byte) (TraceID, error) {
	var id TraceID
	if len(b) != 16 {
		return id, ErrInvalidTraceID
	}
	copy(id[:], b)
	return id, nil
}

// SpanID is a 64-bit span identifier
type SpanID [8]byte

// String returns the hex representation of the span ID
func (s SpanID) String() string {
	return hex.EncodeToString(s[:])
}

// IsZero returns true if the span ID is all zeros
func (s SpanID) IsZero() bool {
	for _, b := range s {
		if b != 0 {
			return false
		}
	}
	return true
}

// MarshalJSON implements json.Marshaler
func (s SpanID) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON implements json.Unmarshaler
func (s *SpanID) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	return s.FromString(str)
}

// FromString parses a hex string into a SpanID
func (s *SpanID) FromString(str string) error {
	if len(str) != 16 {
		return ErrInvalidSpanID
	}
	b, err := hex.DecodeString(str)
	if err != nil {
		return ErrInvalidSpanID
	}
	copy(s[:], b)
	return nil
}

// NewSpanID generates a new random span ID
func NewSpanID() SpanID {
	var id SpanID
	rand.Read(id[:])
	return id
}

// SpanIDFromBytes creates a SpanID from a byte slice
func SpanIDFromBytes(b []byte) (SpanID, error) {
	var id SpanID
	if len(b) != 8 {
		return id, ErrInvalidSpanID
	}
	copy(id[:], b)
	return id, nil
}

// ============================================================================
// SPAN - A SINGLE THREAD IN THE TAPESTRY
// ============================================================================

// SpanKind represents the role of a span in a trace
type SpanKind int

const (
	SpanKindUnspecified SpanKind = iota
	SpanKindInternal
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// String returns the string representation of SpanKind
func (k SpanKind) String() string {
	switch k {
	case SpanKindInternal:
		return "internal"
	case SpanKindServer:
		return "server"
	case SpanKindClient:
		return "client"
	case SpanKindProducer:
		return "producer"
	case SpanKindConsumer:
		return "consumer"
	default:
		return "unspecified"
	}
}

// SpanStatus represents the status of a span
type SpanStatus int

const (
	StatusUnset SpanStatus = iota
	StatusOK
	StatusError
)

// String returns the string representation of SpanStatus
func (s SpanStatus) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusError:
		return "error"
	default:
		return "unset"
	}
}

// SpanEvent represents an event that occurred during a span
type SpanEvent struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// SpanLink represents a link to another span
type SpanLink struct {
	TraceID    TraceID           `json:"trace_id"`
	SpanID     SpanID            `json:"span_id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Span represents an individual operation within a trace
type Span struct {
	TraceID      TraceID           `json:"trace_id"`
	SpanID       SpanID            `json:"span_id"`
	ParentSpanID SpanID            `json:"parent_span_id,omitempty"`
	Name         string            `json:"name"`
	Kind         SpanKind          `json:"kind"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	Duration     time.Duration     `json:"duration_ns"`
	Status       SpanStatus        `json:"status"`
	StatusMsg    string            `json:"status_message,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Events       []SpanEvent       `json:"events,omitempty"`
	Links        []SpanLink        `json:"links,omitempty"`

	// Resource attributes (service info)
	ServiceName    string `json:"service_name"`
	ServiceVersion string `json:"service_version,omitempty"`
	HostName       string `json:"host_name,omitempty"`
	ContainerID    string `json:"container_id,omitempty"`

	// Kernel correlation
	KernelCorrelated bool   `json:"kernel_correlated"`
	KernelEventID    uint64 `json:"kernel_event_id,omitempty"`
}

// IsRoot returns true if this is a root span (no parent)
func (s *Span) IsRoot() bool {
	return s.ParentSpanID.IsZero()
}

// HasError returns true if the span has an error status
func (s *Span) HasError() bool {
	return s.Status == StatusError
}

// ============================================================================
// KERNEL EVENT - THE WHISPERS FROM THE VOID
// ============================================================================

// KernelEventType represents the type of kernel event
type KernelEventType uint32

const (
	KernelEventUnknown KernelEventType = iota
	KernelEventPacketRx
	KernelEventPacketTx
	KernelEventSyscall
	KernelEventSchedule
	KernelEventNetConnect
	KernelEventNetClose
	KernelEventDNS
	KernelEventTCPRetransmit
)

// String returns the string representation of KernelEventType
func (t KernelEventType) String() string {
	switch t {
	case KernelEventPacketRx:
		return "packet_rx"
	case KernelEventPacketTx:
		return "packet_tx"
	case KernelEventSyscall:
		return "syscall"
	case KernelEventSchedule:
		return "schedule"
	case KernelEventNetConnect:
		return "net_connect"
	case KernelEventNetClose:
		return "net_close"
	case KernelEventDNS:
		return "dns"
	case KernelEventTCPRetransmit:
		return "tcp_retransmit"
	default:
		return "unknown"
	}
}

// KernelEvent represents a raw event from eBPF
type KernelEvent struct {
	EventID     uint64          `json:"event_id"`
	Type        KernelEventType `json:"type"`
	Timestamp   time.Time       `json:"timestamp"`
	CPU         uint32          `json:"cpu"`
	PID         uint32          `json:"pid"`
	TID         uint32          `json:"tid"`
	Comm        string          `json:"comm"` // Process name
	ContainerID string          `json:"container_id,omitempty"`

	// Network fields (for packet/connection events)
	SrcIP   net.IP `json:"src_ip,omitempty"`
	DstIP   net.IP `json:"dst_ip,omitempty"`
	SrcPort uint16 `json:"src_port,omitempty"`
	DstPort uint16 `json:"dst_port,omitempty"`
	Proto   uint8  `json:"proto,omitempty"` // IP protocol number

	// Trace correlation
	TraceIDHigh uint64 `json:"trace_id_high,omitempty"`
	TraceIDLow  uint64 `json:"trace_id_low,omitempty"`

	// Latency measurement
	LatencyNs uint64 `json:"latency_ns,omitempty"`

	// Raw data
	RawData []byte `json:"raw_data,omitempty"`
}

// GetTraceID extracts the trace ID from kernel event
func (e *KernelEvent) GetTraceID() TraceID {
	var id TraceID
	binary.BigEndian.PutUint64(id[0:8], e.TraceIDHigh)
	binary.BigEndian.PutUint64(id[8:16], e.TraceIDLow)
	return id
}

// HasTraceID returns true if the event has a trace ID
func (e *KernelEvent) HasTraceID() bool {
	return e.TraceIDHigh != 0 || e.TraceIDLow != 0
}

// ============================================================================
// TRACE - THE COMPLETE TAPESTRY
// ============================================================================

// Trace represents a collection of spans with a shared trace ID
type Trace struct {
	TraceID   TraceID        `json:"trace_id"`
	RootSpan  *Span          `json:"root_span,omitempty"`
	Spans     []*Span        `json:"spans"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time"`
	Duration  time.Duration  `json:"duration_ns"`
	Services  []string       `json:"services"`
	HasError  bool           `json:"has_error"`
	SpanCount int            `json:"span_count"`
	Depth     int            `json:"depth"` // Maximum span depth

	// Kernel events correlated to this trace
	KernelEvents []*KernelEvent `json:"kernel_events,omitempty"`

	// Derived metrics
	TotalLatencyNs uint64 `json:"total_latency_ns"`
	KernelLatencyNs uint64 `json:"kernel_latency_ns"`
}

// AddSpan adds a span to the trace and updates metadata
func (t *Trace) AddSpan(span *Span) {
	t.Spans = append(t.Spans, span)
	t.SpanCount = len(t.Spans)

	// Update time bounds
	if t.StartTime.IsZero() || span.StartTime.Before(t.StartTime) {
		t.StartTime = span.StartTime
	}
	if span.EndTime.After(t.EndTime) {
		t.EndTime = span.EndTime
	}
	t.Duration = t.EndTime.Sub(t.StartTime)

	// Track services
	if span.ServiceName != "" {
		found := false
		for _, svc := range t.Services {
			if svc == span.ServiceName {
				found = true
				break
			}
		}
		if !found {
			t.Services = append(t.Services, span.ServiceName)
		}
	}

	// Track errors
	if span.HasError() {
		t.HasError = true
	}

	// Update root span
	if span.IsRoot() {
		t.RootSpan = span
	}

	// Calculate depth
	t.calculateDepth()
}

// calculateDepth calculates the maximum depth of the trace tree
func (t *Trace) calculateDepth() {
	// Build parent-child map
	children := make(map[SpanID][]*Span)
	var roots []*Span

	for _, span := range t.Spans {
		if span.IsRoot() {
			roots = append(roots, span)
		} else {
			children[span.ParentSpanID] = append(children[span.ParentSpanID], span)
		}
	}

	// Calculate max depth
	var maxDepth func(spanID SpanID, depth int) int
	maxDepth = func(spanID SpanID, depth int) int {
		max := depth
		for _, child := range children[spanID] {
			childDepth := maxDepth(child.SpanID, depth+1)
			if childDepth > max {
				max = childDepth
			}
		}
		return max
	}

	t.Depth = 0
	for _, root := range roots {
		depth := maxDepth(root.SpanID, 1)
		if depth > t.Depth {
			t.Depth = depth
		}
	}
}

// ============================================================================
// QUERY - SEARCHING THE TAPESTRY
// ============================================================================

// TraceQuery defines query parameters for searching traces
type TraceQuery struct {
	// Filter by trace ID (exact match)
	TraceID *TraceID `json:"trace_id,omitempty"`

	// Filter by service name
	ServiceName string `json:"service_name,omitempty"`

	// Filter by time range
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`

	// Filter by duration (slow traces)
	MinDuration time.Duration `json:"min_duration_ns,omitempty"`
	MaxDuration time.Duration `json:"max_duration_ns,omitempty"`

	// Filter by error status
	HasError *bool `json:"has_error,omitempty"`

	// Filter by span name (substring match)
	SpanName string `json:"span_name,omitempty"`

	// Filter by attributes
	Attributes map[string]string `json:"attributes,omitempty"`

	// Pagination
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`

	// Sort order
	OrderBy   string `json:"order_by,omitempty"`   // start_time, duration, span_count
	Ascending bool   `json:"ascending,omitempty"`
}

// Validate validates the query parameters
func (q *TraceQuery) Validate() error {
	if q.Limit < 0 {
		return fmt.Errorf("%w: negative limit", ErrInvalidQuery)
	}
	if q.Offset < 0 {
		return fmt.Errorf("%w: negative offset", ErrInvalidQuery)
	}
	if q.MinDuration < 0 {
		return fmt.Errorf("%w: negative min duration", ErrInvalidQuery)
	}
	if q.MaxDuration < 0 {
		return fmt.Errorf("%w: negative max duration", ErrInvalidQuery)
	}
	if q.MinDuration > 0 && q.MaxDuration > 0 && q.MinDuration > q.MaxDuration {
		return fmt.Errorf("%w: min duration greater than max", ErrInvalidQuery)
	}
	return nil
}

// TraceQueryResult represents the result of a trace query
type TraceQueryResult struct {
	Traces     []*Trace `json:"traces"`
	Total      int      `json:"total"`
	HasMore    bool     `json:"has_more"`
	NextOffset int      `json:"next_offset,omitempty"`
}

// StreamFilter defines parameters for filtering the trace stream
type StreamFilter struct {
	ServiceNames []string `json:"service_names,omitempty"`
	MinDuration  time.Duration `json:"min_duration_ns,omitempty"`
	ErrorsOnly   bool     `json:"errors_only,omitempty"`
}

// ============================================================================
// SAMPLER - SELECTIVE OBSERVATION
// ============================================================================

// SamplerConfig configures trace sampling behavior
type SamplerConfig struct {
	// Sample rate (0.0 to 1.0)
	SampleRate float64 `json:"sample_rate"`

	// Always sample errors
	AlwaysSampleErrors bool `json:"always_sample_errors"`

	// Always sample slow traces
	AlwaysSampleSlowTraces bool `json:"always_sample_slow"`
	SlowTraceThreshold     time.Duration `json:"slow_trace_threshold_ns"`

	// Rate limiting (traces per second)
	RateLimit float64 `json:"rate_limit"`
}

// Sampler implements trace sampling logic
type Sampler struct {
	config     SamplerConfig
	counter    uint64
	lastReset  time.Time
	traceCount uint64
	mu         sync.Mutex
}

// NewSampler creates a new sampler with the given configuration
func NewSampler(config SamplerConfig) *Sampler {
	if config.SampleRate <= 0 {
		config.SampleRate = 1.0 // Default to sampling everything
	}
	if config.SampleRate > 1.0 {
		config.SampleRate = 1.0
	}
	return &Sampler{
		config:    config,
		lastReset: time.Now(),
	}
}

// ShouldSample determines if a span should be sampled
func (s *Sampler) ShouldSample(span *Span) bool {
	// Always sample errors if configured
	if s.config.AlwaysSampleErrors && span.HasError() {
		return true
	}

	// Always sample slow traces if configured
	if s.config.AlwaysSampleSlowTraces && span.Duration >= s.config.SlowTraceThreshold {
		return true
	}

	// Check rate limit
	if s.config.RateLimit > 0 {
		s.mu.Lock()
		now := time.Now()
		if now.Sub(s.lastReset) >= time.Second {
			s.traceCount = 0
			s.lastReset = now
		}
		if float64(s.traceCount) >= s.config.RateLimit {
			s.mu.Unlock()
			return false
		}
		s.traceCount++
		s.mu.Unlock()
	}

	// Rate 1.0 means sample everything — fast-path to avoid float precision bugs.
	if s.config.SampleRate >= 1.0 {
		return true
	}

	// Probabilistic sampling based on trace ID for consistent decisions
	// across all spans within the same trace.
	threshold := uint64(s.config.SampleRate * float64(uint64(1)<<63))

	var hash uint64
	for i := 0; i < 8; i++ {
		hash = hash<<8 | uint64(span.TraceID[i])
	}
	// Fold to 63 bits so the comparison space matches the threshold range.
	hash = hash >> 1

	return hash <= threshold
}

// ============================================================================
// STORAGE - THE MEMORY PALACE
// ============================================================================

// StorageConfig configures trace storage
type StorageConfig struct {
	// Maximum number of traces to store
	MaxTraces int `json:"max_traces"`

	// Maximum storage size in bytes
	MaxStorageBytes int64 `json:"max_storage_bytes"`

	// Retention period
	RetentionPeriod time.Duration `json:"retention_period_ns"`

	// Eviction batch size
	EvictionBatchSize int `json:"eviction_batch_size"`
}

// TraceStorage provides memory-efficient trace storage with eviction
type TraceStorage struct {
	config StorageConfig
	traces map[TraceID]*Trace
	order  []TraceID // LRU order
	size   int64     // Current estimated size
	mu     sync.RWMutex

	// Indexes for efficient queries
	byService   map[string]map[TraceID]struct{}
	byStartTime []traceTimeEntry

	// Metrics
	evictionCount uint64
}

type traceTimeEntry struct {
	TraceID   TraceID
	StartTime time.Time
}

// NewTraceStorage creates a new trace storage
func NewTraceStorage(config StorageConfig) *TraceStorage {
	if config.MaxTraces <= 0 {
		config.MaxTraces = 100000
	}
	if config.MaxStorageBytes <= 0 {
		config.MaxStorageBytes = 1 << 30 // 1GB
	}
	if config.RetentionPeriod <= 0 {
		config.RetentionPeriod = 24 * time.Hour
	}
	if config.EvictionBatchSize <= 0 {
		config.EvictionBatchSize = 100
	}

	return &TraceStorage{
		config:      config,
		traces:      make(map[TraceID]*Trace),
		order:       make([]TraceID, 0, config.MaxTraces),
		byService:   make(map[string]map[TraceID]struct{}),
		byStartTime: make([]traceTimeEntry, 0, config.MaxTraces),
	}
}

// Store stores a trace, evicting old traces if necessary
func (s *TraceStorage) Store(trace *Trace) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Estimate trace size
	traceSize := s.estimateTraceSize(trace)

	// Check if we need to evict
	for len(s.traces) >= s.config.MaxTraces || s.size+traceSize > s.config.MaxStorageBytes {
		if len(s.order) == 0 {
			return ErrStorageFull
		}
		s.evictOldest()
	}

	// Store the trace
	if existing, ok := s.traces[trace.TraceID]; ok {
		// Update existing trace
		s.size -= s.estimateTraceSize(existing)
		s.traces[trace.TraceID] = trace
		s.size += traceSize
		s.updateIndexes(trace)
		return nil
	}

	s.traces[trace.TraceID] = trace
	s.order = append(s.order, trace.TraceID)
	s.size += traceSize

	// Update indexes
	s.updateIndexes(trace)

	return nil
}

// Get retrieves a trace by ID
func (s *TraceStorage) Get(traceID TraceID) (*Trace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trace, ok := s.traces[traceID]
	if !ok {
		return nil, ErrTraceNotFound
	}
	return trace, nil
}

// Query searches traces based on query parameters
func (s *TraceStorage) Query(query *TraceQuery) (*TraceQueryResult, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// If querying by trace ID, short circuit
	if query.TraceID != nil {
		trace, ok := s.traces[*query.TraceID]
		if !ok {
			return &TraceQueryResult{Traces: []*Trace{}, Total: 0}, nil
		}
		return &TraceQueryResult{
			Traces: []*Trace{trace},
			Total:  1,
		}, nil
	}

	// Build candidate set
	candidates := s.getCandidates(query)

	// Filter candidates
	var matches []*Trace
	for traceID := range candidates {
		trace := s.traces[traceID]
		if s.matchesQuery(trace, query) {
			matches = append(matches, trace)
		}
	}

	// Sort results
	s.sortTraces(matches, query)

	// Apply pagination
	total := len(matches)
	offset := query.Offset
	if offset > total {
		offset = total
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100 // Default limit
	}
	end := offset + limit
	if end > total {
		end = total
	}

	result := &TraceQueryResult{
		Traces:  matches[offset:end],
		Total:   total,
		HasMore: end < total,
	}
	if result.HasMore {
		result.NextOffset = end
	}

	return result, nil
}

// Delete removes a trace by ID
func (s *TraceStorage) Delete(traceID TraceID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	trace, ok := s.traces[traceID]
	if !ok {
		return ErrTraceNotFound
	}

	s.size -= s.estimateTraceSize(trace)
	delete(s.traces, traceID)
	s.removeFromOrder(traceID)
	s.removeFromIndexes(trace)

	return nil
}

// Cleanup removes expired traces
func (s *TraceStorage) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-s.config.RetentionPeriod)
	removed := 0

	for i := 0; i < len(s.byStartTime); i++ {
		entry := s.byStartTime[i]
		if entry.StartTime.After(cutoff) {
			break // Sorted by time, no more expired
		}

		trace, ok := s.traces[entry.TraceID]
		if !ok {
			continue
		}

		s.size -= s.estimateTraceSize(trace)
		delete(s.traces, entry.TraceID)
		s.removeFromOrder(entry.TraceID)
		s.removeFromIndexes(trace)
		removed++
	}

	// Compact time index
	if removed > 0 {
		s.byStartTime = s.byStartTime[removed:]
	}

	return removed
}

// Stats returns storage statistics
func (s *TraceStorage) Stats() StorageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return StorageStats{
		TraceCount:    len(s.traces),
		StorageBytes:  s.size,
		EvictionCount: s.evictionCount,
		ServiceCount:  len(s.byService),
	}
}

// StorageStats represents storage statistics
type StorageStats struct {
	TraceCount    int   `json:"trace_count"`
	StorageBytes  int64 `json:"storage_bytes"`
	EvictionCount uint64 `json:"eviction_count"`
	ServiceCount  int   `json:"service_count"`
}

func (s *TraceStorage) estimateTraceSize(trace *Trace) int64 {
	// Rough estimation of trace size in bytes
	size := int64(200) // Base overhead
	for _, span := range trace.Spans {
		size += int64(300) // Base span overhead
		size += int64(len(span.Name))
		size += int64(len(span.ServiceName))
		for k, v := range span.Attributes {
			size += int64(len(k) + len(v))
		}
		for _, event := range span.Events {
			size += int64(100 + len(event.Name))
		}
	}
	for _, ke := range trace.KernelEvents {
		size += int64(150 + len(ke.RawData))
	}
	return size
}

func (s *TraceStorage) evictOldest() {
	if len(s.order) == 0 {
		return
	}

	// Evict oldest trace
	traceID := s.order[0]
	s.order = s.order[1:]

	if trace, ok := s.traces[traceID]; ok {
		s.size -= s.estimateTraceSize(trace)
		delete(s.traces, traceID)
		s.removeFromIndexes(trace)
	}

	s.evictionCount++
}

func (s *TraceStorage) updateIndexes(trace *Trace) {
	// Update service index
	for _, svc := range trace.Services {
		if s.byService[svc] == nil {
			s.byService[svc] = make(map[TraceID]struct{})
		}
		s.byService[svc][trace.TraceID] = struct{}{}
	}

	// Update time index (maintain sorted order)
	entry := traceTimeEntry{TraceID: trace.TraceID, StartTime: trace.StartTime}
	idx := sort.Search(len(s.byStartTime), func(i int) bool {
		return s.byStartTime[i].StartTime.After(entry.StartTime)
	})
	s.byStartTime = append(s.byStartTime, traceTimeEntry{})
	copy(s.byStartTime[idx+1:], s.byStartTime[idx:])
	s.byStartTime[idx] = entry
}

func (s *TraceStorage) removeFromOrder(traceID TraceID) {
	for i, id := range s.order {
		if id == traceID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			return
		}
	}
}

func (s *TraceStorage) removeFromIndexes(trace *Trace) {
	// Remove from service index
	for _, svc := range trace.Services {
		if svcTraces, ok := s.byService[svc]; ok {
			delete(svcTraces, trace.TraceID)
			if len(svcTraces) == 0 {
				delete(s.byService, svc)
			}
		}
	}

	// Remove from time index
	for i, entry := range s.byStartTime {
		if entry.TraceID == trace.TraceID {
			s.byStartTime = append(s.byStartTime[:i], s.byStartTime[i+1:]...)
			return
		}
	}
}

func (s *TraceStorage) getCandidates(query *TraceQuery) map[TraceID]struct{} {
	candidates := make(map[TraceID]struct{})

	// If filtering by service, use service index
	if query.ServiceName != "" {
		if svcTraces, ok := s.byService[query.ServiceName]; ok {
			for id := range svcTraces {
				candidates[id] = struct{}{}
			}
		}
		return candidates
	}

	// If filtering by time, use time index
	if query.StartTime != nil || query.EndTime != nil {
		for _, entry := range s.byStartTime {
			if query.StartTime != nil && entry.StartTime.Before(*query.StartTime) {
				continue
			}
			if query.EndTime != nil && entry.StartTime.After(*query.EndTime) {
				break // Time index is sorted
			}
			candidates[entry.TraceID] = struct{}{}
		}
		return candidates
	}

	// No filter, return all traces
	for id := range s.traces {
		candidates[id] = struct{}{}
	}
	return candidates
}

func (s *TraceStorage) matchesQuery(trace *Trace, query *TraceQuery) bool {
	// Service name filter
	if query.ServiceName != "" {
		found := false
		for _, svc := range trace.Services {
			if svc == query.ServiceName {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Time range filter
	if query.StartTime != nil && trace.StartTime.Before(*query.StartTime) {
		return false
	}
	if query.EndTime != nil && trace.EndTime.After(*query.EndTime) {
		return false
	}

	// Duration filter
	if query.MinDuration > 0 && trace.Duration < query.MinDuration {
		return false
	}
	if query.MaxDuration > 0 && trace.Duration > query.MaxDuration {
		return false
	}

	// Error filter
	if query.HasError != nil && trace.HasError != *query.HasError {
		return false
	}

	// Span name filter (substring match)
	if query.SpanName != "" {
		found := false
		for _, span := range trace.Spans {
			if containsSubstring(span.Name, query.SpanName) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Attribute filter
	if len(query.Attributes) > 0 {
		for _, span := range trace.Spans {
			if matchesAttributes(span.Attributes, query.Attributes) {
				return true
			}
		}
		return false
	}

	return true
}

func (s *TraceStorage) sortTraces(traces []*Trace, query *TraceQuery) {
	orderBy := query.OrderBy
	if orderBy == "" {
		orderBy = "start_time"
	}

	sort.Slice(traces, func(i, j int) bool {
		var cmp int
		switch orderBy {
		case "duration":
			cmp = int(traces[i].Duration - traces[j].Duration)
		case "span_count":
			cmp = traces[i].SpanCount - traces[j].SpanCount
		default: // start_time
			if traces[i].StartTime.Before(traces[j].StartTime) {
				cmp = -1
			} else if traces[i].StartTime.After(traces[j].StartTime) {
				cmp = 1
			}
		}
		if query.Ascending {
			return cmp < 0
		}
		return cmp > 0
	})
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsSubstringHelper(s, substr)))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func matchesAttributes(attrs, query map[string]string) bool {
	for k, v := range query {
		if attrs[k] != v {
			return false
		}
	}
	return true
}

// ============================================================================
// WOTAN INTEGRATION - PUBLISHING TO THE KINGDOM
// ============================================================================

// WotanPublisher publishes traces to Wotan
type WotanPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Close() error
}

// WotanConfig configures Wotan integration
type WotanConfig struct {
	Address           string        `json:"address"`
	TraceTopic        string        `json:"trace_topic"`
	SpanTopic         string        `json:"span_topic"`
	KernelEventTopic  string        `json:"kernel_event_topic"`
	PublishTimeout    time.Duration `json:"publish_timeout_ns"`
	BatchSize         int           `json:"batch_size"`
	FlushInterval     time.Duration `json:"flush_interval_ns"`
}

// ============================================================================
// COLLECTOR - THE TRACE WEAVER
// ============================================================================

// CollectorConfig configures the trace collector
type CollectorConfig struct {
	// gRPC server address
	GRPCAddr string `json:"grpc_addr"`

	// HTTP API address
	HTTPAddr string `json:"http_addr"`

	// Storage configuration
	Storage StorageConfig `json:"storage"`

	// Sampling configuration
	Sampling SamplerConfig `json:"sampling"`

	// Wotan configuration
	Wotan WotanConfig `json:"wotan"`

	// Correlation settings
	CorrelationWindow  time.Duration `json:"correlation_window_ns"`
	CorrelationInterval time.Duration `json:"correlation_interval_ns"`

	// Cleanup settings
	CleanupInterval time.Duration `json:"cleanup_interval_ns"`
}

// Collector is the main trace collection service
type Collector struct {
	config  CollectorConfig
	storage *TraceStorage
	sampler *Sampler

	// Pending spans by trace ID (for correlation)
	pendingSpans   map[TraceID][]*Span
	pendingKernel  map[TraceID][]*KernelEvent
	pendingMu      sync.RWMutex

	// Wotan publisher
	wotan WotanPublisher

	// Stream subscribers
	streamSubs map[chan *Trace]*StreamFilter
	streamMu   sync.RWMutex

	// HTTP server
	httpServer *http.Server

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed int32

	// Metrics
	metrics *collectorMetrics
}

type collectorMetrics struct {
	spansReceived    prometheus.Counter
	kernelEvents     prometheus.Counter
	tracesStored     prometheus.Counter
	tracesEvicted    prometheus.Counter
	storageBytes     prometheus.Gauge
	correlationTime  prometheus.Histogram
	queryLatency     prometheus.Histogram
	samplingRejected prometheus.Counter
	wotanPublished  prometheus.Counter
	wotanErrors     prometheus.Counter
}

func newCollectorMetrics(reg prometheus.Registerer) *collectorMetrics {
	m := &collectorMetrics{
		spansReceived: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "unheaded_traces_spans_received_total",
			Help: "Total number of spans received",
		}),
		kernelEvents: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "unheaded_traces_kernel_events_total",
			Help: "Total number of kernel events received",
		}),
		tracesStored: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "unheaded_traces_stored_total",
			Help: "Total number of traces stored",
		}),
		tracesEvicted: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "unheaded_traces_evicted_total",
			Help: "Total number of traces evicted",
		}),
		storageBytes: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "unheaded_traces_storage_bytes",
			Help: "Current trace storage size in bytes",
		}),
		correlationTime: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Name:    "unheaded_traces_correlation_duration_seconds",
			Help:    "Time taken to correlate traces",
			Buckets: prometheus.DefBuckets,
		}),
		queryLatency: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Name:    "unheaded_traces_query_duration_seconds",
			Help:    "Time taken to execute trace queries",
			Buckets: prometheus.DefBuckets,
		}),
		samplingRejected: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "unheaded_traces_sampling_rejected_total",
			Help: "Total number of spans rejected by sampling",
		}),
		wotanPublished: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "unheaded_traces_wotan_published_total",
			Help: "Total number of traces published to Wotan",
		}),
		wotanErrors: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "unheaded_traces_wotan_errors_total",
			Help: "Total number of Wotan publish errors",
		}),
	}
	return m
}

// NewCollector creates a new trace collector
func NewCollector(config CollectorConfig, wotan WotanPublisher, reg prometheus.Registerer) (*Collector, error) {
	// Set defaults
	if config.CorrelationWindow <= 0 {
		config.CorrelationWindow = 5 * time.Second
	}
	if config.CorrelationInterval <= 0 {
		config.CorrelationInterval = 1 * time.Second
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 1 * time.Minute
	}
	if config.HTTPAddr == "" {
		config.HTTPAddr = ports.DefaultAddr(ports.TraceCollectorHTTP)
	}

	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Collector{
		config:        config,
		storage:       NewTraceStorage(config.Storage),
		sampler:       NewSampler(config.Sampling),
		pendingSpans:  make(map[TraceID][]*Span),
		pendingKernel: make(map[TraceID][]*KernelEvent),
		wotan:        wotan,
		streamSubs:    make(map[chan *Trace]*StreamFilter),
		ctx:           ctx,
		cancel:        cancel,
		metrics:       newCollectorMetrics(reg),
	}

	return c, nil
}

// StartCollector starts all collector components
func (c *Collector) StartCollector(ctx context.Context) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrCollectorClosed
	}

	// Start correlation loop
	c.wg.Add(1)
	go c.correlationLoop()

	// Start cleanup loop
	c.wg.Add(1)
	go c.cleanupLoop()

	// Start HTTP server
	c.wg.Add(1)
	go c.serveHTTP()

	// Start metrics updater
	c.wg.Add(1)
	go c.metricsLoop()

	return nil
}

// IngestSpan receives a span from a service
func (c *Collector) IngestSpan(ctx context.Context, span *Span) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrCollectorClosed
	}

	// Apply sampling
	if !c.sampler.ShouldSample(span) {
		c.metrics.samplingRejected.Inc()
		return ErrSamplingRejected
	}

	c.metrics.spansReceived.Inc()

	// Calculate duration if not set
	if span.Duration == 0 && !span.EndTime.IsZero() && !span.StartTime.IsZero() {
		span.Duration = span.EndTime.Sub(span.StartTime)
	}

	// Add to pending for correlation
	c.pendingMu.Lock()
	c.pendingSpans[span.TraceID] = append(c.pendingSpans[span.TraceID], span)
	c.pendingMu.Unlock()

	return nil
}

// IngestKernelEvent receives an event from eBPF
func (c *Collector) IngestKernelEvent(ctx context.Context, event *KernelEvent) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrCollectorClosed
	}

	c.metrics.kernelEvents.Inc()

	// If event has trace ID, correlate it
	if event.HasTraceID() {
		traceID := event.GetTraceID()
		c.pendingMu.Lock()
		c.pendingKernel[traceID] = append(c.pendingKernel[traceID], event)
		c.pendingMu.Unlock()
	}

	return nil
}

// CorrelateTraces links kernel and application spans
func (c *Collector) CorrelateTraces() error {
	start := time.Now()
	defer func() {
		c.metrics.correlationTime.Observe(time.Since(start).Seconds())
	}()

	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	cutoff := time.Now().Add(-c.config.CorrelationWindow)

	// Process pending spans
	for traceID, spans := range c.pendingSpans {
		// Check if we have enough data or spans are old enough
		shouldProcess := false
		for _, span := range spans {
			if span.StartTime.Before(cutoff) {
				shouldProcess = true
				break
			}
		}

		if !shouldProcess && len(spans) < 10 {
			continue // Wait for more data
		}

		// Create or update trace
		trace, err := c.storage.Get(traceID)
		if err == ErrTraceNotFound {
			trace = &Trace{
				TraceID: traceID,
				Spans:   make([]*Span, 0),
			}
		}

		// Add spans to trace
		for _, span := range spans {
			trace.AddSpan(span)
		}

		// Add kernel events
		if kernelEvents, ok := c.pendingKernel[traceID]; ok {
			for _, ke := range kernelEvents {
				trace.KernelEvents = append(trace.KernelEvents, ke)
				trace.KernelLatencyNs += ke.LatencyNs

				// Mark correlated spans
				for _, span := range trace.Spans {
					if span.ContainerID == ke.ContainerID ||
						(ke.PID > 0 && span.Attributes["pid"] == fmt.Sprintf("%d", ke.PID)) {
						span.KernelCorrelated = true
						span.KernelEventID = ke.EventID
					}
				}
			}
			delete(c.pendingKernel, traceID)
		}

		// Store trace
		if err := c.storage.Store(trace); err != nil {
			continue
		}
		c.metrics.tracesStored.Inc()

		// Publish to stream subscribers
		c.notifyStreamSubscribers(trace)

		// Publish to Wotan
		c.publishToWotan(trace)

		// Remove from pending
		delete(c.pendingSpans, traceID)
	}

	// Clean up old kernel events without matching spans
	for traceID, events := range c.pendingKernel {
		allOld := true
		for _, event := range events {
			if event.Timestamp.After(cutoff) {
				allOld = false
				break
			}
		}
		if allOld {
			delete(c.pendingKernel, traceID)
		}
	}

	return nil
}

// GetTrace retrieves a full trace by ID
func (c *Collector) GetTrace(ctx context.Context, traceID TraceID) (*Trace, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrCollectorClosed
	}

	start := time.Now()
	defer func() {
		c.metrics.queryLatency.Observe(time.Since(start).Seconds())
	}()

	return c.storage.Get(traceID)
}

// QueryTraces searches traces based on query parameters
func (c *Collector) QueryTraces(ctx context.Context, query *TraceQuery) (*TraceQueryResult, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrCollectorClosed
	}

	start := time.Now()
	defer func() {
		c.metrics.queryLatency.Observe(time.Since(start).Seconds())
	}()

	return c.storage.Query(query)
}

// StreamTraces returns a channel of traces matching the filter
func (c *Collector) StreamTraces(ctx context.Context, filter *StreamFilter) (<-chan *Trace, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrCollectorClosed
	}

	ch := make(chan *Trace, 100)

	c.streamMu.Lock()
	c.streamSubs[ch] = filter
	c.streamMu.Unlock()

	// Clean up when context is done
	go func() {
		<-ctx.Done()
		c.streamMu.Lock()
		delete(c.streamSubs, ch)
		c.streamMu.Unlock()
		close(ch)
	}()

	return ch, nil
}

// Close stops the collector and releases resources
func (c *Collector) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil // Already closed
	}

	c.cancel()

	// Close HTTP server
	if c.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.httpServer.Shutdown(ctx)
	}

	// Close stream subscribers
	c.streamMu.Lock()
	for ch := range c.streamSubs {
		close(ch)
	}
	c.streamSubs = nil
	c.streamMu.Unlock()

	c.wg.Wait()
	return nil
}

// correlationLoop periodically correlates pending traces
func (c *Collector) correlationLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.CorrelationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.CorrelateTraces()
		}
	}
}

// cleanupLoop periodically cleans up expired traces
func (c *Collector) cleanupLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			removed := c.storage.Cleanup()
			if removed > 0 {
				c.metrics.tracesEvicted.Add(float64(removed))
			}
		}
	}
}

// metricsLoop updates storage metrics
func (c *Collector) metricsLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			stats := c.storage.Stats()
			c.metrics.storageBytes.Set(float64(stats.StorageBytes))
		}
	}
}

// notifyStreamSubscribers sends trace to matching subscribers
func (c *Collector) notifyStreamSubscribers(trace *Trace) {
	c.streamMu.RLock()
	defer c.streamMu.RUnlock()

	for ch, filter := range c.streamSubs {
		if c.matchesStreamFilter(trace, filter) {
			select {
			case ch <- trace:
			default:
				// Channel full, skip
			}
		}
	}
}

func (c *Collector) matchesStreamFilter(trace *Trace, filter *StreamFilter) bool {
	if filter == nil {
		return true
	}

	// Filter by service names
	if len(filter.ServiceNames) > 0 {
		found := false
		for _, svc := range trace.Services {
			for _, filterSvc := range filter.ServiceNames {
				if svc == filterSvc {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by duration
	if filter.MinDuration > 0 && trace.Duration < filter.MinDuration {
		return false
	}

	// Filter errors only
	if filter.ErrorsOnly && !trace.HasError {
		return false
	}

	return true
}

// publishToWotan publishes trace to Wotan
func (c *Collector) publishToWotan(trace *Trace) {
	if c.wotan == nil {
		fmt.Fprintf(os.Stderr, "tracing: wotan unavailable — trace event dropped (degraded mode)\n")
		return
	}

	data, err := json.Marshal(trace)
	if err != nil {
		c.metrics.wotanErrors.Inc()
		return
	}

	topic := c.config.Wotan.TraceTopic
	if topic == "" {
		topic = "traces.collected"
	}

	ctx, cancel := context.WithTimeout(c.ctx, c.config.Wotan.PublishTimeout)
	defer cancel()

	if err := c.wotan.Publish(ctx, topic, data); err != nil {
		c.metrics.wotanErrors.Inc()
		return
	}

	c.metrics.wotanPublished.Inc()
}

// ============================================================================
// HTTP API - THE QUERY INTERFACE
// ============================================================================

func (c *Collector) serveHTTP() {
	defer c.wg.Done()

	mux := http.NewServeMux()

	// Health and readiness
	mux.HandleFunc("/health", c.handleHealth)
	mux.HandleFunc("/ready", c.handleReady)

	// Trace API
	mux.HandleFunc("/api/v1/traces", c.handleTraces)
	mux.HandleFunc("/api/v1/traces/", c.handleTrace)
	mux.HandleFunc("/api/v1/traces/stream", c.handleTraceStream)

	// Stats
	mux.HandleFunc("/api/v1/stats", c.handleStats)

	c.httpServer = &http.Server{
		Addr:         c.config.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	if err := c.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		// Log error
	}
}

func (c *Collector) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}

func (c *Collector) handleReady(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&c.closed) == 1 {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"not ready"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ready"}`))
}

func (c *Collector) handleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := &TraceQuery{}

	if svc := r.URL.Query().Get("service"); svc != "" {
		query.ServiceName = svc
	}

	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			query.StartTime = &t
		}
	}

	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			query.EndTime = &t
		}
	}

	if minDur := r.URL.Query().Get("min_duration"); minDur != "" {
		if d, err := time.ParseDuration(minDur); err == nil {
			query.MinDuration = d
		}
	}

	if maxDur := r.URL.Query().Get("max_duration"); maxDur != "" {
		if d, err := time.ParseDuration(maxDur); err == nil {
			query.MaxDuration = d
		}
	}

	if hasError := r.URL.Query().Get("has_error"); hasError != "" {
		b := hasError == "true"
		query.HasError = &b
	}

	if spanName := r.URL.Query().Get("span_name"); spanName != "" {
		query.SpanName = spanName
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := parseInt(limitStr); err == nil {
			query.Limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := parseInt(offsetStr); err == nil {
			query.Offset = o
		}
	}

	query.OrderBy = r.URL.Query().Get("order_by")
	query.Ascending = r.URL.Query().Get("ascending") == "true"

	result, err := c.QueryTraces(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (c *Collector) handleTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract trace ID from path
	path := r.URL.Path
	if len(path) < len("/api/v1/traces/") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	traceIDStr := path[len("/api/v1/traces/"):]
	var traceID TraceID
	if err := traceID.FromString(traceIDStr); err != nil {
		http.Error(w, "invalid trace ID", http.StatusBadRequest)
		return
	}

	trace, err := c.GetTrace(r.Context(), traceID)
	if err == ErrTraceNotFound {
		http.Error(w, "trace not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trace)
}

func (c *Collector) handleTraceStream(w http.ResponseWriter, r *http.Request) {
	// Set up SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Parse filter
	filter := &StreamFilter{}
	if services := r.URL.Query().Get("services"); services != "" {
		filter.ServiceNames = splitComma(services)
	}
	if minDur := r.URL.Query().Get("min_duration"); minDur != "" {
		if d, err := time.ParseDuration(minDur); err == nil {
			filter.MinDuration = d
		}
	}
	filter.ErrorsOnly = r.URL.Query().Get("errors_only") == "true"

	// Subscribe to stream
	ch, err := c.StreamTraces(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Stream traces
	for trace := range ch {
		data, err := json.Marshal(trace)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

func (c *Collector) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := c.storage.Stats()

	c.pendingMu.RLock()
	pendingSpans := len(c.pendingSpans)
	pendingKernel := len(c.pendingKernel)
	c.pendingMu.RUnlock()

	c.streamMu.RLock()
	streamSubs := len(c.streamSubs)
	c.streamMu.RUnlock()

	response := map[string]interface{}{
		"storage": stats,
		"pending": map[string]int{
			"spans":         pendingSpans,
			"kernel_events": pendingKernel,
		},
		"stream_subscribers": streamSubs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ============================================================================
// HELPERS
// ============================================================================

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if start < i {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

// ============================================================================
// OTEL COMPATIBILITY - PROTOCOL BRIDGE
// ============================================================================

// OTLPSpan represents an OpenTelemetry span (simplified)
type OTLPSpan struct {
	TraceID           []byte            `json:"trace_id"`
	SpanID            []byte            `json:"span_id"`
	ParentSpanID      []byte            `json:"parent_span_id"`
	Name              string            `json:"name"`
	Kind              int32             `json:"kind"`
	StartTimeUnixNano uint64            `json:"start_time_unix_nano"`
	EndTimeUnixNano   uint64            `json:"end_time_unix_nano"`
	Attributes        map[string]string `json:"attributes"`
	Status            struct {
		Code    int32  `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Events []struct {
		Name              string            `json:"name"`
		TimeUnixNano      uint64            `json:"time_unix_nano"`
		Attributes        map[string]string `json:"attributes"`
	} `json:"events"`
}

// ConvertOTLPSpan converts an OTLP span to our internal Span type
func ConvertOTLPSpan(otlp *OTLPSpan, serviceName, serviceVersion, hostName string) (*Span, error) {
	traceID, err := TraceIDFromBytes(otlp.TraceID)
	if err != nil {
		return nil, err
	}

	spanID, err := SpanIDFromBytes(otlp.SpanID)
	if err != nil {
		return nil, err
	}

	var parentSpanID SpanID
	if len(otlp.ParentSpanID) > 0 {
		parentSpanID, _ = SpanIDFromBytes(otlp.ParentSpanID)
	}

	span := &Span{
		TraceID:        traceID,
		SpanID:         spanID,
		ParentSpanID:   parentSpanID,
		Name:           otlp.Name,
		Kind:           SpanKind(otlp.Kind),
		StartTime:      time.Unix(0, int64(otlp.StartTimeUnixNano)),
		EndTime:        time.Unix(0, int64(otlp.EndTimeUnixNano)),
		Attributes:     otlp.Attributes,
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		HostName:       hostName,
	}

	span.Duration = span.EndTime.Sub(span.StartTime)

	// Convert status
	switch otlp.Status.Code {
	case 0:
		span.Status = StatusUnset
	case 1:
		span.Status = StatusOK
	case 2:
		span.Status = StatusError
		span.StatusMsg = otlp.Status.Message
	}

	// Convert events
	for _, e := range otlp.Events {
		span.Events = append(span.Events, SpanEvent{
			Name:       e.Name,
			Timestamp:  time.Unix(0, int64(e.TimeUnixNano)),
			Attributes: e.Attributes,
		})
	}

	return span, nil
}

// ============================================================================
// KERNEL EVENT PARSING - DECODING THE VOID'S WHISPERS
// ============================================================================

// ParseKernelEvent parses raw eBPF event data into a KernelEvent
func ParseKernelEvent(data []byte) (*KernelEvent, error) {
	if len(data) < 40 {
		return nil, errors.New("kernel event too short")
	}

	event := &KernelEvent{
		Timestamp: time.Now(),
		RawData:   data,
	}

	// Parse header (matches eBPF event structure)
	// Layout: event_id(8) | type(4) | cpu(4) | pid(4) | tid(4) | timestamp_ns(8) | ...
	event.EventID = binary.LittleEndian.Uint64(data[0:8])
	event.Type = KernelEventType(binary.LittleEndian.Uint32(data[8:12]))
	event.CPU = binary.LittleEndian.Uint32(data[12:16])
	event.PID = binary.LittleEndian.Uint32(data[16:20])
	event.TID = binary.LittleEndian.Uint32(data[20:24])

	timestampNs := binary.LittleEndian.Uint64(data[24:32])
	event.Timestamp = time.Unix(0, int64(timestampNs))

	// Parse type-specific data
	switch event.Type {
	case KernelEventPacketRx, KernelEventPacketTx:
		if len(data) >= 72 {
			event.SrcIP = net.IP(data[32:36])
			event.DstIP = net.IP(data[36:40])
			event.SrcPort = binary.BigEndian.Uint16(data[40:42])
			event.DstPort = binary.BigEndian.Uint16(data[42:44])
			event.Proto = data[44]
			event.TraceIDHigh = binary.BigEndian.Uint64(data[48:56])
			event.TraceIDLow = binary.BigEndian.Uint64(data[56:64])
			event.LatencyNs = binary.LittleEndian.Uint64(data[64:72])
		}

	case KernelEventSyscall:
		if len(data) >= 48 {
			event.LatencyNs = binary.LittleEndian.Uint64(data[32:40])
			// Comm is null-terminated string
			commEnd := 40
			for i := 40; i < len(data) && i < 56; i++ {
				if data[i] == 0 {
					break
				}
				commEnd = i + 1
			}
			event.Comm = string(data[40:commEnd])
		}

	case KernelEventNetConnect, KernelEventNetClose:
		if len(data) >= 60 {
			event.SrcIP = net.IP(data[32:36])
			event.DstIP = net.IP(data[36:40])
			event.SrcPort = binary.BigEndian.Uint16(data[40:42])
			event.DstPort = binary.BigEndian.Uint16(data[42:44])
			event.Proto = data[44]
			event.LatencyNs = binary.LittleEndian.Uint64(data[48:56])
		}
	}

	return event, nil
}
