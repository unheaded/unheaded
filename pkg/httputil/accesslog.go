// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package httputil

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// probePaths are the paths health checks and scrapers hit every few
// seconds. Answered well, they are a debug line: at info they were most of
// west's log lines (ADR-147, Developer D15: 247 lines a minute, the
// dashboard 114 of them, kanban 28).
var probePaths = map[string]bool{"/health": true, "/ready": true, "/live": true, "/metrics": true}

// AccessRecord is one answered request.
type AccessRecord struct {
	Method     string
	Path       string
	Status     int
	Duration   time.Duration
	RemoteAddr string
	RequestID  string
}

// Level is the level the request logs at: a probe path answered below 400
// is debug and one answered 400 or above warn; every other request info.
func (a AccessRecord) Level() zerolog.Level {
	if !probePaths[a.Path] {
		return zerolog.InfoLevel
	}
	if a.Status >= http.StatusBadRequest {
		return zerolog.WarnLevel
	}
	return zerolog.DebugLevel
}

// AccessLog logs one line per request to l at the record's Level: method,
// path, status, duration, remote address and the request id requestID finds
// in the context (nil: none), with the message "request".
func AccessLog(next http.Handler, l zerolog.Logger, requestID func(context.Context) string) http.Handler {
	return AccessLogFunc(next, func(a AccessRecord) {
		l.WithLevel(a.Level()).
			Str("method", a.Method).
			Str("path", a.Path).
			Int("status", a.Status).
			Dur("duration", a.Duration).
			Str("remote_addr", a.RemoteAddr).
			Str("request_id", a.RequestID).
			Msg("request")
	}, requestID)
}

// AccessLogFunc hands each answered request to emit, for a service whose
// logger is not zerolog's; emit decides the line from the record and its
// Level.
func AccessLogFunc(next http.Handler, emit func(AccessRecord), requestID func(context.Context) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		a := AccessRecord{
			Method:     r.Method,
			Path:       r.URL.Path,
			Status:     rec.status,
			Duration:   time.Since(start),
			RemoteAddr: r.RemoteAddr,
		}
		if requestID != nil {
			a.RequestID = requestID(r.Context())
		}
		emit(a)
	})
}
