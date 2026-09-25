// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"
)

// dashboard_http_requests_total and dashboard_http_request_duration_seconds
// were registered from the start and never incremented: nothing called them.
// instrumentHTTP is that missing call.
//
// Every label is bounded by construction, because each one is otherwise
// attacker-controlled:
//   - path is the ServeMux pattern that matched ("unmatched" if none), never
//     r.URL.Path, which would let any client mint a series per request;
//   - method folds to the standard verbs, anything else is "OTHER";
//   - status is a numeric code.

type patternKey struct{}

// patternHolder carries the matched pattern back out. The mux writes
// r.Pattern into the request it is handed, but auth passes it a copy
// (WithContext), so the outer middleware cannot read it off its own request.
// A pointer in the context survives those copies.
type patternHolder struct{ pattern string }

// capturePattern wraps the mux itself and reports which pattern matched.
func capturePattern(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		if h, ok := r.Context().Value(patternKey{}).(*patternHolder); ok {
			h.pattern = r.Pattern
		}
	})
}

func (s *Server) instrumentHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		holder := &patternHolder{}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), patternKey{}, holder)))

		path := holder.pattern
		if path == "" {
			path = "unmatched"
		}
		method := boundedMethod(r.Method)
		s.httpRequests.WithLabelValues(method, path, strconv.Itoa(rec.status)).Inc()
		s.httpDuration.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
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

// statusRecorder captures the status code while keeping the two interfaces
// handlers here assert directly: /ws hijacks and the log stream flushes.
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
