// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package logs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/logagg"
)

// ---------------------------------------------------------------------------
// handler.go — parseTime edge cases
// ---------------------------------------------------------------------------

func TestParseTime_InvalidString(t *testing.T) {
	_, err := parseTime("not-a-time")
	if err == nil {
		t.Fatal("expected error for invalid time string")
	}
}

func TestParseTime_EmptyString(t *testing.T) {
	_, err := parseTime("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

// ---------------------------------------------------------------------------
// handler.go — from/to query params
// ---------------------------------------------------------------------------

func TestLogHandler_FromFilter_RFC3339(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	// "from" after the first two entries
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?from=2026-02-24T12:00:02Z", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var entries []logagg.LogEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Only entries at +2s and +3s should match.
	if len(entries) < 1 {
		t.Fatalf("got %d entries, want >=1", len(entries))
	}
	for _, e := range entries {
		if e.Timestamp.Before(time.Date(2026, 2, 24, 12, 0, 2, 0, time.UTC)) {
			t.Errorf("entry timestamp %v is before from filter", e.Timestamp)
		}
	}
}

func TestLogHandler_ToFilter_RFC3339(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	// "to" before the last two entries
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?to=2026-02-24T12:00:01Z", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var entries []logagg.LogEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, e := range entries {
		if e.Timestamp.After(time.Date(2026, 2, 24, 12, 0, 1, 0, time.UTC)) {
			t.Errorf("entry timestamp %v is after to filter", e.Timestamp)
		}
	}
}

func TestLogHandler_FromFilter_Unix(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	from := time.Date(2026, 2, 24, 12, 0, 2, 0, time.UTC).Unix()
	url := fmt.Sprintf("/api/v1/logs?from=%d", from)

	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var entries []logagg.LogEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	for _, e := range entries {
		if e.Timestamp.Unix() < from {
			t.Errorf("entry timestamp %v before from unix %d", e.Timestamp, from)
		}
	}
}

func TestLogHandler_InvalidFromIgnored(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?from=bogus", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var entries []logagg.LogEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	// Invalid from is silently ignored, so all 4 entries returned.
	if len(entries) != 4 {
		t.Errorf("got %d entries, want 4 (invalid from ignored)", len(entries))
	}
}

func TestLogHandler_InvalidToIgnored(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?to=bogus", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var entries []logagg.LogEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 4 {
		t.Errorf("got %d entries, want 4 (invalid to ignored)", len(entries))
	}
}

func TestLogHandler_InvalidLimitIgnored(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?limit=abc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestLogHandler_NegativeLimitIgnored(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?limit=-5", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestLogHandler_InvalidOffsetIgnored(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?offset=xyz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestLogHandler_NegativeOffsetIgnored(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?offset=-1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestLogHandler_CombinedFilters(t *testing.T) {
	buf := seedBuffer(t)
	h := NewLogHandler(buf)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?service=timeguru&level=warn&limit=10&offset=0", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var entries []logagg.LogEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1", len(entries))
	}
	if len(entries) > 0 && entries[0].Message != "timeout approaching" {
		t.Errorf("message = %q, want 'timeout approaching'", entries[0].Message)
	}
}

func TestLogHandler_MethodPUT(t *testing.T) {
	h := NewLogHandler(logagg.NewRingBuffer(10))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/logs", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestLogHandler_MethodDELETE(t *testing.T) {
	h := NewLogHandler(logagg.NewRingBuffer(10))
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/logs", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// websocket.go — LogStream.ServeHTTP (SSE endpoint)
// ---------------------------------------------------------------------------

// nonFlusherWriter is an http.ResponseWriter that does NOT implement
// http.Flusher, used to test the "streaming not supported" code path.
type nonFlusherWriter struct {
	code   int
	header http.Header
	body   strings.Builder
}

func newNonFlusherWriter() *nonFlusherWriter {
	return &nonFlusherWriter{header: make(http.Header)}
}

func (w *nonFlusherWriter) Header() http.Header         { return w.header }
func (w *nonFlusherWriter) WriteHeader(code int)         { w.code = code }
func (w *nonFlusherWriter) Write(b []byte) (int, error)  { return w.body.Write(b) }

func TestLogStream_ServeHTTP_NoFlusherSupport(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	req := httptest.NewRequest(http.MethodGet, "/ws/logs", nil)
	w := newNonFlusherWriter()
	ls.ServeHTTP(w, req)

	if w.code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.code, http.StatusInternalServerError)
	}
	if !strings.Contains(w.body.String(), "streaming not supported") {
		t.Errorf("body = %q, want 'streaming not supported'", w.body.String())
	}
}

