// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// ServiceMetrics holds the HTTP metrics CLAUDE.md requires of every service:
// unheaded_http_requests_total and unheaded_http_request_duration_seconds.
type ServiceMetrics struct {
	RequestsTotal   *prom.CounterVec
	RequestDuration *prom.HistogramVec
}

var (
	serviceMetricsMu sync.Mutex
	serviceMetrics   = map[string]*ServiceMetrics{}
)

// NewServiceMetrics returns the standard HTTP metrics for a service,
// registered on the default registry. The registration is process-global, so
// a second call for the same service returns the first one's metrics rather
// than panicking on a duplicate: servers built more than once in a process
// (tests, restarts) share one set of series.
func NewServiceMetrics(serviceName string) *ServiceMetrics {
	serviceMetricsMu.Lock()
	defer serviceMetricsMu.Unlock()
	if m, ok := serviceMetrics[serviceName]; ok {
		return m
	}
	m := &ServiceMetrics{
		RequestsTotal: auto.NewCounterVec(
			prom.CounterOpts{
				Name:        "unheaded_http_requests_total",
				Help:        "Total HTTP requests",
				ConstLabels: prom.Labels{"service": serviceName},
			},
			[]string{"method", "path", "status"},
		),
		RequestDuration: auto.NewHistogramVec(
			prom.HistogramOpts{
				Name:        "unheaded_http_request_duration_seconds",
				Help:        "HTTP request latency",
				ConstLabels: prom.Labels{"service": serviceName},
				Buckets:     prom.DefBuckets,
			},
			[]string{"method", "path"},
		),
	}
	serviceMetrics[serviceName] = m
	return m
}

// Instrument records every request that reaches next. mux is only consulted
// for the label: path is the ServeMux pattern the request matches
// ("unmatched" if none), never r.URL.Path, which would let any client mint a
// series per request. next is normally mux wrapped in the service's
// middleware (auth, body limits), so rejections by that middleware are
// counted too.
//
// Every label is bounded: path by the mux's pattern set, method by folding
// non-standard verbs to "OTHER", status by being a numeric code.
func (m *ServiceMetrics) Instrument(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		_, path := mux.Handler(r)
		if path == "" {
			path = "unmatched"
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		method := boundedMethod(r.Method)
		m.RequestsTotal.WithLabelValues(method, path, strconv.Itoa(rec.status)).Inc()
		m.RequestDuration.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	})
}

func boundedMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return m
	}
	return "OTHER"
}

// statusRecorder captures the status code while keeping the interfaces
// handlers assert directly: websockets hijack and streams flush.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		r.wroteHeader = true
		f.Flush()
	}
}

// Hijack records 101: a hijacked connection is a protocol switch, and the
// handler writes its status line to the raw conn, where this cannot see it.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
	}
	conn, rw, err := hj.Hijack()
	if err == nil {
		r.status = http.StatusSwitchingProtocols
		r.wroteHeader = true
	}
	return conn, rw, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
