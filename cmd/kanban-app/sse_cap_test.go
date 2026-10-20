// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Open streams were bounded only by the per-address rate limit (120/min),
// and each holds a goroutine and a channel for as long as the client keeps
// it open. Past maxSSEClients a new stream is refused.
func TestHandleSSE_RefusesPastTheCap(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})
	server.sseMu.Lock()
	for i := 0; i < maxSSEClients; i++ {
		server.sseClients[make(chan []byte)] = true
	}
	server.sseMu.Unlock()

	w := httptest.NewRecorder()
	server.handleSSE(w, httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("stream past the cap: %d, want 503", w.Code)
	}
	server.sseMu.RLock()
	n := len(server.sseClients)
	server.sseMu.RUnlock()
	if n != maxSSEClients {
		t.Fatalf("clients = %d after a refused stream, want %d", n, maxSSEClients)
	}
}