func TestLogStream_ServeHTTP_SSEHeaders(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/ws/logs", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		ls.ServeHTTP(w, req)
		close(done)
	}()

	// Give it a moment to set headers and enter the loop.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	if co := w.Header().Get("Connection"); co != "keep-alive" {
		t.Errorf("Connection = %q, want keep-alive", co)
	}
	if ac := w.Header().Get("Access-Control-Allow-Origin"); ac != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", ac)
	}
}

func TestLogStream_ServeHTTP_SendsRecentEntries(t *testing.T) {
	buf := seedBuffer(t)
	ls := NewLogStream(buf)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/ws/logs", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		ls.ServeHTTP(w, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	// Should contain SSE "data: " lines for recent entries.
	count := strings.Count(body, "data: ")
	if count != 4 {
		t.Errorf("SSE data lines = %d, want 4 (recent entries)", count)
	}

	// Verify each line is valid JSON.
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")
		var entry logagg.LogEntry
		if err := json.Unmarshal([]byte(jsonStr), &entry); err != nil {
			t.Errorf("invalid JSON in SSE line: %v — %q", err, jsonStr)
		}
	}
}

func TestLogStream_ServeHTTP_WithServiceFilter(t *testing.T) {
	buf := seedBuffer(t) // 2 timeguru, 1 captain, 1 architect
	ls := NewLogStream(buf)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet,
		"/ws/logs?service=captain", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		ls.ServeHTTP(w, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	count := strings.Count(w.Body.String(), "data: ")
	if count != 1 {
		t.Errorf("SSE data lines = %d, want 1 (captain only)", count)
	}
}

func TestLogStream_ServeHTTP_WithLevelFilter(t *testing.T) {
	buf := seedBuffer(t)
	ls := NewLogStream(buf)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet,
		"/ws/logs?level=error", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		ls.ServeHTTP(w, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	count := strings.Count(w.Body.String(), "data: ")
	if count != 1 {
		t.Errorf("SSE data lines = %d, want 1 (error level only)", count)
	}
}

func TestLogStream_ServeHTTP_StreamsNewEntries(t *testing.T) {
	buf := logagg.NewRingBuffer(100) // empty buffer
	ls := NewLogStream(buf)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/ws/logs", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		ls.ServeHTTP(w, req)
		close(done)
	}()

	// Wait for listener to register.
	time.Sleep(50 * time.Millisecond)

	if ls.ListenerCount() != 1 {
		t.Fatalf("listener count = %d, want 1", ls.ListenerCount())
	}

	// Broadcast an entry — it should arrive in the SSE stream.
	ls.Broadcast(logagg.LogEntry{
		Timestamp: time.Now(),
		Service:   "test-svc",
		Level:     "info",
		Message:   "streamed-entry",
	})

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if !strings.Contains(w.Body.String(), "streamed-entry") {
		t.Errorf("body does not contain streamed entry: %s", w.Body.String())
	}
}

func TestLogStream_ServeHTTP_ListenerCleanup(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/ws/logs", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		ls.ServeHTTP(w, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	if ls.ListenerCount() != 1 {
		t.Fatalf("listener count = %d, want 1 while connected", ls.ListenerCount())
	}

	cancel()
	<-done

	if ls.ListenerCount() != 0 {
		t.Errorf("listener count = %d, want 0 after disconnect", ls.ListenerCount())
	}
}

// ---------------------------------------------------------------------------
// websocket.go — LogStream.Broadcast with level filter
// ---------------------------------------------------------------------------

