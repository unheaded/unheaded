// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestSize     *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// Room metrics
	RoomsTotal        prometheus.Gauge
	RoomMessagesTotal *prometheus.CounterVec
	RoomBufferSize    *prometheus.GaugeVec
	RoomBufferUsage   *prometheus.GaugeVec
	RoomBufferWrapped *prometheus.CounterVec

	// Member metrics
	MembersTotal       *prometheus.GaugeVec
	MembersPending     prometheus.Gauge
	MembersApproved    prometheus.Gauge
	MemberJoinRequests *prometheus.CounterVec

	// Message metrics
	MessagesCreated     *prometheus.CounterVec
	MessagesDeleted     *prometheus.CounterVec
	MessageDeleteFailed *prometheus.CounterVec

	// gRPC streaming metrics
	StreamsActive      *prometheus.GaugeVec
	StreamsTotal       *prometheus.CounterVec
	StreamMessagesSent *prometheus.CounterVec
	StreamErrors       *prometheus.CounterVec

	// System metrics
	GoroutinesActive prometheus.Gauge
	MemoryAllocated  prometheus.Gauge
}

var defaultMetrics *Metrics

// Initialize creates and registers all Prometheus metrics
func Initialize(namespace string) *Metrics {
	m := &Metrics{
		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_size_bytes",
				Help:      "HTTP request size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),

		// Room metrics
		RoomsTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rooms_total",
				Help:      "Total number of active rooms",
			},
		),
		RoomMessagesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "room_messages_total",
				Help:      "Total messages in each room",
			},
			[]string{"room_id"},
		),
		RoomBufferSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "room_buffer_size",
				Help:      "Ring buffer size for each room",
			},
			[]string{"room_id"},
		),
		RoomBufferUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "room_buffer_usage",
				Help:      "Current ring buffer usage for each room",
			},
			[]string{"room_id"},
		),
		RoomBufferWrapped: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "room_buffer_wrapped_total",
				Help:      "Number of times ring buffer wrapped for each room",
			},
			[]string{"room_id"},
		),

		// Member metrics
		MembersTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "members_total",
				Help:      "Total members by status",
			},
			[]string{"status"},
		),
		MembersPending: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "members_pending",
				Help:      "Number of pending member requests",
			},
		),
		MembersApproved: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "members_approved",
				Help:      "Number of approved members",
			},
		),
		MemberJoinRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "member_join_requests_total",
				Help:      "Total member join requests",
			},
			[]string{"room_id", "result"},
		),

		// Message metrics
		MessagesCreated: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "messages_created_total",
				Help:      "Total messages created",
			},
			[]string{"room_id"},
		),
		MessagesDeleted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "messages_deleted_total",
				Help:      "Total messages deleted",
			},
			[]string{"room_id"},
		),
		MessageDeleteFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "messages_delete_failed_total",
				Help:      "Total failed message deletion attempts",
			},
			[]string{"room_id", "reason"},
		),

		// gRPC streaming metrics
		StreamsActive: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "streams_active",
				Help:      "Number of active gRPC streams",
			},
			[]string{"room_id"},
		),
		StreamsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "streams_total",
				Help:      "Total gRPC streams created",
			},
			[]string{"room_id"},
		),
		StreamMessagesSent: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "stream_messages_sent_total",
				Help:      "Total messages sent via streams",
			},
			[]string{"room_id", "event_type"},
		),
		StreamErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "stream_errors_total",
				Help:      "Total stream errors",
			},
			[]string{"room_id", "error_type"},
		),

		// System metrics
		GoroutinesActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "goroutines_active",
				Help:      "Number of active goroutines",
			},
		),
		MemoryAllocated: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memory_allocated_bytes",
				Help:      "Memory allocated in bytes",
			},
		),
	}

	defaultMetrics = m
	initPackageVars(m)
	return m
}

// Get returns the default metrics instance
func Get() *Metrics {
	return defaultMetrics
}

// RecordHTTPRequest records HTTP request metrics
func (m *Metrics) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration) {
	status := prometheus.Labels{
		"method": method,
		"path":   path,
		"status": string(rune(statusCode/100)) + "xx",
	}
	m.HTTPRequestsTotal.With(status).Inc()
	m.HTTPRequestDuration.With(status).Observe(duration.Seconds())
}

