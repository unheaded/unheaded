// SPDX-License-Identifier: GPL-3.0-or-later
//
// Keystroke path (S5 Phase 4): browser WS → per-instance bounded queue →
// GET /api/v1/kbd/drain (loopback only) → `upc-bootctl console` → KBD ring.
//
// The bridge never touches BPF maps itself: the process that owns the loaded
// eBPF object (upc-bootctl) pulls bytes and appends them to the guest's KBD
// ring, respecting the ring's free space. Typing into the UPC shell is a
// write path, so the drain answers loopback callers only.

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"sync"

	"unheaded/pkg/metrics"
)

const (
	// kbdQueueCap bounds each instance's pending keystrokes.
	kbdQueueCap = 4096
	// drainDefaultMax is one KBD ring's worth (KBD_MAP has 64 slots).
	drainDefaultMax = 64
)

// kbdQueue holds typed bytes per UPC instance until the console drains them.
// Overflow drops the NEWEST bytes (what was typed first stays in order) and
// counts them in upc_tty_kbd_dropped_total.
type kbdQueue struct {
	mu      sync.Mutex
	cap     int
	q       map[uint8][]byte
	dropped *metrics.Counter
}

func newKbdQueue(capacity int) *kbdQueue {
	return &kbdQueue{
		cap: capacity,
		q:   make(map[uint8][]byte),
		dropped: metrics.NewCounter("upc_tty_kbd_dropped_total",
			"Keystroke bytes dropped because the per-instance queue was full", nil),
	}
}

// push appends as many bytes as fit; returns how many were kept.
func (k *kbdQueue) push(instance uint8, data []byte) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	cur := k.q[instance]
	free := k.cap - len(cur)
	if free < 0 {
		free = 0
	}
	n := len(data)
	if n > free {
		k.dropped.Add(float64(n - free))
		n = free
	}
	if n > 0 {
		k.q[instance] = append(cur, data[:n]...)
	}
	return n
}

// drain removes and returns up to max queued bytes (oldest first).
func (k *kbdQueue) drain(instance uint8, max int) []byte {
	if max <= 0 {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	cur := k.q[instance]
	if len(cur) == 0 {
		return nil
	}
	if max > len(cur) {
		max = len(cur)
	}
	out := make([]byte, max)
	copy(out, cur[:max])
	rest := cur[max:]
	if len(rest) == 0 {
		delete(k.q, instance)
	} else {
		k.q[instance] = append([]byte(nil), rest...)
	}
	return out
}

// parseDrainMax bounds ?max= to [1, kbdQueueCap]; anything unusable → 64.
func parseDrainMax(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return drainDefaultMax
	}
	if n > kbdQueueCap {
		return kbdQueueCap
	}
	return n
}

// isLoopbackRemote reports whether an http.Request.RemoteAddr is a loopback
// peer. Anything unparseable is not.
func isLoopbackRemote(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// handleKbdDrain: GET /api/v1/kbd/drain?instance=N&max=K → JSON array of up
// to K queued byte values (same number-array shape as /api/v1/tty/ingest).
// 405 on non-GET, 403 for non-loopback callers.
func (h *Hub) handleKbdDrain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRemote(r.RemoteAddr) {
		http.Error(w, "loopback only", http.StatusForbidden)
		return
	}
	instance := parseInstanceParam(r.URL.Query().Get("instance"))
	data := h.kbd.drain(instance, parseDrainMax(r.URL.Query().Get("max")))
	out := make([]int, len(data))
	for i, b := range data {
		out[i] = int(b)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// handleMetrics exposes the bridge's counters in Prometheus text format.
func (h *Hub) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_ = h.kbd.dropped.Write(w)
}