func TestLogStream_BroadcastWithLevelFilter(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	ch := make(chan logagg.LogEntry, 10)
	ls.mu.Lock()
	ls.listeners[ch] = logStreamFilter{level: "error"}
	ls.mu.Unlock()

	// Should not match (info != error).
	ls.Broadcast(logagg.LogEntry{Service: "a", Level: "info", Message: "skip"})
	// Should match.
	ls.Broadcast(logagg.LogEntry{Service: "b", Level: "error", Message: "match"})

	select {
	case received := <-ch:
		if received.Message != "match" {
			t.Errorf("message = %q, want 'match'", received.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}

	select {
	case extra := <-ch:
		t.Errorf("unexpected extra message: %q", extra.Message)
	default:
	}
}

func TestLogStream_BroadcastWithBothFilters(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	ch := make(chan logagg.LogEntry, 10)
	ls.mu.Lock()
	ls.listeners[ch] = logStreamFilter{service: "captain", level: "error"}
	ls.mu.Unlock()

	// Wrong service.
	ls.Broadcast(logagg.LogEntry{Service: "timeguru", Level: "error", Message: "no"})
	// Wrong level.
	ls.Broadcast(logagg.LogEntry{Service: "captain", Level: "info", Message: "no"})
	// Both match.
	ls.Broadcast(logagg.LogEntry{Service: "captain", Level: "error", Message: "yes"})

	select {
	case received := <-ch:
		if received.Message != "yes" {
			t.Errorf("message = %q, want 'yes'", received.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}

	select {
	case extra := <-ch:
		t.Errorf("unexpected extra message: %q", extra.Message)
	default:
	}
}

// ---------------------------------------------------------------------------
// websocket.go — StartBroadcastLoop
// ---------------------------------------------------------------------------

func TestLogStream_StartBroadcastLoop(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	// Register a listener.
	ch := make(chan logagg.LogEntry, 50)
	ls.mu.Lock()
	ls.listeners[ch] = logStreamFilter{}
	ls.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	go ls.StartBroadcastLoop(ctx, buf)

	// Push entries AFTER the loop starts — the loop should detect and broadcast them.
	time.Sleep(50 * time.Millisecond)
	buf.Push(logagg.LogEntry{Service: "s1", Level: "info", Message: "loop-msg-1"})
	buf.Push(logagg.LogEntry{Service: "s2", Level: "warn", Message: "loop-msg-2"})

	// Wait for the loop to pick them up (polls every 100ms).
	time.Sleep(250 * time.Millisecond)
	cancel()

	var received []logagg.LogEntry
	for {
		select {
		case e := <-ch:
			received = append(received, e)
		default:
			goto done
		}
	}
done:
	if len(received) < 2 {
		t.Fatalf("received %d entries, want >=2", len(received))
	}
	if received[0].Message != "loop-msg-1" {
		t.Errorf("first message = %q, want 'loop-msg-1'", received[0].Message)
	}
	if received[1].Message != "loop-msg-2" {
		t.Errorf("second message = %q, want 'loop-msg-2'", received[1].Message)
	}
}

func TestLogStream_StartBroadcastLoop_ContextCancel(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ls.StartBroadcastLoop(ctx, buf)
		close(done)
	}()

	// Cancel immediately — loop should exit.
	cancel()

	select {
	case <-done:
		// Good — loop exited.
	case <-time.After(2 * time.Second):
		t.Fatal("StartBroadcastLoop did not exit after context cancel")
	}
}

func TestLogStream_StartBroadcastLoop_NoNewEntries(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	// Pre-populate so lastLen starts > 0.
	buf.Push(logagg.LogEntry{Service: "pre", Level: "info", Message: "existing"})

	ls := NewLogStream(buf)

	ch := make(chan logagg.LogEntry, 10)
	ls.mu.Lock()
	ls.listeners[ch] = logStreamFilter{}
	ls.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	go ls.StartBroadcastLoop(ctx, buf)

	// Wait a couple poll cycles with no new entries.
	time.Sleep(250 * time.Millisecond)
	cancel()

	select {
	case e := <-ch:
		t.Errorf("unexpected broadcast: %q", e.Message)
	default:
		// Expected — no new entries, no broadcasts.
	}
}