// RecordMessageCreated records message creation
func (m *Metrics) RecordMessageCreated(roomID string, overwritten bool) {
	m.MessagesCreated.WithLabelValues(roomID).Inc()
	if overwritten {
		m.RoomBufferWrapped.WithLabelValues(roomID).Inc()
	}
}

// RecordMessageDeleted records message deletion
func (m *Metrics) RecordMessageDeleted(roomID string, success bool, reason string) {
	if success {
		m.MessagesDeleted.WithLabelValues(roomID).Inc()
	} else {
		m.MessageDeleteFailed.WithLabelValues(roomID, reason).Inc()
	}
}

// UpdateRoomBufferMetrics updates room buffer usage metrics
func (m *Metrics) UpdateRoomBufferMetrics(roomID string, size, usage int, wrapped bool) {
	m.RoomBufferSize.WithLabelValues(roomID).Set(float64(size))
	m.RoomBufferUsage.WithLabelValues(roomID).Set(float64(usage))
}

// RecordStreamCreated records stream creation
func (m *Metrics) RecordStreamCreated(roomID string) {
	m.StreamsActive.WithLabelValues(roomID).Inc()
	m.StreamsTotal.WithLabelValues(roomID).Inc()
}

// RecordStreamClosed records stream closure
func (m *Metrics) RecordStreamClosed(roomID string) {
	m.StreamsActive.WithLabelValues(roomID).Dec()
}

// RecordStreamMessage records message sent via stream
func (m *Metrics) RecordStreamMessage(roomID, eventType string) {
	m.StreamMessagesSent.WithLabelValues(roomID, eventType).Inc()
}

// RecordStreamError records stream error
func (m *Metrics) RecordStreamError(roomID, errorType string) {
	m.StreamErrors.WithLabelValues(roomID, errorType).Inc()
}

// Package-level metric accessors for convenience
var (
	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestSize     *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// Room metrics
	RoomsTotal        prometheus.Gauge
	RoomMessagesTotal *prometheus.CounterVec
	RoomBufferSize    *prometheus.GaugeVec
	RoomBufferUsage   *prometheus.GaugeVec
	RoomBufferWrapped *prometheus.CounterVec

	// Member metrics
	MembersTotal       *prometheus.GaugeVec
	MembersPending     prometheus.Gauge
	MembersApproved    prometheus.Gauge
	MemberJoinRequests *prometheus.CounterVec

	// Message metrics
	MessagesCreated     *prometheus.CounterVec
	MessagesDeleted     *prometheus.CounterVec
	MessageDeleteFailed *prometheus.CounterVec

	// gRPC streaming metrics
	StreamsActive      *prometheus.GaugeVec
	StreamsTotal       *prometheus.CounterVec
	StreamMessagesSent *prometheus.CounterVec
	StreamErrors       *prometheus.CounterVec

	// System metrics
	GoroutinesActive prometheus.Gauge
	MemoryAllocated  prometheus.Gauge
)

// initPackageVars initializes package-level variables
func initPackageVars(m *Metrics) {
	HTTPRequestsTotal = m.HTTPRequestsTotal
	HTTPRequestDuration = m.HTTPRequestDuration
	HTTPRequestSize = m.HTTPRequestSize
	HTTPResponseSize = m.HTTPResponseSize

	RoomsTotal = m.RoomsTotal
	RoomMessagesTotal = m.RoomMessagesTotal
	RoomBufferSize = m.RoomBufferSize
	RoomBufferUsage = m.RoomBufferUsage
	RoomBufferWrapped = m.RoomBufferWrapped

	MembersTotal = m.MembersTotal
	MembersPending = m.MembersPending
	MembersApproved = m.MembersApproved
	MemberJoinRequests = m.MemberJoinRequests

	MessagesCreated = m.MessagesCreated
	MessagesDeleted = m.MessagesDeleted
	MessageDeleteFailed = m.MessageDeleteFailed

	StreamsActive = m.StreamsActive
	StreamsTotal = m.StreamsTotal
	StreamMessagesSent = m.StreamMessagesSent
	StreamErrors = m.StreamErrors

	GoroutinesActive = m.GoroutinesActive
	MemoryAllocated = m.MemoryAllocated
}
