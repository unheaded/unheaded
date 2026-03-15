// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package middleware

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
)

// responseWriter wraps http.ResponseWriter to capture status and size.
type responseWriter struct {
	http.ResponseWriter
	status      int
	size        int
	wroteHeader bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(status int) {
	if rw.wroteHeader {
		return
	}
	rw.status = status
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// Hijack implements the http.Hijacker interface.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Flush implements the http.Flusher interface.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// LoggingMiddleware logs HTTP requests.
type LoggingMiddleware struct {
	log              *logger.Logger
	metrics          *metrics.Registry
	requestsCounter  *metrics.CounterVec
	durationHist     *metrics.HistogramVec
	responseSizeHist *metrics.HistogramVec
}

// NewLoggingMiddleware creates a new logging middleware.
func NewLoggingMiddleware(log *logger.Logger, metricsReg *metrics.Registry) *LoggingMiddleware {
	labelNames := []string{"method", "path", "status"}

	requestsCounter := metrics.NewCounterVec(
		"gateway_requests_total",
		"Total number of HTTP requests",
		nil,
		labelNames,
	)

	durationHist := metrics.NewHistogramVec(
		metrics.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: metrics.DefaultBuckets,
		},
		labelNames,
	)

	responseSizeHist := metrics.NewHistogramVec(
		metrics.HistogramOpts{
			Name:    "gateway_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: metrics.ExponentialBuckets(100, 10, 6),
		},
		labelNames,
	)

	if metricsReg != nil {
		metricsReg.Register(requestsCounter)
		metricsReg.Register(durationHist)
		metricsReg.Register(responseSizeHist)
	}

	return &LoggingMiddleware{
		log:              log,
		metrics:          metricsReg,
		requestsCounter:  requestsCounter,
		durationHist:     durationHist,
		responseSizeHist: responseSizeHist,
	}
}

// Handler returns the middleware handler.
func (m *LoggingMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Get trace ID from context
		traceID := GetTraceID(r.Context())

		// Wrap response writer to capture status and size
		rw := newResponseWriter(w)

		// Process request
		next.ServeHTTP(rw, r)

		// Calculate duration
		duration := time.Since(start)

		// Get client IP
		clientIP := getClientIP(r)

		// Log the request
		m.log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("query", r.URL.RawQuery).
			Int("status", rw.status).
			Int("size", rw.size).
			Int64("duration_ms", duration.Milliseconds()).
			Str("client_ip", clientIP).
			Str("user_agent", r.UserAgent()).
			Str("trace_id", traceID).
			Str("referer", r.Referer()).
			Str("protocol", r.Proto).
			Msg("HTTP request")

		// Record metrics
		if m.metrics != nil {
			labels := metrics.Labels{
				"method": r.Method,
				"path":   normalizePath(r.URL.Path),
				"status": statusCodeClass(rw.status),
			}

			m.requestsCounter.WithLabels(labels).Inc()
			m.durationHist.WithLabels(labels).Observe(duration.Seconds())
			m.responseSizeHist.WithLabels(labels).Observe(float64(rw.size))
		}
	})
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// normalizePath normalizes a path for metrics labels.
func normalizePath(path string) string {
	// Replace numeric path segments with placeholders
	// e.g., /users/123 -> /users/:id
	result := make([]byte, 0, len(path))
	inSegment := false
	isNumeric := true

	for i := 0; i < len(path); i++ {
		c := path[i]
		if c == '/' {
			if inSegment && isNumeric {
				result = append(result, ":id"...)
			}
			result = append(result, '/')
			inSegment = false
			isNumeric = true
		} else {
			inSegment = true
			if c < '0' || c > '9' {
				isNumeric = false
			}
			if !isNumeric {
				result = append(result, c)
			}
		}
	}

	if inSegment && isNumeric {
		result = append(result, ":id"...)
	}

	if len(result) == 0 {
		return "/"
	}
	return string(result)
}

// statusCodeClass returns the status code class (2xx, 3xx, etc.).
func statusCodeClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500:
		return "5xx"
	default:
		return "unknown"
	}
}
