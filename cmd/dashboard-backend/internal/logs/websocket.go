// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logs

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"unheaded/pkg/logagg"
)

// LogStream provides a WebSocket-like endpoint for live log tailing.
// Uses Server-Sent Events (SSE) for broad compatibility without
// requiring a WebSocket library dependency.
type LogStream struct {
	buffer    *logagg.RingBuffer
	mu        sync.RWMutex
	listeners map[chan logagg.LogEntry]logStreamFilter
}

type logStreamFilter struct {
	service string
	level   string
}

// NewLogStream creates a new log stream for live tailing.
func NewLogStream(buf *logagg.RingBuffer) *LogStream {
	return &LogStream{
		buffer:    buf,
		listeners: make(map[chan logagg.LogEntry]logStreamFilter),
	}
}

// Broadcast sends a log entry to all connected SSE listeners matching their filters.
func (ls *LogStream) Broadcast(entry logagg.LogEntry) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	for ch, filter := range ls.listeners {
		if filter.service != "" && filter.service != entry.Service {
			continue
		}
		if filter.level != "" && filter.level != entry.Level {
			continue
		}
		select {
		case ch <- entry:
		default:
			// drop if listener is slow
		}
	}
}

// ServeHTTP handles the /ws/logs SSE endpoint.
// Query params: service, level (optional filters).
func (ls *LogStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	filter := logStreamFilter{
		service: r.URL.Query().Get("service"),
		level:   r.URL.Query().Get("level"),
	}

	ch := make(chan logagg.LogEntry, 100)

	ls.mu.Lock()
	ls.listeners[ch] = filter
	ls.mu.Unlock()

	defer func() {
		ls.mu.Lock()
		delete(ls.listeners, ch)
		ls.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// No Access-Control-Allow-Origin: the dashboard reads this stream from
	// its own origin (/ws/logs). A wildcard let any web page in an
	// operator's browser read every service's live logs.
	flusher.Flush()

	ctx := r.Context()

	// Live only: no replay on connect. The page loads history itself
	// (GET /api/v1/logs?newest=true). The stream used to start with "the last
	// 50", which were really the oldest 50 in the buffer, and the page then
	// showed them twice.

	// Stream new entries
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(entry)
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-time.After(30 * time.Second):
			// keepalive
			_, _ = w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		}
	}
}

// ListenerCount returns the number of active SSE listeners.
func (ls *LogStream) ListenerCount() int {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return len(ls.listeners)
}

// StartBroadcastLoop polls the ring buffer for new entries and broadcasts them.
// This is a simple approach — in production, the subscriber would call Broadcast directly.
func (ls *LogStream) StartBroadcastLoop(ctx context.Context, buf *logagg.RingBuffer) {
	// By sequence, not Len: Len stops growing once the ring is full, and the
	// live tail used to go silent after the first 10,000 entries.
	seq := buf.Seq()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var entries []logagg.LogEntry
			entries, seq = buf.Since(seq)
			for _, entry := range entries {
				ls.Broadcast(entry)
			}
		}
	}
}
