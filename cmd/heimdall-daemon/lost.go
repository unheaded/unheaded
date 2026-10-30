// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import "sort"

// lost.go — LICH-012 L12d (2026-10-30). When a hook's ringbuf is full the
// kernel side drops the event (vfs_write, execve: LOST map), and a slow
// daemon drops decoded records (reader). Both used to vanish silently. Each
// scan reports what was lost since the last one: one log line per hook and
// heimdall_hook_events_lost_total{hook}. File changes in that window are
// still caught by the next full poll; only their event-driven timing is lost.

// reportLost logs and counts the lost-event deltas since the previous call.
func (h *Heimdall) reportLost(logf func(string, ...any)) {
	if h.hookLost == nil {
		return
	}
	cur := h.hookLost()
	if h.lostPrev == nil {
		h.lostPrev = map[string]uint64{}
	}
	names := make([]string, 0, len(cur))
	for k := range cur {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		n, prev := cur[k], h.lostPrev[k]
		d := n - prev
		if n < prev { // counters reset (hooks reloaded)
			d = n
		}
		h.lostPrev[k] = n
		if d == 0 {
			continue
		}
		why := "ringbuf full"
		if k == "reader" {
			why = "daemon fell behind"
		}
		logf("[BPF] %s: %d events lost since the last scan (%s); the next poll still sees those files", k, d, why)
		if h.metrics != nil {
			h.metrics.lost.WithLabels(map[string]string{"hook": k}).Add(float64(d))
		}
	}
}
