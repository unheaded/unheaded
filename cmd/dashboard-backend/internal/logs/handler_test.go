package logs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/logagg"
)

func seedBuffer(t *testing.T) *logagg.RingBuffer {
	t.Helper()
	buf := logagg.NewRingBuffer(100)
	base := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)

	buf.Push(logagg.LogEntry{Timestamp: base, Service: "timeguru", Level: "info", Message: "started"})
	buf.Push(logagg.LogEntry{Timestamp: base.Add(time.Second), Service: "captain", Level: "error", Message: "connection failed"})
	buf.Push(logagg.LogEntry{Timestamp: base.Add(2 * time.Second), Service: "timeguru", Level: "warn", Message: "timeout approaching"})
	buf.Push(logagg.LogEntry{Timestamp: base.Add(3 * time.Second), Service: "architect", Level: "info", Message: "design complete"})
	return buf
}

func TestLogHandler_ReturnsJSONArray(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var entries []logagg.LogEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 4 {
		t.Errorf("got %d entries, want 4", len(entries))
	}
}

func TestLogHandler_ServiceFilter(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?service=timeguru", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var entries []logagg.LogEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
	for _, e := range entries {
		if e.Service != "timeguru" {
			t.Errorf("entry.Service = %q, want timeguru", e.Service)
		}
	}
}

func TestLogHandler_LevelFilter(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?level=error", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var entries []logagg.LogEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1", len(entries))
	}
}

func TestLogHandler_SearchFilter(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?search=connection", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var entries []logagg.LogEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1", len(entries))
	}
}

func TestLogHandler_Pagination(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?limit=2&offset=1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var entries []logagg.LogEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
	if entries[0].Service != "captain" {
		t.Errorf("first entry = %q, want captain (offset 1)", entries[0].Service)
	}
}

func TestLogHandler_EmptyResultsNotNull(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if body == "null\n" {
		t.Error("empty results should be [], not null")
	}
}

func TestLogHandler_MethodNotAllowed(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestParseTime_RFC3339(t *testing.T) {
	ts, err := parseTime("2026-02-24T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if ts.Year() != 2026 || ts.Month() != 2 || ts.Day() != 24 {
		t.Errorf("parsed time = %v", ts)
	}
}

func TestParseTime_Unix(t *testing.T) {
	ts, err := parseTime("1740400000")
	if err != nil {
		t.Fatal(err)
	}
	if ts.IsZero() {
		t.Error("parsed time should not be zero")
	}
}
