// Package logs provides the dashboard log viewer API.
// GET /api/v1/logs — query log entries from the ring buffer.
// WebSocket /ws/logs — live tail of new log entries.
package logs

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"unheaded/pkg/logagg"
)

// LogHandler serves the log query API endpoint.
type LogHandler struct {
	buffer *logagg.RingBuffer
}

// NewLogHandler creates a new log handler backed by the given buffer.
func NewLogHandler(buf *logagg.RingBuffer) *LogHandler {
	return &LogHandler{buffer: buf}
}

// ServeHTTP handles GET /api/v1/logs with query parameters:
//   - service: filter by service name
//   - level: filter by log level
//   - search: full-text search
//   - from: start time (RFC3339 or Unix timestamp)
//   - to: end time (RFC3339 or Unix timestamp)
//   - limit: max results (default 100, max 10000)
//   - offset: pagination offset
func (h *LogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := logagg.LogQuery{
		Service: r.URL.Query().Get("service"),
		Level:   r.URL.Query().Get("level"),
		Search:  r.URL.Query().Get("search"),
	}

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := parseTime(fromStr); err == nil {
			q.From = t
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := parseTime(toStr); err == nil {
			q.To = t
		}
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			q.Limit = n
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
			q.Offset = n
		}
	}

	entries := h.buffer.Query(q)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(entries)
}

// parseTime tries RFC3339 first, then Unix timestamp.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(ts, 0), nil
	}
	return time.Time{}, strconv.ErrRange
}
